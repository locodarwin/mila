package ownership

import (
	"strings"
	"testing"

	"github.com/locodarwin/mila/server/internal/apperr"
)

// One row of a small zip must not unpack into gigabytes in memory.
func TestARowTooLargeIsRefused(t *testing.T) {
	huge := `{"rows":[{"entity":"task","id":"x","fields":{"title":"` + strings.Repeat("a", maxRowBytes+1) + `"}}]}`
	var c Content
	if err := decodeContent(strings.NewReader(huge), 10, 1<<30, &c); apperr.KindOf(err) != apperr.Invalid {
		t.Fatalf("a row past %d bytes = %v", maxRowBytes, err)
	}
	ok := `{"rows":[{"entity":"task","id":"x","fields":{"title":"Paint"}},{"entity":"task","id":"y","fields":{"title":"` + strings.Repeat("a", 1<<20) + `"}}],"activity":[],"people":[]}`
	c = Content{}
	if err := decodeContent(strings.NewReader(ok), 10, 1<<30, &c); err != nil || len(c.Rows) != 2 {
		t.Fatalf("rows within the cap = %d, %v", len(c.Rows), err)
	}
}

// Rows of a small export cannot decode into many times its size in memory:
// values that take few bytes as JSON (empty objects, zeros) take tens in
// memory, so what they would take is counted as they are read.
func TestDenseRowsAreRefused(t *testing.T) {
	for _, val := range []string{"0", "{}", "[]", `"a"`} {
		var b strings.Builder
		b.WriteString(`{"rows":[`)
		for i := range 8 {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"entity":"task","id":"x","fields":{"title":[` + val + strings.Repeat(","+val, (1<<20)/len(val)) + `]}}`)
		}
		b.WriteString(`]}`)
		budget := int64(b.Len()) // what honest rows of this many bytes take, at most
		var c Content
		if err := decodeContent(strings.NewReader(b.String()), 10, budget, &c); apperr.KindOf(err) != apperr.Invalid {
			t.Errorf("rows of %s within %d bytes = %v", val, budget, err)
		}
		c = Content{}
		if err := decodeContent(strings.NewReader(b.String()), 10, 128*budget, &c); err != nil || len(c.Rows) != 8 {
			t.Errorf("rows of %s within a budget that holds them = %d, %v", val, len(c.Rows), err)
		}
	}
}
