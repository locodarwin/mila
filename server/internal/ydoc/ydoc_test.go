package ydoc_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/ydoc"
)

var update = flag.Bool("update", false, "write the server's Markdown into shared/doc-vectors.json")

// vectors is shared/doc-vectors.json, kept in its own order so -update
// changes only what it fills in.
type vectors struct {
	Comment string          `json:"_comment"`
	Schema  json.RawMessage `json:"schema"`
	Parse   []struct {
		Name string          `json:"name"`
		Md   string          `json:"md"`
		JSON json.RawMessage `json:"json"`
	} `json:"parse"`
	Render []renderCase `json:"render"`
	Yjs    []struct {
		Name    string          `json:"name"`
		Updates []string        `json:"updates"`
		JSON    json.RawMessage `json:"json"`
	} `json:"yjs"`
}

type renderCase struct {
	Name string          `json:"name"`
	JSON json.RawMessage `json:"json"`
	Md   *string         `json:"md,omitempty"`
}

func vectorsPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../../shared/doc-vectors.json")
}

func load(t *testing.T) *vectors {
	t.Helper()
	b, err := os.ReadFile(vectorsPath())
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return &v
}

func node(t *testing.T, raw json.RawMessage) *ydoc.Node {
	t.Helper()
	n, err := ydoc.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Where the server reads Markdown differently from the editor, on purpose:
// it reads every entity and every inline HTML mark, where the editor reads
// only some, and an empty list item as one.
var readsDifferently = map[string]string{
	"inline html marks":                       "the editor reads only the first tag",
	"written by the editor: bold into italic": "the editor leaves <em>c</em> as text",
	"written by the editor: empty list item":  `the editor reads "- " as text`,
}

func TestSchemaMatchesVectors(t *testing.T) {
	v := load(t)
	src, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	_ = json.Unmarshal(v.Schema, &a)
	_ = json.Unmarshal(src, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if !bytes.Equal(ja, jb) {
		t.Errorf("schema.json differs from shared/doc-vectors.json's schema; copy it over")
	}
}

func TestParseVectors(t *testing.T) {
	for _, c := range load(t).Parse {
		got := ydoc.Parse(c.Md)
		want := node(t, c.JSON)
		same := ydoc.Equal(got, want)
		if why, differs := readsDifferently[c.Name]; differs {
			if same {
				t.Errorf("%s: now read as the editor reads it (%s); take it off readsDifferently", c.Name, why)
			}
			continue
		}
		if !same {
			t.Errorf("%s: %q\n got  %s\n want %s", c.Name, c.Md, ydoc.Canonical(got), ydoc.Canonical(want))
		}
	}
}

// Documents Markdown cannot hold as they are: the server's own reading of
// what it wrote comes back different.
var lossy = map[string]string{
	"break in heading": "a heading is one line, so its break is a space",
	"spaces in bold":   "Markdown has no spaces just inside bold; they move outside",
	"read from written by the editor: empty list item":      "an empty list item cannot hold the editor's text",
	"empty paragraph last":                                  "the editor's empty last paragraph is left out, as its own Markdown does",
	"read from written by the editor: empty paragraph last": "the editor's empty last paragraph is left out, as its own Markdown does",
	"only an empty paragraph":                               "an empty document is written as nothing, which reads as no paragraph at all",
	"table with pipes":                                      "a table cell is one line, so its break is a space",
	"table cell with two paragraphs":                        "a table cell is one line, so its paragraphs run together",
	"table without a header row":                            "a Markdown table's first row is always its header",
	"image between bold text":                               "Markdown has no spaces just inside bold; they move outside",
	"read from image in bold":                               "Markdown has no spaces just inside bold; they move outside",
}

func TestRenderVectors(t *testing.T) {
	v := load(t)
	changed := false
	for i := range v.Render {
		c := &v.Render[i]
		doc := node(t, c.JSON)
		md := ydoc.Markdown(doc)
		if *update {
			if c.Md == nil || *c.Md != md {
				c.Md, changed = &md, true
			}
		} else if c.Md == nil {
			t.Errorf("%s: no Markdown in the vectors; run go test ./internal/ydoc -update", c.Name)
		} else if *c.Md != md {
			t.Errorf("%s: wrote %q, the vectors hold %q; run with -update if that is intended", c.Name, md, *c.Md)
		}
		back := ydoc.Parse(md)
		same := ydoc.Equal(back, doc)
		if why, isLossy := lossy[c.Name]; isLossy {
			if same {
				t.Errorf("%s: now reads back unchanged (%s); take it off lossy", c.Name, why)
			}
			continue
		}
		if !same {
			t.Errorf("%s: %q reads back as\n %s\nnot\n %s", c.Name, md, ydoc.Canonical(back), ydoc.Canonical(doc))
		}
	}
	if *update && changed {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "\t")
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorsPath(), b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote the server's Markdown into shared/doc-vectors.json")
	}
}

func decodeAll(t *testing.T, updates []string) [][]byte {
	t.Helper()
	out := make([][]byte, len(updates))
	for i, u := range updates {
		b, err := base64.StdEncoding.DecodeString(u)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = b
	}
	return out
}

func TestYjsVectors(t *testing.T) {
	for _, c := range load(t).Yjs {
		updates := decodeAll(t, c.Updates)
		want := node(t, c.JSON)
		got, err := ydoc.Read(updates...)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if !ydoc.Equal(got, want) {
			t.Errorf("%s: read\n %s\nnot\n %s", c.Name, ydoc.Canonical(got), ydoc.Canonical(want))
		}
		// Merged, in reverse order, or compacted, the updates read the same.
		rev := make([][]byte, len(updates))
		for i, u := range updates {
			rev[len(updates)-1-i] = u
		}
		merged, err := ydoc.Merge(rev...)
		if err != nil {
			t.Fatalf("%s: merge: %v", c.Name, err)
		}
		compact, err := ydoc.Compact(updates...)
		if err != nil {
			t.Fatalf("%s: compact: %v", c.Name, err)
		}
		for name, u := range map[string][]byte{"merged": merged, "compacted": compact} {
			got, err := ydoc.Read(u)
			if err != nil || !ydoc.Equal(got, want) {
				t.Errorf("%s %s: read %s (%v)", c.Name, name, ydoc.Canonical(got), err)
			}
		}
		// A compacted update with the originals changes nothing.
		got, err = ydoc.Read(append([][]byte{compact}, updates...)...)
		if err != nil || !ydoc.Equal(got, want) {
			t.Errorf("%s: compacted with the originals read %s (%v)", c.Name, ydoc.Canonical(got), err)
		}
	}
}

func TestReplace(t *testing.T) {
	v := load(t)
	base := decodeAll(t, v.Yjs[0].Updates)
	before, _ := ydoc.Read(base...)

	// Writing the same document changes nothing.
	same, err := ydoc.Replace(base, before, 900)
	if err != nil || same != nil {
		t.Fatalf("replacing a document with itself gave %d bytes, %v", len(same), err)
	}

	// A new last paragraph and a changed heading: the rest stays.
	next := before.Clone()
	next.Content[0] = ydoc.Parse("## Plan for the weekend").Content[0]
	next.Content = append(next.Content, ydoc.Parse("Done by Sunday.").Content...)
	u, err := ydoc.Replace(base, next, 901)
	if err != nil || u == nil {
		t.Fatalf("replace: %d bytes, %v", len(u), err)
	}
	got, err := ydoc.Read(append(base, u)...)
	if err != nil || !ydoc.Equal(got, next) {
		t.Fatalf("after replacing: %s (%v)\nwant %s", ydoc.Canonical(got), err, ydoc.Canonical(next))
	}

	// From Markdown into an empty document, as the server makes a task's
	// first document.
	first, err := ydoc.Replace(nil, ydoc.Parse("Buy *cedar*.\n\n- one\n- two"), 902)
	if err != nil || first == nil {
		t.Fatalf("first document: %v", err)
	}
	got, _ = ydoc.Read(first)
	if md := ydoc.Markdown(got); md != "Buy *cedar*.\n\n- one\n- two" {
		t.Errorf("first document reads as %q", md)
	}

	// Every document the vectors read goes into Yjs and comes back the same.
	for i, c := range v.Parse {
		doc := ydoc.Parse(c.Md)
		u, err := ydoc.Replace(nil, doc, uint64(1000+i))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if u == nil {
			continue
		}
		got, err := ydoc.Read(u)
		if err != nil || !ydoc.Equal(got, doc) {
			t.Errorf("%s: into Yjs and back gives %s (%v)\nwant %s", c.Name, ydoc.Canonical(got), err, ydoc.Canonical(doc))
		}
	}

	// A broken update is refused.
	if _, err := ydoc.Read([]byte{0xff, 0x00, 0x13}); err == nil {
		t.Error("a broken update was read")
	}
}

func TestRenameLinks(t *testing.T) {
	doc := ydoc.Parse("See [Old name](/wiki?page=a) and [**Old** name](/wiki?page=a), not [the plan](/wiki?page=a) or [Old name](/wiki?page=b).\n\n- [Old name](/wiki?page=a) in a list")
	isA := func(href string) bool { return href == "/wiki?page=a" }
	got, changed := ydoc.RenameLinks(doc, isA, "Old name", "New name")
	if !changed {
		t.Fatal("nothing renamed")
	}
	want := "See [New name](/wiki?page=a) and [New name](/wiki?page=a), not [the plan](/wiki?page=a) or [Old name](/wiki?page=b).\n\n- [New name](/wiki?page=a) in a list"
	if md := ydoc.Markdown(got); md != want {
		t.Errorf("renamed:\n%s\nwant\n%s", md, want)
	}
	if ydoc.Markdown(doc) == want {
		t.Error("the document given was changed")
	}
	if _, changed := ydoc.RenameLinks(doc, isA, "Missing", "New"); changed {
		t.Error("renamed a name no link has")
	}
}

// blocks is a document of n one-line paragraphs, each named by prefix.
func blocks(prefix string, n int) *ydoc.Node {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "%s%d\n\n", prefix, i)
	}
	return ydoc.Parse(b.String())
}

// allocated is what f allocates, in bytes.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// A write of many blocks in place of many others costs memory in line
// with the documents, not with the two sizes multiplied: the table of
// what they share is bounded, and past it the middle is replaced whole.
func TestReplaceLargeDocumentsStayBounded(t *testing.T) {
	const n = 6000
	base, err := ydoc.Replace(nil, blocks("a", n), 1)
	if err != nil {
		t.Fatal(err)
	}
	next := blocks("b", n)
	// What reading and writing the documents costs anyway.
	floor := allocated(func() {
		if _, err := ydoc.Replace(nil, next, 2); err != nil {
			t.Fatal(err)
		}
		if _, err := ydoc.Read(base); err != nil {
			t.Fatal(err)
		}
	})
	var u []byte
	cost := allocated(func() {
		if u, err = ydoc.Replace([][]byte{base}, next, 3); err != nil {
			t.Fatal(err)
		}
	})
	if limit := 2*floor + 64<<20; cost > limit {
		t.Errorf("replacing %d blocks with %d others allocated %d MB, over %d MB", n, n, cost>>20, limit>>20)
	}
	got, err := ydoc.Read(base, u)
	if err != nil || !ydoc.Equal(got, next) {
		t.Fatalf("after replacing: %d blocks (%v)", len(got.Content), err)
	}

	// The blocks either side of a large change stay, so an edit to them
	// that another device has not sent yet survives.
	edited := blocks("a", n)
	middle := blocks("c", n)
	edited.Content = append(append(edited.Content[:10:10], middle.Content...), edited.Content[n-10:]...)
	u, err = ydoc.Replace([][]byte{base}, edited, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err = ydoc.Read(base, u)
	if err != nil || !ydoc.Equal(got, edited) {
		t.Fatalf("after replacing the middle: %d blocks (%v)", len(got.Content), err)
	}
	// Another device changed the first block meanwhile: merged, its change
	// stands, once, before the new middle.
	other := blocks("a", n)
	other.Content[0] = ydoc.Parse("changed elsewhere").Content[0]
	v, err := ydoc.Replace([][]byte{base}, other, 5)
	if err != nil {
		t.Fatal(err)
	}
	got, err = ydoc.Read(base, u, v)
	if err != nil {
		t.Fatal(err)
	}
	want := edited.Clone()
	want.Content[0] = other.Content[0]
	if !ydoc.Equal(got, want) {
		t.Errorf("merged with an edit to the first block: %d blocks, starting %s", len(got.Content), ydoc.Canonical(got.Content[0]))
	}
}

// Any document replacing any other reads back as written, with blocks
// shared at the start, the end and in between.
func TestReplaceRandomDocuments(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 7))
	doc := func() *ydoc.Node {
		var b strings.Builder
		for range r.IntN(12) {
			fmt.Fprintf(&b, "p%d\n\n", r.IntN(4))
		}
		return ydoc.Parse(b.String())
	}
	for k := range 300 {
		before, after := doc(), doc()
		base, err := ydoc.Replace(nil, before, uint64(10+2*k))
		if err != nil {
			t.Fatal(err)
		}
		var updates [][]byte
		if base != nil {
			updates = append(updates, base)
		}
		u, err := ydoc.Replace(updates, after, uint64(11+2*k))
		if err != nil {
			t.Fatal(err)
		}
		if u != nil {
			updates = append(updates, u)
		}
		got, err := ydoc.Read(updates...)
		if err != nil || !ydoc.Equal(got, after) {
			t.Fatalf("%s into %s gave %s (%v)", ydoc.Markdown(before), ydoc.Markdown(after), ydoc.Markdown(got), err)
		}
		if (u == nil) != ydoc.Equal(before, after) {
			t.Fatalf("%s into %s: update of %d bytes", ydoc.Markdown(before), ydoc.Markdown(after), len(u))
		}
	}
}

// Reading and writing Markdown takes time in line with its length, for any
// text a write may hold: the server does both inside a write's transaction,
// which every other write waits for. Each case below once took seconds to
// minutes at this length.
//
// Four times the text should take about four times as long, where a case
// gone quadratic takes sixteen: that holds on any machine, under the race
// detector too. The limit in seconds holds only without it.
func TestMarkdownTakesLinearTime(t *testing.T) {
	const size = 1_000_000 // a page body's limit
	units := []string{
		">", "> ", "- > ", "> - ", "1. > ", // quotes and lists opened on one line
		"[a](", "![a](", "[", "[[", "*a ", "a\n", "[!NOTE]\n", // inline text and links
		"<!-- mila:columns -->\n", "<!-- mila:columns -->\n\n", "<details>\n<summary>a</summary>\n\n", // markers never closed
		"<mark data-color=\"yellow\">", "<span data-size=\"large\">a</span>", // highlights and spans
		"[a]:\n", "[a]: /b\n", "[a](b\n", "[a](b \"\n", "| [a]( |\n", "> [!NOTE]\n", // references, links over lines, tables
	}
	readWrite := func(md string) time.Duration {
		start := time.Now()
		ydoc.Markdown(ydoc.Parse(md))
		return time.Since(start)
	}
	for _, unit := range units {
		n := size / len(unit)
		full, quarter := strings.Repeat(unit, n), strings.Repeat(unit, n/4)
		took := readWrite(full)
		if !raceDetector && took > 3*time.Second {
			t.Errorf("%q repeated to %d bytes took %v to read and write", unit, size, took.Round(time.Millisecond))
		}
		if took < 500*time.Millisecond {
			// Too quick for the ratio to be more than noise (a slow runner's
			// memory, a collection), and quick enough: a case gone quadratic
			// takes seconds at this length.
			continue
		}
		// A busy machine can stall any run, so a ratio past the limit is
		// measured again before it counts, keeping the quickest of three.
		part := readWrite(quarter)
		for range 2 {
			if took <= 8*part {
				break
			}
			took, part = min(took, readWrite(full)), min(part, readWrite(quarter))
		}
		if took > 8*part {
			t.Errorf("%q repeated to %d bytes took %v to read and write, a quarter of it %v: %.1f times as long for four times the text",
				unit, size, took.Round(time.Millisecond), part.Round(time.Millisecond), float64(took)/float64(part))
		}
	}

	// A document can nest further than Markdown typed in one line can (its
	// Yjs is anyone's to write): written as Markdown, past a depth its
	// deepest blocks become their text.
	deep := func(wrap func(*ydoc.Node) *ydoc.Node) *ydoc.Node {
		n := ydoc.Parse("deepest").Content[0]
		for range 100_000 {
			n = wrap(n)
		}
		return &ydoc.Node{Type: "doc", Content: []*ydoc.Node{n}}
	}
	quoted := deep(func(n *ydoc.Node) *ydoc.Node { return &ydoc.Node{Type: "blockquote", Content: []*ydoc.Node{n}} })
	listed := deep(func(n *ydoc.Node) *ydoc.Node {
		return &ydoc.Node{Type: "bulletList", Content: []*ydoc.Node{{Type: "listItem", Content: []*ydoc.Node{n}}}}
	})
	for name, doc := range map[string]*ydoc.Node{"quotes": quoted, "lists": listed} {
		start := time.Now()
		md := ydoc.Markdown(doc)
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("100,000 nested %s took %v to write", name, took.Round(time.Millisecond))
		}
		if !strings.Contains(md, "deepest") {
			t.Errorf("100,000 nested %s lost their text", name)
		}
	}
}

// What keeps reading Markdown in line with its length leaves ordinary text
// read as before.
func TestMarkdownBoundsKeepOrdinaryText(t *testing.T) {
	link := func(md string) string {
		var href string
		var walk func(*ydoc.Node)
		walk = func(n *ydoc.Node) {
			for _, m := range n.Marks {
				if m.Type == "link" {
					href, _ = m.Attrs["href"].(string)
				}
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(ydoc.Parse(md))
		return href
	}

	// Reference links, the definitions after the text or before it.
	if got := link("See [the plan] and [b].\n\n[the plan]: /wiki?page=a\n[b]: /x"); got != "/x" {
		t.Errorf("reference links read as %q", got)
	}
	if got := link("[a]: /first\n" + strings.Repeat("x\n", 1500) + "\nSee [a]."); got != "/first" {
		t.Errorf("a definition before a long paragraph reads as %q", got)
	}
	// A link after a great many tries that are not links, and a long
	// address while no try has failed.
	if got := link(strings.Repeat("[a](", 500) + " [ok](/wiki?page=b)"); got != "/wiki?page=b" {
		t.Errorf("a link after failed tries reads as %q", got)
	}
	long := "/files/" + strings.Repeat("a", 5000)
	if got := link("[a](" + long + " \"title\")"); got != long {
		t.Errorf("a long address reads as %d bytes", len(got))
	}

	// Quotes opened on one line, up to the limit, and text past it.
	depth := func(n *ydoc.Node) (d int, text string) {
		for n.Type == "doc" || n.Type == "blockquote" {
			if n.Type == "blockquote" {
				d++
			}
			n = n.Content[0]
		}
		return d, ydoc.Markdown(&ydoc.Node{Type: "doc", Content: []*ydoc.Node{n}})
	}
	if d, text := depth(ydoc.Parse(strings.Repeat(">", 64) + " deep")); d != 64 || text != "deep" {
		t.Errorf("64 quotes read as %d around %q", d, text)
	}
	if d, text := depth(ydoc.Parse(strings.Repeat(">", 70) + " deep")); d != 64 || text != strings.Repeat(">", 6)+" deep" && text != `\>`+strings.Repeat(">", 5)+" deep" {
		t.Errorf("70 quotes read as %d around %q", d, text)
	}
}

// A NUL typed into a document never reaches its text: Postgres refuses one.
func TestMarkdownLeavesOutNULs(t *testing.T) {
	doc := &ydoc.Node{Type: "doc", Content: []*ydoc.Node{
		{Type: "paragraph", Content: []*ydoc.Node{{Type: "text", Text: "Dig\x00 first"}}},
	}}
	if got := ydoc.Markdown(doc); got != "Dig first" {
		t.Fatalf("Markdown = %q", got)
	}
}
