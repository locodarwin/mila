package inbound

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/locodarwin/mila/server/internal/msauth"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	// perCheck is how much mail one check looks at; the rest waits for the next.
	// Mail not for anyone costs only its headers, so this outpaces any flood.
	perCheck = 500
	// checkTimeout bounds one check, large messages included.
	checkTimeout = 10 * time.Minute
	// slowest is the longest wait after failures. A mailbox that refuses a
	// password would otherwise be asked again every minute, and providers lock
	// accounts for that.
	slowest = 30 * time.Minute
	// positionKey holds how far through the mailbox Mila has read.
	positionKey = "inbound_position"
)

// position is how far through a mailbox Mila has read. Reading from a place of
// its own, and not from what is marked unread, is what lets it change nothing.
type position struct {
	Mailbox     string `json:"mailbox"`
	UIDValidity uint32 `json:"uidValidity"`
	LastUID     uint32 `json:"lastUid"`
}

// Status is how checking the mailbox is going, for the admin's page.
type Status struct {
	LastCheck time.Time
	LastError string
}

// Reader checks a mailbox and delivers what it finds.
type Reader struct {
	svc    *Service
	source *url.URL
	poll   time.Duration
	delete bool
	// ms signs in to a Microsoft mailbox, which takes no password; nil for
	// a password in the URL.
	ms *Microsoft

	mu     sync.Mutex
	status Status
}

// NewReader makes a reader of the mailbox the server's settings name. ms is
// how it signs in to a Microsoft mailbox, or nil for the URL's password.
func NewReader(svc *Service, imapURL string, poll time.Duration, deleteAfter bool, ms *Microsoft) (*Reader, error) {
	u, err := url.Parse(imapURL)
	if err != nil {
		return nil, errors.New("imap_url is not a URL")
	}
	return &Reader{svc: svc, source: u, poll: poll, delete: deleteAfter, ms: ms}, nil
}

// Status says how the last check went.
func (r *Reader) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Run checks the mailbox until ctx ends, more slowly while it keeps failing.
func (r *Reader) Run(ctx context.Context) {
	wait := r.poll
	lastPrune := time.Time{}
	for ctx.Err() == nil {
		err := r.safeCheck(ctx)
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		r.status.LastCheck = r.svc.now()
		was := r.status.LastError
		r.status.LastError = ""
		if err != nil {
			r.status.LastError = err.Error()
		}
		r.mu.Unlock()
		if err != nil {
			if was != err.Error() {
				r.svc.log.Warn("email to task: checking the mailbox failed", "err", err)
			}
			if wait = min(wait*2, slowest); wait < r.poll {
				wait = r.poll
			}
			// Waiting for someone to sign in to Microsoft asks the mailbox
			// nothing, so it slows nothing down, and starts soon after.
			if errors.Is(err, ErrNotConnected) || errors.Is(err, msauth.ErrLost) || errors.Is(err, ErrKeyUnreadable) {
				wait = min(r.poll, 15*time.Second)
			}
		} else {
			wait = r.poll
		}
		if time.Since(lastPrune) > 24*time.Hour {
			lastPrune = time.Now()
			if err := r.svc.Prune(ctx); err != nil {
				r.svc.log.Warn("email to task: forgetting old mail", "err", err)
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
}

// safeCheck keeps a panic in reading what a stranger sent from ending the server.
func (r *Reader) safeCheck(ctx context.Context) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.svc.log.Error("email to task: reading the mailbox panicked", "panic", p)
			err = errors.New("reading the mailbox failed unexpectedly")
		}
	}()
	return r.check(ctx)
}

func (r *Reader) check(ctx context.Context) error {
	if r.source.Scheme == "file" {
		return r.checkFolder(ctx)
	}
	return r.checkIMAP(ctx)
}

// checkFolder reads .eml files from a folder, for tests and trying it out.
func (r *Reader) checkFolder(ctx context.Context) error {
	names, err := filepath.Glob(filepath.Join(folderOf(r.source), "*.eml"))
	if err != nil {
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		if ctx.Err() != nil {
			return nil
		}
		named, err := r.deliverFile(ctx, name)
		_ = named
		if err != nil {
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reader) deliverFile(ctx context.Context, name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	head, err := io.ReadAll(io.LimitReader(f, maxHeaderRead))
	if err != nil {
		return false, err
	}
	return r.deliverParts(ctx, head, info.Size(), func() ([]byte, error) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(f, maxMessage+1))
	})
}

// deliverParts deals with one message: its headers first, and only if they pass,
// all of it. A message that can't be read is passed over, since it would otherwise
// stop everything after it for good.
func (r *Reader) deliverParts(ctx context.Context, head []byte, size int64, body func() ([]byte, error)) (named bool, err error) {
	defer func() {
		if p := recover(); p != nil {
			r.svc.log.Error("email to task: a message could not be read", "panic", p)
			named, err = false, nil
		}
	}()
	headers, err := parseHeaders(head, size)
	if err != nil {
		r.svc.log.Warn("email to task: a message could not be read")
		return false, nil
	}
	ok, named, err := r.svc.Screen(ctx, headers)
	if ok == nil || err != nil {
		return named, err
	}
	raw, err := body()
	if err != nil {
		return named, err
	}
	if checkStructure(raw) != nil {
		return named, r.svc.refuse(ctx, ok.addr, headers, RefusedTooLarge)
	}
	m, err := Parse(bytes.NewReader(raw))
	if err != nil {
		return named, r.svc.refuse(ctx, ok.addr, headers, Failed)
	}
	m.Size = size
	return named, r.svc.accept(ctx, ok, m)
}

// connect signs in. Its errors never name the password; only the host is added.
func (r *Reader) connect(ctx context.Context) (*imapclient.Client, func(), error) {
	host := r.source.Host
	query := r.source.Query()
	if r.source.Port() == "" {
		if r.source.Scheme == "imaps" {
			host = net.JoinHostPort(r.source.Hostname(), "993")
		} else {
			host = net.JoinHostPort(r.source.Hostname(), "143")
		}
	}
	dialer := net.Dialer{Timeout: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, fmt.Errorf("could not reach %s", host)
	}
	_ = conn.SetDeadline(time.Now().Add(checkTimeout))
	// A server stopping must not wait out a slow mailbox, or a silent one.
	raw := conn
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	fail := func(err error) (*imapclient.Client, func(), error) {
		stop()
		_ = raw.Close()
		return nil, nil, err
	}
	config := &tls.Config{ServerName: r.source.Hostname(), MinVersion: tls.VersionTLS12}
	// For a server of one's own with a certificate no authority signed, as a
	// local bridge has; the setting says so in as many words.
	config.InsecureSkipVerify = query.Get("verify") == "off"
	options := &imapclient.Options{TLSConfig: config}
	var client *imapclient.Client
	switch {
	case r.source.Scheme == "imaps":
		tlsConn := tls.Client(conn, config)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fail(fmt.Errorf("could not set up TLS with %s: %w", host, err))
		}
		conn = tlsConn
		client = imapclient.New(conn, options)
	case query.Get("tls") == "none":
		client = imapclient.New(conn, options)
	default:
		if client, err = imapclient.NewStartTLS(conn, options); err != nil {
			return fail(fmt.Errorf("could not start TLS with %s: %w", host, err))
		}
	}
	if r.ms != nil {
		token, err := r.ms.Token(ctx)
		if err != nil {
			_ = client.Close()
			return fail(err)
		}
		if err := client.Authenticate(xoauth2{user: r.source.User.Username(), token: token}); err != nil {
			_ = client.Close()
			return fail(fmt.Errorf("%s refused Microsoft's sign-in for %s; check that the account signed in can open that mailbox, and that IMAP is on for it", host, r.source.User.Username()))
		}
		return client, func() { stop() }, nil
	}
	password, _ := r.source.User.Password()
	if err := client.Login(r.source.User.Username(), password).Wait(); err != nil {
		_ = client.Close()
		return fail(fmt.Errorf("%s refused the sign-in; check the user and the password, or the app password", host))
	}
	return client, func() { stop() }, nil
}

// CheckMailbox signs in to the mailbox imap_url names and opens it to read
// only, reading nothing (mila config check). It says what it found.
func CheckMailbox(ctx context.Context, imapURL string, ms *Microsoft) (string, error) {
	u, err := url.Parse(imapURL)
	if err != nil {
		return "", errors.New("imap_url is not a URL")
	}
	if u.Scheme == "file" {
		dir := folderOf(u)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("%s is not a folder", dir)
		}
		return "reads .eml files from " + dir + " (for tests)", nil
	}
	// It writes nothing: a refreshed sign-in is used, not kept, and one
	// that fails is said, not marked, since this may not be the server.
	r := &Reader{source: u}
	if ms != nil {
		r.ms = ms.checking()
	}
	client, stop, err := r.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		stop()
		_ = client.Logout().Wait()
		_ = client.Close()
	}()
	name := r.mailboxName()
	selected, err := client.Select(name, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return "", fmt.Errorf("signed in to %s, but could not open the mailbox %q", u.Host, name)
	}
	return fmt.Sprintf("signed in to %s and opened %s, with %d messages; nothing was read", u.Host, name, selected.NumMessages), nil
}

// folderOf is the folder a file:// URL names, on Windows too, where
// file:///C:/mail has the path /C:/mail.
func folderOf(u *url.URL) string {
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func (r *Reader) mailboxName() string {
	name := strings.TrimPrefix(r.source.Path, "/")
	if name == "" {
		return "INBOX"
	}
	return name
}

func (r *Reader) checkIMAP(ctx context.Context) error {
	client, stop, err := r.connect(ctx)
	if err != nil {
		return err
	}
	defer func() {
		stop()
		_ = client.Logout().Wait()
		_ = client.Close()
	}()

	name := r.mailboxName()
	// Opened to read only, unless told to delete, so nothing can change it.
	selected, err := client.Select(name, &imap.SelectOptions{ReadOnly: !r.delete}).Wait()
	if err != nil {
		return fmt.Errorf("could not open the mailbox %q", name)
	}
	at, known, err := r.loadPosition(ctx)
	if err != nil {
		return err
	}
	if selected.UIDNext == 0 {
		// Without it there is no telling old mail from new, and a guess either
		// skips all of it or makes tasks of the whole mailbox.
		return fmt.Errorf("the mailbox %q did not say what comes next", name)
	}
	if !known || at.Mailbox != name || at.UIDValidity != selected.UIDValidity {
		// Starting out, or a mailbox rebuilt: what is there already stays
		// unread by Mila, rather than old mail all turning into tasks.
		if known {
			r.svc.log.Warn("email to task: the mailbox changed, so starting from its newest mail", "mailbox", name)
		}
		at = position{Mailbox: name, UIDValidity: selected.UIDValidity, LastUID: uint32(selected.UIDNext) - 1}
		return r.savePosition(ctx, at)
	}
	if selected.NumMessages == 0 {
		return nil
	}
	found, err := client.UIDSearch(&imap.SearchCriteria{
		UID: []imap.UIDSet{{imap.UIDRange{Start: imap.UID(at.LastUID + 1), Stop: 0}}},
	}, nil).Wait()
	if err != nil {
		return errors.New("could not search the mailbox")
	}
	uids := found.AllUIDs()
	slices.Sort(uids)
	done := 0
	for _, uid := range uids {
		// A range to * also answers with the newest message, even if already read.
		if uint32(uid) <= at.LastUID {
			continue
		}
		if done == perCheck || ctx.Err() != nil {
			break
		}
		done++
		named, err := r.deliverUID(ctx, client, uid)
		if err != nil {
			return err
		}
		at.LastUID = uint32(uid)
		if err := r.savePosition(ctx, at); err != nil {
			return err
		}
		if named && r.delete {
			r.remove(client, uid)
		}
	}
	return nil
}

// deliverUID deals with one message: fetching its headers, and all of it only if
// they pass, so a stranger's mail costs next to nothing however large.
func (r *Reader) deliverUID(ctx context.Context, client *imapclient.Client, uid imap.UID) (bool, error) {
	set := imap.UIDSetNum(uid)
	section := &imap.FetchItemBodySection{Peek: true, Specifier: imap.PartSpecifierHeader, Partial: &imap.SectionPartial{Size: maxHeaderRead}}
	first, err := client.Fetch(set, &imap.FetchOptions{UID: true, RFC822Size: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return false, errors.New("could not read from the mailbox")
	}
	if len(first) == 0 {
		return false, nil // Gone since the search.
	}
	return r.deliverParts(ctx, first[0].FindBodySection(section), first[0].RFC822Size, func() ([]byte, error) {
		whole := &imap.FetchItemBodySection{Peek: true}
		fetched, err := client.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{whole}}).Collect()
		if err != nil {
			return nil, errors.New("could not read from the mailbox")
		}
		if len(fetched) == 0 {
			return nil, errors.New("a message went while being read")
		}
		return fetched[0].FindBodySection(whole), nil
	})
}

// remove deletes a message dealt with, and only that one. Without UIDPLUS the
// only way to finish would also delete whatever else was marked for deletion,
// so it stays.
func (r *Reader) remove(client *imapclient.Client, uid imap.UID) {
	if !client.Caps().Has(imap.CapUIDPlus) {
		r.svc.log.Warn("email to task: this mail server cannot delete one message alone, so mail is being kept")
		return
	}
	set := imap.UIDSetNum(uid)
	if err := client.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		r.svc.log.Warn("email to task: could not delete a message")
		return
	}
	if err := client.UIDExpunge(set).Close(); err != nil {
		r.svc.log.Warn("email to task: could not delete a message")
	}
}

func (r *Reader) loadPosition(ctx context.Context) (position, bool, error) {
	var at position
	raw, err := r.svc.st.GetInstanceSetting(ctx, positionKey)
	if errors.Is(err, sql.ErrNoRows) {
		return at, false, nil
	}
	if err != nil {
		return at, false, err
	}
	if err := json.Unmarshal([]byte(raw), &at); err != nil {
		return at, false, nil
	}
	return at, true, nil
}

func (r *Reader) savePosition(ctx context.Context, at position) error {
	raw, err := json.Marshal(at)
	if err != nil {
		return err
	}
	return r.svc.st.UpsertInstanceSetting(context.WithoutCancel(ctx), sqlitedb.UpsertInstanceSettingParams{
		Key: positionKey, Value: string(raw), UpdatedAt: r.svc.now(),
	})
}
