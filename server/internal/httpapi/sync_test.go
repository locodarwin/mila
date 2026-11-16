package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

// syncQ names the schema in a read, as the app does.
var syncQ = "schema=" + strconv.Itoa(syncer.Schema)

func hlcNow(node string) string {
	return fmt.Sprintf("%013d-%04x-%s", time.Now().UnixMilli(), 0, node)
}

func TestSyncOverHTTPAndSocket(t *testing.T) {
	s := newAuthServer(t)
	phone := s.browser(t)
	s.setup(t, phone)
	_, list := phone.doList("GET", "/workspaces")
	ws := list[0]["id"].(string)

	// The laptop is the same person, signed in by email.
	laptop := s.browser(t)
	laptop.do("POST", "/auth/magic-link", map[string]string{"email": "shawn@example.com"})
	laptop.do("POST", "/auth/magic-link/verify", map[string]string{"token": s.mail.token(t)})

	// The laptop listens on the socket.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	header := http.Header{"Origin": {laptop.origin}}
	for _, c := range laptop.http.Jar.Cookies(mustURL(s.URL)) {
		header.Add("Cookie", c.Name+"="+c.Value)
	}
	sock, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/api/v1/sync/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sock.CloseNow() }()
	read := func() map[string]any {
		t.Helper()
		_, raw, err := sock.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		return m
	}
	if m := read(); m["type"] != "hello" {
		t.Fatalf("first message = %v", m)
	}

	// The phone creates a task.
	task := uuid.NewString()
	resp, body := phone.do("POST", "/workspaces/"+ws+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
		"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("phone"),
		"fields": map[string]any{"title": "Buy seeds", "sort_key": "a0", "project_id": nil, "status_id": nil},
	}}})
	if resp.StatusCode != 200 || body["seq"] != float64(1) {
		t.Fatalf("push = %d %v", resp.StatusCode, body)
	}

	// The laptop is poked and pulls it.
	if m := read(); m["type"] != "poke" || m["workspace"] != ws || m["seq"] != float64(1) {
		t.Fatalf("poke = %v", m)
	}
	// An app that names no schema, or another, reloads before it reads.
	for _, q := range []string{"", "?schema=" + strconv.Itoa(syncer.Schema-1)} {
		if resp, body := laptop.do("GET", "/workspaces/"+ws+"/sync/bootstrap"+q, nil); resp.StatusCode != 426 || body["error"] != "update_required" {
			t.Fatalf("bootstrap%s: %d %v", q, resp.StatusCode, body)
		}
		if resp, body := laptop.do("GET", "/workspaces/"+ws+"/sync/changes?after=0"+strings.Replace(q, "?", "&", 1), nil); resp.StatusCode != 426 || body["error"] != "update_required" {
			t.Fatalf("changes%s: %d %v", q, resp.StatusCode, body)
		}
	}
	_, snap := laptop.do("GET", "/workspaces/"+ws+"/sync/bootstrap?"+syncQ, nil)
	epoch, _ := snap["epoch"].(string)
	if snap["seq"] != float64(1) || len(snap["rows"].([]any)) != 1 || epoch == "" {
		t.Fatalf("bootstrap = %v", snap)
	}
	_, changes := laptop.do("GET", "/workspaces/"+ws+"/sync/changes?"+syncQ+"&after=0&epoch="+url.QueryEscape(epoch), nil)
	got := changes["changes"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["fields"].(map[string]any)["title"] != "Buy seeds" {
		t.Fatalf("changes = %v", changes)
	}
	// A device without the epoch (or with another) bootstraps again.
	for _, q := range []string{"", "&epoch=restored"} {
		if resp, body := laptop.do("GET", "/workspaces/"+ws+"/sync/changes?"+syncQ+"&after=0"+q, nil); resp.StatusCode != 410 || body["error"] != "gone" {
			t.Fatalf("pull with epoch %q = %d %v, want 410 gone", q, resp.StatusCode, body)
		}
	}

	// An app of another version is refused before anything is applied.
	oldPush := func(schema any) map[string]any {
		push := map[string]any{"ops": []any{map[string]any{
			"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("phone"),
			"fields": map[string]any{"title": "Changed by an old app"},
		}}}
		if schema != nil {
			push["schema"] = schema
		}
		return push
	}
	if resp, body := phone.do("POST", "/workspaces/"+ws+"/sync/push", oldPush(syncer.Schema+1)); resp.StatusCode != 426 || body["error"] != "update_required" {
		t.Fatalf("push from schema %d = %d %v, want 426 update_required", syncer.Schema+1, resp.StatusCode, body)
	}
	if _, snap := laptop.do("GET", "/workspaces/"+ws+"/sync/bootstrap?"+syncQ, nil); snap["seq"] != float64(1) {
		t.Fatalf("a refused push changed the workspace: %v", snap)
	}
	// Signing the laptop out closes its socket.
	laptop.do("POST", "/auth/sign-out", nil)
	if _, _, err := sock.Read(ctx); err == nil {
		t.Fatal("the socket stayed open after signing out")
	}

	// Strangers learn nothing about the workspace.
	stranger := s.browser(t)
	if resp, _ := stranger.do("GET", "/workspaces/"+ws+"/sync/bootstrap?"+syncQ, nil); resp.StatusCode != 401 {
		t.Fatalf("signed-out bootstrap = %d", resp.StatusCode)
	}

	// An app from before the gate names no version; its ops are the first
	// version's, which this one no longer is (2 added live documents), so
	// it is told to update like any other old app.
	if resp, body := phone.do("POST", "/workspaces/"+ws+"/sync/push", oldPush(nil)); resp.StatusCode != 426 || body["error"] != "update_required" {
		t.Fatalf("push with no schema = %d %v, want update_required", resp.StatusCode, body)
	}
}

func TestAttachmentsUploadAndDownload(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)
	s.setup(t, c)
	_, list := c.doList("GET", "/workspaces")
	ws := list[0]["id"].(string)
	task := uuid.NewString()
	c.do("POST", "/workspaces/"+ws+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
		"opId": uuid.NewString(), "entity": "task", "id": task, "hlc": hlcNow("x"),
		"fields": map[string]any{"title": "With files", "sort_key": "a0"},
	}}})

	upload := func(name, ctype string, body []byte) (*http.Response, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
		h.Set("Content-Type", ctype)
		part, _ := mw.CreatePart(h)
		_, _ = part.Write(body)
		_ = mw.Close()
		req, _ := http.NewRequest("POST", c.base+"/workspaces/"+ws+"/tasks/"+task+"/attachments", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Origin", c.origin)
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp, out
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	resp, row := upload("../../photo.png", "image/png", png)
	if resp.StatusCode != 201 {
		t.Fatalf("upload = %d %v", resp.StatusCode, row)
	}
	fields := row["fields"].(map[string]any)
	if fields["filename"] != "photo.png" || fields["size"] != float64(len(png)) || fields["mime"] != "image/png" {
		t.Fatalf("attachment row = %v", fields)
	}

	download := func(id string) *http.Response {
		req, _ := http.NewRequest("GET", c.base+"/workspaces/"+ws+"/attachments/"+id+"/file", nil)
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	got := download(row["id"].(string))
	if got.StatusCode != 200 || got.Header.Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(got.Header.Get("Content-Disposition"), "inline") ||
		!strings.Contains(got.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("image download headers = %d %v", got.StatusCode, got.Header)
	}

	// An HTML file never renders as a page.
	_, row = upload("page.html", "text/html", []byte("<script>alert(1)</script>"))
	got = download(row["id"].(string))
	if got.Header.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(got.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("html download headers = %v", got.Header)
	}

	// Strangers cannot fetch it.
	stranger := s.browser(t)
	req, _ := http.NewRequest("GET", stranger.base+"/workspaces/"+ws+"/attachments/"+row["id"].(string)+"/file", nil)
	r2, _ := stranger.http.Do(req)
	if r2.StatusCode != 401 {
		t.Fatalf("stranger download = %d", r2.StatusCode)
	}

	// Too large is refused with a clear message.
	resp, body := upload("big.bin", "application/octet-stream", make([]byte, MaxUpload+10))
	if resp.StatusCode != http.StatusRequestEntityTooLarge || body["message"] == "" {
		t.Fatalf("oversize upload = %d %v", resp.StatusCode, body)
	}

	// The task's history records the files.
	_, events := c.doList("GET", "/workspaces/"+ws+"/tasks/"+task+"/activity")
	if len(events) != 3 {
		t.Fatalf("activity = %v", events)
	}
}
