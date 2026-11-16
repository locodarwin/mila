package accounts_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// member invites someone to a new shared workspace and has them accept.
func (f *fixture) member(t *testing.T, owner store.User, email, name string) store.User {
	t.Helper()
	ctx := context.Background()
	ws, err := f.svc.CreateWorkspace(ctx, owner.ID, "Team "+name)
	if err != nil {
		t.Fatal(err)
	}
	in, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner, ws.ID, email, authz.Member), nil, name, accounts.Client{})
	if err != nil {
		t.Fatal(err)
	}
	return in.User
}

func TestEmailChange(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		notices := &deviceNotices{}
		f.svc.SetNotices(notices)
		owner := f.setup(t).User
		f.member(t, owner, "dana@example.com", "Dana")

		wantKind(t, f.svc.RequestEmailChange(ctx, owner.ID, "", "not an address"), accounts.Invalid)
		wantKind(t, f.svc.RequestEmailChange(ctx, owner.ID, "", "SHAWN@example.com"), accounts.Invalid)
		wantKind(t, f.svc.RequestEmailChange(ctx, owner.ID, "", "Dana@Example.com"), accounts.Conflict)

		sent := f.mail.count()
		if err := f.svc.RequestEmailChange(ctx, owner.ID, "", " New@Example.com "); err != nil {
			t.Fatal(err)
		}
		if f.mail.count() != sent+1 || f.mail.sent[sent].To != "new@example.com" {
			t.Fatalf("the link went to %+v", f.mail.sent[sent:])
		}
		first := f.mail.lastToken(t)
		// Asking again ends the first link.
		if err := f.svc.RequestEmailChange(ctx, owner.ID, "", "newer@example.com"); err != nil {
			t.Fatal(err)
		}
		second := f.mail.lastToken(t)
		_, err := f.svc.ConfirmEmailChange(ctx, first, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)

		// Nothing has changed until the link is opened; a sign-in link
		// for the old address, still open then, stops working after.
		if err := f.svc.RequestMagicLink(ctx, "shawn@example.com"); err != nil {
			t.Fatal(err)
		}
		signIn := f.mail.lastToken(t)
		u, err := f.svc.ConfirmEmailChange(ctx, second, accounts.Client{UserAgent: "phone", IP: "203.0.113.9"})
		if err != nil || u.ID != owner.ID || u.Email != "newer@example.com" {
			t.Fatalf("ConfirmEmailChange = %+v, %v", u, err)
		}
		_, err = f.svc.VerifyMagicLink(ctx, signIn, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
		_, err = f.svc.ConfirmEmailChange(ctx, second, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)

		if len(notices.changed) != 1 || notices.changed[0] != "shawn@example.com" {
			t.Errorf("old addresses told = %v", notices.changed)
		}
		events, _, err := f.svc.SecurityEvents(ctx, owner.ID, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if ev := events[0]; ev.Kind != accounts.EventEmailChanged || ev.Detail != "shawn@example.com\nnewer@example.com" || ev.Ip != "203.0.113.9" {
			t.Errorf("newest security event = %+v", ev)
		}

		// The old address is nobody's now, and the new one signs in.
		sent = f.mail.count()
		if err := f.svc.RequestMagicLink(ctx, "shawn@example.com"); err != nil || f.mail.count() != sent {
			t.Fatalf("a link to the old address: %v, %d sent", err, f.mail.count()-sent)
		}
		if err := f.svc.RequestMagicLink(ctx, "newer@example.com"); err != nil {
			t.Fatal(err)
		}
		if in, err := f.svc.VerifyMagicLink(ctx, f.mail.lastToken(t), accounts.Client{}); err != nil || in.User.ID != owner.ID {
			t.Fatalf("signing in at the new address = %+v, %v", in, err)
		}
	})
}

func TestEmailChangeExpires(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		if err := f.svc.RequestEmailChange(ctx, owner.ID, "", "new@example.com"); err != nil {
			t.Fatal(err)
		}
		token := f.mail.lastToken(t)
		if _, err := d.ExecContext(ctx, rebind(d, "UPDATE email_changes SET expires_at = ?"), store.Now().Add(-1)); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.ConfirmEmailChange(ctx, token, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
	})
}

func TestEmailChangeLimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		for range 5 {
			if err := f.svc.RequestEmailChange(ctx, owner.ID, "", "new@example.com"); err != nil {
				t.Fatal(err)
			}
		}
		wantKind(t, f.svc.RequestEmailChange(ctx, owner.ID, "", "new@example.com"), accounts.Conflict)
	})
}

// An address someone else took after the link was sent stays theirs, and
// a deleted account's link does nothing.
func TestEmailChangeTakenOrDeleted(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		if err := f.svc.RequestEmailChange(ctx, owner.ID, "", "new@example.com"); err != nil {
			t.Fatal(err)
		}
		token := f.mail.lastToken(t)
		f.member(t, owner, "new@example.com", "Nell")
		_, err := f.svc.ConfirmEmailChange(ctx, token, accounts.Client{})
		wantKind(t, err, accounts.Conflict)
		if u, _ := f.st.GetUser(ctx, owner.ID); u.Email != "shawn@example.com" {
			t.Errorf("owner's address = %q", u.Email)
		}

		dana := f.member(t, owner, "dana@example.com", "Dana")
		if err := f.svc.RequestEmailChange(ctx, dana.ID, "", "dana@elsewhere.example"); err != nil {
			t.Fatal(err)
		}
		token = f.mail.lastToken(t)
		if err := f.svc.DeleteAccount(ctx, dana.ID, "dana@example.com"); err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.ConfirmEmailChange(ctx, token, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
	})
}

// An invitation works only while its sender could still send it.
func TestInviteNeedsItsSender(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		ws, err := f.svc.CreateWorkspace(ctx, owner.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		in, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner, ws.ID, "dana@example.com", authz.Admin), nil, "Dana", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		dana := in.User
		toEve := f.invite(t, dana, ws.ID, "eve@example.com", authz.Admin)
		toFay := f.invite(t, dana, ws.ID, "fay@example.com", authz.Member)

		// Dana is now a member, who cannot invite: neither invitation works.
		if err := f.svc.ChangeRole(ctx, owner.ID, ws.ID, dana.ID, authz.Member); err != nil {
			t.Fatal(err)
		}
		_, _, err = f.svc.AcceptInvite(ctx, toEve, nil, "Eve", accounts.Client{})
		wantKind(t, err, accounts.NotFound)
		_, _, err = f.svc.AcceptInvite(ctx, toFay, nil, "Fay", accounts.Client{})
		wantKind(t, err, accounts.NotFound)

		// Made an admin again, and then removed.
		if err := f.svc.ChangeRole(ctx, owner.ID, ws.ID, dana.ID, authz.Admin); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.svc.AcceptInvite(ctx, toFay, nil, "Fay", accounts.Client{}); err != nil {
			t.Fatalf("an invitation whose sender can send it again = %v", err)
		}
		if err := f.svc.RemoveMember(ctx, owner.ID, ws.ID, dana.ID); err != nil {
			t.Fatal(err)
		}
		_, _, err = f.svc.AcceptInvite(ctx, toEve, nil, "Eve", accounts.Client{})
		wantKind(t, err, accounts.NotFound)
	})
}

func TestPendingInviteCap(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		ws, err := f.svc.CreateWorkspace(ctx, owner.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		for i := range 50 {
			if _, err := f.svc.Invite(ctx, owner, ws.ID, fmt.Sprintf("p%d@example.com", i), authz.Member); err != nil {
				t.Fatal(err)
			}
		}
		_, err = f.svc.Invite(ctx, owner, ws.ID, "one-more@example.com", authz.Member)
		wantKind(t, err, accounts.Conflict)
	})
}

// What someone was told about a workspace goes when they leave it.
func TestRemovalTakesNotifications(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		ws, err := f.svc.CreateWorkspace(ctx, owner.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		in, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner, ws.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		q := "INSERT INTO notifications (id, user_id, workspace_id, kind, title, body, in_app, created_at) VALUES ('n1', ?, ?, 'mention', 'Secret plans', 'the code is 4321', TRUE, ?)"
		count := "SELECT COUNT(*) FROM notifications WHERE user_id = ?"
		if d.Dialect == db.Postgres {
			q = "INSERT INTO notifications (id, user_id, workspace_id, kind, title, body, in_app, created_at) VALUES ('n1', $1, $2, 'mention', 'Secret plans', 'the code is 4321', TRUE, $3)"
			count = "SELECT COUNT(*) FROM notifications WHERE user_id = $1"
		}
		if _, err := d.ExecContext(ctx, q, in.User.ID, ws.ID, store.Now()); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RemoveMember(ctx, owner.ID, ws.ID, in.User.ID); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := d.QueryRowContext(ctx, count, in.User.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("Dana's notifications after leaving = %d, %v", n, err)
		}
	})
}
