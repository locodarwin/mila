package actions

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/filter"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Find is what find-tasks takes. Every part is optional, and the parts
// given must all hold.
type Find struct {
	// List is one of the app's lists: inbox, my-day (the Today page) or
	// upcoming.
	List string
	// View is a saved view's ID or name: its filter, as its page has it.
	View string
	// Project is a project's ID or name.
	Project string
	// DueFrom and DueTo bound the due date, both days included.
	DueFrom, DueTo string
	// State is open (the default), done or all.
	State string
	// Words are looked for in titles and notes, without regard to case.
	Words string
	// Assignee is a person by ID or name, or "me": tasks assigned to them.
	Assignee string
	// Kinds are the kinds of status to keep (backlog, todo, in-progress,
	// done, canceled), as a view's filter has them.
	Kinds []string
	// Limit is how many to answer with: 100 unless given, at most 500.
	Limit int
	// After is an answer's Next: the search goes on from there.
	After string
}

// Found is what find-tasks answers: the tasks, the day it took as today
// in the caller's time zone, and whether there may be more: more matched
// than the limit, or the search stopped looking before the end. Next then
// says where it stopped, for the next search's After.
type Found struct {
	Tasks []Task
	Today string
	More  bool
	Next  string
}

const findMax = 500

// findPage bounds a page of tasks read for find-tasks: a search few tasks
// match reads a large workspace in pages of this many.
const findPage = 2000

// findRead and findTime bound one search: after reading this many tasks,
// or for this long, it answers with what it found and a Next to go on.
const (
	findRead = 20_000
	findTime = 2 * time.Second
)

// cursorOf writes a place in find-tasks' order for an answer's Next.
func cursorOf(k syncer.TaskKey) string {
	raw, _ := json.Marshal([]string{k.Due, k.Time, k.Title, k.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// keyOf reads a Next given back as After.
func keyOf(cursor string) (*syncer.TaskKey, error) {
	bad := &apperr.Error{Kind: apperr.Invalid, Field: "after", Message: "after is the next of an earlier answer, as it gave it."}
	if len(cursor) > 8192 {
		return nil, bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	var parts []string
	// Text holds no NUL, which Postgres can't take.
	if err != nil || json.Unmarshal(raw, &parts) != nil || len(parts) != 4 || parts[3] == "" ||
		(parts[0] != "" && !isDay(parts[0])) || strings.ContainsRune(strings.Join(parts, ""), 0) {
		return nil, bad
	}
	return &syncer.TaskKey{Due: parts[0], Time: parts[1], Title: parts[2], ID: parts[3]}, nil
}

// statusKinds are the kinds a status may be.
var statusKinds = []string{"backlog", "todo", "in-progress", "done", "canceled"}

// FindTasks lists the tasks the caller sees that match, soonest due first.
// Tasks in archived projects stay out unless that project is asked for, as
// in the app's lists.
func (s *Service) FindTasks(ctx context.Context, c Caller, in Find) (Found, error) {
	limit := in.Limit
	switch {
	case limit == 0:
		limit = 100
	case limit < 1 || limit > findMax:
		return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: "limit", Message: "limit must be from 1 to 500."}
	}
	state := cmp.Or(in.State, "open")
	if state != "open" && state != "done" && state != "all" {
		return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: "state", Message: "state is open, done or all."}
	}
	for field, day := range map[string]string{"due_from": in.DueFrom, "due_to": in.DueTo} {
		if day != "" && !isDay(day) {
			return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: field, Message: field + " is a day like 2026-11-13."}
		}
	}
	switch in.List {
	case "", "inbox", "my-day", "upcoming":
	default:
		return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: "list", Message: "list is inbox, my-day or upcoming."}
	}
	for _, k := range in.Kinds {
		if !slices.Contains(statusKinds, k) {
			return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: "kind", Message: "kind is one or more of backlog, todo, in-progress, done and canceled, separated by commas."}
		}
	}
	var after *syncer.TaskKey
	if in.After != "" {
		var err error
		if after, err = keyOf(in.After); err != nil {
			return Found{}, err
		}
	}
	r, err := s.start(ctx, c)
	if err != nil {
		return Found{}, err
	}
	assignee := ""
	if in.Assignee != "" {
		if assignee, err = r.person("assignee", in.Assignee, "", false); err != nil {
			return Found{}, err
		}
	}
	project := ""
	if in.Project != "" {
		if project, err = r.project("project", in.Project); err != nil {
			return Found{}, err
		}
	}
	var view *filter.Filter
	viewProject := ""
	if in.View != "" {
		views, err := r.all("saved_view", nil)
		if err != nil {
			return Found{}, err
		}
		var among []candidate
		for _, v := range views {
			among = append(among, candidate{id: v.ID, name: str(v, "name")})
		}
		id, err := pick("view", "saved view", in.View, among)
		if err != nil {
			return Found{}, err
		}
		i := slices.IndexFunc(views, func(v syncer.Record) bool { return v.ID == id })
		config, _ := json.Marshal(views[i].Fields["config"])
		f, err := filter.FromConfig(config)
		if err != nil {
			return Found{}, &apperr.Error{Kind: apperr.Invalid, Field: "view", Message: "That view's filter can't be read."}
		}
		view, viewProject = &f, str(views[i], "project_id")
	}
	if project != "" && viewProject != "" && project != viewProject {
		return Found{Today: r.today, Tasks: []Task{}}, nil
	}

	// What needs only the task's own row is asked in SQL, which answers in
	// find-tasks' order a page at a time. The rest is checked here, reading
	// only what the view asks about and only for the page, until there is
	// one more than the limit, which says there are more.
	q := syncer.TaskQuery{State: state, Project: cmp.Or(project, viewProject), DueFrom: in.DueFrom, DueTo: in.DueTo,
		Assignee: assignee, Inbox: in.List == "inbox", Dated: in.List == "upcoming", Words: strings.TrimSpace(in.Words),
		// A view, as its page, shows no subtasks.
		TopLevel: view != nil, Kinds: in.Kinds, After: after}
	if in.List == "upcoming" && state == "all" {
		q.State = "open"
	}
	w := &workspace{open: map[string]bool{}, statusOf: map[string]string{}}
	if in.List == "my-day" || (view != nil && view.MyDay) {
		if w.myDay, err = r.myDay(); err != nil {
			return Found{}, err
		}
	}
	if in.List == "my-day" {
		q.MyDayThrough = r.today
	}
	if w.names, err = r.names(); err != nil {
		return Found{}, err
	}
	// Labels and assignees for every task looked at when the view asks
	// about them, else for the ones in the answer alone.
	early := view != nil && (len(view.Labels) > 0 || len(view.Assignees) > 0)
	ctxOf := w.filterContext(c.User, r.today)
	words := strings.ToLower(strings.TrimSpace(in.Words))
	keep := func(t *syncer.Record) bool {
		p, parent, due := str(*t, "project_id"), str(*t, "parent_id"), str(*t, "due_date")
		open := t.Fields["completed_at"] == nil
		switch {
		case state == "open" && !open, state == "done" && open:
			return false
		case in.DueFrom != "" && (due == "" || due < in.DueFrom), in.DueTo != "" && (due == "" || due > in.DueTo):
			return false
		case words != "" && !strings.Contains(strings.ToLower(str(*t, "title")+"\n"+str(*t, "notes")), words):
			return false
		case len(in.Kinds) > 0 && !slices.Contains(in.Kinds, ctxOf.Category(filter.Task{ID: t.ID})):
			return false
		}
		switch in.List {
		case "inbox":
			if p != "" || parent != "" {
				return false
			}
		case "upcoming":
			if due == "" || !open {
				return false
			}
		case "my-day":
			// Picked for today, or picked before and not done, as the
			// Today page has it.
			day, picked := w.myDay[t.ID]
			if !picked || day > r.today || (day < r.today && !open) {
				return false
			}
		}
		if view != nil {
			ft := filter.Task{ID: t.ID, ProjectID: p, StatusID: w.statusOf[t.ID], Title: str(*t, "title"), Notes: str(*t, "notes"),
				Priority: str(*t, "priority"), DueDate: due, Estimate: number(t.Fields["estimate_minutes"])}
			if parent != "" || !filter.Matches(ft, *view, ctxOf) {
				return false
			}
		}
		return true
	}

	var found []*syncer.Record
	looked := map[string]bool{}
	// A search reads at most so many tasks, or for so long, then answers
	// with what it found and where it stopped, so that no one search holds
	// its turn for long.
	began, read, cut := time.Now(), 0, false
	for size := limit + 1; ; size = min(size*2, findPage) {
		q.Limit = size
		page, err := r.s.sync.Tasks(r.ctx, r.c.User, r.c.Workspace, q)
		if err != nil {
			return Found{}, err
		}
		last := len(page) < size
		if len(page) > 0 {
			k := syncer.KeyOf(page[len(page)-1])
			q.After = &k
		}
		read += len(page)
		// A task that moved while paging may come twice.
		page = slices.DeleteFunc(page, func(t syncer.Record) bool { return looked[t.ID] })
		ids := make([]string, len(page))
		for i, t := range page {
			looked[t.ID] = true
			ids[i] = t.ID
			w.open[t.ID] = t.Fields["completed_at"] == nil
			w.statusOf[t.ID] = str(t, "status_id")
		}
		if early {
			if err := w.names.readTasks(r, ids); err != nil {
				return Found{}, err
			}
		}
		if view != nil {
			if err := w.readFor(r, *view, ids); err != nil {
				return Found{}, err
			}
		}
		for i := range page {
			if keep(&page[i]) {
				found = append(found, &page[i])
			}
		}
		if len(found) > limit || last {
			break
		}
		if read >= s.findRead || time.Since(began) >= s.findTime {
			cut = true
			break
		}
	}
	slices.SortFunc(found, func(a, b *syncer.Record) int { return syncer.TaskOrder(*a, *b) })
	out := Found{Today: r.today, Tasks: []Task{}}
	switch {
	case len(found) > limit:
		found, out.More = found[:limit], true
		out.Next = cursorOf(syncer.KeyOf(*found[limit-1]))
	case cut:
		out.More, out.Next = true, cursorOf(*q.After)
	}
	if !early {
		answer := make([]string, len(found))
		for i, t := range found {
			answer[i] = t.ID
		}
		if err := w.names.readTasks(r, answer); err != nil {
			return Found{}, err
		}
	}
	for _, t := range found {
		out.Tasks = append(out.Tasks, w.names.describe(t))
	}
	return out, nil
}

func isDay(s string) bool {
	return len(s) == 10 && s[4] == '-' && s[7] == '-' && strings.Trim(s[:4]+s[5:7]+s[8:], "0123456789") == ""
}

func number(v any) *int64 {
	switch n := v.(type) {
	case int64:
		return &n
	case int:
		x := int64(n)
		return &x
	case float64:
		x := int64(n)
		return &x
	case json.Number:
		if x, err := n.Int64(); err == nil {
			return &x
		}
	}
	return nil
}

// workspace is what find-tasks has read of a workspace: the names, and
// what a view's filter asks about the tasks that answer.
type workspace struct {
	names      *names
	myDay      map[string]string
	fieldTypes map[string]string
	values     map[string]map[string]any
	blockers   map[string][]string // the tasks each waits for
	waiting    map[string][]string // the tasks waiting for each
	deps       map[string]bool     // the dependencies read
	entries    map[string][]filter.Entry
	open       map[string]bool
	statusOf   map[string]string
}

// myDay is the day the caller last picked each task for My Day.
func (r *run) myDay() (map[string]string, error) {
	rows, err := r.all("my_day_entry", nil)
	if err != nil {
		return nil, err
	}
	days := map[string]string{}
	for _, e := range rows {
		if live(e) && str(e, "user_id") == r.c.User {
			days[str(e, "task_id")] = max(days[str(e, "task_id")], str(e, "day"))
		}
	}
	return days, nil
}

// readFor reads what view asks about the tasks given beyond their rows,
// labels and assignees: custom fields, dependencies and time logged. It
// adds to what it read before, a page of tasks at a time.
func (w *workspace) readFor(r *run, view filter.Filter, tasks []string) error {
	if len(view.Fields) > 0 {
		if w.fieldTypes == nil {
			w.fieldTypes, w.values = map[string]string{}, map[string]map[string]any{}
			fields, err := r.all("custom_field", nil)
			if err != nil {
				return err
			}
			for _, f := range fields {
				w.fieldTypes[f.ID] = str(f, "type")
			}
		}
		values, err := r.allIn("custom_field_value", "task_id", tasks)
		if err != nil {
			return err
		}
		for _, v := range values {
			raw, err := json.Marshal(v.Fields["value"])
			if err != nil {
				continue
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			var value any
			if dec.Decode(&value) == nil {
				task := str(v, "task_id")
				if w.values[task] == nil {
					w.values[task] = map[string]any{}
				}
				w.values[task][str(v, "field_id")] = value
			}
		}
	}
	if len(view.Dependencies) > 0 {
		// Both ends of each, and whether the task at the far end is open.
		if w.deps == nil {
			w.blockers, w.waiting, w.deps = map[string][]string{}, map[string][]string{}, map[string]bool{}
		}
		far := map[string]bool{}
		for _, by := range []string{"task_id", "blocker_id"} {
			deps, err := r.allIn("task_dependency", by, tasks)
			if err != nil {
				return err
			}
			for _, d := range deps {
				if w.deps[d.ID] {
					continue
				}
				w.deps[d.ID] = true
				blocker, task := str(d, "blocker_id"), str(d, "task_id")
				w.blockers[task] = append(w.blockers[task], blocker)
				w.waiting[blocker] = append(w.waiting[blocker], task)
				for _, id := range []string{blocker, task} {
					if _, ok := w.open[id]; !ok {
						far[id] = true
					}
				}
			}
		}
		for chunk := range slices.Chunk(slices.Collect(maps.Keys(far)), syncer.MaxLimit) {
			rows, err := r.s.sync.Tasks(r.ctx, r.c.User, r.c.Workspace, syncer.TaskQuery{IDs: chunk, Archived: true})
			if err != nil {
				return err
			}
			for _, id := range chunk {
				w.open[id] = false
			}
			for _, t := range rows {
				w.open[t.ID] = t.Fields["completed_at"] == nil
			}
		}
	}
	if len(view.Time) > 0 {
		if w.entries == nil {
			w.entries = map[string][]filter.Entry{}
		}
		entries, err := r.allIn("time_entry", "task_id", tasks)
		if err != nil {
			return err
		}
		for _, e := range entries {
			var minutes int64
			if n := number(e.Fields["minutes"]); n != nil {
				minutes = *n
			}
			w.entries[str(e, "task_id")] = append(w.entries[str(e, "task_id")], filter.Entry{User: str(e, "user_id"), Day: str(e, "day"), Minutes: minutes})
		}
	}
	return nil
}

// filterContext is what a view's filter asks about tasks, as the view's
// page and calendar feeds answer it.
func (w *workspace) filterContext(me, today string) filter.Context {
	return filter.Context{
		Me: me, Today: today,
		Category: func(t filter.Task) string {
			if st, ok := w.names.statuses[w.statusOf[t.ID]]; ok && st.Category != "" {
				return st.Category
			}
			if !w.open[t.ID] {
				return "done"
			}
			return "todo"
		},
		Labels:    func(id string) []string { return w.names.taskLabel[id] },
		Assignees: func(id string) []string { return w.names.assignees[id] },
		FieldType: func(id string) string { return w.fieldTypes[id] },
		Value:     func(task, field string) any { return w.values[task][field] },
		Blocked: func(id string) bool {
			return slices.ContainsFunc(w.blockers[id], func(b string) bool { return w.open[b] })
		},
		Blocking: func(id string) bool {
			return w.open[id] && slices.ContainsFunc(w.waiting[id], func(t string) bool { return w.open[t] })
		},
		Entries: func(id string) []filter.Entry { return w.entries[id] },
		InMyDay: func(id string) bool {
			day, ok := w.myDay[id]
			return ok && day <= today
		},
	}
}
