package accounts

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/authz"
	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// API tokens (docs/PLAN.md section 11): a person's way in for a script or
// another app. A token acts as its owner, and httpapi.tokenRefuses keeps it to
// the work itself, away from anything that manages the account.

// TokenPrefix starts every token, so a secret scanner can spot a leaked one.
const TokenPrefix = "mila_"

const (
	maxTokens       = 20
	maxTokenName    = 80
	tokenTouchEvery = time.Minute
)

// TokenInput is what a new token is to be.
type TokenInput struct {
	Name          string
	CanWrite      bool
	WorkspaceID   string // "" for every workspace the person is in
	ExpiresInDays int    // 0 for never
	// OAuthClientID is the app the token is for, when an app was given it
	// by OAuth: it reaches only the MCP endpoint, and replaces any the app
	// had from this person before.
	OAuthClientID string
}

// CreateToken makes a token and returns it with its secret, which is never
// available again. The person is told by email.
func (s *Service) CreateToken(ctx context.Context, userID string, in TokenInput, c Client) (sqlitedb.ApiToken, string, error) {
	var t sqlitedb.ApiToken
	var secret string
	var user store.User
	err := s.st.InTx(ctx, func(q store.Queries) error {
		var err error
		t, secret, user, err = s.makeToken(ctx, q, userID, in, c)
		return err
	})
	if err != nil {
		return sqlitedb.ApiToken{}, "", err
	}
	s.tokenMail(ctx, user, t)
	return t, secret, nil
}

// makeToken makes a token inside a transaction.
func (s *Service) makeToken(ctx context.Context, q store.Queries, userID string, in TokenInput, c Client) (sqlitedb.ApiToken, string, store.User, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > maxTokenName {
		return sqlitedb.ApiToken{}, "", store.User{}, fail(Invalid, "Give the token a name of up to 80 characters.")
	}
	// History names a change by the token that made it, beside the changes
	// automations and email make; a token must not pass for one of those.
	if reservedTokenName(name) {
		return sqlitedb.ApiToken{}, "", store.User{}, fail(Invalid, "History names changes made by email or by an automation so. Give the token a name of its own.")
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > 3650 {
		return sqlitedb.ApiToken{}, "", store.User{}, fail(Invalid, "A token can last up to 10 years, or never expire.")
	}
	random, _ := newToken()
	secret := TokenPrefix + random
	hash := hashToken(secret)
	now := store.Now()
	t := sqlitedb.ApiToken{ID: newID(), UserID: userID, Name: name, TokenHash: hash, CanWrite: in.CanWrite, CreatedAt: now}
	if in.WorkspaceID != "" {
		t.WorkspaceID = sql.NullString{String: in.WorkspaceID, Valid: true}
	}
	if in.ExpiresInDays > 0 {
		t.ExpiresAt = sql.NullTime{Time: now.AddDate(0, 0, in.ExpiresInDays), Valid: true}
	}
	if in.OAuthClientID != "" {
		t.OauthClientID = sql.NullString{String: in.OAuthClientID, Valid: true}
	}
	user, err := q.GetUser(ctx, userID)
	if err != nil {
		return sqlitedb.ApiToken{}, "", store.User{}, err
	}
	if in.WorkspaceID != "" {
		if _, _, err := s.access(ctx, q, userID, in.WorkspaceID, authz.ViewWorkspace); err != nil {
			return sqlitedb.ApiToken{}, "", store.User{}, err
		}
	}
	if in.OAuthClientID != "" {
		if err := q.RevokeAppApiTokens(ctx, sqlitedb.RevokeAppApiTokensParams{Now: sql.NullTime{Time: now, Valid: true}, UserID: userID, ClientID: t.OauthClientID}); err != nil {
			return sqlitedb.ApiToken{}, "", store.User{}, err
		}
	}
	n, err := q.CountApiTokens(ctx, userID)
	if err != nil {
		return sqlitedb.ApiToken{}, "", store.User{}, err
	}
	if n >= maxTokens {
		return sqlitedb.ApiToken{}, "", store.User{}, fail(Invalid, "You can have 20 tokens at most. Revoke one you no longer use.")
	}
	if err := q.InsertApiToken(ctx, sqlitedb.InsertApiTokenParams{
		ID: t.ID, UserID: userID, Name: name, TokenHash: hash, CanWrite: in.CanWrite,
		WorkspaceID: t.WorkspaceID, Now: now, ExpiresAt: t.ExpiresAt, OauthClientID: t.OauthClientID,
	}); err != nil {
		return sqlitedb.ApiToken{}, "", store.User{}, err
	}
	event := EventTokenMade
	if in.OAuthClientID != "" {
		event = EventAppConnected
	}
	if _, err := record(ctx, q, userID, event, "", name, c, false, now); err != nil {
		return sqlitedb.ApiToken{}, "", store.User{}, err
	}
	return t, secret, user, nil
}

// reservedTokenName is a name history gives changes that came another way.
func reservedTokenName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return lower == "email" || strings.HasPrefix(lower, "automation") || lower == "an automation"
}

// tokenMail tells the person a token was made, or an app connected.
func (s *Service) tokenMail(ctx context.Context, user store.User, t sqlitedb.ApiToken) {
	m := mailer.Message{
		To:      user.Email,
		Subject: "A new API token was made for your Mila account",
		Text: "Hi " + user.Name + ",\n\nAn API token named \"" + t.Name + "\" was just made for your account. It can act as you, so if this was not you, revoke it now in Settings, API tokens:\n\n" +
			s.cfg.BaseURL + "/settings/tokens\n",
	}
	if t.OauthClientID.Valid {
		m.Subject = "An app was connected to your Mila account"
		// The name is the app's own words, which a stranger chose: said as
		// such, so it can't pass for Mila speaking.
		m.Text = "Hi " + user.Name + ",\n\nAn app that calls itself \"" + t.Name + "\" was just connected to your account, and can now use Mila's tools as you. That name is the app's own; Mila doesn't check it. If this was not you, revoke it now in Settings, API tokens:\n\n" +
			s.cfg.BaseURL + "/settings/tokens\n"
	}
	if err := s.mail.Send(ctx, m); err != nil {
		// The token is made, and is in the security log; that is enough.
		s.log.Warn("API token email not sent", "err", err)
	}
}

// Tokens lists someone's tokens that are not revoked, newest first.
func (s *Service) Tokens(ctx context.Context, userID string) ([]sqlitedb.ApiToken, error) {
	return s.st.ListApiTokens(ctx, userID)
}

// RevokeToken stops one of the person's tokens working, at once.
func (s *Service) RevokeToken(ctx context.Context, userID, tokenID string, c Client) error {
	return s.st.InTx(ctx, func(q store.Queries) error {
		now := store.Now()
		name, err := q.RevokeApiToken(ctx, sqlitedb.RevokeApiTokenParams{ID: tokenID, UserID: userID, Now: sql.NullTime{Time: now, Valid: true}})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "No such token.")
		}
		if err != nil {
			return err
		}
		_, err = record(ctx, q, userID, EventTokenRevoked, "", name, c, false, now)
		return err
	})
}

// AuthenticateToken finds who a token acts as. One that is unknown, revoked or
// past its date is Unauthenticated, as is one whose owner has gone.
func (s *Service) AuthenticateToken(ctx context.Context, secret string) (*Authenticated, error) {
	refused := fail(Unauthenticated, "That API token is not valid, or has expired or been revoked.")
	if !strings.HasPrefix(secret, TokenPrefix) {
		return nil, refused
	}
	t, err := s.st.GetApiTokenByHash(ctx, hashToken(secret))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, refused
	}
	if err != nil {
		return nil, err
	}
	now := store.Now()
	if t.ExpiresAt.Valid && !t.ExpiresAt.Time.After(now) {
		return nil, refused
	}
	user, err := s.st.GetUser(ctx, t.UserID)
	if err != nil {
		return nil, err
	}
	if user.DeletedAt.Valid {
		return nil, refused
	}
	if !t.LastUsedAt.Valid || now.Sub(t.LastUsedAt.Time) > tokenTouchEvery {
		if err := s.st.TouchApiToken(ctx, sqlitedb.TouchApiTokenParams{
			ID: t.ID, Now: sql.NullTime{Time: now, Valid: true}, Before: sql.NullTime{Time: now.Add(-tokenTouchEvery), Valid: true},
		}); err != nil {
			return nil, err
		}
	}
	return &Authenticated{User: user, Token: &t}, nil
}

// NoteWebhook puts a change to where a webhook sends, or who can check it, in the
// person's security log: it is a way out for the workspace's work.
func (s *Service) NoteWebhook(ctx context.Context, userID, address string, c Client) error {
	return s.st.InTx(ctx, func(q store.Queries) error {
		_, err := record(ctx, q, userID, EventWebhookChanged, "", address, c, false, store.Now())
		return err
	})
}
