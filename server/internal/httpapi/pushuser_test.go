package httpapi

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

// A push names whom its edits were made as: one for someone else (a tab
// left open while the browser signed in as another person) is refused
// whole.
func TestAPushForSomeoneElseIsRefused(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	team := ws["id"].(string)
	_, me := owner.do("GET", "/me", nil)
	push := func(user string) (*http.Response, map[string]any) {
		body := map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
			"opId": uuid.NewString(), "entity": "task", "id": uuid.NewString(), "hlc": hlcNow("o"),
			"fields": map[string]any{"title": "Plan", "sort_key": "a0"},
		}}}
		if user != "" {
			body["user"] = user
		}
		return owner.do("POST", "/workspaces/"+team+"/sync/push", body)
	}
	if resp, body := push(uuid.NewString()); resp.StatusCode != http.StatusConflict || body["error"] != "other_user" {
		t.Fatalf("a push for someone else = %d %v", resp.StatusCode, body)
	}
	for _, user := range []string{me["id"].(string), ""} {
		if resp, body := push(user); resp.StatusCode != http.StatusOK {
			t.Fatalf("a push for %q = %d %v", user, resp.StatusCode, body)
		}
	}
}
