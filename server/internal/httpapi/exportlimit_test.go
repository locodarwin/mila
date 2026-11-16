package httpapi

import (
	"net/http"
	"testing"
)

// Deleting an export makes room under the cap on those kept, so starting
// them has a rate of its own: each reads the whole workspace.
func TestExportsAreRateLimited(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	team := ws["id"].(string)
	for i := range 10 {
		resp, e := owner.do("POST", "/workspaces/"+team+"/exports", map[string]string{"format": "csv"})
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("export %d = %d %v", i+1, resp.StatusCode, e)
		}
		if _, err := s.runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		if resp, body := owner.do("DELETE", "/exports/"+e["id"].(string), nil); resp.StatusCode >= 300 {
			t.Fatalf("delete export %d = %d %v", i+1, resp.StatusCode, body)
		}
	}
	resp, body := owner.do("POST", "/workspaces/"+team+"/exports", map[string]string{"format": "csv"})
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" || body["error"] == nil {
		t.Fatalf("an eleventh export in an hour = %d %v", resp.StatusCode, body)
	}
}
