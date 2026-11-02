package platform

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		name, remote, forwarded, want string
	}{
		{"local proxy", "127.0.0.1:1234", "203.0.113.7", "203.0.113.7"},
		{"IPv6 proxy", "[::1]:1234", "2001:db8::7", "2001:db8::7"},
		{"untrusted peer", "192.0.2.4:1234", "203.0.113.7", "192.0.2.4"},
		{"invalid header", "127.0.0.1:1234", "invalid", "127.0.0.1"},
		{"header list", "127.0.0.1:1234", "203.0.113.7, 192.0.2.4", "127.0.0.1"},
		{"unspecified peer", "0.0.0.0:1234", "203.0.113.7", "0.0.0.0"},
		{"plain address", "192.0.2.4", "", "192.0.2.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Real-IP", tc.forwarded)
			if got := ClientIP(req); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
