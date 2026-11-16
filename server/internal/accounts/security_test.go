package accounts_test

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// deviceNotices records the new-device sign-ins accounts reports, and
// the old addresses of email changes.
type deviceNotices struct {
	mu      sync.Mutex
	got     []accounts.SecurityEvent
	changed []string
}

func (n *deviceNotices) PasskeyAdded(context.Context, store.User, accounts.SecurityEvent) {}

func (n *deviceNotices) EmailChanged(_ context.Context, _ store.User, oldEmail string, _ accounts.SecurityEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.changed = append(n.changed, oldEmail)
}

func (n *deviceNotices) WebhooksPaused(context.Context, string)                 {}
func (n *deviceNotices) InviteAccepted(context.Context, string, string, string) {}

func (n *deviceNotices) NewDeviceSignIn(_ context.Context, _ store.User, ev accounts.SecurityEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.got = append(n.got, ev)
}

func (n *deviceNotices) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.got)
}

// signIn signs the owner in by magic link from a browser.
func (f *fixture) signIn(t *testing.T, email string, c accounts.Client) *accounts.SignedIn {
	t.Helper()
	ctx := context.Background()
	if err := f.svc.RequestMagicLink(ctx, email); err != nil {
		t.Fatal(err)
	}
	in, err := f.svc.VerifyMagicLink(ctx, f.mail.lastToken(t), c)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func kinds(t *testing.T, f *fixture, userID string) []string {
	t.Helper()
	events, _, err := f.svc.SecurityEvents(context.Background(), userID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Kind + "/" + ev.Method
		if ev.NewDevice {
			out[i] += "/new"
		}
	}
	return out
}

func TestSecurityLogAndNewDevices(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		notices := &deviceNotices{}
		f.svc.SetNotices(notices)

		// Setting up is the account's first device: no warning about it.
		link, _ := f.svc.SetupLink(ctx)
		u, _ := url.Parse(link)
		owner, err := f.svc.CompleteSetup(ctx, u.Query().Get("token"), "Shawn", "shawn@example.com",
			accounts.Client{UserAgent: "laptop", IP: "203.0.113.5"})
		if err != nil {
			t.Fatal(err)
		}
		if owner.DeviceToken == "" || notices.count() != 0 {
			t.Fatalf("setup: device %q, %d notices", owner.DeviceToken, notices.count())
		}
		laptop := accounts.Client{UserAgent: "laptop", IP: "203.0.113.5", Device: owner.DeviceToken}

		// The same browser again is known.
		again := f.signIn(t, "shawn@example.com", laptop)
		if again.DeviceToken != owner.DeviceToken || notices.count() != 0 {
			t.Fatalf("same browser: device %q, %d notices", again.DeviceToken, notices.count())
		}
		// A browser with no device cookie, or a made-up one, is new.
		phone := f.signIn(t, "shawn@example.com", accounts.Client{UserAgent: "phone", IP: "198.51.100.7", Device: "bad"})
		if phone.DeviceToken == "" || phone.DeviceToken == "bad" || notices.count() != 1 {
			t.Fatalf("new browser: device %q, %d notices", phone.DeviceToken, notices.count())
		}
		if ev := notices.got[0]; ev.UserAgent != "phone" || ev.Ip != "198.51.100.7" || ev.Method != accounts.MethodMagicLink {
			t.Fatalf("notice = %+v", ev)
		}

		// Each session remembers its address, and follows it as it moves.
		auth, err := f.svc.Authenticate(ctx, phone.SessionToken, "198.51.100.8")
		if err != nil || auth.Session.Ip != "198.51.100.8" {
			t.Fatalf("Authenticate = %+v, %v", auth, err)
		}
		sessions, err := f.svc.Sessions(ctx, owner.User.ID)
		if err != nil || len(sessions) != 3 {
			t.Fatalf("Sessions = %v, %v", sessions, err)
		}
		for _, s := range sessions {
			if s.UserAgent == "phone" && s.Ip != "198.51.100.8" || s.UserAgent == "laptop" && s.Ip != "203.0.113.5" {
				t.Fatalf("session %s from %q", s.UserAgent, s.Ip)
			}
		}
		for _, s := range sessions {
			if s.UserAgent == "phone" {
				if err := f.svc.RevokeSession(ctx, owner.User.ID, s.ID, laptop); err != nil {
					t.Fatal(err)
				}
			}
		}

		// Someone joining by invite starts on their first device too, and
		// their log says what they joined.
		team, err := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		dana, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member), nil, "Dana",
			accounts.Client{UserAgent: "tablet"})
		if err != nil {
			t.Fatal(err)
		}
		if notices.count() != 1 {
			t.Fatal("warned about a new account's first device")
		}

		want := []string{"session_revoked/self", "sign_in/magic_link/new", "sign_in/magic_link", "sign_in/setup"}
		if got := kinds(t, f, owner.User.ID); !equal(got, want) {
			t.Fatalf("owner's log = %v, want %v", got, want)
		}
		events, _, _ := f.svc.SecurityEvents(ctx, owner.User.ID, "", 1)
		if events[0].Detail != "phone" || events[0].UserAgent != "laptop" {
			t.Fatalf("sign-out event = %+v", events[0])
		}
		if got := kinds(t, f, dana.User.ID); !equal(got, []string{"sign_in/invite", "invite_accepted/"}) {
			t.Fatalf("Dana's log = %v", got)
		}

		// Paged, newest first.
		page, more, err := f.svc.SecurityEvents(ctx, owner.User.ID, "", 3)
		if err != nil || len(page) != 3 || !more {
			t.Fatalf("first page = %d, more %v, %v", len(page), more, err)
		}
		rest, more, err := f.svc.SecurityEvents(ctx, owner.User.ID, page[2].ID, 3)
		if err != nil || len(rest) != 1 || more || rest[0].Method != accounts.MethodSetup {
			t.Fatalf("second page = %+v, more %v, %v", rest, more, err)
		}
	})
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTidy(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		in := f.signIn(t, "shawn@example.com", accounts.Client{})
		if err := f.svc.SignOut(ctx, in.SessionToken); err != nil {
			t.Fatal(err)
		}
		team, err := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member)

		// Nothing is old enough yet.
		got, err := f.svc.Tidy(ctx, store.Now())
		if err != nil || got != (accounts.Tidied{}) {
			t.Fatalf("Tidy now = %+v, %v", got, err)
		}
		// Two years on, everything has ended: the signed-out session at
		// once past 30 days, the live one since it expired, the sign-in
		// link, the invitation, both sign-ins in the log and the device.
		got, err = f.svc.Tidy(ctx, store.Now().Add(2*365*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		want := accounts.Tidied{Sessions: 2, MagicLinks: 1, Invites: 1, Events: 2, Devices: 2}
		if got != want {
			t.Fatalf("Tidy later = %+v, want %+v", got, want)
		}
	})
}

func TestInstanceAdmins(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		rec := &recorder{}
		f.svc.SetEvents(rec)
		owner := f.setup(t)
		team, err := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		dana, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Admin), nil, "Dana", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		f.invite(t, owner.User, team.ID, "eli@example.com", authz.Member)

		// A workspace admin is no instance admin.
		for _, err := range []error{
			f.svc.InstanceAccess(ctx, dana.User.ID, authz.ViewInstance),
			f.svc.InstanceAccess(ctx, dana.User.ID, authz.BackUpInstance),
			second(f.svc.Users(ctx, dana.User.ID)),
			second(f.svc.SetInstanceAdmin(ctx, dana.User.ID, dana.User.ID, true, accounts.Client{})),
			second(f.svc.SignOutEverywhere(ctx, dana.User.ID, owner.User.ID, accounts.Client{})),
			second(f.svc.AllInvites(ctx, dana.User.ID)),
			f.svc.CancelInvite(ctx, dana.User.ID, uuid.NewString()),
			second(f.svc.CountUsers(ctx, dana.User.ID)),
		} {
			wantKind(t, err, accounts.Forbidden)
		}
		if err := f.svc.InstanceAccess(ctx, owner.User.ID, authz.InstanceAction("instance.nope")); accounts.KindOf(err) != accounts.Forbidden {
			t.Fatalf("an unknown action = %v", err)
		}

		users, err := f.svc.Users(ctx, owner.User.ID)
		if err != nil || len(users) != 2 || users[0].Name != "Dana" || users[1].Devices != 1 || users[1].LastActiveAt == nil {
			t.Fatalf("Users = %+v, %v", users, err)
		}

		// The last admin cannot step down; with a second one, they can.
		_, err = f.svc.SetInstanceAdmin(ctx, owner.User.ID, owner.User.ID, false, accounts.Client{})
		wantKind(t, err, accounts.Conflict)
		got, err := f.svc.SetInstanceAdmin(ctx, owner.User.ID, dana.User.ID, true, accounts.Client{IP: "192.0.2.1"})
		if err != nil || !got.IsInstanceAdmin || got.ID != dana.User.ID {
			t.Fatalf("SetInstanceAdmin = %+v, %v", got, err)
		}
		got, err = f.svc.SetInstanceAdmin(ctx, owner.User.ID, owner.User.ID, false, accounts.Client{})
		if err != nil || got.IsInstanceAdmin {
			t.Fatalf("stepping down = %+v, %v", got, err)
		}
		wantKind(t, f.svc.InstanceAccess(ctx, owner.User.ID, authz.ViewInstance), accounts.Forbidden)
		_, err = f.svc.SetInstanceAdmin(ctx, dana.User.ID, dana.User.ID, false, accounts.Client{})
		wantKind(t, err, accounts.Conflict)
		if got := kinds(t, f, dana.User.ID); got[0] != "admin_granted/admin" {
			t.Fatalf("Dana's log = %v", got)
		}
		if got := kinds(t, f, owner.User.ID); got[0] != "admin_revoked/admin" {
			t.Fatalf("owner's log = %v", got)
		}

		// The rescue at the server needs no rights.
		u, err := f.svc.MakeAdmin(ctx, "Shawn@example.com")
		if err != nil || !u.IsInstanceAdmin {
			t.Fatalf("MakeAdmin = %+v, %v", u, err)
		}
		_, err = f.svc.MakeAdmin(ctx, "nobody@example.com")
		wantKind(t, err, accounts.NotFound)

		// Signing someone out everywhere ends every session of theirs.
		n, err := f.svc.SignOutEverywhere(ctx, owner.User.ID, dana.User.ID, accounts.Client{})
		if err != nil || n != 1 {
			t.Fatalf("SignOutEverywhere = %d, %v", n, err)
		}
		if _, err := f.svc.Authenticate(ctx, dana.SessionToken, ""); accounts.KindOf(err) != accounts.Unauthenticated {
			t.Fatalf("Dana is still signed in: %v", err)
		}
		if len(rec.ended) != 1 {
			t.Fatalf("sockets closed: %v", rec.ended)
		}
		if got := kinds(t, f, dana.User.ID); got[0] != "session_revoked/admin" {
			t.Fatalf("Dana's log = %v", got)
		}

		// Every open invitation, from any workspace, and canceling one.
		invites, err := f.svc.AllInvites(ctx, owner.User.ID)
		if err != nil || len(invites) != 1 || invites[0].Email != "eli@example.com" || invites[0].WorkspaceName != "Team" || invites[0].InvitedByName != "Shawn" {
			t.Fatalf("AllInvites = %+v, %v", invites, err)
		}
		if err := f.svc.CancelInvite(ctx, owner.User.ID, invites[0].ID); err != nil {
			t.Fatal(err)
		}
		wantKind(t, f.svc.CancelInvite(ctx, owner.User.ID, invites[0].ID), accounts.NotFound)
		if invites, _ := f.svc.AllInvites(ctx, owner.User.ID); len(invites) != 0 {
			t.Fatalf("still open: %+v", invites)
		}
	})
}

func second[T any](_ T, err error) error { return err }

func TestShareProject(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		rec := &recorder{}
		f.svc.SetEvents(rec)
		owner := f.setup(t)
		team, err := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		if err != nil {
			t.Fatal(err)
		}
		join := func(email, name string, role authz.Role) string {
			in, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, email, role), nil, name, accounts.Client{})
			if err != nil {
				t.Fatal(err)
			}
			return in.User.ID
		}
		gus := join("gus@example.com", "Gus", authz.Guest)
		mo := join("mo@example.com", "Mo", authz.Member)
		rec.mu.Lock()
		joined := rec.access
		rec.access = nil
		rec.mu.Unlock()
		if want := []string{team.ID + "/" + gus, team.ID + "/" + mo}; !equal(joined, want) {
			t.Fatalf("access changes told on joining = %v", joined)
		}
		project := uuid.NewString()
		now := store.Now()
		if _, err := d.ExecContext(ctx, rebind(d, "INSERT INTO projects (id, workspace_id, name, sort_key, clocks, created_at, updated_at) VALUES (?, ?, 'Garden', 'a0', '{}', ?, ?)"),
			project, team.ID, now, now); err != nil {
			t.Fatal(err)
		}
		seq := func() int64 {
			n, err := f.st.GetChangeSeq(ctx, team.ID)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		resetOf := func(user string) int64 {
			var n int64
			if err := d.QueryRowContext(ctx, rebind(d, "SELECT reset_seq FROM memberships WHERE workspace_id = ? AND user_id = ?"), team.ID, user).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}

		// Only admins share, only with guests, only projects of the
		// workspace.
		wantKind(t, f.svc.ShareProject(ctx, mo, team.ID, project, gus, true), accounts.Forbidden)
		_, err = f.svc.ProjectGuests(ctx, mo, team.ID, project)
		wantKind(t, err, accounts.Forbidden)
		wantKind(t, f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, mo, true), accounts.Invalid)
		wantKind(t, f.svc.ShareProject(ctx, owner.User.ID, team.ID, uuid.NewString(), gus, true), accounts.NotFound)
		wantKind(t, f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, uuid.NewString(), true), accounts.NotFound)

		// Joining resets too, so a device of someone who was here before
		// does not keep what it held then.
		before := seq()
		moJoined := resetOf(mo)
		if moJoined == 0 || moJoined != before || resetOf(gus) != before-1 {
			t.Fatalf("joining: seq %d, Gus reset at %d, Mo at %d", before, resetOf(gus), moJoined)
		}
		if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, gus, true); err != nil {
			t.Fatal(err)
		}
		if seq() != before+1 || resetOf(gus) != before+1 || resetOf(mo) != moJoined {
			t.Fatalf("sharing: seq %d, Gus reset at %d, Mo at %d", seq(), resetOf(gus), resetOf(mo))
		}
		guests, err := f.svc.ProjectGuests(ctx, owner.User.ID, team.ID, project)
		if err != nil || len(guests) != 1 || guests[0].ID != gus || !guests[0].SharedAt.Valid {
			t.Fatalf("ProjectGuests = %+v, %v", guests, err)
		}
		// Sharing it again changes nothing, and resets nobody.
		if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, gus, true); err != nil || seq() != before+1 {
			t.Fatalf("sharing twice: %v, seq %d", err, seq())
		}
		if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, gus, false); err != nil {
			t.Fatal(err)
		}
		if resetOf(gus) != before+2 {
			t.Fatalf("revoking: Gus reset at %d", resetOf(gus))
		}
		if guests, _ := f.svc.ProjectGuests(ctx, owner.User.ID, team.ID, project); guests[0].SharedAt.Valid {
			t.Fatal("still shared")
		}
		if want := []string{team.ID + "/" + gus, team.ID + "/" + gus}; !equal(rec.access, want) {
			t.Fatalf("access changes told = %v", rec.access)
		}

		// A role to or from guest resets too; one between members does not.
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, mo, authz.Guest); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, mo, authz.Member); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, mo, authz.Admin); err != nil {
			t.Fatal(err)
		}
		if resetOf(mo) != before+4 || len(rec.access) != 4 {
			t.Fatalf("role changes: Mo reset at %d, told %v", resetOf(mo), rec.access)
		}

		// A guest who leaves takes no projects along.
		if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, gus, true); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.RemoveMember(ctx, owner.User.ID, team.ID, gus); err != nil {
			t.Fatal(err)
		}
		shared := func() int {
			var n int
			if err := d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM project_access WHERE user_id = ?"), gus).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := shared(); n != 0 {
			t.Fatalf("access left behind: %d", n)
		}

		// Invited back, Gus starts afresh: a device that never heard he
		// left is reset rather than kept.
		left := seq()
		if join("gus@example.com", "Gus", authz.Guest) != gus {
			t.Fatal("Gus came back as someone else")
		}
		if resetOf(gus) != left+1 || seq() != left+1 {
			t.Fatalf("rejoining: Gus reset at %d, seq %d, left at %d", resetOf(gus), seq(), left)
		}

		// A guest who becomes a member keeps no projects of their own, so
		// made a guest again, they are given none they had before.
		if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, project, gus, true); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, gus, authz.Member); err != nil {
			t.Fatal(err)
		}
		if n := shared(); n != 0 {
			t.Fatalf("a member keeps %d projects of their own", n)
		}
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, gus, authz.Guest); err != nil {
			t.Fatal(err)
		}
		if guests, _ := f.svc.ProjectGuests(ctx, owner.User.ID, team.ID, project); len(guests) != 1 || guests[0].SharedAt.Valid {
			t.Fatalf("a guest again: %+v", guests)
		}
	})
}
