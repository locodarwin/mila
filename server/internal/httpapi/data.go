package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	mimelib "mime"
	"net/http"
	"strconv"

	"slices"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/ownership"
)

// Settings, Import and export (exports, imports) and Profile (deleting an account).

func exportJSON(e ownership.Export) api.Export {
	out := api.Export{
		Id: e.ID, WorkspaceId: e.WorkspaceID, WorkspaceName: e.WorkspaceName, Format: api.ExportFormat(e.Format),
		Status: api.JobStatus(e.Status), FileName: e.FileName, Size: e.Size, CreatedAt: e.CreatedAt, ExpiresAt: e.ExpiresAt,
	}
	if e.Error.Valid {
		out.Error = &e.Error.String
	}
	if e.FinishedAt.Valid {
		out.FinishedAt = &e.FinishedAt.Time
	}
	return out
}

func importJSON(i ownership.Import) api.Import {
	out := api.Import{Id: i.ID, FileName: i.FileName, KeepIds: i.KeepIds, Status: api.JobStatus(i.Status), CreatedAt: i.CreatedAt}
	if i.Status == "ready" && i.WorkspaceID.Valid {
		out.WorkspaceId, out.WorkspaceName = &i.WorkspaceID.String, &i.WorkspaceName
	}
	var rep api.ImportReport
	if i.FinishedAt.Valid && json.Unmarshal([]byte(i.Report), &rep) == nil && rep.Restored != nil {
		out.Report = &rep
	}
	if i.Error.Valid {
		out.Error = &i.Error.String
	}
	if i.FinishedAt.Valid {
		out.FinishedAt = &i.FinishedAt.Time
	}
	return out
}

func (h *handlers) StartExport(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.StartExportJSONBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if !h.exportLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "360")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many exports started. Try again later.")
		return
	}
	e, err := h.data.StartExport(r.Context(), a.User.ID, workspaceID, string(body.Format))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, exportJSON(e))
}

func (h *handlers) ListExports(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.data.Exports(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Export, len(list))
	for i, e := range list {
		out[i] = exportJSON(e)
	}
	// A token for one workspace sees that one's.
	if a.Token != nil && a.Token.WorkspaceID.Valid {
		out = slices.DeleteFunc(out, func(e api.Export) bool { return e.WorkspaceId != a.Token.WorkspaceID.String })
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) DownloadExport(w http.ResponseWriter, r *http.Request, exportID api.ExportId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	f, e, err := h.data.OpenExport(r.Context(), a.User.ID, exportID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	if a.Token != nil && a.Token.WorkspaceID.Valid && a.Token.WorkspaceID.String != e.WorkspaceID {
		writeError(w, http.StatusNotFound, "not_found", "No such export.")
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hdr.Set("Cache-Control", "private, no-store")
	hdr.Set("Content-Type", "application/zip")
	hdr.Set("Content-Disposition", mimelib.FormatMediaType("attachment", map[string]string{"filename": e.FileName}))
	http.ServeContent(w, r, "", e.CreatedAt, f)
}

func (h *handlers) DeleteExport(w http.ResponseWriter, r *http.Request, exportID api.ExportId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.data.DeleteExport(r.Context(), a.User.ID, exportID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) ListImports(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.data.Imports(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Import, len(list))
	for i, im := range list {
		out[i] = importJSON(im)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) StartImport(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	limit := h.data.ImportMax()
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "Send the export as multipart/form-data in a field named file.")
		return
	}
	keep := false
	for {
		part, err := reader.NextPart()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "No file was sent.")
			return
		}
		switch part.FormName() {
		case "ids":
			b, _ := io.ReadAll(io.LimitReader(part, 16))
			switch string(b) {
			case string(api.Keep):
				keep = true
			case string(api.Fresh):
			default:
				writeError(w, http.StatusBadRequest, "invalid", "ids must be keep or fresh.")
				return
			}
			continue
		case "file":
		default:
			continue
		}
		im, err := h.data.StartImport(r.Context(), a.User.ID, part, cleanFilename(part.FileName()), keep)
		var tooBig *http.MaxBytesError
		if errors.Is(err, ownership.ErrTooLarge) || errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large",
				"That file is larger than this instance imports ("+mib(limit)+"). An admin can raise MILA_IMPORT_MAX_MB.")
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, importJSON(im))
		return
	}
}

func (h *handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.DeleteAccountJSONBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	if err := h.acc.DeleteAccount(r.Context(), a.User.ID, body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	clearSiteCache(w)
	noContent(w)
}

func (h *handlers) ListWorkspacePeople(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.acc.People(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.WorkspacePerson, len(list))
	for i, p := range list {
		out[i] = api.WorkspacePerson{
			UserId: p.ID, Name: p.Name, Avatar: avatarPath(p.Avatar), Guest: p.Guest,
			Projects: append([]string{}, p.Projects...),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ListFormerMembers(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.acc.FormerMembers(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.FormerMember, len(list))
	for i, m := range list {
		out[i] = api.FormerMember{UserId: m.ID, Name: m.Name, Deleted: m.DeletedAt.Valid}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) GetThemeUse(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	counts, err := h.acc.ThemeUse(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ThemeUse{Counts: counts})
}

func mib(n int64) string { return strconv.FormatInt(n>>20, 10) + " MB" }
