package automations_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/automations"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

type told struct{ notices []notify.Notice }

func (t *told) Send(_ context.Context, n ...notify.Notice) error {
	t.notices = append(t.notices, n...)
	return nil
}

// world is a workspace with an owner, a member and a guest, a project with its
// statuses, and rules wired to sync.
type world struct {
	t                 *testing.T
	ctx               context.Context
	st                *store.Store
	d                 *db.DB
	sync              *syncer.Service
	runner            *jobs.Runner
	svc               *automations.Service
	told              *told
	ws, project       string
	todo, doing, done string
	users             map[string]string
	seq               int
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &world{t: t, ctx: ctx, st: st, d: d, told: &told{}, ws: uuid.NewString(), users: map[string]string{}}
	w.runner = jobs.New(st, log, nil)
	w.sync = syncer.New(st, log, nil)
	w.svc = automations.New(st, log, w.sync, w.runner, w.told)
	w.sync.SetWatcher(w.svc)
	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member", "guest"} {
		id := uuid.NewString()
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "-" + id[:8] + "@example.com", Name: role, Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: id, Role: role, Now: now}))
		w.users[role] = id
	}
	w.project = uuid.NewString()
	w.push("owner", "project", w.project, map[string]any{"name": "Home", "sort_key": "a0"})
	w.todo, w.doing, w.done = uuid.NewString(), uuid.NewString(), uuid.NewString()
	for i, s := range []struct{ id, name, category string }{{w.todo, "To do", "todo"}, {w.doing, "Doing", "in-progress"}, {w.done, "Done", "done"}} {
		w.push("owner", "status", s.id, map[string]any{"project_id": w.project, "name": s.name, "category": s.category, "sort_key": fmt.Sprintf("a%d", i)})
	}
	return w
}

func (w *world) ops(id, entity string, fields map[string]any, bulk bool) []syncer.Op {
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, err := json.Marshal(v)
		must(w.t, err)
		raw[k] = b
	}
	w.seq++
	hlc := fmt.Sprintf("%013d-%04x-%s", time.Now().UnixMilli(), w.seq, "test")
	return []syncer.Op{{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw, Bulk: bulk}}
}

// push sends an edit as a device would, and fails the test if it is refused.
func (w *world) push(role, entity, id string, fields map[string]any) {
	w.t.Helper()
	if err := w.try(role, entity, id, fields); err != nil {
		w.t.Fatalf("%s %s: %v", role, entity, err)
	}
}

// try sends an edit and says what was refused.
func (w *world) try(role, entity, id string, fields map[string]any) error {
	w.t.Helper()
	res, _, err := w.sync.Push(w.ctx, w.users[role], w.ws, w.ops(id, entity, fields, false))
	if err != nil {
		return err
	}
	if res[0].Status != "applied" {
		return fmt.Errorf("%s: %s", res[0].Status, res[0].Message)
	}
	return nil
}

func (w *world) drain() {
	w.t.Helper()
	for range 20 {
		did, err := w.runner.RunDue(w.ctx)
		must(w.t, err)
		if did == 0 {
			return
		}
	}
	w.t.Fatal("jobs never ran out")
}

// task makes an open task in the project.
func (w *world) task(title string, more map[string]any) string {
	w.t.Helper()
	id := uuid.NewString()
	fields := map[string]any{"title": title, "project_id": w.project, "status_id": w.todo, "sort_key": "a0", "priority": "none", "created_by": w.users["member"]}
	for k, v := range more {
		fields[k] = v
	}
	w.push("member", "task", id, fields)
	return id
}

// rule saves a rule as a role and returns its ID.
func (w *world) rule(role string, project any, name string, when map[string]any, then []map[string]any, config map[string]any) string {
	w.t.Helper()
	id := uuid.NewString()
	must(w.t, w.saveRule(role, id, project, name, when, then, config))
	return id
}

func (w *world) saveRule(role, id string, project any, name string, when map[string]any, then []map[string]any, config map[string]any) error {
	rule := map[string]any{"when": when, "then": then}
	if config != nil {
		rule["config"] = config
	}
	return w.try(role, "automation", id, map[string]any{
		"project_id": project, "name": name, "enabled": true, "enabled_at": time.Now().UTC().Format(time.RFC3339Nano),
		"rule": rule, "made_by": w.users[role], "sort_key": "a0",
	})
}

func (w *world) facts(id string) syncer.TaskFacts {
	w.t.Helper()
	f, there, err := w.sync.TaskFactsOf(w.ctx, id)
	must(w.t, err)
	if !there {
		w.t.Fatalf("no task %s", id)
	}
	return f
}

func (w *world) runs() []automations.Run {
	w.t.Helper()
	runs, err := w.svc.Runs(w.ctx, w.ws, "", 100)
	must(w.t, err)
	return runs
}

func (w *world) sql(q string) string {
	if w.d.Dialect == db.Postgres {
		return strings.Replace(q, "?", "$1", 1)
	}
	return q
}

func (w *world) paused(id string) string {
	w.t.Helper()
	var why *string
	must(w.t, w.d.QueryRowContext(w.ctx, w.sql("SELECT paused_why FROM automations WHERE id = ?"), id).Scan(&why))
	if why == nil {
		return ""
	}
	return *why
}

func TestARuleRunsWhenItsTaskChanges(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		label := uuid.NewString()
		w.push("owner", "label", label, map[string]any{"name": "Sampling", "color": "purple", "sort_key": "a0"})
		w.rule("member", w.project, "Follow up",
			map[string]any{"on": "status", "value": w.done},
			[]map[string]any{
				{"do": "subtask", "value": "Book the visit"},
				{"do": "priority", "value": "high"},
				{"do": "assign", "value": "creator"},
				{"do": "notify", "value": w.users["owner"]},
			},
			map[string]any{"filter": map[string]any{"labels": []string{label}}})

		fits, other := w.task("Fits", nil), w.task("Does not", nil)
		w.push("member", "task_label", syncer.PairID(fits, label), map[string]any{"task_id": fits, "label_id": label})
		w.push("member", "task", fits, map[string]any{"status_id": w.done, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.push("member", "task", other, map[string]any{"status_id": w.done, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.drain()

		f := w.facts(fits)
		if f.Priority != "high" || len(f.Subtasks) != 1 || !slices.Contains(f.Assignees, w.users["member"]) {
			t.Fatalf("after the rule: %+v", f)
		}
		if o := w.facts(other); o.Priority != "none" || len(o.Subtasks) != 0 {
			t.Fatalf("a task that doesn't fit changed: %+v", o)
		}
		if len(w.told.notices) != 1 || w.told.notices[0].UserID != w.users["owner"] || !strings.Contains(w.told.notices[0].Title, "Follow up") {
			t.Fatalf("told: %+v", w.told.notices)
		}
		runs := w.runs()
		if len(runs) != 1 || runs[0].Outcome != automations.Ran || runs[0].TaskID != fits {
			t.Fatalf("runs: %+v", runs)
		}
		// In the task's history, under the rule's name.
		history, err := w.sync.Activity(w.ctx, w.users["owner"], w.ws, fits)
		must(t, err)
		if !slices.ContainsFunc(history, func(e syncer.Event) bool { return e.Via == "automation: Follow up" }) {
			t.Fatalf("history: %+v", history)
		}
	})
}

func TestARulesOwnChangesStartNoOtherRule(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "First", map[string]any{"on": "created"},
			[]map[string]any{{"do": "priority", "value": "high"}}, nil)
		w.rule("member", w.project, "Second", map[string]any{"on": "priority", "value": "high"},
			[]map[string]any{{"do": "priority", "value": "urgent"}}, nil)
		id := w.task("Made", nil)
		w.drain()
		if p := w.facts(id).Priority; p != "high" {
			t.Fatalf("priority %s: the first rule's change started the second", p)
		}
		if len(w.runs()) != 1 {
			t.Fatalf("runs: %+v", w.runs())
		}
	})
}

func TestWhatStartsNoRule(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "On new", map[string]any{"on": "created"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)

		// Many rows at once, as a template makes.
		bulk := uuid.NewString()
		_, _, err := w.sync.Push(w.ctx, w.users["member"], w.ws, w.ops(bulk, "task", map[string]any{
			"title": "From a template", "project_id": w.project, "status_id": w.todo, "sort_key": "a1", "priority": "none",
		}, true))
		must(t, err)
		// The next of a repeating task.
		series := w.task("A repeat", map[string]any{"series_id": uuid.NewString()})
		w.drain()
		if w.facts(bulk).Priority != "none" || w.facts(series).Priority != "none" {
			t.Fatal("a template's row or a repeat's next started a rule")
		}
	})
}

func TestWhoMayMakeRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		then := []map[string]any{{"do": "priority", "value": "high"}}
		on := map[string]any{"on": "created"}
		if err := w.saveRule("guest", uuid.NewString(), w.project, "No", on, then, nil); err == nil {
			t.Fatal("a guest made a rule")
		}
		if err := w.saveRule("member", uuid.NewString(), nil, "No", on, then, nil); err == nil {
			t.Fatal("a member made a rule across the workspace")
		}
		wide := uuid.NewString()
		must(t, w.saveRule("owner", wide, nil, "Yes", on, then, nil))

		// Whoever changes a rule's work takes it on, so nobody can leave it running as someone else.
		id := w.rule("owner", w.project, "Theirs", on, then, nil)
		if err := w.try("member", "automation", id, map[string]any{"rule": map[string]any{"when": on, "then": then}}); err == nil {
			t.Fatal("changed a rule without taking it on")
		}
		must(t, w.try("member", "automation", id, map[string]any{"rule": map[string]any{"when": on, "then": then}, "made_by": w.users["member"]}))

		// A bad rule never gets in.
		for name, rule := range map[string]map[string]any{
			"unknown start":  {"when": map[string]any{"on": "explode"}, "then": then},
			"unknown change": {"when": on, "then": []map[string]any{{"do": "delete"}}},
			"nothing to do":  {"when": on, "then": []map[string]any{}},
		} {
			err := w.try("owner", "automation", uuid.NewString(), map[string]any{"project_id": w.project, "name": "Bad", "enabled": true, "rule": rule, "made_by": w.users["owner"], "sort_key": "a0"})
			if err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
		many := make([]map[string]any, syncer.MaxAutomationChanges+1)
		for i := range many {
			many[i] = then[0]
		}
		if err := w.saveRule("owner", uuid.NewString(), w.project, "Too many", on, many, nil); err == nil {
			t.Error("too many changes: accepted")
		}
	})
}

func TestAChangeThatCannotBeMadeLeavesTheRest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		// The guest has no share of this project, so can't be assigned here.
		w.rule("member", w.project, "Partly", map[string]any{"on": "created"},
			[]map[string]any{{"do": "assign", "value": w.users["guest"]}, {"do": "priority", "value": "high"}}, nil)
		id := w.task("Made", nil)
		w.drain()
		if f := w.facts(id); f.Priority != "high" || len(f.Assignees) != 0 {
			t.Fatalf("after: %+v", f)
		}
		runs := w.runs()
		if len(runs) != 1 || runs[0].Outcome != automations.Partly || !strings.Contains(runs[0].Detail, "wasn't assigned") {
			t.Fatalf("runs: %+v", runs)
		}
		if len(w.told.notices) != 1 || w.told.notices[0].UserID != w.users["member"] {
			t.Fatalf("told: %+v", w.told.notices)
		}
	})
}

func TestARuleWhoseMakerCannotRunItPauses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id := w.rule("member", w.project, "Theirs", map[string]any{"on": "created"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		must(t, w.st.UpdateMemberRole(w.ctx, sqlitedb.UpdateMemberRoleParams{WorkspaceID: w.ws, UserID: w.users["member"], Role: "guest"}))
		made := uuid.NewString()
		w.push("owner", "task", made, map[string]any{"title": "New", "project_id": w.project, "status_id": w.todo, "sort_key": "a1", "priority": "none"})
		w.drain()
		if p := w.facts(made).Priority; p != "none" {
			t.Fatalf("it ran: %s", p)
		}
		if w.paused(id) == "" {
			t.Fatal("not paused")
		}
	})
}

func TestARunawayRulePauses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id := w.rule("member", w.project, "Busy", map[string]any{"on": "created"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		for i := range automations.MaxRunsAnHour {
			must(t, w.st.InsertAutomationRun(w.ctx, sqlitedb.InsertAutomationRunParams{
				ID: uuid.NewString(), WorkspaceID: w.ws, AutomationID: id, TaskID: fmt.Sprint(i), TaskTitle: "x", Outcome: automations.Ran, Detail: "", Now: store.Now(),
			}))
		}
		made := w.task("One too many", nil)
		w.drain()
		if w.facts(made).Priority != "none" || w.paused(id) == "" {
			t.Fatal("kept running")
		}
		if len(w.told.notices) != 1 || w.told.notices[0].UserID != w.users["member"] {
			t.Fatalf("told: %+v", w.told.notices)
		}
	})
}

func TestAChangeToManyCountsOnceTowardThePause(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id := w.rule("member", w.project, "Busy", map[string]any{"on": "created"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		for i := range automations.MaxRunsAnHour - 1 {
			must(t, w.st.InsertAutomationRun(w.ctx, sqlitedb.InsertAutomationRunParams{
				ID: uuid.NewString(), WorkspaceID: w.ws, AutomationID: id, TaskID: fmt.Sprint(i), TaskTitle: "x", Outcome: automations.Ran, Detail: "", Now: store.Now(),
			}))
		}
		// Three tasks made as one change to many: one more change, three runs.
		batch := uuid.NewString()
		var ops []syncer.Op
		var made []string
		for i := range 3 {
			task := uuid.NewString()
			made = append(made, task)
			op := w.ops(task, "task", map[string]any{"project_id": w.project, "status_id": w.todo, "title": fmt.Sprint("Batch ", i), "sort_key": "a0", "priority": "none", "created_by": w.users["member"]}, false)[0]
			op.Batch = batch
			ops = append(ops, op)
		}
		res, _, err := w.sync.Push(w.ctx, w.users["member"], w.ws, ops)
		must(t, err)
		for _, r := range res {
			if r.Status != "applied" {
				t.Fatalf("refused: %+v", r)
			}
		}
		w.drain()
		for _, task := range made {
			if w.facts(task).Priority != "high" {
				t.Fatalf("%s: the rule stopped inside one change", task)
			}
		}
		if w.paused(id) != "" {
			t.Fatal("paused at 200 changes")
		}
		// The next change, alone, is one too many.
		one := w.task("One too many", nil)
		w.drain()
		if w.facts(one).Priority != "none" || w.paused(id) == "" {
			t.Fatal("kept running past 200 changes")
		}
	})
}

func TestABatchCannotKeepARuleRunningForever(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id := w.rule("member", w.project, "Busy", map[string]any{"on": "created"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		for i := range automations.MaxBatchedRunsAnHour {
			must(t, w.st.InsertAutomationRun(w.ctx, sqlitedb.InsertAutomationRunParams{
				ID: uuid.NewString(), WorkspaceID: w.ws, AutomationID: id, TaskID: fmt.Sprint(i), TaskTitle: "x", Outcome: automations.Ran, Detail: "",
				Now: store.Now(), Batch: sql.NullString{String: "same", Valid: true},
			}))
		}
		task := uuid.NewString()
		op := w.ops(task, "task", map[string]any{"project_id": w.project, "status_id": w.todo, "title": "More", "sort_key": "a0", "priority": "none", "created_by": w.users["member"]}, false)[0]
		op.Batch = "same"
		_, _, err := w.sync.Push(w.ctx, w.users["member"], w.ws, []syncer.Op{op})
		must(t, err)
		w.drain()
		if w.facts(task).Priority != "none" || w.paused(id) == "" {
			t.Fatal("one batch kept the rule running")
		}
	})
}

func TestDueDatesStartARuleOnceFromWhenItWasTurnedOn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		day := func(n int) string { return time.Now().UTC().AddDate(0, 0, n).Format(time.DateOnly) }
		long := w.task("Overdue before the rule", map[string]any{"due_date": day(-3)})
		rule := w.rule("member", w.project, "Late", map[string]any{"on": "due", "value": "1"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		soon := w.task("Due tomorrow", map[string]any{"due_date": day(1)})
		far := w.task("Due next month", map[string]any{"due_date": day(30)})
		doneTask := w.task("Already done", map[string]any{"due_date": day(1), "status_id": w.done, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.drain()
		must(t, w.svc.CheckDue(w.ctx))
		must(t, w.svc.CheckDue(w.ctx))
		if w.facts(soon).Priority != "high" {
			t.Fatal("what's near didn't start it")
		}
		if w.facts(long).Priority != "none" || w.facts(far).Priority != "none" || w.facts(doneTask).Priority != "none" {
			t.Fatal("started on the wrong tasks")
		}
		if len(w.runs()) != 1 {
			t.Fatalf("ran %d times", len(w.runs()))
		}
		// A new date is a new start, for a rule on since before it came near.
		w.push("member", "automation", rule, map[string]any{"enabled_at": time.Now().UTC().AddDate(0, 0, -2).Format(time.RFC3339Nano)})
		w.push("member", "task", soon, map[string]any{"priority": "none", "due_date": day(0)})
		must(t, w.svc.CheckDue(w.ctx))
		if w.facts(soon).Priority != "high" || len(w.runs()) != 2 {
			t.Fatalf("a new date didn't start it: %d runs", len(w.runs()))
		}
	})
}

func TestCompletingARepeatingTaskMakesTheNextOnce(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		day := time.Now().UTC().Format(time.DateOnly)
		label := uuid.NewString()
		w.push("owner", "label", label, map[string]any{"name": "Go", "color": "red", "sort_key": "a0"})
		w.rule("member", w.project, "Done now", map[string]any{"on": "label", "value": label}, []map[string]any{{"do": "complete"}}, nil)
		task := w.task("Water the plants", map[string]any{"due_date": day, "recurrence": "FREQ=WEEKLY", "recurrence_mode": recur.Schedule})
		w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"task_id": task, "label_id": label})
		w.drain()
		f := w.facts(task)
		if !f.Completed || f.StatusID != w.done || f.SeriesID != task {
			t.Fatalf("not finished: %+v", f)
		}
		next := w.facts(recur.NextTaskID(task))
		if next.Completed || next.StatusID != w.todo || next.DueDate <= day || !slices.Contains(next.Labels, label) {
			t.Fatalf("next: %+v", next)
		}
	})
}

func TestAMoveTakesSubtasksToTheSameKindOfStatus(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		there, open := uuid.NewString(), uuid.NewString()
		w.push("owner", "project", there, map[string]any{"name": "There", "sort_key": "a1"})
		w.push("owner", "status", open, map[string]any{"project_id": there, "name": "Open", "category": "todo", "sort_key": "a0"})
		task := w.task("Moving", nil)
		sub := w.task("Under it", map[string]any{"parent_id": task})
		label := uuid.NewString()
		w.push("owner", "label", label, map[string]any{"name": "Move", "color": "red", "sort_key": "a0"})
		w.rule("owner", nil, "Across", map[string]any{"on": "label", "value": label}, []map[string]any{{"do": "move", "value": there}}, nil)
		w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"task_id": task, "label_id": label})
		w.drain()
		if f := w.facts(task); f.ProjectID != there || f.StatusID != open {
			t.Fatalf("task: %+v", f)
		}
		if f := w.facts(sub); f.ProjectID != there || f.StatusID != open {
			t.Fatalf("subtask: %+v", f)
		}
	})
}

func TestTheLastBlockerFinishing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "Free", map[string]any{"on": "unblocked"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		a, b, waiting := w.task("First", nil), w.task("Second", nil), w.task("Waiting", nil)
		for _, blocker := range []string{a, b} {
			w.push("member", "task_dependency", syncer.PairID(waiting, blocker), map[string]any{"task_id": waiting, "blocker_id": blocker})
		}
		finish := func(id string) {
			w.push("member", "task", id, map[string]any{"status_id": w.done, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
			w.drain()
		}
		finish(a)
		if w.facts(waiting).Priority != "none" {
			t.Fatal("started with a blocker left")
		}
		finish(b)
		if w.facts(waiting).Priority != "high" {
			t.Fatal("the last blocker didn't start it")
		}
	})
}

func TestARuleCanReadCommentsAndEmail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "Said", map[string]any{"on": "comment"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		w.rule("member", w.project, "Mail", map[string]any{"on": "email"}, []map[string]any{{"do": "priority", "value": "low"}}, nil)
		task := w.task("Discussed", nil)
		w.push("member", "comment", uuid.NewString(), map[string]any{"task_id": task, "author_id": w.users["member"], "body": "Hello"})
		w.drain()
		if w.facts(task).Priority != "high" {
			t.Fatal("a comment didn't start it")
		}
		rec, _, err := w.sync.Write(syncer.WithEmail(w.ctx), w.users["member"], w.ws, syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: map[string]json.RawMessage{"title": json.RawMessage(`"By mail"`), "project_id": json.RawMessage(`"` + w.project + `"`)}})
		must(t, err)
		w.drain()
		if w.facts(rec.ID).Priority != "low" {
			t.Fatal("email didn't start it")
		}
	})
}

func TestARuleCannotMoveATaskIntoAProjectThatIsGoing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		for name, gone := range map[string]string{"trashed": "deleted_at", "archived": "archived_at"} {
			there := uuid.NewString()
			w.push("owner", "project", there, map[string]any{"name": name, "sort_key": "a1"})
			w.push("owner", "status", uuid.NewString(), map[string]any{"project_id": there, "name": "Open", "category": "todo", "sort_key": "a0"})
			w.push("owner", "project", there, map[string]any{gone: stamp})
			label := uuid.NewString()
			w.push("owner", "label", label, map[string]any{"name": name, "color": "red", "sort_key": "a0"})
			w.rule("owner", nil, name, map[string]any{"on": "label", "value": label}, []map[string]any{{"do": "move", "value": there}}, nil)
			task := w.task("Staying", nil)
			w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"task_id": task, "label_id": label})
			w.drain()
			if f := w.facts(task); f.ProjectID != w.project {
				t.Fatalf("%s: moved to %s", name, f.ProjectID)
			}
			if runs := w.runs(); runs[0].Outcome != automations.Failed {
				t.Fatalf("%s: run says %+v", name, runs[0])
			}
		}
	})
}

// A rule that hits its limit part way through its due dates does the rest once it's back.
func TestDueDatesPastARulesLimitAreNotLost(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format(time.DateOnly)
		rule := w.rule("member", w.project, "Soon", map[string]any{"on": "due", "value": "1"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		tasks := []string{w.task("One", map[string]any{"due_date": tomorrow}), w.task("Two", map[string]any{"due_date": tomorrow}), w.task("Three", map[string]any{"due_date": tomorrow})}
		w.drain()
		for i := range automations.MaxRunsAnHour - 1 {
			must(t, w.st.InsertAutomationRun(w.ctx, sqlitedb.InsertAutomationRunParams{
				ID: uuid.NewString(), WorkspaceID: w.ws, AutomationID: rule, TaskID: fmt.Sprint(i), TaskTitle: "x", Outcome: automations.Ran, Detail: "", Now: store.Now().Add(-time.Minute),
			}))
		}
		must(t, w.svc.CheckDue(w.ctx))
		high := func() int {
			n := 0
			for _, id := range tasks {
				if w.facts(id).Priority == "high" {
					n++
				}
			}
			return n
		}
		if high() != 1 || w.paused(rule) == "" {
			t.Fatalf("%d done, paused %q", high(), w.paused(rule))
		}
		if len(w.told.notices) != 1 {
			t.Fatalf("told %d times", len(w.told.notices))
		}
		// Back on, an hour later.
		_, err := w.d.ExecContext(w.ctx, w.sql("DELETE FROM automation_runs WHERE automation_id = ?"), rule)
		must(t, err)
		w.push("member", "automation", rule, map[string]any{"enabled": true, "paused_why": nil})
		must(t, w.svc.CheckDue(w.ctx))
		if high() != 3 {
			t.Fatalf("%d done after it came back", high())
		}
	})
}

// The next of a repeating task gets the same rows the app would make, and one deleted
// when a finish was undone comes back.
func TestARuleRepeatsATaskAsTheAppDoes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		day := time.Now().UTC().Format(time.DateOnly)
		label := uuid.NewString()
		w.push("owner", "label", label, map[string]any{"name": "Go", "color": "red", "sort_key": "a0"})
		w.rule("member", w.project, "Done now", map[string]any{"on": "label", "value": label}, []map[string]any{{"do": "complete"}}, nil)
		task := w.task("Water the plants", map[string]any{"due_date": day, "recurrence": "FREQ=WEEKLY", "recurrence_mode": recur.Schedule})
		item := uuid.NewString()
		w.push("member", "checklist_item", item, map[string]any{"task_id": task, "text": "The fern", "done": false, "sort_key": "a0"})
		w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"task_id": task, "label_id": label})
		w.drain()
		id := recur.NextTaskID(task)
		next := w.facts(id)
		if len(next.Checklist) != 1 || next.Checklist[0].ID != recur.HashID(id+"/"+item) || next.Checklist[0].Text != "The fern" {
			t.Fatalf("checklist: %+v", next.Checklist)
		}
		// Undone, as the app does it, and finished again by the rule.
		now := time.Now().UTC().Format(time.RFC3339Nano)
		w.push("member", "task", id, map[string]any{"deleted_at": now})
		w.push("member", "task", task, map[string]any{"completed_at": nil, "status_id": w.todo})
		w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"deleted_at": now})
		w.drain()
		w.push("member", "task_label", syncer.PairID(task, label), map[string]any{"deleted_at": nil})
		w.drain()
		if !w.facts(task).Completed {
			t.Fatal("not finished again")
		}
		if back := w.facts(id); back.Deleted || back.Completed {
			t.Fatalf("the next stayed away: %+v", back)
		}
	})
}

// A rule's Only if can read time logged on the task.
func TestARuleCanAskAboutTimeLogged(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "Over", map[string]any{"on": "comment"}, []map[string]any{{"do": "priority", "value": "high"}},
			map[string]any{"filter": map[string]any{"time": []string{"over"}}})
		over, under := w.task("Over", map[string]any{"estimate_minutes": 10}), w.task("Under", map[string]any{"estimate_minutes": 60})
		for _, id := range []string{over, under} {
			w.push("member", "time_entry", uuid.NewString(), map[string]any{"task_id": id, "user_id": w.users["member"], "day": time.Now().UTC().Format(time.DateOnly), "minutes": 30})
			w.push("member", "comment", uuid.NewString(), map[string]any{"task_id": id, "author_id": w.users["member"], "body": "How long?"})
		}
		w.drain()
		if w.facts(over).Priority != "high" || w.facts(under).Priority != "none" {
			t.Fatalf("over %s, under %s", w.facts(over).Priority, w.facts(under).Priority)
		}
	})
}

// What a rule in one project waits for can finish in another.
func TestABlockerInAnotherProjectFreesATask(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		elsewhere := uuid.NewString()
		w.push("owner", "project", elsewhere, map[string]any{"name": "Elsewhere", "sort_key": "a1"})
		w.rule("member", w.project, "Free", map[string]any{"on": "unblocked"}, []map[string]any{{"do": "priority", "value": "high"}}, nil)
		blocker := uuid.NewString()
		w.push("member", "task", blocker, map[string]any{"title": "Over there", "project_id": elsewhere, "sort_key": "a0", "priority": "none"})
		waiting := w.task("Waiting", nil)
		w.push("member", "task_dependency", syncer.PairID(waiting, blocker), map[string]any{"task_id": waiting, "blocker_id": blocker})
		w.push("member", "task", blocker, map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.drain()
		if w.facts(waiting).Priority != "high" {
			t.Fatal("it didn't start")
		}
		if w.facts(blocker).Priority != "none" {
			t.Fatal("it ran on a task outside its project")
		}
	})
}

// Email starts a rule when email to task says so, not when a token has that name.
func TestATokenNamedEmailIsNotEmail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.rule("member", w.project, "Mail", map[string]any{"on": "email"}, []map[string]any{{"do": "priority", "value": "low"}}, nil)
		rec, _, err := w.sync.Write(syncer.WithVia(w.ctx, "email"), w.users["member"], w.ws, syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: map[string]json.RawMessage{"title": json.RawMessage(`"By token"`), "project_id": json.RawMessage(`"` + w.project + `"`)}})
		must(t, err)
		w.drain()
		if w.facts(rec.ID).Priority != "none" {
			t.Fatal("a token's name started it")
		}
	})
}

// A rule can't turn IDs into names of people outside its workspace, or tell someone who
// can't see the task and say that it did.
func TestARuleNamesOnlyPeopleItsWorkspaceKnows(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		outsider := uuid.NewString()
		must(t, w.st.CreateUser(w.ctx, sqlitedb.CreateUserParams{ID: outsider, Email: "outsider-" + outsider[:8] + "@example.com", Name: "Secret Name", Now: store.Now()}))
		w.rule("member", w.project, "Nosy", map[string]any{"on": "created"}, []map[string]any{{"do": "assign", "value": outsider}, {"do": "notify", "value": outsider}, {"do": "notify", "value": w.users["guest"]}}, nil)
		w.task("Anything", nil)
		w.drain()
		runs := w.runs()
		if len(runs) != 1 || strings.Contains(runs[0].Detail, "Secret Name") {
			t.Fatalf("runs: %+v", runs)
		}
		if strings.Contains(runs[0].Detail, "Told") || len(w.told.notices) > 1 {
			t.Fatalf("said it told someone who can't see the task: %q, %d notices", runs[0].Detail, len(w.told.notices))
		}
	})
}
