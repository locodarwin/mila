package ydoc

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// The editor's nodes and marks with their attributes' defaults, from
// Tiptap's getSchema (web/src/lib/docs/vectors.ts). TestSchemaMatchesVectors
// keeps this copy equal to shared/doc-vectors.json.
//
//go:embed schema.json
var schemaJSON []byte

type schemaDefaults struct {
	Nodes map[string]map[string]any `json:"nodes"`
	Marks map[string]map[string]any `json:"marks"`
}

var schema = func() schemaDefaults {
	var s schemaDefaults
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		panic("ydoc: schema.json: " + err.Error())
	}
	return s
}()

// Normalize returns a copy of a document as y-tiptap stores it, so two
// that mean the same compare equal: a node's attributes with their
// defaults filled in and nulls left out (y-tiptap writes only those that
// are not null); a mark's attributes with their defaults, nulls kept (it
// writes the mark's attributes whole); marks in order of name; text with
// the same marks in one node; and no empty content or empty text.
func Normalize(n *Node) *Node {
	if n == nil {
		return nil
	}
	out := &Node{Type: n.Type, Text: n.Text}
	if !n.IsText() && n.Type != "doc" {
		attrs := map[string]any{}
		for k, d := range schema.Nodes[n.Type] {
			attrs[k] = d
		}
		for k, v := range n.Attrs {
			attrs[k] = v
		}
		for k, v := range attrs {
			if v == nil {
				delete(attrs, k)
			} else {
				attrs[k] = number(v)
			}
		}
		if len(attrs) > 0 {
			out.Attrs = attrs
		}
	}
	if n.IsText() {
		out.Marks = normalMarks(n.Marks)
		return out
	}
	for _, k := range n.Content {
		c := Normalize(k)
		if c.IsText() {
			if c.Text == "" {
				continue
			}
			if last := len(out.Content) - 1; last >= 0 && out.Content[last].IsText() && sameMarks(out.Content[last].Marks, c.Marks) {
				out.Content[last].Text += c.Text
				continue
			}
		}
		out.Content = append(out.Content, c)
	}
	return out
}

func normalMarks(marks []Mark) []Mark {
	if len(marks) == 0 {
		return nil
	}
	out := make([]Mark, 0, len(marks))
	for _, m := range marks {
		attrs := map[string]any{}
		for k, d := range schema.Marks[m.Type] {
			attrs[k] = d
		}
		for k, v := range m.Attrs {
			attrs[k] = number(v)
		}
		nm := Mark{Type: m.Type}
		if len(attrs) > 0 {
			nm.Attrs = attrs
		}
		out = append(out, nm)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// number makes a whole number one type, whichever way it arrived: JSON
// gives float64, Yjs gives int64.
func number(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
	case int:
		return int64(x)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
	}
	return v
}

// markAttrs is a mark's attributes as y-tiptap writes them: every one, with defaults.
func markAttrs(m Mark) map[string]any {
	attrs := map[string]any{}
	for k, d := range schema.Marks[m.Type] {
		attrs[k] = d
	}
	for k, v := range m.Attrs {
		attrs[k] = number(v)
	}
	return attrs
}
