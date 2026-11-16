package syncer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func themeRow(name string, body any) map[string]any {
	return map[string]any{"name": name, "body": body, "sort_key": "a0"}
}

func themeBody(name string) map[string]any {
	return map[string]any{
		"mila": "theme", "version": 1, "id": uuid.NewString(), "name": name, "base": "calm",
		"light": map[string]any{"surface-0": map[string]any{"oklch": []any{0.97, 0.01, 220}}},
	}
}

func TestWorkspaceThemesAreForAdminsToChange(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		w.share(shared, true)
		id := uuid.NewString()
		wantStatus(t, w.push("owner", op("workspace_theme", id, hlcAt(now, "a"), themeRow("Harbor", themeBody("Harbor"))))[0], "applied")
		// Everyone sees it, guests too.
		for _, who := range []string{"member", "guest"} {
			snap, err := w.svc.Bootstrap(ctx, w.users[who], w.ws)
			must(t, err)
			found := false
			for _, r := range snap.Rows {
				found = found || r.Entity == "workspace_theme" && r.ID == id
			}
			if !found {
				t.Errorf("the %s's bootstrap lacks the workspace theme", who)
			}
		}
		// Only owners and admins make or change one.
		for _, who := range []string{"member", "guest"} {
			res := w.push(who, op("workspace_theme", uuid.NewString(), hlcAt(now, who[:1]), themeRow("Mine", themeBody("Mine"))))[0]
			if res.Status != "rejected" || !strings.Contains(res.Message, "owners and admins") {
				t.Errorf("a %s made one: %s %s", who, res.Status, res.Message)
			}
			res = w.push(who, op("workspace_theme", id, hlcAt(now.Add(time.Second), who[:1]), map[string]any{"name": "Renamed"}))[0]
			if res.Status != "rejected" {
				t.Errorf("a %s renamed one: %s", who, res.Status)
			}
		}
		wantStatus(t, w.push("owner", op("workspace_theme", id, hlcAt(now.Add(2*time.Second), "a"), map[string]any{"suggested_at": now.UTC().Format(time.RFC3339)}))[0], "applied")
	})
}

func TestWorkspaceThemesAreChecked(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		unknown := themeBody("Odd")
		unknown["css"] = "body { background: url(x) }"
		noBase := themeBody("No base")
		delete(noBase, "base")
		badToken := themeBody("Bad token")
		badToken["tokens"] = map[string]any{"no-such-token": map[string]any{"px": 4}}
		for _, body := range []any{unknown, noBase, badToken, "a theme", map[string]any{}} {
			res := w.push("owner", op("workspace_theme", uuid.NewString(), hlcAt(now, "a"), themeRow("Bad", body)))[0]
			if res.Status != "rejected" || !strings.Contains(res.Message, "theme") {
				t.Errorf("%v was %s: %s", body, res.Status, res.Message)
			}
		}
		// Ten at most, live; one unpublished makes room.
		ids := make([]string, 10)
		for i := range ids {
			ids[i] = uuid.NewString()
			wantStatus(t, w.push("owner", op("workspace_theme", ids[i], hlcAt(now, "a"), themeRow("T", themeBody("T"))))[0], "applied")
		}
		res := w.push("owner", op("workspace_theme", uuid.NewString(), hlcAt(now, "a"), themeRow("One more", themeBody("One more"))))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "10 themes") {
			t.Errorf("the 11th: %s %s", res.Status, res.Message)
		}
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("workspace_theme", ids[0], hlcAt(later, "a"), map[string]any{"deleted_at": later.UTC().Format(time.RFC3339)}))[0], "applied")
		wantStatus(t, w.push("owner", op("workspace_theme", uuid.NewString(), hlcAt(later, "a"), themeRow("One more", themeBody("One more"))))[0], "applied")
		// Putting the unpublished one back would make eleven.
		res = w.push("owner", op("workspace_theme", ids[0], hlcAt(later.Add(time.Second), "a"), map[string]any{"deleted_at": nil}))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "10 themes") {
			t.Errorf("bringing back an 11th: %s %s", res.Status, res.Message)
		}
	})
}

func TestWorkspaceThemesAreExported(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		owner := w.users["owner"]
		w.push("owner", op("workspace_theme", uuid.NewString(), hlcAt(now, "a"), themeRow("Harbor", themeBody("Harbor"))))
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: true, Owner: owner, Exporter: owner, From: w.ws})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, owner, into)
		must(t, err)
		var name any
		for _, r := range snap.Rows {
			if r.Entity == "workspace_theme" {
				name = r.Fields["name"]
			}
		}
		if name != "Harbor" {
			t.Fatalf("the restored workspace offers %v", name)
		}
	})
}
