package httpapi

import (
	"net/http"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/push"
)

func (h *handlers) GetPush(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	n, err := h.push.Devices(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.PushInfo{PublicKey: h.push.PublicKey(), Devices: n})
}

func (h *handlers) SubscribePush(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.SubscribePushJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	sub := push.Subscription{Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth}
	if err := h.push.Subscribe(r.Context(), a.User.ID, a.Session.ID, userAgent(r), sub); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) UnsubscribePush(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.UnsubscribePushJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.push.Unsubscribe(r.Context(), a.User.ID, body.Endpoint); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) SendTestPush(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.testPushLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "That is enough test pushes for now. Try again in a minute.")
		return
	}
	res, err := h.push.SendTest(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.PushTestResult{Sent: res.Sent, Failed: res.Failed, Removed: res.Removed})
}
