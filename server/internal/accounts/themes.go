package accounts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// A custom theme is a document that overrides some of a built-in theme's
// tokens (shared/theme-tokens.json): web/src/lib/theme/document.ts reads
// it in the app, and ValidateTheme checks every one sent here, strictly,
// to the same rules; shared/theme-vectors.json holds the two together.
// Only the app turns a theme into CSS, from numbers and names it knows,
// so nothing a theme holds ever reaches a stylesheet as written.

const (
	// themeVersion is the document format this server reads and writes.
	themeVersion = 1
	// MaxThemes is how many themes a person may keep.
	MaxThemes = 20
	// TooManyThemes is the error code of a new theme past MaxThemes, so
	// the app can say why it was not saved.
	TooManyThemes = "too_many_themes"
	// MaxThemeBytes is the most a theme may take, as compact JSON.
	MaxThemeBytes = 16 << 10
	maxThemeName  = 60
)

var (
	themeIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	themeBases     = []string{"standard", "calm", "playful", "contrast"}
	themeTargets   = []string{"AA", "AAA"}
	themeFonts     = []string{"inter", "nunito", "source-serif", "jetbrains-mono", "system-sans", "system-serif", "system-mono"}
	themeEases     = []string{"standard", "gentle", "spring", "snappy", "linear"}
	themeFields    = []string{"mila", "version", "id", "name", "base", "target", "accent", "tokens", "light", "dark"}
)

// ValidTheme is a theme document that passed ValidateTheme.
type ValidTheme struct {
	ID   string
	Name string
	// Body is the document as stored: compact JSON, with every default
	// filled in and every renamed token under its new name.
	Body []byte
}

// themeDoc is a theme in the order it is written back.
type themeDoc struct {
	Mila    string                    `json:"mila"`
	Version int                       `json:"version"`
	ID      string                    `json:"id"`
	Name    string                    `json:"name"`
	Base    string                    `json:"base"`
	Target  string                    `json:"target"`
	Accent  map[string]any            `json:"accent"`
	Tokens  map[string]map[string]any `json:"tokens"`
	Light   map[string]map[string]any `json:"light"`
	Dark    map[string]map[string]any `json:"dark"`
}

func invalidTheme(format string, args ...any) error {
	return fail(Invalid, "This theme cannot be saved: "+format, args...)
}

// ValidateTheme checks a theme document and returns it as stored. Unknown
// fields and tokens, values out of bounds, numbers sent as text and refs
// that do not point to a lower layer of the same kind are all refused.
func ValidateTheme(raw []byte) (ValidTheme, error) {
	top, ok := jsonObject(raw)
	if !ok {
		return ValidTheme{}, invalidTheme("it is not a theme document.")
	}
	for key := range top {
		if !slices.Contains(themeFields, key) {
			return ValidTheme{}, invalidTheme("%q is not part of a theme.", key)
		}
	}
	var mila string
	if json.Unmarshal(top["mila"], &mila) != nil || mila != "theme" {
		return ValidTheme{}, invalidTheme("it is not a theme document.")
	}
	version, ok := jsonNumber(top["version"])
	if !ok || version < 1 || version != math.Trunc(version) {
		return ValidTheme{}, invalidTheme("its version is not a version.")
	}
	if version > themeVersion {
		return ValidTheme{}, invalidTheme("it was made by a newer version of Mila.")
	}
	// Documents of an older version would be upgraded here, a step per
	// version, as the app's document.ts does; version 1 is the first.

	doc := themeDoc{Mila: "theme", Version: themeVersion, Target: "AA", Accent: map[string]any{}}
	if json.Unmarshal(top["id"], &doc.ID) != nil || !themeIDPattern.MatchString(doc.ID) {
		return ValidTheme{}, invalidTheme("its id is not an id.")
	}
	var name string
	if json.Unmarshal(top["name"], &name) != nil {
		return ValidTheme{}, invalidTheme("it needs a name.")
	}
	doc.Name = ThemeName(name)
	if n := utf8.RuneCountInString(doc.Name); n == 0 || n > maxThemeName {
		return ValidTheme{}, invalidTheme("a name is 1 to %d characters.", maxThemeName)
	}
	if json.Unmarshal(top["base"], &doc.Base) != nil || !slices.Contains(themeBases, doc.Base) {
		return ValidTheme{}, invalidTheme("its base must be one of %v.", themeBases)
	}
	if t, ok := top["target"]; ok {
		if json.Unmarshal(t, &doc.Target) != nil || !slices.Contains(themeTargets, doc.Target) {
			return ValidTheme{}, invalidTheme("its contrast goal must be AA or AAA.")
		}
	}
	if a, ok := top["accent"]; ok {
		accent, err := themeAccent(a)
		if err != nil {
			return ValidTheme{}, err
		}
		doc.Accent = accent
	}
	var err error
	if doc.Tokens, err = themeTokenMap(top, "tokens", false); err != nil {
		return ValidTheme{}, err
	}
	if doc.Light, err = themeTokenMap(top, "light", true); err != nil {
		return ValidTheme{}, err
	}
	if doc.Dark, err = themeTokenMap(top, "dark", true); err != nil {
		return ValidTheme{}, err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return ValidTheme{}, err
	}
	body := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(body) > MaxThemeBytes {
		return ValidTheme{}, invalidTheme("it is larger than %d KiB.", MaxThemeBytes>>10)
	}
	return ValidTheme{ID: doc.ID, Name: doc.Name, Body: body}, nil
}

// ThemeName is a theme's name as stored: without control characters, and
// without white space at either end.
func ThemeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cc, r) {
			return -1
		}
		return r
	}, s)
	// As JavaScript's trim(), which also takes a byte order mark.
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == 0xFEFF })
}

// themeAccent reads the accent: {} (the base's), {"hue": h} or {"manual": true}.
func themeAccent(raw json.RawMessage) (map[string]any, error) {
	a, ok := jsonObject(raw)
	bad := invalidTheme("its accent must be a hue from 0 to 360, or set by hand.")
	if !ok || len(a) > 1 {
		return nil, bad
	}
	if h, ok := a["hue"]; ok {
		hue, ok := jsonNumber(h)
		if !ok || hue < 0 || hue > 360 {
			return nil, bad
		}
		return map[string]any{"hue": hue}, nil
	}
	if m, ok := a["manual"]; ok {
		var manual bool
		if json.Unmarshal(m, &manual) != nil || !manual || !bytes.Equal(bytes.TrimSpace(m), []byte("true")) {
			return nil, bad
		}
		return map[string]any{"manual": true}, nil
	}
	if len(a) != 0 {
		return nil, bad
	}
	return map[string]any{}, nil
}

// themeTokenMap reads one of a theme's maps: tokens (for both schemes) or
// light or dark.
func themeTokenMap(top map[string]json.RawMessage, field string, perScheme bool) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	raw, ok := top[field]
	if !ok {
		return out, nil
	}
	entries, ok := jsonObject(raw)
	if !ok {
		return nil, invalidTheme("%s must be an object.", field)
	}
	// A token under its old name first, so its new name wins if both are there.
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		_, ra := renamedTokens[a]
		_, rb := renamedTokens[b]
		switch {
		case ra && !rb:
			return -1
		case rb && !ra:
			return 1
		}
		return strings.Compare(a, b)
	})
	for _, key := range keys {
		name := key
		if to, ok := renamedTokens[key]; ok {
			name = to
		}
		tok, ok := themeTokens[name]
		if !ok {
			return nil, invalidTheme("Mila has no setting %q.", key)
		}
		if tok.perScheme != perScheme {
			if perScheme {
				return nil, invalidTheme("%q is set for both schemes, in tokens.", name)
			}
			return nil, invalidTheme("%q is set for light and dark apart.", name)
		}
		v, err := themeValue(name, tok, entries[key])
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, nil
}

// themeValue reads one token's value, of the shapes its type allows.
func themeValue(name string, tok themeToken, raw json.RawMessage) (map[string]any, error) {
	bad := invalidTheme("the value of %q is not one Mila can use.", name)
	v, ok := jsonObject(raw)
	if !ok || len(v) == 0 {
		return nil, bad
	}
	if r, ok := v["ref"]; ok {
		var target string
		if len(v) != 1 || json.Unmarshal(r, &target) != nil {
			return nil, bad
		}
		if to, ok := renamedTokens[target]; ok {
			target = to
		}
		t, ok := themeTokens[target]
		if !ok || t.typ != tok.typ || t.perScheme != tok.perScheme || t.rank >= tok.rank || (t.alpha && !tok.alpha) {
			return nil, invalidTheme("%q cannot be the same as %q.", name, target)
		}
		return map[string]any{"ref": target}, nil
	}
	// The one number a value of this type holds, within the token's bounds.
	single := func(key string, lo, hi float64) (map[string]any, error) {
		n, ok := jsonNumber(v[key])
		if len(v) != 1 || !ok || n < lo || n > hi {
			return nil, bad
		}
		return map[string]any{key: n}, nil
	}
	switch tok.typ {
	case tokColor:
		for key := range v {
			if key != "oklch" && (key != "alpha" || !tok.alpha) {
				return nil, bad
			}
		}
		lch, ok := jsonNumbers(v["oklch"], 3)
		if !ok || !within(lch[0], 0, 1) || !within(lch[1], 0, 0.4) || !within(lch[2], 0, 360) {
			return nil, bad
		}
		out := map[string]any{"oklch": lch}
		if a, ok := v["alpha"]; ok {
			alpha, ok := jsonNumber(a)
			if !ok || !within(alpha, 0, 1) {
				return nil, bad
			}
			out["alpha"] = alpha
		}
		return out, nil
	case tokPx:
		return single("px", tok.min, tok.max)
	case tokMs:
		return single("ms", tok.min, tok.max)
	case tokNumber:
		return single("n", tok.min, tok.max)
	case tokHue:
		return single("hue", 0, 360)
	case tokFont:
		var id string
		if len(v) != 1 || json.Unmarshal(v["font"], &id) != nil || !slices.Contains(themeFonts, id) {
			return nil, bad
		}
		return map[string]any{"font": id}, nil
	case tokEase:
		if len(v) != 1 {
			return nil, bad
		}
		if e, ok := v["ease"]; ok {
			var id string
			if json.Unmarshal(e, &id) != nil || !slices.Contains(themeEases, id) {
				return nil, bad
			}
			return map[string]any{"ease": id}, nil
		}
		b, ok := jsonNumbers(v["bezier"], 4)
		if !ok || !within(b[0], 0, 1) || !within(b[2], 0, 1) || !within(b[1], -1, 2) || !within(b[3], -1, 2) {
			return nil, bad
		}
		return map[string]any{"bezier": b}, nil
	case tokShadow:
		var layers []json.RawMessage
		if len(v) != 1 || !jsonArray(v["layers"], &layers) || len(layers) > 3 {
			return nil, bad
		}
		out := make([]map[string]float64, 0, len(layers))
		for _, raw := range layers {
			l, ok := jsonObject(raw)
			if !ok || len(l) != 3 {
				return nil, bad
			}
			y, okY := jsonNumber(l["y"])
			blur, okB := jsonNumber(l["blur"])
			spread, okS := jsonNumber(l["spread"])
			if !okY || !okB || !okS || !within(y, -32, 64) || !within(blur, 0, 128) || !within(spread, -32, 32) {
				return nil, bad
			}
			out = append(out, map[string]float64{"y": y, "blur": blur, "spread": spread})
		}
		return map[string]any{"layers": out}, nil
	}
	return nil, bad
}

func within(n, lo, hi float64) bool { return n >= lo && n <= hi }

// jsonObject reads a JSON object, and only an object (not null).
func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	return m, true
}

// jsonArray reads a JSON array, and only an array (not null).
func jsonArray(raw json.RawMessage, out *[]json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '[' && json.Unmarshal(raw, out) == nil
}

// jsonNumber reads a JSON number, and only a number: never text, and
// never one too large to hold (1e999).
func jsonNumber(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return 0, false
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

// jsonNumbers reads an array of exactly n numbers.
func jsonNumbers(raw json.RawMessage, n int) ([]float64, bool) {
	var items []json.RawMessage
	if !jsonArray(raw, &items) || len(items) != n {
		return nil, false
	}
	out := make([]float64, n)
	for i, item := range items {
		v, ok := jsonNumber(item)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// Theme is one of a person's themes as stored.
type Theme struct {
	ID        string
	Name      string
	Body      json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

func themeOf(t sqlitedb.UserTheme) Theme {
	return Theme{ID: t.ID, Name: t.Name, Body: json.RawMessage(t.Body), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

// ListThemes returns a person's themes, oldest first.
func (s *Service) ListThemes(ctx context.Context, userID string) ([]Theme, error) {
	rows, err := s.st.ListUserThemes(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Theme, len(rows))
	for i, r := range rows {
		out[i] = themeOf(r)
	}
	return out, nil
}

// GetTheme returns one of a person's themes; someone else's is not found.
func (s *Service) GetTheme(ctx context.Context, userID, id string) (Theme, error) {
	row, err := s.st.GetUserTheme(ctx, sqlitedb.GetUserThemeParams{ID: id, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return Theme{}, fail(NotFound, "There is no such theme.")
	}
	if err != nil {
		return Theme{}, err
	}
	return themeOf(row), nil
}

// SaveTheme makes or replaces one of a person's themes with the document
// raw, whose id must be id. The whole theme is replaced: the later of two
// saves wins, and a save after a delete makes the theme again. A person
// keeps at most MaxThemes; replacing one never counts against that.
func (s *Service) SaveTheme(ctx context.Context, userID, id string, raw []byte) (Theme, error) {
	v, err := ValidateTheme(raw)
	if err != nil {
		return Theme{}, err
	}
	if v.ID != id {
		return Theme{}, invalidTheme("its id is not the one in the address.")
	}
	var saved Theme
	err = s.st.InTx(ctx, func(q store.Queries) error {
		// Held so that two saves of new themes at once cannot both slip
		// under the limit.
		if err := q.LockUser(ctx, userID); err != nil {
			return err
		}
		_, err := q.GetUserTheme(ctx, sqlitedb.GetUserThemeParams{ID: id, UserID: userID})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			n, err := q.CountUserThemes(ctx, userID)
			if err != nil {
				return err
			}
			if n >= MaxThemes {
				return &Error{Kind: Invalid, Code: TooManyThemes,
					Message: fmt.Sprintf("You have %d themes, the most Mila keeps. Delete one to add another.", MaxThemes)}
			}
		case err != nil:
			return err
		}
		rows, err := q.UpsertUserTheme(ctx, sqlitedb.UpsertUserThemeParams{
			ID: id, UserID: userID, Name: v.Name, Body: string(v.Body), Now: store.Now(),
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			// Someone else's theme has this id.
			return fail(NotFound, "There is no such theme.")
		}
		row, err := q.GetUserTheme(ctx, sqlitedb.GetUserThemeParams{ID: id, UserID: userID})
		saved = themeOf(row)
		return err
	})
	if err != nil {
		return Theme{}, err
	}
	s.events.ThemesChanged(userID, id)
	return saved, nil
}

// DeleteTheme deletes one of a person's themes. Their preferences may
// still name it; the app then shows the theme it was based on.
func (s *Service) DeleteTheme(ctx context.Context, userID, id string) error {
	rows, err := s.st.DeleteUserTheme(ctx, sqlitedb.DeleteUserThemeParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return fail(NotFound, "There is no such theme.")
	}
	s.events.ThemesChanged(userID, id)
	return nil
}

// ThemeUse counts, for each theme a workspace offers, the members using it
// now. Owners and admins may ask, to tell whether a theme is worth
// keeping; it names no one.
func (s *Service) ThemeUse(ctx context.Context, userID, workspaceID string) (map[string]int, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ManageMembers); err != nil {
		return nil, err
	}
	ids, err := s.st.ListWorkspaceThemeIDs(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		counts[id] = 0
	}
	members, err := s.st.ListMemberSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if id := PreferencesOf(store.User{Settings: m.Settings}).CustomTheme; id != nil {
			if _, ok := counts[*id]; ok {
				counts[*id]++
			}
		}
	}
	return counts, nil
}
