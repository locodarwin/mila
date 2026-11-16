// Package taskops is what the app does to a task as one action, done on the
// server: setting a status with completion kept in step, completing and
// reopening, making the next task of a repeating series, and moving a task
// with its subtasks. The app's own versions are in web/src/lib/sync/data.ts;
// these make the same rows under the same IDs, so a device and the server
// doing the same thing make it once. Automations and the API's actions both
// use them, each writing as whoever it acts for.
package taskops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// KindPrefix marks a status given by its category ("kind:done") rather than its ID.
const KindPrefix = "kind:"

// Ops does a task's actions through Write, which writes as someone in a workspace.
type Ops struct {
	Sync  *syncer.Service
	Now   func() time.Time
	Write func(ctx context.Context, w syncer.Write) error
}

// Raw is a value as JSON, for a write's fields.
func Raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// CreateOnce makes a row under an ID worked out from what it is for, so doing the
// same thing twice makes it once.
func (o Ops) CreateOnce(ctx context.Context, entity, id string, fields map[string]json.RawMessage) error {
	err := o.Write(ctx, syncer.Write{Entity: entity, ID: id, Mode: syncer.Create, Fields: fields})
	if apperr.KindOf(err) == apperr.Conflict {
		return nil
	}
	return err
}

// SetTask changes a task's fields.
func (o Ops) SetTask(ctx context.Context, id string, fields map[string]json.RawMessage) error {
	return o.Write(ctx, syncer.Write{Entity: "task", ID: id, Mode: syncer.Update, Fields: fields})
}

// Finished says whether a status of this category completes its tasks.
func Finished(category string) bool { return category == "done" || category == "canceled" }

// StatusFor finds a status of a project by its ID, or by its category with KindPrefix.
func (o Ops) StatusFor(ctx context.Context, projectID, value string) (syncer.AutomationStatusRow, bool, error) {
	statuses, err := o.Sync.StatusesOf(ctx, projectID)
	if err != nil {
		return syncer.AutomationStatusRow{}, false, err
	}
	kind, isKind := strings.CutPrefix(value, KindPrefix)
	for _, st := range statuses {
		if (isKind && st.Category == kind) || (!isKind && st.ID == value) {
			return st, true, nil
		}
	}
	return syncer.AutomationStatusRow{}, false, nil
}

// SetStatus moves a task to a status, as the app does: a done or canceled one completes
// the task and any other reopens it.
func (o Ops) SetStatus(ctx context.Context, t *syncer.TaskFacts, value, today string) (string, error) {
	if t.ProjectID == "" {
		return "", apperr.New(apperr.Invalid, "A task in the Inbox has no status to set.")
	}
	st, found, err := o.StatusFor(ctx, t.ProjectID, value)
	if err != nil {
		return "", err
	}
	if !found {
		return "", apperr.New(apperr.Invalid, "That status isn't in this task's project.")
	}
	fields := map[string]json.RawMessage{"status_id": Raw(st.ID)}
	done := Finished(st.Category)
	if done && !t.Completed {
		// The next comes first: finished, this task is never looked at again, and a
		// next that failed would be lost.
		if err := o.Repeat(ctx, *t, today); err != nil {
			return "", err
		}
		fields["completed_at"] = Raw(o.Now().UTC().Format(time.RFC3339Nano))
		if t.Recurrence != "" && t.SeriesID == "" {
			fields["series_id"] = Raw(t.ID)
		}
	}
	if !done && t.Completed {
		fields["completed_at"] = json.RawMessage("null")
	}
	if err := o.SetTask(ctx, t.ID, fields); err != nil {
		return "", err
	}
	t.StatusID, t.Category, t.Completed = st.ID, st.Category, done
	return "Moved it to " + st.Category, nil
}

// Complete finishes a task, as checking it off does.
func (o Ops) Complete(ctx context.Context, t *syncer.TaskFacts, today string) (string, error) {
	if t.Completed {
		return "", nil
	}
	if t.ProjectID != "" {
		return o.SetStatus(ctx, t, KindPrefix+"done", today)
	}
	if err := o.Repeat(ctx, *t, today); err != nil {
		return "", err
	}
	fields := map[string]json.RawMessage{"completed_at": Raw(o.Now().UTC().Format(time.RFC3339Nano))}
	if t.Recurrence != "" && t.SeriesID == "" {
		fields["series_id"] = Raw(t.ID)
	}
	if err := o.SetTask(ctx, t.ID, fields); err != nil {
		return "", err
	}
	t.Completed = true
	return "Completed it", nil
}

// Reopen undoes a completion, as unchecking a task does: in a project it goes back to
// a to-do status. The next task of a series, if one was made, stays.
func (o Ops) Reopen(ctx context.Context, t *syncer.TaskFacts, today string) (string, error) {
	if !t.Completed {
		return "", nil
	}
	if t.ProjectID != "" {
		statuses, err := o.Sync.StatusesOf(ctx, t.ProjectID)
		if err != nil {
			return "", err
		}
		st, ok := OpenStatus(statuses)
		if !ok {
			return "", apperr.New(apperr.Invalid, "This task's project has no statuses.")
		}
		return o.SetStatus(ctx, t, st, today)
	}
	if err := o.SetTask(ctx, t.ID, map[string]json.RawMessage{"completed_at": json.RawMessage("null")}); err != nil {
		return "", err
	}
	t.Completed = false
	return "Reopened it", nil
}

// Repeat makes the next of a repeating task that is being finished, as the app does
// (repeatTask in web/src/lib/sync/data.ts), under the same IDs, so that it's never made
// twice. One that was deleted, by a finish undone, comes back. Other people's reminders
// follow through the server's usual copy. What it copies that's since been refused (a
// label gone, someone who lost the project) stays off the next one.
func (o Ops) Repeat(ctx context.Context, t syncer.TaskFacts, today string) error {
	if t.Recurrence == "" {
		return nil
	}
	due, err := recur.Next(t.Recurrence, t.RecurrenceMode, t.DueDate, today)
	if err != nil {
		return apperr.New(apperr.Invalid, "This task's repeat can't be read.")
	}
	if due == "" {
		return nil // the series has ended
	}
	id := recur.NextTaskID(t.ID)
	series := t.SeriesID
	if series == "" {
		series = t.ID
	}
	fields := map[string]json.RawMessage{
		"title": Raw(t.Title), "notes": Raw(t.Notes), "priority": Raw(t.Priority), "due_date": Raw(due),
		"recurrence": Raw(t.Recurrence), "series_id": Raw(series),
	}
	if t.RecurrenceMode != "" {
		fields["recurrence_mode"] = Raw(t.RecurrenceMode)
	}
	if t.DueTime != "" {
		fields["due_time"] = Raw(t.DueTime)
	}
	if t.ProjectID != "" {
		fields["project_id"] = Raw(t.ProjectID)
		statuses, err := o.Sync.StatusesOf(ctx, t.ProjectID)
		if err != nil {
			return err
		}
		if st, ok := OpenStatus(statuses); ok {
			fields["status_id"] = Raw(st)
		}
	}
	if t.ParentID != "" {
		fields["parent_id"] = Raw(t.ParentID)
	}
	if t.StartDate != "" && t.DueDate != "" {
		from, errFrom := time.Parse(time.DateOnly, t.StartDate)
		to, errTo := time.Parse(time.DateOnly, t.DueDate)
		next, errNext := time.Parse(time.DateOnly, due)
		if errFrom == nil && errTo == nil && errNext == nil {
			fields["start_date"] = Raw(next.Add(from.Sub(to)).Format(time.DateOnly))
		}
	}
	next, there, err := o.Sync.TaskFactsOf(ctx, id)
	if err != nil {
		return err
	}
	if there && next.Deleted {
		fields["deleted_at"] = json.RawMessage("null")
		return o.Write(ctx, syncer.Write{Entity: "task", ID: id, Mode: syncer.Upsert, Fields: fields})
	}
	if err := o.CreateOnce(ctx, "task", id, fields); err != nil {
		return err
	}
	for _, label := range t.Labels {
		if err := unlessRefused(o.Write(ctx, syncer.Write{Entity: "task_label", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": Raw(id), "label_id": Raw(label)}})); err != nil {
			return err
		}
	}
	for _, who := range t.Assignees {
		if err := unlessRefused(o.Write(ctx, syncer.Write{Entity: "task_assignee", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": Raw(id), "user_id": Raw(who)}})); err != nil {
			return err
		}
	}
	for _, item := range t.Checklist {
		if err := unlessRefused(o.CreateOnce(ctx, "checklist_item", recur.HashID(id+"/"+item.ID),
			map[string]json.RawMessage{"task_id": Raw(id), "text": Raw(item.Text), "done": Raw(false), "sort_key": Raw(item.SortKey)})); err != nil {
			return err
		}
	}
	for field, value := range t.FieldValues {
		if err := unlessRefused(o.Write(ctx, syncer.Write{Entity: "custom_field_value", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": Raw(id), "field_id": Raw(field), "value": value}})); err != nil {
			return err
		}
	}
	return nil
}

// unlessRefused lets go of an error that says something was refused, and keeps any other.
func unlessRefused(err error) error {
	var refused *apperr.Error
	if errors.As(err, &refused) {
		return nil
	}
	return err
}

// OpenStatus is where a new task starts: a project's first to-do status, else its first
// that isn't done or canceled, else its first.
func OpenStatus(statuses []syncer.AutomationStatusRow) (string, bool) {
	for _, st := range statuses {
		if st.Category == "todo" {
			return st.ID, true
		}
	}
	for _, st := range statuses {
		if !Finished(st.Category) {
			return st.ID, true
		}
	}
	if len(statuses) > 0 {
		return statuses[0].ID, true
	}
	return "", false
}

// Move takes a task and its subtasks to another project, or to the Inbox (project ""),
// each landing in the status of the same kind there, as moving one by hand does.
func (o Ops) Move(ctx context.Context, workspaceID string, t *syncer.TaskFacts, project string) (string, error) {
	if t.ParentID != "" {
		return "", apperr.New(apperr.Invalid, "A subtask moves with the task it's under.")
	}
	if project == t.ProjectID {
		return "", nil
	}
	var statuses []syncer.AutomationStatusRow
	if project != "" {
		open, err := o.Sync.ProjectOpen(ctx, workspaceID, project)
		if err != nil {
			return "", err
		}
		if statuses, err = o.Sync.StatusesOf(ctx, project); err != nil {
			return "", err
		}
		// Moved into the trash, a task would go when the project does.
		if !open || len(statuses) == 0 {
			return "", apperr.New(apperr.Invalid, "That project is archived, deleted or not there any more.")
		}
	}
	moveOne := func(id, category string) error {
		fields := map[string]json.RawMessage{"project_id": json.RawMessage("null"), "status_id": json.RawMessage("null")}
		if project != "" {
			status := statuses[0].ID
			for _, st := range statuses {
				if st.Category == category {
					status = st.ID
					break
				}
			}
			fields = map[string]json.RawMessage{"project_id": Raw(project), "status_id": Raw(status)}
		}
		return o.SetTask(ctx, id, fields)
	}
	// An Inbox task's category is its completion's (TaskFactsOf), as in the app.
	if err := moveOne(t.ID, t.Category); err != nil {
		return "", err
	}
	for _, id := range t.Subtasks {
		sub, there, err := o.Sync.TaskFactsOf(ctx, id)
		if err != nil {
			return "", err
		}
		if !there {
			continue
		}
		if err := moveOne(id, sub.Category); err != nil {
			return "", err
		}
	}
	t.ProjectID = project
	if project == "" {
		return "Moved it to the Inbox", nil
	}
	return "Moved it to another project", nil
}
