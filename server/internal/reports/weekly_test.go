package reports

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

type vectorTask struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	ProjectID       *string    `json:"projectId"`
	ParentID        *string    `json:"parentId"`
	Category        string     `json:"category"`
	MadeAt          *time.Time `json:"madeAt"`
	CompletedAt     *time.Time `json:"completedAt"`
	StartedAt       *time.Time `json:"startedAt"`
	DueDate         *string    `json:"dueDate"`
	EstimateMinutes *int64     `json:"estimateMinutes"`
	Mine            bool       `json:"mine"`
}

type period struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type figures struct {
	Done    int      `json:"done"`
	Made    int      `json:"made"`
	Overdue int      `json:"overdue"`
	Median  *float64 `json:"median"`
}

type expected struct {
	Week     period  `json:"week"`
	Before   period  `json:"before"`
	Last     figures `json:"last"`
	Prior    figures `json:"prior"`
	Projects []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Done    int    `json:"done"`
		Open    int    `json:"open"`
		Overdue int    `json:"overdue"`
	} `json:"projects"`
	Mine     int64 `json:"mine"`
	Everyone int64 `json:"everyone"`
	Needs    []struct {
		Task    string `json:"task"`
		Late    *int   `json:"late"`
		Waiting int    `json:"waiting"`
		Over    *int64 `json:"over"`
	} `json:"needs"`
	Quiet bool `json:"quiet"`
}

type vectorCase struct {
	Name     string `json:"name"`
	Zone     string `json:"zone"`
	Today    string `json:"today"`
	Me       string `json:"me"`
	Projects []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		InUse bool   `json:"inUse"`
	} `json:"projects"`
	Tasks        []vectorTask `json:"tasks"`
	Dependencies []struct {
		TaskID    string `json:"taskId"`
		BlockerID string `json:"blockerId"`
	} `json:"dependencies"`
	Entries []struct {
		TaskID    string `json:"taskId"`
		ProjectID string `json:"projectId"`
		UserID    string `json:"userId"`
		Day       string `json:"day"`
		Minutes   int64  `json:"minutes"`
	} `json:"entries"`
	Expect *expected `json:"expect"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func inputOf(t *testing.T, c vectorCase) Input {
	t.Helper()
	zone, err := time.LoadLocation(c.Zone)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Today: c.Today, Zone: zone, Me: c.Me}
	for _, p := range c.Projects {
		in.Projects = append(in.Projects, Project{ID: p.ID, Name: p.Name, InUse: p.InUse})
	}
	for _, v := range c.Tasks {
		in.Tasks = append(in.Tasks, Task{ID: v.ID, Title: v.Title, ProjectID: deref(v.ProjectID), ParentID: deref(v.ParentID),
			Category: v.Category, MadeAt: v.MadeAt, CompletedAt: v.CompletedAt, StartedAt: v.StartedAt,
			DueDate: deref(v.DueDate), Estimate: v.EstimateMinutes, Mine: v.Mine})
	}
	for _, d := range c.Dependencies {
		in.Dependencies = append(in.Dependencies, Dependency{TaskID: d.TaskID, BlockerID: d.BlockerID})
	}
	for _, e := range c.Entries {
		in.Entries = append(in.Entries, Entry{TaskID: e.TaskID, ProjectID: e.ProjectID, UserID: e.UserID, Day: e.Day, Minutes: e.Minutes})
	}
	return in
}

func sameMedian(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) < 1e-9
}

func sameFigures(t *testing.T, which string, got Figures, want figures) {
	t.Helper()
	if got.Done != want.Done || got.Made != want.Made || got.Overdue != want.Overdue || !sameMedian(got.Median, want.Median) {
		t.Errorf("%s: got %+v (median %v), want %+v (median %v)", which, got, show(got.Median), want, show(want.Median))
	}
}

func show(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func eqInt(a, b *int) bool     { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func eqInt64(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

// TestWeeklyMatchesTheApp holds Weekly to the app's weeklyReport through
// shared/weekly-vectors.json.
func TestWeeklyMatchesTheApp(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/weekly-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []vectorCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 4 {
		t.Fatalf("only %d cases", len(file.Cases))
	}
	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			want := c.Expect
			if want == nil {
				t.Fatal("the case has no expect: write them from the app (see the file's about)")
			}
			got := Weekly(inputOf(t, c))
			if got.Week != (Period{want.Week.From, want.Week.To}) || got.Before != (Period{want.Before.From, want.Before.To}) {
				t.Errorf("weeks: got %v and %v, want %v and %v", got.Week, got.Before, want.Week, want.Before)
			}
			sameFigures(t, "last", got.Last, want.Last)
			sameFigures(t, "prior", got.Prior, want.Prior)
			if len(got.Projects) != len(want.Projects) {
				t.Errorf("projects: got %+v, want %+v", got.Projects, want.Projects)
			} else {
				for i, p := range want.Projects {
					if g := got.Projects[i]; g != (ProjectLine{p.ID, p.Name, p.Done, p.Open, p.Overdue}) {
						t.Errorf("project %d: got %+v, want %+v", i, g, p)
					}
				}
			}
			if got.Mine != want.Mine || got.Everyone != want.Everyone {
				t.Errorf("time: got %d and %d, want %d and %d", got.Mine, got.Everyone, want.Mine, want.Everyone)
			}
			if len(got.Needs) != len(want.Needs) {
				t.Errorf("needs: got %d, want %d", len(got.Needs), len(want.Needs))
			} else {
				for i, n := range want.Needs {
					g := got.Needs[i]
					if g.Task.ID != n.Task || !eqInt(g.Late, n.Late) || g.Waiting != n.Waiting || !eqInt64(g.Over, n.Over) {
						t.Errorf("need %d: got %s late %v waiting %d over %v, want %+v", i, g.Task.ID, g.Late, g.Waiting, g.Over, n)
					}
				}
			}
			if got.Quiet != want.Quiet {
				t.Errorf("quiet: got %v, want %v", got.Quiet, want.Quiet)
			}
		})
	}
}
