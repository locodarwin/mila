package recur_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/locodarwin/mila/server/internal/recur"
)

// The browser's recurrence.ts is checked against the same cases.
type vectors struct {
	Valid   []string
	Invalid []string
	Next    []struct {
		Note  string
		Rule  string
		Mode  string
		Due   *string
		Today string
		Want  *string
	}
	IDs []struct{ After, Want string }
}

func load(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile("../../../shared/recurrence-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Valid) == 0 || len(v.Invalid) == 0 || len(v.Next) == 0 || len(v.IDs) == 0 {
		t.Fatal("the vectors file is missing a section")
	}
	return v
}

func TestValidation(t *testing.T) {
	v := load(t)
	for _, s := range v.Valid {
		if err := recur.Validate(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range v.Invalid {
		if recur.Validate(s) == nil {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestNext(t *testing.T) {
	for _, c := range load(t).Next {
		due := ""
		if c.Due != nil {
			due = *c.Due
		}
		want := ""
		if c.Want != nil {
			want = *c.Want
		}
		got, err := recur.Next(c.Rule, c.Mode, due, c.Today)
		if err != nil {
			t.Errorf("%s %s from %q on %s: %v", c.Rule, c.Mode, due, c.Today, err)
			continue
		}
		if got != want {
			t.Errorf("%s %s from %q on %s (%s) = %q, want %q", c.Rule, c.Mode, due, c.Today, c.Note, got, want)
		}
	}
}

func TestNextTaskIDs(t *testing.T) {
	for _, c := range load(t).IDs {
		if got := recur.NextTaskID(c.After); got != c.Want {
			t.Errorf("NextTaskID(%q) = %s, want %s", c.After, got, c.Want)
		}
	}
}

func TestNextRefusesBadInput(t *testing.T) {
	for _, c := range [][3]string{
		{"FREQ=SECONDLY", "", "2026-10-28"},
		{"FREQ=DAILY", "28/10/2026", "2026-10-28"},
		{"FREQ=DAILY", "", "tomorrow"},
	} {
		if _, err := recur.Next(c[0], recur.Schedule, c[1], c[2]); err == nil {
			t.Errorf("Next%q should fail", c)
		}
	}
}
