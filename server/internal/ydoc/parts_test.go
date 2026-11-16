package ydoc

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The editor's own updates read to the end, and what they hold is counted.
func TestCountPartsReadsTheEditorsUpdates(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../shared/doc-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Yjs []struct {
			Name    string   `json:"name"`
			Updates []string `json:"updates"`
		} `json:"yjs"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, c := range v.Yjs {
		for _, u := range c.Updates {
			b, err := base64.StdEncoding.DecodeString(u)
			if err != nil {
				t.Fatal(err)
			}
			n, ok := countParts(b)
			if !ok || n == 0 {
				t.Errorf("%s: read %v, %d parts", c.Name, ok, n)
			}
			read++
		}
	}
	if read < 10 {
		t.Fatalf("only %d updates in the vectors", read)
	}
	// And the server's own: a long document written whole, and compacted.
	long := strings.Repeat("A paragraph with **bold** and a [link](/?task=00000000-0000-4000-8000-000000000001).\n\n", 2000)
	u, err := Replace(nil, Parse(long), 7)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Compact(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{u, merged} {
		if n, ok := countParts(b); !ok || n == 0 || n > maxParts {
			t.Errorf("a long document: read %v, %d parts", ok, n)
		}
		if err := Check(b); err != nil {
			t.Errorf("a long document is refused: %v", err)
		}
	}
}

// An update of a few hundred thousand one-character deletions is refused
// before it is built, at a cost in line with its size.
func TestCheckRefusesAnUpdateOfTooManyParts(t *testing.T) {
	// No structs, and a delete set of one client with maxParts+5 ranges of
	// one character each, a few bytes apiece.
	varuint := func(b []byte, v uint64) []byte {
		for v >= 0x80 {
			b = append(b, byte(v)|0x80)
			v >>= 7
		}
		return append(b, byte(v))
	}
	u := varuint(nil, 0)
	u = varuint(u, 1)
	u = varuint(u, 9)
	u = varuint(u, maxParts+5)
	for i := range uint64(maxParts + 5) {
		u = varuint(u, 2*i)
		u = varuint(u, 1)
	}
	if _, err := load(1, false, u); err != nil {
		t.Fatalf("the update is not one Yjs reads: %v", err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := Check(u)
	runtime.ReadMemStats(&after)
	if err != ErrTooManyParts {
		t.Fatalf("Check = %v", err)
	}
	if took := after.TotalAlloc - before.TotalAlloc; took > 1<<20 {
		t.Errorf("refusing an update of %d KB took %d KB", len(u)>>10, took>>10)
	}
}
