package httpapi

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Postgres refuses a NUL in text, so one in a name or a lookup would be a
// 500 there (SQLite stores it, and nothing shows one). Sync and the row
// endpoints refuse one field by field; everything else is refused here, at
// the door: in the address, and in the JSON bodies the other endpoints read.

const nulMessage = "Text can't hold a NUL character."

// refuseNULs answers 400 to a request whose path or query holds a NUL, or
// bytes that aren't UTF-8, which Postgres refuses in text just the same.
func refuseNULs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.ContainsRune(r.URL.Path, 0) || strings.ContainsRune(r.URL.RawQuery, 0) ||
			strings.Contains(r.URL.RawQuery, "%00") || strings.Contains(r.URL.RawPath, "%00") {
			writeError(w, http.StatusBadRequest, "invalid", nulMessage)
			return
		}
		if !utf8.ValidString(r.URL.Path) || !validQuery(r.URL.RawQuery) {
			writeError(w, http.StatusBadRequest, "invalid", "The address must be UTF-8 text.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validQuery says whether every name and value in a query is UTF-8 once
// unescaped. It reads the raw query part by part, so a part that doesn't
// parse (a ; or a bad escape, which url.ParseQuery skips with an error)
// never lets the others through unchecked: r.URL.Query() still gives the
// handlers those. A part that can't be unescaped is checked as it stands.
func validQuery(raw string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	for part := range strings.FieldsFuncSeq(raw, func(r rune) bool { return r == '&' || r == ';' }) {
		for side := range strings.SplitSeq(part, "=") {
			if s, err := url.QueryUnescape(side); err == nil && !utf8.ValidString(s) {
				return false
			}
		}
	}
	return true
}

// escapesNUL says whether a JSON text writes a NUL into a string. JSON can
// only write one as \u0000 (a raw one is no JSON at all), and that is an
// escape only after an even number of backslashes before its own: \\u0000
// is a backslash, then the letters u0000.
func escapesNUL(body []byte) bool {
	for from := 0; ; {
		i := bytes.Index(body[from:], []byte(`\u0000`))
		if i < 0 {
			return false
		}
		i += from
		n := 0
		for j := i - 1; j >= 0 && body[j] == '\\'; j-- {
			n++
		}
		if n%2 == 0 {
			return true
		}
		from = i + 1
	}
}
