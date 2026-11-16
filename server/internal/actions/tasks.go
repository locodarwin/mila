package actions

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/taskops"
)

// Done is what an action on a task answers: the task as it is now, the
// batch its writes went in, what it did in words, and a repeating task's
// next one when finishing it made one.
type Done struct {
	Task  Task
	Next  *Task
	Batch string
	Said  string
}

// AddTask is what add-task takes. Project, status, labels and assignees
// may be names; parent is a task's ID.
type AddTask struct {
	Title     string   `json:"title"`
	Project   *string  `json:"project"`
	Parent    *string  `json:"parent"`
	Status    *string  `json:"status"`
	Priority  *string  `json:"priority"`
	StartDate *string  `json:"start_date"`
	DueDate   *string  `json:"due_date"`
	DueTime   *string  `json:"due_time"`
	Notes     *string  `json:"notes"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
	Checklist []string `json:"checklist"`
}

func given(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

// AddTask makes a task as the app does: in a project, in its to-do status
// unless one is named; a subtask in its parent's project; then its labels,
// assignees and checklist. Everything named is found before anything is
// written, so a name that doesn't match makes nothing.
func (s *Service) AddTask(ctx context.Context, c Caller, in AddTask) (Done, error) {
	r, err := s.start(ctx, c)
	if err != nil {
		return Done{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Done{}, &apperr.Error{Kind: apperr.Invalid, Field: "title", Message: "A task needs a title."}
	}
	fields := map[string]json.RawMessage{"title": raw(title)}
	project := ""
	if given(in.Parent) {
		parent, err := r.task("parent", *in.Parent)
		if err != nil {
			return Done{}, err
		}
		project = parent.ProjectID
		fields["parent_id"] = raw(parent.ID)
		if given(in.Project) {
			named, err := r.project("project", *in.Project)
			if err != nil {
				return Done{}, err
			}
			if named != project {
				return Done{}, &apperr.Error{Kind: apperr.Invalid, Field: "project", Message: "A subtask is in its parent's project; leave project out."}
			}
		}
	} else if given(in.Project) {
		if project, err = r.project("project", *in.Project); err != nil {
			return Done{}, err
		}
	}
	switch {
	case project != "" && given(in.Status):
		st, err := r.status("status", project, *in.Status)
		if err != nil {
			return Done{}, err
		}
		fields["status_id"] = raw(st)
	case project != "":
		statuses, err := s.sync.StatusesOf(ctx, project)
		if err != nil {
			return Done{}, err
		}
		if st, ok := taskops.OpenStatus(statuses); ok {
			fields["status_id"] = raw(st)
		}
	case given(in.Status):
		return Done{}, &apperr.Error{Kind: apperr.Invalid, Field: "status", Message: "A task in the Inbox has no status; give a project too."}
	}
	if project != "" {
		fields["project_id"] = raw(project)
	}
	for name, v := range map[string]*string{"priority": in.Priority, "start_date": in.StartDate, "due_date": in.DueDate, "due_time": in.DueTime, "notes": in.Notes} {
		if v != nil {
			fields[name] = raw(*v)
		}
	}
	labels := make([]string, len(in.Labels))
	for i, ref := range in.Labels {
		if labels[i], err = r.label("labels", ref); err != nil {
			return Done{}, err
		}
	}
	assignees := make([]string, len(in.Assignees))
	for i, ref := range in.Assignees {
		if assignees[i], err = r.person("assignees", ref, project, true); err != nil {
			return Done{}, err
		}
	}
	for _, line := range in.Checklist {
		if strings.TrimSpace(line) == "" {
			return Done{}, &apperr.Error{Kind: apperr.Invalid, Field: "checklist", Message: "A checklist line needs some text."}
		}
	}

	rec, err := r.write(syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields})
	if err != nil {
		return Done{}, err
	}
	for _, l := range labels {
		if _, err := r.write(syncer.Write{Entity: "task_label", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": raw(rec.ID), "label_id": raw(l)}}); err != nil {
			return Done{}, err
		}
	}
	for _, u := range assignees {
		if _, err := r.write(syncer.Write{Entity: "task_assignee", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": raw(rec.ID), "user_id": raw(u)}}); err != nil {
			return Done{}, err
		}
	}
	for _, line := range in.Checklist {
		if _, err := r.write(syncer.Write{Entity: "checklist_item", Mode: syncer.Create,
			Fields: map[string]json.RawMessage{"task_id": raw(rec.ID), "text": raw(strings.TrimSpace(line))}}); err != nil {
			return Done{}, err
		}
	}
	t, err := r.answer(rec.ID)
	return Done{Task: t, Batch: r.batch, Said: "Added it"}, err
}

// TaskRef names a task by its ID.
type TaskRef struct {
	Task string `json:"task"`
}

// CompleteTask finishes a task as checking it off does: in a project, its
// status moves to done, and a repeating task's next one is made.
func (s *Service) CompleteTask(ctx context.Context, c Caller, in TaskRef) (Done, error) {
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		if t.Completed {
			return "It was done already", nil
		}
		return r.ops().Complete(ctx, t, r.today)
	})
}

// ReopenTask undoes a completion: in a project, back to a to-do status.
func (s *Service) ReopenTask(ctx context.Context, c Caller, in TaskRef) (Done, error) {
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		if !t.Completed {
			return "It was open already", nil
		}
		return r.ops().Reopen(ctx, t, r.today)
	})
}

// SetStatus is what set-status takes; status may be a name, or kind
// given instead for the project's first status of that kind.
type SetStatus struct {
	Task   string `json:"task"`
	Status string `json:"status"`
	Kind   string `json:"kind"`
}

// SetStatus moves a task to a status of its project, completing it for a
// done or canceled one and reopening it for any other.
func (s *Service) SetStatus(ctx context.Context, c Caller, in SetStatus) (Done, error) {
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		if t.ProjectID == "" {
			return "", &apperr.Error{Kind: apperr.Invalid, Field: "task", Message: "A task in the Inbox has no status; move it to a project first."}
		}
		var st string
		var err error
		switch {
		case (in.Status == "") == (in.Kind == ""):
			return "", &apperr.Error{Kind: apperr.Invalid, Field: "status", Message: "Give a status, by name or ID, or a kind."}
		case in.Kind != "":
			st, err = r.statusOfKind("kind", t.ProjectID, in.Kind)
		default:
			st, err = r.status("status", t.ProjectID, in.Status)
		}
		if err != nil {
			return "", err
		}
		return r.ops().SetStatus(ctx, t, st, r.today)
	})
}

// MoveTask is what move-task takes: project is a project's ID or name, or
// null for the Inbox.
type MoveTask struct {
	Task    string          `json:"task"`
	Project json.RawMessage `json:"project"`
}

// MoveTask takes a task, with its subtasks, to another project or the
// Inbox, each to the status of the same kind there.
func (s *Service) MoveTask(ctx context.Context, c Caller, in MoveTask) (Done, error) {
	var to *string
	if len(in.Project) == 0 || json.Unmarshal(in.Project, &to) != nil {
		return Done{}, &apperr.Error{Kind: apperr.Invalid, Field: "project", Message: "Say where: a project's name or ID, or null for the Inbox."}
	}
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		project := ""
		if to != nil {
			var err error
			if project, err = r.project("project", *to); err != nil {
				return "", err
			}
		}
		said, err := r.ops().Move(ctx, c.Workspace, t, project)
		if said == "" && err == nil {
			said = "It was there already"
		}
		return said, err
	})
}

// onTask runs an action on one task, and answers with it as it is after,
// and with the next of its series if finishing it made one.
func (s *Service) onTask(ctx context.Context, c Caller, id string, act func(*run, *syncer.TaskFacts) (string, error)) (Done, error) {
	r, err := s.start(ctx, c)
	if err != nil {
		return Done{}, err
	}
	t, err := r.task("task", id)
	if err != nil {
		return Done{}, err
	}
	wasDone := t.Completed
	said, err := act(r, &t)
	if err != nil {
		return Done{}, err
	}
	done := Done{Batch: r.batch, Said: said}
	if done.Task, err = r.answer(t.ID); err != nil {
		return Done{}, err
	}
	// The status by its name, as the person sees it, not its kind.
	if strings.HasPrefix(said, "Moved it to ") && done.Task.Status != nil && done.Task.Status.Name != "" {
		done.Said = "Moved it to " + done.Task.Status.Name
	}
	if t.Recurrence != "" && !wasDone && t.Completed {
		if next, err := r.answer(recur.NextTaskID(t.ID)); err == nil && live(*next.Row) {
			done.Next = &next
		}
	}
	return done, nil
}

// Assign is what assign-task takes: people by ID or name, or me; Remove
// takes them off instead.
type Assign struct {
	Task   string   `json:"task"`
	People []string `json:"people"`
	Remove bool     `json:"remove"`
}

// AssignTask gives a task to people, or takes it off them. Everyone named
// is found, and able to reach the task, before anything is written.
func (s *Service) AssignTask(ctx context.Context, c Caller, in Assign) (Done, error) {
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		if len(in.People) == 0 {
			return "", &apperr.Error{Kind: apperr.Invalid, Field: "people", Message: "Name at least one person."}
		}
		ids := make([]string, len(in.People))
		for i, ref := range in.People {
			var err error
			if ids[i], err = r.person("people", ref, t.ProjectID, !in.Remove); err != nil {
				return "", err
			}
		}
		return r.pairs("task_assignee", "user_id", t.ID, ids, in.Remove, "Assigned it", "Took it off them")
	})
}

// Label is what label-task takes: labels by ID or name; Remove takes them
// off instead.
type Label struct {
	Task   string   `json:"task"`
	Labels []string `json:"labels"`
	Remove bool     `json:"remove"`
}

// LabelTask puts labels on a task, or takes them off.
func (s *Service) LabelTask(ctx context.Context, c Caller, in Label) (Done, error) {
	return s.onTask(ctx, c, in.Task, func(r *run, t *syncer.TaskFacts) (string, error) {
		if len(in.Labels) == 0 {
			return "", &apperr.Error{Kind: apperr.Invalid, Field: "labels", Message: "Name at least one label."}
		}
		ids := make([]string, len(in.Labels))
		for i, ref := range in.Labels {
			var err error
			if ids[i], err = r.label("labels", ref); err != nil {
				return "", err
			}
		}
		return r.pairs("task_label", "label_id", t.ID, ids, in.Remove, "Labeled it", "Took the labels off")
	})
}

// pairs adds a task's pair rows (its labels, its assignees), or deletes
// them; one that isn't there is no matter.
func (r *run) pairs(entity, field, task string, ids []string, remove bool, added, removed string) (string, error) {
	for _, id := range ids {
		var err error
		if remove {
			_, err = r.write(syncer.Write{Entity: entity, ID: syncer.PairID(task, id), Mode: syncer.Delete})
			if apperr.KindOf(err) == apperr.NotFound {
				err = nil
			}
		} else {
			_, err = r.write(syncer.Write{Entity: entity, Mode: syncer.Upsert,
				Fields: map[string]json.RawMessage{"task_id": raw(task), field: raw(id)}})
		}
		if err != nil {
			return "", err
		}
	}
	if remove {
		return removed, nil
	}
	return added, nil
}

// Item is a line of a task's checklist.
type Item struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Comment is a comment on a task, with its author named.
type Comment struct {
	ID        string      `json:"id"`
	Author    PersonNamed `json:"author"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
}

// Whole is a task with everything under it: its checklist in order, its
// comments oldest first, and its subtasks.
type Whole struct {
	Task      Task
	Checklist []Item
	Comments  []Comment
	Subtasks  []Task
	InTrash   bool
}

// GetTask reads a task with its names, checklist, comments and subtasks.
// A task in the trash is read too, and says so.
func (s *Service) GetTask(ctx context.Context, c Caller, id string) (Whole, error) {
	r, err := s.start(ctx, c)
	if err != nil {
		return Whole{}, err
	}
	rec, err := s.sync.Get(ctx, c.User, c.Workspace, "task", strings.TrimSpace(id))
	if apperr.KindOf(err) == apperr.NotFound {
		return Whole{}, &apperr.Error{Kind: apperr.NotFound, Field: "task", Message: "There is no task with that ID that you can see. Tasks are named by their IDs, which find-tasks gives."}
	}
	if err != nil {
		return Whole{}, err
	}
	subs, err := r.all("task", map[string]string{"parent_id": rec.ID})
	if err != nil {
		return Whole{}, err
	}
	ids := []string{rec.ID}
	for _, sub := range subs {
		ids = append(ids, sub.ID)
	}
	n, err := r.names(ids...)
	if err != nil {
		return Whole{}, err
	}
	out := Whole{Task: n.describe(rec), InTrash: !live(*rec), Checklist: []Item{}, Comments: []Comment{}, Subtasks: []Task{}}
	of := map[string]string{"task_id": rec.ID}
	items, err := r.all("checklist_item", of)
	if err != nil {
		return Whole{}, err
	}
	sortRows(items, "sort_key")
	for _, it := range items {
		if live(it) || out.InTrash {
			done, _ := it.Fields["done"].(bool)
			out.Checklist = append(out.Checklist, Item{ID: it.ID, Text: str(it, "text"), Done: done})
		}
	}
	comments, err := r.all("comment", of)
	if err != nil {
		return Whole{}, err
	}
	slices.SortFunc(comments, func(a, b syncer.Record) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, cm := range comments {
		if !live(cm) {
			continue
		}
		who := str(cm, "author_id")
		name, ok := n.people[who]
		if !ok {
			name = syncer.Someone
		}
		out.Comments = append(out.Comments, Comment{ID: cm.ID, Author: PersonNamed{UserID: who, Name: name}, Body: str(cm, "body"), CreatedAt: cm.CreatedAt})
	}
	sortRows(subs, "sort_key")
	for i := range subs {
		if live(subs[i]) {
			out.Subtasks = append(out.Subtasks, n.describe(&subs[i]))
		}
	}
	return out, nil
}

// sortRows puts rows in the order of a field, as the app lists them.
func sortRows(rows []syncer.Record, field string) {
	slices.SortStableFunc(rows, func(a, b syncer.Record) int {
		return cmp.Or(strings.Compare(str(a, field), str(b, field)), strings.Compare(a.ID, b.ID))
	})
}
