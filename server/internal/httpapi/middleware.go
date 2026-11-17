package httpapi

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// securityHeaders sets a strict policy: no inline scripts
// except the app's own bootstrap (allowed by hash), nothing from third
// parties, no framing.
func securityHeaders(next http.Handler, scriptHashes []string, https bool) http.Handler {
	scriptSrc := strings.Join(append([]string{"'self'"}, scriptHashes...), " ")
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src " + scriptSrc,
		// Svelte sets style attributes from script; styles cannot run code.
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self'",
		"connect-src 'self'",
		"worker-src 'self' blob:",
		"manifest-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if https {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// checkOrigin refuses API requests that change data unless they come from
// Mila's own origin. Browsers send Origin on every such request, so a
// cross-site form or fetch cannot ride on the session cookie or sign
// someone in to an account they do not own.
func checkOrigin(next http.Handler, origin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get("Origin")
			// A token is what authenticates a request that has one, cookie or no
			// cookie, and no other site can make a browser send one.
			hasCookie := anySessionCookie(r)
			_, hasToken := bearerToken(r)
			if (got != "" && got != origin) || (got == "" && hasCookie && !hasToken) {
				writeError(w, http.StatusForbidden, "bad_origin", "This request did not come from Mila.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type clientIPKey struct{}

// withClientIP records the client's address: the connection's, or, behind
// a trusted proxy, the nearest untrusted address in X-Forwarded-For.
//
// A request carrying X-Forwarded-For from a peer not on the list is
// logged once: Mila is then behind a proxy it was not told about, every
// visitor shares the proxy's address, and one of them can use up the
// sign-in limit for all.
func withClientIP(next http.Handler, trusted []*net.IPNet, log *slog.Logger) http.Handler {
	var warned atomic.Bool
	isTrusted := func(ip net.IP) bool {
		for _, n := range trusted {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		client := host
		ip := net.ParseIP(host)
		if ip != nil && !isTrusted(ip) && r.Header.Get("X-Forwarded-For") != "" && warned.CompareAndSwap(false, true) {
			log.Warn("a request came through a proxy that MILA_TRUSTED_PROXIES does not name, so all its visitors share one address for sign-in limits and the security log; add its address to MILA_TRUSTED_PROXIES", "proxy", host)
		}
		if ip != nil && isTrusted(ip) {
			// Several lines of the header are one list (RFC 9110): a proxy
			// such as HAProxy adds its own line after the client's.
			hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
			for i := len(hops) - 1; i >= 0; i-- {
				hop := strings.TrimSpace(hops[i])
				hopIP := net.ParseIP(hop)
				if hopIP == nil {
					break
				}
				client = hop
				if !isTrusted(hopIP) {
					break
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, client)))
	})
}

func clientIP(r *http.Request) string {
	ip, _ := r.Context().Value(clientIPKey{}).(string)
	return ip
}

// rateKey is the client address as rate limits count it: an IPv6 address
// by its /64, the network one home or phone is given, since one visitor
// can use any address in it.
func rateKey(r *http.Request) string {
	ip := net.ParseIP(clientIP(r))
	if ip == nil || ip.To4() != nil {
		return clientIP(r)
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// wideRateKey counts by network rather than address: IPv6 by /48, a
// site's usual share, and IPv4 by /24, for what costs everyone when one
// party does much of it (registering OAuth apps fills a registry all
// share), since a /64 or a handful of IPv4 addresses is easy to have.
func wideRateKey(r *http.Request) string {
	ip := net.ParseIP(clientIP(r))
	if ip == nil {
		return clientIP(r)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String() + "/24"
	}
	return ip.Mask(net.CIDRMask(48, 128)).String() + "/48"
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Hijack hands the connection to the WebSocket upgrade.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the connection cannot be taken over")
	}
	s.status = http.StatusSwitchingProtocols
	return hj.Hijack()
}

func logRequests(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Debug("request",
			"method", r.Method,
			"path", loggedPath(r),
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

// loggedPath is the request's path for the log, without the secret a
// calendar feed's path holds.
func loggedPath(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/ical/") {
		return "/ical/(token)"
	}
	return r.URL.Path
}

func recoverPanics(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Error("handler panic", "panic", v, "path", loggedPath(r))
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// readDeadlines bounds how long a request's body may take to arrive, so a
// connection sending it a byte at a time is not held for good: a minute,
// or fifteen for an upload (a file, a picture, an import), which a slow
// phone needs. The sync socket is left alone; it stays open as long as
// the app does.
func readDeadlines(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sync/ws" {
			limit := time.Minute
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") || r.URL.Path == "/api/v1/imports" {
				limit = 15 * time.Minute
			}
			// A writer that cannot set one (a test's recorder) goes without.
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(limit))
		}
		next.ServeHTTP(w, r)
	})
}
