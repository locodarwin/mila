package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	mimelib "mime"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/files"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func (h *handlers) SyncBootstrap(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.SyncBootstrapParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := syncer.CheckSchema(schemaOf(params.Schema)); err != nil {
		h.fail(w, r, err)
		return
	}
	// The rows are written as they are read, a row at a time: a large
	// workspace's snapshot is megabytes of JSON, and encoding it whole
	// would hold all of it, and its copy, in memory at once.
	started, first := false, true
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := h.sync.StreamBootstrap(r.Context(), a.User.ID, workspaceID, func(seq int64, epoch string) error {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		started = true
		head, _ := json.Marshal(epoch)
		_, err := fmt.Fprintf(w, `{"seq":%d,"epoch":%s,"rows":[`, seq, head)
		return err
	}, func(row syncer.Row) error {
		buf.Reset()
		if !first {
			buf.WriteByte(',')
		}
		first = false
		if err := enc.Encode(row); err != nil {
			return err
		}
		_, err := w.Write(buf.Bytes())
		return err
	})
	switch {
	case err != nil && !started:
		h.fail(w, r, err)
	case err != nil:
		// The answer is cut short, and must not look whole.
		h.log.Warn("bootstrap cut short", "err", err)
		panic(http.ErrAbortHandler)
	default:
		_, _ = w.Write([]byte("]}\n"))
	}
}

func (h *handlers) SyncChanges(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, params api.SyncChangesParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := syncer.CheckSchema(schemaOf(params.Schema)); err != nil {
		h.fail(w, r, err)
		return
	}
	epoch := ""
	if params.Epoch != nil {
		epoch = *params.Epoch
	}
	if err := h.sync.CheckEpoch(r.Context(), epoch); err != nil {
		h.fail(w, r, err)
		return
	}
	changes, err := h.sync.Pull(r.Context(), a.User.ID, workspaceID, params.After)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

// schemaOf is the schema version a read names. An app that names none is
// older than reads naming one, and syncs an older version (see SYNC_SCHEMA 3
// in web/src/lib/sync/outbox.ts): it must reload before it reads.
func schemaOf(v *api.SyncSchema) int {
	if v == nil {
		return 0
	}
	return *v
}

type pushBody struct {
	// Schema is missing (0) from apps older than the version gate.
	Schema int `json:"schema"`
	// User is whom the app made the ops as; another account signed in
	// since, in another tab, must not send them as its own.
	User string      `json:"user"`
	Ops  []syncer.Op `json:"ops"`
}

type pushResult struct {
	Results []syncer.Result `json:"results"`
	Seq     int64           `json:"seq"`
}

func (h *handlers) SyncPush(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body pushBody
	if a == nil || !h.decodeLimit(w, r, &body, 4<<20) {
		return
	}
	if err := syncer.CheckSchema(body.Schema); err != nil {
		h.fail(w, r, err)
		return
	}
	if body.User != "" && body.User != a.User.ID {
		writeError(w, http.StatusConflict, "other_user", "These changes were made by someone else signed in on this device.")
		return
	}
	if !h.spend(w, r, a.User.ID, opsSize(body.Ops)) {
		return
	}
	results, seq, err := h.sync.Push(r.Context(), a.User.ID, workspaceID, body.Ops)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pushResult{Results: results, Seq: seq})
}

// syncSocket upgrades to the WebSocket that nudges clients to pull.
func (h *handlers) syncSocket(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	h.hub.Serve(w, r, a.User.ID, a.Session.ID, h.originHosts)
}

func (h *handlers) ListTrash(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	rows, err := h.sync.Trash(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handlers) ListPageVersions(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, pageID api.PageId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	versions, err := h.sync.PageVersions(r.Context(), a.User.ID, workspaceID, pageID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (h *handlers) ListActivity(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, taskID api.TaskId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	events, err := h.sync.Activity(r.Context(), a.User.ID, workspaceID, taskID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// MaxUpload is the largest attachment accepted.
const MaxUpload = 25 << 20

func (h *handlers) UploadAttachment(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, taskID api.TaskId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.sync.CanEditTask(r.Context(), a.User.ID, workspaceID, taskID); err != nil {
		h.fail(w, r, err)
		return
	}
	h.upload(w, r, a.User.ID, workspaceID, syncer.NewAttachment{TaskID: taskID})
}

func (h *handlers) UploadPageAttachment(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, pageID api.PageId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.sync.CanEditPage(r.Context(), a.User.ID, workspaceID, pageID); err != nil {
		h.fail(w, r, err)
		return
	}
	h.upload(w, r, a.User.ID, workspaceID, syncer.NewAttachment{PageID: pageID})
}

// upload saves the file a request sends and adds it to the task or page
// the attachment names.
func (h *handlers) upload(w http.ResponseWriter, r *http.Request, userID, workspaceID string, to syncer.NewAttachment) {
	if !h.uploadLimit.Allow(userID) {
		w.Header().Set("Retry-After", "120")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many files uploaded. Try again later.")
		return
	}
	if err := h.files.Room(); err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUpload+1<<20)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "Send the file as multipart/form-data in a field named file.")
		return
	}
	for {
		part, err := reader.NextPart()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "No file was sent.")
			return
		}
		if part.FormName() != "file" {
			continue
		}
		key, size, head, err := h.files.Save(part, MaxUpload)
		if errors.Is(err, files.ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Files can be at most 25 MB.")
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		to.ID, to.Filename = uuid.NewString(), cleanFilename(part.FileName())
		to.Mime, to.StorageKey, to.Size = mediaType(part.Header.Get("Content-Type"), head), key, size
		row, err := h.sync.CreateAttachment(r.Context(), userID, workspaceID, to)
		if err != nil {
			h.fail(w, r, err) // the file is swept up later
			return
		}
		writeJSON(w, http.StatusCreated, row)
		return
	}
}

// inlineTypes are shown in the browser; everything else downloads. SVG is
// left out on purpose: it can carry script.
var inlineTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

func (h *handlers) DownloadAttachment(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, attachmentID api.AttachmentId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	row, err := h.sync.Attachment(r.Context(), a.User.ID, workspaceID, attachmentID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	f, err := h.files.Open(row.Fields["storage_key"].(string))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	mime, _ := row.Fields["mime"].(string)
	name, _ := row.Fields["filename"].(string)
	hdr := w.Header()
	// Served files are never trusted as pages, whatever they claim to be.
	hdr.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	hdr.Set("Cache-Control", "private, max-age=3600")
	disposition := "attachment"
	if inlineTypes[mime] {
		disposition = "inline"
		hdr.Set("Content-Type", mime)
	} else {
		hdr.Set("Content-Type", "application/octet-stream")
	}
	hdr.Set("Content-Disposition", mimelib.FormatMediaType(disposition, map[string]string{"filename": name}))
	http.ServeContent(w, r, "", time.Time{}, f)
}

func cleanFilename(name string) string { return files.CleanName(name) }

// mediaType trusts the browser's type only if it is well formed, and
// otherwise sniffs the bytes.
func mediaType(declared string, head []byte) string { return files.MediaType(declared, head) }
