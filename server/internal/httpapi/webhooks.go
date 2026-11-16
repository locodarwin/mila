package httpapi

import (
	"net/http"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/webhooks"
)

func webhookJSON(h sqlitedb.Webhook) api.Webhook {
	out := api.Webhook{Id: h.ID, Url: h.Url, CreatedAt: h.CreatedAt, State: api.Working}
	for _, f := range webhooks.FamiliesOf(h) {
		out.Events = append(out.Events, api.WebhookFamily(f))
	}
	if h.ProjectID.Valid {
		out.ProjectId = &h.ProjectID.String
	}
	if h.FailingSince.Valid {
		out.FailingSince = &h.FailingSince.Time
		out.State = api.Failing
	}
	switch {
	case h.DisabledAt.Valid:
		out.State = api.Stopped
	case h.PausedAt.Valid:
		out.State = api.Paused
	}
	return out
}

func families(in []api.WebhookFamily) []string {
	out := make([]string, len(in))
	for i, f := range in {
		out[i] = string(f)
	}
	return out
}

func (h *handlers) ListWebhooks(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.hooks.List(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Webhook, len(list))
	for i, hook := range list {
		out[i] = webhookJSON(hook)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateWebhook(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.CreateWebhookJSONRequestBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	in := webhooks.Input{URL: body.Url, Events: families(body.Events)}
	if body.ProjectId != nil {
		in.ProjectID = *body.ProjectId
	}
	hook, err := h.hooks.Create(r.Context(), a.User.ID, workspaceID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.acc.NoteWebhook(r.Context(), a.User.ID, hook.Url, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.NewWebhook{Webhook: webhookJSON(hook), Secret: hook.Secret})
}

func (h *handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId) {
	a := h.requireUser(w, r)
	var body api.UpdateWebhookJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	// A new address, or another's webhook taken on, is where the work goes.
	moving := body.Url != nil || (body.Paused != nil && !*body.Paused)
	if moving && !h.requireRecentSignIn(w, r, a) {
		return
	}
	p := webhooks.Patch{URL: body.Url, ProjectID: body.ProjectId, Paused: body.Paused}
	if body.Events != nil {
		events := families(*body.Events)
		p.Events = &events
	}
	hook, err := h.hooks.Update(r.Context(), a.User.ID, workspaceID, webhookID, p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if moving {
		if err := h.acc.NoteWebhook(r.Context(), a.User.ID, hook.Url, clientOf(r)); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, webhookJSON(hook))
}

func (h *handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.hooks.Delete(r.Context(), a.User.ID, workspaceID, webhookID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId) {
	a := h.requireUser(w, r)
	if a == nil || !h.requireRecentSignIn(w, r, a) {
		return
	}
	hook, err := h.hooks.RotateSecret(r.Context(), a.User.ID, workspaceID, webhookID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.acc.NoteWebhook(r.Context(), a.User.ID, hook.Url, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.NewWebhook{Webhook: webhookJSON(hook), Secret: hook.Secret})
}

func (h *handlers) TestWebhook(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.testHookLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "180")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many test deliveries. Try again in a few minutes.")
		return
	}
	if err := h.hooks.Test(r.Context(), a.User.ID, workspaceID, webhookID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handlers) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.hooks.Deliveries(r.Context(), a.User.ID, workspaceID, webhookID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.WebhookDelivery, len(list))
	for i, d := range list {
		out[i] = api.WebhookDelivery{
			Id: d.ID, Event: d.Event, Summary: d.Summary, Status: api.WebhookDeliveryStatus(d.Status),
			Attempts: int(d.Attempts), ResponseBody: d.ResponseBody, CreatedAt: d.CreatedAt,
		}
		if d.ResponseCode.Valid {
			code := int(d.ResponseCode.Int64)
			out[i].ResponseCode = &code
		}
		if d.AttemptedAt.Valid {
			out[i].AttemptedAt = &d.AttemptedAt.Time
		}
		if d.NextAt.Valid && d.Status == "pending" {
			out[i].NextAt = &d.NextAt.Time
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ResendWebhookDelivery(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, webhookID api.WebhookId, deliveryID string) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.hooks.Resend(r.Context(), a.User.ID, workspaceID, webhookID, deliveryID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
