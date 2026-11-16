package push

import (
	"errors"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"syscall"
	"time"
)

// errPrivate is a push endpoint that leads to a private address.
var errPrivate = errors.New("push: endpoint is not a public address")

// reserved are ranges that are not the public internet though Go counts
// them as global unicast: carrier-grade NAT (RFC 6598), IETF protocol
// assignments, benchmarking, the old class E, and NAT64's mapping of IPv4
// into IPv6, which can lead back to a private IPv4 address.
var reserved = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
}

// public reports whether an address is on the public internet. Endpoints
// come from browsers, so a crafted one must not reach the server's own
// network.
func public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, p := range reserved {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// publicClient sends only to public addresses, checked on the address
// actually dialed so a name that resolves to a private one is refused
// too. It follows no redirects and uses no proxy.
func publicClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !public(ap.Addr()) || ap.Port() != 443 {
				return errPrivate
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			MaxIdleConns:        20,
			IdleConnTimeout:     90 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// parseAddress is the bare address in "Name <addr>" or "addr".
func parseAddress(s string) (string, error) {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return "", err
	}
	return a.Address, nil
}
