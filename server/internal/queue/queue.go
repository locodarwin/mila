// Package queue is `mila queue`: Mila as a work queue for an agent
// (docs/PLAN.md section 11, Agent access, phase 4). People assign tasks to
// the agent's account; the queue takes each open one in a to-do status,
// marks it in progress, gives it to a command, answers with what the
// command wrote as a comment, and finishes the task. Everything one task
// changes, the command's own writes through `mila mcp` or `mila api`
// included, goes in one batch, so a job can be undone as a whole.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/milaclient"
)

// Options is how the queue works.
type Options struct {
	// Then is what becomes of a task once its job has worked: "complete",
	// or a status of its project, by name. A task in the Inbox is completed.
	Then string
	// Every is how long to wait between looks at the queue.
	Every time.Duration
	// Once looks at the queue once, works through it, and stops.
	Once bool
	// Timeout is the longest one job may take.
	Timeout time.Duration
	// Log is where the queue says what it is doing.
	Log io.Writer
	// From are the people whose tasks the queue takes, by name or user
	// ID: a task anyone else wrote to is handed back. Empty means the
	// workspace's members, guests not among them.
	From []string
	// Anyone takes every task assigned, whoever wrote to it, guests and
	// email senders included.
	Anyone bool
}

// Job is one task for the command.
type Job struct {
	// ID and Title are the task's.
	ID, Title string
	// Batch is the job's batch, for the command's own writes, and
	// Workspace the workspace's ID.
	Batch, Workspace string
	// Whole is get-task's answer: the task, its checklist, comments and
	// subtasks.
	Whole json.RawMessage
	// Text is the same as Markdown, for a command that reads a prompt.
	Text string
}

// Work does one job, answering with what to say on the task. An error
// means it did not do it.
type Work func(ctx context.Context, job Job) (string, error)

// commentMax is the longest a comment may be, in characters.
const commentMax = 20_000

// Run works through the queue of the token's person until ctx ends, or
// once with Once.
func Run(ctx context.Context, c *milaclient.Client, o Options, work Work) error {
	if o.Every <= 0 {
		o.Every = time.Minute
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Minute
	}
	if o.Then == "" {
		o.Then = "complete"
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	q := &queue{c: c, o: o, work: work, gaveUp: map[string]bool{}}
	if !o.Anyone {
		me, err := c.Me(ctx)
		if err != nil {
			return err
		}
		q.me = me.ID
		// The names given must each mean one person now.
		if err := q.lookUp(ctx); err != nil {
			return err
		}
	}
	for {
		if err := q.pass(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// The server may be restarting: say so and look again later,
			// unless this was the only look.
			if o.Once {
				return err
			}
			q.say("looking at the queue: %v", err)
		}
		if o.Once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(o.Every):
		}
	}
}

type queue struct {
	c    *milaclient.Client
	o    Options
	work Work
	// gaveUp are tasks this queue could not hand back after a failed job,
	// so it doesn't take them again and again.
	gaveUp map[string]bool
	// me is the token's person, and trusted the people whose writes a
	// task may hold, worked out again at each look (roles change).
	me      string
	trusted map[string]bool
}

// person is someone in the workspace, as /people lists them.
type person struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Guest  bool   `json:"guest"`
}

// lookUp works out whom the queue trusts: the people From names, or the
// workspace's members.
func (q *queue) lookUp(ctx context.Context) error {
	var people []person
	if err := q.c.Get(ctx, q.c.In("/people"), &people); err != nil {
		return err
	}
	trusted := map[string]bool{q.me: true}
	if len(q.o.From) == 0 {
		for _, p := range people {
			if !p.Guest {
				trusted[p.UserID] = true
			}
		}
		q.trusted = trusted
		return nil
	}
	for _, name := range q.o.From {
		name = strings.TrimSpace(name)
		var found []person
		for _, p := range people {
			if p.UserID == name {
				found = []person{p}
				break
			}
			if strings.EqualFold(strings.TrimSpace(p.Name), name) {
				found = append(found, p)
			}
		}
		switch len(found) {
		case 1:
			trusted[found[0].UserID] = true
		case 0:
			return fmt.Errorf("--from: nobody in the workspace is called %q", name)
		default:
			ids := make([]string, len(found))
			for i, p := range found {
				ids[i] = p.UserID
			}
			return fmt.Errorf("--from: %d people are called %q; give one's user ID: %s", len(found), name, strings.Join(ids, ", "))
		}
	}
	q.trusted = trusted
	return nil
}

// event is a change in a task's history, as its activity lists it.
type event struct {
	ID        string `json:"id"`
	ActorID   string `json:"actorId"`
	ActorName string `json:"actorName"`
	Via       string `json:"via"`
}

// historyMax is the most entries a task's history lists, newest first
// (syncer.Activity). A history that long may have lost older entries,
// which the queue then can't check.
const historyMax = 200

// held is what a job holds of a task, as get-task gives it: the task and
// each of its subtasks, with whom they name.
type held struct {
	Task struct {
		Assignees []named `json:"assignees"`
	} `json:"task"`
	Subtasks []struct {
		ID        string  `json:"id"`
		Title     string  `json:"title"`
		Assignees []named `json:"assignees"`
	} `json:"subtasks"`
}

// named is someone get-task names.
type named struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// stranger says why a job's text can't be trusted, as a clause ("Gus
// wrote to this one"), or "" when nothing says so: someone the queue
// doesn't take work from wrote to the task or to a subtask the job holds
// (their fields, notes, checklists and comments are all in their
// histories), or is named on one as an assignee, which shows their own
// name. A history cut at its limit fails, unless since, the newest entries
// an earlier check read, is still in it: what it lost was checked then.
// newest is what this check read, for a later one.
func (q *queue) stranger(ctx context.Context, c *milaclient.Client, id string, whole json.RawMessage, since map[string]string) (why string, newest map[string]string, err error) {
	var h held
	if err := json.Unmarshal(whole, &h); err != nil {
		return "", nil, err
	}
	type part struct {
		id, where string
		people    []named
	}
	parts := []part{{id, "this one", h.Task.Assignees}}
	for _, s := range h.Subtasks {
		parts = append(parts, part{s.ID, fmt.Sprintf("its subtask %q", s.Title), s.Assignees})
	}
	newest = map[string]string{}
	for _, p := range parts {
		for _, who := range p.people {
			if !q.trusted[who.UserID] {
				return who.Name + " is assigned to " + p.where, nil, nil
			}
		}
		var events []event
		if err := c.Get(ctx, c.In("/tasks/"+url.PathEscape(p.id)+"/activity"), &events); err != nil {
			return "", nil, err
		}
		if len(events) > 0 {
			newest[p.id] = events[0].ID
		}
		for _, e := range events {
			switch {
			case e.Via == "email":
				return "an email wrote to " + p.where, nil, nil
			case !q.trusted[e.ActorID]:
				return e.ActorName + " wrote to " + p.where, nil, nil
			}
		}
		if len(events) >= historyMax && !slices.ContainsFunc(events, func(e event) bool { return since[p.id] != "" && e.ID == since[p.id] }) {
			return "the history of " + p.where + " is too long to check", nil, nil
		}
	}
	return "", newest, nil
}

func (q *queue) say(format string, args ...any) {
	_, _ = fmt.Fprintf(q.o.Log, time.Now().Format("15:04:05")+" "+format+"\n", args...)
}

// waiting is a task in the queue, as find-tasks gives it.
type waiting struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ProjectID string `json:"project_id"`
	StatusID  string `json:"status_id"`
}

// pass works through what is waiting now, one task at a time.
func (q *queue) pass(ctx context.Context) error {
	if !q.o.Anyone {
		if err := q.lookUp(ctx); err != nil {
			return err
		}
	}
	query := url.Values{"assignee": {"me"}, "kind": {"backlog,todo"}, "state": {"open"}, "limit": {"50"}}
	for {
		var found struct {
			Items []waiting `json:"items"`
			Next  string    `json:"next"`
		}
		if err := q.c.Find(ctx, query, &found); err != nil {
			return err
		}
		for _, t := range found.Items {
			if ctx.Err() != nil {
				return nil
			}
			if !q.gaveUp[t.ID] {
				q.one(ctx, t)
			}
		}
		// A search may stop before the end: go on from where it did.
		if found.Next == "" || ctx.Err() != nil {
			return nil
		}
		query.Set("after", found.Next)
	}
}

// one does one task's job.
func (q *queue) one(ctx context.Context, t waiting) {
	c := *q.c
	c.Batch = milaclient.NewBatch("queue")
	q.say("%s %q: starting, in batch %s", t.ID, t.Title, c.Batch)

	// In progress first, so the people who gave it see it taken.
	claimed := false
	if t.ProjectID != "" {
		err := c.Action(ctx, "set-status", map[string]any{"task": t.ID, "kind": "in-progress"}, nil)
		var refused *milaclient.Error
		switch {
		case err == nil:
			claimed = true
		case errors.As(err, &refused) && refused.Status == http.StatusBadRequest && refused.Field == "kind":
			// A project with no status in progress: work on it as it is.
		default:
			q.say("%s: can't take it: %v", t.ID, err)
			return
		}
	}
	var whole json.RawMessage
	if err := c.Get(ctx, c.In("/actions/get-task?"+url.Values{"task": {t.ID}}.Encode()), &whole); err != nil {
		q.say("%s: can't read it: %v", t.ID, err)
		q.handBack(ctx, &c, t, claimed, "")
		return
	}
	// Whoever can write to a task assigned to the agent instructs it, and
	// reads its answer: a guest could ask for what only the agent can see.
	// So, after reading it, who wrote to it, everything read included.
	whose := "the workspace's members"
	if len(q.o.From) > 0 {
		whose = "the people it was started for"
	}
	var checked map[string]string
	if !q.o.Anyone {
		why, newest, err := q.stranger(ctx, &c, t.ID, whole, nil)
		if err != nil {
			q.say("%s: can't read its history: %v", t.ID, err)
			q.handBack(ctx, &c, t, claimed, "")
			return
		}
		if why != "" {
			q.say("%s: not taken: %s", t.ID, why)
			q.handBack(ctx, &c, t, claimed, "The agent takes only tasks that "+whose+" alone have written to, and "+why+". It has taken itself off the task.")
			return
		}
		checked = newest
	}
	job := Job{ID: t.ID, Title: t.Title, Batch: c.Batch, Workspace: c.Workspace.ID, Whole: whole, Text: Markdown(whole)}
	jobCtx, cancel := context.WithTimeout(ctx, q.o.Timeout)
	said, err := q.work(jobCtx, job)
	timedOut := errors.Is(jobCtx.Err(), context.DeadlineExceeded)
	cancel()
	if ctx.Err() != nil {
		// Stopped while it worked: leave it for the next start.
		q.say("%s: stopped before the job finished", t.ID)
		q.handBack(context.WithoutCancel(ctx), &c, t, claimed, "")
		return
	}
	if err != nil {
		why := "The agent stopped without finishing: " + err.Error() + "."
		if timedOut {
			why = fmt.Sprintf("The agent took longer than %s and was stopped.", q.o.Timeout)
		}
		q.say("%s: %s", t.ID, why)
		q.handBack(ctx, &c, t, claimed, why+" It has taken itself off the task.")
		return
	}
	// The agent may have read the task again as it worked: if someone the
	// queue doesn't take work from wrote to it meanwhile, they instructed
	// the agent, and the answer would be theirs to read.
	if !q.o.Anyone {
		why := ""
		var again json.RawMessage
		err := c.Get(ctx, c.In("/actions/get-task?"+url.Values{"task": {t.ID}}.Encode()), &again)
		if err == nil {
			why, _, err = q.stranger(ctx, &c, t.ID, again, checked)
		}
		if err != nil {
			q.say("%s: can't check it again: %v", t.ID, err)
			q.handBack(ctx, &c, t, claimed, "The agent could not check the task again before answering, so it did not post its answer. It has taken itself off the task.")
			return
		}
		if why != "" {
			q.say("%s: answer not posted: while the job ran, %s", t.ID, why)
			q.handBack(ctx, &c, t, claimed, "The agent takes only tasks that "+whose+" alone have written to, and while it worked, "+why+". It did not post its answer, and has taken itself off the task.")
			return
		}
	}
	if said = strings.TrimSpace(said); said != "" {
		if err := c.Write(ctx, http.MethodPost, "/comments", map[string]any{"task_id": t.ID, "body": cut(said, commentMax)}, nil); err != nil {
			q.say("%s: can't comment: %v", t.ID, err)
		}
	}
	if err := q.finish(ctx, &c, t); err != nil {
		q.say("%s: can't finish it: %v", t.ID, err)
		q.gaveUp[t.ID] = true
		return
	}
	q.say("%s: done", t.ID)
}

// finish does what Then says to a task whose job worked.
func (q *queue) finish(ctx context.Context, c *milaclient.Client, t waiting) error {
	if strings.EqualFold(q.o.Then, "complete") || t.ProjectID == "" {
		return c.Action(ctx, "complete-task", map[string]any{"task": t.ID}, nil)
	}
	return c.Action(ctx, "set-status", map[string]any{"task": t.ID, "status": q.o.Then}, nil)
}

// handBack gives a task back after a job that didn't work: its status as
// it was, a word on why, and the agent off it, so it isn't tried again
// until someone assigns it once more. An empty why leaves it assigned, for
// a job cut short by stopping.
func (q *queue) handBack(ctx context.Context, c *milaclient.Client, t waiting, claimed bool, why string) {
	if claimed {
		// A task made without a status (through the API) goes back to
		// the project's first to-do one.
		back := map[string]any{"task": t.ID, "status": t.StatusID}
		if t.StatusID == "" {
			back = map[string]any{"task": t.ID, "kind": "todo"}
		}
		if err := c.Action(ctx, "set-status", back, nil); err != nil {
			q.say("%s: can't put its status back: %v", t.ID, err)
		}
	}
	if why == "" {
		return
	}
	if err := c.Write(ctx, http.MethodPost, "/comments", map[string]any{"task_id": t.ID, "body": why}, nil); err != nil {
		q.say("%s: can't comment: %v", t.ID, err)
	}
	if err := c.Action(ctx, "assign-task", map[string]any{"task": t.ID, "people": []string{"me"}, "remove": true}, nil); err != nil {
		q.say("%s: can't take itself off: %v", t.ID, err)
		q.gaveUp[t.ID] = true
	}
}

// cut keeps at most n characters of s.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	const more = "\n\n(cut short)"
	r := []rune(s)
	return string(r[:n-utf8.RuneCountInString(more)]) + more
}

// Markdown is a task, as get-task gives it, written out for a person or a
// model to read.
func Markdown(raw json.RawMessage) string {
	var w struct {
		Task struct {
			Title    string  `json:"title"`
			Notes    string  `json:"notes"`
			Priority string  `json:"priority"`
			Start    *string `json:"start_date"`
			Due      *string `json:"due_date"`
			DueTime  *string `json:"due_time"`
			Status   *struct {
				Name string `json:"name"`
			} `json:"status"`
			Project *struct {
				Name string `json:"name"`
			} `json:"project"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			Assignees []struct {
				Name string `json:"name"`
			} `json:"assignees"`
		} `json:"task"`
		Checklist []struct {
			Text string `json:"text"`
			Done bool   `json:"done"`
		} `json:"checklist"`
		Comments []struct {
			Author struct {
				Name string `json:"name"`
			} `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"comments"`
		Subtasks []struct {
			Title       string  `json:"title"`
			CompletedAt *string `json:"completed_at"`
		} `json:"subtasks"`
	}
	_ = json.Unmarshal(raw, &w)
	var b strings.Builder
	t := w.Task
	fmt.Fprintf(&b, "# %s\n\n", t.Title)
	line := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", name, value)
		}
	}
	if t.Project != nil {
		line("Project", t.Project.Name)
	} else {
		line("Project", "none (the Inbox)")
	}
	if t.Status != nil {
		line("Status", t.Status.Name)
	}
	if t.Priority != "none" {
		line("Priority", t.Priority)
	}
	if t.Start != nil {
		line("Starts", *t.Start)
	}
	if t.Due != nil {
		due := *t.Due
		if t.DueTime != nil {
			due += " " + *t.DueTime
		}
		line("Due", due)
	}
	names := func(list []struct {
		Name string `json:"name"`
	}) string {
		out := make([]string, len(list))
		for i, n := range list {
			out[i] = n.Name
		}
		return strings.Join(out, ", ")
	}
	line("Labels", names(t.Labels))
	line("Assigned to", names(t.Assignees))
	if notes := strings.TrimSpace(t.Notes); notes != "" {
		fmt.Fprintf(&b, "\n## Notes\n\n%s\n", notes)
	}
	if len(w.Checklist) > 0 {
		b.WriteString("\n## Checklist\n\n")
		for _, item := range w.Checklist {
			mark := " "
			if item.Done {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, item.Text)
		}
	}
	if len(w.Subtasks) > 0 {
		b.WriteString("\n## Subtasks\n\n")
		for _, s := range w.Subtasks {
			mark := " "
			if s.CompletedAt != nil {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, s.Title)
		}
	}
	if len(w.Comments) > 0 {
		b.WriteString("\n## Comments, oldest first\n")
		for _, c := range w.Comments {
			fmt.Fprintf(&b, "\n%s, %s:\n\n%s\n", c.Author.Name, c.CreatedAt.Format("2006-01-02 15:04"), strings.TrimSpace(c.Body))
		}
	}
	return b.String()
}
