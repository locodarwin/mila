package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/locodarwin/mila/server/internal/accounts"
)

func TestThemesOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	laptop := s.browser(t)
	s.setup(t, laptop)
	const id = "5d0c6b8e-2f4a-4c1e-9b7d-3a2f1e0c9d8b"
	theme := func(id, name string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{
			"mila": "theme", "version": 1, "id": id, "name": name, "base": "calm",
			"tokens": map[string]any{"radius-md": map[string]any{"px": 8}},
			"light":  map[string]any{"surface-0": map[string]any{"oklch": []float64{0.99, 0.01, 85}}},
		})
		return raw
	}

	// The user's other device listens on the socket.
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

	if resp, list := laptop.do("GET", "/me/themes", nil); resp.StatusCode != 200 || len(list["themes"].([]any)) != 0 {
		t.Fatalf("first list = %d %v", resp.StatusCode, list)
	}
	resp, saved := laptop.do("PUT", "/me/themes/"+id, theme(id, "Night shift"))
	if resp.StatusCode != 200 || saved["name"] != "Night shift" || saved["target"] != "AA" || saved["id"] != id {
		t.Fatalf("put = %d %v", resp.StatusCode, saved)
	}
	if m := read(); m["type"] != "themes" || m["id"] != id {
		t.Fatalf("message = %v", m)
	}
	if resp, got := laptop.do("GET", "/me/themes/"+id, nil); resp.StatusCode != 200 || got["base"] != "calm" {
		t.Fatalf("get = %d %v", resp.StatusCode, got)
	}
	_, list := laptop.do("GET", "/me/themes", nil)
	if themes := list["themes"].([]any); len(themes) != 1 || themes[0].(map[string]any)["name"] != "Night shift" {
		t.Fatalf("list = %v", list)
	}

	// Refused: a bad theme, an id other than the address's, an address
	// that is no id, and a body past the limit.
	bad, _ := json.Marshal(map[string]any{"mila": "theme", "version": 1, "id": id, "name": "x", "base": "calm", "light": map[string]any{"glow": map[string]any{"oklch": []float64{1, 0, 0}}}})
	for _, c := range []struct {
		path   string
		body   any
		status int
	}{
		{"/me/themes/" + id, json.RawMessage(bad), 400},
		{"/me/themes/" + id, theme("00000000-0000-4000-8000-000000000001", "Other"), 400},
		{"/me/themes/night", theme(id, "x"), 400},
		{"/me/themes/" + id, map[string]string{"name": strings.Repeat("x", 40<<10)}, 413},
	} {
		if resp, body := laptop.do("PUT", c.path, c.body); resp.StatusCode != c.status {
			t.Errorf("PUT %s = %d %v, want %d", c.path, resp.StatusCode, body, c.status)
		}
	}

	// The limit answers with its own code, so the app can say why.
	for i := range accounts.MaxThemes - 1 {
		tid := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+10)
		if resp, body := laptop.do("PUT", "/me/themes/"+tid, theme(tid, fmt.Sprintf("Theme %d", i))); resp.StatusCode != 200 {
			t.Fatalf("theme %d = %d %v", i, resp.StatusCode, body)
		}
	}
	for range accounts.MaxThemes - 1 {
		read()
	}
	more := "00000000-0000-4000-8000-000000000999"
	if resp, body := laptop.do("PUT", "/me/themes/"+more, theme(more, "One too many")); resp.StatusCode != 400 || body["error"] != accounts.TooManyThemes {
		t.Fatalf("one too many = %d %v", resp.StatusCode, body)
	}

	// Someone else's theme is not there for them.
	other := s.browser(t)
	if resp, _ := other.do("GET", "/me/themes/"+id, nil); resp.StatusCode != 401 {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}

	if resp, _ := laptop.do("DELETE", "/me/themes/"+id, nil); resp.StatusCode != 204 {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
	if m := read(); m["type"] != "themes" || m["id"] != id {
		t.Fatalf("message after delete = %v", m)
	}
	for _, method := range []string{"GET", "DELETE"} {
		if resp, _ := laptop.do(method, "/me/themes/"+id, nil); resp.StatusCode != 404 {
			t.Errorf("%s after delete = %d", method, resp.StatusCode)
		}
	}

	// The preference names a theme by id, or none.
	resp, prefs := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"customTheme": id, "theme": "calm"})
	if resp.StatusCode != 200 || prefs["customTheme"] != id {
		t.Fatalf("customTheme = %d %v", resp.StatusCode, prefs)
	}
	if resp, _ := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"customTheme": "night"}); resp.StatusCode != 400 {
		t.Fatalf("bad customTheme = %d", resp.StatusCode)
	}
	if _, prefs = laptop.do("PATCH", "/me/settings/preferences", map[string]any{"customTheme": nil}); prefs["customTheme"] != nil {
		t.Fatalf("customTheme after null = %v", prefs["customTheme"])
	}
}
