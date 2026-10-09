package server

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	proxies := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.10/32"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	tests := []struct {
		name    string
		trusted []netip.Prefix
		remote  string
		xff     []string // one element per header line
		want    string
	}{
		{
			name:   "no proxy configured, no XFF",
			remote: "198.51.100.7:4242",
			want:   "198.51.100.7",
		},
		{
			name:   "no proxy configured, spoofed XFF is ignored",
			remote: "198.51.100.7:4242",
			xff:    []string{"203.0.113.99"},
			want:   "198.51.100.7",
		},
		{
			name:   "no proxy configured, even a loopback peer is not trusted",
			remote: "127.0.0.1:4242",
			xff:    []string{"203.0.113.99"},
			want:   "127.0.0.1",
		},
		{
			name:    "untrusted peer with XFF",
			trusted: proxies,
			remote:  "198.51.100.7:4242",
			xff:     []string{"203.0.113.99, 10.1.2.3"},
			want:    "198.51.100.7",
		},
		{
			name:    "trusted proxy, single XFF",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"203.0.113.5"},
			want:    "203.0.113.5",
		},
		{
			name:    "trusted proxy, client-spoofed leftmost entry is skipped",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"1.2.3.4, 203.0.113.5"},
			want:    "203.0.113.5",
		},
		{
			name:    "chain of trusted proxies",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"1.2.3.4, 203.0.113.5, 192.0.2.10, 10.9.9.9"},
			want:    "203.0.113.5",
		},
		{
			name:    "chain split over several header lines",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"1.2.3.4, 203.0.113.5", "10.9.9.9"},
			want:    "203.0.113.5",
		},
		{
			name:    "IPv6 proxy and client",
			trusted: proxies,
			remote:  "[2001:db8::1]:4242",
			xff:     []string{"2001:db8:ffff::1, [2a00:1450::5]:443, 2001:db8::2"},
			want:    "2a00:1450::5",
		},
		{
			name:    "IPv4-mapped peer counts as IPv4",
			trusted: proxies,
			remote:  "[::ffff:10.0.0.1]:4242",
			xff:     []string{"203.0.113.5"},
			want:    "203.0.113.5",
		},
		{
			name:    "entry with port",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"203.0.113.5:51234"},
			want:    "203.0.113.5",
		},
		{
			name:    "trusted proxy without XFF",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			want:    "10.0.0.1",
		},
		{
			name:    "only trusted addresses in XFF",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"10.2.2.2, 192.0.2.10"},
			want:    "10.0.0.1",
		},
		{
			name:    "malformed rightmost entry falls back to the peer",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"203.0.113.5, not-an-ip"},
			want:    "10.0.0.1",
		},
		{
			name:    "malformed entry behind a trusted hop falls back to the peer",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"garbage, 10.2.2.2"},
			want:    "10.0.0.1",
		},
		{
			name:    "malformed entry left of the client is never reached",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"garbage, 203.0.113.5"},
			want:    "203.0.113.5",
		},
		{
			name:    "empty entries are skipped",
			trusted: proxies,
			remote:  "10.0.0.1:4242",
			xff:     []string{"203.0.113.5, , ", ""},
			want:    "203.0.113.5",
		},
		{
			name:    "unparseable RemoteAddr is returned as is",
			trusted: proxies,
			remote:  "@",
			xff:     []string{"203.0.113.5"},
			want:    "@",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/login", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r, tt.trusted); got != tt.want {
				t.Errorf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
