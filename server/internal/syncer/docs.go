package syncer

// Live documents. A task's notes and a wiki
// page's body are Yjs documents: devices push their edits as rows
// (doc_update, page_update), and the server keeps the notes or body
// column as the document rendered to Markdown, so search, filters, export,
// the API, email and calendar feeds go on reading plain text. A write to
// that column that is not a Yjs update (REST, the CSV import, an app from
// before live documents) becomes one here.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/ydoc"
)

const (
	// sessionGap: one person's edits to the notes this close together are
	// one editing session, and one entry in the task's history.
	sessionGap = 5 * time.Minute
	// compactAt is how many rows a document has before the server merges
	// them, once the small ones add up to more than the largest: a merged
	// row is the whole document, so merging is held back until it has
	// been earned, and the log grows by about twice what was typed.
	compactAt = 64
)

// docKind is one kind of live document: the entity of its rows, the field
// naming what they belong to, and the column its Markdown is kept in.
type docKind struct {
	entity, table, parentField string
	target, field              string
}

var docKinds = []docKind{
	{entity: "doc_update", table: "doc_updates", parentField: "task_id", target: "task", field: "notes"},
	{entity: "page_update", table: "page_updates", parentField: "page_id", target: "page", field: "body"},
}

func kindOfRows(entity string) (docKind, bool) {
	for _, k := range docKinds {
		if k.entity == entity {
			return k, true
		}
	}
	return docKind{}, false
}

func kindOfTarget(entity string) (docKind, bool) {
	for _, k := range docKinds {
		if k.target == entity {
			return k, true
		}
	}
	return docKind{}, false
}

// validUpdate checks a doc_update's data: base64 of a Yjs update the
// server can read.
func validUpdate(s string) error {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) == 0 {
		return errors.New("must be a Yjs update as base64")
	}
	if err := ydoc.Check(b); err != nil {
		return errors.New("is not a Yjs update the server can read")
	}
	return nil
}

// afterApply follows an accepted op with what live documents need: a new
// row of a document renders its notes, and a write to the notes from
// anywhere but a document becomes a row of it.
//
// It also keeps a page's history, and renames the links to a page or task
// whose title changed, or to a project's main page when the project's
// name did. previous is the row as it was, news the accepted
// fields whose value changed.
func (s *Service) afterApply(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, e *entity, r *row, accepted, news []string, previous map[string]any, created bool, op Op, now time.Time) error {
	if e.name == "page" && slices.Contains(news, "body") {
		body, _ := r.values["body"].(string)
		before, _ := previous["body"].(string)
		if !created || body != "" {
			if err := recordVersion(ctx, q, workspaceID, r.id, v.userID, before, body, op.derived, now); err != nil {
				return err
			}
		}
	}
	if (e.name == "page" || e.name == "task") && slices.Contains(news, "title") && !created && !op.derived {
		old, _ := previous["title"].(string)
		title, _ := r.values["title"].(string)
		scope, err := projectOf(ctx, tx, s.st.DB().Dialect, e, r)
		if err != nil {
			return err
		}
		if err := s.renameLinks(ctx, q, tx, v, workspaceID, r.id, scope, old, title, now); err != nil {
			return err
		}
	}
	// A project's wiki's main page is called by the project's name.
	if e.name == "project" && slices.Contains(news, "name") && !created && !op.derived {
		old, _ := previous["name"].(string)
		name, _ := r.values["name"].(string)
		if err := s.renameLinks(ctx, q, tx, v, workspaceID, MainPageID(r.id), r.id, old, name, now); err != nil {
			return err
		}
	}
	if op.derived {
		return nil
	}
	if k, ok := kindOfRows(e.name); ok && created && r.values["deleted_at"] == nil {
		id, _ := r.values[k.parentField].(string)
		return s.renderDoc(ctx, q, tx, v, workspaceID, k, id, now)
	}
	if k, ok := kindOfTarget(e.name); ok && slices.Contains(accepted, k.field) {
		md, _ := r.values[k.field].(string)
		return s.markdownWritten(ctx, q, tx, v, workspaceID, k, r.id, md, now)
	}
	return nil
}

// docRow is one of a document's rows.
type docRow struct {
	id     string
	update []byte
}

func loadDoc(ctx context.Context, tx *sql.Tx, dialect db.Dialect, k docKind, id string) ([]docRow, error) {
	rows, err := tx.QueryContext(ctx, rebind(dialect, "SELECT id, data FROM "+k.table+" WHERE "+k.parentField+" = ? AND deleted_at IS NULL ORDER BY created_at, id"), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []docRow
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", k.entity, id, err)
		}
		out = append(out, docRow{id, b})
	}
	return out, rows.Err()
}

func updatesOf(rows []docRow) [][]byte {
	out := make([][]byte, len(rows))
	for i, r := range rows {
		out[i] = r.update
	}
	return out
}

// derivedOp applies an op of the server's own for whoever made the change
// it follows, in the same transaction: it is logged, merged and checked
// like any other, and attributed to them.
func (s *Service) derivedOp(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID, entityName, id, hlc string, fields map[string]any, now time.Time) error {
	raw := map[string]json.RawMessage{}
	for k, val := range fields {
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		raw[k] = b
	}
	sv := *v
	sv.server = true
	op := Op{OpID: uuid.Must(uuid.NewV7()).String(), Entity: entityName, ID: id, HLC: hlc, Fields: raw, trusted: true, derived: true}
	res := Result{OpID: op.OpID, Status: "applied"}
	var changed, created bool
	return s.applyIn(ctx, q, tx, &sv, workspaceID, op, now, &res, &changed, &created)
}

// renderDoc writes a document to its column (a task's notes, a page's
// body) as Markdown, when that differs from what the column holds, and
// merges the document's rows once there are enough of them.
func (s *Service) renderDoc(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, k docKind, id string, now time.Time) error {
	d := s.st.DB()
	rows, err := loadDoc(ctx, tx, d.Dialect, k, id)
	if err != nil || len(rows) == 0 {
		return err
	}
	doc, err := ydoc.Read(updatesOf(rows)...)
	if err != nil {
		return err
	}
	md := ydoc.Markdown(doc)
	target, err := loadRow(ctx, tx, d.Dialect, entities[k.target], id)
	if err != nil {
		return err
	}
	if target.exists && target.values[k.field] != md {
		hlc := stampAfter(now, target.clocks, []string{k.field})
		if err := s.derivedOp(ctx, q, tx, v, workspaceID, k.target, id, hlc, map[string]any{k.field: md}, now); err != nil {
			return err
		}
	}
	return s.compactDoc(ctx, q, tx, v, workspaceID, k, id, rows, now)
}

// compactDoc merges a document's rows into one and deletes the rest, once
// there are compactAt of them and the small ones outweigh the largest. A
// device that pulls the deletes drops the old rows; the merged row
// changes nothing it already holds. The old rows leave the database at
// once: their deletes are in the log, and nothing restores them.
func (s *Service) compactDoc(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, k docKind, parentID string, rows []docRow, now time.Time) error {
	if len(rows) < compactAt {
		return nil
	}
	largest, total := 0, 0
	for _, r := range rows {
		largest = max(largest, len(r.update))
		total += len(r.update)
	}
	if total-largest < largest {
		return nil
	}
	merged, err := ydoc.Compact(updatesOf(rows)...)
	if err != nil {
		return err
	}
	id := uuid.Must(uuid.NewV7()).String()
	hlc := serverHLC(now)
	if err := s.derivedOp(ctx, q, tx, v, workspaceID, k.entity, id, hlc, map[string]any{
		k.parentField: parentID, "data": base64.StdEncoding.EncodeToString(merged), "deleted_at": nil,
	}, now); err != nil {
		return err
	}
	d := s.st.DB()
	for _, r := range rows {
		// After the row's own clock, which a device may have set a little
		// ahead of the server's.
		old, err := loadRow(ctx, tx, d.Dialect, entities[k.entity], r.id)
		if err != nil {
			return err
		}
		stamp := stampAfter(now, old.clocks, []string{"deleted_at"})
		if err := s.derivedOp(ctx, q, tx, v, workspaceID, k.entity, r.id, stamp, map[string]any{"deleted_at": now}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, rebind(d.Dialect, "DELETE FROM "+k.table+" WHERE id = ?"), r.id); err != nil {
			return err
		}
	}
	return nil
}

// freshClient is a Yjs client ID for one write of the server's: no device
// shares it, so the server's edits never collide with a device's.
func freshClient() uint64 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	// Yjs makes client IDs of 32 bits; any but zero will do.
	return uint64(binary.LittleEndian.Uint32(b[:])) | 1
}

// markdownWritten makes a write of Markdown to a document's column (a
// task's notes, a page's body) a row of its document: the update that
// replaces only the blocks that differ, so an edit a device has not sent
// yet survives unless it was in a changed block. The column keeps what
// was written.
func (s *Service) markdownWritten(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, k docKind, id, md string, now time.Time) error {
	d := s.st.DB()
	rows, err := loadDoc(ctx, tx, d.Dialect, k, id)
	if err != nil {
		return err
	}
	update, err := ydoc.Replace(updatesOf(rows), ydoc.Parse(md), freshClient())
	if err != nil || update == nil {
		return err
	}
	return s.derivedOp(ctx, q, tx, v, workspaceID, k.entity, uuid.Must(uuid.NewV7()).String(), serverHLC(now), map[string]any{
		k.parentField: id, "data": base64.StdEncoding.EncodeToString(update), "deleted_at": nil,
	}, now)
}

// sessionActivity records a change to a task's notes made through its
// document. The same person's edits within sessionGap of their last
// change to the notes update that entry rather than adding one, so the
// history keeps one version of the notes per editing session.
func sessionActivity(ctx context.Context, q store.Queries, workspaceID, taskID, actor string, data []byte, now time.Time) (bool, error) {
	last, err := q.LatestTaskActivity(ctx, sqlitedb.LatestTaskActivityParams{WorkspaceID: workspaceID, TaskID: taskID})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if last.ActorID != actor || last.Entity != "task" || last.Kind != "changed" || now.Sub(last.CreatedAt) > sessionGap {
		return false, nil
	}
	var fields map[string]any
	if json.Unmarshal([]byte(last.Data), &fields) != nil || len(fields) != 1 || fields["notes"] == nil {
		return false, nil
	}
	return true, q.UpdateActivityData(ctx, sqlitedb.UpdateActivityDataParams{ID: last.ID, Data: string(data), Now: now})
}

// SeedDocs gives every task whose notes are not empty, and every page
// whose body is not, that has no document its first document, made from
// the Markdown: text written before live documents, and what an import
// brings in (an export holds the Markdown, not the documents). It runs at
// start, for every workspace (workspaceID ""), and after a restore, for
// the one restored. A device holding such text shows it read-only until
// the document arrives, so two devices never make it twice.
func (s *Service) SeedDocs(ctx context.Context, workspaceID string) (int, error) {
	seeded := 0
	for _, k := range docKinds {
		n, err := s.seedKind(ctx, k, workspaceID)
		seeded += n
		if err != nil {
			return seeded, err
		}
	}
	return seeded, nil
}

func (s *Service) seedKind(ctx context.Context, k docKind, workspaceID string) (int, error) {
	d := s.st.DB()
	// The server acts for whoever made the task; a page records no maker.
	creator := "''"
	if k.target == "task" {
		creator = "COALESCE(t.created_by, '')"
	}
	query := `SELECT t.id, t.workspace_id, t.` + k.field + `, ` + creator + ` FROM ` + entities[k.target].table + ` t
		WHERE t.` + k.field + ` <> '' AND NOT EXISTS (SELECT 1 FROM ` + k.table + ` u WHERE u.` + k.parentField + ` = t.id AND u.deleted_at IS NULL)`
	args := []any{}
	if workspaceID != "" {
		query += " AND t.workspace_id = ?"
		args = append(args, workspaceID)
	}
	type pending struct{ id, ws, text, creator string }
	var todo []pending
	rows, err := d.QueryContext(ctx, rebind(d.Dialect, query+" ORDER BY t.workspace_id, t.id"), args...)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.ws, &p.text, &p.creator); err != nil {
			_ = rows.Close()
			return 0, err
		}
		todo = append(todo, p)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	seeded := 0
	for start := 0; start < len(todo); start += restoreChunk {
		batch := todo[start:min(start+restoreChunk, len(todo))]
		err := s.st.InTxConn(ctx, func(q store.Queries, tx *sql.Tx) error {
			for _, p := range batch {
				// A rename may have made it since the list was read, from
				// the same text: writing that again would undo the rename.
				if made, err := loadDoc(ctx, tx, d.Dialect, k, p.id); err != nil {
					return err
				} else if len(made) > 0 {
					continue
				}
				v := &viewer{userID: p.creator, role: authz.Owner, server: true}
				if err := s.markdownWritten(ctx, q, tx, v, p.ws, k, p.id, p.text, s.now()); err != nil {
					return fmt.Errorf("%s %s: %w", k.target, p.id, err)
				}
			}
			return nil
		})
		if err != nil {
			return seeded, err
		}
		seeded += len(batch)
		for _, ws := range distinctWorkspaces(batch, func(p pending) string { return p.ws }) {
			s.poke(ctx, ws)
		}
	}
	return seeded, nil
}

func distinctWorkspaces[T any](items []T, ws func(T) string) []string {
	var out []string
	for _, it := range items {
		if w := ws(it); !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// poke tells a workspace's open devices that it changed.
func (s *Service) poke(ctx context.Context, workspaceID string) {
	if s.notify == nil {
		return
	}
	if seq, err := s.st.GetChangeSeq(ctx, workspaceID); err == nil {
		s.notify.Changed(workspaceID, seq)
	}
}
