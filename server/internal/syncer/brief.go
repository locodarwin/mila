package syncer

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
)

// Applied describes one accepted op to a Watcher: the row before and
// after, and which fields the op set.
type Applied struct {
	Entity      string
	ID          string
	WorkspaceID string
	ActorID     string
	// Created is true when the op made the row.
	Created  bool
	Accepted []string
	// Values is the row after the op, Before the row before it (empty when
	// created). Values are strings, bools, int64s, time.Times or nil.
	Values map[string]any
	Before map[string]any
	// At is when the server applied it, and Clock when it was made, by
	// its HLC.
	At    time.Time
	Clock time.Time
	// Via is the API token it came through, if any. FromDoc says it was the
	// live document writing notes or a page's text, as it does while typing.
	Via string
	// Emailed says it came from an email, which only email to task can say.
	Emailed bool
	FromDoc bool
	// Automated says a rule made this change, and Bulk that it is one of many made
	// at once (a template's rows). Neither starts a rule.
	Automated bool
	Bulk      bool
	// Batch names the change to many tasks this is part of, if any (Op.Batch).
	Batch string
	// Fields is the row as REST shows it, worked out only if asked for.
	Fields func() map[string]any
}

type viaKey struct{}

// WithVia marks what is done under this context as coming through an API token.
func WithVia(ctx context.Context, tokenName string) context.Context {
	return context.WithValue(ctx, viaKey{}, tokenName)
}

func viaOf(ctx context.Context) string {
	name, _ := ctx.Value(viaKey{}).(string)
	return name
}

// Watchers is several Watchers as one.
type Watchers []Watcher

// Applied tells each in turn; the first error fails the push.
func (ws Watchers) Applied(ctx context.Context, q store.Queries, a Applied) error {
	for _, w := range ws {
		if err := w.Applied(ctx, q, a); err != nil {
			return err
		}
	}
	return nil
}

// Committed tells each.
func (ws Watchers) Committed() {
	for _, w := range ws {
		w.Committed()
	}
}

// Watcher hears about every accepted op inside the op's transaction, so
// work it queues (notifications) exists exactly when the change does, and
// once a push has committed. An error from Applied fails the push, so it
// should only queue work, never deliver it.
type Watcher interface {
	Applied(ctx context.Context, q store.Queries, a Applied) error
	Committed()
}

// SetWatcher connects a Watcher.
func (s *Service) SetWatcher(w Watcher) { s.watch = w }

// TaskBrief is what notifications need to know about a task.
type TaskBrief struct {
	ID          string
	WorkspaceID string
	Title       string
	ProjectID   string // "" for the inbox
	ProjectName string
	CreatedBy   string
	SeriesID    string
	Assignees   []string
	DueDate     string
	DueTime     string
	// DueSetAt is when the due date or time was last set, by its clock.
	DueSetAt  time.Time
	Completed bool
	// Deleted is true for a task in the trash, or in a project that is.
	Deleted bool
	// Archived is true for a task in an archived project: still there, but
	// nobody should be told about it because of a date.
	Archived  bool
	CreatedAt time.Time
	// LiveSince is when it was last made or brought back, by the clock of
	// its deleted_at; zero if that was never set.
	LiveSince time.Time
}

// Task describes a task, and reports false if there is no such task.
func (s *Service) Task(ctx context.Context, taskID string) (TaskBrief, bool, error) {
	d := s.st.DB()
	r, err := loadRow(ctx, d, d.Dialect, entities["task"], taskID)
	if err != nil || !r.exists {
		return TaskBrief{}, false, err
	}
	t := TaskBrief{ID: r.id, WorkspaceID: r.workspaceID}
	t.Title, _ = r.values["title"].(string)
	t.ProjectID, _ = r.values["project_id"].(string)
	t.CreatedBy, _ = r.values["created_by"].(string)
	t.SeriesID, _ = r.values["series_id"].(string)
	t.DueDate, _ = r.values["due_date"].(string)
	t.DueTime, _ = r.values["due_time"].(string)
	t.DueSetAt = hlcTime(max(r.clocks["due_date"], r.clocks["due_time"]))
	t.Completed = r.values["completed_at"] != nil
	t.Deleted = r.values["deleted_at"] != nil
	if !t.Deleted {
		t.LiveSince = hlcTime(r.clocks["deleted_at"])
	}
	if err := d.QueryRowContext(ctx, rebind(d.Dialect, "SELECT created_at FROM tasks WHERE id = ?"), taskID).Scan(&t.CreatedAt); err != nil {
		return t, false, err
	}
	if t.ProjectID != "" {
		p, err := loadRow(ctx, d, d.Dialect, entities["project"], t.ProjectID)
		if err != nil {
			return t, false, err
		}
		t.ProjectName, _ = p.values["name"].(string)
		t.Deleted = t.Deleted || p.values["deleted_at"] != nil
		t.Archived = p.values["archived_at"] != nil
	}
	rows, err := d.QueryContext(ctx, rebind(d.Dialect,
		"SELECT user_id FROM task_assignees WHERE task_id = ? AND deleted_at IS NULL ORDER BY user_id"), taskID)
	if err != nil {
		return t, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return t, false, err
		}
		t.Assignees = append(t.Assignees, id)
	}
	return t, true, rows.Err()
}

// Waiting describes the tasks that wait for a task (its live
// dependencies' tasks), done and deleted ones too: the caller decides.
func (s *Service) Waiting(ctx context.Context, taskID string) ([]TaskBrief, error) {
	return s.briefsOf(ctx, "SELECT task_id FROM task_dependencies WHERE blocker_id = ? AND deleted_at IS NULL ORDER BY task_id", taskID)
}

// Blockers describes the tasks a task waits for.
func (s *Service) Blockers(ctx context.Context, taskID string) ([]TaskBrief, error) {
	return s.briefsOf(ctx, "SELECT blocker_id FROM task_dependencies WHERE task_id = ? AND deleted_at IS NULL ORDER BY blocker_id", taskID)
}

func (s *Service) briefsOf(ctx context.Context, query, taskID string) ([]TaskBrief, error) {
	d := s.st.DB()
	ids, err := queryIDs(ctx, d, rebind(d.Dialect, query), taskID)
	if err != nil {
		return nil, err
	}
	var out []TaskBrief
	for _, id := range ids {
		t, ok, err := s.Task(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// ThreadBrief is what notifications need of a thread on a wiki page.
type ThreadBrief struct {
	ID, PageID, ProjectID, WorkspaceID string
	// PageTitle is what the page is called: a main page goes by its wiki.
	PageTitle string
	Deleted   bool
}

// Thread describes a thread on a wiki page. It counts as deleted while it,
// its page or its project is in the trash.
func (s *Service) Thread(ctx context.Context, threadID string) (ThreadBrief, bool, error) {
	d := s.st.DB()
	r, err := loadRow(ctx, d, d.Dialect, entities["page_thread"], threadID)
	if err != nil || !r.exists {
		return ThreadBrief{}, false, err
	}
	pageID, _ := r.values["page_id"].(string)
	page, err := loadRow(ctx, d, d.Dialect, entities["page"], pageID)
	if err != nil || !page.exists {
		return ThreadBrief{}, false, err
	}
	t := ThreadBrief{ID: r.id, PageID: pageID, WorkspaceID: r.workspaceID}
	t.ProjectID, _ = page.values["project_id"].(string)
	t.PageTitle, _ = page.values["title"].(string)
	t.Deleted = r.values["deleted_at"] != nil || page.values["deleted_at"] != nil
	if t.ProjectID != "" {
		project, err := loadRow(ctx, d, d.Dialect, entities["project"], t.ProjectID)
		if err != nil {
			return ThreadBrief{}, false, err
		}
		t.Deleted = t.Deleted || project.values["deleted_at"] != nil
		if name, _ := project.values["name"].(string); pageID == MainPageID(t.ProjectID) {
			t.PageTitle = name
		}
	} else if pageID == MainPageID(r.workspaceID) {
		t.PageTitle = "Workspace Wiki"
	}
	if t.PageTitle == "" {
		t.PageTitle = "Untitled"
	}
	return t, true, nil
}

// ThreadPeople are who wrote in a thread, and who began it.
func (s *Service) ThreadPeople(ctx context.Context, threadID string) ([]string, error) {
	d := s.st.DB()
	return queryIDs(ctx, d, rebind(d.Dialect,
		"SELECT author_id FROM page_comments WHERE thread_id = ? AND deleted_at IS NULL UNION SELECT created_by FROM page_threads WHERE id = ? ORDER BY 1"),
		threadID, threadID)
}

// SeesProject reports whether a user is a member of the workspace who may
// see a project in it ("" for the inbox) right now. Guests see only the
// projects shared with them.
func (s *Service) SeesProject(ctx context.Context, userID, workspaceID, projectID string) (bool, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if apperr.KindOf(err) == apperr.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var project any
	if projectID != "" {
		project = projectID
	}
	return v.sees(project), nil
}

// DueTasks lists the open tasks due on or before a date ("2026-10-28")
// that are the user's, across every workspace they belong to: assigned to
// them, or made by them and assigned to nobody. Only tasks they can still
// see count, and none in an archived project. Earliest due first.
func (s *Service) DueTasks(ctx context.Context, userID, through string) ([]TaskBrief, error) {
	workspaces, err := s.st.ListWorkspacesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	query := rebind(d.Dialect, `SELECT t.id FROM tasks t
		WHERE t.workspace_id = ? AND t.deleted_at IS NULL AND t.completed_at IS NULL
		  AND t.due_date IS NOT NULL AND t.due_date <= ?
		  AND `+mine+`
		ORDER BY t.due_date, COALESCE(t.due_time, '99'), t.id`)
	var out []TaskBrief
	for _, ws := range workspaces {
		v, err := s.viewer(ctx, s.st, userID, ws.ID)
		if err != nil {
			return nil, err
		}
		ids, err := queryIDs(ctx, d, query, ws.ID, through, userID, userID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			t, ok, err := s.Task(ctx, id)
			if err != nil {
				return nil, err
			}
			var project any
			if t.ProjectID != "" {
				project = t.ProjectID
			}
			if ok && !t.Deleted && !t.Archived && v.sees(project) {
				out = append(out, t)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b TaskBrief) int {
		return cmp.Or(cmp.Compare(a.DueDate, b.DueDate), cmp.Compare(cmp.Or(a.DueTime, "99"), cmp.Or(b.DueTime, "99")))
	})
	return out, nil
}

func queryIDs(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TimeBrief is a task with its estimate and the time logged on it, in
// minutes.
type TimeBrief struct {
	TaskBrief
	Estimate, Logged int64
	// Itself is time logged to the project itself, not to a task: then
	// only ProjectID, ProjectName and WorkspaceID are set, and Title is the
	// project's name.
	Itself bool
}

// mine is the SQL condition for a task being a user's in the digest's
// sense, as DueTasks has it: assigned to them, or made by them and
// assigned to nobody. It takes the user's ID twice.
const mine = `(EXISTS (SELECT 1 FROM task_assignees a WHERE a.task_id = t.id AND a.user_id = ? AND a.deleted_at IS NULL)
	OR (t.created_by = ? AND NOT EXISTS (SELECT 1 FROM task_assignees a WHERE a.task_id = t.id AND a.deleted_at IS NULL)))`

// visibleBriefs describes the tasks a user can still see in a workspace,
// in the order given, with the numbers that came with them.
func (s *Service) visibleBriefs(ctx context.Context, userID, workspaceID string, ids []string, nums [][2]int64) ([]TimeBrief, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	var out []TimeBrief
	for i, id := range ids {
		t, ok, err := s.Task(ctx, id)
		if err != nil {
			return nil, err
		}
		var project any
		if t.ProjectID != "" {
			project = t.ProjectID
		}
		if ok && !t.Deleted && v.sees(project) {
			out = append(out, TimeBrief{TaskBrief: t, Estimate: nums[i][0], Logged: nums[i][1]})
		}
	}
	return out, nil
}

// queryTimes reads rows of a task ID and two whole numbers.
func queryTimes(ctx context.Context, q querier, query string, args ...any) ([]string, [][2]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	var nums [][2]int64
	for rows.Next() {
		var id string
		var a, b int64
		if err := rows.Scan(&id, &a, &b); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		nums = append(nums, [2]int64{a, b})
	}
	return ids, nums, rows.Err()
}

// LoggedOn lists the tasks, and projects themselves, a user logged time on
// for a day ("2026-10-28"), across every workspace they belong to, with the
// minutes (in Logged), most first. Only what they can still see counts.
func (s *Service) LoggedOn(ctx context.Context, userID, day string) ([]TimeBrief, error) {
	workspaces, err := s.st.ListWorkspacesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	query := rebind(d.Dialect, `SELECT e.task_id, 0, SUM(e.minutes) FROM time_entries e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.workspace_id = ? AND e.user_id = ? AND e.day = ? AND e.deleted_at IS NULL AND t.deleted_at IS NULL
		GROUP BY e.task_id ORDER BY SUM(e.minutes) DESC, e.task_id`)
	var out []TimeBrief
	for _, ws := range workspaces {
		ids, nums, err := queryTimes(ctx, d, query, ws.ID, userID, day)
		if err != nil {
			return nil, err
		}
		briefs, err := s.visibleBriefs(ctx, userID, ws.ID, ids, nums)
		if err != nil {
			return nil, err
		}
		out = append(out, briefs...)
		own, err := s.projectLoggedOn(ctx, userID, ws.ID, day)
		if err != nil {
			return nil, err
		}
		out = append(out, own...)
	}
	slices.SortStableFunc(out, func(a, b TimeBrief) int { return cmp.Compare(b.Logged, a.Logged) })
	return out, nil
}

// projectLoggedOn is the time a user logged to projects themselves on a
// day in one workspace, for the projects they still see.
func (s *Service) projectLoggedOn(ctx context.Context, userID, workspaceID, day string) ([]TimeBrief, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	rows, err := d.QueryContext(ctx, rebind(d.Dialect, `SELECT p.id, p.name, SUM(e.minutes) FROM project_time_entries e
		JOIN projects p ON p.id = e.project_id
		WHERE e.workspace_id = ? AND e.user_id = ? AND e.day = ? AND e.deleted_at IS NULL AND p.deleted_at IS NULL
		GROUP BY p.id, p.name ORDER BY SUM(e.minutes) DESC, p.id`), workspaceID, userID, day)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []TimeBrief
	for rows.Next() {
		var b TimeBrief
		if err := rows.Scan(&b.ProjectID, &b.ProjectName, &b.Logged); err != nil {
			return nil, err
		}
		if !v.sees(b.ProjectID) {
			continue
		}
		b.Itself, b.WorkspaceID, b.Title = true, workspaceID, b.ProjectName
		out = append(out, b)
	}
	return out, rows.Err()
}

// OverEstimate lists a user's open tasks, in the sense DueTasks has, with
// more time logged on them than their estimate, across their workspaces.
// A task in an archived project is nobody's to act on, so it is left out.
func (s *Service) OverEstimate(ctx context.Context, userID string) ([]TimeBrief, error) {
	workspaces, err := s.st.ListWorkspacesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	query := rebind(d.Dialect, `SELECT t.id, t.estimate_minutes, SUM(e.minutes) FROM tasks t
		JOIN time_entries e ON e.task_id = t.id AND e.deleted_at IS NULL
		WHERE t.workspace_id = ? AND t.deleted_at IS NULL AND t.completed_at IS NULL AND t.estimate_minutes IS NOT NULL
		  AND `+mine+`
		GROUP BY t.id, t.estimate_minutes
		HAVING SUM(e.minutes) > t.estimate_minutes
		ORDER BY t.id`)
	var out []TimeBrief
	for _, ws := range workspaces {
		ids, nums, err := queryTimes(ctx, d, query, ws.ID, userID, userID)
		if err != nil {
			return nil, err
		}
		briefs, err := s.visibleBriefs(ctx, userID, ws.ID, ids, nums)
		if err != nil {
			return nil, err
		}
		for _, b := range briefs {
			if !b.Archived {
				out = append(out, b)
			}
		}
	}
	return out, nil
}

// StartingStatus is the status a new task in a project starts in, as the app's
// statusFor has it: its first of category todo, or its first at all.
func (s *Service) StartingStatus(ctx context.Context, projectID string) (string, error) {
	d := s.st.DB()
	ids, err := queryIDs(ctx, d, rebind(d.Dialect,
		"SELECT id FROM statuses WHERE project_id = ? AND deleted_at IS NULL ORDER BY CASE WHEN category = 'todo' THEN 0 ELSE 1 END, sort_key, id LIMIT 1"), projectID)
	if err != nil || len(ids) == 0 {
		return "", err
	}
	return ids[0], nil
}

// CanEdit reports whether someone is a member of a workspace, no guest, who may
// change its content.
func (s *Service) CanEdit(ctx context.Context, userID, workspaceID string) (bool, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if apperr.KindOf(err) == apperr.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !v.guest() && authz.Can(v.role, authz.ReceiveEmail), nil
}
