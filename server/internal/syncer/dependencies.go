package syncer

// Dependencies (docs/PLAN.md section 3): a task_dependency row says its
// task waits for its blocker to be done. Either may be in any project of
// the workspace; the row's scope is both projects (bothScope), so a guest
// sees it only when they see both tasks. A dependency that would close a
// loop is refused.

import (
	"context"
	"database/sql"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
)

// maxDependencyWalk bounds how many tasks a loop check visits.
const maxDependencyWalk = 10_000

// checkDependency refuses a dependency of a task on itself, or one whose
// blocker already waits, directly or down a chain, for the task.
func checkDependency(ctx context.Context, q querier, dialect db.Dialect, r *row) error {
	task, _ := r.values["task_id"].(string)
	blocker, _ := r.values["blocker_id"].(string)
	if task == blocker {
		return refuse(apperr.Invalid, "blocker_id", "A task cannot wait for itself.")
	}
	// Walk from the task to everything waiting on it; reaching the
	// blocker means the blocker already waits for the task.
	seen := map[string]bool{task: true}
	queue := []string{task}
	for len(queue) > 0 {
		if len(seen) > maxDependencyWalk {
			return refuse(apperr.Invalid, "blocker_id", "That chain of tasks is too long to check.")
		}
		at := queue[0]
		queue = queue[1:]
		waiting, err := queryIDs(ctx, q, rebind(dialect, "SELECT task_id FROM task_dependencies WHERE blocker_id = ? AND deleted_at IS NULL"), at)
		if err != nil {
			return err
		}
		for _, w := range waiting {
			if w == blocker {
				return refuse(apperr.Invalid, "blocker_id", "That would make a loop: %s already waits for %s.", titleOf(ctx, q, dialect, blocker), titleOf(ctx, q, dialect, task))
			}
			if !seen[w] {
				seen[w] = true
				queue = append(queue, w)
			}
		}
	}
	return nil
}

func titleOf(ctx context.Context, q querier, dialect db.Dialect, id string) string {
	var title string
	if err := q.QueryRowContext(ctx, rebind(dialect, "SELECT title FROM tasks WHERE id = ?"), id).Scan(&title); err != nil && err != sql.ErrNoRows {
		return "that task"
	}
	if title == "" {
		return "that task"
	}
	return `"` + title + `"`
}
