package syncer_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func automationRow(project any, by string, enabled bool) map[string]any {
	return map[string]any{
		"project_id": project, "name": "Follow up", "enabled": enabled, "made_by": by, "sort_key": "a0",
		"rule": map[string]any{"when": map[string]any{"on": "created"}, "then": []any{map[string]any{"do": "priority", "value": "high"}}},
	}
}

// A guest sees none of a project's rules, or the workspace's, however they ask.
func TestAGuestNeverSeesARule(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		w.share(shared, true)
		guest := w.users["guest"]
		before, err := w.svc.Bootstrap(ctx, guest, w.ws)
		must(t, err)
		inProject, wide := uuid.NewString(), uuid.NewString()
		wantStatus(t, w.push("member", op("automation", inProject, hlcAt(now, "m"), automationRow(shared, w.users["member"], true)))[0], "applied")
		wantStatus(t, w.push("owner", op("automation", wide, hlcAt(now, "o"), automationRow(nil, w.users["owner"], true)))[0], "applied")

		snap, err := w.svc.Bootstrap(ctx, guest, w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.Entity == "automation" {
				t.Fatal("a guest's bootstrap holds a rule")
			}
		}
		changes, err := w.svc.Pull(ctx, guest, w.ws, before.Seq)
		must(t, err)
		for _, c := range changes.Changes {
			if c.Entity == "automation" {
				t.Fatal("a guest's pull holds a rule")
			}
		}
		dump, err := w.svc.Dump(ctx, guest, w.ws)
		if err == nil {
			for _, r := range dump.Rows {
				if r.Entity == "automation" {
					t.Fatal("a guest's export holds a rule")
				}
			}
		}
		for _, id := range []string{inProject, wide} {
			if _, err := w.svc.Get(ctx, guest, w.ws, "automation", id); apperr.KindOf(err) != apperr.NotFound {
				t.Fatalf("a guest read a rule: %v", err)
			}
		}
		page, err := w.svc.List(ctx, guest, w.ws, "automation", syncer.Query{Limit: 10})
		must(t, err)
		if len(page.Rows) != 0 {
			t.Fatalf("a guest listed %d rules", len(page.Rows))
		}
		// Refusing a guest's write doesn't hand them the row it would have changed.
		res := w.push("guest", op("automation", inProject, hlcAt(now.Add(time.Second), "g"), map[string]any{"name": "Mine"}))
		wantStatus(t, res[0], "rejected")
		if res[0].Row != nil {
			t.Fatalf("a refusal gave a guest the rule: %+v", res[0].Row)
		}

		// Members see them all.
		snap, err = w.svc.Bootstrap(ctx, w.users["member"], w.ws)
		must(t, err)
		found := 0
		for _, r := range snap.Rows {
			if r.Entity == "automation" {
				found++
			}
		}
		if found != 2 {
			t.Fatalf("a member's bootstrap holds %d rules", found)
		}
	})
}

// A rule runs as whoever last saved it, so nothing sets one going as someone else, and
// what it does never parts from whom it runs as.
func TestOnlyWhoeverSavedARuleCanSetItGoing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		at := func(n int) string { return hlcAt(now.Add(time.Duration(n)*time.Second), "x") }
		project, _ := w.project("Work")
		id := uuid.NewString()
		// As a project from a template is made: off, and the owner's.
		wantStatus(t, w.push("owner", op("automation", id, at(0), automationRow(project, w.users["owner"], false)))[0], "applied")

		wantStatus(t, w.push("member", op("automation", id, at(1), map[string]any{"enabled": true}))[0], "rejected")
		wantStatus(t, w.push("member", op("automation", id, at(2), map[string]any{"rule": automationRow(nil, "", false)["rule"]}))[0], "rejected")
		wantStatus(t, w.push("member", op("automation", id, at(3), map[string]any{"made_by": w.users["member"]}))[0], "rejected")
		wantStatus(t, w.push("member", op("automation", id, at(4), map[string]any{"made_by": w.users["owner"], "rule": automationRow(nil, "", false)["rule"]}))[0], "rejected")
		// A name isn't what it does.
		wantStatus(t, w.push("member", op("automation", id, at(5), map[string]any{"name": "Renamed"}))[0], "applied")
		// Saving it, as turning it on in the app does, makes it theirs.
		wantStatus(t, w.push("member", op("automation", id, at(6), map[string]any{"enabled": true, "rule": automationRow(nil, "", false)["rule"], "made_by": w.users["member"]}))[0], "applied")

		// Deleted and brought back by someone else, it would run as whoever saved it.
		wantStatus(t, w.push("owner", op("automation", id, at(7), map[string]any{"deleted_at": now.UTC().Format(time.RFC3339Nano)}))[0], "applied")
		wantStatus(t, w.push("owner", op("automation", id, at(8), map[string]any{"deleted_at": nil}))[0], "rejected")
		wantStatus(t, w.push("member", op("automation", id, at(9), map[string]any{"deleted_at": nil}))[0], "applied")
		// Anyone can turn it off, and it's still whose it was.
		wantStatus(t, w.push("owner", op("automation", id, at(10), map[string]any{"enabled": false}))[0], "applied")
		wantStatus(t, w.push("owner", op("automation", id, at(11), map[string]any{"deleted_at": now.UTC().Format(time.RFC3339Nano)}))[0], "applied")
		wantStatus(t, w.push("owner", op("automation", id, at(12), map[string]any{"deleted_at": nil}))[0], "applied")
	})
}

// The limit holds for a rule coming back from the trash too.
func TestAWorkspacesRulesHaveALimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("Work")
		ids := make([]string, syncer.MaxAutomations)
		for i := range ids {
			ids[i] = uuid.NewString()
			wantStatus(t, w.push("member", op("automation", ids[i], hlcAt(now, "m"), automationRow(project, w.users["member"], false)))[0], "applied")
		}
		wantStatus(t, w.push("member", op("automation", uuid.NewString(), hlcAt(now, "m"), automationRow(project, w.users["member"], false)))[0], "rejected")
		wantStatus(t, w.push("member", op("automation", ids[0], hlcAt(now.Add(time.Second), "m"), map[string]any{"deleted_at": now.UTC().Format(time.RFC3339Nano)}))[0], "applied")
		wantStatus(t, w.push("member", op("automation", uuid.NewString(), hlcAt(now.Add(2*time.Second), "m"), automationRow(project, w.users["member"], false)))[0], "applied")
		wantStatus(t, w.push("member", op("automation", ids[0], hlcAt(now.Add(3*time.Second), "m"), map[string]any{"deleted_at": nil}))[0], "rejected")
	})
}

// An import leaves every rule off, whether or not it keeps IDs.
func TestAnImportedRuleWaitsToBeTurnedOn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("Work")
		owner := w.users["owner"]
		wantStatus(t, w.push("owner", op("automation", uuid.NewString(), hlcAt(now, "o"), automationRow(project, owner, true)))[0], "applied")
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		for _, fresh := range []bool{true, false} {
			if !fresh {
				must(t, w.st.PurgeWorkspace(ctx, w.ws))
			}
			into := newHidden(t, w.st)
			_, err := w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: fresh, Owner: owner, Exporter: owner})
			must(t, err)
			must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
			snap, err := w.svc.Bootstrap(ctx, owner, into)
			must(t, err)
			found := false
			for _, r := range snap.Rows {
				if r.Entity == "automation" {
					found = true
					if r.Fields["enabled"] != false {
						t.Errorf("fresh %v: imported turned on", fresh)
					}
				}
			}
			if !found {
				t.Errorf("fresh %v: no rule came across", fresh)
			}
		}
	})
}

// A rule made or turned on through REST acts on due dates from then, as the app's does.
func TestRESTStampsWhenARuleWasTurnedOn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		project, _ := w.project("Work")
		fields := map[string]json.RawMessage{}
		for k, v := range automationRow(project, w.users["member"], true) {
			b, err := json.Marshal(v)
			must(t, err)
			fields[k] = b
		}
		delete(fields, "made_by")
		rec, _, err := w.svc.Write(ctx, w.users["member"], w.ws, syncer.Write{Entity: "automation", Mode: syncer.Create, Fields: fields})
		must(t, err)
		if rec.Fields["enabled_at"] == nil || rec.Fields["made_by"] != w.users["member"] {
			t.Fatalf("made: %+v", rec.Fields)
		}
	})
}

// History names a rule for those who can see rules.
func TestAGuestReadsThatARuleDidItNotWhich(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		w.share(shared, true)
		task := uuid.NewString()
		wantStatus(t, w.push("member", op("task", task, hlcAt(now, "m"), map[string]any{"title": "Seen", "project_id": shared, "sort_key": "a0"}))[0], "applied")
		_, _, err := w.svc.Write(syncer.WithAutomation(ctx, "Escalate quietly"), w.users["member"], w.ws, syncer.Write{Entity: "task", ID: task, Mode: syncer.Update,
			Fields: map[string]json.RawMessage{"priority": json.RawMessage(`"high"`)}})
		must(t, err)
		said := func(role string) string {
			events, err := w.svc.Activity(ctx, w.users[role], w.ws, task)
			must(t, err)
			var via []string
			for _, e := range events {
				if e.Via != "" {
					via = append(via, e.Via)
				}
			}
			return strings.Join(via, ",")
		}
		if got := said("guest"); got != "an automation" {
			t.Errorf("a guest read %q", got)
		}
		if got := said("member"); got != "automation: Escalate quietly" {
			t.Errorf("a member read %q", got)
		}
	})
}
