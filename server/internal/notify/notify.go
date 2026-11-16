// Package notify tells people what happened: it turns changes (a mention,
// an assignment, a comment, an accepted invite) into notifications for
// the people concerned, keeps the in-app list, and queues delivery by
// email (and push) as each person's preferences say. Nobody is told about
// their own actions, or about a task they cannot see.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/reports"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Job kinds this package handles.
const (
	jobChange = "notify.change" // a synced change that may notify people
	jobEmail  = "notify.email"  // email one notification
	jobPush   = "notify.push"   // push one notification
	jobMail   = "notify.mail"   // send a prepared message (the digest)
	jobWeekly = "notify.weekly" // write one person's weekly report
	// copy a repeating task's reminders to the task that follows it
	jobCopyReminders = "notify.copy_reminders"
)

// Tasks is what notifications need from sync.
type Tasks interface {
	Task(ctx context.Context, taskID string) (syncer.TaskBrief, bool, error)
	Thread(ctx context.Context, threadID string) (syncer.ThreadBrief, bool, error)
	ThreadPeople(ctx context.Context, threadID string) ([]string, error)
	SeesProject(ctx context.Context, userID, workspaceID, projectID string) (bool, error)
	DueTasks(ctx context.Context, userID, through string) ([]syncer.TaskBrief, error)
	DueBetween(ctx context.Context, from, through string) ([]syncer.TaskBrief, error)
	Reminders(ctx context.Context, from, to time.Time, dueFrom string) ([]syncer.ReminderBrief, error)
	CopyReminders(ctx context.Context, taskID string) (int, error)
	Waiting(ctx context.Context, taskID string) ([]syncer.TaskBrief, error)
	Blockers(ctx context.Context, taskID string) ([]syncer.TaskBrief, error)
	LoggedOn(ctx context.Context, userID, day string) ([]syncer.TimeBrief, error)
	OverEstimate(ctx context.Context, userID string) ([]syncer.TimeBrief, error)
	WeeklyInput(ctx context.Context, userID, workspaceID string) (reports.Input, error)
}

// Poker tells a user's open apps that they have something new.
type Poker interface {
	Notify(userIDs ...string)
}

// PushMessage is a notification as a web push carries it.
type PushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	// Urgent pushes (reminders, due tasks) ask to be delivered at once.
	Urgent bool `json:"-"`
}

// Pusher delivers web pushes to a user's devices.
type Pusher interface {
	Push(ctx context.Context, userID string, m PushMessage) error
}

// Config is what the service needs from the server's settings.
type Config struct {
	BaseURL string // public URL, for links in emails
	// Email is true when email is set up; otherwise messages go to the
	// server log, which settings tell people.
	Email bool
}

// Service creates and delivers notifications.
type Service struct {
	st      *store.Store
	log     *slog.Logger
	jobs    *jobs.Runner
	mail    mail.Sender
	tasks   Tasks
	poker   Poker
	pusher  Pusher
	baseURL string
	email   bool
}

// mailWorkers is how many emails of each kind go out at once.
const mailWorkers = 2

// New builds the service and registers its jobs and the digest with the
// runner.
func New(st *store.Store, log *slog.Logger, runner *jobs.Runner, sender mail.Sender, tasks Tasks, poker Poker, cfg Config) *Service {
	s := &Service{st: st, log: log, jobs: runner, mail: sender, tasks: tasks, poker: poker,
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"), email: cfg.Email}
	runner.Handle(jobChange, s.handleChange)
	// Mail waits on the mail server, which may not answer for a while;
	// in a lane of its own it holds up nothing else.
	runner.HandleConcurrent(jobEmail, mailWorkers, s.handleEmail)
	runner.HandleConcurrent(jobMail, mailWorkers, s.handleMail)
	// A weekly report reads all a person sees of a workspace: one at a
	// time, beside the rest, so a Monday morning holds up no reminder or
	// notification.
	runner.HandleConcurrent(jobWeekly, 1, s.handleWeekly)
	runner.Handle(jobCopyReminders, s.handleCopyReminders)
	runner.Every("digest", time.Minute, s.SendDigests)
	runner.Every("weekly", time.Minute, s.SendWeekly)
	runner.Every("reminders", jobs.Poll, s.SendTimed)
	return s
}

// QueueMail queues a prepared email, which the mail lane sends with
// retries: messages from elsewhere in the server, such as a failed
// backup's.
func QueueMail(ctx context.Context, q store.Queries, m mail.Message, now time.Time) error {
	return jobs.Enqueue(ctx, q, jobMail, m, now, now)
}

// CanEmail reports whether email is set up on this instance.
func (s *Service) CanEmail() bool { return s.email }

// CanPush reports whether web push is available on this instance.
func (s *Service) CanPush() bool { return s.pusher != nil }

// SetPusher turns on delivery by web push.
func (s *Service) SetPusher(p Pusher) {
	s.pusher = p
	s.jobs.Handle(jobPush, s.handlePush)
}

func (s *Service) now() time.Time { return s.jobs.Now() }

// poke tells users' open apps to reload their notifications, so a read
// on one device clears the count on the others too.
func (s *Service) poke(userIDs ...string) {
	if len(userIDs) > 0 && s.poker != nil {
		s.poker.Notify(userIDs...)
	}
}

// Link is the address of a task in the app, which also switches to its
// workspace. With no task it opens the workspace.
func (s *Service) Link(workspaceID, taskID string) string {
	return s.baseURL + Path(workspaceID, taskID)
}

// Path is Link without the host.
func Path(workspaceID, taskID string) string {
	q := url.Values{}
	if taskID != "" {
		q.Set("task", taskID)
	}
	q.Set("ws", workspaceID)
	// Encode sorts the keys, which puts task first.
	return "/?" + q.Encode()
}

// NoticePath is Path for opening a notification from outside the app (a
// push, an email): the app marks it read as it opens.
func NoticePath(workspaceID, taskID, notificationID string) string {
	return Path(workspaceID, taskID) + "&n=" + url.QueryEscape(notificationID)
}

// ThreadPath is where a thread on a wiki page opens in the app.
func ThreadPath(t syncer.ThreadBrief, workspaceID string) string {
	q := url.Values{}
	q.Set("page", t.PageID)
	q.Set("thread", t.ID)
	q.Set("ws", workspaceID)
	if t.ProjectID == "" {
		return "/wiki?" + q.Encode()
	}
	q.Set("tab", "wiki")
	return "/projects/" + url.PathEscape(t.ProjectID) + "?" + q.Encode()
}

// noticePath is NoticePath for any notification: one about a thread leads to
// the thread, and all the others as NoticePath says.
func (s *Service) noticePath(ctx context.Context, n sqlitedb.Notification) (string, error) {
	if n.PageThreadID.Valid {
		t, ok, err := s.tasks.Thread(ctx, n.PageThreadID.String)
		if err != nil {
			return "", err
		}
		if ok {
			return ThreadPath(t, n.WorkspaceID) + "&n=" + url.QueryEscape(n.ID), nil
		}
	}
	return NoticePath(n.WorkspaceID, n.TaskID.String, n.ID), nil
}

// noticeLink is noticePath with the host.
func (s *Service) noticeLink(ctx context.Context, n sqlitedb.Notification) (string, error) {
	path, err := s.noticePath(ctx, n)
	return s.baseURL + path, err
}

// Notice is a notification to create.
type Notice struct {
	UserID      string
	WorkspaceID string
	Kind        Kind
	Task        *syncer.TaskBrief   // nil when not about a task
	Thread      *syncer.ThreadBrief // nil when not about a thread on a wiki page
	ActorID     string
	Title       string
	Body        string
	// Batch is the change to many tasks it came from, if any: its pushes
	// are folded into one (pushBatch).
	Batch string
}

// foldWait is how long the push of an assignment from a change to many waits,
// so that the rest of the change arrives and goes out with it as one.
const foldWait = 5 * time.Second

// folds says whether a notification's push is folded with the rest of its
// batch: an assignment from a change to many tasks.
func folds(n Notice) bool { return n.Batch != "" && n.Kind == Assigned }

// planned is a notification checked and ready to insert.
type planned struct {
	Notice
	channels Channels
}

// plan decides whether and where a notification goes: never to the actor,
// never to someone who cannot see its task, and only on the channels the
// person chose.
func (s *Service) plan(ctx context.Context, n Notice) (*planned, error) {
	if n.UserID == "" || n.UserID == n.ActorID {
		return nil, nil
	}
	if n.Thread != nil {
		if sees, err := s.seesThread(ctx, n.UserID, n.WorkspaceID, *n.Thread); err != nil || !sees {
			return nil, err
		}
	} else if sees, err := s.sees(ctx, n.UserID, n.WorkspaceID, n.Task); err != nil || !sees {
		return nil, err
	}
	prefs, _, err := s.Prefs(ctx, n.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := prefs.Kinds[n.Kind]
	pushes := c.Push && s.pusher != nil
	if !c.App && !c.Email && !pushes {
		return nil, nil
	}
	return &planned{Notice: n, channels: c}, nil
}

// sees reports whether a user may hear about a task right now (or, with
// no task, about the workspace): they are a member who can see the task's
// project, and the task is not deleted.
func (s *Service) sees(ctx context.Context, userID, workspaceID string, task *syncer.TaskBrief) (bool, error) {
	if task != nil {
		if task.Deleted {
			return false, nil
		}
		return s.tasks.SeesProject(ctx, userID, workspaceID, task.ProjectID)
	}
	_, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// seesThread is sees for a thread on a wiki page.
func (s *Service) seesThread(ctx context.Context, userID, workspaceID string, t syncer.ThreadBrief) (bool, error) {
	if t.Deleted {
		return false, nil
	}
	return s.tasks.SeesProject(ctx, userID, workspaceID, t.ProjectID)
}

// stillSees is sees for a notification about to be delivered, with its
// task as it is now: access lost since it was made stops the delivery.
// The task is nil for a notification about no task.
func (s *Service) stillSees(ctx context.Context, n sqlitedb.Notification) (*syncer.TaskBrief, bool, error) {
	if n.PageThreadID.Valid {
		t, ok, err := s.tasks.Thread(ctx, n.PageThreadID.String)
		if err != nil || !ok {
			return nil, false, err
		}
		sees, err := s.seesThread(ctx, n.UserID, n.WorkspaceID, t)
		return nil, sees, err
	}
	var task *syncer.TaskBrief
	if n.TaskID.Valid {
		t, ok, err := s.tasks.Task(ctx, n.TaskID.String)
		if err != nil || !ok {
			return nil, false, err
		}
		task = &t
	}
	sees, err := s.sees(ctx, n.UserID, n.WorkspaceID, task)
	return task, sees, err
}

// insert writes a planned notification and queues its deliveries, inside
// the caller's transaction.
func (s *Service) insert(ctx context.Context, q store.Queries, p *planned) error {
	now := s.now()
	id := uuid.Must(uuid.NewV7()).String()
	params := sqlitedb.InsertNotificationParams{
		ID: id, UserID: p.UserID, WorkspaceID: p.WorkspaceID, Kind: string(p.Kind),
		Title: p.Title, Body: p.Body, InApp: p.channels.App, Now: now,
	}
	if p.Task != nil {
		params.TaskID = sql.NullString{String: p.Task.ID, Valid: true}
	}
	if p.Thread != nil {
		params.PageThreadID = sql.NullString{String: p.Thread.ID, Valid: true}
	}
	if p.ActorID != "" {
		params.ActorID = sql.NullString{String: p.ActorID, Valid: true}
	}
	if folds(p.Notice) {
		params.Batch = sql.NullString{String: p.Batch, Valid: true}
	}
	if err := q.InsertNotification(ctx, params); err != nil {
		return err
	}
	ref := struct {
		ID string `json:"id"`
	}{id}
	if p.channels.Email {
		if err := jobs.Enqueue(ctx, q, jobEmail, ref, now, now); err != nil {
			return err
		}
	}
	if p.channels.Push && s.pusher != nil {
		// A change to many waits a moment, for the rest of it to arrive.
		at := now
		if folds(p.Notice) {
			at = now.Add(foldWait)
		}
		if err := jobs.Enqueue(ctx, q, jobPush, ref, at, now); err != nil {
			return err
		}
	}
	return nil
}

// Send creates notifications, skipping anyone who should not get them, in
// one transaction, then tells the open apps and the runner.
func (s *Service) Send(ctx context.Context, list ...Notice) error {
	var ready []*planned
	for _, n := range list {
		p, err := s.plan(ctx, n)
		if err != nil {
			return err
		}
		if p != nil {
			ready = append(ready, p)
		}
	}
	if len(ready) == 0 {
		return nil
	}
	err := s.st.InTx(ctx, func(q store.Queries) error {
		for _, p := range ready {
			if err := s.insert(ctx, q, p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	var poke []string
	for _, p := range ready {
		if p.channels.App {
			poke = append(poke, p.UserID)
		}
	}
	s.poke(poke...)
	s.jobs.Wake()
	return nil
}

// change is the payload of a notify.change job: one synced change that
// may concern people.
type change struct {
	Kind        string `json:"kind"` // assigned, comment, comment_edited or completed
	WorkspaceID string `json:"workspaceId"`
	ActorID     string `json:"actorId"`
	TaskID      string `json:"taskId"`
	ThreadID    string `json:"threadId,omitempty"`
	UserID      string `json:"userId,omitempty"`   // assigned: who
	Body        string `json:"body,omitempty"`     // comments: the text
	Previous    string `json:"previous,omitempty"` // edits: the text before
	At          string `json:"at"`                 // when sync applied the change
	Made        string `json:"made,omitempty"`     // when it was made, by its clock
	Batch       string `json:"batch,omitempty"`    // the change to many it is part of
}

// Applied queues work for an accepted op that may concern people. It runs
// inside the op's transaction and only writes a job, so delivering never
// slows or fails a push.
func (s *Service) Applied(ctx context.Context, q store.Queries, a syncer.Applied) error {
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	c := change{WorkspaceID: a.WorkspaceID, ActorID: a.ActorID, TaskID: str(a.Values, "task_id")}
	// Removed or restored: what people's lists show about it has changed.
	if (a.Entity == "task" || a.Entity == "project" || a.Entity == "page") && !a.Created && slices.Contains(a.Accepted, "deleted_at") &&
		(a.Before["deleted_at"] == nil) != (a.Values["deleted_at"] == nil) {
		now := s.now()
		return jobs.Enqueue(ctx, q, jobChange, change{Kind: kindRemoved, TaskID: a.ID}, now, now)
	}
	switch a.Entity {
	case "task":
		// The next task of a repeating series: everyone's reminders follow.
		if series := str(a.Values, "series_id"); a.Created && series != "" && series != a.ID {
			now := s.now()
			return jobs.Enqueue(ctx, q, jobCopyReminders, ref{ID: a.ID}, now, now)
		}
		// Done: the tasks waiting for it may start.
		finished := slices.Contains(a.Accepted, "completed_at") && a.Values["completed_at"] != nil && a.Before["completed_at"] == nil
		if a.Created || !finished || a.Values["deleted_at"] != nil {
			return nil
		}
		c.Kind, c.TaskID = "completed", a.ID
	case "task_assignee":
		live := a.Values["deleted_at"] == nil
		revived := !a.Created && a.Before["deleted_at"] != nil && slices.Contains(a.Accepted, "deleted_at")
		added := a.Created || revived
		if !live || !added || str(a.Values, "user_id") == a.ActorID {
			return nil
		}
		c.Kind, c.UserID, c.Batch = "assigned", str(a.Values, "user_id"), a.Batch
	case "comment", "page_comment":
		// A comment one of many made at once brought in (an import's) tells
		// nobody: they were said elsewhere, before.
		if a.Values["deleted_at"] != nil || (a.Bulk && a.Created) {
			return nil
		}
		c.Body = str(a.Values, "body")
		c.ThreadID = str(a.Values, "thread_id")
		switch {
		case a.Created:
			c.Kind = "comment"
		case slices.Contains(a.Accepted, "body"):
			c.Kind, c.Previous = "comment_edited", str(a.Before, "body")
			if len(newMentions(c.Body, c.Previous)) == 0 {
				return nil
			}
		default:
			return nil
		}
	default:
		return nil
	}
	c.At = a.At.Format(time.RFC3339Nano)
	if !a.Clock.IsZero() {
		c.Made = a.Clock.Format(time.RFC3339Nano)
	}
	now := s.now()
	return jobs.Enqueue(ctx, q, jobChange, c, now, now)
}

// Committed runs the queued work promptly.
func (s *Service) Committed() { s.jobs.Wake() }

// kindRemoved is a change saying a task or project, named in TaskID, was
// removed or restored. It reaches no one's list, only their open apps.
const kindRemoved = "removed"

// recentCopy is how soon after a repeating task's next copy is made that
// assigning it is part of making it.
const recentCopy = time.Minute

func (s *Service) handleChange(ctx context.Context, payload []byte) error {
	var c change
	if err := json.Unmarshal(payload, &c); err != nil {
		return err
	}
	if c.Kind == kindRemoved {
		users, err := s.st.ListUsersNotifiedAbout(ctx, c.TaskID)
		if err == nil {
			s.poke(users...)
		}
		return err
	}
	if c.ThreadID != "" {
		return s.threadChanged(ctx, c)
	}
	task, ok, err := s.tasks.Task(ctx, c.TaskID)
	if err != nil || !ok || task.Deleted {
		return err
	}
	actor := s.nameOf(ctx, c.ActorID)
	if c.Kind == "completed" {
		return s.unblocked(ctx, c, task, actor)
	}
	base := Notice{WorkspaceID: c.WorkspaceID, Task: &task, ActorID: c.ActorID}
	var out []Notice
	switch c.Kind {
	case "assigned":
		if !slices.Contains(task.Assignees, c.UserID) {
			return nil // unassigned again since
		}
		// The next task of a series comes with its assignees copied; they
		// were assigned to the series already and need not hear it again.
		// That holds when it comes back too (completed again after an
		// undo), long after it was first made: the assignments are made
		// just after the task's return, by the same device's clock.
		at, _ := time.Parse(time.RFC3339Nano, c.At)
		made, _ := time.Parse(time.RFC3339Nano, c.Made)
		back := !task.LiveSince.IsZero() && !made.Before(task.LiveSince) && made.Sub(task.LiveSince) < recentCopy
		if task.SeriesID != "" && task.SeriesID != task.ID && (at.Sub(task.CreatedAt) < recentCopy || back) {
			return nil
		}
		n := base
		n.UserID, n.Kind, n.Title = c.UserID, Assigned, actor+" assigned you to "+task.Title
		n.Body = where(task)
		n.Batch = c.Batch
		out = append(out, n)
	case "comment", "comment_edited":
		mentioned := newMentions(c.Body, c.Previous)
		excerpt := Excerpt(c.Body, 200)
		for _, id := range mentioned {
			n := base
			n.UserID, n.Kind, n.Title, n.Body = id, Mention, actor+" mentioned you in "+task.Title, excerpt
			out = append(out, n)
		}
		if c.Kind == "comment" {
			concerned := append([]string{task.CreatedBy}, task.Assignees...)
			seen := map[string]bool{}
			for _, id := range concerned {
				if id == "" || seen[id] || slices.Contains(mentioned, id) {
					continue
				}
				seen[id] = true
				n := base
				n.UserID, n.Kind, n.Title, n.Body = id, Comment, actor+" commented on "+task.Title, excerpt
				out = append(out, n)
			}
		}
	}
	return s.Send(ctx, out...)
}

// threadChanged tells those mentioned in a comment on a wiki page and, for a
// new comment, those already in its thread. plan keeps out anyone who cannot
// see the page.
func (s *Service) threadChanged(ctx context.Context, c change) error {
	thread, ok, err := s.tasks.Thread(ctx, c.ThreadID)
	if err != nil || !ok || thread.Deleted {
		return err
	}
	actor := s.nameOf(ctx, c.ActorID)
	base := Notice{WorkspaceID: c.WorkspaceID, Thread: &thread, ActorID: c.ActorID}
	mentioned := newMentions(c.Body, c.Previous)
	excerpt := Excerpt(c.Body, 200)
	var out []Notice
	for _, id := range mentioned {
		n := base
		n.UserID, n.Kind, n.Title, n.Body = id, Mention, actor+" mentioned you in "+thread.PageTitle, excerpt
		out = append(out, n)
	}
	if c.Kind == "comment" {
		people, err := s.tasks.ThreadPeople(ctx, thread.ID)
		if err != nil {
			return err
		}
		for _, id := range people {
			if slices.Contains(mentioned, id) {
				continue
			}
			n := base
			n.UserID, n.Kind, n.Title, n.Body = id, PageComment, actor+" commented on "+thread.PageTitle, excerpt
			out = append(out, n)
		}
	}
	return s.Send(ctx, out...)
}

// unblocked tells the assignees of each open task that waited for one
// just done (docs/PLAN.md section 3) whether it can start or what it
// still waits for. Someone who finished it themselves needs no telling,
// and nobody hears of a task in a project they cannot see.
func (s *Service) unblocked(ctx context.Context, c change, done syncer.TaskBrief, actor string) error {
	if !done.Completed {
		return nil // reopened since
	}
	waiting, err := s.tasks.Waiting(ctx, done.ID)
	if err != nil {
		return err
	}
	var out []Notice
	for _, w := range waiting {
		if w.Completed || w.Deleted || w.Archived || len(w.Assignees) == 0 {
			continue
		}
		blockers, err := s.tasks.Blockers(ctx, w.ID)
		if err != nil {
			return err
		}
		var still []syncer.TaskBrief
		for _, b := range blockers {
			if b.ID != done.ID && !b.Completed && !b.Deleted {
				still = append(still, b)
			}
		}
		for _, id := range w.Assignees {
			if id == c.ActorID {
				continue
			}
			if ok, err := s.tasks.SeesProject(ctx, id, c.WorkspaceID, done.ProjectID); err != nil {
				return err
			} else if !ok {
				continue
			}
			var names []string
			for _, b := range still {
				if ok, err := s.tasks.SeesProject(ctx, id, c.WorkspaceID, b.ProjectID); err != nil {
					return err
				} else if ok {
					names = append(names, b.Title)
				}
			}
			n := Notice{WorkspaceID: c.WorkspaceID, Task: &w, ActorID: c.ActorID, UserID: id, Kind: Unblocked}
			switch {
			case len(still) == 0:
				n.Title = actor + " finished " + done.Title + ": " + w.Title + " can start"
			case len(names) == 0:
				n.Title = actor + " finished " + done.Title + ": " + w.Title + " still waits for other tasks"
			default:
				n.Title = actor + " finished " + done.Title + ": " + w.Title + " still waits for " + strings.Join(names, ", ")
			}
			n.Body = where(w)
			out = append(out, n)
		}
	}
	return s.Send(ctx, out...)
}

// where names a task's project for a notification's text.
func where(t syncer.TaskBrief) string {
	if t.ProjectName == "" {
		return "In the inbox"
	}
	return "In " + t.ProjectName
}

func (s *Service) nameOf(ctx context.Context, userID string) string {
	if u, err := s.st.GetUser(ctx, userID); err == nil {
		return u.Name
	}
	return "Someone"
}

// InviteAccepted tells whoever sent an invitation that it was accepted.
// It runs after the accept has committed; a failure is only logged.
func (s *Service) InviteAccepted(ctx context.Context, workspaceID, inviterID, userID string) {
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		s.log.Warn("notify: invite accepted", "err", err)
		return
	}
	ws, err := s.st.GetWorkspace(ctx, workspaceID)
	if err != nil {
		s.log.Warn("notify: invite accepted", "err", err)
		return
	}
	err = s.Send(ctx, Notice{
		UserID: inviterID, WorkspaceID: workspaceID, Kind: InviteAccepted, ActorID: userID,
		Title: u.Name + " joined " + ws.Name,
		Body:  u.Name + " (" + u.Email + ") accepted your invitation.",
	})
	if err != nil {
		s.log.Warn("notify: invite accepted", "err", err)
	}
}

// mentionPattern is how a comment stores a mention: @[Name](user:<id>).
var mentionPattern = regexp.MustCompile(`@\[([^\]\n]{1,100})\]\(user:([A-Za-z0-9-]{1,64})\)`)

// referencePattern is how a comment stores a task or page it refers to: a
// Markdown link to an address in the app (web/src/lib/mentions.ts).
var referencePattern = regexp.MustCompile(`\[([^\]\n]{1,200})\]\(/[^)\s]*\)`)

// Mentions lists the users a comment mentions, each once, in order.
func Mentions(body string) []string {
	var ids []string
	for _, m := range mentionPattern.FindAllStringSubmatch(body, -1) {
		if !slices.Contains(ids, m[2]) {
			ids = append(ids, m[2])
		}
	}
	return ids
}

// newMentions are the users body mentions that previous did not.
func newMentions(body, previous string) []string {
	before := Mentions(previous)
	var out []string
	for _, id := range Mentions(body) {
		if !slices.Contains(before, id) {
			out = append(out, id)
		}
	}
	return out
}

// Excerpt is a comment as one short line: mentions as @Name, white space
// collapsed, cut at max characters.
func Excerpt(body string, max int) string {
	text := mentionPattern.ReplaceAllString(body, "@$1")
	text = referencePattern.ReplaceAllString(text, "$1")
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	r := []rune(text)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// Inform tells someone, in the app only, that something they asked for is
// done (an export is ready to download, an import finished). They asked
// and will look, so it goes to no other channel, whatever their choices.
func (s *Service) Inform(ctx context.Context, userID, workspaceID string, kind Kind, title, body string) error {
	err := s.st.InsertNotification(ctx, sqlitedb.InsertNotificationParams{
		ID: uuid.Must(uuid.NewV7()).String(), UserID: userID, WorkspaceID: workspaceID, Kind: string(kind),
		Title: title, Body: body, InApp: true, Now: s.now(),
	})
	if err == nil {
		s.poke(userID)
	}
	return err
}

// WebhookStopped tells who made a webhook that it was stopped for failing, by
// the webhooks' Notices.
func (s *Service) WebhookStopped(ctx context.Context, userID, workspaceID, url string) error {
	return s.Inform(ctx, userID, workspaceID, WebhookStopped, "A webhook was stopped",
		"Nothing has reached "+url+" for 3 days. Turn it back on in Settings once it is fixed.")
}

// WebhookTaken tells who had a webhook that someone else set where it sends
// or made its secret anew, so it is theirs now, by the webhooks' Notices.
func (s *Service) WebhookTaken(ctx context.Context, userID, workspaceID, url, by string) error {
	return s.Inform(ctx, userID, workspaceID, WebhookStopped, "A webhook you had is someone else's now",
		by+" changed the address or the secret of a webhook you had, so it is theirs now. It sends to "+url+". If that is not right, check it in Settings.")
}

// WebhooksPaused tells a workspace's owners and admins that webhooks were paused
// because whoever made them can no longer manage them.
func (s *Service) WebhooksPaused(ctx context.Context, workspaceID string) {
	members, err := s.st.ListMembers(ctx, workspaceID)
	if err != nil {
		s.log.Warn("telling of paused webhooks", "err", err)
		return
	}
	for _, m := range members {
		if !authz.Can(authz.Role(m.Role), authz.ManageWebhooks) {
			continue
		}
		if err := s.Inform(ctx, m.ID, workspaceID, WebhookStopped, "A webhook was paused",
			"Whoever made it can no longer manage this workspace's webhooks. Check its address in Settings, Webhooks, then turn it back on if it should go on."); err != nil {
			s.log.Warn("telling of paused webhooks", "err", err)
		}
	}
}

// itemPath is where a notification leads in the app: the workspace's
// Import and export settings for an export, its Webhooks settings for a
// paused webhook, the workspace an import made, or the task.
func itemPath(kind, workspaceID, taskID string) string {
	switch Kind(kind) {
	case ExportReady:
		return "/settings/w/" + workspaceID + "/data"
	case WebhookStopped:
		return "/settings/w/" + workspaceID + "/webhooks"
	}
	return Path(workspaceID, taskID)
}

// Item is a notification as its user sees it in the app.
type Item struct {
	sqlitedb.ListNotificationsRow
	URL string
}

// Page is a page of the in-app list.
type Page struct {
	Items  []Item
	Unread int64
	// Next is the cursor for the following page, "" at the end.
	Next string
}

// List returns a user's notifications, newest first, after the cursor
// before ("" for the first page).
func (s *Service) List(ctx context.Context, userID, before string, limit int) (Page, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	params := sqlitedb.ListNotificationsParams{
		UserID: userID, BeforeAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), MaxRows: int64(limit + 1),
	}
	if before != "" {
		n, err := s.st.GetNotification(ctx, before)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && n.UserID != userID) {
			return Page{}, apperr.New(apperr.Invalid, "That page of notifications does not exist.")
		}
		if err != nil {
			return Page{}, err
		}
		params.BeforeAt, params.BeforeID = n.CreatedAt, n.ID
	}
	rows, err := s.st.ListNotifications(ctx, params)
	if err != nil {
		return Page{}, err
	}
	unread, err := s.st.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return Page{}, err
	}
	p := Page{Items: []Item{}, Unread: unread}
	if len(rows) > limit {
		rows = rows[:limit]
		p.Next = rows[limit-1].ID
	}
	for _, r := range rows {
		ok, err := s.listable(ctx, userID, r)
		if err != nil {
			return Page{}, err
		}
		if ok {
			path := itemPath(r.Kind, r.WorkspaceID, r.TaskID.String)
			if r.PageThreadID.Valid {
				t, found, err := s.tasks.Thread(ctx, r.PageThreadID.String)
				if err != nil {
					return Page{}, err
				}
				if found {
					path = ThreadPath(t, r.WorkspaceID)
				}
			}
			p.Items = append(p.Items, Item{ListNotificationsRow: r, URL: path})
		}
	}
	return p, nil
}

// listable reports whether a notification is still its user's to read:
// they are in its workspace and, for one about a task, see the task's
// project. Leaving a workspace or losing a project deletes them (see
// accounts); this catches the rest, such as a task moved out of a guest's
// projects.
func (s *Service) listable(ctx context.Context, userID string, r sqlitedb.ListNotificationsRow) (bool, error) {
	if r.PageThreadID.Valid {
		t, ok, err := s.tasks.Thread(ctx, r.PageThreadID.String)
		if err != nil {
			return false, err
		}
		if !ok {
			// Gone for good: what it was about can no longer say who may
			// read it, so nobody does (a guest it was hidden from, above all).
			return false, nil
		}
		return s.tasks.SeesProject(ctx, userID, r.WorkspaceID, t.ProjectID)
	}
	if r.TaskID.Valid {
		t, ok, err := s.tasks.Task(ctx, r.TaskID.String)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		return s.tasks.SeesProject(ctx, userID, r.WorkspaceID, t.ProjectID)
	}
	return s.sees(ctx, userID, r.WorkspaceID, nil)
}

// MarkRead marks one of the user's notifications read.
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	n, err := s.st.GetNotification(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n.UserID != userID) {
		return apperr.New(apperr.NotFound, "No such notification.")
	}
	if err != nil {
		return err
	}
	_, err = s.st.MarkNotificationRead(ctx, sqlitedb.MarkNotificationReadParams{ID: id, UserID: userID, Now: sql.NullTime{Time: s.now(), Valid: true}})
	if err == nil {
		s.poke(userID)
	}
	return err
}

// Clear deletes one of the user's notifications for good. Asking again, as a second
// device may, or for someone else's, changes nothing. An email or push still waiting
// for it finds it gone and isn't sent.
func (s *Service) Clear(ctx context.Context, userID, id string) error {
	err := s.st.ClearNotification(ctx, sqlitedb.ClearNotificationParams{ID: id, UserID: userID})
	if err == nil {
		s.poke(userID)
	}
	return err
}

// ClearAll deletes the user's notifications up to and including through, the newest
// one they have seen. Newer ones stay.
func (s *Service) ClearAll(ctx context.Context, userID, through string) error {
	n, err := s.st.GetNotification(ctx, through)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n.UserID != userID) {
		return apperr.New(apperr.NotFound, "No such notification.")
	}
	if err != nil {
		return err
	}
	err = s.st.ClearNotificationsThrough(ctx, sqlitedb.ClearNotificationsThroughParams{
		UserID: userID, ThroughAt: n.CreatedAt, ThroughID: n.ID,
	})
	if err == nil {
		s.poke(userID)
	}
	return err
}

// MarkAllRead marks the user's notifications read up to and including
// through, the newest one they have seen. Newer ones stay unread.
func (s *Service) MarkAllRead(ctx context.Context, userID, through string) error {
	n, err := s.st.GetNotification(ctx, through)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n.UserID != userID) {
		return apperr.New(apperr.NotFound, "No such notification.")
	}
	if err != nil {
		return err
	}
	err = s.st.MarkAllNotificationsRead(ctx, sqlitedb.MarkAllNotificationsReadParams{
		UserID: userID, ThroughAt: n.CreatedAt, ThroughID: n.ID, Now: sql.NullTime{Time: s.now(), Valid: true},
	})
	if err == nil {
		s.poke(userID)
	}
	return err
}
