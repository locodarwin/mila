package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const testLocalKey = "a-key-of-enough-length-0123456789abcdef"

func localServer(t *testing.T) *testServer {
	t.Helper()
	s := newServerWith(t, dbtest.Open(t), func(d *Deps) { d.LocalKey = testLocalKey })
	if err := s.acc.EnsureLocalOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

// signIn opens the app's first address, as its window does.
func signInLocal(t *testing.T, c *client, key string) int {
	t.Helper()
	resp, _ := c.do("GET", "/auth/local?key="+key, nil)
	return resp.StatusCode
}

func TestLocalModeSignsInOnceWithItsKey(t *testing.T) {
	s := localServer(t)
	window := s.browser(t)
	if resp, _ := window.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Fatalf("before = %d", resp.StatusCode)
	}
	if code := signInLocal(t, s.browser(t), "wrong"); code != 403 {
		t.Fatalf("a wrong key = %d", code)
	}
	signInLocal(t, window, testLocalKey)
	resp, me := window.do("GET", "/me", nil)
	if resp.StatusCode != 200 || me["name"] != "You" {
		t.Fatalf("after = %d %v", resp.StatusCode, me)
	}
	// Used once, it's gone: another program that read it later gets nothing.
	other := s.browser(t)
	if code := signInLocal(t, other, testLocalKey); code != 403 {
		t.Fatalf("again = %d", code)
	}
	if resp, _ := other.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Fatalf("the other program = %d", resp.StatusCode)
	}
	_, state := window.do("GET", "/auth/state", nil)
	if state["local"] != true || state["setupRequired"] != false {
		t.Fatalf("state = %v", state)
	}
}

func TestLocalModeProvesItKnowsTheKey(t *testing.T) {
	s := localServer(t)
	resp, err := http.Get(s.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Mila-Local-Proof"); got != challengeOf(testLocalKey) {
		t.Fatalf("proof = %q", got)
	}
	// Only there: nothing else needs it.
	resp, err = http.Get(s.URL + "/api/v1/auth/state")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Mila-Local-Proof"); got != "" {
		t.Fatalf("elsewhere = %q", got)
	}
}

func TestLocalModeAnswersOnlyToItsOwnAddress(t *testing.T) {
	s := localServer(t)
	for _, path := range []string{"/", "/api/v1/auth/state"} {
		req, err := http.NewRequest("GET", s.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "evil.example"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s under another name = %d", path, resp.StatusCode)
		}
	}
}

func TestLocalModeClosesWhatNeedsOtherPeople(t *testing.T) {
	s := localServer(t)
	window := s.browser(t)
	signInLocal(t, window, testLocalKey)
	_, list := window.doList("GET", "/workspaces")
	ws := "/workspaces/" + list[0]["id"].(string)
	for _, c := range [][2]string{
		{"POST", "/auth/magic-link"},
		{"POST", "/auth/setup"},
		{"POST", "/auth/sign-out"},
		{"POST", "/auth/passkey/begin"},
		{"POST", "/auth/desktop/code"},
		{"GET", "/me/tokens"},
		{"POST", "/me/passkeys/begin"},
		{"POST", "/me/delete"},
		{"POST", ws + "/invites"},
		{"GET", ws + "/webhooks"},
		{"GET", ws + "/email-to-task"},
		{"DELETE", ws + "/members/x"},
		{"GET", "/admin/status"},
	} {
		if resp, _ := window.do(c[0], c[1], map[string]string{}); resp.StatusCode != 403 {
			t.Errorf("%s %s = %d", c[0], c[1], resp.StatusCode)
		}
	}
	// The rest is Mila as usual, exports included, which elsewhere want a recent sign-in.
	if resp, made := window.do("POST", ws+"/tasks", map[string]string{"title": "A task"}); resp.StatusCode != 201 {
		t.Fatalf("a task = %d %v", resp.StatusCode, made)
	}
	if resp, _ := window.do("GET", ws+"/members", nil); resp.StatusCode != 200 {
		t.Fatalf("members = %d", resp.StatusCode)
	}
	if resp, body := window.do("POST", "/me/exports", map[string]string{"workspaceId": list[0]["id"].(string), "format": "json"}); resp.StatusCode >= 400 && strings.Contains(strings.ToLower(body["error"].(string)), "sign") {
		t.Fatalf("an export = %d %v", resp.StatusCode, body)
	}
}

// The app starts the server with nothing of the environment around it, but the
// mode turns off what would reach beyond this computer whatever it was started with:
// apps connecting by OAuth, and a test email.
func TestLocalModeKeepsOAuthAndMailOffWhateverItIsTold(t *testing.T) {
	s := newServerWith(t, dbtest.Open(t), func(d *Deps) { d.LocalKey, d.OAuth = testLocalKey, true })
	if err := s.acc.EnsureLocalOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{
		{"GET", "/.well-known/oauth-authorization-server"},
		{"GET", "/.well-known/oauth-protected-resource"},
		{"GET", "/.well-known/oauth-protected-resource/api/v1/mcp"},
		{"POST", "/oauth/register"},
		{"POST", "/oauth/token"},
	} {
		req, _ := http.NewRequest(c[0], s.URL+c[1], strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Errorf("%s %s = %d", c[0], c[1], resp.StatusCode)
		}
	}
	window := s.browser(t)
	signInLocal(t, window, testLocalKey)
	if resp, _ := window.do("POST", "/me/settings/notifications/test-email", nil); resp.StatusCode != 403 {
		t.Errorf("a test email = %d", resp.StatusCode)
	}
}

func TestElsewhereThereIsNoLocalSignIn(t *testing.T) {
	s := newServerOn(t, dbtest.Open(t))
	if code := signInLocal(t, s.browser(t), testLocalKey); code != 404 {
		t.Fatalf("= %d", code)
	}
}

const testWatchKey = "a-watch-key-of-enough-length-0123456789ab"

// watch asks for the place's news as the desktop app does, with the key given.
func watch(t *testing.T, s *testServer, key, after string) (int, map[string]any) {
	t.Helper()
	path := s.URL + "/api/v1/local/watch"
	if after != "" {
		path += "?after=" + url.QueryEscape(after)
	}
	req, _ := http.NewRequest("GET", path, nil)
	if key != "" {
		req.Header.Set("X-Mila-Watch", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestTheAppHearsOfNotificationsWhileItsWindowIsElsewhere(t *testing.T) {
	s := newServerWith(t, dbtest.Open(t), func(d *Deps) { d.LocalKey, d.LocalWatchKey = testLocalKey, testWatchKey })
	if err := s.acc.EnsureLocalOwner(t.Context()); err != nil {
		t.Fatal(err)
	}
	window := s.browser(t)
	signInLocal(t, window, testLocalKey)
	_, ws := window.do("POST", "/workspaces", map[string]string{"name": "Personal"})
	_, me := window.do("GET", "/me", nil)

	for _, key := range []string{"", "wrong", testLocalKey} {
		if code, _ := watch(t, s, key, ""); code == 200 {
			t.Fatalf("the key %q was let in", key)
		}
	}
	// The first time, only where to start, and the workspaces.
	spaces, err := s.acc.Workspaces(t.Context(), me["id"].(string))
	if err != nil || len(spaces) < 2 {
		t.Fatalf("workspaces = %d %v", len(spaces), err)
	}
	code, first := watch(t, s, testWatchKey, "")
	if code != 200 || first["workspaces"] != float64(len(spaces)) || len(first["notices"].([]any)) != 0 {
		t.Fatalf("first = %d %v", code, first)
	}
	start := first["cursor"].(string)

	notice := func(title string, read bool) {
		t.Helper()
		id := uuid.NewString()
		at := store.Now()
		time.Sleep(time.Millisecond)
		err := s.st.InsertNotification(t.Context(), sqlitedb.InsertNotificationParams{
			ID: id, UserID: me["id"].(string), WorkspaceID: ws["id"].(string), Kind: "reminder",
			Title: title, Body: "body of " + title, InApp: true, Now: at,
		})
		if err != nil {
			t.Fatal(err)
		}
		if read {
			if _, err := s.st.MarkNotificationRead(t.Context(), sqlitedb.MarkNotificationReadParams{ID: id, UserID: me["id"].(string), Now: store.NullNow()}); err != nil {
				t.Fatal(err)
			}
		}
	}
	notice("Read already", true)
	notice("Call the broker", false)
	_, next := watch(t, s, testWatchKey, start)
	got := next["notices"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["title"] != "Call the broker" || got[0].(map[string]any)["body"] != "body of Call the broker" {
		t.Fatalf("after = %v", next)
	}
	// Asked again from where it left off, nothing new.
	if _, again := watch(t, s, testWatchKey, next["cursor"].(string)); len(again["notices"].([]any)) != 0 {
		t.Fatalf("again = %v", again)
	}
	// Twenty at a time, the oldest first.
	for i := range accounts.LocalNotices + 2 {
		notice(fmt.Sprintf("Reminder %02d", i), false)
	}
	_, many := watch(t, s, testWatchKey, next["cursor"].(string))
	list := many["notices"].([]any)
	if len(list) != accounts.LocalNotices || list[0].(map[string]any)["title"] != "Reminder 00" {
		t.Fatalf("many = %d, first %v", len(list), list[0])
	}
	_, rest := watch(t, s, testWatchKey, many["cursor"].(string))
	if len(rest["notices"].([]any)) != 2 {
		t.Fatalf("rest = %v", rest)
	}
}

func TestWatchingIsOnlyForTheAppsOwnPlace(t *testing.T) {
	// A server of other people's has no such thing, whatever is sent.
	if code, _ := watch(t, newServerOn(t, dbtest.Open(t)), testWatchKey, ""); code != 404 {
		t.Fatalf("a server = %d", code)
	}
	// Nor has a place started without a watch key, as an app from before starts it.
	if code, _ := watch(t, localServer(t), testWatchKey, ""); code != 404 {
		t.Fatalf("no watch key = %d", code)
	}
}
