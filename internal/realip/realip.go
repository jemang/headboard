// Package realip sets the request's client address from forwarding headers,
// but only when the request arrived from a proxy the operator listed.
//
// chi's RealIP believes X-Forwarded-For from anyone, so any client could pick
// the address the sign-in rate limiter and the audit log see. Here a header is
// ignored unless the direct peer is trusted.
package realip

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Parse reads a comma-separated list of CIDRs or single addresses.
func Parse(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix

	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if !strings.Contains(part, "/") {
			addr, err := netip.ParseAddr(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address or CIDR", part)
			}
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))

			continue
		}

		p, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not an address or CIDR", part)
		}
		out = append(out, p.Masked())
	}

	return out, nil
}

// Middleware rewrites RemoteAddr to the client's address when the direct peer
// is a trusted proxy. With no trusted proxies it changes nothing.
func Middleware(trusted []netip.Prefix) func(http.Handler) http.Handler {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a.Unmap()) {
				return true
			}
		}

		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peer, ok := addrOf(r.RemoteAddr); ok && isTrusted(peer) {
				if client, ok := clientFrom(r.Header, isTrusted); ok {
					r.RemoteAddr = net.JoinHostPort(client.String(), "0")
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientFrom walks X-Forwarded-For from the right, past our own proxies; the
// first other address is the client. Entries further left were written by the
// client itself and prove nothing. X-Real-IP is the fallback.
func clientFrom(h http.Header, isTrusted func(netip.Addr) bool) (netip.Addr, bool) {
	if xff := h.Values("X-Forwarded-For"); len(xff) > 0 {
		hops := strings.Split(strings.Join(xff, ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				return netip.Addr{}, false
			}
			if !isTrusted(a) {
				return a.Unmap(), true
			}
		}

		return netip.Addr{}, false
	}

	if a, err := netip.ParseAddr(strings.TrimSpace(h.Get("X-Real-IP"))); err == nil {
		return a.Unmap(), true
	}

	return netip.Addr{}, false
}

func addrOf(remote string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}

	a, err := netip.ParseAddr(host)

	return a, err == nil
}
