package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// What would let a stolen session outlast its theft needs a recent sign-in;
// signing in again on the same browser ends the session it replaces.
func TestRecentSignIn(t *testing.T) {
	d := dbtest.Open(t)
	s := newServerOn(t, d)
	owner := s.browser(t)
	s.setup(t, owner)

	q := "UPDATE sessions SET created_at = ?"
	if d.Dialect == db.Postgres {
		q = "UPDATE sessions SET created_at = $1"
	}
	if _, err := d.ExecContext(context.Background(), q, time.Now().Add(-11*time.Minute).UTC()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		body any
	}{
		{"/me/passkeys/begin", nil},
		{"/me/email", map[string]string{"email": "new@example.com"}},
		{"/me/delete", map[string]string{"email": "shawn@example.com"}},
	} {
		resp, body := owner.do("POST", c.path, c.body)
		if resp.StatusCode != 403 || body["error"] != "sign_in_again" {
			t.Errorf("%s with an old sign-in = %d %v", c.path, resp.StatusCode, body)
		}
	}
	if _, list := owner.doList("GET", "/me/sessions"); len(list) != 1 {
		t.Fatalf("sessions before signing in again = %v", list)
	}

	if resp, _ := owner.do("POST", "/auth/magic-link", map[string]string{"email": "shawn@example.com"}); resp.StatusCode != 204 {
		t.Fatalf("magic link = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/auth/magic-link/verify", map[string]string{"token": s.mail.token(t)}); resp.StatusCode != 200 {
		t.Fatalf("verify = %d", resp.StatusCode)
	}
	if _, list := owner.doList("GET", "/me/sessions"); len(list) != 1 {
		t.Errorf("sessions after signing in again = %v, want the new one only", list)
	}
	if resp, body := owner.do("POST", "/me/email", map[string]string{"email": "new@example.com"}); resp.StatusCode != 204 {
		t.Errorf("email change after signing in again = %d %v", resp.StatusCode, body)
	}

	// With a passkey on the account, an email link is not enough.
	exec := func(sqlite, postgres string, args ...any) {
		t.Helper()
		q := sqlite
		if d.Dialect == db.Postgres {
			q = postgres
		}
		if _, err := d.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatal(err)
		}
	}
	_, me := owner.do("GET", "/me", nil)
	exec("INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at) VALUES ('pk', ?, ?, '{}', 'Phone', ?)",
		"INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at) VALUES ('pk', $1, $2, '{}', 'Phone', $3)",
		me["id"], []byte{1, 2, 3}, time.Now().UTC())
	if resp, body := owner.do("POST", "/me/email", map[string]string{"email": "new@example.com"}); resp.StatusCode != 403 || body["error"] != "sign_in_again" {
		t.Errorf("email change by email link with a passkey on the account = %d %v", resp.StatusCode, body)
	}
	exec("UPDATE sessions SET method = 'passkey'", "UPDATE sessions SET method = 'passkey'")
	if resp, body := owner.do("POST", "/me/email", map[string]string{"email": "new@example.com"}); resp.StatusCode != 204 {
		t.Errorf("email change after a passkey sign-in = %d %v", resp.StatusCode, body)
	}

	// However often it is used, a session ends 90 days after signing in.
	exec("UPDATE sessions SET created_at = ?", "UPDATE sessions SET created_at = $1", time.Now().Add(-91*24*time.Hour).UTC())
	if resp, _ := owner.do("GET", "/me", nil); resp.StatusCode != 401 {
		t.Errorf("a session from 91 days ago = %d", resp.StatusCode)
	}
}

// Removing a passkey needs a recent sign-in, with a passkey while the
// account has one: otherwise an email link could remove every passkey and
// then do all a passkey sign-in guards, and any stolen session could
// remove them. The person is told by email.
func TestRemovingAPasskeyNeedsARecentSignIn(t *testing.T) {
	d := dbtest.Open(t)
	s := newServerOn(t, d)
	owner := s.browser(t)
	s.setup(t, owner)
	exec := func(sqlite, postgres string, args ...any) {
		t.Helper()
		q := sqlite
		if d.Dialect == db.Postgres {
			q = postgres
		}
		if _, err := d.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatal(err)
		}
	}
	_, me := owner.do("GET", "/me", nil)
	exec("INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at) VALUES ('pk', ?, ?, '{}', 'Phone', ?)",
		"INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at) VALUES ('pk', $1, $2, '{}', 'Phone', $3)",
		me["id"], []byte{1, 2, 3}, time.Now().UTC())

	// A sign-in by email link a moment ago.
	exec("UPDATE sessions SET method = 'magic_link'", "UPDATE sessions SET method = 'magic_link'")
	if resp, body := owner.do("DELETE", "/me/passkeys/pk", nil); resp.StatusCode != 403 || body["error"] != "sign_in_again" {
		t.Errorf("removing a passkey after an email link = %d %v", resp.StatusCode, body)
	}
	if resp, body := owner.do("POST", "/me/email", map[string]string{"email": "new@example.com"}); resp.StatusCode != 403 {
		t.Errorf("an email change once the passkey should still be there = %d %v", resp.StatusCode, body)
	}

	// A passkey sign-in from 11 minutes ago.
	exec("UPDATE sessions SET method = 'passkey', created_at = ?", "UPDATE sessions SET method = 'passkey', created_at = $1", time.Now().Add(-11*time.Minute).UTC())
	if resp, body := owner.do("DELETE", "/me/passkeys/pk", nil); resp.StatusCode != 403 || body["error"] != "sign_in_again" {
		t.Errorf("removing a passkey after an old sign-in = %d %v", resp.StatusCode, body)
	}

	// A passkey sign-in a moment ago.
	exec("UPDATE sessions SET created_at = ?", "UPDATE sessions SET created_at = $1", time.Now().UTC())
	if resp, body := owner.do("DELETE", "/me/passkeys/pk", nil); resp.StatusCode != 204 {
		t.Fatalf("removing a passkey after a passkey sign-in = %d %v", resp.StatusCode, body)
	}
	if text := s.mail.lastText(); !strings.Contains(text, `"Phone"`) || !strings.Contains(text, "/settings/security") {
		t.Errorf("the email about the removed passkey reads %q", text)
	}
}
