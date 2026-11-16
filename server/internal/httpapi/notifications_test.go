package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestNotificationsOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)

	// The owner's app listens on the socket.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	header := http.Header{"Origin": {owner.origin}}
	for _, c := range owner.http.Jar.Cookies(mustURL(s.URL)) {
		header.Add("Cookie", c.Name+"="+c.Value)
	}
	sock, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/api/v1/sync/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sock.CloseNow() }()
	read := func() string {
		t.Helper()
		_, raw, err := sock.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		return m["type"].(string)
	}
	if m := read(); m != "hello" {
		t.Fatalf("first message = %v", m)
	}

	// Dana accepts an invitation; the owner hears of it.
	owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "dana@example.com", "role": "member"})
	dana := s.browser(t)
	dana.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Dana"})
	for m := read(); m != "notify"; m = read() {
		if m != "workspaces" {
			t.Fatalf("message = %v", m)
		}
	}
	_, page := owner.do("GET", "/me/notifications", nil)
	items := page["items"].([]any)
	if page["unread"] != float64(1) || len(items) != 1 {
		t.Fatalf("notifications = %v", page)
	}
	n := items[0].(map[string]any)
	if n["kind"] != "invite_accepted" || n["title"] != "Dana joined Team" || n["actorName"] != "Dana" || n["url"] != "/?ws="+wsID {
		t.Fatalf("notification = %v", n)
	}

	// Dana mentions the owner in a comment.
	_, me := owner.do("GET", "/me", nil)
	task, comment := uuid.NewString(), uuid.NewString()
	resp, body := dana.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("dana"),
			"fields": map[string]any{"title": "Repaint the porch", "sort_key": "a0"}},
		map[string]any{"opId": uuid.NewString(), "entity": "comment", "id": comment, "hlc": hlcNow("dana"),
			"fields": map[string]any{"task_id": task, "author_id": dana.userID(), "body": "Ready, @[Shawn](user:" + me["id"].(string) + ")?"}},
	}})
	if resp.StatusCode != 200 {
		t.Fatalf("push = %d %v", resp.StatusCode, body)
	}
	if _, err := s.runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	_, page = owner.do("GET", "/me/notifications?limit=1", nil)
	items = page["items"].([]any)
	n = items[0].(map[string]any)
	if page["unread"] != float64(2) || n["kind"] != "mention" || n["body"] != "Ready, @Shawn?" || page["next"] == nil {
		t.Fatalf("after a mention = %v", page)
	}
	if !strings.Contains(s.mail.lastText(), "> Ready, @Shawn?") {
		t.Fatalf("mention email = %q", s.mail.lastText())
	}
	if resp, _ := dana.do("POST", "/me/notifications/"+n["id"].(string)+"/read", nil); resp.StatusCode != 404 {
		t.Fatalf("someone else's read = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/me/notifications/"+n["id"].(string)+"/read", nil); resp.StatusCode != 204 {
		t.Fatalf("read = %d", resp.StatusCode)
	}
	if _, page = owner.do("GET", "/me/notifications", nil); page["unread"] != float64(1) {
		t.Fatalf("after one read = %v", page["unread"])
	}
	if resp, _ := owner.do("POST", "/me/notifications/read-all", map[string]any{"through": "nope"}); resp.StatusCode != 404 {
		t.Fatalf("read-all through no notification = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/me/notifications/read-all", map[string]any{"through": n["id"]}); resp.StatusCode != 204 {
		t.Fatalf("read-all = %d", resp.StatusCode)
	}
	if _, page = owner.do("GET", "/me/notifications", nil); page["unread"] != float64(0) {
		t.Fatalf("after all read = %v", page["unread"])
	}

	// Clearing: someone else's asks change nothing, and one's own empties the list.
	listed := func() int {
		_, page := owner.do("GET", "/me/notifications", nil)
		return len(page["items"].([]any))
	}
	count := listed()
	if count < 2 {
		t.Fatalf("notifications to clear = %d", count)
	}
	if resp, _ := dana.do("DELETE", "/me/notifications/"+n["id"].(string), nil); resp.StatusCode != 204 || listed() != count {
		t.Fatalf("someone else's clear = %d, left %d", resp.StatusCode, listed())
	}
	if resp, _ := dana.do("POST", "/me/notifications/clear", map[string]any{"through": n["id"]}); resp.StatusCode != 404 {
		t.Fatalf("someone else's clear-all = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/me/notifications/clear", map[string]any{"through": "nope"}); resp.StatusCode != 404 {
		t.Fatalf("clear-all through no notification = %d", resp.StatusCode)
	}
	_, page = owner.do("GET", "/me/notifications", nil)
	first := page["items"].([]any)[0].(map[string]any)["id"].(string)
	last := page["items"].([]any)[count-1].(map[string]any)["id"].(string)
	if resp, _ := owner.do("DELETE", "/me/notifications/"+last, nil); resp.StatusCode != 204 || listed() != count-1 {
		t.Fatalf("clear = %d, left %d", resp.StatusCode, listed())
	}
	if resp, _ := owner.do("POST", "/me/notifications/clear", map[string]any{"through": first}); resp.StatusCode != 204 || listed() != 0 {
		t.Fatalf("clear-all = %d, left %d", resp.StatusCode, listed())
	}
}

func TestNotificationSettingsOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)
	s.setup(t, c)

	_, set := c.do("GET", "/me/settings/notifications", nil)
	kinds := set["kinds"].(map[string]any)
	if set["timezone"] != "UTC" || set["push"] != true || set["email"] != true ||
		kinds["mention"].(map[string]any)["email"] != true || set["digest"].(map[string]any)["hour"] != float64(7) {
		t.Fatalf("settings = %v", set)
	}

	resp, me := c.do("PUT", "/me/timezone", map[string]string{"timezone": "America/Chicago"})
	if resp.StatusCode != 200 || me["timezone"] != "America/Chicago" {
		t.Fatalf("timezone = %d %v", resp.StatusCode, me)
	}
	if resp, _ := c.do("PUT", "/me/timezone", map[string]string{"timezone": "Mars/Olympus"}); resp.StatusCode != 400 {
		t.Fatalf("a made-up time zone = %d", resp.StatusCode)
	}
	if _, me := c.do("GET", "/me", nil); me["timezone"] != "America/Chicago" {
		t.Fatalf("/me = %v", me)
	}

	kinds["comment"] = map[string]any{"app": false, "push": false, "email": true}
	kinds["mention"] = map[string]any{"app": false, "push": false, "email": false}
	resp, saved := c.do("PUT", "/me/settings/notifications", map[string]any{"kinds": kinds, "digest": map[string]any{"enabled": true, "hour": 18}})
	sk := saved["kinds"].(map[string]any)
	if resp.StatusCode != 200 || sk["comment"].(map[string]any)["email"] != true || sk["mention"].(map[string]any)["app"] != true ||
		saved["digest"].(map[string]any)["hour"] != float64(18) || saved["timezone"] != "America/Chicago" {
		t.Fatalf("saved = %d %v", resp.StatusCode, saved)
	}
	if resp, _ := c.do("PUT", "/me/settings/notifications", map[string]any{"kinds": kinds, "digest": map[string]any{"enabled": true, "hour": 30}}); resp.StatusCode != 400 {
		t.Fatalf("hour 30 = %d", resp.StatusCode)
	}

	// Test emails, a few at most.
	for i := range 3 {
		if resp, _ := c.do("POST", "/me/settings/notifications/test-email", nil); resp.StatusCode != 204 {
			t.Fatalf("test email %d = %d", i, resp.StatusCode)
		}
	}
	if !strings.Contains(s.mail.lastText(), "test email") {
		t.Fatalf("test email = %q", s.mail.lastText())
	}
	if resp, _ := c.do("POST", "/me/settings/notifications/test-email", nil); resp.StatusCode != 429 {
		t.Fatalf("a fourth test email = %d", resp.StatusCode)
	}
}

// userID is the signed-in user's ID.
func (c *client) userID() string {
	c.t.Helper()
	_, me := c.do("GET", "/me", nil)
	id, _ := me["id"].(string)
	if id == "" {
		c.t.Fatalf("/me = %v", me)
	}
	return id
}
