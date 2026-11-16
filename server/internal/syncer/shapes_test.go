package syncer_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// TestJSONShapes: a kJSON field the app could not read is refused, by
// sync and by REST alike, with a reason naming the part.
func TestJSONShapes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		view := w.mustWrite("owner", syncer.Write{Entity: "saved_view", Mode: syncer.Create,
			Fields: fields(map[string]any{"name": "Mine", "type": "list", "config": map[string]any{"filter": map[string]any{}}})})

		badConfigs := map[string]string{
			`"{}"`:                                  "must be an object",
			`[]`:                                    "must be an object",
			`{"filter":"x"}`:                        "filter must be an object",
			`{"filter":{"labels":"l1"}}`:            "filter.labels must be a list of strings",
			`{"filter":{"statuses":["todo",5]}}`:    "filter.statuses must be a list of strings",
			`{"filter":{"text":5}}`:                 "filter.text must be a string",
			`{"filter":{"due":true}}`:               "filter.due must be a string",
			`{"filter":{"showDone":"no"}}`:          "filter.showDone must be true or false",
			`{"filter":{"fields":{}}}`:              "filter.fields must be a list",
			`{"filter":{"fields":[{"field":"f"}]}}`: "filter.fields[0] must be an object",
		}
		for raw, want := range badConfigs {
			_, _, err := w.write("owner", syncer.Write{Entity: "saved_view", ID: view.ID, Mode: syncer.Update,
				Fields: map[string]json.RawMessage{"config": json.RawMessage(raw)}})
			wantKind(t, err, apperr.Invalid)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("config %s: %v, want %q", raw, err, want)
			}
			res := w.push("owner", syncer.Op{OpID: uuid.NewString(), Entity: "saved_view", ID: view.ID, HLC: later(0),
				Fields: map[string]json.RawMessage{"config": json.RawMessage(raw)}})[0]
			if res.Status != "rejected" || !strings.Contains(res.Message, want) {
				t.Errorf("pushed config %s: %s %s", raw, res.Status, res.Message)
			}
		}
		for _, raw := range []string{`{}`, `{"filter":null}`, `{"filter":{"text":null,"labels":null}}`, `{"filter":{"labels":["l1"],"other":5},"newer":1}`,
			`{"filter":{"text":"x","projects":["inbox"],"statuses":[],"priorities":["high"],"assignees":["me"],"due":"week","showDone":true,"fields":[{"field":"f","op":"empty"}]}}`} {
			w.mustWrite("owner", syncer.Write{Entity: "saved_view", ID: view.ID, Mode: syncer.Update,
				Fields: map[string]json.RawMessage{"config": json.RawMessage(raw)}})
		}
		for _, raw := range []string{`"x"`, `[]`, `5`, `{"report":{"from":"1000-01-10"}}`, `{"report":{"to":"2026-13-40"}}`, `{"report":{"from":5}}`} {
			_, _, err := w.write("owner", syncer.Write{Entity: "saved_view", ID: view.ID, Mode: syncer.Update,
				Fields: map[string]json.RawMessage{"display": json.RawMessage(raw)}})
			wantKind(t, err, apperr.Invalid)
		}
		for _, raw := range []string{`null`, `{}`, `{"sort":[{"by":"title","dir":"asc"}]}`, `{"report":{"from":"2026-01-05","to":null,"tab":"flow"}}`} {
			w.mustWrite("owner", syncer.Write{Entity: "saved_view", ID: view.ID, Mode: syncer.Update,
				Fields: map[string]json.RawMessage{"display": json.RawMessage(raw)}})
		}

		field := w.mustWrite("owner", syncer.Write{Entity: "custom_field", Mode: syncer.Create,
			Fields: fields(map[string]any{"name": "Size", "type": "select", "options": []map[string]string{{"id": "o1", "name": "Large", "color": "red"}}})})
		for _, raw := range []string{`{}`, `"o1"`, `[5]`, `[{"name":"Large","color":"red"}]`, `[{"id":"o1","name":"A","color":"red"},{"id":"o1","name":"B","color":"blue"}]`,
			`[{"id":"o1","name":5,"color":"red"}]`, `[{"id":"o1","name":"A","color":"mauve"}]`, `[{"id":"o1","name":"A"}]`} {
			_, _, err := w.write("owner", syncer.Write{Entity: "custom_field", ID: field.ID, Mode: syncer.Update,
				Fields: map[string]json.RawMessage{"options": json.RawMessage(raw)}})
			wantKind(t, err, apperr.Invalid)
		}
		w.mustWrite("owner", syncer.Write{Entity: "custom_field", ID: field.ID, Mode: syncer.Update,
			Fields: map[string]json.RawMessage{"options": json.RawMessage(`[{"id":"o1","name":"Large","color":"red"},{"id":"o2","name":"","color":"blue","newer":true}]`)}})
	})
}

// TestFieldValueShapes: a task's value for a field has the JSON type the
// field's type gives it.
func TestFieldValueShapes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create, Fields: fields(map[string]any{"title": "Paint"})})
		cases := map[string]struct{ good, bad []any }{
			"text":         {[]any{"words", ""}, []any{5, true, []string{"a"}}},
			"url":          {[]any{"https://example.com"}, []any{5}},
			"number":       {[]any{0, -2.5, 1e300}, []any{"5", true}},
			"date":         {[]any{"2026-10-28"}, []any{"2026-02-30", "tomorrow", 3}},
			"select":       {[]any{"o1"}, []any{[]string{"o1"}, 1}},
			"multi_select": {[]any{[]string{"o1", "o2"}, []string{}}, []any{"o1", []any{"o1", 2}}},
			"checkbox":     {[]any{true, false}, []any{"true", 1}},
			"user":         {[]any{w.users["member"]}, []any{7}},
		}
		for kind, c := range cases {
			opts := []any{}
			if kind == "select" || kind == "multi_select" {
				opts = []any{map[string]string{"id": "o1", "name": "One", "color": "red"}, map[string]string{"id": "o2", "name": "Two", "color": "blue"}}
			}
			field := w.mustWrite("owner", syncer.Write{Entity: "custom_field", Mode: syncer.Create,
				Fields: fields(map[string]any{"name": kind, "type": kind, "options": opts})})
			id := syncer.PairID(task.ID, field.ID)
			for _, v := range c.good {
				w.mustWrite("owner", syncer.Write{Entity: "custom_field_value", Mode: syncer.Upsert,
					Fields: fields(map[string]any{"task_id": task.ID, "field_id": field.ID, "value": v})})
			}
			for _, v := range c.bad {
				_, _, err := w.write("owner", syncer.Write{Entity: "custom_field_value", Mode: syncer.Upsert,
					Fields: fields(map[string]any{"task_id": task.ID, "field_id": field.ID, "value": v})})
				wantKind(t, err, apperr.Invalid)
				res := w.push("member", op("custom_field_value", id, later(5000), map[string]any{"value": v}))[0]
				if res.Status != "rejected" {
					t.Errorf("%s field: a pushed %v was %s", kind, v, res.Status)
				}
			}
		}
	})
}

// TestJSONLimitIsCompact: the byte limit counts the JSON as stored, so a
// pretty-printed body that fits once compacted is taken.
func TestJSONLimitIsCompact(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		labels := make([]string, 450)
		for i := range labels {
			labels[i] = uuid.NewString()
		}
		pretty, err := json.MarshalIndent(map[string]any{"filter": map[string]any{"labels": labels}}, "", "        ")
		must(t, err)
		compact, err := json.Marshal(map[string]any{"filter": map[string]any{"labels": labels}})
		must(t, err)
		if len(pretty) <= 20_000 || len(compact) > 20_000 {
			t.Fatalf("pretty %d bytes, compact %d: the case does not straddle the limit", len(pretty), len(compact))
		}
		w.mustWrite("owner", syncer.Write{Entity: "saved_view", Mode: syncer.Create,
			Fields: map[string]json.RawMessage{"name": json.RawMessage(`"Long"`), "type": json.RawMessage(`"list"`), "config": pretty}})
		too := make([]string, 600)
		for i := range too {
			too[i] = uuid.NewString()
		}
		big, err := json.Marshal(map[string]any{"filter": map[string]any{"labels": too}})
		must(t, err)
		_, _, err = w.write("owner", syncer.Write{Entity: "saved_view", Mode: syncer.Create,
			Fields: map[string]json.RawMessage{"name": json.RawMessage(`"Longer"`), "type": json.RawMessage(`"list"`), "config": big}})
		wantKind(t, err, apperr.Invalid)
	})
}

// TestMovingAParentLeavesSubtasksEditable: a parent moved to another
// project by REST, its subtasks can still be edited where they are, and
// then moved after it, one by one.
func TestMovingAParentLeavesSubtasksEditable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		home, homeTodo := w.project("Home")
		work, workTodo := w.project("Work")
		parent := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: fields(map[string]any{"title": "Move house", "project_id": home, "status_id": homeTodo})})
		sub := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: fields(map[string]any{"title": "Pack", "project_id": home, "status_id": homeTodo, "parent_id": parent.ID})})
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: parent.ID, Mode: syncer.Update,
			Fields: fields(map[string]any{"project_id": work, "status_id": workTodo})})
		// The app edits the subtask where it is.
		wantStatus(t, w.push("member", op("task", sub.ID, later(10), map[string]any{"title": "Pack the books", "priority": "high"}))[0], "applied")
		// Moving it elsewhere than its parent is still refused...
		third, thirdTodo := w.project("Third")
		_, _, err := w.write("owner", syncer.Write{Entity: "task", ID: sub.ID, Mode: syncer.Update,
			Fields: fields(map[string]any{"project_id": third, "status_id": thirdTodo})})
		wantKind(t, err, apperr.Invalid)
		// ...and moving it after its parent is taken.
		w.mustWrite("owner", syncer.Write{Entity: "task", ID: sub.ID, Mode: syncer.Update,
			Fields: fields(map[string]any{"project_id": work, "status_id": workTodo})})
		// A new parent in another project is refused as before.
		other := w.mustWrite("owner", syncer.Write{Entity: "task", Mode: syncer.Create,
			Fields: fields(map[string]any{"title": "Elsewhere", "project_id": home, "status_id": homeTodo})})
		_, _, err = w.write("owner", syncer.Write{Entity: "task", ID: sub.ID, Mode: syncer.Update,
			Fields: fields(map[string]any{"parent_id": other.ID})})
		wantKind(t, err, apperr.Invalid)
	})
}
