package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type fakeDB struct{ err error }

func (f fakeDB) PingContext(context.Context) error { return f.err }

const bootstrap = `console.log("start")`

var web = fstest.MapFS{
	"index.html":              {Data: []byte(`<!doctype html><div id="app"></div><script>` + bootstrap + `</script>`)},
	"_app/immutable/entry.js": {Data: []byte(`export {}`)},
	"favicon.svg":             {Data: []byte(`<svg/>`)},
}

func newTest(t *testing.T, d Deps) http.Handler {
	t.Helper()
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if d.DB == nil {
		d.DB = fakeDB{}
	}
	if d.Web == nil {
		d.Web = web
	}
	h, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestHealth(t *testing.T) {
	h := newTest(t, Deps{})
	if rec := get(h, "/healthz"); rec.Code != 200 {
		t.Errorf("/healthz = %d", rec.Code)
	}
	if rec := get(h, "/readyz"); rec.Code != 200 {
		t.Errorf("/readyz = %d", rec.Code)
	}
	down := newTest(t, Deps{DB: fakeDB{errors.New("gone")}})
	if rec := get(down, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz with a dead database = %d, want 503", rec.Code)
	}
}

func TestAPI(t *testing.T) {
	h := newTest(t, Deps{})
	rec := get(h, "/api/v1/instance")
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body["name"] != "Mila" {
		t.Errorf("/api/v1/instance = %d %v %v", rec.Code, body, err)
	}
	rec = get(h, "/api/v1/nope")
	if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Errorf("unknown API path = %d %s, want a JSON 404", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSPA(t *testing.T) {
	h := newTest(t, Deps{})
	for _, p := range []string{"/", "/dev/ui", "/projects/123"} {
		rec := get(h, p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="app"`) {
			t.Errorf("%s = %d, want the app", p, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s must not be cached", p)
		}
	}
	rec := get(h, "/_app/immutable/entry.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("immutable asset = %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := get(h, "/_app/immutable/gone.js"); rec.Code != 404 {
		t.Errorf("missing asset = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/dev/ui", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to the app = %d, want 405", rec.Code)
	}
}

func TestNotBuilt(t *testing.T) {
	h := newTest(t, Deps{Web: fstest.MapFS{}})
	if rec := get(h, "/"); !strings.Contains(rec.Body.String(), "make build") {
		t.Errorf("unbuilt frontend should say so, got %q", rec.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := get(newTest(t, Deps{}), "/")
	csp := rec.Header().Get("Content-Security-Policy")
	sum := sha256.Sum256([]byte(bootstrap))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(csp, "script-src 'self' "+hash) {
		t.Errorf("CSP does not allow the bootstrap script by hash: %s", csp)
	}
	for _, directive := range strings.Split(csp, ";") {
		if strings.Contains(directive, "script-src") && strings.Contains(directive, "unsafe-inline") {
			t.Errorf("scripts must not allow unsafe-inline: %s", csp)
		}
	}
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP must forbid framing: %s", csp)
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("no HSTS over plain http")
	}
	rec = get(newTest(t, Deps{HTTPS: true}), "/")
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS missing for an https instance")
	}
}
