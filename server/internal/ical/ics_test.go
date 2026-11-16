package ical

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// prop is one content line read back: its name with any parameters, and
// its value as written.
type prop struct{ name, value string }

// parse reads an iCalendar body back the way a calendar app does, failing
// the test on anything RFC 5545 forbids: a line not ending in CRLF, a
// physical line over 75 octets, a fold that splits a character, a line
// with no colon or unbalanced BEGIN and END.
func parse(t *testing.T, body []byte) []prop {
	t.Helper()
	if !bytes.HasSuffix(body, []byte("\r\n")) {
		t.Fatal("the body does not end in CRLF")
	}
	var logical []string
	for i, l := range strings.Split(strings.TrimSuffix(string(body), "\r\n"), "\r\n") {
		if strings.ContainsAny(l, "\r\n") {
			t.Fatalf("line %d has a bare CR or LF: %q", i, l)
		}
		if len(l) > maxLine {
			t.Fatalf("line %d is %d octets: %q", i, len(l), l)
		}
		if !utf8.ValidString(l) {
			t.Fatalf("line %d splits a character: %q", i, l)
		}
		if strings.HasPrefix(l, " ") {
			if len(logical) == 0 {
				t.Fatal("the body starts with a continuation line")
			}
			logical[len(logical)-1] += l[1:]
			continue
		}
		logical = append(logical, l)
	}
	var out []prop
	var open []string
	for _, l := range logical {
		name, value, ok := strings.Cut(l, ":")
		if !ok {
			t.Fatalf("no colon in %q", l)
		}
		switch name {
		case "BEGIN":
			open = append(open, value)
		case "END":
			if len(open) == 0 || open[len(open)-1] != value {
				t.Fatalf("END:%s does not close %v", value, open)
			}
			open = open[:len(open)-1]
		}
		out = append(out, prop{name, value})
	}
	if len(open) > 0 {
		t.Fatalf("not closed: %v", open)
	}
	return out
}

// unescape reads a TEXT value.
func unescape(t *testing.T, s string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ',' || c == ';' {
			t.Fatalf("unescaped %q in %q", c, s)
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i == len(s) {
			t.Fatalf("a lone backslash ends %q", s)
		}
		switch s[i] {
		case 'n', 'N':
			b.WriteByte('\n')
		case '\\', ';', ',':
			b.WriteByte(s[i])
		default:
			t.Fatalf("unknown escape \\%c in %q", s[i], s)
		}
	}
	return b.String()
}

func TestTextRoundTrips(t *testing.T) {
	values := []string{
		"",
		"Plain",
		`Semi; colon, comma \ backslash`,
		"Line one\nline two\r\nline three",
		strings.Repeat("ab", 100),
		strings.Repeat("é", 80),                // two octets each
		strings.Repeat("日本語", 40),              // three octets each
		strings.Repeat("😀x", 40),               // four octets, then one
		"x" + strings.Repeat("\\", 60) + "y,z", // escapes that double near a fold
	}
	for _, v := range values {
		var w writer
		w.text("SUMMARY", v)
		w.text("X-LONG-PROPERTY-NAME-THAT-TAKES-ROOM", v)
		props := parse(t, w.b.Bytes())
		for _, p := range props {
			if got := unescape(t, p.value); got != strings.ReplaceAll(v, "\r\n", "\n") {
				t.Errorf("%s round trip: got %q, want %q", p.name, got, v)
			}
		}
	}
	// Control characters TEXT cannot hold are dropped; tabs stay.
	var w writer
	w.text("SUMMARY", "a\x00b\x1bc\td\x7f")
	if got := unescape(t, parse(t, w.b.Bytes())[0].value); got != "abc\td" {
		t.Errorf("got %q", got)
	}
}

func TestFoldsAtExactly75Octets(t *testing.T) {
	var w writer
	w.line("X", strings.Repeat("a", 200))
	lines := strings.Split(strings.TrimSuffix(w.b.String(), "\r\n"), "\r\n")
	if len(lines[0]) != 75 || len(lines[1]) != 75 || lines[1][0] != ' ' {
		t.Fatalf("lines = %q", lines)
	}
	// A line of exactly 75 octets is not folded.
	w = writer{}
	w.line("X", strings.Repeat("a", 73))
	if strings.Count(w.b.String(), "\r\n") != 1 {
		t.Fatalf("folded a line that fits: %q", w.b.String())
	}
}

func TestOffsets(t *testing.T) {
	for sec, want := range map[int]string{0: "+0000", -5 * 3600: "-0500", 19800: "+0530", -(3*3600 + 30*60): "-0330", 3600 + 45: "+010045"} {
		if got := offset(sec); got != want {
			t.Errorf("offset(%d) = %q, want %q", sec, got, want)
		}
	}
}

// zone writes a VTIMEZONE and returns its lines without BEGIN/END of the
// zone itself.
func zone(t *testing.T, name string, first int) []prop {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	var w writer
	vtimezone(&w, loc, first)
	return parse(t, w.b.Bytes())
}

func joined(props []prop) string {
	var b strings.Builder
	for _, p := range props {
		fmt.Fprintf(&b, "%s:%s\n", p.name, p.value)
	}
	return b.String()
}

func TestVTimezone(t *testing.T) {
	cases := map[string][]string{
		// The second Sunday of March, the first of November.
		"America/Chicago": {
			"TZID:America/Chicago",
			"BEGIN:DAYLIGHT\nDTSTART:20250309T020000\nTZOFFSETFROM:-0600\nTZOFFSETTO:-0500\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU\nTZNAME:CDT\nEND:DAYLIGHT",
			"BEGIN:STANDARD\nDTSTART:20251102T020000\nTZOFFSETFROM:-0500\nTZOFFSETTO:-0600\nRRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU\nTZNAME:CST\nEND:STANDARD",
		},
		// The last Sunday, which is the fourth some years and the fifth others.
		"Europe/Berlin": {
			"BEGIN:DAYLIGHT\nDTSTART:20250330T020000\nTZOFFSETFROM:+0100\nTZOFFSETTO:+0200\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU\nTZNAME:CEST",
			"BEGIN:STANDARD\nDTSTART:20251026T030000\nTZOFFSETFROM:+0200\nTZOFFSETTO:+0100\nRRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU\nTZNAME:CET",
		},
		// Southern summer, and a half-hour change.
		"Australia/Lord_Howe": {
			"BEGIN:DAYLIGHT\nDTSTART:20251005T020000\nTZOFFSETFROM:+1030\nTZOFFSETTO:+1100\nRRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=1SU",
			"BEGIN:STANDARD\nDTSTART:20250406T020000\nTZOFFSETFROM:+1100\nTZOFFSETTO:+1030\nRRULE:FREQ=YEARLY;BYMONTH=4;BYDAY=1SU",
		},
		// No changes at all.
		"Asia/Kolkata": {
			"BEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETFROM:+0530\nTZOFFSETTO:+0530\nTZNAME:IST\nEND:STANDARD",
		},
	}
	for name, want := range cases {
		got := joined(zone(t, name, 2025))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing\n%s\nin\n%s", name, w, got)
			}
		}
	}
	// Mexico City gave up daylight saving after October 2022: from a
	// window starting in 2021 its rules end there, so no client keeps
	// moving summer an hour.
	got := joined(zone(t, "America/Mexico_City", 2021))
	for _, w := range []string{
		"RRULE:FREQ=YEARLY;BYMONTH=4;BYDAY=1SU;UNTIL=20220403T080000Z\n",
		"RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20221030T070000Z\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("Mexico City: missing %s in\n%s", w, got)
		}
	}
	// Changes in a fixed week of the month are a rule too, so they hold
	// past the years looked at: the Friday before the last Sunday of
	// March, and Chile's Sunday on or after the 2nd.
	for name, want := range map[string][]string{
		"Asia/Jerusalem": {
			"BEGIN:DAYLIGHT\nDTSTART:20250328T020000\nTZOFFSETFROM:+0200\nTZOFFSETTO:+0300\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29\n",
			"RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU\n",
		},
		"America/Santiago": {
			"RRULE:FREQ=YEARLY;BYMONTH=9;BYDAY=SU;BYMONTHDAY=2,3,4,5,6,7,8\n",
			"RRULE:FREQ=YEARLY;BYMONTH=4;BYDAY=SU;BYMONTHDAY=2,3,4,5,6,7,8\n",
		},
	} {
		got := joined(zone(t, name, 2025))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing\n%s\nin\n%s", name, w, got)
			}
		}
		if n := strings.Count(got, "BEGIN:DAYLIGHT") + strings.Count(got, "BEGIN:STANDARD"); n != 2 {
			t.Errorf("%s: %d observances, want 2:\n%s", name, n, got)
		}
	}
	// A zone with no DAYLIGHT part has exactly one STANDARD.
	if got := joined(zone(t, "Asia/Kolkata", 2025)); strings.Count(got, "BEGIN:STANDARD") != 1 || strings.Contains(got, "DAYLIGHT") {
		t.Errorf("Kolkata:\n%s", got)
	}
}

func TestYearlyRuleRefusesIrregularChanges(t *testing.T) {
	at := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339, s); return tm }
	// Two years on different weekdays of the month: no rule fits.
	group := []transition{
		{at: at("2025-03-09T08:00:00Z"), from: -21600, to: -18000},
		{at: at("2026-03-14T08:00:00Z"), from: -21600, to: -18000},
	}
	if rule, ok := yearlyRule(group, 2037); ok {
		t.Fatalf("got %q", rule)
	}
	// A gap of a year: not every year.
	group = []transition{
		{at: at("2025-03-09T08:00:00Z"), from: -21600, to: -18000},
		{at: at("2027-03-14T08:00:00Z"), from: -21600, to: -18000},
	}
	if rule, ok := yearlyRule(group, 2037); ok {
		t.Fatalf("got %q", rule)
	}
	// The same weekday, the second one and then the first: some week
	// holds both, but which is not known yet.
	group = []transition{
		{at: at("2030-03-08T00:00:00Z"), from: 7200, to: 10800},
		{at: at("2031-03-07T00:00:00Z"), from: 7200, to: 10800},
	}
	if rule, ok := yearlyRule(group, 2031); ok {
		t.Fatalf("got %q", rule)
	}
}
