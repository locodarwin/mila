package ical

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/filter"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// feedEntities are the rows a feed is built from.
var feedEntities = []string{"project", "status", "label", "saved_view", "task", "task_label", "task_dependency", "task_assignee", "my_day_entry"}

// fieldEntities are read as well for a view's feed whose filter has
// conditions on custom fields.
var fieldEntities = []string{"custom_field", "custom_field_value"}

// needsFields reports whether a view feed's filter has conditions on
// custom fields, so their rows must be read too.
func needsFields(feed Feed, rows []syncer.Row) bool {
	if feed.Scope != ScopeView {
		return false
	}
	for _, r := range rows {
		if r.Entity == "saved_view" && r.ID == feed.TargetID {
			config, _ := json.Marshal(r.Fields["config"])
			f, err := filter.FromConfig(config)
			return err == nil && len(f.Fields) > 0
		}
	}
	return false
}

const (
	// eventLength is how long a task with a due time is in a calendar.
	eventLength = 30 * time.Minute
	// maxNotes is how much of a task's notes an event carries.
	maxNotes = 1000
)

// task is what a feed reads of a task row.
type task struct {
	id, projectID, parentID, statusID string
	title, notes, priority            string
	dueDate, dueTime                  string
	recurrence, mode                  string
	open                              bool
	modified                          time.Time
	estimate                          *int64
}

// workspace is what a feed's owner sees of a workspace, by ID.
type workspace struct {
	projects   map[string]projectRow
	categories map[string]string // status → category, in projects not archived
	labels     map[string]labelRow
	views      map[string]viewRow
	tasks      []task
	taskLabels map[string][]string
	assignees  map[string][]string
	deps       [][2]string               // blocker, then the task waiting for it
	myDay      map[string]string         // task → the latest day it is in the owner's My Day
	fieldTypes map[string]string         // custom field → its type
	values     map[string]map[string]any // task → field → its value, numbers as json.Number
	entries    map[string][]filter.Entry // task → the time logged on it
}

type projectRow struct {
	name     string
	archived bool
}

type labelRow struct{ name, sortKey string }

type viewRow struct {
	name, projectID string
	config          []byte
}

// number reads a whole number as rows hold it, or nil for none.
func number(v any) *int64 {
	var n int64
	switch x := v.(type) {
	case int64:
		n = x
	case int:
		n = int64(x)
	case float64:
		n = int64(x)
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return nil
		}
		n = i
	default:
		return nil
	}
	return &n
}

func str(f map[string]any, name string) string {
	s, _ := f[name].(string)
	return s
}

// lastChange is when a row last changed, by the newest clock of its
// fields.
func lastChange(clocks map[string]string) time.Time {
	var newest string
	for _, c := range clocks {
		newest = max(newest, c)
	}
	ms, err := strconv.ParseInt(strings.SplitN(newest, "-", 2)[0], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func read(rows []syncer.Row, owner string) *workspace {
	w := &workspace{
		projects: map[string]projectRow{}, categories: map[string]string{}, labels: map[string]labelRow{},
		views: map[string]viewRow{}, taskLabels: map[string][]string{}, assignees: map[string][]string{},
		myDay: map[string]string{}, fieldTypes: map[string]string{}, values: map[string]map[string]any{},
		entries: map[string][]filter.Entry{},
	}
	var statuses []syncer.Row
	for _, r := range rows {
		f := r.Fields
		switch r.Entity {
		case "project":
			w.projects[r.ID] = projectRow{name: str(f, "name"), archived: f["archived_at"] != nil}
		case "status":
			statuses = append(statuses, r)
		case "label":
			w.labels[r.ID] = labelRow{name: str(f, "name"), sortKey: str(f, "sort_key")}
		case "saved_view":
			config, _ := json.Marshal(f["config"])
			w.views[r.ID] = viewRow{name: str(f, "name"), projectID: str(f, "project_id"), config: config}
		case "task":
			project := str(f, "project_id")
			if _, ok := w.projects[project]; project != "" && !ok {
				continue // in a project in the trash
			}
			w.tasks = append(w.tasks, task{
				id: r.ID, projectID: project, parentID: str(f, "parent_id"), statusID: str(f, "status_id"),
				title: str(f, "title"), notes: str(f, "notes"), priority: str(f, "priority"),
				dueDate: str(f, "due_date"), dueTime: str(f, "due_time"),
				recurrence: str(f, "recurrence"), mode: str(f, "recurrence_mode"),
				open: f["completed_at"] == nil, modified: lastChange(r.Clocks), estimate: number(f["estimate_minutes"]),
			})
		case "task_label":
			id := str(f, "task_id")
			w.taskLabels[id] = append(w.taskLabels[id], str(f, "label_id"))
		case "task_assignee":
			id := str(f, "task_id")
			w.assignees[id] = append(w.assignees[id], str(f, "user_id"))
		case "time_entry":
			id := str(f, "task_id")
			var minutes int64
			if n := number(f["minutes"]); n != nil {
				minutes = *n
			}
			w.entries[id] = append(w.entries[id], filter.Entry{User: str(f, "user_id"), Day: str(f, "day"), Minutes: minutes})
		case "task_dependency":
			w.deps = append(w.deps, [2]string{str(f, "blocker_id"), str(f, "task_id")})
		case "my_day_entry":
			if str(f, "user_id") == owner {
				id := str(f, "task_id")
				w.myDay[id] = max(w.myDay[id], str(f, "day"))
			}
		case "custom_field":
			w.fieldTypes[r.ID] = str(f, "type")
		case "custom_field_value":
			raw, err := json.Marshal(f["value"])
			if err != nil {
				continue
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			var v any
			if d.Decode(&v) != nil {
				continue
			}
			id := str(f, "task_id")
			if w.values[id] == nil {
				w.values[id] = map[string]any{}
			}
			w.values[id][str(f, "field_id")] = v
		}
	}
	// A label in the trash is on no task, as the app has it, though the
	// rows that put it there stay live.
	for id, labels := range w.taskLabels {
		w.taskLabels[id] = slices.DeleteFunc(labels, func(l string) bool {
			_, ok := w.labels[l]
			return !ok
		})
	}
	// As the app's views do, a status counts only in a project that is
	// not archived; a task elsewhere is done or to do by its completion.
	for _, r := range statuses {
		if p, ok := w.projects[str(r.Fields, "project_id")]; ok && !p.archived {
			w.categories[r.ID] = str(r.Fields, "category")
		}
	}
	return w
}

// event is one VEVENT.
type event struct {
	t        task
	date     string // the day it is on
	clock    string // its time, or "" all day
	repeat   *recur.Rule
	labels   []string
	project  string
	link     string
	modified time.Time
}

// build makes the body of a feed, or reports false when its owner can no
// longer see its project or view.
func (s *Service) build(feed Feed, workspaceID, owner string, loc *time.Location, rows []syncer.Row, now time.Time) ([]byte, bool) {
	w := read(rows, owner)
	today := now.In(loc).Format(time.DateOnly)
	var events []event
	add := func(t task, date, clock string) {
		e := event{t: t, date: date, clock: clock, link: s.baseURL + notify.Path(workspaceID, t.id), modified: t.modified}
		if e.modified.IsZero() {
			e.modified = feed.CreatedAt
		}
		e.project = "Inbox"
		if p, ok := w.projects[t.projectID]; ok {
			e.project = p.name
		}
		ids := w.taskLabels[t.id]
		slices.SortFunc(ids, func(a, b string) int {
			return cmp.Or(cmp.Compare(w.labels[a].sortKey, w.labels[b].sortKey), cmp.Compare(a, b))
		})
		for _, id := range ids {
			if l, ok := w.labels[id]; ok {
				e.labels = append(e.labels, l.name)
			}
		}
		// The rule goes out for an open task on its due date in schedule
		// mode, the only kind a calendar can repeat on its own.
		if t.open && t.recurrence != "" && t.mode == recur.Schedule && date == t.dueDate {
			if r, err := recur.Parse(t.recurrence); err == nil {
				e.repeat = &r
			}
		}
		events = append(events, e)
	}
	switch feed.Scope {
	case ScopeMe:
		for _, t := range w.tasks {
			if !t.open {
				continue
			}
			day, inMyDay := w.myDay[t.id]
			switch {
			case t.dueDate != "" && (inMyDay || slices.Contains(w.assignees[t.id], owner)):
				add(t, t.dueDate, t.dueTime)
			case inMyDay:
				// An undated task in My Day is on its My Day date, and
				// stays on today while it is carried over.
				add(t, max(day, today), "")
			}
		}
	case ScopeProject:
		if _, ok := w.projects[feed.TargetID]; !ok {
			return nil, false
		}
		for _, t := range w.tasks {
			if t.open && t.projectID == feed.TargetID && t.dueDate != "" {
				add(t, t.dueDate, t.dueTime)
			}
		}
	case ScopeView:
		v, ok := w.views[feed.TargetID]
		if !ok {
			return nil, false
		}
		f, err := filter.FromConfig(v.config)
		if err != nil {
			s.log.Warn("a saved view's filter could not be read; its feed is empty", "view", feed.TargetID, "err", err)
			break
		}
		byID := map[string]task{}
		for _, t := range w.tasks {
			byID[t.id] = t
		}
		ctx := filter.Context{
			Me: owner, Today: today,
			// The status's category, or done or to do by completion, as
			// the view page has it.
			Category: func(ft filter.Task) string {
				t := byID[ft.ID]
				if c := w.categories[t.statusID]; c != "" {
					return c
				}
				if !t.open {
					return "done"
				}
				return "todo"
			},
			Labels:    func(id string) []string { return w.taskLabels[id] },
			Assignees: func(id string) []string { return w.assignees[id] },
			// A field the owner cannot see is unknown, so its conditions
			// read as no task having a value, as the app has it.
			FieldType: func(id string) string { return w.fieldTypes[id] },
			Value:     func(task, field string) any { return w.values[task][field] },
			// Dependencies between tasks the owner sees, as the app has
			// them: a task in the trash holds nothing up.
			Blocked: func(id string) bool {
				return slices.ContainsFunc(w.deps, func(d [2]string) bool {
					b, ok := byID[d[0]]
					return d[1] == id && ok && b.open
				})
			},
			Blocking: func(id string) bool {
				return byID[id].open && slices.ContainsFunc(w.deps, func(d [2]string) bool {
					t, ok := byID[d[1]]
					return d[0] == id && ok && t.open
				})
			},
			Entries: func(id string) []filter.Entry { return w.entries[id] },
			// The owner's My Day: picked for today or an earlier day.
			InMyDay: func(id string) bool {
				day, ok := w.myDay[id]
				return ok && day <= today
			},
		}
		for _, t := range w.tasks {
			if t.parentID != "" || (v.projectID != "" && t.projectID != v.projectID) || t.dueDate == "" {
				continue
			}
			// Across projects, tasks in archived projects stay out, as on
			// the view page.
			if v.projectID == "" && w.projects[t.projectID].archived {
				continue
			}
			ft := filter.Task{ID: t.id, ProjectID: t.projectID, StatusID: t.statusID, Title: t.title, Notes: t.notes, Priority: t.priority, DueDate: t.dueDate, Estimate: t.estimate}
			if filter.Matches(ft, f, ctx) {
				add(t, t.dueDate, t.dueTime)
			}
		}
	}
	slices.SortFunc(events, func(a, b event) int {
		return cmp.Or(cmp.Compare(a.date, b.date), cmp.Compare(cmp.Or(a.clock, "99"), cmp.Or(b.clock, "99")), cmp.Compare(a.t.id, b.t.id))
	})
	return s.render(feed.Name, loc, events), true
}

// render writes the calendar.
func (s *Service) render(name string, loc *time.Location, events []event) []byte {
	var w writer
	w.line("BEGIN", "VCALENDAR")
	w.line("VERSION", "2.0")
	w.line("PRODID", "-//Mila//Mila calendar feed//EN")
	w.line("CALSCALE", "GREGORIAN")
	w.text("X-WR-CALNAME", name)
	w.line("REFRESH-INTERVAL;VALUE=DURATION", "PT1H")
	w.line("X-PUBLISHED-TTL", "PT1H")
	// Repeating tasks with a time are written in the owner's zone, which
	// then needs describing; everything else is in UTC or all day.
	zoned := loc.String() != "UTC"
	first := 0
	for _, e := range events {
		if zoned && e.repeat != nil && e.clock != "" {
			if y, err := strconv.Atoi(e.date[:4]); err == nil && (first == 0 || y < first) {
				first = y
			}
		}
	}
	if first != 0 {
		vtimezone(&w, loc, first-1)
	}
	for _, e := range events {
		s.writeEvent(&w, e, loc, zoned)
	}
	w.line("END", "VCALENDAR")
	return w.b.Bytes()
}

func (s *Service) writeEvent(w *writer, e event, loc *time.Location, zoned bool) {
	date, err := time.Parse(time.DateOnly, e.date)
	if err != nil {
		return
	}
	w.line("BEGIN", "VEVENT")
	w.line("UID", e.t.id+"@"+s.host)
	w.line("DTSTAMP", utc(e.modified))
	w.line("LAST-MODIFIED", utc(e.modified))
	var until string
	if e.clock == "" {
		w.line("DTSTART;VALUE=DATE", day(date))
		w.line("DTEND;VALUE=DATE", day(date.AddDate(0, 0, 1)))
		if e.repeat != nil && e.repeat.Until != "" {
			u, _ := time.Parse(time.DateOnly, e.repeat.Until)
			until = day(u)
		}
	} else {
		start, err := time.ParseInLocation("2006-01-02 15:04", e.date+" "+e.clock, loc)
		if err != nil {
			start = date
		}
		if e.repeat != nil && e.repeat.Until != "" {
			// The end of that day where the owner is, in UTC, which UNTIL
			// must be even with a TZID.
			u, _ := time.ParseInLocation(time.DateOnly, e.repeat.Until, loc)
			until = utc(u.AddDate(0, 0, 1).Add(-time.Second))
		}
		if e.repeat != nil && zoned {
			tzid := "TZID=" + loc.String()
			w.line("DTSTART;"+tzid, local(start))
			w.line("DTEND;"+tzid, local(start.Add(eventLength)))
		} else {
			w.line("DTSTART", utc(start))
			w.line("DTEND", utc(start.Add(eventLength)))
		}
	}
	if e.repeat != nil {
		w.line("RRULE", rrule(*e.repeat, until))
	}
	w.text("SUMMARY", e.t.title)
	w.text("DESCRIPTION", description(e))
	w.line("URL", e.link)
	if len(e.labels) > 0 {
		parts := make([]string, len(e.labels))
		for i, l := range e.labels {
			parts[i] = escape(l)
		}
		w.line("CATEGORIES", strings.Join(parts, ","))
	}
	w.line("END", "VEVENT")
}

// description is the notes, cut short, then where the task lives and the
// link back to it, which some calendar apps show only here.
func description(e event) string {
	var b strings.Builder
	if notes := strings.TrimSpace(e.t.notes); notes != "" {
		if utf8.RuneCountInString(notes) > maxNotes {
			notes = strings.TrimSpace(string([]rune(notes)[:maxNotes])) + "…"
		}
		b.WriteString(notes + "\n\n")
	}
	b.WriteString("Project: " + e.project + "\nOpen in Mila: " + e.link)
	return b.String()
}

// rrule writes a repeat rule, with UNTIL already in the form DTSTART
// needs.
func rrule(r recur.Rule, until string) string {
	parts := []string{"FREQ=" + r.Freq}
	if r.Interval > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(r.Interval))
	}
	if len(r.Weekdays) > 0 {
		days := make([]string, len(r.Weekdays))
		for i, d := range r.Weekdays {
			days[i] = weekdays[d]
		}
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	if r.Ordinal != 0 {
		parts = append(parts, "BYDAY="+strconv.Itoa(r.Ordinal)+weekdays[r.Weekday])
	}
	if r.MonthDay != 0 {
		parts = append(parts, "BYMONTHDAY="+strconv.Itoa(r.MonthDay))
	}
	if until != "" {
		parts = append(parts, "UNTIL="+until)
	}
	return strings.Join(parts, ";")
}
