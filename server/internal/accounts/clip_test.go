package accounts

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A name cut to fit the security log stays valid UTF-8, which Postgres
// needs: a workspace's name of 98 emoji, cut by bytes, once failed the
// insert and the action with it.
func TestClipCutsOnACharactersEdge(t *testing.T) {
	name := "aa" + strings.Repeat("😀", 98)
	for n := range 12 {
		if got := clip(name, 300+n); !utf8.ValidString(got) || len(got) > 300+n {
			t.Fatalf("clip(%d) = %q", 300+n, got)
		}
	}
	if got := clip("ok\xffthen", 100); got != "ok�then" {
		t.Fatalf("invalid bytes = %q", got)
	}
	if got := clip("short", 100); got != "short" {
		t.Fatalf("short = %q", got)
	}
}

// Names show on other people's screens and in email: escapes and bidi
// overrides become spaces, while emoji joined by a ZWJ stay whole.
func TestCleanNameKeepsOnlyText(t *testing.T) {
	for raw, want := range map[string]string{
		"Team\x1b[2K one":                "Team [2K one",
		"invoice\u202Etxt.exe":           "invoice txt.exe",
		"Dev \U0001F469\u200D\U0001F4BB": "Dev \U0001F469\u200D\U0001F4BB",
		"tab\there":                      "tab here",
	} {
		got, err := cleanName(raw, "A name")
		if err != nil || got != want {
			t.Errorf("cleanName(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := cleanName("a\x00b", "A name"); err == nil {
		t.Error("a NUL was taken")
	}
}

// Invisible format characters (a zero-width space, the marks that set a
// direction, a BOM, a soft hyphen) are left out of a name, or one member
// could read exactly as another. The joiners stay, and so do the tags
// that make a subdivision's flag.
func TestCleanNameDropsInvisibleCharacters(t *testing.T) {
	england := "\xf0\x9f\x8f\xb4\xf3\xa0\x81\xa7\xf3\xa0\x81\xa2\xf3\xa0\x81\xa5\xf3\xa0\x81\xae\xf3\xa0\x81\xa7\xf3\xa0\x81\xbf"
	for raw, want := range map[string]string{
		"Shawn\xe2\x80\x8b":             "Shawn",          // U+200B
		"Sh\xe2\x80\x8eaw\xe2\x80\x8fn": "Shawn",          // LRM, RLM
		"\xef\xbb\xbfShawn":             "Shawn",          // BOM
		"Sh\xc2\xadawn":                 "Shawn",          // soft hyphen
		"Sha\xe2\x81\xa0wn\xe2\x81\xa4": "Shawn",          // word joiner, invisible plus
		"\xd8\x9cShawn":                 "Shawn",          // ALM
		"Shawn\xf3\xa0\x81\x81":         "Shawn",          // a tag with no flag before it
		"a\xe2\x80\x8cb":                "a\xe2\x80\x8cb", // ZWNJ stays
		"Go " + england:                 "Go " + england,
		"\xe2\x80\x8b":                  "",
	} {
		got, err := cleanName(raw, "A name")
		if want == "" {
			if err == nil {
				t.Errorf("cleanName(%q) = %q, want it refused as empty", raw, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("cleanName(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

// A slug is ASCII letters, digits and dashes, cut on a character's edge,
// whatever the name: a non-ASCII digit at the cut once made it invalid
// UTF-8, which Postgres refuses, failing sign-up, a new workspace or an
// import with such a name.
func TestSlugIsASCII(t *testing.T) {
	for _, name := range []string{
		strings.Repeat("a", 39) + "\xd9\xa3", // ARABIC-INDIC DIGIT THREE
		strings.Repeat("a", 38) + "\xd9\xa3\xd9\xa3",
		"\xd9\xa3\xd9\xa4",
		"D\xc3\xa9j\xc3\xa0 vu 2",
		strings.Repeat("\xf0\x9f\x98\x80", 50),
		"Team \xef\xbc\x91", // FULLWIDTH DIGIT ONE
		"",
	} {
		slug := slugFor(name)
		if !utf8.ValidString(slug) || len(slug) > 47 {
			t.Errorf("slugFor(%q) = %q", name, slug)
		}
		for _, r := range slug {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				t.Errorf("slugFor(%q) = %q, which holds %q", name, slug, r)
				break
			}
		}
	}
	if got := slugFor("Launch: v2"); !strings.HasPrefix(got, "launch-v2-") {
		t.Errorf("slugFor(Launch: v2) = %q", got)
	}
}
