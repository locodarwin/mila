package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The copier moves a whole instance between databases of either dialect:
// `mila migrate-db`, backups of a Postgres instance (into a SQLite file)
// and restoring one (from it). It reads the tables from the database
// itself rather than from a list, so a table added later is copied
// without anyone remembering to add it, and converts each value to what
// the destination column holds: SQLite keeps booleans as integers and
// times as text, Postgres has types of its own for both.

// gooseTable is the migration record, which each side keeps for itself.
const gooseTable = "goose_db_version"

// kind is what a column holds, whatever each dialect calls it.
type kind int

const (
	kText kind = iota
	kInt
	kBool
	kTime
	kBytes
)

func kindOf(declared string) kind {
	t := strings.ToUpper(declared)
	switch {
	case strings.Contains(t, "BOOL"):
		return kBool
	case strings.Contains(t, "INT"):
		return kInt
	case strings.Contains(t, "TIMESTAMP"):
		return kTime
	case strings.Contains(t, "BLOB"), strings.Contains(t, "BYTEA"):
		return kBytes
	}
	return kText
}

type column struct {
	name string
	kind kind
}

// table is one table, its columns in order and the references it makes.
type table struct {
	name string
	cols []column
	// refs maps a column to the table it references, and to its column.
	refs map[string]string
	to   map[string]string
	// self maps a column that references this same table (a subtask's
	// parent) to the column it references.
	self map[string]string
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// tables lists the tables of the database q reads, every table after the
// ones it references, so inserting in this order never breaks a foreign
// key.
func (d *DB) tables(ctx context.Context, q querier) ([]table, error) {
	var listSQL, colsSQL, refsSQL string
	if d.Dialect == Postgres {
		listSQL = `SELECT table_name FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`
		colsSQL = `SELECT column_name, data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`
		refsSQL = `SELECT r.relname, a.attname, af.attname
			FROM pg_constraint k
			JOIN pg_class c ON c.oid = k.conrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			JOIN pg_class r ON r.oid = k.confrelid
			JOIN pg_attribute a ON a.attrelid = k.conrelid AND a.attnum = k.conkey[1]
			JOIN pg_attribute af ON af.attrelid = k.confrelid AND af.attnum = k.confkey[1]
			WHERE k.contype = 'f' AND n.nspname = current_schema() AND c.relname = $1`
	} else {
		listSQL = `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
		colsSQL = `SELECT name, type FROM pragma_table_info(?) ORDER BY cid`
		refsSQL = `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`
	}
	names, err := strings2(ctx, q, listSQL)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	byName := map[string]*table{}
	for _, n := range names {
		if n == gooseTable {
			continue
		}
		t := &table{name: n, refs: map[string]string{}, to: map[string]string{}, self: map[string]string{}}
		cols, err := strings2(ctx, q, colsSQL, n)
		if err != nil {
			return nil, fmt.Errorf("columns of %s: %w", n, err)
		}
		for i := 0; i+1 < len(cols); i += 2 {
			t.cols = append(t.cols, column{name: cols[i], kind: kindOf(cols[i+1])})
		}
		refs, err := strings3(ctx, q, refsSQL, n)
		if err != nil {
			return nil, fmt.Errorf("references of %s: %w", n, err)
		}
		for _, r := range refs {
			if r[0] == n {
				t.self[r[1]] = r[2]
			} else {
				t.refs[r[1]], t.to[r[1]] = r[0], r[2]
			}
		}
		byName[n] = t
	}
	// Topological order; names are sorted, so the order is stable.
	var out []table
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(n string) error
	visit = func(n string) error {
		switch state[n] {
		case 1:
			return fmt.Errorf("tables reference each other in a loop through %s", n)
		case 2:
			return nil
		}
		state[n] = 1
		t := byName[n]
		deps := make([]string, 0, len(t.refs))
		for _, to := range t.refs {
			deps = append(deps, to)
		}
		slices.Sort(deps)
		for _, to := range deps {
			if byName[to] == nil {
				return fmt.Errorf("%s references %s, which is not there", n, to)
			}
			if err := visit(to); err != nil {
				return err
			}
		}
		state[n] = 2
		out = append(out, *t)
		return nil
	}
	for _, n := range names {
		if byName[n] != nil {
			if err := visit(n); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func strings2(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []string
	for rows.Next() {
		vals := make([]string, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, vals...)
	}
	return out, rows.Err()
}

func strings3(ctx context.Context, q querier, query string, args ...any) ([][3]string, error) {
	flat, err := strings2(ctx, q, query, args...)
	if err != nil {
		return nil, err
	}
	out := make([][3]string, 0, len(flat)/3)
	for i := 0; i+2 < len(flat); i += 3 {
		out = append(out, [3]string{flat[i], flat[i+1], flat[i+2]})
	}
	return out, nil
}

// quote quotes an identifier the same way for both dialects.
func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

// ErrNotEmpty means the destination of a copy already holds data.
var ErrNotEmpty = errors.New("the destination database is not empty")

// CopyOptions says how Copy treats the destination.
type CopyOptions struct {
	// Replace deletes everything in the destination first, in the same
	// transaction as the copy. Without it, a destination with any row in
	// any table is refused with ErrNotEmpty.
	Replace bool
}

// Copy copies every row of every table from src to dst, which must be at
// the same schema version. src is read in one transaction, a consistent
// snapshot even while a Postgres server keeps writing; dst is written in
// one transaction, so a failed copy leaves it as it was. It returns how
// many rows it copied.
func Copy(ctx context.Context, src, dst *DB, opts CopyOptions) (int64, error) {
	sv, _, err := src.SchemaVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("source: %w", err)
	}
	dv, _, err := dst.SchemaVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("destination: %w", err)
	}
	if sv != dv {
		return 0, fmt.Errorf("the source is at schema %d and the destination at %d; migrate both to the same version first", sv, dv)
	}
	readOpts := &sql.TxOptions{ReadOnly: true}
	if src.Dialect == Postgres {
		readOpts.Isolation = sql.LevelRepeatableRead
	}
	in, err := src.BeginTx(ctx, readOpts)
	if err != nil {
		return 0, err
	}
	defer func() { _ = in.Rollback() }()
	out, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = out.Rollback() }()

	from, err := src.tables(ctx, in)
	if err != nil {
		return 0, fmt.Errorf("source: %w", err)
	}
	to, err := dst.tables(ctx, out)
	if err != nil {
		return 0, fmt.Errorf("destination: %w", err)
	}
	dstTables := map[string]table{}
	for _, t := range to {
		dstTables[t.name] = t
	}
	if len(from) != len(to) {
		return 0, fmt.Errorf("the source has %d tables and the destination %d", len(from), len(to))
	}
	for _, t := range from {
		dt, ok := dstTables[t.name]
		if !ok {
			return 0, fmt.Errorf("table %s is missing from the destination", t.name)
		}
		if !sameColumns(t, dt) {
			return 0, fmt.Errorf("table %s has different columns in the two databases", t.name)
		}
	}

	if opts.Replace {
		for _, t := range slices.Backward(to) {
			if _, err := out.ExecContext(ctx, "DELETE FROM "+quote(t.name)); err != nil {
				return 0, fmt.Errorf("empty %s: %w", t.name, err)
			}
		}
	} else if err := empty(ctx, out, to); err != nil {
		return 0, err
	}

	var total int64
	for _, t := range from {
		n, err := copyTable(ctx, in, out, dst.Dialect, dstTables[t.name])
		if err != nil {
			return 0, fmt.Errorf("copy %s: %w", t.name, err)
		}
		total += n
	}
	if err := out.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// empty fails with ErrNotEmpty if any of the tables has a row.
func empty(ctx context.Context, q rowQuerier, tables []table) error {
	for _, t := range tables {
		var one int
		err := q.QueryRowContext(ctx, "SELECT 1 FROM "+quote(t.name)+" LIMIT 1").Scan(&one)
		if err == nil {
			return fmt.Errorf("%w (table %s has rows)", ErrNotEmpty, t.name)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// CheckEmpty fails with ErrNotEmpty if any table but the migration
// record has a row: a database a copy may be written into.
func (d *DB) CheckEmpty(ctx context.Context) error {
	tables, err := d.tables(ctx, d.DB)
	if err != nil {
		return err
	}
	return empty(ctx, d.DB, tables)
}

func sameColumns(a, b table) bool {
	if len(a.cols) != len(b.cols) {
		return false
	}
	names := map[string]bool{}
	for _, c := range a.cols {
		names[c.name] = true
	}
	for _, c := range b.cols {
		if !names[c.name] {
			return false
		}
	}
	return true
}

// copyTable copies one table, with t as the destination describes it.
// Rows that reference their own table are written parents first.
func copyTable(ctx context.Context, in, out *sql.Tx, dialect Dialect, t table) (int64, error) {
	names := make([]string, len(t.cols))
	marks := make([]string, len(t.cols))
	for i, c := range t.cols {
		names[i] = quote(c.name)
		marks[i] = "?"
		if dialect == Postgres {
			marks[i] = "$" + strconv.Itoa(i+1)
		}
	}
	rows, err := in.QueryContext(ctx, "SELECT "+strings.Join(names, ", ")+" FROM "+quote(t.name))
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	insert, err := out.PrepareContext(ctx, "INSERT INTO "+quote(t.name)+" ("+strings.Join(names, ", ")+") VALUES ("+strings.Join(marks, ", ")+")")
	if err != nil {
		return 0, err
	}
	defer func() { _ = insert.Close() }()

	var held [][]any
	var n int64
	for rows.Next() {
		vals := make([]any, len(t.cols))
		ptrs := make([]any, len(t.cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return 0, err
		}
		for i, c := range t.cols {
			v, err := convert(vals[i], c.kind)
			if err != nil {
				return 0, fmt.Errorf("column %s: %w", c.name, err)
			}
			vals[i] = v
		}
		if len(t.self) > 0 {
			held = append(held, vals)
			continue
		}
		if _, err := insert.ExecContext(ctx, vals...); err != nil {
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(held) == 0 {
		return n, nil
	}
	ordered, err := parentsFirst(t, held)
	if err != nil {
		return 0, err
	}
	for _, vals := range ordered {
		if _, err := insert.ExecContext(ctx, vals...); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

// parentsFirst orders the rows of a table that references itself so that
// every row comes after the row it names.
func parentsFirst(t table, rows [][]any) ([][]any, error) {
	col := map[string]int{}
	for i, c := range t.cols {
		col[c.name] = i
	}
	type link struct{ from, to int }
	var links []link
	for from, to := range t.self {
		links = append(links, link{col[from], col[to]})
	}
	// Index rows by every column another row may name.
	index := map[int]map[any]int{}
	for _, l := range links {
		if index[l.to] == nil {
			index[l.to] = map[any]int{}
			for i, r := range rows {
				index[l.to][key(r[l.to])] = i
			}
		}
	}
	out := make([][]any, 0, len(rows))
	state := make([]int, len(rows)) // 1 visiting, 2 written
	var visit func(i int) error
	visit = func(i int) error {
		switch state[i] {
		case 1:
			return fmt.Errorf("rows of %s name each other in a loop", t.name)
		case 2:
			return nil
		}
		state[i] = 1
		for _, l := range links {
			if v := rows[i][l.from]; v != nil {
				if p, ok := index[l.to][key(v)]; ok && p != i {
					if err := visit(p); err != nil {
						return err
					}
				}
			}
		}
		state[i] = 2
		out = append(out, rows[i])
		return nil
	}
	for i := range rows {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func key(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// timeLayouts are the ways a time may be written as text in SQLite.
var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
}

// convert turns a value read from either database into what a column of
// kind k takes, in UTC for times.
func convert(v any, k kind) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch k {
	case kBool:
		switch x := v.(type) {
		case bool:
			return x, nil
		case int64:
			return x != 0, nil
		case string:
			return strconv.ParseBool(x)
		}
	case kInt:
		switch x := v.(type) {
		case int64:
			return x, nil
		case bool:
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		case float64:
			return int64(x), nil
		case string:
			return strconv.ParseInt(x, 10, 64)
		}
	case kTime:
		switch x := v.(type) {
		case time.Time:
			return x.UTC().Truncate(time.Microsecond), nil
		case string:
			for _, layout := range timeLayouts {
				if t, err := time.Parse(layout, x); err == nil {
					return t.UTC().Truncate(time.Microsecond), nil
				}
			}
			return nil, fmt.Errorf("%q is not a time", x)
		}
	case kBytes:
		switch x := v.(type) {
		case []byte:
			return x, nil
		case string:
			return []byte(x), nil
		}
	case kText:
		switch x := v.(type) {
		case string:
			return x, nil
		case []byte:
			return string(x), nil
		}
	}
	return nil, fmt.Errorf("cannot store a %T here", v)
}
