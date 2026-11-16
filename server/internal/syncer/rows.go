package syncer

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
)

// row is one synced row as the merge sees it: field values by name, and
// the HLC that last set each field.
type row struct {
	id          string
	workspaceID string
	values      map[string]any // string, bool, int64, time.Time or nil
	clocks      map[string]string
	exists      bool
}

// rebind turns ? placeholders into $1, $2... for Postgres.
func rebind(dialect db.Dialect, q string) string {
	if dialect != db.Postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func columns(e *entity) []string {
	cols := []string{"id", "workspace_id", "clocks"}
	for _, f := range e.fields {
		cols = append(cols, f.name)
	}
	return cols
}

// restColumns is columns(e) and then the row's created_at and updated_at,
// which REST shows and sync does not.
func restColumns(e *entity) []string {
	return append(columns(e), "created_at", "updated_at")
}

// scanRow reads one row in columns(e) order, and then into extra (the
// timestamps of restColumns) when given.
func scanRow(e *entity, scan func(dest ...any) error, extra ...any) (*row, error) {
	var id, ws, clocks string
	dest := []any{&id, &ws, &clocks}
	holders := make([]any, len(e.fields))
	for i, f := range e.fields {
		switch f.kind {
		case kTime:
			holders[i] = new(sql.NullTime)
		case kBool:
			holders[i] = new(sql.NullBool)
		case kInt:
			holders[i] = new(sql.NullInt64)
		default:
			holders[i] = new(sql.NullString)
		}
		dest = append(dest, holders[i])
	}
	dest = append(dest, extra...)
	if err := scan(dest...); err != nil {
		return nil, err
	}
	r := &row{id: id, workspaceID: ws, values: map[string]any{}, clocks: map[string]string{}, exists: true}
	if err := json.Unmarshal([]byte(clocks), &r.clocks); err != nil {
		return nil, err
	}
	for i, f := range e.fields {
		switch h := holders[i].(type) {
		case *sql.NullTime:
			if h.Valid {
				r.values[f.name] = h.Time.UTC()
			} else {
				r.values[f.name] = nil
			}
		case *sql.NullString:
			if h.Valid && f.kind == kJSON {
				r.values[f.name] = jsonText(h.String)
			} else if h.Valid {
				r.values[f.name] = h.String
			} else {
				r.values[f.name] = nil
			}
		case *sql.NullBool:
			if h.Valid {
				r.values[f.name] = h.Bool
			} else {
				r.values[f.name] = nil
			}
		case *sql.NullInt64:
			if h.Valid {
				r.values[f.name] = h.Int64
			} else {
				r.values[f.name] = nil
			}
		}
	}
	return r, nil
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// loadRow reads a row by ID, or returns an empty row that does not exist.
func loadRow(ctx context.Context, q querier, dialect db.Dialect, e *entity, id string) (*row, error) {
	query := rebind(dialect, "SELECT "+strings.Join(columns(e), ", ")+" FROM "+e.table+" WHERE id = ?")
	r, err := scanRow(e, q.QueryRowContext(ctx, query, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return &row{id: id, values: map[string]any{}, clocks: map[string]string{}}, nil
	}
	return r, err
}

// loadRowLocked is loadRow for a REST write: on Postgres it also locks
// the row until the transaction ends, so a precondition checked against
// it still holds when the write lands. SQLite's write transactions are
// serialized already (db.Open's _txlock=immediate).
func loadRowLocked(ctx context.Context, q querier, dialect db.Dialect, e *entity, id string) (*row, error) {
	if dialect != db.Postgres {
		return loadRow(ctx, q, dialect, e, id)
	}
	query := rebind(dialect, "SELECT "+strings.Join(columns(e), ", ")+" FROM "+e.table+" WHERE id = ? FOR UPDATE")
	r, err := scanRow(e, q.QueryRowContext(ctx, query, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return &row{id: id, values: map[string]any{}, clocks: map[string]string{}}, nil
	}
	return r, err
}

// listRows reads every live row of an entity in a workspace.
func listRows(ctx context.Context, q querier, dialect db.Dialect, e *entity, workspaceID string) ([]*row, error) {
	return queryRows(ctx, q, dialect, e, " WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY id", workspaceID)
}

// listAll reads every row of an entity in a workspace, deleted ones the
// trash still holds included.
func listAll(ctx context.Context, q querier, dialect db.Dialect, e *entity, workspaceID string) ([]*row, error) {
	return queryRows(ctx, q, dialect, e, " WHERE workspace_id = ? ORDER BY id", workspaceID)
}

// queryRows reads the rows of an entity a WHERE clause picks.
func queryRows(ctx context.Context, q querier, dialect db.Dialect, e *entity, where string, args ...any) ([]*row, error) {
	query := rebind(dialect, "SELECT "+strings.Join(columns(e), ", ")+" FROM "+e.table+where)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*row
	for rows.Next() {
		r, err := scanRow(e, rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// saveRow writes the given fields of r, inserting the row if it is new.
func saveRow(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row, changed []string, now time.Time) error {
	clocks, err := json.Marshal(r.clocks)
	if err != nil {
		return err
	}
	if !r.exists {
		cols := []string{"id", "workspace_id", "clocks", "created_at", "updated_at"}
		args := []any{r.id, r.workspaceID, string(clocks), now, now}
		for _, name := range changed {
			cols = append(cols, name)
			args = append(args, r.values[name])
		}
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
		query := rebind(dialect, "INSERT INTO "+e.table+" ("+strings.Join(cols, ", ")+") VALUES ("+marks+")")
		if _, err := q.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		return savedLinks(ctx, q, dialect, e, r, changed, true)
	}
	sets := []string{"clocks = ?", "updated_at = ?"}
	args := []any{string(clocks), now}
	for _, name := range changed {
		sets = append(sets, name+" = ?")
		args = append(args, r.values[name])
	}
	args = append(args, r.id)
	query := rebind(dialect, "UPDATE "+e.table+" SET "+strings.Join(sets, ", ")+" WHERE id = ?")
	if _, err := q.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return savedLinks(ctx, q, dialect, e, r, changed, false)
}

// savedLinks keeps doc_links in step when a row's notes or body were
// among what saveRow wrote.
func savedLinks(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row, changed []string, fresh bool) error {
	k, ok := kindOfTarget(e.name)
	if !ok || !slices.Contains(changed, k.field) {
		return nil
	}
	md, _ := r.values[k.field].(string)
	return indexLinks(ctx, q, dialect, k, r.workspaceID, r.id, md, fresh)
}

// jsonValue turns a stored value into what goes over the wire.
// jsonText is a kJSON field's value: stored as text, sent as the JSON it
// holds, so clients get back exactly the value they wrote.
type jsonText string

func (j jsonText) MarshalJSON() ([]byte, error) { return []byte(j), nil }

// Value stores it as text in both dialects.
func (j jsonText) Value() (driver.Value, error) { return string(j), nil }

func jsonValue(v any) any {
	if t, ok := v.(time.Time); ok {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return v
}

// snapshot is a row as clients receive it.
func (r *row) snapshot(e *entity) Row {
	fields := make(map[string]any, len(r.values))
	for k, v := range r.values {
		fields[k] = jsonValue(v)
	}
	return Row{Entity: e.name, ID: r.id, Fields: fields, Clocks: r.clocks}
}
