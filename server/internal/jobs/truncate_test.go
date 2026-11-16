package jobs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// An error's text cut to fit stays valid UTF-8, for Postgres.
func TestTruncateCutsOnACharactersEdge(t *testing.T) {
	text := strings.Repeat("é", 300)
	for n := range 4 {
		if got := truncate(text, 101+n); !utf8.ValidString(got) || len(got) > 101+n {
			t.Fatalf("truncate(%d) = %q", 101+n, got)
		}
	}
	if got := truncate("bad\xff", 100); got != "bad\uFFFD" {
		t.Fatalf("invalid bytes = %q", got)
	}
}
