package syncer_test

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// TestCompactionSendsLateDevicesBack has a laptop go offline with an
// edit unsent while the log moves on and is compacted. When it comes
// back it pushes, is told to bootstrap, and its edit is in what it gets;
// a device that kept up is not disturbed.
func TestCompactionSendsLateDevicesBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner := w.users["owner"]
		project, task := uuid.NewString(), uuid.NewString()
		now := time.Now()
		for _, r := range w.push("owner",
			op("project", project, hlcAt(now, "own1"), newProject("Garden")),
			op("task", task, hlcAt(now.Add(time.Millisecond), "own1"), newTask(project, nil, "Water")),
		) {
			wantStatus(t, r, "applied")
		}
		snap, err := w.svc.Bootstrap(ctx, owner, w.ws)
		must(t, err)
		laptopSeq := snap.Seq
		// Made offline, sent only when the laptop is back.
		unsent := op("task", task, hlcAt(now.Add(2*time.Millisecond), "lap1"), map[string]any{"title": "Water the beans"})

		for _, r := range w.push("owner",
			op("task", uuid.NewString(), hlcAt(now.Add(3*time.Millisecond), "own1"), newTask(project, nil, "Weed")),
			op("task", task, hlcAt(now.Add(4*time.Millisecond), "own1"), map[string]any{"notes": "Before noon"}),
		) {
			wantStatus(t, r, "applied")
		}
		current, err := w.st.GetChangeSeq(ctx, w.ws)
		must(t, err)

		// Nothing is old enough yet.
		got, err := w.svc.Compact(ctx, time.Now())
		must(t, err)
		if got.Changes != 0 {
			t.Fatalf("compacted %d fresh changes", got.Changes)
		}
		got, err = w.svc.Compact(ctx, time.Now().Add(syncer.LogKeep+time.Hour))
		must(t, err)
		if got.Changes != current {
			t.Fatalf("compacted %d changes, want %d", got.Changes, current)
		}

		// A device that had pulled everything carries on.
		if _, err := w.svc.Pull(ctx, owner, w.ws, current); err != nil {
			t.Fatalf("pull from the top of the log: %v", err)
		}
		// The laptop sends its edit first, as the app does on reconnect,
		// then is told it is too far behind.
		wantStatus(t, w.push("owner", unsent)[0], "applied")
		if _, err := w.svc.Pull(ctx, owner, w.ws, laptopSeq); apperr.KindOf(err) != apperr.Gone {
			t.Fatalf("pull from below the floor = %v, want Gone", err)
		}
		snap, err = w.svc.Bootstrap(ctx, owner, w.ws)
		must(t, err)
		var title, notes any
		for _, r := range snap.Rows {
			if r.ID == task {
				title, notes = r.Fields["title"], r.Fields["notes"]
			}
		}
		if title != "Water the beans" || notes != "Before noon" {
			t.Fatalf("after bootstrapping again the task is %q / %q, want the laptop's title and the owner's notes", title, notes)
		}
		if _, err := w.svc.Pull(ctx, owner, w.ws, snap.Seq); err != nil {
			t.Fatalf("pull after bootstrapping again: %v", err)
		}

		// Long after, the op IDs are forgotten too; the laptop's edit sent
		// again (its answer lost) still changes nothing.
		got, err = w.svc.Compact(ctx, time.Now().Add(100*24*time.Hour))
		must(t, err)
		if got.Ops == 0 {
			t.Fatal("no op IDs were forgotten")
		}
		before, err := w.st.GetChangeSeq(ctx, w.ws)
		must(t, err)
		wantStatus(t, w.push("owner", unsent)[0], "applied")
		if after, err := w.st.GetChangeSeq(ctx, w.ws); err != nil || after != before {
			t.Fatalf("an op sent again logged a change: seq %d -> %d (%v)", before, after, err)
		}
	})
}

func TestEpoch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		snap, err := w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		must(t, err)
		if snap.Epoch == "" {
			t.Fatal("bootstrap gave no epoch")
		}
		must(t, w.svc.CheckEpoch(ctx, snap.Epoch))
		for _, e := range []string{"", "another"} {
			if err := w.svc.CheckEpoch(ctx, e); apperr.KindOf(err) != apperr.Gone {
				t.Errorf("epoch %q: %v, want Gone", e, err)
			}
		}
		// A restore writes a new one, which a restarted server reads.
		must(t, w.st.SetInstanceSetting(ctx, store.SyncEpochKey, "restored"))
		fresh := syncer.New(w.st, nil, nil)
		if err := fresh.CheckEpoch(ctx, snap.Epoch); apperr.KindOf(err) != apperr.Gone {
			t.Errorf("old epoch after a restore: %v, want Gone", err)
		}
	})
}

func TestCheckSchema(t *testing.T) {
	if err := syncer.CheckSchema(syncer.Schema); err != nil {
		t.Errorf("current schema: %v", err)
	}
	// A push with none comes from an app older than the gate, whose ops
	// are the first version's.
	if err := syncer.CheckSchema(0); (syncer.Schema == 1) != (err == nil) {
		t.Errorf("no schema, with schema %d current: %v", syncer.Schema, err)
	}
	for _, v := range []int{syncer.Schema - 1, syncer.Schema + 1} {
		if v < 1 {
			continue
		}
		if err := syncer.CheckSchema(v); apperr.KindOf(err) != apperr.UpdateRequired {
			t.Errorf("schema %d: %v, want UpdateRequired", v, err)
		}
	}
}

// fingerprints records the synced fields each schema version had. When
// this test fails, the fields have changed: if an app from before the
// change could send ops this server refuses or misreads (a new required
// field, a renamed one), raise syncer.Schema and SYNC_SCHEMA in
// web/src/lib/sync/entities.ts and add the new fingerprint; otherwise
// update the current one.
var fingerprints = map[int]string{
	1: "0a5dd67342f6fd2f",
	// 2: doc_update, the rows of live documents;
	// then page, page_update and page_attachment, the wiki, which apps of
	// 2 never send and keep without reading; then task_dependency,
	// dependencies (section 3), the same; and the timeline view type,
	// which an app of 2 shows as an empty list; then template, which apps
	// of 2 never send and keep without reading; then a task's
	// estimate_minutes, optional, and time_entry, which apps of 2 never
	// send and keep without reading; then workspace_theme, which apps of
	// 2 never send and keep without reading; then page_thread and
	// page_comment, the same, and a template's kind page, which apps of 2
	// never make, and which one still open lists as a task template with
	// nothing in it until it reloads; then automation, which apps of 2
	// never send and keep without reading.
	2: "f32b235729088feb",
	// 3: the same fields. Documents gained blocks an app of 2 can't hold
	// (the wiki's formatting beyond Markdown), and reads from it are refused.
	3: "f32b235729088feb",
	// 4: the same fields. A view in a page gained a layout, which an app of 3
	// would drop from a document it edited.
	4: "f32b235729088feb",
	// 5: the same fields. A pair's ID became a hash of the pair, so an app
	// of 4 would make pair rows under IDs now refused. Then a task's
	// made_at and started_at, for reports: optional, filled in by the
	// server, so an app of 5 from before them sends neither and keeps them
	// without reading.
	5: "a39fedd6e3fc25fb",
	// 6: the report view type, and a report in a page (a chart from a report
	// view), neither of which an app of 5 can show; it would drop the block
	// from a page it edited. Then project_time_entry, time logged to a
	// project itself, which apps of 6 from before it never send and keep
	// without reading.
	6: "b35e3604732a8f76",
}

func TestSchemaFingerprint(t *testing.T) {
	if got := syncer.Fingerprint(); fingerprints[syncer.Schema] != got {
		t.Errorf("the synced fields changed (fingerprint %s, recorded %q for schema %d); see fingerprints", got, fingerprints[syncer.Schema], syncer.Schema)
	}
	src, err := os.ReadFile("../../../web/src/lib/sync/entities.ts")
	must(t, err)
	m := regexp.MustCompile(`export const SYNC_SCHEMA = (\d+);`).FindSubmatch(src)
	if m == nil || string(m[1]) != strconv.Itoa(syncer.Schema) {
		t.Errorf("entities.ts has SYNC_SCHEMA %q, want %d", m, syncer.Schema)
	}
}
