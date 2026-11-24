package accounts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

type settingsEvents struct {
	mu      sync.Mutex
	changed []string
}

func (e *settingsEvents) SessionEnded(string)          {}
func (e *settingsEvents) MembershipChanged(...string)  {}
func (e *settingsEvents) AccessChanged(string, string) {}
func (e *settingsEvents) PeopleChanged(...string)      {}
func (e *settingsEvents) ThemesChanged(string, string) {}
func (e *settingsEvents) SettingsChanged(userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.changed = append(e.changed, userID)
}

func patch(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var p map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreferences(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		events := &settingsEvents{}
		f.svc.SetEvents(events)
		in := f.setup(t)
		id := in.User.ID

		if got := accounts.PreferencesOf(in.User); !reflect.DeepEqual(got, accounts.DefaultPreferences()) {
			t.Fatalf("new user's preferences = %+v", got)
		}

		// One device changes the theme, another the clock: both stay.
		if _, _, err := f.svc.SavePreferences(ctx, id, patch(t, `{"theme":"calm","accentHue":200}`)); err != nil {
			t.Fatal(err)
		}
		p, u, err := f.svc.SavePreferences(ctx, id, patch(t, `{"clock":"24","sidebarOrder":["projects","inbox"]}`))
		if err != nil {
			t.Fatal(err)
		}
		if p.Theme != "calm" || p.AccentHue == nil || *p.AccentHue != 200 || p.Clock != "24" {
			t.Fatalf("merged preferences = %+v", p)
		}
		// Items left out of an order each go after the one before them in
		// the usual order.
		if want := []string{"projects", "inbox", "today", "upcoming", "wiki", "time", "reports", "views"}; !reflect.DeepEqual(p.SidebarOrder, want) {
			t.Fatalf("sidebar order = %v", p.SidebarOrder)
		}
		if !reflect.DeepEqual(accounts.PreferencesOf(u), p) {
			t.Fatalf("stored %+v, returned %+v", accounts.PreferencesOf(u), p)
		}

		// Every font size, XL and XXL included, is kept.
		for _, size := range []string{"s", "m", "l", "xl", "xxl"} {
			if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"fontSize":"`+size+`"}`)); err != nil || p.FontSize != size {
				t.Fatalf("font size %s = %+v, %v", size, p, err)
			}
		}

		// The wiki's toolbar shows until it is turned off.
		if !accounts.DefaultPreferences().WikiToolbar {
			t.Fatal("the wiki toolbar is off by default")
		}
		if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"wikiToolbar":false}`)); err != nil || p.WikiToolbar {
			t.Fatalf("wiki toolbar off = %+v, %v", p, err)
		}

		// Null resets the accent to the theme's.
		if p, _, _ = f.svc.SavePreferences(ctx, id, patch(t, `{"accentHue":null}`)); p.AccentHue != nil {
			t.Fatalf("accent after reset = %v", *p.AccentHue)
		}

		// One bad value saves nothing.
		for _, bad := range []string{
			`{"theme":"neon"}`,
			`{"accentHue":360}`,
			`{"accentHue":"red"}`,
			`{"showCompleted":"yes"}`,
			`{"sidebarHidden":["favorites"]}`,
			`{"sidebarOrder":["inbox","inbox"]}`,
			`{"startPage":"trash"}`,
			`{"taskPanel":"tabs"}`,
			`{"wikiToolbar":"no"}`,
			`{"fontSize":"xxxl"}`,
			`{"density":"dense","colour":"blue"}`,
			`{"shortcuts":{"Quick-Add":["q"]}}`,
			`{"shortcuts":{"quick-add":["q","q"]}}`,
			`{"shortcuts":{"quick-add":["a","b","c","d","e"]}}`,
			`{"shortcuts":{"quick-add":["Ctrl+k"]}}`,
			`{"shortcuts":{"quick-add":"q"}}`,
			`{"shortcuts":["q"]}`,
			`{"themeAnswers":["not-an-id"]}`,
			`{"themeAnswers":"0b0e9f39-2a43-4d9c-9a6e-31f5c0a1d001"}`,
		} {
			_, _, err := f.svc.SavePreferences(ctx, id, patch(t, bad))
			wantKind(t, err, accounts.Invalid)
		}
		u, err = f.st.GetUser(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got := accounts.PreferencesOf(u); got.Density != "comfortable" || got.Theme != "calm" {
			t.Fatalf("after refused changes = %+v", got)
		}

		// Shortcuts: changed keys by command id, kept as sent; null and
		// empty lists stay empty, never null.
		p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"shortcuts":{"quick-add":["n","Alt+n"],"go.inbox":["g n"],"undo":null}}`))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][]string{"quick-add": {"n", "Alt+n"}, "go.inbox": {"g n"}, "undo": {}}
		if !reflect.DeepEqual(p.Shortcuts, want) {
			t.Fatalf("shortcuts = %#v", p.Shortcuts)
		}
		if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"shortcuts":null}`)); err != nil || p.Shortcuts == nil || len(p.Shortcuts) != 0 {
			t.Fatalf("shortcuts after null = %#v, %v", p.Shortcuts, err)
		}
		// Up to 200 commands.
		many := map[string][]string{}
		for i := range 201 {
			many[fmt.Sprintf("command-%d", i)] = []string{"F2"}
		}
		raw, _ := json.Marshal(map[string]any{"shortcuts": many})
		_, _, err = f.svc.SavePreferences(ctx, id, patch(t, string(raw)))
		wantKind(t, err, accounts.Invalid)
		delete(many, "command-0")
		raw, _ = json.Marshal(map[string]any{"shortcuts": many})
		if _, _, err := f.svc.SavePreferences(ctx, id, patch(t, string(raw))); err != nil {
			t.Fatalf("200 shortcuts: %v", err)
		}

		// The suggestions answered: theme ids, up to 100; null is empty.
		answers := make([]string, 101)
		for i := range answers {
			answers[i] = fmt.Sprintf("0b0e9f39-2a43-4d9c-9a6e-31f5c0a1%04d", i)
		}
		raw, _ = json.Marshal(map[string]any{"themeAnswers": answers})
		_, _, err = f.svc.SavePreferences(ctx, id, patch(t, string(raw)))
		wantKind(t, err, accounts.Invalid)
		raw, _ = json.Marshal(map[string]any{"themeAnswers": answers[1:]})
		if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, string(raw))); err != nil || len(p.ThemeAnswers) != 100 || p.ThemeAnswers[0] != answers[1] {
			t.Fatalf("100 answers: %v, %v", p.ThemeAnswers, err)
		}
		if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"themeAnswers":null}`)); err != nil || p.ThemeAnswers == nil || len(p.ThemeAnswers) != 0 {
			t.Fatalf("answers after null = %#v, %v", p.ThemeAnswers, err)
		}

		// The user's other devices hear of each saved change.
		events.mu.Lock()
		if len(events.changed) != 14 || events.changed[0] != id {
			t.Fatalf("settings events = %v", events.changed)
		}
		events.mu.Unlock()

		// Other settings are left alone.
		if _, err := f.svc.SetTimezone(ctx, id, "Europe/Berlin", accounts.Locale{Clock: "24", DateOrder: "day-month"}); err != nil {
			t.Fatal(err)
		}
		u, _ = f.st.GetUser(ctx, id)
		if accounts.Timezone(u) != "Europe/Berlin" || accounts.PreferencesOf(u).Theme != "calm" {
			t.Fatalf("settings = %s", u.Settings)
		}
		_, err = f.svc.SetTimezone(ctx, id, "Europe/Berlin", accounts.Locale{Clock: "13"})
		wantKind(t, err, accounts.Invalid)
	})
}

func TestStoredPreferencesThatNoLongerFit(t *testing.T) {
	u := store.User{Settings: `{"preferences":{"theme":"neon","density":"dense","sidebarHidden":["views"],"accentHue":999}}`}
	p := accounts.PreferencesOf(u)
	if p.Theme != "standard" || p.Density != "dense" || p.AccentHue != nil || !reflect.DeepEqual(p.SidebarHidden, []string{"views"}) {
		t.Fatalf("preferences = %+v", p)
	}
	// A stored shortcuts map that no longer fits keeps the default, {}.
	if p := accounts.PreferencesOf(store.User{Settings: `{"preferences":{"shortcuts":{"quick-add":["q","q"]},"clock":"12"}}`}); p.Shortcuts == nil || len(p.Shortcuts) != 0 || p.Clock != "12" {
		t.Fatalf("preferences with a bad shortcuts map = %+v", p)
	}
	if p := accounts.PreferencesOf(store.User{Settings: `not json`}); !reflect.DeepEqual(p, accounts.DefaultPreferences()) {
		t.Fatalf("unreadable settings gave %+v", p)
	}
}

// TestKeyBindings holds the server to the grammar the web app writes
// bindings in, through the cases both read.
func TestKeyBindings(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/keybinding-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct{ Good, Bad []string }
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Good) < 10 || len(vectors.Bad) < 10 {
		t.Fatalf("too few vectors: %d good, %d bad", len(vectors.Good), len(vectors.Bad))
	}
	stored := func(b string) map[string][]string {
		settings, _ := json.Marshal(map[string]any{"preferences": map[string]any{"shortcuts": map[string][]string{"quick-add": {b}}}})
		return accounts.PreferencesOf(store.User{Settings: string(settings)}).Shortcuts
	}
	for _, b := range vectors.Good {
		if got := stored(b); len(got["quick-add"]) != 1 {
			t.Errorf("%q refused", b)
		}
	}
	for _, b := range vectors.Bad {
		if got := stored(b); len(got) != 0 {
			t.Errorf("%q taken: %v", b, got)
		}
	}
}

func TestDateStyle(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	nov2 := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	jan4 := time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		settings          string
		clock, day, other string
		long              string
	}{
		// Before the device has said anything: 24-hour, month first.
		{`{}`, "14:30", "Mon, Nov 2", "Mon, Jan 4, 2027", "Thursday, October 29"},
		// "auto" follows the device.
		{`{"locale":{"clock":"12","dateOrder":"month-day"}}`, "2:30 PM", "Mon, Nov 2", "Mon, Jan 4, 2027", "Thursday, October 29"},
		{`{"locale":{"clock":"24","dateOrder":"day-month"}}`, "14:30", "Mon, 2 Nov", "Mon, 4 Jan 2027", "Thursday 29 October"},
		// A choice wins over the device.
		{`{"locale":{"clock":"12","dateOrder":"month-day"},"preferences":{"clock":"24","dateFormat":"iso"}}`, "14:30", "2026-11-02", "2027-01-04", "Thursday, 2026-10-29"},
		{`{"preferences":{"clock":"12","dateFormat":"day-month"}}`, "2:30 PM", "Mon, 2 Nov", "Mon, 4 Jan 2027", "Thursday 29 October"},
	} {
		s := accounts.DateStyleOf(store.User{Settings: c.settings})
		if got := s.Time("14:30"); got != c.clock {
			t.Errorf("%s: time = %q, want %q", c.settings, got, c.clock)
		}
		if got := s.Day(nov2, true, now); got != c.day {
			t.Errorf("%s: day = %q, want %q", c.settings, got, c.day)
		}
		if got := s.Day(jan4, true, now); got != c.other {
			t.Errorf("%s: next year's day = %q, want %q", c.settings, got, c.other)
		}
		if got := s.LongDay(now); got != c.long {
			t.Errorf("%s: long day = %q, want %q", c.settings, got, c.long)
		}
	}
	if got := (accounts.DateStyle{Hour12: true}).Time("09:05"); got != "9:05 AM" {
		t.Errorf("morning = %q", got)
	}
	if got := (accounts.DateStyle{Hour12: true}).Time("soon"); got != "soon" {
		t.Errorf("not a time = %q", got)
	}
}
