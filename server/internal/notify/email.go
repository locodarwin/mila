package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// digestWindow is how long after the chosen hour a digest may still go
// out, for a server that was down at the hour itself.
const digestWindow = 3

func (s *Service) footer() string {
	return "\n-- \nYou get this email because of your notification settings in Mila.\nChange them at " +
		s.baseURL + "/settings/notifications\n"
}

type ref struct {
	ID string `json:"id"`
}

// handleEmail emails one notification. A notification already emailed,
// or gone, or about something its user can no longer see, is not sent.
func (s *Service) handleEmail(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	n, err := s.st.GetNotification(ctx, r.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n.EmailedAt.Valid) {
		return nil
	}
	if err != nil {
		return err
	}
	u, err := s.st.GetUser(ctx, n.UserID)
	if err != nil {
		return err
	}
	ws, err := s.st.GetWorkspace(ctx, n.WorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	task, sees, err := s.stillSees(ctx, n)
	if err != nil || !sees {
		return err
	}
	link, err := s.noticeLink(ctx, n)
	if err != nil {
		return err
	}
	m := mail.Message{To: u.Email, Subject: n.Title, Text: s.emailText(n, ws, task, link)}
	if err := s.mail.Send(ctx, m); err != nil {
		return err
	}
	// Sent is sent, even if the server is stopping meanwhile.
	return s.st.SetNotificationEmailed(context.WithoutCancel(ctx), sqlitedb.SetNotificationEmailedParams{ID: n.ID, Now: sql.NullTime{Time: s.now(), Valid: true}})
}

// emailText is a notification as a plain-text email: what happened, the
// comment where there is one, where the task lives and a link to it.
func (s *Service) emailText(n sqlitedb.Notification, ws store.Workspace, task *syncer.TaskBrief, link string) string {
	var b strings.Builder
	b.WriteString(n.Title + ".\n\n")
	switch Kind(n.Kind) {
	case Mention, Comment, PageComment:
		for _, line := range strings.Split(n.Body, "\n") {
			b.WriteString("> " + line + "\n")
		}
		b.WriteString("\n")
	case Assigned:
		// The body only names the project, which follows.
	default:
		if n.Body != "" {
			b.WriteString(n.Body + "\n\n")
		}
	}
	if task != nil {
		project := task.ProjectName
		if project == "" {
			project = "Inbox"
		}
		fmt.Fprintf(&b, "Task: %s\nProject: %s (%s)\nOpen it: %s\n", task.Title, project, ws.Name, link)
	} else {
		fmt.Fprintf(&b, "Open %s: %s\n", ws.Name, link)
	}
	b.WriteString(s.footer())
	return b.String()
}

// handleMail sends a message prepared in advance (the digest).
func (s *Service) handleMail(ctx context.Context, payload []byte) error {
	var m mail.Message
	if err := json.Unmarshal(payload, &m); err != nil {
		return err
	}
	return s.mail.Send(ctx, m)
}

// handlePush sends one notification to the user's devices, unless it is
// about something they can no longer see.
func (s *Service) handlePush(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	n, err := s.st.GetNotification(ctx, r.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n.PushedAt.Valid) {
		return nil
	}
	if err != nil {
		return err
	}
	if n.Batch.Valid {
		if done, err := s.pushBatch(ctx, n); done || err != nil {
			return err
		}
	}
	if _, sees, err := s.stillSees(ctx, n); err != nil || !sees {
		return err
	}
	tag := n.ID
	if n.TaskID.Valid {
		tag = n.TaskID.String
	}
	if n.PageThreadID.Valid {
		tag = n.PageThreadID.String
	}
	path, err := s.noticePath(ctx, n)
	if err != nil {
		return err
	}
	err = s.pusher.Push(ctx, n.UserID, PushMessage{
		Title: n.Title, Body: n.Body, URL: path, Tag: tag,
		Urgent: Kind(n.Kind) == Due || Kind(n.Kind) == Reminder,
	})
	if err != nil {
		return err
	}
	return s.st.SetNotificationPushed(context.WithoutCancel(ctx), sqlitedb.SetNotificationPushedParams{ID: n.ID, Now: sql.NullTime{Time: s.now(), Valid: true}})
}

// pushBatch pushes a person's notifications from one change to many tasks as
// one ("Shawn assigned you 12 tasks"), from the first one's job; the others'
// jobs find them pushed. It reports false when there is only this one to
// push, which goes as usual.
func (s *Service) pushBatch(ctx context.Context, n sqlitedb.Notification) (bool, error) {
	group, err := s.st.ListBatchToPush(ctx, sqlitedb.ListBatchToPushParams{UserID: n.UserID, Batch: n.Batch, Kind: n.Kind})
	if err != nil {
		return false, err
	}
	if len(group) == 0 || group[0].ID != n.ID {
		return true, nil
	}
	var seen []sqlitedb.Notification
	for _, g := range group {
		_, sees, err := s.stillSees(ctx, g)
		if err != nil {
			return false, err
		}
		if sees {
			seen = append(seen, g)
		}
	}
	if len(seen) == 1 && seen[0].ID == n.ID {
		return false, nil
	}
	if len(seen) > 0 {
		body := seen[0].Body
		for _, g := range seen[1:] {
			if g.Body != body {
				body = "In several projects"
				break
			}
		}
		actor := "Someone"
		if seen[0].ActorID.Valid {
			actor = s.nameOf(ctx, seen[0].ActorID.String)
		}
		title := fmt.Sprintf("%s assigned you %d tasks", actor, len(seen))
		if len(seen) == 1 {
			title = seen[0].Title
		}
		path, err := s.noticePath(ctx, seen[0])
		if err != nil {
			return false, err
		}
		if err := s.pusher.Push(ctx, n.UserID, PushMessage{
			Title: title, Body: body, URL: path, Tag: "batch:" + n.Batch.String,
		}); err != nil {
			return false, err
		}
	}
	at := sql.NullTime{Time: s.now(), Valid: true}
	for _, g := range group {
		if err := s.st.SetNotificationPushed(context.WithoutCancel(ctx), sqlitedb.SetNotificationPushedParams{ID: g.ID, Now: at}); err != nil {
			return true, err
		}
	}
	return true, nil
}

// SendTestEmail sends the user a message to prove email reaches them.
func (s *Service) SendTestEmail(ctx context.Context, userID string) error {
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	err = s.mail.Send(ctx, mail.Message{
		To: u.Email, Subject: "Mila test email",
		Text: "Hi " + u.Name + ",\n\nThis is the test email you asked for. If you can read it, Mila's email notifications reach you.\n" + s.footer(),
	})
	if err != nil {
		s.log.Warn("test email failed", "user", userID, "err", err)
		return apperr.New(apperr.Unavailable, "Mila could not send the email. The server log says why.")
	}
	return nil
}

// SendDigests queues the daily digest for everyone whose chosen local
// hour has come and who has not had today's yet. It runs every minute.
func (s *Service) SendDigests(ctx context.Context) error {
	users, err := s.st.ListUsers(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	queued := false
	for _, u := range users {
		prefs := PrefsOf(u)
		if !prefs.Digest.Enabled {
			continue
		}
		local := now.In(accounts.Location(u))
		if h := local.Hour(); h < prefs.Digest.Hour || h >= prefs.Digest.Hour+digestWindow {
			continue
		}
		today := local.Format(time.DateOnly)
		since := now.Add(-24 * time.Hour)
		last, err := s.st.GetDigest(ctx, u.ID)
		switch {
		case err == nil && last.LastDate >= today:
			continue
		case err == nil && last.SentAt.After(now.Add(-7*24*time.Hour)):
			since = last.SentAt
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return err
		}
		m, err := s.digest(ctx, u, local, since)
		if err != nil {
			s.log.Warn("digest", "user", u.ID, "err", err)
			continue
		}
		err = s.st.InTx(ctx, func(q store.Queries) error {
			if err := q.UpsertDigest(ctx, sqlitedb.UpsertDigestParams{UserID: u.ID, LastDate: today, Now: now}); err != nil {
				return err
			}
			if m == nil {
				return nil
			}
			queued = true
			return jobs.Enqueue(ctx, q, jobMail, m, now, now)
		})
		if err != nil {
			return err
		}
	}
	if queued {
		s.jobs.Wake()
	}
	return nil
}

// digest writes a user's digest: their tasks overdue and due today, those
// past their estimate, what they have not read since the last one, and
// the time they logged yesterday. Nil means nothing to say; time logged
// alone is not news enough to send one.
func (s *Service) digest(ctx context.Context, u store.User, local time.Time, since time.Time) (*mail.Message, error) {
	today := local.Format(time.DateOnly)
	tasks, err := s.tasks.DueTasks(ctx, u.ID, today)
	if err != nil {
		return nil, err
	}
	unread, err := s.st.ListUnreadNotificationsSince(ctx, sqlitedb.ListUnreadNotificationsSinceParams{UserID: u.ID, Since: since})
	if err != nil {
		return nil, err
	}
	over, err := s.tasks.OverEstimate(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	logged, err := s.tasks.LoggedOn(ctx, u.ID, local.AddDate(0, 0, -1).Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	var overdue, dueToday []syncer.TaskBrief
	for _, t := range tasks {
		if t.DueDate < today {
			overdue = append(overdue, t)
		} else {
			dueToday = append(dueToday, t)
		}
	}
	if len(tasks) == 0 && len(unread) == 0 && len(over) == 0 {
		return nil, nil
	}

	var b strings.Builder
	style := accounts.DateStyleOf(u)
	fmt.Fprintf(&b, "Hi %s,\n\nHere is your day in Mila, %s.\n", u.Name, style.LongDay(local))
	section := func(title string, list []syncer.TaskBrief) {
		if len(list) == 0 {
			return
		}
		b.WriteString("\n" + title + "\n\n")
		for _, t := range list {
			line := "- " + t.Title
			if t.DueDate < today {
				if d, err := time.Parse(time.DateOnly, t.DueDate); err == nil {
					line += ", due " + style.Day(d, true, local)
				}
			}
			if t.DueTime != "" {
				line += " at " + style.Time(t.DueTime)
			}
			project := t.ProjectName
			if project == "" {
				project = "Inbox"
			}
			b.WriteString(line + " (" + project + ")\n  " + s.Link(t.WorkspaceID, t.ID) + "\n")
		}
	}
	section("Overdue", overdue)
	section("Due today", dueToday)
	if len(over) > 0 {
		b.WriteString("\nPast their estimate\n\n")
		for _, t := range over {
			project := t.ProjectName
			if project == "" {
				project = "Inbox"
			}
			fmt.Fprintf(&b, "- %s: %s logged of %s (%s)\n  %s\n", t.Title, Duration(t.Logged), Duration(t.Estimate), project, s.Link(t.WorkspaceID, t.ID))
		}
	}
	if len(unread) > 0 {
		b.WriteString("\nUnread notifications\n\n")
		for _, n := range unread {
			link, err := s.noticeLink(ctx, n)
			if err != nil {
				return nil, err
			}
			b.WriteString("- " + n.Title + "\n  " + link + "\n")
		}
	}
	if len(logged) > 0 {
		var total int64
		projects := 0
		for _, t := range logged {
			total += t.Logged
			if t.Itself {
				projects++
			}
		}
		// Time on a project itself is named for the project.
		name := func(t syncer.TimeBrief) string {
			if t.Itself {
				return t.Title + " itself"
			}
			return t.Title
		}
		things := "tasks"
		switch projects {
		case len(logged):
			things = "projects"
		case 0:
		default:
			things = "tasks and projects"
		}
		if len(logged) == 1 {
			fmt.Fprintf(&b, "\nYesterday you logged %s on %s.\n", Duration(total), name(logged[0]))
		} else {
			fmt.Fprintf(&b, "\nYesterday you logged %s on %d %s, most on %s.\n", Duration(total), len(logged), things, name(logged[0]))
		}
	}
	b.WriteString(s.footer())

	var parts []string
	if len(overdue) > 0 {
		parts = append(parts, fmt.Sprintf("%d overdue", len(overdue)))
	}
	if len(dueToday) > 0 {
		parts = append(parts, fmt.Sprintf("%d due today", len(dueToday)))
	}
	if len(over) > 0 {
		parts = append(parts, fmt.Sprintf("%d past %s estimate", len(over), map[bool]string{true: "its", false: "their"}[len(over) == 1]))
	}
	if len(unread) > 0 {
		parts = append(parts, fmt.Sprintf("%d unread", len(unread)))
	}
	return &mail.Message{To: u.Email, Subject: "Your Mila digest: " + strings.Join(parts, ", "), Text: b.String()}, nil
}

// Duration writes minutes as the app does: "1h 30m", "2h" or "45m".
func Duration(minutes int64) string {
	h, m := minutes/60, minutes%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
