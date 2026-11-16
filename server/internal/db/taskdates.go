package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

// taskDatesVersion fills in when each task from before was made and when
// it first went to a status of kind in-progress (00042_task_dates.sql),
// from its history, and clears the sync epoch so every device bootstraps
// a copy that has them.
const taskDatesVersion = 43

// Querier is what FillTaskDates needs of a transaction.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// FillTaskDates fills in made_at and started_at where a task has none,
// from what is known of it: made_at is the earliest of the row's own
// creation and its history (an import keeps the history of where it came
// from), and started_at the time of the first entry in its history that
// put it in a status of kind in-progress, if any did. A workspace of ""
// is every workspace. Times are the server's, as history records them.
func FillTaskDates(ctx context.Context, q Querier, dialect Dialect, workspaceID string) error {
	rebind := func(s string) string {
		if dialect != Postgres {
			return s
		}
		n := 0
		var b strings.Builder
		for _, r := range s {
			if r == '?' {
				n++
				b.WriteString("$" + strconv.Itoa(n))
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	scope, args := "", []any{}
	if workspaceID != "" {
		scope, args = " AND tasks.workspace_id = ?", []any{workspaceID}
	}
	_, err := q.ExecContext(ctx, rebind(`UPDATE tasks SET made_at = (
		SELECT CASE WHEN MIN(a.created_at) IS NOT NULL AND MIN(a.created_at) < tasks.created_at
			THEN MIN(a.created_at) ELSE tasks.created_at END
		FROM activity a WHERE a.task_id = tasks.id AND a.entity = 'task')
		WHERE made_at IS NULL`+scope), args...)
	if err != nil {
		return err
	}

	// The statuses that count as started.
	started := map[string]bool{}
	rows, err := q.QueryContext(ctx, "SELECT id FROM statuses WHERE category = 'in-progress'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		started[id] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(started) == 0 {
		return nil
	}
	// The history of tasks with no start yet that names a status, oldest
	// first; the first entry naming a started status is when it started.
	rows, err = q.QueryContext(ctx, rebind(`SELECT a.task_id, a.id, a.data FROM activity a
		JOIN tasks ON tasks.id = a.task_id
		WHERE a.entity = 'task' AND tasks.started_at IS NULL AND a.data LIKE '%"status_id"%'`+scope+`
		ORDER BY a.task_id, a.created_at, a.id`), args...)
	if err != nil {
		return err
	}
	first := map[string]string{} // task to the history entry that started it
	for rows.Next() {
		var task, id, data string
		if err := rows.Scan(&task, &id, &data); err != nil {
			_ = rows.Close()
			return err
		}
		if _, done := first[task]; done {
			continue
		}
		var fields struct {
			StatusID *string `json:"status_id"`
		}
		if json.Unmarshal([]byte(data), &fields) == nil && fields.StatusID != nil && started[*fields.StatusID] {
			first[task] = id
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	set := rebind("UPDATE tasks SET started_at = (SELECT created_at FROM activity WHERE id = ?) WHERE id = ? AND started_at IS NULL")
	for task, entry := range first {
		if _, err := q.ExecContext(ctx, set, entry, task); err != nil {
			return err
		}
	}
	return nil
}

func taskDates(dialect Dialect) *goose.Migration {
	up := func(ctx context.Context, tx *sql.Tx) error {
		if err := FillTaskDates(ctx, tx, dialect, ""); err != nil {
			return err
		}
		q := "DELETE FROM instance_setting WHERE key = ?"
		if dialect == Postgres {
			q = "DELETE FROM instance_setting WHERE key = $1"
		}
		_, err := tx.ExecContext(ctx, q, "sync_epoch")
		return err
	}
	// Going back drops the columns (42); there is nothing to undo here.
	down := func(context.Context, *sql.Tx) error { return nil }
	return goose.NewGoMigration(taskDatesVersion, &goose.GoFunc{RunTx: up}, &goose.GoFunc{RunTx: down})
}
