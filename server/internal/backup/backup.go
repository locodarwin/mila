// Package backup keeps copies of the instance's database: nightly ones on
// a persisted schedule, one before every migration and every restore, and
// any asked for by hand. A backup is always a SQLite file, whichever
// database the instance runs on: SQLite is copied with VACUUM INTO, and
// Postgres with the copier in package db, so restoring and moving between
// databases use the same path and no pg_dump of a matching version needs
// to be installed.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
)

// Kind says why a backup was made.
type Kind string

const (
	// Nightly backups are made by the server on its schedule.
	Nightly Kind = "nightly"
	// Manual ones are asked for: `mila backup now` or the admin page.
	Manual Kind = "manual"
	// PreMigrate is taken just before migrations run.
	PreMigrate Kind = "premigrate"
	// PreRestore is the database as it was before a restore replaced it.
	PreRestore Kind = "prerestore"
)

func (k Kind) valid() bool {
	switch k {
	case Nightly, Manual, PreMigrate, PreRestore:
		return true
	}
	return false
}

// Info describes one backup file.
type Info struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	Kind Kind      `json:"kind"`
	// Schema is the database's migration version when it was made.
	Schema int64 `json:"schema"`
	Size   int64 `json:"size"`
	// seq numbers the backups of a kind made in the same second, from 1.
	seq int
}

// stamp is how a backup's time is written in its name.
const stamp = "20060102T150405Z"

// FileName is the name of a backup: when, why, and at which schema, so
// listing them needs no file opened, as in
// mila-20261028T030000Z-nightly-s13.db.
func FileName(at time.Time, kind Kind, schema int64) string {
	return fileName(at, kind, schema, 1)
}

// fileName numbers a later backup of the same kind in the same second,
// as in mila-20261028T030000Z-manual-s13-2.db.
func fileName(at time.Time, kind Kind, schema int64, seq int) string {
	name := fmt.Sprintf("mila-%s-%s-s%d", at.UTC().Format(stamp), kind, schema)
	if seq > 1 {
		name += "-" + strconv.Itoa(seq)
	}
	return name + ".db"
}

// newer orders backups newest first.
func newer(a, b Info) bool {
	if !a.At.Equal(b.At) {
		return a.At.After(b.At)
	}
	if a.seq != b.seq {
		return a.seq > b.seq
	}
	return a.Name > b.Name
}

// parse reads a backup's name; ok is false for any other file.
func parse(name string) (Info, bool) {
	rest, ok := strings.CutPrefix(name, "mila-")
	if !ok {
		return Info{}, false
	}
	rest, ok = strings.CutSuffix(rest, ".db")
	if !ok {
		return Info{}, false
	}
	parts := strings.Split(rest, "-")
	seq := 1
	if len(parts) == 4 {
		n, err := strconv.Atoi(parts[3])
		if err != nil || n < 2 || strconv.Itoa(n) != parts[3] {
			return Info{}, false
		}
		seq, parts = n, parts[:3]
	}
	if len(parts) != 3 || !strings.HasPrefix(parts[2], "s") {
		return Info{}, false
	}
	at, err := time.Parse(stamp, parts[0])
	if err != nil {
		return Info{}, false
	}
	schema, err := strconv.ParseInt(parts[2][1:], 10, 64)
	if err != nil || !Kind(parts[1]).valid() {
		return Info{}, false
	}
	return Info{Name: name, At: at, Kind: Kind(parts[1]), Schema: schema, seq: seq}, true
}

// Snapshot writes a backup of d into dir and returns it. The file is
// readable by its owner only, since it holds sign-in secrets, and appears
// under its final name only once complete. A backup never replaces
// another: a second one of the same kind in the same second (two clicks
// on "Back up now", or the command run twice) is numbered.
func Snapshot(ctx context.Context, d *db.DB, dir string, kind Kind, now time.Time) (Info, error) {
	if !kind.valid() {
		return Info{}, fmt.Errorf("unknown kind of backup %q", kind)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, fmt.Errorf("backup directory: %w", err)
	}
	schema, _, err := d.SchemaVersion(ctx)
	if err != nil {
		return Info{}, err
	}
	info := Info{At: now.UTC().Truncate(time.Second), Kind: kind, Schema: schema}
	// The copy is made under a name of its own, so two backups under way
	// at once (the server's and the command's) never write one file. It
	// starts empty, which VACUUM INTO and SQLite both accept.
	tmp, err := os.CreateTemp(dir, ".mila-*.partial")
	if err != nil {
		return Info{}, fmt.Errorf("backup directory: %w", err)
	}
	partial := tmp.Name()
	_ = tmp.Close()
	defer removeDB(partial)

	switch d.Dialect {
	case db.SQLite:
		// A consistent copy even while the server writes, compacted.
		if _, err := d.ExecContext(ctx, "VACUUM INTO ?", partial); err != nil {
			return Info{}, fmt.Errorf("copy the database: %w", err)
		}
	default:
		if err := copyInto(ctx, d, partial, schema); err != nil {
			return Info{}, err
		}
	}
	if err := os.Chmod(partial, 0o600); err != nil {
		return Info{}, err
	}
	if err := syncFile(partial); err != nil {
		return Info{}, err
	}
	if err := place(partial, dir, &info); err != nil {
		return Info{}, err
	}
	st, err := os.Stat(filepath.Join(dir, info.Name))
	if err != nil {
		return Info{}, err
	}
	info.Size = st.Size()
	return info, nil
}

// place gives the finished copy the first free name for its second. A
// hard link takes a name only if it is free, so two processes finishing
// at once cannot take the same one; where links are not supported, a
// rename to a name seen to be free does nearly as well.
func place(partial, dir string, info *Info) error {
	for seq := 1; seq <= 100; seq++ {
		name := fileName(info.At, info.Kind, info.Schema, seq)
		final := filepath.Join(dir, name)
		err := os.Link(partial, final)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			if _, serr := os.Lstat(final); serr == nil {
				continue
			}
			if err := os.Rename(partial, final); err != nil {
				return err
			}
		}
		info.Name, info.seq = name, seq
		return nil
	}
	return fmt.Errorf("too many %s backups in the second %s", info.Kind, info.At.Format(stamp))
}

// copyInto copies d into a new SQLite file at path, migrated to the same
// version as d.
func copyInto(ctx context.Context, d *db.DB, path string, schema int64) error {
	out, err := db.Open(ctx, "sqlite://"+path)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := out.MigrateTo(ctx, schema); err != nil {
		return fmt.Errorf("prepare the backup file: %w", err)
	}
	if _, err := db.Copy(ctx, d, out, db.CopyOptions{}); err != nil {
		return fmt.Errorf("copy the database: %w", err)
	}
	return settle(ctx, out)
}

// settle folds a SQLite file's write-ahead log into it and leaves it in
// rollback mode, so the one file is the whole database.
func settle(ctx context.Context, d *db.DB) error {
	if _, err := d.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	_, err := d.ExecContext(ctx, "PRAGMA journal_mode=DELETE")
	return err
}

// removeDB removes a SQLite file and the files SQLite keeps beside it.
func removeDB(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		_ = os.Remove(p)
	}
}

// syncFile opens path for writing because Windows flushes a file only
// through a handle that may write to it; a read-only one is refused.
func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

// List returns the backups in dir, newest first. A missing directory has
// none.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		info, ok := parse(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		if fi, err := e.Info(); err == nil {
			info.Size = fi.Size()
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return newer(out[i], out[j]) })
	return out, nil
}

// Retention: the newest nightly or manual backup of each of the last 7
// days that have one, of the last 4 weeks and of the last 6 months; and
// the last 3 taken before migrations and before restores.
const (
	keepDaily   = 7
	keepWeekly  = 4
	keepMonthly = 6
	keepBefore  = 3
)

// Retain splits backups into those to keep and those to delete. Days,
// weeks (ISO, starting Monday) and months are counted in UTC, and only
// those that have a backup count, so a server that was off for a week
// still keeps seven days of them. Each backup is kept if any rule keeps
// it.
func Retain(backups []Info) (keep, drop []Info) {
	sorted := append([]Info(nil), backups...)
	sort.Slice(sorted, func(i, j int) bool { return newer(sorted[i], sorted[j]) })
	kept := map[string]bool{}
	periodic := func(n int, bucket func(time.Time) string) {
		seen := map[string]bool{}
		for _, b := range sorted {
			if b.Kind != Nightly && b.Kind != Manual {
				continue
			}
			k := bucket(b.At.UTC())
			if seen[k] {
				continue
			}
			if len(seen) == n {
				return
			}
			seen[k] = true
			kept[b.Name] = true
		}
	}
	periodic(keepDaily, func(t time.Time) string { return t.Format(time.DateOnly) })
	periodic(keepWeekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	})
	periodic(keepMonthly, func(t time.Time) string { return t.Format("2006-01") })
	for _, kind := range []Kind{PreMigrate, PreRestore} {
		n := 0
		for _, b := range sorted {
			if b.Kind == kind && n < keepBefore {
				kept[b.Name] = true
				n++
			}
		}
	}
	for _, b := range sorted {
		if kept[b.Name] {
			keep = append(keep, b)
		} else {
			drop = append(drop, b)
		}
	}
	return keep, drop
}

// Prune deletes the backups in dir that Retain lets go, and returns them.
func Prune(dir string) ([]Info, error) {
	list, err := List(dir)
	if err != nil {
		return nil, err
	}
	_, drop := Retain(list)
	var removed []Info
	for _, b := range drop {
		if err := os.Remove(filepath.Join(dir, b.Name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, err
		}
		removed = append(removed, b)
	}
	return removed, nil
}
