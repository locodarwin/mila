// Package msauth signs Mila in to a Microsoft mailbox (Outlook.com, Microsoft
// 365) for email to task, which these take no password for over IMAP. It is
// OAuth 2.0's device code flow against Microsoft's identity platform, with an
// app the server's admin registered in Microsoft Entra: a public client, so
// there is no secret to keep. Mila asks to read the mailbox over IMAP, to stay
// signed in, and for the account's name to show who signed in; nothing else.
package msauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Scope is everything Mila asks for: IMAP to the mailbox, staying signed in,
// and the account's name.
const Scope = "https://outlook.office.com/IMAP.AccessAsUser.All offline_access openid profile"

var (
	// ErrLost means Microsoft no longer takes the sign-in: the password
	// changed, an admin revoked it, or it went unused too long. Someone must
	// sign in again.
	ErrLost = errors.New("the sign-in is no longer accepted by Microsoft")
	// ErrDeclined means the person said no at Microsoft.
	ErrDeclined = errors.New("the sign-in was declined at Microsoft")
	// ErrExpired means nobody entered the code in time.
	ErrExpired = errors.New("the code ran out before anyone signed in")
)

// Config is the app Mila signs in through.
type Config struct {
	ClientID string
	// Tenant is a directory ID, a domain, consumers or common.
	Tenant string
	// Authority is https://login.microsoftonline.com, or a stand-in in tests.
	Authority string
	HTTP      *http.Client
}

func (c Config) endpoint(name string) string {
	return strings.TrimSuffix(c.Authority, "/") + "/" + url.PathEscape(c.Tenant) + "/oauth2/v2.0/" + name
}

func (c Config) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	// No redirects: on a 307 Go posts the form again to wherever it
	// points, refresh token and all.
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// What the identity platform says is held to sense, since it could answer
// with anything: a pace and lifetimes in bounds, and text that is only text.
const (
	maxInterval  = 60 * time.Second
	maxCodeLife  = time.Hour
	maxTokenLife = 24 * time.Hour
	maxTextRunes = 500
)

// seconds is n seconds, at least least and at most most, n being whatever
// came back (an overflowing number among it).
func seconds(n int, least, most time.Duration) time.Duration {
	if n < int(least/time.Second) {
		return least
	}
	if n > int(most/time.Second) {
		return most
	}
	return time.Duration(n) * time.Second
}

// plain is text from the identity platform made safe to store, show and
// print to a terminal: control and format characters become spaces, and it
// is cut to a sentence's length.
func plain(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, "\uFFFD"))
	if r := []rune(s); len(r) > maxTextRunes {
		s = string(r[:maxTextRunes])
	}
	return strings.TrimSpace(s)
}

// Code is what the person needs to sign Mila in: a code to enter at an address.
type Code struct {
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
	deviceCode      string
	interval        time.Duration
}

// Token is a sign-in: an access token for IMAP and the refresh token that
// gets the next one.
type Token struct {
	Access    string
	Refresh   string
	ExpiresAt time.Time
	// Account is who signed in, as Microsoft names them (an email address).
	Account string
}

// answer is the identity platform's JSON, success or error.
type answer struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	IDToken         string `json:"id_token"`
	Error           string `json:"error"`
	Description     string `json:"error_description"`
}

func (c Config) post(ctx context.Context, name string, form url.Values) (answer, error) {
	form.Set("client_id", c.ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(name), strings.NewReader(form.Encode()))
	if err != nil {
		return answer{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return answer{}, fmt.Errorf("could not reach Microsoft's sign-in: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var a answer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&a); err != nil {
		return answer{}, fmt.Errorf("the sign-in answered %d with no JSON", resp.StatusCode)
	}
	return a, nil
}

// failure says what an error answer means, in a sentence for the admin.
func failure(a answer) error {
	// The first line of Microsoft's description carries its own code (AADSTS...),
	// which is what an admin searches for.
	desc := firstLine(a.Description)
	if desc == "" {
		desc = plain(a.Error)
	}
	return fmt.Errorf("refused at Microsoft: %s", desc)
}

// Start asks for a code for someone to enter at Microsoft.
func (c Config) Start(ctx context.Context) (Code, error) {
	a, err := c.post(ctx, "devicecode", url.Values{"scope": {Scope}})
	if err != nil {
		return Code{}, err
	}
	if a.Error != "" || a.DeviceCode == "" || a.UserCode == "" {
		return Code{}, failure(a)
	}
	// The address must be Microsoft's page over HTTPS, or the usual one.
	uri := plain(a.VerificationURI)
	if u, err := url.Parse(uri); err != nil || u.Scheme != "https" || u.Host == "" {
		uri = "https://microsoft.com/devicelogin"
	}
	return Code{
		UserCode: plain(a.UserCode), VerificationURI: uri, deviceCode: a.DeviceCode,
		interval:  seconds(a.Interval, time.Second, maxInterval),
		ExpiresAt: time.Now().Add(seconds(a.ExpiresIn, time.Second, maxCodeLife)),
	}, nil
}

// Wait asks Microsoft, at the pace it set, whether the code has been
// entered, until it has, the person said no, the code ran out or ctx ends.
func (c Config) Wait(ctx context.Context, code Code) (Token, error) {
	interval := code.interval
	for {
		select {
		case <-ctx.Done():
			return Token{}, ctx.Err()
		case <-time.After(interval):
		}
		// The code's own lifetime ends the wait, whatever Microsoft says.
		if !code.ExpiresAt.IsZero() && time.Now().After(code.ExpiresAt) {
			return Token{}, ErrExpired
		}
		a, err := c.post(ctx, "token", url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {code.deviceCode},
		})
		if err != nil {
			if ctx.Err() != nil {
				return Token{}, ctx.Err()
			}
			continue // Microsoft unreachable for a moment: ask again
		}
		switch a.Error {
		case "":
			return tokenOf(a, "")
		case "authorization_pending":
			continue
		case "slow_down":
			interval = min(interval+5*time.Second, maxInterval)
			continue
		case "authorization_declined", "access_denied":
			return Token{}, ErrDeclined
		case "expired_token", "code_expired":
			return Token{}, ErrExpired
		default:
			return Token{}, failure(a)
		}
	}
}

// Refresh trades a refresh token for a new sign-in. Microsoft may hand back
// a new refresh token too, which replaces the old.
func (c Config) Refresh(ctx context.Context, refresh, account string) (Token, error) {
	a, err := c.post(ctx, "token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"scope":         {Scope},
	})
	if err != nil {
		return Token{}, err
	}
	if a.Error == "invalid_grant" || a.Error == "interaction_required" {
		return Token{}, fmt.Errorf("%w (%s)", ErrLost, firstLine(a.Description))
	}
	if a.Error != "" {
		return Token{}, failure(a)
	}
	if a.RefreshToken == "" {
		a.RefreshToken = refresh
	}
	return tokenOf(a, account)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return plain(line)
}

func tokenOf(a answer, account string) (Token, error) {
	if a.AccessToken == "" || a.RefreshToken == "" {
		return Token{}, errors.New("no token to stay signed in with came back; check that the app has offline_access")
	}
	life := time.Hour
	if a.ExpiresIn > 0 {
		life = seconds(a.ExpiresIn, time.Minute, maxTokenLife)
	}
	if name := plain(accountOf(a.IDToken)); name != "" {
		account = name
	}
	return Token{Access: a.AccessToken, Refresh: a.RefreshToken, ExpiresAt: time.Now().Add(life), Account: account}, nil
}

// accountOf is the account an ID token names, for showing who signed in.
// It comes straight from Microsoft over TLS, and is only shown, so its
// signature is not checked.
func accountOf(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Preferred string `json:"preferred_username"`
		Email     string `json:"email"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	if claims.Preferred != "" {
		return claims.Preferred
	}
	return claims.Email
}
