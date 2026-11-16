package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
)

func (h *handlers) SetTimezone(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.SetTimezoneJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	var l accounts.Locale
	if body.Clock != nil {
		l.Clock = string(*body.Clock)
	}
	if body.DateOrder != nil {
		l.DateOrder = string(*body.DateOrder)
	}
	l.WeekStart = body.WeekStart
	u, err := h.acc.SetTimezone(r.Context(), a.User.ID, body.Timezone, l)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (h *handlers) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	// Read as raw values, so a preference left out stays as it is while
	// one sent as null (the accent) is reset.
	var patch map[string]json.RawMessage
	if a == nil || !h.decode(w, r, &patch) {
		return
	}
	p, _, err := h.acc.SavePreferences(r.Context(), a.User.ID, patch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preferencesJSON(p))
}

func (h *handlers) ListNotifications(w http.ResponseWriter, r *http.Request, params api.ListNotificationsParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	before, limit := "", 0
	if params.Before != nil {
		before = *params.Before
	}
	if params.Limit != nil {
		limit = *params.Limit
	}
	page, err := h.notify.List(r.Context(), a.User.ID, before, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.NotificationPage{Items: make([]api.Notification, len(page.Items)), Unread: page.Unread}
	if page.Next != "" {
		out.Next = &page.Next
	}
	for i, n := range page.Items {
		item := api.Notification{
			Id: n.ID, WorkspaceId: n.WorkspaceID, Kind: api.NotificationKind(n.Kind),
			Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt, Url: n.URL,
		}
		if n.TaskID.Valid {
			item.TaskId = &n.TaskID.String
		}
		if n.ActorID.Valid {
			item.ActorId, item.ActorName = &n.ActorID.String, &n.ActorName
			item.ActorAvatar = avatarPath(n.ActorAvatar)
		}
		if n.ReadAt.Valid {
			item.ReadAt = &n.ReadAt.Time
		}
		out.Items[i] = item
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) MarkNotificationRead(w http.ResponseWriter, r *http.Request, id api.NotificationId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.notify.MarkRead(r.Context(), a.User.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.MarkAllNotificationsReadJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.notify.MarkAllRead(r.Context(), a.User.ID, body.Through); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) DeleteNotification(w http.ResponseWriter, r *http.Request, id api.NotificationId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.notify.Clear(r.Context(), a.User.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) ClearNotifications(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.ClearNotificationsJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.notify.ClearAll(r.Context(), a.User.ID, body.Through); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// notificationSettings is api.NotificationSettings, built from the
// preferences, whose JSON has the same shape.
type notificationSettings struct {
	notify.Prefs
	Timezone string `json:"timezone"`
	Push     bool   `json:"push"`
	Email    bool   `json:"email"`
}

func (h *handlers) settingsJSON(p notify.Prefs, u store.User) notificationSettings {
	return notificationSettings{Prefs: p, Timezone: accounts.Timezone(u), Push: h.notify.CanPush(), Email: h.notify.CanEmail()}
}

func (h *handlers) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	p, u, err := h.notify.Prefs(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.settingsJSON(p, u))
}

func (h *handlers) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	// Weekly is a pointer here, so that a body without it (from an app
	// older than the weekly report) leaves it as it was.
	var body struct {
		notify.Prefs
		Weekly *notify.Weekly `json:"weekly"`
	}
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	p, u, err := h.notify.SavePrefs(r.Context(), a.User.ID, body.Prefs, body.Weekly)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.settingsJSON(p, u))
}

func (h *handlers) SendTestEmail(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.testMailLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "1200")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "That is enough test emails for now. Try again in half an hour.")
		return
	}
	if err := h.notify.SendTestEmail(r.Context(), a.User.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// ListAutomationRuns says what a workspace's automations did lately.
func (h *handlers) ListAutomationRuns(w http.ResponseWriter, r *http.Request, id api.WorkspaceId, params api.ListAutomationRunsParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.AutomationAccess(r.Context(), a.User.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	automation, limit := "", int64(0)
	if params.Automation != nil {
		automation = *params.Automation
	}
	if params.Limit != nil {
		limit = int64(*params.Limit)
	}
	runs, err := h.automations.Runs(r.Context(), id, automation, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]api.AutomationRun, len(runs))
	for i, run := range runs {
		items[i] = api.AutomationRun{Id: run.ID, AutomationId: run.AutomationID, TaskId: run.TaskID, TaskTitle: run.TaskTitle,
			Outcome: api.AutomationRunOutcome(run.Outcome), Detail: run.Detail, CreatedAt: run.CreatedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
