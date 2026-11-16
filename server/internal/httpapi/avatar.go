package httpapi

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/locodarwin/mila/server/internal/avatar"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
)

// People's pictures: set and removed by their owner (or taken down by an
// instance admin), and served to those who may see them under a URL that
// names a random key, so a URL never changes what it shows.

var avatarKey = regexp.MustCompile(`^[0-9a-f]{32}$`)

// avatarPath is where a picture is served, or nil for none.
func avatarPath(key sql.NullString) *string {
	if !key.Valid || key.String == "" {
		return nil
	}
	p := "/api/v1/avatars/" + key.String
	return &p
}

// clearSiteCache asks the browser to empty its HTTP cache, which holds
// pictures and attachments, when someone signs out or leaves. Cache
// Storage (the app shell) is left alone. Browsers honor it only in secure
// contexts.
func clearSiteCache(w http.ResponseWriter) {
	w.Header().Set("Clear-Site-Data", `"cache"`)
}

func (h *handlers) SetAvatar(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.avatarLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "120")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many picture changes. Try again later.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, avatar.MaxBytes+1<<20)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "Send the picture as multipart/form-data in a field named file.")
		return
	}
	var data []byte
	for {
		part, err := reader.NextPart()
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Pictures can be at most 4 MB.")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "No picture was sent.")
			return
		}
		if part.FormName() != "file" {
			continue
		}
		data, err = io.ReadAll(io.LimitReader(part, avatar.MaxBytes+1))
		if errors.As(err, &tooBig) || len(data) > avatar.MaxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Pictures can be at most 4 MB.")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "The picture did not arrive whole.")
			return
		}
		break
	}
	out, _, err := avatar.Process(r.Context(), data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	key, _, _, err := h.files.Save(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	u, err := h.acc.SetAvatar(r.Context(), a.User.ID, key)
	if err != nil {
		h.fail(w, r, err) // the file is swept up later
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (h *handlers) RemoveAvatar(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.avatarLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "120")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many picture changes. Try again later.")
		return
	}
	u, err := h.acc.SetAvatar(r.Context(), a.User.ID, "")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (h *handlers) GetAvatar(w http.ResponseWriter, r *http.Request, key api.AvatarKey) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	notFound := func() { writeError(w, http.StatusNotFound, "not_found", "No such picture.") }
	// The spec's pattern is not enforced by the generated router.
	if !avatarKey.MatchString(key) {
		notFound()
		return
	}
	ok, err := h.acc.AvatarVisible(r.Context(), a.User, key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !ok {
		notFound()
		return
	}
	f, err := h.files.Open(key)
	if err != nil {
		notFound()
		return
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := http.DetectContentType(head[:n])
	if _, err := f.Seek(0, io.SeekStart); err != nil || (mime != "image/jpeg" && mime != "image/png") {
		notFound()
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", mime)
	// A key is never reused, so what it names never changes.
	hdr.Set("Cache-Control", "private, max-age=31536000, immutable")
	hdr.Set("ETag", `"`+key+`"`)
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hdr.Set("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (h *handlers) AdminRemoveAvatar(w http.ResponseWriter, r *http.Request, userID api.UserId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	u, err := h.acc.RemoveAvatarAsAdmin(r.Context(), a.User.ID, userID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserJSON(u))
}
