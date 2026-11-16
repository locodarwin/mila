package syncer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// A title or a name is one line: it goes into email as it is. An export
// from before the rule comes in with its line breaks made spaces.
func TestOneLineFieldsHoldNoLineBreaks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner, member := w.users["owner"], w.users["member"]
		project := uuid.NewString()
		wantStatus(t, w.push("owner", op("project", project, later(0), newProject("Garden")))[0], "applied")
		for _, title := range []string{"Plan\r.\r\nMAIL FROM:<x@example.net>", "Plan\rthe beds", "Plan\nthe beds"} {
			res := w.push("owner", op("task", uuid.NewString(), later(1), newTask(project, nil, title)))[0]
			if res.Status != "rejected" || !strings.Contains(res.Message, "one line") {
				t.Errorf("a title of %q: %s (%s)", title, res.Status, res.Message)
			}
		}
		task := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", task, later(2), newTask(project, nil, "Plan the beds")))[0], "applied")

		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		for _, r := range dump.Rows {
			if r.ID == task {
				r.Fields["title"] = "Plan \r\n  the\rbeds"
			}
		}
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: true, Owner: member, Exporter: owner, From: w.ws})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: member, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, member, into)
		must(t, err)
		found := false
		for _, r := range snap.Rows {
			if r.Entity == "task" {
				found = true
				if r.Fields["title"] != "Plan the beds" {
					t.Errorf("an imported title = %q", r.Fields["title"])
				}
			}
		}
		if !found {
			t.Fatal("the task did not come in")
		}
	})
}

// Postgres refuses a NUL in text, so sync refuses one in any text field, and
// an export made on SQLite, which takes them, comes in without its NULs.
func TestTextHoldsNoNULs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner, member := w.users["owner"], w.users["member"]
		project := uuid.NewString()
		wantStatus(t, w.push("owner", op("project", project, later(0), newProject("Garden")))[0], "applied")
		res := w.push("owner", op("task", uuid.NewString(), later(1), newTask(project, nil, "Plan\x00 the beds")))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "NUL") {
			t.Errorf("a title with a NUL: %s (%s)", res.Status, res.Message)
		}
		task := uuid.NewString()
		fields := newTask(project, nil, "Plan the beds")
		fields["notes"] = "Dig\x00 first"
		res = w.push("owner", op("task", task, later(2), fields))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "NUL") {
			t.Errorf("notes with a NUL: %s (%s)", res.Status, res.Message)
		}
		delete(fields, "notes")
		wantStatus(t, w.push("owner", op("task", task, later(3), fields))[0], "applied")

		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		for _, r := range dump.Rows {
			if r.ID == task {
				r.Fields["title"] = "Plan\x00 the beds"
				r.Fields["notes"] = "Dig\x00 first"
			}
		}
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: true, Owner: member, Exporter: owner, From: w.ws})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: member, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, member, into)
		must(t, err)
		found := false
		for _, r := range snap.Rows {
			if r.Entity == "task" {
				found = true
				if r.Fields["title"] != "Plan the beds" || r.Fields["notes"] != "Dig first" {
					t.Errorf("imported: title %q, notes %q", r.Fields["title"], r.Fields["notes"])
				}
			}
		}
		if !found {
			t.Fatal("the task did not come in")
		}
	})
}
