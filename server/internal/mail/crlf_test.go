package mail

import (
	"net/url"
	"strings"
	"testing"
)

// Text people wrote reaches SMTP with every line ended by CRLF, so a bare
// CR cannot end the message early ("\r.\r\n") on a lenient server, and the
// subject cannot add a header.
func TestRenderEndsEveryLineWithCRLF(t *testing.T) {
	out := string(render("Mila <mila@example.com>", Message{
		To:      "a@example.com",
		Subject: "Plan\r\nBcc: someone@example.net",
		Text:    "Task: Plan\r.\r\nMAIL FROM:<x@example.net>\rmore\nlast\r\n",
	}))
	for i := range len(out) {
		switch {
		case out[i] == '\r' && (i+1 == len(out) || out[i+1] != '\n'):
			t.Fatalf("a bare CR at %d in %q", i, out)
		case out[i] == '\n' && (i == 0 || out[i-1] != '\r'):
			t.Fatalf("a bare LF at %d in %q", i, out)
		}
	}
	head, body, _ := strings.Cut(out, "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") {
		t.Errorf("the subject added a header: %q", head)
	}
	want := "Task: Plan\r\n.\r\nMAIL FROM:<x@example.net>\r\nmore\r\nlast\r\n"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

// Plain text is for a server on this computer, or one the URL says to trust.
func TestPlainSMTPOnlyWhereAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"smtp://127.0.0.1:25":                   true,
		"smtp://localhost:25":                   true,
		"smtp://[::1]:25":                       true,
		"smtp://relay.lan:25":                   false,
		"smtp://relay.lan:25?tls=none":          true,
		"smtp://u:p@smtp.example.com:587":       false,
		"smtp://u:p@smtp.example.com:587?tls=x": false,
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := plainAllowed(u); got != want {
			t.Errorf("%s: %v", raw, got)
		}
	}
}
