package db_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// fill puts two rows in every table of a SQLite database, made up from
// the table's own description so that a table added later is filled too:
// every column set in the first row, every nullable one NULL in the
// second, and references pointing at real rows. A row that references its
// own table is written before the row it names, so a copy that keeps the
// order it reads in breaks the foreign key on Postgres.
func fill(t *testing.T, d *db.DB) {
	t.Helper()
	ctx := context.Background()
	tables, err := d.Tables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range tables {
		nullable := map[string]bool{}
		rows, err := d.QueryContext(ctx, `SELECT name, "notnull", pk FROM pragma_table_info(?)`, tb.Name)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var name string
			var notNull, pk int
			if err := rows.Scan(&name, &notNull, &pk); err != nil {
				t.Fatal(err)
			}
			nullable[name] = notNull == 0 && pk == 0
		}
		_ = rows.Close()

		var self []string
		values := func(r int) []any {
			vals := make([]any, len(tb.Columns))
			for i, c := range tb.Columns {
				ref, isRef := tb.Refs[c]
				switch {
				case isRef && ref.Table == tb.Name:
					self = append(self, c)
					vals[i] = nil // set below, once the row it names exists
				case isRef:
					vals[i] = value(ref.Table, ref.Column, "", r)
				case r == 2 && nullable[c]:
					vals[i] = nil
				default:
					vals[i] = value(tb.Name, c, tb.Kinds[c], r)
				}
			}
			return vals
		}
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(tb.Columns)), ", ")
		insert := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)", tb.Name, `"`+strings.Join(tb.Columns, `", "`)+`"`, marks)
		for r := 1; r <= 2; r++ {
			if _, err := d.ExecContext(ctx, insert, values(r)...); err != nil {
				t.Fatalf("fill %s: %v", tb.Name, err)
			}
		}
		for _, c := range self {
			ref := tb.Refs[c]
			if _, err := d.ExecContext(ctx, fmt.Sprintf("UPDATE %q SET %q = ? WHERE %q = ?", tb.Name, c, ref.Column),
				value(tb.Name, ref.Column, tb.Kinds[ref.Column], 2), value(tb.Name, ref.Column, tb.Kinds[ref.Column], 1)); err != nil {
				t.Fatalf("fill %s.%s: %v", tb.Name, c, err)
			}
		}
	}
}

// value is the made-up value of a column in row r. A referenced column is
// always text here (every key is), so references agree with their rows.
func value(table, col, kind string, r int) any {
	switch kind {
	case "int":
		return int64(r * 7)
	case "bool":
		return r == 1
	case "time":
		return time.Date(2026, 3, 8, r, 30, 0, 123456000, time.UTC)
	case "bytes":
		return []byte{byte(r), 0, 255, 'x'}
	}
	return fmt.Sprintf("%s.%s.%d", table, col, r)
}

// dump reads every table as comparable text, rows sorted.
func dump(t *testing.T, d *db.DB) map[string][]string {
	t.Helper()
	ctx := context.Background()
	tables, err := d.Tables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, tb := range tables {
		cols := append([]string{}, tb.Columns...)
		sort.Strings(cols)
		rows, err := d.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %q", `"`+strings.Join(cols, `", "`)+`"`, tb.Name))
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			for i, c := range cols {
				fmt.Fprintf(&b, "%s=%s ", c, canonical(vals[i], tb.Kinds[c]))
			}
			lines = append(lines, b.String())
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		sort.Strings(lines)
		out[tb.Name] = lines
	}
	return out
}

func canonical(v any, kind string) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case []byte:
		if kind == "bytes" {
			return fmt.Sprintf("%x", x)
		}
		return string(x)
	case int64:
		if kind == "bool" {
			return strconv.FormatBool(x != 0)
		}
		return strconv.FormatInt(x, 10)
	case string:
		if kind == "time" {
			for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano} {
				if tm, err := time.Parse(layout, x); err == nil {
					return tm.UTC().Format(time.RFC3339Nano)
				}
			}
		}
		return x
	}
	return fmt.Sprint(v)
}

// TestCopyRoundTrip copies a full SQLite instance into each database and
// back into a new SQLite file, and expects every row of every table to
// come through unchanged: types converted both ways, times to the
// microsecond, subtasks before their parents fixed up.
func TestCopyRoundTrip(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, mid *db.DB) {
		ctx := context.Background()
		src := dbtest.Open(t)
		fill(t, src)
		want := dump(t, src)
		for name, rows := range want {
			if len(rows) != 2 {
				t.Fatalf("%s has %d rows after filling, want 2", name, len(rows))
			}
		}
		n, err := db.Copy(ctx, src, mid, db.CopyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(2*len(want)) {
			t.Errorf("copied %d rows, want %d", n, 2*len(want))
		}
		if got := dump(t, mid); !reflect.DeepEqual(got, want) {
			diff(t, "copy", want, got)
		}
		back := dbtest.Open(t)
		if _, err := db.Copy(ctx, mid, back, db.CopyOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := dump(t, back); !reflect.DeepEqual(got, want) {
			diff(t, "copy back", want, got)
		}

		// A destination with data is refused, unless it is to be replaced.
		if _, err := db.Copy(ctx, src, back, db.CopyOptions{}); !errors.Is(err, db.ErrNotEmpty) {
			t.Errorf("copy into a full database: %v, want ErrNotEmpty", err)
		}
		if _, err := src.ExecContext(ctx, "DELETE FROM instance_setting"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Copy(ctx, src, mid, db.CopyOptions{Replace: true}); err != nil {
			t.Fatal(err)
		}
		if got := dump(t, mid)["instance_setting"]; len(got) != 0 {
			t.Errorf("replace kept %d rows the source no longer has", len(got))
		}
	})
}

func diff(t *testing.T, what string, want, got map[string][]string) {
	t.Helper()
	for name, rows := range want {
		if !reflect.DeepEqual(rows, got[name]) {
			t.Errorf("%s: table %s\n got %q\nwant %q", what, name, got[name], rows)
		}
	}
}

func TestCopyRefusesDifferentSchemas(t *testing.T) {
	ctx := context.Background()
	src := dbtest.Open(t)
	dst, err := db.Open(ctx, "sqlite://"+t.TempDir()+"/old.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := dst.MigrateTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(ctx, src, dst, db.CopyOptions{}); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("copy between schema versions: %v", err)
	}
}
