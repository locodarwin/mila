package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/locodarwin/mila/server/internal/actions"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// The API's actions (docs/PLAN.md section 11, Agent access): what the app
// does as one action, for scripts and agents.

// actionBody is the most an action's body may be: a page's text and some.
const actionBody = 512 << 10

// action reads an action's request strictly, so a field the action doesn't
// know (a misspelling, say) is refused rather than dropped, and says who it
// is for. False when it has answered already.
func (h *handlers) action(w http.ResponseWriter, r *http.Request, workspaceID string, batch *api.MilaBatch, key *api.IdempotencyKey, in any) (actions.Caller, bool) {
	a := h.apiUser(w, r, true)
	if a == nil {
		return actions.Caller{}, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, actionBody+1))
	if err != nil || len(body) > actionBody {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "The request body is too large.")
		return actions.Caller{}, false
	}
	if !h.spend(w, r, a.User.ID, len(body)) {
		return actions.Caller{}, false
	}
	if escapesNUL(body) {
		writeError(w, http.StatusBadRequest, "invalid", nulMessage)
		return actions.Caller{}, false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid for this action: "+err.Error()+".")
		return actions.Caller{}, false
	}
	c := actions.Caller{User: a.User.ID, Workspace: workspaceID}
	if batch != nil {
		if !syncer.ValidBatch(*batch) {
			writeError(w, http.StatusBadRequest, "invalid", "A Mila-Batch is 1 to 64 letters, digits and dashes.")
			return actions.Caller{}, false
		}
		c.Batch = *batch
	}
	if key != nil {
		if len(*key) > 200 {
			writeError(w, http.StatusBadRequest, "invalid", "The Idempotency-Key is longer than 200 characters.")
			return actions.Caller{}, false
		}
		c.Key = *key
	}
	return c, true
}

func taskJSON(t actions.Task) map[string]any {
	out := rowJSON(t.Row)
	out["status"] = t.Status
	out["project"] = t.Project
	out["labels"] = t.Labels
	out["assignees"] = t.Assignees
	return out
}

func (h *handlers) answerTask(w http.ResponseWriter, r *http.Request, done actions.Done, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := map[string]any{"task": taskJSON(done.Task), "batch": done.Batch, "said": done.Said}
	if done.Next != nil {
		out["next"] = taskJSON(*done.Next)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) AddTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.AddTaskActionParams) {
	var in actions.AddTask
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.AddTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) CompleteTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.CompleteTaskActionParams) {
	var in actions.TaskRef
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.CompleteTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) ReopenTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.ReopenTaskActionParams) {
	var in actions.TaskRef
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.ReopenTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) SetStatusAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.SetStatusActionParams) {
	var in actions.SetStatus
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.SetStatus(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) MoveTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.MoveTaskActionParams) {
	var in actions.MoveTask
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.MoveTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) AssignTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.AssignTaskActionParams) {
	var in actions.Assign
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.AssignTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) LabelTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.LabelTaskActionParams) {
	var in actions.Label
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		done, err := h.actions.LabelTask(r.Context(), c, in)
		h.answerTask(w, r, done, err)
	}
}

func (h *handlers) CreateProjectAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.CreateProjectActionParams) {
	var in actions.CreateProject
	c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in)
	if !ok {
		return
	}
	made, err := h.actions.CreateProject(r.Context(), c, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": rowJSON(made.Project), "statuses": made.Statuses, "batch": made.Batch})
}

func (h *handlers) DeleteStatusAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.DeleteStatusActionParams) {
	var in actions.DeleteStatus
	c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in)
	if !ok {
		return
	}
	out, err := h.actions.DeleteStatus(r.Context(), c, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": out.Moved, "to": out.To, "batch": out.Batch})
}

func (h *handlers) answerText(w http.ResponseWriter, r *http.Request, out actions.Written, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"row": rowJSON(out.Row), "batch": out.Batch, "said": out.Said})
}

func (h *handlers) AppendNotesAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.AppendNotesActionParams) {
	var in actions.Text
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		out, err := h.actions.AppendNotes(r.Context(), c, in)
		h.answerText(w, r, out, err)
	}
}

func (h *handlers) ReplaceSectionAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.ReplaceSectionActionParams) {
	var in actions.Text
	if c, ok := h.action(w, r, workspaceID, params.MilaBatch, params.IdempotencyKey, &in); ok {
		out, err := h.actions.ReplaceSection(r.Context(), c, in)
		h.answerText(w, r, out, err)
	}
}

func (h *handlers) GetTaskAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.GetTaskActionParams) {
	a := h.apiUser(w, r, false)
	if a == nil {
		return
	}
	whole, err := h.actions.GetTask(r.Context(), actions.Caller{User: a.User.ID, Workspace: workspaceID}, params.Task)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	subtasks := make([]map[string]any, len(whole.Subtasks))
	for i, t := range whole.Subtasks {
		subtasks[i] = taskJSON(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": taskJSON(whole.Task), "checklist": whole.Checklist,
		"comments": whole.Comments, "subtasks": subtasks, "in_trash": whole.InTrash})
}

func (h *handlers) FindTasksAction(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.FindTasksActionParams) {
	a := h.apiUser(w, r, false)
	if a == nil {
		return
	}
	in := actions.Find{}
	if params.List != nil {
		in.List = string(*params.List)
	}
	if params.View != nil {
		in.View = *params.View
	}
	if params.Project != nil {
		in.Project = *params.Project
	}
	if params.DueFrom != nil {
		in.DueFrom = params.DueFrom.String()
	}
	if params.DueTo != nil {
		in.DueTo = params.DueTo.String()
	}
	if params.State != nil {
		in.State = string(*params.State)
	}
	if params.Words != nil {
		in.Words = *params.Words
	}
	if params.Assignee != nil {
		in.Assignee = *params.Assignee
	}
	if params.Kind != nil {
		for k := range strings.SplitSeq(*params.Kind, ",") {
			if k = strings.TrimSpace(k); k != "" {
				in.Kinds = append(in.Kinds, k)
			}
		}
	}
	if params.Limit != nil {
		in.Limit = *params.Limit
	}
	if params.After != nil {
		in.After = *params.After
	}
	done, ok := h.heavy(w, r, a.User.ID)
	if !ok {
		return
	}
	defer done()
	found, err := h.actions.FindTasks(r.Context(), actions.Caller{User: a.User.ID, Workspace: workspaceID}, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]map[string]any, len(found.Tasks))
	for i, t := range found.Tasks {
		items[i] = taskJSON(t)
	}
	out := map[string]any{"items": items, "today": found.Today, "more": found.More}
	if found.Next != "" {
		out["next"] = found.Next
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) UndoBatch(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.apiUser(w, r, true)
	var body api.UndoBatchJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	out, err := h.sync.UndoBatch(r.Context(), a.User.ID, workspaceID, body.Batch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
