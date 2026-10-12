package sshsync

import (
	"net/netip"
	"sort"
	"strings"
)

// Server is a policy hosts entry that names exactly one machine.
type Server struct {
	Name string
	IP   string
}

// Servers keeps the hosts entries that are a bare IPv4 address or an IPv4 /32.
// A CIDR names a network, not a machine to put keys on.
func Servers(hosts map[string]string) []Server {
	out := []Server{}

	for name, v := range hosts {
		v = strings.TrimSpace(v)

		if p, err := netip.ParsePrefix(v); err == nil {
			if p.Addr().Is4() && p.Bits() == 32 {
				out = append(out, Server{Name: name, IP: p.Addr().String()})
			}

			continue
		}

		if a, err := netip.ParseAddr(v); err == nil && a.Is4() {
			out = append(out, Server{Name: name, IP: a.String()})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}
