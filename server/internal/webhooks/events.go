package webhooks

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// eventOf says what an accepted change is called, and to which family it belongs;
// "" for a change no webhook hears about. Private rows (My Day, reminders, personal
// views) are not among these, so they never go out.
func eventOf(a syncer.Applied) (family, event string) {
	changed := func(name string) bool { return slices.Contains(a.Accepted, name) }
	deleted := changed("deleted_at") && a.Values["deleted_at"] != nil
	switch a.Entity {
	case "task":
		// Notes as they are typed would be a delivery every few seconds.
		if a.FromDoc {
			return "", ""
		}
		switch {
		case a.Created:
			return "task", "task.created"
		case deleted:
			return "task", "task.deleted"
		case changed("completed_at") && a.Values["completed_at"] != nil:
			return "task", "task.completed"
		case changed("completed_at"):
			return "task", "task.reopened"
		case len(a.Accepted) == 1 && changed("sort_key"):
			return "", "" // moved within its list
		}
		return "task", "task.changed"
	case "comment", "page_comment":
		switch {
		case a.Created:
			return "comment", "comment.added"
		case deleted:
			return "comment", "comment.deleted"
		case changed("body"):
			return "comment", "comment.edited"
		}
	case "project":
		switch {
		case a.Created:
			return "project", "project.created"
		case deleted:
			return "project", "project.deleted"
		case changed("archived_at") && a.Values["archived_at"] != nil:
			return "project", "project.archived"
		case len(a.Accepted) == 1 && changed("sort_key"):
			return "", ""
		}
		return "project", "project.changed"
	case "page":
		switch {
		case a.FromDoc:
			return "", ""
		case a.Created:
			return "page", "page.created"
		case deleted:
			return "page", "page.deleted"
		case changed("title"):
			return "page", "page.renamed"
		}
	case "time_entry", "project_time_entry":
		// A running timer is not time logged until it stops.
		running, _ := a.Values["running"].(bool)
		switch {
		case deleted:
			return "time", "time.deleted"
		case running:
		case a.Created || changed("running"):
			return "time", "time.logged"
		case changed("minutes") || changed("note") || changed("day"):
			return "time", "time.changed"
		}
	}
	return "", ""
}

// summaryOf is the name a delivery shows under in the log.
func summaryOf(a syncer.Applied) string {
	for _, field := range []string{"title", "name"} {
		if s, ok := a.Values[field].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// projectOf is the project a change belongs to; "" for none or for not knowing.
func projectOf(ctx context.Context, q store.Queries, a syncer.Applied) (string, error) {
	if a.Entity == "project" {
		return a.ID, nil
	}
	if p, ok := a.Values["project_id"].(string); ok {
		return p, nil
	}
	if task, ok := a.Values["task_id"].(string); ok {
		return q.ProjectOfTask(ctx, task)
	}
	if page, ok := a.Values["page_id"].(string); ok {
		return q.ProjectOfPage(ctx, page)
	}
	return "", nil
}

// Applied writes a delivery for each webhook that hears about this change, in the
// change's own transaction.
func (s *Service) Applied(ctx context.Context, q store.Queries, a syncer.Applied) error {
	family, event := eventOf(a)
	if family == "" {
		return nil
	}
	hooks, err := q.ListLiveWebhooks(ctx, a.WorkspaceID)
	if err != nil || len(hooks) == 0 {
		return err
	}
	project, knowProject := "", false
	var payload map[string]any
	for _, h := range hooks {
		if !slices.Contains(FamiliesOf(h), family) {
			continue
		}
		if h.ProjectID.Valid {
			if !knowProject {
				if project, err = projectOf(ctx, q, a); err != nil {
					return err
				}
				knowProject = true
			}
			if project != h.ProjectID.String {
				continue
			}
		}
		if payload == nil {
			if payload, err = s.payload(ctx, q, a, event); err != nil {
				return err
			}
		}
		id := uuid.Must(uuid.NewV7()).String()
		payload["id"] = id
		body, err := encode(payload)
		if err != nil {
			return err
		}
		if err := s.enqueue(ctx, q, h.ID, id, event, summaryOf(a), body, s.jobs.Now()); err != nil {
			return err
		}
	}
	return nil
}

// Committed sends what the change queued.
func (s *Service) Committed() { s.jobs.Wake() }

func (s *Service) payload(ctx context.Context, q store.Queries, a syncer.Applied, event string) (map[string]any, error) {
	actor := map[string]any{"id": a.ActorID}
	user, err := q.GetUser(ctx, a.ActorID)
	if err != nil {
		return nil, err
	}
	actor["name"] = user.Name
	if a.Via != "" {
		actor["via"] = a.Via
	}
	data := a.Fields()
	data["id"] = a.ID
	// Text someone deleted does not go out as they delete it.
	if strings.HasSuffix(event, ".deleted") {
		delete(data, "body")
		delete(data, "notes")
	}
	changed := slices.Clone(a.Accepted)
	if changed == nil {
		changed = []string{}
	}
	out := map[string]any{
		"event": event, "at": a.At.UTC().Format(time.RFC3339Nano), "workspace_id": a.WorkspaceID,
		"actor": actor, "data": data, "changed": changed,
	}
	link, err := s.linkTo(ctx, q, a)
	if err != nil {
		return nil, err
	}
	if link != "" {
		out["url"] = link
	}
	return out, nil
}

func (s *Service) linkTo(ctx context.Context, q store.Queries, a syncer.Applied) (string, error) {
	if a.Entity == "page_comment" {
		page, _ := a.Values["page_id"].(string)
		thread, _ := a.Values["thread_id"].(string)
		project, err := q.ProjectOfPage(ctx, page)
		if err != nil {
			return "", err
		}
		query := "?page=" + page + "&thread=" + thread + "&ws=" + a.WorkspaceID
		if project == "" {
			return s.baseURL + "/wiki" + query, nil
		}
		return s.baseURL + "/projects/" + project + query + "&tab=wiki", nil
	}
	base := s.baseURL + "/?ws=" + a.WorkspaceID
	switch a.Entity {
	case "task":
		return base + "&task=" + a.ID, nil
	case "comment", "time_entry":
		if task, ok := a.Values["task_id"].(string); ok {
			return base + "&task=" + task, nil
		}
	case "project_time_entry":
		if project, ok := a.Values["project_id"].(string); ok {
			return s.baseURL + "/projects/" + project + "?ws=" + a.WorkspaceID + "&tab=time", nil
		}
	}
	return "", nil
}

// encode makes a delivery's body, leaving out long text if it would be too large.
func encode(payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil || len(body) <= maxBody {
		return body, err
	}
	data, _ := payload["data"].(map[string]any)
	trimmed := make(map[string]any, len(data))
	for k, v := range data {
		if text, ok := v.(string); ok && len(text) > 1024 {
			continue
		}
		trimmed[k] = v
	}
	cut := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		cut[k] = v
	}
	cut["data"], cut["truncated"] = trimmed, true
	return json.Marshal(cut)
}

var _ syncer.Watcher = (*Service)(nil)
