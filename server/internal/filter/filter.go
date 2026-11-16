// Package filter is the server's copy of the saved-view filter in
// web/src/lib/filter.ts, for calendar feeds of a view. The two must agree
// exactly, down to how text is trimmed and lowercased;
// shared/filter-vectors.json holds both to the same cases.
package filter

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Filter is what a saved view shows (its config's "filter"). Every part is
// optional; an empty filter shows every open task.
type Filter struct {
	Text     string   `json:"text"`
	Projects []string `json:"projects"` // "inbox" for tasks in no project
	Statuses []string `json:"statuses"` // status categories
	// StatusIDs are statuses picked by ID (a project's own, such as one
	// named Agent review). They and Statuses are one part: a task matches
	// if its status's category is in Statuses or its status is in
	// StatusIDs.
	StatusIDs  []string `json:"statusIds"`
	Priorities []string `json:"priorities"`
	Labels     []string `json:"labels"`
	Assignees  []string `json:"assignees"` // "me" is whoever is looking
	Due        string   `json:"due"`       // overdue, today, week, dated or none
	ShowDone   bool     `json:"showDone"`
	// MyDay keeps the tasks in the viewer's My Day: picked for today or an
	// earlier day.
	MyDay bool `json:"myDay"`
	// Dependencies are "blocked" (waits for a task not done) and
	// "blocking" (an open task holding up another not done); a task
	// matches if it is either one named.
	Dependencies []string `json:"dependencies"`
	// Time is about estimates and time logged, and unlike the other lists
	// a task must be every one named: "estimated" (it has an estimate),
	// "unestimated" (it has none), "over" (more logged on it than its
	// estimate), "recent" (time logged in the seven days to today) and
	// "mine" (time of the viewer's logged on it). An unknown one matches
	// nothing.
	Time []string `json:"time"`
	// Fields are conditions on custom fields, each read once (see
	// ReadCondition); one malformed condition matches nothing rather than
	// failing the whole filter.
	Fields []Condition `json:"-"`
	// malformed counts the parts of the wrong JSON type (a list that is
	// not a list of strings, text or due that is not a string, showDone
	// that is not a boolean, fields that is not a list). Each is set, and
	// matches nothing, as the app has it.
	malformed int
	// unreadable: the filter, or the config holding it, is not an object.
	// It matches nothing.
	unreadable bool
}

// filterKeys are the parts of a filter, by their exact names.
var filterKeys = []string{"text", "projects", "statuses", "statusIds", "priorities", "labels", "assignees", "due", "showDone", "myDay", "dependencies", "time"}

// exact reads a JSON object's keys as they are written. encoding/json
// matches struct fields regardless of case ("Text" for "text"), which the
// app's copy does not, so only exact names may reach a struct.
func exact(b []byte, keys ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

// UnmarshalJSON reads a filter by its exact key names. A part of the
// wrong JSON type is malformed and matches nothing, as is a filter that is
// not an object; null for a part, or for the whole, leaves it unset.
func (f *Filter) UnmarshalJSON(b []byte) error {
	*f = Filter{}
	b = bytes.TrimSpace(b)
	if string(b) == "null" {
		return nil
	}
	if !bytes.HasPrefix(b, []byte("{")) {
		if !json.Valid(b) {
			return errors.New("filter: not JSON")
		}
		f.unreadable = true
		return nil
	}
	parts, err := exact(b, append(filterKeys, "fields")...)
	if err != nil {
		return err
	}
	given := func(name string) (json.RawMessage, bool) {
		raw, ok := parts[name]
		raw = bytes.TrimSpace(raw)
		return raw, ok && string(raw) != "null"
	}
	text := func(name string, to *string) {
		if raw, ok := given(name); ok && (!bytes.HasPrefix(raw, []byte(`"`)) || json.Unmarshal(raw, to) != nil) {
			f.malformed++
		}
	}
	list := func(name string, to *[]string) {
		raw, ok := given(name)
		if !ok {
			return
		}
		var values []any
		if json.Unmarshal(raw, &values) != nil {
			f.malformed++
			return
		}
		if l, ok := stringList(values); ok {
			*to = l
		} else {
			f.malformed++
		}
	}
	text("text", &f.Text)
	list("projects", &f.Projects)
	list("statuses", &f.Statuses)
	list("statusIds", &f.StatusIDs)
	list("priorities", &f.Priorities)
	list("labels", &f.Labels)
	list("assignees", &f.Assignees)
	text("due", &f.Due)
	list("dependencies", &f.Dependencies)
	list("time", &f.Time)
	flag := func(name string, to *bool) {
		if raw, ok := given(name); ok {
			switch string(raw) {
			case "true":
				*to = true
			case "false":
			default:
				f.malformed++
			}
		}
	}
	flag("showDone", &f.ShowDone)
	flag("myDay", &f.MyDay)
	if raw, ok := given("fields"); ok {
		var conditions []json.RawMessage
		if !bytes.HasPrefix(raw, []byte("[")) || json.Unmarshal(raw, &conditions) != nil {
			f.malformed++
		} else {
			f.Fields = make([]Condition, len(conditions))
			for i, r := range conditions {
				f.Fields[i] = ReadCondition(r)
			}
		}
	}
	return nil
}

// FromConfig reads the filter out of a saved view's config ("filter",
// exactly). A config that is not an object matches nothing; one with no
// filter, or a null one, shows every open task.
func FromConfig(config []byte) (Filter, error) {
	config = bytes.TrimSpace(config)
	if !bytes.HasPrefix(config, []byte("{")) && string(config) != "null" {
		if !json.Valid(config) {
			return Filter{}, errors.New("config: not JSON")
		}
		return Filter{unreadable: true}, nil
	}
	parts, err := exact(config, "filter")
	if err != nil {
		return Filter{}, err
	}
	raw, ok := parts["filter"]
	if !ok {
		return Filter{}, nil
	}
	var f Filter
	err = json.Unmarshal(raw, &f)
	return f, err
}

// Task is what the filter reads of a task.
type Task struct {
	ID        string
	ProjectID string // "" for the inbox
	StatusID  string // "" for none
	Title     string
	Notes     string
	Priority  string
	DueDate   string // "2026-10-28", or "" for none
	// Estimate is the task's own estimate in minutes, or nil for none.
	Estimate *int64
}

// Entry is one piece of time logged on a task, as the filter reads it.
type Entry struct {
	User    string
	Day     string // "2026-10-28"
	Minutes int64  // 0 for a timer still running
}

// Context is what the filter needs to know beyond the task itself.
type Context struct {
	Me    string
	Today string // "2026-10-28", in the viewer's time zone
	// Category is the status category of the task's status.
	Category  func(Task) string
	Labels    func(taskID string) []string
	Assignees func(taskID string) []string
	// FieldType is a custom field's type, or "" for a field this viewer
	// does not know (deleted, or in a project they cannot see).
	FieldType func(fieldID string) string
	// Value is a task's stored value for a field, decoded with UseNumber
	// (a string, json.Number, bool, []any), or nil for none.
	Value func(taskID, fieldID string) any
	// Blocked is whether a task waits for one not done; Blocking whether
	// an open task holds up one not done. Nil reads as false.
	Blocked, Blocking func(taskID string) bool
	// Entries is the time logged on a task. Nil reads as none.
	Entries func(taskID string) []Entry
	// InMyDay is whether the viewer picked a task for My Day, today or on
	// an earlier day. Nil reads as false.
	InMyDay func(taskID string) bool
}

func addDays(day string, n int) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, n).Format(time.DateOnly)
}

func dueMatches(due string, t Task, today string) bool {
	d := t.DueDate
	switch due {
	case "none":
		return d == ""
	case "overdue":
		return d != "" && d < today
	case "today":
		return d != "" && d <= today
	case "week":
		return d != "" && d <= addDays(today, 7)
	case "dated":
		return d != ""
	}
	return false
}

// Matches reports whether a task passes a filter. Within a part any choice
// matches; every part must. A malformed part matches nothing, and so does
// an unreadable filter.
func Matches(t Task, f Filter, ctx Context) bool {
	if f.unreadable || f.malformed > 0 {
		return false
	}
	category := ctx.Category(t)
	done := category == "done" || category == "canceled"
	// A finished task shows when asked for: by showDone, by a finished
	// category, or by its own status picked (only a finished status picked
	// can reach one; the status part then decides).
	picked := t.StatusID != "" && slices.Contains(f.StatusIDs, t.StatusID)
	if done && !f.ShowDone && !picked && !slices.ContainsFunc(f.Statuses, func(s string) bool { return s == "done" || s == "canceled" }) {
		return false
	}
	if (len(f.Statuses) > 0 || len(f.StatusIDs) > 0) && !picked && !slices.Contains(f.Statuses, category) {
		return false
	}
	project := t.ProjectID
	if project == "" {
		project = "inbox"
	}
	if len(f.Projects) > 0 && !slices.Contains(f.Projects, project) {
		return false
	}
	if len(f.Priorities) > 0 && !slices.Contains(f.Priorities, t.Priority) {
		return false
	}
	if len(f.Labels) > 0 {
		on := ctx.Labels(t.ID)
		if !slices.ContainsFunc(f.Labels, func(l string) bool { return slices.Contains(on, l) }) {
			return false
		}
	}
	if len(f.Assignees) > 0 {
		on := ctx.Assignees(t.ID)
		if !slices.ContainsFunc(f.Assignees, func(a string) bool {
			if a == "me" {
				a = ctx.Me
			}
			return slices.Contains(on, a)
		}) {
			return false
		}
	}
	if f.Due != "" && !dueMatches(f.Due, t, ctx.Today) {
		return false
	}
	if f.MyDay && (ctx.InMyDay == nil || !ctx.InMyDay(t.ID)) {
		return false
	}
	if len(f.Dependencies) > 0 {
		is := func(fn func(string) bool) bool { return fn != nil && fn(t.ID) }
		blocked := slices.Contains(f.Dependencies, "blocked") && is(ctx.Blocked)
		blocking := slices.Contains(f.Dependencies, "blocking") && is(ctx.Blocking)
		if !blocked && !blocking {
			return false
		}
	}
	for _, want := range f.Time {
		if !timeMatches(want, t, ctx) {
			return false
		}
	}
	if text := lower(trim(f.Text)); text != "" && !strings.Contains(lower(t.Title+"\n"+t.Notes), text) {
		return false
	}
	for _, c := range f.Fields {
		if !matchesField(c, t, ctx) {
			return false
		}
	}
	return true
}

// timeMatches is one of a filter's time conditions, as the app has it.
func timeMatches(want string, t Task, ctx Context) bool {
	var entries []Entry
	if ctx.Entries != nil {
		entries = ctx.Entries(t.ID)
	}
	switch want {
	case "estimated":
		return t.Estimate != nil
	case "unestimated":
		return t.Estimate == nil
	case "over":
		var logged int64
		for _, e := range entries {
			logged += e.Minutes
		}
		return t.Estimate != nil && logged > *t.Estimate
	case "recent":
		from := addDays(ctx.Today, -6)
		return slices.ContainsFunc(entries, func(e Entry) bool { return e.Day >= from && e.Day <= ctx.Today })
	case "mine":
		return slices.ContainsFunc(entries, func(e Entry) bool { return e.User == ctx.Me })
	}
	return false
}

// Count is how many parts of a filter are set, for a badge.
func Count(f Filter) int {
	if f.unreadable {
		return 1
	}
	n := f.malformed
	// Categories and statuses picked by ID are one row of the panel, so
	// one part.
	if len(f.Statuses) > 0 || len(f.StatusIDs) > 0 {
		n++
	}
	for _, part := range [][]string{f.Projects, f.Priorities, f.Labels, f.Assignees, f.Dependencies, f.Time} {
		if len(part) > 0 {
			n++
		}
	}
	if f.Due != "" {
		n++
	}
	if trim(f.Text) != "" {
		n++
	}
	if f.ShowDone {
		n++
	}
	if f.MyDay {
		n++
	}
	for _, c := range f.Fields {
		if !c.ok || !c.unset() {
			n++
		}
	}
	return n
}

// Conditions on custom fields. Every condition must hold, as every other
// part must. The rules, which web/src/lib/filter.ts follows exactly:
//
//   - Empty means no value: absent, null, "", [] or false, as the app's
//     setFieldValue has it. A task whose project lacks the field has none.
//   - A malformed condition (not an object, a field or op that is not a
//     string, an op the field's type does not have, a value of the wrong
//     JSON type, a date bound that is not a real YYYY-MM-DD or a whole
//     number of days within 3660 of today) matches nothing, like an
//     unknown due.
//   - An unset condition (any, all or none_of with no choices, contains or
//     not_contains with blank text) is ignored and not counted.
//     Only a condition that can be read (a string field and op) is unset;
//     any other counts.
//   - Keys are read exactly as written ("field", not "Field"), and a field
//     of a type this server does not know matches nothing.
//   - A field this viewer does not know is evaluated as if no task had a
//     value: empty, not_contains, none_of and unchecked match, the rest
//     match nothing. A guest so sees a shared view's tasks within their
//     own projects, never more.
//   - not_contains and none_of match tasks with no value; unchecked
//     matches any value that is not true. A stored value of the wrong
//     shape never matches a comparison, but still counts as a value for
//     empty and not_empty.

// Condition is one element of a filter's "fields", read once.
type Condition struct {
	// ok: an object with a string "field" and "op" (exact names).
	ok        bool
	field, op string
	// value is decoded with UseNumber; nil when missing or null.
	value any
}

// ReadCondition reads one element of "fields".
func ReadCondition(raw json.RawMessage) Condition {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return Condition{}
	}
	parts, err := exact(raw, "field", "op", "value")
	if err != nil {
		return Condition{}
	}
	var c Condition
	str := func(name string, to *string) bool {
		v := bytes.TrimSpace(parts[name])
		return bytes.HasPrefix(v, []byte(`"`)) && json.Unmarshal(v, to) == nil
	}
	if !str("field", &c.field) || !str("op", &c.op) {
		return Condition{}
	}
	if v, ok := parts["value"]; ok {
		d := json.NewDecoder(bytes.NewReader(v))
		d.UseNumber()
		if d.Decode(&c.value) != nil {
			c.value = nil
		}
	}
	c.ok = true
	return c
}

func (c Condition) unset() bool {
	switch c.op {
	case "any", "all", "none_of":
		list, isList := c.value.([]any)
		return c.value == nil || (isList && len(list) == 0)
	case "contains", "not_contains":
		s, isString := c.value.(string)
		return c.value == nil || (isString && trim(s) == "")
	}
	return false
}

var fieldOps = map[string][]string{
	"text":         {"contains", "not_contains", "empty", "not_empty"},
	"url":          {"contains", "not_contains", "empty", "not_empty"},
	"number":       {"eq", "lt", "lte", "gt", "gte", "empty", "not_empty"},
	"date":         {"on", "before", "after", "between", "empty", "not_empty"},
	"select":       {"any", "none_of", "empty", "not_empty"},
	"user":         {"any", "none_of", "empty", "not_empty"},
	"multi_select": {"any", "all", "none_of", "empty", "not_empty"},
	"checkbox":     {"checked", "unchecked"},
}

func hasValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case []any:
		return len(x) > 0
	}
	return true
}

// finite reads a JSON number as JavaScript would, false for anything else
// or a number out of range (Infinity there).
func finite(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

// bound reads a date bound: a real YYYY-MM-DD, or a whole number of days
// from today within 3660 either way.
func bound(v any, today string) (string, bool) {
	if s, ok := v.(string); ok {
		if !datePattern(s) {
			return "", false
		}
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return "", false
		}
		return s, true
	}
	n, ok := finite(v)
	if !ok || n != math.Trunc(n) || math.Abs(n) > 3660 {
		return "", false
	}
	return addDays(today, int(n)), true
}

// datePattern is ^\d{4}-\d{2}-\d{2}$.
func datePattern(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range []byte(s) {
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// stringList reads a list of strings; false for anything else.
func stringList(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(list))
	for i, x := range list {
		s, ok := x.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

func matchesField(c Condition, t Task, ctx Context) bool {
	if !c.ok {
		return false
	}
	if c.unset() {
		return true
	}
	op := c.op
	kind := ""
	if ctx.FieldType != nil {
		kind = ctx.FieldType(c.field)
	}
	if kind == "" {
		return op == "empty" || op == "not_contains" || op == "none_of" || op == "unchecked"
	}
	if !slices.Contains(fieldOps[kind], op) {
		return false
	}
	var stored any
	if ctx.Value != nil {
		stored = ctx.Value(t.ID, c.field)
	}
	has := hasValue(stored)
	value := c.value
	switch op {
	case "empty":
		return !has
	case "not_empty":
		return has
	case "checked":
		return stored == true
	case "unchecked":
		return stored != true
	case "contains", "not_contains":
		want, ok := value.(string)
		if !ok {
			return false
		}
		needle := lower(trim(want))
		s, isString := stored.(string)
		if op == "contains" {
			return isString && strings.Contains(lower(s), needle)
		}
		return !has || (isString && !strings.Contains(lower(s), needle))
	case "eq", "lt", "lte", "gt", "gte":
		want, ok := finite(value)
		if !ok {
			return false
		}
		n, ok := finite(stored)
		if !ok {
			return false
		}
		switch op {
		case "eq":
			return n == want
		case "lt":
			return n < want
		case "lte":
			return n <= want
		case "gt":
			return n > want
		}
		return n >= want
	case "on", "before", "after", "between":
		var from, to string
		if op == "between" {
			pair, ok := value.([]any)
			if !ok || len(pair) != 2 {
				return false
			}
			var okFrom, okTo bool
			from, okFrom = bound(pair[0], ctx.Today)
			to, okTo = bound(pair[1], ctx.Today)
			if !okFrom || !okTo {
				return false
			}
		} else {
			b, ok := bound(value, ctx.Today)
			if !ok {
				return false
			}
			from, to = b, b
		}
		s, ok := stored.(string)
		if !ok || !datePattern(s) {
			return false
		}
		switch op {
		case "on":
			return s == from
		case "before":
			return s < from
		case "after":
			return s > from
		}
		return from <= s && s <= to
	case "any", "all", "none_of":
		set, ok := stringList(value)
		if !ok {
			return false
		}
		if kind == "user" {
			// stringList made the list afresh, so the condition keeps "me".
			for i, s := range set {
				if s == "me" {
					set[i] = ctx.Me
				}
			}
		}
		if kind == "multi_select" {
			list, isList := stored.([]any)
			in := func(x any) bool {
				s, ok := x.(string)
				return ok && slices.Contains(set, s)
			}
			switch op {
			case "any":
				return isList && slices.ContainsFunc(list, in)
			case "all":
				return isList && !slices.ContainsFunc(set, func(s string) bool { return !slices.Contains(list, any(s)) })
			}
			return !has || (isList && !slices.ContainsFunc(list, in))
		}
		s, isString := stored.(string)
		if op == "any" {
			return isString && slices.Contains(set, s)
		}
		return !has || (isString && !slices.Contains(set, s))
	}
	return false
}

// trim is JavaScript's String.prototype.trim: it removes the byte order
// mark too, but not U+0085, unlike strings.TrimSpace.
func trim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0xFEFF, 0x2028, 0x2029:
			return true
		}
		return unicode.Is(unicode.Zs, r)
	})
}

// lower is JavaScript's String.prototype.toLowerCase: Go's simple mapping
// plus the two rules it adds that apply in every language. A dotted
// capital I becomes "i" and a combining dot, and a capital sigma ending a
// word becomes a final sigma.
func lower(s string) string {
	if !strings.ContainsAny(s, "İΣ") {
		return strings.ToLower(s)
	}
	var b strings.Builder
	for i, r := range s {
		switch r {
		case 'İ':
			b.WriteString("i̇")
		case 'Σ':
			if finalSigma(s, i) {
				b.WriteRune('ς')
			} else {
				b.WriteRune('σ')
			}
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// finalSigma is Unicode's Final_Sigma condition for the sigma at byte i:
// a cased letter comes before it and none after it, skipping
// case-ignorable characters both ways.
func finalSigma(s string, i int) bool {
	before := false
	for j := i; j > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		j -= size
		if caseIgnorable(r) {
			continue
		}
		before = cased(r)
		break
	}
	if !before {
		return false
	}
	for j := i + len("Σ"); j < len(s); {
		r, size := utf8.DecodeRuneInString(s[j:])
		j += size
		if caseIgnorable(r) {
			continue
		}
		return !cased(r)
	}
	return true
}

func cased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r)
}

func caseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0xB7, 0x2018, 0x2019, 0x2024, 0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
