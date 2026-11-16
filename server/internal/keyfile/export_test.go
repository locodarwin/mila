package keyfile

// SetLink stands in for os.Link until the returned func puts it back.
func SetLink(f func(oldname, newname string) error) func() {
	was := link
	link = f
	return func() { link = was }
}

// SetEUID stands in for os.Geteuid until the returned func puts it back.
func SetEUID(f func() int) func() {
	was := euid
	euid = f
	return func() { euid = was }
}
