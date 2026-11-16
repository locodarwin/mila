// Package files keeps uploaded attachment files and people's pictures on
// local disk, each under a random key. Rows in the database name the
// keys; files nothing names are swept away.
package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// ErrTooLarge means an upload passed the size limit.
var ErrTooLarge = errors.New("the file is too large")

var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Store is a directory of files named by key.
type Store struct{ dir string }

// New opens (creating if needed) a store in dir.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Save writes r to a new file of at most max bytes and returns its key,
// its size and its first 512 bytes (for sniffing the type).
func (s *Store) Save(r io.Reader, max int64) (key string, size int64, head []byte, err error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	key = hex.EncodeToString(b)
	tmp, err := os.CreateTemp(s.dir, ".upload-*")
	if err != nil {
		return "", 0, nil, err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	head = make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	head = head[:n]
	if _, err = tmp.Write(head); err != nil {
		return "", 0, nil, err
	}
	rest, err := io.Copy(tmp, io.LimitReader(r, max-int64(n)+1))
	if err != nil {
		return "", 0, nil, err
	}
	size = int64(n) + rest
	if size > max {
		return "", 0, nil, ErrTooLarge
	}
	if err = tmp.Sync(); err != nil {
		return "", 0, nil, err
	}
	if err = tmp.Close(); err != nil {
		return "", 0, nil, err
	}
	if err = os.Rename(tmp.Name(), filepath.Join(s.dir, key)); err != nil {
		return "", 0, nil, err
	}
	return key, size, head, nil
}

// Open opens the file for a key.
func (s *Store) Open(key string) (*os.File, error) {
	if !keyPattern.MatchString(key) {
		return nil, fmt.Errorf("invalid file key %q", key)
	}
	return os.Open(filepath.Join(s.dir, key))
}

// Sweep deletes files no row refers to, sparing anything younger than
// grace so an upload whose row is being written is not caught mid-way.
func (s *Store) Sweep(keep map[string]bool, grace time.Duration) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || keep[e.Name()] || time.Since(info.ModTime()) < grace {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, e.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}
