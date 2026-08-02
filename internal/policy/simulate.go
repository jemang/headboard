package policy

import (
	"fmt"
	"net/netip"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/views"
)

// Simulation answers "can A reach B on port N", and says why.
type Simulation struct {
	Source Endpoint `json:"source"`
	Dest   Endpoint `json:"dest"`
	Port   uint16   `json:"port"`

	Allowed bool `json:"allowed"`

	// LiteralDestination marks an answer for an address that is not a
	// Tailnet node. Route approval and packet forwarding are outside the
	// policy engine and must be presented separately by the UI.
	LiteralDestination bool `json:"literalDestination,omitempty"`

	// Because points at the policy entry responsible for an allow. Nil when
	// the connection is denied, or when no single entry could be credited.
	Because *Attribution `json:"because,omitempty"`

	// Rule is the compiled rule that permitted the connection, with both
	// sides resolved to labels.
	Rule *EffectiveRule `json:"rule,omitempty"`
}

// Simulate evaluates a connection against the current policy.
//
// The evaluation runs against the *destination's* filter, not the tailnet-wide
// one. That is not an optimisation, it is a correctness requirement: rules with
// autogroup:self destinations are compiled per node and contribute nothing to
// the global filter (globalFilterRules keeps only the non-self portion of a
// self grant). Simulating against Filter() therefore reports "denied" for every
// connection between two devices owned by the same person — which is the single
// most common thing a member will check.
func (m *Manager) Simulate(src, dst types.NodeID, port uint16) (Simulation, error) {
	m.mu.RLock()

	srcNode, srcOK := m.byID[src]
	dstNode, dstOK := m.byID[dst]

	if !srcOK {
		m.mu.RUnlock()

		return Simulation{}, fmt.Errorf("%w: source %d", ErrUnknownNode, src)
	}

	if !dstOK {
		m.mu.RUnlock()

		return Simulation{}, fmt.Errorf("%w: destination %d", ErrUnknownNode, dst)
	}

	pm, labels := m.pm, m.labels
	m.mu.RUnlock()

	sim := Simulation{
		Source: labels.endpoint(firstIP(srcNode)),
		Dest:   labels.endpoint(firstIP(dstNode)),
		Port:   port,
	}

	match, err := firstAllowingRule(pm, srcNode, dstNode, port)
	if err != nil {
		return Simulation{}, err
	}

	if match == nil {
		return sim, nil
	}

	sim.Allowed = true

	decorated := m.decorateOne(*match)
	sim.Rule = &decorated

	because, err := m.attribute(srcNode, dstNode, port)
	if err != nil {
		// Attribution is a convenience for the editor. Failing to name
		// the responsible line must not turn a correct answer into an
		// error.
		return sim, nil
	}

	sim.Because = because

	return sim, nil
}

// SimulateIP evaluates a connection from one Tailnet node to a literal IP
// address. A literal destination has no Tailnet identity, so normal grants
// come from the global filter while via grants come from filters compiled for
// routers that serve the destination's approved route.
func (m *Manager) SimulateIP(src types.NodeID, dst netip.Addr, port uint16) (Simulation, error) {
	m.mu.RLock()

	srcNode, srcOK := m.byID[src]
	if !srcOK {
		m.mu.RUnlock()

		return Simulation{}, fmt.Errorf("%w: source %d", ErrUnknownNode, src)
	}

	pm, labels, nodes := m.pm, m.labels, m.views
	m.mu.RUnlock()

	sim := Simulation{
		Source:             labels.endpoint(firstIP(srcNode)),
		Dest:               labels.endpoint(dst.String()),
		Port:               port,
		LiteralDestination: true,
	}

	match := firstAllowingIPRule(pm, nodes, srcNode, dst, port)
	if match == nil {
		return sim, nil
	}

	sim.Allowed = true

	decorated := m.decorateOne(*match)
	sim.Rule = &decorated

	because, err := m.attributeIP(nodes, srcNode, dst, port)
	if err != nil {
		return sim, nil
	}

	sim.Because = because

	return sim, nil
}

// firstAllowingRule returns the first rule in the destination's filter that
// permits src to reach dst on port, or nil if none does.
func firstAllowingRule(
	pm *policyv2.PolicyManager,
	src, dst types.NodeView,
	port uint16,
) (*tailcfg.FilterRule, error) {
	rules, err := pm.FilterForNode(dst)
	if err != nil {
		return nil, fmt.Errorf("filter for node %d: %w", dst.ID(), err)
	}

	srcIPs, dstIPs := src.IPs(), dst.IPs()

	for i := range rules {
		if allows(rules[i], srcIPs, dstIPs, port) {
			return &rules[i], nil
		}
	}

	return nil, nil
}

// filterRulesForAddr returns rules that can apply to a literal address.
// Headscale omits via grants from Filter(), so router-specific filters are
// included for nodes that serve an approved subnet or exit route containing
// the address.
func filterRulesForAddr(
	pm *policyv2.PolicyManager,
	nodes views.Slice[types.NodeView],
	dst netip.Addr,
) []tailcfg.FilterRule {
	rules, _ := pm.Filter()

	for _, node := range nodes.All() {
		for _, route := range node.AllApprovedRoutes() {
			if !route.Contains(dst) {
				continue
			}

			perNode, err := pm.FilterForNode(node)
			if err == nil {
				rules = append(rules, perNode...)
			}

			break
		}
	}

	return rules
}

func firstAllowingIPRule(
	pm *policyv2.PolicyManager,
	nodes views.Slice[types.NodeView],
	src types.NodeView,
	dst netip.Addr,
	port uint16,
) *tailcfg.FilterRule {
	rules := filterRulesForAddr(pm, nodes, dst)

	for i := range rules {
		if allows(rules[i], src.IPs(), []netip.Addr{dst}, port) {
			return &rules[i]
		}
	}

	return nil
}

// attribute finds the policy entry responsible for allowing a connection, by
// asking each entry on its own.
func (m *Manager) attribute(src, dst types.NodeView, port uint16) (*Attribution, error) {
	entries, err := m.attributionFor()
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		match, err := firstAllowingRule(e.pm, src, dst, port)
		if err != nil || match == nil {
			continue
		}

		return &Attribution{
			Section: e.section,
			Index:   e.index,
			Pointer: fmt.Sprintf("/%s/%d", e.section, e.index),
		}, nil
	}

	return nil, nil
}

func (m *Manager) attributeIP(
	nodes views.Slice[types.NodeView],
	src types.NodeView,
	dst netip.Addr,
	port uint16,
) (*Attribution, error) {
	entries, err := m.attributionFor()
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if firstAllowingIPRule(e.pm, nodes, src, dst, port) == nil {
			continue
		}

		return &Attribution{
			Section: e.section,
			Index:   e.index,
			Pointer: fmt.Sprintf("/%s/%d", e.section, e.index),
		}, nil
	}

	return nil, nil
}

// allows reports whether one compiled rule covers this connection.
//
// Source and destination are checked against the rule's own strings rather than
// through a matcher.Match, because Match merges every destination into a single
// IP set and drops the ports with them — so a rule opening only :443 would look
// like it opened :22 as well.
func allows(rule tailcfg.FilterRule, srcIPs, dstIPs []netip.Addr, port uint16) bool {
	var srcMatched bool

	for _, s := range rule.SrcIPs {
		if s == "*" || containsAny(s, srcIPs) {
			srcMatched = true

			break
		}
	}

	if !srcMatched {
		return false
	}

	for _, dp := range rule.DstPorts {
		if port < dp.Ports.First || port > dp.Ports.Last {
			continue
		}

		if dp.IP == "*" || containsAny(dp.IP, dstIPs) {
			return true
		}
	}

	return false
}

func (m *Manager) decorateOne(rule tailcfg.FilterRule) EffectiveRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.decorate([]tailcfg.FilterRule{rule})[0]
}

func firstIP(n types.NodeView) string {
	ips := n.IPs()
	if len(ips) == 0 {
		return ""
	}

	return ips[0].String()
}
