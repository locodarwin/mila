package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/keyfile"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/msauth"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
)

// backupConfig is the backup service's settings. Failure emails are
// queued as jobs, which the server sends; with no runner (a command) they
// wait for the server's next look at the jobs table.
func backupConfig(cfg config.Config, st *store.Store, runner *jobs.Runner) backup.Config {
	return backup.Config{
		Dir: cfg.BackupDir, Nightly: cfg.NightlyBackup(), Hour: cfg.BackupAt(), BaseURL: cfg.BaseURL,
		Mail: queueMail(st, runner),
	}
}

// queueMail is how a service sends an email: queued as a job, which the
// server sends; with no runner (a command) it waits for the server's next
// look at the jobs table.
func queueMail(st *store.Store, runner *jobs.Runner) func(context.Context, mail.Message) error {
	return func(ctx context.Context, m mail.Message) error {
		if err := notify.QueueMail(ctx, st, m, store.Now()); err != nil {
			return err
		}
		if runner != nil {
			runner.Wake()
		}
		return nil
	}
}

// microsoftMailbox keeps the sign-in to a Microsoft mailbox for email to
// task, or is nil when the mailbox takes a password.
func microsoftMailbox(cfg config.Config, st *store.Store, log *slog.Logger, runner *jobs.Runner) *inbound.Microsoft {
	if cfg.IMAPURL == "" || cfg.IMAPAuth != "microsoft" {
		return nil
	}
	var mailbox string
	if u, err := url.Parse(cfg.IMAPURL); err == nil && u.User != nil {
		mailbox = u.User.Username()
	}
	return inbound.NewMicrosoft(msauth.Config{
		ClientID: cfg.MicrosoftClientID, Tenant: cfg.MicrosoftTenant, Authority: cfg.MicrosoftAuthority,
	}, mailbox, st, keyfile.At(cfg.DataDir), log, queueMail(st, runner), cfg.BaseURL)
}

// interrupted is a context that ends on Ctrl+C, for the long commands.
func interrupted() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// backupNow backs the database up beside the nightly backups. It may run
// while the server does: both ways of copying read a consistent snapshot.
func backupNow(args []string) error {
	cfg, err := config.LoadFromOS("mila backup now", args)
	if err != nil {
		return err
	}
	ctx, stop := interrupted()
	defer stop()
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	st := store.New(d)
	info, err := backup.New(st, newLogger(cfg), nil, backupConfig(cfg, st, nil)).Run(ctx, backup.Manual)
	if err != nil {
		return err
	}
	fmt.Printf("Backed up to %s (%s)\n", filepath.Join(cfg.BackupDir, info.Name), size(info.Size))
	return nil
}

func backupList(args []string) error {
	cfg, err := config.LoadFromOS("mila backup list", args)
	if err != nil {
		return err
	}
	list, err := backup.List(cfg.BackupDir)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Printf("No backups in %s\n", cfg.BackupDir)
		return nil
	}
	fmt.Printf("Backups in %s, newest first:\n\n", cfg.BackupDir)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FILE\tMADE (UTC)\tWHY\tSIZE")
	for _, b := range list {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Name, b.At.Format("2006-01-02 15:04"), b.Kind, size(b.Size))
	}
	return w.Flush()
}

func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// restore replaces the database with a backup. The file may be named by
// its path or, for one in the backup directory, by its name alone.
func restore(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: mila restore <file> [settings flags]; \"mila backup list\" shows the backups")
	}
	file := args[0]
	cfg, err := config.LoadFromOS("mila restore", args[1:])
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) && filepath.Base(file) == file {
		file = filepath.Join(cfg.BackupDir, file)
	}
	if _, err := os.Stat(file); err != nil {
		return err
	}
	unlock, err := backup.Lock(cfg.DataDir)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, stop := interrupted()
	defer stop()
	safety, err := backup.Restore(ctx, cfg.DatabaseURL, cfg.BackupDir, file, newLogger(cfg))
	if err != nil {
		return err
	}
	fmt.Printf("Restored %s.\n", file)
	if safety != nil {
		fmt.Printf("The database as it was is in %s.\n", filepath.Join(cfg.BackupDir, safety.Name))
	}
	fmt.Println("Attachments are not in backups: if files are missing, restore the attachments folder from your own copy of it.")
	fmt.Println("Start the server again; every device reloads its copy, keeping edits it had not sent.")
	return nil
}

// migrateDB copies the whole instance into another, empty database, for
// moving from SQLite to Postgres or back. The database in use is backed
// up and migrated first, and is left as it was.
func migrateDB(args []string) error {
	var to string
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--to" || a == "-to":
			if i+1 < len(args) {
				to = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--to="), strings.HasPrefix(a, "-to="):
			to = a[strings.Index(a, "=")+1:]
		default:
			rest = append(rest, a)
		}
	}
	if to == "" {
		return errors.New("usage: mila migrate-db --to <sqlite://path or postgres://...> [settings flags]")
	}
	cfg, err := config.LoadFromOS("mila migrate-db", rest)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(to, "sqlite://") && !strings.HasPrefix(to, "postgres://") && !strings.HasPrefix(to, "postgresql://") {
		return fmt.Errorf("--to must start with sqlite://, postgres:// or postgresql://")
	}
	if to == cfg.DatabaseURL {
		return errors.New("--to names the database already in use")
	}
	unlock, err := backup.Lock(cfg.DataDir)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, stop := interrupted()
	defer stop()
	log := newLogger(cfg)

	src, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if _, err := backup.Prepare(ctx, src, cfg.BackupDir, log); err != nil {
		return err
	}
	dst, err := db.Open(ctx, to)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()
	if current, _, err := dst.SchemaVersion(ctx); err != nil {
		return err
	} else if current > 0 {
		if err := dst.CheckEmpty(ctx); err != nil {
			return fmt.Errorf("%w; Mila copies only into an empty database", err)
		}
	}
	if _, err := dst.Migrate(ctx); err != nil {
		return fmt.Errorf("prepare the new database: %w", err)
	}
	start := time.Now()
	n, err := db.Copy(ctx, src, dst, db.CopyOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("Copied %d rows in %s.\n", n, time.Since(start).Round(time.Millisecond))
	fmt.Printf("To run on the new database, set MILA_DATABASE_URL (or --database-url) to %s and start the server.\n", redact(to))
	fmt.Println("The old database is left as it was.")
	return nil
}

// redact hides a URL's password, as config check does.
func redact(raw string) string {
	c := config.Config{DatabaseURL: raw}
	return c.Redacted().DatabaseURL
}
