package syncer_test

import (
	"context"
	"database/sql"
	"encoding/json"
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

func later(n int) string { return hlcAt(time.Now().Add(time.Duration(n)*time.Millisecond), "a") }

func TestTaskDepthRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		p1, p2 := uuid.NewString(), uuid.NewString()
		task, other := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("project", p1, later(0), newProject("One")),
			op("project", p2, later(0), newProject("Two")),
			op("task", task, later(1), newTask(p1, nil, "Parent")),
			op("task", other, later(1), newTask(p2, nil, "Elsewhere")),
		)
		comment := uuid.NewString()
		wantStatus(t, w.push("owner", op("comment", comment, later(2), map[string]any{
			"task_id": task, "author_id": w.users["owner"], "body": "Mine", "edited_at": nil, "deleted_at": nil}))[0], "applied")

		cases := map[string]struct {
			role string
			op   syncer.Op
		}{
			"comment as someone else": {"member", op("comment", uuid.NewString(), later(3), map[string]any{
				"task_id": task, "author_id": w.users["owner"], "body": "Forged", "edited_at": nil, "deleted_at": nil})},
			"editing another's comment": {"member", op("comment", comment, later(3), map[string]any{"body": "Changed"})},
			"attachment from a client": {"owner", op("attachment", uuid.NewString(), later(3), map[string]any{
				"task_id": task, "filename": "x.txt", "mime": "text/plain", "size": 1, "storage_key": "k", "uploaded_by": w.users["owner"], "deleted_at": nil})},
			"assigning a stranger": {"owner", op("task_assignee", uuid.NewString(), later(3), map[string]any{
				"task_id": task, "user_id": uuid.NewString(), "deleted_at": nil})},
			"subtask in another project": {"owner", op("task", uuid.NewString(), later(3), merge(newTask(p2, nil, "Stray"), map[string]any{"parent_id": task}))},
			"task under itself":          {"owner", op("task", task, later(3), map[string]any{"parent_id": task})},
			"bad date":                   {"owner", op("task", task, later(3), map[string]any{"due_date": "2026-13-45"})},
			"bad time":                   {"owner", op("task", task, later(3), map[string]any{"due_time": "25:00"})},
			"bad priority":               {"owner", op("task", task, later(3), map[string]any{"priority": "whenever"})},
		}
		for name, c := range cases {
			t.Run(name, func(t *testing.T) {
				wantStatus(t, w.push(c.role, c.op)[0], "rejected")
			})
		}

		// An admin may delete someone's comment; the owner here is an admin.
		mine := uuid.NewString()
		w.push("member", op("comment", mine, later(4), map[string]any{
			"task_id": task, "author_id": w.users["member"], "body": "From a member", "edited_at": nil, "deleted_at": nil}))
		wantStatus(t, w.push("owner", op("comment", mine, later(5), map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339Nano)}))[0], "applied")

		// Deeper cycles are refused too.
		sub := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", sub, later(6), merge(newTask(p1, nil, "Child"), map[string]any{"parent_id": task})))[0], "applied")
		wantStatus(t, w.push("owner", op("task", task, later(7), map[string]any{"parent_id": sub}))[0], "rejected")

		// A member can be assigned; the same pair always makes the same row.
		assign := pairID(task, w.users["member"])
		wantStatus(t, w.push("owner", op("task_assignee", assign, later(8), map[string]any{"task_id": task, "user_id": w.users["member"], "deleted_at": nil}))[0], "applied")
		wantStatus(t, w.push("member", op("task_assignee", assign, later(9), map[string]any{"task_id": task, "user_id": w.users["member"], "deleted_at": nil}))[0], "applied")
	})
}

func TestGuestsSeeLabelsAndSharedChildren(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		shared, hidden := uuid.NewString(), uuid.NewString()
		t1, t2, label := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("project", shared, later(0), newProject("Shared")),
			op("project", hidden, later(0), newProject("Hidden")),
			op("task", t1, later(1), newTask(shared, nil, "Visible")),
			op("task", t2, later(1), newTask(hidden, nil, "Not visible")),
			op("label", label, later(1), map[string]any{"name": "Urgent", "color": "red", "sort_key": "a0", "deleted_at": nil}),
			op("checklist_item", uuid.NewString(), later(2), map[string]any{"task_id": t1, "text": "Seen", "done": false, "sort_key": "a0", "deleted_at": nil}),
			op("checklist_item", uuid.NewString(), later(2), map[string]any{"task_id": t2, "text": "Unseen", "done": false, "sort_key": "a0", "deleted_at": nil}),
		)
		_, err := d.ExecContext(ctx, rebind(d, "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"), shared, w.users["guest"], store.Now())
		must(t, err)
		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		seen := map[string]int{}
		for _, r := range snap.Rows {
			seen[r.Entity]++
			if r.Entity == "checklist_item" && r.Fields["text"] != "Seen" {
				t.Errorf("guest sees a checklist item from a hidden project")
			}
		}
		if seen["label"] != 1 || seen["checklist_item"] != 1 || seen["task"] != 1 {
			t.Fatalf("guest sees %v", seen)
		}
		changes, err := w.svc.Pull(ctx, w.users["guest"], w.ws, 0)
		must(t, err)
		pulled := map[string]int{}
		for _, c := range changes.Changes {
			pulled[c.Entity]++
		}
		if pulled["label"] != 1 || pulled["checklist_item"] != 1 {
			t.Fatalf("guest pulls %v", pulled)
		}
	})
}

func TestActivityTrashAndPurge(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		p, task := uuid.NewString(), uuid.NewString()
		w.push("owner", op("project", p, later(0), newProject("Home")), op("task", task, later(1), newTask(p, nil, "Paint")))
		w.push("member", op("task", task, later(2), map[string]any{"title": "Paint the fence", "priority": "high"}))
		w.push("member", op("task", task, later(3), map[string]any{"sort_key": "b00"})) // not news
		deleted := time.Now().UTC().Format(time.RFC3339Nano)
		w.push("owner", op("task", task, later(4), map[string]any{"deleted_at": deleted}))

		trash, err := w.svc.Trash(ctx, w.users["owner"], w.ws)
		must(t, err)
		if len(trash) != 1 || trash[0].ID != task {
			t.Fatalf("trash = %+v", trash)
		}
		w.push("owner", op("task", task, later(5), map[string]any{"deleted_at": nil}))

		events, err := w.svc.Activity(ctx, w.users["member"], w.ws, task)
		must(t, err)
		var kinds []string
		for _, e := range events {
			kinds = append(kinds, e.Kind)
		}
		want := []string{"restored", "deleted", "changed", "created"}
		if len(kinds) != len(want) {
			t.Fatalf("activity kinds = %v, want %v", kinds, want)
		}
		for i := range want {
			if kinds[i] != want[i] {
				t.Fatalf("activity kinds = %v, want %v", kinds, want)
			}
		}
		if events[2].ActorName != "member" || events[2].Fields["title"] != "Paint the fence" {
			t.Fatalf("change event = %+v", events[2])
		}

		// Purge removes only what has been in the trash long enough.
		old := time.Now().Add(-(syncer.TrashDays + 1) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		gone := uuid.NewString()
		w.push("owner", op("task", gone, later(6), merge(newTask(p, nil, "Old"), map[string]any{"deleted_at": old})))
		n, err := w.svc.Purge(ctx)
		must(t, err)
		if n != 1 {
			t.Fatalf("purged %d rows, want 1", n)
		}
		snap, _ := w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		if len(snap.Rows) != 2 {
			t.Fatalf("after purge %d rows, want project and restored task", len(snap.Rows))
		}
	})
}

func TestMyDayIsPrivate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		task := uuid.NewString()
		w.push("owner", op("task", task, later(0), newTask(nil, nil, "Shared task")))
		entry := pairID(task, w.users["owner"])
		wantStatus(t, w.push("owner", op("my_day_entry", entry, later(1), map[string]any{
			"task_id": task, "user_id": w.users["owner"], "day": "2026-10-28", "sort_key": "a0", "deleted_at": nil}))[0], "applied")
		// Nobody can put a task in someone else's day, or move theirs.
		wantStatus(t, w.push("member", op("my_day_entry", uuid.NewString(), later(2), map[string]any{
			"task_id": task, "user_id": w.users["owner"], "day": "2026-10-28", "sort_key": "a0", "deleted_at": nil}))[0], "rejected")
		res := w.push("member", op("my_day_entry", entry, later(3), map[string]any{"day": "2026-10-29"}))[0]
		wantStatus(t, res, "rejected")
		if res.Row != nil {
			t.Fatal("a refused edit must not reveal someone else's entry")
		}

		count := func(role string) (int, int) {
			snap, err := w.svc.Bootstrap(ctx, w.users[role], w.ws)
			must(t, err)
			changes, err := w.svc.Pull(ctx, w.users[role], w.ws, 0)
			must(t, err)
			n, m := 0, 0
			for _, r := range snap.Rows {
				if r.Entity == "my_day_entry" {
					n++
				}
			}
			for _, c := range changes.Changes {
				if c.Entity == "my_day_entry" {
					m++
				}
			}
			return n, m
		}
		if n, m := count("owner"); n != 1 || m != 1 {
			t.Fatalf("owner sees %d entries, pulls %d", n, m)
		}
		if n, m := count("member"); n != 0 || m != 0 {
			t.Fatalf("member sees %d entries, pulls %d", n, m)
		}
	})
}

func TestFieldsAndViews(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		field := uuid.NewString()
		wantStatus(t, w.push("owner", op("custom_field", field, later(0), map[string]any{
			"project_id": nil, "name": "Cost", "type": "number", "options": []any{}, "sort_key": "a0", "deleted_at": nil}))[0], "applied")
		wantStatus(t, w.push("owner", op("custom_field", uuid.NewString(), later(1), map[string]any{
			"project_id": nil, "name": "Odd", "type": "color", "sort_key": "a1"}))[0], "rejected")
		// A field's type is fixed once made.
		wantStatus(t, w.push("owner", op("custom_field", field, later(2), map[string]any{"type": "text"}))[0], "rejected")
		task := uuid.NewString()
		w.push("owner", op("task", task, later(3), newTask(nil, nil, "Paint")))
		wantStatus(t, w.push("member", op("custom_field_value", pairID(task, field), later(4), map[string]any{
			"task_id": task, "field_id": field, "value": 42.5, "deleted_at": nil}))[0], "applied")
		wantStatus(t, w.push("member", op("custom_field_value", pairID(task, field), later(5), map[string]any{
			"value": json.RawMessage(`{"broken"`)}))[0], "rejected")

		shared, mine := uuid.NewString(), uuid.NewString()
		view := func(owner any, name string) map[string]any {
			return map[string]any{"project_id": nil, "owner_id": owner, "name": name, "type": "board",
				"config": map[string]any{"priority": []string{"high"}}, "sort_key": "a0", "deleted_at": nil}
		}
		wantStatus(t, w.push("owner", op("saved_view", shared, later(6), view(nil, "Hot")))[0], "applied")
		wantStatus(t, w.push("owner", op("saved_view", mine, later(7), view(w.users["owner"], "Mine")))[0], "applied")
		// Nobody makes a view in someone else's name or edits their private one.
		wantStatus(t, w.push("member", op("saved_view", uuid.NewString(), later(8), view(w.users["owner"], "Theirs")))[0], "rejected")
		wantStatus(t, w.push("member", op("saved_view", mine, later(9), map[string]any{"name": "Taken"}))[0], "rejected")
		// A shared view is anyone's to edit.
		wantStatus(t, w.push("member", op("saved_view", shared, later(10), map[string]any{"name": "Hotter"}))[0], "applied")

		seen := func(role string) map[string]int {
			snap, err := w.svc.Bootstrap(ctx, w.users[role], w.ws)
			must(t, err)
			changes, err := w.svc.Pull(ctx, w.users[role], w.ws, 0)
			must(t, err)
			n := map[string]int{}
			for _, r := range snap.Rows {
				n[r.Entity]++
			}
			for _, c := range changes.Changes {
				if c.Entity == "saved_view" {
					n["pulled view"]++
				}
			}
			return n
		}
		if n := seen("owner"); n["saved_view"] != 2 || n["custom_field"] != 1 || n["custom_field_value"] != 1 {
			t.Fatalf("owner sees %v", n)
		}
		if n := seen("member"); n["saved_view"] != 1 || n["pulled view"] != 2 { // the shared view, made then renamed
			t.Fatalf("member sees %v", n)
		}
		// JSON fields go back out as the JSON that came in, not as text.
		snap, err := w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.ID == shared {
				if got, _ := json.Marshal(r.Fields["config"]); string(got) != `{"priority":["high"]}` {
					t.Fatalf("config came back as %s", got)
				}
			}
		}
		// Guests see workspace-wide fields, as they see labels.
		if n := seen("guest"); n["custom_field"] != 1 || n["saved_view"] != 1 {
			t.Fatalf("guest sees %v", n)
		}
	})
}

// A view's display (sort, grouping, columns, visible fields) is a JSON
// field of its own: it goes out as the JSON that came in, a view made by
// an app from before it has none, and guests cannot change it.
func TestViewDisplay(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		field := uuid.NewString()
		display := `{"sort":[{"by":"field","field":"` + field + `","dir":"asc"},{"by":"due","dir":"desc"}],` +
			`"group":{"by":"field","field":"` + field + `"},"fields":{"list":{"show":["` + field + `"],"hide":["labels"]}},"later":{"x":1}}`
		withDisplay, old := uuid.NewString(), uuid.NewString()
		wantStatus(t, w.push("owner", op("saved_view", withDisplay, later(0), map[string]any{"project_id": nil, "owner_id": nil,
			"name": "Sorted", "type": "list", "config": map[string]any{"filter": map[string]any{}},
			"display": json.RawMessage(display), "sort_key": "a0", "deleted_at": nil}))[0], "applied")
		// As an app from before display would make it.
		wantStatus(t, w.push("owner", op("saved_view", old, later(1), map[string]any{"project_id": nil, "owner_id": nil,
			"name": "Old", "type": "list", "config": map[string]any{"filter": map[string]any{}}, "sort_key": "a1", "deleted_at": nil}))[0], "applied")
		// Members change a shared view's display; guests change nothing.
		wantStatus(t, w.push("member", op("saved_view", old, later(2), map[string]any{"display": map[string]any{"group": map[string]any{"by": "priority"}}}))[0], "applied")
		wantStatus(t, w.push("guest", op("saved_view", withDisplay, later(3), map[string]any{"display": map[string]any{}}))[0], "rejected")
		wantStatus(t, w.push("owner", op("saved_view", uuid.NewString(), later(4), map[string]any{"project_id": nil, "owner_id": nil,
			"name": "Big", "type": "list", "config": map[string]any{}, "display": strings.Repeat(" ", 10_001), "sort_key": "a2"}))[0], "rejected")

		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		got := map[string]string{}
		for _, r := range snap.Rows {
			if r.Entity == "saved_view" {
				raw, _ := json.Marshal(r.Fields["display"])
				got[r.ID] = string(raw)
			}
		}
		if got[withDisplay] != display || got[old] != `{"group":{"by":"priority"}}` {
			t.Fatalf("displays came back as %v", got)
		}
		// A view no one has given a display has none.
		fresh := uuid.NewString()
		wantStatus(t, w.push("owner", op("saved_view", fresh, later(5), map[string]any{"project_id": nil, "owner_id": nil,
			"name": "Fresh", "type": "board", "config": map[string]any{}, "sort_key": "a3"}))[0], "applied")
		snap, err = w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		must(t, err)
		if r := find(snap.Rows, "saved_view", fresh); r == nil || r.Fields["display"] != nil {
			t.Fatalf("a view made without a display: %+v", r)
		}
	})
}

func TestStorageKeysIncludePictures(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		p, task := uuid.NewString(), uuid.NewString()
		w.push("owner", op("project", p, later(0), newProject("One")), op("task", task, later(1), newTask(p, nil, "Has a file")))
		if _, err := w.svc.CreateAttachment(ctx, w.users["owner"], w.ws, syncer.NewAttachment{
			ID: uuid.NewString(), TaskID: task, Filename: "a.txt", Mime: "text/plain", StorageKey: "file-key", Size: 1,
		}); err != nil {
			t.Fatal(err)
		}
		must(t, w.st.SetUserAvatar(ctx, sqlitedb.SetUserAvatarParams{
			ID: w.users["member"], Avatar: sql.NullString{String: "picture-key", Valid: true}, Now: store.Now(),
		}))
		keys, err := w.svc.StorageKeys(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 2 || !keys["file-key"] || !keys["picture-key"] {
			t.Fatalf("StorageKeys = %v", keys)
		}
	})
}

// A comment's text is not in its task's history, which everyone who sees
// the task can read for good, nor in an export: deleting the comment must
// take the text with it.
func TestCommentTextLeavesNoHistory(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		p, task, comment := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner", op("project", p, later(0), newProject("Home")), op("task", task, later(1), newTask(p, nil, "Paint")))
		wantStatus(t, w.push("owner", op("comment", comment, later(2), map[string]any{
			"task_id": task, "author_id": w.users["owner"], "body": "the alarm code is 4321", "edited_at": nil, "deleted_at": nil}))[0], "applied")
		w.push("owner", op("comment", comment, later(3), map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339Nano)}))

		events, err := w.svc.Activity(ctx, w.users["member"], w.ws, task)
		must(t, err)
		dump, err := w.svc.Dump(ctx, w.users["owner"], w.ws)
		must(t, err)
		for _, e := range events {
			if _, ok := e.Fields["body"]; ok && e.Entity == "comment" {
				t.Errorf("history keeps a comment's text: %+v", e)
			}
		}
		for _, e := range dump.Activity {
			if _, ok := e.Fields["body"]; ok && e.Entity == "comment" {
				t.Errorf("the export's history keeps a comment's text: %+v", e)
			}
		}
	})
}

// A guest sees the names of members and of guests they share a project
// with in a task's history and in an export, and "Someone" for a guest
// they share none with.
func TestGuestsSeeNames(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		p, other, task := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner", op("project", p, later(0), newProject("Shared")), op("task", task, later(1), newTask(p, nil, "Paint")),
			op("project", other, later(2), newProject("Other")))
		w.share(p, true)
		w.push("guest", op("task", task, later(3), map[string]any{"title": "Paint it blue"}))

		// A second guest works on the task while they have the project.
		now := store.Now()
		second := uuid.NewString()
		must(t, w.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: second, Email: "second-" + second[:8] + "@example.com", Name: "second", Now: now}))
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: second, Role: "guest", Now: now}))
		_, err := w.st.GrantProjectAccess(ctx, sqlitedb.GrantProjectAccessParams{ProjectID: p, UserID: second, Now: now})
		must(t, err)
		_, _, err = w.svc.Push(ctx, second, w.ws, []syncer.Op{op("task", task, later(4), map[string]any{"title": "Paint it green"})})
		must(t, err)
		named := func(want string) {
			t.Helper()
			events, err := w.svc.Activity(ctx, w.users["guest"], w.ws, task)
			must(t, err)
			dump, err := w.svc.Dump(ctx, w.users["guest"], w.ws)
			must(t, err)
			names := map[string]string{}
			for _, e := range events {
				names[e.ActorID] = e.ActorName
			}
			// Someone's ID is left out of history with their name.
			secondID := second
			if want == syncer.Someone {
				secondID = ""
				if _, ok := names[second]; ok {
					t.Errorf("history names the hidden guest by their ID: %v", names)
				}
			}
			for _, h := range dump.Activity {
				id := h.ActorID
				if h.ActorName == syncer.Someone {
					id = ""
				}
				if names[id] != h.ActorName {
					t.Errorf("the export says %q where history says %q", h.ActorName, names[id])
				}
			}
			if names[w.users["owner"]] != "owner" || names[w.users["guest"]] != "guest" || names[secondID] != want {
				t.Errorf("names the guest sees = %v, want the second guest as %q", names, want)
			}
		}
		named("second")

		// Moved to another project, they share none with the first guest.
		_, err = w.st.RevokeProjectAccess(ctx, sqlitedb.RevokeProjectAccessParams{ProjectID: p, UserID: second})
		must(t, err)
		_, err = w.st.GrantProjectAccess(ctx, sqlitedb.GrantProjectAccessParams{ProjectID: other, UserID: second, Now: now})
		must(t, err)
		named(syncer.Someone)

		events, err := w.svc.Activity(ctx, w.users["member"], w.ws, task)
		must(t, err)
		for _, e := range events {
			if e.ActorName == syncer.Someone {
				t.Errorf("a member sees %+v", e)
			}
		}
	})
}

// A new row is held to what the app makes: a pair's ID from its pair, and
// a field's value on a task in the field's project.
func TestNewRowsAreWhatTheAppMakes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		home, work, task, label, field := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("project", home, later(0), newProject("Home")),
			op("project", work, later(0), newProject("Work")),
			op("task", task, later(1), newTask(home, nil, "Paint")),
			op("label", label, later(1), map[string]any{"name": "Urgent", "color": "red", "sort_key": "a0", "deleted_at": nil}),
			op("custom_field", field, later(1), map[string]any{
				"project_id": work, "name": "Shift", "type": "text", "options": []any{}, "sort_key": "a0", "deleted_at": nil}),
		)
		// Another ID for the pair would take the place of the real one.
		wantStatus(t, w.push("member", op("task_label", uuid.NewString(), later(2), map[string]any{
			"task_id": task, "label_id": label, "deleted_at": nil}))[0], "rejected")
		wantStatus(t, w.push("member", op("task_label", pairID(task, label), later(3), map[string]any{
			"task_id": task, "label_id": label, "deleted_at": nil}))[0], "applied")
		// Work's field has no place on a Home task.
		wantStatus(t, w.push("member", op("custom_field_value", pairID(task, field), later(4), map[string]any{
			"task_id": task, "field_id": field, "value": "Nights", "deleted_at": nil}))[0], "rejected")
	})
}

// A task goes only to someone who can reach it. Taking someone off is
// always allowed, and someone already assigned stays when they lose it.
func TestAssigningNeedsReach(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shared, other, inTask, otherTask, inboxTask := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner", op("project", shared, later(0), newProject("Shared")), op("project", other, later(1), newProject("Other")),
			op("task", inTask, later(2), newTask(shared, nil, "In shared")),
			op("task", otherTask, later(3), newTask(other, nil, "In other")),
			op("task", inboxTask, later(4), newTask(nil, nil, "In the inbox")))
		w.share(shared, true)

		assign := func(task string, at int, deleted any) syncer.Result {
			return w.push("owner", op("task_assignee", syncer.PairID(task, w.users["guest"]), later(at),
				map[string]any{"task_id": task, "user_id": w.users["guest"], "deleted_at": deleted}))[0]
		}
		for _, task := range []string{otherTask, inboxTask} {
			res := assign(task, 10, nil)
			if res.Status != "rejected" || !strings.Contains(res.Message, "cannot reach") {
				t.Fatalf("assigning a guest to a task out of reach: %s (%s)", res.Status, res.Message)
			}
		}
		wantStatus(t, assign(inTask, 11, nil), "applied")

		// Everyone else can have any of them.
		for i, task := range []string{otherTask, inboxTask} {
			wantStatus(t, w.push("owner", op("task_assignee", syncer.PairID(task, w.users["member"]), later(20+i),
				map[string]any{"task_id": task, "user_id": w.users["member"], "deleted_at": nil}))[0], "applied")
		}

		// Losing the project keeps the assignment, but it cannot come back
		// once taken off, and taking off always works.
		w.share(shared, false)
		wantStatus(t, assign(inTask, 30, store.Now().Format(time.RFC3339Nano)), "applied")
		res := assign(inTask, 31, nil)
		if res.Status != "rejected" {
			t.Fatalf("bringing back an assignment out of reach: %s", res.Status)
		}
		w.share(shared, true)
		wantStatus(t, assign(inTask, 32, nil), "applied")
	})
}
