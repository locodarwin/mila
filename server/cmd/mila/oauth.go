package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
)

const oauthUsage = `usage: mila oauth list [settings flags]
       mila oauth delete <app ID>... [settings flags]`

// oauthRun lists or deletes the apps registered by OAuth. Both work with the
// server running, and with OAuth off.
func oauthRun(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(oauthUsage)
	}
	switch args[0] {
	case "list":
		return oauthList(args[1:], out)
	case "delete":
		return oauthDelete(args[1:], out)
	}
	return errors.New(oauthUsage)
}

// withAccounts opens the database the settings name, migrated as the server
// would, and runs fn with the accounts service on it.
func withAccounts(name string, args []string, fn func(context.Context, *accounts.Service) error) error {
	cfg, err := config.LoadFromOS(name, args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute) // room for a backup before migrating
	defer cancel()
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	log := newLogger(cfg)
	if _, err := backup.Prepare(ctx, d, cfg.BackupDir, log); err != nil {
		return err
	}
	sender, err := mail.New(cfg.SMTPURL, cfg.MailFrom, log)
	if err != nil {
		return err
	}
	acc, err := newAccounts(cfg, store.New(d), sender, log)
	if err != nil {
		return err
	}
	return fn(ctx, acc)
}

func oauthList(args []string, out io.Writer) error {
	return withAccounts("mila oauth list", args, func(ctx context.Context, acc *accounts.Service) error {
		apps, err := acc.OAuthApps(ctx)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			_, err := fmt.Fprintln(out, "No apps are registered.")
			return err
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tNAME\tREGISTERED (UTC)\tLAST SAID YES TO\tTOKENS")
		for _, a := range apps {
			yes := "never"
			if !a.LastYes.IsZero() {
				yes = a.LastYes.UTC().Format("2006-01-02 15:04")
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", a.ID, printable(a.Name), a.Registered.UTC().Format("2006-01-02 15:04"), yes, a.Tokens)
		}
		return w.Flush()
	})
}

func oauthDelete(args []string, out io.Writer) error {
	var ids []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		ids, args = append(ids, args[0]), args[1:]
	}
	if len(ids) == 0 {
		return errors.New(oauthUsage + "\n\"mila oauth list\" shows the apps and their IDs")
	}
	return withAccounts("mila oauth delete", args, func(ctx context.Context, acc *accounts.Service) error {
		for _, id := range ids {
			app, people, err := acc.DeleteOAuthClient(ctx, id)
			if err != nil {
				return fmt.Errorf("%s: %w", id, err)
			}
			if app.Tokens == 0 {
				_, _ = fmt.Fprintf(out, "Deleted the app %q (%s); it held no working tokens.\n", printable(app.Name), app.ID)
				continue
			}
			names := make([]string, len(people))
			for i, u := range people {
				names[i] = fmt.Sprintf("%s <%s>", printable(u.Name), printable(u.Email))
			}
			_, _ = fmt.Fprintf(out, "Deleted the app %q (%s), ending its %s, held by %s.\n", printable(app.Name), app.ID, plural(app.Tokens, "token"), strings.Join(names, ", "))
		}
		return nil
	})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
