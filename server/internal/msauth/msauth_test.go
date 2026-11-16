package msauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// answers serves one answer after another from the token endpoint.
func answers(t *testing.T, list ...map[string]any) Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(list) == 0 {
			t.Error("asked once more than expected")
			return
		}
		a := list[0]
		list = list[1:]
		w.Header().Set("Content-Type", "application/json")
		if a["error"] != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(a)
	}))
	t.Cleanup(srv.Close)
	return Config{ClientID: "c", Tenant: "common", Authority: srv.URL}
}

func TestWaitingForTheCode(t *testing.T) {
	c := answers(t,
		map[string]any{"error": "authorization_pending"},
		map[string]any{"error": "expired_token"},
	)
	_, err := c.Wait(context.Background(), Code{deviceCode: "d", interval: time.Millisecond})
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("an expired code: %v", err)
	}
	c = answers(t, map[string]any{"error": "authorization_declined"})
	if _, err := c.Wait(context.Background(), Code{deviceCode: "d", interval: time.Millisecond}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("a declined code: %v", err)
	}
	// Anything else says Microsoft's own first line, which names its code.
	c = answers(t, map[string]any{"error": "invalid_client", "error_description": "AADSTS7000218: The request body must contain client_assertion.\r\nTrace ID: 123"})
	_, err = c.Wait(context.Background(), Code{deviceCode: "d", interval: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "AADSTS7000218") || strings.Contains(err.Error(), "Trace ID") {
		t.Fatalf("another refusal: %v", err)
	}
}

func TestRefreshing(t *testing.T) {
	c := answers(t, map[string]any{"error": "invalid_grant", "error_description": "AADSTS700082: The refresh token has expired.\r\nTrace ID: 1"})
	if _, err := c.Refresh(context.Background(), "r", "a@example.com"); !errors.Is(err, ErrLost) || !strings.Contains(err.Error(), "AADSTS700082") {
		t.Fatalf("a refresh token Microsoft no longer takes: %v", err)
	}
	// With no new refresh token, the one given stays; the account stays as known.
	c = answers(t, map[string]any{"access_token": "a1", "expires_in": 3600})
	tok, err := c.Refresh(context.Background(), "r", "a@example.com")
	if err != nil || tok.Refresh != "r" || tok.Access != "a1" || tok.Account != "a@example.com" || time.Until(tok.ExpiresAt) < 59*time.Minute {
		t.Fatalf("a refresh: %+v %v", tok, err)
	}
}

func TestAccountOf(t *testing.T) {
	for token, want := range map[string]string{
		"e30.eyJwcmVmZXJyZWRfdXNlcm5hbWUiOiJzaGF3bkBjb250b3NvLmNvbSJ9.s": "shawn@contoso.com",
		"e30.eyJlbWFpbCI6ImVAeC5jb20ifQ.s":                               "e@x.com",
		"not a token":                                                    "",
		"a.!!!.c":                                                        "",
	} {
		if got := accountOf(token); got != want {
			t.Errorf("accountOf(%q) = %q, want %q", token, got, want)
		}
	}
}

// What the identity platform answers is held to sense: an overflowing pace
// or lifetime, an address that isn't HTTPS, and escapes for a terminal.
func TestAnswersAreHeldToSense(t *testing.T) {
	c := answers(t, map[string]any{
		"device_code": "d", "user_code": "AB\x1b]52;c;aGk=\x07CD", "verification_uri": "javascript:alert(1)",
		"interval": 9223372037, "expires_in": 9223372037,
	})
	code, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code.interval < time.Second || code.interval > maxInterval {
		t.Errorf("interval = %v", code.interval)
	}
	if life := time.Until(code.ExpiresAt); life <= 0 || life > maxCodeLife {
		t.Errorf("the code lives %v", life)
	}
	if code.VerificationURI != "https://microsoft.com/devicelogin" {
		t.Errorf("address = %q", code.VerificationURI)
	}
	if strings.ContainsAny(code.UserCode, "\x1b\x07") {
		t.Errorf("user code = %q", code.UserCode)
	}
	c = answers(t, map[string]any{"error": "invalid_client", "error_description": "AADSTS1: \x1b[2Kbad\nmore"})
	_, err = c.Wait(context.Background(), Code{deviceCode: "d", interval: time.Millisecond})
	if err == nil || strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "more") {
		t.Errorf("a refusal with escapes: %q", err)
	}
	// A code past its time ends the wait, whatever Microsoft keeps saying.
	c = answers(t, map[string]any{"error": "authorization_pending"})
	if _, err := c.Wait(context.Background(), Code{deviceCode: "d", interval: time.Millisecond, ExpiresAt: time.Now().Add(-time.Second)}); !errors.Is(err, ErrExpired) {
		t.Errorf("a code past its time: %v", err)
	}
}

// A redirect is not followed: Go would post the refresh token again to
// wherever it points.
func TestRedirectsAreNotFollowed(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the refresh token was posted to the redirect's target")
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/token", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := Config{ClientID: "c", Tenant: "common", Authority: srv.URL}
	if _, err := c.Refresh(context.Background(), "secret-refresh", "a@example.com"); err == nil {
		t.Fatal("a redirect was taken for an answer")
	}
}
