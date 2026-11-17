package ydoc

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// The formatting beyond Markdown: what Markdown
// can't say is written as an HTML comment marker or a little HTML that other
// Markdown apps show as plain words. web/src/lib/docs/formatting.ts writes and
// reads the same, and shared/doc-vectors.json holds the two together.

// The names each kind of formatting may take. The app's lists match.
var (
	HighlightColors = []string{"yellow", "green", "blue", "pink", "gray"}
	PageFonts       = []string{"inter", "nunito", "source-serif", "jetbrains-mono", "system-sans", "system-serif", "system-mono"}
	WordSizes       = []string{"small", "large", "huge"}
	PageSizes       = []string{"small", "large"}
	ColumnLayouts   = map[int][]string{2: {"1:1", "2:1", "1:2"}, 3: {"1:1:1"}}
	ImageSizes      = []int64{25, 50, 75, 100}
	ImageSides      = []string{"left", "center", "right"}
	ViewLayouts     = []string{"list", "board", "table"}
)

var (
	markOpen     = regexp.MustCompile(`^<mark data-color="(yellow|green|blue|pink|gray)">$`)
	spanOpen     = regexp.MustCompile(`^<span(?: data-font="(inter|nunito|source-serif|jetbrains-mono|system-sans|system-serif|system-mono)")?(?: data-size="(small|large|huge)")?>$`)
	columnsStart = regexp.MustCompile(`^<!-- mila:columns(?: (1:1|2:1|1:2|1:1:1))? -->$`)
	foldStart    = regexp.MustCompile(`^<details( open)?>\n<summary>(.*)</summary>$`)
	viewMarker   = regexp.MustCompile(`^<!-- mila:view ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?: (list|board|table))? -->$`)
	// A chart's key is kept as written, known to this server or not.
	reportMarker = regexp.MustCompile(`^<!-- mila:report ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}) ([a-z][a-z0-9-]{0,39}) -->$`)
	pageMarker   = regexp.MustCompile(`^<!-- mila:page(?: font=(inter|nunito|source-serif|jetbrains-mono|system-sans|system-serif|system-mono))?(?: size=(small|large))? -->$`)
	imageMarker  = regexp.MustCompile(`^<!-- mila:image (25|50|75|100) (left|center|right) -->`)
)

const (
	columnMarker = "<!-- mila:column -->"
	columnsEnd   = "<!-- mila:end -->"
	foldEnd      = "</details>"
)

// ViewAddress is where a saved view is in the app.
func ViewAddress(id string) string { return "/views/" + id }

// htmlMarkOpen is the tag that opens a highlight or a font and size, or "".
func htmlMarkOpen(m Mark) string {
	switch m.Type {
	case "highlight":
		color, _ := m.Attrs["color"].(string)
		if !slices.Contains(HighlightColors, color) {
			color = HighlightColors[0]
		}
		return `<mark data-color="` + color + `">`
	case "textStyle":
		font, _ := m.Attrs["font"].(string)
		size, _ := m.Attrs["size"].(string)
		var attrs string
		if slices.Contains(PageFonts, font) {
			attrs += ` data-font="` + font + `"`
		}
		if slices.Contains(WordSizes, size) {
			attrs += ` data-size="` + size + `"`
		}
		if attrs == "" {
			return ""
		}
		return "<span" + attrs + ">"
	}
	return ""
}

func htmlMarkClose(m Mark) string {
	switch {
	case htmlMarkOpen(m) == "":
		return ""
	case m.Type == "highlight":
		return "</mark>"
	case m.Type == "textStyle":
		return "</span>"
	}
	return ""
}

// readHTMLMark reads a tag that opens a highlight or a font and size.
func readHTMLMark(raw string) (Mark, string, bool) {
	if m := markOpen.FindStringSubmatch(raw); m != nil {
		return Mark{Type: "highlight", Attrs: map[string]any{"color": m[1]}}, "</mark>", true
	}
	if m := spanOpen.FindStringSubmatch(raw); m != nil && (m[1] != "" || m[2] != "") {
		attrs := map[string]any{"font": nil, "size": nil}
		if m[1] != "" {
			attrs["font"] = m[1]
		}
		if m[2] != "" {
			attrs["size"] = m[2]
		}
		return Mark{Type: "textStyle", Attrs: attrs}, "</span>", true
	}
	return Mark{}, "", false
}

// rawText is an inline HTML node's text.
func rawText(t *ast.RawHTML, src []byte) string {
	var s strings.Builder
	for i := 0; i < t.Segments.Len(); i++ {
		seg := t.Segments.At(i)
		s.Write(seg.Value(src))
	}
	return s.String()
}

// tagsAfter is where the tags among a node's children are, worked out once
// for all of them: looking along the later siblings for each tag that
// opens would take time of their count squared.
type tagsAfter struct {
	index map[ast.Node]int // each raw tag's place among the children
	raw   []string         // each child's text, if a raw tag
	last  map[string]int   // per closing tag asked about, its last place
}

func closingTags(n ast.Node, src []byte) *tagsAfter {
	t := &tagsAfter{index: map[ast.Node]int{}, last: map[string]int{}}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		raw := ""
		if r, ok := c.(*ast.RawHTML); ok {
			t.index[c] = len(t.raw)
			raw = rawText(r, src)
		}
		t.raw = append(t.raw, raw)
	}
	return t
}

// later says whether a tag that opens is closed by a later sibling, as the
// editor reads such a tag only when it is.
func (t *tagsAfter) later(c ast.Node, closing string) bool {
	last, ok := t.last[closing]
	if !ok {
		last = -1
		for k := len(t.raw) - 1; k >= 0; k-- {
			if t.raw[k] != "" && strings.EqualFold(t.raw[k], closing) {
				last = k
				break
			}
		}
		t.last[closing] = last
	}
	return last > t.index[c]
}

// htmlBlockText is an HTML block's text, its lines joined and trimmed.
func htmlBlockText(b *ast.HTMLBlock, src []byte) string {
	var s strings.Builder
	for i := 0; i < b.Lines().Len(); i++ {
		seg := b.Lines().At(i)
		s.Write(seg.Value(src))
	}
	if b.HasClosure() {
		s.Write(b.ClosureLine.Value(src))
	}
	return strings.TrimRight(strings.ReplaceAll(s.String(), "\r\n", "\n"), " \t\n")
}

func blockText(n ast.Node, src []byte) string {
	if b, ok := n.(*ast.HTMLBlock); ok {
		return htmlBlockText(b, src)
	}
	return ""
}

// markerMatch is where a columns or fold marker's end is, as a distance
// from it (-1 for none), and for columns where the markers between its
// columns are. Distances hold in any run of the blocks it is in, as columns
// and folds are read from runs of their siblings.
type markerMatch struct {
	end      int
	dividers []int
}

var matchAttr = []byte("mila-match")

// matchOf is the match of the marker at kids[i], worked out for every marker
// from there on at once and kept on each: a marker that never ends would
// otherwise send each one before it looking along the rest of the document.
// A marker's end is the first after it that no marker opened since takes.
func matchOf(kids []ast.Node, i int, src []byte) *markerMatch {
	if m, ok := kids[i].Attribute(matchAttr); ok {
		return m.(*markerMatch)
	}
	var cols, folds []int
	open := func(j int) {
		kids[j].SetAttribute(matchAttr, &markerMatch{end: -1})
	}
	at := func(j int) *markerMatch {
		m, _ := kids[j].Attribute(matchAttr)
		return m.(*markerMatch)
	}
	for j := i; j < len(kids); j++ {
		t := blockText(kids[j], src)
		switch {
		case t == "":
		case columnsStart.MatchString(t):
			open(j)
			cols = append(cols, j)
		case t == columnsEnd && len(cols) > 0:
			s := cols[len(cols)-1]
			cols = cols[:len(cols)-1]
			at(s).end = j - s
		case t == columnMarker && len(cols) > 0:
			s := cols[len(cols)-1]
			at(s).dividers = append(at(s).dividers, j-s)
		case foldStart.MatchString(t):
			open(j)
			folds = append(folds, j)
		case t == foldEnd && len(folds) > 0:
			s := folds[len(folds)-1]
			folds = folds[:len(folds)-1]
			at(s).end = j - s
		}
	}
	return at(i)
}

// readFormatted reads, from the blocks starting at kids[i], one of the blocks
// written as markers: columns, a foldable section, a view or a page's style.
// It returns the node and how many blocks it took, or nil.
func readFormatted(kids []ast.Node, i int, src []byte, read func([]ast.Node) []*Node) (*Node, int) {
	text := blockText(kids[i], src)
	if text == "" {
		return nil, 0
	}
	if m := columnsStart.FindStringSubmatch(text); m != nil {
		match := matchOf(kids, i, src)
		if match.end < 0 || i+match.end >= len(kids) {
			return nil, 0
		}
		j := i + match.end
		parts, from := [][]ast.Node{}, i+1
		for _, d := range match.dividers {
			parts = append(parts, kids[from:i+d])
			from = i + d + 1
		}
		parts = append(parts, kids[from:j])
		if len(parts) < 2 || len(parts) > 3 {
			return nil, 0
		}
		layout := m[1]
		if !slices.Contains(ColumnLayouts[len(parts)], layout) {
			layout = ColumnLayouts[len(parts)][0]
		}
		cols := &Node{Type: "columns", Attrs: map[string]any{"layout": layout}}
		for _, p := range parts {
			content := read(p)
			if len(content) == 0 {
				content = []*Node{{Type: "paragraph"}}
			}
			cols.Content = append(cols.Content, &Node{Type: "column", Content: content})
		}
		return cols, j - i + 1
	}
	if m := foldStart.FindStringSubmatch(text); m != nil {
		match := matchOf(kids, i, src)
		if match.end < 0 || i+match.end >= len(kids) {
			return nil, 0
		}
		j := i + match.end
		content := read(kids[i+1 : j])
		if len(content) == 0 {
			content = []*Node{{Type: "paragraph"}}
		}
		title := &Node{Type: "foldTitle"}
		if s := decode([]byte(m[2])); s != "" {
			title.Content = []*Node{{Type: "text", Text: s}}
		}
		fold := &Node{Type: "fold", Attrs: map[string]any{"open": m[1] != ""}, Content: append([]*Node{title}, content...)}
		return fold, j - i + 1
	}
	if m := viewMarker.FindStringSubmatch(text); m != nil {
		view := &Node{Type: "viewEmbed", Attrs: map[string]any{"id": m[1], "name": "", "layout": nil}}
		if m[2] != "" {
			view.Attrs["layout"] = m[2]
		}
		if i+1 < len(kids) {
			if p, ok := kids[i+1].(*ast.Paragraph); ok && p.Lines().Len() == 1 && blankLinesBefore(src, p.Lines().At(0).Start) == 0 {
				if name, ok := onlyLink(readInline(p, src), ViewAddress(m[1])); ok {
					view.Attrs["name"] = name
					return view, 2
				}
			}
		}
		return view, 1
	}
	if m := reportMarker.FindStringSubmatch(text); m != nil {
		report := &Node{Type: "reportEmbed", Attrs: map[string]any{"id": m[1], "chart": m[2], "name": ""}}
		if i+1 < len(kids) {
			if p, ok := kids[i+1].(*ast.Paragraph); ok && p.Lines().Len() == 1 && blankLinesBefore(src, p.Lines().At(0).Start) == 0 {
				if name, ok := onlyLink(readInline(p, src), ViewAddress(m[1])); ok {
					report.Attrs["name"] = name
					return report, 2
				}
			}
		}
		return report, 1
	}
	if m := pageMarker.FindStringSubmatch(text); m != nil {
		style := &Node{Type: "pageStyle", Attrs: map[string]any{"font": nil, "size": nil}}
		if m[1] != "" {
			style.Attrs["font"] = m[1]
		}
		if m[2] != "" {
			style.Attrs["size"] = m[2]
		}
		return style, 1
	}
	return nil, 0
}

// onlyLink is the text of inline content that is one link to an address, and nothing else.
func onlyLink(content []*Node, href string) (string, bool) {
	var b strings.Builder
	for _, n := range content {
		if !n.IsText() || len(n.Marks) != 1 || n.Marks[0].Type != "link" || n.Marks[0].Attrs["href"] != href || n.Marks[0].Attrs["title"] != nil {
			return "", false
		}
		b.WriteString(n.Text)
	}
	return b.String(), b.Len() > 0
}

// imageMarkers reads the marker after an image, its size and side, and the
// italic line under an image alone in its paragraph, its caption.
func imageMarkers(content []*Node) []*Node {
	for i := 0; i+1 < len(content); i++ {
		img, next := content[i], content[i+1]
		if img.Type != "image" || !next.IsText() || len(next.Marks) > 0 {
			continue
		}
		m := imageMarker.FindStringSubmatch(next.Text)
		if m == nil {
			continue
		}
		size, _ := strconv.ParseInt(m[1], 10, 64)
		img.Attrs = withAttrs(img.Attrs, map[string]any{"size": size, "side": m[2]})
		next.Text = next.Text[len(m[0]):]
		if next.Text == "" {
			content = append(content[:i+1], content[i+2:]...)
		}
	}
	if len(content) == 3 && content[0].Type == "image" && content[0].Attr("size") != nil &&
		content[1].IsText() && content[1].Text == "\n" && len(content[1].Marks) == 0 &&
		content[2].IsText() && len(content[2].Marks) == 1 && content[2].Marks[0].Type == "italic" {
		content[0].Attrs = withAttrs(content[0].Attrs, map[string]any{"caption": content[2].Text})
		content = content[:1]
	}
	return content
}

func withAttrs(a, more map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// Writing.

// formatted writes a block written as markers, and reports whether it was one.
func formatted(n *Node) (string, bool) {
	switch n.Type {
	case "columns":
		layout, _ := n.Attr("layout").(string)
		marker := "<!-- mila:columns -->"
		if layout != "" && layout != ColumnLayouts[len(n.Content)][0] && slices.Contains(ColumnLayouts[len(n.Content)], layout) {
			marker = "<!-- mila:columns " + layout + " -->"
		}
		parts := make([]string, 0, len(n.Content))
		for _, col := range n.Content {
			parts = append(parts, strings.Join(blocks(col.Content), "\n\n"))
		}
		return marker + "\n\n" + strings.Join(parts, "\n\n"+columnMarker+"\n\n") + "\n\n" + columnsEnd, true
	case "fold":
		open := "<details>"
		if on, _ := n.Attr("open").(bool); on {
			open = "<details open>"
		}
		title := ""
		var body []*Node
		for _, c := range n.Content {
			if c.Type == "foldTitle" {
				title = plain(c.Content)
			} else {
				body = append(body, c)
			}
		}
		inner := strings.Join(blocks(body), "\n\n")
		if inner == "" {
			inner = "&nbsp;"
		}
		return open + "\n<summary>" + htmlText.Replace(strings.ReplaceAll(title, "\n", " ")) + "</summary>\n\n" + inner + "\n\n" + foldEnd, true
	case "viewEmbed":
		id, _ := n.Attr("id").(string)
		marker := "<!-- mila:view " + strings.ToLower(id) + " -->"
		if !viewMarker.MatchString(marker) {
			return "", true
		}
		if layout, _ := n.Attr("layout").(string); slices.Contains(ViewLayouts, layout) {
			marker = "<!-- mila:view " + strings.ToLower(id) + " " + layout + " -->"
		}
		name, _ := n.Attr("name").(string)
		name = strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
		if name == "" {
			return marker, true
		}
		return marker + "\n" + inline([]*Node{{Type: "text", Text: name, Marks: []Mark{{Type: "link", Attrs: map[string]any{"href": ViewAddress(strings.ToLower(id))}}}}}, false), true
	case "reportEmbed":
		id, _ := n.Attr("id").(string)
		chart, _ := n.Attr("chart").(string)
		marker := "<!-- mila:report " + strings.ToLower(id) + " " + chart + " -->"
		if !reportMarker.MatchString(marker) {
			return "", true
		}
		name, _ := n.Attr("name").(string)
		name = strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
		if name == "" {
			return marker, true
		}
		return marker + "\n" + inline([]*Node{{Type: "text", Text: name, Marks: []Mark{{Type: "link", Attrs: map[string]any{"href": ViewAddress(strings.ToLower(id))}}}}}, false), true
	case "pageStyle":
		font, _ := n.Attr("font").(string)
		size, _ := n.Attr("size").(string)
		var attrs string
		if slices.Contains(PageFonts, font) {
			attrs += " font=" + font
		}
		if slices.Contains(PageSizes, size) {
			attrs += " size=" + size
		}
		if attrs == "" {
			return "", true
		}
		return "<!-- mila:page" + attrs + " -->", true
	}
	return "", false
}

var htmlText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// imageExtras is the marker after an image with a size or side.
func imageExtras(n *Node) string {
	size, hasSize := n.Attr("size").(int64)
	side, hasSide := n.Attr("side").(string)
	if !hasSize && !hasSide {
		return ""
	}
	if !slices.Contains(ImageSizes, size) {
		size = 100
	}
	if !slices.Contains(ImageSides, side) {
		side = "center"
	}
	return "<!-- mila:image " + strconv.FormatInt(size, 10) + " " + side + " -->"
}

// caption is the line under an image alone in its paragraph.
func caption(content []*Node) string {
	if len(content) != 1 || content[0].Type != "image" || imageExtras(content[0]) == "" {
		return ""
	}
	c, _ := content[0].Attr("caption").(string)
	c = strings.TrimSpace(strings.ReplaceAll(c, "\n", " "))
	if c == "" {
		return ""
	}
	return "\n" + inline([]*Node{{Type: "text", Text: c, Marks: []Mark{{Type: "italic"}}}}, false)
}

// htmlCloses are the tags that close a highlight or a font and size.
var htmlCloses = map[string]string{"</mark>": "highlight", "</span>": "textStyle"}
