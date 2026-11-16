package syncer_test

import (
	"context"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// The weekly report counts tasks: it leaves their notes, which may be
// long, unread.
func TestTasksCanLeaveNotesUnread(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		home, status := w.project("Home")
		w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{
			"title": "Paint", "notes": "Two coats, the blue.", "project_id": home, "status_id": status})})
		for _, lean := range []bool{false, true} {
			tasks, err := w.svc.Tasks(ctx, w.users["owner"], w.ws, syncer.TaskQuery{NoNotes: lean})
			must(t, err)
			want := "Two coats, the blue."
			if lean {
				want = ""
			}
			if len(tasks) != 1 || tasks[0].Fields["title"] != "Paint" || tasks[0].Fields["notes"] != want {
				t.Fatalf("NoNotes %v: %v", lean, tasks)
			}
		}
	})
}
