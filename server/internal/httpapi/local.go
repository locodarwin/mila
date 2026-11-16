package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/store"
)

// localClosed lists what makes no sense for one person with nobody else, no email
// and no way to sign in again: refusing it is the rule, and hiding it is only
// manners. Reading a workspace's people stays, since the app reads its own name there.
var localClosed = regexp.MustCompile(`^/api/v1/(` +
	`auth/(magic-link|setup|passkey|desktop|email|sign-out)` +
	`|me/(email|passkeys|tokens|delete|push|sessions|settings/notifications/test-email)` +
	`|invites` +
	`|admin` +
	`|workspaces/[^/]+/(invites|webhooks|email-to-task|join)` +
	`|workspaces/[^/]+/projects/[^/]+/guests` +
	`)(/|$)` +
	// Apps connecting by OAuth, which the mode never turns on.
	`|^/(oauth|\.well-known/oauth-[^/]+)(/|$)`)

// localOnly guards the desktop app's mode. It answers only to its own address, so
// that a website cannot reach it through the person's browser under a name of its
// own (DNS rebinding), and closes what doesn't belong. Its health check proves it
// knows the key, so that the app never gives the key to something else that has
// taken the port.
func localOnly(next http.Handler, origin, key string) http.Handler {
	own := ""
	if u, err := url.Parse(origin); err == nil {
		own = u.Host
	}
	sum := sha256.Sum256([]byte(key))
	proof := base64.RawURLEncoding.EncodeToString(sum[:])
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, own) {
			writeError(w, http.StatusForbidden, "forbidden", "This is only for the Mila app on this computer.")
			return
		}
		if r.URL.Path == "/healthz" {
			w.Header().Set("X-Mila-Local-Proof", proof)
		}
		if localClosed.MatchString(r.URL.Path) {
			writeError(w, http.StatusForbidden, "forbidden", "This isn't available when Mila keeps your work on this computer.")
			return
		}
		// Changing who is in a workspace: there is nobody else to change.
		if strings.HasPrefix(r.URL.Path, "/api/v1/workspaces/") && strings.Contains(r.URL.Path, "/members") && r.Method != http.MethodGet {
			writeError(w, http.StatusForbidden, "forbidden", "This isn't available when Mila keeps your work on this computer.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SignInLocal signs in the desktop app's window, with the key it started the
// server with. The key works once: the window then holds a session in its cookie,
// which nothing else on the computer has.
func (h *handlers) SignInLocal(w http.ResponseWriter, r *http.Request, params api.SignInLocalParams) {
	if !h.local {
		writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
		return
	}
	key := h.localKey.Load()
	if key == nil || subtle.ConstantTimeCompare([]byte(*key), []byte(params.Key)) != 1 || !h.localKey.CompareAndSwap(key, nil) {
		writeError(w, http.StatusForbidden, "forbidden", "That key is not valid.")
		return
	}
	in, err := h.acc.SignInLocal(r.Context(), clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setSessionCookie(w, in.SessionToken)
	h.setDeviceCookie(w, in.DeviceToken)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// localNotice is one of LocalWatch's notices, as the spec has it.
type localNotice = struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Id        string    `json:"id"`
	Title     string    `json:"title"`
}

// WatchLocal tells the desktop app, while its window shows another place, how many
// workspaces this place has and which notifications came since it last asked, so it
// can show them on the desktop. The watch key lasts as long as the server; unlike the
// sign-in key it gives no session, and reads nothing else.
func (h *handlers) WatchLocal(w http.ResponseWriter, r *http.Request, params api.WatchLocalParams) {
	if !h.local || h.localWatch == "" {
		writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(h.localWatch), []byte(params.XMilaWatch)) != 1 {
		writeError(w, http.StatusForbidden, "forbidden", "That key is not valid.")
		return
	}
	// The first time, only where to start: what came before is in the bell already.
	now := store.Now()
	if params.After == nil {
		spaces, _, err := h.acc.LocalWatch(r.Context(), now)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, api.LocalWatch{Workspaces: spaces, Notices: []localNotice{}, Cursor: now})
		return
	}
	spaces, unread, err := h.acc.LocalWatch(r.Context(), *params.After)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.LocalWatch{Workspaces: spaces, Cursor: *params.After}
	out.Notices = make([]localNotice, 0, len(unread))
	for _, n := range unread {
		out.Notices = append(out.Notices, localNotice{Body: n.Body, CreatedAt: n.CreatedAt, Id: n.ID, Title: n.Title})
		out.Cursor = n.CreatedAt
	}
	writeJSON(w, http.StatusOK, out)
}
