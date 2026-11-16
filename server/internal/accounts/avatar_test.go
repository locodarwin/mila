package accounts_test

import (
	"context"
	"slices"
	"testing"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// takePeople returns the users told of a change in people so far, sorted,
// and forgets them.
func (r *recorder) takePeople() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.people)
	slices.Sort(out)
	r.people = nil
	return out
}

func sorted(ids ...string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func TestAvatars(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		rec := &recorder{}
		f.svc.SetEvents(rec)
		f.svc.SetContent(rec)
		owner := f.setup(t)
		join := func(from store.User, ws, email, name string, role authz.Role) store.User {
			t.Helper()
			in, _, err := f.svc.AcceptInvite(ctx, f.invite(t, from, ws, email, role), nil, name, accounts.Client{UserAgent: "test"})
			if err != nil {
				t.Fatal(err)
			}
			return in.User
		}
		team, _ := f.svc.CreateWorkspace(ctx, owner.User.ID, "Team")
		dana := join(owner.User, team.ID, "dana@example.com", "Dana", authz.Member)
		gil := join(owner.User, team.ID, "gil@example.com", "Gil", authz.Guest)
		eli := join(owner.User, team.ID, "eli@example.com", "Eli", authz.Member)
		co, _ := f.svc.CreateWorkspace(ctx, dana.ID, "Dana Co")
		sam := join(dana, co.ID, "sam@example.com", "Sam", authz.Member)
		if err := f.svc.RemoveMember(ctx, owner.User.ID, team.ID, eli.ID); err != nil {
			t.Fatal(err)
		}
		rec.takePeople()

		// Setting a picture tells everyone Dana shares a workspace with,
		// and herself; Eli, who left, is not told.
		u, err := f.svc.SetAvatar(ctx, dana.ID, "0123456789abcdef0123456789abcdef")
		if err != nil || u.Avatar.String != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("SetAvatar = %+v, %v", u.Avatar, err)
		}
		if got, want := rec.takePeople(), sorted(owner.User.ID, dana.ID, gil.ID, sam.ID); !slices.Equal(got, want) {
			t.Errorf("told %v, want %v", got, want)
		}
		u, err = f.svc.SetAvatar(ctx, dana.ID, "")
		if err != nil || u.Avatar.Valid {
			t.Fatalf("clearing = %+v, %v", u.Avatar, err)
		}
		rec.takePeople()

		// Changing nothing tells nobody: removing a picture already gone,
		// and renaming to the same name.
		if _, err := f.svc.SetAvatar(ctx, dana.ID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Rename(ctx, dana.ID, " Dana "); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.RemoveAvatarAsAdmin(ctx, owner.User.ID, dana.ID); err != nil {
			t.Fatal(err)
		}
		if got := rec.takePeople(); len(got) != 0 {
			t.Errorf("a change of nothing told %v", got)
		}

		keys := map[string]string{}
		for i, p := range []store.User{owner.User, dana, gil, eli, sam} {
			k := string(rune('a'+i)) + "0000000000000000000000000000000"
			keys[p.ID] = k
			if _, err := f.svc.SetAvatar(ctx, p.ID, k); err != nil {
				t.Fatal(err)
			}
		}
		see := func(viewerID, ownerID string) bool {
			t.Helper()
			viewer, err := f.st.GetUser(ctx, viewerID)
			if err != nil {
				t.Fatal(err)
			}
			ok, err := f.svc.AvatarVisible(ctx, viewer, keys[ownerID])
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		for _, c := range []struct {
			name           string
			viewer, holder string
			want           bool
		}{
			{"self", sam.ID, sam.ID, true},
			{"co-member", dana.ID, owner.User.ID, true},
			{"guest co-member", gil.ID, dana.ID, true},
			{"a member sees a guest", dana.ID, gil.ID, true},
			{"stranger", gil.ID, sam.ID, false},
			{"stranger the other way", sam.ID, gil.ID, false},
			{"someone who left", dana.ID, eli.ID, false},
			{"to someone who left", eli.ID, dana.ID, false},
			{"instance admin", owner.User.ID, sam.ID, true},
		} {
			if got := see(c.viewer, c.holder); got != c.want {
				t.Errorf("%s: visible = %v, want %v", c.name, got, c.want)
			}
		}
		if ok, _ := f.svc.AvatarVisible(ctx, owner.User, "ffffffffffffffffffffffffffffffff"); ok {
			t.Error("a key nobody holds is visible")
		}

		// An admin sees everyone's; once demoted, only those they share
		// a workspace with.
		if _, err := f.svc.SetInstanceAdmin(ctx, owner.User.ID, gil.ID, true, accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		if !see(gil.ID, sam.ID) {
			t.Error("an admin cannot see a stranger's picture")
		}
		if _, err := f.svc.SetInstanceAdmin(ctx, owner.User.ID, gil.ID, false, accounts.Client{}); err != nil {
			t.Fatal(err)
		}
		if see(gil.ID, sam.ID) {
			t.Error("a demoted admin still sees a stranger's picture")
		}

		// A deleted shared workspace shares nothing.
		if !see(sam.ID, dana.ID) {
			t.Fatal("Sam cannot see Dana's picture to begin with")
		}
		if err := f.svc.DeleteWorkspace(ctx, dana.ID, co.ID); err != nil {
			t.Fatal(err)
		}
		if see(sam.ID, dana.ID) {
			t.Error("a deleted workspace still shares pictures")
		}

		// A deleted account's picture is gone.
		if err := f.svc.DeleteAccount(ctx, sam.ID, "sam@example.com"); err != nil {
			t.Fatal(err)
		}
		if see(owner.User.ID, sam.ID) {
			t.Error("a deleted account's picture is visible")
		}
		if gone, _ := f.st.GetUser(ctx, sam.ID); gone.Avatar.Valid {
			t.Error("deleting the account kept the picture")
		}

		// Renaming tells the same people a picture change would.
		rec.takePeople()
		if _, err := f.svc.Rename(ctx, gil.ID, "Gilbert"); err != nil {
			t.Fatal(err)
		}
		if got, want := rec.takePeople(), sorted(owner.User.ID, dana.ID, gil.ID); !slices.Equal(got, want) {
			t.Errorf("a rename told %v, want %v", got, want)
		}

		// Only an admin takes someone's picture down.
		_, err = f.svc.RemoveAvatarAsAdmin(ctx, dana.ID, gil.ID)
		wantKind(t, err, accounts.Forbidden)
		au, err := f.svc.RemoveAvatarAsAdmin(ctx, owner.User.ID, gil.ID)
		if err != nil || au.ID != gil.ID || au.Avatar.Valid {
			t.Fatalf("RemoveAvatarAsAdmin = %+v, %v", au, err)
		}
		if got, want := rec.takePeople(), sorted(owner.User.ID, dana.ID, gil.ID); !slices.Equal(got, want) {
			t.Errorf("a removal told %v, want %v", got, want)
		}
		_, err = f.svc.RemoveAvatarAsAdmin(ctx, owner.User.ID, sam.ID)
		wantKind(t, err, accounts.NotFound)
	})
}
