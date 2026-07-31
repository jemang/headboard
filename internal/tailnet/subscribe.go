package tailnet

import (
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
)

// timeValue aliases time.Time so the nil-safe comparison below reads the same
// as the pointer comparisons next to it.
type timeValue = time.Time

// Subscribe returns a channel of snapshots and a function to stop listening.
//
// The channel has a small buffer and drops rather than blocks. A browser that
// cannot keep up is not a reason to stall the poller for everyone else, and a
// dropped intermediate snapshot costs nothing: each one is complete on its own,
// so the next delivery is still correct.
func (w *Watcher) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 1)

	w.subsMu.Lock()

	id := w.nextID
	w.nextID++
	w.subs[id] = ch

	w.subsMu.Unlock()

	return ch, func() {
		w.subsMu.Lock()
		defer w.subsMu.Unlock()

		if c, ok := w.subs[id]; ok {
			delete(w.subs, id)
			close(c)
		}
	}
}

// Subscribers reports how many listeners are attached.
func (w *Watcher) Subscribers() int {
	w.subsMu.Lock()
	defer w.subsMu.Unlock()

	return len(w.subs)
}

func (w *Watcher) broadcast(s Snapshot) {
	w.subsMu.Lock()
	defer w.subsMu.Unlock()

	for _, ch := range w.subs {
		select {
		case ch <- s:
		default:
			// Slow reader. It will get the next one.
		}
	}
}

// sameNodes compares the fields a browser renders. It is deliberately wider
// than the policy comparison in internal/policy: online state and last-seen do
// not affect the rules, but they are the whole point of a live device list.
func sameNodes(a, b []types.Node) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		x, y := a[i], b[i]

		if x.ID != y.ID || x.GivenName != y.GivenName || x.Hostname != y.Hostname {
			return false
		}

		if !eqBool(x.IsOnline, y.IsOnline) || !eqTime(x.LastSeen, y.LastSeen) {
			return false
		}

		if !eqTime(x.Expiry, y.Expiry) || !eqUint(x.UserID, y.UserID) {
			return false
		}

		if len(x.Tags) != len(y.Tags) {
			return false
		}

		for j := range x.Tags {
			if x.Tags[j] != y.Tags[j] {
				return false
			}
		}

		if len(x.ApprovedRoutes) != len(y.ApprovedRoutes) {
			return false
		}

		for j := range x.ApprovedRoutes {
			if x.ApprovedRoutes[j] != y.ApprovedRoutes[j] {
				return false
			}
		}

		if !sameRoutable(x, y) {
			return false
		}
	}

	return true
}

func sameRoutable(a, b types.Node) bool {
	var ar, br int

	if a.Hostinfo != nil {
		ar = len(a.Hostinfo.RoutableIPs)
	}

	if b.Hostinfo != nil {
		br = len(b.Hostinfo.RoutableIPs)
	}

	if ar != br {
		return false
	}

	if ar == 0 {
		return true
	}

	for i := range a.Hostinfo.RoutableIPs {
		if a.Hostinfo.RoutableIPs[i] != b.Hostinfo.RoutableIPs[i] {
			return false
		}
	}

	return true
}

func sameUsers(a, b []types.User) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name ||
			a[i].Email != b[i].Email || a[i].DisplayName != b[i].DisplayName ||
			a[i].ProviderIdentifier != b[i].ProviderIdentifier {
			return false
		}
	}

	return true
}

func eqBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

func eqUint(a, b *uint) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

func eqTime(a, b *timeValue) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Equal(*b)
}
