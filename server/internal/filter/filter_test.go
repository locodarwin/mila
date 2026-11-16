package filter

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"testing"
)

// vectorTask is a task as the shared vectors write it.
type vectorTask struct {
	ID        *string `json:"id"`
	ProjectID *string `json:"projectId"`
	StatusID  *string `json:"statusId"`
	Title     *string `json:"title"`
	Notes     *string `json:"notes"`
	Priority  *string `json:"priority"`
	DueDate   *string `json:"dueDate"`
	Estimate  *int64  `json:"estimateMinutes"`
}

type vectorContext struct {
	Me         *string                    `json:"me"`
	Today      *string                    `json:"today"`
	Category   *string                    `json:"category"`
	Labels     []string                   `json:"labels"`
	Assignees  []string                   `json:"assignees"`
	FieldTypes map[string]string          `json:"fieldTypes"`
	Values     map[string]json.RawMessage `json:"values"`
	Blocked    bool                       `json:"blocked"`
	Blocking   bool                       `json:"blocking"`
	InMyDay    bool                       `json:"inMyDay"`
	Entries    []struct {
		User    string `json:"user"`
		Day     string `json:"day"`
		Minutes int64  `json:"minutes"`
	} `json:"entries"`
}

// stored decodes a field value as the feeds do, numbers as json.Number.
func stored(raw json.RawMessage) any {
	if raw == nil {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		panic(err)
	}
	return v
}

// over replaces the fields of a task or context a case names; a present
// null clears it.
func over(dst, src map[string]json.RawMessage) {
	maps.Copy(dst, src)
}

func TestSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/filter-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Defaults struct {
			Task    map[string]json.RawMessage `json:"task"`
			Context map[string]json.RawMessage `json:"context"`
		} `json:"defaults"`
		Matches []struct {
			Name    string                     `json:"name"`
			Task    map[string]json.RawMessage `json:"task"`
			Context map[string]json.RawMessage `json:"context"`
			Filter  json.RawMessage            `json:"filter"`
			Config  json.RawMessage            `json:"config"`
			Want    bool                       `json:"want"`
		} `json:"matches"`
		Count []struct {
			Filter json.RawMessage `json:"filter"`
			Want   int             `json:"want"`
		} `json:"count"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Matches) < 100 || len(v.Count) < 10 {
		t.Fatalf("too few vectors: %d matches, %d counts", len(v.Matches), len(v.Count))
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for _, c := range v.Matches {
		taskFields := map[string]json.RawMessage{}
		over(taskFields, v.Defaults.Task)
		over(taskFields, c.Task)
		ctxFields := map[string]json.RawMessage{}
		over(ctxFields, v.Defaults.Context)
		over(ctxFields, c.Context)
		var vt vectorTask
		var vc vectorContext
		if err := remarshal(taskFields, &vt); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if err := remarshal(ctxFields, &vc); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		task := Task{ID: str(vt.ID), ProjectID: str(vt.ProjectID), StatusID: str(vt.StatusID), Title: str(vt.Title), Notes: str(vt.Notes),
			Priority: str(vt.Priority), DueDate: str(vt.DueDate), Estimate: vt.Estimate}
		var entries []Entry
		for _, e := range vc.Entries {
			entries = append(entries, Entry{User: e.User, Day: e.Day, Minutes: e.Minutes})
		}
		ctx := Context{
			Me: str(vc.Me), Today: str(vc.Today),
			Category:  func(Task) string { return str(vc.Category) },
			Labels:    func(string) []string { return vc.Labels },
			Assignees: func(string) []string { return vc.Assignees },
			FieldType: func(id string) string { return vc.FieldTypes[id] },
			Value:     func(_, id string) any { return stored(vc.Values[id]) },
			Blocked:   func(string) bool { return vc.Blocked },
			Blocking:  func(string) bool { return vc.Blocking },
			Entries:   func(string) []Entry { return entries },
			InMyDay:   func(string) bool { return vc.InMyDay },
		}
		var f Filter
		var err error
		if c.Config != nil {
			f, err = FromConfig(c.Config)
		} else {
			err = json.Unmarshal(c.Filter, &f)
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if got := Matches(task, f, ctx); got != c.Want {
			t.Errorf("%s: got %v, want %v", c.Name, got, c.Want)
		}
	}
	for _, c := range v.Count {
		var f Filter
		if err := json.Unmarshal(c.Filter, &f); err != nil {
			t.Fatal(err)
		}
		if got := Count(f); got != c.Want {
			t.Errorf("Count(%s) = %d, want %d", c.Filter, got, c.Want)
		}
	}
}

func remarshal(from map[string]json.RawMessage, to any) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, to)
}

func TestFromConfig(t *testing.T) {
	f, err := FromConfig([]byte(`{"filter":{"labels":["l1"],"due":"week","showDone":true}}`))
	if err != nil || len(f.Labels) != 1 || f.Due != "week" || !f.ShowDone {
		t.Fatalf("got %+v, %v", f, err)
	}
	for _, empty := range []string{`{}`, `{"filter":null}`, `{"other":1}`} {
		if f, err := FromConfig([]byte(empty)); err != nil || Count(f) != 0 {
			t.Errorf("%s: got %+v, %v", empty, f, err)
		}
	}
	// A part of the wrong type is set, and matches nothing; so does a
	// filter or a config that is not an object.
	open := Context{Today: "2026-10-28", Category: func(Task) string { return "todo" },
		Labels: func(string) []string { return nil }, Assignees: func(string) []string { return nil }}
	for _, odd := range []string{`{"filter":{"labels":"l1"}}`, `{"filter":{"text":5}}`, `{"filter":{"showDone":"no"}}`,
		`{"filter":{"statuses":["todo",5]}}`, `{"filter":{"due":true}}`, `{"filter":"x"}`, `{"filter":[]}`, `"{}"`, `[]`, `5`} {
		f, err := FromConfig([]byte(odd))
		if err != nil || Count(f) != 1 || Matches(Task{ID: "t1"}, f, open) {
			t.Errorf("%s: got %+v, %v", odd, f, err)
		}
	}
	if _, err := FromConfig([]byte(`{"filter":`)); err == nil {
		t.Error("broken JSON was read")
	}
	if f, err := FromConfig([]byte(`null`)); err != nil || !Matches(Task{ID: "t1"}, f, open) {
		t.Errorf("a null config: got %+v, %v", f, err)
	}
	// Keys are read as written, as the app reads them: "Filter" and
	// "Labels" are not "filter" and "labels".
	for _, other := range []string{`{"Filter":{"labels":["l1"]}}`, `{"filter":{"Labels":["l1"],"FIELDS":[{"field":"f","op":"empty"}]}}`} {
		if f, err := FromConfig([]byte(other)); err != nil || Count(f) != 0 {
			t.Errorf("%s: got %+v, %v", other, f, err)
		}
	}
	// One malformed condition does not fail the filter; it matches nothing.
	f, err = FromConfig([]byte(`{"filter":{"due":"week","fields":[{"field":"f1","op":"empty"},7,{"field":3}]}}`))
	if err != nil || len(f.Fields) != 3 || f.Due != "week" || Count(f) != 4 {
		t.Fatalf("got %+v, %v", f, err)
	}
	// A fields that is not a list is malformed as a whole; null is unset.
	for odd, want := range map[string]int{`{"fields":{"field":"f1"}}`: 1, `{"fields":"x"}`: 1, `{"fields":null}`: 0} {
		f, err := FromConfig([]byte(`{"filter":` + odd + `}`))
		if err != nil || f.Fields != nil || Count(f) != want {
			t.Errorf("%s: got %+v, %v", odd, f, err)
		}
	}
}

// A number too large for a float64 is Infinity in JavaScript, which is no
// finite number there, and a range error here; either way it never
// compares. The shared vectors cannot hold it: Go cannot read it into the
// harness.
func TestNonFiniteNumber(t *testing.T) {
	f, err := FromConfig([]byte(`{"filter":{"fields":[{"field":"n","op":"gt","value":1}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{
		Today:     "2026-10-28",
		Category:  func(Task) string { return "todo" },
		Labels:    func(string) []string { return nil },
		Assignees: func(string) []string { return nil },
		FieldType: func(string) string { return "number" },
		Value:     func(string, string) any { return json.Number("1e400") },
	}
	if Matches(Task{ID: "t1"}, f, ctx) {
		t.Error("an infinite number compared")
	}
	f.Fields = []Condition{ReadCondition(json.RawMessage(`{"field":"n","op":"not_empty"}`))}
	if !Matches(Task{ID: "t1"}, f, ctx) {
		t.Error("an infinite number is no value")
	}
	f.Fields = []Condition{ReadCondition(json.RawMessage(`{"field":"n","op":"lt","value":1e400}`))}
	ctx.Value = func(string, string) any { return json.Number("5") }
	if Matches(Task{ID: "t1"}, f, ctx) {
		t.Error("an infinite bound compared")
	}
}
