package syncer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/filter"
	"github.com/locodarwin/mila/server/internal/store"
)

// What can start a rule, what a rule can do, and how many it may hold
// (docs/PLAN.md section 3). web/src/lib/automations.ts has the same lists.
const (
	AutomationCreated   = "created"
	AutomationCompleted = "completed"
	AutomationReopened  = "reopened"
	AutomationStatus    = "status"
	AutomationPriority  = "priority"
	AutomationAssigned  = "assigned"
	AutomationLabel     = "label"
	AutomationField     = "field"
	AutomationComment   = "comment"
	AutomationUnblocked = "unblocked"
	AutomationEmail     = "email"
	AutomationDue       = "due"

	// MaxAutomationChanges is how many changes one rule makes.
	MaxAutomationChanges = 10
	// MaxAutomations is how many rules a workspace holds.
	MaxAutomations = 100
)

var automationStarts = []string{
	AutomationCreated, AutomationCompleted, AutomationReopened, AutomationStatus, AutomationPriority,
	AutomationAssigned, AutomationLabel, AutomationField, AutomationComment, AutomationUnblocked,
	AutomationEmail, AutomationDue,
}

// Who a change can name besides a person. The rule's own words, never IDs.
const (
	WhoMadeTask = "creator"
	WhoDidThis  = "actor"
)

// What a rule can do.
const (
	DoStatus      = "status"
	DoPriority    = "priority"
	DoDue         = "due"
	DoComplete    = "complete"
	DoAssign      = "assign"
	DoUnassign    = "unassign"
	DoAddLabel    = "add_label"
	DoRemoveLabel = "remove_label"
	DoMove        = "move"
	DoComment     = "comment"
	DoSubtask     = "subtask"
	DoNotify      = "notify"
)

var automationChanges = []string{
	DoStatus, DoPriority, DoDue, DoComplete, DoAssign, DoUnassign, DoAddLabel, DoRemoveLabel, DoMove,
	DoComment, DoSubtask, DoNotify,
}

// AutomationRule is a rule as its row holds it. A status is the ID of one of a
// project's statuses, or for a rule across projects "kind:" and a status's category.
type AutomationRule struct {
	When struct {
		On    string `json:"on"`
		Value string `json:"value"`
	} `json:"when"`
	// Config is a saved view's config, whose filter is the rule's Only if.
	Config json.RawMessage    `json:"config"`
	Then   []AutomationChange `json:"then"`
}

// AutomationChange is one thing a rule does.
type AutomationChange struct {
	Do    string `json:"do"`
	Value string `json:"value"`
}

// ParseAutomationRule reads a rule's JSON.
func ParseAutomationRule(text string) (AutomationRule, error) {
	var r AutomationRule
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return AutomationRule{}, err
	}
	return r, nil
}

// automationDoc says what a rule holds, for the API reference.
const automationDoc = `What starts the rule, what it needs and what it does: {"when":{"on":"status","value":"<id>"},"config":{"filter":{}},"then":[{"do":"priority","value":"high"}]}.`

// automationShape checks a rule for what the server and the app read: when's on is
// known, then holds 1 to MaxAutomationChanges known changes, and config is a view's.
func automationShape(v any) error {
	rule, ok := v.(map[string]any)
	if !ok {
		return errors.New("must be an object")
	}
	when, ok := rule["when"].(map[string]any)
	if !ok {
		return errors.New("when must be an object")
	}
	on, _ := when["on"].(string)
	if !slices.Contains(automationStarts, on) {
		return fmt.Errorf("when's on must be one of %v", automationStarts)
	}
	if value, given := when["value"]; given {
		if _, isText := value.(string); !isText {
			return errors.New("when's value must be a string")
		}
	}
	if config, given := rule["config"]; given {
		if err := configShape(config); err != nil {
			return fmt.Errorf("config %w", err)
		}
	}
	then, ok := rule["then"].([]any)
	if !ok || len(then) == 0 {
		return errors.New("then must be a list with something in it")
	}
	if len(then) > MaxAutomationChanges {
		return fmt.Errorf("holds %d changes; a rule holds %d at most", len(then), MaxAutomationChanges)
	}
	for i, x := range then {
		change, ok := x.(map[string]any)
		do, _ := change["do"].(string)
		if !ok || !slices.Contains(automationChanges, do) {
			return fmt.Errorf("then[%d]'s do must be one of %v", i, automationChanges)
		}
		if value, given := change["value"]; given {
			if _, isText := value.(string); !isText {
				return fmt.Errorf("then[%d]'s value must be a string", i)
			}
		}
	}
	return nil
}

// checkAutomation holds a write to a rule to who may make it: a rule across the
// workspace is for owners and admins, and a rule runs as whoever last saved it, so nobody
// can write what it does, or set it going, and leave it running as someone else. What it
// does and whom it runs as are written together, so their clocks stay the same and a
// write never takes one without the other.
func checkAutomation(ctx context.Context, q store.Queries, v *viewer, workspaceID string, r *row, accepted []string) error {
	if v.server {
		return nil
	}
	if r.values["project_id"] == nil && !authz.Can(v.role, authz.ManageMembers) {
		return refuse(apperr.Forbidden, "", "Only owners and admins can change automations for the whole workspace.")
	}
	saved := slices.Contains(accepted, "rule")
	if saved != slices.Contains(accepted, "made_by") {
		return refuse(apperr.Invalid, "made_by", "A rule and made_by are saved together.")
	}
	on, _ := r.values["enabled"].(bool)
	live := r.values["deleted_at"] == nil
	cameBack := r.exists && live && slices.Contains(accepted, "deleted_at")
	started := on && live && (cameBack || slices.Contains(accepted, "enabled"))
	if (saved || started) && r.values["made_by"] != v.userID {
		return refuse(apperr.Invalid, "made_by", "A rule runs as whoever saved it last. Save it to make it yours, then turn it on.")
	}
	if live && (!r.exists || cameBack) {
		n, err := q.CountLiveAutomations(ctx, workspaceID)
		if err != nil {
			return err
		}
		if n >= MaxAutomations {
			return refuse(apperr.Invalid, "", "A workspace holds %d automations at most.", MaxAutomations)
		}
	}
	return nil
}

type (
	automatedKey struct{}
	emailKey     struct{}
)

// automationVia starts how history says a rule did something.
const automationVia = "automation: "

// WithAutomation marks what is done under this context as a rule's: its history says
// so, and it starts no other rule.
func WithAutomation(ctx context.Context, name string) context.Context {
	return context.WithValue(WithVia(ctx, automationVia+name), automatedKey{}, true)
}

func isAutomated(ctx context.Context) bool {
	done, _ := ctx.Value(automatedKey{}).(bool)
	return done
}

// WithEmail marks what is done under this context as coming from an email. It's kept
// apart from the words history shows, which an API token's name could match.
func WithEmail(ctx context.Context) context.Context {
	return context.WithValue(WithVia(ctx, "email"), emailKey{}, true)
}

func isEmailed(ctx context.Context) bool {
	mailed, _ := ctx.Value(emailKey{}).(bool)
	return mailed
}

// viaFor is how history names what a change came through, for someone reading it. A
// rule's name stays with those who can see the rule.
func viaFor(v *viewer, via string) string {
	if v.guest() && strings.HasPrefix(via, automationVia) {
		return "an automation"
	}
	return via
}

// ProjectOpen says whether a project is in a workspace, and neither archived nor in
// the trash.
func (s *Service) ProjectOpen(ctx context.Context, workspaceID, projectID string) (bool, error) {
	d := s.st.DB()
	var n int
	err := d.QueryRowContext(ctx, rebind(d.Dialect,
		"SELECT COUNT(*) FROM projects WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL AND archived_at IS NULL"), projectID, workspaceID).Scan(&n)
	return n > 0, err
}

// PauseAutomation stops a rule, saying why. The server acts here, since whoever
// made the rule may be what is wrong with it.
func (s *Service) PauseAutomation(ctx context.Context, workspaceID, id, actor, why string) error {
	enabled, _ := json.Marshal(false)
	reason, _ := json.Marshal(why)
	now := s.now()
	v := &viewer{userID: actor, role: authz.Owner, server: true}
	op := Op{OpID: uuid.NewString(), Entity: "automation", ID: id, HLC: serverHLC(now), trusted: true,
		Fields: map[string]json.RawMessage{"enabled": enabled, "paused_why": reason}}
	_, changed, err := s.apply(ctx, v, workspaceID, op)
	if err != nil {
		return err
	}
	if changed && s.notify != nil {
		if seq, err := s.st.GetChangeSeq(ctx, workspaceID); err == nil {
			s.notify.Changed(workspaceID, seq)
		}
	}
	return nil
}

// AutomationActor reports whether someone can still run a rule: they are in the
// workspace and no guest, and for a rule across it, an owner or admin.
func (s *Service) AutomationActor(ctx context.Context, userID, workspaceID string, wide bool) (bool, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if apperr.KindOf(err) == apperr.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if v.guest() || !authz.Can(v.role, authz.EditContent) {
		return false, nil
	}
	return !wide || authz.Can(v.role, authz.ManageMembers), nil
}

// AutomationStatusRow is one of a project's statuses.
type AutomationStatusRow struct {
	ID       string
	Category string
}

// StatusesOf lists a project's statuses in their order.
func (s *Service) StatusesOf(ctx context.Context, projectID string) ([]AutomationStatusRow, error) {
	d := s.st.DB()
	rows, err := d.QueryContext(ctx, rebind(d.Dialect,
		"SELECT id, category FROM statuses WHERE project_id = ? AND deleted_at IS NULL ORDER BY sort_key, id"), projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AutomationStatusRow
	for rows.Next() {
		var st AutomationStatusRow
		if err := rows.Scan(&st.ID, &st.Category); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// TaskFacts is what a rule reads of a task.
type TaskFacts struct {
	ID, WorkspaceID, ProjectID, ParentID, Title, Notes, Priority string
	StatusID, Category, CreatedBy, DueDate, DueTime, StartDate   string
	Recurrence, RecurrenceMode, SeriesID                         string
	Completed, Deleted                                           bool
	Estimate                                                     *int64
	Labels, Assignees, Subtasks                                  []string
	Checklist                                                    []ChecklistFact
	Entries                                                      []filter.Entry
	FieldValues                                                  map[string]json.RawMessage
}

// ChecklistFact is a line of a task's checklist.
type ChecklistFact struct{ ID, Text, SortKey string }

// TaskFactsOf reads a task for a rule, and reports false if there is no such task.
func (s *Service) TaskFactsOf(ctx context.Context, taskID string) (TaskFacts, bool, error) {
	d := s.st.DB()
	var (
		t                                                       TaskFacts
		project, parent, status, createdBy, due, dueTime, start sql.NullString
		notes, recurrence, mode, series                         sql.NullString
		estimate                                                sql.NullInt64
		completed, deleted                                      sql.NullTime
	)
	err := d.QueryRowContext(ctx, rebind(d.Dialect, `SELECT id, workspace_id, project_id, parent_id, title, notes, priority,
		status_id, created_by, due_date, due_time, start_date, recurrence, recurrence_mode, series_id, estimate_minutes,
		completed_at, deleted_at FROM tasks WHERE id = ?`), taskID).Scan(&t.ID, &t.WorkspaceID, &project, &parent, &t.Title, &notes,
		&t.Priority, &status, &createdBy, &due, &dueTime, &start, &recurrence, &mode, &series, &estimate, &completed, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskFacts{}, false, nil
	}
	if err != nil {
		return TaskFacts{}, false, err
	}
	t.ProjectID, t.ParentID, t.StatusID, t.CreatedBy = project.String, parent.String, status.String, createdBy.String
	t.DueDate, t.DueTime, t.StartDate, t.Notes = due.String, dueTime.String, start.String, notes.String
	t.Recurrence, t.RecurrenceMode, t.SeriesID = recurrence.String, mode.String, series.String
	t.Completed, t.Deleted = completed.Valid, deleted.Valid
	if estimate.Valid {
		t.Estimate = &estimate.Int64
	}
	t.Category = "todo"
	if t.Completed {
		t.Category = "done"
	}
	if t.StatusID != "" {
		if err := d.QueryRowContext(ctx, rebind(d.Dialect, "SELECT category FROM statuses WHERE id = ?"), t.StatusID).Scan(&t.Category); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return TaskFacts{}, false, err
		}
	}
	if t.Labels, err = queryIDs(ctx, d, rebind(d.Dialect, "SELECT label_id FROM task_labels WHERE task_id = ? AND deleted_at IS NULL ORDER BY label_id"), taskID); err != nil {
		return TaskFacts{}, false, err
	}
	if t.Assignees, err = queryIDs(ctx, d, rebind(d.Dialect, "SELECT user_id FROM task_assignees WHERE task_id = ? AND deleted_at IS NULL ORDER BY user_id"), taskID); err != nil {
		return TaskFacts{}, false, err
	}
	if t.Subtasks, err = queryIDs(ctx, d, rebind(d.Dialect, "SELECT id FROM tasks WHERE parent_id = ? AND deleted_at IS NULL ORDER BY sort_key, id"), taskID); err != nil {
		return TaskFacts{}, false, err
	}
	rows, err := d.QueryContext(ctx, rebind(d.Dialect, "SELECT id, text, sort_key FROM checklist_items WHERE task_id = ? AND deleted_at IS NULL ORDER BY sort_key, id"), taskID)
	if err != nil {
		return TaskFacts{}, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c ChecklistFact
		if err := rows.Scan(&c.ID, &c.Text, &c.SortKey); err != nil {
			return TaskFacts{}, false, err
		}
		t.Checklist = append(t.Checklist, c)
	}
	if err := rows.Err(); err != nil {
		return TaskFacts{}, false, err
	}
	logged, err := d.QueryContext(ctx, rebind(d.Dialect, "SELECT user_id, day, minutes FROM time_entries WHERE task_id = ? AND deleted_at IS NULL"), taskID)
	if err != nil {
		return TaskFacts{}, false, err
	}
	defer func() { _ = logged.Close() }()
	for logged.Next() {
		var e filter.Entry
		if err := logged.Scan(&e.User, &e.Day, &e.Minutes); err != nil {
			return TaskFacts{}, false, err
		}
		t.Entries = append(t.Entries, e)
	}
	if err := logged.Err(); err != nil {
		return TaskFacts{}, false, err
	}
	values, err := d.QueryContext(ctx, rebind(d.Dialect, "SELECT field_id, value FROM custom_field_values WHERE task_id = ? AND deleted_at IS NULL"), taskID)
	if err != nil {
		return TaskFacts{}, false, err
	}
	defer func() { _ = values.Close() }()
	t.FieldValues = map[string]json.RawMessage{}
	for values.Next() {
		var id, value string
		if err := values.Scan(&id, &value); err != nil {
			return TaskFacts{}, false, err
		}
		t.FieldValues[id] = json.RawMessage(value)
	}
	return t, true, values.Err()
}

// Fits says whether a task fits a rule's Only if, as a saved view's filter would have
// it for the person the rule runs as, on their today. Tasks that are done count, since
// finishing one is a thing a rule can start on.
func (s *Service) Fits(ctx context.Context, t TaskFacts, config json.RawMessage, me, today string) (bool, error) {
	if len(config) == 0 {
		return true, nil
	}
	f, err := filter.FromConfig(config)
	if err != nil {
		return false, nil
	}
	f.ShowDone = true
	d := s.st.DB()
	blockers, err := s.Blockers(ctx, t.ID)
	if err != nil {
		return false, err
	}
	waiting, err := s.Waiting(ctx, t.ID)
	if err != nil {
		return false, err
	}
	open := func(b TaskBrief) bool { return !b.Completed && !b.Deleted }
	types := map[string]string{}
	return filter.Matches(filter.Task{ID: t.ID, ProjectID: t.ProjectID, StatusID: t.StatusID, Title: t.Title, Notes: t.Notes, Priority: t.Priority, DueDate: t.DueDate, Estimate: t.Estimate}, f, filter.Context{
		Me: me, Today: today,
		Category:  func(filter.Task) string { return t.Category },
		Labels:    func(string) []string { return t.Labels },
		Assignees: func(string) []string { return t.Assignees },
		FieldType: func(id string) string {
			if kind, ok := types[id]; ok {
				return kind
			}
			var kind string
			if err := d.QueryRowContext(ctx, rebind(d.Dialect, "SELECT type FROM custom_fields WHERE id = ? AND deleted_at IS NULL"), id).Scan(&kind); err != nil {
				kind = ""
			}
			types[id] = kind
			return kind
		},
		Value: func(_, id string) any {
			raw, ok := t.FieldValues[id]
			if !ok {
				return nil
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			var v any
			if dec.Decode(&v) != nil {
				return nil
			}
			return v
		},
		Blocked:  func(string) bool { return slices.ContainsFunc(blockers, open) },
		Blocking: func(string) bool { return !t.Completed && !t.Deleted && slices.ContainsFunc(waiting, open) },
		Entries:  func(string) []filter.Entry { return t.Entries },
		// The My Day of the person the rule runs as.
		InMyDay: func(id string) bool {
			var n int
			err := d.QueryRowContext(ctx, rebind(d.Dialect, "SELECT COUNT(*) FROM my_day_entries WHERE task_id = ? AND user_id = ? AND day <= ? AND deleted_at IS NULL"), id, me, today).Scan(&n)
			return err == nil && n > 0
		},
	}), nil
}

// AutomationDay is a time as a day in a place, or "" for none.
func AutomationDay(t sql.NullTime, loc *time.Location) string {
	if !t.Valid {
		return ""
	}
	return t.Time.In(loc).Format(time.DateOnly)
}
