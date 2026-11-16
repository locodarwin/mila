package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

// join invites someone into a workspace as role and accepts in a new
// browser, signed in.
func (s *testServer) join(t *testing.T, owner *client, wsID, email, name, role string) *client {
	t.Helper()
	if resp, body := owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": email, "role": role}); resp.StatusCode != 201 {
		t.Fatalf("invite = %d %v", resp.StatusCode, body)
	}
	c := s.browser(t)
	if _, body := c.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": name}); body["signedIn"] != true {
		t.Fatalf("accept = %v", body)
	}
	return c
}

func TestAdminOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)
	// Dana owns a workspace of her own, but is no admin of the instance.
	dana := s.join(t, owner, wsID, "dana@example.com", "Dana", "owner")
	_, me := dana.do("GET", "/me", nil)
	danaID := me["id"].(string)
	if resp, _ := owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "eli@example.com", "role": "member"}); resp.StatusCode != 201 {
		t.Fatal("invite Eli")
	}

	calls := []struct{ method, path string }{
		{"GET", "/admin/status"},
		{"POST", "/admin/backups"},
		{"GET", "/admin/users"},
		{"PATCH", "/admin/users/" + danaID},
		{"POST", "/admin/users/" + danaID + "/sign-out"},
		{"GET", "/admin/invites"},
		{"DELETE", "/admin/invites/" + uuid.NewString()},
		{"POST", "/admin/mailbox/microsoft"},
		{"POST", "/admin/mailbox/microsoft/cancel"},
		{"DELETE", "/admin/mailbox/microsoft"},
	}
	nobody := s.browser(t)
	for _, call := range calls {
		var body any
		if call.method == "PATCH" {
			body = map[string]bool{"isInstanceAdmin": true}
		}
		if resp, out := dana.do(call.method, call.path, body); resp.StatusCode != http.StatusForbidden || out["error"] != "forbidden" {
			t.Errorf("%s %s by a non-admin = %d %v", call.method, call.path, resp.StatusCode, out)
		}
		if resp, _ := nobody.do(call.method, call.path, body); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d", call.method, call.path, resp.StatusCode)
		}
	}
	// An admin's cookie does not carry a write from another site.
	owner.origin = "https://evil.example"
	for _, call := range calls {
		if call.method == "GET" {
			continue
		}
		if resp, out := owner.do(call.method, call.path, map[string]bool{"isInstanceAdmin": true}); resp.StatusCode != http.StatusForbidden || out["error"] != "bad_origin" {
			t.Errorf("cross-origin %s %s = %d %v", call.method, call.path, resp.StatusCode, out)
		}
	}
	owner.origin = s.URL

	resp, status := owner.do("GET", "/admin/status", nil)
	if resp.StatusCode != 200 || status["dialect"] == "" || status["emailMode"] != "file" || status["users"] != float64(2) ||
		status["schema"] != status["latestSchema"] || status["migratedAt"] == nil || status["baseUrl"] != s.URL {
		t.Fatalf("status = %d %v", resp.StatusCode, status)
	}
	if b := status["backup"].(map[string]any); b["lastOkAt"] != nil || len(b["recent"].([]any)) != 0 || b["hour"] != float64(3) {
		t.Fatalf("backup status before any = %v", b)
	}

	// Back up now: queued, then made by the runner.
	if resp, _ := owner.do("POST", "/admin/backups", nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("back up now = %d", resp.StatusCode)
	}
	if _, err := s.runner.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, status = owner.do("GET", "/admin/status", nil)
	b := status["backup"].(map[string]any)
	recent := b["recent"].([]any)
	if b["lastOkAt"] == nil || len(recent) != 1 || recent[0].(map[string]any)["kind"] != "manual" {
		t.Fatalf("backup status after one = %v", b)
	}

	_, users := owner.doList("GET", "/admin/users")
	if len(users) != 2 || users[0]["name"] != "Dana" || users[0]["isInstanceAdmin"] != false || users[0]["devices"] != float64(1) {
		t.Fatalf("users = %v", users)
	}
	resp, user := owner.do("PATCH", "/admin/users/"+danaID, map[string]bool{"isInstanceAdmin": true})
	if resp.StatusCode != 200 || user["isInstanceAdmin"] != true {
		t.Fatalf("make Dana an admin = %d %v", resp.StatusCode, user)
	}
	if resp, _ := dana.do("GET", "/admin/status", nil); resp.StatusCode != 200 {
		t.Fatalf("Dana as admin = %d", resp.StatusCode)
	}
	resp, out := dana.do("POST", "/admin/users/"+danaID+"/sign-out", nil)
	if resp.StatusCode != 200 || out["sessions"] != float64(1) {
		t.Fatalf("sign Dana out everywhere = %d %v", resp.StatusCode, out)
	}
	if resp, _ := dana.do("GET", "/me", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Dana after signing out everywhere = %d", resp.StatusCode)
	}

	_, invites := owner.doList("GET", "/admin/invites")
	if len(invites) != 1 || invites[0]["email"] != "eli@example.com" || invites[0]["workspaceName"] != "Team" {
		t.Fatalf("invites = %v", invites)
	}
	if resp, _ := owner.do("DELETE", "/admin/invites/"+invites[0]["id"].(string), nil); resp.StatusCode != 204 {
		t.Fatalf("cancel invite = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("DELETE", "/admin/invites/"+invites[0]["id"].(string), nil); resp.StatusCode != 404 {
		t.Fatalf("cancel it again = %d", resp.StatusCode)
	}
}

func TestSecurityLogOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	laptop := s.browser(t)
	s.setup(t, laptop)

	// A second browser signs in by email: a new device, from the
	// address the server saw.
	phone := s.browser(t)
	phone.do("POST", "/auth/magic-link", map[string]string{"email": "shawn@example.com"})
	if resp, _ := phone.do("POST", "/auth/magic-link/verify", map[string]string{"token": s.mail.token(t)}); resp.StatusCode != 200 {
		t.Fatal("magic link sign-in")
	}
	_, page := laptop.do("GET", "/me/security-events", nil)
	events := page["events"].([]any)
	if len(events) != 2 || page["more"] != false {
		t.Fatalf("security events = %v", page)
	}
	first := events[0].(map[string]any)
	if first["kind"] != "sign_in" || first["method"] != "magic_link" || first["newDevice"] != true || first["ip"] != "127.0.0.1" {
		t.Fatalf("newest event = %v", first)
	}
	_, page = laptop.do("GET", "/me/security-events?limit=1&before="+first["id"].(string), nil)
	if events := page["events"].([]any); len(events) != 1 || events[0].(map[string]any)["method"] != "setup" {
		t.Fatalf("second page = %v", page)
	}
	_, sessions := laptop.doList("GET", "/me/sessions")
	if len(sessions) != 2 || sessions[0]["ip"] != "127.0.0.1" {
		t.Fatalf("sessions = %v", sessions)
	}
	// The same browser signing in again is not new.
	laptop.do("POST", "/auth/sign-out", nil)
	laptop.do("POST", "/auth/magic-link", map[string]string{"email": "shawn@example.com"})
	laptop.do("POST", "/auth/magic-link/verify", map[string]string{"token": s.mail.token(t)})
	_, page = laptop.do("GET", "/me/security-events", nil)
	if ev := page["events"].([]any)[0].(map[string]any); ev["newDevice"] != false {
		t.Fatalf("same browser again = %v", ev)
	}
}

func TestShareProjectOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)
	guest := s.join(t, owner, wsID, "gus@example.com", "Gus", "guest")
	_, me := guest.do("GET", "/me", nil)
	gusID := me["id"].(string)

	project, task := uuid.NewString(), uuid.NewString()
	resp, _ := owner.do("POST", "/workspaces/"+wsID+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
		map[string]any{"opId": uuid.NewString(), "entity": "project", "id": project, "hlc": hlcNow("owner"),
			"fields": map[string]any{"name": "Garden", "sort_key": "a0"}},
		map[string]any{"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("owner"),
			"fields": map[string]any{"title": "Plant beans", "sort_key": "a0", "project_id": project, "status_id": nil}},
	}})
	if resp.StatusCode != 200 {
		t.Fatalf("push = %d", resp.StatusCode)
	}
	rows := func() int {
		_, snap := guest.do("GET", "/workspaces/"+wsID+"/sync/bootstrap?"+syncQ, nil)
		return len(snap["rows"].([]any))
	}
	if n := rows(); n != 0 {
		t.Fatalf("the guest sees %d rows before sharing", n)
	}
	_, snap := guest.do("GET", "/workspaces/"+wsID+"/sync/bootstrap?"+syncQ, nil)
	seq := snap["seq"].(float64)

	path := "/workspaces/" + wsID + "/projects/" + project + "/guests"
	if resp, _ := guest.do("PUT", path+"/"+gusID, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a guest sharing = %d", resp.StatusCode)
	}
	_, guests := owner.doList("GET", path)
	if len(guests) != 1 || guests[0]["userId"] != gusID || guests[0]["shared"] != false {
		t.Fatalf("guests = %v", guests)
	}
	if resp, _ := owner.do("PUT", path+"/"+gusID, nil); resp.StatusCode != 204 {
		t.Fatalf("share = %d", resp.StatusCode)
	}
	// The guest's copy from before is no good now: the pull says so.
	if resp, _ := guest.do("GET", "/workspaces/"+wsID+"/sync/changes?"+syncQ+"&after="+strconv.FormatInt(int64(seq), 10)+"&epoch="+snap["epoch"].(string), nil); resp.StatusCode != http.StatusGone {
		t.Fatalf("pull after sharing = %d", resp.StatusCode)
	}
	if n := rows(); n != 2 {
		t.Fatalf("the guest sees %d rows after sharing", n)
	}
	_, guests = owner.doList("GET", path)
	if guests[0]["shared"] != true || guests[0]["sharedAt"] == nil {
		t.Fatalf("guests after sharing = %v", guests)
	}
	if resp, _ := owner.do("DELETE", path+"/"+gusID, nil); resp.StatusCode != 204 {
		t.Fatalf("unshare = %d", resp.StatusCode)
	}
	if n := rows(); n != 0 {
		t.Fatalf("the guest sees %d rows after unsharing", n)
	}

	// A member removed and invited back as a guest: a device of theirs
	// that missed the removal holds every project, so its pull from then
	// says to load afresh.
	mo := s.join(t, owner, wsID, "mo@example.com", "Mo", "member")
	_, snap = mo.do("GET", "/workspaces/"+wsID+"/sync/bootstrap?"+syncQ, nil)
	if n := len(snap["rows"].([]any)); n != 2 {
		t.Fatalf("the member sees %d rows", n)
	}
	moSeq := strconv.FormatInt(int64(snap["seq"].(float64)), 10)
	if resp, _ := owner.do("DELETE", "/workspaces/"+wsID+"/members/"+mo.userID(), nil); resp.StatusCode != 204 {
		t.Fatalf("remove = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "mo@example.com", "role": "guest"}); resp.StatusCode != 201 {
		t.Fatalf("invite again = %d", resp.StatusCode)
	}
	if resp, body := mo.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t)}); resp.StatusCode != 200 {
		t.Fatalf("accept again = %d %v", resp.StatusCode, body)
	}
	if resp, _ := mo.do("GET", "/workspaces/"+wsID+"/sync/changes?"+syncQ+"&after="+moSeq+"&epoch="+snap["epoch"].(string), nil); resp.StatusCode != http.StatusGone {
		t.Fatalf("pull from before leaving = %d", resp.StatusCode)
	}
	if _, snap := mo.do("GET", "/workspaces/"+wsID+"/sync/bootstrap?"+syncQ, nil); len(snap["rows"].([]any)) != 0 {
		t.Fatalf("back as a guest, Mo sees %v", snap["rows"])
	}
}
