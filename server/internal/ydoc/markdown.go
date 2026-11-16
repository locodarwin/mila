package ydoc

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Writing Markdown. The rule is that the editor reads back what is
// written as the same document (TestRenderReadsBack, docVectors.spec.ts),
// not that the text matches what the editor itself writes: the editor's
// writer leaves some text that reads back differently ("# not a heading"
// comes back as a heading), which the server's escapes.

// Markdown writes a document as Markdown.
func Markdown(doc *Node) string {
	if doc == nil {
		return ""
	}
	content := shallow(Normalize(doc).Content, 1, true)
	// The editor keeps an empty paragraph after a last block that is not
	// one (a list, a table), for the cursor to go to; it is not written,
	// as the editor's own Markdown leaves it out.
	for len(content) > 0 && content[len(content)-1].Type == "paragraph" && len(content[len(content)-1].Content) == 0 {
		content = content[:len(content)-1]
	}
	// A NUL typed into the document never reaches the text: Postgres refuses
	// it, and nothing shows one.
	return strings.ReplaceAll(strings.Join(blocks(content), "\n\n"), "\x00", "")
}

// maxDepth is how deep a document is written as Markdown. Each level
// writes everything inside it again (a quote's "> " before each line), so
// a document nested further, which its Yjs can be, would take time of
// its depth squared.
const maxDepth = 200

// holdsBlocks are the nodes any block may go in.
var holdsBlocks = map[string]bool{"doc": true, "blockquote": true, "callout": true, "listItem": true, "taskItem": true, "column": true, "fold": true, "tableCell": true, "tableHeader": true}

// textblocks hold only inline content, so they nest no further.
var textblocks = map[string]bool{"paragraph": true, "heading": true, "codeBlock": true, "foldTitle": true}

// shallow is nodes at a depth with anything below maxDepth turned into a
// paragraph of its text, where a block may go (in holds); lists and
// tables, whose children cannot be paragraphs, go on to their first nodes
// that hold blocks, two levels down at most. It gives back nodes itself
// when nothing is that deep.
func shallow(nodes []*Node, depth int, holds bool) []*Node {
	var out []*Node
	for i, n := range nodes {
		next := n
		switch {
		case len(n.Content) == 0 || textblocks[n.Type]:
		case depth >= maxDepth && holds:
			next = &Node{Type: "paragraph", Content: inlineOf(n, nil)}
		default:
			if kids := shallow(n.Content, depth+1, holdsBlocks[n.Type]); &kids[0] != &n.Content[0] {
				c := *n
				c.Content = kids
				next = &c
			}
		}
		if next != n && out == nil {
			out = append(make([]*Node, 0, len(nodes)), nodes[:i]...)
		}
		if out != nil {
			out = append(out, next)
		}
	}
	if out == nil {
		return nodes
	}
	return out
}

// inlineOf is the inline content of every textblock in n, a line apart.
func inlineOf(n *Node, out []*Node) []*Node {
	if textblocks[n.Type] {
		if len(out) > 0 {
			out = append(out, &Node{Type: "hardBreak"})
		}
		return append(out, n.Content...)
	}
	for _, c := range n.Content {
		out = inlineOf(c, out)
	}
	return out
}

func blocks(nodes []*Node) []string {
	out := make([]string, 0, len(nodes))
	alt := false
	for i, n := range nodes {
		// A block that writes nothing (a page style with nothing set, a view
		// with no ID) takes no place, or it would read as empty paragraphs.
		if s, ok := formatted(n); ok {
			if s != "" {
				out = append(out, s)
			}
			alt = false
			continue
		}
		// Two lists of a kind in a row would read as one: the second
		// takes the other marker.
		if i > 0 && nodes[i-1].Type == n.Type && isList(n.Type) {
			alt = !alt
		} else {
			alt = false
		}
		if alt {
			out = append(out, list(n, true))
		} else {
			out = append(out, block(n))
		}
	}
	return out
}

func isList(t string) bool {
	return t == "bulletList" || t == "orderedList" || t == "taskList"
}

func block(n *Node) string {
	switch n.Type {
	case "paragraph":
		if len(n.Content) == 0 {
			// Markdown has no empty paragraph; the editor reads this as one.
			return "&nbsp;"
		}
		return inline(n.Content, false) + caption(n.Content)
	case "heading":
		level := 1
		if l, ok := n.Attr("level").(int64); ok && l >= 1 && l <= 6 {
			level = int(l)
		}
		return strings.Repeat("#", level) + " " + inline(n.Content, true)
	case "codeBlock":
		body := plain(n.Content)
		fence := "```"
		for strings.Contains(body, fence) {
			fence += "`"
		}
		lang, _ := n.Attr("language").(string)
		return fence + lang + "\n" + body + "\n" + fence
	case "horizontalRule":
		return "---"
	case "blockquote":
		return quote(strings.Join(blocks(n.Content), "\n\n"))
	case "callout":
		kind, _ := n.Attr("kind").(string)
		if !calloutKinds[kind] {
			kind = "note"
		}
		head := "> [!" + strings.ToUpper(kind) + "]"
		if len(n.Content) == 1 && n.Content[0].Type == "paragraph" && len(n.Content[0].Content) == 0 {
			return head
		}
		return head + "\n" + quote(strings.Join(blocks(n.Content), "\n\n"))
	case "bulletList", "orderedList", "taskList":
		return list(n, false)
	case "table":
		return table(n)
	}
	// A node this server does not know keeps its text.
	return inline(n.Content, false)
}

// quote puts Markdown in a block quote.
func quote(md string) string {
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

// calloutKinds are GitHub's boxes, written `> [!NOTE]` and so on.
var calloutKinds = map[string]bool{"note": true, "tip": true, "important": true, "warning": true, "caution": true}

func list(n *Node, alt bool) string {
	start := int64(1)
	if s, ok := n.Attr("start").(int64); ok {
		start = s
	}
	items := make([]string, 0, len(n.Content))
	for i, item := range n.Content {
		marker := "- "
		if alt {
			marker = "* "
		}
		box := ""
		if n.Type == "taskList" {
			box = "[ ] "
			if checked, _ := item.Attr("checked").(bool); checked {
				box = "[x] "
			}
		}
		if n.Type == "orderedList" {
			delim := ". "
			if alt {
				delim = ") "
			}
			marker = strconv.FormatInt(start+int64(i), 10) + delim
		}
		indent := strings.Repeat(" ", len(marker))
		var b strings.Builder
		for j, child := range item.Content {
			if j > 0 {
				// A nested list follows its text directly; anything else
				// after a blank line.
				if isList(child.Type) {
					b.WriteString("\n")
				} else {
					b.WriteString("\n\n")
				}
			}
			s := block(child)
			if child.Type == "paragraph" && len(child.Content) == 0 {
				s = ""
			}
			b.WriteString(s)
		}
		lines := strings.Split(b.String(), "\n")
		for k := range lines {
			switch {
			case k == 0 && box != "" && lines[k] == "":
				// An empty item keeps its box, with the space after it.
				lines[k] = marker + box
			case k == 0:
				lines[k] = strings.TrimRight(marker+box+lines[k], " ")
			case lines[k] != "":
				lines[k] = indent + lines[k]
			}
		}
		items = append(items, strings.Join(lines, "\n"))
	}
	return strings.Join(items, "\n")
}

// table writes a table: its first row is the header, as Markdown has it,
// and each column takes the alignment its first cell has. A cell holds
// one line of text, so its paragraphs and line breaks become spaces.
func table(n *Node) string {
	cols := 0
	for _, r := range n.Content {
		cols = max(cols, len(r.Content))
	}
	if cols == 0 {
		return ""
	}
	line := func(cells []string) string {
		return "| " + strings.Join(cells, " | ") + " |"
	}
	var out []string
	for i, r := range n.Content {
		cells := make([]string, cols)
		for j, c := range r.Content {
			var parts []string
			for _, b := range c.Content {
				if b.Type == "paragraph" || b.Type == "heading" {
					parts = append(parts, inline(b.Content, true))
				} else {
					parts = append(parts, strings.ReplaceAll(block(b), "\n", " "))
				}
			}
			cells[j] = strings.ReplaceAll(strings.Join(parts, " "), "|", `\|`)
		}
		out = append(out, line(cells))
		if i == 0 {
			rule := make([]string, cols)
			for j := range rule {
				rule[j] = "---"
				if j < len(r.Content) {
					switch r.Content[j].Attr("align") {
					case "left":
						rule[j] = ":--"
					case "center":
						rule[j] = ":-:"
					case "right":
						rule[j] = "--:"
					}
				}
			}
			out = append(out, line(rule))
		}
	}
	return strings.Join(out, "\n")
}

// plain is the text of a code block.
func plain(nodes []*Node) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(n.Text)
	}
	return b.String()
}

// markOrder is the order marks open in, outermost first: code is always
// innermost, since nothing nests inside it.
var markOrder = map[string]int{"link": 0, "bold": 1, "italic": 2, "strike": 3, "underline": 4, "highlight": 5, "textStyle": 6, "code": 7}

func openMark(m Mark) string {
	switch m.Type {
	case "bold":
		return "**"
	case "italic":
		return "*"
	case "strike":
		return "~~"
	case "underline":
		return "<u>"
	case "link":
		return "["
	case "highlight", "textStyle":
		return htmlMarkOpen(m)
	}
	return ""
}

func closeMark(m Mark) string {
	switch m.Type {
	case "bold":
		return "**"
	case "italic":
		return "*"
	case "strike":
		return "~~"
	case "underline":
		return "</u>"
	case "link":
		href, _ := m.Attrs["href"].(string)
		dest := href
		if dest == "" || strings.ContainsAny(dest, " ()<>") {
			dest = "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(dest) + ">"
		}
		if title, ok := m.Attrs["title"].(string); ok && title != "" {
			return "](" + dest + ` "` + strings.ReplaceAll(title, `"`, `\"`) + `")`
		}
		return "](" + dest + ")"
	case "highlight", "textStyle":
		return htmlMarkClose(m)
	}
	return ""
}

// inline writes text with its marks. In a heading, a hard break becomes a
// space, since a heading is one line.
func inline(nodes []*Node, heading bool) string {
	var b strings.Builder
	type opened struct {
		mark  Mark
		delim string
	}
	var open []opened
	last := func() byte {
		s := b.String()
		if s == "" {
			return 0
		}
		return s[len(s)-1]
	}
	closeTo := func(keep int) {
		for len(open) > keep {
			o := open[len(open)-1]
			if o.delim != "" {
				b.WriteString(o.delim)
			} else {
				b.WriteString(closeMark(o.mark))
			}
			open = open[:len(open)-1]
		}
	}
	// prefix is how many of the open marks the marks given continue.
	prefix := func(marks []Mark) int {
		k := 0
		for k < len(open) && k < len(marks) && sameMarks([]Mark{open[k].mark}, marks[k:k+1]) {
			k++
		}
		return k
	}
	textMarks := func(n *Node) []Mark {
		var out []Mark
		for _, m := range n.Marks {
			if m.Type != "code" {
				out = append(out, m)
			}
		}
		sortMarks(out)
		return out
	}
	lineStart := true
	for i, n := range nodes {
		if n.Type == "hardBreak" {
			closeTo(0)
			if heading {
				b.WriteString(" ")
			} else {
				b.WriteString("\\\n")
				lineStart = true
			}
			continue
		}
		if n.Type == "image" {
			// An image carries no marks: those around it close first.
			closeTo(0)
			b.WriteString(image(n))
			b.WriteString(imageExtras(n))
			lineStart = false
			continue
		}
		if !n.IsText() {
			continue
		}
		marks := textMarks(n)
		code := hasCode(n.Marks)
		keep := prefix(marks)
		closeTo(keep)
		body := n.Text
		// Spaces at the edges of marked text go outside the marks that
		// open or close there, where Markdown expects them.
		if len(marks) > keep && !code && delimited(marks[keep:]) {
			trimmed := strings.TrimLeft(body, " \t")
			b.WriteString(body[:len(body)-len(trimmed)])
			body = trimmed
		}
		for _, m := range marks[keep:] {
			delim := ""
			switch m.Type {
			case "bold", "italic":
				// After an asterisk, another would run into it: underscores
				// say the same and stay apart.
				c := "*"
				if last() == '*' {
					c = "_"
				}
				delim = c
				if m.Type == "bold" {
					delim = c + c
				}
				b.WriteString(delim)
			default:
				b.WriteString(openMark(m))
			}
			open = append(open, opened{m, delim})
		}
		next := 0
		if i+1 < len(nodes) && nodes[i+1].IsText() {
			next = prefix(textMarks(nodes[i+1]))
		}
		trail := ""
		closing := make([]Mark, 0, len(open))
		for _, o := range open[min(next, len(open)):] {
			closing = append(closing, o.mark)
		}
		if next < len(open) && !code && delimited(closing) {
			trimmed := strings.TrimRight(body, " \t")
			trail = body[len(trimmed):]
			body = trimmed
		}
		if code {
			b.WriteString(codeSpan(body))
		} else {
			b.WriteString(escape(body, lineStart))
		}
		lineStart = false
		if trail != "" {
			closeTo(next)
			b.WriteString(trail)
		}
	}
	closeTo(0)
	return b.String()
}

// image writes an image: ![alt](src "title").
func image(n *Node) string {
	alt, _ := n.Attr("alt").(string)
	src, _ := n.Attr("src").(string)
	title, _ := n.Attr("title").(string)
	return "![" + imageAlt.Replace(alt) + closeMark(Mark{Type: "link", Attrs: map[string]any{"href": src, "title": title}})
}

// imageAlt escapes an image's alt text: only its brackets, since the
// editor reads every other backslash in it as written.
var imageAlt = strings.NewReplacer("[", `\[`, "]", `\]`)

// imageAltOf is an image's alt text as the editor reads it: the text
// between its brackets as written, but for escaped brackets.
func imageAltOf(img *ast.Image, src []byte) string {
	var first, last *ast.Text
	for k := img.FirstChild(); k != nil; k = k.NextSibling() {
		if tx, ok := k.(*ast.Text); ok {
			if first == nil {
				first = tx
			}
			last = tx
		}
	}
	if first == nil {
		return ""
	}
	start, stop := first.Segment.Start, last.Segment.Stop
	for start > 1 && (src[start-1] != '[' || src[start-2] != '!') {
		start--
	}
	for stop < len(src) && src[stop] != ']' {
		stop++
	}
	if start > stop {
		return ""
	}
	return strings.NewReplacer(`\[`, "[", `\]`, "]").Replace(string(src[start:stop]))
}

// delimited says whether any of these marks is written with Markdown's
// delimiters, which can't have spaces just inside them; the HTML ones can.
func delimited(marks []Mark) bool {
	for _, m := range marks {
		if m.Type != "highlight" && m.Type != "textStyle" {
			return true
		}
	}
	return false
}

func sortMarks(marks []Mark) {
	for i := 1; i < len(marks); i++ {
		for j := i; j > 0 && markOrder[marks[j].Type] < markOrder[marks[j-1].Type]; j-- {
			marks[j], marks[j-1] = marks[j-1], marks[j]
		}
	}
}

func hasCode(marks []Mark) bool {
	for _, m := range marks {
		if m.Type == "code" {
			return true
		}
	}
	return false
}

// codeSpan writes code between enough backticks that none inside ends it.
func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	ticks := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || (strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.TrimSpace(s) != "") {
		s = " " + s + " "
	}
	return ticks + s + ticks
}

var (
	// What starts a block when it starts a line: a heading, a list item,
	// a quote, a fence, a rule or the line under a heading.
	blockStart = regexp.MustCompile(`^(#{1,6}(\s|$)|[-+*](\s|$)|\d{1,9}[.)](\s|$)|>|={2,}\s*$|-{2,}\s*$|` + "```|~~~" + `)`)
	escapable  = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`, "~", `\~`, "&", "&amp;", "<", "&lt;")
)

// escape writes text so Markdown reads it back as text.
func escape(s string, lineStart bool) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = escapable.Replace(l)
		if (i > 0 || lineStart) && blockStart.MatchString(l) {
			l = escapeStart(l)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// escapeStart puts a backslash before the character that would start a block.
func escapeStart(l string) string {
	i := 0
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	if i > 0 {
		return l[:i] + `\` + l[i:]
	}
	return `\` + l
}

// Reading Markdown, into the structure the editor would read from it.

var reader = goldmark.New(
	goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(inlineParsers()...),
		parser.WithParagraphTransformers(paragraphTransformers()...),
	)),
	goldmark.WithExtensions(extension.Strikethrough, extension.Table, extension.TaskList),
)

// maxLineNesting is how many quotes and list items one line may open.
// The Markdown reader looks back along a line once for each it opens, so
// a line opening thousands would take time of their count squared.
const maxLineNesting = 64

// shallowLines escapes the marker that would open more than
// maxLineNesting quotes and list items on a line, so the rest of the line
// is text inside the deepest; it gives back md itself when no line does.
func shallowLines(md string) string {
	var out []byte
	done := 0 // how much of md is in out
	for start := 0; start < len(md); {
		end := strings.IndexByte(md[start:], '\n')
		if end < 0 {
			end = len(md)
		} else {
			end += start
		}
		if at := deepMarker(md[start:end]); at >= 0 {
			out = append(out, md[done:start+at]...)
			out = append(out, '\\')
			done = start + at
		}
		start = end + 1
	}
	if out == nil {
		return md
	}
	return string(append(out, md[done:]...))
}

// deepMarker is where in a line the marker opening one more than
// maxLineNesting quotes and list items is, at the escape that would
// make it text, or -1.
func deepMarker(line string) int {
	i, n := 0, 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	for i < len(line) {
		at := i
		var next int
		switch c := line[i]; {
		case c == '>':
			next = i + 1
		case c == '-' || c == '+' || c == '*':
			next = i + 1
			if next < len(line) && line[next] != ' ' && line[next] != '\t' {
				return -1
			}
		case c >= '0' && c <= '9':
			j := i
			for j < len(line) && j-i < 9 && line[j] >= '0' && line[j] <= '9' {
				j++
			}
			if j == len(line) || (line[j] != '.' && line[j] != ')') || (j+1 < len(line) && line[j+1] != ' ' && line[j+1] != '\t') {
				return -1
			}
			at, next = j, j+1
		default:
			return -1
		}
		if n == maxLineNesting {
			return at
		}
		n++
		for i = next; i < len(line) && (line[i] == ' ' || line[i] == '\t'); i++ {
		}
	}
	return -1
}

// Parse reads Markdown as a document.
func Parse(md string) *Node {
	src := []byte(shallowLines(md))
	root := reader.Parser().Parse(text.NewReader(src))
	doc := Doc()
	kids := children(root)
	doc.Content = readSeq(kids, src, true, true)
	if len(kids) > 0 {
		trailing := len(src) - len(bytes.TrimRight(src, "\n"))
		for range trailing / 2 {
			doc.Content = append(doc.Content, &Node{Type: "paragraph"})
		}
	}
	return doc
}

// children lists a node's blocks, but for link definitions, which say nothing.
func children(n ast.Node) []ast.Node {
	var out []ast.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.LinkReferenceDefinition); !ok {
			out = append(out, c)
		}
	}
	return out
}

// readSeq reads a run of sibling blocks, those written as markers among them
// (formatting.go). With blanks, blank lines beyond the usual one between
// blocks are empty paragraphs, one for each two, as the editor reads them;
// first says whether those before the first block count too.
func readSeq(kids []ast.Node, src []byte, blanks, first bool) []*Node {
	var out []*Node
	part := func(p []ast.Node) []*Node { return readSeq(p, src, blanks, false) }
	for i := 0; i < len(kids); {
		c := kids[i]
		if p := c.Pos(); blanks && p >= 0 {
			blank := blankLinesBefore(src, p)
			extra := 0
			switch {
			case i > 0:
				extra = max(0, (blank-1)/2)
			case first:
				extra = blank / 2
			}
			for range extra {
				out = append(out, &Node{Type: "paragraph"})
			}
		}
		if n, took := readFormatted(kids, i, src, part); n != nil {
			out = append(out, n)
			i += took
			continue
		}
		out = append(out, readBlock(c, src)...)
		i++
	}
	return out
}

// blankLinesBefore counts the blank lines just before the line holding pos.
func blankLinesBefore(src []byte, pos int) int {
	start := bytes.LastIndexByte(src[:pos], '\n') + 1
	n := 0
	for start > 0 {
		prevStart := bytes.LastIndexByte(src[:start-1], '\n') + 1
		if len(bytes.TrimSpace(src[prevStart:start-1])) != 0 {
			break
		}
		n++
		start = prevStart
	}
	return n
}

// readBlock reads a block as the editor does: as one node, none, or
// several (a list whose items are partly checklist items is split).
func readBlock(n ast.Node, src []byte) []*Node {
	if out := readOne(n, src); out != nil {
		return []*Node{out}
	}
	switch b := n.(type) {
	case *ast.List:
		return readList(b, src)
	case *extast.Table:
		return []*Node{readTable(b, src)}
	}
	return nil
}

func readOne(n ast.Node, src []byte) *Node {
	switch b := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		content := imageMarkers(readInline(n, src))
		if isBlankText(content) {
			content = nil
		}
		return &Node{Type: "paragraph", Content: content}
	case *ast.Heading:
		return &Node{Type: "heading", Attrs: map[string]any{"level": int64(b.Level)}, Content: readInline(n, src)}
	case *ast.ThematicBreak:
		return &Node{Type: "horizontalRule"}
	case *ast.FencedCodeBlock:
		node := codeBlock(b.Lines(), src)
		if lang := string(b.Language(src)); lang != "" {
			node.Attrs = map[string]any{"language": lang}
		}
		return node
	case *ast.CodeBlock:
		return codeBlock(b.Lines(), src)
	case *ast.Blockquote:
		if c := readCallout(b, src); c != nil {
			return c
		}
		return &Node{Type: "blockquote", Content: readBlocks(n, src)}
	case *ast.HTMLBlock:
		var s strings.Builder
		for i := 0; i < b.Lines().Len(); i++ {
			seg := b.Lines().At(i)
			s.Write(seg.Value(src))
		}
		if b.HasClosure() {
			s.Write(b.ClosureLine.Value(src))
		}
		t := strings.TrimRight(s.String(), "\n")
		if t == "" {
			return nil
		}
		return &Node{Type: "paragraph", Content: []*Node{{Type: "text", Text: t}}}
	case *ast.LinkReferenceDefinition:
		return nil
	}
	return nil
}

func readBlocks(n ast.Node, src []byte) []*Node {
	return readSeq(children(n), src, false, false)
}

// readList reads a list. A bullet list's checklist items make checklists
// of their own, each run of them apart from the plain items around it; a
// numbered list has no checklist items, so its boxes stay text.
func readList(b *ast.List, src []byte) []*Node {
	var out []*Node
	var cur *Node
	for item := b.FirstChild(); item != nil; item = item.NextSibling() {
		box := checkBox(item, src)
		typ, itemType := "bulletList", "listItem"
		switch {
		case b.IsOrdered():
			typ = "orderedList"
		case box != nil:
			typ, itemType = "taskList", "taskItem"
		}
		// The editor reads a blank line between checklist items as the
		// end of one checklist and the start of another.
		apart := typ == "taskList" && item.PreviousSibling() != nil && blankBefore(item, src)
		if cur == nil || cur.Type != typ || apart {
			cur = &Node{Type: typ}
			if b.IsOrdered() && b.Start != 1 {
				cur.Attrs = map[string]any{"start": int64(b.Start)}
			}
			out = append(out, cur)
		}
		content := readBlocks(item, src)
		if len(content) == 0 {
			content = []*Node{{Type: "paragraph"}}
		}
		it := &Node{Type: itemType, Content: content}
		raw := rawBox(item, src)
		switch {
		case itemType == "taskItem":
			it.Attrs = map[string]any{"checked": box.IsChecked}
		case raw != "":
			// A box the editor does not read as one (in a numbered list,
			// or with nothing after it) is text.
			text := raw
			if first := content[0]; first.Type == "paragraph" {
				first.Content = append([]*Node{{Type: "text", Text: text}}, first.Content...)
				first.Content = Normalize(&Node{Type: "paragraph", Content: first.Content}).Content
			}
		}
		cur.Content = append(cur.Content, it)
	}
	return out
}

// checkBox is a list item's checkbox, if it starts with one the editor
// reads as a box: followed by a space or the end of the line, not the end
// of the text.
func checkBox(item ast.Node, src []byte) *extast.TaskCheckBox {
	first := item.FirstChild()
	if first == nil {
		return nil
	}
	box, _ := first.FirstChild().(*extast.TaskCheckBox)
	if box == nil || first.Lines().Len() == 0 {
		return box
	}
	// Lines lose their trailing spaces: look at the text itself.
	at := first.Lines().At(0).Start + 3
	if at >= len(src) || (src[at] != ' ' && src[at] != '\t' && src[at] != '\n' && src[at] != '\r') {
		return nil
	}
	return box
}

// rawBox is the text of the checkbox goldmark read at the start of a list
// item, as it was written, or "" if there is none.
func rawBox(item ast.Node, src []byte) string {
	first := item.FirstChild()
	if first == nil || first.Lines().Len() == 0 {
		return ""
	}
	if _, ok := first.FirstChild().(*extast.TaskCheckBox); !ok {
		return ""
	}
	seg := first.Lines().At(0)
	line := seg.Value(src)
	end := 3
	for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		end++
	}
	if end == len(line) {
		// Nothing follows on the line: the box is all the item says.
		return string(line[:3])
	}
	return string(line[:end])
}

// blankBefore says whether a blank line comes just before a list item.
func blankBefore(item ast.Node, src []byte) bool {
	first := item.FirstChild()
	if first == nil || first.Lines().Len() == 0 {
		return false
	}
	return blankLinesBefore(src, first.Lines().At(0).Start) > 0
}

// readTable reads a table: its header row's cells are headers, and each
// cell holds a paragraph and its column's alignment.
func readTable(t *extast.Table, src []byte) *Node {
	table := &Node{Type: "table"}
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		kind := "tableCell"
		if _, ok := r.(*extast.TableHeader); ok {
			kind = "tableHeader"
		}
		row := &Node{Type: "tableRow"}
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cell := &Node{Type: kind, Content: []*Node{{Type: "paragraph", Content: readCellInline(c, src)}}}
			if tc, ok := c.(*extast.TableCell); ok {
				switch tc.Alignment {
				case extast.AlignLeft:
					cell.Attrs = map[string]any{"align": "left"}
				case extast.AlignCenter:
					cell.Attrs = map[string]any{"align": "center"}
				case extast.AlignRight:
					cell.Attrs = map[string]any{"align": "right"}
				}
			}
			row.Content = append(row.Content, cell)
		}
		table.Content = append(table.Content, row)
	}
	return table
}

var calloutLine = regexp.MustCompile(`^\[!((?i)note|tip|important|warning|caution)\][ \t]*$`)

// readCallout reads a quote whose first line is `[!NOTE]` (or another of
// GitHub's kinds) as a box, or returns nil.
func readCallout(q *ast.Blockquote, src []byte) *Node {
	para, ok := q.FirstChild().(*ast.Paragraph)
	if !ok || para.Lines().Len() == 0 {
		return nil
	}
	first := para.Lines().At(0)
	m := calloutLine.FindSubmatch(bytes.TrimRight(first.Value(src), "\r\n"))
	if m == nil {
		return nil
	}
	content := readBlocks(q, src)
	// The marker's line goes from the first paragraph, and the paragraph
	// with it when that was all it held.
	p := content[0]
	for len(p.Content) > 0 {
		c := p.Content[0]
		if c.Type == "hardBreak" {
			p.Content = p.Content[1:]
			break
		}
		if i := strings.IndexByte(c.Text, '\n'); c.IsText() && i >= 0 {
			c.Text = c.Text[i+1:]
			if c.Text == "" {
				p.Content = p.Content[1:]
			}
			break
		}
		p.Content = p.Content[1:]
	}
	if len(p.Content) == 0 {
		content = content[1:]
	} else if isBlankText(p.Content) {
		p.Content = nil
	}
	if len(content) == 0 {
		content = []*Node{{Type: "paragraph"}}
	}
	return &Node{Type: "callout", Attrs: map[string]any{"kind": strings.ToLower(string(m[1]))}, Content: content}
}

func codeBlock(lines *text.Segments, src []byte) *Node {
	var s strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		s.Write(seg.Value(src))
	}
	body := strings.TrimSuffix(s.String(), "\n")
	node := &Node{Type: "codeBlock"}
	if body != "" {
		node.Content = []*Node{{Type: "text", Text: body}}
	}
	return node
}

// isBlankText says whether a paragraph holds only a no-break space, which
// is how the editor writes an empty one.
func isBlankText(content []*Node) bool {
	if len(content) != 1 || !content[0].IsText() || len(content[0].Marks) > 0 {
		return false
	}
	return strings.Trim(content[0].Text, "  ") == "" && strings.Contains(content[0].Text, " ")
}

// htmlMarks are the inline HTML tags read as marks.
var htmlMarks = map[string]string{
	"u": "underline", "em": "italic", "i": "italic", "strong": "bold", "b": "bold",
	"s": "strike", "del": "strike", "strike": "strike", "code": "code",
}

var htmlTag = regexp.MustCompile(`^<(/?)([a-zA-Z]+)\s*/?>$`)

// readCellInline reads a table cell's text, where the editor reads HTML,
// even a line break, as text.
func readCellInline(n ast.Node, src []byte) []*Node {
	return readInlineIn(n, src, true)
}

func readInline(n ast.Node, src []byte) []*Node {
	return readInlineIn(n, src, false)
}

func readInlineIn(n ast.Node, src []byte, cell bool) []*Node {
	var out []*Node
	var marks []Mark
	// The last text node's text, gathered here rather than added to it piece
	// by piece, which would copy it again for every piece.
	var tail strings.Builder
	flush := func() {
		if last := len(out) - 1; last >= 0 && out[last].IsText() && tail.Len() > 0 {
			out[last].Text = tail.String()
		}
		tail.Reset()
	}
	add := func(s string) {
		if s == "" {
			return
		}
		ms := append([]Mark(nil), marks...)
		if last := len(out) - 1; last >= 0 && out[last].IsText() && sameMarks(normalMarks(out[last].Marks), normalMarks(ms)) {
			if tail.Len() == 0 {
				tail.WriteString(out[last].Text)
			}
			tail.WriteString(s)
			return
		}
		flush()
		out = append(out, &Node{Type: "text", Text: s, Marks: ms})
	}
	push := func(n *Node) {
		flush()
		out = append(out, n)
	}
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		closes := closingTags(n, src)
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				add(decode(t.Segment.Value(src)))
				switch {
				case t.HardLineBreak():
					push(&Node{Type: "hardBreak"})
				case t.SoftLineBreak():
					add("\n")
				}
			case *ast.String:
				add(string(t.Value))
			case *ast.CodeSpan:
				var s strings.Builder
				for k := t.FirstChild(); k != nil; k = k.NextSibling() {
					if tx, ok := k.(*ast.Text); ok {
						s.Write(tx.Segment.Value(src))
					}
				}
				marks = append(marks, Mark{Type: "code"})
				add(s.String())
				marks = marks[:len(marks)-1]
			case *ast.Emphasis:
				name := "italic"
				if t.Level == 2 {
					name = "bold"
				}
				marks = append(marks, Mark{Type: name})
				walk(t)
				marks = marks[:len(marks)-1]
			case *extast.Strikethrough:
				marks = append(marks, Mark{Type: "strike"})
				walk(t)
				marks = marks[:len(marks)-1]
			case *ast.Link:
				attrs := map[string]any{"href": string(t.Destination)}
				if len(t.Title) > 0 {
					attrs["title"] = string(t.Title)
				}
				marks = append(marks, Mark{Type: "link", Attrs: attrs})
				walk(t)
				marks = marks[:len(marks)-1]
			case *ast.AutoLink:
				url := string(t.URL(src))
				marks = append(marks, Mark{Type: "link", Attrs: map[string]any{"href": url}})
				add(string(t.Label(src)))
				marks = marks[:len(marks)-1]
			case *ast.Image:
				// Images take no marks, not even a link around them.
				attrs := map[string]any{"src": string(t.Destination), "alt": imageAltOf(t, src)}
				if len(t.Title) > 0 {
					attrs["title"] = string(t.Title)
				}
				push(&Node{Type: "image", Attrs: attrs})
			case *extast.TaskCheckBox:
				// Read by the list.
			case *ast.RawHTML:
				raw := rawText(t, src)
				// A highlight or a font and size, read only when closed later, as the editor does.
				if mark, closing, ok := readHTMLMark(raw); ok && !cell && closes.later(c, closing) {
					marks = append(marks, mark)
					continue
				}
				if want := htmlCloses[strings.ToLower(raw)]; want != "" && !cell {
					found := false
					for i := len(marks) - 1; i >= 0 && !found; i-- {
						if marks[i].Type == want {
							marks = append(marks[:i], marks[i+1:]...)
							found = true
						}
					}
					if found {
						continue
					}
				}
				m := htmlTag.FindStringSubmatch(raw)
				name := ""
				if m != nil {
					name = strings.ToLower(m[2])
				}
				switch {
				case cell:
					add(raw)
				case name == "br":
					push(&Node{Type: "hardBreak"})
				case htmlMarks[name] != "" && m[1] == "":
					marks = append(marks, Mark{Type: htmlMarks[name]})
				case htmlMarks[name] != "" && m[1] == "/":
					for i := len(marks) - 1; i >= 0; i-- {
						if marks[i].Type == htmlMarks[name] {
							marks = append(marks[:i], marks[i+1:]...)
							break
						}
					}
				default:
					add(raw)
				}
			default:
				walk(c)
			}
		}
	}
	walk(n)
	flush()
	return out
}

// decode undoes Markdown's escapes and the entities the editor reads: the
// few HTML needs, numeric references, and the no-break space.
func decode(b []byte) string {
	b = util.UnescapePunctuations(b)
	b = util.ResolveNumericReferences(b)
	return entities.Replace(string(b))
}

var entities = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&nbsp;", " ")
