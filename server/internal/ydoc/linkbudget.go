package ydoc

import (
	"bytes"
	"sort"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The Markdown reader tries each "](" as a link, looking for its address
// along the rest of the line and for its title's end along the rest of the
// paragraph. A paragraph of thousands that are no links (a line of "[a]("
// over and over) takes time of their count squared: minutes for a page.
// So past linkTries failed tries in a document, a link's address and title
// are looked for in the next linkReach bytes of their line only. Nobody
// writes that many "](" that are not links, nor an address that long.
const (
	linkTries = 64
	linkReach = 1024
)

var failedLinks = parser.NewContextKey()

// boundedLinks is goldmark's link reader, held to the limits above.
type boundedLinks struct{ parser.InlineParser }

func (b boundedLinks) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) == 0 || line[0] != ']' {
		return b.InlineParser.Parse(parent, block, pc)
	}
	// Each "]" reads the text since its "[" back, to look it up as a
	// reference; goldmark finds the line it starts on from the last line.
	r := block
	if lines := parent.Lines(); lines != nil && lines.Len() > 0 {
		r = lineReader{block, lines}
	}
	if len(line) < 2 || line[1] != '(' {
		return b.InlineParser.Parse(parent, r, pc)
	}
	failed, _ := pc.Get(failedLinks).(int)
	if failed >= linkTries {
		r = nearReader{r}
	}
	n := b.InlineParser.Parse(parent, r, pc)
	if n == nil {
		pc.Set(failedLinks, failed+1)
	}
	return n
}

// lineReader reads text from a block's lines as goldmark's own reader does,
// finding the line a piece starts on by halves rather than from the end,
// which for each "]" of a long paragraph took time of its length.
type lineReader struct {
	text.Reader
	lines *text.Segments
}

func (r lineReader) Value(seg text.Segment) []byte {
	n := r.lines.Len()
	line := sort.Search(n, func(k int) bool { return r.lines.At(k).Start > seg.Start }) - 1
	if line < 0 {
		return r.Reader.Value(seg)
	}
	src := r.Source()
	out := make([]byte, 0, seg.Stop-seg.Start+1)
	i := seg.Start
	for ; line < n; line++ {
		s := r.lines.At(line)
		if i < 0 {
			i = s.Start
		}
		out = s.ConcatPadding(out)
		for ; i < seg.Stop && i < s.Stop; i++ {
			out = append(out, src[i])
		}
		i = -1
		if s.Stop > seg.Stop {
			break
		}
	}
	return out
}

func (b boundedLinks) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	b.InlineParser.(parser.CloseBlocker).CloseBlock(parent, block, pc)
}

// nearReader shows only the next linkReach bytes of the line, and finds a
// closing character only there.
type nearReader struct{ text.Reader }

func (r nearReader) PeekLine() ([]byte, text.Segment) {
	line, seg := r.Reader.PeekLine()
	if len(line) > linkReach {
		line, seg = line[:linkReach], seg.WithStop(seg.Start+linkReach)
	}
	return line, seg
}

func (r nearReader) FindClosure(opener, closer byte, opts text.FindClosureOptions) (*text.Segments, bool) {
	line, seg := r.PeekLine()
	if bytes.IndexByte(line, closer) < 0 {
		return nil, false
	}
	open := 1
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && i < len(line)-1 && util.IsPunct(line[i+1]):
			i++
		case c == closer:
			if open--; open == 0 {
				segs := text.NewSegments()
				segs.Append(seg.WithStop(seg.Start + i - seg.Padding))
				r.Advance(i + 1)
				return segs, true
			}
		case c == opener:
			if !opts.Nesting {
				return nil, false
			}
			open++
		}
	}
	return nil, false
}

// refLines is how many of a paragraph's first lines are read for link
// reference definitions ("[a]: /address"). goldmark goes over the rest of
// the paragraph again for each one, so a paragraph of thousands took
// time of their count squared; past these lines they are its text.
const refLines = 1000

// boundedRefs is goldmark's reader of link reference definitions, given
// only a paragraph's first refLines lines.
type boundedRefs struct{ parser.ParagraphTransformer }

func (b boundedRefs) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	lines := node.Lines()
	if lines.Len() <= refLines {
		b.ParagraphTransformer.Transform(node, reader, pc)
		return
	}
	// The first lines as a paragraph of their own: the definitions read
	// from it go before it, and it is removed if they were all it held.
	head := ast.NewParagraph()
	head.SetBlankPreviousLines(node.HasBlankPreviousLines())
	first := text.NewSegments()
	first.AppendAll(lines.Sliced(0, refLines))
	head.SetLines(first)
	parent := node.Parent()
	parent.InsertBefore(parent, node, head)
	b.ParagraphTransformer.Transform(head, reader, pc)
	kept := text.NewSegments()
	if head.Parent() != nil {
		kept.AppendAll(head.Lines().Sliced(0, head.Lines().Len()))
		parent.RemoveChild(parent, head)
	}
	kept.AppendAll(lines.Sliced(refLines, lines.Len()))
	node.SetLines(kept)
}

// paragraphTransformers are goldmark's, with its reference reader bounded.
func paragraphTransformers() []util.PrioritizedValue {
	ps := parser.DefaultParagraphTransformers()
	for i, p := range ps {
		if p.Value == parser.LinkReferenceParagraphTransformer {
			ps[i].Value = boundedRefs{p.Value.(parser.ParagraphTransformer)}
		}
	}
	return ps
}

// inlineParsers are goldmark's, with its link reader bounded.
func inlineParsers() []util.PrioritizedValue {
	ps := parser.DefaultInlineParsers()
	for i, p := range ps {
		if p.Value == parser.NewLinkParser() {
			ps[i].Value = boundedLinks{p.Value.(parser.InlineParser)}
		}
	}
	return ps
}
