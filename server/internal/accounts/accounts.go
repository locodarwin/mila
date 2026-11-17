// Package accounts is everything about people: first-run setup, signing in
// by passkey or magic link, sessions, workspaces, members and invites.
// Handlers call it; it calls the store and asks authz before acting.
package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
)

// Session lifetimes.
const (
	SessionLifetime = 30 * 24 * time.Hour
	// SessionMaxAge is how long a session lasts however often it is used:
	// a stolen cookie used now and then would otherwise never expire.
	SessionMaxAge     = 90 * 24 * time.Hour
	sessionTouchEvery = time.Hour // sliding renewal writes at most this often
	magicLinkLifetime = 15 * time.Minute
	magicLinksPerHour = 5
	// emailChangeLifetime is how long the link to confirm a new address
	// works, and emailChangesPerHour how many such links one person may
	// have sent in an hour.
	emailChangeLifetime = 24 * time.Hour
	emailChangesPerHour = 5
	inviteLifetime      = 7 * 24 * time.Hour
	// maxPendingInvites is how many invitations a workspace may have waiting.
	maxPendingInvites = 50
	challengeLifetime = 5 * time.Minute
)

// The error kinds handlers map to status codes, shared with other packages.
type (
	Kind  = apperr.Kind
	Error = apperr.Error
)

const (
	Invalid         = apperr.Invalid
	NotFound        = apperr.NotFound
	Forbidden       = apperr.Forbidden
	Conflict        = apperr.Conflict
	Unauthenticated = apperr.Unauthenticated
)

func fail(k Kind, format string, args ...any) error { return apperr.New(k, format, args...) }

// KindOf returns the Kind of err, or 0 for unexpected errors.
func KindOf(err error) Kind { return apperr.KindOf(err) }

// Config is what the service needs from the server's settings.
type Config struct {
	BaseURL string // public URL, no trailing slash
	Origin  string // scheme and host of BaseURL
	RPID    string // WebAuthn relying party: BaseURL's hostname
	// SignInMailAfter sends sign-in links after answering, as the server
	// does: how long the answer takes then says nothing about whether the
	// address has an account, nor does a mail server that fails. Tests
	// leave it off to read the mail at once.
	SignInMailAfter bool
}

// Events hears about account changes that other parts of the server act
// on: the sync hub closes a signed-out session's socket, tells people
// whose memberships changed to reload their workspaces, tells a user's
// other devices their preferences changed, tells a member whose access
// to a workspace's projects changed to load it afresh, and tells the
// people someone shares a workspace with that their name or picture
// changed, and tells a user's devices one of their themes changed.
type Events interface {
	SessionEnded(sessionID string)
	MembershipChanged(userIDs ...string)
	SettingsChanged(userID string)
	AccessChanged(workspaceID, userID string)
	PeopleChanged(userIDs ...string)
	ThemesChanged(userID, themeID string)
}

// Notices hears about account changes people are told about.
type Notices interface {
	// InviteAccepted is called once someone joined a workspace through an
	// invitation, with who sent it.
	InviteAccepted(ctx context.Context, workspaceID, inviterID, userID string)
	// NewDeviceSignIn is called after someone signed in from a browser
	// their account had not used before, with the sign-in's event.
	NewDeviceSignIn(ctx context.Context, user store.User, ev SecurityEvent)
	// EmailChanged is called after someone's new email address took
	// over, with the user as they are now, the address they had, and the
	// change's event.
	EmailChanged(ctx context.Context, user store.User, oldEmail string, ev SecurityEvent)
	// WebhooksPaused is called after a workspace's webhooks were paused
	// because whoever made them can no longer manage them.
	WebhooksPaused(ctx context.Context, workspaceID string)
	// PasskeyAdded is called after a passkey was added to an account,
	// with the event that records it.
	PasskeyAdded(ctx context.Context, user store.User, ev SecurityEvent)
}

type noEvents struct{}

func (noEvents) SessionEnded(string)          {}
func (noEvents) MembershipChanged(...string)  {}
func (noEvents) SettingsChanged(string)       {}
func (noEvents) AccessChanged(string, string) {}
func (noEvents) PeopleChanged(...string)      {}
func (noEvents) ThemesChanged(string, string) {}

type noNotices struct{}

func (noNotices) InviteAccepted(context.Context, string, string, string)          {}
func (noNotices) WebhooksPaused(context.Context, string)                          {}
func (noNotices) NewDeviceSignIn(context.Context, store.User, SecurityEvent)      {}
func (noNotices) EmailChanged(context.Context, store.User, string, SecurityEvent) {}
func (noNotices) PasskeyAdded(context.Context, store.User, SecurityEvent)         {}

// Service is the accounts API used by the HTTP handlers and the CLI.
type Service struct {
	st       *store.Store
	mail     mailer.Sender
	log      *slog.Logger
	cfg      Config
	webauthn *webauthn.WebAuthn
	events   Events
	notices  Notices
	content  Content
}

// SetEvents connects a listener for account changes.
func (s *Service) SetEvents(e Events) { s.events = e }

// SetNotices connects the notifications for account changes.
func (s *Service) SetNotices(n Notices) { s.notices = n }

// New builds the service. Passkeys need a domain name: served from an IP
// address, Mila runs with magic links only.
func New(st *store.Store, m mailer.Sender, log *slog.Logger, cfg Config) (*Service, error) {
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	if net.ParseIP(cfg.RPID) != nil {
		log.Warn("passkeys are off because Mila is served from an IP address; use a domain name (even localhost) to turn them on", "host", cfg.RPID)
		return &Service{st: st, mail: m, log: log, cfg: cfg, events: noEvents{}, notices: noNotices{}}, nil
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: "Mila",
		RPOrigins:     []string{cfg.Origin},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn: %w", err)
	}
	return &Service{st: st, mail: m, log: log, cfg: cfg, webauthn: wa, events: noEvents{}, notices: noNotices{}}, nil
}

// PasskeysAvailable reports whether passkeys can be used on this instance.
func (s *Service) PasskeysAvailable() bool { return s.webauthn != nil }

func (s *Service) requirePasskeys() error {
	if s.webauthn == nil {
		return fail(Invalid, "Passkeys need Mila to be served from a domain name, not an IP address. Sign in with an email link instead.")
	}
	return nil
}

// newToken returns a random secret for a link or cookie, and the hash that
// is stored in its place.
func newToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// normalizeEmail lowercases and validates an address.
func normalizeEmail(raw string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil || a.Name != "" || !strings.Contains(a.Address, "@") {
		return "", fail(Invalid, "%q is not an email address", raw)
	}
	return strings.ToLower(a.Address), nil
}

// cleanName trims a display name and checks its length.
func cleanName(raw, what string) (string, error) {
	// Postgres refuses a NUL in text; the API refuses one at the door, and
	// this keeps any other way in from storing one.
	if strings.ContainsRune(raw, 0) {
		return "", fail(Invalid, "%s can't hold a NUL character", what)
	}
	// Names show on other people's screens and in plain-text email, so
	// control characters (a terminal's escapes) and the marks that turn
	// text around (bidi overrides, which make a name read as another)
	// become spaces. Other invisible format characters (a zero-width
	// space, the marks that set a direction, a BOM, a soft hyphen) are left
	// out, or one member's name could read exactly as another's. Joiners
	// stay: emoji and some scripts need them, as a subdivision's flag needs
	// the tags after its black flag.
	inFlag := false
	raw = strings.Map(func(r rune) rune {
		flag := inFlag
		inFlag = r == '\U0001F3F4' || inFlag && r >= '\U000E0020' && r <= '\U000E007F'
		switch {
		case unicode.IsControl(r) || r >= '\u202A' && r <= '\u202E' || r >= '\u2066' && r <= '\u2069':
			return ' '
		case r == '\u200C' || r == '\u200D' || flag && r >= '\U000E0020' && r <= '\U000E007F':
			return r
		case unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, raw)
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", fail(Invalid, "%s must not be empty", what)
	}
	if len([]rune(name)) > 100 {
		return "", fail(Invalid, "%s must be at most 100 characters", what)
	}
	return name, nil
}

// slugFor makes a URL slug from a name, with a random suffix so two
// workspaces with the same name never collide. It holds ASCII letters,
// digits and dashes only, so cutting it by bytes never splits a character.
func slugFor(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	base := strings.Trim(b.String(), "-")
	if len(base) > 40 {
		base = strings.Trim(base[:40], "-")
	}
	suffix := make([]byte, 5)
	_, _ = rand.Read(suffix)
	tail := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(suffix))[:6]
	if base == "" {
		return tail
	}
	return base + "-" + tail
}

// SlugFor makes a new workspace's URL slug from its name.
func SlugFor(name string) string { return slugFor(name) }
