package notify_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// page makes a wiki page under the wiki's main page; thread pins a thread to
// it, as the first comment's author does.
func (w *world) page(role, project, title string) string {
	main := syncer.MainPageID(project)
	w.push(role, "page", main, map[string]any{"project_id": project, "sort_key": "a0"})
	id := uuid.NewString()
	w.push(role, "page", id, map[string]any{"project_id": project, "parent_id": main, "title": title, "sort_key": "a1"})
	return id
}

func (w *world) thread(role, page string) string {
	id := uuid.NewString()
	w.push(role, "page_thread", id, map[string]any{"page_id": page, "created_by": w.users[role], "quote": "the first crack"})
	return id
}

func (w *world) threadComment(role, page, thread, body string) string {
	id := uuid.NewString()
	w.push(role, "page_comment", id, map[string]any{"page_id": page, "thread_id": thread, "author_id": w.users[role], "body": body})
	return id
}

func TestCommentsOnPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		page := w.page("member", w.project, "Roasting schedule")
		thread := w.thread("member", page)
		w.threadComment("member", page, thread, "Which day?")
		w.run()
		// Nobody else is in the thread yet, and nobody was mentioned.
		want(t, w.inbox("owner"))

		// A mention tells that person, who then counts as in the thread.
		w.threadComment("member", page, thread, "Ask "+w.mention("owner"))
		w.run()
		want(t, w.inbox("owner"), "mention: Dana mentioned you in Roasting schedule")

		// A reply tells who wrote there before, and nobody about their own.
		w.threadComment("owner", page, thread, "Tuesday")
		w.run()
		want(t, w.inbox("member"), "page_comment: Shawn commented on Roasting schedule")
		if got := w.inbox("owner"); len(got) != 1 {
			t.Fatalf("the owner heard about their own comment: %q", got)
		}

		// It leads to the thread.
		list, err := w.svc.List(w.ctx, w.users["member"], "", 10)
		must(t, err)
		url := list.Items[0].URL
		for _, part := range []string{"/projects/" + w.project + "?", "page=" + page, "thread=" + thread, "tab=wiki", "ws=" + w.ws} {
			if !strings.Contains(url, part) {
				t.Fatalf("url %q lacks %q", url, part)
			}
		}
	})
}

// A guest hears of a thread only on a project they are given, and no comment
// on a page tells anyone who could not open it.
func TestPageCommentsReachOnlyWhoSeesThePage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		hidden := uuid.NewString()
		w.push("owner", "project", hidden, map[string]any{"name": "Hidden", "sort_key": "a1"})
		page := w.page("owner", hidden, "Secret")
		thread := w.thread("owner", page)
		w.threadComment("owner", page, thread, "Ask "+w.mention("guest"))
		w.run()
		want(t, w.inbox("guest"))

		// The workspace's own wiki is never a guest's either.
		main := syncer.MainPageID(w.ws)
		w.push("owner", "page", main, map[string]any{"sort_key": "a0"})
		onWiki := w.thread("owner", main)
		w.threadComment("owner", main, onWiki, "Ask "+w.mention("guest"))
		w.run()
		want(t, w.inbox("guest"))
	})
}

// Sending the page to the trash takes its notifications out of lists, and
// they come back with it.
func TestPageNotificationsFollowThePage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		page := w.page("member", w.project, "Roasting schedule")
		thread := w.thread("member", page)
		w.threadComment("member", page, thread, "Ask "+w.mention("owner"))
		w.run()
		want(t, w.inbox("owner"), "mention: Dana mentioned you in Roasting schedule")
		w.push("member", "page", page, map[string]any{"deleted_at": "2026-11-03T00:00:00Z"})
		w.run()
		want(t, w.inbox("owner"))
	})
}
