package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store"
)

// Prepare readies a database for this binary, and is the only way the
// server and the commands migrate one: it refuses a database from a newer
// Mila, backs up a database that has migrations pending into dir, then
// migrates it. A new, empty database has nothing to back up. It returns
// how many migrations ran.
func Prepare(ctx context.Context, d *db.DB, dir string, log *slog.Logger) (int, error) {
	current, _, err := d.SchemaVersion(ctx)
	if err != nil {
		return 0, err
	}
	pending, err := d.Pending(ctx)
	if err != nil || !pending {
		return 0, err
	}
	if current > 0 {
		info, err := Snapshot(ctx, d, dir, PreMigrate, store.Now())
		if err != nil {
			return 0, fmt.Errorf("back up before migrating (nothing was changed): %w", err)
		}
		log.Info("backed up the database before migrating", "file", filepath.Join(dir, info.Name))
		if _, err := Prune(dir); err != nil {
			log.Warn("prune backups", "err", err)
		}
	}
	return d.Migrate(ctx)
}

// Restore replaces the database at databaseURL with the backup file.
// The server must be stopped; the caller holds the data directory's
// lock. The database as it was is backed up into dir first, the backup is
// migrated to this binary's schema if it is older (a newer one is
// refused), and a new sync epoch makes every device reload its copy. It
// returns the safety backup, if the database had anything in it.
func Restore(ctx context.Context, databaseURL, dir, file string, log *slog.Logger) (*Info, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("backup directory: %w", err)
	}
	// Work on a copy, so the backup itself is never changed.
	work := filepath.Join(dir, ".restore-"+random()+".db")
	defer removeDB(work)
	if err := copyFile(file, work); err != nil {
		return nil, fmt.Errorf("read the backup: %w", err)
	}
	from, err := db.Open(ctx, "sqlite://"+work)
	if err != nil {
		return nil, fmt.Errorf("open the backup: %w", err)
	}
	defer func() { _ = from.Close() }()
	version, _, err := from.SchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s is not a Mila backup: %w", file, err)
	}
	if version == 0 {
		return nil, fmt.Errorf("%s is not a Mila backup", file)
	}
	if _, err := from.Pending(ctx); err != nil {
		return nil, err // a backup from a newer Mila
	}

	to, err := db.Open(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = to.Close() }()
	if to.Dialect == db.Postgres {
		if _, err := to.Pending(ctx); err != nil {
			return nil, err // a database from a newer Mila
		}
	}
	var safety *Info
	if current, _, err := to.SchemaVersion(ctx); err != nil {
		return nil, err
	} else if current > 0 {
		info, err := Snapshot(ctx, to, dir, PreRestore, store.Now())
		if err != nil {
			return nil, fmt.Errorf("back up the current database (nothing was changed): %w", err)
		}
		safety = &info
		log.Info("backed up the current database", "file", filepath.Join(dir, info.Name))
	}

	if n, err := from.Migrate(ctx); err != nil {
		return safety, fmt.Errorf("migrate the backup: %w", err)
	} else if n > 0 {
		log.Info("migrated the backup", "applied", n)
	}
	if err := store.New(from).SetInstanceSetting(ctx, store.SyncEpochKey, uuid.Must(uuid.NewV7()).String()); err != nil {
		return safety, err
	}

	if to.Dialect == db.SQLite {
		path := strings.TrimPrefix(databaseURL, "sqlite://")
		_ = to.Close()
		// Write the new file beside the old, then swap them, so a failure
		// part way leaves the old one whole.
		next := path + ".restoring"
		removeDB(next)
		if _, err := from.ExecContext(ctx, "VACUUM INTO ?", next); err != nil {
			removeDB(next)
			return safety, fmt.Errorf("write the database: %w", err)
		}
		if err := syncFile(next); err != nil {
			return safety, err
		}
		// The old file's write-ahead log must not be replayed into the new.
		for _, p := range []string{path + "-wal", path + "-shm", path + "-journal"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return safety, err
			}
		}
		if err := os.Rename(next, path); err != nil {
			return safety, err
		}
		return safety, nil
	}
	if _, err := to.Migrate(ctx); err != nil {
		return safety, err
	}
	if _, err := db.Copy(ctx, from, to, db.CopyOptions{Replace: true}); err != nil {
		return safety, fmt.Errorf("write the database (it is unchanged): %w", err)
	}
	return safety, nil
}

func random() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
