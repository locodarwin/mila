package datagen_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/datagen"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestGenerate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		st := store.New(d)
		svc := syncer.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
		spec := datagen.Spec{Email: "perf@example.com", Tasks: 300, Projects: 3, BoardCards: 60, Members: 2, Seed: 7,
			Today: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)}
		res, err := datagen.Generate(ctx, st, svc, spec)
		if err != nil {
			t.Fatal(err)
		}
		if res.Rows["task"] != 300 || res.Rows["project"] != 4 || res.Rows["status"] != 20 {
			t.Fatalf("rows = %v", res.Rows)
		}
		snap, err := svc.Bootstrap(ctx, res.OwnerID, res.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		// Every op made a row that the owner sees (My Day entries are
		// theirs), and every task with notes a row of its document.
		withNotes, docs := 0, 0
		for _, r := range snap.Rows {
			if r.Entity == "task" && r.Fields["notes"] != "" {
				withNotes++
			}
			if r.Entity == "doc_update" {
				docs++
			}
		}
		if withNotes == 0 || docs != withNotes {
			t.Fatalf("%d tasks with notes, %d document rows", withNotes, docs)
		}
		if want := res.Ops + docs; len(snap.Rows) != want || snap.Seq != int64(want) {
			t.Fatalf("bootstrap has %d rows at seq %d, want %d", len(snap.Rows), snap.Seq, want)
		}
		count := map[string]int{}
		cards := 0
		for _, r := range snap.Rows {
			count[r.Entity]++
			if r.Entity == "task" && r.Fields["project_id"] == res.BoardProjectID {
				cards++
			}
		}
		if cards != 60 || count["comment"] == 0 || count["task_label"] == 0 || count["checklist_item"] == 0 || count["task_assignee"] == 0 ||
			count["custom_field"] != 3 || count["custom_field_value"] < 100 {
			t.Fatalf("board cards %d, rows %v", cards, count)
		}
		owner, err := st.GetUser(ctx, res.OwnerID)
		if err != nil || !owner.IsInstanceAdmin {
			t.Fatalf("owner = %+v, %v; want the instance's admin", owner, err)
		}

		// The same seed makes the same rows again, in a second workspace,
		// for the same people.
		again, err := datagen.Generate(ctx, st, svc, spec)
		if err != nil {
			t.Fatal(err)
		}
		if again.OwnerID != res.OwnerID || again.WorkspaceID == res.WorkspaceID || again.Ops != res.Ops {
			t.Fatalf("second run = %+v, first %+v", again, res)
		}
		for k, v := range res.Rows {
			if again.Rows[k] != v {
				t.Fatalf("second run made %v, first %v", again.Rows, res.Rows)
			}
		}
	})
}
