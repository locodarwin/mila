package ydoc

import "slices"

// RenameLinks gives the links a document holds to something renamed its
// new name: a link whose whole text is the old name, and whose address
// match accepts, takes the new name, keeping the marks that covered all
// of it. A link whose text says something else was worded on purpose,
// and stays. It returns the new document and whether anything changed;
// the one given is left as it was.
func RenameLinks(doc *Node, match func(href string) bool, oldName, newName string) (*Node, bool) {
	if doc == nil || oldName == "" || oldName == newName {
		return doc, false
	}
	out := doc.Clone()
	changed := renameIn(out, match, oldName, newName)
	return out, changed
}

// Hrefs lists the addresses of the links a document holds, each once, in
// the order they first appear.
func Hrefs(doc *Node) []string {
	var out []string
	var walk func(n *Node)
	walk = func(n *Node) {
		if href := linkOf(n); href != "" && !slices.Contains(out, href) {
			out = append(out, href)
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	if doc != nil {
		walk(doc)
	}
	return out
}

func renameIn(n *Node, match func(string) bool, oldName, newName string) bool {
	changed := false
	var content []*Node
	for i := 0; i < len(n.Content); {
		c := n.Content[i]
		href := linkOf(c)
		if href == "" || !match(href) {
			if renameIn(c, match, oldName, newName) {
				changed = true
			}
			content = append(content, c)
			i++
			continue
		}
		// The run of text under the same link.
		j, text := i, ""
		for j < len(n.Content) && linkOf(n.Content[j]) == href {
			text += n.Content[j].Text
			j++
		}
		if text == oldName {
			content = append(content, &Node{Type: "text", Text: newName, Marks: sharedMarks(n.Content[i:j])})
			changed = true
		} else {
			content = append(content, n.Content[i:j]...)
		}
		i = j
	}
	n.Content = content
	return changed
}

// sharedMarks are the marks every node of a run has: the link, and any
// formatting that covers the whole of its text.
func sharedMarks(run []*Node) []Mark {
	var out []Mark
	for _, m := range run[0].Marks {
		all := true
		for _, n := range run[1:] {
			if !hasMark(n.Marks, m) {
				all = false
				break
			}
		}
		if all {
			out = append(out, m)
		}
	}
	return out
}

func hasMark(marks []Mark, m Mark) bool {
	for _, x := range marks {
		if sameMarks([]Mark{x}, []Mark{m}) {
			return true
		}
	}
	return false
}

// linkOf is the address of the link a text node is under, or "".
func linkOf(n *Node) string {
	if !n.IsText() {
		return ""
	}
	for _, m := range n.Marks {
		if m.Type == "link" {
			href, _ := m.Attrs["href"].(string)
			return href
		}
	}
	return ""
}
