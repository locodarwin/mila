package push

import (
	"net/netip"
	"testing"
)

func TestEndpoints(t *testing.T) {
	s := &Service{}
	for endpoint, ok := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc": true,
		"https://web.push.apple.com/abc":                         true,
		"https://wns2-par02p.notify.windows.com/w/?token=abc":    true,
		"https://fcm.googleapis.com:443/fcm/send/abc":            true,
		"https://fcm.googleapis.com:8443/fcm/send/abc":           false,
		"http://fcm.googleapis.com/fcm/send/abc":                 false,
		"https://evil.example/fcm.googleapis.com":                false,
		"https://fcm.googleapis.com.evil.example/x":              false,
		"https://nas.example.com/admin":                          false,
		"https://user:pass@fcm.googleapis.com/fcm/send/abc":      false,
		"https://192.168.1.10/":                                  false,
	} {
		if err := s.checkEndpoint(endpoint); (err == nil) != ok {
			t.Errorf("%s: accepted = %v, want %v (%v)", endpoint, err == nil, ok, err)
		}
	}
}

func TestPublicAddresses(t *testing.T) {
	for addr, want := range map[string]bool{
		"142.250.80.10": true, "2607:f8b0:4004::200a": true,
		"10.0.0.5": false, "192.168.1.10": false, "127.0.0.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "198.18.0.1": false, "240.0.0.1": false, "64:ff9b::a00:5": false, "::1": false,
	} {
		if got := public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("public(%s) = %v, want %v", addr, got, want)
		}
	}
}
