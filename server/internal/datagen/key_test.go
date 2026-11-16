package datagen

import (
	"strings"
	"testing"
)

// Sort keys and clocks keep their order however many there are: past the
// 238,328 keys of four characters, and past the counter of a millisecond.
func TestKeysAndClocksKeepOrder(t *testing.T) {
	g := &gen{clock: 1_700_000_000_000}
	if k := g.key(); k != "c000" {
		t.Fatalf("first key = %q, want c000", k)
	}
	if h := g.hlc(); h != "1700000000000-0000-datagen" {
		t.Fatalf("first HLC = %q", h)
	}

	g.keys = 62*62*62 - 2
	want := []string{"czzy", "czzz", "d0000", "d0001"}
	for _, w := range want {
		if k := g.key(); k != w {
			t.Fatalf("key = %q, want %q", k, w)
		}
	}
	g.keys = 62*62*62 + 62*62*62*62 - 1
	if k, next := g.key(), g.key(); k != "dzzzz" || next != "e00000" {
		t.Fatalf("keys = %q, %q; want dzzzz, e00000", k, next)
	}

	prev := ""
	for range 100 {
		h := g.hlc()
		if h <= prev || !strings.HasSuffix(h, "-datagen") {
			t.Fatalf("HLC %q does not follow %q", h, prev)
		}
		prev = h
	}
	g.clocks = 3_600_000 * 16
	if h := g.hlc(); h != "1700003600000-0000-datagen" {
		t.Fatalf("HLC after an hour of ops = %q", h)
	}
}
