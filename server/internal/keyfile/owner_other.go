//go:build !unix

package keyfile

// dirOwner says nothing where files have no Unix owner.
func dirOwner(string) (int, bool) { return 0, false }
