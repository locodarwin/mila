package ydoc

import (
	"errors"
	"fmt"
	"sort"

	"github.com/reearth/ygo/crdt"
)

// Fragment is the name y-tiptap keeps a document under.
const Fragment = "default"

// ErrUpdate is a Yjs update that cannot be read.
var ErrUpdate = errors.New("not a Yjs update")

// load builds a Yjs document from updates, in any order.
func load(client uint64, gc bool, updates ...[]byte) (*crdt.Doc, error) {
	d := crdt.New(crdt.WithClientID(crdt.ClientID(client)), crdt.WithGC(gc))
	for _, u := range updates {
		if err := crdt.ApplyUpdateV1(d, u, nil); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpdate, err)
		}
	}
	return d, nil
}

// Check says whether an update can be read.
func Check(update []byte) error {
	if n, ok := countParts(update); ok && n > maxParts {
		return ErrTooManyParts
	}
	_, err := load(1, false, update)
	return err
}

// Merge combines updates into one that holds them all, deleted content
// included. Merging the same updates in any order gives an equivalent
// update.
func Merge(updates ...[]byte) ([]byte, error) {
	if len(updates) == 1 {
		return updates[0], nil
	}
	out, err := crdt.MergeUpdatesV1(updates...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpdate, err)
	}
	return out, nil
}

// Compact turns updates into one that holds the document as it now is,
// with deleted text kept only as its length: the smallest update a device
// can start from. Devices that already hold the old updates lose nothing
// by applying it.
func Compact(updates ...[]byte) ([]byte, error) {
	d, err := load(1, true, updates...)
	if err != nil {
		return nil, err
	}
	crdt.RunGC(d)
	return crdt.EncodeStateAsUpdateV1(d, nil), nil
}

// Read is the document the updates come to.
func Read(updates ...[]byte) (*Node, error) {
	d, err := load(1, false, updates...)
	if err != nil {
		return nil, err
	}
	return fromFragment(d.GetXmlFragment(Fragment)), nil
}

func fromFragment(f *crdt.YXmlFragment) *Node {
	doc := Doc()
	for _, k := range f.Children() {
		doc.Content = appendNode(doc.Content, k)
	}
	return doc
}

func appendNode(out []*Node, n any) []*Node {
	switch t := n.(type) {
	case *crdt.YXmlText:
		for _, d := range t.ToDelta() {
			s, ok := d.Insert.(string)
			if !ok || s == "" {
				continue
			}
			node := &Node{Type: "text", Text: s}
			for name, v := range d.Attributes {
				m := Mark{Type: name}
				if a, ok := v.(map[string]any); ok && len(a) > 0 {
					m.Attrs = a
				}
				node.Marks = append(node.Marks, m)
			}
			sort.Slice(node.Marks, func(i, j int) bool { return node.Marks[i].Type < node.Marks[j].Type })
			out = append(out, node)
		}
	case *crdt.YXmlElement:
		node := &Node{Type: t.NodeName}
		if a := t.GetAttributeValues(); len(a) > 0 {
			node.Attrs = map[string]any{}
			for k, v := range a {
				node.Attrs[k] = number(v)
			}
		}
		for _, k := range t.Children() {
			node.Content = appendNode(node.Content, k)
		}
		out = append(out, node)
	}
	return out
}

type inserter interface {
	InsertElement(*crdt.Transaction, int, *crdt.YXmlElement)
	InsertText(*crdt.Transaction, int, *crdt.YXmlText)
}

// build inserts nodes into a Yjs parent at an index, as y-tiptap would:
// a run of text nodes is one XML text with marks as formatting, and an
// element carries only the attributes that are not null.
func build(txn *crdt.Transaction, parent inserter, index int, content []*Node) {
	for i := 0; i < len(content); {
		n := content[i]
		if n.IsText() {
			t := crdt.NewYXmlText()
			parent.InsertText(txn, index, t)
			var ds []crdt.Delta
			for ; i < len(content) && content[i].IsText(); i++ {
				tn := content[i]
				attrs := crdt.Attributes{}
				for _, m := range tn.Marks {
					attrs[m.Type] = markAttrs(m)
				}
				ds = append(ds, crdt.Delta{Op: crdt.DeltaOpInsert, Insert: tn.Text, Attributes: attrs})
			}
			t.ApplyDelta(txn, ds)
		} else {
			e := crdt.NewYXmlElement(n.Type)
			parent.InsertElement(txn, index, e)
			norm := Normalize(&Node{Type: n.Type, Attrs: n.Attrs})
			for k, v := range norm.Attrs {
				e.SetAttributeValue(txn, k, v)
			}
			if len(n.Content) > 0 {
				build(txn, e, 0, n.Content)
			}
			i++
		}
		index++
	}
}

// maxDiffCells bounds the table shared fills: a write of many blocks in
// place of many others would otherwise need memory of the two counts
// multiplied, gigabytes for a long page.
const maxDiffCells = 1 << 22

type pair struct{ i, j int }

// shared is the blocks old and next have in common, in order, as pairs of
// their indexes, ending with (len(old), len(next)). The runs both start
// and end with always stay; between them, the longest common run is
// worked out while its table fits in maxDiffCells, and past that the
// middle is replaced whole.
func shared(old, next []string) []pair {
	n, m := len(old), len(next)
	pre := 0
	for pre < n && pre < m && old[pre] == next[pre] {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && old[n-1-suf] == next[m-1-suf] {
		suf++
	}
	pairs := make([]pair, 0, pre+suf+1)
	for k := range pre {
		pairs = append(pairs, pair{k, k})
	}
	a, b := old[pre:n-suf], next[pre:m-suf]
	if len(a) > 0 && len(b) > 0 && (len(a)+1)*(len(b)+1) <= maxDiffCells {
		w := len(b) + 1
		dp := make([]int32, (len(a)+1)*w)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					dp[i*w+j] = dp[(i+1)*w+j+1] + 1
				} else {
					dp[i*w+j] = max(dp[(i+1)*w+j], dp[i*w+j+1])
				}
			}
		}
		for i, j := 0, 0; i < len(a) && j < len(b); {
			switch {
			case a[i] == b[j]:
				pairs = append(pairs, pair{pre + i, pre + j})
				i++
				j++
			case dp[(i+1)*w+j] >= dp[i*w+j+1]:
				i++
			default:
				j++
			}
		}
	}
	for k := range suf {
		pairs = append(pairs, pair{n - suf + k, m - suf + k})
	}
	return append(pairs, pair{n, m})
}

// Replace writes the update that turns the document the updates come to
// into next. Only the top-level blocks that differ are replaced, so an
// edit another device has not yet sent survives unless it was inside a
// block that changed. client must be an ID no device uses: each write
// the server makes takes a fresh one. It returns nil when nothing differs.
func Replace(updates [][]byte, next *Node, client uint64) ([]byte, error) {
	d, err := load(client, false, updates...)
	if err != nil {
		return nil, err
	}
	frag := d.GetXmlFragment(Fragment)
	old := fromFragment(frag).Content
	nw := Normalize(next).Content
	// The longest run of blocks the two share, in order, stays.
	ok := make([]string, len(old))
	for i, b := range old {
		ok[i] = Canonical(b)
	}
	nk := make([]string, len(nw))
	for j, b := range nw {
		nk[j] = Canonical(b)
	}
	pairs := shared(ok, nk)
	n, m := len(old), len(nw)
	if n == m && len(pairs) == n+1 {
		return nil, nil
	}
	var out []byte
	stop := d.OnUpdate(func(u []byte, _ any) { out = u })
	defer stop()
	d.Transact(func(txn *crdt.Transaction) {
		// From the end, so earlier indexes stay where they are.
		for k := len(pairs) - 1; k >= 0; k-- {
			os, ns := 0, 0
			if k > 0 {
				os, ns = pairs[k-1].i+1, pairs[k-1].j+1
			}
			if pairs[k].i > os {
				frag.Delete(txn, os, pairs[k].i-os)
			}
			if pairs[k].j > ns {
				build(txn, frag, os, nw[ns:pairs[k].j])
			}
		}
	})
	return out, nil
}
