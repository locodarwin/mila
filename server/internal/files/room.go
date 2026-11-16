package files

import (
	"fmt"

	"github.com/locodarwin/mila/server/internal/apperr"
)

// MinFree is the disk space below which uploads and imports are refused,
// so that people's files cannot fill the disk the database, backups and,
// on a home server, other services need.
const MinFree = 1 << 30

// CheckRoom refuses, with a message to show, when the disk holding dir
// has less than MinFree left. A disk whose space cannot be read is let be.
func CheckRoom(dir string) error {
	free, err := freeBytes(dir)
	if err != nil || free >= MinFree {
		return nil
	}
	return apperr.New(apperr.Unavailable, "The server is almost out of disk space (%s left), so it is not taking files now. Tell whoever runs it.", humanBytes(free))
}

func humanBytes(n uint64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", n>>10)
}

// Room is CheckRoom for the store's own disk.
func (s *Store) Room() error { return CheckRoom(s.dir) }
