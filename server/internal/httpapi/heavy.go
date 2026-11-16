package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// heavyReads bounds the reads that may go through a whole workspace
// (find-tasks, which pages through the tasks in order until it has its
// answer, all of them for a search that few match): a few at once in the
// server, and one at a time for each person. A request waits a little for
// its turn and is then answered 503, so a script or assistant sending many
// at once slows itself down rather than the server.
type heavyReads struct {
	slots chan struct{}
	mu    sync.Mutex
	busy  map[string]bool
	wait  time.Duration
}

func newHeavyReads(atOnce int, wait time.Duration) *heavyReads {
	return &heavyReads{slots: make(chan struct{}, atOnce), busy: map[string]bool{}, wait: wait}
}

// heavy takes a turn for userID, or answers the request and gives false.
// The caller runs the returned function when done.
func (h *handlers) heavy(w http.ResponseWriter, r *http.Request, userID string) (done func(), ok bool) {
	g := h.heavyReads
	ctx, cancel := context.WithTimeout(r.Context(), g.wait)
	defer cancel()
	// One at a time for each person: wait while their last is running.
	for {
		g.mu.Lock()
		if !g.busy[userID] {
			g.busy[userID] = true
			g.mu.Unlock()
			break
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.refuse(w)
			return nil, false
		case <-time.After(20 * time.Millisecond):
		}
	}
	free := func() {
		g.mu.Lock()
		delete(g.busy, userID)
		g.mu.Unlock()
	}
	select {
	case g.slots <- struct{}{}:
	case <-ctx.Done():
		free()
		g.refuse(w)
		return nil, false
	}
	return func() { <-g.slots; free() }, true
}

func (g *heavyReads) refuse(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "5")
	writeError(w, http.StatusServiceUnavailable, "busy", "The server is busy with other searches. Try again in a few seconds, one search at a time.")
}
