// Package mstest is a stand-in for Microsoft's identity platform, for tests
// of signing in to a Microsoft mailbox: the device code flow and refreshing,
// with a person who can approve, decline or have their sign-in revoked.
package mstest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Microsoft is the stand-in. Its URL is the authority to configure.
type Microsoft struct {
	URL     string
	Account string

	mu       sync.Mutex
	approved bool
	declined bool
	n        int
	refresh  map[string]bool
	access   map[string]bool
	asked    map[string]int
}

// Asked counts the requests of a kind: a grant type, or "devicecode".
func (m *Microsoft) Asked(kind string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.asked[kind]
}

// New starts the stand-in for the test's length.
func New(t *testing.T, account string) *Microsoft {
	m := &Microsoft{Account: account, refresh: map[string]bool{}, access: map[string]bool{}, asked: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(srv.Close)
	m.URL = srv.URL
	return m
}

// Approve is the person entering the code and saying yes.
func (m *Microsoft) Approve() { m.mu.Lock(); m.approved = true; m.mu.Unlock() }

// Decline is the person saying no.
func (m *Microsoft) Decline() { m.mu.Lock(); m.declined = true; m.mu.Unlock() }

// Revoke ends every sign-in, as a password change would.
func (m *Microsoft) Revoke() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refresh = map[string]bool{}
	m.access = map[string]bool{}
}

// ValidAccess says whether an access token is one handed out and not revoked,
// for an IMAP server in the test to check.
func (m *Microsoft) ValidAccess(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.access[token]
}

func (m *Microsoft) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.mu.Lock()
	defer m.mu.Unlock()
	reply := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(code, desc string) {
		reply(http.StatusBadRequest, map[string]string{"error": code, "error_description": "AADSTS0: " + desc + "\r\nTrace ID: x"})
	}
	if r.FormValue("client_id") == "" {
		fail("invalid_client", "no client_id")
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/devicecode"):
		m.asked["devicecode"]++
		if !strings.Contains(r.FormValue("scope"), "offline_access") {
			fail("invalid_scope", "no offline_access")
			return
		}
		reply(http.StatusOK, map[string]any{
			"device_code": "device-1", "user_code": "K7PQ2XMF", "verification_uri": "https://microsoft.com/devicelogin",
			"expires_in": 900, "interval": 1,
		})
	case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token"):
		grant := r.FormValue("grant_type")
		m.asked[grant]++
		switch grant {
		case "urn:ietf:params:oauth:grant-type:device_code":
			switch {
			case m.declined:
				fail("authorization_declined", "the user declined")
			case !m.approved:
				fail("authorization_pending", "waiting")
			default:
				m.approved = false
				reply(http.StatusOK, m.issue())
			}
		case "refresh_token":
			if !m.refresh[r.FormValue("refresh_token")] {
				fail("invalid_grant", "the refresh token was revoked")
				return
			}
			delete(m.refresh, r.FormValue("refresh_token"))
			reply(http.StatusOK, m.issue())
		default:
			fail("unsupported_grant_type", grant)
		}
	default:
		http.NotFound(w, r)
	}
}

// issue hands out a new access and refresh token; the caller holds mu.
func (m *Microsoft) issue() map[string]any {
	m.n++
	access, refresh := fmt.Sprintf("access-%d", m.n), fmt.Sprintf("refresh-%d", m.n)
	m.access[access] = true
	m.refresh[refresh] = true
	claims, _ := json.Marshal(map[string]string{"preferred_username": m.Account})
	id := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
	return map[string]any{
		"token_type": "Bearer", "access_token": access, "refresh_token": refresh, "id_token": id, "expires_in": 3600,
	}
}
