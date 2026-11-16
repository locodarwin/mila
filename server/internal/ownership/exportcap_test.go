package ownership_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ownership"
)

// One person keeps ten exports at most, each a copy of a workspace kept
// for a week; deleting one, or its expiry, makes room. A failed one holds
// no file and does not count.
func TestExportsKeptAreCapped(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		owner := f.users["owner"]
		start := func() error {
			_, err := f.svc.StartExport(ctx, owner, f.ws, ownership.CSV)
			if err == nil {
				_, err = f.runner.RunDue(ctx)
			}
			return err
		}
		for range 10 {
			must(t, start())
		}
		err := start()
		if apperr.KindOf(err) != apperr.Conflict || !strings.Contains(err.Error(), "kept") {
			t.Fatalf("an eleventh export: %v", err)
		}
		// Someone else's count is their own.
		if _, err := f.svc.StartExport(ctx, f.users["member"], f.ws, ownership.CSV); err != nil {
			t.Errorf("another person's first export: %v", err)
		}
		list, err := f.svc.Exports(ctx, owner)
		must(t, err)
		must(t, f.svc.DeleteExport(ctx, owner, list[0].ID))
		must(t, start())
		if err := start(); apperr.KindOf(err) != apperr.Conflict {
			t.Errorf("an eleventh after one was replaced: %v", err)
		}
		_, err = f.d.ExecContext(ctx, rebind(f.d, "UPDATE exports SET status = 'failed' WHERE id = ?"), list[1].ID)
		must(t, err)
		must(t, start())
		f.clock.advance(ownership.ExportLifetime + time.Minute)
		must(t, start())
	})
}
