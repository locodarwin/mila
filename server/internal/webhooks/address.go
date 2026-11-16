package webhooks

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/locodarwin/mila/server/internal/apperr"
)

// Anyone who can add a webhook can make this server send a request, so unless
// the operator says otherwise it may only reach the public internet: a webhook
// must not be a way to probe what sits beside Mila on its network.

var errRefusedAddress = errors.New("refused address")

// notPublic are the ranges no public server has, beyond what netip names.
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT, and Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

var (
	globalIPv6 = netip.MustParsePrefix("2000::/3")
	teredo     = netip.MustParsePrefix("2001::/32")
	sixToFour  = netip.MustParsePrefix("2002::/16")
)

// isPublic reports whether an address is one out on the internet.
func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// IPv6 has one range of global addresses. Anything else, and the two ranges in it
	// that tunnel to an IPv4 address, could lead back inside.
	if ip.Is6() && (!globalIPv6.Contains(ip) || teredo.Contains(ip) || sixToFour.Contains(ip)) {
		return false
	}
	for _, p := range notPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// checkURL reads a webhook's address, as it will be used.
func checkURL(raw string, allowPrivate bool) (string, error) {
	raw = strings.TrimSpace(raw)
	bad := func(msg string) (string, error) { return "", apperr.New(apperr.Invalid, "%s", msg) }
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || len(raw) > maxURL {
		return bad("That is not a web address.")
	}
	if u.User != nil {
		return bad("Leave the name and password out of the address.")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowPrivate {
			return bad("The address must start with https://.")
		}
	default:
		return bad("The address must start with https://.")
	}
	if allowPrivate {
		return u.String(), nil
	}
	host := strings.ToLower(u.Hostname())
	inside := "The address must not point inside this server's network. The server's MILA_WEBHOOK_ALLOW_PRIVATE setting allows it."
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return bad(inside)
	}
	if ip, err := netip.ParseAddr(host); err == nil && !isPublic(ip) {
		return bad(inside)
	}
	return u.String(), nil
}

// newClient makes the client deliveries go through. The address is checked as
// the connection opens, after the name has been looked up, so a name that
// answers with an inside address is refused too; and redirects are not followed,
// so a public address cannot send a delivery somewhere that was never checked.
func newClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !isPublic(ap.Addr()) {
				return errRefusedAddress
			}
			return nil
		}
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: sendTimeout,
			MaxIdleConns:          8,
			IdleConnTimeout:       30 * time.Second,
			DisableCompression:    true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
