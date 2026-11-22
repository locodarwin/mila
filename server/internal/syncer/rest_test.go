package syncer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// fields turns plain values into a write's fields.
func fields(values map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range values {
		raw, _ := json.Marshal(v)
		out[k] = raw
	}
	return out
}

func (w *world) write(role string, wr syncer.Write) (*syncer.Record, bool, error) {
	return w.svc.Write(context.Background(), w.users[role], w.ws, wr)
}

func (w *world) mustWrite(role string, wr syncer.Write) *syncer.Record {
	w.t.Helper()
	rec, _, err := w.write(role, wr)
	must(w.t, err)
	return rec
}

func wantKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	if apperr.KindOf(err) != kind {
		t.Fatalf("error = %v (kind %d), want kind %d", err, apperr.KindOf(err), kind)
	}
}

// count counts a table's rows that a WHERE clause picks.
func (w *world) count(table, where string, args ...any) int {
	w.t.Helper()
	d := w.st.DB()
	var n int
	must(w.t, d.QueryRowContext(context.Background(), rebind(d, "SELECT COUNT(*) FROM "+table+" WHERE "+where), args...).Scan(&n))
	return n
}

// project makes a project with a status through REST.
func (w *world) project(name string) (project, status string) {
	p := w.mustWrite("owner", syncer.Write{Entity: "project", Mode: syncer.Create, Fields: fields(map[string]any{"name": name})})
	s := w.mustWrite("owner", syncer.Write{Entity: "status", Mode: syncer.Create, Fields: fields(map[string]any{"project_id": p.ID, "name": "To do", "category": "todo"})})
	return p.ID, s.ID
}

func TestRESTStampObservesTheFieldsItWrites(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now().UTC().Truncate(time.Millisecond)
		w.svc.SetNow(func() time.Time { return now })
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Buy seeds"})})
		// A device 50 seconds ahead writes the title.
		ahead := hlcAt(now.Add(50*time.Second), "fast")
		wantStatus(t, w.push("owner", op("task", task.ID, ahead, map[string]any{"title": "From the future"}))[0], "applied")

		rec := w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"title": "Mine"})})
		if rec.Fields["title"] != "Mine" || rec.Clocks["title"] <= ahead || !strings.HasSuffix(rec.Clocks["title"], "-api") {
			t.Fatalf("after a REST write over a clock ahead: title %v at %s, the device's was %s", rec.Fields["title"], rec.Clocks["title"], ahead)
		}
		// Another field of the row is stamped now, not after the skewed one.
		rec = w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"notes": "Tomatoes"})})
		if want := fmt.Sprintf("%013d-", now.UnixMilli()); !strings.HasPrefix(rec.Clocks["notes"], want) {
			t.Fatalf("notes stamped %s, want the millisecond %s", rec.Clocks["notes"], want)
		}

		// Two writes to one field in the same millisecond both apply, in order.
		first := w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"notes": "One"})})
		second := w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"notes": "Two"})})
		if second.Fields["notes"] != "Two" || second.Clocks["notes"] <= first.Clocks["notes"] {
			t.Fatalf("same-millisecond writes: %v at %s after %s", second.Fields["notes"], second.Clocks["notes"], first.Clocks["notes"])
		}
	})
}

func TestStampAfter(t *testing.T) {
	now := time.UnixMilli(1793237400000)
	cases := []struct {
		clocks map[string]string
		fields []string
		want   string
	}{
		{nil, []string{"title"}, "1793237400000-0000-api"},
		{map[string]string{"title": "1793237399000-0005-x"}, []string{"title"}, "1793237400000-0000-api"},
		{map[string]string{"title": "1793237400000-0000-x"}, []string{"title"}, "1793237400000-0001-api"},
		{map[string]string{"title": "1793237450000-0003-x", "notes": "1793237460000-0000-y"}, []string{"title"}, "1793237450000-0004-api"},
		{map[string]string{"title": "1793237450000-ffff-x"}, []string{"title"}, "1793237450001-0000-api"},
		{map[string]string{"title": "1793237450000-0003-x", "notes": "1793237450000-0007-y"}, []string{"title", "notes"}, "1793237450000-0008-api"},
	}
	for _, c := range cases {
		if got := syncer.StampAfter(now, c.clocks, c.fields); got != c.want {
			t.Errorf("stampAfter(%v, %v) = %s, want %s", c.clocks, c.fields, got, c.want)
		}
	}
}

func TestRESTIfMatch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Buy seeds"})})
		etag := syncer.ETag(task.Clocks)
		changed := w.mustWrite("member", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, IfMatch: etag, Fields: fields(map[string]any{"title": "Buy more seeds"})})
		if syncer.ETag(changed.Clocks) == etag {
			t.Fatal("the ETag stayed the same after a change")
		}
		logged := w.count("change_log", "workspace_id = ?", w.ws)
		_, _, err := w.write("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, IfMatch: etag, Fields: fields(map[string]any{"title": "Stale"})})
		wantKind(t, err, apperr.PreconditionFailed)
		_, _, err = w.write("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Delete, IfMatch: etag})
		wantKind(t, err, apperr.PreconditionFailed)
		if n := w.count("change_log", "workspace_id = ?", w.ws); n != logged {
			t.Fatalf("a refused write logged %d changes", n-logged)
		}

		// Writers with the same version at once: exactly one wins. On
		// Postgres this needs the row lock; SQLite's writes take turns.
		for round := range 5 {
			current, err := w.svc.Get(context.Background(), w.users["owner"], w.ws, "task", task.ID)
			must(t, err)
			etag := syncer.ETag(current.Clocks)
			var wg sync.WaitGroup
			errs := make([]error, 8)
			for i := range errs {
				wg.Go(func() {
					_, _, errs[i] = w.write("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, IfMatch: etag,
						Fields: fields(map[string]any{"title": fmt.Sprintf("Racer %d.%d", round, i)})})
				})
			}
			wg.Wait()
			won := 0
			for _, err := range errs {
				if err == nil {
					won++
				} else {
					wantKind(t, err, apperr.PreconditionFailed)
				}
			}
			if won != 1 {
				t.Fatalf("round %d: %d of %d racing writes with the same If-Match went through, want 1", round, won, len(errs))
			}
		}
	})
}

func TestRESTIdempotencyKey(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		p, s := w.project("Garden")
		create := syncer.Write{Entity: "task", Mode: syncer.Create, Key: "retry-me",
			Fields: fields(map[string]any{"title": "Plant", "project_id": p, "status_id": s})}
		first, created, err := w.write("owner", create)
		must(t, err)
		again, createdAgain, err := w.write("owner", create)
		must(t, err)
		if !created || createdAgain || again.ID != first.ID {
			t.Fatalf("retry: created %v then %v, IDs %s and %s", created, createdAgain, first.ID, again.ID)
		}
		if n := w.count("change_log", "entity_id = ?", first.ID); n != 1 {
			t.Fatalf("a retried create logged %d changes", n)
		}
		if n := w.count("activity", "entity_id = ?", first.ID); n != 1 {
			t.Fatalf("a retried create wrote %d history entries", n)
		}
		// The same key from someone else is their own.
		theirs, created, err := w.write("member", create)
		must(t, err)
		if !created || theirs.ID == first.ID {
			t.Fatalf("another user's key made %v, row %s (mine %s)", created, theirs.ID, first.ID)
		}
	})
}

func TestRESTCreates(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		p, s := w.project("Garden")
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Plant", "project_id": p, "status_id": s})})

		// The log entry of a create holds every field, defaults and all.
		changes, err := w.svc.Pull(ctx, w.users["owner"], w.ws, 0)
		must(t, err)
		var logged *syncer.Change
		for i, c := range changes.Changes {
			if c.ID == task.ID {
				logged = &changes.Changes[i]
			}
		}
		info := describe(t, "task")
		// Every field but those only the server sets, which a new task in a
		// status of kind todo has none of yet (started_at).
		want := 0
		for _, f := range info.Fields {
			if !f.ServerSet {
				want++
			}
		}
		if logged == nil || len(logged.Fields) != want {
			t.Fatalf("the create's log entry = %v, want all %d fields", logged, want)
		}
		if logged.Fields["created_by"] != w.users["owner"] || logged.Fields["priority"] != "none" || logged.Fields["sort_key"] != "a0" {
			t.Fatalf("filled fields = %v", logged.Fields)
		}

		// Racing creates of one ID: one makes it, the rest are told it is
		// taken (on Postgres, some by the unique key).
		racing := uuid.NewString()
		errs := make([]error, 8)
		var wg sync.WaitGroup
		for i := range errs {
			wg.Go(func() {
				_, _, errs[i] = w.write("owner", syncer.Write{Entity: "task", ID: racing, Mode: syncer.Create, Fields: fields(map[string]any{"title": "Race"})})
			})
		}
		wg.Wait()
		made := 0
		for _, err := range errs {
			if err == nil {
				made++
			} else {
				wantKind(t, err, apperr.Conflict)
			}
		}
		if made != 1 {
			t.Fatalf("%d racing creates of one ID went through", made)
		}

		// Taken IDs and bad pairs.
		_, _, err = w.write("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Create, Fields: fields(map[string]any{"title": "Again"})})
		wantKind(t, err, apperr.Conflict)
		label := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Urgent"})})
		_, _, err = w.write("owner", syncer.Write{Entity: "task_label", ID: uuid.NewString(), Mode: syncer.Upsert,
			Fields: fields(map[string]any{"task_id": task.ID, "label_id": label.ID})})
		wantKind(t, err, apperr.Invalid)

		// Adding a pair twice is the same row, brought back if deleted.
		pair := syncer.Write{Entity: "task_label", Mode: syncer.Upsert, Fields: fields(map[string]any{"task_id": task.ID, "label_id": label.ID})}
		rec, created, err := w.write("owner", pair)
		must(t, err)
		if !created || rec.ID != syncer.PairID(task.ID, label.ID) {
			t.Fatalf("pair created %v with ID %s", created, rec.ID)
		}
		w.mustWrite("owner", syncer.Write{Entity: "task_label", ID: rec.ID, Mode: syncer.Delete})
		rec, created, err = w.write("owner", pair)
		must(t, err)
		if created || rec.Fields["deleted_at"] != nil {
			t.Fatalf("adding a deleted pair again: created %v, %v", created, rec.Fields)
		}

		// My Day: adding a task again keeps its place.
		day := syncer.Write{Entity: "my_day_entry", Mode: syncer.Upsert, Fields: fields(map[string]any{"task_id": task.ID, "day": "2026-10-28"})}
		entry := w.mustWrite("member", day)
		other := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Water"})})
		w.mustWrite("member", syncer.Write{Entity: "my_day_entry", Mode: syncer.Upsert,
			Fields: fields(map[string]any{"task_id": other.ID, "day": "2026-10-28"})})
		_, _, err = w.write("member", syncer.Write{Entity: "my_day_entry", Mode: syncer.Upsert,
			Fields: fields(map[string]any{"task_id": task.ID, "day": "2026-10-28", "user_id": w.users["owner"]})})
		wantKind(t, err, apperr.Forbidden)
		again := w.mustWrite("member", day)
		if again.ID != entry.ID || again.Fields["sort_key"] != entry.Fields["sort_key"] || again.Fields["user_id"] != w.users["member"] {
			t.Fatalf("My Day entry added again: %v, was %v", again.Fields, entry.Fields)
		}

		// An unchanged write is the newest word on its fields, but no news
		// for the task's history.
		history := w.count("activity", "task_id = ?", task.ID)
		same := w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"title": "Plant", "priority": "none"})})
		if same.Clocks["title"] <= task.Clocks["title"] || w.count("activity", "task_id = ?", task.ID) != history {
			t.Fatalf("an unchanged write: title clock %s from %s, %d history entries from %d",
				same.Clocks["title"], task.Clocks["title"], w.count("activity", "task_id = ?", task.ID), history)
		}
		// Deleting twice keeps the first time, and makes one history entry.
		deleted := w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Delete})
		history = w.count("activity", "task_id = ?", task.ID)
		again = w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Delete})
		if again.Fields["deleted_at"] != deleted.Fields["deleted_at"] || again.Clocks["deleted_at"] <= deleted.Clocks["deleted_at"] ||
			w.count("activity", "task_id = ?", task.ID) != history {
			t.Fatalf("deleting a deleted task: %v at %s, was %v at %s", again.Fields["deleted_at"], again.Clocks["deleted_at"], deleted.Fields["deleted_at"], deleted.Clocks["deleted_at"])
		}
	})
}

// A REST write that leaves a value as it is still wins over an older edit
// a device made offline, which arrives after it: for a field, for a
// delete of what is deleted, and for adding a pair that is there.
func TestRESTUnchangedWritesStillWin(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now().UTC().Truncate(time.Millisecond)
		at := func(offset time.Duration) { w.svc.SetNow(func() time.Time { return now.Add(offset) }) }
		at(0)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "A"})})
		label := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Urgent"})})
		pair := w.mustWrite("owner", syncer.Write{Entity: "task_label", Mode: syncer.Upsert, Fields: fields(map[string]any{"task_id": task.ID, "label_id": label.ID})})
		other := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Gone"})})
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: other.ID, Mode: syncer.Delete})

		// Offline at +1s, a phone renames the task, restores the deleted
		// one and removes the label.
		offline := []syncer.Op{
			op("task", task.ID, hlcAt(now.Add(time.Second), "phone"), map[string]any{"title": "B"}),
			op("task", other.ID, hlcAt(now.Add(time.Second), "phone"), map[string]any{"deleted_at": nil}),
			op("task_label", pair.ID, hlcAt(now.Add(time.Second), "phone"), map[string]any{"deleted_at": now.Add(time.Second).Format(time.RFC3339Nano)}),
		}
		// At +5s a script says the same as the server holds.
		at(5 * time.Second)
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: task.ID, Mode: syncer.Update, Fields: fields(map[string]any{"title": "A"})})
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: other.ID, Mode: syncer.Delete})
		w.mustWrite("owner", syncer.Write{Entity: "task_label", Mode: syncer.Upsert, Fields: fields(map[string]any{"task_id": task.ID, "label_id": label.ID})})
		// Then the phone comes back.
		for _, r := range w.push("member", offline...) {
			wantStatus(t, r, "applied")
		}
		ctx := context.Background()
		got, err := w.svc.Get(ctx, w.users["owner"], w.ws, "task", task.ID)
		must(t, err)
		gone, err := w.svc.Get(ctx, w.users["owner"], w.ws, "task", other.ID)
		must(t, err)
		labeled, err := w.svc.Get(ctx, w.users["owner"], w.ws, "task_label", pair.ID)
		must(t, err)
		if got.Fields["title"] != "A" || gone.Fields["deleted_at"] == nil || labeled.Fields["deleted_at"] != nil {
			t.Fatalf("an older offline edit undid the script's: title %v, deleted %v, label removed %v",
				got.Fields["title"], gone.Fields["deleted_at"], labeled.Fields["deleted_at"])
		}
	})
}

func TestRESTRetryAfterLosingSight(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		p, s := w.project("Shared")
		w.share(p, true)
		create := syncer.Write{Entity: "task", Mode: syncer.Create, Key: "k1",
			Fields: fields(map[string]any{"title": "Mine", "project_id": p, "status_id": s})}
		made := w.mustWrite("guest", create)
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: made.ID, Mode: syncer.Update, Fields: fields(map[string]any{"notes": "Private now"})})
		w.share(p, false)
		_, _, err := w.write("guest", create)
		wantKind(t, err, apperr.NotFound)
	})
}

func TestRESTSelfFieldsNameOnlyTheCaller(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Plant"})})
		quiet := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Water"})})
		w.mustWrite("owner", syncer.Write{Entity: "my_day_entry", Mode: syncer.Upsert, Fields: fields(map[string]any{"task_id": task.ID, "day": "2026-10-28"})})
		// Whether or not the owner has the task in their day, the answer is
		// the same.
		for _, id := range []string{task.ID, quiet.ID} {
			_, _, err := w.write("member", syncer.Write{Entity: "my_day_entry", Mode: syncer.Upsert,
				Fields: fields(map[string]any{"task_id": id, "day": "2026-10-28", "user_id": w.users["owner"]})})
			wantKind(t, err, apperr.Forbidden)
		}
		_, _, err := w.write("member", syncer.Write{Entity: "comment", Mode: syncer.Create,
			Fields: fields(map[string]any{"task_id": task.ID, "body": "Hi", "author_id": w.users["owner"]})})
		wantKind(t, err, apperr.Forbidden)
		w.mustWrite("member", syncer.Write{Entity: "saved_view", Mode: syncer.Create,
			Fields: fields(map[string]any{"name": "Shared", "type": "list", "config": map[string]any{}, "owner_id": nil})})
	})
}

func TestRESTPairUpsertRace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Plant"})})
		label := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Urgent"})})
		errs := make([]error, 8)
		made := make([]bool, 8)
		var wg sync.WaitGroup
		for i := range errs {
			wg.Go(func() {
				_, made[i], errs[i] = w.write("owner", syncer.Write{Entity: "task_label", Mode: syncer.Upsert,
					Fields: fields(map[string]any{"task_id": task.ID, "label_id": label.ID})})
			})
		}
		wg.Wait()
		n := 0
		for i, err := range errs {
			must(t, err)
			if made[i] {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%d of 8 racing adds of one pair made it", n)
		}
	})
}

func TestRESTCommentEdits(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Plant"})})
		c := w.mustWrite("owner", syncer.Write{Entity: "comment", Mode: syncer.Create, Fields: fields(map[string]any{"task_id": task.ID, "body": "Hi"})})
		if c.Fields["edited_at"] != nil {
			t.Fatal("a new comment is marked edited")
		}
		same := w.mustWrite("owner", syncer.Write{Entity: "comment", ID: c.ID, Mode: syncer.Update, Fields: fields(map[string]any{"body": "Hi"})})
		if same.Fields["edited_at"] != nil {
			t.Fatal("a comment written with the same words is marked edited")
		}
		edited := w.mustWrite("owner", syncer.Write{Entity: "comment", ID: c.ID, Mode: syncer.Update, Fields: fields(map[string]any{"body": "Hello"})})
		if edited.Fields["edited_at"] == nil {
			t.Fatal("a comment whose words changed is not marked edited")
		}
	})
}

func TestETagMatches(t *testing.T) {
	etag := `"abc"`
	cases := []struct {
		header      string
		weak, match bool
	}{
		{`"abc"`, false, true},
		{`*`, false, true},
		{`"x", "abc"`, false, true},
		{`"x","y"`, false, false},
		{`W/"abc"`, false, false},
		{`W/"abc"`, true, true},
		{`"x", W/"abc"`, true, true},
		{`abc`, true, false},
	}
	for _, c := range cases {
		if got := syncer.ETagMatches(c.header, etag, c.weak); got != c.match {
			t.Errorf("ETagMatches(%q, weak %v) = %v", c.header, c.weak, got)
		}
	}
}

func describe(t *testing.T, name string) syncer.EntityInfo {
	for _, e := range syncer.Describe() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("Describe has no %s", name)
	return syncer.EntityInfo{}
}

func TestRESTSight(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		shared, s := w.project("Shared")
		private, _ := w.project("Private")
		w.share(shared, true)
		mine := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Seen", "project_id": shared, "status_id": s})})
		hidden := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Hidden", "project_id": private})})
		inbox := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Inbox"})})

		// A guest neither changes nor reads what they cannot see.
		for _, id := range []string{hidden.ID, inbox.ID} {
			_, _, err := w.write("guest", syncer.Write{Entity: "task", ID: id, Mode: syncer.Update, Fields: fields(map[string]any{"title": "Mine now"})})
			wantKind(t, err, apperr.NotFound)
			_, err = w.svc.Get(ctx, w.users["guest"], w.ws, "task", id)
			wantKind(t, err, apperr.NotFound)
		}
		w.mustWrite("guest", syncer.Write{Entity: "task", ID: mine.ID, Mode: syncer.Update, Fields: fields(map[string]any{"title": "Seen and changed"})})
		_, _, err := w.write("guest", syncer.Write{Entity: "task", ID: mine.ID, Mode: syncer.Update, Fields: fields(map[string]any{"project_id": private, "status_id": nil})})
		wantKind(t, err, apperr.Forbidden)
		_, _, err = w.write("guest", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Mine"})})
		wantKind(t, err, apperr.Forbidden)

		// Lists show a guest only what they see, a page at a time.
		for range 3 {
			w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "More", "project_id": shared, "status_id": s})})
			w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Hidden", "project_id": private})})
		}
		var got []string
		after := ""
		for pages := 0; ; pages++ {
			page, err := w.svc.List(ctx, w.users["guest"], w.ws, "task", syncer.Query{After: after, Limit: 2})
			must(t, err)
			if len(page.Rows) != 2 && page.Next != "" {
				t.Fatalf("page %d holds %d rows and a next", pages, len(page.Rows))
			}
			for _, r := range page.Rows {
				if r.Fields["project_id"] != shared {
					t.Fatalf("the guest's list shows %v", r.Fields)
				}
				got = append(got, r.ID)
			}
			if page.Next == "" {
				break
			}
			after = page.Next
		}
		if len(got) != 4 {
			t.Fatalf("the guest listed %d tasks, want 4", len(got))
		}
		// Filters, null included.
		page, err := w.svc.List(ctx, w.users["owner"], w.ws, "task", syncer.Query{Filters: map[string]string{"project_id": ""}})
		must(t, err)
		if len(page.Rows) != 1 || page.Rows[0].ID != inbox.ID {
			t.Fatalf("the inbox lists %d tasks", len(page.Rows))
		}
		_, err = w.svc.List(ctx, w.users["owner"], w.ws, "task", syncer.Query{Filters: map[string]string{"title": "Seen"}})
		wantKind(t, err, apperr.Invalid)
		_, err = w.svc.List(ctx, w.users["owner"], w.ws, "task", syncer.Query{Filters: map[string]string{"priority": "dire"}})
		wantKind(t, err, apperr.Invalid)

		// Someone's reminder is theirs alone.
		reminder := w.mustWrite("member", syncer.Write{Entity: "reminder", Mode: syncer.Create,
			Fields: fields(map[string]any{"task_id": mine.ID, "remind_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})})
		_, err = w.svc.Get(ctx, w.users["owner"], w.ws, "reminder", reminder.ID)
		wantKind(t, err, apperr.NotFound)
		_, _, err = w.write("owner", syncer.Write{Entity: "reminder", ID: reminder.ID, Mode: syncer.Delete})
		wantKind(t, err, apperr.NotFound)
		page, err = w.svc.List(ctx, w.users["owner"], w.ws, "reminder", syncer.Query{})
		must(t, err)
		if len(page.Rows) != 0 {
			t.Fatalf("the owner lists %d of someone else's reminders", len(page.Rows))
		}
		// Strangers learn nothing about the workspace.
		_, err = w.svc.List(ctx, uuid.NewString(), w.ws, "task", syncer.Query{})
		wantKind(t, err, apperr.NotFound)
	})
}

func TestRESTDefaultsAreTheDatabases(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		wrong, err := w.svc.BareDefaults(context.Background(), w.ws, w.users["owner"])
		must(t, err)
		for _, msg := range wrong {
			t.Error(msg)
		}
	})
}

func TestRESTSortKeys(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/sortkey-vectors.json")
	must(t, err)
	var v struct {
		First   string
		After   [][2]string
		Invalid []string
	}
	must(t, json.Unmarshal(raw, &v))
	if got, _ := syncer.KeyAfter(""); got != v.First {
		t.Errorf("the first key = %s, want %s", got, v.First)
	}
	for _, c := range v.After {
		if got, err := syncer.KeyAfter(c[0]); err != nil || got != c[1] {
			t.Errorf("keyAfter(%s) = %s, %v; want %s", c[0], got, err, c[1])
		}
		if err := syncer.ValidateKey(c[1]); err != nil {
			t.Errorf("%s should be valid: %v", c[1], err)
		}
	}
	for _, k := range v.Invalid {
		if syncer.ValidateKey(k) == nil {
			t.Errorf("%q should be invalid", k)
		}
	}

	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		// "a0" sorts after "Zz" byte by byte, though not in most locales.
		wantStatus(t, w.push("owner", op("label", uuid.NewString(), hlcAt(now, "a"), map[string]any{"name": "One", "color": "red", "sort_key": "a0"}))[0], "applied")
		wantStatus(t, w.push("owner", op("label", uuid.NewString(), hlcAt(now, "a"), map[string]any{"name": "Two", "color": "red", "sort_key": "Zz"}))[0], "applied")
		label := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Three"})})
		if label.Fields["sort_key"] != "a1" {
			t.Fatalf("a new label's key = %v, want a1", label.Fields["sort_key"])
		}
		_, _, err := w.write("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Bad", "sort_key": "a00"})})
		wantKind(t, err, apperr.Invalid)
		// Sync refuses a key the server cannot follow, as REST does, and
		// takes every key the app could make.
		for _, k := range v.Invalid {
			res := w.push("owner", op("label", uuid.NewString(), hlcAt(now, "a"), map[string]any{"name": "Odd", "color": "red", "sort_key": k}))[0]
			wantStatus(t, res, "rejected")
		}
		for _, c := range v.After[:5] {
			wantStatus(t, w.push("owner", op("label", uuid.NewString(), hlcAt(now, "a"), map[string]any{"name": "Fine", "color": "red", "sort_key": c[1]}))[0], "applied")
		}
		// A key from before the check, which the server cannot follow,
		// is passed over: a new row goes after the largest valid key.
		odd := uuid.NewString()
		wantStatus(t, w.push("owner", op("label", odd, hlcAt(now, "a"), map[string]any{"name": "Odd", "color": "red", "sort_key": "a5"}))[0], "applied")
		_, err = d.ExecContext(context.Background(), rebind(d, "UPDATE labels SET sort_key = ? WHERE id = ?"), "~~~", odd)
		must(t, err)
		four := w.mustWrite("owner", syncer.Write{Entity: "label", Mode: syncer.Create, Fields: fields(map[string]any{"name": "Four"})})
		if four.Fields["sort_key"] != "b01" { // after b00, the largest valid key
			t.Fatalf("a label after a key the server cannot follow got %v, want b01", four.Fields["sort_key"])
		}
	})
}

func TestDescribeCoversEveryEntity(t *testing.T) {
	want := []string{"project", "status", "label", "custom_field", "saved_view", "task", "task_label", "task_dependency", "task_assignee", "checklist_item", "comment", "page", "page_thread", "page_comment", "attachment", "page_attachment", "page_drawing", "drawing_shape", "my_day_entry", "reminder", "custom_field_value", "template", "time_entry", "project_time_entry", "workspace_theme", "automation"}
	got := syncer.Describe()
	if len(got) != len(want) {
		t.Fatalf("Describe lists %d entities, want %d", len(got), len(want))
	}
	for i, e := range got {
		if e.Name != want[i] || e.Collection == "" || e.Doc == "" || len(e.Fields) == 0 {
			t.Errorf("entity %d = %+v", i, e)
		}
		for _, f := range e.Fields {
			if !f.Required && !f.HasDefault {
				t.Errorf("%s.%s is optional with no default", e.Name, f.Name)
			}
		}
	}
}
