package backup_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func backups(kind backup.Kind, times ...time.Time) []backup.Info {
	var out []backup.Info
	for _, t := range times {
		out = append(out, backup.Info{Name: backup.FileName(t, kind, 13), At: t, Kind: kind})
	}
	return out
}

func names(list []backup.Info) []string {
	var out []string
	for _, b := range list {
		out = append(out, b.At.Format("2006-01-02T15")+"/"+string(b.Kind))
	}
	slices.Sort(out)
	return out
}

func TestRetain(t *testing.T) {
	// A nightly backup at 03:00 every day from 2026-01-01 to 2026-10-28.
	var nightly []time.Time
	for d := at("2026-01-01T03:00:00Z"); !d.After(at("2026-10-28T03:00:00Z")); d = d.AddDate(0, 0, 1) {
		nightly = append(nightly, d)
	}
	tests := []struct {
		name string
		in   []backup.Info
		keep []string
	}{
		{"nothing", nil, nil},
		{
			"every night for ten months",
			backups(backup.Nightly, nightly...),
			[]string{
				// The newest of each of the last six months: May to October,
				// October's being the last day there is.
				"2026-05-31T03/nightly", "2026-06-30T03/nightly", "2026-07-31T03/nightly",
				"2026-08-31T03/nightly", "2026-09-30T03/nightly",
				// The newest of each of the last four weeks (ISO, from
				// Monday): weeks 41 to 44, which ends on Sunday Nov 1.
				"2026-10-11T03/nightly", "2026-10-18T03/nightly", "2026-10-25T03/nightly",
				// The last seven days.
				"2026-10-22T03/nightly", "2026-10-23T03/nightly", "2026-10-24T03/nightly",
				"2026-10-26T03/nightly", "2026-10-27T03/nightly", "2026-10-28T03/nightly",
			},
		},
		{
			"days count only when they have a backup",
			backups(backup.Nightly, at("2026-10-01T03:00:00Z"), at("2026-10-02T03:00:00Z"), at("2026-10-20T03:00:00Z"),
				at("2026-10-21T03:00:00Z"), at("2026-10-22T03:00:00Z"), at("2026-10-23T03:00:00Z"),
				at("2026-10-24T03:00:00Z"), at("2026-10-25T03:00:00Z")),
			// Oct 1 goes: it is not among the last seven days that have one,
			// and Oct 2 is newer in its week and month.
			[]string{"2026-10-02T03/nightly", "2026-10-20T03/nightly", "2026-10-21T03/nightly",
				"2026-10-22T03/nightly", "2026-10-23T03/nightly", "2026-10-24T03/nightly", "2026-10-25T03/nightly"},
		},
		{
			"the newest of a day, manual ones counting with nightly ones",
			append(backups(backup.Nightly, at("2026-10-28T03:00:00Z")),
				backups(backup.Manual, at("2026-10-28T09:00:00Z"), at("2026-10-28T15:00:00Z"))...),
			[]string{"2026-10-28T15/manual"},
		},
		{
			"the last three before migrations and before restores, apart from the rest",
			append(append(backups(backup.PreMigrate, at("2026-01-01T10:00:00Z"), at("2026-03-01T10:00:00Z"),
				at("2026-05-01T10:00:00Z"), at("2026-07-01T10:00:00Z")),
				backups(backup.PreRestore, at("2026-02-01T10:00:00Z"))...),
				backups(backup.Nightly, at("2026-07-01T03:00:00Z"))...),
			[]string{"2026-02-01T10/prerestore", "2026-03-01T10/premigrate", "2026-05-01T10/premigrate",
				"2026-07-01T03/nightly", "2026-07-01T10/premigrate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep, drop := backup.Retain(tt.in)
			slices.Sort(tt.keep)
			if got := names(keep); !slices.Equal(got, tt.keep) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.keep)
			}
			if len(keep)+len(drop) != len(tt.in) {
				t.Errorf("kept %d and dropped %d of %d", len(keep), len(drop), len(tt.in))
			}
		})
	}
}

func TestSlot(t *testing.T) {
	for _, tt := range []struct {
		now  string
		hour int
		want string
	}{
		{"2026-10-28T03:00:00Z", 3, "2026-10-28T03:00:00Z"},
		{"2026-10-28T02:59:59Z", 3, "2026-10-27T03:00:00Z"},
		{"2026-10-28T23:00:00Z", 3, "2026-10-28T03:00:00Z"},
		{"2026-10-28T01:00:00+05:00", 0, "2026-10-27T00:00:00Z"},
	} {
		if got := backup.Slot(at(tt.now), tt.hour); !got.Equal(at(tt.want)) {
			t.Errorf("Slot(%s, %d) = %s, want %s", tt.now, tt.hour, got, tt.want)
		}
	}
}

// seed gives a database a user, so a copy of it can be told apart.
func seed(t *testing.T, st *store.Store, email string) {
	t.Helper()
	must(t, st.CreateUser(context.Background(), sqlitedb.CreateUserParams{
		ID: uuid.NewString(), Email: email, Name: email, IsInstanceAdmin: true, Now: store.Now(),
	}))
}

func emails(t *testing.T, d *db.DB) []string {
	t.Helper()
	users, err := store.New(d).ListUsers(context.Background())
	must(t, err)
	var out []string
	for _, u := range users {
		out = append(out, u.Email)
	}
	slices.Sort(out)
	return out
}

func openFile(t *testing.T, path string) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), "sqlite://"+path)
	must(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSnapshot(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		seed(t, store.New(d), "ann@example.com")
		dir := filepath.Join(t.TempDir(), "backups")
		now := at("2026-10-28T03:00:07Z")
		info, err := backup.Snapshot(ctx, d, dir, backup.Nightly, now)
		must(t, err)
		_, latest, _ := d.SchemaVersion(ctx)
		if info.Schema != latest || info.Kind != backup.Nightly || !info.At.Equal(now) || info.Size == 0 {
			t.Errorf("info = %+v", info)
		}
		path := filepath.Join(dir, info.Name)
		st, err := os.Stat(path)
		must(t, err)
		// Windows has no such modes; files there take their access from
		// the folder they are in.
		if runtime.GOOS != "windows" {
			if st.Mode().Perm() != 0o600 {
				t.Errorf("backup mode %v, want 0600", st.Mode().Perm())
			}
			if dst, err := os.Stat(dir); err != nil || dst.Mode().Perm() != 0o700 {
				t.Errorf("directory mode %v (%v), want 0700", dst.Mode().Perm(), err)
			}
		}
		list, err := backup.List(dir)
		must(t, err)
		if len(list) != 1 || list[0] != info {
			t.Errorf("List = %+v, want %+v", list, info)
		}
		// The backup is a whole SQLite database on its own.
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("the directory holds %d files, want the backup alone", len(entries))
		}
		copyDB := openFile(t, path)
		if v, _, err := copyDB.SchemaVersion(ctx); err != nil || v != latest {
			t.Errorf("backup at schema %d (%v), want %d", v, err, latest)
		}
		if got := emails(t, copyDB); !slices.Equal(got, []string{"ann@example.com"}) {
			t.Errorf("backup holds %v", got)
		}
		// A second one in the same second (a double click on "Back up
		// now") is kept beside the first, numbered, and listed as newer.
		again, err := backup.Snapshot(ctx, d, dir, backup.Nightly, now.Add(300*time.Millisecond))
		must(t, err)
		if want := strings.TrimSuffix(info.Name, ".db") + "-2.db"; again.Name != want {
			t.Errorf("second backup named %q, want %q", again.Name, want)
		}
		third, err := backup.Snapshot(ctx, d, dir, backup.Nightly, now.Add(600*time.Millisecond))
		must(t, err)
		list, err = backup.List(dir)
		must(t, err)
		var got []string
		for _, b := range list {
			got = append(got, b.Name)
		}
		if want := []string{third.Name, again.Name, info.Name}; !slices.Equal(got, want) {
			t.Errorf("List = %v, want %v", got, want)
		}
		// No partial copy is left behind (the first backup, opened above,
		// has SQLite's own files beside it).
		entries, _ = os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".partial") {
				t.Errorf("left behind %s", e.Name())
			}
		}
	})
}

func TestPrepare(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backups")

	t.Run("a new database is migrated without a backup", func(t *testing.T) {
		d := openFile(t, filepath.Join(t.TempDir(), "new.db"))
		n, err := backup.Prepare(ctx, d, dir, quiet)
		must(t, err)
		if n == 0 {
			t.Error("nothing migrated")
		}
		if list, _ := backup.List(dir); len(list) != 0 {
			t.Errorf("made %d backups of an empty database", len(list))
		}
	})

	t.Run("pending migrations are backed up first", func(t *testing.T) {
		d := openFile(t, filepath.Join(t.TempDir(), "old.db"))
		_, latest, err := d.SchemaVersion(ctx)
		must(t, err)
		_, err = d.MigrateTo(ctx, latest-1)
		must(t, err)
		seed(t, store.New(d), "ann@example.com")
		n, err := backup.Prepare(ctx, d, dir, quiet)
		must(t, err)
		if n != 1 {
			t.Errorf("applied %d migrations, want 1", n)
		}
		list, _ := backup.List(dir)
		if len(list) != 1 || list[0].Kind != backup.PreMigrate || list[0].Schema != latest-1 {
			t.Fatalf("backups = %+v, want one before migrating, at %d", list, latest-1)
		}
		// Nothing is pending now, so nothing more is backed up.
		if _, err := backup.Prepare(ctx, d, dir, quiet); err != nil {
			t.Fatal(err)
		}
		if again, _ := backup.List(dir); len(again) != 1 {
			t.Errorf("%d backups after a second start, want 1", len(again))
		}
	})

	t.Run("a newer database is refused before anything", func(t *testing.T) {
		dbtest.Each(t, func(t *testing.T, d *db.DB) {
			dir := filepath.Join(t.TempDir(), "backups")
			_, err := d.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, true)")
			must(t, err)
			if _, err := backup.Prepare(ctx, d, dir, quiet); !errors.Is(err, db.ErrDatabaseNewer) {
				t.Errorf("Prepare = %v, want ErrDatabaseNewer", err)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Error("a backup was made of a database this Mila cannot read")
			}
		})
	})
}

func TestRestore(t *testing.T) {
	dbtest.EachURL(t, func(t *testing.T, rawURL string) {
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "backups")
		d, err := db.Open(ctx, rawURL)
		must(t, err)
		_, err = backup.Prepare(ctx, d, dir, quiet)
		must(t, err)
		st := store.New(d)
		seed(t, st, "ann@example.com")
		epoch, err := st.InitInstanceSetting(ctx, store.SyncEpochKey, "first")
		must(t, err)
		info, err := backup.Snapshot(ctx, d, dir, backup.Manual, at("2026-10-28T03:00:00Z"))
		must(t, err)
		seed(t, st, "ben@example.com")
		must(t, d.Close())

		safety, err := backup.Restore(ctx, rawURL, dir, filepath.Join(dir, info.Name), quiet)
		must(t, err)
		if safety == nil || safety.Kind != backup.PreRestore {
			t.Fatalf("safety backup = %+v", safety)
		}
		d, err = db.Open(ctx, rawURL)
		must(t, err)
		defer func() { _ = d.Close() }()
		if got := emails(t, d); !slices.Equal(got, []string{"ann@example.com"}) {
			t.Errorf("after the restore the database holds %v", got)
		}
		if v, _, _ := store.New(d).InstanceSetting(ctx, store.SyncEpochKey); v == epoch || v == "" {
			t.Errorf("sync epoch %q after the restore, want a new one", v)
		}
		// The database as it was is in the safety backup.
		if got := emails(t, openFile(t, filepath.Join(dir, safety.Name))); !slices.Equal(got, []string{"ann@example.com", "ben@example.com"}) {
			t.Errorf("the safety backup holds %v", got)
		}
		// The backup restored from is untouched, and no work files are left.
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				t.Errorf("left %s behind", e.Name())
			}
		}
		if strings.HasPrefix(rawURL, "sqlite://") {
			if _, err := os.Stat(strings.TrimPrefix(rawURL, "sqlite://") + ".restoring"); !errors.Is(err, os.ErrNotExist) {
				t.Error("left the new file beside the database")
			}
		}
	})
}

func TestRestoreRefusesNewerAndForeignFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	target := "sqlite://" + filepath.Join(dir, "mila.db")

	newer := filepath.Join(dir, "newer.db")
	d := openFile(t, newer)
	_, err := d.Migrate(ctx)
	must(t, err)
	_, err = d.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, true)")
	must(t, err)
	must(t, d.Close())
	if _, err := backup.Restore(ctx, target, dir, newer, quiet); !errors.Is(err, db.ErrDatabaseNewer) {
		t.Errorf("restoring a newer backup: %v", err)
	}
	notes := filepath.Join(dir, "notes.txt")
	must(t, os.WriteFile(notes, []byte("not a database"), 0o600))
	if _, err := backup.Restore(ctx, target, dir, notes, quiet); err == nil {
		t.Error("restored a file that is not a backup")
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	unlock, err := backup.Lock(dir)
	must(t, err)
	if _, err := backup.Lock(dir); !errors.Is(err, backup.ErrLocked) {
		t.Errorf("second lock: %v, want ErrLocked", err)
	}
	unlock()
	again, err := backup.Lock(dir)
	must(t, err)
	again()
}

// TestNightly runs the schedule on a moving clock: the first check only
// notes the slot, the next night's is queued once and made in the slow
// lane, and failures email the admins once per run of failures.
func TestNightly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		st := store.New(d)
		seed(t, st, "admin@example.com")
		now := at("2026-10-28T12:00:00Z")
		runner := jobs.New(st, quiet, func() time.Time { return now })
		dir := filepath.Join(t.TempDir(), "backups")
		var sent []mail.Message
		svc := backup.New(st, quiet, runner, backup.Config{
			Dir: dir, Nightly: true, Hour: 3, BaseURL: "https://mila.example",
			Mail: func(_ context.Context, m mail.Message) error { sent = append(sent, m); return nil },
		})
		count := func() int {
			list, err := backup.List(dir)
			must(t, err)
			return len(list)
		}

		runner.Pass(ctx)
		if n := count(); n != 0 {
			t.Fatalf("first start made %d backups, want none until the next night", n)
		}
		now = at("2026-10-29T02:50:00Z")
		runner.Pass(ctx)
		if n := count(); n != 0 {
			t.Fatalf("%d backups before the hour", n)
		}
		now = at("2026-10-29T03:01:00Z")
		runner.Pass(ctx)
		runner.Pass(ctx)
		if n := count(); n != 1 {
			t.Fatalf("%d backups after the hour, want 1", n)
		}
		status, err := svc.Status(ctx)
		must(t, err)
		if status.LastOKAt == nil || !status.LastOKAt.Equal(now.Truncate(time.Second)) || len(status.Recent) != 1 || status.LastErrorAt != nil {
			t.Errorf("status = %+v", status)
		}

		// The server was off for two nights; it catches up once.
		now = at("2026-10-31T18:00:00Z")
		runner.Pass(ctx)
		runner.Pass(ctx)
		if n := count(); n != 2 {
			t.Fatalf("%d backups after catching up, want 2", n)
		}

		// Backups start failing: the directory is now a file.
		must(t, os.RemoveAll(dir))
		must(t, os.WriteFile(dir, nil, 0o600))
		for day := range 2 {
			now = at("2026-11-01T04:00:00Z").AddDate(0, 0, day)
			runner.Pass(ctx)
		}
		status, err = svc.Status(ctx)
		must(t, err)
		if status.LastError == "" || status.LastErrorAt == nil || !status.LastErrorAt.Equal(now) {
			t.Errorf("status after failures = %+v", status)
		}
		if len(sent) != 1 || sent[0].To != "admin@example.com" || !strings.Contains(sent[0].Text, "could not back up") {
			t.Fatalf("sent %+v, want one email for the run of failures", sent)
		}
		if !strings.Contains(sent[0].Text, "Sun, Nov 1") {
			t.Errorf("email does not say when in the admin's way:\n%s", sent[0].Text)
		}

		// It works again, then fails again: a new run, a new email.
		must(t, os.Remove(dir))
		_, err = svc.Run(ctx, backup.Manual)
		must(t, err)
		// Asked for twice in the same second: both are made, and nothing
		// is reported as failed.
		if _, err := svc.Run(ctx, backup.Manual); err != nil {
			t.Fatalf("a second backup in the same second: %v", err)
		}
		if len(sent) != 1 {
			t.Fatalf("sent %d emails after two good backups, want still 1", len(sent))
		}
		must(t, os.RemoveAll(dir))
		must(t, os.WriteFile(dir, nil, 0o600))
		if _, err := svc.Run(ctx, backup.Manual); err == nil {
			t.Fatal("a backup into a file worked")
		}
		if len(sent) != 2 {
			t.Errorf("sent %d emails, want a second for the new run of failures", len(sent))
		}
	})
}

// A backup the server stops partway is no failure: nothing is recorded
// and nobody is emailed.
func TestBackupStoppedByShutdown(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		st := store.New(d)
		seed(t, st, "admin@example.com")
		var sent []mail.Message
		svc := backup.New(st, quiet, nil, backup.Config{
			Dir: filepath.Join(t.TempDir(), "backups"), BaseURL: "https://mila.example",
			Mail: func(_ context.Context, m mail.Message) error { sent = append(sent, m); return nil },
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc.Run(ctx, backup.Nightly); err == nil {
			t.Fatal("a backup worked with its context canceled")
		}
		status, err := svc.Status(context.Background())
		must(t, err)
		if status.LastError != "" || status.LastErrorAt != nil || len(sent) != 0 {
			t.Errorf("a stopped backup was reported: %+v, sent %d", status, len(sent))
		}
	})
}
