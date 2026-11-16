package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestYourDataOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	team := ws["id"].(string)
	owner.do("POST", "/workspaces/"+team+"/invites", map[string]string{"email": "dana@example.com", "role": "member"})
	dana := s.browser(t)
	dana.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Dana"})
	_, me := dana.do("GET", "/me", nil)
	danaID := me["id"].(string)
	task, comment := uuid.NewString(), uuid.NewString()
	owner.do("POST", "/workspaces/"+team+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
		"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("o"),
		"fields": map[string]any{"title": "Plan the trip", "sort_key": "a0"},
	}}})
	dana.do("POST", "/workspaces/"+team+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
		"opId": uuid.NewString(), "entity": "comment", "id": comment, "hlc": hlcNow("d"),
		"fields": map[string]any{"task_id": task, "author_id": danaID, "body": "Booked"},
	}}})

	// An export, run in the background, then downloaded by its maker only.
	resp, e := owner.do("POST", "/workspaces/"+team+"/exports", map[string]string{"format": "json"})
	if resp.StatusCode != http.StatusAccepted || e["status"] != "queued" {
		t.Fatalf("start export = %d %v", resp.StatusCode, e)
	}
	if resp, body := owner.do("POST", "/workspaces/"+team+"/exports", map[string]string{"format": "pdf"}); resp.StatusCode != 400 {
		t.Fatalf("an unknown format = %d %v", resp.StatusCode, body)
	}
	if _, err := s.runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, list := owner.doList("GET", "/me/exports")
	if len(list) != 1 || list[0]["status"] != "ready" || list[0]["workspaceName"] != "Team" {
		t.Fatalf("exports = %v", list)
	}
	id := e["id"].(string)
	download := func(c *client) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", c.base+"/exports/"+id+"/download", nil)
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	got, zipped := download(owner)
	if got.StatusCode != 200 || got.Header.Get("Content-Type") != "application/zip" ||
		!strings.HasPrefix(got.Header.Get("Content-Disposition"), "attachment; filename=mila-team-json-") ||
		!strings.Contains(got.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("download = %d %v", got.StatusCode, got.Header)
	}
	if got, _ := download(dana); got.StatusCode != 404 {
		t.Fatalf("someone else's download = %d", got.StatusCode)
	}
	_, page := owner.do("GET", "/me/notifications", nil)
	items := page["items"].([]any)
	if n := items[0].(map[string]any); n["kind"] != "export_ready" || n["url"] != "/settings/w/"+team+"/data" {
		t.Fatalf("notification = %v", n)
	}

	// The zip comes back as a new workspace.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("ids", "fresh")
	part, _ := mw.CreateFormFile("file", "team.zip")
	_, _ = part.Write(zipped)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", owner.base+"/imports", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", owner.origin)
	r, err := owner.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var im map[string]any
	_ = json.NewDecoder(r.Body).Decode(&im)
	_ = r.Body.Close()
	if r.StatusCode != http.StatusAccepted || im["keepIds"] != false {
		t.Fatalf("start import = %d %v", r.StatusCode, im)
	}
	if _, err := s.runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, imports := owner.doList("GET", "/me/imports")
	if len(imports) != 1 || imports[0]["status"] != "ready" || imports[0]["workspaceName"] != "Team" || imports[0]["report"] == nil {
		t.Fatalf("imports = %v", imports)
	}
	_, all := owner.doList("GET", "/workspaces")
	if len(all) != 3 {
		t.Fatalf("workspaces after import = %v", all)
	}

	// Dana deletes her account; her comment stays, credited to Deleted user.
	if resp, body := dana.do("POST", "/me/delete", map[string]string{"email": "nope@example.com"}); resp.StatusCode != 400 || body["message"] == "" {
		t.Fatalf("a wrong address = %d %v", resp.StatusCode, body)
	}
	resp, _ = dana.do("POST", "/me/delete", map[string]string{"email": "dana@example.com"})
	if resp.StatusCode != 204 || !strings.Contains(resp.Header.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("delete = %d %v", resp.StatusCode, resp.Header)
	}
	if resp, _ := dana.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Fatalf("after deletion /me = %d", resp.StatusCode)
	}
	_, former := owner.doList("GET", "/workspaces/"+team+"/former-members")
	if len(former) != 1 || former[0]["userId"] != danaID || former[0]["name"] != "Deleted user" || former[0]["deleted"] != true {
		t.Fatalf("former members = %v", former)
	}
	_, snap := owner.do("GET", "/workspaces/"+team+"/sync/bootstrap?"+syncQ, nil)
	kept := false
	for _, row := range snap["rows"].([]any) {
		if m := row.(map[string]any); m["id"] == comment {
			kept = true
		}
	}
	if !kept {
		t.Fatal("Dana's comment went with her account")
	}
	// The only admin cannot delete theirs.
	if resp, body := owner.do("POST", "/me/delete", map[string]string{"email": "shawn@example.com"}); resp.StatusCode != 409 {
		t.Fatalf("the only admin = %d %v", resp.StatusCode, body)
	}
}
