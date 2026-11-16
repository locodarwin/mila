package files

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveOpenSweep(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, size, head, err := s.Save(bytes.NewReader([]byte("hello, world")), 100)
	if err != nil || size != 12 || string(head) != "hello, world" {
		t.Fatalf("Save = %q %d %q %v", key, size, head, err)
	}
	f, err := s.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if string(got) != "hello, world" {
		t.Fatalf("read back %q", got)
	}
	if _, err := s.Open("../etc/passwd"); err == nil {
		t.Fatal("a key must not reach outside the store")
	}
	if _, _, _, err := s.Save(bytes.NewReader(make([]byte, 101)), 100); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize save = %v", err)
	}
	// The oversize upload left nothing behind; an unreferenced old file goes.
	old, _, _, _ := s.Save(bytes.NewReader([]byte("old")), 100)
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(filepath.Join(s.dir, old), past, past)
	_ = os.Chtimes(filepath.Join(s.dir, key), past, past)
	n, err := s.Sweep(map[string]bool{key: true}, time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("Sweep removed %d, %v", n, err)
	}
	entries, _ := os.ReadDir(s.dir)
	if len(entries) != 1 || entries[0].Name() != key {
		t.Fatalf("left %v", entries)
	}
}
