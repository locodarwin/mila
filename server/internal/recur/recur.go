// Package recur reads a task's repeat rule and works out when it comes
// back. A rule is a small subset of RFC 5545 RRULE: FREQ (DAILY, WEEKLY,
// MONTHLY or YEARLY), INTERVAL, BYDAY (weekdays for WEEKLY; one ordinal
// weekday such as 1MO or -1FR for MONTHLY), BYMONTHDAY (MONTHLY; -1 is the
// last day) and UNTIL (a date). Anything else is refused, so every rule
// Mila stores means the same thing here, in the browser
// (web/src/lib/recurrence.ts) and in a calendar app reading the iCal feed.
//
// Dates are calendar dates with no time zone. The shared vectors in
// shared/recurrence-vectors.json hold both implementations to the same
// answers.
package recur

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/ids"
)

// Modes: the next date follows the rule's schedule, or comes a set time
// after the task was completed.
const (
	Schedule   = "schedule"
	Completion = "completion"
)

// MaxInterval bounds INTERVAL.
const MaxInterval = 999

// Rule is a parsed repeat rule.
type Rule struct {
	Freq     string         // DAILY, WEEKLY, MONTHLY or YEARLY
	Interval int            // 1 or more
	Weekdays []time.Weekday // WEEKLY: the days of the week, in order from Monday
	Ordinal  int            // MONTHLY BYDAY: 1 to 5, or -1 to -5 from the end; 0 if unset
	Weekday  time.Weekday   // MONTHLY BYDAY: the weekday Ordinal counts
	MonthDay int            // MONTHLY BYMONTHDAY: 1 to 31, or -1 for the last day; 0 if unset
	Until    string         // the last date it may fall on, "2026-12-31", or ""
}

var dayCodes = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

var (
	ordinalDay = regexp.MustCompile(`^(-?[1-5])(MO|TU|WE|TH|FR|SA|SU)$`)
	number     = regexp.MustCompile(`^-?[0-9]{1,3}$`)
	untilDate  = regexp.MustCompile(`^[0-9]{8}$`)
)

// Parse reads a rule, refusing anything outside the subset.
func Parse(s string) (Rule, error) {
	r := Rule{Interval: 1}
	if s == "" {
		return r, errors.New("is empty")
	}
	seen := map[string]bool{}
	for part := range strings.SplitSeq(s, ";") {
		key, val, ok := strings.Cut(part, "=")
		if !ok || val == "" {
			return r, fmt.Errorf("%q is not KEY=VALUE", part)
		}
		if seen[key] {
			return r, fmt.Errorf("%s is given twice", key)
		}
		seen[key] = true
		switch key {
		case "FREQ":
			if !slices.Contains([]string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY"}, val) {
				return r, fmt.Errorf("FREQ %q is not DAILY, WEEKLY, MONTHLY or YEARLY", val)
			}
			r.Freq = val
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || !number.MatchString(val) || n < 1 || n > MaxInterval {
				return r, fmt.Errorf("INTERVAL must be a whole number from 1 to %d", MaxInterval)
			}
			r.Interval = n
		case "BYDAY":
			if m := ordinalDay.FindStringSubmatch(val); m != nil {
				r.Ordinal, _ = strconv.Atoi(m[1])
				r.Weekday = dayCodes[m[2]]
				continue
			}
			for code := range strings.SplitSeq(val, ",") {
				d, ok := dayCodes[code]
				if !ok {
					return r, fmt.Errorf("BYDAY %q is not a weekday like MO", code)
				}
				if slices.Contains(r.Weekdays, d) {
					return r, fmt.Errorf("BYDAY names %s twice", code)
				}
				r.Weekdays = append(r.Weekdays, d)
			}
			slices.SortFunc(r.Weekdays, func(a, b time.Weekday) int { return fromMonday(a) - fromMonday(b) })
		case "BYMONTHDAY":
			n, err := strconv.Atoi(val)
			if err != nil || !number.MatchString(val) || n == 0 || n < -1 || n > 31 {
				return r, errors.New("BYMONTHDAY must be a day from 1 to 31, or -1 for the last day")
			}
			r.MonthDay = n
		case "UNTIL":
			t, err := time.Parse("20060102", val)
			if err != nil || !untilDate.MatchString(val) {
				return r, errors.New("UNTIL must be a date like 20261231")
			}
			r.Until = t.Format(time.DateOnly)
		default:
			return r, fmt.Errorf("%s is not supported", key)
		}
	}
	switch {
	case r.Freq == "":
		return r, errors.New("FREQ is missing")
	case len(r.Weekdays) > 0 && r.Freq != "WEEKLY":
		return r, errors.New("a list of weekdays needs FREQ=WEEKLY")
	case r.Ordinal != 0 && r.Freq != "MONTHLY":
		return r, errors.New("an ordinal weekday like 1MO needs FREQ=MONTHLY")
	case r.MonthDay != 0 && r.Freq != "MONTHLY":
		return r, errors.New("BYMONTHDAY needs FREQ=MONTHLY")
	case r.MonthDay != 0 && r.Ordinal != 0:
		return r, errors.New("BYDAY and BYMONTHDAY cannot be combined")
	}
	return r, nil
}

// Validate reports whether s is a rule Mila understands.
func Validate(s string) error {
	_, err := Parse(s)
	return err
}

func fromMonday(d time.Weekday) int { return (int(d) + 6) % 7 }

// date is a calendar date at midnight UTC, so adding days is plain
// arithmetic with no daylight saving to trip over.
type date = time.Time

func parseDate(s string) (date, error) { return time.Parse(time.DateOnly, s) }

func mkdate(y int, m time.Month, d int) date { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func daysIn(y int, m time.Month) int { return mkdate(y, m+1, 0).Day() }

func addDays(d date, n int) date { return d.AddDate(0, 0, n) }

func daysBetween(a, b date) int { return int(b.Sub(a).Hours() / 24) }

// monday is the Monday on or before d; weeks start on Monday (WKST=MO).
func monday(d date) date { return addDays(d, -fromMonday(d.Weekday())) }

// periods bounds how many months or years are tried before deciding a
// rule never fires again (BYMONTHDAY=31 every 12 months from April).
const periods = 200

// After returns the rule's first date after both `after` and the anchor
// (the date the series counts from, like an RRULE's DTSTART), ignoring
// UNTIL. It reports false if there is none.
func (r Rule) After(anchor, after date) (date, bool) {
	if after.Before(anchor) {
		after = anchor
	}
	switch r.Freq {
	case "DAILY":
		k := daysBetween(anchor, after)/r.Interval + 1
		return addDays(anchor, k*r.Interval), true
	case "WEEKLY":
		days := r.Weekdays
		if len(days) == 0 {
			days = []time.Weekday{anchor.Weekday()}
		}
		start := monday(anchor)
		for d := addDays(after, 1); daysBetween(after, d) <= 14*r.Interval; d = addDays(d, 1) {
			week := daysBetween(start, monday(d)) / 7
			if week%r.Interval == 0 && slices.Contains(days, d.Weekday()) {
				return d, true
			}
		}
		return date{}, false
	case "MONTHLY":
		first := anchor.Year()*12 + int(anchor.Month()) - 1
		cur := after.Year()*12 + int(after.Month()) - 1
		if off := (cur - first) % r.Interval; off != 0 {
			cur += r.Interval - off
		}
		for range periods {
			y, m := cur/12, time.Month(cur%12+1)
			if d, ok := r.inMonth(anchor, y, m); ok && d.After(after) {
				return d, true
			}
			cur += r.Interval
		}
		return date{}, false
	case "YEARLY":
		y := after.Year()
		if off := (y - anchor.Year()) % r.Interval; off != 0 {
			y += r.Interval - off
		}
		for range periods {
			if anchor.Day() <= daysIn(y, anchor.Month()) {
				if d := mkdate(y, anchor.Month(), anchor.Day()); d.After(after) {
					return d, true
				}
			}
			y += r.Interval
		}
	}
	return date{}, false
}

// inMonth is the day a monthly rule falls on in a month, if it has one
// there: a month too short for the day is skipped, as RFC 5545 says.
func (r Rule) inMonth(anchor date, y int, m time.Month) (date, bool) {
	n := daysIn(y, m)
	switch {
	case r.MonthDay == -1:
		return mkdate(y, m, n), true
	case r.Ordinal > 0:
		first := (int(r.Weekday) - int(mkdate(y, m, 1).Weekday()) + 7) % 7
		day := 1 + first + (r.Ordinal-1)*7
		return mkdate(y, m, day), day <= n
	case r.Ordinal < 0:
		last := (int(mkdate(y, m, n).Weekday()) - int(r.Weekday) + 7) % 7
		day := n - last + (r.Ordinal+1)*7
		return mkdate(y, m, day), day >= 1
	}
	day := r.MonthDay
	if day == 0 {
		day = anchor.Day()
	}
	return mkdate(y, m, day), day <= n
}

// Next is the due date of the task that follows one completed on `today`,
// or "" when the series has ended. due is the completed task's due date
// ("" if it had none, in which case the completion date stands in).
//
// In schedule mode the next date is the rule's next one after the due
// date, skipping any before today, so finishing late does not leave a
// pile of overdue copies. In completion mode it is the completion date
// plus the interval; only FREQ, INTERVAL and UNTIL apply.
func Next(rule, mode, due, today string) (string, error) {
	r, err := Parse(rule)
	if err != nil {
		return "", err
	}
	now, err := parseDate(today)
	if err != nil {
		return "", err
	}
	var next date
	if mode == Completion {
		next = r.addInterval(now)
	} else {
		base := now
		if due != "" {
			if base, err = parseDate(due); err != nil {
				return "", err
			}
		}
		after := addDays(now, -1)
		if base.After(after) {
			after = base
		}
		var ok bool
		if next, ok = r.After(base, after); !ok {
			return "", nil
		}
	}
	if r.Until != "" && next.Format(time.DateOnly) > r.Until {
		return "", nil
	}
	return next.Format(time.DateOnly), nil
}

// addInterval moves a date on by one interval. A month or year that lacks
// the day ends on its last day instead (January 31 plus a month is the end
// of February).
func (r Rule) addInterval(d date) date {
	switch r.Freq {
	case "DAILY":
		return addDays(d, r.Interval)
	case "WEEKLY":
		return addDays(d, 7*r.Interval)
	}
	months := r.Interval
	if r.Freq == "YEARLY" {
		months *= 12
	}
	total := d.Year()*12 + int(d.Month()) - 1 + months
	y, m := total/12, time.Month(total%12+1)
	return mkdate(y, m, min(d.Day(), daysIn(y, m)))
}

// NextTaskID is the ID of the task that follows a completed one: a
// UUID-shaped hash (version 8, RFC 9562) of the completed task alone, so
// devices that complete the same task offline make the same next task
// instead of two, whatever day or timezone each completes it in. It comes
// from the completed task, not the series, so a copy never gets the ID of
// the task before it.
func NextTaskID(after string) string {
	return HashID(after + "/next")
}

// HashID is a UUID-shaped (version 8) SHA-256 of a string.
func HashID(s string) string { return ids.HashID(s) }
