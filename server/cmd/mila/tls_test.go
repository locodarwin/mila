package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/httpapi"
)

func TestToHTTPS(t *testing.T) {
	h := toHTTPS("https://mila.example.com")
	for _, c := range []struct{ method, target, host string }{
		{http.MethodGet, "/projects/1?tab=wiki", "mila.example.com"},
		// Whatever the request names as its host, it goes to the configured one.
		{http.MethodPost, "/api/v1/me", "evil.example.net"},
	} {
		r := httptest.NewRequest(c.method, c.target, nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "https://mila.example.com"+c.target {
			t.Errorf("%s %s: %d to %q", c.method, c.target, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestCertManager(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.TLSDomain = "mila.example.com"
	cfg.TLSCA = "https://ca.example.com/directory"
	m := certManager(cfg)
	if m.Client.DirectoryURL != cfg.TLSCA {
		t.Errorf("directory = %q", m.Client.DirectoryURL)
	}
	// The certificate is for the domain alone.
	if _, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"}); err == nil {
		t.Error("a certificate for another name")
	}
	// The challenges of plain HTTP reach the manager, not the redirect.
	h := m.HTTPHandler(toHTTPS("https://mila.example.com"))
	r := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/unknown", nil)
	r.Host = "mila.example.com"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == http.StatusPermanentRedirect || w.Code == http.StatusFound {
		t.Errorf("challenge redirected: %d", w.Code)
	}
	if c := tlsConfig(m); c.MinVersion != tls.VersionTLS12 || len(c.NextProtos) == 0 {
		t.Errorf("tls config: %+v", c)
	}
}

func TestCheckAddress(t *testing.T) {
	serve := func(h http.HandlerFunc) string {
		s := httptest.NewServer(h)
		t.Cleanup(s.Close)
		return s.URL
	}
	mila := func(id string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.Header().Set(httpapi.InstanceHeader, httpapi.InstanceFingerprint(id))
			}
			_, _ = w.Write([]byte("ok\n"))
		}
	}
	ctx := context.Background()
	if what, err := checkAddress(ctx, serve(mila("one")), "one"); err != nil || !strings.HasSuffix(what, "reaches this Mila") {
		t.Errorf("this Mila: %q %v", what, err)
	}
	if _, err := checkAddress(ctx, serve(mila("two")), "one"); err == nil || !strings.Contains(err.Error(), "not this one") {
		t.Errorf("another Mila: %v", err)
	}
	notMila := serve(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad gateway", http.StatusBadGateway) })
	if _, err := checkAddress(ctx, notMila, "one"); err == nil || !strings.Contains(err.Error(), "not as Mila") {
		t.Errorf("a proxy with nothing behind it: %v", err)
	}
	if _, err := checkAddress(ctx, "http://127.0.0.1:1", "one"); err == nil || !strings.Contains(err.Error(), "cannot be reached") {
		t.Errorf("nothing there: %v", err)
	}
}

// A stranger's failed handshake is noise at Debug; other server errors stay
// warnings.
func TestServerErrorsQuietHandshakes(t *testing.T) {
	var out strings.Builder
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelWarn}))
	l := serverErrors(log)
	l.Printf("http: TLS handshake error from 192.0.2.1:5555: missing server name")
	if out.Len() != 0 {
		t.Fatalf("a handshake error was logged at Warn: %s", out.String())
	}
	l.Printf("http: Accept error: too many open files")
	if !strings.Contains(out.String(), "level=WARN") || !strings.Contains(out.String(), "Accept error") {
		t.Fatalf("another server error was not a warning: %q", out.String())
	}
}
