package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

func TestEscapesNUL(t *testing.T) {
	for body, want := range map[string]bool{
		`{"name":"Team"}`:          false,
		`{"name":"Te\u0000am"}`:    true,
		`{"name":"\u0000"}`:        true,
		`{"\u0000":"a"}`:           true,
		`{"name":"a\\u0000b"}`:     false, // a backslash, then the letters
		`{"name":"a\\\u0000b"}`:    true,  // a backslash, then a NUL
		`{"name":"a\\\\u0000b"}`:   false,
		`{"name":"\u00000"}`:       true,
		`{"name":"\u0001\u00100"}`: false,
		``:                         false,
	} {
		if got := escapesNUL([]byte(body)); got != want {
			t.Errorf("escapesNUL(%s) = %v, want %v", body, got, want)
		}
	}
}

// A query part that doesn't parse (a ; or a bad escape) must not let the
// parts that do through unchecked: the handlers read those all the same.
func TestQueryUTF8CheckedWhateverParses(t *testing.T) {
	reached := false
	h := refuseNULs(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	for query, want := range map[string]int{
		"project_id=%FF":            http.StatusBadRequest,
		"project_id=%FF&x=;":        http.StatusBadRequest,
		"project_id=%FF&y=%zz":      http.StatusBadRequest,
		"y=%zz&project_id=%C3":      http.StatusBadRequest,
		"a;b=%FF":                   http.StatusBadRequest,
		"%FF=1&y=%zz":               http.StatusBadRequest,
		"project_id=abc&y=%zz":      http.StatusOK,
		"name=%C3%A9t%C3%A9&x=;":    http.StatusOK,
		"name=%C3%A9t%C3%A9":        http.StatusOK,
		"":                          http.StatusOK,
		"q=caf%C3%A9+cr%C3%A8me&z=": http.StatusOK,
	} {
		reached = false
		r := httptest.NewRequest("GET", "/x", nil)
		r.URL.RawQuery = query
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != want || reached != (want == http.StatusOK) {
			t.Errorf("?%s = %d (handler reached %v), want %d", query, rec.Code, reached, want)
		}
	}
}

// Postgres refuses a NUL in text, so every way in refuses one with a 400
// rather than failing with a 500 there: names, lookups in the address,
// actions, rows and an app registering itself.
func TestNULsAreRefused(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		refused := func(what string, resp *http.Response, body map[string]any) {
			t.Helper()
			if resp.StatusCode != http.StatusBadRequest || body["message"] != nulMessage {
				t.Errorf("%s = %d %v, want 400 %q", what, resp.StatusCode, body, nulMessage)
			}
		}

		resp, body := w.owner.do("POST", "/workspaces", map[string]string{"name": "Te\x00am"})
		refused("a workspace named with a NUL", resp, body)
		resp, body = w.owner.do("PATCH", "/workspaces/"+w.ws, map[string]string{"name": "Te\x00am"})
		refused("renaming a workspace to one", resp, body)
		resp, body = w.owner.do("PATCH", "/me", map[string]string{"name": "Sha\x00wn"})
		refused("a person's name with one", resp, body)
		resp, body = w.owner.do("POST", "/me/tokens", map[string]any{"name": "To\x00ken", "workspaceId": w.ws})
		refused("a token's name with one", resp, body)

		// The text \u0000, a backslash and five letters, is a name like any other.
		resp, body = w.owner.do("POST", "/workspaces", map[string]string{"name": `Odd \u0000`})
		if resp.StatusCode != http.StatusCreated || body["name"] != `Odd \u0000` {
			t.Errorf("a workspace named with a backslash = %d %v", resp.StatusCode, body)
		}

		resp, body = w.owner.do("GET", "/workspaces/"+w.ws+"/tasks?project_id=%00", nil)
		refused("a lookup by a NUL", resp, body)
		resp, body = w.owner.do("GET", "/workspaces/%00/tasks", nil)
		refused("a NUL in the path", resp, body)

		// Bytes that aren't UTF-8 are refused at the door too (Postgres
		// refuses them in text), and in a JSON field.
		for _, path := range []string{"/workspaces/%FF/tasks", "/workspaces/" + w.ws + "/tasks?project_id=%FF"} {
			resp, body = w.owner.do("GET", path, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s = %d %v, want 400", path, resp.StatusCode, body)
			}
		}
		raw, _ := json.Marshal(map[string]any{"name": "Board", "type": "list", "config": map[string]any{}})
		raw = bytes.Replace(raw, []byte(`{}`), []byte("{\"x\":\"\xff\"}"), 1)
		bad, err := http.NewRequest("POST", w.owner.base+"/workspaces/"+w.ws+"/saved-views", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		bad.Header.Set("Content-Type", "application/json")
		bad.Header.Set("Origin", w.owner.origin)
		got, err := w.owner.http.Do(bad)
		if err != nil {
			t.Fatal(err)
		}
		_ = got.Body.Close()
		if got.StatusCode != http.StatusBadRequest {
			t.Errorf("a view whose config isn't UTF-8 = %d, want 400", got.StatusCode)
		}

		resp, body = w.owner.do("POST", "/workspaces/"+w.ws+"/actions/create-project", map[string]string{"name": "Gar\x00den"})
		refused("an action with one", resp, body)
		resp, body = w.owner.do("POST", "/workspaces/"+w.ws+"/projects", map[string]string{"name": "Gar\x00den"})
		refused("a row with one", resp, body)

		raw, _ = json.Marshal(map[string]any{"client_name": "Ap\x00p", "redirect_uris": []string{"http://127.0.0.1:9/cb"}})
		reg, err := http.Post(w.s.URL+"/oauth/register", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(reg.Body)
		_ = reg.Body.Close()
		if reg.StatusCode != http.StatusBadRequest || !bytes.Contains(out, []byte("NUL")) {
			t.Errorf("an app registering with a NUL in its name = %d %s", reg.StatusCode, out)
		}

		// None of it was stored.
		_, me := w.owner.do("GET", "/me", nil)
		if me["name"] != "Shawn" {
			t.Errorf("the name is now %v", me["name"])
		}
	})
}
