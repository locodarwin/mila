package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

// oauthWorld is a server that lets apps connect, with a workspace.
func oauthWorld(t *testing.T, d *db.DB) actionsWorld {
	t.Helper()
	s := newServerWith(t, d, func(d *Deps) { d.OAuth = true })
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	return actionsWorld{s: s, owner: owner, ws: ws["id"].(string)}
}

// browserSaysYes is the person on Mila's page: it reads what the app asked
// for, as the page does, and answers.
func browserSaysYes(t *testing.T, w actionsWorld, authorize string, answer map[string]any) (*url.URL, map[string]any) {
	t.Helper()
	u, err := url.Parse(authorize)
	if err != nil || u.Path != authorizePath {
		t.Fatalf("authorize address %q", authorize)
	}
	q := u.Query()
	resp, asked := w.owner.do("GET", "/oauth/consent?"+url.Values{"client_id": {q.Get("client_id")}, "redirect_uri": {q.Get("redirect_uri")}, "resource": {q.Get("resource")}}.Encode(), nil)
	if resp.StatusCode != 200 {
		return nil, asked
	}
	body := map[string]any{"clientId": q.Get("client_id"), "redirectUri": q.Get("redirect_uri"), "state": q.Get("state"),
		"codeChallenge": q.Get("code_challenge"), "codeChallengeMethod": q.Get("code_challenge_method"), "resource": q.Get("resource"),
		"allow": true, "workspaceId": w.ws, "canWrite": true, "expiresInDays": 30}
	for k, v := range answer {
		body[k] = v
	}
	resp, out := w.owner.do("POST", "/oauth/consent", body)
	if resp.StatusCode != 200 {
		return nil, out
	}
	back, err := url.Parse(out["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return back, asked
}

func TestAnAppConnectsByOAuth(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		var asked map[string]any
		h, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
			DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName: "Test assistant", RedirectURIs: []string{"http://localhost:7777/callback"}}},
			AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
				var back *url.URL
				back, asked = browserSaysYes(t, w, args.URL, nil)
				if back == nil {
					t.Fatalf("consent = %v", asked)
				}
				q := back.Query()
				return &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		ct := &mcp.StreamableClientTransport{Endpoint: w.s.URL + mcpPath + "?workspace=Team", OAuthHandler: h, DisableStandaloneSSE: true, MaxRetries: -1}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cs.Close() }()
		if asked["app"] != "Test assistant" || asked["returnsTo"] != "an app on this computer" || asked["workspace"] != "Team" {
			t.Errorf("the page was asked %v", asked)
		}
		out, failed := call(t, cs, "add_task", map[string]any{"title": "From the app"})
		if failed {
			t.Fatalf("add_task = %v", out)
		}

		// The app's token is a token of the person's, marked as the app's,
		// in history under the app's name.
		_, tokens := w.owner.doList("GET", "/me/tokens")
		if len(tokens) != 1 || tokens[0]["name"] != "Test assistant" || tokens[0]["app"] != true || tokens[0]["workspaceId"] != w.ws {
			t.Fatalf("tokens = %v", tokens)
		}
		id := named(out["task"], "id")
		if _, events := w.owner.doList("GET", "/workspaces/"+w.ws+"/tasks/"+id+"/activity"); len(events) == 0 || events[0]["via"] != "Test assistant" {
			t.Errorf("activity = %v", events)
		}
		_, log := w.owner.do("GET", "/me/security-events", nil)
		if events, _ := log["events"].([]any); len(events) == 0 || named(events[0], "kind") != "app_connected" || named(events[0], "detail") != "Test assistant" {
			t.Errorf("security log = %v", log)
		}
		if !strings.Contains(w.s.mail.last, "Test assistant") {
			t.Errorf("mail = %q", w.s.mail.last)
		}
	})
}

// pkce is a verifier and its S256 challenge.
func pkce(seed string) (verifier, challenge string) {
	verifier = strings.Repeat(seed, 43/len(seed)+1)[:43]
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func postJSON(t *testing.T, s *testServer, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(s.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func postForm(t *testing.T, s *testServer, path string, form url.Values) (int, map[string]any) {
	t.Helper()
	resp, err := http.PostForm(s.URL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getPublic(t *testing.T, s *testServer, path string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(s.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestOAuthIsOffUnlessSwitchedOn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		for _, path := range []string{prmPath + mcpPath, asmPath} {
			if code, _ := getPublic(t, w.s, path); code != 404 {
				t.Errorf("%s = %d", path, code)
			}
		}
		if code, _ := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{"https://app.example/cb"}}); code != 404 {
			t.Errorf("register = %d", code)
		}
		if resp, _ := w.owner.do("GET", "/oauth/consent?client_id=x&redirect_uri=y", nil); resp.StatusCode != 404 {
			t.Errorf("consent = %d", resp.StatusCode)
		}
		req, _ := http.NewRequest("POST", w.s.URL+mcpPath, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if got := resp.Header.Get("WWW-Authenticate"); strings.Contains(got, "resource_metadata") {
			t.Errorf("challenge with OAuth off = %q", got)
		}
	})
}

func TestOAuthRefusesWhatItShould(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		origin := w.s.URL

		// What apps read first.
		code, prm := getPublic(t, w.s, prmPath+mcpPath+"?workspace=Team")
		if code != 200 || prm["resource"] != origin+mcpPath+"?workspace=Team" {
			t.Fatalf("resource metadata = %d %v", code, prm)
		}
		code, asm := getPublic(t, w.s, asmPath)
		if code != 200 || asm["issuer"] != origin || asm["code_challenge_methods_supported"].([]any)[0] != "S256" {
			t.Fatalf("server metadata = %d %v", code, asm)
		}

		// Only addresses an app can safely be sent back to.
		// A scheme of the app's own is a reversed domain (RFC 8252 section
		// 7.1): a bare name could be any handler the computer has. The
		// shapes in full are TestRedirectShapes, in accounts.
		for _, bad := range []string{"http://evil.example/cb", "javascript:alert(1)", "https://app.example/cb#x", "/cb", "https://user:pw@app.example/cb", "data:text/html,x", "ms-msdt:/id PCWDiagnostic", "search-ms:query=x", "myapp:/cb"} {
			if code, out := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{bad}}); code != 400 || out["error"] != "invalid_redirect_uri" {
				t.Errorf("register %q = %d %v", bad, code, out)
			}
		}
		if code, out := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{"https://app.example/cb"}, "grant_types": []string{"client_credentials"}}); code != 400 {
			t.Errorf("register for another grant = %d %v", code, out)
		}
		code, app := postJSON(t, w.s, registerPath, map[string]any{"client_name": "  Helper  ", "redirect_uris": []string{"https://app.example/cb", "http://127.0.0.1/cb", "com.example.myapp:/cb"},
			"token_endpoint_auth_method": "client_secret_basic"})
		if code != 201 || app["client_name"] != "Helper" || app["token_endpoint_auth_method"] != "none" {
			t.Fatalf("register = %d %v", code, app)
		}
		client := app["client_id"].(string)
		// A name that would read otherwise on the page, and a host that would pass for another.
		code, spoof := postJSON(t, w.s, registerPath, map[string]any{"client_name": "Help\u202eer\u200b", "redirect_uris": []string{"https://\u0430pp.example/cb"}})
		if code != 201 || spoof["client_name"] != "Help er" {
			t.Fatalf("a name with direction marks = %d %v", code, spoof)
		}
		if resp, asked := w.owner.do("GET", "/oauth/consent?"+url.Values{"client_id": {spoof["client_id"].(string)}, "redirect_uri": {"https://\u0430pp.example/cb"}}.Encode(), nil); resp.StatusCode != 200 || asked["returnsTo"] != "xn--pp-6kc.example" {
			t.Fatalf("a look-alike host = %d %v", resp.StatusCode, asked)
		}
		verifier, challenge := pkce("abc")
		authorize := func(redirect string) string {
			return origin + authorizePath + "?" + url.Values{"client_id": {client}, "redirect_uri": {redirect}, "state": {"s1"},
				"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {origin + mcpPath}}.Encode()
		}
		exchange := func(code, redirect, verifier string) (int, map[string]any) {
			return postForm(t, w.s, tokenPath, url.Values{"grant_type": {"authorization_code"}, "client_id": {client}, "code": {code},
				"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {origin + mcpPath}})
		}

		// An address the app didn't register is refused on the page, never redirected to.
		if back, out := browserSaysYes(t, w, authorize("https://evil.example/cb"), nil); back != nil || !strings.Contains(out["message"].(string), "won't send you there") {
			t.Fatalf("an unregistered address = %v %v", back, out)
		}
		if back, out := browserSaysYes(t, w, strings.Replace(authorize("https://app.example/cb"), "resource=", "resource=https%3A%2F%2Felsewhere.example%2Fmcp&x=", 1), nil); back != nil || out["error"] != "invalid" {
			t.Fatalf("another server's resource = %v %v", back, out)
		}
		// No: the app is told so.
		back, _ := browserSaysYes(t, w, authorize("https://app.example/cb"), map[string]any{"allow": false})
		if back == nil || back.Host != "app.example" || back.Query().Get("error") != "access_denied" || back.Query().Get("state") != "s1" || back.Query().Get("code") != "" {
			t.Fatalf("no = %v", back)
		}
		if back, out := browserSaysYes(t, w, authorize("https://app.example/cb"), map[string]any{"codeChallengeMethod": "plain"}); back != nil || !strings.Contains(out["message"].(string), "S256") {
			t.Fatalf("plain PKCE = %v %v", back, out)
		}
		if back, out := browserSaysYes(t, w, authorize("https://app.example/cb"), map[string]any{"expiresInDays": 3650}); back != nil {
			t.Fatalf("ten years = %v %v", back, out)
		}

		// Yes, on this computer at a port it didn't register: a code, once.
		redirect := "http://127.0.0.1:51234/cb"
		back, _ = browserSaysYes(t, w, authorize(redirect), nil)
		if back == nil || back.Query().Get("iss") != origin || back.Query().Get("code") == "" || back.Port() != "51234" {
			t.Fatalf("yes = %v", back)
		}
		code1 := back.Query().Get("code")
		if code, out := exchange(code1, "http://127.0.0.1:9/cb", verifier); code != 400 || out["error"] != "invalid_grant" {
			t.Fatalf("another redirect = %d %v", code, out)
		}
		// That try used the code up.
		if code, out := exchange(code1, redirect, verifier); code != 400 || out["error"] != "invalid_grant" {
			t.Fatalf("a code tried before = %d %v", code, out)
		}
		back, _ = browserSaysYes(t, w, authorize(redirect), nil)
		code2 := back.Query().Get("code")
		if code, out := exchange(code2, redirect, "wrong-verifier-wrong-verifier-wrong-verifier"); code != 400 || out["error"] != "invalid_grant" {
			t.Fatalf("a wrong verifier = %d %v", code, out)
		}
		back, _ = browserSaysYes(t, w, authorize(redirect), nil)
		code3 := back.Query().Get("code")
		code, tok := exchange(code3, redirect, verifier)
		if code != 200 || tok["token_type"] != "Bearer" || tok["scope"] != "read write" || tok["expires_in"].(float64) < 29*24*3600 {
			t.Fatalf("exchange = %d %v", code, tok)
		}
		secret := tok["access_token"].(string)
		if _, err := mcpOver(t, w.s, secret, ""); err != nil {
			t.Fatalf("the app's token at the MCP endpoint: %v", err)
		}
		// It reaches nothing else.
		if code, out := tokenCall(t, w.s, secret, "GET", "/workspaces/"+w.ws+"/tasks", "", nil); code != 403 || out["error"] != "token_not_allowed" {
			t.Fatalf("the app's token on REST = %d %v", code, out)
		}
		// A code that comes back revokes what it was traded for.
		if code, out := exchange(code3, redirect, verifier); code != 400 || out["error"] != "invalid_grant" {
			t.Fatalf("a code used twice = %d %v", code, out)
		}
		if _, err := mcpOver(t, w.s, secret, ""); err == nil {
			t.Fatal("the token of a code used twice still works")
		}

		// Connecting again replaces the app's token.
		back, _ = browserSaysYes(t, w, authorize(redirect), nil)
		_, first := exchange(back.Query().Get("code"), redirect, verifier)
		back, _ = browserSaysYes(t, w, authorize(redirect), map[string]any{"canWrite": false})
		_, second := exchange(back.Query().Get("code"), redirect, verifier)
		if second["scope"] != "read" {
			t.Fatalf("read only = %v", second)
		}
		if _, err := mcpOver(t, w.s, first["access_token"].(string), ""); err == nil {
			t.Error("the app's earlier token still works")
		}
		if _, tokens := w.owner.doList("GET", "/me/tokens"); len(tokens) != 1 {
			t.Errorf("tokens = %v", tokens)
		}
		if code, out := postForm(t, w.s, tokenPath, url.Values{"grant_type": {"refresh_token"}, "client_id": {client}}); code != 400 || out["error"] != "unsupported_grant_type" {
			t.Errorf("refresh = %d %v", code, out)
		}
		if code, out := postForm(t, w.s, tokenPath, url.Values{"grant_type": {"authorization_code"}, "client_id": {"app_nobody"}}); code != 401 || out["error"] != "invalid_client" {
			t.Errorf("an unknown app = %d %v", code, out)
		}

		// Switched off, the apps' tokens stop working.
		off := newServerWith(t, d, nil)
		if code, out := tokenCall(t, off, second["access_token"].(string), "GET", "/me", "", nil); code != 401 {
			t.Errorf("an app's token with OAuth off = %d %v", code, out)
		}
	})
}

func TestTheServerDeletesAnApp(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		ctx := context.Background()
		origin := w.s.URL
		redirect := "https://app.example/cb"
		_, app := postJSON(t, w.s, registerPath, map[string]any{"client_name": "Helper", "redirect_uris": []string{redirect}})
		client := app["client_id"].(string)
		if code, _ := postJSON(t, w.s, registerPath, map[string]any{"client_name": "Idle", "redirect_uris": []string{redirect}}); code != 201 {
			t.Fatalf("register = %d", code)
		}
		verifier, challenge := pkce("xyz")
		back, out := browserSaysYes(t, w, origin+authorizePath+"?"+url.Values{"client_id": {client}, "redirect_uri": {redirect}, "state": {"s"},
			"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {origin + mcpPath}}.Encode(), nil)
		if back == nil {
			t.Fatalf("yes = %v", out)
		}
		_, tok := postForm(t, w.s, tokenPath, url.Values{"grant_type": {"authorization_code"}, "client_id": {client}, "code": {back.Query().Get("code")},
			"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {origin + mcpPath}})
		secret, _ := tok["access_token"].(string)
		if _, err := mcpOver(t, w.s, secret, ""); err != nil {
			t.Fatalf("the app's token: %v", err)
		}

		apps, err := w.s.acc.OAuthApps(ctx)
		if err != nil || len(apps) != 2 || apps[0].Name != "Helper" || apps[0].Tokens != 1 || apps[0].LastYes.IsZero() ||
			apps[1].Name != "Idle" || apps[1].Tokens != 0 || !apps[1].LastYes.IsZero() {
			t.Fatalf("apps = %+v, %v", apps, err)
		}
		gone, people, err := w.s.acc.DeleteOAuthClient(ctx, client)
		if err != nil || gone.Name != "Helper" || gone.Tokens != 1 || len(people) != 1 || people[0].Email != "shawn@example.com" {
			t.Fatalf("delete = %+v %v %v", gone, people, err)
		}
		if _, err := mcpOver(t, w.s, secret, ""); err == nil {
			t.Error("a deleted app's token still works")
		}
		if _, tokens := w.owner.doList("GET", "/me/tokens"); len(tokens) != 0 {
			t.Errorf("tokens = %v", tokens)
		}
		_, log := w.owner.do("GET", "/me/security-events", nil)
		if events, _ := log["events"].([]any); len(events) == 0 || named(events[0], "kind") != "token_revoked" ||
			named(events[0], "method") != "command" || named(events[0], "detail") != "Helper" {
			t.Errorf("security log = %v", log)
		}
		if code, out := postForm(t, w.s, tokenPath, url.Values{"grant_type": {"authorization_code"}, "client_id": {client}}); code != 401 || out["error"] != "invalid_client" {
			t.Errorf("the deleted app = %d %v", code, out)
		}
		if apps, _ := w.s.acc.OAuthApps(ctx); len(apps) != 1 || apps[0].Name != "Idle" {
			t.Errorf("apps left = %+v", apps)
		}
		if _, _, err := w.s.acc.DeleteOAuthClient(ctx, client); accounts.KindOf(err) != accounts.NotFound {
			t.Errorf("deleting it again = %v", err)
		}
	})
}

// A quote in the address asked for can't end the challenge's quoted string
// and add a second resource_metadata pointing elsewhere.
func TestTheChallengeQuotesTheQuery(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		req, err := http.NewRequest("POST", w.s.URL+`/api/v1/mcp?workspace=a",resource_metadata="https://evil.example/m`, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		got := resp.Header.Get("WWW-Authenticate")
		if strings.Count(got, `resource_metadata="`) != 1 || strings.Count(got, `"`) != 4 {
			t.Fatalf("WWW-Authenticate = %q", got)
		}
	})
}

// An app said yes to long ago, whose tokens have all ended, is cleared when
// another registers, so nobody can fill the registry for good with apps of
// their own. One with a token that still works stays.
func TestLongUnusedAppsAreCleared(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		ctx := context.Background()
		old := store.Now().Add(-40 * 24 * time.Hour)
		for _, id := range []string{"spent", "live"} {
			if _, err := d.ExecContext(ctx, dollars(d, "INSERT INTO oauth_clients (id, name, redirect_uris, created_at, used_at) VALUES (?, ?, '[]', ?, ?)"), id, id, old, old); err != nil {
				t.Fatal(err)
			}
		}
		for _, tok := range []struct {
			client  string
			revoked any
		}{{"spent", old}, {"live", nil}} {
			if _, err := d.ExecContext(ctx, dollars(d, "INSERT INTO api_tokens (id, user_id, name, token_hash, can_write, workspace_id, created_at, revoked_at, oauth_client_id) VALUES (?, ?, 'App', ?, false, ?, ?, ?, ?)"),
				uuid.NewString(), w.owner.userID(), uuid.NewString(), w.ws, old, tok.revoked, tok.client); err != nil {
				t.Fatal(err)
			}
		}
		if code, out := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{"https://app.example/cb"}}); code != 201 {
			t.Fatalf("register = %d %v", code, out)
		}
		var left []string
		rows, err := d.QueryContext(ctx, "SELECT id FROM oauth_clients WHERE id IN ('spent', 'live')")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			left = append(left, id)
		}
		_ = rows.Close()
		if len(left) != 1 || left[0] != "live" {
			t.Fatalf("apps left = %v, want only live", left)
		}
	})
}

// A full registry makes room by dropping apps nobody said yes to within an
// hour, so filling it once doesn't block every new app for a day. One that
// is full of apps that registered just now still refuses.
func TestAFullRegistryMakesRoom(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := oauthWorld(t, d)
		ctx := context.Background()
		fill := func(prefix string, at time.Time) {
			t.Helper()
			for i := range 1000 {
				if _, err := d.ExecContext(ctx, dollars(d, "INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, 'Filler', '[]', ?)"), fmt.Sprintf("%s%d", prefix, i), at); err != nil {
					t.Fatal(err)
				}
			}
		}
		count := func() (n int) {
			t.Helper()
			if err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM oauth_clients").Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		fill("old", store.Now().Add(-2*time.Hour))
		if code, out := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{"https://app.example/cb"}}); code != 201 {
			t.Fatalf("register into a registry full of apps left waiting = %d %v", code, out)
		}
		if n := count(); n != 1 {
			t.Fatalf("apps after making room = %d, want 1", n)
		}
		fill("new", store.Now())
		if code, out := postJSON(t, w.s, registerPath, map[string]any{"redirect_uris": []string{"https://app.example/cb"}}); code != 400 || out["error"] != "temporarily_unavailable" {
			t.Fatalf("register into a registry full of new apps = %d %v", code, out)
		}
	})
}

// Registering is counted by network: an IPv4 /24 as an IPv6 /48, since
// one party easily has many addresses in either.
func TestWideRateKeyCountsNetworks(t *testing.T) {
	key := func(ip string) string {
		r := httptest.NewRequest("POST", "/", nil)
		return wideRateKey(r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip)))
	}
	for a, b := range map[string]string{
		"203.0.113.9":     "203.0.113.200",
		"2001:db8:1:2::1": "2001:db8:1:ffff::9",
	} {
		if key(a) != key(b) {
			t.Errorf("%s and %s counted apart: %s, %s", a, b, key(a), key(b))
		}
	}
	for a, b := range map[string]string{
		"203.0.113.9":     "203.0.114.9",
		"2001:db8:1:2::1": "2001:db8:2:2::1",
	} {
		if key(a) == key(b) {
			t.Errorf("%s and %s counted together: %s", a, b, key(a))
		}
	}
}

// dollars is a query written with ? as Postgres wants it, with $1, $2.
func dollars(d *db.DB, q string) string {
	if d.Dialect != db.Postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
