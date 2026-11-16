package syncer

import (
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
)

// Migration 38 moves pair rows by db.PairTables, which must be these.
func TestPairTablesAreTheSyncers(t *testing.T) {
	if len(db.PairTables) != len(pairs) {
		t.Fatalf("db.PairTables has %d tables, syncer %d pairs", len(db.PairTables), len(pairs))
	}
	for _, p := range db.PairTables {
		pair, ok := pairs[p.Entity]
		if !ok || entities[p.Entity] == nil || entities[p.Entity].table != p.Table || pair != [2]string{p.A, p.B} {
			t.Errorf("db.PairTables has %+v; syncer has %v in %s", p, pair, entities[p.Entity].table)
		}
	}
}
