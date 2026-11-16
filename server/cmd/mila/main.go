// Command mila is the whole Mila server: the web app, the API, sync and
// background jobs, in one binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"golang.org/x/crypto/acme/autocert"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/actions"
	"github.com/locodarwin/mila/server/internal/automations"
	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/config"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/httpapi"
	"github.com/locodarwin/mila/server/internal/ical"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/ownership"
	"github.com/locodarwin/mila/server/internal/push"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/updates"
	"github.com/locodarwin/mila/server/internal/version"
	"github.com/locodarwin/mila/server/internal/webhooks"
	"github.com/locodarwin/mila/server/internal/webui"
)

const usage = `Usage: mila <command> [flags]

Commands:
  serve                     run the server
  config check              validate the settings, connect to the database, sign in to the
                            email server and mailbox, and check the address reaches Mila
  user login-link <email>   print a one-time sign-in link for a user
  user make-admin <email>   make a user an admin of this Mila
  oauth list                list the apps registered by OAuth
  oauth delete <app ID>...  delete apps, ending their tokens
  mailbox connect           sign Mila in to a Microsoft mailbox for email to task
  mailbox disconnect        sign Mila out of it
  backup now                back up the database (the server may be running)
  backup list               list the backups
  restore <file>            replace the database with a backup (stop the server first)
  migrate-db --to <url>     copy the whole instance into another database
                            (sqlite://<path> or postgres://...; stop the server first)
  dev generate --email <a>  fill a new workspace with made-up work for the
                            performance budgets (--tasks, --board-cards, --seed...)
  mcp                       serve Mila's tools to an agent over MCP, on stdin
                            and stdout, with an API token (see "mila mcp -h")
  api <command>             use a Mila server's API from a shell with an API
                            token (see "mila api help")
  queue -- <command>        run a command for each task assigned to the
                            token's person (see "mila queue -h")
  version                   print the version

Run "mila serve -h" for the settings each command accepts.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mila:", errorLine(err))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command given")
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "config":
		if len(args) < 2 || args[1] != "check" {
			return errors.New(`unknown config command; try "mila config check"`)
		}
		return configCheck(args[2:])
	case "user":
		if len(args) >= 2 && args[1] == "login-link" {
			return loginLink(args[2:])
		}
		if len(args) >= 2 && args[1] == "make-admin" {
			return makeAdmin(args[2:])
		}
		return errors.New(`unknown user command; try "mila user login-link <email>" or "mila user make-admin <email>"`)
	case "oauth":
		return oauthRun(args[1:], os.Stdout)
	case "mailbox":
		return mailboxRun(args[1:], os.Stdout)
	case "backup":
		if len(args) < 2 {
			return errors.New(`unknown backup command; try "mila backup now" or "mila backup list"`)
		}
		switch args[1] {
		case "now":
			return backupNow(args[2:])
		case "list":
			return backupList(args[2:])
		}
		return errors.New(`unknown backup command; try "mila backup now" or "mila backup list"`)
	case "restore":
		return restore(args[1:])
	case "migrate-db":
		return migrateDB(args[1:])
	case "dev":
		if len(args) < 2 || args[1] != "generate" {
			return errors.New(`unknown dev command; try "mila dev generate --email <address>"`)
		}
		return devGenerate(args[2:])
	case "mcp":
		return mcpServe(args[1:])
	case "api":
		return apiRun(args[1:], os.Stdin, os.Stdout)
	case "queue":
		return queueRun(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("mila %s (%s)\n", version.Version, version.Commit)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo // unreachable: config validates the level
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

func serve(args []string) error {
	cfg, err := config.LoadFromOS("mila serve", args)
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	log.Info("starting", "version", version.Version, "commit", version.Commit)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.ExitWithStdin == "on" {
		// However the app that started this ended, its end of the pipe closes.
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			log.Info("standard input closed, stopping")
			stop()
		}()
	}

	unlock, err := backup.Lock(cfg.DataDir)
	if err != nil {
		return err
	}
	defer unlock()
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	applied, err := backup.Prepare(ctx, d, cfg.BackupDir, log)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if applied > 0 {
		log.Info("migrated database", "dialect", d.Dialect, "applied", applied)
	}

	st := store.New(d)
	instanceID, err := st.InitInstanceSetting(ctx, "instance_id", uuid.Must(uuid.NewV7()).String())
	if err != nil {
		return fmt.Errorf("instance id: %w", err)
	}
	sender, err := mail.New(cfg.SMTPURL, cfg.MailFrom, log)
	if err != nil {
		return err
	}
	acc, err := newAccounts(cfg, st, sender, log)
	if err != nil {
		return err
	}
	if cfg.Local == "on" {
		if err := acc.EnsureLocalOwner(ctx); err != nil {
			return fmt.Errorf("the local owner: %w", err)
		}
	} else if link, err := acc.SetupLink(ctx); err != nil {
		return fmt.Errorf("setup link: %w", err)
	} else if link != "" {
		log.Warn("Mila has no users yet. Open this one-time link to create the first account: " + link)
	}

	hub := syncer.NewHub(st, log)
	acc.SetEvents(hub)
	syncSvc := syncer.New(st, log, hub)
	syncer.SetAppAddress(cfg.BaseURL)
	hub.SetDocs(syncSvc)
	attachments, err := files.New(filepath.Join(cfg.DataDir, "attachments"))
	if err != nil {
		return fmt.Errorf("attachments: %w", err)
	}
	runner := jobs.New(st, log, nil)
	notifier := notify.New(st, log, runner, sender, syncSvc, hub, notify.Config{BaseURL: cfg.BaseURL, Email: cfg.SMTPURL != ""})
	hooks := webhooks.New(st, log, runner, notifier, webhooks.Options{BaseURL: cfg.BaseURL, AllowPrivate: cfg.WebhookAllowPrivate == "on"})
	rules := automations.New(st, log, syncSvc, runner, notifier)
	syncSvc.SetWatcher(syncer.Watchers{notifier, hooks, rules})
	acc.SetNotices(notifier)
	pusher, err := push.New(ctx, st, log, runner, push.Options{Subject: push.Subject(cfg.MailFrom, cfg.BaseURL)})
	if err != nil {
		return err
	}
	notifier.SetPusher(pusher)
	acc.SetContent(syncSvc)
	data, err := ownership.New(st, log, syncSvc, attachments, runner, notifier, hub,
		ownership.Config{DataDir: cfg.DataDir, ImportMax: cfg.ImportMax()})
	if err != nil {
		return fmt.Errorf("exports: %w", err)
	}
	backups := backup.New(st, log, runner, backupConfig(cfg, st, runner))
	// Links written before the server listed them, listed before the
	// first rename can need them.
	if n, err := syncSvc.IndexLinks(ctx); err != nil {
		return fmt.Errorf("listing links: %w", err)
	} else if n > 0 {
		log.Info("listed the links in notes and pages", "documents", n)
	}
	// Notes from before live documents get their first document, once.
	go func() {
		if n, err := syncSvc.SeedDocs(ctx, ""); err != nil {
			log.Error("making documents of notes", "err", err, "made", n)
		} else if n > 0 {
			log.Info("made documents of notes", "tasks", n)
		}
	}()
	// Email to task: the mailbox is read beside the jobs, since a large message
	// must not hold them up, and stops with them.
	var emailReader *inbound.Reader
	microsoft := microsoftMailbox(cfg, st, log, runner)
	if cfg.IMAPURL != "" {
		emailReader, err = inbound.NewReader(inbound.New(st, log, syncSvc, attachments, notifier, cfg.InboundAddress), cfg.IMAPURL, cfg.IMAPPollEvery(), cfg.IMAPAfter == "delete", microsoft)
		if err != nil {
			return err
		}
	}
	// Whether a newer Mila is out, asked of GitHub once a day.
	var newerRelease func() *updates.Release
	if cfg.ChecksForUpdates() {
		checker := updates.New(version.Version, log)
		newerRelease = checker.Newer
		go checker.Run(ctx)
	}
	emailDone := make(chan struct{})
	go func() {
		defer close(emailDone)
		if emailReader != nil {
			emailReader.Run(ctx)
		}
	}()
	runner.Every("maintenance", time.Hour, func(ctx context.Context) error {
		maintain(ctx, log, syncSvc, acc, attachments, runner.Now())
		if err := rules.Tidy(ctx, runner.Now()); err != nil {
			log.Warn("tidy automation runs", "err", err)
		}
		return nil
	})
	// Jobs stop with the server, which waits for the ones under way to
	// record how they went before closing the database.
	jobsDone := make(chan struct{})
	go func() {
		defer close(jobsDone)
		runner.Run(ctx)
	}()

	proxies, _ := cfg.ProxyNets() // validated by config
	handler, err := httpapi.New(httpapi.Deps{
		Log:             log,
		DB:              d,
		Web:             webui.FS(),
		Accounts:        acc,
		Sync:            syncSvc,
		Hub:             hub,
		Files:           attachments,
		Notify:          notifier,
		Push:            pusher,
		Calendar:        ical.New(st, syncSvc, log, cfg.BaseURL),
		Data:            data,
		Webhooks:        hooks,
		Backup:          backups,
		Schema:          d,
		Dialect:         string(d.Dialect),
		MailMode:        mailMode(cfg.SMTPURL),
		LocalKey:        localKeyOf(cfg),
		LocalWatchKey:   localWatchKeyOf(cfg),
		InboundAddress:  cfg.InboundAddress,
		InstanceID:      instanceID,
		Updates:         newerRelease,
		Automations:     rules,
		Actions:         actions.New(st, syncSvc, acc),
		EmailIn:         emailInStatus(emailReader),
		Microsoft:       microsoftDeps(microsoft),
		BaseURL:         cfg.BaseURL,
		Origin:          cfg.Origin(),
		HTTPS:           cfg.HTTPS(),
		TrustedProxies:  proxies,
		SignInPerMinute: cfg.SignInPerMinute(),
		InvitesPerHour:  cfg.InvitesPerHour(),
		// The desktop app's mode makes no tokens, so it lets no app connect.
		OAuth: cfg.OAuth == "on" && localKeyOf(cfg) == "",
	})
	if err != nil {
		return err
	}
	// The servers: one on listen, or with Mila's own HTTPS, one serving it
	// and one of plain HTTP for the certificate's checks and redirects.
	type server struct {
		srv *http.Server
		tls bool
	}
	var servers []server
	var certs *autocert.Manager
	if cfg.OwnHTTPS() {
		m := certManager(cfg)
		certs = m
		errLog := serverErrors(log)
		servers = []server{
			{&http.Server{
				Addr:              cfg.TLSListen,
				Handler:           handler,
				TLSConfig:         tlsConfig(m),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
				ErrorLog:          errLog,
			}, true},
			{&http.Server{
				Addr:              cfg.TLSHTTPListen,
				Handler:           m.HTTPHandler(toHTTPS(cfg.BaseURL)),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       30 * time.Second,
				ErrorLog:          errLog,
			}, false},
		}
	} else {
		servers = []server{{&http.Server{
			Addr:              cfg.Listen,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}, false}}
	}

	// Bound before saying so, so a port that cannot be had is the error
	// and nothing claims Mila is listening.
	listeners := make([]net.Listener, len(servers))
	for i, s := range servers {
		ln, err := net.Listen("tcp", s.srv.Addr)
		if err != nil {
			for _, l := range listeners[:i] {
				_ = l.Close()
			}
			return err
		}
		listeners[i] = ln
	}
	errc := make(chan error, len(servers))
	for i, s := range servers {
		go func() {
			if s.tls {
				errc <- s.srv.ServeTLS(listeners[i], "", "")
			} else {
				errc <- s.srv.Serve(listeners[i])
			}
		}()
	}
	if certs != nil {
		go warmCertificate(log, certs, cfg.TLSDomain)
		log.Info("listening", "https", cfg.TLSListen, "http", cfg.TLSHTTPListen, "domain", cfg.TLSDomain,
			"base_url", cfg.BaseURL, "database", d.Dialect, "instance_id", instanceID)
	} else {
		log.Info("listening", "addr", cfg.Listen, "base_url", cfg.BaseURL,
			"database", d.Dialect, "instance_id", instanceID)
	}

	select {
	case err := <-errc:
		for _, s := range servers {
			_ = s.srv.Close()
		}
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		if e := s.srv.Shutdown(shutdownCtx); e != nil && err == nil {
			err = e
		}
	}
	select {
	case <-jobsDone:
	case <-shutdownCtx.Done():
		log.Warn("jobs still running at shutdown")
	}
	select {
	case <-emailDone:
	case <-shutdownCtx.Done():
		log.Warn("still reading the mailbox at shutdown")
	}
	return err
}

// maintain is the hourly upkeep the job runner does: deleted items past
// the trash period are purged, attachment files no row names any more
// are removed, the sync log is compacted, and sign-in records past their
// use (ended sessions, used links, old security events) are removed.
func maintain(ctx context.Context, log *slog.Logger, s *syncer.Service, acc *accounts.Service, store *files.Store, now time.Time) {
	if n, err := acc.Tidy(ctx, now); err != nil {
		log.Warn("tidy sign-in records", "err", err)
	} else if n != (accounts.Tidied{}) {
		log.Info("removed sign-in records past their use", "sessions", n.Sessions, "links", n.MagicLinks,
			"email_changes", n.EmailChanges, "challenges", n.Challenges, "invites", n.Invites, "security_events", n.Events, "devices", n.Devices)
	}
	if n, err := s.Compact(ctx, now); err != nil {
		log.Warn("compact the sync log", "err", err)
	} else if n.Changes > 0 || n.Ops > 0 || n.Batches > 0 {
		log.Info("compacted the sync log", "changes", n.Changes, "ops", n.Ops, "batches", n.Batches)
	}
	if n, err := s.Purge(ctx); err != nil {
		log.Warn("purge", "err", err)
	} else if n > 0 {
		log.Info("purged deleted items past the trash period", "rows", n)
	}
	if keep, err := s.StorageKeys(ctx); err != nil {
		log.Warn("list attachment files", "err", err)
	} else if n, err := store.Sweep(keep, time.Hour); err != nil {
		log.Warn("sweep attachment files", "err", err)
	} else if n > 0 {
		log.Info("removed unused attachment files", "files", n)
	}
}

func newAccounts(cfg config.Config, st *store.Store, sender mail.Sender, log *slog.Logger) (*accounts.Service, error) {
	u, _ := url.Parse(cfg.BaseURL) // validated by config
	return accounts.New(st, sender, log, accounts.Config{
		BaseURL: cfg.BaseURL, Origin: cfg.Origin(), RPID: u.Hostname(), SignInMailAfter: true,
	})
}

// loginLink prints a sign-in link for an admin who cannot receive email.
func loginLink(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: mila user login-link <email> [settings flags]")
	}
	email := args[0]
	cfg, err := config.LoadFromOS("mila user login-link", args[1:])
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
	link, err := acc.LoginLink(ctx, email)
	if err != nil {
		return err
	}
	fmt.Println(link)
	fmt.Fprintln(os.Stderr, "The link works once and expires in 15 minutes.")
	return nil
}

// makeAdmin gives a user admin rights over the instance: the rescue when
// no admin is left who can, run by someone at the server.
func makeAdmin(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: mila user make-admin <email> [settings flags]")
	}
	email := args[0]
	cfg, err := config.LoadFromOS("mila user make-admin", args[1:])
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
	u, err := acc.MakeAdmin(ctx, email)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%s) is an admin of this Mila.\n", u.Name, u.Email)
	return nil
}

func localKeyOf(cfg config.Config) string {
	if cfg.Local == "on" {
		return cfg.LocalKey
	}
	return ""
}

func localWatchKeyOf(cfg config.Config) string {
	if cfg.Local == "on" {
		return cfg.LocalWatchKey
	}
	return ""
}

// emailInStatus says how reading the mailbox last went, for the admin page; nil
// without a mailbox.
func emailInStatus(r *inbound.Reader) func() (time.Time, string) {
	if r == nil {
		return nil
	}
	return func() (time.Time, string) {
		s := r.Status()
		return s.LastCheck, s.LastError
	}
}

// microsoftDeps is the Microsoft mailbox for the API, or none: a nil
// *inbound.Microsoft in the interface would not be nil to the handlers.
func microsoftDeps(m *inbound.Microsoft) httpapi.MicrosoftMailbox {
	if m == nil {
		return nil
	}
	return m
}

// mailMode is how email goes out, for the admin page.
func mailMode(smtpURL string) string {
	switch {
	case smtpURL == "":
		return "log"
	case strings.HasPrefix(smtpURL, "file:"):
		return "file"
	}
	return "smtp"
}

func configCheck(args []string) error {
	cfg, err := config.LoadFromOS("mila config check", args)
	if err != nil {
		return err
	}
	fmt.Println("# Effective settings")
	if err := toml.NewEncoder(os.Stdout).Encode(cfg.Redacted()); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	current, latest, err := d.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("\n# Database\n%s reachable; schema at %d, this binary carries up to %d\n", d.Dialect, current, latest)
	if current > latest {
		return db.ErrDatabaseNewer
	}
	instance, _, err := store.New(d).InstanceSetting(ctx, "instance_id")
	if err != nil {
		return err
	}

	// What else must work: email out, the mailbox in, and the address.
	problems := 0
	report := func(heading, what string, err error) {
		fmt.Printf("\n# %s\n", heading)
		if err != nil {
			problems++
			fmt.Printf("PROBLEM: %v\n", err)
			return
		}
		fmt.Println(what)
	}
	check := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Second)
	}
	if cfg.Local != "on" {
		c, done := check()
		_, what, err := mail.Check(c, cfg.SMTPURL, cfg.MailFrom)
		done()
		report("Email", what, err)
		if cfg.IMAPURL != "" {
			c, done := check()
			what, err := inbound.CheckMailbox(c, cfg.IMAPURL, microsoftMailbox(cfg, store.New(d), slog.Default(), nil))
			done()
			report("Email to task", what, err)
		}
		c, done = check()
		what, err = checkAddress(c, cfg.BaseURL, instance)
		done()
		report("Address", what, err)
	}
	if problems > 0 {
		return fmt.Errorf("%d %s to fix", problems, map[bool]string{true: "problem", false: "problems"}[problems == 1])
	}
	fmt.Println("\nOK")
	return nil
}

// checkAddress says whether the base URL reaches this Mila: its /healthz
// answers, with the fingerprint of the instance in this database.
func checkAddress(ctx context.Context, base, instance string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s cannot be reached from this machine: %w. Some routers do not let a machine reach its own public address; try it from another", base, err)
	}
	defer func() { _ = res.Body.Close() }()
	got := res.Header.Get(httpapi.InstanceHeader)
	switch {
	case res.StatusCode != http.StatusOK || got == "":
		return "", fmt.Errorf("%s answers, but not as Mila (status %d); is the proxy pointed at Mila?", base, res.StatusCode)
	case instance == "":
		return base + " reaches a Mila; this database has not been started yet, so which one cannot be told", nil
	case got != httpapi.InstanceFingerprint(instance):
		return "", fmt.Errorf("%s reaches a Mila, but not this one: another database answers there", base)
	}
	return base + " reaches this Mila", nil
}

// serverErrors is the HTTP servers' error log for Mila's own HTTPS. A port open
// to the internet meets scanners all day, each failed handshake a line ("http:
// TLS handshake error from ..."), so those are logged at Debug, where they
// don't bury the warnings that matter.
func serverErrors(log *slog.Logger) *stdlog.Logger {
	return stdlog.New(errorLines{log}, "", 0)
}

type errorLines struct{ log *slog.Logger }

func (e errorLines) Write(p []byte) (int, error) {
	line := strings.TrimSuffix(string(p), "\n")
	level := slog.LevelWarn
	if strings.HasPrefix(line, "http: TLS handshake error") {
		level = slog.LevelDebug
	}
	e.log.Log(context.Background(), level, line)
	return len(p), nil
}
