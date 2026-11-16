package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	ceremonyRegister = "register"
	ceremonyLogin    = "login"
)

// waUser adapts a user and their passkeys to the webauthn library.
type waUser struct {
	user  store.User
	creds []webauthn.Credential
}

// The user handle is the user ID's 16 raw bytes: stable, opaque and short.
func userHandle(id string) []byte {
	u := uuid.MustParse(id)
	return u[:]
}

func (u waUser) WebAuthnID() []byte                         { return userHandle(u.user.ID) }
func (u waUser) WebAuthnName() string                       { return u.user.Email }
func (u waUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Service) loadWAUser(ctx context.Context, user store.User) (waUser, error) {
	keys, err := s.st.ListPasskeys(ctx, user.ID)
	if err != nil {
		return waUser{}, err
	}
	wu := waUser{user: user}
	for _, k := range keys {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(k.Credential), &c); err != nil {
			return waUser{}, err
		}
		wu.creds = append(wu.creds, c)
	}
	return wu, nil
}

// Ceremony is the first half of a WebAuthn exchange: options for the
// browser, and the ID that brings the server's half back.
type Ceremony struct {
	ID      string
	Options any
}

func (s *Service) saveChallenge(ctx context.Context, kind string, userID *string, data *webauthn.SessionData) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	id := newID()
	uid := sql.NullString{}
	if userID != nil {
		uid = sql.NullString{String: *userID, Valid: true}
	}
	return id, s.st.CreateChallenge(ctx, sqlitedb.CreateChallengeParams{
		ID: id, UserID: uid, Kind: kind, Data: string(raw), ExpiresAt: store.Now().Add(challengeLifetime),
	})
}

func (s *Service) takeChallenge(ctx context.Context, kind, id string) (sqlitedb.WebauthnChallenge, webauthn.SessionData, error) {
	var data webauthn.SessionData
	ch, err := s.st.TakeChallenge(ctx, sqlitedb.TakeChallengeParams{ID: id, Kind: kind, Now: store.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return ch, data, fail(Invalid, "That took too long. Try again.")
	}
	if err != nil {
		return ch, data, err
	}
	return ch, data, json.Unmarshal([]byte(ch.Data), &data)
}

// BeginPasskeyRegistration starts adding a passkey to a signed-in user.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, user store.User) (*Ceremony, error) {
	if err := s.requirePasskeys(); err != nil {
		return nil, err
	}
	wu, err := s.loadWAUser(ctx, user)
	if err != nil {
		return nil, err
	}
	exclude := make([]protocol.CredentialDescriptor, len(wu.creds))
	for i := range wu.creds {
		exclude[i] = wu.creds[i].Descriptor()
	}
	options, data, err := s.webauthn.BeginRegistration(wu,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude),
	)
	if err != nil {
		return nil, err
	}
	id, err := s.saveChallenge(ctx, ceremonyRegister, &user.ID, data)
	if err != nil {
		return nil, err
	}
	return &Ceremony{ID: id, Options: options}, nil
}

// FinishPasskeyRegistration checks the browser's response and stores the
// new passkey under name, noting it in the security log.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, user store.User, ceremonyID string, response []byte, name string, c Client) (store.Passkey, error) {
	if err := s.requirePasskeys(); err != nil {
		return store.Passkey{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > 60 {
		return store.Passkey{}, fail(Invalid, "Passkey names must be at most 60 characters")
	}
	ch, data, err := s.takeChallenge(ctx, ceremonyRegister, ceremonyID)
	if err != nil {
		return store.Passkey{}, err
	}
	if !ch.UserID.Valid || ch.UserID.String != user.ID {
		return store.Passkey{}, fail(Forbidden, "That passkey request belongs to someone else.")
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return store.Passkey{}, fail(Invalid, "The browser's passkey response could not be read.")
	}
	wu, err := s.loadWAUser(ctx, user)
	if err != nil {
		return store.Passkey{}, err
	}
	cred, err := s.webauthn.CreateCredential(wu, data, parsed)
	if err != nil {
		s.log.Info("passkey registration rejected", "user", user.ID, "err", err)
		return store.Passkey{}, fail(Invalid, "The passkey could not be verified.")
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return store.Passkey{}, err
	}
	id := newID()
	var ev SecurityEvent
	var first bool
	err = s.st.InTx(ctx, func(q store.Queries) error {
		now := store.Now()
		before, err := q.ListPasskeys(ctx, user.ID)
		if err != nil {
			return err
		}
		first = len(before) == 0
		if err := q.CreatePasskey(ctx, sqlitedb.CreatePasskeyParams{
			ID: id, UserID: user.ID, CredentialID: cred.ID, Credential: string(raw), Name: name, Now: now,
		}); err != nil {
			return err
		}
		ev, err = record(ctx, q, user.ID, EventPasskeyAdded, "", name, c, false, now)
		return err
	})
	if err != nil {
		return store.Passkey{}, err
	}
	// A passkey is a way in for good, so the person hears of each one;
	// but not of the first, added in the hour an account was made (the
	// step after setting up or joining), which is no news to them.
	if !first || time.Since(user.CreatedAt) > time.Hour {
		s.notices.PasskeyAdded(ctx, user, ev)
	}
	return s.st.GetPasskeyByCredentialID(ctx, cred.ID)
}

// BeginPasskeyLogin starts a sign-in where the browser offers whichever
// of this site's passkeys it holds, so no email is needed.
func (s *Service) BeginPasskeyLogin(ctx context.Context) (*Ceremony, error) {
	if err := s.requirePasskeys(); err != nil {
		return nil, err
	}
	options, data, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, err
	}
	id, err := s.saveChallenge(ctx, ceremonyLogin, nil, data)
	if err != nil {
		return nil, err
	}
	return &Ceremony{ID: id, Options: options}, nil
}

// FinishPasskeyLogin checks the browser's assertion and signs the owner of
// the passkey in.
func (s *Service) FinishPasskeyLogin(ctx context.Context, ceremonyID string, response []byte, c Client) (*SignedIn, error) {
	if err := s.requirePasskeys(); err != nil {
		return nil, err
	}
	_, data, err := s.takeChallenge(ctx, ceremonyLogin, ceremonyID)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, fail(Invalid, "The browser's passkey response could not be read.")
	}
	var found store.User
	handler := func(rawID, handle []byte) (webauthn.User, error) {
		key, err := s.st.GetPasskeyByCredentialID(ctx, rawID)
		if err != nil {
			return nil, err
		}
		user, err := s.st.GetUser(ctx, key.UserID)
		if err != nil {
			return nil, err
		}
		found = user
		return s.loadWAUser(ctx, user)
	}
	_, cred, err := s.webauthn.ValidatePasskeyLogin(handler, data, parsed)
	if err != nil {
		s.log.Info("passkey sign-in rejected", "err", err)
		return nil, fail(Unauthenticated, "That passkey is not registered here, or could not be verified.")
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	key, err := s.st.GetPasskeyByCredentialID(ctx, cred.ID)
	if err != nil {
		return nil, err
	}
	if err := s.st.UpdatePasskeyAfterLogin(ctx, sqlitedb.UpdatePasskeyAfterLoginParams{
		ID: key.ID, Credential: string(raw), Now: store.NullNow(),
	}); err != nil {
		return nil, err
	}
	return s.startSession(ctx, found, c, MethodPasskey)
}

// Passkeys lists a user's passkeys.
func (s *Service) Passkeys(ctx context.Context, userID string) ([]store.Passkey, error) {
	return s.st.ListPasskeys(ctx, userID)
}

// RenamePasskey relabels one of the user's passkeys.
func (s *Service) RenamePasskey(ctx context.Context, userID, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return fail(Invalid, "Passkey names must be 1 to 60 characters")
	}
	n, err := s.st.RenamePasskey(ctx, sqlitedb.RenamePasskeyParams{ID: id, UserID: userID, Name: name})
	if err == nil && n == 0 {
		return fail(NotFound, "No such passkey.")
	}
	return err
}

// DeletePasskey removes one of the user's passkeys, noting it in the
// security log. Magic links still work afterward, so removing the last
// one cannot lock anyone out.
func (s *Service) DeletePasskey(ctx context.Context, userID, id string, c Client) error {
	var user store.User
	name := ""
	err := s.st.InTx(ctx, func(q store.Queries) error {
		var err error
		if user, err = q.GetUser(ctx, userID); err != nil {
			return err
		}
		keys, err := q.ListPasskeys(ctx, userID)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if k.ID == id {
				name = k.Name
			}
		}
		n, err := q.DeletePasskey(ctx, sqlitedb.DeletePasskeyParams{ID: id, UserID: userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return fail(NotFound, "No such passkey.")
		}
		_, err = record(ctx, q, userID, EventPasskeyRemoved, "", name, c, false, store.Now())
		return err
	})
	if err != nil {
		return err
	}
	if err := s.mail.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "A passkey was removed from your Mila account",
		Text: "Hi " + user.Name + ",\n\nThe passkey named \"" + name + "\" was just removed from your account, and can no longer sign you in. If this was not you, sign in, sign out the devices you do not know and look over your passkeys in Settings, Security:\n\n" +
			s.cfg.BaseURL + "/settings/security\n",
	}); err != nil {
		// The passkey is gone, and that is in the security log; that is enough.
		s.log.Warn("passkey removal email not sent", "err", err)
	}
	return nil
}
