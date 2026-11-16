package httpapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// asToken sends a request as a script would: the token, and no cookie or Origin.
func asToken(t *testing.T, s *testServer, secret, method, path string, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, s.URL+"/api/v1"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var text strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		text.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, text.String()
}

func TestAPITokens(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		_, other := owner.do("POST", "/workspaces", map[string]string{"name": "Other"})
		wsID, otherID := ws["id"].(string), other["id"].(string)
		tasks := "/workspaces/" + wsID + "/tasks"

		make := func(body map[string]any) string {
			t.Helper()
			resp, made := owner.do("POST", "/me/tokens", body)
			if resp.StatusCode != 201 {
				t.Fatalf("make = %d %v", resp.StatusCode, made)
			}
			secret := made["secret"].(string)
			if !strings.HasPrefix(secret, "mila_") || len(secret) < 40 {
				t.Fatalf("secret = %q", secret)
			}
			return secret
		}
		reader := make(map[string]any{"name": "Reader", "canWrite": false})
		writer := make(map[string]any{"name": "Zapier", "canWrite": true, "workspaceId": wsID, "expiresInDays": 90})

		// The list never holds the secret.
		resp, list := owner.doList("GET", "/me/tokens")
		if resp.StatusCode != 200 || len(list) != 2 || list[0]["name"] != "Zapier" || list[0]["secret"] != nil || list[0]["expiresAt"] == nil {
			t.Fatalf("list = %v", list)
		}
		// Nor the database: only its fingerprint.
		var kept string
		query := "SELECT token_hash FROM api_tokens WHERE name = 'Reader'"
		if err := d.QueryRowContext(t.Context(), query).Scan(&kept); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(kept, reader) || kept == reader {
			t.Fatal("the token itself is stored")
		}

		// In the security log.
		_, page := owner.do("GET", "/me/security-events", nil)
		if !strings.Contains(fmt.Sprint(page), "token_made") {
			t.Fatalf("not in the security log: %v", page)
		}

		// Reads and writes, as far as it may.
		if code, body := asToken(t, s, reader, "GET", tasks, ""); code != 200 {
			t.Fatalf("read = %d %s", code, body)
		}
		if code, body := asToken(t, s, reader, "POST", tasks, `{"title":"No"}`); code != 403 || !strings.Contains(body, "read only") {
			t.Fatalf("write with read only = %d %s", code, body)
		}
		code, body := asToken(t, s, writer, "POST", tasks, `{"title":"From a script"}`)
		if code != 201 {
			t.Fatalf("write = %d %s", code, body)
		}

		// One workspace means that one.
		if code, _ := asToken(t, s, writer, "GET", "/workspaces/"+otherID+"/tasks", ""); code != 403 {
			t.Fatalf("other workspace = %d", code)
		}
		if code, body := asToken(t, s, writer, "GET", "/workspaces", ""); code != 200 || strings.Contains(body, otherID) || !strings.Contains(body, wsID) {
			t.Fatalf("workspaces = %d %s", code, body)
		}
		if code, body := asToken(t, s, reader, "GET", "/workspaces", ""); code != 200 || !strings.Contains(body, otherID) {
			t.Fatalf("every workspace = %d %s", code, body)
		}

		// What needs the person, even with the strongest token.
		strong := make(map[string]any{"name": "Strong", "canWrite": true})
		for _, c := range []struct{ method, path string }{
			{"GET", "/me/tokens"}, {"POST", "/me/tokens"}, {"DELETE", "/me/tokens/" + uuid.NewString()},
			{"GET", "/me/sessions"}, {"GET", "/me/passkeys"}, {"POST", "/me/delete"}, {"POST", "/me/email"},
			{"GET", "/me/security-events"}, {"PATCH", "/me/settings/preferences"}, {"POST", "/auth/sign-out"},
			{"GET", "/admin/users"}, {"GET", "/admin/status"}, {"POST", "/admin/backups"},
			{"GET", "/workspaces/" + wsID + "/webhooks"}, {"POST", "/workspaces/" + wsID + "/webhooks"},
			{"GET", "/workspaces/" + wsID + "/sync/bootstrap?" + syncQ}, {"POST", "/workspaces/" + wsID + "/sync/push"},
			{"POST", "/workspaces/" + wsID + "/invites"}, {"PATCH", "/workspaces/" + wsID + "/members/" + owner.userID()},
			{"DELETE", "/workspaces/" + wsID}, {"PATCH", "/workspaces/" + wsID}, {"GET", "/workspaces/" + wsID + "/calendar-feeds"},
			{"POST", "/workspaces"}, {"POST", "/me/imports"},
			{"PUT", "/workspaces/" + wsID + "/projects/" + uuid.NewString() + "/guests/" + owner.userID()},
			{"DELETE", "/workspaces/" + wsID + "/projects/" + uuid.NewString() + "/guests/" + owner.userID()},
			{"GET", "/workspaces/" + wsID + "/tasks/" + uuid.NewString() + "/anything"},
			{"GET", "/workspaces/" + wsID + "/tasks/" + uuid.NewString() + "/activity/more"},
		} {
			if code, body := asToken(t, s, strong, c.method, c.path, ""); code != 403 || !strings.Contains(body, "token_not_allowed") {
				t.Errorf("%s %s = %d %s", c.method, c.path, code, body)
			}
		}
		// An export takes a token that can write.
		if code, _ := asToken(t, s, reader, "POST", "/workspaces/"+wsID+"/exports", `{"format":"json"}`); code != 403 {
			t.Errorf("export with read only = %d", code)
		}
		if code, body := asToken(t, s, reader, "GET", "/me/exports", ""); code != 403 {
			t.Errorf("exports with read only = %d %s", code, body)
		}
		if code, body := asToken(t, s, strong, "POST", "/workspaces/"+wsID+"/exports", `{"format":"json"}`); code != 202 {
			t.Errorf("export = %d %s", code, body)
		}

		// A token that is no good says so, with no fallback to anything else.
		for _, bad := range []string{"mila_nonsense", "nonsense", ""} {
			req, _ := http.NewRequest("GET", s.URL+"/api/v1"+tasks, nil)
			req.Header.Set("Authorization", "Bearer "+bad)
			r, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = r.Body.Close()
			if r.StatusCode != 401 {
				t.Errorf("%q = %d", bad, r.StatusCode)
			}
		}

		// Expired.
		expiring := make(map[string]any{"name": "Short", "canWrite": false, "expiresInDays": 1})
		past := time.Now().UTC().Add(-time.Hour)
		update := "UPDATE api_tokens SET expires_at = ? WHERE name = 'Short'"
		if d.Dialect == db.Postgres {
			update = "UPDATE api_tokens SET expires_at = $1 WHERE name = 'Short'"
		}
		if _, err := d.ExecContext(t.Context(), update, sql.NullTime{Time: past, Valid: true}); err != nil {
			t.Fatal(err)
		}
		if code, _ := asToken(t, s, expiring, "GET", tasks, ""); code != 401 {
			t.Errorf("expired = %d", code)
		}

		// Revoked, at once.
		_, list = owner.doList("GET", "/me/tokens")
		var readerID string
		for _, l := range list {
			if l["name"] == "Reader" {
				readerID = l["id"].(string)
				if l["lastUsedAt"] == nil {
					t.Errorf("last used not noted: %v", l)
				}
			}
		}
		if resp, _ := owner.do("DELETE", "/me/tokens/"+readerID, nil); resp.StatusCode != 204 {
			t.Fatalf("revoke = %d", resp.StatusCode)
		}
		if code, _ := asToken(t, s, reader, "GET", tasks, ""); code != 401 {
			t.Errorf("revoked = %d", code)
		}
		if resp, _ := s.browser(t).do("GET", "/me/tokens", nil); resp.StatusCode != 401 {
			t.Errorf("signed out list = %d", resp.StatusCode)
		}
	})
}

func TestATokenHasOnlyItsOwnersRights(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		member := s.join(t, owner, wsID, "dana@example.com", "Dana", "member")
		_, made := member.do("POST", "/me/tokens", map[string]any{"name": "Dana's", "canWrite": true})
		secret := made["secret"].(string)
		_, scoped := member.do("POST", "/me/tokens", map[string]any{"name": "Dana's team", "canWrite": true, "workspaceId": wsID})
		tasks := "/workspaces/" + wsID + "/tasks"

		// A change is the person's, marked with the token it came through.
		code, body := asToken(t, s, secret, "POST", tasks, `{"title":"Scripted"}`)
		if code != 201 {
			t.Fatalf("write = %d %s", code, body)
		}
		id := body[strings.Index(body, `"id":"`)+6:]
		id = id[:strings.Index(id, `"`)]
		_, events := owner.doList("GET", tasks+"/"+id+"/activity")
		if len(events) == 0 || events[0]["actorName"] != "Dana" || events[0]["via"] != "Dana's" || events[0]["fields"].(map[string]any)["_via"] != nil {
			t.Fatalf("activity = %v", events)
		}

		// Nothing another workspace of someone else's.
		_, private := owner.do("POST", "/workspaces", map[string]string{"name": "Mine alone"})
		if code, _ := asToken(t, s, secret, "GET", "/workspaces/"+private["id"].(string)+"/tasks", ""); code != 404 && code != 403 {
			t.Fatalf("not theirs = %d", code)
		}
		// A workspace someone can't reach can't have a token.
		if resp, _ := member.do("POST", "/me/tokens", map[string]any{"name": "No", "canWrite": false, "workspaceId": private["id"]}); resp.StatusCode != 404 {
			t.Fatalf("token for not theirs = %d", resp.StatusCode)
		}

		// Removed from the workspace, they are cut off at once, for good for one
		// made for it alone.
		if resp, _ := owner.do("DELETE", "/workspaces/"+wsID+"/members/"+member.userID(), nil); resp.StatusCode >= 300 {
			t.Fatalf("remove = %d", resp.StatusCode)
		}
		if code, _ := asToken(t, s, secret, "GET", tasks, ""); code != 404 && code != 403 {
			t.Fatalf("removed = %d", code)
		}
		if code, _ := asToken(t, s, scoped["secret"].(string), "GET", tasks, ""); code != 401 {
			t.Fatalf("one for that workspace = %d", code)
		}

		// An admin signing them out everywhere ends the rest.
		if resp, _ := owner.do("POST", "/admin/users/"+member.userID()+"/sign-out", nil); resp.StatusCode >= 300 {
			t.Fatalf("sign out = %d", resp.StatusCode)
		}
		if code, _ := asToken(t, s, secret, "GET", "/me", ""); code != 401 {
			t.Fatalf("after sign out = %d", code)
		}
	})
}

func TestWebhooksOverHTTP(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		hooks := "/workspaces/" + wsID + "/webhooks"

		member := s.join(t, owner, wsID, "dana@example.com", "Dana", "member")
		if resp, _ := member.do("GET", hooks, nil); resp.StatusCode != 403 {
			t.Fatalf("member's list = %d", resp.StatusCode)
		}

		resp, made := owner.do("POST", hooks, map[string]any{"url": "https://hooks.example.com/mila", "events": []string{"task", "comment"}})
		if resp.StatusCode != 201 || !strings.HasPrefix(made["secret"].(string), "whsec_") {
			t.Fatalf("make = %d %v", resp.StatusCode, made)
		}
		id := made["webhook"].(map[string]any)["id"].(string)
		_, list := owner.doList("GET", hooks)
		if len(list) != 1 || list[0]["secret"] != nil || list[0]["state"] != "working" {
			t.Fatalf("list = %v", list)
		}

		resp, changed := owner.do("PATCH", hooks+"/"+id, map[string]any{"paused": true})
		if resp.StatusCode != 200 || changed["state"] != "paused" {
			t.Fatalf("pause = %d %v", resp.StatusCode, changed)
		}
		resp, rotated := owner.do("POST", hooks+"/"+id+"/secret", nil)
		if resp.StatusCode != 200 || rotated["secret"] == made["secret"] {
			t.Fatalf("rotate = %d %v", resp.StatusCode, rotated)
		}
		if resp, _ := owner.do("POST", hooks+"/"+id+"/test", nil); resp.StatusCode != 202 {
			t.Fatalf("test = %d", resp.StatusCode)
		}
		resp, deliveries := owner.doList("GET", hooks+"/"+id+"/deliveries")
		if resp.StatusCode != 200 || len(deliveries) != 1 || deliveries[0]["event"] != "ping" {
			t.Fatalf("log = %d %v", resp.StatusCode, deliveries)
		}
		if resp, _ := owner.do("POST", hooks+"/"+id+"/deliveries/"+uuid.NewString()+"/resend", nil); resp.StatusCode != 404 {
			t.Fatalf("resend nothing = %d", resp.StatusCode)
		}
		if resp, _ := owner.do("DELETE", hooks+"/"+id, nil); resp.StatusCode != 204 {
			t.Fatalf("delete = %d", resp.StatusCode)
		}
	})
}

func TestAWebhookStopsWithWhoeverMadeIt(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		hooks := "/workspaces/" + wsID + "/webhooks"
		state := func() (string, string) {
			t.Helper()
			_, list := owner.doList("GET", hooks)
			return list[0]["id"].(string), list[0]["state"].(string)
		}

		// Removed.
		removed := s.join(t, owner, wsID, "ada@example.com", "Ada", "admin")
		if resp, made := removed.do("POST", hooks, map[string]any{"url": "https://hooks.example.com/ada", "events": []string{"task"}}); resp.StatusCode != 201 {
			t.Fatalf("make = %d %v", resp.StatusCode, made)
		}
		_, page := removed.do("GET", "/me/security-events", nil)
		if !strings.Contains(fmt.Sprint(page), "webhook_changed") {
			t.Fatalf("not in the security log: %v", page)
		}
		if resp, _ := owner.do("DELETE", "/workspaces/"+wsID+"/members/"+removed.userID(), nil); resp.StatusCode >= 300 {
			t.Fatalf("remove = %d", resp.StatusCode)
		}
		id, got := state()
		if got != "paused" {
			t.Fatalf("after removal = %s", got)
		}
		_, inbox := owner.do("GET", "/me/notifications", nil)
		if !strings.Contains(fmt.Sprint(inbox), "A webhook was paused") {
			t.Fatalf("not told: %v", inbox)
		}

		// Turned back on by someone who can, it is theirs.
		if resp, _ := owner.do("PATCH", hooks+"/"+id, map[string]any{"paused": false}); resp.StatusCode != 200 {
			t.Fatalf("back on = %d", resp.StatusCode)
		}
		if _, got := state(); got != "working" {
			t.Fatalf("back on = %s", got)
		}
		if resp, _ := owner.do("DELETE", hooks+"/"+id, nil); resp.StatusCode != 204 {
			t.Fatalf("delete = %d", resp.StatusCode)
		}

		// Made a plain member.
		demoted := s.join(t, owner, wsID, "bo@example.com", "Bo", "admin")
		demoted.do("POST", hooks, map[string]any{"url": "https://hooks.example.com/bo", "events": []string{"task"}})
		if resp, _ := owner.do("PATCH", "/workspaces/"+wsID+"/members/"+demoted.userID(), map[string]any{"role": "member"}); resp.StatusCode >= 300 {
			t.Fatalf("demote = %d", resp.StatusCode)
		}
		id, got = state()
		if got != "paused" {
			t.Fatalf("after demotion = %s", got)
		}
		owner.do("DELETE", hooks+"/"+id, nil)

		// Their account deleted.
		gone := s.join(t, owner, wsID, "cy@example.com", "Cy", "admin")
		gone.do("POST", hooks, map[string]any{"url": "https://hooks.example.com/cy", "events": []string{"task"}})
		if resp, body := gone.do("POST", "/me/delete", map[string]any{"email": "cy@example.com"}); resp.StatusCode >= 300 {
			t.Fatalf("delete account = %d %v", resp.StatusCode, body)
		}
		if _, got = state(); got != "paused" {
			t.Fatalf("after account deleted = %s", got)
		}
	})
}

func TestAHiddenGuestsTokenStaysHidden(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		root := "/workspaces/" + wsID
		_, project := owner.do("POST", root+"/projects", map[string]any{"name": "Shared"})
		pid := project["id"].(string)
		_, task := owner.do("POST", root+"/tasks", map[string]any{"title": "Shared task", "project_id": pid})

		ann := s.join(t, owner, wsID, "ann@example.com", "Ann", "guest")
		ben := s.join(t, owner, wsID, "ben@example.com", "Ben", "guest")
		share := root + "/projects/" + pid + "/guests/"
		if resp, _ := owner.do("PUT", share+ann.userID(), nil); resp.StatusCode >= 300 {
			t.Fatalf("share = %d", resp.StatusCode)
		}
		_, made := ann.do("POST", "/me/tokens", map[string]any{"name": "Ann's secret project", "canWrite": true})
		if code, body := asToken(t, s, made["secret"].(string), "PATCH", root+"/tasks/"+task["id"].(string), `{"title":"Changed"}`); code != 200 {
			t.Fatalf("write = %d %s", code, body)
		}
		// Ann loses the project, and Ben, who shares nothing with her, gets it.
		owner.do("DELETE", share+ann.userID(), nil)
		owner.do("PUT", share+ben.userID(), nil)
		_, events := ben.doList("GET", root+"/tasks/"+task["id"].(string)+"/activity")
		if strings.Contains(fmt.Sprint(events), "Ann") {
			t.Fatalf("a hidden guest shows: %v", events)
		}
		_, seen := owner.doList("GET", root+"/tasks/"+task["id"].(string)+"/activity")
		if !strings.Contains(fmt.Sprint(seen), "secret project") {
			t.Fatalf("a member should see it: %v", seen)
		}
	})
}
