//go:build !(linux || darwin || freebsd || openbsd || netbsd || dragonfly || windows)

package backup

// lockFile does nothing where the system offers no lock Mila knows how
// to take; stopping the server first is then up to the operator.
func lockFile(string) (func(), error) { return func() {}, nil }
