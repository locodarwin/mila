package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/actions"
	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/ical"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/ownership"
	"github.com/locodarwin/mila/server/internal/push"
	"github.com/locodarwin/mila/server/internal/push/pushtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/webhooks"
)

type outbox struct {
	mu   sync.Mutex
	last string
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.last = m.Text
	return nil
}

func (o *outbox) lastText() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.last
}

var tokenRE = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

func (o *outbox) token(t *testing.T) string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	m := tokenRE.FindStringSubmatch(o.last)
	if m == nil {
		t.Fatalf("no token in %q", o.last)
	}
	return m[1]
}

type testServer struct {
	*httptest.Server
	acc    *accounts.Service
	st     *store.Store
	sync   *syncer.Service
	mail   *outbox
	runner *jobs.Runner
	push   *pushtest.Server
}

func newAuthServer(t *testing.T) *testServer {
	t.Helper()
	return newServerOn(t, dbtest.Open(t))
}

// newServerOn is the whole server on a given database.
func newServerOn(t *testing.T, d *db.DB) *testServer {
	t.Helper()
	return newServerWith(t, d, nil)
}

// newServerWith is newServerOn with its Deps changed first (tighter
// limits, say).
func newServerWith(t *testing.T, d *db.DB, change func(*Deps)) *testServer {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := store.New(d)
	box := &outbox{}
	srv := httptest.NewUnstartedServer(nil)
	origin := "http://" + srv.Listener.Addr().String()
	acc, err := accounts.New(st, box, log, accounts.Config{BaseURL: origin, Origin: origin, RPID: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	hub := syncer.NewHub(st, log)
	acc.SetEvents(hub)
	store, err := files.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	syncSvc := syncer.New(st, log, hub)
	runner := jobs.New(st, log, nil)
	notifier := notify.New(st, log, runner, box, syncSvc, hub, notify.Config{BaseURL: origin, Email: true})
	syncSvc.SetWatcher(notifier)
	acc.SetNotices(notifier)
	fake := pushtest.New(t)
	pusher, err := push.New(context.Background(), st, log, runner, push.Options{Subject: "mailto:mila@example.com", Client: fake.Client(), AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	fake.Key = pusher.PublicKey()
	notifier.SetPusher(pusher)
	acc.SetContent(syncSvc)
	data, err := ownership.New(st, log, syncSvc, store, runner, notifier, hub, ownership.Config{DataDir: t.TempDir(), ImportMax: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	backups := backup.New(st, log, runner, backup.Config{Dir: t.TempDir(), Hour: 3})
	hooks := webhooks.New(st, log, runner, notifier, webhooks.Options{BaseURL: origin, AllowPrivate: true})
	syncSvc.SetWatcher(syncer.Watchers{notifier, hooks})
	deps := Deps{Log: log, DB: fakeDB{}, Web: web, Accounts: acc, Origin: origin, Webhooks: hooks,
		Sync: syncSvc, Hub: hub, Files: store, Notify: notifier, Push: pusher, Calendar: ical.New(st, syncSvc, log, origin), Data: data,
		Backup: backups, Schema: d, Dialect: string(d.Dialect), MailMode: "file", BaseURL: origin, Actions: actions.New(st, syncSvc, acc)}
	if change != nil {
		change(&deps)
	}
	h, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = h
	srv.Start()
	t.Cleanup(srv.Close)
	return &testServer{Server: srv, acc: acc, st: st, sync: syncSvc, mail: box, runner: runner, push: fake}
}

// client is a browser: it keeps cookies and sends the server's Origin.
type client struct {
	t      *testing.T
	http   *http.Client
	base   string
	origin string
}

func (s *testServer) browser(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, http: &http.Client{Jar: jar}, base: s.URL + "/api/v1", origin: s.URL}
}

func (c *client) do(method, path string, body any) (*http.Response, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func (s *testServer) setup(t *testing.T, c *client) {
	t.Helper()
	link, err := s.acc.SetupLink(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	resp, body := c.do("POST", "/auth/setup", map[string]string{
		"token": u.Query().Get("token"), "name": "Shawn", "email": "shawn@example.com",
	})
	if resp.StatusCode != 200 || body["email"] != "shawn@example.com" {
		t.Fatalf("setup = %d %v", resp.StatusCode, body)
	}
}

func TestSetupAndSession(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)

	_, state := c.do("GET", "/auth/state", nil)
	if state["setupRequired"] != true || state["user"] != nil {
		t.Fatalf("state before setup = %v", state)
	}
	if resp, _ := c.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Fatalf("/me signed out = %d", resp.StatusCode)
	}
	s.setup(t, c)

	cookies := c.http.Jar.Cookies(mustURL(s.URL))
	if len(cookies) != 2 || cookies[0].Name != cookieName || cookies[1].Name != deviceCookie {
		t.Fatalf("cookies = %v", cookies)
	}
	_, me := c.do("GET", "/me", nil)
	if me["name"] != "Shawn" || me["isInstanceAdmin"] != true {
		t.Fatalf("/me = %v", me)
	}
	_, list := c.doList("GET", "/me/sessions")
	if len(list) != 1 || list[0]["current"] != true {
		t.Fatalf("sessions = %v", list)
	}

	if resp, _ := c.do("POST", "/auth/sign-out", nil); resp.StatusCode != 204 {
		t.Fatalf("sign out = %d", resp.StatusCode)
	}
	if resp, _ := c.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Fatalf("/me after sign-out = %d", resp.StatusCode)
	}
}

func (c *client) doList(method, path string) (*http.Response, []map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func TestSessionCookieAttributes(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)
	c.http.Jar = nil // read the raw Set-Cookie
	link, _ := s.acc.SetupLink(context.Background())
	u, _ := url.Parse(link)
	resp, _ := c.do("POST", "/auth/setup", map[string]string{"token": u.Query().Get("token"), "name": "A", "email": "a@example.com"})
	set := resp.Header.Values("Set-Cookie")
	if len(set) != 2 {
		t.Fatalf("Set-Cookie = %q", set)
	}
	for _, want := range []string{cookieName + "=", "HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=2592000"} {
		if !strings.Contains(set[0], want) {
			t.Errorf("Set-Cookie %q lacks %q", set[0], want)
		}
	}
	// The device cookie outlives sessions, so a browser is recognized at
	// its next sign-in.
	for _, want := range []string{deviceCookie + "=", "HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=34560000"} {
		if !strings.Contains(set[1], want) {
			t.Errorf("Set-Cookie %q lacks %q", set[1], want)
		}
	}
}

func TestCrossOriginRequestsAreRefused(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)
	s.setup(t, c)

	c.origin = "https://evil.example"
	if resp, body := c.do("POST", "/workspaces", map[string]string{"name": "Pwned"}); resp.StatusCode != 403 || body["error"] != "bad_origin" {
		t.Fatalf("cross-origin POST = %d %v", resp.StatusCode, body)
	}
	c.origin = "" // a cookie with no Origin is not a browser we trust
	if resp, _ := c.do("POST", "/workspaces", map[string]string{"name": "Pwned"}); resp.StatusCode != 403 {
		t.Fatalf("origin-less POST with cookie = %d", resp.StatusCode)
	}
	c.origin = s.URL
	if resp, _ := c.do("POST", "/workspaces", map[string]string{"name": "Fine"}); resp.StatusCode != 201 {
		t.Fatalf("same-origin POST = %d", resp.StatusCode)
	}
	// Reads are not affected.
	c.origin = "https://evil.example"
	if resp, _ := c.do("GET", "/me", nil); resp.StatusCode != 200 {
		t.Fatalf("cross-origin GET = %d", resp.StatusCode)
	}
}

func TestMagicLinkAndInviteOverHTTP(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)

	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	wsID := ws["id"].(string)
	if resp, body := owner.do("POST", "/workspaces/"+wsID+"/invites", map[string]string{"email": "dana@example.com", "role": "member"}); resp.StatusCode != 201 {
		t.Fatalf("invite = %d %v", resp.StatusCode, body)
	}
	token := s.mail.token(t)

	dana := s.browser(t)
	_, preview := dana.do("POST", "/invites/preview", map[string]string{"token": token})
	if preview["workspaceName"] != "Team" || preview["invitedByName"] != "Shawn" {
		t.Fatalf("preview = %v", preview)
	}
	_, accepted := dana.do("POST", "/invites/accept", map[string]string{"token": token, "name": "Dana"})
	if accepted["signedIn"] != true || accepted["workspaceId"] != wsID {
		t.Fatalf("accept = %v", accepted)
	}
	_, me := dana.do("GET", "/me", nil)
	if me["email"] != "dana@example.com" {
		t.Fatalf("Dana /me = %v", me)
	}

	// Dana signs in on another device with a magic link.
	phone := s.browser(t)
	if resp, _ := phone.do("POST", "/auth/magic-link", map[string]string{"email": "dana@example.com"}); resp.StatusCode != 204 {
		t.Fatalf("magic link = %d", resp.StatusCode)
	}
	if resp, body := phone.do("POST", "/auth/magic-link/verify", map[string]string{"token": s.mail.token(t)}); resp.StatusCode != 200 || body["name"] != "Dana" {
		t.Fatalf("verify = %d %v", resp.StatusCode, body)
	}
	_, list := dana.doList("GET", "/me/sessions")
	if len(list) != 2 {
		t.Fatalf("Dana's sessions = %v", list)
	}

	// Members cannot list invites; the error message is meant for people.
	resp, body := dana.do("GET", "/workspaces/"+wsID+"/invites", nil)
	if resp.StatusCode != 403 || body["error"] != "forbidden" || body["message"] == "" {
		t.Fatalf("member listing invites = %d %v", resp.StatusCode, body)
	}
}

func TestSignInRateLimit(t *testing.T) {
	s := newAuthServer(t)
	c := s.browser(t)
	limited := false
	for range 30 {
		if resp, _ := c.do("POST", "/auth/magic-link", map[string]string{"email": "x@example.com"}); resp.StatusCode == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("30 sign-in attempts in a row were never limited")
	}
}

func TestClientIP(t *testing.T) {
	_, proxyNet, _ := net.ParseCIDR("10.0.0.0/8")
	var got string
	h := withClientIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = clientIP(r) }), []*net.IPNet{proxyNet}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cases := []struct {
		remote string
		xff    []string
		want   string
	}{
		{"203.0.113.9:5555", nil, "203.0.113.9"},
		{"203.0.113.9:5555", []string{"1.2.3.4"}, "203.0.113.9"},             // untrusted peers cannot claim an address
		{"10.0.0.2:5555", []string{"198.51.100.7"}, "198.51.100.7"},          // behind Caddy
		{"10.0.0.2:5555", []string{"6.6.6.6, 198.51.100.7"}, "198.51.100.7"}, // the client's own header is not believed
		{"10.0.0.2:5555", []string{"198.51.100.7, 10.0.0.3"}, "198.51.100.7"},
		// HAProxy's option forwardfor adds a line of its own after the
		// client's: the lines are one list (RFC 9110), read from the right.
		{"10.0.0.2:5555", []string{"6.6.6.6", "198.51.100.9"}, "198.51.100.9"},
		{"10.0.0.2:5555", []string{"6.6.6.6, 7.7.7.7", "198.51.100.9, 10.0.0.3"}, "198.51.100.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		for _, line := range c.xff {
			r.Header.Add("X-Forwarded-For", line)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != c.want {
			t.Errorf("remote %s, XFF %q: got %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

// A browser that goes away cancels its requests. That is no error of the
// server's, while a failure with the browser still there is.
func TestClientGoingAwayIsNotAnError(t *testing.T) {
	var buf bytes.Buffer
	h := &handlers{log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	h.fail(httptest.NewRecorder(), r, context.Canceled)
	if buf.Len() != 0 {
		t.Fatalf("logged a client going away: %s", buf.String())
	}

	h.fail(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil), errors.New("disk on fire"))
	if !strings.Contains(buf.String(), "level=ERROR") || !strings.Contains(buf.String(), "disk on fire") {
		t.Fatalf("a real failure was not logged as an error: %q", buf.String())
	}
}
