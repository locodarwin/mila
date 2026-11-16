package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/store"
)

// Dump is what one member may see of a workspace, for an export: the same
// rows a bootstrap sends them, plus the deleted ones the trash still
// holds, and the history of the tasks among them.
type Dump struct {
	// Role is the member's role, which decides what else an export may
	// tell (the member list is not every role's to see).
	Role authz.Role
	// Seq is the change number the rows reflect.
	Seq  int64
	Rows []Row
	// Activity is the tasks' history, oldest first.
	Activity []HistoryEntry
	// Versions are the pages' history, oldest first.
	Versions []PageVersionEntry
	// People are the users the rows and history name, each once.
	People []string
	// Hidden are the people among them this member may not name.
	Hidden map[string]bool
}

// PageVersionEntry is one version of a page's body, naming its page.
type PageVersionEntry struct {
	PageVersion
	PageID string `json:"pageId"`
}

// HistoryEntry is one entry in a task's history, naming its task.
type HistoryEntry struct {
	Event
	TaskID string `json:"taskId"`
}

// Dump returns everything a member may see of a workspace. Like Bootstrap
// it reads outside a transaction, so a long export never holds up a push;
// a change made meanwhile may leave a row whose parent is missing, which
// a restore drops.
func (s *Service) Dump(ctx context.Context, userID, workspaceID string) (*Dump, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	seq, err := s.st.GetChangeSeq(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := &Dump{Role: v.role, Seq: seq, Rows: []Row{}, Activity: []HistoryEntry{}, Versions: []PageVersionEntry{}, People: []string{}}
	people := map[string]bool{}
	person := func(id string) {
		if id != "" && !people[id] {
			people[id] = true
			out.People = append(out.People, id)
		}
	}
	if out.Hidden, err = s.hiddenFrom(ctx, v, workspaceID); err != nil {
		return nil, err
	}
	tasks, pages := map[string]bool{}, map[string]bool{}
	err = s.visible(ctx, v, workspaceID, syncOrder, true, func(e *entity, r *row) {
		// A live document's rows are the editor's own record: an export
		// holds its Markdown (a task's notes, a page's body), from which
		// an import makes the document again.
		if e.internal {
			return
		}
		switch e.name {
		case "task":
			tasks[r.id] = true
		case "page":
			pages[r.id] = true
		}
		for _, f := range e.fields {
			if id, ok := r.values[f.name].(string); ok && (f.kind == kSelf || f.kind == kMember) {
				person(id)
			}
		}
		out.Rows = append(out.Rows, r.snapshot(e))
	})
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	rows, err := d.QueryContext(ctx, rebind(d.Dialect, `SELECT a.id, a.task_id, a.actor_id, COALESCE(u.name, ''), a.entity, a.entity_id, a.kind, a.data, a.created_at
		FROM activity a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.workspace_id = ? ORDER BY a.created_at, a.id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var h HistoryEntry
		var data string
		if err := rows.Scan(&h.ID, &h.TaskID, &h.ActorID, &h.ActorName, &h.Entity, &h.EntityID, &h.Kind, &data, &h.At); err != nil {
			return nil, err
		}
		if !tasks[h.TaskID] {
			continue
		}
		if err := json.Unmarshal([]byte(data), &h.Fields); err != nil {
			return nil, err
		}
		scrub(h.Entity, h.Fields)
		if out.Hidden[h.ActorID] {
			delete(h.Fields, viaField)
		} else if via, ok := h.Fields[viaField].(string); ok {
			// An automation's name is for members, as the history shows it.
			h.Fields[viaField] = viaFor(v, via)
		}
		h.ActorName = nameFor(out.Hidden, h.ActorID, h.ActorName)
		h.At = h.At.UTC()
		person(h.ActorID)
		out.Activity = append(out.Activity, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	versions, err := d.QueryContext(ctx, rebind(d.Dialect, `SELECT v.id, v.page_id, COALESCE(v.actor_id, ''), COALESCE(u.name, ''), v.body, v.updated_at
		FROM page_versions v LEFT JOIN users u ON u.id = v.actor_id
		WHERE v.workspace_id = ? ORDER BY v.created_at, v.id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = versions.Close() }()
	for versions.Next() {
		var pv PageVersionEntry
		if err := versions.Scan(&pv.ID, &pv.PageID, &pv.ActorID, &pv.ActorName, &pv.Body, &pv.At); err != nil {
			return nil, err
		}
		if !pages[pv.PageID] {
			continue
		}
		pv.ActorName = nameFor(out.Hidden, pv.ActorID, pv.ActorName)
		pv.At = pv.At.UTC()
		person(pv.ActorID)
		out.Versions = append(out.Versions, pv)
	}
	return out, versions.Err()
}

// PairID is the ID of a row that stands for a pair (a label on a task, a
// person assigned to it, a task in someone's day, a task's value for a
// field, a dependency): a hash of the two IDs in order (ids.Pair), as
// web/src/lib/sync/entities.ts makes it, so the same pair always makes
// the same row and nobody can choose two IDs of their own that make
// another's. It is "" when either is empty.
func PairID(a, b string) string { return ids.Pair(a, b) }

// LegacyPairID is how pair IDs were made before PairID (the two UUIDs
// XORed), for exports and edits from then.
func LegacyPairID(a, b string) string { return ids.XOR(a, b) }

var pairs = map[string][2]string{
	"task_label":         {"task_id", "label_id"},
	"task_assignee":      {"task_id", "user_id"},
	"task_dependency":    {"task_id", "blocker_id"},
	"my_day_entry":       {"task_id", "user_id"},
	"custom_field_value": {"task_id", "field_id"},
}

// Forget takes someone who is leaving Mila out of a workspace they leave
// behind: their assignments end through the merge, as any edit would, so
// every member's devices drop them; and the rows only they saw (their My
// Day, their reminders, their personal views) are removed, with the log
// entries that carried them. What they wrote for others stays.
func (s *Service) Forget(ctx context.Context, workspaceID, userID string) error {
	d := s.st.DB()
	e := entities["task_assignee"]
	ids, err := queryIDs(ctx, d, rebind(d.Dialect,
		"SELECT id FROM task_assignees WHERE workspace_id = ? AND user_id = ? AND deleted_at IS NULL ORDER BY id"), workspaceID, userID)
	if err != nil {
		return err
	}
	// The server acts for them: an owner's view, so nothing is refused.
	v := &viewer{userID: userID, role: authz.Owner, server: true}
	changed := false
	for _, id := range ids {
		now := s.now()
		raw, _ := json.Marshal(now.Format(time.RFC3339Nano))
		op := Op{OpID: uuid.NewString(), Entity: e.name, ID: id, HLC: serverHLC(now),
			Fields: map[string]json.RawMessage{"deleted_at": raw}, trusted: true}
		res, applied, err := s.apply(ctx, v, workspaceID, op)
		if err != nil {
			return err
		}
		if res.Status == "rejected" {
			s.log.Warn("assignment not ended", "row", id, "reason", res.Message)
		}
		changed = changed || applied
	}
	err = s.st.InTxConn(ctx, func(_ store.Queries, tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM my_day_entries WHERE workspace_id = ? AND user_id = ?",
			"DELETE FROM reminders WHERE workspace_id = ? AND user_id = ?",
			"DELETE FROM saved_views WHERE workspace_id = ? AND owner_id = ?",
		} {
			if _, err := tx.ExecContext(ctx, rebind(d.Dialect, q), workspaceID, userID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, rebind(d.Dialect, "DELETE FROM change_log WHERE workspace_id = ? AND project_id = ?"),
			workspaceID, privateScope(userID))
		return err
	})
	if err != nil {
		return err
	}
	if changed && s.notify != nil {
		if seq, err := s.st.GetChangeSeq(ctx, workspaceID); err == nil {
			s.notify.Changed(workspaceID, seq)
		}
	}
	return nil
}
