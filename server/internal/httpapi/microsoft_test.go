package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/inbound"
)

// fakeMicrosoft is a Microsoft mailbox's sign-in as the admin page sees it.
type fakeMicrosoft struct {
	state                   inbound.MicrosoftState
	stateErr                error
	connectedBy             string
	cancelled, disconnected bool
}

func (f *fakeMicrosoft) Mailbox() string { return "tasks@contoso.com" }
func (f *fakeMicrosoft) State(context.Context) (inbound.MicrosoftState, error) {
	return f.state, f.stateErr
}
func (f *fakeMicrosoft) Connect(_ context.Context, by string) (inbound.MicrosoftState, error) {
	at := time.Now().Add(15 * time.Minute)
	f.connectedBy = by
	f.state = inbound.MicrosoftState{State: "signing-in", UserCode: "K7PQ2XMF", VerificationURI: "https://microsoft.com/devicelogin", ExpiresAt: &at}
	return f.state, nil
}
func (f *fakeMicrosoft) Cancel()                          { f.cancelled = true }
func (f *fakeMicrosoft) Disconnect(context.Context) error { f.disconnected = true; return nil }

// The admin page shows how the Microsoft mailbox's sign-in stands, and an
// admin starts one, cancels it or forgets it.
func TestMicrosoftMailboxOverHTTP(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ms := &fakeMicrosoft{state: inbound.MicrosoftState{State: "not-connected"}}
		s := newServerWith(t, d, func(d *Deps) {
			d.EmailIn = func() (time.Time, string) { return time.Time{}, inbound.ErrNotConnected.Error() }
			d.Microsoft = ms
		})
		owner := s.browser(t)
		s.setup(t, owner)

		_, status := owner.do("GET", "/admin/status", nil)
		in := status["emailIn"].(map[string]any)
		if in["auth"] != "microsoft" || in["microsoft"].(map[string]any)["state"] != "not-connected" || in["microsoft"].(map[string]any)["mailbox"] != "tasks@contoso.com" {
			t.Fatalf("emailIn = %v", in)
		}
		resp, body := owner.do("POST", "/admin/mailbox/microsoft", nil)
		if resp.StatusCode != http.StatusOK || body["state"] != "signing-in" || body["userCode"] != "K7PQ2XMF" || body["expiresAt"] == nil {
			t.Fatalf("connect = %d %v", resp.StatusCode, body)
		}
		if ms.connectedBy != "Shawn" {
			t.Fatalf("connected by %q", ms.connectedBy)
		}
		if resp, _ := owner.do("POST", "/admin/mailbox/microsoft/cancel", nil); resp.StatusCode != http.StatusNoContent || !ms.cancelled {
			t.Fatalf("cancel = %d", resp.StatusCode)
		}
		if resp, _ := owner.do("DELETE", "/admin/mailbox/microsoft", nil); resp.StatusCode != http.StatusNoContent || !ms.disconnected {
			t.Fatalf("disconnect = %d", resp.StatusCode)
		}
	})
}

// A sign-in that can't be read is the mailbox's trouble, said there, not a
// 500 for the whole of Settings, Admin; what went wrong stays in the log.
func TestAdminStatusSurvivesTheSignIn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ms := &fakeMicrosoft{stateErr: errors.New("open /var/lib/mila/secret.key: permission denied")}
		s := newServerWith(t, d, func(d *Deps) {
			d.EmailIn = func() (time.Time, string) { return time.Time{}, "" }
			d.Microsoft = ms
		})
		owner := s.browser(t)
		s.setup(t, owner)
		resp, status := owner.do("GET", "/admin/status", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d %v", resp.StatusCode, status)
		}
		m := status["emailIn"].(map[string]any)["microsoft"].(map[string]any)
		if m["state"] != "lost" || m["problem"] == nil || strings.Contains(m["problem"].(string), "secret.key") {
			t.Fatalf("the mailbox = %v", m)
		}
	})
}

// A mailbox that takes a password has no Microsoft sign-in to start.
func TestMicrosoftMailboxNeedsTheSetting(t *testing.T) {
	s := newAuthServer(t)
	owner := s.browser(t)
	s.setup(t, owner)
	if resp, body := owner.do("POST", "/admin/mailbox/microsoft", nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("connect with no Microsoft mailbox = %d %v", resp.StatusCode, body)
	}
}
