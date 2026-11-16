// Package config loads Mila's settings. Precedence, highest first:
// command-line flags, MILA_* environment variables, a TOML file, defaults.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is every setting the server reads at startup.
type Config struct {
	Listen      string `toml:"listen"`
	BaseURL     string `toml:"base_url"`
	DataDir     string `toml:"data_dir"`
	DatabaseURL string `toml:"database_url"`
	LogFormat   string `toml:"log_format"`
	LogLevel    string `toml:"log_level"`
	SMTPURL     string `toml:"smtp_url"`
	MailFrom    string `toml:"mail_from"`
	// IMAPURL is the mailbox email to task reads; empty turns it off.
	IMAPURL string `toml:"imap_url"`
	// InboundAddress is what a person's address looks like, with {token} in it.
	InboundAddress string `toml:"inbound_address"`
	// IMAPPoll is how often the mailbox is checked, in seconds.
	IMAPPoll string `toml:"imap_poll"`
	// IMAPAfter says what becomes of mail once dealt with: keep or delete.
	IMAPAfter string `toml:"imap_after"`
	// IMAPAuth is how Mila signs in to the mailbox: password, or microsoft
	// for Outlook.com and Microsoft 365, which take no password over IMAP.
	IMAPAuth string `toml:"imap_auth"`
	// MicrosoftClientID and MicrosoftTenant name the app registered in
	// Microsoft Entra that Mila signs in to a Microsoft mailbox through.
	MicrosoftClientID string `toml:"microsoft_client_id"`
	MicrosoftTenant   string `toml:"microsoft_tenant"`
	// MicrosoftAuthority is where Microsoft's sign-in answers; for tests.
	MicrosoftAuthority string `toml:"microsoft_authority"`
	// Local is the desktop app's mode, on or off: one person on one computer, no
	// sign-in, nobody else and nothing leaving it.
	Local string `toml:"local"`
	// LocalKey signs the app's window in, once, as it starts. An environment
	// variable, as a flag would show in the list of processes.
	LocalKey string `toml:"local_key"`
	// LocalWatchKey lets the desktop app hear of new notifications while the
	// place's window is elsewhere; unlike LocalKey it lasts as long as the server.
	LocalWatchKey string `toml:"local_watch_key"`
	// ExitWithStdin stops the server when its input closes, on or off, so that it
	// can't outlive the desktop app that started it however that app ended.
	ExitWithStdin string `toml:"exit_with_stdin"`
	// TrustedProxies is a comma-separated list of addresses and CIDRs
	// whose X-Forwarded-For header is believed, such as Caddy's network.
	// An address alone is that one machine, as the log's warning names it.
	TrustedProxies string `toml:"trusted_proxies"`
	// SignInRate is how many sign-in attempts one address may make a
	// minute, after a burst of twice that.
	SignInRate string `toml:"sign_in_rate"`
	// InviteRate is how many invitations one person may send an hour,
	// after a burst of ten times that (at least 20).
	InviteRate string `toml:"invite_rate"`
	// WebhookAllowPrivate lets webhooks reach inside this server's network: on or off.
	WebhookAllowPrivate string `toml:"webhook_allow_private"`
	// OAuth lets apps that sign in by OAuth, such as hosted assistants,
	// connect to the MCP endpoint once a person says yes: on or off.
	OAuth string `toml:"oauth"`
	// ImportMaxMB is the largest workspace export, in megabytes, that an
	// import accepts.
	ImportMaxMB string `toml:"import_max_mb"`
	// Backup turns the nightly backup on or off.
	Backup string `toml:"backup"`
	// BackupDir holds the backups; empty means <data_dir>/backups.
	BackupDir string `toml:"backup_dir"`
	// BackupHour is the hour, UTC, from which the nightly backup is due.
	BackupHour string `toml:"backup_hour"`
	// TLSDomain is the domain Mila gets its own HTTPS certificate for, from
	// Let's Encrypt; empty leaves HTTPS to a proxy in front of it.
	TLSDomain string `toml:"tls_domain"`
	// TLSEmail is told of the certificate's problems by Let's Encrypt; optional.
	TLSEmail string `toml:"tls_email"`
	// TLSCA is the ACME directory certificates come from; empty is Let's Encrypt's.
	TLSCA string `toml:"tls_ca"`
	// TLSListen is where HTTPS is served, and TLSHTTPListen where plain HTTP
	// answers the certificate's checks and sends everyone else to HTTPS.
	TLSListen     string `toml:"tls_listen"`
	TLSHTTPListen string `toml:"tls_http_listen"`
	// UpdateCheck asks GitHub once a day for the latest release, so Settings,
	// Admin can say when a newer Mila is out: on or off.
	UpdateCheck string `toml:"update_check"`
}

// setting ties one field to its flag, environment variable and help text.
type setting struct {
	flag, env, help string
	field           func(*Config) *string
}

var settings = []setting{
	{"listen", "MILA_LISTEN", "address to listen on", func(c *Config) *string { return &c.Listen }},
	{"base-url", "MILA_BASE_URL", "public URL of this instance; a domain or subdomain, never a sub-path", func(c *Config) *string { return &c.BaseURL }},
	{"data-dir", "MILA_DATA_DIR", "directory for the SQLite database, attachments, exports and backups", func(c *Config) *string { return &c.DataDir }},
	{"database-url", "MILA_DATABASE_URL", "sqlite://<path> or postgres://...; defaults to SQLite in the data directory", func(c *Config) *string { return &c.DatabaseURL }},
	{"log-format", "MILA_LOG_FORMAT", "json or text", func(c *Config) *string { return &c.LogFormat }},
	{"log-level", "MILA_LOG_LEVEL", "debug, info, warn or error", func(c *Config) *string { return &c.LogLevel }},
	{"smtp-url", "MILA_SMTP_URL", "smtp://user:pass@host:587, smtps://...:465 or file:///dir; empty writes email to the log", func(c *Config) *string { return &c.SMTPURL }},
	{"mail-from", "MILA_MAIL_FROM", "sender address for email; defaults to mila@ the base URL's host", func(c *Config) *string { return &c.MailFrom }},
	{"imap-url", "MILA_IMAP_URL", "the mailbox email to task reads: imaps://user:pass@host:993/INBOX, imap://... for STARTTLS, or file:///dir; empty turns it off", func(c *Config) *string { return &c.IMAPURL }},
	{"inbound-address", "MILA_INBOUND_ADDRESS", "what a person's address looks like, such as mila+{token}@example.com or {token}@in.example.com", func(c *Config) *string { return &c.InboundAddress }},
	{"imap-poll", "MILA_IMAP_POLL", "how often to check the mailbox, in seconds", func(c *Config) *string { return &c.IMAPPoll }},
	{"imap-after", "MILA_IMAP_AFTER", "what becomes of mail once dealt with: keep or delete", func(c *Config) *string { return &c.IMAPAfter }},
	{"imap-auth", "MILA_IMAP_AUTH", "how Mila signs in to the mailbox: password, or microsoft for Outlook.com and Microsoft 365 (connected from Settings, Admin)", func(c *Config) *string { return &c.IMAPAuth }},
	{"microsoft-client-id", "MILA_MICROSOFT_CLIENT_ID", "the Application (client) ID of the app registered in Microsoft Entra, with imap_auth microsoft", func(c *Config) *string { return &c.MicrosoftClientID }},
	{"microsoft-tenant", "MILA_MICROSOFT_TENANT", "the Directory (tenant) ID of a Microsoft 365 organization, or consumers for Outlook.com, or common for both", func(c *Config) *string { return &c.MicrosoftTenant }},
	{"microsoft-authority", "MILA_MICROSOFT_AUTHORITY", "where Microsoft's sign-in answers; only for tests", func(c *Config) *string { return &c.MicrosoftAuthority }},
	{"local", "MILA_LOCAL", "the desktop app's mode, for one person on this computer: on or off", func(c *Config) *string { return &c.Local }},
	{"local-key", "MILA_LOCAL_KEY", "signs the desktop app's window in, once; set by the app, in the environment", func(c *Config) *string { return &c.LocalKey }},
	{"local-watch-key", "MILA_LOCAL_WATCH_KEY", "lets the desktop app hear of notifications while its window is elsewhere; set by the app, in the environment", func(c *Config) *string { return &c.LocalWatchKey }},
	{"exit-with-stdin", "MILA_EXIT_WITH_STDIN", "stop when standard input closes: on or off", func(c *Config) *string { return &c.ExitWithStdin }},
	{"sign-in-rate", "MILA_SIGN_IN_RATE", "sign-in attempts allowed per minute from one address", func(c *Config) *string { return &c.SignInRate }},
	{"invite-rate", "MILA_INVITE_RATE", "invitations one person may send an hour, after a burst of ten times as many (at least 20)", func(c *Config) *string { return &c.InviteRate }},
	{"import-max-mb", "MILA_IMPORT_MAX_MB", "largest JSON export, in megabytes, that an import accepts", func(c *Config) *string { return &c.ImportMaxMB }},
	{"backup", "MILA_BACKUP", "nightly backup of the database: on or off", func(c *Config) *string { return &c.Backup }},
	{"backup-dir", "MILA_BACKUP_DIR", "directory for backups; defaults to backups in the data directory", func(c *Config) *string { return &c.BackupDir }},
	{"backup-hour", "MILA_BACKUP_HOUR", "hour of the day, UTC, from 0 to 23, at which the nightly backup is made", func(c *Config) *string { return &c.BackupHour }},
	{"webhook-allow-private", "MILA_WEBHOOK_ALLOW_PRIVATE", "let webhooks reach addresses inside this server's network, and plain http: on or off", func(c *Config) *string { return &c.WebhookAllowPrivate }},
	{"oauth", "MILA_OAUTH", "let apps connect to the MCP endpoint by OAuth, once a person says yes: on or off", func(c *Config) *string { return &c.OAuth }},
	{"tls-domain", "MILA_TLS_DOMAIN", "a domain to get Mila's own HTTPS certificate for, from Let's Encrypt; empty leaves HTTPS to a proxy", func(c *Config) *string { return &c.TLSDomain }},
	{"tls-email", "MILA_TLS_EMAIL", "an address Let's Encrypt writes to about the certificate; optional", func(c *Config) *string { return &c.TLSEmail }},
	{"tls-ca", "MILA_TLS_CA", "the ACME directory certificates come from; empty is Let's Encrypt's", func(c *Config) *string { return &c.TLSCA }},
	{"tls-listen", "MILA_TLS_LISTEN", "where HTTPS is served, with tls_domain set", func(c *Config) *string { return &c.TLSListen }},
	{"tls-http-listen", "MILA_TLS_HTTP_LISTEN", "where plain HTTP answers the certificate's checks and sends people to HTTPS, with tls_domain set", func(c *Config) *string { return &c.TLSHTTPListen }},
	{"update-check", "MILA_UPDATE_CHECK", "ask GitHub once a day whether a newer Mila is out, sending nothing else: on or off", func(c *Config) *string { return &c.UpdateCheck }},
	{"trusted-proxies", "MILA_TRUSTED_PROXIES", "comma-separated addresses or CIDRs of reverse proxies whose X-Forwarded-For is trusted", func(c *Config) *string { return &c.TrustedProxies }},
}

// Defaults returns the settings used when nothing else sets them.
func Defaults() Config {
	return Config{
		Listen:              "127.0.0.1:8080",
		BaseURL:             "http://localhost:8080",
		DataDir:             "data",
		LogFormat:           "json",
		LogLevel:            "info",
		SignInRate:          "10",
		InviteRate:          "2",
		WebhookAllowPrivate: "off",
		OAuth:               "off",
		ImportMaxMB:         "256",
		IMAPPoll:            "60",
		Local:               "off",
		ExitWithStdin:       "off",
		IMAPAfter:           "keep",
		IMAPAuth:            "password",
		MicrosoftTenant:     "common",
		MicrosoftAuthority:  "https://login.microsoftonline.com",
		Backup:              "on",
		BackupHour:          "3",
		TLSListen:           ":443",
		TLSHTTPListen:       ":80",
		UpdateCheck:         "on",
	}
}

// Load parses args (without the program and subcommand names) and applies
// the environment and the config file named by --config or MILA_CONFIG.
func Load(name string, args []string, getenv func(string) string) (Config, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	configFile := fs.String("config", "", "path to a TOML config file (env MILA_CONFIG)")
	flagValues := make(map[string]*string, len(settings))
	for _, s := range settings {
		flagValues[s.flag] = fs.String(s.flag, "", fmt.Sprintf("%s (env %s)", s.help, s.env))
	}
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	cfg := Defaults()

	path := *configFile
	if path == "" {
		path = getenv("MILA_CONFIG")
	}
	baseSet := false
	if path != "" {
		md, err := toml.DecodeFile(path, &cfg)
		baseSet = md.IsDefined("base_url")
		if err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", path, err)
		}
		if undecoded := md.Undecoded(); len(undecoded) > 0 {
			return Config{}, fmt.Errorf("config file %s: unknown setting %q", path, undecoded[0].String())
		}
	}

	for _, s := range settings {
		if v := getenv(s.env); v != "" {
			*s.field(&cfg) = v
		}
	}

	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	baseSet = baseSet || given["base-url"] || getenv("MILA_BASE_URL") != ""
	for _, s := range settings {
		if given[s.flag] {
			*s.field(&cfg) = *flagValues[s.flag]
		}
	}

	// With Mila's own HTTPS, the address people use is the domain's.
	if cfg.TLSDomain != "" && !baseSet {
		cfg.BaseURL = "https://" + cfg.TLSDomain
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "sqlite://" + filepath.Join(cfg.DataDir, "mila.db")
	}
	if cfg.BackupDir == "" {
		cfg.BackupDir = filepath.Join(cfg.DataDir, "backups")
	}
	if cfg.MailFrom == "" {
		if u, err := url.Parse(cfg.BaseURL); err == nil && u.Hostname() != "" {
			cfg.MailFrom = "Mila <mila@" + u.Hostname() + ">"
		}
	}
	return cfg, cfg.Validate()
}

// LoadFromOS is Load with the real environment.
func LoadFromOS(name string, args []string) (Config, error) {
	return Load(name, args, os.Getenv)
}

// validateLocal holds the desktop app's mode to what makes it safe: reachable from
// this computer alone, over the database that needs nothing else running.
func (c Config) validateLocal() error {
	switch c.ExitWithStdin {
	case "on", "off":
	default:
		return fmt.Errorf("exit_with_stdin: %q is not on or off", c.ExitWithStdin)
	}
	switch c.Local {
	case "off":
		return nil
	case "on":
	default:
		return fmt.Errorf("local: %q is not on or off", c.Local)
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("local: listen must be on this computer alone, such as 127.0.0.1:8790, not %q", c.Listen)
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("base_url: %w", err)
	}
	if ip := net.ParseIP(base.Hostname()); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("local: base_url must be on this computer alone, such as http://127.0.0.1:8790, not %q", c.BaseURL)
	}
	if len(c.LocalKey) < 32 {
		return errors.New("local: local_key must be at least 32 characters")
	}
	if c.LocalWatchKey != "" && len(c.LocalWatchKey) < 32 {
		return errors.New("local: local_watch_key must be at least 32 characters")
	}
	if !strings.HasPrefix(c.DatabaseURL, "sqlite://") {
		return errors.New("local: uses SQLite")
	}
	if c.SMTPURL != "" || c.IMAPURL != "" {
		return errors.New("local: nothing leaves this computer, so smtp_url and imap_url must be empty")
	}
	if c.TLSDomain != "" {
		return errors.New("local: nothing leaves this computer, so tls_domain must be empty")
	}
	if c.OAuth == "on" {
		return errors.New("local: no app connects to Mila on this computer, so oauth must be off")
	}
	if c.WebhookAllowPrivate == "on" {
		return errors.New("local: there are no webhooks on this computer, so webhook_allow_private must be off")
	}
	return nil
}

// domainName is a name a certificate can be had for: labels of letters,
// digits and hyphens, ending in a top-level name of letters.
// microsoftName is a client or tenant ID as Entra shows it (a GUID), or one of
// the names a tenant may go by instead.
var microsoftName = regexp.MustCompile(`^([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|common|consumers|organizations|[a-z0-9-]+(\.[a-z0-9-]+)+)$`)

var domainName = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// validateTLS holds Mila's own HTTPS to what can work: a domain that can
// have a certificate, the public address on it, and places to listen.
func (c Config) validateTLS() error {
	if c.TLSDomain == "" {
		return nil
	}
	u, _ := url.Parse(c.BaseURL) // checked by Validate
	if u.Scheme != "https" || u.Hostname() != c.TLSDomain || (u.Port() != "" && u.Port() != "443") {
		return fmt.Errorf("base_url: with tls_domain set it must be https://%s, or left out to be that, not %q", c.TLSDomain, c.BaseURL)
	}
	if c.TLSEmail != "" {
		local, domain, ok := strings.Cut(c.TLSEmail, "@")
		if !ok || local == "" || !strings.Contains(domain, ".") || strings.ContainsAny(c.TLSEmail, " \t<>,") {
			return fmt.Errorf("tls_email: %q is not an email address", c.TLSEmail)
		}
	}
	if c.TLSCA != "" {
		if ca, err := url.Parse(c.TLSCA); err != nil || ca.Scheme != "https" || ca.Host == "" {
			return fmt.Errorf("tls_ca: %q is not an https URL of an ACME directory", c.TLSCA)
		}
	}
	for name, addr := range map[string]string{"tls_listen": c.TLSListen, "tls_http_listen": c.TLSHTTPListen} {
		if _, port, err := net.SplitHostPort(addr); err != nil || port == "" {
			return fmt.Errorf("%s: %q is not an address to listen on, such as :443", name, addr)
		}
	}
	if c.TLSListen == c.TLSHTTPListen {
		return errors.New("tls_listen and tls_http_listen: must not be the same")
	}
	return nil
}

// validateIMAP holds the settings of email to task together: the mailbox and
// what its addresses look like only make sense as a pair.
func (c Config) validateIMAP() error {
	if n, err := strconv.Atoi(c.IMAPPoll); err != nil || n < 1 || n > 86400 {
		return fmt.Errorf("imap_poll: %q is not a whole number of seconds from 1 to 86400", c.IMAPPoll)
	}
	if c.IMAPAfter != "keep" && c.IMAPAfter != "delete" {
		return fmt.Errorf("imap_after: %q is not keep or delete", c.IMAPAfter)
	}
	if c.IMAPAuth != "password" && c.IMAPAuth != "microsoft" {
		return fmt.Errorf("imap_auth: %q is not password or microsoft", c.IMAPAuth)
	}
	if c.IMAPAuth == "microsoft" {
		if c.MicrosoftClientID == "" {
			return errors.New("microsoft_client_id: imap_auth microsoft needs the Application (client) ID of the app registered in Microsoft Entra")
		}
		if !microsoftName.MatchString(c.MicrosoftClientID) || !microsoftName.MatchString(c.MicrosoftTenant) {
			return errors.New("microsoft_client_id and microsoft_tenant: copy them from the app's Overview in Microsoft Entra")
		}
		if u, err := url.Parse(c.MicrosoftAuthority); err != nil || u.Host == "" || (u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
			return errors.New("microsoft_authority: must be an https:// address")
		}
	}
	if c.IMAPURL == "" && c.InboundAddress == "" {
		return nil
	}
	if c.IMAPURL == "" || c.InboundAddress == "" {
		return errors.New("imap_url and inbound_address: set both, or neither")
	}
	u, err := url.Parse(c.IMAPURL)
	if err != nil {
		// The parser's message would repeat the password.
		return errors.New("imap_url: is not a URL; a password with special characters must be percent-encoded")
	}
	switch u.Scheme {
	case "imaps", "imap":
		if u.Host == "" || u.User == nil {
			return errors.New("imap_url: needs a user and a host, as in imaps://user:pass@host:993/INBOX")
		}
		if _, has := u.User.Password(); has && c.IMAPAuth == "microsoft" {
			return errors.New("imap_url: with imap_auth microsoft, give the mailbox's address and no password, as in imaps://tasks%40example.com@outlook.office365.com:993/INBOX")
		}
		if u.Query().Get("tls") == "none" {
			if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
				return errors.New("imap_url: tls=none is only for a mailbox on this machine")
			}
		}
	case "file":
	default:
		return errors.New("imap_url: must start with imaps://, imap:// or file://")
	}
	local, domain, ok := strings.Cut(c.InboundAddress, "@")
	if !ok || domain == "" || strings.Contains(domain, "@") || strings.Count(local, "{token}") != 1 || strings.Contains(domain, "{token}") {
		return fmt.Errorf("inbound_address: %q must look like mila+{token}@example.com or {token}@in.example.com", c.InboundAddress)
	}
	return nil
}

// Validate reports the first setting that cannot work.
func (c Config) Validate() error {
	if c.Listen == "" {
		return errors.New("listen: must not be empty")
	}
	// Before the base URL, which is made from it.
	if c.TLSDomain != "" && (!domainName.MatchString(c.TLSDomain) || len(c.TLSDomain) > 253) {
		return fmt.Errorf("tls_domain: %q is not a domain name in lower case, such as mila.example.com, with no https:// or port", c.TLSDomain)
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("base_url: %q is not an absolute http or https URL", c.BaseURL)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("base_url: %q has a path; Mila needs its own domain or subdomain", c.BaseURL)
	}
	if c.DataDir == "" {
		return errors.New("data_dir: must not be empty")
	}
	if !strings.HasPrefix(c.DatabaseURL, "sqlite://") &&
		!strings.HasPrefix(c.DatabaseURL, "postgres://") &&
		!strings.HasPrefix(c.DatabaseURL, "postgresql://") {
		return fmt.Errorf("database_url: must start with sqlite://, postgres:// or postgresql://")
	}
	if n, err := strconv.Atoi(c.SignInRate); err != nil || n < 1 {
		return fmt.Errorf("sign_in_rate: %q is not a positive whole number", c.SignInRate)
	}
	if n, err := strconv.Atoi(c.InviteRate); err != nil || n < 1 {
		return fmt.Errorf("invite_rate: %q is not a positive whole number", c.InviteRate)
	}
	if n, err := strconv.Atoi(c.ImportMaxMB); err != nil || n < 1 || n > 1<<20 {
		return fmt.Errorf("import_max_mb: %q is not a whole number of megabytes from 1 to 1048576", c.ImportMaxMB)
	}
	if err := c.validateIMAP(); err != nil {
		return err
	}
	if err := c.validateLocal(); err != nil {
		return err
	}
	if err := c.validateTLS(); err != nil {
		return err
	}
	switch c.WebhookAllowPrivate {
	case "on", "off":
	default:
		return fmt.Errorf("webhook_allow_private: %q is not on or off", c.WebhookAllowPrivate)
	}
	switch c.OAuth {
	case "on", "off":
	default:
		return fmt.Errorf("oauth: %q is not on or off", c.OAuth)
	}
	switch c.UpdateCheck {
	case "on", "off":
	default:
		return fmt.Errorf("update_check: %q is not on or off", c.UpdateCheck)
	}
	switch c.Backup {
	case "on", "off":
	default:
		return fmt.Errorf("backup: %q is not on or off", c.Backup)
	}
	if n, err := strconv.Atoi(c.BackupHour); err != nil || n < 0 || n > 23 {
		return fmt.Errorf("backup_hour: %q is not an hour from 0 to 23", c.BackupHour)
	}
	if _, err := c.ProxyNets(); err != nil {
		return err
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("log_format: %q is not json or text", c.LogFormat)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level: %q is not debug, info, warn or error", c.LogLevel)
	}
	return nil
}

// IMAPPollEvery is how often to check the mailbox.
func (c Config) IMAPPollEvery() time.Duration {
	n, _ := strconv.Atoi(c.IMAPPoll)
	return time.Duration(n) * time.Second
}

// ProxyNets parses TrustedProxies.
func (c Config) ProxyNets() ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, part := range strings.Split(c.TrustedProxies, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if ip := net.ParseIP(part); ip != nil {
			bits := 128
			if v4 := ip.To4(); v4 != nil {
				ip, bits = v4, 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("trusted_proxies: %q is neither an address nor a CIDR", part)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// SignInPerMinute is SignInRate as a number.
func (c Config) SignInPerMinute() int {
	n, _ := strconv.Atoi(c.SignInRate)
	return n
}

// InvitesPerHour is InviteRate as a number.
func (c Config) InvitesPerHour() int {
	n, _ := strconv.Atoi(c.InviteRate)
	return n
}

// ImportMax is ImportMaxMB in bytes.
func (c Config) ImportMax() int64 {
	n, _ := strconv.Atoi(c.ImportMaxMB)
	return int64(n) << 20
}

// NightlyBackup reports whether the nightly backup is on.
func (c Config) NightlyBackup() bool { return c.Backup == "on" }

// BackupAt is BackupHour as a number.
func (c Config) BackupAt() int {
	n, _ := strconv.Atoi(c.BackupHour)
	return n
}

// Origin is the scheme and host of BaseURL, as browsers send it.
func (c Config) Origin() string {
	u, _ := url.Parse(c.BaseURL)
	return u.Scheme + "://" + u.Host
}

// ChecksForUpdates reports whether Mila asks for the latest release: never in
// the desktop app's mode, where nothing leaves the computer.
func (c Config) ChecksForUpdates() bool { return c.UpdateCheck == "on" && c.Local != "on" }

// OwnHTTPS reports whether Mila gets and serves its own certificate.
func (c Config) OwnHTTPS() bool { return c.TLSDomain != "" }

// HTTPS reports whether the public URL is https.
func (c Config) HTTPS() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// Redacted returns the config with passwords hidden, for printing.
func (c Config) Redacted() Config {
	c.DatabaseURL = redactURL(c.DatabaseURL)
	c.SMTPURL = redactURL(c.SMTPURL)
	c.IMAPURL = redactURL(c.IMAPURL)
	if c.LocalKey != "" {
		c.LocalKey = "redacted"
	}
	if c.LocalWatchKey != "" {
		c.LocalWatchKey = "redacted"
	}
	return c
}

// redactURL hides a URL's user name as well as its password: some mail
// services (Postmark) take the same secret token as both.
func redactURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword("redacted", "redacted")
		} else {
			u.User = url.User("redacted")
		}
		return u.String()
	}
	return raw
}
