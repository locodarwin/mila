package syncer

import (
	"context"
	"time"

	"github.com/locodarwin/mila/server/internal/reports"
)

// WeeklyInput reads what the weekly report counts for a person in a
// workspace: every live task they see (in archived projects too, which the
// report leaves out itself, as the app's does), the projects, and the
// dependencies and time entries they see, on tasks and on projects
// themselves. Today and the zone are the
// caller's to fill in.
func (s *Service) WeeklyInput(ctx context.Context, userID, workspaceID string) (reports.Input, error) {
	in := reports.Input{Me: userID}
	tasks, err := s.Tasks(ctx, userID, workspaceID, TaskQuery{Archived: true, NoNotes: true})
	if err != nil {
		return in, err
	}
	all := func(entity string) ([]Record, error) {
		var out []Record
		q := Query{Limit: MaxLimit}
		for {
			page, err := s.List(ctx, userID, workspaceID, entity, q)
			if err != nil {
				return nil, err
			}
			out = append(out, page.Rows...)
			if page.Next == "" {
				return out, nil
			}
			q.After = page.Next
		}
	}
	projects, err := all("project")
	if err != nil {
		return in, err
	}
	for _, p := range projects {
		in.Projects = append(in.Projects, reports.Project{ID: p.ID, Name: str(p, "name"), InUse: p.Fields["archived_at"] == nil})
	}
	statuses, err := all("status")
	if err != nil {
		return in, err
	}
	kinds := map[string]string{}
	for _, st := range statuses {
		kinds[st.ID] = str(st, "category")
	}
	assignees, err := all("task_assignee")
	if err != nil {
		return in, err
	}
	assigned := map[string]bool{}
	mine := map[string]bool{}
	for _, a := range assignees {
		assigned[str(a, "task_id")] = true
		if str(a, "user_id") == userID {
			mine[str(a, "task_id")] = true
		}
	}
	// Records write times as RFC 3339 text, as REST sends them.
	at := func(r Record, field string) *time.Time {
		t, err := time.Parse(time.RFC3339Nano, str(r, field))
		if err != nil {
			return nil
		}
		return &t
	}
	for _, t := range tasks {
		task := reports.Task{ID: t.ID, Title: str(t, "title"), ProjectID: str(t, "project_id"), ParentID: str(t, "parent_id"),
			MadeAt: at(t, "made_at"), CompletedAt: at(t, "completed_at"), StartedAt: at(t, "started_at"),
			DueDate: str(t, "due_date"),
			Mine:    mine[t.ID] || (!assigned[t.ID] && str(t, "created_by") == userID)}
		if n, ok := t.Fields["estimate_minutes"].(int64); ok {
			task.Estimate = &n
		}
		task.Category = kinds[str(t, "status_id")]
		if task.Category == "" {
			task.Category = "todo"
			if task.CompletedAt != nil {
				task.Category = "done"
			}
		}
		in.Tasks = append(in.Tasks, task)
	}
	deps, err := all("task_dependency")
	if err != nil {
		return in, err
	}
	for _, d := range deps {
		in.Dependencies = append(in.Dependencies, reports.Dependency{TaskID: str(d, "task_id"), BlockerID: str(d, "blocker_id")})
	}
	entries, err := all("time_entry")
	if err != nil {
		return in, err
	}
	for _, e := range entries {
		minutes, _ := e.Fields["minutes"].(int64)
		in.Entries = append(in.Entries, reports.Entry{TaskID: str(e, "task_id"), UserID: str(e, "user_id"), Day: str(e, "day"), Minutes: minutes})
	}
	own, err := all("project_time_entry")
	if err != nil {
		return in, err
	}
	for _, e := range own {
		minutes, _ := e.Fields["minutes"].(int64)
		in.Entries = append(in.Entries, reports.Entry{ProjectID: str(e, "project_id"), UserID: str(e, "user_id"), Day: str(e, "day"), Minutes: minutes})
	}
	return in, nil
}
