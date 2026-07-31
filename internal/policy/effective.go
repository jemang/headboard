package policy

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
)

// EffectiveRule is one compiled filter rule, with both sides resolved to labels.
//
// It is not a policy document rule: the engine merges and reduces ACL entries
// on the way to the wire format, so one EffectiveRule can descend from several `acls`
// entries and vice versa. Attribute maps a decision back to the document.
type EffectiveRule struct {
	// Protocols is empty when the rule covers all protocols.
	Protocols []int `json:"protocols,omitempty"`

	Sources []Endpoint `json:"sources"`
	Dests   []Dest     `json:"dests"`
}

// Dest is a destination endpoint together with the ports it opens.
type Dest struct {
	Endpoint

	// Ports is the rendered range: "443", "1000-2000" or "*".
	Ports string `json:"ports"`
}

// Peer is one node another node can see.
type Peer struct {
	ID        types.NodeID `json:"id"`
	GivenName string       `json:"givenName"`
	Owner     string       `json:"owner,omitempty"`
	IPs       []string     `json:"ips"`
	Online    bool         `json:"online"`
}

// ErrUnknownNode is returned for a node id that is not in the tailnet.
var ErrUnknownNode = fmt.Errorf("unknown node")

// Inbound returns the rules that let other devices reach this one — the
// question "who can reach my laptop". This is the reduced rule set Headscale
// would actually hand this node.
func (m *Manager) Inbound(id types.NodeID) ([]EffectiveRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownNode, id)
	}

	rules, err := m.pm.FilterForNode(node)
	if err != nil {
		return nil, fmt.Errorf("filter for node %d: %w", id, err)
	}

	return m.decorate(rules), nil
}

// Outbound returns the rules that let this device reach others.
//
// There is no FilterFromNode in the engine — the wire format is written from
// the receiver's point of view — so this asks every peer what it would accept
// and keeps the answers that name this node as a source.
//
// Using the tailnet-wide Filter() instead would be cheaper and wrong: rules
// with autogroup:self destinations are compiled per node and never appear in
// the global filter, so "what can my laptop reach" would omit the user's own
// other devices.
func (m *Manager) Outbound(id types.NodeID) ([]EffectiveRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownNode, id)
	}

	ips := node.IPs()
	seen := make(map[string]struct{})

	var out []tailcfg.FilterRule

	for _, peer := range m.pm.BuildPeerMap(m.views)[id] {
		rules, err := m.pm.FilterForNode(peer)
		if err != nil {
			return nil, fmt.Errorf("filter for peer %d: %w", peer.ID(), err)
		}

		for _, r := range rules {
			if !sourcedFrom(r, ips) {
				continue
			}

			// Peers share rules, so the same rule arrives once per
			// peer it applies to.
			key := ruleKey(r)
			if _, dup := seen[key]; dup {
				continue
			}

			seen[key] = struct{}{}

			out = append(out, r)
		}
	}

	return m.decorate(out), nil
}

func sourcedFrom(rule tailcfg.FilterRule, ips []netip.Addr) bool {
	for _, s := range rule.SrcIPs {
		if s == "*" || containsAny(s, ips) {
			return true
		}
	}

	return false
}

// ruleKey identifies a compiled rule for deduplication. Rules are small and
// fully described by their sources and destinations.
func ruleKey(rule tailcfg.FilterRule) string {
	var b strings.Builder

	for _, s := range rule.SrcIPs {
		b.WriteString(s)
		b.WriteByte(',')
	}

	b.WriteByte('>')

	for _, dp := range rule.DstPorts {
		fmt.Fprintf(&b, "%s:%d-%d,", dp.IP, dp.Ports.First, dp.Ports.Last)
	}

	return b.String()
}

// Peers returns the nodes this device is allowed to see — literally the peer
// list Headscale puts in its map response, not an approximation derived from
// the rules.
func (m *Manager) Peers(id types.NodeID) ([]Peer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.byID[id]; !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownNode, id)
	}

	peerMap := m.pm.BuildPeerMap(m.views)

	peers := make([]Peer, 0, len(peerMap[id]))

	for _, p := range peerMap[id] {
		ips := make([]string, 0, 2)
		for _, ip := range p.IPs() {
			ips = append(ips, ip.String())
		}

		peer := Peer{
			ID:        p.ID(),
			GivenName: p.GivenName(),
			IPs:       ips,
			Online:    p.IsOnline().Get(),
		}

		if n, ok := m.nodeByID(p.ID()); ok {
			peer.Owner = ownerOf(n)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}

// A dual-stack node is covered by one IPv4 range and one IPv6 range, and the
// engine emits both. Resolved back to labels they are the same devices, so
// rendering them verbatim shows every name twice — "alice-laptop, alice-phone,
// alice-laptop, alice-phone". Merge on what the endpoint resolved to, keeping
// each raw expression so the underlying set is still inspectable.
func appendEndpoint(list []Endpoint, e Endpoint) []Endpoint {
	for i := range list {
		if !sameEndpoint(list[i], e) {
			continue
		}

		list[i].Raw += ", " + e.Raw

		return list
	}

	return append(list, e)
}

func appendDest(list []Dest, d Dest) []Dest {
	for i := range list {
		if list[i].Ports != d.Ports || !sameEndpoint(list[i].Endpoint, d.Endpoint) {
			continue
		}

		list[i].Raw += ", " + d.Raw

		return list
	}

	return append(list, d)
}

func sameEndpoint(a, b Endpoint) bool {
	if a.Kind != b.Kind || a.Label != b.Label {
		return false
	}

	if len(a.NodeIDs) != len(b.NodeIDs) {
		return false
	}

	for i := range a.NodeIDs {
		if a.NodeIDs[i] != b.NodeIDs[i] {
			return false
		}
	}

	return true
}

func (m *Manager) nodeByID(id types.NodeID) (types.Node, bool) {
	for _, n := range m.nodes {
		if n.ID == id {
			return n, true
		}
	}

	return types.Node{}, false
}

// decorate resolves raw CIDRs in compiled rules back to labels. Callers hold
// at least a read lock.
func (m *Manager) decorate(rules []tailcfg.FilterRule) []EffectiveRule {
	out := make([]EffectiveRule, 0, len(rules))

	for _, r := range rules {
		rule := EffectiveRule{
			Protocols: r.IPProto,
			Sources:   make([]Endpoint, 0, len(r.SrcIPs)),
			Dests:     make([]Dest, 0, len(r.DstPorts)),
		}

		for _, src := range r.SrcIPs {
			rule.Sources = appendEndpoint(rule.Sources, m.labels.endpoint(src))
		}

		for _, dp := range r.DstPorts {
			rule.Dests = appendDest(rule.Dests, Dest{
				Endpoint: m.labels.endpoint(dp.IP),
				Ports:    formatPorts(dp.Ports),
			})
		}

		out = append(out, rule)
	}

	return out
}
