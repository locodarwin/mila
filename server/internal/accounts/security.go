package accounts

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Client is where a request came from, as far as the server can tell:
// the browser's user agent, the client address (behind a trusted proxy,
// the one it reported), and the token in the browser's device cookie, ""
// when it has none yet.
type Client struct {
	UserAgent string
	IP        string
	Device    string
}

// SecurityEvent is one entry in a user's security log.
type SecurityEvent = sqlitedb.SecurityEvent

// The kinds of security event. What detail holds depends on the kind.
const (
	// EventSignIn is a sign-in; method says how, and new_device marks a
	// browser the account had not signed in from before.
	EventSignIn = "sign_in"
	// EventInviteAccepted is joining a workspace; detail is its name.
	EventInviteAccepted = "invite_accepted"
	// EventPasskeyAdded and EventPasskeyRemoved name the passkey.
	EventPasskeyAdded   = "passkey_added"
	EventPasskeyRemoved = "passkey_removed"
	// EventSignedOut is a device signed out from elsewhere. With method
	// "self", the person did it and detail is that device's user agent;
	// with "admin", an admin signed them out everywhere and detail is the
	// admin's name.
	EventSignedOut = "session_revoked"
	// EventAdminGranted and EventAdminRevoked are instance admin rights
	// given and taken. With method "admin", detail names the admin who
	// did it; "command" is `mila user make-admin`.
	EventAdminGranted = "admin_granted"
	EventAdminRevoked = "admin_revoked"
	// EventEmailChanged is a new email address confirmed; detail is the
	// old address and the new one, a line each.
	EventEmailChanged = "email_changed"
	// EventTokenMade and EventTokenRevoked name the API token in their detail.
	EventTokenMade    = "token_made"
	EventTokenRevoked = "token_revoked"
	// EventAppConnected is an app given a token by OAuth; detail is the app's name.
	EventAppConnected = "app_connected"
	// EventEmailAddressMade and EventEmailAddressRemoved name the workspace of an
	// address that turns email into tasks.
	EventEmailAddressMade    = "email_address_made"
	EventEmailAddressRemoved = "email_address_removed"
	EventEmailSenderAdded    = "email_sender_added"
	// EventWebhookChanged notes a new address or signing secret, the address in its detail.
	EventWebhookChanged = "webhook_changed"
)

// How someone signed in, and who changed something for them.
const (
	MethodSetup     = "setup"
	MethodMagicLink = "magic_link"
	MethodPasskey   = "passkey"
	MethodInvite    = "invite"
	MethodSelf      = "self"
	MethodAdmin     = "admin"
	MethodCommand   = "command"
)

// Retention of what the security layer keeps.
const (
	// DeviceLifetime is how long a browser's device cookie lasts, and how
	// long a device unused for that long is remembered.
	DeviceLifetime = 400 * 24 * time.Hour
	// securityLogKept is how long security events are kept.
	securityLogKept = 365 * 24 * time.Hour
	// endedKept is how long ended sessions and closed invitations stay,
	// so the admin page can still say when someone was last active.
	endedKept = 30 * 24 * time.Hour
)

// clip cuts what a browser sent, or a name someone chose, to at most n
// bytes worth storing, on a character's edge and as valid UTF-8, which
// Postgres insists on: a name cut through a character made the security
// log's insert fail, and the action it went with.
func clip(s string, n int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// validDevice reports whether a device cookie's value looks like one the
// server handed out; anything else is replaced.
func validDevice(token string) bool {
	if len(token) < 32 || len(token) > 64 {
		return false
	}
	return strings.IndexFunc(token, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_'
	}) < 0
}

// record writes a security event for a user.
func record(ctx context.Context, q store.Queries, userID, kind, method, detail string, c Client, newDevice bool, at time.Time) (SecurityEvent, error) {
	ev := SecurityEvent{
		ID: newID(), UserID: userID, Kind: kind, Method: method, NewDevice: newDevice,
		Detail: clip(detail, 300), Ip: clip(c.IP, 64), UserAgent: clip(c.UserAgent, 300), CreatedAt: at,
	}
	return ev, q.InsertSecurityEvent(ctx, sqlitedb.InsertSecurityEventParams{
		ID: ev.ID, UserID: ev.UserID, Kind: ev.Kind, Method: ev.Method, NewDevice: ev.NewDevice,
		Detail: ev.Detail, Ip: ev.Ip, UserAgent: ev.UserAgent, Now: ev.CreatedAt,
	})
}

// startSession signs a user in: a new session, the browser remembered as
// one of theirs, and the sign-in in their security log. A sign-in from a
// browser the account has not used before emails them, unless it is the
// account's first device of all (setting up Mila, or joining through an
// invitation, or the first sign-in since the security log began).
func (s *Service) startSession(ctx context.Context, user store.User, c Client, method string) (*SignedIn, error) {
	token, hash := newToken()
	device := c.Device
	if !validDevice(device) {
		device, _ = newToken()
	}
	now := store.Now()
	var ev SecurityEvent
	err := s.st.InTx(ctx, func(q store.Queries) error {
		err := q.CreateSession(ctx, sqlitedb.CreateSessionParams{
			ID: newID(), TokenHash: hash, UserID: user.ID, UserAgent: clip(c.UserAgent, 300), Ip: clip(c.IP, 64),
			Now: now, ExpiresAt: now.Add(SessionLifetime), Method: method,
		})
		if err != nil {
			return err
		}
		newDevice := false
		deviceHash := hashToken(device)
		seen, err := q.TouchKnownDevice(ctx, sqlitedb.TouchKnownDeviceParams{UserID: user.ID, DeviceHash: deviceHash, Now: now})
		if err != nil {
			return err
		}
		if seen == 0 {
			known, err := q.CountKnownDevices(ctx, user.ID)
			if err != nil {
				return err
			}
			newDevice = known > 0
			if err := q.InsertKnownDevice(ctx, sqlitedb.InsertKnownDeviceParams{UserID: user.ID, DeviceHash: deviceHash, Now: now}); err != nil {
				return err
			}
		}
		ev, err = record(ctx, q, user.ID, EventSignIn, method, "", c, newDevice, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	if ev.NewDevice {
		s.notices.NewDeviceSignIn(ctx, user, ev)
	}
	return &SignedIn{User: user, SessionToken: token, DeviceToken: device}, nil
}

// SecurityEvents lists a user's security log, newest first, at most limit
// entries from before the event with ID `before` ("" for the newest). The
// second result reports whether older ones remain.
func (s *Service) SecurityEvents(ctx context.Context, userID, before string, limit int) ([]SecurityEvent, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	list, err := s.st.ListSecurityEvents(ctx, sqlitedb.ListSecurityEventsParams{UserID: userID, Before: before, MaxRows: int64(limit + 1)})
	if err != nil {
		return nil, false, err
	}
	more := len(list) > limit
	if more {
		list = list[:limit]
	}
	return list, more, nil
}

// Tidied counts what Tidy removed.
type Tidied struct {
	Sessions, MagicLinks, EmailChanges, Challenges, Invites, Events, Devices int64
}

// Tidy removes sign-in records past their use: sessions and invitations
// that ended more than 30 days ago, used or expired sign-in links, email
// changes and passkey ceremonies, security events older than a year, and devices not
// signed in from for longer than their cookie lasts. The server runs it
// hourly.
func (s *Service) Tidy(ctx context.Context, now time.Time) (Tidied, error) {
	var t Tidied
	steps := []struct {
		n   *int64
		del func(context.Context, time.Time) (int64, error)
		at  time.Time
	}{
		{&t.Sessions, s.st.DeleteEndedSessions, now.Add(-endedKept)},
		// A day's grace keeps the hourly count of sign-in links whole.
		{&t.MagicLinks, s.st.DeleteOldMagicLinks, now.Add(-24 * time.Hour)},
		{&t.EmailChanges, s.st.DeleteOldEmailChanges, now.Add(-24 * time.Hour)},
		{&t.Challenges, s.st.DeleteExpiredChallenges, now},
		{&t.Invites, s.st.DeleteClosedInvites, now.Add(-endedKept)},
		{&t.Events, s.st.DeleteOldSecurityEvents, now.Add(-securityLogKept)},
		{&t.Devices, s.st.DeleteOldKnownDevices, now.Add(-DeviceLifetime)},
	}
	for _, step := range steps {
		n, err := step.del(ctx, step.at.UTC().Truncate(time.Microsecond))
		if err != nil {
			return t, err
		}
		*step.n = n
	}
	return t, nil
}
