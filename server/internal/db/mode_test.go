//go:build !windows

package db

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A database other users could read, made before Mila kept it from them,
// is kept from them as it opens; a new one never was readable.
func TestTheDatabaseIsTheOwnersOnly(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	dir := t.TempDir()
	path := filepath.Join(dir, "mila.db")
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := Open(context.Background(), "sqlite://"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(context.Background(), "CREATE TABLE t (x)"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s is %o", filepath.Base(p), perm)
		}
	}

	fresh := filepath.Join(dir, "new.db")
	d2, err := Open(context.Background(), "sqlite://"+fresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d2.ExecContext(context.Background(), "CREATE TABLE t (x)"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d2.Close() }()
	for _, p := range []string{fresh, fresh + "-wal"} {
		if info, err := os.Stat(p); err == nil && info.Mode().Perm()&0o077 != 0 {
			t.Errorf("a new %s is %o", filepath.Base(p), info.Mode().Perm())
		}
	}
}
