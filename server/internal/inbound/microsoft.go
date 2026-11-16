package inbound

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/keyfile"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/msauth"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// A Microsoft mailbox (Outlook.com, Microsoft 365) takes no password over
// IMAP. Mila signs in to it through Microsoft instead (docs/PLAN.md section
// 11): an instance admin enters a code at Microsoft once, and the refresh
// token that comes back is kept here, in instance_setting, never shown, and
// sealed with the data directory's key (keyfile), so a backup of the
// database alone holds nothing usable. Each check trades it for an access
// token, which signs in to IMAP with XOAUTH2.

// microsoftKey holds the sign-in.
const microsoftKey = "inbound_microsoft"

// ErrNotConnected means nobody has signed Mila in to the Microsoft mailbox yet.
var ErrNotConnected = errors.New("not signed in to the Microsoft mailbox yet: connect it from Settings, Admin")

// ErrKeyUnreadable means this process can't open the sign-in: the key it
// was sealed with is missing here, unreadable, or another. That is never
// written down as lost, since another process (the server, with the right
// data directory) may have the key, and putting it back brings it back.
var ErrKeyUnreadable = errors.New("can't open the Microsoft sign-in here")

// msSignIn is the sign-in kept.
type msSignIn struct {
	Refresh     string    `json:"refresh"`
	Account     string    `json:"account"`
	ConnectedAt time.Time `json:"connectedAt"`
	// ConnectedBy is the name of the admin who signed it in, or empty from the
	// command line.
	ConnectedBy string     `json:"connectedBy,omitempty"`
	LostAt      *time.Time `json:"lostAt,omitempty"`
	LostWhy     string     `json:"lostWhy,omitempty"`
	// KeyMissing was set by Mila before 3.0 with LostAt, for a sign-in it
	// couldn't open. Such a row is read as not lost once the key opens it.
	KeyMissing bool `json:"keyMissing,omitempty"`

	// stored is Refresh as the row holds it, kept as it is when it can't
	// be opened here, in case the key that can is put back.
	stored     string
	unreadable bool
	// keyProblem is why it can't be opened here.
	keyProblem string
}

// errKeyMissing is why a sign-in this Mila can't open is lost.
const errKeyMissing = "it was sealed with a key this Mila does not have (" + keyfile.Name + " in its data directory), as when the database is restored on another server"

// keyProblem says why a sealed sign-in won't open here.
func keyProblem(err error) string {
	if errors.Is(err, keyfile.ErrUnreadable) {
		return errKeyMissing
	}
	return "the key it was sealed with, " + keyfile.Name + " in the data directory, can't be read: " + err.Error()
}

// MicrosoftState is how the Microsoft mailbox's sign-in stands, for the admin.
type MicrosoftState struct {
	// State is not-connected, signing-in, connected or lost.
	State       string
	Account     string
	ConnectedAt *time.Time
	ConnectedBy string
	LostAt      *time.Time
	LostWhy     string
	KeyMissing  bool
	// While signing in: the code to enter, where, and until when.
	UserCode        string
	VerificationURI string
	ExpiresAt       *time.Time
	// Problem is why the last attempt to sign in didn't work, if it didn't.
	Problem string
}

// Microsoft keeps the sign-in to a Microsoft mailbox.
type Microsoft struct {
	cfg     msauth.Config
	mailbox string
	st      *store.Store
	secrets *keyfile.Key
	log     *slog.Logger
	mail    func(context.Context, mail.Message) error
	baseURL string
	now     func() time.Time

	mu      sync.Mutex
	access  string
	expires time.Time
	// cachedFor is the stored sign-in the access token came from, as
	// stored: once the row says otherwise (a disconnect, or a sign-in made
	// by the command), the token is no longer used.
	cachedFor string
	pending   *pendingSignIn
	problem   string
	// toldKey is whether the admins were told this process can't open the
	// sign-in. It is kept in memory, never in the row, since another
	// process may open it.
	toldKey bool
	// readOnly writes nothing, for mila config check: a refresh's result
	// is used, not kept, and a refusal is said, not marked.
	readOnly bool
}

type pendingSignIn struct {
	code   msauth.Code
	cancel context.CancelFunc
}

// NewMicrosoft keeps the sign-in to mailbox (its address) made through cfg's
// app, sealed with secrets, the data directory's key. sendMail tells the
// instance admins when it stops working; nil tells nobody.
func NewMicrosoft(cfg msauth.Config, mailbox string, st *store.Store, secrets *keyfile.Key, log *slog.Logger, sendMail func(context.Context, mail.Message) error, baseURL string) *Microsoft {
	return &Microsoft{cfg: cfg, mailbox: mailbox, st: st, secrets: secrets, log: log, mail: sendMail, baseURL: baseURL, now: store.Now}
}

// Mailbox is the address of the mailbox Mila signs in to.
func (m *Microsoft) Mailbox() string { return m.mailbox }

// checking is m for CheckMailbox: it writes nothing and tells nobody.
func (m *Microsoft) checking() *Microsoft {
	return &Microsoft{cfg: m.cfg, mailbox: m.mailbox, st: m.st, secrets: m.secrets, log: m.log, now: m.now, readOnly: true}
}

// CheckKey says whether a new sign-in could be sealed here, before anyone
// signs in at Microsoft for it (mila mailbox connect).
func (m *Microsoft) CheckKey() error { return m.secrets.Check() }

func (m *Microsoft) load(ctx context.Context) (msSignIn, bool, error) {
	s, _, ok, err := m.loadRaw(ctx)
	return s, ok, err
}

// loadRaw is load with the row as stored, for replace.
func (m *Microsoft) loadRaw(ctx context.Context) (msSignIn, string, bool, error) {
	raw, err := m.st.GetInstanceSetting(ctx, microsoftKey)
	if errors.Is(err, sql.ErrNoRows) {
		return msSignIn{}, "", false, nil
	}
	if err != nil {
		return msSignIn{}, "", false, err
	}
	var s msSignIn
	if err := json.Unmarshal([]byte(raw), &s); err != nil || s.Refresh == "" {
		return msSignIn{}, raw, false, nil
	}
	// One stored before sealing is read as it is, and sealed when next
	// written, at its next refresh. One that won't open here, for whatever
	// reason the key has (missing, another, a file this process may not
	// read), is unreadable here, and kept as it is.
	s.stored = s.Refresh
	if keyfile.IsSealed(s.Refresh) {
		plain, err := m.secrets.Open(s.Refresh, microsoftKey)
		if err != nil {
			s.Refresh, s.unreadable, s.keyProblem = "", true, keyProblem(err)
		} else {
			s.Refresh = plain
		}
	}
	return s, raw, true, nil
}

// encode is the row for s, its refresh token sealed.
func (m *Microsoft) encode(s msSignIn) (string, error) {
	if s.unreadable {
		s.Refresh = s.stored
	} else {
		sealed, err := m.secrets.Seal(s.Refresh, microsoftKey)
		if err != nil {
			return "", fmt.Errorf("sealing the Microsoft sign-in: %w", err)
		}
		s.Refresh = sealed
	}
	raw, err := json.Marshal(s)
	return string(raw), err
}

// save keeps a new sign-in, replacing whatever was there.
func (m *Microsoft) save(ctx context.Context, s msSignIn) (string, error) {
	raw, err := m.encode(s)
	if err != nil {
		return "", err
	}
	return raw, m.st.UpsertInstanceSetting(context.WithoutCancel(ctx), sqlitedb.UpsertInstanceSettingParams{
		Key: microsoftKey, Value: raw, UpdatedAt: m.now(),
	})
}

// errChanged means the stored sign-in changed while a refresh was out (a
// disconnect, or a new sign-in), so what the refresh brought back is
// dropped. The next check reads the sign-in as it is then.
var errChanged = errors.New("the Microsoft sign-in changed meanwhile: the next check uses it as it is now")

// replace changes the stored sign-in only if it still holds was, so a
// refresh that was out when someone disconnected can't bring it back.
func (m *Microsoft) replace(ctx context.Context, was string, s msSignIn) (string, error) {
	raw, err := m.encode(s)
	if err != nil {
		return "", err
	}
	n, err := m.st.ReplaceInstanceSetting(context.WithoutCancel(ctx), sqlitedb.ReplaceInstanceSettingParams{
		Key: microsoftKey, Value: raw, UpdatedAt: m.now(), Was: was,
	})
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", errChanged
	}
	return raw, nil
}

// State says how the sign-in stands.
func (m *Microsoft) State(ctx context.Context) (MicrosoftState, error) {
	s, ok, err := m.load(ctx)
	if err != nil {
		return MicrosoftState{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := MicrosoftState{State: "not-connected", Problem: m.problem}
	if ok {
		at := s.ConnectedAt
		out.State, out.Account, out.ConnectedAt, out.ConnectedBy = "connected", s.Account, &at, s.ConnectedBy
		switch {
		case s.unreadable:
			// Never marked in the row: it stands until the key opens it.
			out.State, out.LostWhy, out.KeyMissing = "lost", s.keyProblem, true
		case s.LostAt != nil && !s.KeyMissing:
			out.State, out.LostAt, out.LostWhy = "lost", s.LostAt, s.LostWhy
		}
	}
	if p := m.pending; p != nil {
		at := p.code.ExpiresAt
		out.State, out.UserCode, out.VerificationURI, out.ExpiresAt = "signing-in", p.code.UserCode, p.code.VerificationURI, &at
	}
	return out, nil
}

// Connect asks Microsoft for a code and waits, in the background, for someone
// to enter it and sign in; State shows the code meanwhile. by is the admin's
// name.
func (m *Microsoft) Connect(ctx context.Context, by string) (MicrosoftState, error) {
	code, err := m.cfg.Start(ctx)
	if err != nil {
		return MicrosoftState{}, err
	}
	wait, cancel := context.WithDeadline(context.Background(), code.ExpiresAt.Add(time.Minute))
	m.mu.Lock()
	if m.pending != nil {
		m.pending.cancel()
	}
	p := &pendingSignIn{code: code, cancel: cancel}
	m.pending, m.problem = p, ""
	m.mu.Unlock()
	go func() {
		defer cancel()
		tok, err := m.cfg.Wait(wait, code)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.pending != p {
			return // cancelled, or another code asked for since: not kept
		}
		m.pending = nil
		if err == nil {
			err = m.keep(wait, tok, by)
		}
		switch {
		case err == nil:
			m.log.Info("email to task: signed in to the Microsoft mailbox", "account", tok.Account)
		case errors.Is(err, context.Canceled):
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, msauth.ErrExpired):
			m.problem = "The code ran out before anyone signed in. Connect again for a new one."
		default:
			m.problem = err.Error()
		}
	}()
	return m.State(ctx)
}

// ConnectHere is Connect for the command line: it says the code, then waits.
func (m *Microsoft) ConnectHere(ctx context.Context, say func(msauth.Code)) (string, error) {
	code, err := m.cfg.Start(ctx)
	if err != nil {
		return "", err
	}
	say(code)
	wait, cancel := context.WithDeadline(ctx, code.ExpiresAt)
	defer cancel()
	tok, err := m.cfg.Wait(wait, code)
	if errors.Is(err, context.DeadlineExceeded) {
		err = msauth.ErrExpired
	}
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return tok.Account, m.keep(ctx, tok, "")
}

// keep stores a new sign-in, replacing any before it, with m.mu held.
func (m *Microsoft) keep(ctx context.Context, tok msauth.Token, by string) error {
	raw, err := m.save(ctx, msSignIn{Refresh: tok.Refresh, Account: tok.Account, ConnectedAt: m.now(), ConnectedBy: by})
	if err != nil {
		return err
	}
	m.access, m.expires, m.cachedFor = tok.Access, tok.ExpiresAt, raw
	return nil
}

// Cancel stops waiting for a code to be entered.
func (m *Microsoft) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != nil {
		m.pending.cancel()
		m.pending = nil
	}
}

// Disconnect forgets the sign-in. Microsoft still lists Mila among the
// account's apps until removed there.
func (m *Microsoft) Disconnect(ctx context.Context) error {
	m.Cancel()
	m.mu.Lock()
	m.access, m.expires, m.cachedFor, m.problem = "", time.Time{}, "", ""
	m.mu.Unlock()
	return m.st.DeleteInstanceSetting(ctx, microsoftKey)
}

// Token is an access token for IMAP, from the one kept while it lasts and
// otherwise from the refresh token. A sign-in Microsoft no longer takes is
// marked lost, and the instance admins are told, once.
func (m *Microsoft) Token(ctx context.Context) (string, error) {
	// The row first, every time: another process (mila mailbox disconnect
	// or connect) changes it without telling this one.
	s, raw, ok, err := m.loadRaw(ctx)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	if m.access != "" && raw == m.cachedFor && m.now().Before(m.expires.Add(-2*time.Minute)) {
		defer m.mu.Unlock()
		return m.access, nil
	}
	m.access, m.expires, m.cachedFor = "", time.Time{}, ""
	m.mu.Unlock()
	if !ok {
		return "", ErrNotConnected
	}
	if s.unreadable {
		// Not marked lost: a process with the wrong data directory (mila
		// config check, say) must not stop the server that has the key,
		// and the key put back brings it back. The admins are told once
		// by this process.
		m.mu.Lock()
		tell := !m.toldKey && !m.readOnly
		m.toldKey = true
		m.mu.Unlock()
		if tell {
			at := m.now()
			s.LostAt = &at
			m.tellAdmins(ctx, s, true)
		}
		return "", fmt.Errorf("%w: %s", ErrKeyUnreadable, s.keyProblem)
	}
	m.mu.Lock()
	m.toldKey = false
	m.mu.Unlock()
	if s.LostAt != nil && !s.KeyMissing {
		return "", fmt.Errorf("%w: connect it again from Settings, Admin", msauth.ErrLost)
	}
	// Marked lost by a Mila before 3.0 for a key it didn't have, and the
	// key opens it now: it was never lost at Microsoft.
	s.LostAt, s.LostWhy, s.KeyMissing = nil, "", false
	tok, err := m.cfg.Refresh(ctx, s.Refresh, s.Account)
	if errors.Is(err, msauth.ErrLost) {
		if m.readOnly {
			return "", err
		}
		at := m.now()
		s.LostAt, s.LostWhy = &at, err.Error()
		if _, err := m.replace(ctx, raw, s); err != nil {
			return "", err
		}
		m.tellAdmins(ctx, s, false)
		return "", fmt.Errorf("%w: connect it again from Settings, Admin", msauth.ErrLost)
	}
	if err != nil {
		return "", err
	}
	if m.readOnly {
		return tok.Access, nil
	}
	// Microsoft may hand back a new refresh token; the old one may stop
	// working. Either way the row must still be the one refreshed: a
	// disconnect while the refresh was out stays a disconnect.
	s.Refresh, s.Account = tok.Refresh, tok.Account
	m.mu.Lock()
	defer m.mu.Unlock()
	now, err := m.replace(ctx, raw, s)
	if err != nil {
		return "", err
	}
	m.access, m.expires, m.cachedFor = tok.Access, tok.ExpiresAt, now
	return tok.Access, nil
}

// tellAdmins emails every instance admin that email to task has stopped, as a
// failed backup does: once, since the sign-in stays lost until replaced, or
// for a key that won't open it (keyMissing), once a process.
func (m *Microsoft) tellAdmins(ctx context.Context, s msSignIn, keyMissing bool) {
	if m.mail == nil {
		return
	}
	admins, err := m.st.ListInstanceAdmins(ctx)
	if err != nil {
		m.log.Warn("email to task: listing admins to tell", "err", err)
		return
	}
	for _, u := range admins {
		local := s.LostAt.In(accounts.Location(u))
		style := accounts.DateStyleOf(u)
		when := fmt.Sprintf("%s at %s (%s)", style.Day(local, true, local), style.Time(local.Format("15:04")), accounts.Timezone(u))
		var b strings.Builder
		if keyMissing {
			fmt.Fprintf(&b, "On %s, Mila found it can't open its sign-in to the mailbox for email to task (%s), so email to task is paused.\n\n", when, s.Account)
			b.WriteString("The key the sign-in was sealed with (" + keyfile.Name + " in the data directory) is missing, another, or can't be read by Mila. This happens when the database is restored on another server, the key file is lost or its owner changes, or Mila runs with another data directory.\n\n")
			b.WriteString("To start it again, put the key back where Mila can read it, and email to task goes on at its next check. Or open Settings, Admin, choose Connect again, and sign in to the mailbox at Microsoft. Mail that arrives meanwhile waits in the mailbox and is read then.\n")
		} else {
			fmt.Fprintf(&b, "On %s, Microsoft stopped accepting Mila's sign-in to the mailbox for email to task (%s), so email to task is paused.\n\n", when, s.Account)
			b.WriteString("This happens when the account's password changes, an admin revokes the sign-in, or it goes unused for a long time.\n\n")
			b.WriteString("To start it again, open Settings, Admin, choose Connect again, and sign in to the mailbox at Microsoft. Mail that arrives meanwhile waits in the mailbox and is read then.\n")
		}
		if m.baseURL != "" {
			b.WriteString("\n" + strings.TrimSuffix(m.baseURL, "/") + "/settings/admin\n")
		}
		if err := m.mail(ctx, mail.Message{To: u.Email, Subject: "Mila: email to task needs signing in again", Text: b.String()}); err != nil {
			m.log.Warn("email to task: telling an admin the sign-in stopped", "err", err)
		}
	}
}

// xoauth2 is the SASL mechanism Microsoft's IMAP takes a token by.
type xoauth2 struct{ user, token string }

var _ sasl.Client = xoauth2{}

func (x xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}

// Next answers a challenge, which XOAUTH2 sends only to say why it refused,
// with nothing, as the mechanism asks, so the server ends with its error.
func (x xoauth2) Next([]byte) ([]byte, error) { return []byte{}, nil }
