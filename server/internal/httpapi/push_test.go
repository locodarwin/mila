package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestPushOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)
	ctx := context.Background()

	resp, info := owner.do("GET", "/me/push", nil)
	if resp.StatusCode != 200 || info["devices"] != float64(0) || len(info["publicKey"].(string)) != 87 {
		t.Fatalf("push info = %d %v", resp.StatusCode, info)
	}
	if resp, _ := s.browser(t).do("GET", "/me/push", nil); resp.StatusCode != 401 {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}

	// The owner turns push on in a browser, as PushSubscription.toJSON() gives it.
	sub := s.push.Subscribe("laptop")
	body := map[string]any{"endpoint": sub.Endpoint, "expirationTime": nil, "keys": map[string]string{"p256dh": sub.P256dh, "auth": sub.Auth}}
	if resp, _ := owner.do("POST", "/me/push/subscribe", body); resp.StatusCode != 204 {
		t.Fatalf("subscribe = %d", resp.StatusCode)
	}
	bad := map[string]any{"endpoint": sub.Endpoint, "keys": map[string]string{"p256dh": "AAAA", "auth": sub.Auth}}
	if resp, e := owner.do("POST", "/me/push/subscribe", bad); resp.StatusCode != 400 || e["message"] != "The browser's push key is not valid." {
		t.Fatalf("bad key = %d %v", resp.StatusCode, e)
	}
	if _, info := owner.do("GET", "/me/push", nil); info["devices"] != float64(1) {
		t.Fatalf("devices = %v", info["devices"])
	}

	// Dana joins and mentions the owner: mentions go by push by default.
	owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "dana@example.com", "role": "member"})
	dana := s.browser(t)
	dana.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Dana"})
	_, me := owner.do("GET", "/me", nil)
	task := uuid.NewString()
	dana.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("dana"),
			"fields": map[string]any{"title": "Repaint the porch", "sort_key": "a0"}},
		map[string]any{"opId": uuid.NewString(), "entity": "comment", "id": uuid.NewString(), "hlc": hlcNow("dana"),
			"fields": map[string]any{"task_id": task, "author_id": dana.userID(), "body": "Ready, @[Shawn](user:" + me["id"].(string) + ")?"}},
	}})
	if _, err := s.runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	got := s.push.Received()
	if len(got) != 1 {
		t.Fatalf("pushes = %+v", got)
	}
	if m := got[0]; m.Title != "Dana mentioned you in Repaint the porch" || m.Body != "Ready, @Shawn?" ||
		!strings.HasPrefix(m.URL, "/?task="+task+"&ws="+wsID+"&n=") || m.Tag != task || m.Urgency != "normal" {
		t.Fatalf("push = %+v", m)
	}

	// A test push reports on each device.
	resp, res := owner.do("POST", "/me/push/test", nil)
	if resp.StatusCode != 200 || res["sent"] != float64(1) || res["failed"] != float64(0) || res["removed"] != float64(0) {
		t.Fatalf("test push = %d %v", resp.StatusCode, res)
	}
	if got := s.push.Received(); len(got) != 1 || got[0].Title != "Mila test push" {
		t.Fatalf("test push received %+v", got)
	}
	for range 4 {
		owner.do("POST", "/me/push/test", nil)
	}
	if resp, e := owner.do("POST", "/me/push/test", nil); resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("sixth test push = %d %v", resp.StatusCode, e)
	}
	s.push.Received()

	// Someone else cannot turn off the owner's device.
	if resp, _ := dana.do("POST", "/me/push/unsubscribe", map[string]string{"endpoint": sub.Endpoint}); resp.StatusCode != 204 {
		t.Fatalf("unsubscribe = %d", resp.StatusCode)
	}
	if _, info := owner.do("GET", "/me/push", nil); info["devices"] != float64(1) {
		t.Fatalf("devices after someone else's unsubscribe = %v", info["devices"])
	}
	if resp, _ := owner.do("POST", "/me/push/unsubscribe", map[string]string{"endpoint": sub.Endpoint}); resp.StatusCode != 204 {
		t.Fatalf("unsubscribe = %d", resp.StatusCode)
	}
	if _, info := owner.do("GET", "/me/push", nil); info["devices"] != float64(0) {
		t.Fatalf("devices after unsubscribe = %v", info["devices"])
	}

	// Signing out stops pushes to that browser.
	owner.do("POST", "/me/push/subscribe", body)
	if resp, _ := owner.do("POST", "/auth/sign-out", nil); resp.StatusCode != 204 {
		t.Fatalf("sign out = %d", resp.StatusCode)
	}
	dana.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "comment", "id": uuid.NewString(), "hlc": hlcNow("dana"),
			"fields": map[string]any{"task_id": task, "author_id": dana.userID(), "body": "Still there, @[Shawn](user:" + me["id"].(string) + ")?"}},
	}})
	if _, err := s.runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := s.push.Received(); len(got) != 0 {
		t.Fatalf("a signed-out browser got %+v", got)
	}
}
