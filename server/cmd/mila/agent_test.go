package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fake Mila that answers what `mila api` asks, and keeps what it was sent.
func fakeMila(t *testing.T) (*httptest.Server, *[]*http.Request, *[]string) {
	t.Helper()
	var reqs []*http.Request
	var bodies []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		reqs, bodies = append(reqs, r), append(bodies, string(raw))
		if r.Header.Get("Authorization") != "Bearer mila_secret" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces":
			_, _ = w.Write([]byte(`[{"id":"ws1","name":"Home","role":"owner"}]`))
		case "/api/v1/workspaces/ws1/actions/find-tasks":
			_, _ = w.Write([]byte(`{"items":[{"id":"t1","title":"Water the plants","due_date":"2026-11-13","status":{"name":"To do"},"project":{"name":"Garden"}}],"today":"2026-11-07","more":false}`))
		case "/api/v1/workspaces/ws1/actions/add-task":
			_, _ = w.Write([]byte(`{"task":{"id":"t2"},"batch":"b","said":"Added it"}`))
		case "/api/v1/workspaces/ws1/actions/complete-task":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"There is no such task that you can see.","field":"task"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(s.Close)
	return s, &reqs, &bodies
}

func TestMilaAPI(t *testing.T) {
	s, reqs, bodies := fakeMila(t)
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("mila_secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MILA_SERVER", s.URL)
	t.Setenv("MILA_TOKEN_FILE", token)
	t.Setenv("MILA_TOKEN", "")
	t.Setenv("MILA_WORKSPACE", "")
	t.Setenv("MILA_BATCH", "")

	var out bytes.Buffer
	if err := apiRun([]string{"find", "--list", "upcoming", "--due-to", "2026-11-14"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Water the plants") || !strings.Contains(out.String(), "Garden") || !strings.Contains(out.String(), "t1") {
		t.Errorf("find printed:\n%s", out.String())
	}
	if q := (*reqs)[len(*reqs)-1].URL.Query(); q.Get("list") != "upcoming" || q.Get("due_to") != "2026-11-14" {
		t.Errorf("find asked %v", q)
	}

	out.Reset()
	if err := apiRun([]string{"do", "--batch", "tidy-1", "add-task"}, strings.NewReader(`{"title":"Call","project":"Garden"}`), &out); err != nil {
		t.Fatal(err)
	}
	last := (*reqs)[len(*reqs)-1]
	if last.Method != "POST" || last.Header.Get("Mila-Batch") != "tidy-1" || last.Header.Get("Idempotency-Key") == "" {
		t.Errorf("do sent %s with batch %q and key %q", last.Method, last.Header.Get("Mila-Batch"), last.Header.Get("Idempotency-Key"))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[len(*bodies)-1]), &sent); err != nil || sent["title"] != "Call" {
		t.Errorf("do sent %q", (*bodies)[len(*bodies)-1])
	}
	if !strings.Contains(out.String(), `"t2"`) {
		t.Errorf("do printed %s", out.String())
	}

	// The server's refusal comes back in its own words.
	err := apiRun([]string{"do", "complete-task", `{"task":"x"}`}, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "no such task") || !strings.Contains(err.Error(), "404") {
		t.Errorf("a refusal = %v", err)
	}
	if err := apiRun([]string{"do", "add-task", `not json`}, nil, &out); err == nil {
		t.Error("a body that isn't JSON was sent")
	}
	if err := apiRun([]string{"nope"}, nil, &out); err == nil {
		t.Error("an unknown command ran")
	}
	// The token never travels anywhere but its header.
	for _, r := range *reqs {
		if strings.Contains(r.URL.String(), "secret") {
			t.Errorf("the token is in %s", r.URL)
		}
	}
}

// JSON and errors reach the terminal without C1 controls or the marks
// that turn text around: JSON as \u escapes, so a script reads the same
// text, and an error's words with them made spaces.
func TestJSONAndErrorsLeaveOutEscapes(t *testing.T) {
	// A C1 CSI (U+009B), an RLO (U+202E), a zero-width space (U+200B), a
	// tag (U+E0041) and an ESC, as bytes.
	raw := "Pay\xc2\x9b2J in\xe2\x80\xaevoice\xe2\x80\x8b\xf3\xa0\x81\x81\x1b]52;c;aGk=\x07"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces":
			_, _ = w.Write([]byte(`[{"id":"ws1","name":"Home","role":"owner"}]`))
		case "/api/v1/workspaces/ws1/tasks/t1":
			b, _ := json.Marshal(map[string]any{"id": "t1", "title": raw})
			_, _ = w.Write(b)
		default:
			w.WriteHeader(404)
			b, _ := json.Marshal(map[string]any{"error": "not_found", "message": "No project " + raw})
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(s.Close)
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("mila_secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MILA_SERVER", s.URL)
	t.Setenv("MILA_TOKEN_FILE", token)
	t.Setenv("MILA_TOKEN", "")
	t.Setenv("MILA_WORKSPACE", "")
	t.Setenv("MILA_BATCH", "")
	bad := "\x1b\x07\xc2\x9b\xe2\x80\xae\xe2\x80\x8b\xf3\xa0\x81\x81"

	var out bytes.Buffer
	if err := apiRun([]string{"get", "tasks/t1"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), bad) {
		t.Errorf("get printed %q", out.String())
	}
	var back map[string]any
	if err := json.Unmarshal(out.Bytes(), &back); err != nil || back["title"] != raw {
		t.Errorf("get's JSON reads back as %q, %v", back["title"], err)
	}

	err := apiRun([]string{"do", "move-task", `{}`}, nil, &out)
	if err == nil {
		t.Fatal("the refusal was taken")
	}
	if line := errorLine(err); strings.ContainsAny(line, bad) || !strings.Contains(line, "No project Pay") {
		t.Errorf("the error printed %q", line)
	}
	if line := errorLine(errors.New("usage: a\n  b\tc")); line != "usage: a\n  b\tc" {
		t.Errorf("an error's own lines and tabs = %q", line)
	}
}

// A title anyone could choose reaches the terminal without its escapes.
func TestTheTableLeavesOutEscapes(t *testing.T) {
	var out strings.Builder
	items := []map[string]any{{"title": "Pay invoice\x1b]52;c;aGk=\x07\x1b[2K\x1b[1A", "id": "t1", "project": map[string]any{"name": "Ho\u202Eme"}}}
	if err := table(&out, items, false, ""); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), "\x1b\x07\u202E") || !strings.Contains(out.String(), "Pay invoice") {
		t.Fatalf("table = %q", out.String())
	}
}
