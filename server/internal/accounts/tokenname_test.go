package accounts_test

import (
	"context"
	"testing"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// A token's name shows in history beside email's and automations' changes,
// so it may not be one of theirs.
func TestATokenMayNotPassForEmailOrAnAutomation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		f := newFixture(t, d)
		owner := f.setup(t).User
		for _, name := range []string{"email", "Email", "automation: Cleanup", "An automation", "Automations"} {
			_, _, err := f.svc.CreateToken(context.Background(), owner.ID, accounts.TokenInput{Name: name}, accounts.Client{})
			wantKind(t, err, accounts.Invalid)
		}
		if _, _, err := f.svc.CreateToken(context.Background(), owner.ID, accounts.TokenInput{Name: "Email sync script"}, accounts.Client{}); err != nil {
			t.Fatalf("an ordinary name: %v", err)
		}
	})
}
