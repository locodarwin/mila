package mail

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSender(t *testing.T) {
	dir := t.TempDir()
	s, err := New("file://"+dir, "Mila <mila@example.com>", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Send(context.Background(), Message{To: "a@example.com", Subject: "Sign in to Mila", Text: "Open this:\nhttps://x/y"})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.eml"))
	if len(files) != 1 {
		t.Fatalf("want one message, found %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	for _, want := range []string{"To: a@example.com\r\n", "Subject: Sign in to Mila\r\n", "https://x/y"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("message lacks %q:\n%s", want, raw)
		}
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, c := range []struct{ url, from string }{
		{"ftp://host", "a@b.c"},
		{"smtp://", "a@b.c"},
		{"smtp://host", "not an address"},
	} {
		if _, err := New(c.url, c.from, log); err == nil {
			t.Errorf("New(%q, %q) should fail", c.url, c.from)
		}
	}
	if _, err := New("", "", log); err != nil {
		t.Errorf("no URL means the log sender: %v", err)
	}
}

// fakeSMTP answers one plain SMTP session, accepting the message, and
// hangs up instead of answering QUIT. It reports whether DATA ended.
func fakeSMTP(t *testing.T) (addr string, accepted <-chan bool) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	done := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- false
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		say := func(s string) { _, _ = io.WriteString(conn, s+"\r\n") }
		say("220 fake ESMTP")
		inData, ended := false, false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				done <- ended
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case inData && line == ".":
				inData, ended = false, true
				say("250 queued")
			case inData:
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				say("250 fake")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				say("354 go ahead")
			case strings.HasPrefix(line, "QUIT"):
				done <- ended
				return // hang up without 221
			default:
				say("250 ok")
			}
		}
	}()
	return ln.Addr().String(), done
}

// Once the server has accepted the message, a failed QUIT is not a failed
// send: the job would send the same email again.
func TestSMTPAcceptedMessageIsSent(t *testing.T) {
	addr, accepted := fakeSMTP(t)
	s, err := New("smtp://"+addr, "Mila <mila@example.com>", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), Message{To: "a@example.com", Subject: "Hello", Text: "Hi"}); err != nil {
		t.Fatalf("a message the server accepted failed to send: %v", err)
	}
	if !<-accepted {
		t.Fatal("the server never got the message")
	}
}

func TestFileDir(t *testing.T) {
	for in, want := range map[string]string{
		"/srv/mila/mail": filepath.FromSlash("/srv/mila/mail"),
		"/C:/mila/mail":  filepath.FromSlash("C:/mila/mail"),
		`C:\mila\mail`:   `C:\mila\mail`,
	} {
		if got := fileDir(in); got != want {
			t.Errorf("fileDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// A check signs in as sending would, and sends nothing.
func TestCheckSendsNothing(t *testing.T) {
	addr, accepted := fakeSMTP(t)
	ok, what, err := Check(context.Background(), "smtp://"+addr, "Mila <mila@example.com>")
	if err != nil || !ok || !strings.Contains(what, "nothing was sent") {
		t.Fatalf("check: %v %q %v", ok, what, err)
	}
	if <-accepted {
		t.Fatal("a check sent a message")
	}
	// No server set is no failure, but is not set up.
	if ok, what, err := Check(context.Background(), "", "Mila <mila@example.com>"); ok || err != nil || !strings.Contains(what, "not set up") {
		t.Errorf("no URL: %v %q %v", ok, what, err)
	}
	// One that cannot be reached is.
	if _, _, err := Check(context.Background(), "smtp://127.0.0.1:1", "Mila <mila@example.com>"); err == nil {
		t.Error("a server that is not there passed")
	}
}
