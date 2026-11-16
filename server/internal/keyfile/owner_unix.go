//go:build unix

package keyfile

import (
	"os"
	"path/filepath"
	"syscall"
)

// dirOwner is the user owning dir, or the nearest folder above it that
// exists, since the key's folder is made when missing.
func dirOwner(dir string) (int, bool) {
	for {
		info, err := os.Stat(dir)
		if err == nil {
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return 0, false
			}
			return int(st.Uid), true
		}
		up := filepath.Dir(dir)
		if up == dir {
			return 0, false
		}
		dir = up
	}
}
