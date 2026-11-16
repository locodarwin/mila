package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	cfg, err := Load("serve", nil, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8080" || cfg.LogFormat != "json" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if want := "sqlite://" + filepath.Join("data", "mila.db"); cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, want)
	}
}

func TestPrecedence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mila.toml")
	if err := os.WriteFile(file, []byte("listen = \":7000\"\nlog_level = \"debug\"\nlog_format = \"text\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("serve",
		[]string{"--config", file, "--listen", ":9000"},
		env(map[string]string{"MILA_LISTEN": ":8000", "MILA_LOG_LEVEL": "warn"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" {
		t.Errorf("flag should beat env and file: Listen = %q", cfg.Listen)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("env should beat file: LogLevel = %q", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("file should beat default: LogFormat = %q", cfg.LogFormat)
	}
}

func TestConfigFileFromEnv(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mila.toml")
	if err := os.WriteFile(file, []byte("data_dir = \"/srv/mila\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("serve", nil, env(map[string]string{"MILA_CONFIG": file}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "sqlite://"+filepath.Join("/srv/mila", "mila.db") {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.BackupDir != filepath.Join("/srv/mila", "backups") || !cfg.NightlyBackup() || cfg.BackupAt() != 3 {
		t.Errorf("backups: dir %q, nightly %v at %d", cfg.BackupDir, cfg.NightlyBackup(), cfg.BackupAt())
	}
}

func TestUnknownFileSetting(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mila.toml")
	if err := os.WriteFile(file, []byte("listne = \":7000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load("serve", []string{"--config", file}, env(nil))
	if err == nil || !strings.Contains(err.Error(), "listne") {
		t.Errorf("want an error naming the misspelled key, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]func(*Config){
		"sub-path base URL":  func(c *Config) { c.BaseURL = "https://example.com/pm" },
		"relative base URL":  func(c *Config) { c.BaseURL = "example.com" },
		"unknown database":   func(c *Config) { c.DatabaseURL = "mysql://x" },
		"unknown log format": func(c *Config) { c.LogFormat = "xml" },
		"unknown log level":  func(c *Config) { c.LogLevel = "trace" },
		"empty listen":       func(c *Config) { c.Listen = "" },
		"bad proxy CIDR":     func(c *Config) { c.TrustedProxies = "10.0.0.0/8, nope" },
		"zero sign-in rate":  func(c *Config) { c.SignInRate = "0" },
		"zero import size":   func(c *Config) { c.ImportMaxMB = "0" },
		"wordy import size":  func(c *Config) { c.ImportMaxMB = "lots" },
		"backup maybe":       func(c *Config) { c.Backup = "maybe" },
		"backup hour 24":     func(c *Config) { c.BackupHour = "24" },
		"backup at noonish":  func(c *Config) { c.BackupHour = "noon" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.DatabaseURL = "sqlite://data/mila.db"
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("want an error")
			}
		})
	}
	cfg := Defaults()
	cfg.DatabaseURL = "postgres://u:p@localhost/mila"
	cfg.BaseURL = "https://mila.example.com/"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

// An address alone, as the log's warning gives it, is that one machine.
func TestProxyNetsTakeAddresses(t *testing.T) {
	cfg := Defaults()
	cfg.TrustedProxies = "172.18.0.5, fd00::7 ,10.0.0.0/8"
	nets, err := cfg.ProxyNets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 {
		t.Fatalf("got %d networks, want 3", len(nets))
	}
	for addr, want := range map[string]bool{
		"172.18.0.5": true, "172.18.0.6": false, "fd00::7": true, "fd00::8": false,
		"10.1.2.3": true, "::ffff:172.18.0.5": true,
	} {
		ip := net.ParseIP(addr)
		got := false
		for _, n := range nets {
			got = got || n.Contains(ip)
		}
		if got != want {
			t.Errorf("%s trusted = %v, want %v", addr, got, want)
		}
	}
}

func TestRedacted(t *testing.T) {
	cfg := Defaults()
	cfg.DatabaseURL = "postgres://mila:secret@db/mila"
	// Postmark's token is the user name and the password both.
	cfg.SMTPURL = "smtp://tok3n:tok3n@mail.example.com:587"
	r := cfg.Redacted()
	if strings.Contains(r.DatabaseURL, "secret") || strings.Contains(r.SMTPURL, "tok3n") {
		t.Errorf("secret not redacted: %s %s", r.DatabaseURL, r.SMTPURL)
	}
	cfg.IMAPURL = "imaps://mila%40example.com:hunter2@imap.example.com:993/INBOX"
	if r := cfg.Redacted(); strings.Contains(r.IMAPURL, "hunter2") || strings.Contains(r.IMAPURL, "mila%40") || !strings.Contains(r.IMAPURL, "imap.example.com") {
		t.Errorf("imap_url = %s", r.IMAPURL)
	}
	if !strings.Contains(r.SMTPURL, "@mail.example.com:587") {
		t.Errorf("the host went too: %s", r.SMTPURL)
	}
}

func TestDerivedMailFrom(t *testing.T) {
	cfg, err := Load("serve", []string{"--base-url", "https://mila.example.com"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MailFrom != "Mila <mila@mila.example.com>" {
		t.Errorf("MailFrom = %q", cfg.MailFrom)
	}
	if cfg.Origin() != "https://mila.example.com" || !cfg.HTTPS() {
		t.Errorf("Origin = %q, HTTPS = %v", cfg.Origin(), cfg.HTTPS())
	}
}

func TestEmailToTaskSettings(t *testing.T) {
	good := func() Config {
		c := Defaults()
		c.BaseURL, c.DataDir = "https://mila.example.com", "/data"
		c.DatabaseURL = "sqlite:///data/mila.db"
		return c
	}
	if err := good().Validate(); err != nil {
		t.Fatalf("off: %v", err)
	}
	const plus = "mila+{token}@example.com"
	cases := []struct {
		name, url, address, poll, after string
		ok                              bool
		mustNotSay                      string
	}{
		{"imaps", "imaps://u:p@imap.example.com:993/INBOX", plus, "60", "keep", true, ""},
		{"starttls", "imap://u:p@imap.example.com:143", plus, "60", "delete", true, ""},
		{"catch-all", "imaps://u:p@imap.example.com", "{token}@in.example.com", "60", "keep", true, ""},
		{"a folder, for tests", "file:///tmp/in", plus, "1", "keep", true, ""},
		{"no TLS on this machine", "imap://u:p@127.0.0.1:1143?tls=none", plus, "60", "keep", true, ""},
		{"no TLS elsewhere", "imap://u:p@imap.example.com?tls=none", plus, "60", "keep", false, ""},
		{"a mailbox and no address", "imaps://u:p@imap.example.com", "", "60", "keep", false, ""},
		{"an address and no mailbox", "", plus, "60", "keep", false, ""},
		{"not imap", "http://u:p@imap.example.com", plus, "60", "keep", false, ""},
		{"no user", "imaps://imap.example.com", plus, "60", "keep", false, ""},
		{"no token", "imaps://u:p@imap.example.com", "mila@example.com", "60", "keep", false, ""},
		{"two tokens", "imaps://u:p@imap.example.com", "{token}{token}@example.com", "60", "keep", false, ""},
		{"a token in the domain", "imaps://u:p@imap.example.com", "mila@{token}.example.com", "60", "keep", false, ""},
		{"no domain", "imaps://u:p@imap.example.com", "mila+{token}", "60", "keep", false, ""},
		{"poll", "imaps://u:p@imap.example.com", plus, "0", "keep", false, ""},
		{"after", "imaps://u:p@imap.example.com", plus, "60", "archive", false, ""},
		{"a bad URL never repeats its password", "imaps://u:hunter2%zz@imap.example.com", plus, "60", "keep", false, "hunter2"},
	}
	for _, x := range cases {
		c := good()
		c.IMAPURL, c.InboundAddress, c.IMAPPoll, c.IMAPAfter = x.url, x.address, x.poll, x.after
		err := c.Validate()
		if (err == nil) != x.ok {
			t.Errorf("%s: %v", x.name, err)
		}
		if err != nil && x.mustNotSay != "" && strings.Contains(err.Error(), x.mustNotSay) {
			t.Errorf("%s: says %v", x.name, err)
		}
	}
}

func TestLocalModeStaysOnThisComputer(t *testing.T) {
	good := func() Config {
		c := Defaults()
		c.Local, c.LocalKey = "on", "a-key-long-enough-for-this-test-0123456789"
		c.Listen, c.BaseURL, c.DataDir = "127.0.0.1:8790", "http://127.0.0.1:8790", "/data"
		c.DatabaseURL = "sqlite:///data/mila.db"
		return c
	}
	if err := good().Validate(); err != nil {
		t.Fatalf("on this computer: %v", err)
	}
	for name, change := range map[string]func(*Config){
		"listening everywhere": func(c *Config) { c.Listen = "0.0.0.0:8790" },
		"a public address":     func(c *Config) { c.BaseURL = "https://mila.example.com" },
		"a name, not an IP":    func(c *Config) { c.BaseURL = "http://localhost.example.com:8790" },
		"a short key":          func(c *Config) { c.LocalKey = "short" },
		// Nothing reaches beyond this computer, whatever the environment says.
		"apps by OAuth": func(c *Config) { c.OAuth = "on" },
		"sending email": func(c *Config) { c.SMTPURL = "smtp://mail.example.com:587" },
		"reading a mailbox": func(c *Config) {
			c.IMAPURL, c.InboundAddress = "imaps://u:p@mail.example.com/INBOX", "tasks@example.com"
		},
		"webhooks to the inside": func(c *Config) { c.WebhookAllowPrivate = "on" },
	} {
		c := good()
		change(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestOwnHTTPS(t *testing.T) {
	cfg, err := Load("serve", nil, env(map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_TLS_EMAIL": "ada@example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	// The address people use is the domain's, and the sender follows it.
	if !cfg.OwnHTTPS() || cfg.BaseURL != "https://mila.example.com" || cfg.MailFrom != "Mila <mila@mila.example.com>" {
		t.Errorf("BaseURL = %q, MailFrom = %q", cfg.BaseURL, cfg.MailFrom)
	}
	if cfg.TLSListen != ":443" || cfg.TLSHTTPListen != ":80" {
		t.Errorf("listen on %q and %q", cfg.TLSListen, cfg.TLSHTTPListen)
	}
	// A base URL given in a file counts as given.
	file := filepath.Join(t.TempDir(), "mila.toml")
	if err := os.WriteFile(file, []byte("base_url = \"https://mila.example.com:443\"\ntls_domain = \"mila.example.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load("serve", []string{"--config", file}, env(nil)); err != nil || cfg.BaseURL != "https://mila.example.com:443" {
		t.Errorf("from a file: %q, %v", cfg.BaseURL, err)
	}

	bad := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"MILA_TLS_DOMAIN": "https://mila.example.com"}, "tls_domain"},
		{map[string]string{"MILA_TLS_DOMAIN": "Mila.Example.com"}, "tls_domain"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com:443"}, "tls_domain"},
		{map[string]string{"MILA_TLS_DOMAIN": "localhost"}, "tls_domain"},
		{map[string]string{"MILA_TLS_DOMAIN": "192.168.1.5"}, "tls_domain"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_BASE_URL": "https://other.example.com"}, "base_url"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_BASE_URL": "http://mila.example.com"}, "base_url"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_BASE_URL": "https://mila.example.com:8443"}, "base_url"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_TLS_EMAIL": "not an address"}, "tls_email"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_TLS_CA": "http://ca.example.com/dir"}, "tls_ca"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_TLS_LISTEN": "443"}, "tls_listen"},
		{map[string]string{"MILA_TLS_DOMAIN": "mila.example.com", "MILA_TLS_HTTP_LISTEN": ":443"}, "must not be the same"},
	}
	for _, c := range bad {
		if _, err := Load("serve", nil, env(c.env)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: err = %v, want one about %s", c.env, err, c.want)
		}
	}

	// The desktop app's mode gets no certificate.
	_, err = Load("serve", nil, env(map[string]string{
		"MILA_LOCAL": "on", "MILA_LOCAL_KEY": strings.Repeat("k", 32), "MILA_LISTEN": "127.0.0.1:8790",
		"MILA_BASE_URL": "http://127.0.0.1:8790", "MILA_TLS_DOMAIN": "mila.example.com",
	}))
	if err == nil || !strings.Contains(err.Error(), "tls_domain") {
		t.Errorf("local with a domain: %v", err)
	}
}
