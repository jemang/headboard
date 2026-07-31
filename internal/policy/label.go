package policy

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/util"
	"tailscale.com/tailcfg"
)

// maxExpandedNodes bounds how many nodes a prefix is expanded into before the
// label falls back to showing the prefix. A rule sourced from
// autogroup:member covers 100.64.0.0/10; listing four hundred devices in a
// table cell helps nobody.
const maxExpandedNodes = 8

// EndpointKind tells the UI how to render a label, so it can badge a tagged
// server differently from a person's laptop without parsing the text.
type EndpointKind string

const (
	KindNode     EndpointKind = "node"
	KindNodes    EndpointKind = "nodes"
	KindInternet EndpointKind = "internet"
	KindAny      EndpointKind = "any"
	KindPrefix   EndpointKind = "prefix"
)

// Endpoint is one side of a rule, resolved from a raw CIDR back to whatever
// the operator would recognise.
type Endpoint struct {
	// Raw is exactly what the policy engine emitted.
	Raw string `json:"raw"`

	// Label is the human rendering: a device name, a count, or the prefix
	// itself when it covers too much to name.
	Label string `json:"label"`

	Kind EndpointKind `json:"kind"`

	// NodeIDs are the tailnet nodes this endpoint covers, when few enough
	// to enumerate. The UI links each one.
	NodeIDs []types.NodeID `json:"nodeIds,omitempty"`

	// Owner is the user or tag that owns the endpoint, when it resolves to
	// exactly one node.
	Owner string `json:"owner,omitempty"`
}

// labelIndex resolves addresses back to the nodes that hold them.
type labelIndex struct {
	byAddr map[netip.Addr]types.Node
	nodes  []types.Node
}

func newLabelIndex(nodes []types.Node) *labelIndex {
	idx := &labelIndex{
		byAddr: make(map[netip.Addr]types.Node, len(nodes)*2),
		nodes:  nodes,
	}

	for _, n := range nodes {
		for _, ip := range n.IPs() {
			idx.byAddr[ip] = n
		}
	}

	return idx
}

// endpoint resolves one raw address expression from a filter rule.
//
// The engine does not only emit CIDRs. Expanding a group produces contiguous
// ranges like "100.64.0.1-100.64.0.3", so this parses with Headscale's own
// util.ParseIPSet rather than netip: the label must describe exactly the set
// the engine matched on, in the same syntax it emitted.
func (idx *labelIndex) endpoint(raw string) Endpoint {
	if raw == "*" {
		return Endpoint{Raw: raw, Label: "everything", Kind: KindAny}
	}

	set, err := util.ParseIPSet(raw, nil)
	if err != nil || set == nil {
		return Endpoint{Raw: raw, Label: raw, Kind: KindPrefix}
	}

	for _, p := range set.Prefixes() {
		if p.Bits() == 0 {
			return Endpoint{Raw: raw, Label: "the internet", Kind: KindInternet}
		}
	}

	var covered []types.Node

	for _, n := range idx.nodes {
		for _, ip := range n.IPs() {
			if set.Contains(ip) {
				covered = append(covered, n)
				break
			}
		}
	}

	switch {
	case len(covered) == 0:
		return Endpoint{Raw: raw, Label: raw, Kind: KindPrefix}

	case len(covered) == 1:
		return Endpoint{
			Raw:     raw,
			Label:   covered[0].GivenName,
			Kind:    KindNode,
			NodeIDs: []types.NodeID{covered[0].ID},
			Owner:   ownerOf(covered[0]),
		}

	case len(covered) <= maxExpandedNodes:
		names := make([]string, 0, len(covered))
		ids := make([]types.NodeID, 0, len(covered))

		for _, n := range covered {
			names = append(names, n.GivenName)
			ids = append(ids, n.ID)
		}

		sort.Strings(names)

		return Endpoint{
			Raw:     raw,
			Label:   strings.Join(names, ", "),
			Kind:    KindNodes,
			NodeIDs: ids,
		}

	default:
		ids := make([]types.NodeID, 0, len(covered))
		for _, n := range covered {
			ids = append(ids, n.ID)
		}

		return Endpoint{
			Raw:     raw,
			Label:   fmt.Sprintf("%d devices (%s)", len(covered), raw),
			Kind:    KindNodes,
			NodeIDs: ids,
		}
	}
}

// ownerOf names whoever the node belongs to. Tagged nodes have no user — the
// tags are the identity.
func ownerOf(n types.Node) string {
	if len(n.Tags) > 0 {
		return strings.Join(n.Tags, ", ")
	}

	if n.User != nil {
		return n.User.Username()
	}

	return ""
}

// containsAny reports whether a raw address expression from a filter rule
// covers any of these addresses. Parsed with Headscale's own helper so ranges
// ("100.64.0.1-100.64.0.3") and wildcards behave exactly as the engine
// intended.
func containsAny(raw string, ips []netip.Addr) bool {
	set, err := util.ParseIPSet(raw, nil)
	if err != nil || set == nil {
		return false
	}

	for _, ip := range ips {
		if set.Contains(ip) {
			return true
		}
	}

	return false
}

// formatPorts renders a port range the way an operator writes it in HuJSON.
func formatPorts(p tailcfg.PortRange) string {
	switch {
	case p.First == 0 && p.Last == 65535:
		return "*"
	case p.First == p.Last:
		return fmt.Sprintf("%d", p.First)
	default:
		return fmt.Sprintf("%d-%d", p.First, p.Last)
	}
}
