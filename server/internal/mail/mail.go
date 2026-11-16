// Package mail sends Mila's email: sign-in links and invites. It speaks
// SMTP, can write messages to a directory for tests, and without either
// writes them to the server log so a self-hoster is never locked out.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Message is a plain-text email.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// New returns the sender for rawURL:
//
//	smtp://user:pass@host:587    SMTP, upgraded with STARTTLS when offered
//	smtps://user:pass@host:465   SMTP over TLS from the start
//	file:///path/to/dir          one .eml file per message (tests)
//	empty                        the server log
func New(rawURL, from string, log *slog.Logger) (Sender, error) {
	if rawURL == "" {
		return logSender{log}, nil
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, fmt.Errorf("mail from address %q: %w", from, err)
	}
	if dir, ok := strings.CutPrefix(rawURL, "file://"); ok {
		dir = fileDir(dir)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
		return fileSender{dir: dir, from: from}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// Not err itself: it quotes the URL, password and all, into the log.
		return nil, errors.New("mail URL: not a valid URL; characters such as @, : or / in the user name or password must be percent-encoded")
	}
	switch u.Scheme {
	case "smtp", "smtps":
		if u.Hostname() == "" {
			return nil, errors.New("mail URL: no host")
		}
		return &smtpSender{u: u, from: from}, nil
	}
	return nil, fmt.Errorf("mail URL: unsupported scheme %q", u.Scheme)
}

// plainAllowed says mail may go without TLS: to a server on this computer,
// or where the URL says so (?tls=none), for a relay on a network trusted.
func plainAllowed(u *url.URL) bool {
	if u.Query().Get("tls") == "none" {
		return true
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

// fileDir is the directory a file:// mail URL names. It is read as a path
// rather than parsed as a URL, so a Windows one works either way it is
// written: file://C:\mail or file:///C:/mail.
func fileDir(p string) string {
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// render produces the message as RFC 5322 text.
func render(from string, m Message) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := "mila"
	if a, err := mail.ParseAddress(from); err == nil {
		if at := strings.LastIndex(a.Address, "@"); at >= 0 {
			domain = a.Address[at+1:]
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id), domain)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(crlf(m.Text))
	return []byte(b.String())
}

// crlf ends every line with CRLF, whatever ended it: a bare CR in text
// people wrote (a task's title, a comment) reaching SMTP as it is would
// let a lenient server read "\r.\r\n" as the message's end, and what
// follows as another message from Mila.
func crlf(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

type logSender struct{ log *slog.Logger }

func (s logSender) Send(_ context.Context, m Message) error {
	s.log.Warn("no email configured; message written to the log instead",
		"to", m.To, "subject", m.Subject, "body", m.Text)
	return nil
}

type fileSender struct{ dir, from string }

func (s fileSender) Send(_ context.Context, m Message) error {
	name := fmt.Sprintf("%d-%s.eml", time.Now().UnixNano(), strings.NewReplacer("@", "_at_", "/", "_").Replace(m.To))
	tmp := filepath.Join(s.dir, "."+name)
	if err := os.WriteFile(tmp, render(s.from, m), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, name))
}

type smtpSender struct {
	u    *url.URL
	from string
}

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	c, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	fromAddr, _ := mail.ParseAddress(s.from)
	if err := c.Mail(fromAddr.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(render(s.from, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// The server accepted the message when DATA ended. A QUIT that fails
	// after that (a dropped connection, the deadline) is no reason to send
	// it again.
	_ = c.Quit()
	return nil
}

// open connects to the server, with TLS, and signs in: everything sending
// does before a message.
func (s *smtpSender) open(ctx context.Context) (*smtp.Client, error) {
	port := s.u.Port()
	if port == "" {
		port = map[string]string{"smtp": "587", "smtps": "465"}[s.u.Scheme]
	}
	host := s.u.Hostname()
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 15 * time.Second}

	var conn net.Conn
	var err error
	if s.u.Scheme == "smtps" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(time.Minute))
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	fail := func(err error) (*smtp.Client, error) {
		_ = c.Close()
		return nil, err
	}

	if s.u.Scheme == "smtp" {
		ok, _ := c.Extension("STARTTLS")
		switch {
		case ok:
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fail(fmt.Errorf("smtp starttls: %w", err))
			}
		case !plainAllowed(s.u):
			// Someone between here and the server can take STARTTLS out of
			// its answer, and read every sign-in link sent after.
			return fail(errors.New("smtp: the server offers no STARTTLS; use smtps://, or add ?tls=none to the URL to send in plain text on a network you trust"))
		}
	}
	if user := s.u.User; user != nil {
		pass, _ := user.Password()
		// net/smtp refuses PLAIN auth over an unencrypted connection.
		if err := c.Auth(smtp.PlainAuth("", user.Username(), pass, host)); err != nil {
			return fail(fmt.Errorf("smtp auth: %w", err))
		}
	}
	return c, nil
}

// Check does what sending does up to a message, and sends none: it
// connects, sets up TLS and signs in (mila config check). ok says email is
// set up; what says what was found, or what is missing.
func Check(ctx context.Context, rawURL, from string) (ok bool, what string, err error) {
	s, err := New(rawURL, from, slog.New(slog.DiscardHandler))
	if err != nil {
		return false, "", err
	}
	switch s := s.(type) {
	case logSender:
		return false, "not set up: sign-in links, invitations and reminders are printed in the log; smtp_url sends them", nil
	case fileSender:
		return true, "written to files in " + s.dir + " (for tests)", nil
	case *smtpSender:
		c, err := s.open(ctx)
		if err != nil {
			return false, "", err
		}
		_ = c.Quit()
		_ = c.Close()
		if s.u.User == nil {
			return true, "connected to " + s.u.Host + ", which asks no sign-in; nothing was sent", nil
		}
		// Not the user name: some services (Postmark) take the secret token
		// as both, which is why the settings hide it too.
		return true, "signed in to " + s.u.Host + "; nothing was sent", nil
	}
	return false, "", errors.New("mail: unknown sender")
}
