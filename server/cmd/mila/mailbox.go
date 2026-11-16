package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/msauth"
	"github.com/locodarwin/mila/server/internal/store"
)

const mailboxUsage = `usage: mila mailbox connect [settings flags]
       mila mailbox disconnect [settings flags]`

// mailboxRun signs Mila in to, or out of, a Microsoft mailbox for email to
// task (MILA_IMAP_AUTH=microsoft), as Settings, Admin does. Both work with
// the server running: it reads the sign-in at its next check.
func mailboxRun(args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "connect" && args[0] != "disconnect") {
		return errors.New(mailboxUsage)
	}
	cfg, err := config.LoadFromOS("mila mailbox "+args[0], args[1:])
	if err != nil {
		return err
	}
	if cfg.IMAPURL == "" || cfg.IMAPAuth != "microsoft" {
		return errors.New("this server's mailbox takes a password: set imap_auth to microsoft (MILA_IMAP_AUTH=microsoft) to sign in to a Microsoft mailbox")
	}
	ctx, cancel := interrupted()
	defer cancel()
	open, done := context.WithTimeout(ctx, 10*time.Minute) // room for a backup before migrating
	defer done()
	d, err := db.Open(open, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	log := newLogger(cfg)
	if _, err := backup.Prepare(open, d, cfg.BackupDir, log); err != nil {
		return err
	}
	st := store.New(d)
	m := microsoftMailbox(cfg, st, log, nil)
	if args[0] == "disconnect" {
		if err := m.Disconnect(ctx); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Mila is signed out of %s; email to task is paused until someone connects it again.\n", m.Mailbox())
		return nil
	}
	// Before anyone signs in at Microsoft: the sign-in is sealed with the
	// data directory's key, which this must be able to read or make for
	// the server (run as root, it would make one the server can't read).
	if err := m.CheckKey(); err != nil {
		return fmt.Errorf("the sign-in couldn't be kept: %w", err)
	}
	account, err := m.ConnectHere(ctx, func(c msauth.Code) {
		_, _ = fmt.Fprintf(out, "On any device, open %s and enter the code %s,\nthen sign in as %s (or someone with full access to it).\nThe code works until %s.\n\n",
			c.VerificationURI, c.UserCode, m.Mailbox(), c.ExpiresAt.Local().Format("15:04"))
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Signed in as %s. Email to task reads %s from its next check.\n", account, m.Mailbox())
	return nil
}
