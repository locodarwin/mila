package main

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"path/filepath"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/locodarwin/mila/server/internal/config"
)

// Mila's own HTTPS (MILA_TLS_DOMAIN): a certificate from Let's Encrypt, or
// another ACME directory, for the one domain, kept in the data directory
// and renewed before it runs out. HTTPS is served on tls_listen; plain
// HTTP on tls_http_listen answers the certificate's checks and sends
// everyone else to the HTTPS address.

// certManager gets and renews the domain's certificate.
func certManager(cfg config.Config) *autocert.Manager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.TLSDomain),
		Cache:      autocert.DirCache(filepath.Join(cfg.DataDir, "certs")),
		Email:      cfg.TLSEmail,
	}
	if cfg.TLSCA != "" {
		m.Client = &acme.Client{DirectoryURL: cfg.TLSCA}
	}
	return m
}

// tlsConfig is the manager's, which also answers the TLS-ALPN check, held
// to TLS 1.2 and up.
func tlsConfig(m *autocert.Manager) *tls.Config {
	c := m.TLSConfig()
	c.MinVersion = tls.VersionTLS12
	return c
}

// toHTTPS sends a plain HTTP request to the same path on the public HTTPS
// address. The address is the configured one, never the request's Host,
// so the redirect cannot be pointed elsewhere.
func toHTTPS(base string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.Redirect(w, r, base+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}

// warmCertificate gets the certificate as Mila starts, so the first visitor
// does not wait for it and a domain that cannot have one says so in the log.
func warmCertificate(log *slog.Logger, m *autocert.Manager, domain string) {
	// A browser's hello, so the certificate is the ECDSA one browsers get.
	hello := &tls.ClientHelloInfo{
		ServerName:        domain,
		CipherSuites:      []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		SignatureSchemes:  []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		SupportedCurves:   []tls.CurveID{tls.CurveP256},
		SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
	}
	if _, err := m.GetCertificate(hello); err != nil {
		log.Warn("could not get the HTTPS certificate yet; is the domain pointed at this machine, with ports 80 and 443 open to the internet?",
			"domain", domain, "err", err)
		return
	}
	log.Info("HTTPS certificate ready", "domain", domain)
}
