package files

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// CleanName is a file's name as Mila keeps it: its last part only, with no
// control characters, at most 255 characters.
func CleanName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if utf8.RuneCountInString(name) > 255 {
		name = string([]rune(name)[:255])
	}
	return name
}

// MediaType trusts the type a file was declared as only if it is well formed,
// and otherwise sniffs the bytes.
func MediaType(declared string, head []byte) string {
	if t, _, err := mime.ParseMediaType(declared); err == nil && t != "application/octet-stream" && len(t) <= 255 {
		return t
	}
	t, _, _ := mime.ParseMediaType(http.DetectContentType(head))
	return t
}
