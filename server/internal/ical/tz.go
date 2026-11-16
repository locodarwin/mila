package ical

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A repeating task with a due time goes out in its owner's time zone
// (DTSTART;TZID=...), not UTC: a weekly 09:00 must stay at 09:00 across
// daylight saving, and on the right weekday when UTC is already on the
// next day. RFC 5545 wants a VTIMEZONE for every TZID, which this writes
// from Go's zone data.

// transition is a change of UTC offset.
type transition struct {
	at       time.Time // the instant it happens
	from, to int       // offsets in seconds
	name     string    // the zone's abbreviation after it, such as "CDT"
}

// transitions finds the offset changes from the start of year first to
// the end of year last.
func transitions(loc *time.Location, first, last int) []transition {
	var out []transition
	t := time.Date(first, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(last+1, 1, 1, 0, 0, 0, 0, time.UTC)
	_, off := t.In(loc).Zone()
	for t.Before(end) {
		next := t.Add(24 * time.Hour)
		if _, o := next.In(loc).Zone(); o != off {
			// Narrow it to the second.
			lo, hi := t, next
			for hi.Sub(lo) > time.Second {
				mid := lo.Add(hi.Sub(lo) / 2)
				if _, o := mid.In(loc).Zone(); o == off {
					lo = mid
				} else {
					hi = mid
				}
			}
			name, to := hi.In(loc).Zone()
			out = append(out, transition{at: hi, from: off, to: to, name: name})
			off = to
		}
		t = next
	}
	return out
}

// observance is one STANDARD or DAYLIGHT part of a VTIMEZONE.
type observance struct {
	daylight bool
	start    string // local time in the offset before it
	from, to int
	name     string
	rrule    string // a yearly rule, or "" for this one change only
}

// vtimezone writes the zone's definition, covering the years from first
// on. Changes that follow a yearly weekday rule (the second Sunday of
// March) are written as that rule, so they hold beyond the years looked
// at, or until the last of them when the zone stopped changing; any
// others are listed one by one.
func vtimezone(w *writer, loc *time.Location, first int) {
	const span = 12
	all := transitions(loc, first, first+span)
	w.line("BEGIN", "VTIMEZONE")
	w.line("TZID", loc.String())
	if len(all) == 0 {
		name, off := time.Date(first, 1, 1, 0, 0, 0, 0, loc).Zone()
		writeObservance(w, observance{start: "19700101T000000", from: off, to: off, name: name})
		w.line("END", "VTIMEZONE")
		return
	}
	// Changes by kind: into daylight time (a bigger offset) or out of it.
	var ruled []observance
	var rest []transition
	for _, daylight := range []bool{true, false} {
		var group []transition
		for _, t := range all {
			if (t.to > t.from) == daylight {
				group = append(group, t)
			}
		}
		if len(group) == 0 {
			continue
		}
		if rule, ok := yearlyRule(group, first+span); ok {
			g := group[0]
			ruled = append(ruled, observance{daylight: daylight, start: local(g.at.Add(time.Duration(g.from) * time.Second).UTC()),
				from: g.from, to: g.to, name: g.name, rrule: rule})
			continue
		}
		rest = append(rest, group...)
	}
	slices.SortFunc(rest, func(a, b transition) int { return a.at.Compare(b.at) })
	if len(rest) > 0 && len(ruled) == 0 {
		// Before the first change listed, the zone had that change's from
		// offset.
		g := rest[0]
		start := time.Date(first-1, 1, 1, 0, 0, 0, 0, time.UTC)
		writeObservance(w, observance{start: local(start), from: g.from, to: g.from,
			name: offsetName(g.from)})
	}
	for _, o := range ruled {
		writeObservance(w, o)
	}
	for _, t := range rest {
		writeObservance(w, observance{daylight: t.to > t.from, start: local(t.at.Add(time.Duration(t.from) * time.Second).UTC()),
			from: t.from, to: t.to, name: t.name})
	}
	w.line("END", "VTIMEZONE")
}

func writeObservance(w *writer, o observance) {
	kind := "STANDARD"
	if o.daylight {
		kind = "DAYLIGHT"
	}
	w.line("BEGIN", kind)
	w.line("DTSTART", o.start)
	w.line("TZOFFSETFROM", offset(o.from))
	w.line("TZOFFSETTO", offset(o.to))
	if o.rrule != "" {
		w.line("RRULE", o.rrule)
	}
	if o.name != "" {
		w.text("TZNAME", o.name)
	}
	w.line("END", kind)
}

// yearlyRule describes changes that fall once a year on the same month,
// local time and weekday as an RRULE: the nth or last such weekday, or
// the one in a fixed week of the month (the Friday from the 23rd to the
// 29th, the Sunday on or after the 2nd). Changes that stop before the
// last year looked at, last, end in an UNTIL, so a zone that gave up
// daylight saving is not described as keeping it.
func yearlyRule(group []transition, last int) (string, bool) {
	if len(group) < 2 {
		return "", false
	}
	type when struct {
		month   time.Month
		weekday time.Weekday
		clock   string
		from    int
		to      int
	}
	var first when
	nth, isLast := true, true
	n, lo, hi := 0, 31, 1
	for i, t := range group {
		l := t.at.Add(time.Duration(t.from) * time.Second).UTC()
		w := when{l.Month(), l.Weekday(), l.Format("150405"), t.from, t.to}
		if i == 0 {
			first, n = w, (l.Day()-1)/7+1
		} else if w != first || group[i-1].at.Year()+1 != t.at.Year() {
			return "", false
		}
		nth = nth && (l.Day()-1)/7+1 == n
		isLast = isLast && l.Day()+7 > time.Date(l.Year(), l.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		lo, hi = min(lo, l.Day()), max(hi, l.Day())
	}
	var rule string
	switch {
	case nth && n < 5:
		rule = fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%d%s", first.month, n, weekdays[first.weekday])
	case isLast:
		rule = fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=-1%s", first.month, weekdays[first.weekday])
	case hi-lo == 6:
		// Every day of one week was seen, so the week is known; with fewer
		// it could be any of several.
		days := make([]string, 7)
		for i := range days {
			days[i] = strconv.Itoa(lo + i)
		}
		rule = fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%s;BYMONTHDAY=%s", first.month, weekdays[first.weekday], strings.Join(days, ","))
	default:
		return "", false
	}
	if end := group[len(group)-1].at; end.Year() < last {
		rule += ";UNTIL=" + utc(end)
	}
	return rule, true
}

// offset is a UTC-OFFSET: "-0500", or "+0530", with seconds when it has
// them.
func offset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	s := fmt.Sprintf("%s%02d%02d", sign, sec/3600, sec/60%60)
	if sec%60 != 0 {
		s += fmt.Sprintf("%02d", sec%60)
	}
	return s
}

func offsetName(sec int) string {
	if sec == 0 {
		return "UTC"
	}
	return "UTC" + offset(sec)
}

var weekdays = map[time.Weekday]string{
	time.Monday: "MO", time.Tuesday: "TU", time.Wednesday: "WE", time.Thursday: "TH",
	time.Friday: "FR", time.Saturday: "SA", time.Sunday: "SU",
}
