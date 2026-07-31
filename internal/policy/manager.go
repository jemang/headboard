// Package policy answers "which rules apply here" by driving Headscale's own
// policy engine rather than re-implementing it.
//
// Everything in this package is a thin arrangement of calls into
// hscontrol/policy/v2. Alias expansion, autogroup:self, tag resolution, grants
// and rule reduction are Headscale's, so they stay correct on the day Headscale
// changes them — which is the entire reason Headboard is written in Go.
package policy

import (
	"fmt"
	"sync"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/types/views"
)

// Manager wraps Headscale's PolicyManager together with the tailnet it was
// built from, so effective rules can be reported with human labels instead of
// raw CIDRs.
//
// Safe for concurrent use: the poller calls Update while HTTP handlers read.
type Manager struct {
	mu sync.RWMutex

	pm *policyv2.PolicyManager

	// hujson is the policy text this manager was built from, kept so
	// attribution can recompile individual rules from the same source.
	hujson string

	users []types.User
	nodes []types.Node
	views views.Slice[types.NodeView]
	byID  map[types.NodeID]types.NodeView

	labels *labelIndex

	// attributionEntries holds one engine per policy rule, compiled from
	// that rule alone. Built on first use because most requests never need
	// it, and discarded on every rebuild.
	attributionEntries []attributionEntry
	attributionErr     error
	attributionOnce    *sync.Once
}

// New builds a manager from a policy document and the tailnet it applies to.
func New(hujson string, users []types.User, nodes []types.Node) (*Manager, error) {
	m := &Manager{}

	if err := m.rebuild(hujson, users, nodes); err != nil {
		return nil, err
	}

	return m, nil
}

// Update rebuilds the manager when the policy, users or nodes changed. It
// reports whether anything actually changed, so callers can skip fanning out a
// no-op to connected browsers.
func (m *Manager) Update(hujson string, users []types.User, nodes []types.Node) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pm != nil && m.hujson == hujson && sameUsers(m.users, users) && sameNodes(m.nodes, nodes) {
		return false, nil
	}

	if err := m.rebuildLocked(hujson, users, nodes); err != nil {
		return false, err
	}

	return true, nil
}

func (m *Manager) rebuild(hujson string, users []types.User, nodes []types.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.rebuildLocked(hujson, users, nodes)
}

func (m *Manager) rebuildLocked(hujson string, users []types.User, nodes []types.Node) error {
	nodeViews := make([]types.NodeView, 0, len(nodes))
	byID := make(map[types.NodeID]types.NodeView, len(nodes))

	// The views must be taken from a stable backing array: NodeView holds a
	// pointer, so ranging by value and viewing the loop variable would make
	// every view observe the same node on older toolchains and, more
	// importantly, obscures that the slice must outlive the views.
	for i := range nodes {
		v := nodes[i].View()
		nodeViews = append(nodeViews, v)
		byID[nodes[i].ID] = v
	}

	slice := views.SliceOf(nodeViews)

	pm, err := policyv2.NewPolicyManager([]byte(hujson), users, slice)
	if err != nil {
		return fmt.Errorf("building policy manager: %w", err)
	}

	m.pm = pm
	m.hujson = hujson
	m.users = users
	m.nodes = nodes
	m.views = slice
	m.byID = byID
	m.labels = newLabelIndex(nodes)
	m.attributionEntries = nil
	m.attributionErr = nil
	m.attributionOnce = new(sync.Once)

	return nil
}

// Node returns the view for a node id.
func (m *Manager) Node(id types.NodeID) (types.NodeView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.byID[id]

	return v, ok
}

// Nodes returns every node in the tailnet.
func (m *Manager) Nodes() []types.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.nodes
}

// UserCanHaveTag reports whether tagOwners grants this user the tag. Tagged
// nodes have no owning user, so the member portal scopes them by this instead.
func (m *Manager) UserCanHaveTag(user types.UserView, tag string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pm.UserCanHaveTag(user, tag)
}

func sameUsers(a, b []types.User) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name ||
			a[i].ProviderIdentifier != b[i].ProviderIdentifier {
			return false
		}
	}

	return true
}

// sameNodes compares only the fields the policy engine reads. Online state and
// last-seen change constantly and must not force a policy rebuild.
func sameNodes(a, b []types.Node) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		x, y := a[i], b[i]

		if x.ID != y.ID || x.GivenName != y.GivenName {
			return false
		}

		if !equalPtr(x.UserID, y.UserID) || !equalSlice(x.Tags, y.Tags) {
			return false
		}

		if !equalPrefixes(x.ApprovedRoutes, y.ApprovedRoutes) {
			return false
		}

		if !equalAddrPtr(x.IPv4, y.IPv4) || !equalAddrPtr(x.IPv6, y.IPv6) {
			return false
		}

		if !equalRoutable(x, y) {
			return false
		}
	}

	return true
}
