package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
)

// clientIP returns the address of the client that made r.
//
// Forwarding headers are only believed when the request comes straight
// from a trusted proxy (PONY_TRUSTED_PROXIES, e.g. 127.0.0.1/32 for Nginx
// on the same box); anyone else can put anything in them, so trusting them
// would let an attacker pick a fresh IP, and a fresh login allowance, on
// every request. From a trusted proxy, X-Real-IP is preferred; otherwise
// the rightmost X-Forwarded-For entry that isn't itself a trusted proxy
// (entries to its left were supplied by the client).
func clientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	peer := remote.Addr().Unmap()
	if !isTrusted(peer, trusted) {
		return peer
	}
	if ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
		return ip.Unmap()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // garbage: stop rather than trust anything further left
		}
		if ip = ip.Unmap(); !isTrusted(ip, trusted) {
			return ip
		}
	}
	return peer
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies parses a comma-separated list of CIDR prefixes
// ("127.0.0.1/32,::1/128"). Empty means trust no proxy.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
