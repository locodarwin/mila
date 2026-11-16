package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestPreferencesOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	laptop := s.browser(t)
	s.setup(t, laptop)

	// They come with the user, so they reach the app on its first request.
	_, state := laptop.do("GET", "/auth/state", nil)
	prefs := state["user"].(map[string]any)["preferences"].(map[string]any)
	if prefs["theme"] != "standard" || prefs["clock"] != "auto" || prefs["singleKeyShortcuts"] != true || prefs["accentHue"] != nil {
		t.Fatalf("first preferences = %v", prefs)
	}
	if shortcuts, ok := prefs["shortcuts"].(map[string]any); !ok || len(shortcuts) != 0 {
		t.Fatalf("first shortcuts = %#v", prefs["shortcuts"])
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

	resp, saved := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"theme": "calm", "accentHue": 120, "sidebarHidden": []string{"views"}})
	if resp.StatusCode != 200 || saved["theme"] != "calm" || saved["accentHue"] != float64(120) || saved["density"] != "comfortable" {
		t.Fatalf("patch = %d %v", resp.StatusCode, saved)
	}
	if m := read(); m != "settings" {
		t.Fatalf("message = %v", m)
	}
	// Left out, the accent stays; sent as null, it goes back to the theme's.
	_, saved = laptop.do("PATCH", "/me/settings/preferences", map[string]any{"density": "dense"})
	if saved["accentHue"] != float64(120) || saved["theme"] != "calm" {
		t.Fatalf("after another change = %v", saved)
	}
	_, saved = laptop.do("PATCH", "/me/settings/preferences", map[string]any{"accentHue": nil})
	if saved["accentHue"] != nil {
		t.Fatalf("after reset = %v", saved)
	}
	_, me := laptop.do("GET", "/me", nil)
	if p := me["preferences"].(map[string]any); p["density"] != "dense" || p["sidebarHidden"].([]any)[0] != "views" {
		t.Fatalf("/me preferences = %v", p)
	}

	// Changed keys go and come back as sent.
	resp, saved = laptop.do("PATCH", "/me/settings/preferences", map[string]any{"shortcuts": map[string][]string{"quick-add": {"n"}, "go.inbox": {"g n", "Mod+Alt+i"}}})
	if read() != "settings" {
		t.Fatal("no settings message for shortcuts")
	}
	shortcuts, _ := saved["shortcuts"].(map[string]any)
	if resp.StatusCode != 200 || len(shortcuts) != 2 || shortcuts["quick-add"].([]any)[0] != "n" || shortcuts["go.inbox"].([]any)[1] != "Mod+Alt+i" {
		t.Fatalf("shortcuts patch = %d %v", resp.StatusCode, saved)
	}
	_, me = laptop.do("GET", "/me", nil)
	if p := me["preferences"].(map[string]any); p["shortcuts"].(map[string]any)["quick-add"].([]any)[0] != "n" || p["density"] != "dense" {
		t.Fatalf("/me shortcuts = %v", p)
	}
	if resp, _ := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"shortcuts": map[string][]string{"quick-add": {"Ctrl+n"}}}); resp.StatusCode != 400 {
		t.Fatalf("a bad binding = %d", resp.StatusCode)
	}

	if resp, body := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"theme": "neon"}); resp.StatusCode != 400 || body["message"] == nil {
		t.Fatalf("a bad theme = %d %v", resp.StatusCode, body)
	}
	if resp, _ := laptop.do("PATCH", "/me/settings/preferences", map[string]any{"wallpaper": "cats"}); resp.StatusCode != 400 {
		t.Fatalf("an unknown preference = %d", resp.StatusCode)
	}
	if resp, _ := s.browser(t).do("PATCH", "/me/settings/preferences", map[string]any{"theme": "calm"}); resp.StatusCode != 401 {
		t.Fatalf("signed out = %d", resp.StatusCode)
	}

	// The device's clock and date order ride along with its time zone.
	resp, _ = laptop.do("PUT", "/me/timezone", map[string]any{"timezone": "Europe/Paris", "clock": "24", "dateOrder": "day-month"})
	if resp.StatusCode != 200 {
		t.Fatalf("timezone = %d", resp.StatusCode)
	}
	if resp, _ := laptop.do("PUT", "/me/timezone", map[string]any{"timezone": "Europe/Paris", "clock": "36"}); resp.StatusCode != 400 {
		t.Fatalf("a bad clock = %d", resp.StatusCode)
	}
}
