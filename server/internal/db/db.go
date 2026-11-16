// Package db opens Mila's database, SQLite or Postgres, and migrates it.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Dialect names the SQL engine behind a DB.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// DB is an open database and the dialect it speaks.
type DB struct {
	*sql.DB
	Dialect Dialect
}

//go:embed migrations
var migrations embed.FS

// ErrDatabaseNewer means the database was migrated by a newer Mila. Running
// an older binary against it could corrupt data, so the server refuses.
var ErrDatabaseNewer = errors.New("database was migrated by a newer version of Mila")

// Open connects to sqlite://<path> or postgres://... and checks the
// connection. A SQLite file and its directory are created if missing.
func Open(ctx context.Context, rawURL string) (*DB, error) {
	var d *DB
	var path string
	switch {
	case strings.HasPrefix(rawURL, "sqlite://"):
		path = strings.TrimPrefix(rawURL, "sqlite://")
		if path == "" {
			return nil, errors.New("sqlite URL has no path")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		q := url.Values{}
		for _, p := range []string{
			"journal_mode(WAL)",
			"busy_timeout(5000)",
			"foreign_keys(1)",
			"synchronous(NORMAL)",
		} {
			q.Add("_pragma", p)
		}
		q.Set("_txlock", "immediate")
		// Times are written as "2006-01-02 15:04:05.999999999-07:00"; with every
		// time in UTC, text comparison in SQL orders them correctly.
		q.Set("_time_format", "sqlite")
		sqlDB, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
		if err != nil {
			return nil, err
		}
		d = &DB{sqlDB, SQLite}
	case strings.HasPrefix(rawURL, "postgres://"), strings.HasPrefix(rawURL, "postgresql://"):
		sqlDB, err := sql.Open("pgx", rawURL)
		if err != nil {
			return nil, err
		}
		d = &DB{sqlDB, Postgres}
	default:
		return nil, fmt.Errorf("unsupported database URL %q", rawURL)
	}
	if err := d.PingContext(ctx); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("connect to %s: %w", d.Dialect, err)
	}
	if path != "" {
		if err := ownersOnly(path); err != nil {
			_ = d.Close()
			return nil, err
		}
	}
	return d, nil
}

// ownersOnly keeps a SQLite database, its write-ahead log and its shared
// memory file from other users of the computer: they hold everything,
// webhook secrets in plain text among it. SQLite makes them as the umask
// allows, often readable by all, and the log and memory file take the
// database's mode, so a database made before this is put right as it opens.
func ownersOnly(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			if err := os.Chmod(p, perm&0o700); err != nil {
				return fmt.Errorf("keep %s from other users: %w", p, err)
			}
		}
	}
	return nil
}

func (d *DB) provider() (*goose.Provider, error) {
	sub, err := fs.Sub(migrations, "migrations/"+string(d.Dialect))
	if err != nil {
		return nil, err
	}
	dialect := goose.DialectSQLite3
	if d.Dialect == Postgres {
		dialect = goose.DialectPostgres
	}
	return goose.NewProvider(dialect, d.DB, sub, goose.WithGoMigrations(pairIDs(d.Dialect), taskDates(d.Dialect)))
}

// SchemaVersion reports the database's migration version and the newest
// version this binary carries.
func (d *DB) SchemaVersion(ctx context.Context) (current, latest int64, err error) {
	p, err := d.provider()
	if err != nil {
		return 0, 0, err
	}
	current, err = p.GetDBVersion(ctx)
	if err != nil {
		return 0, 0, err
	}
	sources := p.ListSources()
	if len(sources) > 0 {
		latest = sources[len(sources)-1].Version
	}
	return current, latest, nil
}

// LastMigrated reports when the newest applied migration ran, for the
// admin page; the zero time if none has.
func (d *DB) LastMigrated(ctx context.Context) (time.Time, error) {
	p, err := d.provider()
	if err != nil {
		return time.Time{}, err
	}
	list, err := p.Status(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var last time.Time
	for _, m := range list {
		if m.State == goose.StateApplied && m.AppliedAt.After(last) {
			last = m.AppliedAt
		}
	}
	return last.UTC(), nil
}

// Pending reports whether migrations are waiting to run. It fails with
// ErrDatabaseNewer for a database ahead of this binary.
func (d *DB) Pending(ctx context.Context) (bool, error) {
	current, latest, err := d.SchemaVersion(ctx)
	if err != nil {
		return false, err
	}
	if current > latest {
		return false, newer(current, latest)
	}
	return current < latest, nil
}

func newer(current, latest int64) error {
	return fmt.Errorf("%w (database at %d, this binary knows up to %d)", ErrDatabaseNewer, current, latest)
}

// Migrate applies every pending migration and returns how many ran. It
// refuses, with ErrDatabaseNewer, a database ahead of this binary. Callers
// outside tests go through backup.Prepare, which backs the database up
// first.
func (d *DB) Migrate(ctx context.Context) (int, error) {
	_, latest, err := d.SchemaVersion(ctx)
	if err != nil {
		return 0, err
	}
	return d.MigrateTo(ctx, latest)
}

// MigrateTo applies the migrations up to and including version, and
// returns how many ran. A backup of an older database is made at its own
// version this way, and tests use it to leave migrations pending.
func (d *DB) MigrateTo(ctx context.Context, version int64) (int, error) {
	current, latest, err := d.SchemaVersion(ctx)
	if err != nil {
		return 0, err
	}
	if current > latest {
		return 0, newer(current, latest)
	}
	p, err := d.provider()
	if err != nil {
		return 0, err
	}
	results, err := p.UpTo(ctx, version)
	return len(results), err
}

// IsUniqueViolation reports whether err is Postgres refusing a row whose
// key another row already has (SQLSTATE 23505), as when two transactions
// create the same row at once.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
