package syncer_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestMainPageIDsAreTheAppsToo(t *testing.T) {
	// web/src/lib/sync/pages.spec.ts holds the same.
	if got := syncer.MainPageID("0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"); got != "2ecfcdbc-494f-3221-17a1-670361153603" {
		t.Fatalf("MainPageID = %s", got)
	}
}

func newPage(project, parent any, title string) map[string]any {
	return map[string]any{"project_id": project, "parent_id": parent, "title": title, "body": "", "sort_key": "a0", "deleted_at": nil}
}

func TestAWikiKeepsItsShape(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		h := hlcAt(now, "a")
		project, _ := w.project("House")
		other, _ := w.project("Garden")
		main := syncer.MainPageID(project)

		// A page with no parent is only the main page, with its own ID.
		wantStatus(t, w.push("owner", op("page", uuid.NewString(), h, newPage(project, nil, "Loose")))[0], "rejected")
		wantStatus(t, w.push("owner", op("page", main, h, newPage(project, nil, "")))[0], "applied")
		// Two devices making the main page at once make one row.
		wantStatus(t, w.push("member", op("page", main, hlcAt(now, "b"), newPage(project, nil, "")))[0], "applied")
		if n := w.count("pages", "project_id = ?", project); n != 1 {
			t.Fatalf("%d main pages", n)
		}

		a, b := uuid.NewString(), uuid.NewString()
		wantStatus(t, w.push("owner", op("page", a, h, newPage(project, main, "Paint")))[0], "applied")
		wantStatus(t, w.push("owner", op("page", b, h, newPage(project, a, "Colors")))[0], "applied")
		// A parent in another wiki is refused, and so is one below the page.
		otherMain := syncer.MainPageID(other)
		w.push("owner", op("page", otherMain, h, newPage(other, nil, "")))
		wantStatus(t, w.push("owner", op("page", uuid.NewString(), h, newPage(project, otherMain, "Stray")))[0], "rejected")
		wantStatus(t, w.push("owner", op("page", a, hlcAt(now.Add(time.Second), "a"), map[string]any{"parent_id": b}))[0], "rejected")
		wantStatus(t, w.push("owner", op("page", a, hlcAt(now.Add(time.Second), "a"), map[string]any{"parent_id": a}))[0], "rejected")
		// The main page cannot be deleted or given a parent.
		wantStatus(t, w.push("owner", op("page", main, hlcAt(now.Add(time.Second), "a"), map[string]any{"deleted_at": now}))[0], "rejected")
		wantStatus(t, w.push("owner", op("page", main, hlcAt(now.Add(time.Second), "a"), map[string]any{"parent_id": a}))[0], "rejected")
		// A page moves to another parent in its wiki.
		wantStatus(t, w.push("owner", op("page", b, hlcAt(now.Add(time.Second), "a"), map[string]any{"parent_id": main}))[0], "applied")

		// The workspace's wiki: its main page is the workspace's.
		wantStatus(t, w.push("owner", op("page", syncer.MainPageID(w.ws), h, newPage(nil, nil, "")))[0], "applied")
	})
}

func TestAPagesBodyIsALiveDocument(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("House")
		main := syncer.MainPageID(project)
		w.push("owner", op("page", main, hlcAt(now, "a"), newPage(project, nil, "")))
		u := edit(t, 60, "## Hallway\n\nPale Oak, eggshell.")
		wantStatus(t, w.push("owner", op("page_update", uuid.NewString(), hlcAt(now, "a"), map[string]any{"page_id": main, "data": encode(u), "deleted_at": nil}))[0], "applied")
		if body := w.rowFields("page", main)["body"]; body != "## Hallway\n\nPale Oak, eggshell." {
			t.Fatalf("body = %v", body)
		}
		// A body written through REST becomes a row of the document.
		w.mustWrite("owner", syncer.Write{Entity: "page", ID: main, Mode: syncer.Update, Fields: fields(map[string]any{"body": "## Hallway\n\nRepose Gray."})})
		if n := w.count("page_updates", "page_id = ?", main); n != 2 {
			t.Fatalf("%d rows", n)
		}
		// Presence goes by who may see the page.
		seen, err := w.svc.SeesDoc(context.Background(), w.users["member"], w.ws, "page:"+main)
		if err != nil || !seen {
			t.Fatalf("a member cannot see the page: %v", err)
		}
	})
}

func TestGuestsSeeTheWikisOfTheirProjectsOnly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		h := hlcAt(now, "a")
		shared, _ := w.project("Shared")
		hidden, _ := w.project("Hidden")
		w.share(shared, true)
		for _, p := range []string{shared, hidden} {
			w.push("owner", op("page", syncer.MainPageID(p), h, newPage(p, nil, "")))
		}
		home := syncer.MainPageID(w.ws)
		w.push("owner", op("page", home, h, newPage(nil, nil, "")))
		snap, err := w.svc.Bootstrap(context.Background(), w.users["guest"], w.ws)
		must(t, err)
		var pages []string
		for _, r := range snap.Rows {
			if r.Entity == "page" {
				pages = append(pages, r.ID)
			}
		}
		if len(pages) != 1 || pages[0] != syncer.MainPageID(shared) {
			t.Fatalf("the guest sees pages %v", pages)
		}
		// The guest writes in the shared wiki, not the workspace's.
		wantStatus(t, w.push("guest", op("page", uuid.NewString(), h, newPage(shared, syncer.MainPageID(shared), "Mine")))[0], "applied")
		wantStatus(t, w.push("guest", op("page", uuid.NewString(), h, newPage(nil, home, "Sneaky")))[0], "rejected")
		if seen, _ := w.svc.SeesDoc(context.Background(), w.users["guest"], w.ws, "page:"+home); seen {
			t.Fatal("the guest can open the workspace wiki's main page")
		}
	})
}

func TestDeletedPagesAreInTheTrash(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("House")
		main := syncer.MainPageID(project)
		a := uuid.NewString()
		w.push("owner", op("page", main, hlcAt(now, "a"), newPage(project, nil, "")), op("page", a, hlcAt(now, "a"), newPage(project, main, "Old plans")))
		w.push("owner", op("page", a, hlcAt(now.Add(time.Second), "a"), map[string]any{"deleted_at": now}))
		trash, err := w.svc.Trash(context.Background(), w.users["owner"], w.ws)
		must(t, err)
		found := false
		for _, r := range trash {
			found = found || (r.Entity == "page" && r.ID == a)
		}
		if !found {
			t.Fatal("the deleted page is not in the trash")
		}
	})
}

// An import gives the workspace a new ID, and with new IDs its projects
// too: their wikis' main pages are named after the new ones, and the
// pages under them and their bodies follow.
func TestRestoredWikisKeepTheirMainPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		h := hlcAt(time.Now(), "a")
		project, _ := w.project("House")
		wsMain, projectMain := syncer.MainPageID(w.ws), syncer.MainPageID(project)
		for _, res := range w.push("owner",
			op("page", wsMain, h, newPage(nil, nil, "")),
			op("page", uuid.NewString(), h, newPage(nil, wsMain, "Utilities")),
			op("page", projectMain, h, newPage(project, nil, "")),
			op("page", uuid.NewString(), h, newPage(project, projectMain, "Paint")),
		) {
			wantStatus(t, res, "applied")
		}
		w.push("owner", op("page", projectMain, hlcAt(time.Now().Add(time.Second), "a"), map[string]any{"body": "Notes for the fall."}))
		owner := w.users["owner"]
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		// A page whose parent the export lacks goes under its main page.
		stray := syncer.Row{Entity: "page", ID: uuid.NewString(), Fields: newPage(project, uuid.NewString(), "Stray"), Clocks: map[string]string{}}
		dump.Rows = append(dump.Rows, stray)

		for _, fresh := range []bool{true, false} {
			if !fresh {
				// Keeping IDs needs them free: the old workspace goes first.
				must(t, w.st.PurgeWorkspace(ctx, w.ws))
			}
			into := newHidden(t, w.st)
			_, err := w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: fresh, Owner: owner, Exporter: owner})
			must(t, err)
			must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
			snap, err := w.svc.Bootstrap(ctx, owner, into)
			must(t, err)
			var newProject string
			pages := map[string]syncer.Row{}
			for _, r := range snap.Rows {
				switch r.Entity {
				case "project":
					newProject = r.ID
				case "page":
					pages[r.ID] = r
				}
			}
			ws, pm := pages[syncer.MainPageID(into)], pages[syncer.MainPageID(newProject)]
			if ws.ID == "" || pm.ID == "" || len(pages) != 5 {
				t.Fatalf("fresh %v: pages %v", fresh, pages)
			}
			if pm.Fields["body"] != "Notes for the fall." {
				t.Errorf("fresh %v: main page body %q", fresh, pm.Fields["body"])
			}
			for _, p := range pages {
				switch p.Fields["title"] {
				case "Utilities":
					if p.Fields["parent_id"] != ws.ID {
						t.Errorf("fresh %v: Utilities under %v", fresh, p.Fields["parent_id"])
					}
				case "Paint", "Stray":
					if p.Fields["parent_id"] != pm.ID {
						t.Errorf("fresh %v: %v under %v", fresh, p.Fields["title"], p.Fields["parent_id"])
					}
				}
			}
		}
	})
}
