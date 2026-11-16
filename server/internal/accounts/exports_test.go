package accounts_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// An export made as a member goes when its maker becomes a guest or
// leaves: it holds what they may no longer see.
func TestExportsGoWithTheAccessTheyWereMadeWith(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		dana := f.member(t, owner, "dana@example.com", "Dana")
		ws, err := f.svc.Workspaces(ctx, dana.ID)
		if err != nil {
			t.Fatal(err)
		}
		var team string
		for _, w := range ws {
			if !w.IsPersonal {
				team = w.ID
			}
		}
		export := func() {
			t.Helper()
			now := store.Now()
			if _, err := d.ExecContext(ctx, rebind(d, "INSERT INTO exports (id, user_id, workspace_id, format, status, file_name, created_at, expires_at) VALUES (?, ?, ?, 'json', 'ready', 'x.zip', ?, ?)"),
				uuid.NewString(), dana.ID, team, now, now.Add(24*7*3600*1e9)); err != nil {
				t.Fatal(err)
			}
		}
		count := func() int {
			t.Helper()
			var n int
			if err := d.QueryRowContext(ctx, rebind(d, "SELECT COUNT(*) FROM exports WHERE user_id = ?"), dana.ID).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		export()
		if err := f.svc.ChangeRole(ctx, owner.ID, team, dana.ID, authz.Admin); err != nil || count() != 1 {
			t.Fatalf("made an admin: %v, %d exports", err, count())
		}
		if err := f.svc.ChangeRole(ctx, owner.ID, team, dana.ID, authz.Guest); err != nil || count() != 0 {
			t.Fatalf("made a guest: %v, %d exports", err, count())
		}
		if err := f.svc.ChangeRole(ctx, owner.ID, team, dana.ID, authz.Member); err != nil {
			t.Fatal(err)
		}
		export()
		if err := f.svc.RemoveMember(ctx, owner.ID, team, dana.ID); err != nil || count() != 0 {
			t.Fatalf("removed: %v, %d exports", err, count())
		}
	})
}
