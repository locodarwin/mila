package accounts

import (
	"context"
	"database/sql"
	"errors"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Authenticated is a request's signed-in user and session.
type Authenticated struct {
	User    store.User
	Session store.Session
	// Token is the API token the request came with; nil with a session.
	Token *sqlitedb.ApiToken
}

// Authenticate looks up the session for a cookie token, renewing its
// 30-day expiry as it is used, but never past 90 days from signing in,
// and noting the address it was last used from. It returns an Unauthenticated error for a missing, expired or
// revoked session.
func (s *Service) Authenticate(ctx context.Context, token, ip string) (*Authenticated, error) {
	if token == "" {
		return nil, fail(Unauthenticated, "Not signed in.")
	}
	now := store.Now()
	sess, err := s.st.GetActiveSession(ctx, sqlitedb.GetActiveSessionParams{TokenHash: hashToken(token), Now: now})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fail(Unauthenticated, "Not signed in.")
	}
	if err != nil {
		return nil, err
	}
	if now.Sub(sess.CreatedAt) > SessionMaxAge {
		return nil, fail(Unauthenticated, "Not signed in.")
	}
	if ip = clip(ip, 64); now.Sub(sess.LastSeenAt) > sessionTouchEvery || (ip != "" && ip != sess.Ip) {
		sess.LastSeenAt, sess.ExpiresAt = now, now.Add(SessionLifetime)
		if limit := sess.CreatedAt.Add(SessionMaxAge); sess.ExpiresAt.After(limit) {
			sess.ExpiresAt = limit
		}
		if ip != "" {
			sess.Ip = ip
		}
		if err := s.st.TouchSession(ctx, sqlitedb.TouchSessionParams{ID: sess.ID, Now: now, ExpiresAt: sess.ExpiresAt, Ip: sess.Ip}); err != nil {
			return nil, err
		}
	}
	user, err := s.st.GetUser(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	return &Authenticated{User: user, Session: sess}, nil
}

// SignOut ends the session behind a cookie token.
func (s *Service) SignOut(ctx context.Context, token string) error {
	sess, err := s.st.GetActiveSession(ctx, sqlitedb.GetActiveSessionParams{TokenHash: hashToken(token), Now: store.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.st.RevokeSessionByTokenHash(ctx, sqlitedb.RevokeSessionByTokenHashParams{TokenHash: hashToken(token), Now: store.NullNow()}); err != nil {
		return err
	}
	s.events.SessionEnded(sess.ID)
	return nil
}

// Sessions lists a user's active sessions, most recently used first.
func (s *Service) Sessions(ctx context.Context, userID string) ([]store.Session, error) {
	return s.st.ListActiveSessions(ctx, sqlitedb.ListActiveSessionsParams{UserID: userID, Now: store.Now()})
}

// RevokeSession signs one of the user's devices out, and notes it in
// their security log.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string, c Client) error {
	err := s.st.InTx(ctx, func(q store.Queries) error {
		sess, err := q.GetUserSession(ctx, sqlitedb.GetUserSessionParams{ID: sessionID, UserID: userID})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "No such session.")
		}
		if err != nil {
			return err
		}
		n, err := q.RevokeSession(ctx, sqlitedb.RevokeSessionParams{ID: sessionID, UserID: userID, Now: store.NullNow()})
		if err != nil {
			return err
		}
		if n == 0 {
			return fail(NotFound, "No such session.")
		}
		_, err = record(ctx, q, userID, EventSignedOut, MethodSelf, sess.UserAgent, c, false, store.Now())
		return err
	})
	if err == nil {
		s.events.SessionEnded(sessionID)
	}
	return err
}

// Rename changes the user's display name. The name they have already
// changes nothing and tells nobody.
func (s *Service) Rename(ctx context.Context, userID, name string) (store.User, error) {
	name, err := cleanName(name, "Name")
	if err != nil {
		return store.User{}, err
	}
	u, err := s.st.GetUser(ctx, userID)
	if err != nil || u.Name == name {
		return u, err
	}
	if err := s.st.UpdateUserName(ctx, sqlitedb.UpdateUserNameParams{ID: userID, Name: name, Now: store.Now()}); err != nil {
		return store.User{}, err
	}
	s.peopleChanged(ctx, userID)
	return s.st.GetUser(ctx, userID)
}
