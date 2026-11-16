package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

func withMailbox(d *Deps) { d.InboundAddress = "mila+{token}@example.com" }

func TestEmailAddresses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerWith(t, d, withMailbox)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		base := "/workspaces/" + wsID + "/email-to-task"

		resp, state := owner.do("GET", base, nil)
		if resp.StatusCode != 200 || state["enabled"] != true || !strings.Contains(state["signInAddress"].(string), "@") {
			t.Fatalf("state = %d %v", resp.StatusCode, state)
		}

		resp, made := owner.do("POST", base+"/addresses", map[string]any{"assignToMe": true})
		if resp.StatusCode != 201 || made["assignToMe"] != true || made["projectId"] != nil {
			t.Fatalf("make = %d %v", resp.StatusCode, made)
		}
		address := made["address"].(string)
		token := strings.TrimSuffix(strings.TrimPrefix(address, "mila+"), "@example.com")
		if len(token) != 26 || strings.ToLower(token) != token {
			t.Fatalf("address = %q", address)
		}
		id := made["id"].(string)

		// It shows again, for copying, as long as it lasts.
		_, state = owner.do("GET", base, nil)
		if list := state["addresses"].([]any); len(list) != 1 || list[0].(map[string]any)["address"] != address {
			t.Fatalf("list = %v", list)
		}

		// Senders are tidied: bare, lowercase, once, and never the person's own.
		own := state["signInAddress"].(string)
		resp, updated := owner.do("PATCH", base+"/addresses/"+id, map[string]any{
			"assignToMe": false, "senders": []string{"Dana <DANA@example.org>", "dana@example.org", own},
		})
		if resp.StatusCode != 200 || updated["assignToMe"] != false {
			t.Fatalf("update = %d %v", resp.StatusCode, updated)
		}
		if got := updated["senders"].([]any); len(got) != 1 || got[0] != "dana@example.org" {
			t.Fatalf("senders = %v", got)
		}
		if resp, _ := owner.do("PATCH", base+"/addresses/"+id, map[string]any{"assignToMe": true, "senders": []string{"not an address"}}); resp.StatusCode != 400 {
			t.Fatalf("a bad sender = %d", resp.StatusCode)
		}

		// A project must be here, and in this workspace.
		_, project := owner.do("POST", "/workspaces/"+wsID+"/projects", map[string]string{"name": "Wholesale"})
		if resp, body := owner.do("PATCH", base+"/addresses/"+id, map[string]any{"assignToMe": true, "senders": []string{}, "projectId": project["id"]}); resp.StatusCode != 200 || body["projectId"] != project["id"] {
			t.Fatalf("a project = %d %v", resp.StatusCode, body)
		}
		_, other := owner.do("POST", "/workspaces", map[string]string{"name": "Other"})
		_, theirs := owner.do("POST", "/workspaces/"+other["id"].(string)+"/projects", map[string]string{"name": "Elsewhere"})
		if resp, _ := owner.do("PATCH", base+"/addresses/"+id, map[string]any{"assignToMe": true, "senders": []string{}, "projectId": theirs["id"]}); resp.StatusCode != 400 {
			t.Fatalf("another workspace's project = %d", resp.StatusCode)
		}

		// Replacing ends the old address and keeps the rest.
		resp, replaced := owner.do("POST", base+"/addresses/"+id+"/replace", nil)
		if resp.StatusCode != 200 || replaced["address"] == address || replaced["id"] != id || replaced["projectId"] != project["id"] {
			t.Fatalf("replace = %d %v", resp.StatusCode, replaced)
		}

		// A limit of 10.
		for range 9 {
			if resp, _ := owner.do("POST", base+"/addresses", map[string]any{"assignToMe": true}); resp.StatusCode != 201 {
				t.Fatalf("make = %d", resp.StatusCode)
			}
		}
		if resp, _ := owner.do("POST", base+"/addresses", map[string]any{"assignToMe": true}); resp.StatusCode != 400 {
			t.Fatalf("an eleventh = %d", resp.StatusCode)
		}

		if resp, _ := owner.do("DELETE", base+"/addresses/"+id, nil); resp.StatusCode != 204 {
			t.Fatalf("delete = %d", resp.StatusCode)
		}
		if resp, _ := owner.do("DELETE", base+"/addresses/"+id, nil); resp.StatusCode != 404 {
			t.Fatalf("delete again = %d", resp.StatusCode)
		}

		// It went in the security log, and the person was told.
		_, log := owner.do("GET", "/me/security-events", nil)
		kinds := ""
		for _, e := range log["events"].([]any) {
			kinds += " " + e.(map[string]any)["kind"].(string)
		}
		if !strings.Contains(kinds, "email_address_made") || !strings.Contains(kinds, "email_address_removed") {
			t.Fatalf("log = %s", kinds)
		}
		if !strings.Contains(s.mail.lastText(), "turns email into tasks") {
			t.Fatalf("not told: %s", s.mail.lastText())
		}
	})
}

func TestWhoHasEmailAddresses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerWith(t, d, withMailbox)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		wsID := ws["id"].(string)
		base := "/workspaces/" + wsID + "/email-to-task"
		member := s.join(t, owner, wsID, "dana@example.com", "Dana", "member")
		guest := s.join(t, owner, wsID, "lee@example.com", "Lee", "guest")

		// Guests have none, and see nothing of it.
		if resp, _ := guest.do("GET", base, nil); resp.StatusCode != 403 {
			t.Fatalf("a guest's list = %d", resp.StatusCode)
		}
		if resp, _ := guest.do("POST", base+"/addresses", map[string]any{"assignToMe": true}); resp.StatusCode != 403 {
			t.Fatalf("a guest making one = %d", resp.StatusCode)
		}

		_, mine := owner.do("POST", base+"/addresses", map[string]any{"assignToMe": true})
		_, hers := member.do("POST", base+"/addresses", map[string]any{"assignToMe": true})
		if hers["address"] == nil {
			t.Fatalf("a member's = %v", hers)
		}

		// Each person sees and changes only their own, even an owner.
		_, state := member.do("GET", base, nil)
		if list := state["addresses"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != hers["id"] {
			t.Fatalf("list = %v", list)
		}
		for _, method := range []string{"DELETE", "PATCH"} {
			if resp, _ := member.do(method, base+"/addresses/"+mine["id"].(string), map[string]any{"assignToMe": true, "senders": []string{}}); resp.StatusCode != 404 {
				t.Fatalf("%s someone else's = %d", method, resp.StatusCode)
			}
			if resp, _ := owner.do(method, base+"/addresses/"+hers["id"].(string), map[string]any{"assignToMe": true, "senders": []string{}}); resp.StatusCode != 404 {
				t.Fatalf("%s a member's, as the owner = %d", method, resp.StatusCode)
			}
		}

		// Becoming a guest, or leaving, ends them.
		memberID := member.userID()
		if resp, _ := owner.do("PATCH", "/workspaces/"+wsID+"/members/"+memberID, map[string]any{"role": "guest"}); resp.StatusCode != 204 {
			t.Fatalf("to a guest = %d", resp.StatusCode)
		}
		if resp, _ := owner.do("PATCH", "/workspaces/"+wsID+"/members/"+memberID, map[string]any{"role": "member"}); resp.StatusCode != 204 {
			t.Fatalf("back = %d", resp.StatusCode)
		}
		_, state = member.do("GET", base, nil)
		if list := state["addresses"].([]any); len(list) != 0 {
			t.Fatalf("after being a guest = %v", list)
		}
		_, again := member.do("POST", base+"/addresses", map[string]any{"assignToMe": true})
		if again["id"] == nil {
			t.Fatalf("again = %v", again)
		}
		if resp, _ := owner.do("DELETE", "/workspaces/"+wsID+"/members/"+memberID, nil); resp.StatusCode != 204 {
			t.Fatalf("remove = %d", resp.StatusCode)
		}
		left, err := store.New(d).CountInboundAddresses(t.Context(), sqlitedb.CountInboundAddressesParams{UserID: memberID, WorkspaceID: wsID})
		if err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Fatalf("%d addresses after leaving", left)
		}
	})
}

func TestNoMailboxNoAddresses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		base := "/workspaces/" + ws["id"].(string) + "/email-to-task"
		resp, state := owner.do("GET", base, nil)
		if resp.StatusCode != 200 || state["enabled"] != false {
			t.Fatalf("state = %d %v", resp.StatusCode, state)
		}
		if resp, _ := owner.do("POST", base+"/addresses", map[string]any{"assignToMe": true}); resp.StatusCode != 400 {
			t.Fatalf("make = %d", resp.StatusCode)
		}
	})
}

func TestATokenCannotReachEmailAddresses(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerWith(t, d, withMailbox)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		base := "/workspaces/" + ws["id"].(string) + "/email-to-task"
		_, made := owner.do("POST", "/me/tokens", map[string]any{"name": "Script", "canWrite": true})
		secret := made["secret"].(string)
		for _, call := range [][3]string{
			{"GET", base, ""},
			{"POST", base + "/addresses", `{"assignToMe":true}`},
			{"DELETE", base + "/addresses/x", ""},
			{"POST", base + "/addresses/x/replace", ""},
		} {
			if code, body := asToken(t, s, secret, call[0], call[1], call[2]); code != 403 {
				t.Fatalf("%s %s = %d %s", call[0], call[1], code, body)
			}
		}
	})
}

// An old session can change where an address goes, but not let someone new in.
func TestAddingASenderNeedsARecentSignIn(t *testing.T) {
	d := dbtest.Open(t)
	s := newServerWith(t, d, withMailbox)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	base := "/workspaces/" + ws["id"].(string) + "/email-to-task/addresses"
	_, made := owner.do("POST", base, map[string]any{"assignToMe": true})
	id := made["id"].(string)
	if resp, _ := owner.do("PATCH", base+"/"+id, map[string]any{"assignToMe": true, "senders": []string{"kept@example.org"}}); resp.StatusCode != 200 {
		t.Fatalf("with a recent sign-in = %d", resp.StatusCode)
	}

	q := "UPDATE sessions SET created_at = ?"
	if d.Dialect == db.Postgres {
		q = "UPDATE sessions SET created_at = $1"
	}
	if _, err := d.ExecContext(context.Background(), q, time.Now().Add(-11*time.Minute).UTC()); err != nil {
		t.Fatal(err)
	}
	resp, body := owner.do("PATCH", base+"/"+id, map[string]any{"assignToMe": true, "senders": []string{"kept@example.org", "new@example.org"}})
	if resp.StatusCode != 403 || body["error"] != "sign_in_again" {
		t.Fatalf("adding one = %d %v", resp.StatusCode, body)
	}
	if resp, _ := owner.do("PATCH", base+"/"+id, map[string]any{"assignToMe": false, "senders": []string{"kept@example.org"}}); resp.StatusCode != 200 {
		t.Fatalf("changing nothing about who = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("PATCH", base+"/"+id, map[string]any{"assignToMe": false, "senders": []string{}}); resp.StatusCode != 200 {
		t.Fatalf("removing one = %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", base+"/"+id+"/replace", nil); resp.StatusCode != 403 {
		t.Fatalf("replacing = %d", resp.StatusCode)
	}

	// It went in the security log, as one sender added.
	_, log := owner.do("GET", "/me/security-events", nil)
	added := 0
	for _, e := range log["events"].([]any) {
		if e.(map[string]any)["kind"] == "email_sender_added" {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("%d in the log", added)
	}
}
