// Package actions is what the app does as one action, for scripts and agents
// through the API (docs/PLAN.md section 11, Agent access): the ops the app
// would make, one after another through syncer.Write as the caller, with the
// same checks, history and notifications as any write of theirs. Wherever an
// action takes a project, status, label or person it takes a name too,
// matched among what the caller sees; a name that matches nothing, or more
// than one thing, is refused with the candidates, never guessed.
package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/taskops"
)

// Service runs actions.
type Service struct {
	st   *store.Store
	sync *syncer.Service
	acc  *accounts.Service
	now  func() time.Time
	// findRead and findTime bound one find-tasks.
	findRead int
	findTime time.Duration
}

// New makes the service.
func New(st *store.Store, sync *syncer.Service, acc *accounts.Service) *Service {
	return &Service{st: st, sync: sync, acc: acc, now: time.Now, findRead: findRead, findTime: findTime}
}

// BoundSearches changes how far one find-tasks reads before it answers
// with where it stopped: tasks read, and time taken. For tests.
func (s *Service) BoundSearches(tasks int, d time.Duration) {
	s.findRead, s.findTime = tasks, d
}

// Caller is who an action is for, where, and what its request said of
// batches and retries.
type Caller struct {
	User, Workspace string
	// Batch is the request's Mila-Batch, or "" for the action's own.
	Batch string
	// Key is the request's Idempotency-Key, or "": a retry with the same
	// key makes no row twice.
	Key string
}

// run is one action under way: its batch, and a count of the rows it
// makes, which names each under the request's key.
type run struct {
	s     *Service
	ctx   context.Context
	c     Caller
	batch string
	made  int
	today string
}

func (s *Service) start(ctx context.Context, c Caller) (*run, error) {
	batch := c.Batch
	switch {
	case batch != "":
	case c.Key != "":
		// A retry names the same batch, so undoing it undoes both.
		sum := sha256.Sum256([]byte("action\n" + c.User + "\n" + c.Workspace + "\n" + c.Key))
		batch = "action-" + hex.EncodeToString(sum[:])[:32]
	default:
		batch = "action-" + uuid.NewString()
	}
	u, err := s.st.GetUser(ctx, c.User)
	if err != nil {
		return nil, err
	}
	return &run{s: s, ctx: ctx, c: c, batch: batch, today: s.now().In(accounts.Location(u)).Format(time.DateOnly)}, nil
}

// write is one of the action's writes, in its batch; a new row without an
// ID of its own is named from the request's key, so a retry finds it made.
func (r *run) write(w syncer.Write) (*syncer.Record, error) {
	w.Batch = r.batch
	if w.Mode == syncer.Create && w.ID == "" && r.c.Key != "" {
		r.made++
		w.Key = r.c.Key + "/" + strconv.Itoa(r.made)
	}
	rec, _, err := r.s.sync.Write(r.ctx, r.c.User, r.c.Workspace, w)
	return rec, err
}

// ops does a task's actions as the caller, in this run.
func (r *run) ops() taskops.Ops {
	return taskops.Ops{Sync: r.s.sync, Now: r.s.now, Write: func(_ context.Context, w syncer.Write) error {
		_, err := r.write(w)
		return err
	}}
}

// all lists every row of an entity the caller sees, with the filters given.
func (r *run) all(entity string, filters map[string]string) ([]syncer.Record, error) {
	var out []syncer.Record
	q := syncer.Query{Filters: filters, Limit: syncer.MaxLimit}
	for {
		page, err := r.s.sync.List(r.ctx, r.c.User, r.c.Workspace, entity, q)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Rows...)
		if page.Next == "" {
			return out, nil
		}
		q.After = page.Next
	}
}

// allIn is the rows of an entity whose field is one of ids, read a few
// hundred IDs at a time to stay inside the databases' limits on a query.
func (r *run) allIn(entity, field string, ids []string) ([]syncer.Record, error) {
	var out []syncer.Record
	for chunk := range slices.Chunk(ids, syncer.MaxLimit) {
		q := syncer.Query{In: map[string][]string{field: chunk}, Limit: syncer.MaxLimit}
		for {
			page, err := r.s.sync.List(r.ctx, r.c.User, r.c.Workspace, entity, q)
			if err != nil {
				return nil, err
			}
			out = append(out, page.Rows...)
			if page.Next == "" {
				break
			}
			q.After = page.Next
		}
	}
	return out, nil
}

// candidate is something a name may mean, with where it is for telling
// apart two of the same name.
type candidate struct{ id, name, where string }

// pick finds what ref names among candidates: an ID, or else a name without
// regard to case. field is the request's field, for the refusal.
func pick(field, kind, ref string, among []candidate) (string, error) {
	ref = strings.TrimSpace(ref)
	for _, c := range among {
		if c.id == ref {
			return c.id, nil
		}
	}
	var found []candidate
	for _, c := range among {
		if strings.EqualFold(strings.TrimSpace(c.name), ref) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 1:
		return found[0].id, nil
	case 0:
		names := make([]string, 0, len(among))
		for _, c := range among {
			names = append(names, strconv.Quote(c.name))
		}
		slices.Sort(names)
		names = slices.Compact(names)
		if len(names) > 20 {
			names = append(names[:20], "and more")
		}
		msg := fmt.Sprintf("There is no %s called %q that you can see.", kind, ref)
		if len(names) > 0 {
			msg += " There are: " + strings.Join(names, ", ") + "."
		} else {
			msg += " There are none."
		}
		return "", &apperr.Error{Kind: apperr.Invalid, Field: field, Message: msg}
	}
	parts := make([]string, len(found))
	for i, c := range found {
		parts[i] = c.id
		if c.where != "" {
			parts[i] += " (" + c.where + ")"
		}
	}
	return "", &apperr.Error{Kind: apperr.Conflict, Field: field,
		Message: fmt.Sprintf("More than one %s is called %q: %s. Give its ID instead.", kind, ref, strings.Join(parts, ", "))}
}

func str(rec syncer.Record, field string) string {
	s, _ := rec.Fields[field].(string)
	return s
}

func live(rec syncer.Record) bool { return rec.Fields["deleted_at"] == nil }

// project finds a project the caller sees by ID or name.
func (r *run) project(field, ref string) (string, error) {
	rows, err := r.all("project", nil)
	if err != nil {
		return "", err
	}
	var among []candidate
	for _, p := range rows {
		if live(p) {
			among = append(among, candidate{id: p.ID, name: str(p, "name")})
		}
	}
	return pick(field, "project", ref, among)
}

// status finds a status of a project by ID or name.
func (r *run) status(field, projectID, ref string) (string, error) {
	rows, err := r.all("status", map[string]string{"project_id": projectID})
	if err != nil {
		return "", err
	}
	var among []candidate
	for _, st := range rows {
		if live(st) {
			among = append(among, candidate{id: st.ID, name: str(st, "name")})
		}
	}
	return pick(field, "status in this project", ref, among)
}

// statusOfKind is the first status of a kind in a project, in its order.
func (r *run) statusOfKind(field, projectID, kind string) (string, error) {
	if !slices.Contains(statusKinds, kind) {
		return "", &apperr.Error{Kind: apperr.Invalid, Field: field, Message: "A kind is backlog, todo, in-progress, done or canceled."}
	}
	rows, err := r.all("status", map[string]string{"project_id": projectID})
	if err != nil {
		return "", err
	}
	first, key := "", ""
	for _, st := range rows {
		if live(st) && str(st, "category") == kind && (first == "" || str(st, "sort_key") < key) {
			first, key = st.ID, str(st, "sort_key")
		}
	}
	if first == "" {
		return "", &apperr.Error{Kind: apperr.Invalid, Field: field, Message: "This task's project has no status of the kind " + kind + "."}
	}
	return first, nil
}

// label finds a label of the workspace by ID or name.
func (r *run) label(field, ref string) (string, error) {
	rows, err := r.all("label", nil)
	if err != nil {
		return "", err
	}
	var among []candidate
	for _, l := range rows {
		if live(l) {
			among = append(among, candidate{id: l.ID, name: str(l, "name")})
		}
	}
	return pick(field, "label", ref, among)
}

// person finds someone the caller sees in the workspace, by ID or name, or
// "me" for the caller. When reach is set they must be able to reach a task
// in project ("" for the Inbox): a guest only in the projects shared with
// them.
func (r *run) person(field, ref, project string, reach bool) (string, error) {
	people, err := r.s.acc.People(r.ctx, r.c.User, r.c.Workspace)
	if err != nil {
		return "", err
	}
	id := r.c.User
	if !strings.EqualFold(strings.TrimSpace(ref), "me") {
		among := make([]candidate, len(people))
		for i, p := range people {
			among[i] = candidate{id: p.ID, name: p.Name}
		}
		if id, err = pick(field, "person", ref, among); err != nil {
			return "", err
		}
	}
	for _, p := range people {
		if reach && p.ID == id && p.Guest && (project == "" || !slices.Contains(p.Projects, project)) {
			return "", &apperr.Error{Kind: apperr.Invalid, Field: field, Message: p.Name + " is a guest who can't reach this task."}
		}
	}
	return id, nil
}

// task reads a task the caller sees, as a rule reads one.
func (r *run) task(field, id string) (syncer.TaskFacts, error) {
	rec, err := r.s.sync.Get(r.ctx, r.c.User, r.c.Workspace, "task", strings.TrimSpace(id))
	if err != nil {
		if apperr.KindOf(err) == apperr.NotFound {
			return syncer.TaskFacts{}, &apperr.Error{Kind: apperr.NotFound, Field: field, Message: "There is no task with that ID that you can see. Tasks are named by their IDs, which find-tasks gives."}
		}
		return syncer.TaskFacts{}, err
	}
	if !live(*rec) {
		return syncer.TaskFacts{}, &apperr.Error{Kind: apperr.Invalid, Field: field, Message: "That task is in the trash."}
	}
	facts, _, err := r.s.sync.TaskFactsOf(r.ctx, rec.ID)
	return facts, err
}

// Named is a row named in an answer.
type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StatusNamed is a task's status in an answer.
type StatusNamed struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// PersonNamed is someone named in an answer.
type PersonNamed struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// Task is a task as an action answers with it: the row, and the names an
// agent would otherwise look up.
type Task struct {
	Row       *syncer.Record
	Status    *StatusNamed
	Project   *Named
	Labels    []Named
	Assignees []PersonNamed
}

// names is what a run has read of the workspace for naming things in its
// answers, read once.
type names struct {
	projects  map[string]string
	statuses  map[string]StatusNamed
	labels    map[string]string
	people    map[string]string
	taskLabel map[string][]string
	assignees map[string][]string
}

// names reads what naming tasks takes: the workspace's projects, statuses,
// labels and people, and the labels and assignees of the tasks given.
func (r *run) names(tasks ...string) (*names, error) {
	n := &names{projects: map[string]string{}, statuses: map[string]StatusNamed{}, labels: map[string]string{},
		people: map[string]string{}, taskLabel: map[string][]string{}, assignees: map[string][]string{}}
	for _, read := range []struct {
		entity string
		into   func(syncer.Record)
	}{
		{"project", func(p syncer.Record) { n.projects[p.ID] = str(p, "name") }},
		{"status", func(s syncer.Record) {
			n.statuses[s.ID] = StatusNamed{ID: s.ID, Name: str(s, "name"), Category: str(s, "category")}
		}},
		{"label", func(l syncer.Record) { n.labels[l.ID] = str(l, "name") }},
	} {
		rows, err := r.all(read.entity, nil)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			read.into(row)
		}
	}
	people, err := r.s.acc.People(r.ctx, r.c.User, r.c.Workspace)
	if err != nil {
		return nil, err
	}
	for _, p := range people {
		n.people[p.ID] = p.Name
	}
	return n, n.readTasks(r, tasks)
}

// readTasks reads the labels and assignees of the tasks given, by their
// rows alone: reading every task's to describe one took seconds in a
// large workspace.
func (n *names) readTasks(r *run, tasks []string) error {
	for _, read := range []struct {
		entity string
		into   func(syncer.Record)
	}{
		{"task_label", func(tl syncer.Record) {
			n.taskLabel[str(tl, "task_id")] = append(n.taskLabel[str(tl, "task_id")], str(tl, "label_id"))
		}},
		{"task_assignee", func(ta syncer.Record) {
			n.assignees[str(ta, "task_id")] = append(n.assignees[str(ta, "task_id")], str(ta, "user_id"))
		}},
	} {
		rows, err := r.allIn(read.entity, "task_id", tasks)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if live(row) {
				read.into(row)
			}
		}
	}
	return nil
}

// describe is a task with its names.
func (n *names) describe(rec *syncer.Record) Task {
	t := Task{Row: rec, Labels: []Named{}, Assignees: []PersonNamed{}}
	if st, ok := n.statuses[str(*rec, "status_id")]; ok {
		t.Status = &st
	}
	if p := str(*rec, "project_id"); p != "" {
		t.Project = &Named{ID: p, Name: n.projects[p]}
	}
	for _, l := range n.taskLabel[rec.ID] {
		t.Labels = append(t.Labels, Named{ID: l, Name: n.labels[l]})
	}
	for _, u := range n.assignees[rec.ID] {
		name, ok := n.people[u]
		if !ok {
			name = syncer.Someone
		}
		t.Assignees = append(t.Assignees, PersonNamed{UserID: u, Name: name})
	}
	return t
}

// answer reads a task again after an action, with its names.
func (r *run) answer(id string) (Task, error) {
	rec, err := r.s.sync.Get(r.ctx, r.c.User, r.c.Workspace, "task", id)
	if err != nil {
		return Task{}, err
	}
	n, err := r.names(rec.ID)
	if err != nil {
		return Task{}, err
	}
	return n.describe(rec), nil
}

// raw is a value as JSON, for a write's fields.
func raw(v any) json.RawMessage { return taskops.Raw(v) }
