// Package reports works out the weekly report's figures on the server,
// which emails it while nobody has the app open. It is the arithmetic
// of the app's reports.ts (inScope, flowReport and weeklyReport), on
// the same rules, and shared/weekly-vectors.json holds the two to the
// same answers.
package reports

import (
	"cmp"
	"slices"
	"time"
	"unicode/utf16"
)

// Task is a live task the person sees, with what the report reads of it.
type Task struct {
	ID, Title           string
	ProjectID, ParentID string // "" for none
	// Category is its status's kind: backlog, todo, in-progress, done or
	// canceled; with no status, done when completed and todo otherwise.
	Category                       string
	MadeAt, CompletedAt, StartedAt *time.Time
	DueDate                        string // "" for none
	Estimate                       *int64 // minutes
	// Mine is whether it is the person's, as the digest has it: assigned
	// to them, or made by them and assigned to nobody.
	Mine bool
}

// Project is a project the person sees; one not in use is archived.
type Project struct {
	ID, Name string
	InUse    bool
}

// Dependency is a task waiting for another.
type Dependency struct{ TaskID, BlockerID string }

// Entry is time someone logged on a day: on a task, or with no task, on a
// project itself.
type Entry struct {
	TaskID, ProjectID, UserID, Day string
	Minutes                        int64
}

// Input is what the report reads: every live task the person sees, and
// the rows that hang off them.
type Input struct {
	// Today is the day it goes out, in Zone: the first day of the week
	// after the one reported.
	Today        string
	Zone         *time.Location
	Me           string
	Projects     []Project
	Tasks        []Task
	Dependencies []Dependency
	Entries      []Entry
}

// Period is a span of days, both ends counted.
type Period struct{ From, To string }

// Figures are one week's.
type Figures struct {
	Done, Made int
	// Overdue is how many were open as the week ended and due on or
	// before its last day.
	Overdue int
	// Median is the median cycle time of the tasks done in the week, in
	// days, or nil when none of them went in progress.
	Median *float64
}

// ProjectLine is a project's: done in the week, and open and overdue as
// it ended.
type ProjectLine struct {
	ID, Name            string
	Done, Open, Overdue int
}

// Need is one of the person's open tasks that wants something.
type Need struct {
	Task Task
	// Late is the days past its due date, when overdue.
	Late *int
	// Waiting is how many open tasks it waits for.
	Waiting int
	// Over is the minutes logged past its estimate, when over.
	Over *int64
}

// Report is the weekly report.
type Report struct {
	// Week is the week reported, the seven days before Today; Before is
	// the week before it.
	Week, Before Period
	Last, Prior  Figures
	// Projects have anything done in the week or open as it ended, most
	// done first.
	Projects []ProjectLine
	// Mine and Everyone are the minutes logged in the week.
	Mine, Everyone int64
	Needs          []Need
	// Quiet is a week with nothing done, made or logged: no email.
	Quiet bool
}

// Weekly works out the report.
func Weekly(in Input) Report {
	live := make(map[string]*Task, len(in.Tasks))
	for i := range in.Tasks {
		live[in.Tasks[i].ID] = &in.Tasks[i]
	}
	inUse := map[string]bool{}
	names := map[string]string{}
	for _, p := range in.Projects {
		inUse[p.ID] = p.InUse
		names[p.ID] = p.Name
	}
	tasks := inScope(in.Tasks, live, inUse)
	counted := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		counted[t.ID] = true
	}
	logged := map[string]int64{}
	for _, e := range in.Entries {
		if e.TaskID != "" {
			logged[e.TaskID] += e.Minutes
		}
	}
	blockers := map[string]int{}
	for _, d := range in.Dependencies {
		if b, ok := live[d.BlockerID]; ok && b.CompletedAt == nil {
			blockers[d.TaskID]++
		}
	}

	day := func(t time.Time) string { return t.In(in.Zone).Format(time.DateOnly) }
	r := Report{
		Week:   Period{addDays(in.Today, -7), addDays(in.Today, -1)},
		Before: Period{addDays(in.Today, -14), addDays(in.Today, -8)},
	}
	w := weekOf{day: day}
	r.Last = w.figures(tasks, r.Week)
	r.Prior = w.figures(tasks, r.Before)

	lines := map[string]*ProjectLine{}
	for _, t := range tasks {
		finished := t.Category == "done"
		if t.ProjectID != "" {
			at, known := w.doneDay(t)
			doneIn := finished && known && at >= r.Week.From && at <= r.Week.To
			open := w.openAt(t, r.Week.To, finished)
			if doneIn || open {
				line := lines[t.ProjectID]
				if line == nil {
					line = &ProjectLine{ID: t.ProjectID, Name: names[t.ProjectID]}
					lines[t.ProjectID] = line
				}
				if doneIn {
					line.Done++
				}
				if open {
					line.Open++
					if t.DueDate != "" && t.DueDate <= r.Week.To {
						line.Overdue++
					}
				}
			}
		}
		if finished || !t.Mine {
			continue
		}
		n := Need{Task: t, Waiting: blockers[t.ID]}
		if t.DueDate != "" && t.DueDate < in.Today {
			late := daysBetween(t.DueDate, in.Today)
			n.Late = &late
		}
		if t.Estimate != nil && logged[t.ID] > *t.Estimate {
			over := logged[t.ID] - *t.Estimate
			n.Over = &over
		}
		if n.Late != nil || n.Waiting > 0 || n.Over != nil {
			r.Needs = append(r.Needs, n)
		}
	}
	or := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	or64 := func(p *int64) int64 {
		if p == nil {
			return -1
		}
		return *p
	}
	slices.SortStableFunc(r.Needs, func(a, b Need) int {
		return cmp.Or(cmp.Compare(or(b.Late), or(a.Late)), cmp.Compare(b.Waiting, a.Waiting),
			cmp.Compare(or64(b.Over), or64(a.Over)), compareCode(a.Task.Title, b.Task.Title), compareCode(a.Task.ID, b.Task.ID))
	})
	for _, l := range lines {
		r.Projects = append(r.Projects, *l)
	}
	slices.SortFunc(r.Projects, func(a, b ProjectLine) int {
		return cmp.Or(cmp.Compare(b.Done, a.Done), cmp.Compare(b.Open, a.Open), compareCode(a.Name, b.Name), compareCode(a.ID, b.ID))
	})

	for _, e := range in.Entries {
		// Time on a project itself counts while the project is in use.
		counts := counted[e.TaskID]
		if e.TaskID == "" {
			counts = inUse[e.ProjectID]
		}
		if e.Day < r.Week.From || e.Day > r.Week.To || !counts {
			continue
		}
		r.Everyone += e.Minutes
		if e.UserID == in.Me {
			r.Mine += e.Minutes
		}
	}
	r.Quiet = r.Last.Done == 0 && r.Last.Made == 0 && r.Everyone == 0
	return r
}

// inScope keeps the tasks the reports count, as reports.ts has it: in a
// project in use or the inbox, not canceled, under live tasks only.
func inScope(all []Task, live map[string]*Task, inUse map[string]bool) []Task {
	var out []Task
	for _, t := range all {
		if (t.ProjectID != "" && !inUse[t.ProjectID]) || t.Category == "canceled" || !parentsLive(t, live) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func parentsLive(t Task, live map[string]*Task) bool {
	parent := t.ParentID
	for depth := 0; parent != "" && depth < 50; depth++ {
		p, ok := live[parent]
		if !ok {
			return false
		}
		parent = p.ParentID
	}
	return true
}

// weekOf holds how moments become days, in the person's zone.
type weekOf struct{ day func(time.Time) string }

// madeDay is the day a task came into being, or false when nobody knows
// (it counts as always there). One done before it was made was made then.
func (w weekOf) madeDay(t Task) (string, bool) {
	at := t.MadeAt
	if at != nil && t.CompletedAt != nil && t.CompletedAt.Before(*at) {
		at = t.CompletedAt
	}
	if at == nil {
		return "", false
	}
	return w.day(*at), true
}

// doneDay is the day a task was last completed, or false when unknown.
func (w weekOf) doneDay(t Task) (string, bool) {
	if t.CompletedAt == nil {
		return "", false
	}
	return w.day(*t.CompletedAt), true
}

// openAt is whether a task was made by the end of a day and still open at
// its end. A finished task done at a time nobody kept was never open.
func (w weekOf) openAt(t Task, day string, finished bool) bool {
	if made, ok := w.madeDay(t); ok && made > day {
		return false
	}
	if !finished {
		return true
	}
	at, ok := w.doneDay(t)
	return ok && at > day
}

// figures are flowReport's made, done and median cycle time for one week,
// and what was overdue as it ended.
func (w weekOf) figures(tasks []Task, p Period) Figures {
	var f Figures
	var cycle []float64
	in := func(day string) bool { return day >= p.From && day <= p.To }
	for _, t := range tasks {
		finished := t.Category == "done"
		if made, ok := w.madeDay(t); ok && in(made) {
			f.Made++
		}
		if t.DueDate != "" && t.DueDate <= p.To && w.openAt(t, p.To, finished) {
			f.Overdue++
		}
		if !finished || t.CompletedAt == nil {
			continue
		}
		if at, _ := w.doneDay(t); !in(at) {
			continue
		}
		f.Done++
		if t.StartedAt != nil {
			if took := float64(t.CompletedAt.Sub(*t.StartedAt)) / float64(24*time.Hour); took >= 0 {
				cycle = append(cycle, took)
			}
		}
	}
	slices.Sort(cycle)
	if n := len(cycle); n > 0 {
		m := cycle[n/2]
		if n%2 == 0 {
			m = (cycle[n/2-1] + cycle[n/2]) / 2
		}
		f.Median = &m
	}
	return f
}

func addDays(day string, n int) string {
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return d.AddDate(0, 0, n).Format(time.DateOnly)
}

func daysBetween(from, to string) int {
	a, errA := time.Parse(time.DateOnly, from)
	b, errB := time.Parse(time.DateOnly, to)
	if errA != nil || errB != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

// compareCode compares as JavaScript's < does, by UTF-16 code units, so
// that the app and the server order ties alike.
func compareCode(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}
