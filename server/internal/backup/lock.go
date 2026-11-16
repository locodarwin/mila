package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrLocked means another Mila process is using the data directory: the
// server, or a restore or a migrate-db under way.
var ErrLocked = errors.New("another Mila process is using the data directory")

// Lock takes the data directory for this process until the returned
// function is called or the process ends: the server holds it while it
// runs, and a restore or a move to another database needs it, since
// either would pull the database out from under a running server. The
// operating system lets the lock go when the process dies, so a crash
// leaves nothing to clean up.
func Lock(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "mila.lock")
	unlock, err := lockFile(path)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return nil, fmt.Errorf("%w (%s): stop the server first", ErrLocked, dataDir)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return unlock, nil
}
