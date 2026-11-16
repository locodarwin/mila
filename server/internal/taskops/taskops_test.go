package taskops_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/taskops"
)

// world is a workspace with its owner and two projects, each with statuses
// whose first is not To do, so a task landing in its first by mistake shows.
type world struct {
	t     *testing.T
	ctx   context.Context
	sync  *syncer.Service
	ops   taskops.Ops
	ws    string
	owner string
}

type project struct{ id, backlog, todo, done string }

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	w := &world{t: t, ctx: ctx, sync: syncer.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil), ws: uuid.NewString(), owner: uuid.NewString()}
	now := store.Now()
	if err := st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Home", Slug: "home-" + w.ws[:8], Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: w.owner, Email: "o-" + w.owner[:8] + "@example.com", Name: "Owner", Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: w.owner, Role: "owner", Now: now}); err != nil {
		t.Fatal(err)
	}
	w.ops = taskops.Ops{Sync: w.sync, Now: time.Now, Write: func(ctx context.Context, wr syncer.Write) error {
		_, _, err := w.sync.Write(ctx, w.owner, w.ws, wr)
		return err
	}}
	return w
}

func (w *world) write(entity, id string, mode syncer.WriteMode, fields map[string]any) string {
	w.t.Helper()
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		raw[k] = taskops.Raw(v)
	}
	rec, _, err := w.sync.Write(w.ctx, w.owner, w.ws, syncer.Write{Entity: entity, ID: id, Mode: mode, Fields: raw})
	if err != nil {
		w.t.Fatalf("%s %v: %v", entity, fields, err)
	}
	return rec.ID
}

func (w *world) project(name string) project {
	p := project{id: w.write("project", "", syncer.Create, map[string]any{"name": name})}
	for i, s := range []struct {
		id       *string
		name, of string
	}{{&p.backlog, "Backlog", "backlog"}, {&p.todo, "To do", "todo"}, {&p.done, "Done", "done"}} {
		*s.id = w.write("status", "", syncer.Create, map[string]any{"project_id": p.id, "name": s.name, "category": s.of, "sort_key": "a" + string(rune('0'+i))})
	}
	return p
}

func (w *world) facts(id string) syncer.TaskFacts {
	w.t.Helper()
	f, there, err := w.sync.TaskFactsOf(w.ctx, id)
	if err != nil || !there {
		w.t.Fatalf("task %s: %v, %v", id, there, err)
	}
	return f
}

func TestMovingTakesSubtasksToTheStatusOfTheSameKind(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		garden := w.project("Garden")
		inbox := w.write("task", "", syncer.Create, map[string]any{"title": "Fence"})
		sub := w.write("task", "", syncer.Create, map[string]any{"title": "Posts", "parent_id": inbox})

		// From the Inbox, open tasks land in To do, not the first status.
		f := w.facts(inbox)
		if _, err := w.ops.Move(w.ctx, w.ws, &f, garden.id); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{inbox, sub} {
			if got := w.facts(id); got.ProjectID != garden.id || got.StatusID != garden.todo {
				t.Errorf("%s: project %q status %q, want %q %q", id, got.ProjectID, got.StatusID, garden.id, garden.todo)
			}
		}

		// A subtask moves only with its task.
		s := w.facts(sub)
		if _, err := w.ops.Move(w.ctx, w.ws, &s, ""); err == nil {
			t.Error("a subtask moved on its own")
		}

		// Back to the Inbox: no project and no status, for both.
		f = w.facts(inbox)
		if _, err := w.ops.Move(w.ctx, w.ws, &f, ""); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{inbox, sub} {
			if got := w.facts(id); got.ProjectID != "" || got.StatusID != "" {
				t.Errorf("%s: project %q status %q, want the Inbox", id, got.ProjectID, got.StatusID)
			}
		}
	})
}

func TestCompletingAndReopeningKeepTheStatusInStep(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		garden := w.project("Garden")
		today := time.Now().Format(time.DateOnly)
		task := w.write("task", "", syncer.Create, map[string]any{"title": "Water", "project_id": garden.id, "status_id": garden.backlog,
			"due_date": today, "recurrence": "FREQ=DAILY", "recurrence_mode": recur.Schedule})
		w.write("checklist_item", "", syncer.Create, map[string]any{"task_id": task, "text": "Tomatoes", "done": true})

		f := w.facts(task)
		if _, err := w.ops.Complete(w.ctx, &f, today); err != nil {
			t.Fatal(err)
		}
		got := w.facts(task)
		if !got.Completed || got.StatusID != garden.done || got.SeriesID != task {
			t.Errorf("completed: %v status %q series %q", got.Completed, got.StatusID, got.SeriesID)
		}
		next := w.facts(recur.NextTaskID(task))
		if next.Completed || next.StatusID != garden.todo || next.DueDate <= today || next.SeriesID != task {
			t.Errorf("next: completed %v status %q due %q series %q", next.Completed, next.StatusID, next.DueDate, next.SeriesID)
		}
		if len(next.Checklist) != 1 || next.Checklist[0].Text != "Tomatoes" {
			t.Errorf("next checklist: %+v", next.Checklist)
		}

		// Reopened, it goes back to To do; the next one stays.
		if _, err := w.ops.Reopen(w.ctx, &got, today); err != nil {
			t.Fatal(err)
		}
		if again := w.facts(task); again.Completed || again.StatusID != garden.todo {
			t.Errorf("reopened: completed %v status %q", again.Completed, again.StatusID)
		}
		if _, there, _ := w.sync.TaskFactsOf(w.ctx, recur.NextTaskID(task)); !there {
			t.Error("the next task went with the reopening")
		}

		// Completed again, the same next task, not a second.
		f = w.facts(task)
		if _, err := w.ops.Complete(w.ctx, &f, today); err != nil {
			t.Fatal(err)
		}
		if _, there, _ := w.sync.TaskFactsOf(w.ctx, recur.NextTaskID(recur.NextTaskID(task))); there {
			t.Error("completing twice made a second next task")
		}

		// In the Inbox, completing touches no status.
		inbox := w.write("task", "", syncer.Create, map[string]any{"title": "Call"})
		f = w.facts(inbox)
		if _, err := w.ops.Complete(w.ctx, &f, today); err != nil {
			t.Fatal(err)
		}
		if got := w.facts(inbox); !got.Completed || got.StatusID != "" {
			t.Errorf("inbox: completed %v status %q", got.Completed, got.StatusID)
		}
		if _, err := w.ops.Reopen(w.ctx, &f, today); err != nil {
			t.Fatal(err)
		}
		if got := w.facts(inbox); got.Completed {
			t.Error("inbox task still completed")
		}
	})
}
