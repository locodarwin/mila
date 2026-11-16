package syncer_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Migration 38 moves every pair's row from the XOR of its two IDs to their
// hash, with the change log and task history; makes every device fetch a
// fresh copy; and still takes an edit, or a REST read, that names the old ID.
func TestPairRowsMoveToTheirHash(t *testing.T) {
	dbtest.EachURL(t, func(t *testing.T, url string) {
		ctx := context.Background()
		d, err := db.Open(ctx, url)
		must(t, err)
		defer func() { _ = d.Close() }()
		_, err = d.MigrateTo(ctx, 37)
		must(t, err)
		exec0 := func(query string) {
			t.Helper()
			if _, err := d.ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
		}
		// As staging had it: the first version of 00037, before old IDs were
		// named with their entity, which migration 38 makes anew.
		exec0("DROP TABLE pair_ids")
		exec0("CREATE TABLE pair_ids (old_id TEXT PRIMARY KEY, new_id TEXT NOT NULL)")
		// Today's sync writes the rows below, and a task now has columns
		// from migration 42; they go again before migrating, which adds them.
		stamp := "TIMESTAMP"
		if d.Dialect == db.Postgres {
			stamp = "TIMESTAMPTZ"
		}
		exec0("ALTER TABLE tasks ADD COLUMN made_at " + stamp)
		exec0("ALTER TABLE tasks ADD COLUMN started_at " + stamp)
		exec := func(query string, args ...any) {
			t.Helper()
			if d.Dialect == db.Postgres {
				for i, n := 0, 1; i < len(query); i++ {
					if query[i] == '?' {
						query = query[:i] + "$" + string(rune('0'+n)) + query[i+1:]
						n++
					}
				}
			}
			if _, err := d.ExecContext(ctx, query, args...); err != nil {
				t.Fatal(err)
			}
		}

		w := newWorld(t, d)
		epoch, err := w.svc.Epoch(ctx)
		must(t, err)
		project, task, label, other := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		member := w.users["member"]
		w.push("owner",
			op("project", project, later(0), newProject("Garden")),
			op("task", task, later(1), newTask(project, nil, "Paint")),
			op("label", label, later(1), map[string]any{"name": "Urgent", "color": "red", "sort_key": "a0", "deleted_at": nil}),
			op("label", other, later(1), map[string]any{"name": "Later", "color": "blue", "sort_key": "a1", "deleted_at": nil}),
			op("task_label", syncer.PairID(task, label), later(2), map[string]any{"task_id": task, "label_id": label, "deleted_at": nil}),
			op("task_label", syncer.PairID(task, other), later(2), map[string]any{"task_id": task, "label_id": other, "deleted_at": nil}),
			op("task_assignee", syncer.PairID(task, member), later(2), map[string]any{"task_id": task, "user_id": member, "deleted_at": nil}),
		)
		// The same task in the member's My Day: the same pair, so the same
		// IDs, in another table.
		w.push("member", op("my_day_entry", syncer.PairID(task, member), later(2), map[string]any{"task_id": task, "user_id": member, "sort_key": "a0", "day": "2026-11-05", "deleted_at": nil}))
		// As an install from before held them: pair rows under the XOR of
		// their IDs, and one from before rows were held to their pair.
		odd := uuid.NewString()
		for _, p := range []struct{ entity, table, id, was string }{
			{"task_label", "task_labels", syncer.PairID(task, label), syncer.LegacyPairID(task, label)},
			{"task_label", "task_labels", syncer.PairID(task, other), odd},
			{"task_assignee", "task_assignees", syncer.PairID(task, member), syncer.LegacyPairID(task, member)},
			{"my_day_entry", "my_day_entries", syncer.PairID(task, member), syncer.LegacyPairID(task, member)},
		} {
			exec("UPDATE "+p.table+" SET id = ? WHERE id = ?", p.was, p.id)
			exec("UPDATE change_log SET entity_id = ? WHERE entity = ? AND entity_id = ?", p.was, p.entity, p.id)
			exec("UPDATE activity SET entity_id = ? WHERE entity = ? AND entity_id = ?", p.was, p.entity, p.id)
		}

		exec0("ALTER TABLE tasks DROP COLUMN started_at")
		exec0("ALTER TABLE tasks DROP COLUMN made_at")
		_, err = d.Migrate(ctx)
		must(t, err)
		count := func(query string, args ...any) int {
			t.Helper()
			if d.Dialect == db.Postgres {
				for i, n := 0, 1; i < len(query); i++ {
					if query[i] == '?' {
						query = query[:i] + "$" + string(rune('0'+n)) + query[i+1:]
						n++
					}
				}
			}
			var n int
			must(t, d.QueryRowContext(ctx, query, args...).Scan(&n))
			return n
		}
		for _, id := range []string{syncer.PairID(task, label), odd} {
			if count("SELECT COUNT(*) FROM task_labels WHERE id = ?", id) != 1 {
				t.Errorf("no task label %s after migrating", id)
			}
		}
		if count("SELECT COUNT(*) FROM task_assignees WHERE id = ?", syncer.PairID(task, member)) != 1 {
			t.Error("the assignee did not move to its hash")
		}
		if count("SELECT COUNT(*) FROM my_day_entries WHERE id = ?", syncer.PairID(task, member)) != 1 {
			t.Error("the My Day entry did not move to its hash")
		}
		for _, old := range []string{syncer.LegacyPairID(task, label), syncer.LegacyPairID(task, member)} {
			if n := count("SELECT COUNT(*) FROM change_log WHERE entity_id = ?", old) + count("SELECT COUNT(*) FROM activity WHERE entity_id = ?", old); n != 0 {
				t.Errorf("%d log and history entries still name %s", n, old)
			}
		}
		if count("SELECT COUNT(*) FROM change_log WHERE entity_id = ?", syncer.PairID(task, label)) == 0 {
			t.Error("the change log does not follow the label's row")
		}
		fresh := syncer.New(store.New(d), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
		if now, err := fresh.Epoch(ctx); err != nil || now == epoch {
			t.Errorf("the sync epoch after migrating = %q (%v), was %q", now, err, epoch)
		}

		// An edit from before, by the old ID, reaches the row.
		w.svc = fresh
		wantStatus(t, w.push("owner", op("task_label", syncer.LegacyPairID(task, label), later(10), map[string]any{"deleted_at": "2026-11-05T10:00:00Z"}))[0], "applied")
		if count("SELECT COUNT(*) FROM task_labels WHERE id = ? AND deleted_at IS NOT NULL", syncer.PairID(task, label)) != 1 {
			t.Error("an edit by the old ID did not reach the row")
		}
		rec, err := fresh.Get(ctx, w.users["owner"], w.ws, "task_assignee", syncer.LegacyPairID(task, member))
		if err != nil || rec.ID != syncer.PairID(task, member) {
			t.Errorf("a REST read by the old ID = %+v, %v", rec, err)
		}
	})
}
