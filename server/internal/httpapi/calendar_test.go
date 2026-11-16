package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// fetch gets a calendar feed the way a calendar app does: no cookies, no
// Origin.
func fetch(t *testing.T, link, etag string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", link, nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestCalendarFeedsOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)
	feeds := "/workspaces/" + wsID + "/calendar-feeds"

	task := uuid.NewString()
	owner.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("owner"),
			"fields": map[string]any{"title": "Repaint the porch", "sort_key": "a0", "due_date": "2026-11-02"}},
		map[string]any{"opId": uuid.NewString(), "entity": "task_assignee", "id": pairID(task, owner.userID()), "hlc": hlcNow("owner"),
			"fields": map[string]any{"task_id": task, "user_id": owner.userID()}},
	}})

	if resp, _ := s.browser(t).do("POST", feeds, map[string]string{"scope": "me"}); resp.StatusCode != 401 {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}
	if resp, e := owner.do("POST", feeds, map[string]string{"scope": "project", "targetId": uuid.NewString()}); resp.StatusCode != 404 || e["message"] == "" {
		t.Fatalf("unknown project = %d %v", resp.StatusCode, e)
	}
	resp, made := owner.do("POST", feeds, map[string]string{"scope": "me"})
	if resp.StatusCode != 201 {
		t.Fatalf("create = %d %v", resp.StatusCode, made)
	}
	link := made["url"].(string)
	feed := made["feed"].(map[string]any)
	if !strings.HasPrefix(link, s.URL+"/ical/") || !strings.HasSuffix(link, ".ics") || feed["name"] != "My tasks" || feed["scope"] != "me" {
		t.Fatalf("made = %v", made)
	}

	resp, body := fetch(t, link, "")
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/calendar; charset=utf-8" ||
		resp.Header.Get("Cache-Control") != "private, max-age=300" || len(etag) != 34 || !strings.HasPrefix(etag, `"`) {
		t.Fatalf("feed = %d %v", resp.StatusCode, resp.Header)
	}
	if !strings.HasPrefix(body, "BEGIN:VCALENDAR\r\n") || !strings.Contains(body, "SUMMARY:Repaint the porch\r\n") ||
		!strings.Contains(body, "DTSTART;VALUE=DATE:20261102\r\n") {
		t.Fatalf("body = %s", body)
	}
	// The calendar app's copy is current.
	for _, header := range []string{etag, "W/" + etag, `"other", ` + etag, "*"} {
		if resp, body := fetch(t, link, header); resp.StatusCode != 304 || body != "" || resp.Header.Get("ETag") != etag {
			t.Fatalf("If-None-Match %s = %d %q", header, resp.StatusCode, body)
		}
	}
	if resp, _ := fetch(t, link, `"stale"`); resp.StatusCode != 200 {
		t.Fatalf("stale = %d", resp.StatusCode)
	}
	// A change makes a new ETag.
	owner.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("owner"),
			"fields": map[string]any{"title": "Repaint the deck"}},
	}})
	if resp, body := fetch(t, link, etag); resp.StatusCode != 200 || resp.Header.Get("ETag") == etag || !strings.Contains(body, "Repaint the deck") {
		t.Fatalf("after a change = %d %s", resp.StatusCode, body)
	}

	resp, list := owner.doList("GET", feeds)
	if resp.StatusCode != 200 || len(list) != 1 || list[0]["id"] != feed["id"] || list[0]["lastUsedAt"] == nil {
		t.Fatalf("list = %d %v", resp.StatusCode, list)
	}

	// Someone outside the workspace can neither list nor revoke it.
	dana := s.browser(t)
	owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "dana@example.com", "role": "member"})
	_, personal := owner.do("POST", "/workspaces", map[string]string{"name": "Mine"})
	dana.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Dana"})
	if resp, _ := dana.do("GET", "/workspaces/"+personal["id"].(string)+"/calendar-feeds", nil); resp.StatusCode != 404 {
		t.Fatalf("outsider's list = %d", resp.StatusCode)
	}
	if resp, _ := dana.do("DELETE", feeds+"/"+feed["id"].(string), nil); resp.StatusCode != 404 {
		t.Fatalf("someone else's revoke = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("DELETE", feeds+"/"+feed["id"].(string), nil); resp.StatusCode != 204 {
		t.Fatalf("revoke = %d", resp.StatusCode)
	}
	for _, l := range []string{link, s.URL + "/ical/nothing.ics", strings.TrimSuffix(link, ".ics"), strings.TrimSuffix(link, ".ics") + ".ical"} {
		if resp, body := fetch(t, l, ""); resp.StatusCode != 404 || strings.Contains(body, "VCALENDAR") {
			t.Fatalf("%s = %d %s", l, resp.StatusCode, body)
		}
	}
	if _, list := owner.doList("GET", feeds); len(list) != 0 {
		t.Fatalf("list after revoking = %v", list)
	}

	// Leaving the workspace ends someone's feeds for good: invited back,
	// even as a guest, their old link stays dead.
	resp, made = dana.do("POST", feeds, map[string]string{"scope": "me"})
	if resp.StatusCode != 201 {
		t.Fatalf("member's feed = %d %v", resp.StatusCode, made)
	}
	danas := made["url"].(string)
	if resp, _ := fetch(t, danas, ""); resp.StatusCode != 200 {
		t.Fatalf("member's feed = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("DELETE", "/workspaces/"+wsID+"/members/"+dana.userID(), nil); resp.StatusCode != 204 {
		t.Fatalf("remove = %d", resp.StatusCode)
	}
	owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "dana@example.com", "role": "guest"})
	if _, back := dana.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t)}); back["workspaceId"] != wsID {
		t.Fatalf("rejoin = %v", back)
	}
	if resp, body := fetch(t, danas, ""); resp.StatusCode != 404 || strings.Contains(body, "VCALENDAR") {
		t.Fatalf("a feed from before leaving = %d %s", resp.StatusCode, body)
	}

	// Fetches are limited per address.
	limited := false
	for range 200 {
		if resp, _ := fetch(t, link, ""); resp.StatusCode == 429 {
			limited = resp.Header.Get("Retry-After") == "60"
			break
		}
	}
	if !limited {
		t.Fatal("200 fetches in a row were never limited")
	}
}

// pairID is the ID of a task's assignee or label row (pairId in the app).
func pairID(a, b string) string { return ids.Pair(a, b) }
