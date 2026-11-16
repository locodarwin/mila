package httpapi

import (
	"net/http"
	"strings"
	"sync"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// bearerToken reads the token from an Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(token), true
}

var restCollections = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	for _, e := range syncer.Describe() {
		out[e.Collection] = true
	}
	return out
})

// tokenRefuses says why an API token may not make this request, or "" if it may.
// It is a list of what is open, so a new endpoint stays closed until someone adds it
// here: a token is for the work itself, and never for signing in, sessions, passkeys,
// other tokens, webhooks, deleting an account or instance admin.
func tokenRefuses(method, path string, t *sqlitedb.ApiToken) string {
	read := method == http.MethodGet || method == http.MethodHead
	const closed = "An API token cannot do this. It needs you, signed in."
	const readOnly = "This API token is read only."
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return closed
	}
	parts := strings.Split(rest, "/")
	switch parts[0] {
	case "instance", "openapi.json":
		return ""
	case "mcp":
		// The MCP endpoint's tools call the API as the token, each refused
		// here or not on its own.
		if len(parts) == 1 {
			return ""
		}
		return closed
	case "me":
		switch {
		case rest == "me" && read:
			return ""
		case rest == "me/exports" && read:
			if !t.CanWrite {
				return readOnly
			}
			return ""
		}
		return closed
	case "exports":
		// An export holds everything, so it takes more than reading a row at a time.
		if !read || len(parts) < 2 {
			return closed
		}
		if !t.CanWrite {
			return readOnly
		}
		return ""
	case "workspaces":
		if len(parts) == 1 {
			if read {
				return ""
			}
			return closed
		}
	default:
		return closed
	}
	if t.WorkspaceID.Valid && t.WorkspaceID.String != parts[1] {
		return "This API token does not reach that workspace."
	}
	if len(parts) == 2 {
		if read {
			return ""
		}
		return closed
	}
	switch collection := parts[2]; {
	case collection == "exports":
		if !t.CanWrite {
			return readOnly
		}
		if len(parts) == 3 && method == http.MethodPost {
			return ""
		}
		return closed
	case collection == "people" || collection == "members" || collection == "trash":
		if read && len(parts) == 3 {
			return ""
		}
		return closed
	case collection == "actions":
		// The app's actions, for scripts and agents: reads with any token,
		// the rest with one that can write.
		if len(parts) != 4 {
			return closed
		}
		if !read && !t.CanWrite {
			return readOnly
		}
		return ""
	case !restCollections()[collection]:
		return closed
	}
	// A row, or one of the few things named under a row: what else hangs off
	// one, such as a project's guests, is not the work itself.
	if len(parts) == 5 && !tokenSubpaths[parts[2]+"/"+parts[4]] || len(parts) > 5 {
		return closed
	}
	if !read && !t.CanWrite {
		return readOnly
	}
	return ""
}

var tokenSubpaths = map[string]bool{
	"tasks/activity": true, "tasks/attachments": true, "pages/versions": true,
	"pages/attachments": true, "attachments/file": true,
}

func tokenJSON(t sqlitedb.ApiToken) api.ApiToken {
	out := api.ApiToken{Id: t.ID, Name: t.Name, CanWrite: t.CanWrite, CreatedAt: t.CreatedAt}
	if t.OauthClientID.Valid {
		app := true
		out.App = &app
	}
	if t.WorkspaceID.Valid {
		out.WorkspaceId = &t.WorkspaceID.String
	}
	if t.ExpiresAt.Valid {
		out.ExpiresAt = &t.ExpiresAt.Time
	}
	if t.LastUsedAt.Valid {
		out.LastUsedAt = &t.LastUsedAt.Time
	}
	return out
}

func (h *handlers) ListApiTokens(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.acc.Tokens(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.ApiToken, len(list))
	for i, t := range list {
		out[i] = tokenJSON(t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.CreateApiTokenJSONRequestBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	in := accounts.TokenInput{Name: body.Name, CanWrite: body.CanWrite}
	if body.WorkspaceId != nil {
		in.WorkspaceID = *body.WorkspaceId
	}
	if body.ExpiresInDays != nil {
		in.ExpiresInDays = *body.ExpiresInDays
		if in.ExpiresInDays < 1 {
			writeError(w, http.StatusBadRequest, "invalid", "A token lasts at least a day.")
			return
		}
	}
	t, secret, err := h.acc.CreateToken(r.Context(), a.User.ID, in, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.NewApiToken{Token: tokenJSON(t), Secret: secret})
}

func (h *handlers) RevokeApiToken(w http.ResponseWriter, r *http.Request, tokenID string) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.RevokeToken(r.Context(), a.User.ID, tokenID, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
