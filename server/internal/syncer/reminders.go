package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/recur"
)

// ReminderBrief is a reminder as the scan that fires reminders sees it.
type ReminderBrief struct {
	ID          string
	WorkspaceID string
	TaskID      string
	UserID      string
	RemindAt    time.Time
	// Offset is the minutes before the task's due moment, for a reminder
	// made relative to it; Relative says whether it was.
	Offset   int64
	Relative bool
	// SetAt is when its time was last set (or it was brought back), by its
	// clocks.
	SetAt time.Time
}

// Reminders lists the live reminders on open, live tasks that may come
// due around now: those at a fixed time in (from, to], and every relative
// one on a task due on or after dueFrom ("2026-10-28"), whose moment the
// caller works out in its owner's timezone. A relative reminder on a task
// with no due date keeps the time it last worked out to.
func (s *Service) Reminders(ctx context.Context, from, to time.Time, dueFrom string) ([]ReminderBrief, error) {
	d := s.st.DB()
	rows, err := d.QueryContext(ctx, rebind(d.Dialect, `SELECT r.id, r.workspace_id, r.task_id, r.user_id, r.remind_at, r.offset_minutes, r.clocks
		FROM reminders r JOIN tasks t ON t.id = r.task_id
		WHERE r.deleted_at IS NULL AND t.deleted_at IS NULL AND t.completed_at IS NULL
		  AND (((r.offset_minutes IS NULL OR t.due_date IS NULL) AND r.remind_at > ? AND r.remind_at <= ?)
		       OR (r.offset_minutes IS NOT NULL AND t.due_date >= ?))
		ORDER BY r.remind_at, r.id`), from, to, dueFrom)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ReminderBrief
	for rows.Next() {
		var r ReminderBrief
		var offset sql.NullInt64
		var raw string
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &r.TaskID, &r.UserID, &r.RemindAt, &offset, &raw); err != nil {
			return nil, err
		}
		r.RemindAt = r.RemindAt.UTC()
		r.Offset, r.Relative = offset.Int64, offset.Valid
		var clocks map[string]string
		if err := json.Unmarshal([]byte(raw), &clocks); err != nil {
			return nil, err
		}
		r.SetAt = hlcTime(max(clocks["remind_at"], clocks["offset_minutes"], clocks["deleted_at"]))
		out = append(out, r)
	}
	return out, rows.Err()
}

// DueBetween lists the open, live tasks due from one date to another
// ("2026-10-27", "2026-10-29"), in every workspace.
func (s *Service) DueBetween(ctx context.Context, from, through string) ([]TaskBrief, error) {
	d := s.st.DB()
	ids, err := queryIDs(ctx, d, rebind(d.Dialect, `SELECT id FROM tasks
		WHERE deleted_at IS NULL AND completed_at IS NULL AND due_date >= ? AND due_date <= ?
		ORDER BY due_date, id`), from, through)
	if err != nil {
		return nil, err
	}
	out := make([]TaskBrief, 0, len(ids))
	for _, id := range ids {
		t, ok, err := s.Task(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok && !t.Deleted {
			out = append(out, t)
		}
	}
	return out, nil
}

// CopyReminders gives the next task of a repeating series the reminders
// made relative to the due moment that the task it follows had, for
// everyone who set one. The person who completed the task copies their own
// as they do; the others are not there to. Each copy has the ID the app
// would give it, so the two meet in one row, and a copy that exists
// already (even deleted) is left alone. It reports how many it made.
func (s *Service) CopyReminders(ctx context.Context, taskID string) (int, error) {
	d := s.st.DB()
	task, err := loadRow(ctx, d, d.Dialect, entities["task"], taskID)
	if err != nil || !task.exists {
		return 0, err
	}
	series, _ := task.values["series_id"].(string)
	if series == "" {
		return 0, nil
	}
	// The task it follows is in the same series and names it by ID, from
	// its own ID alone, so the match holds after the new task is moved.
	ids, err := queryIDs(ctx, d, rebind(d.Dialect,
		"SELECT id FROM tasks WHERE workspace_id = ? AND (series_id = ? OR id = ?) AND id <> ? ORDER BY id"),
		task.workspaceID, series, series, taskID)
	if err != nil {
		return 0, err
	}
	var prev *row
	for _, id := range ids {
		if recur.NextTaskID(id) == taskID {
			if prev, err = loadRow(ctx, d, d.Dialect, entities["task"], id); err != nil {
				return 0, err
			}
			break
		}
	}
	if prev == nil {
		return 0, nil
	}
	// A copy keeps its distance from the due moment; the scan works out
	// the exact time in its owner's timezone, so this only moves remind_at
	// by as much as the due date and time moved.
	var shift time.Duration
	if task.values["due_date"] != nil && prev.values["due_date"] != nil {
		shift = naiveDue(task.values).Sub(naiveDue(prev.values))
	}

	e := entities["reminder"]
	rows, err := d.QueryContext(ctx, rebind(d.Dialect,
		"SELECT id FROM reminders WHERE task_id = ? AND deleted_at IS NULL AND offset_minutes IS NOT NULL ORDER BY id"), prev.id)
	if err != nil {
		return 0, err
	}
	var reminderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		reminderIDs = append(reminderIDs, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	made := 0
	for _, id := range reminderIDs {
		old, err := loadRow(ctx, d, d.Dialect, e, id)
		if err != nil {
			return made, err
		}
		copyID := recur.HashID(taskID + "/" + id)
		existing, err := loadRow(ctx, d, d.Dialect, e, copyID)
		if err != nil {
			return made, err
		}
		if existing.exists {
			continue
		}
		owner, _ := old.values["user_id"].(string)
		v, err := s.viewer(ctx, s.st, owner, task.workspaceID)
		if apperr.KindOf(err) == apperr.NotFound {
			continue // no longer a member
		}
		if err != nil {
			return made, err
		}
		remindAt, _ := old.values["remind_at"].(time.Time)
		fields := map[string]any{
			"task_id": taskID, "user_id": owner, "offset_minutes": old.values["offset_minutes"],
			"remind_at": remindAt.Add(shift).UTC().Format(time.RFC3339Nano), "deleted_at": nil,
		}
		raw := map[string]json.RawMessage{}
		for k, val := range fields {
			b, _ := json.Marshal(val)
			raw[k] = b
		}
		op := Op{OpID: uuid.NewString(), Entity: e.name, ID: copyID, HLC: serverHLC(s.now()), Fields: raw, trusted: true}
		res, changed, err := s.apply(ctx, v, task.workspaceID, op)
		if err != nil {
			return made, err
		}
		if res.Status == "rejected" {
			// A guest who lost the project, say; theirs simply does not copy.
			s.log.Debug("reminder not copied", "reminder", id, "reason", res.Message)
			continue
		}
		if changed {
			made++
			if s.notify != nil {
				seq, err := s.st.GetChangeSeq(ctx, task.workspaceID)
				if err != nil {
					return made, err
				}
				s.notify.Changed(task.workspaceID, seq)
			}
		}
	}
	return made, nil
}

// naiveDue is a task's due date and time as if in UTC, for measuring how
// far a due moment moved; all-day tasks count from 09:00.
func naiveDue(values map[string]any) time.Time {
	date, _ := values["due_date"].(string)
	clock, _ := values["due_time"].(string)
	if clock == "" {
		clock = "09:00"
	}
	t, _ := time.Parse("2006-01-02 15:04", date+" "+clock)
	return t
}
