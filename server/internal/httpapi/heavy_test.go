package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

// One heavy read at a time for each person, a few in the server; a request
// that can't get a turn in time is told to come back.
func TestHeavyReadsTakeTurns(t *testing.T) {
	h := &handlers{heavyReads: newHeavyReads(2, 50*time.Millisecond)}
	turn := func(user string) (func(), int) {
		w := httptest.NewRecorder()
		done, ok := h.heavy(w, httptest.NewRequest("GET", "/", nil), user)
		if !ok {
			return nil, w.Code
		}
		return done, 200
	}
	doneA, code := turn("a")
	if code != 200 {
		t.Fatalf("a's first = %d", code)
	}
	if _, code := turn("a"); code != 503 {
		t.Fatalf("a's second while the first runs = %d, want 503", code)
	}
	doneB, code := turn("b")
	if code != 200 {
		t.Fatalf("b alongside a = %d", code)
	}
	if _, code := turn("c"); code != 503 {
		t.Fatalf("a third person with both slots taken = %d, want 503", code)
	}
	// A turn freed while someone waits goes to them.
	go func() { time.Sleep(10 * time.Millisecond); doneA() }()
	doneA2, code := turn("a")
	if code != 200 {
		t.Fatalf("a after the first ended = %d", code)
	}
	doneA2()
	doneB()
	if done, code := turn("c"); code != 200 {
		t.Fatalf("c once both are free = %d", code)
	} else {
		done()
	}
}
