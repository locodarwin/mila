// Package ydoc reads and writes live documents: a task's notes and a wiki
// page's body, held as Yjs updates in the shape y-tiptap gives them (the
// XML fragment "default" of ProseMirror nodes). It merges and compacts
// updates, reads them as a document, writes the Yjs update that turns one
// document into another, and reads and writes Markdown.
// shared/doc-vectors.json holds all of it to the editor's own answers.
package ydoc

import (
	"encoding/json"
	"maps"
	"slices"
)

// Node is a ProseMirror node as JSON: a block, or text with its marks.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*Node        `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is a ProseMirror mark: bold, a link and so on.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Doc is an empty document.
func Doc(content ...*Node) *Node { return &Node{Type: "doc", Content: content} }

// IsText says whether a node is text.
func (n *Node) IsText() bool { return n.Type == "text" }

// Attr is one of a node's attributes, or nil.
func (n *Node) Attr(name string) any {
	if n.Attrs == nil {
		return nil
	}
	return n.Attrs[name]
}

// Clone copies a node deeply.
func (n *Node) Clone() *Node {
	c := &Node{Type: n.Type, Text: n.Text, Attrs: maps.Clone(n.Attrs)}
	for _, k := range n.Content {
		c.Content = append(c.Content, k.Clone())
	}
	for _, m := range n.Marks {
		c.Marks = append(c.Marks, Mark{Type: m.Type, Attrs: maps.Clone(m.Attrs)})
	}
	return c
}

// ParseJSON reads a document from ProseMirror JSON.
func ParseJSON(b []byte) (*Node, error) {
	var n Node
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// Canonical is a node as stable JSON: normalized as y-tiptap stores it,
// with object keys in order, so two documents compare by their bytes.
func Canonical(n *Node) string {
	b, err := json.Marshal(Normalize(n))
	if err != nil {
		return ""
	}
	return string(b)
}

// Equal says whether two documents are the same once normalized.
func Equal(a, b *Node) bool { return Canonical(a) == Canonical(b) }

// sameMarks says whether two text nodes carry the same marks.
func sameMarks(a, b []Mark) bool {
	return slices.EqualFunc(a, b, func(x, y Mark) bool {
		if x.Type != y.Type {
			return false
		}
		xb, _ := json.Marshal(x.Attrs)
		yb, _ := json.Marshal(y.Attrs)
		return string(xb) == string(yb)
	})
}
