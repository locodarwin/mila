package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestDesktopSignIn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		browser := s.browser(t)
		s.setup(t, browser)
		verifier := "a-secret-only-the-app-holds-0123456789abcdef"

		resp, made := browser.do("POST", "/auth/desktop/code", map[string]string{"challenge": challengeOf(verifier)})
		if resp.StatusCode != 201 {
			t.Fatalf("code = %d %v", resp.StatusCode, made)
		}
		code := made["code"].(string)

		// The app's window starts with nothing, and ends up signed in as the browser is.
		app := s.browser(t)
		if resp, _ := app.do("GET", "/me", nil); resp.StatusCode != 401 {
			t.Fatalf("before = %d", resp.StatusCode)
		}
		if resp, body := app.do("POST", "/auth/desktop/finish", map[string]string{"code": code, "verifier": verifier}); resp.StatusCode != 200 {
			t.Fatalf("finish = %d %v", resp.StatusCode, body)
		}
		resp, me := app.do("GET", "/me", nil)
		if resp.StatusCode != 200 || me["id"] != browser.userID() {
			t.Fatalf("after = %d %v", resp.StatusCode, me)
		}

		// A window signed in this way can't make codes of its own: a session taken while
		// recent would stay recent for good, each one making the next.
		if resp, _ := app.do("POST", "/auth/desktop/code", map[string]string{"challenge": challengeOf(verifier)}); resp.StatusCode != 403 {
			t.Fatalf("a code from a code = %d", resp.StatusCode)
		}

		// Each code works once.
		other := s.browser(t)
		if resp, _ := other.do("POST", "/auth/desktop/finish", map[string]string{"code": code, "verifier": verifier}); resp.StatusCode != 400 {
			t.Fatalf("again = %d", resp.StatusCode)
		}

		// Without the secret a code is worthless, and trying it spends it.
		_, made = browser.do("POST", "/auth/desktop/code", map[string]string{"challenge": challengeOf(verifier)})
		code = made["code"].(string)
		if resp, _ := other.do("POST", "/auth/desktop/finish", map[string]string{"code": code, "verifier": "a guess"}); resp.StatusCode != 400 {
			t.Fatalf("wrong secret = %d", resp.StatusCode)
		}
		if resp, _ := other.do("POST", "/auth/desktop/finish", map[string]string{"code": code, "verifier": verifier}); resp.StatusCode != 400 {
			t.Fatalf("after a wrong try = %d", resp.StatusCode)
		}
		if resp, _ := other.do("GET", "/me", nil); resp.StatusCode != 401 {
			t.Fatalf("signed in after all that = %d", resp.StatusCode)
		}

		// Someone signed out gets no code, and a challenge must look like one.
		if resp, _ := other.do("POST", "/auth/desktop/code", map[string]string{"challenge": challengeOf(verifier)}); resp.StatusCode != 401 {
			t.Fatalf("signed out = %d", resp.StatusCode)
		}
		if resp, _ := browser.do("POST", "/auth/desktop/code", map[string]string{"challenge": "short"}); resp.StatusCode != 400 {
			t.Fatalf("a bad challenge = %d", resp.StatusCode)
		}
	})
}

func TestATokenCannotMakeADesktopCode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, made := owner.do("POST", "/me/tokens", map[string]any{"name": "Script", "canWrite": true})
		if code, body := asToken(t, s, made["secret"].(string), "POST", "/auth/desktop/code", `{"challenge":"`+challengeOf("x")+`"}`); code != 403 {
			t.Fatalf("= %d %s", code, body)
		}
	})
}
