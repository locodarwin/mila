package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/ical"
)

func feedJSON(f ical.Feed) api.CalendarFeed {
	out := api.CalendarFeed{Id: f.ID, Scope: api.CalendarFeedScope(f.Scope), Name: f.Name, CreatedAt: f.CreatedAt, LastUsedAt: f.LastUsedAt}
	if f.TargetID != "" {
		out.TargetId = &f.TargetID
	}
	return out
}

func (h *handlers) ListCalendarFeeds(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	feeds, err := h.calendar.List(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.CalendarFeed, len(feeds))
	for i, f := range feeds {
		out[i] = feedJSON(f)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateCalendarFeed(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.CreateCalendarFeedJSONRequestBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	var target string
	if body.TargetId != nil {
		target = *body.TargetId
	}
	f, link, err := h.calendar.Create(r.Context(), a.User.ID, workspaceID, string(body.Scope), target)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.NewCalendarFeed{Feed: feedJSON(f), Url: link})
}

func (h *handlers) RevokeCalendarFeed(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, feedID api.FeedId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.calendar.Revoke(r.Context(), a.User.ID, workspaceID, feedID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// calendarFeed serves GET /ical/<token>.ics to calendar apps, which have
// no session: the token is the key. Every reason there is no feed is the
// same 404.
func (h *handlers) calendarFeed(w http.ResponseWriter, r *http.Request) {
	if !h.feedLimit.Allow(rateKey(r)) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too many requests. Try again in a minute.", http.StatusTooManyRequests)
		return
	}
	token, ok := strings.CutSuffix(r.PathValue("file"), ".ics")
	if !ok {
		http.Error(w, "No such calendar feed.", http.StatusNotFound)
		return
	}
	body, err := h.calendar.Calendar(r.Context(), token)
	if apperr.KindOf(err) == apperr.NotFound {
		http.Error(w, "No such calendar feed.", http.StatusNotFound)
		return
	}
	if err != nil {
		h.log.Error("calendar feed failed", "err", err)
		http.Error(w, "Something went wrong on the server.", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	hd := w.Header()
	hd.Set("ETag", etag)
	hd.Set("Cache-Control", "private, max-age=300")
	if noneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hd.Set("Content-Type", "text/calendar; charset=utf-8")
	_, _ = w.Write(body)
}

// noneMatch reports whether an If-None-Match header names etag, which
// means the client's copy is current. The comparison is weak, as RFC 9110
// asks for this header.
func noneMatch(header, etag string) bool {
	for tag := range strings.SplitSeq(header, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == "*" || tag == etag {
			return true
		}
	}
	return false
}
