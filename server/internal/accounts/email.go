package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// RequestEmailChange emails a link to a new address for the user's
// account. The address changes only when the link is opened, so a typo
// locks nobody out, and only the newest link works: asking again ends the
// ones before. It works only while the session that asked lasts (sessionID,
// empty for none), so ending a session someone took ends what they asked
// for with it. An address another account uses is refused, as is a sixth
// link in an hour.
func (s *Service) RequestEmailChange(ctx context.Context, userID, sessionID, rawEmail string) error {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	user, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if email == user.Email {
		return fail(Invalid, "%s is already your email address.", email)
	}
	if _, err := s.st.GetUserByEmail(ctx, email); err == nil {
		return fail(Conflict, "Another account uses %s.", email)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	token, hash := newToken()
	now := store.Now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		recent, err := q.CountRecentEmailChanges(ctx, sqlitedb.CountRecentEmailChangesParams{
			UserID: userID, Since: now.Add(-time.Hour),
		})
		if err != nil {
			return err
		}
		if recent >= emailChangesPerHour {
			return fail(Conflict, "That is %d links in the last hour. Try again later.", emailChangesPerHour)
		}
		if err := q.EndEmailChanges(ctx, sqlitedb.EndEmailChangesParams{UserID: userID, Now: now}); err != nil {
			return err
		}
		return q.CreateEmailChange(ctx, sqlitedb.CreateEmailChangeParams{
			TokenHash: hash, UserID: userID, Email: email, Now: now, ExpiresAt: now.Add(emailChangeLifetime),
			SessionID: sql.NullString{String: sessionID, Valid: sessionID != ""},
		})
	})
	if err != nil {
		return err
	}
	return s.mail.Send(ctx, mailer.Message{
		To:      email,
		Subject: "Confirm your new email address for Mila",
		Text: "Hi " + user.Name + ",\n\nOpen this link to make " + email + " the email address of your Mila account:\n\n" +
			s.cfg.BaseURL + "/auth/email?token=" + token +
			"\n\nIt works once and expires in 24 hours. Until then your account keeps " + user.Email +
			". If you did not ask for this, you can ignore this email.\n",
	})
}

// ConfirmEmailChange makes the address a link was sent to the account's
// own. The link proves the address is the person's; it needs no session.
// Sign-in links still open for the old address stop working, the change
// goes in the security log, and the old address is told.
func (s *Service) ConfirmEmailChange(ctx context.Context, token string, c Client) (store.User, error) {
	now := store.Now()
	var user store.User
	var old string
	var ev SecurityEvent
	err := s.st.InTx(ctx, func(q store.Queries) error {
		change, err := q.UseEmailChange(ctx, sqlitedb.UseEmailChangeParams{TokenHash: hashToken(token), Now: store.NullNow()})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(Forbidden, "This link has expired, was already used, or a newer one replaced it. Ask for a new one in Settings, Profile.")
		}
		if err != nil {
			return err
		}
		user, err = q.GetUser(ctx, change.UserID)
		if err != nil {
			return err
		}
		if user.DeletedAt.Valid {
			return fail(Forbidden, "This account was deleted.")
		}
		if change.SessionID.Valid {
			sess, err := q.GetUserSession(ctx, sqlitedb.GetUserSessionParams{ID: change.SessionID.String, UserID: user.ID})
			if errors.Is(err, sql.ErrNoRows) || (err == nil && (sess.RevokedAt.Valid || !sess.ExpiresAt.After(now))) {
				return fail(Forbidden, "The session that asked for this link has signed out since, so the link no longer works. Sign in and ask for a new one in Settings, Profile.")
			}
			if err != nil {
				return err
			}
		}
		if other, err := q.GetUserByEmail(ctx, change.Email); err == nil && other.ID != user.ID {
			return fail(Conflict, "Another account has started using %s since the link was sent.", change.Email)
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		old = user.Email
		if err := q.UpdateUserEmail(ctx, sqlitedb.UpdateUserEmailParams{ID: user.ID, Email: change.Email, Now: now}); err != nil {
			return err
		}
		if err := q.DeleteMagicLinks(ctx, old); err != nil {
			return err
		}
		ev, err = record(ctx, q, user.ID, EventEmailChanged, MethodSelf, old+"\n"+change.Email, c, false, now)
		if err != nil {
			return err
		}
		user, err = q.GetUser(ctx, user.ID)
		return err
	})
	if err != nil {
		return store.User{}, err
	}
	// Members see each other's addresses, so their apps hear of it.
	s.peopleChanged(ctx, user.ID)
	s.notices.EmailChanged(ctx, user, old, ev)
	return user, nil
}
