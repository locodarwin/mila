package accounts

import "testing"

// An app is sent back only to https, http to this computer (RFC 8252
// section 7.3), or a scheme of its own written as a domain it holds,
// reversed (section 7.1), or the bare scheme of a known MCP client. Any
// other bare name could be any handler on the computer, several of which
// have run what a link gave them.
func TestRedirectShapes(t *testing.T) {
	for _, good := range []string{
		"https://app.example/cb",
		"http://127.0.0.1:51234/cb",
		"http://localhost/cb",
		"http://[::1]:9/cb",
		"com.example.app:/cb",
		"com.example:/oauth",
		"io.github.some-one.tool://cb",
		"cursor://anysphere.cursor-mcp/oauth/callback",
		"windsurf://codeium.windsurf/oauth/callback",
		"vscode://vscode.github-authentication/did-authenticate",
		"vscode-insiders://vscode.github-authentication/did-authenticate",
		"zed://oauth/callback",
	} {
		if why := checkRedirect(good); why != "" {
			t.Errorf("checkRedirect(%q) = %q, want it taken", good, why)
		}
	}
	for _, bad := range []string{
		"myapp:/cb",
		"cursorx://cb",
		"code://cb",
		"slack://open",
		"itms-services://?action=download-manifest",
		"zoomus://x",
		"tg://x",
		"webcal://x",
		"magnet:?xt=x",
		"irc://x",
		"ms-officecmd:x",
		"vnd.ms-excel:x",
		"x-apple.systempreferences:x",
		"com..example:/cb",
		"com.example.:/cb",
		".com.example:/cb",
		"com.exam+ple:/cb",
		"com.-example:/cb",
		"javascript:alert(1)",
		"http://evil.example/cb",
	} {
		if why := checkRedirect(bad); why == "" {
			t.Errorf("checkRedirect(%q) took it", bad)
		}
	}
}
