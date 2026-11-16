package webhooks

import (
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	for address, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.0.0.5": false, "172.16.0.1": false, "192.168.1.2": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "224.0.0.1": false,
		"::1": false, "::": false, "fe80::1": false, "fc00::1": false, "fd00::1": false,
		"::ffff:127.0.0.1": false, "::ffff:10.0.0.1": false, "::ffff:8.8.8.8": true,
		// Forms that can tunnel back to an inside address.
		"::7f00:1": false, "2002:7f00:1::1": false, "64:ff9b::7f00:1": false, "64:ff9b:1::7f00:1": false,
		"fec0::1": false, "2001::1": false, "100::1": false, "2001:db8::1": false,
	} {
		if got := isPublic(netip.MustParseAddr(address)); got != want {
			t.Errorf("%s: %v", address, got)
		}
	}
}
