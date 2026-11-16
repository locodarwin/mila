// Package dbtest runs a test against every supported database: always a
// fresh SQLite file, and Postgres when MILA_TEST_POSTGRES_URL is set. CI
// sets MILA_TEST_REQUIRE_POSTGRES=1 so a missing Postgres fails instead of
// skipping.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
)

// Each runs fn once per database, each on an empty, migrated database.
func Each(t *testing.T, fn func(t *testing.T, d *db.DB)) {
	t.Helper()
	EachURL(t, func(t *testing.T, rawURL string) {
		fn(t, open(t, rawURL))
	})
}

// EachURL runs fn once per database with the URL of an empty database
// that does not exist yet (SQLite) or has no tables (Postgres), for tests
// that open, migrate or replace it themselves.
func EachURL(t *testing.T, fn func(t *testing.T, rawURL string)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		fn(t, "sqlite://"+filepath.Join(t.TempDir(), "mila.db"))
	})
	t.Run("postgres", func(t *testing.T) {
		base := os.Getenv("MILA_TEST_POSTGRES_URL")
		if base == "" {
			if os.Getenv("MILA_TEST_REQUIRE_POSTGRES") != "" {
				t.Fatal("MILA_TEST_REQUIRE_POSTGRES is set but MILA_TEST_POSTGRES_URL is not")
			}
			t.Skip("MILA_TEST_POSTGRES_URL not set")
		}
		fn(t, freshSchema(t, base))
	})
}

// Open returns an empty, migrated SQLite database.
func Open(t *testing.T) *db.DB {
	t.Helper()
	return open(t, "sqlite://"+filepath.Join(t.TempDir(), "mila.db"))
}

func open(t *testing.T, rawURL string) *db.DB {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return d
}

// freshSchema creates a uniquely named schema, drops it when the test ends,
// and returns base with its search_path pointing there.
func freshSchema(t *testing.T, base string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := db.Open(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	rand.Read(b)
	schema := "test_" + hex.EncodeToString(b)
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
