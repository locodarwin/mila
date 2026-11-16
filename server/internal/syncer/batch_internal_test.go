package syncer

import (
	"testing"
	"time"
)

// A batch is its sender's, in one workspace, for one stretch.
func TestBatchesAreTheSendersForAWhile(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	op := Op{Batch: "b-1"}
	mine := batchOf(op, "ann", "w", at)
	if mine == "" || batchOf(op, "ann", "w", at.Add(time.Minute)) != mine && at.Unix()/600 == at.Add(time.Minute).Unix()/600 {
		t.Fatal("one batch read twice in its stretch differs")
	}
	for name, other := range map[string]string{
		"someone else's":    batchOf(op, "bo", "w", at),
		"another workspace": batchOf(op, "ann", "x", at),
		"a stretch later":   batchOf(op, "ann", "w", at.Add(batchSpan)),
	} {
		if other == mine {
			t.Errorf("%s is the same batch", name)
		}
	}
	if batchOf(Op{Batch: "not one!"}, "ann", "w", at) != "" {
		t.Error("an ID an app could not make is a batch")
	}
}
