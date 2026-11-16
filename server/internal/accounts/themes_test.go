package accounts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// testThemeID is a theme id made from a number.
func testThemeID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// testTheme is a small valid theme document.
func testTheme(id, name string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"mila": "theme", "version": 1, "id": id, "name": name, "base": "calm",
		"light": map[string]any{"surface-0": map[string]any{"oklch": []float64{0.98, 0.01, 85}}},
	})
	return raw
}

// themeEvents records the themes changed, as user/theme.
type themeEvents struct {
	recorder
	mu     sync.Mutex
	themes []string
}

func (e *themeEvents) ThemesChanged(userID, themeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.themes = append(e.themes, userID+"/"+themeID)
}

func (e *themeEvents) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.themes
	e.themes = nil
	return out
}

func TestThemeVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/theme-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Valid []struct {
			Name       string          `json:"name"`
			Raw        string          `json:"raw"`
			Normalized json.RawMessage `json:"normalized"`
		} `json:"valid"`
		Invalid []struct {
			Name string `json:"name"`
			Raw  string `json:"raw"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Valid) == 0 || len(vectors.Invalid) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range vectors.Valid {
		got, err := accounts.ValidateTheme([]byte(v.Raw))
		if err != nil {
			t.Errorf("%s: refused: %v", v.Name, err)
			continue
		}
		var a, b any
		_ = json.Unmarshal(got.Body, &a)
		_ = json.Unmarshal(v.Normalized, &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s:\n got %s\nwant %s", v.Name, got.Body, v.Normalized)
		}
		// What is stored validates to itself.
		again, err := accounts.ValidateTheme(got.Body)
		if err != nil || string(again.Body) != string(got.Body) {
			t.Errorf("%s: stored form changes on a second pass: %s, %v", v.Name, again.Body, err)
		}
	}
	for _, v := range vectors.Invalid {
		_, err := accounts.ValidateTheme([]byte(v.Raw))
		if accounts.KindOf(err) != accounts.Invalid {
			t.Errorf("%s: want refused, got %v", v.Name, err)
		}
	}
}

func TestThemes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		events := &themeEvents{}
		f.svc.SetEvents(events)
		owner := f.setup(t)
		me := owner.User.ID
		ws, err := f.svc.CreateWorkspace(ctx, me, "Team")
		if err != nil {
			t.Fatal(err)
		}
		other, _, err := f.svc.AcceptInvite(ctx, f.invite(t, owner.User, ws.ID, "dana@example.com", authz.Member), nil, "Dana", accounts.Client{UserAgent: "test"})
		if err != nil {
			t.Fatal(err)
		}

		// Saved, listed and read back as stored, with its name.
		id := testThemeID(1)
		saved, err := f.svc.SaveTheme(ctx, me, id, testTheme(id, "  Dusk\u0007 "))
		if err != nil {
			t.Fatal(err)
		}
		if saved.ID != id || saved.Name != "Dusk" || saved.CreatedAt.IsZero() || !saved.CreatedAt.Equal(saved.UpdatedAt) {
			t.Fatalf("saved = %+v", saved)
		}
		if got := events.take(); !reflect.DeepEqual(got, []string{me + "/" + id}) {
			t.Fatalf("theme events = %v", got)
		}
		list, err := f.svc.ListThemes(ctx, me)
		if err != nil || len(list) != 1 || string(list[0].Body) != string(saved.Body) {
			t.Fatalf("list = %+v, %v", list, err)
		}

		// Saving again replaces it, keeping when it was made.
		again, err := f.svc.SaveTheme(ctx, me, id, testTheme(id, "Dawn"))
		if err != nil {
			t.Fatal(err)
		}
		if again.Name != "Dawn" || !again.CreatedAt.Equal(saved.CreatedAt) || again.UpdatedAt.Before(saved.UpdatedAt) {
			t.Fatalf("replaced = %+v", again)
		}
		got, err := f.svc.GetTheme(ctx, me, id)
		if err != nil || got.Name != "Dawn" {
			t.Fatalf("get = %+v, %v", got, err)
		}

		// A refused document, or one whose id is not the address's, saves nothing.
		_, err = f.svc.SaveTheme(ctx, me, id, []byte(`{"mila":"theme"}`))
		wantKind(t, err, accounts.Invalid)
		_, err = f.svc.SaveTheme(ctx, me, testThemeID(2), testTheme(id, "Elsewhere"))
		wantKind(t, err, accounts.Invalid)
		if got, _ := f.svc.GetTheme(ctx, me, id); got.Name != "Dawn" {
			t.Fatalf("after refused saves: %+v", got)
		}
		if got := events.take(); len(got) != 1 {
			t.Fatalf("theme events = %v", got)
		}

		// Someone else's theme is not found, whatever is done to it.
		_, err = f.svc.GetTheme(ctx, other.User.ID, id)
		wantKind(t, err, accounts.NotFound)
		_, err = f.svc.SaveTheme(ctx, other.User.ID, id, testTheme(id, "Mine now"))
		wantKind(t, err, accounts.NotFound)
		wantKind(t, f.svc.DeleteTheme(ctx, other.User.ID, id), accounts.NotFound)
		if got, _ := f.svc.GetTheme(ctx, me, id); got.Name != "Dawn" {
			t.Fatalf("after someone else's tries: %+v", got)
		}
		if got, _ := f.svc.ListThemes(ctx, other.User.ID); len(got) != 0 {
			t.Fatalf("someone else lists %+v", got)
		}

		// Deleted, it is gone; deleting again finds nothing.
		if err := f.svc.DeleteTheme(ctx, me, id); err != nil {
			t.Fatal(err)
		}
		if got := events.take(); !reflect.DeepEqual(got, []string{me + "/" + id}) {
			t.Fatalf("theme events after delete = %v", got)
		}
		_, err = f.svc.GetTheme(ctx, me, id)
		wantKind(t, err, accounts.NotFound)
		wantKind(t, f.svc.DeleteTheme(ctx, me, id), accounts.NotFound)

		// At most MaxThemes, even when new ones arrive at once; replacing
		// one at the limit still works.
		var wg sync.WaitGroup
		for i := range accounts.MaxThemes + 5 {
			wg.Go(func() {
				tid := testThemeID(100 + i)
				_, err := f.svc.SaveTheme(ctx, me, tid, testTheme(tid, fmt.Sprintf("Theme %d", i)))
				if err != nil && accounts.KindOf(err) != accounts.Invalid {
					t.Errorf("save %d: %v", i, err)
				}
			})
		}
		wg.Wait()
		list, err = f.svc.ListThemes(ctx, me)
		if err != nil || len(list) != accounts.MaxThemes {
			t.Fatalf("%d themes after saving %d at once, %v", len(list), accounts.MaxThemes+5, err)
		}
		tid := testThemeID(999)
		_, err = f.svc.SaveTheme(ctx, me, tid, testTheme(tid, "One too many"))
		wantKind(t, err, accounts.Invalid)
		if e, ok := err.(*accounts.Error); !ok || e.Code != accounts.TooManyThemes {
			t.Fatalf("one too many = %#v, want code %s", err, accounts.TooManyThemes)
		}
		if _, err := f.svc.SaveTheme(ctx, me, list[0].ID, testTheme(list[0].ID, "Renamed")); err != nil {
			t.Fatalf("replacing at the limit: %v", err)
		}
	})
}

func TestThemeSize(t *testing.T) {
	id := testThemeID(1)
	// A theme just within the limit is kept; one byte more is not.
	doc := func(name string, n int) []byte {
		light := map[string]any{}
		for i := range n {
			light[fmt.Sprintf("palette-%s", []string{"red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}[i])] = map[string]any{"oklch": []float64{0.5, 0.1, 20}}
		}
		raw, _ := json.Marshal(map[string]any{"mila": "theme", "version": 1, "id": id, "name": name, "base": "standard", "light": light, "dark": light})
		return raw
	}
	small, err := accounts.ValidateTheme(doc("x", 12))
	if err != nil {
		t.Fatal(err)
	}
	if len(small.Body) > accounts.MaxThemeBytes {
		t.Fatalf("a small theme is %d bytes", len(small.Body))
	}
	// Names are text, never HTML-escaped: < counts one byte, as in the app.
	v, err := accounts.ValidateTheme(doc("<&>", 0))
	if err != nil || !json.Valid(v.Body) || !bytes.Contains(v.Body, []byte(`"name":"<&>"`)) {
		t.Fatalf("name with markup: %s, %v", v.Body, err)
	}
}

func TestCustomThemePreference(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		in := f.setup(t)
		id := in.User.ID
		if accounts.DefaultPreferences().CustomTheme != nil {
			t.Fatal("a custom theme by default")
		}
		// Any id of the right shape, whether or not the theme exists yet.
		p, _, err := f.svc.SavePreferences(ctx, id, patch(t, `{"customTheme":"`+testThemeID(7)+`","theme":"calm"}`))
		if err != nil || p.CustomTheme == nil || *p.CustomTheme != testThemeID(7) {
			t.Fatalf("customTheme = %v, %v", p.CustomTheme, err)
		}
		for _, bad := range []string{`{"customTheme":"night"}`, `{"customTheme":7}`, `{"customTheme":"` + testThemeID(7)[:35] + `"}`, `{"customTheme":"00000000-0000-4000-8000-00000000000A"}`} {
			_, _, err := f.svc.SavePreferences(ctx, id, patch(t, bad))
			wantKind(t, err, accounts.Invalid)
		}
		if p, _, err = f.svc.SavePreferences(ctx, id, patch(t, `{"customTheme":null}`)); err != nil || p.CustomTheme != nil {
			t.Fatalf("customTheme after null = %v, %v", p.CustomTheme, err)
		}
	})
}

func TestThemeUse(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d)
		owner := f.setup(t).User
		dana := f.member(t, owner, "dana@example.com", "Dana")
		list, err := f.svc.Workspaces(ctx, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		var ws string
		for _, w := range list {
			if !w.IsPersonal {
				ws = w.ID
			}
		}
		offered, gone := uuid.NewString(), uuid.NewString()
		for _, id := range []string{offered, gone} {
			var deleted any
			if id == gone {
				deleted = store.Now()
			}
			if _, err := d.ExecContext(ctx, rebind(d, `INSERT INTO workspace_themes (id, workspace_id, name, body, sort_key, deleted_at, clocks, created_at, updated_at)
				VALUES (?, ?, 'Harbor', '{}', 'a0', ?, '{}', ?, ?)`), id, ws, deleted, store.Now(), store.Now()); err != nil {
				t.Fatal(err)
			}
		}
		// Both use the offered theme; one of their own themes is not counted.
		for _, u := range []string{owner.ID, dana.ID} {
			if _, _, err := f.svc.SavePreferences(ctx, u, patch(t, `{"customTheme":"`+offered+`"}`)); err != nil {
				t.Fatal(err)
			}
		}
		counts, err := f.svc.ThemeUse(ctx, owner.ID, ws)
		if err != nil {
			t.Fatal(err)
		}
		if len(counts) != 1 || counts[offered] != 2 {
			t.Fatalf("counts = %v", counts)
		}
		// A member may not ask.
		_, err = f.svc.ThemeUse(ctx, dana.ID, ws)
		wantKind(t, err, accounts.Forbidden)
	})
}
