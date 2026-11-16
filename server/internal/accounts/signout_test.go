package accounts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// Signing someone out everywhere leaves nothing that would let whoever
// took the account back in: an email change or a sign-in link asked for
// before, or a passkey they added.
func TestSignOutEverywhereLeavesNoWayBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		dana := f.member(t, owner, "dana@example.com", "Dana")
		f.signIn(t, "dana@example.com", accounts.Client{})

		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(f.svc.RequestEmailChange(ctx, dana.ID, "", "taker@example.com"))
		change := f.mail.lastToken(t)
		must(f.svc.RequestMagicLink(ctx, "dana@example.com"))
		link := f.mail.lastToken(t)
		for _, name := range []string{"Dana's laptop", "Not Dana's"} {
			_, err := d.ExecContext(ctx, rebind(d, "INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at) VALUES (?, ?, ?, ?, ?, ?)"),
				uuid.NewString(), dana.ID, []byte(uuid.NewString()), []byte("{}"), name, store.Now())
			must(err)
		}

		sent := f.mail.count()
		if _, err := f.svc.SignOutEverywhere(ctx, owner.ID, dana.ID, accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.ConfirmEmailChange(ctx, change, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
		_, err = f.svc.VerifyMagicLink(ctx, link, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
		var keys int
		must(d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM passkeys WHERE user_id = ?"), dana.ID).Scan(&keys))
		if keys != 0 {
			t.Errorf("%d passkeys left", keys)
		}
		got := kinds(t, f, dana.ID)
		if len(got) < 3 || got[0] != "session_revoked/admin" || got[1] != "passkey_removed/admin" || got[2] != "passkey_removed/admin" {
			t.Errorf("Dana's log = %v", got)
		}
		if f.mail.count() != sent+1 {
			t.Fatalf("%d emails sent", f.mail.count()-sent)
		}
		if m := f.mail.sent[sent]; m.To != "dana@example.com" || !strings.Contains(m.Text, "removed its 2 passkeys") {
			t.Errorf("the email = %+v", m)
		}
		// A new link signs her in again.
		f.signIn(t, "dana@example.com", accounts.Client{})
	})
}

// An email change works only while the session that asked for it lasts.
func TestEmailChangeEndsWithItsSession(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		session := func() string {
			t.Helper()
			in := f.signIn(t, "shawn@example.com", accounts.Client{})
			a, err := f.svc.Authenticate(ctx, in.SessionToken, "")
			if err != nil {
				t.Fatal(err)
			}
			return a.Session.ID
		}
		taken := session()
		if err := f.svc.RequestEmailChange(ctx, owner.ID, taken, "taker@example.com"); err != nil {
			t.Fatal(err)
		}
		link := f.mail.lastToken(t)
		if err := f.svc.RevokeSession(ctx, owner.ID, taken, accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.ConfirmEmailChange(ctx, link, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)

		if err := f.svc.RequestEmailChange(ctx, owner.ID, session(), "new@example.com"); err != nil {
			t.Fatal(err)
		}
		if u, err := f.svc.ConfirmEmailChange(ctx, f.mail.lastToken(t), accounts.Client{}); err != nil || u.Email != "new@example.com" {
			t.Fatalf("a change from a live session = %+v, %v", u, err)
		}
	})
}
