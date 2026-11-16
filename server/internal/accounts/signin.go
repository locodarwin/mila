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

const setupTokenKey = "setup_token_hash"

// SetupRequired reports whether the instance has no users yet.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	n, err := s.st.CountUsers(ctx)
	return n == 0, err
}

// SetupLink creates a fresh one-time link that creates the instance owner,
// or returns "" when the instance already has users. The server prints it
// at startup; a new one replaces the old on every start.
func (s *Service) SetupLink(ctx context.Context) (string, error) {
	needed, err := s.SetupRequired(ctx)
	if err != nil || !needed {
		return "", err
	}
	token, hash := newToken()
	if err := s.st.SetInstanceSetting(ctx, setupTokenKey, hash); err != nil {
		return "", err
	}
	return s.cfg.BaseURL + "/setup?token=" + token, nil
}

// SignedIn is the result of any successful sign-in: who, and the session
// token to put in their cookie.
type SignedIn struct {
	User         store.User
	SessionToken string
	// DeviceToken is the browser's device cookie: the one it sent, or a
	// new one when it had none.
	DeviceToken string
}

// CompleteSetup creates the first user as instance admin, with a personal
// workspace, and signs them in. The setup token works once.
func (s *Service) CompleteSetup(ctx context.Context, token, name, email string, c Client) (*SignedIn, error) {
	name, err := cleanName(name, "Name")
	if err != nil {
		return nil, err
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	var user store.User
	err = s.st.InTx(ctx, func(q store.Queries) error {
		stored, err := q.GetInstanceSetting(ctx, setupTokenKey)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && stored != hashToken(token)) {
			return fail(Forbidden, "This setup link is not valid. Restart Mila to print a new one.")
		}
		if err != nil {
			return err
		}
		if n, err := q.CountUsers(ctx); err != nil {
			return err
		} else if n > 0 {
			return fail(Conflict, "Mila is already set up.")
		}
		user, err = createUser(ctx, q, name, email, true)
		if err != nil {
			return err
		}
		return q.DeleteInstanceSetting(ctx, setupTokenKey)
	})
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, c, MethodSetup)
}

// createUser makes a user and their personal workspace.
func createUser(ctx context.Context, q store.Queries, name, email string, admin bool) (store.User, error) {
	now := store.Now()
	id := newID()
	if err := q.CreateUser(ctx, sqlitedb.CreateUserParams{
		ID: id, Email: email, Name: name, IsInstanceAdmin: admin, Now: now,
	}); err != nil {
		return store.User{}, err
	}
	wsID := newID()
	if err := q.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{
		ID: wsID, Name: "Personal", Slug: slugFor(name), IsPersonal: true, Now: now,
	}); err != nil {
		return store.User{}, err
	}
	if err := q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{
		WorkspaceID: wsID, UserID: id, Role: "owner", Now: now,
	}); err != nil {
		return store.User{}, err
	}
	return q.GetUser(ctx, id)
}

// RequestMagicLink emails a sign-in link if the address belongs to a user.
// It reports success either way, so the form cannot be used to find out
// who has an account, and quietly sends nothing past five links an hour.
func (s *Service) RequestMagicLink(ctx context.Context, rawEmail string) error {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	user, err := s.st.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		// The same work as for an account, so the time taken tells nothing:
		// a link nobody is sent, for an address no account has, which
		// expires unused.
		_, err := s.createMagicLink(ctx, email)
		return err
	}
	if err != nil {
		return err
	}
	link, err := s.createMagicLink(ctx, email)
	if err != nil || link == "" {
		return err
	}
	return s.sendSignInLink(ctx, mailer.Message{
		To:      email,
		Subject: "Sign in to Mila",
		Text: "Hi " + user.Name + ",\n\nOpen this link to sign in to Mila:\n\n" + link +
			"\n\nIt works once and expires in 15 minutes. If you did not ask for it, you can ignore this email.\n",
	})
}

// LoginLink makes a sign-in link without sending it, for the admin
// command that rescues an instance with no email set up.
func (s *Service) LoginLink(ctx context.Context, rawEmail string) (string, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return "", err
	}
	if _, err := s.st.GetUserByEmail(ctx, email); errors.Is(err, sql.ErrNoRows) {
		return "", fail(NotFound, "No user has the email %s.", email)
	} else if err != nil {
		return "", err
	}
	token, hash := newToken()
	now := store.Now()
	err = s.st.CreateMagicLink(ctx, sqlitedb.CreateMagicLinkParams{
		TokenHash: hash, Email: email, Now: now, ExpiresAt: now.Add(magicLinkLifetime),
	})
	return s.cfg.BaseURL + "/auth/link?token=" + token, err
}

func (s *Service) createMagicLink(ctx context.Context, email string) (string, error) {
	now := store.Now()
	recent, err := s.st.CountRecentMagicLinks(ctx, sqlitedb.CountRecentMagicLinksParams{
		Email: email, Since: now.Add(-time.Hour),
	})
	if err != nil {
		return "", err
	}
	if recent >= magicLinksPerHour {
		s.log.Warn("magic link limit reached", "email", email)
		return "", nil
	}
	token, hash := newToken()
	err = s.st.CreateMagicLink(ctx, sqlitedb.CreateMagicLinkParams{
		TokenHash: hash, Email: email, Now: now, ExpiresAt: now.Add(magicLinkLifetime),
	})
	return s.cfg.BaseURL + "/auth/link?token=" + token, err
}

// MagicLinkEmail is the address a sign-in link signs in to, without
// spending it: the link's page asks before it switches a browser signed
// in to another account, so a link someone sends cannot quietly sign a
// person in as them.
func (s *Service) MagicLinkEmail(ctx context.Context, token string) (string, error) {
	email, err := s.st.PeekMagicLink(ctx, sqlitedb.PeekMagicLinkParams{TokenHash: hashToken(token), Now: store.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return "", fail(Forbidden, "This sign-in link has expired or was already used. Ask for a new one.")
	}
	return email, err
}

// VerifyMagicLink signs in with a link's token. Each token works once.
func (s *Service) VerifyMagicLink(ctx context.Context, token string, c Client) (*SignedIn, error) {
	email, err := s.st.UseMagicLink(ctx, sqlitedb.UseMagicLinkParams{TokenHash: hashToken(token), Now: store.NullNow()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fail(Forbidden, "This sign-in link has expired or was already used. Ask for a new one.")
	}
	if err != nil {
		return nil, err
	}
	user, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, c, MethodMagicLink)
}

// sendSignInLink sends a sign-in link, after the answer when
// cfg.SignInMailAfter says so; a failure is then only logged.
func (s *Service) sendSignInLink(ctx context.Context, m mailer.Message) error {
	if !s.cfg.SignInMailAfter {
		return s.mail.Send(ctx, m)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if err := s.mail.Send(ctx, m); err != nil {
			s.log.Warn("sending a sign-in link", "err", err)
		}
	}()
	return nil
}
