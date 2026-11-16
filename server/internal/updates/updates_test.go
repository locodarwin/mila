package updates

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"v2.9.0", "v2.8.0", true},
		{"v2.10.0", "v2.9.3", true},
		{"v3.0.0", "v2.99.99", true},
		{"v2.8.0", "v2.8.0", false},
		// A build after a release is that release, so the release is no news.
		{"v2.8.0", "v2.8.0-6-g0d7dbdd-dirty", false},
		{"v2.8.1", "v2.8.0-6-g0d7dbdd", true},
		{"v2.7.0", "v2.8.0", false},
		{"v2.9.0", "dev", false},
		{"nonsense", "v2.8.0", false},
	} {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestCheck(t *testing.T) {
	answer := `{"tag_name":"v2.9.0","html_url":"https://github.com/locodarwin/mila/releases/tag/v2.9.0","published_at":"2026-11-20T10:00:00Z"}`
	var agent string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(answer))
	}))
	defer s.Close()
	c := New("v2.8.0", slog.New(slog.DiscardHandler))
	c.url = s.URL
	if c.Newer() != nil {
		t.Fatal("news before any check")
	}
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := c.Newer()
	if r == nil || r.Version != "v2.9.0" || r.URL != "https://github.com/locodarwin/mila/releases/tag/v2.9.0" || r.PublishedAt.Day() != 20 {
		t.Fatalf("newer: %+v", r)
	}
	if agent != "Mila" {
		t.Errorf("sent %q as the user agent", agent)
	}

	// A link anywhere else is not shown; one to the release is made.
	answer = `{"tag_name":"v2.9.1","html_url":"https://evil.example.net/x","published_at":"2026-11-21T10:00:00Z"}`
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r := c.Newer(); r.URL != "https://github.com/locodarwin/mila/releases/tag/v2.9.1" {
		t.Errorf("link: %q", r.URL)
	}
	// Prereleases and tags that are no version are not news, and leave what was known.
	for _, a := range []string{
		`{"tag_name":"v3.0.0","prerelease":true}`,
		`{"tag_name":"latest"}`,
	} {
		answer = a
		if err := c.Check(context.Background()); err == nil {
			t.Errorf("%s was taken", a)
		}
	}
	if r := c.Newer(); r.Version != "v2.9.1" {
		t.Errorf("after a bad answer: %+v", r)
	}
	// A private repository answers 404.
	s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if err := c.Check(context.Background()); err == nil {
		t.Error("a 404 was taken")
	}
}
