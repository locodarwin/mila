package actions

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Text is what append-notes and replace-section take: a task's notes or a
// page's body, by the task's or page's ID (one of the two), and the text.
// Heading names the section replace-section replaces.
type Text struct {
	Task    *string `json:"task"`
	Page    *string `json:"page"`
	Heading string  `json:"heading"`
	Text    string  `json:"text"`
}

// Written is a task or page after its text changed.
type Written struct {
	Row   *syncer.Record
	Batch string
	Said  string
}

// AppendNotes adds text at the end of a task's notes or a page's body.
func (s *Service) AppendNotes(ctx context.Context, c Caller, in Text) (Written, error) {
	add := strings.TrimSpace(in.Text)
	if add == "" {
		return Written{}, &apperr.Error{Kind: apperr.Invalid, Field: "text", Message: "Give some text to add."}
	}
	return s.editText(ctx, c, in, func(old string) (string, string) {
		if strings.TrimSpace(old) == "" {
			return add + "\n", "Wrote it"
		}
		return strings.TrimRight(old, "\n") + "\n\n" + add + "\n", "Added it at the end"
	})
}

// ReplaceSection replaces what is under a heading, down to the next
// heading of the same or a higher level, or adds the section at the end
// when there is no such heading. The rest is left as it is, Mila's
// markers and all.
func (s *Service) ReplaceSection(ctx context.Context, c Caller, in Text) (Written, error) {
	heading := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(in.Heading), "#"))
	if heading == "" {
		return Written{}, &apperr.Error{Kind: apperr.Invalid, Field: "heading", Message: "Name the section's heading."}
	}
	body := strings.TrimSpace(in.Text)
	return s.editText(ctx, c, in, func(old string) (string, string) {
		return replaceSection(old, heading, body)
	})
}

// editText changes a task's notes or a page's body from what it holds now,
// written only if nobody changed it meanwhile, and tried again if someone
// did: a person typing in it loses nothing.
func (s *Service) editText(ctx context.Context, c Caller, in Text, change func(string) (string, string)) (Written, error) {
	entity, field, id := "task", "notes", ""
	switch {
	case given(in.Task) && given(in.Page):
		return Written{}, &apperr.Error{Kind: apperr.Invalid, Field: "page", Message: "Give a task or a page, not both."}
	case given(in.Task):
		id = strings.TrimSpace(*in.Task)
	case given(in.Page):
		entity, field, id = "page", "body", strings.TrimSpace(*in.Page)
	default:
		return Written{}, &apperr.Error{Kind: apperr.Invalid, Field: "task", Message: "Give a task's or a page's ID."}
	}
	r, err := s.start(ctx, c)
	if err != nil {
		return Written{}, err
	}
	for range 4 {
		rec, err := s.sync.Get(ctx, c.User, c.Workspace, entity, id)
		if err != nil {
			return Written{}, err
		}
		if !live(*rec) {
			return Written{}, &apperr.Error{Kind: apperr.Invalid, Field: entity, Message: "That " + entity + " is in the trash."}
		}
		old := str(*rec, field)
		next, said := change(old)
		if next == old {
			return Written{Row: rec, Batch: r.batch, Said: "It said that already"}, nil
		}
		out, err := r.write(syncer.Write{Entity: entity, ID: id, Mode: syncer.Update, IfMatch: syncer.ETag(rec.Clocks),
			Fields: map[string]json.RawMessage{field: raw(next)}})
		if apperr.KindOf(err) == apperr.PreconditionFailed {
			continue // someone changed it meanwhile: start again from theirs
		}
		if err != nil {
			return Written{}, err
		}
		return Written{Row: out, Batch: r.batch, Said: said}, nil
	}
	return Written{}, apperr.New(apperr.Conflict, "It kept changing while this was written; try again in a moment.")
}

// heading reads an ATX heading line: its level and its words.
func heading(line string) (int, string, bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return 0, "", false
	}
	level := len(t) - len(strings.TrimLeft(t, "#"))
	if level < 1 || level > 6 || (len(t) > level && t[level] != ' ' && t[level] != '\t') {
		return 0, "", false
	}
	words := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[level:]), "#"))
	return level, words, true
}

// fence says whether a line opens or closes a fenced code block, inside
// which nothing is a heading.
func fence(line string) bool {
	t := strings.TrimLeft(line, " ")
	return len(line)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"))
}

func replaceSection(md, title, body string) (string, string) {
	lines := strings.Split(md, "\n")
	start, end, level := -1, len(lines), 0
	inCode := false
	for i, line := range lines {
		if fence(line) {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		lvl, words, ok := heading(line)
		if !ok {
			continue
		}
		if start >= 0 && lvl <= level {
			end = i
			break
		}
		if start < 0 && strings.EqualFold(words, title) {
			start, level = i, lvl
		}
	}
	if start < 0 {
		section := "## " + title + "\n"
		if body != "" {
			section += "\n" + body + "\n"
		}
		if strings.TrimSpace(md) == "" {
			return section, "Added the section"
		}
		return strings.TrimRight(md, "\n") + "\n\n" + section, "Added the section at the end"
	}
	out := append([]string{}, lines[:start+1]...)
	if body != "" {
		out = append(out, "", body)
	}
	if end < len(lines) {
		out = append(out, "")
		out = append(out, lines[end:]...)
	} else {
		out = append(out, "")
	}
	return strings.Join(out, "\n"), "Replaced the section"
}
