package httpapi

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// The REST endpoints for synced rows. rows_gen.go (written by restgen from
// the entity registry) hands every operation to one of these, by entity
// name; the rules are syncer's (rest.go), which writes through the merge.

// openAPI is shared/openapi.yaml as JSON, which npm run generate writes.
// It is served from here rather than web/static, which the service
// worker would download to every device.
//
//go:embed openapi.json
var openAPI []byte

// openAPIGzip is openAPI compressed, once: it is the same for every
// request.
var openAPIGzip = sync.OnceValue(func() []byte {
	var b bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	_, _ = gz.Write(openAPI)
	_ = gz.Close()
	return b.Bytes()
})

func (h *handlers) GetOpenApi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if accepts(r, "gzip") {
		// Set here, so compressJSON (which adds the Vary) leaves it as it is.
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(openAPIGzip())
		return
	}
	_, _ = w.Write(openAPI)
}

// maxRowBody is the largest entity body: notes are up to 100,000
// characters.
const maxRowBody = 1 << 20

// readOnly are fields a row shows that no request sets.
var readOnly = []string{"created_at", "updated_at"}

func writeFieldError(w http.ResponseWriter, field, format string, args ...any) {
	f := field
	writeJSON(w, http.StatusBadRequest, api.Error{Error: "invalid", Message: fmt.Sprintf(format, args...), Field: &f})
}

// apiUser returns the signed-in user if they are within the rate limit
// for reads or for writes; otherwise it answers and returns nil.
func (h *handlers) apiUser(w http.ResponseWriter, r *http.Request, write bool) *accounts.Authenticated {
	a := h.requireUser(w, r)
	if a == nil {
		return nil
	}
	limit := h.apiReads
	if write {
		limit = h.apiWrites
	}
	if !limit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Wait a minute and try again.")
		return nil
	}
	return a
}

// rowJSON is a row as REST shows it: flat, with its ID and timestamps.
func rowJSON(rec *syncer.Record) map[string]any {
	out := make(map[string]any, len(rec.Fields)+3)
	maps.Copy(out, rec.Fields)
	out["id"] = rec.ID
	out["created_at"] = rec.CreatedAt.Format(time.RFC3339Nano)
	out["updated_at"] = rec.UpdatedAt.Format(time.RFC3339Nano)
	return out
}

func writeRow(w http.ResponseWriter, status int, rec *syncer.Record) {
	w.Header().Set("ETag", syncer.ETag(rec.Clocks))
	writeJSON(w, status, rowJSON(rec))
}

// rowBody reads an entity body, keeping each field's JSON as sent, so a
// null and a field left out stay apart. It refuses the fields no request
// sets, and id unless allowed.
func rowBody(w http.ResponseWriter, r *http.Request, allowID bool) (map[string]json.RawMessage, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRowBody)
	var body map[string]json.RawMessage
	raw, err := io.ReadAll(r.Body)
	if err == nil && escapesNUL(raw) {
		writeError(w, http.StatusBadRequest, "invalid", nulMessage)
		return nil, false
	}
	if err == nil {
		err = json.Unmarshal(raw, &body)
	}
	if err != nil || body == nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest, "invalid", "The request body is larger than 1 MiB.")
		} else {
			writeError(w, http.StatusBadRequest, "invalid", "The request body is not a JSON object.")
		}
		return nil, false
	}
	for _, name := range readOnly {
		if _, ok := body[name]; ok {
			writeFieldError(w, name, "%s is set by the server.", name)
			return nil, false
		}
	}
	if _, ok := body["id"]; ok && !allowID {
		writeFieldError(w, "id", "id cannot be changed.")
		return nil, false
	}
	return body, true
}

// batchHeader reads a write's Mila-Batch header: none, or a name sync takes;
// anything else is refused.
func batchHeader(w http.ResponseWriter, r *http.Request) (string, bool) {
	b := r.Header.Get("Mila-Batch")
	if b != "" && !syncer.ValidBatch(b) {
		writeError(w, http.StatusBadRequest, "invalid", "A Mila-Batch is 1 to 64 letters, digits and dashes.")
		return "", false
	}
	return b, true
}

func (h *handlers) listRows(w http.ResponseWriter, r *http.Request, entity, workspaceID string) {
	a := h.apiUser(w, r, false)
	if a == nil {
		return
	}
	q := syncer.Query{Filters: map[string]string{}}
	for name, values := range r.URL.Query() {
		if len(values) != 1 {
			writeFieldError(w, name, "Give %s once.", name)
			return
		}
		v := values[0]
		switch name {
		case "after":
			q.After = v
		case "limit":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeFieldError(w, name, "limit must be from 1 to %d.", syncer.MaxLimit)
				return
			}
			q.Limit = n
		case "include_deleted":
			b, err := strconv.ParseBool(v)
			if err != nil {
				writeFieldError(w, name, "include_deleted must be true or false.")
				return
			}
			q.IncludeDeleted = b
		default:
			q.Filters[name] = v
		}
	}
	page, err := h.sync.List(r.Context(), a.User.ID, workspaceID, entity, q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]map[string]any, len(page.Rows))
	for i := range page.Rows {
		items[i] = rowJSON(&page.Rows[i])
	}
	out := map[string]any{"items": items}
	if page.Next != "" {
		out["next"] = page.Next
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) getRow(w http.ResponseWriter, r *http.Request, entity, workspaceID, id string) {
	a := h.apiUser(w, r, false)
	if a == nil {
		return
	}
	rec, err := h.sync.Get(r.Context(), a.User.ID, workspaceID, entity, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if etag := syncer.ETag(rec.Clocks); r.Header.Get("If-None-Match") != "" && syncer.ETagMatches(r.Header.Get("If-None-Match"), etag, true) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeRow(w, http.StatusOK, rec)
}

func (h *handlers) createRow(w http.ResponseWriter, r *http.Request, entity, workspaceID string, mode syncer.WriteMode) {
	a := h.apiUser(w, r, true)
	if a == nil {
		return
	}
	body, ok := rowBody(w, r, true)
	if !ok || !h.spend(w, r, a.User.ID, fieldsSize(body)) {
		return
	}
	var id string
	if raw, given := body["id"]; given {
		if json.Unmarshal(raw, &id) != nil {
			writeFieldError(w, "id", "id: must be a string")
			return
		}
		delete(body, "id")
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 200 {
		writeError(w, http.StatusBadRequest, "invalid", "The Idempotency-Key is longer than 200 characters.")
		return
	}
	batch, ok := batchHeader(w, r)
	if !ok {
		return
	}
	rec, created, err := h.sync.Write(r.Context(), a.User.ID, workspaceID, syncer.Write{Entity: entity, ID: id, Fields: body, Mode: mode, Key: key, Batch: batch})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		w.Header().Set("Location", r.URL.Path+"/"+rec.ID)
	}
	writeRow(w, status, rec)
}

func (h *handlers) patchRow(w http.ResponseWriter, r *http.Request, entity, workspaceID, id string) {
	a := h.apiUser(w, r, true)
	if a == nil {
		return
	}
	body, ok := rowBody(w, r, false)
	if !ok || !h.spend(w, r, a.User.ID, fieldsSize(body)) {
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "invalid", "Give at least one field to change.")
		return
	}
	batch, ok := batchHeader(w, r)
	if !ok {
		return
	}
	rec, _, err := h.sync.Write(r.Context(), a.User.ID, workspaceID, syncer.Write{
		Entity: entity, ID: id, Fields: body, Mode: syncer.Update, IfMatch: r.Header.Get("If-Match"), Batch: batch,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeRow(w, http.StatusOK, rec)
}

func (h *handlers) deleteRow(w http.ResponseWriter, r *http.Request, entity, workspaceID, id string) {
	a := h.apiUser(w, r, true)
	if a == nil {
		return
	}
	batch, ok := batchHeader(w, r)
	if !ok {
		return
	}
	_, _, err := h.sync.Write(r.Context(), a.User.ID, workspaceID, syncer.Write{
		Entity: entity, ID: id, Mode: syncer.Delete, IfMatch: r.Header.Get("If-Match"), Batch: batch,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}
