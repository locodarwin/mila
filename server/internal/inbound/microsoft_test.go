package inbound_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/keyfile"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/msauth"
	"github.com/locodarwin/mila/server/internal/msauth/mstest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// microsoftSession is a mailbox's session that takes a token by XOAUTH2, as
// Microsoft's IMAP does, and no password.
type microsoftSession struct {
	// Session is the memory server's own, which goes on once signed in.
	imapserver.Session
	valid func(string) bool
}

func (s *microsoftSession) AuthenticateMechanisms() []string { return []string{"XOAUTH2"} }

func (s *microsoftSession) Authenticate(mech string) (sasl.Server, error) {
	if mech != "XOAUTH2" {
		return nil, errors.New("only XOAUTH2")
	}
	return &xoauth2Server{s: s}, nil
}

func (s *microsoftSession) Login(string, string) error {
	return errors.New("this mailbox takes no password")
}

type xoauth2Server struct{ s *microsoftSession }

func (x *xoauth2Server) Next(resp []byte) ([]byte, bool, error) {
	parts := bytes.Split(resp, []byte{1})
	var user, token string
	for _, p := range parts {
		if v, ok := bytes.CutPrefix(p, []byte("user=")); ok {
			user = string(v)
		}
		if v, ok := bytes.CutPrefix(p, []byte("auth=Bearer ")); ok {
			token = string(v)
		}
	}
	if user != "tasks@contoso.com" || !x.s.valid(token) {
		return nil, true, errors.New("AUTHENTICATE failed")
	}
	// Signed in: the memory server's own session goes on from here.
	return nil, true, x.s.Session.Login("tasks@contoso.com", "unused")
}

// microsoftMailbox is an IMAP server that takes the stand-in Microsoft's tokens.
func microsoftMailbox(t *testing.T, ms *mstest.Microsoft) (addr string, user *imapmemserver.User) {
	t.Helper()
	mem := imapmemserver.New()
	user = imapmemserver.NewUser("tasks@contoso.com", "unused")
	must(t, user.Create("INBOX", nil))
	mem.AddUser(user)
	server := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &microsoftSession{Session: mem.NewSession(), valid: ms.ValidAccess}, nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapUIDPlus: {}, imap.CapSASLIR: {}},
		InsecureAuth: true,
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().String(), user
}

type sentMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (s *sentMail) send(_ context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

func (s *sentMail) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

// waitFor waits for the sign-in to reach a state.
func waitFor(t *testing.T, m *inbound.Microsoft, state string) inbound.MicrosoftState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s, err := m.State(context.Background())
		must(t, err)
		if s.State == state {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("the sign-in is %q (%s), not %q", s.State, s.Problem, state)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A Microsoft mailbox: an admin enters a code at Microsoft, Mila keeps the
// sign-in and reads the mailbox with it; when Microsoft stops taking it,
// email to task pauses, the admins are told once, and signing in again
// starts it where it left off.
func TestAMicrosoftMailbox(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		must(t, w.st.SetInstanceAdmin(w.ctx, sqlitedb.SetInstanceAdminParams{IsInstanceAdmin: true, Now: store.Now(), ID: w.user}))
		_, token := w.address("", false)
		ms := mstest.New(t, "shawn@contoso.com")
		addr, _ := microsoftMailbox(t, ms)
		sent := &sentMail{}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		key := keyfile.At(t.TempDir())
		m := inbound.NewMicrosoft(msauth.Config{ClientID: "3f2a0000-0000-0000-0000-000000009c1e", Tenant: "common", Authority: ms.URL}, "tasks@contoso.com", w.st, key, log, sent.send, "https://mila.example.com")
		r, err := inbound.NewReader(w.svc, "imap://tasks%40contoso.com@"+addr+"/INBOX?tls=none", time.Hour, false, m)
		must(t, err)

		// Before anyone signs in, it waits, asking the mailbox nothing.
		if err := r.CheckOnce(w.ctx); !errors.Is(err, inbound.ErrNotConnected) {
			t.Fatalf("checking before signing in: %v", err)
		}
		// Declined at Microsoft: it says so.
		ms.Decline()
		if _, err := m.Connect(w.ctx, "Shawn"); err != nil {
			t.Fatal(err)
		}
		if s := waitFor(t, m, "not-connected"); !strings.Contains(s.Problem, "declined") {
			t.Fatalf("a declined sign-in says %q", s.Problem)
		}

		// Signed in.
		ms = restart(t, ms)
		m = inbound.NewMicrosoft(msauth.Config{ClientID: "3f2a0000-0000-0000-0000-000000009c1e", Tenant: "common", Authority: ms.URL}, "tasks@contoso.com", w.st, key, log, sent.send, "https://mila.example.com")
		addr, box := microsoftMailbox(t, ms)
		r, err = inbound.NewReader(w.svc, "imap://tasks%40contoso.com@"+addr+"/INBOX?tls=none", time.Hour, false, m)
		must(t, err)
		s, err := m.Connect(w.ctx, "Shawn")
		must(t, err)
		if s.State != "signing-in" || s.UserCode == "" || s.VerificationURI == "" || s.ExpiresAt == nil {
			t.Fatalf("while signing in: %+v", s)
		}
		ms.Approve()
		s = waitFor(t, m, "connected")
		if s.Account != "shawn@contoso.com" || s.ConnectedBy != "Shawn" || s.ConnectedAt == nil {
			t.Fatalf("signed in: %+v", s)
		}
		// Kept sealed: the database alone holds no refresh token.
		raw, ok, err := w.st.InstanceSetting(w.ctx, "inbound_microsoft")
		must(t, err)
		if !ok || strings.Contains(raw, "refresh-") || !strings.Contains(raw, `"refresh":"sealed:v1:`) {
			t.Fatalf("the sign-in kept: %q", raw)
		}

		// It reads the mailbox: first where it starts, then new mail.
		must(t, r.CheckOnce(w.ctx))
		appendTo(t, box, "From: "+w.email+"\r\nTo: mila+"+token+"@example.com\r\nMessage-ID: <m1@example.org>\r\nSubject: Fix the gate\r\n\r\nHello.\r\n")
		must(t, r.CheckOnce(w.ctx))
		if tasks := w.tasks(); len(tasks) != 1 || tasks[0].Fields["title"] != "Fix the gate" {
			t.Fatalf("tasks after a check: %v", tasks)
		}

		// The access token runs out: the refresh token gets another, and the
		// new refresh token Microsoft hands back replaces the old.
		m.Expire()
		must(t, r.CheckOnce(w.ctx))
		m.Expire()
		must(t, r.CheckOnce(w.ctx))
		if ms.Asked("refresh_token") != 2 {
			t.Fatalf("refreshed %d times", ms.Asked("refresh_token"))
		}

		// Microsoft stops taking it: email to task pauses, the admin is told, once.
		ms.Revoke()
		m.Expire()
		if err := r.CheckOnce(w.ctx); !errors.Is(err, msauth.ErrLost) {
			t.Fatalf("checking after the sign-in was revoked: %v", err)
		}
		if err := r.CheckOnce(w.ctx); !errors.Is(err, msauth.ErrLost) {
			t.Fatalf("checking again: %v", err)
		}
		s = waitFor(t, m, "lost")
		if s.LostAt == nil || s.Account != "shawn@contoso.com" {
			t.Fatalf("lost: %+v", s)
		}
		if n := sent.count(); n != 1 {
			t.Fatalf("told the admins %d times", n)
		}
		if msg := sent.sent[0]; msg.To != w.email || !strings.Contains(msg.Text, "Connect again") || !strings.Contains(msg.Text, "https://mila.example.com/settings/admin") {
			t.Fatalf("the email: %+v", msg)
		}

		// Mail arrives meanwhile, and is read once signed in again.
		appendTo(t, box, "From: "+w.email+"\r\nTo: mila+"+token+"@example.com\r\nMessage-ID: <m2@example.org>\r\nSubject: Oil the hinges\r\n\r\nHello.\r\n")
		_, err = m.Connect(w.ctx, "Shawn")
		must(t, err)
		ms.Approve()
		waitFor(t, m, "connected")
		must(t, r.CheckOnce(w.ctx))
		if tasks := w.tasks(); len(tasks) != 2 {
			t.Fatalf("tasks after signing in again: %v", tasks)
		}

		// Disconnected: it is forgotten.
		must(t, m.Disconnect(w.ctx))
		waitFor(t, m, "not-connected")
		if _, ok, _ := w.st.InstanceSetting(w.ctx, "inbound_microsoft"); ok {
			t.Fatal("the sign-in is still kept after disconnecting")
		}
		if err := r.CheckOnce(w.ctx); !errors.Is(err, inbound.ErrNotConnected) {
			t.Fatalf("checking after disconnecting: %v", err)
		}
	})
}

// restart is a new stand-in, after one that a person declined.
func restart(t *testing.T, old *mstest.Microsoft) *mstest.Microsoft {
	return mstest.New(t, old.Account)
}

func appendTo(t *testing.T, box *imapmemserver.User, raw string) {
	t.Helper()
	_, err := box.Append("INBOX", literal{strings.NewReader(raw), int64(len(raw))}, &imap.AppendOptions{Time: time.Now()})
	must(t, err)
}

// holdRefreshes is a transport that, while holding, stops a refresh on its
// way to Microsoft until let go: a refresh out when someone disconnects.
type holdRefreshes struct {
	mu      sync.Mutex
	holding bool
	started chan struct{}
	release chan struct{}
}

func (h *holdRefreshes) RoundTrip(r *http.Request) (*http.Response, error) {
	h.mu.Lock()
	hold := h.holding
	h.mu.Unlock()
	if hold && strings.HasSuffix(r.URL.Path, "/token") {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if bytes.Contains(body, []byte("grant_type=refresh_token")) {
			close(h.started)
			<-h.release
		}
	}
	return http.DefaultTransport.RoundTrip(r)
}

// A disconnect stays a disconnect: a refresh out at the time can't bring the
// sign-in back, and one made by another process (mila mailbox disconnect)
// stops this one using the access token it holds.
func TestDisconnectingStopsTheSignIn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		ms := mstest.New(t, "tasks@contoso.com")
		hold := &holdRefreshes{started: make(chan struct{}), release: make(chan struct{})}
		cfg := msauth.Config{ClientID: "c", Tenant: "common", Authority: ms.URL, HTTP: &http.Client{Transport: hold}}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		key := keyfile.At(t.TempDir())
		m := inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, key, log, nil, "")
		signIn := func() {
			t.Helper()
			_, err := m.Connect(w.ctx, "Shawn")
			must(t, err)
			ms.Approve()
			waitFor(t, m, "connected")
		}
		signIn()

		m.Expire()
		hold.mu.Lock()
		hold.holding = true
		hold.mu.Unlock()
		got := make(chan error, 1)
		go func() { _, err := m.Token(w.ctx); got <- err }()
		<-hold.started
		must(t, m.Disconnect(w.ctx))
		close(hold.release)
		if err := <-got; err == nil {
			t.Fatal("a refresh out during a disconnect gave a token")
		}
		hold.mu.Lock()
		hold.holding = false
		hold.mu.Unlock()
		if s, err := m.State(w.ctx); err != nil || s.State != "not-connected" {
			t.Fatalf("after the refresh came back: %+v %v", s, err)
		}
		if _, err := m.Token(w.ctx); !errors.Is(err, inbound.ErrNotConnected) {
			t.Fatalf("a token after disconnecting: %v", err)
		}

		// Another process disconnects: this one stops at its next check,
		// though its access token has an hour left.
		signIn()
		if _, err := m.Token(w.ctx); err != nil {
			t.Fatal(err)
		}
		other := inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, key, log, nil, "")
		must(t, other.Disconnect(w.ctx))
		if _, err := m.Token(w.ctx); !errors.Is(err, inbound.ErrNotConnected) {
			t.Fatalf("a token after another process disconnected: %v", err)
		}
	})
}

// The sign-in is sealed with the data directory's key. One stored before
// sealing still works, and is sealed at its next refresh; a database moved
// to a server without the key reads as lost, says why, and tells the admins
// once, keeping what it can't open as it was.
func TestTheSignInIsSealed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		must(t, w.st.SetInstanceAdmin(w.ctx, sqlitedb.SetInstanceAdminParams{IsInstanceAdmin: true, Now: store.Now(), ID: w.user}))
		ms := mstest.New(t, "tasks@contoso.com")
		cfg := msauth.Config{ClientID: "c", Tenant: "common", Authority: ms.URL}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		key := keyfile.At(t.TempDir())
		sent := &sentMail{}
		m := inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, key, log, sent.send, "https://mila.example.com")
		_, err := m.Connect(w.ctx, "Shawn")
		must(t, err)
		ms.Approve()
		waitFor(t, m, "connected")
		row := func() map[string]any {
			t.Helper()
			raw, ok, err := w.st.InstanceSetting(w.ctx, "inbound_microsoft")
			must(t, err)
			var v map[string]any
			if !ok || json.Unmarshal([]byte(raw), &v) != nil {
				t.Fatalf("the row: %q", raw)
			}
			return v
		}

		// A sign-in stored before sealing, in plain text: used, then sealed.
		v := row()
		plain, err := key.Open(v["refresh"].(string), "inbound_microsoft")
		must(t, err)
		v["refresh"] = plain
		old, _ := json.Marshal(v)
		must(t, w.st.SetInstanceSetting(w.ctx, "inbound_microsoft", string(old)))
		m.Expire()
		if _, err := m.Token(w.ctx); err != nil {
			t.Fatalf("a token from a sign-in stored before sealing: %v", err)
		}
		sealed := row()["refresh"].(string)
		if !keyfile.IsSealed(sealed) {
			t.Fatalf("not sealed after its refresh: %q", sealed)
		}

		// The database on another server, without the key.
		elsewhere := inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, keyfile.At(t.TempDir()), log, sent.send, "https://mila.example.com")
		if s, err := elsewhere.State(w.ctx); err != nil || s.State != "lost" || !s.KeyMissing || !strings.Contains(s.LostWhy, keyfile.Name) {
			t.Fatalf("its state elsewhere: %+v %v", s, err)
		}
		if sent.count() != 0 {
			t.Fatal("looking at the state told the admins")
		}
		asked := ms.Asked("refresh_token")
		before, _, _ := w.st.InstanceSetting(w.ctx, "inbound_microsoft")
		for range 2 {
			if _, err := elsewhere.Token(w.ctx); !errors.Is(err, inbound.ErrKeyUnreadable) {
				t.Fatalf("a token elsewhere: %v", err)
			}
		}
		if ms.Asked("refresh_token") != asked {
			t.Fatal("asked Microsoft with a token it couldn't open")
		}
		s, err := elsewhere.State(w.ctx)
		must(t, err)
		if s.State != "lost" || !s.KeyMissing {
			t.Fatalf("lost elsewhere: %+v", s)
		}
		if n := sent.count(); n != 1 || !strings.Contains(sent.sent[0].Text, "can't open") || strings.Contains(sent.sent[0].Text, "Microsoft stopped") {
			t.Fatalf("told the admins %d times: %+v", n, sent.sent)
		}
		// Nothing is written: with the key, the sign-in is as good as ever.
		if after, _, _ := w.st.InstanceSetting(w.ctx, "inbound_microsoft"); after != before {
			t.Fatalf("the row was changed: %q", after)
		}
		if got := row()["refresh"].(string); got != sealed {
			t.Fatalf("what it couldn't open was changed: %q", got)
		}

		// Signing in again there works, with a key of its own.
		_, err = elsewhere.Connect(w.ctx, "Shawn")
		must(t, err)
		ms.Approve()
		if s := waitFor(t, elsewhere, "connected"); s.KeyMissing || s.LostAt != nil {
			t.Fatalf("signed in again: %+v", s)
		}
		if _, err := elsewhere.Token(w.ctx); err != nil {
			t.Fatal(err)
		}
	})
}

// connectedMicrosoft is a sign-in made and kept with a key in dir.
func connectedMicrosoft(t *testing.T, w *world, ms *mstest.Microsoft, dir string, sent *sentMail) *inbound.Microsoft {
	t.Helper()
	cfg := msauth.Config{ClientID: "c", Tenant: "common", Authority: ms.URL}
	m := inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, keyfile.At(dir), slog.New(slog.NewTextHandler(io.Discard, nil)), sent.send, "https://mila.example.com")
	_, err := m.Connect(w.ctx, "Shawn")
	must(t, err)
	ms.Approve()
	waitFor(t, m, "connected")
	return m
}

// A key this process can't use (another data directory, a key file taken
// away or unreadable) never marks the sign-in lost for good: the server
// with the key goes on, and putting the key back brings it back.
func TestAKeyProblemIsNeverLostForGood(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		must(t, w.st.SetInstanceAdmin(w.ctx, sqlitedb.SetInstanceAdminParams{IsInstanceAdmin: true, Now: store.Now(), ID: w.user}))
		ms := mstest.New(t, "tasks@contoso.com")
		cfg := msauth.Config{ClientID: "c", Tenant: "common", Authority: ms.URL}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		dir := t.TempDir()
		sent := &sentMail{}
		server := connectedMicrosoft(t, w, ms, dir, sent)
		fresh := func(at string) *inbound.Microsoft {
			return inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, keyfile.At(at), log, sent.send, "https://mila.example.com")
		}

		// Another process with the wrong data directory: it can't, and
		// the server can, at once and later.
		if _, err := fresh(t.TempDir()).Token(w.ctx); !errors.Is(err, inbound.ErrKeyUnreadable) {
			t.Fatalf("a token with the wrong data directory: %v", err)
		}
		server.Expire()
		if _, err := server.Token(w.ctx); err != nil {
			t.Fatalf("the server after another process lacked the key: %v", err)
		}

		// The key file taken away, then put back.
		key := filepath.Join(dir, keyfile.Name)
		must(t, os.Rename(key, key+".away"))
		gone := fresh(dir)
		if s, err := gone.State(w.ctx); err != nil || s.State != "lost" || !s.KeyMissing {
			t.Fatalf("with the key away: %+v %v", s, err)
		}
		if _, err := gone.Token(w.ctx); !errors.Is(err, inbound.ErrKeyUnreadable) {
			t.Fatalf("a token with the key away: %v", err)
		}
		must(t, os.Rename(key+".away", key))
		back := fresh(dir)
		if s, err := back.State(w.ctx); err != nil || s.State != "connected" {
			t.Fatalf("with the key back: %+v %v", s, err)
		}
		if _, err := back.Token(w.ctx); err != nil {
			t.Fatalf("a token with the key back: %v", err)
		}

		// A key file this process may not read: said so, not a failure.
		if runtime.GOOS != "windows" && os.Geteuid() != 0 {
			must(t, os.Chmod(key, 0))
			locked := fresh(dir)
			s, err := locked.State(w.ctx)
			if err != nil || s.State != "lost" || !s.KeyMissing || !strings.Contains(s.LostWhy, "permission denied") {
				t.Fatalf("with the key unreadable: %+v %v", s, err)
			}
			if _, err := locked.Token(w.ctx); !errors.Is(err, inbound.ErrKeyUnreadable) {
				t.Fatalf("a token with the key unreadable: %v", err)
			}
			must(t, os.Chmod(key, 0o600))
			if _, err := locked.Token(w.ctx); err != nil {
				t.Fatalf("a token with the key readable again: %v", err)
			}
		}

		// A sign-in an older Mila marked lost for a missing key comes back
		// once the key opens it.
		raw, _, err := w.st.InstanceSetting(w.ctx, "inbound_microsoft")
		must(t, err)
		var v map[string]any
		must(t, json.Unmarshal([]byte(raw), &v))
		v["lostAt"], v["lostWhy"], v["keyMissing"] = time.Now().UTC().Format(time.RFC3339), "it was sealed with a key this Mila does not have", true
		marked, _ := json.Marshal(v)
		must(t, w.st.SetInstanceSetting(w.ctx, "inbound_microsoft", string(marked)))
		older := fresh(dir)
		if s, err := older.State(w.ctx); err != nil || s.State != "connected" {
			t.Fatalf("marked lost by an older Mila, with the key: %+v %v", s, err)
		}
		if _, err := older.Token(w.ctx); err != nil {
			t.Fatalf("a token marked lost by an older Mila, with the key: %v", err)
		}
		if raw, _, _ := w.st.InstanceSetting(w.ctx, "inbound_microsoft"); strings.Contains(raw, "lostAt") || strings.Contains(raw, "keyMissing") {
			t.Fatalf("still marked: %q", raw)
		}
	})
}

// mila config check signs in to the mailbox and writes nothing: not a
// refreshed sign-in, and not a lost one, whether its key is wrong or
// Microsoft refuses; and it tells nobody.
func TestCheckingTheMailboxWritesNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		must(t, w.st.SetInstanceAdmin(w.ctx, sqlitedb.SetInstanceAdminParams{IsInstanceAdmin: true, Now: store.Now(), ID: w.user}))
		ms := mstest.New(t, "tasks@contoso.com")
		addr, _ := microsoftMailbox(t, ms)
		cfg := msauth.Config{ClientID: "c", Tenant: "common", Authority: ms.URL}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		dir := t.TempDir()
		sent := &sentMail{}
		connectedMicrosoft(t, w, ms, dir, sent)
		row := func() string {
			raw, _, err := w.st.InstanceSetting(w.ctx, "inbound_microsoft")
			must(t, err)
			return raw
		}
		before := row()
		imapURL := "imap://tasks%40contoso.com@" + addr + "/INBOX?tls=none"
		check := func(at string) (string, error) {
			return inbound.CheckMailbox(w.ctx, imapURL, inbound.NewMicrosoft(cfg, "tasks@contoso.com", w.st, keyfile.At(at), log, sent.send, "https://mila.example.com"))
		}

		if what, err := check(dir); err != nil || !strings.Contains(what, "opened INBOX") {
			t.Fatalf("checking: %q %v", what, err)
		}
		if _, err := check(t.TempDir()); !errors.Is(err, inbound.ErrKeyUnreadable) {
			t.Fatalf("checking with the wrong data directory: %v", err)
		}
		ms.Revoke()
		if _, err := check(dir); !errors.Is(err, msauth.ErrLost) {
			t.Fatalf("checking a sign-in Microsoft refuses: %v", err)
		}
		if after := row(); after != before {
			t.Fatalf("checking wrote %q", after)
		}
		if sent.count() != 0 {
			t.Fatalf("checking told the admins: %+v", sent.sent)
		}
	})
}
