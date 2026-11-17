// Package keyfile seals the secrets Mila keeps in its database with a key
// kept apart from it, in the data directory (the
// Microsoft mailbox's sign-in), so the database alone, and so every backup of it,
// holds nothing usable. The key is made the first time something is sealed:
// 32 random bytes, base64, in <data dir>/secret.key, readable by its owner
// alone. A sealed value is AES-256-GCM over the secret, bound to what it is
// for, so one can't stand in for another.
package keyfile

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Name is the key's file in the data directory.
const Name = "secret.key"

// prefix marks a sealed value, and its version.
const prefix = "sealed:v1:"

// ErrUnreadable means a sealed value can't be opened here: the key that
// sealed it is missing or another, as after restoring the database on
// another server, or the value was changed.
var ErrUnreadable = errors.New("sealed with a key this Mila does not have")

// Key is the data directory's key, read or made when first needed.
type Key struct {
	dir  string
	mu   sync.Mutex
	aead cipher.AEAD
}

// At is the key in dir, the data directory. Nothing is read or made yet.
func At(dir string) *Key { return &Key{dir: dir} }

// Path is where the key is kept.
func (k *Key) Path() string { return filepath.Join(k.dir, Name) }

// IsSealed says whether s is a sealed value rather than one stored before
// sealing.
func IsSealed(s string) bool { return strings.HasPrefix(s, prefix) }

// Seal seals secret for purpose, making the key if there is none yet.
func (k *Key) Seal(secret, purpose string) (string, error) {
	aead, err := k.cipher(true)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := aead.Seal(nonce, nonce, []byte(secret), []byte(purpose))
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open is the secret sealed for purpose. It never makes a key: without one,
// or with another, it is ErrUnreadable.
func (k *Key) Open(sealed, purpose string) (string, error) {
	body, ok := strings.CutPrefix(sealed, prefix)
	if !ok {
		return "", fmt.Errorf("keyfile: not a sealed value")
	}
	raw, err := base64.RawStdEncoding.DecodeString(body)
	if err != nil {
		return "", ErrUnreadable
	}
	aead, err := k.cipher(false)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrUnreadable
	}
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", ErrUnreadable
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(purpose))
	if err != nil {
		return "", ErrUnreadable
	}
	return string(plain), nil
}

// cipher is the key's AEAD, read from its file, or made when orMake is set
// and there is no file.
func (k *Key) cipher(orMake bool) (cipher.AEAD, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.aead != nil {
		return k.aead, nil
	}
	key, err := k.read()
	if errors.Is(err, fs.ErrNotExist) && orMake {
		key, err = k.create()
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	k.aead = aead
	return aead, nil
}

// Check says, before a secret is worth asking for (mila mailbox connect),
// whether one could be sealed here: the key is there and readable, or this
// process may make one.
func (k *Key) Check() error {
	_, err := k.read()
	if errors.Is(err, fs.ErrNotExist) {
		return k.mayMake()
	}
	return err
}

// Stand-ins for tests.
var (
	link = os.Link
	euid = os.Geteuid
)

// mayMake refuses to make the key as another user than the data
// directory's owner: run as root beside a server running as mila, it would
// make a key the server can't read, and the sign-in sealed with it would
// be lost to the server.
func (k *Key) mayMake() error {
	owner, ok := dirOwner(k.dir)
	if !ok {
		return nil
	}
	if me := euid(); me >= 0 && me != owner {
		return fmt.Errorf("%s belongs to user %d, but this runs as user %d, so the key it would make there (%s) is one the server couldn't read: run it as the server's user, as with sudo -u mila", k.dir, owner, me, Name)
	}
	return nil
}

func (k *Key) read() ([]byte, error) {
	text, err := os.ReadFile(k.Path())
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(text)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%s is not a key Mila made: it should be 32 bytes in base64", k.Path())
	}
	return key, nil
}

// create makes the key file whole or not at all: written under another name,
// then linked into place, so the server and a command run beside it (mila
// mailbox connect) can't each make a different one. Whichever links first
// wins, and the other reads it. Where the filesystem has no hard links, it
// is made in place instead, by whoever creates the file first.
func (k *Key) create() ([]byte, error) {
	if err := k.mayMake(); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(k.dir, 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(k.dir, ".secret.key-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	_, err = tmp.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if err := link(tmp.Name(), k.Path()); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return k.read()
		}
		return k.createInPlace(key)
	}
	syncDir(k.dir)
	return key, nil
}

// createInPlace makes the key file where it goes, for a filesystem without
// hard links: created only if there is none (O_EXCL), so the first process
// wins as with a link. Another that finds it may read it mid-write, so it
// reads again for a moment before calling it broken.
func (k *Key) createInPlace(key []byte) ([]byte, error) {
	f, err := os.OpenFile(k.Path(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		for range 20 {
			if got, err := k.read(); err == nil {
				return got, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		return k.read()
	}
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		// Half a key would be a broken one for good: take it away.
		_ = os.Remove(k.Path())
		return nil, err
	}
	syncDir(k.dir)
	return key, nil
}

// syncDir makes a new name in dir last through a crash, so nothing is
// sealed with a key the disk then forgets. Not every system can.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
