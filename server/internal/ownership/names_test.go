package ownership

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNamesAndQuoting(t *testing.T) {
	for in, want := range map[string]string{
		`Plan: "v2"`:     `Plan- -v2-`,
		"CON":            "_CON",
		"lpt1.txt":       "_lpt1.txt",
		" ..Notes.. ":    "Notes",
		"a/b\\c?*":       "a-b-c--",
		"":               "Untitled",
		"tab\there\nnow": "tab here now",
	} {
		if got := safeName(in, "Untitled"); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
	used := map[string]bool{}
	if a, b, c := unique(used, "Call", ".md"), unique(used, "call", ".md"), unique(used, "Call", ".md"); a != "Call.md" || b != "call (2).md" || c != "Call (3).md" {
		t.Errorf("unique: %q %q %q", a, b, c)
	}
	for in, want := range map[string]string{
		"plain":           `"plain"`,
		"say \"hi\": now": `"say \"hi\": now"`,
		"two\nlines":      `"two\nlines"`,
		"back\\slash":     `"back\\slash"`,
		"bell\a":          `"bell\u0007"`,
	} {
		if got := yamlString(in); got != want {
			t.Errorf("yamlString(%q) = %s, want %s", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Size": "Size", "Due date": "Due date", "yes": `"yes"`, "2026": `"2026"`, "a:b": `"a:b"`, "-x": `"-x"`,
	} {
		if got := yamlKey(in); got != want {
			t.Errorf("yamlKey(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestCSVFieldsMatchTheApp holds csvField to shared/csv-vectors.json,
// which web/src/lib/csv.spec.ts holds the app's writer to.
func TestCSVFieldsMatchTheApp(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/csv-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Fields [][2]string }
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Fields) < 10 {
		t.Fatalf("only %d vectors", len(v.Fields))
	}
	for _, c := range v.Fields {
		if got := csvField(c[0]); got != c[1] {
			t.Errorf("csvField(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}
