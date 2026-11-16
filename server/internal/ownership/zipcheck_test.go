package ownership

import (
	"archive/zip"
	"bytes"
	"fmt"
	"testing"
)

func zipOf(t *testing.T, n int, prefix []byte, comment string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(prefix)
	w := zip.NewWriter(&buf)
	w.SetOffset(int64(len(prefix)))
	for i := range n {
		if _, err := w.Create(fmt.Sprintf("f%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if comment != "" {
		if err := w.SetComment(comment); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// countEntries reads as many records as archive/zip does, zip64 and
// data before the zip included, and stops counting past its limit.
func TestCountEntries(t *testing.T) {
	cases := []struct {
		name    string
		b       []byte
		limit   int
		want    int
		archive bool // archive/zip must agree
	}{
		{"empty", zipOf(t, 0, nil, ""), 10, 0, true},
		{"a few", zipOf(t, 3, nil, "a comment"), 10, 3, true},
		{"zip64", zipOf(t, 70_000, nil, ""), 100_000, 70_000, true},
		{"after other data", zipOf(t, 5, bytes.Repeat([]byte("x"), 300), ""), 10, 5, true},
		{"too many", zipOf(t, 50, nil, ""), 10, 11, false},
	}
	for _, c := range cases {
		got, err := countEntries(bytes.NewReader(c.b), int64(len(c.b)), c.limit)
		if err != nil || got != c.want {
			t.Errorf("%s: counted %d, %v; want %d", c.name, got, err, c.want)
		}
		if c.archive {
			z, err := zip.NewReader(bytes.NewReader(c.b), int64(len(c.b)))
			if err != nil || len(z.File) != got {
				t.Errorf("%s: archive/zip read %v, %v", c.name, z, err)
			}
		}
	}
	if _, err := countEntries(bytes.NewReader([]byte("not a zip")), 9, 10); err == nil {
		t.Error("counted entries in what is not a zip")
	}
}

func TestRowLimit(t *testing.T) {
	if rowLimit(0) != minRows || rowLimit(1<<20) != (1<<20)/minRowBytes || rowLimit(1<<40) != 2_000_000 {
		t.Errorf("row limits %d %d %d", rowLimit(0), rowLimit(1<<20), rowLimit(1<<40))
	}
}
