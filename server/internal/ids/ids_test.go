package ids_test

import (
	"testing"

	"github.com/locodarwin/mila/server/internal/ids"
)

// The app makes the same IDs (web/src/lib/sync/entities.spec.ts holds it
// to these).
func TestIDsAreTheApps(t *testing.T) {
	task, label := "0190a4c2-6f1e-7b3a-9c4d-2e5f6a7b8c9d", "4f8e2a1c-3b5d-4e6f-8a9b-0c1d2e3f4a5b"
	if got, want := ids.Pair(task, label), "5ed720da-10d3-843a-a10e-001833259355"; got != want {
		t.Errorf("Pair = %s, want %s", got, want)
	}
	if ids.Pair(task, label) == ids.Pair(label, task) {
		t.Error("a pair's ID does not depend on its order")
	}
	if got, want := ids.XOR(task, label), "4e1e8ede-5443-3555-16d6-22424444c6c6"; got != want {
		t.Errorf("XOR = %s, want %s", got, want)
	}
	for id, want := range map[string]ids.Shape{
		task:                                   ids.Drawn,
		label:                                  ids.Drawn,
		ids.Pair(task, label):                  ids.Hashed,
		ids.XOR(task, label):                   ids.Odd,
		"4F8E2A1C-3B5D-4E6F-8A9B-0C1D2E3F4A5B": ids.Other,
		"t1":                                   ids.Other,
		"00000000-0000-0000-0000-000000000000": ids.Odd,
	} {
		if got := ids.ShapeOf(id); got != want {
			t.Errorf("ShapeOf(%s) = %d, want %d", id, got, want)
		}
	}
}
