package inbound_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/inbound"
)

// mailbox is a real IMAP server, in this process.
type mailbox struct {
	t    *testing.T
	addr string
	user *imapmemserver.User
}

func newMailbox(t *testing.T) *mailbox {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("mila@example.com", "s3cret-pass")
	must(t, user.Create("INBOX", nil))
	mem.AddUser(user)
	server := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapUIDPlus: {}},
		InsecureAuth: true,
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return &mailbox{t: t, addr: listener.Addr().String(), user: user}
}

func (b *mailbox) url(password string) string {
	return "imap://mila%40example.com:" + password + "@" + b.addr + "/INBOX?tls=none"
}

type literal struct {
	*strings.Reader
	size int64
}

func (l literal) Size() int64 { return l.size }

func (b *mailbox) add(raw string) {
	b.t.Helper()
	raw = strings.ReplaceAll(raw, "\n", "\r\n")
	_, err := b.user.Append("INBOX", literal{strings.NewReader(raw), int64(len(raw))}, &imap.AppendOptions{Time: time.Now()})
	must(b.t, err)
}

// look reads the mailbox as a person's mail program would.
func (b *mailbox) look() (subjects []string, unread int) {
	b.t.Helper()
	c, err := imapclient.DialInsecure(b.addr, nil)
	must(b.t, err)
	defer func() { _ = c.Close() }()
	must(b.t, c.Login("mila@example.com", "s3cret-pass").Wait())
	selected, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
	must(b.t, err)
	if selected.NumMessages == 0 {
		return nil, 0
	}
	section := &imap.FetchItemBodySection{Peek: true, Specifier: imap.PartSpecifierHeader}
	msgs, err := c.Fetch(allMessages(), &imap.FetchOptions{Flags: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	must(b.t, err)
	for _, m := range msgs {
		seen := false
		for _, f := range m.Flags {
			seen = seen || f == imap.FlagSeen
		}
		if !seen {
			unread++
		}
		for _, line := range strings.Split(string(m.FindBodySection(section)), "\r\n") {
			if rest, ok := strings.CutPrefix(line, "Subject: "); ok {
				subjects = append(subjects, rest)
			}
		}
	}
	return subjects, unread
}

func allMessages() imap.SeqSet {
	var set imap.SeqSet
	set.AddRange(1, 0)
	return set
}

func (w *world) readerOf(t *testing.T, url string, deleteAfter bool) *inbound.Reader {
	t.Helper()
	r, err := inbound.NewReader(w.svc, url, time.Hour, deleteAfter, nil)
	must(t, err)
	return r
}

func TestReadingAMailbox(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		box := newMailbox(t)
		mail := func(id, subject, to string) string {
			return fmt.Sprintf("From: %s\nTo: %s\nMessage-ID: <%s@example.org>\nSubject: %s\n\nHello.\n", w.email, to, id, subject)
		}
		ours := "mila+" + token + "@example.com"
		// Mail from before Mila was told of the mailbox stays unread by it.
		box.add(mail("old", "From before", ours))
		r := w.readerOf(t, box.url("s3cret-pass"), false)
		must(t, r.CheckOnce(w.ctx))
		if n := len(w.tasks()); n != 0 {
			t.Fatalf("made %d tasks from old mail", n)
		}

		box.add(mail("new", "New task", ours))
		box.add(mail("private", "Dinner on Friday", "someone@example.com"))
		must(t, r.CheckOnce(w.ctx))
		tasks := w.tasks()
		if len(tasks) != 1 || tasks[0].Fields["title"] != "New task" {
			t.Fatalf("%v", tasks)
		}
		// It changed nothing: all of it is still there, still unread.
		subjects, unread := box.look()
		if len(subjects) != 3 || unread != 3 {
			t.Fatalf("%v, %d unread", subjects, unread)
		}

		// A later check does not read it again, or a new reader after a restart.
		must(t, r.CheckOnce(w.ctx))
		must(t, w.readerOf(t, box.url("s3cret-pass"), false).CheckOnce(w.ctx))
		wantOutcomes(t, w.outcomes(), inbound.Made)
		box.add(mail("newer", "Newer task", ours))
		must(t, w.readerOf(t, box.url("s3cret-pass"), false).CheckOnce(w.ctx))
		if n := len(w.tasks()); n != 2 {
			t.Fatalf("%d tasks", n)
		}
	})
}

func TestDeletingWhatIsDealtWithAndNothingElse(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		box := newMailbox(t)
		r := w.readerOf(t, box.url("s3cret-pass"), true)
		must(t, r.CheckOnce(w.ctx))
		box.add(fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nMessage-ID: <a@example.org>\nSubject: Ours\n\nHello.\n", w.email, token))
		box.add(fmt.Sprintf("From: stranger@example.org\nTo: mila+%s@example.com\nMessage-ID: <b@example.org>\nSubject: Refused\n\nHello.\n", token))
		box.add("From: a@example.org\nTo: someone@example.org\nMessage-ID: <c@example.org>\nSubject: Not ours\n\nHello.\n")
		must(t, r.CheckOnce(w.ctx))
		if subjects, _ := box.look(); len(subjects) != 1 || subjects[0] != "Not ours" {
			t.Fatalf("left %v", subjects)
		}
		if n := len(w.tasks()); n != 1 {
			t.Fatalf("%d tasks", n)
		}
	})
}

func TestAWrongPasswordSaysSoAndNeverRepeatsIt(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		box := newMailbox(t)
		err := w.readerOf(t, box.url("wrong-pass-value"), false).CheckOnce(w.ctx)
		if err == nil || !strings.Contains(err.Error(), "refused the sign-in") {
			t.Fatalf("%v", err)
		}
		if strings.Contains(err.Error(), "wrong-pass-value") {
			t.Fatalf("repeats the password: %v", err)
		}
	})
}

func TestARebuiltMailboxStartsFromItsNewest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		first := newMailbox(t)
		r := w.readerOf(t, first.url("s3cret-pass"), false)
		must(t, r.CheckOnce(w.ctx))
		// Another server's numbers mean nothing here, so old mail is left alone.
		second := newMailbox(t)
		second.add(fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nMessage-ID: <x@example.org>\nSubject: Old there\n\nHello.\n", w.email, token))
		must(t, w.st.UpsertInstanceSetting(w.ctx, inbound.PositionParams(`{"mailbox":"INBOX","uidValidity":99,"lastUid":0}`)))
		must(t, w.readerOf(t, second.url("s3cret-pass"), false).CheckOnce(w.ctx))
		if n := len(w.tasks()); n != 0 {
			t.Fatalf("%d tasks", n)
		}
	})
}

func TestStoppingDoesNotHang(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer func() { _ = listener.Close() }()
	// A server that accepts and then says nothing.
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()
	r, err := inbound.NewReader(nil, "imap://u:p@"+listener.Addr().String()+"/INBOX?tls=none", time.Hour, false, nil)
	must(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.CheckOnce(ctx) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a silent server was taken for a working one")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting for a silent server after being told to stop")
	}
}

func TestCheckingAMailboxReadsNothing(t *testing.T) {
	b := newMailbox(t)
	b.add("From: a@example.com\nTo: mila@example.com\nSubject: Kept\n\nHi\n")
	what, err := inbound.CheckMailbox(context.Background(), b.url("s3cret-pass"), nil)
	if err != nil || !strings.Contains(what, "with 1 messages") {
		t.Fatalf("check: %q %v", what, err)
	}
	if _, unread := b.look(); unread != 1 {
		t.Errorf("the check marked mail read: %d unread", unread)
	}
	if _, err := inbound.CheckMailbox(context.Background(), b.url("wrong"), nil); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Errorf("a wrong password: %v", err)
	}
	missing := strings.Replace(b.url("s3cret-pass"), "/INBOX", "/Nowhere", 1)
	if _, err := inbound.CheckMailbox(context.Background(), missing, nil); err == nil || !strings.Contains(err.Error(), "Nowhere") {
		t.Errorf("a missing mailbox: %v", err)
	}
	folder := "file:///" + strings.TrimPrefix(filepath.ToSlash(t.TempDir()), "/")
	if what, err := inbound.CheckMailbox(context.Background(), folder, nil); err != nil || !strings.Contains(what, ".eml") {
		t.Errorf("a folder: %q %v", what, err)
	}
}

// A file:// URL names a folder on Windows too, where its path starts /C:.
func TestFolderOfAFileURL(t *testing.T) {
	for raw, want := range map[string]string{
		"file:///C:/mail":   filepath.FromSlash("C:/mail"),
		"file:///var/mail":  filepath.FromSlash("/var/mail"),
		"file:///srv/C:odd": filepath.FromSlash("/srv/C:odd"),
	} {
		u, err := url.Parse(raw)
		must(t, err)
		if got := inbound.FolderOf(u); got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
}
