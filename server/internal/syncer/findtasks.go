package syncer

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/locodarwin/mila/server/internal/db"
)

// TaskQuery narrows the tasks a viewer sees in SQL, for find-tasks: the
// parts of a search that need nothing but the task's own row. Every part
// given must hold.
type TaskQuery struct {
	// State is open, done or all ("" is all).
	State string
	// Project keeps one project's tasks. With none, tasks in archived
	// projects stay out, as in the app's lists, unless Archived is set.
	Project  string
	Archived bool
	// Inbox keeps the tasks in no project that are not subtasks.
	Inbox bool
	// TopLevel keeps the tasks that are not subtasks.
	TopLevel bool
	// Dated keeps the tasks with a due date.
	Dated bool
	// DueFrom and DueTo bound the due date, both days included.
	DueFrom, DueTo string
	// Assignee keeps the tasks assigned to that person.
	Assignee string
	// MyDayThrough keeps the tasks the viewer picked for My Day on that
	// day or before.
	MyDayThrough string
	// IDs, when not nil, keeps only those tasks; an empty list keeps none.
	IDs []string
	// Words keeps the tasks whose title or notes hold them, without regard
	// to case. It may keep more than that: the caller checks each task
	// again.
	Words string
	// Kinds keeps the tasks whose status is of one of these kinds, or, with
	// no status of a kind, todo while open and done once completed. It may
	// keep more than that: the caller checks each task again.
	Kinds []string
	// Limit, when above 0, keeps only the first Limit tasks in TaskOrder,
	// after After when it is given.
	Limit int
	After *TaskKey
	// NoNotes leaves the notes unread, empty in every task given, for a
	// caller that counts tasks (the weekly report): they may be long.
	NoNotes bool
}

// TaskOrder is find-tasks' order: soonest due first and undated last,
// then by time with untimed last, then by title with its ASCII letters
// lowercased (as both databases can), then by ID.
func TaskOrder(a, b Record) int {
	da, db := str(a, "due_date"), str(b, "due_date")
	if (da == "") != (db == "") {
		if da == "" {
			return 1
		}
		return -1
	}
	return cmp.Or(cmp.Compare(da, db), cmp.Compare(cmp.Or(str(a, "due_time"), "99"), cmp.Or(str(b, "due_time"), "99")),
		cmp.Compare(asciiLower(str(a, "title")), asciiLower(str(b, "title"))), cmp.Compare(a.ID, b.ID))
}

// TaskKey is where a task stands in TaskOrder: its due date and time
// ("" for none), title and ID, as the task holds them.
type TaskKey struct {
	Due, Time, Title, ID string
}

// KeyOf is a task's place in TaskOrder.
func KeyOf(r Record) TaskKey {
	return TaskKey{Due: str(r, "due_date"), Time: str(r, "due_time"), Title: str(r, "title"), ID: r.ID}
}

func str(r Record, field string) string {
	s, _ := r.Fields[field].(string)
	return s
}

func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

// Tasks lists the tasks the user sees that hold to q, in no order unless
// q has a Limit. Tasks in a project in the trash stay out.
func (s *Service) Tasks(ctx context.Context, userID, workspaceID string, q TaskQuery) ([]Record, error) {
	e := entities["task"]
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	lq := Query{}
	if q.IDs != nil {
		lq.In = map[string][]string{"id": q.IDs}
	}
	d := s.st.DB()
	where, args, none, err := listWhere(d.Dialect, e, v, workspaceID, lq)
	if err != nil || none {
		return []Record{}, err
	}
	and := func(cond string, values ...any) {
		where += " AND " + cond
		args = append(args, values...)
	}
	switch q.State {
	case "open":
		and("completed_at IS NULL")
	case "done":
		and("completed_at IS NOT NULL")
	}
	// A project in the trash takes its tasks with it; an archived one
	// keeps them out of a search not about it.
	gone := "p.deleted_at IS NOT NULL"
	if q.Project != "" {
		and("project_id = ?", q.Project)
	} else if !q.Archived {
		gone += " OR p.archived_at IS NOT NULL"
	}
	and("(project_id IS NULL OR NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = tasks.project_id AND (" + gone + ")))")
	if q.Inbox {
		and("project_id IS NULL")
	}
	if q.Inbox || q.TopLevel {
		and("parent_id IS NULL")
	}
	if q.Dated {
		and("due_date IS NOT NULL")
	}
	if q.DueFrom != "" {
		and("due_date >= ?", q.DueFrom)
	}
	if q.DueTo != "" {
		and("due_date <= ?", q.DueTo)
	}
	if q.MyDayThrough != "" {
		and("EXISTS (SELECT 1 FROM my_day_entries m WHERE m.task_id = tasks.id AND m.user_id = ? AND m.day <= ? AND m.deleted_at IS NULL)", v.userID, q.MyDayThrough)
	}
	if q.Assignee != "" {
		and("EXISTS (SELECT 1 FROM task_assignees a WHERE a.task_id = tasks.id AND a.user_id = ? AND a.deleted_at IS NULL)", q.Assignee)
	}
	if cond, values := wordsSQL(d.Dialect, q.Words); cond != "" {
		and(cond, values...)
	}
	if len(q.Kinds) > 0 {
		// A superset of the caller's own check, whichever statuses it
		// knows: a status of one of the kinds, or what a task without one
		// counts as. The statuses are a list read once (IN, not a
		// correlated EXISTS): under OR, Postgres costs an EXISTS as if it
		// ran for every task, which took a 5,000-task workspace's estimate
		// past jit_above_cost, and compiling the query cost more than
		// running it.
		values := []any{workspaceID}
		for _, k := range q.Kinds {
			values = append(values, k)
		}
		conds := []string{"status_id IN (SELECT s.id FROM statuses s WHERE s.workspace_id = ? AND s.category IN (" + marks(len(q.Kinds)) + "))"}
		if slices.Contains(q.Kinds, "todo") {
			conds = append(conds, "completed_at IS NULL")
		}
		if slices.Contains(q.Kinds, "done") {
			conds = append(conds, "completed_at IS NOT NULL")
		}
		and("("+strings.Join(conds, " OR ")+")", values...)
	}
	if q.Limit > 0 {
		// TaskOrder, in bytes on both databases. A page after another
		// starts after its last task's place, compared through the same
		// expressions, so each page reads only what is left rather than
		// skipping all before it again.
		c := ""
		if d.Dialect == db.Postgres {
			c = ` COLLATE "C"`
		}
		order := []string{"CASE WHEN due_date IS NULL THEN 1 ELSE 0 END", "COALESCE(due_date, '')" + c, "COALESCE(due_time, '99')" + c, "LOWER(title" + c + ")" + c, "id" + c}
		if k := q.After; k != nil {
			undated := 0
			if k.Due == "" {
				undated = 1
			}
			text := "CAST(? AS TEXT)" + c
			and("("+strings.Join(order, ", ")+") > (?, "+text+", COALESCE(NULLIF(CAST(? AS TEXT), ''), '99')"+c+", LOWER("+text+")"+c+", "+text+")",
				undated, k.Due, k.Time, k.Title, k.ID)
		}
		where += " ORDER BY " + strings.Join(order, ", ") + " LIMIT ?"
		args = append(args, q.Limit)
	}
	cols := restColumns(e)
	if q.NoNotes {
		cols = slices.Clone(cols)
		cols[slices.Index(cols, "notes")] = "'' AS notes"
	}
	recs, err := queryColumns(ctx, d, d.Dialect, e, cols, where, args...)
	if err != nil {
		return nil, err
	}
	return s.seen(ctx, e, v, recs)
}

// wordsSQL narrows a search for words in SQL, letting through at least
// every task whose title or notes, lowercased as Go does, hold them. Only
// printable ASCII narrows: the databases fold the case of ASCII letters
// alone. Two other letters lowercase to ASCII in Go, the dotted capital I
// to i and the Kelvin sign to k, so for words with an i or a k a task
// holding that one always gets through.
func wordsSQL(dialect db.Dialect, words string) (string, []any) {
	words = strings.ToLower(words)
	if words == "" || strings.Trim(words, " !\"#$%&'()*+,-./0123456789:;<=>?@[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~") != "" {
		return "", nil
	}
	like := func(col string) string {
		if dialect == db.Postgres {
			// The C collation lowercases ASCII alone, as SQLite's LIKE does.
			return "LOWER(" + col + ` COLLATE "C") LIKE ? ESCAPE '\'`
		}
		return col + ` LIKE ? ESCAPE '\'`
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(words) + "%"
	conds := []string{like("title"), like("notes")}
	values := []any{pattern, pattern}
	for i, odd := range []string{"\u0130", "\u212A"} {
		if !strings.ContainsRune(words, rune("ik"[i])) {
			continue
		}
		conds = append(conds, "title LIKE ? ESCAPE '\\'", "notes LIKE ? ESCAPE '\\'")
		values = append(values, "%"+odd+"%", "%"+odd+"%")
	}
	return "(" + strings.Join(conds, " OR ") + ")", values
}
