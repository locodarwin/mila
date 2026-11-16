package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/syncer"
)

const (
	// writeBudget is how much one person may write through sync and the
	// REST endpoints together: this much at once, then this much an hour.
	// A day of typing is well under a megabyte, and a CSV import of 5000
	// tasks a few; a page written whole a thousand times is not.
	writeBudget = 32 << 20
	// smallWrite is what may still be written while the disk is low: an
	// edit, a deletion to make room; never a document written whole.
	smallWrite = 64 << 10
)

// spend takes n bytes from the person's write budget and, while the
// database's disk is low, refuses more than a small write; otherwise it
// answers 429 or 503 with a Retry-After. The app keeps what it could not
// send and sends it then.
func (h *handlers) spend(w http.ResponseWriter, r *http.Request, userID string, n int) bool {
	if n > smallWrite && h.dialect == "sqlite" && h.files != nil && h.files.Room() != nil {
		w.Header().Set("Retry-After", "300")
		h.fail(w, r, apperr.New(apperr.Unavailable, "The server is almost out of disk space, so it is taking only small changes now. Tell whoever runs it."))
		return false
	}
	ok, wait := h.writeBytes.Take(userID, float64(n))
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too much was written in the last hour. Try again in a few minutes.")
		return false
	}
	return true
}

// opsSize is what a push asks to write, roughly as it was sent.
func opsSize(ops []syncer.Op) int {
	n := 0
	for _, op := range ops {
		n += len(op.OpID) + len(op.Entity) + len(op.ID) + len(op.HLC) + fieldsSize(op.Fields)
	}
	return n
}

func fieldsSize(fields map[string]json.RawMessage) int {
	n := 0
	for k, v := range fields {
		n += len(k) + len(v)
	}
	return n
}
