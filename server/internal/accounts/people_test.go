package accounts_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// Who a guest can know of: the members, and the guests they share a project
// with. Members see everyone, and which projects each guest is given.
func TestPeople(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
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
		mo := join("mo@example.com", "Mo", authz.Member)
		gus := join("gus@example.com", "Gus", authz.Guest)
		gia := join("gia@example.com", "Gia", authz.Guest)
		gil := join("gil@example.com", "Gil", authz.Guest)
		lone := join("lone@example.com", "Lone", authz.Guest)

		project := func() string {
			id := uuid.NewString()
			now := store.Now()
			if _, err := d.ExecContext(ctx, rebind(d, "INSERT INTO projects (id, workspace_id, name, sort_key, clocks, created_at, updated_at) VALUES (?, ?, 'P', 'a0', '{}', ?, ?)"),
				id, team.ID, now, now); err != nil {
				t.Fatal(err)
			}
			return id
		}
		share := func(p, guest string) {
			if err := f.svc.ShareProject(ctx, owner.User.ID, team.ID, p, guest, true); err != nil {
				t.Fatal(err)
			}
		}
		both, gusOnly, giaOnly := project(), project(), project()
		share(both, gus)
		share(both, gia)
		share(gusOnly, gus)
		share(giaOnly, gia)
		share(giaOnly, gil)

		names := func(list []accounts.Person) []string {
			out := []string{}
			for _, p := range list {
				out = append(out, p.Name)
			}
			slices.Sort(out)
			return out
		}
		byID := func(list []accounts.Person, id string) accounts.Person {
			for _, p := range list {
				if p.ID == id {
					return p
				}
			}
			t.Fatalf("%s is not listed in %v", id, names(list))
			return accounts.Person{}
		}

		// A member sees everyone, with all of each guest's projects.
		list, err := f.svc.People(ctx, mo, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 6 {
			t.Fatalf("a member sees %v", names(list))
		}
		if got := byID(list, gia).Projects; len(got) != 2 || !slices.Contains(got, both) || !slices.Contains(got, giaOnly) {
			t.Fatalf("a member sees Gia's projects as %v", got)
		}
		if p := byID(list, mo); p.Guest || len(p.Projects) != 0 {
			t.Fatalf("a member's own row = %+v", p)
		}

		// A guest sees the members, and only the guests who share a project.
		list, err = f.svc.People(ctx, gus, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		byID(list, owner.User.ID)
		byID(list, mo)
		for _, hidden := range []string{gil, lone} {
			if slices.ContainsFunc(list, func(p accounts.Person) bool { return p.ID == hidden }) {
				t.Fatalf("a guest sees a guest they share nothing with: %v", names(list))
			}
		}
		// Of another guest, only the projects they share; of themselves, all.
		if got := byID(list, gia).Projects; !slices.Equal(got, []string{both}) {
			t.Fatalf("Gus sees Gia's projects as %v, want only the shared one", got)
		}
		if got := byID(list, gus).Projects; len(got) != 2 {
			t.Fatalf("Gus sees his own projects as %v", got)
		}

		// A guest with no projects still sees the members, and no other guests.
		list, err = f.svc.People(ctx, lone, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(list, func(p accounts.Person) bool { return p.Guest && p.ID != lone }) {
			t.Fatalf("a guest with no projects sees other guests: %v", names(list))
		}
		byID(list, mo)

		// The fuller list, with addresses, stays closed to guests.
		if _, err := f.svc.Members(ctx, gus, team.ID); err == nil {
			t.Fatal("a guest listed the members, addresses and all")
		}
		// And nobody outside the workspace gets either.
		outsider, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, mustWorkspace(t, f, owner.User.ID), "out@example.com", authz.Member), nil, "Out", accounts.Client{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.People(ctx, outsider.User.ID, team.ID); err == nil {
			t.Fatal("someone outside the workspace listed its people")
		}
	})
}

func mustWorkspace(t *testing.T, f *fixture, owner string) string {
	t.Helper()
	w, err := f.svc.CreateWorkspace(context.Background(), owner, "Other")
	if err != nil {
		t.Fatal(err)
	}
	return w.ID
}
