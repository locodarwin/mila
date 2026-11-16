package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/locodarwin/mila/server/internal/authz"
	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Instance administration: the people with accounts on the instance, who
// among them are its admins, and every open invitation. Each call checks
// the caller's admin rights as they stand in the database now.

// InstanceAccess checks that someone may perform an instance action, for
// the admin endpoints whose work lies outside this package (the status
// page, a backup).
func (s *Service) InstanceAccess(ctx context.Context, userID string, action authz.InstanceAction) error {
	_, err := s.admin(ctx, s.st, userID, action)
	return err
}

// admin loads the caller and checks they may perform action.
func (s *Service) admin(ctx context.Context, q store.Queries, userID string, action authz.InstanceAction) (store.User, error) {
	u, err := q.GetUser(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return u, fail(Unauthenticated, "Sign in first.")
	}
	if err != nil {
		return u, err
	}
	if !authz.CanInstance(u.IsInstanceAdmin && !u.DeletedAt.Valid, action) {
		return u, fail(Forbidden, "Only an admin of this Mila can do that.")
	}
	return u, nil
}

// AdminUser is an account as the admin page lists it.
type AdminUser struct {
	store.User
	// Devices is how many sessions are signed in now.
	Devices int
	// LastActiveAt is when any of their sessions was last used, as far
	// back as ended sessions are kept (30 days); nil before that.
	LastActiveAt *time.Time
}

// Users lists every account on the instance, deleted ones left out, by
// name.
func (s *Service) Users(ctx context.Context, userID string) ([]AdminUser, error) {
	if _, err := s.admin(ctx, s.st, userID, authz.ViewInstance); err != nil {
		return nil, err
	}
	return s.users(ctx)
}

func (s *Service) users(ctx context.Context) ([]AdminUser, error) {
	users, err := s.st.ListLiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	activity, err := s.st.ListSessionActivity(ctx)
	if err != nil {
		return nil, err
	}
	now := store.Now()
	devices := map[string]int{}
	last := map[string]time.Time{}
	for _, a := range activity {
		if !a.RevokedAt.Valid && a.ExpiresAt.After(now) {
			devices[a.UserID]++
		}
		if a.LastSeenAt.After(last[a.UserID]) {
			last[a.UserID] = a.LastSeenAt
		}
	}
	out := make([]AdminUser, len(users))
	for i, u := range users {
		out[i] = AdminUser{User: u, Devices: devices[u.ID]}
		if t, ok := last[u.ID]; ok {
			out[i].LastActiveAt = &t
		}
	}
	return out, nil
}

// SetInstanceAdmin gives someone admin rights over the instance, or takes
// them away. The instance always keeps at least one admin; an admin may
// step down while another remains.
func (s *Service) SetInstanceAdmin(ctx context.Context, userID, targetID string, on bool, c Client) (AdminUser, error) {
	err := s.st.InTx(ctx, func(q store.Queries) error {
		actor, err := s.admin(ctx, q, userID, authz.ManageUsers)
		if err != nil {
			return err
		}
		return setAdmin(ctx, q, targetID, on, MethodAdmin, actor.Name, c)
	})
	if err != nil {
		return AdminUser{}, err
	}
	// Read without the check: an admin who just stepped down is told how
	// it went too.
	users, err := s.users(ctx)
	for _, u := range users {
		if u.ID == targetID {
			return u, err
		}
	}
	return AdminUser{}, err
}

// setAdmin changes one user's admin flag, keeping at least one admin, and
// notes the change in their security log.
func setAdmin(ctx context.Context, q store.Queries, targetID string, on bool, method, by string, c Client) error {
	target, err := q.GetUser(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && target.DeletedAt.Valid) {
		return fail(NotFound, "No such user.")
	}
	if err != nil {
		return err
	}
	if target.IsInstanceAdmin == on {
		return nil
	}
	if !on {
		if err := q.LockInstanceAdmins(ctx); err != nil {
			return err
		}
		n, err := q.CountInstanceAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return fail(Conflict, "Mila needs at least one admin. Make someone else an admin first.")
		}
	}
	now := store.Now()
	if err := q.SetInstanceAdmin(ctx, sqlitedb.SetInstanceAdminParams{ID: targetID, IsInstanceAdmin: on, Now: now}); err != nil {
		return err
	}
	kind := EventAdminGranted
	if !on {
		kind = EventAdminRevoked
	}
	_, err = record(ctx, q, targetID, kind, method, by, c, false, now)
	return err
}

// MakeAdmin gives an account admin rights, for `mila user make-admin`:
// the rescue for an instance whose admins are gone or locked out, which
// only someone at the server can run.
func (s *Service) MakeAdmin(ctx context.Context, rawEmail string) (store.User, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return u, fail(NotFound, "No user has the email %s.", email)
	}
	if err != nil {
		return u, err
	}
	if err := s.st.InTx(ctx, func(q store.Queries) error {
		return setAdmin(ctx, q, u.ID, true, MethodCommand, "", Client{})
	}); err != nil {
		return u, err
	}
	return s.st.GetUser(ctx, u.ID)
}

// SignOutEverywhere ends every session of a user, as an admin, and
// returns how many there were. Each of their devices wipes its copy on
// its next connection. Whatever else would let someone who took the
// account back in goes too: tokens, feeds, pending email changes, unused
// sign-in links and every passkey, so the person signs in again by email
// and adds theirs back; they are emailed to say so.
func (s *Service) SignOutEverywhere(ctx context.Context, userID, targetID string, c Client) (int, error) {
	var ended []string
	var actor, target store.User
	var keys []sqlitedb.Passkey
	err := s.st.InTx(ctx, func(q store.Queries) error {
		var err error
		actor, err = s.admin(ctx, q, userID, authz.ManageUsers)
		if err != nil {
			return err
		}
		target, err = q.GetUser(ctx, targetID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && target.DeletedAt.Valid) {
			return fail(NotFound, "No such user.")
		}
		if err != nil {
			return err
		}
		now := store.Now()
		if ended, err = q.RevokeUserSessions(ctx, sqlitedb.RevokeUserSessionsParams{UserID: targetID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		if err := q.RevokeUserApiTokens(ctx, sqlitedb.RevokeUserApiTokensParams{UserID: targetID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		if err := q.DeleteUserDesktopCodes(ctx, targetID); err != nil {
			return err
		}
		// An address outlives sessions as a token does, so a leaked one would too.
		if err := q.DeleteUserInboundAddresses(ctx, targetID); err != nil {
			return err
		}
		if err := q.RevokeUserIcalTokens(ctx, sqlitedb.RevokeUserIcalTokensParams{UserID: targetID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		// A link asked for meanwhile would undo it all: a new address for
		// the account, or a way back in.
		if err := q.EndEmailChanges(ctx, sqlitedb.EndEmailChangesParams{UserID: targetID, Now: now}); err != nil {
			return err
		}
		if err := q.EndMagicLinks(ctx, sqlitedb.EndMagicLinksParams{Email: target.Email, Now: now}); err != nil {
			return err
		}
		if keys, err = q.ListPasskeys(ctx, targetID); err != nil {
			return err
		}
		if err := q.DeleteUserPasskeys(ctx, targetID); err != nil {
			return err
		}
		if err := q.DeleteUserChallenges(ctx, sql.NullString{String: targetID, Valid: true}); err != nil {
			return err
		}
		for _, k := range keys {
			if _, err := record(ctx, q, targetID, EventPasskeyRemoved, MethodAdmin, k.Name, c, false, now); err != nil {
				return err
			}
		}
		_, err = record(ctx, q, targetID, EventSignedOut, MethodAdmin, actor.Name, c, false, now)
		return err
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ended {
		s.events.SessionEnded(id)
	}
	s.log.Info("signed out everywhere by an admin", "user", targetID, "admin", userID, "sessions", len(ended), "passkeys", len(keys))
	removed := ""
	if len(keys) > 0 {
		removed = " and removed its passkey, which can no longer sign you in"
		if len(keys) > 1 {
			removed = fmt.Sprintf(" and removed its %d passkeys, which can no longer sign you in", len(keys))
		}
	}
	if err := s.mail.Send(ctx, mailer.Message{
		To:      target.Email,
		Subject: "You were signed out of Mila everywhere",
		Text: "Hi " + target.Name + ",\n\n" + actor.Name + ", an admin of Mila, signed your account out on every device" + removed +
			". Your API tokens, calendar feeds and email-to-task addresses stopped working too.\n\nTo sign in again, ask for a sign-in link at:\n\n" +
			s.cfg.BaseURL + "/login\n\nThen make new ones in Settings as you need them, passkeys included.\n",
	}); err != nil {
		// It is done, and in the security log; that is enough.
		s.log.Warn("signed out everywhere email not sent", "err", err)
	}
	return len(ended), nil
}

// OpenInvite is an open invitation as the admin page lists it.
type OpenInvite = sqlitedb.ListAllPendingInvitesRow

// AllInvites lists every open invitation on the instance, oldest first.
func (s *Service) AllInvites(ctx context.Context, userID string) ([]OpenInvite, error) {
	if _, err := s.admin(ctx, s.st, userID, authz.ViewInstance); err != nil {
		return nil, err
	}
	return s.st.ListAllPendingInvites(ctx, store.Now())
}

// CancelInvite cancels any open invitation, as an admin.
func (s *Service) CancelInvite(ctx context.Context, userID, inviteID string) error {
	if _, err := s.admin(ctx, s.st, userID, authz.ManageInvites); err != nil {
		return err
	}
	n, err := s.st.RevokeAnyInvite(ctx, sqlitedb.RevokeAnyInviteParams{ID: inviteID, Now: store.NullNow()})
	if err == nil && n == 0 {
		return fail(NotFound, "No such invitation.")
	}
	return err
}

// CountUsers counts the accounts on the instance, deleted ones left out.
func (s *Service) CountUsers(ctx context.Context, userID string) (int, error) {
	if _, err := s.admin(ctx, s.st, userID, authz.ViewInstance); err != nil {
		return 0, err
	}
	users, err := s.st.ListLiveUsers(ctx)
	return len(users), err
}
