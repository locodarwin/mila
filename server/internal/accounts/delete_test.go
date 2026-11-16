package accounts_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// recorder hears account events and what deletion asks of sync.
type recorder struct {
	mu        sync.Mutex
	ended     []string
	changed   []string
	forgotten []string // workspace IDs
	access    []string // workspace/user
	people    []string // users told of a change in people
	onForget  func()   // runs while deletion takes content out, between its checks
}

func (r *recorder) SessionEnded(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, id)
}

func (r *recorder) MembershipChanged(ids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changed = append(r.changed, ids...)
}

func (r *recorder) SettingsChanged(string)       {}
func (r *recorder) ThemesChanged(string, string) {}

func (r *recorder) PeopleChanged(ids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.people = append(r.people, ids...)
}

func (r *recorder) AccessChanged(workspaceID, userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.access = append(r.access, workspaceID+"/"+userID)
}

func (r *recorder) Forget(_ context.Context, workspaceID, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forgotten = append(r.forgotten, workspaceID)
	if r.onForget != nil {
		r.onForget()
	}
	return nil
}

func TestDeleteAccount(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		rec := &recorder{}
		f.svc.SetEvents(rec)
		f.svc.SetContent(rec)
		owner := f.setup(t)
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		dana, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{UserAgent: "laptop"})
		if err != nil {
			t.Fatal(err)
		}
		solo, _ := f.svc.CreateWorkspace(ctx, dana.User.ID, "Solo")
		co, _ := f.svc.CreateWorkspace(ctx, dana.User.ID, "Dana Co")
		eli, _, err := f.svc.AcceptInvite(ctx, f.invite(t, dana.User, co.ID, "eli@example.com", authz.Member), nil, "Eli", accounts.Client{UserAgent: "phone"})
		if err != nil {
			t.Fatal(err)
		}
		personal, _ := f.svc.Workspaces(ctx, dana.User.ID)

		// Typing the wrong address, being the only admin, and leaving a
		// workspace others use with no owner are all refused.
		wantKind(t, f.svc.DeleteAccount(ctx, dana.User.ID, "dan@example.com"), accounts.Invalid)
		wantKind(t, f.svc.DeleteAccount(ctx, owner.User.ID, owner.User.Email), accounts.Conflict)
		err = f.svc.DeleteAccount(ctx, dana.User.ID, "DANA@example.com ")
		wantKind(t, err, accounts.Conflict)
		if !strings.Contains(err.Error(), "Dana Co") {
			t.Errorf("the refusal does not name the workspace: %v", err)
		}
		if len(rec.forgotten) != 0 {
			t.Fatal("a refused deletion changed content")
		}

		if err := f.svc.ChangeRole(ctx, dana.User.ID, co.ID, eli.User.ID, authz.Owner); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.SetAvatar(ctx, dana.User.ID, "0123456789abcdef0123456789abcdef"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.SaveTheme(ctx, dana.User.ID, testThemeID(1), testTheme(testThemeID(1), "Dusk")); err != nil {
			t.Fatal(err)
		}
		rec.changed = nil
		if err := f.svc.DeleteAccount(ctx, dana.User.ID, "dana@example.com"); err != nil {
			t.Fatal(err)
		}

		gone, err := f.st.GetUser(ctx, dana.User.ID)
		if err != nil || gone.Name != accounts.DeletedName || !gone.DeletedAt.Valid || gone.Email == "dana@example.com" || gone.Settings != "{}" || gone.Avatar.Valid {
			t.Fatalf("the user row after deletion: %+v, %v", gone, err)
		}
		if _, err := f.svc.Authenticate(ctx, dana.SessionToken, ""); accounts.KindOf(err) != accounts.Unauthenticated {
			t.Errorf("their session still works: %v", err)
		}
		if len(rec.ended) != 1 {
			t.Errorf("sessions ended: %v", rec.ended)
		}
		// Her security log, the browsers she used and her themes go with her.
		for _, table := range []string{"security_events", "known_devices", "user_themes"} {
			var n int
			if err := d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?"), dana.User.ID).Scan(&n); err != nil || n != 0 {
				t.Errorf("%s left: %d, %v", table, n, err)
			}
		}
		// Their own workspaces are gone for good; shared ones stay without them.
		for _, ws := range []string{personal[0].ID, solo.ID} {
			if _, err := f.st.GetAnyWorkspace(ctx, ws); err == nil {
				t.Errorf("workspace %s is still there", ws)
			}
		}
		for _, ws := range []string{team.ID, co.ID} {
			ids, _ := f.st.ListMemberIDs(ctx, ws)
			if slices.Contains(ids, dana.User.ID) || len(ids) != 1 {
				t.Errorf("members of %s: %v", ws, ids)
			}
		}
		slices.Sort(rec.forgotten)
		want := []string{team.ID, co.ID}
		slices.Sort(want)
		if !slices.Equal(rec.forgotten, want) {
			t.Errorf("content forgotten in %v, want %v", rec.forgotten, want)
		}
		for _, id := range []string{owner.User.ID, eli.User.ID, dana.User.ID} {
			if !slices.Contains(rec.changed, id) {
				t.Errorf("%s was not told their workspaces changed", id)
			}
		}
		// Others can be invited with the address again, as someone new.
		if _, err := f.st.GetUserByEmail(ctx, "dana@example.com"); err == nil {
			t.Error("the address still names the deleted account")
		}
		wantKind(t, f.svc.DeleteAccount(ctx, dana.User.ID, "dana@example.com"), accounts.NotFound)

		// Once someone else is an admin, the first admin may go too.
		if _, err := d.ExecContext(ctx, rebind(d, "UPDATE users SET is_instance_admin = TRUE WHERE id = ?"), eli.User.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeleteAccount(ctx, owner.User.ID, owner.User.Email); err != nil {
			t.Fatalf("the owner, alone in Team, could not leave: %v", err)
		}
		if _, err := f.st.GetAnyWorkspace(ctx, team.ID); err == nil {
			t.Error("a workspace left with nobody in it stayed")
		}
	})
}

// A shared workspace its only owner deleted, with others still in it,
// cannot be reached to hand over: it goes with the account instead of
// blocking it for good.
func TestDeleteAccountWithDeletedWorkspace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		alice, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "alice@example.com", authz.Member), nil, "Alice", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		old, _ := f.svc.CreateWorkspace(ctx, alice.User.ID, "Old team")
		if _, _, err := f.svc.AcceptInvite(ctx, f.invite(t, alice.User, old.ID, "eli@example.com", authz.Member), nil, "Eli", accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeleteWorkspace(ctx, alice.User.ID, old.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeleteAccount(ctx, alice.User.ID, "alice@example.com"); err != nil {
			t.Fatalf("a deleted workspace blocked deleting the account: %v", err)
		}
		if _, err := f.st.GetAnyWorkspace(ctx, old.ID); err == nil {
			t.Error("the deleted workspace is still there")
		}
	})
}

// The last-admin and last-owner checks hold for someone leaving at the
// same moment: they are made again inside deletion's transaction.
func TestDeleteAccountChecksAgain(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		rec := &recorder{}
		f.svc.SetEvents(rec)
		f.svc.SetContent(rec)
		owner := f.setup(t)
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		dana, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		exec := func(q string, args ...any) {
			t.Helper()
			if _, err := d.ExecContext(ctx, rebind(d, q), args...); err != nil {
				t.Fatal(err)
			}
		}
		// The owner, the first admin, deletes his account while Dana, the
		// other admin, gives up her rights.
		exec("UPDATE users SET is_instance_admin = TRUE WHERE id = ?", dana.User.ID)
		if err := f.svc.ChangeRole(ctx, owner.User.ID, team.ID, dana.User.ID, authz.Owner); err != nil {
			t.Fatal(err)
		}
		rec.onForget = func() { exec("UPDATE users SET is_instance_admin = FALSE WHERE id = ?", dana.User.ID) }
		err = f.svc.DeleteAccount(ctx, owner.User.ID, owner.User.Email)
		wantKind(t, err, accounts.Conflict)
		if !strings.Contains(err.Error(), "only admin") {
			t.Errorf("refused for %v", err)
		}
		// Dana is an admin again, and stops being an owner meanwhile.
		exec("UPDATE users SET is_instance_admin = TRUE WHERE id = ?", dana.User.ID)
		rec.onForget = func() { exec("UPDATE memberships SET role = 'member' WHERE user_id = ?", dana.User.ID) }
		wantKind(t, f.svc.DeleteAccount(ctx, owner.User.ID, owner.User.Email), accounts.Conflict)
		if gone, _ := f.st.GetUser(ctx, owner.User.ID); gone.DeletedAt.Valid {
			t.Fatal("a refused deletion went ahead")
		}
	})
}

func TestFormerMembers(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t)
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		dana, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, team.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, rebind(d, `INSERT INTO tasks (id, workspace_id, title, sort_key, clocks, created_by, created_at, updated_at)
			VALUES ('t1', ?, 'Plan', 'a0', '{}', ?, ?, ?)`), team.ID, dana.User.ID, "2026-10-28T09:00:00Z", "2026-10-28T09:00:00Z"); err != nil {
			t.Fatal(err)
		}
		if list, _ := f.svc.FormerMembers(ctx, owner.User.ID, team.ID); len(list) != 0 {
			t.Fatalf("a member is listed as former: %+v", list)
		}
		// What the owner was told of her keeps her name and address in its
		// text; deleting her takes both out.
		notice := func(id, kind, title, body string) {
			t.Helper()
			if _, err := d.ExecContext(ctx, rebind(d, `INSERT INTO notifications (id, user_id, workspace_id, kind, actor_id, title, body, in_app, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, TRUE, ?)`), id, owner.User.ID, team.ID, kind, dana.User.ID, title, body, "2026-10-28T09:00:00Z"); err != nil {
				t.Fatal(err)
			}
		}
		notice("n1", "invite_accepted", "Dana joined Team", "Dana (dana@example.com) accepted your invitation.")
		notice("n2", "assigned", "Dana assigned you to Plan", "In the inbox")
		if err := f.svc.DeleteAccount(ctx, dana.User.ID, "dana@example.com"); err != nil {
			t.Fatal(err)
		}
		for id, want := range map[string]string{
			"n1": "Deleted user joined Team / Deleted user accepted your invitation.",
			"n2": "Deleted user assigned you to Plan / In the inbox",
		} {
			var title, body string
			if err := d.QueryRowContext(ctx, rebind(d, "SELECT title, body FROM notifications WHERE id = ?"), id).Scan(&title, &body); err != nil {
				t.Fatal(err)
			}
			if got := title + " / " + body; got != want {
				t.Errorf("notification %s reads %q, want %q", id, got, want)
			}
		}
		list, err := f.svc.FormerMembers(ctx, owner.User.ID, team.ID)
		if err != nil || len(list) != 1 || list[0].Name != accounts.DeletedName || !list[0].DeletedAt.Valid {
			t.Fatalf("former members: %+v, %v", list, err)
		}
		_, err = f.svc.FormerMembers(ctx, dana.User.ID, team.ID)
		wantKind(t, err, accounts.NotFound)
	})
}
