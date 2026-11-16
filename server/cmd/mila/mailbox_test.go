package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/msauth/mstest"
)

// mila mailbox connect says the code to enter, waits for the sign-in and keeps
// it; disconnect forgets it.
func TestMailboxCommand(t *testing.T) {
	ms := mstest.New(t, "shawn@contoso.com")
	flags := []string{
		"--data-dir", t.TempDir(),
		"--imap-url", "imaps://tasks%40contoso.com@outlook.office365.com:993/INBOX",
		"--inbound-address", "tasks+{token}@contoso.com",
		"--imap-auth", "microsoft",
		"--microsoft-client-id", "3f2a0000-0000-0000-0000-000000009c1e",
		"--microsoft-authority", ms.URL,
	}
	go func() {
		for ms.Asked("devicecode") == 0 {
			time.Sleep(20 * time.Millisecond)
		}
		ms.Approve()
	}()
	var out bytes.Buffer
	if err := mailboxRun(append([]string{"connect"}, flags...), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://microsoft.com/devicelogin", "K7PQ2XMF", "tasks@contoso.com", "Signed in as shawn@contoso.com"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("connect said %q, without %q", out.String(), want)
		}
	}
	out.Reset()
	if err := mailboxRun(append([]string{"disconnect"}, flags...), &out); err != nil || !strings.Contains(out.String(), "signed out of tasks@contoso.com") {
		t.Fatalf("disconnect = %v, %q", err, out.String())
	}
	if err := mailboxRun([]string{"connect", "--data-dir", t.TempDir()}, &out); err == nil || !strings.Contains(err.Error(), "takes a password") {
		t.Fatalf("connect with no Microsoft mailbox = %v", err)
	}
}
