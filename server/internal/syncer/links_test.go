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

func pageRow(page string, update []byte) map[string]any {
	return map[string]any{"page_id": page, "data": encode(update), "deleted_at": nil}
}

func TestLinkTargets(t *testing.T) {
	syncer.SetAppAddress("https://mila.example.com")
	defer syncer.SetAppAddress("")
	id := "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	for href, want := range map[string]string{
		"/wiki?page=" + id:                         id,
		"/projects/" + id + "?tab=wiki&page=" + id: id,
		"https://mila.example.com/wiki?page=" + id: id,
		"/?task=" + id:                             id,
		"/projects/" + id + "?task=" + id:          id,
		"/wiki?page=not-an-id":                     "",
		"/settings?page=" + id:                     "",
		"https://example.com":                      "",
		"mailto:someone@example.com":               "",
		// Another site, however it is written, is not the app.
		"https://evil.example/wiki?page=" + id:                 "",
		"//evil.example/wiki?page=" + id:                       "",
		"/\\evil.example/?task=" + id:                          "",
		"/\t/evil.example/?task=" + id:                         "",
		"http://mila.example.com.evil.example/wiki?page=" + id: "",
	} {
		if got := syncer.LinkTarget(href); got != want {
			t.Errorf("LinkTarget(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestAPagesHistoryIsOneVersionPerSession(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now().UTC().Truncate(time.Millisecond)
		w.svc.SetNow(func() time.Time { return now })
		start := now
		project, _ := w.project("House")
		main := syncer.MainPageID(project)
		w.push("owner", op("page", main, hlcAt(now, "a"), newPage(project, nil, "")))
		var updates [][]byte
		for i, s := range []struct {
			role string
			at   time.Duration
			md   string
		}{
			{"owner", 0, "One"}, {"owner", time.Minute, "One two"},
			{"member", 2 * time.Minute, "One two three"},
			{"owner", 10 * time.Minute, "Four"},
		} {
			now = start.Add(s.at)
			u := edit(t, uint64(50+i), s.md, updates...)
			updates = append(updates, u)
			wantStatus(t, w.push(s.role, op("page_update", uuid.NewString(), hlcAt(now, s.role), pageRow(main, u)))[0], "applied")
		}
		// A body written whole, as a restore writes it, is a version of its
		// own even within the owner's session.
		now = start.Add(11 * time.Minute)
		wantStatus(t, w.push("owner", op("page", main, hlcAt(now, "a"), map[string]any{"body": "One two"}))[0], "applied")

		versions, err := w.svc.PageVersions(context.Background(), w.users["owner"], w.ws, main)
		must(t, err)
		var got []string
		for _, v := range versions {
			got = append(got, v.ActorName+": "+v.Body)
		}
		want := []string{"owner: One two", "owner: Four", "member: One two three", "owner: One two"}
		if len(got) != len(want) {
			t.Fatalf("history = %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("history = %q, want %q", got, want)
			}
		}
		// Someone who cannot see the page cannot read its history.
		if _, err := w.svc.PageVersions(context.Background(), w.users["owner"], w.ws, uuid.NewString()); err == nil {
			t.Error("the history of a page that does not exist")
		}
	})
}

func TestRenamingKeepsLinksCurrent(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("House")
		main, plan, task := syncer.MainPageID(project), uuid.NewString(), uuid.NewString()
		href := "/projects/" + project + "?tab=wiki&page=" + plan
		w.push("owner",
			op("page", main, hlcAt(now, "a"), newPage(project, nil, "")),
			op("page", plan, hlcAt(now, "a"), newPage(project, main, "Paint plan")),
			op("task", task, hlcAt(now, "a"), newTask(project, nil, "Buy paint")),
		)
		// The main page and the task's notes link to the plan; one link is
		// worded on purpose.
		page := edit(t, 70, "See [Paint plan]("+href+") and [the colors]("+href+").")
		notes := edit(t, 71, "From [Paint plan]("+href+").")
		w.push("owner",
			op("page_update", uuid.NewString(), hlcAt(now, "a"), pageRow(main, page)),
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, notes)),
		)
		later := now.Add(time.Second)
		wantStatus(t, w.push("member", op("page", plan, hlcAt(later, "b"), map[string]any{"title": "Paint and colors"}))[0], "applied")

		if got := w.rowFields("page", main)["body"]; got != "See [Paint and colors]("+href+") and [the colors]("+href+")." {
			t.Errorf("the page reads %q", got)
		}
		if got := w.taskFields(task)["notes"]; got != "From [Paint and colors]("+href+")." {
			t.Errorf("the notes read %q", got)
		}
		// A task renamed: its links follow too.
		todo := uuid.NewString()
		w.push("owner",
			op("page", todo, hlcAt(later, "a"), newPage(project, main, "To do")),
			op("page_update", uuid.NewString(), hlcAt(later, "a"), pageRow(todo, edit(t, 72, "Do [Buy paint](/?task="+task+")."))),
		)
		wantStatus(t, w.push("owner", op("task", task, hlcAt(later.Add(time.Second), "a"), map[string]any{"title": "Buy primer"}))[0], "applied")
		if got := w.rowFields("page", todo)["body"]; got != "Do [Buy primer](/?task="+task+")." {
			t.Errorf("after renaming the task the page reads %q", got)
		}
	})
}

func TestRenamingAProjectRenamesLinksToItsWiki(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		project, _ := w.project("House")
		main, task := syncer.MainPageID(project), uuid.NewString()
		href := "/projects/" + project + "?tab=wiki&page=" + main
		w.push("owner",
			op("page", main, hlcAt(now, "a"), newPage(project, nil, "")),
			op("task", task, hlcAt(now, "a"), newTask(project, nil, "Paint")),
		)
		w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, edit(t, 73, "In [House]("+href+")."))))
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("project", project, hlcAt(later, "a"), map[string]any{"name": "Home"}))[0], "applied")
		if got := w.taskFields(task)["notes"]; got != "In [Home]("+href+")." {
			t.Errorf("the notes read %q", got)
		}
	})
}

func TestRenamingReachesNotesWrittenBeforeTheLinkList(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		plan, task := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", plan, hlcAt(now, "a"), newTask(nil, nil, "Plan")),
			op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Old")),
		)
		// Notes from before live documents and before doc_links: in the
		// column, with no document and no links listed.
		dd := w.st.DB()
		_, err := dd.ExecContext(ctx, rebind(dd, "UPDATE tasks SET notes = ? WHERE id = ?"), "See [Plan](/?task="+plan+").", task)
		must(t, err)
		if n := w.links(plan); n != 0 {
			t.Fatalf("%d links listed before IndexLinks", n)
		}
		n, err := w.svc.IndexLinks(ctx)
		must(t, err)
		if n != 1 || w.links(plan) != 1 {
			t.Fatalf("IndexLinks read %d documents and listed %d links", n, w.links(plan))
		}
		if n, _ := w.svc.IndexLinks(ctx); n != 0 {
			t.Fatalf("IndexLinks read %d documents again", n)
		}
		// The rename makes the notes' document, then renames the link.
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("task", plan, hlcAt(later, "a"), map[string]any{"title": "The plan"}))[0], "applied")
		if got := w.taskFields(task)["notes"]; got != "See [The plan](/?task="+plan+")." {
			t.Errorf("the notes read %q", got)
		}
		if _, rows := w.docOf(task); rows == 0 {
			t.Error("no document made of the notes")
		}
		// SeedDocs then finds nothing left to make.
		if n, _ := w.svc.SeedDocs(ctx, w.ws); n != 0 {
			t.Errorf("seeded %d documents", n)
		}
	})
}

func TestTheLinkListFollowsTheNotes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		plan, task := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", plan, hlcAt(now, "a"), newTask(nil, nil, "Plan")),
			op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Paint")),
		)
		first := edit(t, 74, "See [Plan](/?task="+plan+") and [it again](/?task="+plan+").")
		w.push("owner", op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, first)))
		if n := w.links(plan); n != 1 {
			t.Fatalf("%d links listed, want 1", n)
		}
		// A write through REST or an older app, without the link.
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("task", task, hlcAt(later, "a"), map[string]any{"notes": "No links now."}))[0], "applied")
		if n := w.links(plan); n != 0 {
			t.Fatalf("%d links listed after the link went", n)
		}
		// A task made with a link in its notes.
		other := uuid.NewString()
		fields := newTask(nil, nil, "Other")
		fields["notes"] = "[Plan](/?task=" + plan + ")"
		w.push("owner", op("task", other, hlcAt(later, "a"), fields))
		if n := w.links(plan); n != 1 {
			t.Fatalf("%d links listed after a task was made with one, want 1", n)
		}
		// The list goes with the task.
		dd := w.st.DB()
		_, err := dd.ExecContext(context.Background(), rebind(dd, "DELETE FROM tasks WHERE id = ?"), other)
		must(t, err)
		if n := w.links(plan); n != 0 {
			t.Fatalf("%d links listed after the task went", n)
		}
	})
}

// links counts the documents doc_links lists as linking to target.
func (w *world) links(target string) int {
	w.t.Helper()
	dd := w.st.DB()
	var n int
	must(w.t, dd.QueryRowContext(context.Background(), rebind(dd, "SELECT COUNT(*) FROM doc_links WHERE target_id = ?"), target).Scan(&n))
	return n
}

func TestAnImportPointsDocumentsAtTheNewRows(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		owner := w.users["owner"]
		project, _ := w.project("House")
		main, plan, task := syncer.MainPageID(project), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("page", main, hlcAt(now, "a"), newPage(project, nil, "")),
			op("page", plan, hlcAt(now, "a"), newPage(project, main, "Plan")),
			op("task", task, hlcAt(now, "a"), newTask(project, nil, "Buy paint")),
		)
		file := uuid.NewString()
		_, err := w.svc.CreateAttachment(ctx, owner, w.ws, syncer.NewAttachment{ID: file, TaskID: task, Filename: "a.png", Mime: "image/png", StorageKey: "k", Size: 1})
		must(t, err)
		planHref := "/projects/" + project + "?tab=wiki&page=" + plan
		body := "See [Plan](" + planHref + "), [Buy paint](/?task=" + task + ") and ![a](/api/v1/workspaces/" + w.ws + "/attachments/" + file + "/file)"
		w.push("owner",
			op("page_update", uuid.NewString(), hlcAt(now, "a"), pageRow(main, edit(t, 80, body))),
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(task, edit(t, 81, "Back to [Plan]("+planHref+")"))),
		)
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		for _, r := range dump.Rows {
			if r.Entity == "doc_update" || r.Entity == "page_update" {
				t.Fatalf("the dump holds a %s", r.Entity)
			}
		}
		if len(dump.Versions) == 0 {
			t.Fatal("the dump holds no page history")
		}

		into := newHidden(t, w.st)
		rep, err := w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{
			FreshIDs: true, Owner: owner, Exporter: owner, From: w.ws, Versions: dump.Versions,
		})
		must(t, err)
		if rep.Versions != len(dump.Versions) {
			t.Errorf("restored %d versions of %d", rep.Versions, len(dump.Versions))
		}
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, owner, into)
		must(t, err)
		ids := map[string]string{}
		var pageBody, notes string
		docs := 0
		for _, r := range snap.Rows {
			switch {
			case r.Entity == "project":
				ids["project"] = r.ID
			case r.Entity == "task":
				ids["task"] = r.ID
				notes, _ = r.Fields["notes"].(string)
			case r.Entity == "attachment":
				ids["file"] = r.ID
			case r.Entity == "page" && r.Fields["title"] == "Plan":
				ids["plan"] = r.ID
			case r.Entity == "page" && r.Fields["parent_id"] == nil:
				ids["main"] = r.ID
				pageBody, _ = r.Fields["body"].(string)
			case r.Entity == "page_update" || r.Entity == "doc_update":
				docs++
			}
		}
		newPlan := "/projects/" + ids["project"] + "?tab=wiki&page=" + ids["plan"]
		want := "See [Plan](" + newPlan + "), [Buy paint](/?task=" + ids["task"] + ") and ![a](/api/v1/workspaces/" + into + "/attachments/" + ids["file"] + "/file)"
		if pageBody != want {
			t.Errorf("the page reads\n%s\nwant\n%s", pageBody, want)
		}
		if notes != "Back to [Plan]("+newPlan+")" {
			t.Errorf("the notes read %q", notes)
		}
		// Both documents are made again, from the rewritten Markdown.
		if docs != 2 {
			t.Errorf("%d documents after the import", docs)
		}
		versions, err := w.svc.PageVersions(ctx, owner, into, ids["main"])
		must(t, err)
		if len(versions) == 0 || versions[0].Body != want {
			t.Errorf("the page's history after the import: %+v", versions)
		}
	})
}
