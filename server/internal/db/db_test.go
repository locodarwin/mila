package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		n, err := d.Migrate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("second migrate applied %d migrations, want 0", n)
		}
		current, latest, err := d.SchemaVersion(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if current != latest || latest < 1 {
			t.Errorf("schema at %d, latest %d", current, latest)
		}
		at, err := d.LastMigrated(context.Background())
		if err != nil || at.IsZero() || time.Since(at) > time.Hour || time.Until(at) > time.Minute {
			t.Errorf("LastMigrated = %v, %v", at, err)
		}
	})
}

func TestRefusesNewerDatabase(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		// Pretend a newer binary applied migration 9999.
		_, err := d.ExecContext(ctx,
			"INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, true)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Migrate(ctx); !errors.Is(err, db.ErrDatabaseNewer) {
			t.Errorf("Migrate on a newer database = %v, want ErrDatabaseNewer", err)
		}
	})
}

func TestOpenRejectsUnknownScheme(t *testing.T) {
	if _, err := db.Open(context.Background(), "mysql://x"); err == nil {
		t.Error("want an error")
	}
}

// A second row with a taken key is a unique violation on Postgres, which
// a REST create racing another one meets.
func TestIsUniqueViolation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		insert := "INSERT INTO instance_setting (key, value, updated_at) VALUES ('rest-test', 'v', CURRENT_TIMESTAMP)"
		if _, err := d.ExecContext(ctx, insert); err != nil {
			t.Fatal(err)
		}
		_, err := d.ExecContext(ctx, insert)
		if err == nil {
			t.Fatal("a taken key was taken again")
		}
		if got := db.IsUniqueViolation(err); got != (d.Dialect == db.Postgres) {
			t.Fatalf("IsUniqueViolation(%v) = %v on %s", err, got, d.Dialect)
		}
		if db.IsUniqueViolation(errors.New("other")) {
			t.Fatal("any error is a unique violation")
		}
	})
}
