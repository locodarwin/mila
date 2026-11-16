package accounts

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The server's token table and renames against shared/theme-tokens.json,
// which the app reads: every token in both, the same in each.
func TestThemeTokensMatchRegistry(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/theme-tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Renamed map[string]string `json:"renamed"`
		Tokens  []struct {
			Name      string   `json:"name"`
			Type      string   `json:"type"`
			PerScheme bool     `json:"perScheme"`
			Rank      int      `json:"rank"`
			Alpha     bool     `json:"alpha"`
			Min       *float64 `json:"min"`
			Max       *float64 `json:"max"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Tokens) == 0 {
		t.Fatal("no tokens in the registry")
	}
	seen := map[string]bool{}
	for _, r := range registry.Tokens {
		seen[r.Name] = true
		got, ok := themeTokens[r.Name]
		if !ok {
			t.Errorf("%s is in the registry, not in themetokens.go", r.Name)
			continue
		}
		want := themeToken{typ: tokenType(r.Type), perScheme: r.PerScheme, rank: r.Rank, alpha: r.Alpha}
		if r.Min != nil {
			want.min, want.max = *r.Min, *r.Max
		}
		if got != want {
			t.Errorf("%s: themetokens.go has %+v, the registry %+v", r.Name, got, want)
		}
	}
	for name := range themeTokens {
		if !seen[name] {
			t.Errorf("%s is in themetokens.go, not in the registry", name)
		}
	}
	if registry.Renamed == nil {
		registry.Renamed = map[string]string{}
	}
	if !reflect.DeepEqual(renamedTokens, registry.Renamed) {
		t.Errorf("renamed: themetokens.go has %v, the registry %v", renamedTokens, registry.Renamed)
	}
}
