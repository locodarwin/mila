package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/syncer"
)

// What one person writes is budgeted in bytes, sync and REST together: a
// push past it is refused whole, with a Retry-After, and a refused push
// spends nothing, so small edits still go.
func TestWritesAreBudgetedInBytes(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	team := ws["id"].(string)
	push := func(title string) *http.Response {
		resp, _ := owner.do("POST", "/workspaces/"+team+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{map[string]any{
			"opId": uuid.NewString(), "entity": "task", "id": uuid.NewString(), "hlc": hlcNow("o"),
			"fields": map[string]any{"title": title, "sort_key": "a0"},
		}}})
		return resp
	}
	// Titles this long are refused, but only after they are counted.
	big := strings.Repeat("x", 3<<20)
	sent := 0
	for ; sent+len(big) < writeBudget-64<<10; sent += len(big) {
		if resp := push(big); resp.StatusCode != http.StatusOK {
			t.Fatalf("a push of 3 MB after %d MB = %d", sent>>20, resp.StatusCode)
		}
	}
	resp := push(big)
	wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
	if resp.StatusCode != http.StatusTooManyRequests || wait < 60 {
		t.Fatalf("a push past the budget = %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if resp := push("Plan the trip"); resp.StatusCode != http.StatusOK {
		t.Fatalf("a small edit after a refusal = %d", resp.StatusCode)
	}
	// About 2 MB is left, which REST writes spend too.
	codes := []int{}
	for range 3 {
		resp, _ := owner.do("POST", "/workspaces/"+team+"/tasks", map[string]any{"title": strings.Repeat("y", 900<<10)})
		codes = append(codes, resp.StatusCode)
	}
	if codes[0] == http.StatusTooManyRequests || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("REST writes of 900 KB after the budget's last 2 MB = %v", codes)
	}
}
