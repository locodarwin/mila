package syncer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// A task's made_at and started_at, what reports read: the server fills
// both in under the edit's own clock, a start stays once set, and tasks
// from before get both from their history.
func TestTaskDates(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		project := uuid.NewString()
		todo, doing, done := uuid.NewString(), uuid.NewString(), uuid.NewString()
		wantStatus(t, w.push("owner",
			op("project", project, later(0), newProject("Garden")),
			op("status", todo, later(0), newStatus(project, "To do", "todo")),
			op("status", doing, later(0), newStatus(project, "Doing", "in-progress")),
			op("status", done, later(0), newStatus(project, "Done", "done")),
		)[3], "applied")
		at := func(fields map[string]any, name string) string {
			v, _ := fields[name].(string)
			return v
		}
		// Made: when the edit that made it was, by its clock.
		task := uuid.NewString()
		made := hlcAt(time.Now().Add(-time.Hour), "owner")
		wantStatus(t, w.push("owner", op("task", task, made, newTask(project, todo, "Prune")))[0], "applied")
		f := w.rowFields("task", task)
		if want := hlcTime(made); !when(f, "made_at").Equal(want) || f["started_at"] != nil {
			t.Fatalf("made = %v, started = %v; want made %s", f["made_at"], f["started_at"], want)
		}

		// Started: the first move to a status of kind in-progress, and it stays.
		start := later(5)
		wantStatus(t, w.push("owner", op("task", task, start, map[string]any{"status_id": doing}))[0], "applied")
		wantStatus(t, w.push("owner", op("task", task, later(6), map[string]any{"status_id": todo}))[0], "applied")
		wantStatus(t, w.push("owner", op("task", task, later(7), map[string]any{"status_id": doing}))[0], "applied")
		wantStatus(t, w.push("owner", op("task", task, later(8), map[string]any{"status_id": done, "completed_at": "2026-11-07T10:00:00Z"}))[0], "applied")
		if got := when(w.rowFields("task", task), "started_at"); !got.Equal(hlcTime(start)) {
			t.Fatalf("started = %s, want %s", got, hlcTime(start))
		}
		// Only the server sets it.
		res := w.push("owner", op("task", task, later(9), map[string]any{"started_at": "2026-01-01T00:00:00Z"}))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "set by the server") {
			t.Fatalf("a device setting started_at: %s %s", res.Status, res.Message)
		}
		// Made in a status of kind in-progress: started as it was made.
		quick := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", quick, later(10), newTask(project, doing, "Water")))[0], "applied")
		if f := w.rowFields("task", quick); at(f, "started_at") == "" || !when(f, "started_at").Equal(when(f, "made_at")) {
			t.Fatalf("made doing: made %v, started %v", f["made_at"], f["started_at"])
		}
		// An import may say when it was made, once.
		old := uuid.NewString()
		imported := newTask(project, todo, "From GitHub")
		imported["made_at"] = "2024-03-04T05:06:07Z"
		wantStatus(t, w.push("owner", op("task", old, later(11), imported))[0], "applied")
		if got := at(w.rowFields("task", old), "made_at"); got != "2024-03-04T05:06:07Z" {
			t.Fatalf("an imported made_at = %q", got)
		}
		res = w.push("owner", op("task", old, later(12), map[string]any{"made_at": "2025-01-01T00:00:00Z"}))[0]
		if res.Status != "rejected" {
			t.Fatalf("changing made_at: %s %s", res.Status, res.Message)
		}
		// By REST: made now, and nothing for started_at sent with it.
		rec, _, err := w.svc.Write(ctx, w.users["owner"], w.ws, syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "By script", "project_id": project, "status_id": doing})})
		must(t, err)
		if f := w.rowFields("task", rec.ID); at(f, "made_at") == "" || at(f, "started_at") == "" {
			t.Fatalf("a REST create in progress: %v", f)
		}

		// From before: the columns empty, as migration 42 leaves them, and
		// filled in from the history.
		for _, q := range []string{"UPDATE tasks SET made_at = NULL, started_at = NULL WHERE id = ?"} {
			if d.Dialect == db.Postgres {
				q = strings.Replace(q, "?", "$1", 1)
			}
			if _, err := d.ExecContext(ctx, q, task); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.FillTaskDates(ctx, d, d.Dialect, ""); err != nil {
			t.Fatal(err)
		}
		f = w.rowFields("task", task)
		if at(f, "made_at") == "" || at(f, "started_at") == "" || when(f, "started_at").Before(when(f, "made_at")) {
			t.Fatalf("filled from history: made %v, started %v", f["made_at"], f["started_at"])
		}
	})
}

// hlcTime is the time an HLC names.
func hlcTime(h string) time.Time {
	ms := int64(0)
	for _, c := range h[:13] {
		ms = ms*10 + int64(c-'0')
	}
	return time.UnixMilli(ms).UTC()
}

// when reads a time field as the API writes it.
func when(f map[string]any, name string) time.Time {
	s, _ := f[name].(string)
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
