package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// desktopCodeLifetime is how long the desktop app has to use a code. It is
// listening when the browser hands it over, so this is only for a slow machine.
const desktopCodeLifetime = 5 * time.Minute

// desktopChallenge is a base64url SHA-256, as RFC 7636's S256 has it.
var desktopChallenge = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// CreateDesktopCode makes the one-time code a signed-in browser hands to the
// desktop app on the same computer. The app asked with the hash of a secret only
// it holds, so a code that goes astray is no use to anyone else. Only a browser's
// session can do this: one that came from a code can't make another, or a session
// taken while recent could stay recent for good by making more of itself.
func (s *Service) CreateDesktopCode(ctx context.Context, userID, method, challenge string) (string, error) {
	if method == MethodDesktop {
		return "", fail(Forbidden, "Sign in to the app from your browser.")
	}
	if !desktopChallenge.MatchString(challenge) {
		return "", fail(Invalid, "That sign-in request is not one Mila can use.")
	}
	code, hash := newToken()
	now := store.Now()
	err := s.st.InsertDesktopCode(ctx, sqlitedb.InsertDesktopCodeParams{
		CodeHash: hash, UserID: userID, Challenge: challenge, Now: now, ExpiresAt: now.Add(desktopCodeLifetime),
	})
	if err != nil {
		return "", err
	}
	// Old codes go here rather than on a schedule: nothing else makes them.
	if err := s.st.PurgeDesktopCodes(ctx, now); err != nil {
		s.log.Warn("forgetting old desktop codes", "err", err)
	}
	return code, nil
}

// FinishDesktopSignIn signs the desktop app's window in with a code and the secret
// behind it. It goes through startSession, so it is in the security log, and a
// computer not seen before is noticed like any other. Whatever the browser signed in
// with, this never counts as a passkey: the person didn't use one in this window.
func (s *Service) FinishDesktopSignIn(ctx context.Context, code, verifier string, c Client) (*SignedIn, error) {
	refused := fail(Invalid, "That sign-in has expired or was already used. Try again from the app.")
	taken, err := s.st.TakeDesktopCode(ctx, sqlitedb.TakeDesktopCodeParams{CodeHash: hashToken(code), Now: store.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(want), []byte(taken.Challenge)) != 1 {
		return nil, refused
	}
	user, err := s.st.GetUser(ctx, taken.UserID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && user.DeletedAt.Valid {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, c, MethodDesktop)
}

const (
	// MethodDesktop is how the desktop app's window signs in to a server.
	MethodDesktop = "desktop"
	// MethodLocal is how the desktop app's window signs in on this computer.
	MethodLocal = "local"
)

// EnsureLocalOwner makes the one person of the desktop app's mode, the first time.
func (s *Service) EnsureLocalOwner(ctx context.Context) error {
	needed, err := s.SetupRequired(ctx)
	if err != nil || !needed {
		return err
	}
	return s.st.InTx(ctx, func(q store.Queries) error {
		_, err := createUser(ctx, q, "You", "you@local.invalid", true)
		return err
	})
}

// SignInLocal signs in the one person of the desktop app's mode.
func (s *Service) SignInLocal(ctx context.Context, c Client) (*SignedIn, error) {
	user, err := s.st.GetUserByEmail(ctx, "you@local.invalid")
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, c, MethodLocal)
}

// LocalNotices is how many notices the desktop app takes at once, while the
// window shows another place.
const LocalNotices = 20

// LocalWatch is what the desktop app reads of its place on this computer while the
// window shows another place: how many workspaces the one person has, and their
// unread notifications made after since, oldest first and LocalNotices at most.
func (s *Service) LocalWatch(ctx context.Context, since time.Time) (int, []sqlitedb.Notification, error) {
	user, err := s.st.GetUserByEmail(ctx, "you@local.invalid")
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	spaces, err := s.Workspaces(ctx, user.ID)
	if err != nil {
		return 0, nil, err
	}
	unread, err := s.st.ListUnreadNotificationsSince(ctx, sqlitedb.ListUnreadNotificationsSinceParams{UserID: user.ID, Since: since})
	if err != nil {
		return 0, nil, err
	}
	slices.SortFunc(unread, func(a, b sqlitedb.Notification) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(unread) > LocalNotices {
		unread = unread[:LocalNotices]
	}
	return len(spaces), unread, nil
}
