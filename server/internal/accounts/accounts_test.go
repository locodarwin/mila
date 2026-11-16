package accounts_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
)

type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, m)
	return nil
}

var linkRE = regexp.MustCompile(`https?://\S+`)

// lastToken returns the token from the newest message's link.
func (o *outbox) lastToken(t *testing.T) string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.sent) == 0 {
		t.Fatal("no email sent")
	}
	link := linkRE.FindString(o.sent[len(o.sent)-1].Text)
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func (o *outbox) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.sent)
}

type fixture struct {
	svc  *accounts.Service
	st   *store.Store
	d    *db.DB
	mail *outbox
}

func newFixture(t *testing.T, d *db.DB) *fixture {
	t.Helper()
	st := store.New(d)
	box := &outbox{}
	svc, err := accounts.New(st, box, slog.New(slog.NewTextHandler(io.Discard, nil)), accounts.Config{
		BaseURL: "http://localhost:8080", Origin: "http://localhost:8080", RPID: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, st: st, d: d, mail: box}
}

// setup creates the instance owner and returns them signed in.
func (f *fixture) setup(t *testing.T) *accounts.SignedIn {
	t.Helper()
	ctx := context.Background()
	link, err := f.svc.SetupLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	in, err := f.svc.CompleteSetup(ctx, u.Query().Get("token"), "Shawn", "Shawn@Example.com", accounts.Client{UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func wantKind(t *testing.T, err error, kind accounts.Kind) {
	t.Helper()
	if got := accounts.KindOf(err); got != kind {
		t.Fatalf("got error %v (kind %d), want kind %d", err, got, kind)
	}
}

func TestSetup(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)

		_, err := f.svc.CompleteSetup(ctx, "guess", "X", "x@example.com", accounts.Client{})
		wantKind(t, err, accounts.Forbidden)

		in := f.setup(t)
		if in.User.Email != "shawn@example.com" || !in.User.IsInstanceAdmin {
			t.Errorf("owner = %+v", in.User)
		}
		ws, err := f.svc.Workspaces(ctx, in.User.ID)
		if err != nil || len(ws) != 1 || !ws[0].IsPersonal || ws[0].Role != "owner" {
			t.Errorf("personal workspace = %+v, %v", ws, err)
		}
		if link, err := f.svc.SetupLink(ctx); err != nil || link != "" {
			t.Errorf("SetupLink after setup = %q, %v", link, err)
		}
		if needed, _ := f.svc.SetupRequired(ctx); needed {
			t.Error("setup still required")
		}
	})
}

func TestSetupLinkWorksOnce(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		link, _ := f.svc.SetupLink(ctx)
		u, _ := url.Parse(link)
		token := u.Query().Get("token")
		if _, err := f.svc.CompleteSetup(ctx, token, "A", "a@example.com", accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.CompleteSetup(ctx, token, "B", "b@example.com", accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
	})
}

func TestSessions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		in := f.setup(t)

		auth, err := f.svc.Authenticate(ctx, in.SessionToken, "")
		if err != nil || auth.User.ID != in.User.ID {
			t.Fatalf("Authenticate = %+v, %v", auth, err)
		}
		_, err = f.svc.Authenticate(ctx, "forged", "")
		wantKind(t, err, accounts.Unauthenticated)

		sessions, err := f.svc.Sessions(ctx, in.User.ID)
		if err != nil || len(sessions) != 1 {
			t.Fatalf("Sessions = %v, %v", sessions, err)
		}
		if err := f.svc.RevokeSession(ctx, in.User.ID, sessions[0].ID, accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.Authenticate(ctx, in.SessionToken, "")
		wantKind(t, err, accounts.Unauthenticated)
		wantKind(t, f.svc.RevokeSession(ctx, in.User.ID, sessions[0].ID, accounts.Client{}), accounts.NotFound)
	})
}

func TestExpiredSessionIsRejected(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		in := f.setup(t)
		past := store.Now().Add(-accounts.SessionLifetime)
		if _, err := d.ExecContext(ctx, rebind(d, "UPDATE sessions SET expires_at = ?"), past); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.Authenticate(ctx, in.SessionToken, "")
		wantKind(t, err, accounts.Unauthenticated)
	})
}

func rebind(d *db.DB, q string) string {
	if d.Dialect != db.Postgres {
		return q
	}
	n := 0
	var b strings.Builder
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + string(rune('0'+n)))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestMagicLinks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)

		// Unknown addresses look the same to the caller and send nothing.
		if err := f.svc.RequestMagicLink(ctx, "nobody@example.com"); err != nil {
			t.Fatal(err)
		}
		if f.mail.count() != 0 {
			t.Fatal("mailed an address with no account")
		}
		wantKind(t, f.svc.RequestMagicLink(ctx, "not an address"), accounts.Invalid)

		if err := f.svc.RequestMagicLink(ctx, " SHAWN@example.com "); err != nil {
			t.Fatal(err)
		}
		token := f.mail.lastToken(t)
		in, err := f.svc.VerifyMagicLink(ctx, token, accounts.Client{UserAgent: "phone"})
		if err != nil || in.User.ID != owner.User.ID || in.SessionToken == "" {
			t.Fatalf("VerifyMagicLink = %+v, %v", in, err)
		}
		_, err = f.svc.VerifyMagicLink(ctx, token, accounts.Client{UserAgent: "phone"})
		wantKind(t, err, accounts.Forbidden)
	})
}

func TestMagicLinkExpires(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		f.setup(t)
		if err := f.svc.RequestMagicLink(ctx, "shawn@example.com"); err != nil {
			t.Fatal(err)
		}
		token := f.mail.lastToken(t)
		if _, err := d.ExecContext(ctx, rebind(d, "UPDATE magic_links SET expires_at = ?"), store.Now().Add(-1)); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.VerifyMagicLink(ctx, token, accounts.Client{})
		wantKind(t, err, accounts.Forbidden)
	})
}

func TestMagicLinkRateLimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		f.setup(t)
		for range 8 {
			if err := f.svc.RequestMagicLink(ctx, "shawn@example.com"); err != nil {
				t.Fatal(err)
			}
		}
		if n := f.mail.count(); n != 5 {
			t.Errorf("sent %d links in an hour, want 5", n)
		}
	})
}

func TestWorkspaces(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)

		ws, err := f.svc.CreateWorkspace(ctx, owner.User.ID, "  Home   Projects ")
		if err != nil || ws.Name != "Home Projects" || ws.IsPersonal {
			t.Fatalf("CreateWorkspace = %+v, %v", ws, err)
		}
		if !strings.HasPrefix(ws.Slug, "home-projects-") {
			t.Errorf("slug = %q", ws.Slug)
		}
		if err := f.svc.RenameWorkspace(ctx, owner.User.ID, ws.ID, "Home"); err != nil {
			t.Fatal(err)
		}
		list, _ := f.svc.Workspaces(ctx, owner.User.ID)
		if len(list) != 2 || list[1].Name != "Home" {
			t.Errorf("workspaces = %+v", list)
		}
		personal := list[0]
		wantKind(t, f.svc.DeleteWorkspace(ctx, owner.User.ID, personal.ID), accounts.Invalid)
		wantKind(t, f.svc.RenameWorkspace(ctx, owner.User.ID, "no-such-id", "x"), accounts.NotFound)
		if err := f.svc.DeleteWorkspace(ctx, owner.User.ID, ws.ID); err != nil {
			t.Fatal(err)
		}
		list, _ = f.svc.Workspaces(ctx, owner.User.ID)
		if len(list) != 1 {
			t.Errorf("deleted workspace still listed: %+v", list)
		}
	})
}

// invite sends an invitation and returns its link token.
func (f *fixture) invite(t *testing.T, from store.User, workspaceID, email string, role authz.Role) string {
	t.Helper()
	if _, err := f.svc.Invite(context.Background(), from, workspaceID, email, role); err != nil {
		t.Fatal(err)
	}
	return f.mail.lastToken(t)
}

func TestInviteNewPerson(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		ws, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")

		token := f.invite(t, owner.User, ws.ID, "dana@example.com", authz.Member)
		preview, err := f.svc.PreviewInvite(ctx, token)
		if err != nil || preview.WorkspaceName != "Team" || preview.InvitedByName != "Shawn" || preview.Email != "dana@example.com" {
			t.Fatalf("preview = %+v, %v", preview, err)
		}
		_, _, err = f.svc.AcceptInvite(ctx, token, nil, "", accounts.Client{})
		wantKind(t, err, accounts.Invalid) // a new person must give a name

		in, wsID, err := f.svc.AcceptInvite(ctx, token, nil, "Dana", accounts.Client{UserAgent: "laptop"})
		if err != nil || in.SessionToken == "" || in.User.Email != "dana@example.com" || wsID != ws.ID {
			t.Fatalf("accept = %+v %q %v", in, wsID, err)
		}
		members, _ := f.svc.Members(ctx, owner.User.ID, ws.ID)
		if len(members) != 2 || members[1].Role != "member" {
			t.Errorf("members = %+v", members)
		}
		list, _ := f.svc.Workspaces(ctx, in.User.ID)
		if len(list) != 2 {
			t.Errorf("Dana's workspaces = %+v", list)
		}
		_, _, err = f.svc.AcceptInvite(ctx, token, nil, "Dana", accounts.Client{})
		wantKind(t, err, accounts.NotFound)
	})
}

func TestInviteRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		list, _ := f.svc.Workspaces(ctx, owner.User.ID)
		_, err := f.svc.Invite(ctx, owner.User, list[0].ID, "x@example.com", authz.Member)
		wantKind(t, err, accounts.Invalid) // personal workspace

		ws, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		_, err = f.svc.Invite(ctx, owner.User, ws.ID, "shawn@example.com", authz.Member)
		wantKind(t, err, accounts.Conflict)
		_, err = f.svc.Invite(ctx, owner.User, ws.ID, "x@example.com", authz.Role("boss"))
		wantKind(t, err, accounts.Invalid)

		// A member cannot invite; an admin cannot invite an owner.
		token := f.invite(t, owner.User, ws.ID, "m@example.com", authz.Member)
		m, _, _ := f.svc.AcceptInvite(ctx, token, nil, "Mo", accounts.Client{})
		_, err = f.svc.Invite(ctx, m.User, ws.ID, "y@example.com", authz.Guest)
		wantKind(t, err, accounts.Forbidden)
		token = f.invite(t, owner.User, ws.ID, "a@example.com", authz.Admin)
		a, _, _ := f.svc.AcceptInvite(ctx, token, nil, "Ari", accounts.Client{})
		_, err = f.svc.Invite(ctx, a.User, ws.ID, "y@example.com", authz.Owner)
		wantKind(t, err, accounts.Forbidden)

		inv, err := f.svc.Invite(ctx, a.User, ws.ID, "y@example.com", authz.Guest)
		if err != nil {
			t.Fatal(err)
		}
		pending, _ := f.svc.PendingInvites(ctx, owner.User.ID, ws.ID)
		if len(pending) != 1 || pending[0].ID != inv.ID {
			t.Fatalf("pending = %+v", pending)
		}
		if err := f.svc.RevokeInvite(ctx, owner.User.ID, ws.ID, inv.ID); err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.PreviewInvite(ctx, f.mail.lastToken(t))
		wantKind(t, err, accounts.NotFound)
	})
}

func TestSignedInUserAcceptsAsThemselves(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		other, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Other")
		dana, _, _ := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{})

		token := f.invite(t, owner.User, other.ID, "dana.work@example.com", authz.Guest)
		in, _, err := f.svc.AcceptInvite(ctx, token, &dana.User, "", accounts.Client{})
		if err != nil || in.User.ID != dana.User.ID || in.SessionToken != "" {
			t.Fatalf("accept as signed-in user = %+v, %v", in, err)
		}
	})
}

func TestLastOwnerStays(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		ws, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")

		wantKind(t, f.svc.ChangeRole(ctx, owner.User.ID, ws.ID, owner.User.ID, authz.Admin), accounts.Conflict)
		wantKind(t, f.svc.RemoveMember(ctx, owner.User.ID, ws.ID, owner.User.ID), accounts.Conflict)

		b, _, _ := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, ws.ID, "b@example.com", authz.Member), nil, "Bea", accounts.Client{})
		if err := f.svc.ChangeRole(ctx, owner.User.ID, ws.ID, b.User.ID, authz.Owner); err != nil {
			t.Fatal(err)
		}
		// With a second owner, the first can step down and then leave.
		if err := f.svc.ChangeRole(ctx, owner.User.ID, ws.ID, owner.User.ID, authz.Member); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RemoveMember(ctx, owner.User.ID, ws.ID, owner.User.ID); err != nil {
			t.Fatal(err)
		}
		_, err := f.svc.Members(ctx, owner.User.ID, ws.ID)
		wantKind(t, err, accounts.NotFound)
	})
}

func TestMembersCannotManage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		ws, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		m, _, _ := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, ws.ID, "m@example.com", authz.Member), nil, "Mo", accounts.Client{})
		g, _, _ := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, ws.ID, "g@example.com", authz.Guest), nil, "Gus", accounts.Client{})

		wantKind(t, f.svc.ChangeRole(ctx, m.User.ID, ws.ID, g.User.ID, authz.Member), accounts.Forbidden)
		wantKind(t, f.svc.RemoveMember(ctx, m.User.ID, ws.ID, g.User.ID), accounts.Forbidden)
		wantKind(t, f.svc.RenameWorkspace(ctx, m.User.ID, ws.ID, "Mine"), accounts.Forbidden)
		_, err := f.svc.Members(ctx, g.User.ID, ws.ID)
		wantKind(t, err, accounts.Forbidden)
		// Anyone can leave.
		if err := f.svc.RemoveMember(ctx, g.User.ID, ws.ID, g.User.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPasskeysOffOnAnIPAddress(t *testing.T) {
	st := store.New(dbtest.Open(t))
	svc, err := accounts.New(st, &outbox{}, slog.New(slog.NewTextHandler(io.Discard, nil)), accounts.Config{
		BaseURL: "http://192.168.1.10:8080", Origin: "http://192.168.1.10:8080", RPID: "192.168.1.10",
	})
	if err != nil {
		t.Fatalf("an IP address must not stop the server: %v", err)
	}
	if svc.PasskeysAvailable() {
		t.Error("passkeys reported available on an IP address")
	}
	_, err = svc.BeginPasskeyLogin(context.Background())
	wantKind(t, err, accounts.Invalid)
}

// Two settings changes at once, to different keys, both stick: the second
// waits for the first instead of starting from the settings it replaces.
func TestConcurrentSettingsChanges(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		in := f.setup(t)

		done := make(chan error, 1)
		_, err := accounts.UpdateSettings(ctx, f.st, in.User.ID, func(s accounts.Settings) error {
			s["first"] = json.RawMessage(`1`)
			// The other change starts while this one is still open, and
			// would read the settings without "first" if nothing held it.
			go func() {
				_, err := accounts.UpdateSettings(ctx, f.st, in.User.ID, func(s accounts.Settings) error {
					s["second"] = json.RawMessage(`2`)
					return nil
				})
				done <- err
			}()
			time.Sleep(100 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		u, err := f.st.GetUser(ctx, in.User.ID)
		if err != nil {
			t.Fatal(err)
		}
		s := accounts.SettingsOf(u)
		if string(s["first"]) != "1" || string(s["second"]) != "2" {
			t.Fatalf("settings = %s", u.Settings)
		}
	})
}

// slowMail is a mail server that holds each message until released.
type slowMail struct {
	release chan struct{}
	sent    chan string
}

func (m slowMail) Send(_ context.Context, msg mail.Message) error {
	<-m.release
	m.sent <- msg.To
	return nil
}

// The server answers a sign-in link request before sending the link, so
// the answer's timing says nothing about whether the address has an
// account.
func TestSignInMailAfterAnswering(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, dbtest.Open(t))
	f.setup(t)
	m := slowMail{release: make(chan struct{}), sent: make(chan string, 1)}
	svc, err := accounts.New(f.st, m, slog.New(slog.NewTextHandler(io.Discard, nil)), accounts.Config{
		BaseURL: "http://localhost:8080", Origin: "http://localhost:8080", RPID: "localhost", SignInMailAfter: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestMagicLink(ctx, "shawn@example.com"); err != nil {
		t.Fatal(err)
	}
	close(m.release)
	select {
	case to := <-m.sent:
		if to != "shawn@example.com" {
			t.Errorf("sent to %q", to)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the link was never sent")
	}
}
