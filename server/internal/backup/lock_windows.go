//go:build windows

package backup

import (
	"errors"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION.
const errSharingViolation syscall.Errno = 32

// lockFile opens the file sharing it with nobody, which Windows holds
// until the handle closes or the process ends.
func lockFile(path string) (func(), error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errSharingViolation) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() { _ = syscall.CloseHandle(h) }, nil
}
