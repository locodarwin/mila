package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
)

// People's own themes. A theme is sent and stored as its document, which
// accounts checks; it is never decoded through the generated types, which
// would drop what they do not know and change its numbers.

// themeBodyLimit is how much of a PUT is read: twice the largest theme,
// for white space.
const themeBodyLimit = 32 << 10

func (h *handlers) ListThemes(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	themes, err := h.acc.ListThemes(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := struct {
		Themes []json.RawMessage `json:"themes"`
	}{Themes: make([]json.RawMessage, len(themes))}
	for i, t := range themes {
		out.Themes[i] = t.Body
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) GetTheme(w http.ResponseWriter, r *http.Request, themeID api.ThemeId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	t, err := h.acc.GetTheme(r.Context(), a.User.ID, themeID.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t.Body)
}

func (h *handlers) PutTheme(w http.ResponseWriter, r *http.Request, themeID api.ThemeId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, themeBodyLimit))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "A theme can be at most 16 KiB.")
		return
	}
	if err != nil || !json.Valid(raw) {
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid JSON for this endpoint.")
		return
	}
	t, err := h.acc.SaveTheme(r.Context(), a.User.ID, themeID.String(), raw)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t.Body)
}

func (h *handlers) DeleteTheme(w http.ResponseWriter, r *http.Request, themeID api.ThemeId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.DeleteTheme(r.Context(), a.User.ID, themeID.String()); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// customThemeJSON is the customTheme preference as the API types hold it.
func customThemeJSON(id *string) *api.ThemeId {
	if id == nil {
		return nil
	}
	var u api.ThemeId
	if u.UnmarshalText([]byte(*id)) != nil {
		return nil
	}
	return &u
}

// themeAnswersJSON is the suggestions answered, as ids; never null, which
// the app would not take.
func themeAnswersJSON(ids []string) []openapi_types.UUID {
	out := make([]openapi_types.UUID, 0, len(ids))
	for _, id := range ids {
		var u openapi_types.UUID
		if u.UnmarshalText([]byte(id)) == nil {
			out = append(out, u)
		}
	}
	return out
}
