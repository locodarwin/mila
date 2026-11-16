package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Kept is a change of a batch that undoing it left as it is, and why.
type Kept struct {
	Entity string   `json:"entity"`
	ID     string   `json:"id"`
	Fields []string `json:"fields,omitempty"`
	Why    string   `json:"why"`
}

// PutBack is a row undoing a batch put back: deleted, if the batch made
// it, or its fields as they were.
type PutBack struct {
	Entity  string `json:"entity"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted,omitempty"`
}

// Undone is what undoing a batch did: the rows it put back, and the
// changes it left.
type Undone struct {
	Undone  int       `json:"undone"`
	PutBack []PutBack `json:"put_back"`
	Kept    []Kept    `json:"kept"`
}

// UndoBatch puts back what a batch the caller wrote through the API changed,
// newest first, as the app's undo puts back a change to many tasks. The rows
// it made go to the trash, and each field it changed goes back to what it
// was, unless that field has been changed since (by anyone): that one is
// left, and said. Each putting back is the caller's own write, through the
// same checks as any other; one refused is left, and said. Undoing it again
// finds every field changed since, by the undo, and changes nothing.
func (s *Service) UndoBatch(ctx context.Context, userID, workspaceID, batch string) (Undone, error) {
	out := Undone{Kept: []Kept{}, PutBack: []PutBack{}}
	if !ValidBatch(batch) {
		return out, &apperr.Error{Kind: apperr.Invalid, Field: "batch", Message: "A batch is named by 1 to 64 letters, digits and dashes."}
	}
	if _, err := s.viewer(ctx, s.st, userID, workspaceID); err != nil {
		return out, err
	}
	changes, err := s.st.ListBatchChanges(ctx, sqlitedb.ListBatchChangesParams{WorkspaceID: workspaceID, UserID: userID, Batch: batch})
	if err != nil {
		return out, err
	}
	if len(changes) == 0 {
		return out, apperr.New(apperr.NotFound, "No batch of yours called %q in this workspace in the last %d days.", batch, TrashDays)
	}
	// A row the batch changed more than once is put back once: each field
	// to what it held before the batch's first change of it, if nobody has
	// changed it since the batch's last.
	type field struct {
		was json.RawMessage
		hlc string
	}
	type touched struct {
		entity, id string
		made       bool
		fields     map[string]field
	}
	var order []*touched
	byRow := map[string]*touched{}
	for _, c := range changes { // newest first
		key := c.Entity + "/" + c.EntityID
		t := byRow[key]
		if t == nil {
			t = &touched{entity: c.Entity, id: c.EntityID, fields: map[string]field{}}
			byRow[key] = t
			order = append(order, t)
		}
		t.made = t.made || c.Made
		var before map[string]json.RawMessage
		if err := json.Unmarshal([]byte(c.Before), &before); err != nil {
			return out, err
		}
		for name, was := range before {
			// What only the server sets stays as it is: a task that started
			// has started, whatever is undone.
			if e := entities[c.Entity]; e != nil {
				if ef := e.field(name); ef != nil && ef.serverSet {
					continue
				}
			}
			f, seen := t.fields[name]
			if !seen {
				f.hlc = c.Hlc // the batch's last change of it
			}
			f.was = was // older changes overwrite: the first one's before stays
			t.fields[name] = f
		}
	}
	d := s.st.DB()
	for _, t := range order {
		e := entities[t.entity]
		if e == nil {
			continue
		}
		recs, err := queryRecords(ctx, d, d.Dialect, e, " WHERE id = ?", t.id)
		if err != nil {
			return out, err
		}
		if len(recs) == 0 {
			out.Kept = append(out.Kept, Kept{Entity: e.name, ID: t.id, Why: "It is no longer there."})
			continue
		}
		now := recs[0].out(e)
		var w Write
		if t.made {
			if now.Fields["deleted_at"] != nil {
				continue // in the trash already
			}
			w = Write{Entity: e.name, ID: t.id, Mode: Delete}
		} else {
			fields := map[string]json.RawMessage{}
			var since []string
			for name, f := range t.fields {
				if now.Clocks[name] == f.hlc {
					fields[name] = f.was
				} else {
					since = append(since, name)
				}
			}
			if len(since) > 0 {
				slices.Sort(since)
				out.Kept = append(out.Kept, Kept{Entity: e.name, ID: t.id, Fields: since, Why: "Changed again since."})
			}
			if len(fields) == 0 {
				continue
			}
			w = Write{Entity: e.name, ID: t.id, Mode: Update, Fields: fields}
		}
		if _, _, err := s.Write(ctx, userID, workspaceID, w); err != nil {
			var refused *apperr.Error
			if !errors.As(err, &refused) {
				return out, err
			}
			out.Kept = append(out.Kept, Kept{Entity: e.name, ID: t.id, Why: refused.Message})
			continue
		}
		out.Undone++
		out.PutBack = append(out.PutBack, PutBack{Entity: e.name, ID: t.id, Deleted: t.made})
	}
	return out, nil
}
