package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/idna"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
)

// OAuth for apps that connect to the MCP endpoint (docs/PLAN.md section 11,
// Agent access, phase 4), when the server lets them (MILA_OAUTH): the two
// metadata documents apps look for first (RFC 9728 for the endpoint, RFC
// 8414 for the server), registration (RFC 7591), the token endpoint, and the
// app's page at /oauth/authorize, where the person says yes or no through
// the consent endpoints. accounts/oauth.go holds the rules.

const (
	prmPath      = "/.well-known/oauth-protected-resource"
	asmPath      = "/.well-known/oauth-authorization-server"
	registerPath = "/oauth/register"
	tokenPath    = "/oauth/token"
	// authorizePath is the app's page.
	authorizePath = "/oauth/authorize"
)

// oauthRoutes adds OAuth's endpoints. They answer 404 while it is off.
func (h *handlers) oauthRoutes(mux *http.ServeMux) {
	on := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !h.oauth {
				http.NotFound(w, r)
				return
			}
			// Apps in a browser may ask too; none of this uses a cookie.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			f(w, r)
		}
	}
	mux.HandleFunc(prmPath, on(h.resourceMetadata))
	mux.HandleFunc(prmPath+mcpPath, on(h.resourceMetadata))
	mux.HandleFunc(asmPath, on(h.serverMetadata))
	mux.HandleFunc(registerPath, on(h.registerApp))
	mux.HandleFunc(tokenPath, on(h.oauthToken))
}

// mcpResource is the MCP endpoint's address, as OAuth names it.
func (h *handlers) mcpResource() string { return h.origin + mcpPath }

// ourResource says whether an app's resource (RFC 8707) is this server's
// MCP endpoint, with or without a query naming the workspace, and gives
// that query's workspace. An app may name the server itself instead.
func (h *handlers) ourResource(raw string) (workspace string, ok bool) {
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Scheme+"://"+u.Host != h.origin {
		return "", false
	}
	switch strings.TrimSuffix(u.Path, "/") {
	case mcpPath, "":
		return u.Query().Get("workspace"), true
	}
	return "", false
}

func (h *handlers) resourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// The resource is the address the app asked about, query and all, which
	// it checks against the one it connects to (RFC 9728 section 3.3).
	resource := h.origin + strings.TrimPrefix(r.URL.Path, prmPath)
	if r.URL.RawQuery != "" {
		resource += "?" + r.URL.RawQuery
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 resource,
		"authorization_servers":    []string{h.origin},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Mila",
	})
}

func (h *handlers) serverMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         h.origin,
		"authorization_endpoint":                         h.origin + authorizePath,
		"token_endpoint":                                 h.origin + tokenPath,
		"registration_endpoint":                          h.origin + registerPath,
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"authorization_response_iss_parameter_supported": true,
		"client_id_metadata_document_supported":          false,
	})
}

// oauthFail answers in OAuth's terms (RFC 6749 section 5.2).
func oauthFail(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (h *handlers) registerApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.oauthRegister.Allow(wideRateKey(r)) {
		w.Header().Set("Retry-After", "3600")
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Too many apps registered from here. Try again later.")
		return
	}
	var in struct {
		Name          string   `json:"client_name"`
		RedirectURIs  []string `json:"redirect_uris"`
		GrantTypes    []string `json:"grant_types"`
		ResponseTypes []string `json:"response_types"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err == nil && escapesNUL(body) {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", nulMessage)
		return
	}
	if err != nil || json.Unmarshal(body, &in) != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "The body is not JSON client metadata.")
		return
	}
	// What an app asks to do must include what Mila offers; the rest of what
	// it asks (a secret, say) it is told it doesn't get, in the answer.
	if len(in.GrantTypes) > 0 && !slices.Contains(in.GrantTypes, "authorization_code") || len(in.ResponseTypes) > 0 && !slices.Contains(in.ResponseTypes, "code") {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "Mila offers the authorization_code grant, with response type code, only.")
		return
	}
	c, err := h.acc.RegisterOAuthClient(r.Context(), in.Name, in.RedirectURIs)
	var oe *accounts.OAuthError
	if errors.As(err, &oe) {
		oauthFail(w, http.StatusBadRequest, oe.Code, oe.Description)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  c.ID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.RedirectURIs,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

func (h *handlers) oauthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.oauthTokens.Allow(rateKey(r)) {
		w.Header().Set("Retry-After", "60")
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Too many tries. Wait a minute.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_request", "The body is not a form.")
		return
	}
	f := r.PostForm
	for _, vs := range f {
		for _, v := range vs {
			if strings.ContainsRune(v, 0) {
				oauthFail(w, http.StatusBadRequest, "invalid_request", nulMessage)
				return
			}
		}
	}
	clientID := f.Get("client_id")
	// An app that sends its client_id as client_secret_basic would: Mila's
	// apps have no secret, so the ID is all that counts.
	if id, _, ok := r.BasicAuth(); ok && clientID == "" {
		clientID, _ = url.QueryUnescape(id)
		if strings.ContainsRune(clientID, 0) {
			oauthFail(w, http.StatusBadRequest, "invalid_request", nulMessage)
			return
		}
	}
	switch {
	case f.Get("grant_type") != "authorization_code":
		oauthFail(w, http.StatusBadRequest, "unsupported_grant_type", "Mila offers the authorization_code grant only, and no refresh tokens: connect the app again when its token ends.")
		return
	case clientID == "":
		oauthFail(w, http.StatusUnauthorized, "invalid_client", "Send the client_id.")
		return
	}
	if _, ok := h.ourResource(f.Get("resource")); !ok {
		oauthFail(w, http.StatusBadRequest, "invalid_target", "This server's resource is "+h.mcpResource()+".")
		return
	}
	if _, err := h.acc.OAuthClientOf(r.Context(), clientID); err != nil {
		if accounts.KindOf(err) == accounts.NotFound {
			oauthFail(w, http.StatusUnauthorized, "invalid_client", "No app is registered with that client_id.")
			return
		}
		h.fail(w, r, err)
		return
	}
	t, secret, err := h.acc.ExchangeOAuthCode(r.Context(), clientID, f.Get("code"), f.Get("redirect_uri"), f.Get("code_verifier"), clientOf(r))
	var oe *accounts.OAuthError
	if errors.As(err, &oe) {
		oauthFail(w, http.StatusBadRequest, oe.Code, oe.Description)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	scope := "read"
	if t.CanWrite {
		scope = "read write"
	}
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": secret,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(t.ExpiresAt.Time).Seconds()),
		"scope":        scope,
	})
}

// returnsTo says where an address sends the person, for the page.
func returnsTo(raw string) string {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return raw
	case u.Scheme == "http":
		return "an app on this computer"
	case u.Scheme == "https":
		// As ASCII, so a name in another script can't pass for a known one.
		if host, err := idna.Lookup.ToASCII(u.Hostname()); err == nil {
			return host
		}
		return u.Hostname()
	}
	return "the app " + u.Scheme + ":"
}

func (h *handlers) OauthConsentRequest(w http.ResponseWriter, r *http.Request, params api.OauthConsentRequestParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if !h.oauth {
		writeError(w, http.StatusNotFound, "not_found", "This server doesn't let apps connect by OAuth.")
		return
	}
	workspace, ok := h.ourResource(deref(params.Resource))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid", "The app asked to connect to somewhere other than this server's MCP endpoint, "+h.mcpResource()+".")
		return
	}
	c, err := h.acc.OAuthRequest(r.Context(), params.ClientId, params.RedirectUri)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.OAuthRequest{App: c.Name, ReturnsTo: returnsTo(params.RedirectUri)}
	if workspace != "" {
		out.Workspace = &workspace
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) OauthConsent(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	var body api.OauthConsentJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	if !h.oauth {
		writeError(w, http.StatusNotFound, "not_found", "This server doesn't let apps connect by OAuth.")
		return
	}
	if _, ok := h.ourResource(deref(body.Resource)); !ok {
		writeError(w, http.StatusBadRequest, "invalid", "The app asked to connect to somewhere other than this server's MCP endpoint.")
		return
	}
	if _, err := h.acc.OAuthRequest(r.Context(), body.ClientId, body.RedirectUri); err != nil {
		h.fail(w, r, err)
		return
	}
	back, _ := url.Parse(body.RedirectUri)
	q := back.Query()
	if s := deref(body.State); s != "" {
		q.Set("state", s)
	}
	// Which server answered, so an app talking to several can tell (RFC 9207).
	q.Set("iss", h.origin)
	if !body.Allow {
		q.Set("error", "access_denied")
		q.Set("error_description", "The person said no.")
		back.RawQuery = q.Encode()
		writeJSON(w, http.StatusOK, map[string]string{"redirect": back.String()})
		return
	}
	if !h.requireRecentSignIn(w, r, a) {
		return
	}
	if body.CodeChallengeMethod != "S256" {
		writeError(w, http.StatusBadRequest, "invalid", "The app didn't use PKCE with S256, so Mila can't let it in. It may be too old for Mila.")
		return
	}
	g := accounts.OAuthGrant{ClientID: body.ClientId, RedirectURI: body.RedirectUri, Challenge: body.CodeChallenge,
		WorkspaceID: deref(body.WorkspaceId), CanWrite: body.CanWrite != nil && *body.CanWrite, ExpiresInDays: 90}
	if body.ExpiresInDays != nil {
		g.ExpiresInDays = int(*body.ExpiresInDays)
	}
	if g.WorkspaceID == "" {
		writeError(w, http.StatusBadRequest, "invalid", "Choose the workspace the app may reach.")
		return
	}
	code, err := h.acc.GrantOAuth(r.Context(), a.User.ID, g)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q.Set("code", code)
	back.RawQuery = q.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"redirect": back.String()})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
