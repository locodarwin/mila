package syncer_test

import (
	"context"
	"slices"
	"testing"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Undoing a batch puts back what it changed and deletes what it made, but
// leaves a field someone has changed since; it is the batch's maker's
// alone, and undoing it twice changes nothing more.
func TestUndoingABatch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		ctx := context.Background()
		garden, todo := w.project("Garden")
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: fields(map[string]any{"title": "Fence", "project_id": garden, "status_id": todo, "priority": "low"})}).ID
		label := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Wood"})}).ID

		const batch = "agent-run-1"
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: task, Mode: syncer.Update, Batch: batch,
			Fields: fields(map[string]any{"title": "Fence, mended", "priority": "high"})})
		made := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Batch: batch,
			Fields: fields(map[string]any{"title": "Buy posts", "project_id": garden, "status_id": todo})}).ID
		tagged := w.mustWrite("owner", syncer.Write{Entity: "task_label", Mode: syncer.Upsert, Batch: batch,
			Fields: fields(map[string]any{"task_id": task, "label_id": label})}).ID
		// Someone else changes the priority after the batch.
		w.mustWrite("member", syncer.Write{Entity: "task", ID: task, Mode: syncer.Update,
			Fields: fields(map[string]any{"priority": "urgent"})})

		// Only its maker may undo it.
		_, err := w.svc.UndoBatch(ctx, w.users["member"], w.ws, batch)
		wantKind(t, err, apperr.NotFound)
		_, err = w.svc.UndoBatch(ctx, w.users["owner"], w.ws, "not a batch")
		wantKind(t, err, apperr.Invalid)

		got, err := w.svc.UndoBatch(ctx, w.users["owner"], w.ws, batch)
		must(t, err)
		if got.Undone != 3 || len(got.Kept) != 1 || got.Kept[0].ID != task || !slices.Equal(got.Kept[0].Fields, []string{"priority"}) {
			t.Fatalf("undone %d, kept %+v", got.Undone, got.Kept)
		}
		rec, err := w.svc.Get(ctx, w.users["owner"], w.ws, "task", task)
		must(t, err)
		if rec.Fields["title"] != "Fence" || rec.Fields["priority"] != "urgent" {
			t.Errorf("task after undo: %v, %v", rec.Fields["title"], rec.Fields["priority"])
		}
		for _, gone := range []struct{ entity, id string }{{"task", made}, {"task_label", tagged}} {
			rec, err := w.svc.Get(ctx, w.users["owner"], w.ws, gone.entity, gone.id)
			must(t, err)
			if rec.Fields["deleted_at"] == nil {
				t.Errorf("%s made by the batch is still there", gone.entity)
			}
		}

		// Again: everything has changed since, so nothing more is undone.
		again, err := w.svc.UndoBatch(ctx, w.users["owner"], w.ws, batch)
		must(t, err)
		if again.Undone != 0 {
			t.Errorf("undone again: %d", again.Undone)
		}
		if rec, _ := w.svc.Get(ctx, w.users["owner"], w.ws, "task", task); rec.Fields["title"] != "Fence" {
			t.Errorf("title after the second undo: %v", rec.Fields["title"])
		}
	})
}

// A batch that changed a field twice puts back what it was before the
// first change, not the middle value.
func TestUndoingABatchThatChangedAFieldTwice(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		ctx := context.Background()
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Fence", "notes": "Start"})}).ID
		const batch = "twice"
		for _, notes := range []string{"First draft", "Second draft"} {
			w.mustWrite("owner", syncer.Write{Entity: "task", ID: task, Mode: syncer.Update, Batch: batch, Fields: fields(map[string]any{"notes": notes})})
		}
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: task, Mode: syncer.Delete, Batch: batch})
		got, err := w.svc.UndoBatch(ctx, w.users["owner"], w.ws, batch)
		must(t, err)
		if len(got.Kept) != 0 {
			t.Errorf("kept %+v", got.Kept)
		}
		rec, err := w.svc.Get(ctx, w.users["owner"], w.ws, "task", task)
		must(t, err)
		if rec.Fields["notes"] != "Start" || rec.Fields["deleted_at"] != nil {
			t.Errorf("after undo: notes %q, deleted_at %v", rec.Fields["notes"], rec.Fields["deleted_at"])
		}
	})
}
