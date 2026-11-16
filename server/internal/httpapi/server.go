// Package httpapi is Mila's HTTP surface: health checks, the REST API
// under /api/v1 (routed by code generated from shared/openapi.yaml) and
// the embedded single-page app.
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/actions"
	"github.com/locodarwin/mila/server/internal/automations"
	"github.com/locodarwin/mila/server/internal/backup"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/ical"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/ownership"
	"github.com/locodarwin/mila/server/internal/push"
	"github.com/locodarwin/mila/server/internal/ratelimit"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/updates"
	"github.com/locodarwin/mila/server/internal/webhooks"
)

// Pinger is the part of the database the readiness check needs.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Schema is the part of the database the admin page shows: its migration
// version, the newest this binary carries, and when it last migrated.
type Schema interface {
	SchemaVersion(ctx context.Context) (current, latest int64, err error)
	LastMigrated(ctx context.Context) (time.Time, error)
}

// Deps is what the handlers need from the rest of the server.
// InstanceHeader carries a server's InstanceFingerprint on /healthz.
const InstanceHeader = "Mila-Instance"

// InstanceFingerprint names an instance without giving its ID away.
func InstanceFingerprint(id string) string {
	sum := sha256.Sum256([]byte("mila instance " + id))
	return hex.EncodeToString(sum[:8])
}

type Deps struct {
	Log         *slog.Logger
	DB          Pinger
	Web         fs.FS
	Accounts    *accounts.Service
	Sync        *syncer.Service
	Hub         *syncer.Hub
	Files       *files.Store
	Notify      *notify.Service
	Push        *push.Service
	Calendar    *ical.Service
	Data        *ownership.Service
	Webhooks    *webhooks.Service
	Automations *automations.Service
	// Actions are the app's actions for scripts and agents.
	Actions *actions.Service
	// Backup makes and lists backups for the admin page; nil when there
	// is none (tests).
	Backup *backup.Service
	// Schema, Dialect ("sqlite" or "postgres") and MailMode ("smtp",
	// "file" or "log") describe the instance on the admin page.
	Schema   Schema
	Dialect  string
	MailMode string
	// LocalKey turns on the desktop app's mode: the key signs its window in, once.
	LocalKey string
	// LocalWatchKey, in that mode, lets the app read new notifications while its
	// window shows another place (GET /local/watch).
	LocalWatchKey string
	// Updates is the newer release out, when one is known; nil when the
	// server does not ask.
	Updates func() *updates.Release
	// InstanceID is this server's own, from its database; /healthz names it
	// by a fingerprint, so mila config check can tell its base URL reaches
	// this Mila and not another.
	InstanceID string
	// InboundAddress is the template of email to task's addresses, and EmailIn says
	// how reading its mailbox last went; both empty without a mailbox.
	InboundAddress string
	EmailIn        func() (time.Time, string)
	// Microsoft keeps the sign-in to a Microsoft mailbox, or is nil when the
	// mailbox takes a password.
	Microsoft MicrosoftMailbox
	// BaseURL is the public URL, shown on the admin page.
	BaseURL string
	// Origin is the public scheme and host; requests that change data must
	// come from it.
	Origin string
	// HTTPS means the public URL is https: cookies are Secure and browsers
	// get HSTS.
	HTTPS bool
	// TrustedProxies are reverse proxies whose X-Forwarded-For is believed.
	TrustedProxies []*net.IPNet
	// SignInPerMinute limits sign-in attempts per client address, after a
	// burst of twice as many. Zero means the default of 10.
	SignInPerMinute int
	// InvitesPerHour limits the invitations each person sends, after a
	// burst of ten times as many (at least 20). Zero means the default of 2.
	InvitesPerHour int
	// OAuth lets apps connect to the MCP endpoint by OAuth, once a person
	// says yes.
	OAuth bool
	// APIReadsPerMinute and APIWritesPerMinute limit each user's calls to
	// the REST entity endpoints, after a burst of twice as many. Zero means
	// the defaults of 1,200 and 300.
	APIReadsPerMinute, APIWritesPerMinute int
}

// New returns the server's root handler.
func New(d Deps) (http.Handler, error) {
	spa, err := newSPA(d.Web)
	if err != nil {
		return nil, err
	}
	rate := d.SignInPerMinute
	if rate == 0 {
		rate = 10
	}
	invites := d.InvitesPerHour
	if invites == 0 {
		invites = 2
	}
	reads, writes := d.APIReadsPerMinute, d.APIWritesPerMinute
	if reads == 0 {
		reads = 1200
	}
	if writes == 0 {
		writes = 300
	}
	h := &handlers{
		acc:         d.Accounts,
		sync:        d.Sync,
		actions:     d.Actions,
		hub:         d.Hub,
		files:       d.Files,
		log:         d.Log,
		https:       d.HTTPS,
		signInLimit: ratelimit.New(rate, 2*rate),
		notify:      d.Notify,
		// Three at once, then three an hour.
		testMailLimit: ratelimit.NewPerHour(3, 3),
		push:          d.Push,
		// Five at once, then one a minute.
		testPushLimit: ratelimit.New(1, 5),
		calendar:      d.Calendar,
		data:          d.Data,
		hooks:         d.Webhooks,
		automations:   d.Automations,
		// Calendar apps fetch a feed every few minutes at most; a person
		// with many feeds behind one address still fits easily.
		feedLimit:  ratelimit.New(60, 120),
		apiReads:   ratelimit.New(reads, 2*reads),
		apiWrites:  ratelimit.New(writes, 2*writes),
		heavyReads: newHeavyReads(max(2, runtime.GOMAXPROCS(0)/2), 10*time.Second),
		// Ten at once, then thirty an hour.
		avatarLimit: ratelimit.NewPerHour(30, 10),
		renameLimit: ratelimit.NewPerHour(30, 10),
		// Every request counts, refused ones too, so asking for addresses
		// already taken cannot list who has an account.
		emailLimit: ratelimit.NewPerHour(5, 5),
		// By default twenty at once, then two an hour.
		inviteLimit: ratelimit.NewPerHour(invites, max(20, 10*invites)),
		// Thirty at once, then a hundred an hour.
		uploadLimit: ratelimit.NewPerHour(100, 30),
		// Ten at once, then ten an hour.
		exportLimit: ratelimit.NewPerHour(10, 10),
		writeBytes:  ratelimit.NewPerHour(writeBudget, writeBudget),
		// Ten at once, then twenty an hour.
		testHookLimit:  ratelimit.NewPerHour(20, 10),
		backup:         d.Backup,
		schema:         d.Schema,
		dialect:        d.Dialect,
		mailMode:       d.MailMode,
		inboundAddress: d.InboundAddress,
		emailIn:        d.EmailIn,
		microsoft:      d.Microsoft,
		updates:        d.Updates,
		baseURL:        d.BaseURL,
		local:          d.LocalKey != "",
		localWatch:     d.LocalWatchKey,
		cookie:         cookieName,
		origin:         d.Origin,
		// The desktop app's mode lets no app connect, whatever it was started with.
		oauth: d.OAuth && d.LocalKey == "",
		// Twenty at once, then five an hour, from one network (wideRateKey).
		// A full registry drops apps left waiting over an hour, so keeping
		// it full takes a thousand registrations an hour: two hundred
		// networks at this rate.
		oauthRegister: ratelimit.NewPerHour(5, 20),
		// A minute's sixty, after a hundred and twenty at once.
		oauthTokens: ratelimit.New(60, 120),
	}
	if u, err := url.Parse(d.Origin); err == nil && d.LocalKey != "" && u.Port() != "" {
		h.cookie = cookieName + "_" + u.Port()
	}
	if d.LocalKey != "" {
		key := d.LocalKey
		h.localKey.Store(&key)
	}
	if u, err := url.Parse(d.Origin); err == nil && u.Host != "" {
		h.originHosts = []string{u.Host}
	}

	mux := http.NewServeMux()
	instance := ""
	if d.InstanceID != "" {
		instance = InstanceFingerprint(d.InstanceID)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if instance != "" {
			w.Header().Set(InstanceHeader, instance)
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.DB.PingContext(r.Context()); err != nil {
			d.Log.Warn("readiness check failed", "err", err)
			http.Error(w, "database unavailable\n", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/v1/sync/ws", h.syncSocket)
	mux.HandleFunc("GET /ical/{file}", h.calendarFeed)
	mux.Handle(mcpPath, h.mcpEndpoint())
	h.oauthRoutes(mux)
	api.HandlerWithOptions(h, api.StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid", err.Error())
		},
	})
	// The API reference is a page of the app, though it lives under /api/.
	mux.Handle("GET /api/docs", spa)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
	})
	mux.Handle("/", spa)

	var root http.Handler = mux
	root = h.authenticate(root)
	root = refuseNULs(root)
	if h.local {
		root = localOnly(root, d.Origin, d.LocalKey)
	}
	root = checkOrigin(root, d.Origin)
	root = withClientIP(root, d.TrustedProxies, d.Log)
	root = securityHeaders(root, spa.scriptHashes, d.HTTPS)
	root = compressJSON(root)
	root = logRequests(root, d.Log)
	root = readDeadlines(root)
	root = recoverPanics(root, d.Log)
	h.root = root
	return root, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// An encoding error here means the client went away; there is no one
	// left to tell.
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, api.Error{Error: code, Message: message})
}
