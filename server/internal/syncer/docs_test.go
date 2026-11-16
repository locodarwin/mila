package syncer_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/ydoc"
)

// edit is a device's edit to a document: the update that turns the
// document its updates come to into the one given by Markdown.
func edit(t *testing.T, client uint64, md string, base ...[]byte) []byte {
	t.Helper()
	u, err := ydoc.Replace(base, ydoc.Parse(md), client)
	if err != nil || u == nil {
		t.Fatalf("edit to %q: %v", md, err)
	}
	return u
}

func encode(update []byte) string { return base64.StdEncoding.EncodeToString(update) }

func docRow(task string, update []byte) map[string]any {
	return map[string]any{"task_id": task, "data": base64.StdEncoding.EncodeToString(update), "deleted_at": nil}
}

// taskFields is a task's fields as the owner's bootstrap holds them.
func (w *world) taskFields(id string) map[string]any { return w.rowFields("task", id) }

func (w *world) rowFields(entity, id string) map[string]any {
	w.t.Helper()
	snap, err := w.svc.Bootstrap(context.Background(), w.users["owner"], w.ws)
	must(w.t, err)
	for _, r := range snap.Rows {
		if r.Entity == entity && r.ID == id {
			return r.Fields
		}
	}
	w.t.Fatalf("no %s %s", entity, id)
	return nil
}

// docOf is a task's document as its live rows hold it.
func (w *world) docOf(task string) (*ydoc.Node, int) {
	w.t.Helper()
	snap, err := w.svc.Bootstrap(context.Background(), w.users["owner"], w.ws)
	must(w.t, err)
	var updates [][]byte
	for _, r := range snap.Rows {
		if r.Entity == "doc_update" && r.Fields["task_id"] == task && r.Fields["deleted_at"] == nil {
			b, err := base64.StdEncoding.DecodeString(r.Fields["data"].(string))
			must(w.t, err)
			updates = append(updates, b)
		}
	}
	doc, err := ydoc.Read(updates...)
	must(w.t, err)
	return doc, len(updates)
}

func TestADocumentRendersItsNotes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		task := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Beds")))[0], "applied")

		first := edit(t, 11, "Buy **cedar** boards.")
		wantStatus(t, w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, first)))[0], "applied")
		if got := w.taskFields(task)["notes"]; got != "Buy **cedar** boards." {
			t.Fatalf("notes = %q", got)
		}

		// A second device edits from the same start: both edits stay.
		a := edit(t, 11, "Buy **cedar** boards.\n\nCut them to length.", first)
		b := edit(t, 12, "## Beds\n\nBuy **cedar** boards.", first)
		w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, a)))
		w.push("member", op("doc_update", uuid.NewString(), hlcAt(now, "b"), docRow(task, b)))
		want := "## Beds\n\nBuy **cedar** boards.\n\nCut them to length."
		if got := w.taskFields(task)["notes"]; got != want {
			t.Fatalf("merged notes = %q, want %q", got, want)
		}

		// Something that is not a Yjs update is refused.
		res := w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), map[string]any{"task_id": task, "data": "bm90IHlqcw==", "deleted_at": nil}))
		wantStatus(t, res[0], "rejected")
		res = w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), map[string]any{"task_id": task, "data": "!!", "deleted_at": nil}))
		wantStatus(t, res[0], "rejected")
	})
}

func TestAWriteToTheNotesBecomesARowOfTheDocument(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		task := uuid.NewString()
		made := newTask(nil, nil, "Beds")
		made["notes"] = "First draft.\n\n- one\n- two"
		w.push("owner", op("task", task, hlcAt(now, "a"), made))
		doc, rows := w.docOf(task)
		if rows != 1 || ydoc.Markdown(doc) != "First draft.\n\n- one\n- two" {
			t.Fatalf("document = %q in %d rows", ydoc.Markdown(doc), rows)
		}

		// A device edits the list while REST changes the first paragraph:
		// the device's edit survives, in a block REST left alone.
		var base [][]byte
		snap, err := w.svc.Bootstrap(context.Background(), w.users["owner"], w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.Entity == "doc_update" {
				b, _ := base64.StdEncoding.DecodeString(r.Fields["data"].(string))
				base = append(base, b)
			}
		}
		device := edit(t, 21, "First draft.\n\n- one\n- two\n- three", base...)
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: task, Mode: syncer.Update, Fields: fields(map[string]any{"notes": "Second draft.\n\n- one\n- two"})})
		w.push("member", op("doc_update", uuid.NewString(), hlcAt(now.Add(time.Second), "b"), docRow(task, device)))
		want := "Second draft.\n\n- one\n- two\n- three"
		if got := w.taskFields(task)["notes"]; got != want {
			t.Fatalf("notes = %q, want %q", got, want)
		}
		doc, _ = w.docOf(task)
		if got := ydoc.Markdown(doc); got != want {
			t.Fatalf("document = %q, want %q", got, want)
		}
	})
}

func TestNotesHistoryIsOneEntryPerEditingSession(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now().UTC().Truncate(time.Millisecond)
		w.svc.SetNow(func() time.Time { return now })
		task := uuid.NewString()
		w.push("owner", op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Beds")))
		start := now
		var updates [][]byte
		for i, s := range []struct {
			role string
			at   time.Duration
			md   string
		}{
			{"owner", 0, "One"}, {"owner", time.Minute, "One two"}, {"owner", 3 * time.Minute, "One two three"},
			{"member", 4 * time.Minute, "One two three four"},
			{"owner", 5 * time.Minute, "Five"}, {"owner", 12 * time.Minute, "Six"},
		} {
			now = start.Add(s.at)
			u := edit(t, uint64(30+i), s.md, updates...)
			updates = append(updates, u)
			w.push(s.role, op("doc_update", uuid.NewString(), hlcAt(now, s.role), docRow(task, u)))
		}
		events, err := w.svc.Activity(context.Background(), w.users["owner"], w.ws, task)
		must(t, err)
		var notes []string
		for _, e := range events {
			if n, ok := e.Fields["notes"].(string); ok && e.Kind == "changed" {
				notes = append(notes, e.ActorName+": "+n)
			}
		}
		// Newest first: the owner's session after the member's edit, the
		// member's, and the owner's first session of three edits.
		want := []string{"owner: Six", "owner: Five", "member: One two three four", "owner: One two three"}
		if len(notes) != len(want) {
			t.Fatalf("history = %q, want %q", notes, want)
		}
		for i := range want {
			if notes[i] != want[i] {
				t.Fatalf("history = %q, want %q", notes, want)
			}
		}
	})
}

func TestADocumentsRowsAreMerged(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		task := uuid.NewString()
		w.push("owner", op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Long notes")))
		var updates [][]byte
		text := "Start."
		for i := 0; i < 80; i++ {
			text += " More."
			u := edit(t, 40, text, updates...)
			updates = append(updates, u)
			w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, u)))
		}
		doc, rows := w.docOf(task)
		if ydoc.Markdown(doc) != text {
			t.Fatalf("document after merging = %q", ydoc.Markdown(doc))
		}
		if rows >= 64 {
			t.Fatalf("%d rows, not merged", rows)
		}
		// The rows merged away are gone from the database, not in the trash.
		if n := w.count("doc_updates", "task_id = ?", task); n != rows {
			t.Fatalf("%d rows in the database, %d live", n, rows)
		}
		// A device that had the old rows hears they are gone.
		changes, err := w.svc.Pull(context.Background(), w.users["member"], w.ws, 0)
		must(t, err)
		deleted := 0
		for _, c := range changes.Changes {
			if c.Entity == "doc_update" && c.Fields["deleted_at"] != nil && c.Fields["data"] == nil {
				deleted++
			}
		}
		if deleted == 0 {
			t.Fatal("no deletes of merged rows in the log")
		}
		if got := w.taskFields(task)["notes"]; got != text {
			t.Fatalf("notes = %q", got)
		}
	})
}

func TestNotesWithoutADocumentGetOne(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		task := uuid.NewString()
		w.push("owner", op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Old")))
		// Notes from before live documents: in the column, with no rows.
		dd := w.st.DB()
		_, err := dd.ExecContext(context.Background(), rebind(dd, "UPDATE tasks SET notes = ? WHERE id = ?"), "Written in **v1**.\n\n\n\nWith a gap.", task)
		must(t, err)
		n, err := w.svc.SeedDocs(context.Background(), "")
		must(t, err)
		if n != 1 {
			t.Fatalf("seeded %d tasks", n)
		}
		doc, rows := w.docOf(task)
		if rows != 1 || len(doc.Content) != 3 || ydoc.Markdown(doc) != "Written in **v1**.\n\n&nbsp;\n\nWith a gap." {
			t.Fatalf("document = %q in %d rows", ydoc.Markdown(doc), rows)
		}
		// Once is enough.
		if n, _ := w.svc.SeedDocs(context.Background(), w.ws); n != 0 {
			t.Fatalf("seeded %d tasks again", n)
		}
	})
}

func TestAGuestSeesOnlyTheDocumentsOfTheirTasks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		shared, status := w.project("Shared")
		hidden, hiddenStatus := w.project("Hidden")
		w.share(shared, true)
		a, b := uuid.NewString(), uuid.NewString()
		w.push("owner", op("task", a, hlcAt(now, "a"), newTask(shared, status, "Seen")), op("task", b, hlcAt(now, "a"), newTask(hidden, hiddenStatus, "Not seen")))
		w.push("owner",
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(a, edit(t, 50, "Shared notes"))),
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(b, edit(t, 51, "Hidden notes"))))
		snap, err := w.svc.Bootstrap(context.Background(), w.users["guest"], w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.Entity == "doc_update" && r.Fields["task_id"] != a {
				t.Fatalf("the guest got a row of a hidden task's document")
			}
		}
		// The guest writes to the shared task's notes, not the hidden one's.
		wantStatus(t, w.push("guest", op("doc_update", uuid.NewString(), hlcAt(now, "g"), docRow(a, edit(t, 52, "From the guest"))))[0], "applied")
		wantStatus(t, w.push("guest", op("doc_update", uuid.NewString(), hlcAt(now, "g"), docRow(b, edit(t, 53, "Sneaky"))))[0], "rejected")
	})
}
