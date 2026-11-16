package ical

import (
	"bytes"
	"strings"
	"time"
	"unicode/utf8"
)

// The writing half of RFC 5545: content lines end in CRLF and are folded
// at 75 octets, and text values are escaped.

// maxLine is the longest a content line may be, in octets, without its
// CRLF.
const maxLine = 75

// writer builds an iCalendar body one content line at a time.
type writer struct{ b bytes.Buffer }

// line writes one content line, folding it: a long line goes on in
// continuation lines that start with a space, never splitting a UTF-8
// character.
func (w *writer) line(name, value string) {
	s := name + ":" + value
	limit := maxLine
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		w.b.WriteString(s[:cut])
		w.b.WriteString("\r\n ")
		s = s[cut:]
		limit = maxLine - 1 // the leading space counts
	}
	w.b.WriteString(s)
	w.b.WriteString("\r\n")
}

// text writes a property whose value is TEXT.
func (w *writer) text(name, value string) { w.line(name, escape(value)) }

// escape makes a TEXT value: backslashes, semicolons and commas are
// escaped, line breaks become \n, and other control characters, which
// TEXT may not hold, are dropped.
func escape(s string) string {
	var b strings.Builder
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, r := range s {
		switch {
		case r == '\\', r == ';', r == ',':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n', r == '\r':
			b.WriteString(`\n`)
		case r < 0x20 && r != '\t', r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// utc is a DATE-TIME in UTC: "20261028T193000Z".
func utc(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// local is a DATE-TIME without a zone, to go with a TZID: "20261028T143000".
func local(t time.Time) string { return t.Format("20060102T150405") }

// day is a DATE: "20261028".
func day(t time.Time) string { return t.Format("20060102") }
