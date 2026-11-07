package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	nginx := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	tests := []struct {
		name    string
		remote  string
		realIP  string
		xff     string
		trusted []netip.Prefix
		want    string
	}{
		{"direct client", "203.0.113.7:4000", "", "", nginx, "203.0.113.7"},
		{"direct client spoofing headers", "203.0.113.7:4000", "1.2.3.4", "5.6.7.8", nginx, "203.0.113.7"},
		{"no proxies trusted: headers ignored", "127.0.0.1:4000", "1.2.3.4", "5.6.7.8", nil, "127.0.0.1"},
		{"trusted proxy, X-Real-IP", "127.0.0.1:4000", "198.51.100.9", "", nginx, "198.51.100.9"},
		{"trusted proxy, client-supplied entries left of the real one", "127.0.0.1:4000", "", "6.6.6.6, 198.51.100.9", nginx, "198.51.100.9"},
		{"trusted proxy, garbage X-Real-IP falls back", "127.0.0.1:4000", "not-an-ip", "198.51.100.9", nginx, "198.51.100.9"},
		{"trusted proxy, no headers", "127.0.0.1:4000", "", "", nginx, "127.0.0.1"},
		{"IPv6 client", "[2001:db8::1]:4000", "", "", nginx, "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := clientIP(r, tt.trusted).String(); got != tt.want {
				t.Errorf("clientIP = %s, want %s", got, tt.want)
			}
		})
	}
}
