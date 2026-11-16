package syncer_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// threadWorld is a project with a page, and a thread on it by the member.
func threadWorld(t *testing.T, w *world) (project, page, thread, comment string) {
	t.Helper()
	project, page, thread, comment = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	main := syncer.MainPageID(project)
	for _, r := range w.push("owner", op("project", project, later(0), newProject("Garden")),
		op("page", main, later(1), newPage(project, nil, "Garden")),
		op("page", page, later(2), newPage(project, main, "Plan"))) {
		wantStatus(t, r, "applied")
	}
	for _, r := range w.push("member", op("page_thread", thread, later(3), map[string]any{
		"page_id": page, "created_by": w.users["member"], "quote": "the soil", "anchor": "pinned",
	}), op("page_comment", comment, later(4), map[string]any{
		"page_id": page, "thread_id": thread, "author_id": w.users["member"], "body": "Which soil?",
	})) {
		wantStatus(t, r, "applied")
	}
	return project, page, thread, comment
}

func TestPageThreadsAreEveryones(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, _, thread, comment := threadWorld(t, w)

		// Anyone who can comment resolves and reopens a thread.
		wantStatus(t, w.push("owner", op("page_thread", thread, later(5), map[string]any{
			"resolved_at": "2026-11-03T00:00:00Z",
		}))[0], "applied")
		// Where it is pinned and the words it is on stay as they were.
		wantStatus(t, w.push("owner", op("page_thread", thread, later(6), map[string]any{"quote": "other words"}))[0], "rejected")
		wantStatus(t, w.push("owner", op("page_thread", thread, later(7), map[string]any{"anchor": "elsewhere"}))[0], "rejected")

		// A comment stays its author's; an admin may only delete it.
		wantStatus(t, w.push("owner", op("page_comment", comment, later(8), map[string]any{"body": "Mine now"}))[0], "rejected")
		wantStatus(t, w.push("owner", op("page_comment", comment, later(9), map[string]any{"deleted_at": "2026-11-03T00:00:00Z"}))[0], "applied")
	})
}

// A thread's comments are their authors' to delete, so deleting a thread that
// holds someone else's is for owners and admins.
func TestDeletingAThread(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, page, thread, _ := threadWorld(t, w)
		gone := map[string]any{"deleted_at": "2026-11-03T00:00:00Z"}
		// The member wrote all of it, so far: the owner's reply changes that.
		wantStatus(t, w.push("owner", op("page_comment", uuid.NewString(), later(5), map[string]any{
			"page_id": page, "thread_id": thread, "author_id": w.users["owner"], "body": "Mine",
		}))[0], "applied")
		wantStatus(t, w.push("member", op("page_thread", thread, later(6), gone))[0], "rejected")
		wantStatus(t, w.push("owner", op("page_thread", thread, later(7), gone))[0], "applied")

		// A thread of one's own words alone is one's own to delete.
		mine := uuid.NewString()
		wantStatus(t, w.push("member", op("page_thread", mine, later(8), map[string]any{
			"page_id": page, "created_by": w.users["member"], "quote": "more",
		}))[0], "applied")
		wantStatus(t, w.push("member", op("page_comment", uuid.NewString(), later(9), map[string]any{
			"page_id": page, "thread_id": mine, "author_id": w.users["member"], "body": "Mine",
		}))[0], "applied")
		wantStatus(t, w.push("member", op("page_thread", mine, later(10), gone))[0], "applied")
	})
}

// An app from before page templates reads one as a task template, and must not
// be able to write tasks over its pages.
func TestPageTemplatesHoldTheirPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id := uuid.NewString()
		pages := map[string]any{"v": 1, "tasks": []any{}, "pages": []any{map[string]any{"id": "a", "parent": nil, "title": "T", "body": "B"}}}
		wantStatus(t, w.push("owner", op("template", id, later(0), map[string]any{
			"kind": "page", "name": "Notes", "data": pages, "sort_key": "a0",
		}))[0], "applied")
		wantStatus(t, w.push("owner", op("template", id, later(1), map[string]any{
			"data": map[string]any{"v": 1, "tasks": []any{}},
		}))[0], "rejected")
		wantStatus(t, w.push("owner", op("template", uuid.NewString(), later(2), map[string]any{
			"kind": "page", "name": "Empty", "data": map[string]any{"v": 1, "tasks": []any{}}, "sort_key": "a1",
		}))[0], "rejected")
	})
}

func TestPageCommentMustBeOnItsThreadsPage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		project, _, thread, _ := threadWorld(t, w)
		other := uuid.NewString()
		wantStatus(t, w.push("owner", op("page", other, later(10), newPage(project, syncer.MainPageID(project), "Other")))[0], "applied")
		wantStatus(t, w.push("member", op("page_comment", uuid.NewString(), later(11), map[string]any{
			"page_id": other, "thread_id": thread, "author_id": w.users["member"], "body": "Astray",
		}))[0], "rejected")
	})
}

// A guest has only the threads of the projects they are given, as they have
// only those projects' pages, and can write in them.
func TestGuestsAndPageThreads(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		guest := w.users["guest"]
		project, page, thread, _ := threadWorld(t, w)

		seen := func() map[string]int {
			snap, err := w.svc.Bootstrap(ctx, guest, w.ws)
			must(t, err)
			out := map[string]int{}
			for _, r := range snap.Rows {
				out[r.Entity]++
			}
			return out
		}
		if got := seen(); got["page_thread"] != 0 || got["page_comment"] != 0 {
			t.Fatalf("a guest without the project has its threads: %v", got)
		}
		wantStatus(t, w.push("guest", op("page_comment", uuid.NewString(), later(12), map[string]any{
			"page_id": page, "thread_id": thread, "author_id": guest, "body": "Let me in",
		}))[0], "rejected")

		w.share(project, true)
		if got := seen(); got["page_thread"] != 1 || got["page_comment"] != 1 {
			t.Fatalf("a guest with the project lacks its threads: %v", got)
		}
		wantStatus(t, w.push("guest", op("page_comment", uuid.NewString(), later(13), map[string]any{
			"page_id": page, "thread_id": thread, "author_id": guest, "body": "Now I can",
		}))[0], "applied")
	})
}

// An import makes a page's document again, so a pin means nothing there: the
// thread keeps its words and finds its place by them. Its IDs follow the page.
func TestRestoreRepinsThreadsByTheirWords(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner, member := w.users["owner"], w.users["member"]
		_, _, thread, comment := threadWorld(t, w)
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{
			FreshIDs: true, Owner: member, Exporter: owner, From: w.ws,
		})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: member, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, member, into)
		must(t, err)
		var threads, comments []syncer.Row
		pages := map[string]bool{}
		for _, r := range snap.Rows {
			switch r.Entity {
			case "page_thread":
				threads = append(threads, r)
			case "page_comment":
				comments = append(comments, r)
			case "page":
				pages[r.ID] = true
			}
		}
		if len(threads) != 1 || len(comments) != 1 {
			t.Fatalf("restored %d threads and %d comments", len(threads), len(comments))
		}
		if threads[0].ID == thread || comments[0].ID == comment {
			t.Error("kept the old IDs")
		}
		if threads[0].Fields["anchor"] != "" || threads[0].Fields["quote"] != "the soil" {
			t.Errorf("thread %v", threads[0].Fields)
		}
		if comments[0].Fields["thread_id"] != threads[0].ID || !pages[comments[0].Fields["page_id"].(string)] {
			t.Errorf("comment %v", comments[0].Fields)
		}
	})
}
