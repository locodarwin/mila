package syncer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// filled is a workspace with something of everything: two projects (one
// shared with the guest), statuses, a label on a task, a subtask, a task
// in the trash, a repeating task and the copy that follows it, an
// assignment, comments, My Day entries of two people, a reminder, a field
// with a value and a view whose filter names a project, a label and the
// field, and whose display names the field.
type filled struct {
	*world
	shared, private, status, label, field, view string
	task, sub, trashed, first, next             string
	item, nextItem                              string
}

func fill(t *testing.T, d *db.DB) *filled {
	t.Helper()
	ctx := context.Background()
	w := newWorld(t, d)
	f := &filled{world: w, shared: uuid.NewString(), private: uuid.NewString(), status: uuid.NewString(),
		label: uuid.NewString(), field: uuid.NewString(), view: uuid.NewString(), task: uuid.NewString(),
		sub: uuid.NewString(), trashed: uuid.NewString(), first: uuid.NewString(), item: uuid.NewString()}
	f.next = recur.NextTaskID(f.first)
	f.nextItem = recur.HashID(f.next + "/" + f.item)
	h := hlcAt(time.Now().Add(-time.Minute), "a")
	owner, member := w.users["owner"], w.users["member"]
	sub := newTask(f.shared, f.status, "Subtask")
	sub["parent_id"] = f.task
	repeating := newTask(f.shared, nil, "Water plants")
	repeating["recurrence"], repeating["series_id"] = "FREQ=DAILY", f.first
	follows := newTask(f.shared, nil, "Water plants")
	follows["recurrence"], follows["series_id"], follows["created_by"] = "FREQ=DAILY", f.first, owner
	trashed := newTask(f.private, nil, "Old idea")
	trashed["deleted_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	task := newTask(f.shared, f.status, "Write the report")
	task["created_by"] = owner
	task["notes"] = "First line\nsecond: line"
	for _, res := range w.push("owner",
		op("project", f.shared, h, newProject("Shared")),
		op("project", f.private, h, newProject("Private")),
		op("status", f.status, h, newStatus(f.shared, "To do", "todo")),
		op("label", f.label, h, map[string]any{"name": "urgent", "color": "red", "sort_key": "a0", "deleted_at": nil}),
		op("custom_field", f.field, h, map[string]any{"project_id": nil, "name": "Size", "type": "select",
			"options": []map[string]string{{"id": "o1", "name": "Large", "color": "red"}}, "sort_key": "a0", "deleted_at": nil}),
		op("saved_view", f.view, h, map[string]any{"project_id": nil, "owner_id": nil, "name": "Urgent", "type": "list",
			"config": map[string]any{"filter": map[string]any{"projects": []string{f.shared}, "labels": []string{f.label},
				"fields": []map[string]any{{"field": f.field, "op": "any", "value": []string{"o1"}}}}},
			"display": map[string]any{"sort": []map[string]any{{"by": "field", "field": f.field, "dir": "asc"}},
				"group": map[string]any{"by": "field", "field": f.field}, "columns": map[string]any{"by": "field", "field": f.field},
				"fields": map[string]any{"list": map[string]any{"show": []string{f.field}}}},
			"sort_key": "a0", "deleted_at": nil}),
		op("task", f.task, h, task),
		op("task", f.sub, h, sub),
		op("task", f.trashed, h, trashed),
		op("task", f.first, h, repeating),
		op("task", f.next, h, follows),
		op("checklist_item", f.item, h, map[string]any{"task_id": f.first, "text": "Ferns", "done": false, "sort_key": "a0", "deleted_at": nil}),
		op("checklist_item", f.nextItem, h, map[string]any{"task_id": f.next, "text": "Ferns", "done": false, "sort_key": "a0", "deleted_at": nil}),
		op("task_label", syncer.PairID(f.task, f.label), h, map[string]any{"task_id": f.task, "label_id": f.label, "deleted_at": nil}),
		op("task_assignee", syncer.PairID(f.task, member), h, map[string]any{"task_id": f.task, "user_id": member, "deleted_at": nil}),
		op("task_assignee", syncer.PairID(f.sub, owner), h, map[string]any{"task_id": f.sub, "user_id": owner, "deleted_at": nil}),
		op("custom_field_value", syncer.PairID(f.task, f.field), h, map[string]any{"task_id": f.task, "field_id": f.field, "value": "o1", "deleted_at": nil}),
		op("my_day_entry", syncer.PairID(f.task, owner), h, map[string]any{"task_id": f.task, "user_id": owner, "day": "2026-10-28", "sort_key": "a0", "deleted_at": nil}),
		op("reminder", uuid.NewString(), h, map[string]any{"task_id": f.task, "user_id": owner, "remind_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "offset_minutes": nil, "deleted_at": nil}),
	) {
		wantStatus(t, res, "applied")
	}
	for _, res := range w.push("member",
		op("comment", uuid.NewString(), hlcAt(time.Now().Add(-time.Minute), "m"), map[string]any{"task_id": f.task, "author_id": member, "body": "On it, @[owner](user:" + owner + ")"}),
		op("my_day_entry", syncer.PairID(f.task, member), hlcAt(time.Now().Add(-time.Minute), "m"), map[string]any{"task_id": f.task, "user_id": member, "day": "2026-10-28", "sort_key": "a0", "deleted_at": nil}),
	) {
		wantStatus(t, res, "applied")
	}
	_, err := d.ExecContext(ctx, rebind(d, "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"), f.shared, w.users["guest"], store.Now())
	must(t, err)
	return f
}

func counts(rows []syncer.Row) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Entity]++
	}
	return out
}

func find(rows []syncer.Row, entity, id string) *syncer.Row {
	for i := range rows {
		if rows[i].Entity == entity && rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func TestDumpHoldsWhatTheMemberSees(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := fill(t, d)
		dump, err := f.svc.Dump(ctx, f.users["owner"], f.ws)
		must(t, err)
		// The trash comes along; the member's own My Day does not, nor do
		// the documents' rows, whose Markdown the notes hold.
		want := map[string]int{"project": 2, "status": 1, "label": 1, "custom_field": 1, "saved_view": 1, "task": 5,
			"checklist_item": 2, "task_label": 1, "task_assignee": 2, "comment": 1, "my_day_entry": 1, "reminder": 1, "custom_field_value": 1}
		if got := counts(dump.Rows); !equalCounts(got, want) {
			t.Errorf("the owner's dump holds %v, want %v", got, want)
		}
		if find(dump.Rows, "task", f.trashed) == nil {
			t.Error("a task in the trash is missing")
		}
		if len(dump.Activity) == 0 || dump.Activity[0].TaskID == "" {
			t.Error("the history is missing")
		}
		for _, h := range dump.Activity {
			if h.Entity == "my_day_entry" || h.Entity == "reminder" {
				t.Errorf("a private row is in the history: %+v", h)
			}
		}
		sort.Strings(dump.People)
		people := []string{f.users["owner"], f.users["member"]}
		sort.Strings(people)
		if !equalStrings(dump.People, people) {
			t.Errorf("people %v, want %v", dump.People, people)
		}

		guest, err := f.svc.Dump(ctx, f.users["guest"], f.ws)
		must(t, err)
		if find(guest.Rows, "project", f.private) != nil || find(guest.Rows, "task", f.trashed) != nil {
			t.Error("a guest's dump holds a project not shared with them")
		}
		for _, r := range guest.Rows {
			if r.Entity == "my_day_entry" || r.Entity == "reminder" {
				t.Errorf("a guest's dump holds someone's private %s", r.Entity)
			}
		}
		if find(guest.Rows, "task", f.task) == nil || find(guest.Rows, "label", f.label) == nil {
			t.Error("a guest's dump misses what is shared with them")
		}
		if _, err := f.svc.Dump(ctx, uuid.NewString(), f.ws); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("a stranger's dump: %v", err)
		}
	})
}

// newHidden makes an empty workspace to restore into, as an import does.
func newHidden(t *testing.T, st *store.Store) string {
	t.Helper()
	id := uuid.NewString()
	must(t, st.CreateWorkspace(context.Background(), sqlitedb.CreateWorkspaceParams{ID: id, Name: "Imported", Slug: "imported-" + id[:8], Now: store.Now()}))
	return id
}

func TestRestoreWithFreshIDs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := fill(t, d)
		owner, member := f.users["owner"], f.users["member"]
		dump, err := f.svc.Dump(ctx, owner, f.ws)
		must(t, err)
		into := newHidden(t, f.st)
		rep, err := f.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{
			FreshIDs: true, Owner: owner, Exporter: owner,
		})
		must(t, err)
		// The member's assignment goes: they are not in the new workspace.
		// Their comment becomes the owner's.
		if rep.Dropped["task_assignee"] != 1 || rep.Restored["task"] != 5 || rep.Activity == 0 || rep.Credited != 1 {
			t.Errorf("report %+v", rep)
		}
		must(t, f.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
		snap, err := f.svc.Bootstrap(ctx, owner, into)
		must(t, err)
		rows := snap.Rows
		byTitle := map[string]syncer.Row{}
		var label, field, project, status string
		for _, r := range rows {
			if find(dump.Rows, r.Entity, r.ID) != nil {
				t.Fatalf("%s %s kept its ID", r.Entity, r.ID)
			}
			switch r.Entity {
			case "task":
				if r.Fields["parent_id"] == nil {
					byTitle[r.Fields["title"].(string)+map[bool]string{true: " (next)"}[r.Fields["created_by"] != nil && r.Fields["recurrence"] != nil]] = r
				} else {
					byTitle["sub"] = r
				}
			case "label":
				label = r.ID
			case "custom_field":
				field = r.ID
			case "project":
				if r.Fields["name"] == "Shared" {
					project = r.ID
				}
			case "status":
				status = r.ID
			}
		}
		task, sub := byTitle["Write the report"], byTitle["sub"]
		first, next := byTitle["Water plants"], byTitle["Water plants (next)"]
		if task.Fields["project_id"] != project || task.Fields["status_id"] != status || sub.Fields["parent_id"] != task.ID {
			t.Errorf("references not followed: %+v / %+v", task.Fields, sub.Fields)
		}
		if task.Fields["notes"] != "First line\nsecond: line" || task.Clocks["title"] == "" {
			t.Errorf("fields or clocks lost: %+v", task)
		}
		// Rows made from others are made the app's way from the new IDs.
		if next.ID != recur.NextTaskID(first.ID) || next.Fields["series_id"] != first.ID || first.Fields["series_id"] != first.ID {
			t.Errorf("series: first %s next %s (%v)", first.ID, next.ID, next.Fields["series_id"])
		}
		for _, want := range []struct{ entity, id string }{
			{"task_label", syncer.PairID(task.ID, label)},
			{"custom_field_value", syncer.PairID(task.ID, field)},
			{"my_day_entry", syncer.PairID(task.ID, owner)},
			{"task_assignee", syncer.PairID(sub.ID, owner)},
		} {
			if find(rows, want.entity, want.id) == nil {
				t.Errorf("no %s under its pair's ID", want.entity)
			}
		}
		var items []string
		for _, r := range rows {
			if r.Entity == "checklist_item" {
				items = append(items, r.ID)
			}
		}
		if len(items) != 2 || (items[0] != recur.HashID(next.ID+"/"+items[1]) && items[1] != recur.HashID(next.ID+"/"+items[0])) {
			t.Errorf("the copied checklist item lost its link: %v", items)
		}
		// The view's filter names the new project and label.
		for _, r := range rows {
			if r.Entity != "saved_view" {
				continue
			}
			raw, _ := json.Marshal(r.Fields["config"])
			var cfg struct {
				Filter struct {
					Projects, Labels []string
					Fields           []struct{ Field string }
				}
			}
			must(t, json.Unmarshal(raw, &cfg))
			if len(cfg.Filter.Projects) != 1 || cfg.Filter.Projects[0] != project || cfg.Filter.Labels[0] != label ||
				len(cfg.Filter.Fields) != 1 || cfg.Filter.Fields[0].Field != field {
				t.Errorf("view filter %s", raw)
			}
			// The display's references are bare IDs in value positions,
			// which the restore remaps like the filter's.
			raw, _ = json.Marshal(r.Fields["display"])
			var display struct {
				Sort           []struct{ Field string }
				Group, Columns struct{ Field string }
				Fields         struct{ List struct{ Show []string } }
			}
			must(t, json.Unmarshal(raw, &display))
			if len(display.Sort) != 1 || display.Sort[0].Field != field || display.Group.Field != field ||
				display.Columns.Field != field || len(display.Fields.List.Show) != 1 || display.Fields.List.Show[0] != field {
				t.Errorf("view display %s", raw)
			}
		}
		// Nothing proves the member wrote the comment, so it is the owner's,
		// and so is its history.
		for _, r := range rows {
			if r.Entity == "comment" && r.Fields["author_id"] != owner {
				t.Errorf("comment credited to %v", r.Fields["author_id"])
			}
		}
		history, err := f.svc.Dump(ctx, owner, into)
		must(t, err)
		for _, h := range history.Activity {
			if h.ActorID == member || h.Fields["author_id"] == member {
				t.Errorf("history credited to the member: %+v", h)
			}
		}
		// The trashed task is still in the trash.
		trash, err := f.svc.Trash(ctx, owner, into)
		must(t, err)
		if len(trash) != 1 || trash[0].Fields["title"] != "Old idea" {
			t.Errorf("trash after restore: %v", trash)
		}
	})
}

// Someone else importing the export becomes its exporter: the exporter's
// private rows and assignments are theirs, under IDs made with their ID,
// and work by others stays theirs only where Keep says so.
// A comment's references follow the restore as a note's links do, and its
// mentions stay as written.
func TestRestoreFollowsCommentLinks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner, member := w.users["owner"], w.users["member"]
		project, task, page, comment := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		main := syncer.MainPageID(project)
		body := func(ws, project, task, page, person string) string {
			return fmt.Sprintf("See [Beds](/projects/%s?ws=%s&task=%s), [Plan](/projects/%s?tab=wiki&page=%s#soil) and @[Ada](user:%s), @[Gone](user:%s)",
				project, ws, task, project, page, person, "00000000-0000-4000-8000-000000000001")
		}
		for _, r := range w.push("owner", op("project", project, later(0), newProject("Garden")),
			op("task", task, later(1), newTask(project, nil, "Beds")),
			op("page", main, later(2), newPage(project, nil, "Garden")),
			op("page", page, later(3), newPage(project, main, "Plan")),
			op("comment", comment, later(4), map[string]any{"task_id": task, "author_id": owner, "body": body(w.ws, project, task, page, owner)})) {
			wantStatus(t, r, "applied")
		}
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
		ids := map[string]string{}
		got := ""
		for _, r := range snap.Rows {
			switch r.Entity {
			case "comment":
				got = r.Fields["body"].(string)
			case "project", "task":
				ids[r.Entity] = r.ID
			case "page":
				if r.Fields["title"] == "Plan" {
					ids["page"] = r.ID
				}
			}
		}
		if want := body(into, ids["project"], ids["task"], ids["page"], owner); got != want {
			t.Errorf("comment body\n got %s\nwant %s", got, want)
		}
	})
}

func TestRestoreForSomeoneElse(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := fill(t, d)
		owner, member := f.users["owner"], f.users["member"]
		dump, err := f.svc.Dump(ctx, owner, f.ws)
		must(t, err)
		must(t, f.st.PurgeWorkspace(ctx, f.ws))
		newcomer := uuid.NewString()
		must(t, f.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: newcomer, Email: "new@example.com", Name: "New", Now: store.Now()}))
		into := newHidden(t, f.st)
		rep, err := f.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{
			FreshIDs: false, Owner: newcomer, Exporter: owner,
			Keep: func(ctx context.Context, id string) (bool, error) { return id == member, nil },
		})
		must(t, err)
		if rep.Credited != 0 {
			t.Errorf("credited %d rows, want none", rep.Credited)
		}
		var mine int
		rows, err := f.svc.Restored(ctx, into)
		must(t, err)
		for _, r := range rows {
			switch r.Entity {
			case "my_day_entry", "reminder":
				if r.Fields["user_id"] != newcomer {
					t.Errorf("%s belongs to %v", r.Entity, r.Fields["user_id"])
				}
				mine++
			case "comment":
				if r.Fields["author_id"] != member {
					t.Errorf("comment by %v", r.Fields["author_id"])
				}
			case "task_assignee":
				if r.Fields["user_id"] != newcomer || r.ID != syncer.PairID(f.sub, newcomer) {
					t.Errorf("assignment %s to %v", r.ID, r.Fields["user_id"])
				}
			}
			if r.Entity == "task" && r.ID != f.task && r.ID != f.sub && r.ID != f.trashed && r.ID != f.first && r.ID != f.next {
				t.Errorf("task %s did not keep its ID", r.ID)
			}
		}
		if mine != 2 {
			t.Errorf("%d private rows came in, want 2", mine)
		}
	})
}

func TestRestoreKeepingIDsRefusesTakenOnes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := fill(t, d)
		owner := f.users["owner"]
		dump, err := f.svc.Dump(ctx, owner, f.ws)
		must(t, err)
		into := newHidden(t, f.st)
		opts := syncer.RestoreOptions{Owner: owner, Exporter: owner}
		if _, err := f.svc.Restore(ctx, into, dump.Rows, dump.Activity, opts); apperr.KindOf(err) != apperr.Conflict {
			t.Fatalf("keeping IDs that are taken: %v", err)
		}
		if rows, _ := f.svc.Restored(ctx, into); len(rows) != 0 {
			t.Fatalf("a refused restore wrote %d rows", len(rows))
		}
		// Once the workspace is gone the same export comes back as it was.
		must(t, f.st.PurgeWorkspace(ctx, f.ws))
		rep, err := f.svc.Restore(ctx, into, dump.Rows, dump.Activity, opts)
		must(t, err)
		if rep.Restored["task"] != 5 {
			t.Errorf("report %+v", rep)
		}
		rows, err := f.svc.Restored(ctx, into)
		must(t, err)
		if find(rows, "task", f.task) == nil || find(rows, "task_label", syncer.PairID(f.task, f.label)) == nil {
			t.Error("rows did not keep their IDs")
		}
	})
}

func TestRestoreLeavesOutWhatDoesNotFit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		owner := w.users["owner"]
		h := hlcAt(time.Now().Add(-time.Minute), "x")
		p, s, a, b, label := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		row := func(entity, id string, fields map[string]any) syncer.Row {
			clocks := map[string]string{}
			for k := range fields {
				clocks[k] = h
			}
			return syncer.Row{Entity: entity, ID: id, Fields: fields, Clocks: clocks}
		}
		ta, tb := newTask(p, uuid.NewString(), "A"), newTask(p, s, "B")
		ta["parent_id"], tb["parent_id"] = b, a // a loop
		rows := []syncer.Row{
			row("project", p, newProject("P")),
			row("status", s, newStatus(p, "Doing", "in-progress")),
			row("status", uuid.NewString(), newStatus(uuid.NewString(), "Lost", "todo")), // no such project
			row("task", a, ta), // no such status
			row("task", b, tb),
			row("task", uuid.NewString(), map[string]any{"title": "No sort key"}),
			row("task", "bad id!", newTask(nil, nil, "Bad")),
			row("task", uuid.NewString(), map[string]any{"title": "Wrong", "sort_key": "a0", "priority": "extreme"}),
			row("task_label", syncer.PairID(a, label), map[string]any{"task_id": a, "label_id": label}), // no such label
			row("ghost", uuid.NewString(), map[string]any{}),
		}
		into := newHidden(t, w.st)
		rep, err := w.svc.Restore(ctx, into, rows, nil, syncer.RestoreOptions{FreshIDs: true, Owner: owner, Exporter: owner})
		must(t, err)
		if rep.Restored["task"] != 2 || rep.Dropped["task"] != 3 || rep.Dropped["status"] != 1 || rep.Dropped["task_label"] != 1 || rep.Dropped["unknown"] != 1 {
			t.Fatalf("report %+v", rep)
		}
		got, err := w.svc.Restored(ctx, into)
		must(t, err)
		parents := 0
		for _, r := range got {
			if r.Entity != "task" {
				continue
			}
			if r.Fields["parent_id"] != nil {
				parents++
			}
			if r.Fields["title"] == "A" && r.Fields["status_id"] != nil {
				t.Error("a missing status was kept")
			}
		}
		if parents > 1 {
			t.Error("a loop of subtasks came in")
		}
	})
}

func TestForgetEndsAssignmentsAndPrivateRows(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := fill(t, d)
		member := f.users["member"]
		before, err := f.svc.Pull(ctx, f.users["owner"], f.ws, 0)
		must(t, err)
		must(t, f.svc.Forget(ctx, f.ws, member))
		changes, err := f.svc.Pull(ctx, f.users["owner"], f.ws, before.Seq)
		must(t, err)
		if len(changes.Changes) != 1 || changes.Changes[0].Entity != "task_assignee" || changes.Changes[0].Fields["deleted_at"] == nil {
			t.Fatalf("the owner's devices hear %+v", changes.Changes)
		}
		var n int
		must(t, d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM my_day_entries WHERE user_id = ?"), member).Scan(&n))
		if n != 0 {
			t.Error("their My Day stayed")
		}
		must(t, d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM change_log WHERE project_id = ?"), "@"+member).Scan(&n))
		if n != 0 {
			t.Error("the log kept their private rows")
		}
		must(t, d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM comments WHERE author_id = ?"), member).Scan(&n))
		if n != 1 {
			t.Error("their comment went")
		}
	})
}

func equalCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
