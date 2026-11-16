package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"

	"github.com/locodarwin/mila/server/internal/ids"
)

// PairTable is a table of rows that each stand for a pair, and the two
// columns the row's ID is made from.
type PairTable struct {
	Entity, Table, A, B string
}

// PairTables are syncer's pairs, as tables (TestPairTablesAreTheSyncers
// holds the two together).
var PairTables = []PairTable{
	{"task_label", "task_labels", "task_id", "label_id"},
	{"task_assignee", "task_assignees", "task_id", "user_id"},
	{"task_dependency", "task_dependencies", "task_id", "blocker_id"},
	{"my_day_entry", "my_day_entries", "task_id", "user_id"},
	{"custom_field_value", "custom_field_values", "task_id", "field_id"},
}

// pairIDsVersion moves every pair's row from the ID it was given as the
// two IDs XORed to ids.Pair, a hash of the two, which nobody can choose a
// pair of their own to make (00037_pair_ids.sql). What it was called goes
// in pair_ids, for edits on their way from before; the change log and
// task history follow it; and the sync epoch is cleared, so every device
// bootstraps a copy with the new IDs. A row whose ID was never its pair's
// XOR (from before such rows were checked) is left as it is.
const pairIDsVersion = 38

func pairIDs(dialect Dialect) *goose.Migration {
	q := func(s string) string {
		if dialect != Postgres {
			return s
		}
		var b strings.Builder
		n := 0
		for _, c := range s {
			if c == '?' {
				n++
				b.WriteString("$" + strconv.Itoa(n))
				continue
			}
			b.WriteRune(c)
		}
		return b.String()
	}
	entities := make([]string, len(PairTables))
	for i, t := range PairTables {
		entities[i] = "'" + t.Entity + "'"
	}
	inPairs := strings.Join(entities, ", ")
	follow := func(ctx context.Context, tx *sql.Tx, from, to string) error {
		for _, t := range PairTables {
			if _, err := tx.ExecContext(ctx, q("UPDATE "+t.Table+" SET id = (SELECT "+to+" FROM pair_ids WHERE entity = ? AND "+from+" = "+t.Table+".id) WHERE id IN (SELECT "+from+" FROM pair_ids WHERE entity = ?)"), t.Entity, t.Entity); err != nil {
				return err
			}
		}
		for _, table := range []string{"change_log", "activity"} {
			if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET entity_id = (SELECT p."+to+" FROM pair_ids p WHERE p.entity = "+table+".entity AND p."+from+" = "+table+".entity_id) WHERE entity IN ("+inPairs+") AND EXISTS (SELECT 1 FROM pair_ids p WHERE p.entity = "+table+".entity AND p."+from+" = "+table+".entity_id)"); err != nil {
				return err
			}
		}
		return nil
	}
	up := func(ctx context.Context, tx *sql.Tx) error {
		// pair_ids as 00037_pair_ids.sql makes it now: its first version, which
		// some databases have, named old IDs without their entity, and one
		// pair can have rows of two (an assignee, a My Day entry). No database
		// got past this migration with that version, so it is empty.
		for _, s := range []string{"DROP TABLE pair_ids", "CREATE TABLE pair_ids (entity TEXT NOT NULL, old_id TEXT NOT NULL, new_id TEXT NOT NULL, PRIMARY KEY (entity, old_id))"} {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		insert, err := tx.PrepareContext(ctx, q("INSERT INTO pair_ids (entity, old_id, new_id) VALUES (?, ?, ?)"))
		if err != nil {
			return err
		}
		defer func() { _ = insert.Close() }()
		moved := 0
		for _, t := range PairTables {
			rows, err := tx.QueryContext(ctx, "SELECT id, "+t.A+", "+t.B+" FROM "+t.Table)
			if err != nil {
				return err
			}
			var pairs [][2]string
			for rows.Next() {
				var id, a, b string
				if err := rows.Scan(&id, &a, &b); err != nil {
					_ = rows.Close()
					return err
				}
				if id == ids.XOR(a, b) {
					pairs = append(pairs, [2]string{id, ids.Pair(a, b)})
				}
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, p := range pairs {
				if _, err := insert.ExecContext(ctx, t.Entity, p[0], p[1]); err != nil {
					return err
				}
			}
			moved += len(pairs)
		}
		if moved == 0 {
			return nil
		}
		if err := follow(ctx, tx, "old_id", "new_id"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, q("DELETE FROM instance_setting WHERE key = ?"), "sync_epoch")
		return err
	}
	down := func(ctx context.Context, tx *sql.Tx) error {
		if err := follow(ctx, tx, "new_id", "old_id"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM pair_ids"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, q("DELETE FROM instance_setting WHERE key = ?"), "sync_epoch")
		return err
	}
	return goose.NewGoMigration(pairIDsVersion, &goose.GoFunc{RunTx: up}, &goose.GoFunc{RunTx: down})
}
