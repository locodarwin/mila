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

// A view in a page names the view by its ID, which an import gives anew, as it does a link's.
func TestAnImportedPageKeepsItsView(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		owner := w.users["owner"]
		project, _ := w.project("Garden")
		view := uuid.NewString()
		wantStatus(t, w.push("owner", op("saved_view", view, hlcAt(now, "v"), map[string]any{
			"project_id": project, "owner_id": nil, "name": "This month", "type": "list", "config": map[string]any{"filter": map[string]any{}}, "sort_key": "a0",
		}))[0], "applied")
		body := "<!-- mila:view " + view + " -->\n[This month](/views/" + view + ")"
		page := newPage(project, nil, "Plan")
		page["body"] = body
		wantStatus(t, w.push("owner", op("page", syncer.MainPageID(project), hlcAt(now, "p"), page))[0], "applied")

		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: true, Owner: owner, Exporter: owner})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, owner, into)
		must(t, err)
		var newView, newBody string
		for _, r := range snap.Rows {
			switch {
			case r.Entity == "saved_view":
				newView = r.ID
			case r.Entity == "page" && r.Fields["title"] == "Plan":
				newBody, _ = r.Fields["body"].(string)
			}
		}
		if newView == "" || newView == view {
			t.Fatalf("the view came across as %q", newView)
		}
		if want := "<!-- mila:view " + newView + " -->\n[This month](/views/" + newView + ")"; !strings.Contains(newBody, want) {
			t.Fatalf("the page says %q, not %q", newBody, want)
		}
	})
}
