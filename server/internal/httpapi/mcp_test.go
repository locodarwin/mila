package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// bearer sends a token with every request.
type bearer struct{ secret string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpOver connects to the server's MCP endpoint with a token.
func mcpOver(t *testing.T, s *testServer, secret, query string) (*mcp.ClientSession, error) {
	t.Helper()
	ct := &mcp.StreamableClientTransport{Endpoint: s.URL + mcpPath + query, HTTPClient: &http.Client{Transport: bearer{secret}},
		DisableStandaloneSSE: true, MaxRetries: -1}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func TestTheMCPEndpointWantsAToken(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`
		for name, send := range map[string]func(*http.Request){
			"nothing": func(*http.Request) {},
			// The app's own session is not enough: agents use tokens.
			"a session": func(r *http.Request) {
				for _, c := range w.owner.http.Jar.Cookies(r.URL) {
					r.AddCookie(c)
				}
				r.Header.Set("Origin", w.owner.origin)
			},
			"a token that isn't one": func(r *http.Request) { r.Header.Set("Authorization", "Bearer mila_nope") },
		} {
			req, _ := http.NewRequest("POST", w.s.URL+mcpPath, strings.NewReader(init))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			send(req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("%s: %d", name, resp.StatusCode)
			}
			if name != "a token that isn't one" && !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer") {
				t.Errorf("%s: WWW-Authenticate %q", name, resp.Header.Get("WWW-Authenticate"))
			}
		}
		// Only POST: there is no session to listen to or end.
		req, _ := http.NewRequest("GET", w.s.URL+mcpPath, nil)
		req.Header.Set("Authorization", "Bearer "+w.writer)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET = %d", resp.StatusCode)
		}
		// Nothing below the endpoint is open to tokens.
		if code, _ := tokenCall(t, w.s, w.writer, "POST", "/mcp/other", "{}", nil); code != 403 {
			t.Errorf("below the endpoint = %d", code)
		}
	})
}

func TestTheMCPEndpointServesTheTools(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		w.act(t, "create-project", `{"name":"Garden"}`, nil)
		cs, err := mcpOver(t, w.s, w.writer, "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cs.InitializeResult().Instructions, "pass the batch from your first write") {
			t.Error("the instructions don't say how to batch without a session")
		}

		// Each write is a batch of its own unless one is named.
		out, failed := call(t, cs, "add_task", map[string]any{"title": "Mend the fence", "project": "Garden"})
		batch, _ := out["batch"].(string)
		if failed || !strings.HasPrefix(batch, "mcp-") {
			t.Fatalf("add_task = %v", out)
		}
		fence := named(out["task"], "id")
		out, failed = call(t, cs, "add_task", map[string]any{"title": "Paint the gate", "project": "Garden", "batch": batch})
		if failed || out["batch"] != batch {
			t.Fatalf("add_task in a batch = %v", out)
		}
		gate := named(out["task"], "id")
		out, failed = call(t, cs, "add_task", map[string]any{"title": "Weed", "project": "Garden"})
		if failed || out["batch"] == batch || out["batch"] == "" {
			t.Fatalf("a second batch = %v", out)
		}
		weed := named(out["task"], "id")
		if out, failed := call(t, cs, "add_comment", map[string]any{"task": weed, "text": "Mind the roses"}); failed || out["batch"] == batch {
			t.Fatalf("add_comment = %v", out)
		}
		if out, failed := call(t, cs, "add_task", map[string]any{"title": "x", "batch": "no spaces"}); !failed || !strings.Contains(out["error"].(string), "Mila-Batch") {
			t.Fatalf("a bad batch = %v", out)
		}
		if out, _ := call(t, cs, "whoami", nil); out["batch"] != nil || out["today"] == "" {
			t.Fatalf("whoami = %v", out)
		}

		// There is no session's batch to undo.
		if out, failed := call(t, cs, "undo", nil); !failed || !strings.Contains(out["error"].(string), "Name the batch") {
			t.Fatalf("undo with no batch = %v", out)
		}
		out, failed = call(t, cs, "undo", map[string]any{"batch": batch})
		if failed || out["undone"] != float64(2) {
			t.Fatalf("undo = %v", out)
		}
		for id, gone := range map[string]bool{fence: true, gate: true, weed: false} {
			if _, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/tasks/"+id, "", nil); (got["deleted_at"] != nil) != gone {
				t.Errorf("task %s deleted_at = %v", id, got["deleted_at"])
			}
		}
		// History names the token, as for any call with it.
		if _, events := w.owner.doList("GET", "/workspaces/"+w.ws+"/tasks/"+weed+"/activity"); len(events) == 0 || events[0]["via"] != "Agent" {
			t.Errorf("activity = %v", events)
		}

		// A read-only token reads, and its writes are refused in the API's words.
		ro, err := mcpOver(t, w.s, w.reader, "")
		if err != nil {
			t.Fatal(err)
		}
		if out, failed := call(t, ro, "find_tasks", map[string]any{"words": "weed"}); failed || len(out["items"].([]any)) != 1 {
			t.Fatalf("find_tasks, read only = %v", out)
		}
		if out, failed := call(t, ro, "add_task", map[string]any{"title": "x"}); !failed || !strings.Contains(out["error"].(string), "read only") {
			t.Fatalf("add_task, read only = %v", out)
		}
	})
}

func TestTheMCPEndpointChoosesAWorkspace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		w.owner.do("POST", "/workspaces", map[string]string{"name": "Home"})
		resp, made := w.owner.do("POST", "/me/tokens", map[string]any{"name": "Everywhere", "canWrite": true})
		if resp.StatusCode != 201 {
			t.Fatalf("token = %d %v", resp.StatusCode, made)
		}
		every := made["secret"].(string)
		if _, err := mcpOver(t, w.s, every, ""); err == nil {
			t.Fatal("a token for two workspaces connected with neither chosen")
		}
		req, _ := http.NewRequest("POST", w.s.URL+mcpPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Authorization", "Bearer "+every)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != 400 || !strings.Contains(body["message"].(string), "?workspace=") {
			t.Fatalf("no workspace chosen = %d %v", resp.StatusCode, body)
		}
		cs, err := mcpOver(t, w.s, every, "?workspace=home")
		if err != nil {
			t.Fatal(err)
		}
		if out, _ := call(t, cs, "whoami", nil); named(out["workspace"], "name") != "Home" {
			t.Fatalf("whoami = %v", out)
		}
		// A token for one workspace reaches no other through the endpoint.
		if _, err := mcpOver(t, w.s, w.writer, "?workspace=Home"); err == nil {
			t.Fatal("a token for Team reached Home")
		}
	})
}
