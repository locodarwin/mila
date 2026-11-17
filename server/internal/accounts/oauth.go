package accounts

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// OAuth for apps that connect to the MCP endpoint: RFC 6749's authorization
// code grant, with PKCE (RFC 7636, S256 only) and apps registering
// themselves (RFC 7591), all of them public clients with no secret. A
// person says yes in the app, signed in, which makes a code good once for
// ten minutes; the app trades it, with its PKCE verifier, for an ordinary
// API token marked as the app's, which httpapi keeps to the MCP endpoint.
// There are no refresh tokens: when the token ends, the person says yes
// again.

const (
	// oauthCodeLife is how long a code is good for.
	oauthCodeLife = 10 * time.Minute
	// oauthClientsMax is the most apps one server keeps registered.
	oauthClientsMax = 1000
	// oauthIdle is how long an app may stay registered with nobody saying
	// yes to it.
	oauthIdle = 24 * time.Hour
	// oauthIdleWhenFull is the same while the registry is full: a person
	// says yes within minutes of their app registering.
	oauthIdleWhenFull = time.Hour
	// oauthUnused is how long an app said yes to, whose tokens have all
	// ended, stays registered.
	oauthUnused  = 30 * 24 * time.Hour
	maxAppName   = 80
	maxRedirects = 10
	maxRedirect  = 2000
)

// OAuthLifetimes are how long, in days, a person may let an app in for.
var OAuthLifetimes = []int{30, 90, 365}

// OAuthClient is an app that registered.
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
}

// OAuthGrant is what a person said yes to.
type OAuthGrant struct {
	ClientID    string
	RedirectURI string
	// Challenge is the PKCE code challenge, by S256.
	Challenge     string
	WorkspaceID   string
	CanWrite      bool
	ExpiresInDays int
}

// OAuthError is a refusal in OAuth's terms (RFC 6749 section 5.2): the
// code an app reads, and words for whoever reads its log.
type OAuthError struct {
	Code, Description string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

// privateSchemeRE is a scheme of an app's own as RFC 8252 section 7.1 has
// it: a domain the app's maker holds, reversed (com.example.app), so at
// least two labels. A bare name could be any handler the computer has.
var privateSchemeRE = regexp.MustCompile(`^[a-z](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

// knownClientSchemes are the bare schemes allowed besides reversed
// domains: each is a known MCP client's own, which it registers for its
// OAuth callback. Any other bare scheme stays refused.
var knownClientSchemes = []string{
	"cursor",          // Cursor: cursor://anysphere.cursor-mcp/oauth/callback
	"windsurf",        // Windsurf: its OAuth callback for MCP servers
	"vscode",          // Visual Studio Code: its OAuth callback for MCP servers
	"vscode-insiders", // Visual Studio Code Insiders: the same, for its own build
	"zed",             // Zed: its OAuth callback for MCP servers
}

// ownScheme says whether scheme may be an app's own: a reversed domain,
// or one of the known clients' bare schemes.
func ownScheme(scheme string) bool {
	return privateSchemeRE.MatchString(scheme) || slices.Contains(knownClientSchemes, scheme)
}

// Schemes an app may never be sent back to, even in a private scheme's
// shape: they run something, read something, or reach somewhere other
// than the app.
var badSchemes = []string{"javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss", "mailto", "tel", "sms", "intent", "chrome", "view-source",
	// The system's own handlers, several of which have run code from a
	// link (Follina was ms-msdt:): a page of Mila's must never open them,
	// even after the browser asks.
	"search", "search-ms", "shell", "res", "its", "mk", "mhtml", "jar", "smb", "nfs", "afp", "ssh", "telnet", "rlogin", "ldap", "ldaps", "gopher", "news", "nntp", "hcp", "help", "keyboard", "jnlp", "jnlps", "calculator", "msteams", "zoommtg", "steam"}

// badSchemePrefixes are families of the same.
var badSchemePrefixes = []string{"ms-", "vnd.ms-", "microsoft-", "x-apple", "x-help", "windows"}

func badScheme(scheme string) bool {
	if slices.Contains(badSchemes, scheme) {
		return true
	}
	for _, p := range badSchemePrefixes {
		if strings.HasPrefix(scheme, p) {
			return true
		}
	}
	return false
}

// checkRedirect says what is wrong with an address an app wants to be sent
// back to, or "": https, http to this computer (RFC 8252 section 7.3), or a
// scheme of the app's own, a reversed domain (section 7.1); never one with
// a fragment.
func checkRedirect(raw string) string {
	if len(raw) > maxRedirect {
		return "A redirect URI is 2000 characters at most."
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil || !u.IsAbs():
		return "A redirect URI must be a whole address."
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return "A redirect URI cannot have a fragment."
	case u.User != nil:
		return "A redirect URI cannot hold a user name or password."
	case u.Scheme == "https":
		if u.Hostname() == "" {
			return "An https redirect URI needs a host."
		}
	case u.Scheme == "http":
		if !loopback(u.Hostname()) {
			return "An http redirect URI must be to this computer (localhost, 127.0.0.1 or [::1]); others use https."
		}
	case !ownScheme(u.Scheme) || badScheme(u.Scheme):
		return "That redirect URI's scheme isn't allowed: use https, http to this computer, or a scheme of the app's own written as a domain reversed, such as com.example.app (RFC 8252)."
	}
	return ""
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectMatches says whether an address is one the app registered:
// exactly, except that an address on this computer may use any port, since
// an app on it is given one when it starts (RFC 8252 section 7.3).
func (c OAuthClient) redirectMatches(raw string) bool {
	if slices.Contains(c.RedirectURIs, raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || !loopback(u.Hostname()) {
		return false
	}
	for _, r := range c.RedirectURIs {
		ru, err := url.Parse(r)
		if err == nil && ru.Scheme == "http" && ru.Hostname() == u.Hostname() && ru.EscapedPath() == u.EscapedPath() && ru.RawQuery == u.RawQuery {
			return true
		}
	}
	return false
}

// RegisterOAuthClient registers an app (RFC 7591). Anyone may: an app can
// do nothing until a person says yes to it, by name and address.
func (s *Service) RegisterOAuthClient(ctx context.Context, name string, redirects []string) (OAuthClient, error) {
	// Only what prints, so no character turns the name around or hides
	// part of it on the page that asks the person.
	name = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = "An app"
	}
	if r := []rune(name); len(r) > maxAppName {
		name = strings.TrimSpace(string(r[:maxAppName]))
	}
	if reservedTokenName(name) {
		return OAuthClient{}, &OAuthError{"invalid_client_metadata", "Mila names changes made by email or by an automation so; give the app a name of its own."}
	}
	if len(redirects) == 0 || len(redirects) > maxRedirects {
		return OAuthClient{}, &OAuthError{"invalid_redirect_uri", "Give 1 to 10 redirect URIs."}
	}
	for _, r := range redirects {
		if why := checkRedirect(r); why != "" {
			return OAuthClient{}, &OAuthError{"invalid_redirect_uri", why}
		}
	}
	raw, err := json.Marshal(redirects)
	if err != nil {
		return OAuthClient{}, err
	}
	c := OAuthClient{ID: "app_" + strings.ReplaceAll(newID(), "-", ""), Name: name, RedirectURIs: redirects}
	now := store.Now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.DeleteIdleOAuthClients(ctx, now.Add(-oauthIdle)); err != nil {
			return err
		}
		if err := q.DeleteUnusedOAuthClients(ctx, sqlitedb.DeleteUnusedOAuthClientsParams{Before: sql.NullTime{Time: now.Add(-oauthUnused), Valid: true}, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		n, err := q.CountOAuthClients(ctx)
		if err != nil {
			return err
		}
		if n >= oauthClientsMax {
			// Full: make room from the apps nobody said yes to within the
			// hour, which an app in use is long past, so that filling the
			// registry once blocks nobody for a day.
			if err := q.DeleteIdleOAuthClients(ctx, now.Add(-oauthIdleWhenFull)); err != nil {
				return err
			}
			if n, err = q.CountOAuthClients(ctx); err != nil {
				return err
			}
		}
		if n >= oauthClientsMax {
			return &OAuthError{"temporarily_unavailable", "Too many apps are registered here. Try again tomorrow."}
		}
		return q.InsertOAuthClient(ctx, sqlitedb.InsertOAuthClientParams{ID: c.ID, Name: name, RedirectUris: string(raw), Now: now})
	})
	return c, err
}

// OAuthClientOf reads an app's registration; NotFound when there is none.
func (s *Service) OAuthClientOf(ctx context.Context, id string) (OAuthClient, error) {
	row, err := s.st.GetOAuthClient(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthClient{}, fail(NotFound, "No app is registered with that client_id. Connect it again from the app.")
	}
	if err != nil {
		return OAuthClient{}, err
	}
	c := OAuthClient{ID: row.ID, Name: row.Name}
	if err := json.Unmarshal([]byte(row.RedirectUris), &c.RedirectURIs); err != nil {
		return OAuthClient{}, err
	}
	return c, nil
}

// OAuthRequest checks the app and the address it asked to be sent back to,
// before anything is shown. A wrong address is never redirected to.
func (s *Service) OAuthRequest(ctx context.Context, clientID, redirectURI string) (OAuthClient, error) {
	c, err := s.OAuthClientOf(ctx, clientID)
	if err != nil {
		return OAuthClient{}, err
	}
	if !c.redirectMatches(redirectURI) {
		return OAuthClient{}, fail(Invalid, "That app didn't register the address it asked to be sent back to, so Mila won't send you there. Connect it again from the app.")
	}
	return c, nil
}

var challengeRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// GrantOAuth is a person saying yes: a code for the app, good once for ten
// minutes, which it trades for a token with ExchangeOAuthCode.
func (s *Service) GrantOAuth(ctx context.Context, userID string, g OAuthGrant) (string, error) {
	c, err := s.OAuthRequest(ctx, g.ClientID, g.RedirectURI)
	if err != nil {
		return "", err
	}
	if !challengeRE.MatchString(g.Challenge) {
		return "", fail(Invalid, "The app didn't send a PKCE code challenge made with S256, so Mila can't let it in.")
	}
	if !slices.Contains(OAuthLifetimes, g.ExpiresInDays) {
		return "", fail(Invalid, "An app can be let in for 30 days, 90 days or a year.")
	}
	random, _ := newToken()
	now := store.Now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if _, _, err := s.access(ctx, q, userID, g.WorkspaceID, authz.ViewWorkspace); err != nil {
			return err
		}
		// Room for the app's token now, so a full list is said here and not
		// as a failure in the app.
		n, err := q.CountApiTokens(ctx, userID)
		if err != nil {
			return err
		}
		mine, err := q.ListApiTokens(ctx, userID)
		if err != nil {
			return err
		}
		replaced := 0
		for _, t := range mine {
			if t.OauthClientID.String == c.ID {
				replaced++
			}
		}
		if n-int64(replaced) >= maxTokens {
			return fail(Invalid, "You have 20 API tokens, the most there can be. Revoke one in Settings, API tokens, then connect the app again.")
		}
		if err := q.DeleteOldOAuthCodes(ctx, now.Add(-oauthIdle)); err != nil {
			return err
		}
		if err := q.TouchOAuthClient(ctx, sqlitedb.TouchOAuthClientParams{ID: c.ID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		return q.InsertOAuthCode(ctx, sqlitedb.InsertOAuthCodeParams{
			CodeHash: hashToken(random), ClientID: c.ID, UserID: userID, WorkspaceID: g.WorkspaceID, CanWrite: g.CanWrite,
			ExpiresInDays: int64(g.ExpiresInDays), RedirectUri: g.RedirectURI, Challenge: g.Challenge, Now: now, ExpiresAt: now.Add(oauthCodeLife),
		})
	})
	if err != nil {
		return "", err
	}
	return random, nil
}

var verifierRE = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// ExchangeOAuthCode trades a code for the app's token (RFC 6749 section
// 4.1.3, RFC 7636 section 4.6). A code is good once: one that comes back
// revokes the token it was traded for, since someone else has it (section
// 10.5).
func (s *Service) ExchangeOAuthCode(ctx context.Context, clientID, code, redirectURI, verifier string, c Client) (sqlitedb.ApiToken, string, error) {
	refused := &OAuthError{"invalid_grant", "That code is not valid, was used already, or has expired."}
	if code == "" || !verifierRE.MatchString(verifier) {
		return sqlitedb.ApiToken{}, "", &OAuthError{"invalid_request", "Send the code and its PKCE code_verifier."}
	}
	hash := hashToken(code)
	now := store.Now()
	var t sqlitedb.ApiToken
	var secret string
	var user store.User
	reused := false
	err := s.st.InTx(ctx, func(q store.Queries) error {
		row, err := q.GetOAuthCode(ctx, hash)
		if errors.Is(err, sql.ErrNoRows) {
			return refused
		}
		if err != nil {
			return err
		}
		n, err := q.UseOAuthCode(ctx, sqlitedb.UseOAuthCodeParams{CodeHash: hash, Now: sql.NullTime{Time: now, Valid: true}})
		if err != nil {
			return err
		}
		if n == 0 {
			// Revoked here, and the transaction kept, before refusing. The
			// row is read again: on Postgres, a code traded at the same
			// moment sets its token after the read above, and this update
			// waited for it.
			reused = true
			if again, err := q.GetOAuthCode(ctx, hash); err == nil {
				row = again
			}
			if row.TokenID.Valid {
				return q.RevokeApiTokenByID(ctx, sqlitedb.RevokeApiTokenByIDParams{ID: row.TokenID.String, Now: sql.NullTime{Time: now, Valid: true}})
			}
			return nil
		}
		sum := sha256.Sum256([]byte(verifier))
		switch {
		case !row.ExpiresAt.After(now), row.ClientID != clientID, row.RedirectUri != redirectURI,
			base64.RawURLEncoding.EncodeToString(sum[:]) != row.Challenge:
			// Used up all the same, so a code can't be guessed at.
			return nil
		}
		app, err := q.GetOAuthClient(ctx, clientID)
		if err != nil {
			return err
		}
		t, secret, user, err = s.makeToken(ctx, q, row.UserID, TokenInput{
			Name: app.Name, CanWrite: row.CanWrite, WorkspaceID: row.WorkspaceID, ExpiresInDays: int(row.ExpiresInDays), OAuthClientID: app.ID,
		}, c)
		if err != nil {
			return err
		}
		return q.SetOAuthCodeToken(ctx, sqlitedb.SetOAuthCodeTokenParams{CodeHash: hash, TokenID: sql.NullString{String: t.ID, Valid: true}})
	})
	var oe *OAuthError
	switch {
	case errors.As(err, &oe):
		return sqlitedb.ApiToken{}, "", oe
	case err != nil && (KindOf(err) == Invalid || KindOf(err) == Forbidden || KindOf(err) == NotFound):
		// The person can no longer reach the workspace, or has no room.
		return sqlitedb.ApiToken{}, "", &OAuthError{"invalid_grant", err.Error()}
	case err != nil:
		return sqlitedb.ApiToken{}, "", err
	case reused:
		s.log.Warn("an OAuth code was used a second time; its token is revoked", "client", clientID)
		return sqlitedb.ApiToken{}, "", refused
	case secret == "":
		return sqlitedb.ApiToken{}, "", refused
	}
	s.tokenMail(ctx, user, t)
	return t, secret, nil
}

// OAuthApp is a registered app as the server's admin sees it.
type OAuthApp struct {
	ID         string
	Name       string
	Registered time.Time
	// LastYes is when a person last said yes to it; zero if nobody has.
	LastYes time.Time
	// Tokens is how many of its tokens still work.
	Tokens int
}

// OAuthApps lists the apps registered, oldest first, for `mila oauth list`.
func (s *Service) OAuthApps(ctx context.Context) ([]OAuthApp, error) {
	rows, err := s.st.ListOAuthClients(ctx, sql.NullTime{Time: store.Now(), Valid: true})
	if err != nil {
		return nil, err
	}
	apps := make([]OAuthApp, len(rows))
	for i, r := range rows {
		apps[i] = OAuthApp{ID: r.ID, Name: r.Name, Registered: r.CreatedAt, LastYes: r.UsedAt.Time, Tokens: int(r.LiveTokens)}
	}
	return apps, nil
}

// DeleteOAuthClient removes an app's registration for `mila oauth delete`,
// with its codes and tokens. Each token that still worked goes in its
// owner's security log as revoked from the server. It returns the app and
// the people whose tokens ended; NotFound when there is no such app.
func (s *Service) DeleteOAuthClient(ctx context.Context, id string) (OAuthApp, []store.User, error) {
	var app OAuthApp
	var people []store.User
	err := s.st.InTx(ctx, func(q store.Queries) error {
		row, err := q.GetOAuthClient(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "No app is registered with that ID; \"mila oauth list\" shows them.")
		}
		if err != nil {
			return err
		}
		now := store.Now()
		tokens, err := q.ListOAuthClientTokens(ctx, sqlitedb.ListOAuthClientTokensParams{ClientID: sql.NullString{String: id, Valid: true}, Now: sql.NullTime{Time: now, Valid: true}})
		if err != nil {
			return err
		}
		app = OAuthApp{ID: row.ID, Name: row.Name, Registered: row.CreatedAt, LastYes: row.UsedAt.Time, Tokens: len(tokens)}
		seen := map[string]bool{}
		for _, t := range tokens {
			if _, err := record(ctx, q, t.UserID, EventTokenRevoked, MethodCommand, t.Name, Client{}, false, now); err != nil {
				return err
			}
			if seen[t.UserID] {
				continue
			}
			seen[t.UserID] = true
			u, err := q.GetUser(ctx, t.UserID)
			if err != nil {
				return err
			}
			people = append(people, u)
		}
		_, err = q.DeleteOAuthClient(ctx, id)
		return err
	})
	if err != nil {
		return OAuthApp{}, nil, err
	}
	return app, people, nil
}
