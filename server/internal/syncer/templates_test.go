package syncer_test

import (
	"context"
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

func templateRow(kind, name string, data any) map[string]any {
	return map[string]any{"kind": kind, "name": name, "data": data, "sort_key": "a0"}
}

func snapshot(tasks ...map[string]any) map[string]any {
	list := make([]any, len(tasks))
	for i, t := range tasks {
		list[i] = t
	}
	return map[string]any{"v": 1, "tasks": list}
}

func TestTemplatesAreForMembersOnly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		w.share(shared, true)
		id := uuid.NewString()
		wantStatus(t, w.push("member", op("template", id, hlcAt(now, "m"), templateRow("task", "Bug report", snapshot(map[string]any{"id": "t1", "title": "Bug"}))))[0], "applied")
		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.Entity == "template" {
				t.Fatal("a guest's bootstrap holds a template")
			}
		}
		snap, err = w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		must(t, err)
		found := false
		for _, r := range snap.Rows {
			found = found || r.Entity == "template" && r.ID == id
		}
		if !found {
			t.Fatal("the owner's bootstrap lacks the template")
		}
		wantStatus(t, w.push("guest", op("template", uuid.NewString(), hlcAt(now, "g"), templateRow("task", "Mine", snapshot())))[0], "rejected")
		// A guest's REST list of templates is empty, not an error.
		page, err := w.svc.List(ctx, w.users["guest"], w.ws, "template", syncer.Query{Limit: 10})
		must(t, err)
		if len(page.Rows) != 0 {
			t.Fatalf("a guest listed %d templates", len(page.Rows))
		}
	})
}

func TestTemplatesAreChecked(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		bad := []any{
			map[string]any{"v": 2, "tasks": []any{}},
			map[string]any{"v": 1},
			map[string]any{"v": 1, "tasks": []any{map[string]any{"id": "t1"}}},
			map[string]any{"v": 1, "tasks": []any{}, "project": "Kitchen"},
			map[string]any{"v": 1, "tasks": []any{map[string]any{"id": "t1", "title": " Spaces "}}},
			map[string]any{"v": 1, "tasks": []any{map[string]any{"id": "t1", "title": ""}}},
			map[string]any{"v": 1, "tasks": []any{map[string]any{"id": "t1", "title": strings.Repeat("x", 501)}}},
			"a template",
		}
		for _, data := range bad {
			res := w.push("owner", op("template", uuid.NewString(), hlcAt(now, "a"), templateRow("project", "Bad", data)))[0]
			if res.Status != "rejected" {
				t.Errorf("%v was %s", data, res.Status)
			}
		}
		many := make([]map[string]any, 501)
		for i := range many {
			many[i] = map[string]any{"id": uuid.NewString(), "title": "Task"}
		}
		res := w.push("owner", op("template", uuid.NewString(), hlcAt(now, "a"), templateRow("project", "Huge", snapshot(many...))))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "500 at most") {
			t.Errorf("501 tasks: %s %s", res.Status, res.Message)
		}
		// 200 templates at most in a workspace.
		for range 200 {
			wantStatus(t, w.push("owner", op("template", uuid.NewString(), hlcAt(now, "a"), templateRow("task", "T", snapshot())))[0], "applied")
		}
		res = w.push("owner", op("template", uuid.NewString(), hlcAt(now, "a"), templateRow("task", "One more", snapshot())))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "200 templates") {
			t.Errorf("the 201st: %s %s", res.Status, res.Message)
		}
	})
}

func TestAnImportPointsTemplatesAtTheNewRows(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		owner := w.users["owner"]
		project, _ := w.project("House")
		task := uuid.NewString()
		w.push("owner", op("task", task, hlcAt(now, "a"), newTask(project, nil, "Buy paint")))
		// A template saved from the project names its task and links to it.
		tpl := uuid.NewString()
		data := snapshot(map[string]any{"id": task, "title": "Buy paint", "notes": "See [it](/?task=" + task + ")"})
		w.push("owner", op("template", tpl, hlcAt(now, "a"), templateRow("project", "House", data)))
		dump, err := w.svc.Dump(ctx, owner, w.ws)
		must(t, err)
		into := newHidden(t, w.st)
		_, err = w.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: true, Owner: owner, Exporter: owner, From: w.ws})
		must(t, err)
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: into, UserID: owner, Role: "owner", Now: store.Now()}))
		snap, err := w.svc.Bootstrap(ctx, owner, into)
		must(t, err)
		var newTask string
		var restored map[string]any
		for _, r := range snap.Rows {
			switch r.Entity {
			case "task":
				newTask = r.ID
			case "template":
				raw, _ := json.Marshal(r.Fields["data"])
				must(t, json.Unmarshal(raw, &restored))
			}
		}
		if newTask == "" || newTask == task || restored == nil {
			t.Fatalf("restored task %q, template %v", newTask, restored)
		}
		got := restored["tasks"].([]any)[0].(map[string]any)
		if got["id"] != newTask || got["notes"] != "See [it](/?task="+newTask+")" {
			t.Errorf("the template reads %v, want its task %s", got, newTask)
		}
	})
}
