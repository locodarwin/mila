package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"slices"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/actions"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/automations"
	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/ical"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/ownership"
	"github.com/locodarwin/mila/server/internal/push"
	"github.com/locodarwin/mila/server/internal/ratelimit"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/updates"
	"github.com/locodarwin/mila/server/internal/version"
	"github.com/locodarwin/mila/server/internal/webhooks"
)

const (
	cookieName = "mila_session"
	// deviceCookie holds a random token that stays with the browser across
	// sign-ins, so a sign-in from a browser an account has not used before
	// can be told apart.
	deviceCookie = "mila_device"
)

// handlers implements api.ServerInterface.
type handlers struct {
	// root is the whole server, for requests made inside it (the MCP
	// endpoint's tools); set once New has built it.
	root http.Handler
	// origin is the public scheme and host, OAuth's issuer.
	origin string
	// oauth lets apps connect to the MCP endpoint by OAuth, and
	// oauthRegister bounds how many register from one network.
	oauth         bool
	oauthRegister *ratelimit.Limiter
	// oauthTokens bounds codes traded from one address, apart from
	// sign-ins: a hosted assistant trades every person's from the same few.
	oauthTokens *ratelimit.Limiter
	// cookie is the session cookie's name: in the desktop app's mode it
	// names the port too, since cookies ignore ports and other servers on
	// this computer would read, or overwrite, the local place's.
	cookie      string
	acc         *accounts.Service
	sync        *syncer.Service
	actions     *actions.Service
	hub         *syncer.Hub
	files       *files.Store
	log         *slog.Logger
	https       bool
	signInLimit *ratelimit.Limiter
	originHosts []string
	notify      *notify.Service
	// testMailLimit keeps "send a test email" from becoming a spam button.
	testMailLimit *ratelimit.Limiter
	push          *push.Service
	// testPushLimit does the same for "send a test push".
	testPushLimit *ratelimit.Limiter
	calendar      *ical.Service
	// feedLimit bounds calendar feed fetches per client address.
	feedLimit *ratelimit.Limiter
	// apiReads and apiWrites bound the REST entity endpoints per user; the
	// app itself never calls them.
	apiReads, apiWrites *ratelimit.Limiter
	// heavyReads bounds find-tasks, which reads a whole workspace.
	heavyReads *heavyReads
	// avatarLimit bounds picture changes per user, each a decode, and
	// renameLimit name changes; both tell everyone the user works with.
	avatarLimit, renameLimit *ratelimit.Limiter
	// emailLimit is requests to change one's email address, per person.
	emailLimit *ratelimit.Limiter
	// inviteLimit is invitations sent, per person: each is an email with
	// words its sender chose.
	inviteLimit *ratelimit.Limiter
	// uploadLimit is attachments uploaded, per person, which with
	// files.MinFree keeps one person from filling the disk.
	uploadLimit *ratelimit.Limiter
	// exportLimit is exports started, per person: each reads a whole
	// workspace and copies its files, and deleting one makes room for
	// another under ownership's cap on those kept.
	exportLimit *ratelimit.Limiter
	// testHookLimit is webhook test deliveries, per person.
	testHookLimit *ratelimit.Limiter
	// writeBytes is what each person writes through sync and REST, in
	// bytes (writebudget.go).
	writeBytes  *ratelimit.Limiter
	data        *ownership.Service
	hooks       *webhooks.Service
	automations *automations.Service
	// For the admin page.
	backup   *backup.Service
	schema   Schema
	dialect  string
	mailMode string
	// localKey is the desktop app's one-time key, while it lasts, and local says
	// this is that mode at all.
	local    bool
	localKey atomic.Pointer[string]
	// localWatch is the key the app reads new notifications with; empty for none.
	localWatch string
	// inboundAddress is what a person's email to task address looks like; empty
	// when this server reads no mailbox.
	inboundAddress string
	emailIn        func() (time.Time, string)
	microsoft      MicrosoftMailbox
	updates        func() *updates.Release
	baseURL        string
}

var _ api.ServerInterface = (*handlers)(nil)

// Sessions

func (h *handlers) sessionCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(h.cookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

// anySessionCookie is whether the request carries a session cookie of any
// Mila's, for the origin check, which must not let one through unchecked.
func anySessionCookie(r *http.Request) bool {
	for _, c := range r.Cookies() {
		if strings.HasPrefix(c.Name, cookieName) && c.Value != "" {
			return true
		}
	}
	return false
}

func (h *handlers) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: h.cookie, Value: token, Path: "/",
		MaxAge:   int(accounts.SessionLifetime / time.Second),
		HttpOnly: true, Secure: h.https, SameSite: http.SameSiteLaxMode,
	})
}

func (h *handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: h.cookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.https, SameSite: http.SameSiteLaxMode,
	})
}

// setDeviceCookie keeps the browser's device token for as long as a
// device is remembered, renewed at every sign-in.
func (h *handlers) setDeviceCookie(w http.ResponseWriter, token string) {
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: deviceCookie, Value: token, Path: "/",
		MaxAge:   int(accounts.DeviceLifetime / time.Second),
		HttpOnly: true, Secure: h.https, SameSite: http.SameSiteLaxMode,
	})
}

// client is where a request came from.
func clientOf(r *http.Request) accounts.Client {
	c := accounts.Client{UserAgent: userAgent(r), IP: clientIP(r)}
	if cookie, err := r.Cookie(deviceCookie); err == nil {
		c.Device = cookie.Value
	}
	return c
}

type authKey struct{}

// authenticate resolves the session cookie on API requests and puts the
// result in the context. A stale cookie is cleared.
func (h *handlers) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret, ok := bearerToken(r); ok && strings.HasPrefix(r.URL.Path, "/api/") {
			auth, err := h.acc.AuthenticateToken(r.Context(), secret)
			if accounts.KindOf(err) == accounts.Unauthenticated {
				writeError(w, http.StatusUnauthorized, "invalid_token", err.Error())
				return
			}
			if err != nil {
				h.fail(w, r, err)
				return
			}
			if why := tokenRefuses(r.Method, r.URL.Path, auth.Token); why != "" {
				writeError(w, http.StatusForbidden, "token_not_allowed", why)
				return
			}
			// An app's token is for the MCP endpoint, whose tools reach the
			// rest of the API from inside the server; and only while apps
			// may connect.
			if auth.Token.OauthClientID.Valid {
				if !h.oauth {
					writeError(w, http.StatusUnauthorized, "invalid_token", "This server no longer lets apps connect by OAuth.")
					return
				}
				if r.URL.Path != mcpPath && !fromInside(r.Context()) {
					writeError(w, http.StatusForbidden, "token_not_allowed", "This token was given to an app for Mila's MCP endpoint, and reaches nothing else.")
					return
				}
			}
			ctx := context.WithValue(r.Context(), authKey{}, auth)
			next.ServeHTTP(w, r.WithContext(syncer.WithVia(ctx, auth.Token.Name)))
			return
		}
		token, ok := h.sessionCookie(r)
		if !ok || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		auth, err := h.acc.Authenticate(r.Context(), token, clientIP(r))
		switch {
		case err == nil:
			r = r.WithContext(context.WithValue(r.Context(), authKey{}, auth))
		case accounts.KindOf(err) == accounts.Unauthenticated:
			h.clearSessionCookie(w)
		default:
			h.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authOf(r *http.Request) *accounts.Authenticated {
	a, _ := r.Context().Value(authKey{}).(*accounts.Authenticated)
	return a
}

// requireUser returns the signed-in user, or answers 401 and returns nil.
func (h *handlers) requireUser(w http.ResponseWriter, r *http.Request) *accounts.Authenticated {
	a := authOf(r)
	if a == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Sign in first.")
	}
	return a
}

// limited answers 429 when the client has made too many sign-in attempts.
func (h *handlers) limited(w http.ResponseWriter, r *http.Request) bool {
	if h.signInLimit.Allow(rateKey(r)) {
		return false
	}
	w.Header().Set("Retry-After", "60")
	writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Wait a minute and try again.")
	return true
}

// Plumbing

// decode reads a JSON body of at most 64 KiB into v, refusing one that
// writes a NUL into any of its text (refusenul.go).
func (h *handlers) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err == nil && escapesNUL(body) {
		writeError(w, http.StatusBadRequest, "invalid", nulMessage)
		return false
	}
	if err != nil || json.Unmarshal(body, v) != nil {
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid JSON for this endpoint.")
		return false
	}
	return true
}

// decodeLimit reads a JSON body of at most limit bytes into v. Sync's push
// uses it, and refuses a NUL op by op instead.
func (h *handlers) decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "The request body is not valid JSON for this endpoint.")
		return false
	}
	return true
}

var kindStatus = map[accounts.Kind]struct {
	status int
	code   string
}{
	accounts.Invalid:          {http.StatusBadRequest, "invalid"},
	accounts.NotFound:         {http.StatusNotFound, "not_found"},
	accounts.Forbidden:        {http.StatusForbidden, "forbidden"},
	accounts.Conflict:         {http.StatusConflict, "conflict"},
	accounts.Unauthenticated:  {http.StatusUnauthorized, "unauthenticated"},
	apperr.Gone:               {http.StatusGone, "gone"},
	apperr.Unavailable:        {http.StatusServiceUnavailable, "unavailable"},
	apperr.UpdateRequired:     {http.StatusUpgradeRequired, "update_required"},
	apperr.PreconditionFailed: {http.StatusPreconditionFailed, "precondition_failed"},
}

// fail answers with an accounts error's status and message, or a 500 that
// reveals nothing for anything unexpected.
func (h *handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var e *accounts.Error
	if errors.As(err, &e) {
		s := kindStatus[e.Kind]
		out := api.Error{Error: s.code, Message: e.Message}
		if e.Code != "" {
			out.Error = e.Code
		}
		if e.Field != "" {
			out.Field = &e.Field
		}
		writeJSON(w, s.status, out)
		return
	}
	// A browser that closed the page or moved on cancels its requests. That
	// is no failure of the server's, and nobody is left to read the answer.
	if r.Context().Err() != nil {
		h.log.Debug("request ended by the client", "method", r.Method, "path", r.URL.Path, "err", err)
	} else {
		h.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong on the server.")
}

func noContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

func userJSON(u store.User) api.User {
	return api.User{Id: u.ID, Email: u.Email, Name: u.Name, Avatar: avatarPath(u.Avatar), IsInstanceAdmin: u.IsInstanceAdmin,
		Timezone: accounts.Timezone(u), Preferences: preferencesJSON(accounts.PreferencesOf(u))}
}

func preferencesJSON(p accounts.Preferences) api.Preferences {
	items := func(list []string) []api.SidebarItem {
		out := make([]api.SidebarItem, len(list))
		for i, v := range list {
			out[i] = api.SidebarItem(v)
		}
		return out
	}
	// Sent as {} rather than null, which the app would not take.
	shortcuts := p.Shortcuts
	if shortcuts == nil {
		shortcuts = map[string][]string{}
	}
	return api.Preferences{
		Theme: api.PreferencesTheme(p.Theme), Scheme: api.PreferencesScheme(p.Scheme), AccentHue: p.AccentHue,
		Density: api.PreferencesDensity(p.Density), FontSize: api.PreferencesFontSize(p.FontSize),
		WeekStart: api.PreferencesWeekStart(p.WeekStart), DateFormat: api.PreferencesDateFormat(p.DateFormat),
		Clock: api.PreferencesClock(p.Clock), ShowCompleted: p.ShowCompleted,
		StartPage: api.PreferencesStartPage(p.StartPage), SidebarOrder: items(p.SidebarOrder),
		SidebarHidden: items(p.SidebarHidden), SingleKeyShortcuts: p.SingleKeyShortcuts,
		Shortcuts: shortcuts, CustomTheme: customThemeJSON(p.CustomTheme),
		ThemeAnswers: themeAnswersJSON(p.ThemeAnswers),
		TaskPanel:    api.PreferencesTaskPanel(p.TaskPanel),
		WikiToolbar:  p.WikiToolbar,
	}
}

// signedIn sends a new session's cookies and the user. A session the
// browser already had ends: signing in again, for something that wants
// a recent sign-in, leaves one session for the browser, not two.
func (h *handlers) signedIn(w http.ResponseWriter, r *http.Request, in *accounts.SignedIn) {
	if old, ok := h.sessionCookie(r); ok {
		if err := h.acc.SignOut(r.Context(), old); err != nil {
			h.log.Warn("ending the session a sign-in replaced", "err", err)
		}
	}
	h.setSessionCookie(w, in.SessionToken)
	h.setDeviceCookie(w, in.DeviceToken)
	writeJSON(w, http.StatusOK, userJSON(in.User))
}

// Instance and auth

func (h *handlers) GetInstance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.Instance{Name: "Mila", Version: version.Version})
}

func (h *handlers) GetAuthState(w http.ResponseWriter, r *http.Request) {
	needed, err := h.acc.SetupRequired(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	state := api.AuthState{SetupRequired: needed, Passkeys: h.acc.PasskeysAvailable()}
	if h.local {
		state.Local = &h.local
	}
	if a := authOf(r); a != nil {
		u := userJSON(a.User)
		state.User = &u
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *handlers) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	var body api.CompleteSetupJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	in, err := h.acc.CompleteSetup(r.Context(), body.Token, body.Name, body.Email, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, in)
}

func (h *handlers) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var body api.RequestMagicLinkJSONBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	if err := h.acc.RequestMagicLink(r.Context(), body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) PeekMagicLink(w http.ResponseWriter, r *http.Request) {
	var body api.PeekMagicLinkJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	email, err := h.acc.MagicLinkEmail(r.Context(), body.Token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (h *handlers) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	var body api.VerifyMagicLinkJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	in, err := h.acc.VerifyMagicLink(r.Context(), body.Token, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, in)
}

// CreateDesktopCode makes the code a signed-in browser hands to the desktop app.
// It asks for a recent sign-in: an old session left in a browser must not be able
// to start a new one elsewhere.
func (h *handlers) CreateDesktopCode(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.CreateDesktopCodeJSONRequestBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	code, err := h.acc.CreateDesktopCode(r.Context(), a.User.ID, a.Session.Method, body.Challenge)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.DesktopCode{Code: code})
}

// FinishDesktopSignIn signs the desktop app's window in.
func (h *handlers) FinishDesktopSignIn(w http.ResponseWriter, r *http.Request) {
	var body api.FinishDesktopSignInJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	in, err := h.acc.FinishDesktopSignIn(r.Context(), body.Code, body.Verifier, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, in)
}

func (h *handlers) RequestEmailChange(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.RequestEmailChangeJSONBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	if !h.emailLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "720")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many changes of address asked for. Try again later.")
		return
	}
	if err := h.acc.RequestEmailChange(r.Context(), a.User.ID, a.Session.ID, body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) ConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	var body api.ConfirmEmailChangeJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	u, err := h.acc.ConfirmEmailChange(r.Context(), body.Token, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func ceremonyJSON(c *accounts.Ceremony) (api.Ceremony, error) {
	// The webauthn library's options marshal to the JSON the browser's
	// PublicKeyCredential.parse*OptionsFromJSON expects.
	raw, err := json.Marshal(c.Options)
	if err != nil {
		return api.Ceremony{}, err
	}
	var options map[string]any
	if err := json.Unmarshal(raw, &options); err != nil {
		return api.Ceremony{}, err
	}
	return api.Ceremony{CeremonyId: c.ID, Options: options}, nil
}

func (h *handlers) writeCeremony(w http.ResponseWriter, r *http.Request, c *accounts.Ceremony, err error) {
	if err == nil {
		var out api.Ceremony
		if out, err = ceremonyJSON(c); err == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	h.fail(w, r, err)
}

func (h *handlers) BeginPasskeySignIn(w http.ResponseWriter, r *http.Request) {
	if h.limited(w, r) {
		return
	}
	c, err := h.acc.BeginPasskeyLogin(r.Context())
	h.writeCeremony(w, r, c, err)
}

func (h *handlers) FinishPasskeySignIn(w http.ResponseWriter, r *http.Request) {
	var body api.FinishPasskeySignInJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	cred, err := json.Marshal(body.Credential)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	in, err := h.acc.FinishPasskeyLogin(r.Context(), body.CeremonyId, cred, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, in)
}

func (h *handlers) SignOut(w http.ResponseWriter, r *http.Request) {
	if token, ok := h.sessionCookie(r); ok {
		if err := h.acc.SignOut(r.Context(), token); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	h.clearSessionCookie(w)
	clearSiteCache(w)
	noContent(w)
}

// Me

func (h *handlers) GetMe(w http.ResponseWriter, r *http.Request) {
	if a := h.requireUser(w, r); a != nil {
		writeJSON(w, http.StatusOK, userJSON(a.User))
	}
}

func (h *handlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.UpdateMeJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	// Each rename tells everyone who shares a workspace with them.
	if !h.renameLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "120")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many name changes. Try again later.")
		return
	}
	u, err := h.acc.Rename(r.Context(), a.User.ID, body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (h *handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	sessions, err := h.acc.Sessions(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Session, len(sessions))
	for i, s := range sessions {
		out[i] = api.Session{
			Id: s.ID, UserAgent: s.UserAgent, Ip: s.Ip, CreatedAt: s.CreatedAt, LastSeenAt: s.LastSeenAt,
			Current: s.ID == a.Session.ID,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) RevokeSession(w http.ResponseWriter, r *http.Request, sessionID api.SessionId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.RevokeSession(r.Context(), a.User.ID, sessionID, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	if sessionID == a.Session.ID {
		h.clearSessionCookie(w)
	}
	noContent(w)
}

func passkeyJSON(k store.Passkey) api.Passkey {
	p := api.Passkey{Id: k.ID, Name: k.Name, CreatedAt: k.CreatedAt}
	if k.LastUsedAt.Valid {
		p.LastUsedAt = &k.LastUsedAt.Time
	}
	return p
}

func (h *handlers) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	keys, err := h.acc.Passkeys(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Passkey, len(keys))
	for i, k := range keys {
		out[i] = passkeyJSON(k)
	}
	writeJSON(w, http.StatusOK, out)
}

// recentSignIn is how long after signing in someone may do what would let
// a stolen session outlast its theft: add a passkey, change the email
// address, delete the account, make a calendar feed.
const recentSignIn = 10 * time.Minute

// requireRecentSignIn refuses, with the code sign_in_again, a session
// signed in longer ago than recentSignIn, and for an account with a
// passkey, one not signed in with a passkey: an email link is no proof
// against someone who can read the mailbox for a moment. The app offers
// to sign in again.
func (h *handlers) requireRecentSignIn(w http.ResponseWriter, r *http.Request, a *accounts.Authenticated) bool {
	if h.local {
		return true // Nobody signs in here: the computer is the door.
	}
	recent := time.Since(a.Session.CreatedAt) <= recentSignIn
	if recent && a.Session.Method != accounts.MethodPasskey {
		keys, err := h.acc.Passkeys(r.Context(), a.User.ID)
		if err != nil {
			h.fail(w, r, err)
			return false
		}
		if len(keys) > 0 {
			writeError(w, http.StatusForbidden, "sign_in_again",
				"For your account's safety, this needs a sign-in with one of your passkeys from the last 10 minutes. Sign in again with a passkey, then try once more.")
			return false
		}
	}
	if !recent {
		writeError(w, http.StatusForbidden, "sign_in_again",
			"For your account's safety, this needs a sign-in from the last 10 minutes. Sign in again, then try once more.")
	}
	return recent
}

func (h *handlers) BeginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil || !h.requireRecentSignIn(w, r, a) {
		return
	}
	c, err := h.acc.BeginPasskeyRegistration(r.Context(), a.User)
	h.writeCeremony(w, r, c, err)
}

func (h *handlers) FinishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.FinishPasskeyRegistrationJSONBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	cred, err := json.Marshal(body.Credential)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	key, err := h.acc.FinishPasskeyRegistration(r.Context(), a.User, body.CeremonyId, cred, name, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, passkeyJSON(key))
}

func (h *handlers) RenamePasskey(w http.ResponseWriter, r *http.Request, passkeyID api.PasskeyId) {
	a := h.requireUser(w, r)
	var body api.RenamePasskeyJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.acc.RenamePasskey(r.Context(), a.User.ID, passkeyID, body.Name); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) DeletePasskey(w http.ResponseWriter, r *http.Request, passkeyID api.PasskeyId) {
	// Without a passkey, a sign-in by email link would do for all that a
	// passkey guards, so taking one away needs what adding one does.
	a := h.requireUser(w, r)
	if a == nil || !h.requireRecentSignIn(w, r, a) {
		return
	}
	if err := h.acc.DeletePasskey(r.Context(), a.User.ID, passkeyID, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// Workspaces

func workspaceJSON(ws accounts.WorkspaceSummary) api.Workspace {
	return api.Workspace{Id: ws.ID, Name: ws.Name, Slug: ws.Slug, IsPersonal: ws.IsPersonal, Role: api.Role(ws.Role)}
}

func (h *handlers) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	list, err := h.acc.Workspaces(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// A token for one workspace knows of that one.
	if a.Token != nil && a.Token.WorkspaceID.Valid {
		list = slices.DeleteFunc(list, func(ws accounts.WorkspaceSummary) bool { return ws.ID != a.Token.WorkspaceID.String })
	}
	out := make([]api.Workspace, len(list))
	for i, ws := range list {
		out[i] = workspaceJSON(ws)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.CreateWorkspaceJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	ws, err := h.acc.CreateWorkspace(r.Context(), a.User.ID, body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspaceJSON(ws))
}

func (h *handlers) RenameWorkspace(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.RenameWorkspaceJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.acc.RenameWorkspace(r.Context(), a.User.ID, workspaceID, body.Name); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) DeleteWorkspace(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.DeleteWorkspace(r.Context(), a.User.ID, workspaceID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) ListMembers(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	members, err := h.acc.Members(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Member, len(members))
	for i, m := range members {
		out[i] = api.Member{UserId: m.ID, Email: m.Email, Name: m.Name, Avatar: avatarPath(m.Avatar), Role: api.Role(m.Role), JoinedAt: m.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ChangeMemberRole(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, userID api.UserId) {
	a := h.requireUser(w, r)
	var body api.ChangeMemberRoleJSONBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if err := h.acc.ChangeRole(r.Context(), a.User.ID, workspaceID, userID, authz.Role(body.Role)); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) RemoveMember(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, userID api.UserId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.RemoveMember(r.Context(), a.User.ID, workspaceID, userID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func inviteJSON(i store.Invite) api.Invite {
	return api.Invite{Id: i.ID, Email: i.Email, Role: api.Role(i.Role), CreatedAt: i.CreatedAt, ExpiresAt: i.ExpiresAt}
}

func (h *handlers) ListInvites(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	invites, err := h.acc.PendingInvites(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.Invite, len(invites))
	for i, inv := range invites {
		out[i] = inviteJSON(inv)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateInvite(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.CreateInviteJSONBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if !h.inviteLimit.Allow(a.User.ID) {
		w.Header().Set("Retry-After", "1800")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many invitations sent. Try again later.")
		return
	}
	inv, err := h.acc.Invite(r.Context(), a.User, workspaceID, body.Email, authz.Role(body.Role))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, inviteJSON(inv))
}

func (h *handlers) RevokeInvite(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, inviteID api.InviteId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.RevokeInvite(r.Context(), a.User.ID, workspaceID, inviteID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// Invites, from the link

func (h *handlers) PreviewInvite(w http.ResponseWriter, r *http.Request) {
	var body api.PreviewInviteJSONRequestBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	p, err := h.acc.PreviewInvite(r.Context(), body.Token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.InvitePreview{
		WorkspaceId: p.WorkspaceID, WorkspaceName: p.WorkspaceName, InvitedByName: p.InvitedByName, Email: p.Email, Role: api.Role(p.Role),
	})
}

type acceptResult struct {
	WorkspaceID string `json:"workspaceId"`
	SignedIn    bool   `json:"signedIn"`
}

func (h *handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var body api.AcceptInviteJSONBody
	if h.limited(w, r) || !h.decode(w, r, &body) {
		return
	}
	var current *store.User
	if a := authOf(r); a != nil {
		current = &a.User
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	in, workspaceID, err := h.acc.AcceptInvite(r.Context(), body.Token, current, name, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if in.SessionToken != "" {
		h.setSessionCookie(w, in.SessionToken)
		h.setDeviceCookie(w, in.DeviceToken)
	}
	writeJSON(w, http.StatusOK, acceptResult{WorkspaceID: workspaceID, SignedIn: current != nil || in.SessionToken != ""})
}

// userAgent is the request's User-Agent as text worth storing: Go lets a
// header hold bytes that aren't UTF-8, which Postgres refuses.
func userAgent(r *http.Request) string {
	return strings.ToValidUTF8(r.UserAgent(), "\uFFFD")
}
