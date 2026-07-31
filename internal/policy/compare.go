package policy

import (
	"net/netip"

	"github.com/juanfont/headscale/hscontrol/types"
)

// The comparisons below exist so that a poll tick which changes nothing the
// policy engine reads does not rebuild the engine. Rebuilding is not just
// wasted work: it invalidates the per-rule attribution cache and pushes a
// no-op change out to every connected browser.

func equalPtr[T comparable](a, b *T) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func equalAddrPtr(a, b *netip.Addr) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func equalSlice(a, b types.Strings) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func equalPrefixes(a, b types.Prefixes) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// equalRoutable compares the synthesised Hostinfo.RoutableIPs, which is what
// SubnetRoutes() and IsExitNode() read. It is the only part of Hostinfo
// Headboard populates.
func equalRoutable(a, b types.Node) bool {
	var ar, br []netip.Prefix

	if a.Hostinfo != nil {
		ar = a.Hostinfo.RoutableIPs
	}

	if b.Hostinfo != nil {
		br = b.Hostinfo.RoutableIPs
	}

	if len(ar) != len(br) {
		return false
	}

	for i := range ar {
		if ar[i] != br[i] {
			return false
		}
	}

	return true
}
