package keyfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/locodarwin/mila/server/internal/keyfile"
)

// A secret sealed with the data directory's key opens with it, for the same
// purpose only, and with no other key; the key is made once, for its owner.
func TestSealing(t *testing.T) {
	dir := t.TempDir()
	k := keyfile.At(dir)

	// Opening makes no key.
	if _, err := k.Open("sealed:v1:AAAA", "p"); !errors.Is(err, keyfile.ErrUnreadable) {
		t.Fatalf("opening with no key: %v", err)
	}
	if _, err := os.Stat(k.Path()); !os.IsNotExist(err) {
		t.Fatalf("opening made a key: %v", err)
	}

	sealed, err := k.Seal("refresh-123", "inbound_microsoft")
	if err != nil {
		t.Fatal(err)
	}
	if !keyfile.IsSealed(sealed) || strings.Contains(sealed, "refresh-123") {
		t.Fatalf("sealed: %q", sealed)
	}
	again, _ := k.Seal("refresh-123", "inbound_microsoft")
	if again == sealed {
		t.Fatal("two seals of one secret are alike")
	}
	info, err := os.Stat(k.Path())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("the key file is %v", info.Mode().Perm())
	}

	// A fresh reader of the same directory opens it; another purpose, a
	// changed value, or another directory's key does not.
	same := keyfile.At(dir)
	if got, err := same.Open(sealed, "inbound_microsoft"); err != nil || got != "refresh-123" {
		t.Fatalf("opened: %q %v", got, err)
	}
	if _, err := same.Open(sealed, "something_else"); !errors.Is(err, keyfile.ErrUnreadable) {
		t.Fatalf("opened for another purpose: %v", err)
	}
	changed := sealed[:len(sealed)-2] + "AA"
	if changed == sealed {
		changed = sealed[:len(sealed)-2] + "BB"
	}
	if _, err := same.Open(changed, "inbound_microsoft"); !errors.Is(err, keyfile.ErrUnreadable) {
		t.Fatalf("opened a changed value: %v", err)
	}
	other := keyfile.At(t.TempDir())
	if _, err := other.Seal("x", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(sealed, "inbound_microsoft"); !errors.Is(err, keyfile.ErrUnreadable) {
		t.Fatalf("opened with another key: %v", err)
	}

	// A key file that isn't one is said so, not replaced.
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, keyfile.Name), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := keyfile.At(bad).Seal("x", "p"); err == nil || !strings.Contains(err.Error(), "not a key") {
		t.Fatalf("sealing with a broken key file: %v", err)
	}
}

// Two processes making the key at once end up with one key: each seals, and
// each opens what the other sealed.
func TestOneKeyWhenMadeAtOnce(t *testing.T) {
	dir := t.TempDir()
	keys := make([]*keyfile.Key, 8)
	sealed := make([]string, len(keys))
	var wg sync.WaitGroup
	for i := range keys {
		keys[i] = keyfile.At(dir)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := keys[i].Seal("secret", "p")
			if err != nil {
				t.Error(err)
			}
			sealed[i] = s
		}()
	}
	wg.Wait()
	for i, s := range sealed {
		for j, k := range keys {
			if got, err := k.Open(s, "p"); err != nil || got != "secret" {
				t.Fatalf("key %d opening what %d sealed: %q %v", j, i, got, err)
			}
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".secret.key-*"))
	if len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// On a data directory without hard links (some SMB mounts, FAT, some FUSE
// filesystems) the key is still made, once, whole and for its owner alone.
func TestAKeyWithoutHardLinks(t *testing.T) {
	defer keyfile.SetLink(func(oldname, newname string) error {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: syscall.EPERM}
	})()
	dir := t.TempDir()
	keys := make([]*keyfile.Key, 8)
	sealed := make([]string, len(keys))
	var wg sync.WaitGroup
	for i := range keys {
		keys[i] = keyfile.At(dir)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := keys[i].Seal("secret", "p")
			if err != nil {
				t.Error(err)
			}
			sealed[i] = s
		}()
	}
	wg.Wait()
	for i, s := range sealed {
		if got, err := keyfile.At(dir).Open(s, "p"); err != nil || got != "secret" {
			t.Fatalf("opening what %d sealed: %q %v", i, got, err)
		}
	}
	info, err := os.Stat(filepath.Join(dir, keyfile.Name))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("the key file is %v", info.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".secret.key-*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// A key made by another user than the data directory's (sudo mila mailbox
// connect, as root) would be one the server can't read: it is refused,
// saying how to run it, and nothing is made.
func TestNoKeyForAnotherUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owners are Unix's")
	}
	dir := t.TempDir()
	defer keyfile.SetEUID(func() int { return os.Geteuid() + 1 })()
	k := keyfile.At(dir)
	if _, err := k.Seal("x", "p"); err == nil || !strings.Contains(err.Error(), "sudo -u") {
		t.Fatalf("sealing as another user: %v", err)
	}
	if err := k.Check(); err == nil {
		t.Fatal("Check took another user")
	}
	if _, err := os.Stat(k.Path()); !os.IsNotExist(err) {
		t.Fatalf("a key was made: %v", err)
	}
	// A key already there, readable, is used whoever reads it.
	ok := keyfile.SetEUID(os.Geteuid)
	sealed, err := keyfile.At(dir).Seal("x", "p")
	ok()
	if err != nil {
		t.Fatal(err)
	}
	if err := keyfile.At(dir).Check(); err != nil {
		t.Fatalf("Check with the key there: %v", err)
	}
	if got, err := keyfile.At(dir).Open(sealed, "p"); err != nil || got != "x" {
		t.Fatalf("opened %q %v", got, err)
	}
}
