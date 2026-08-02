package policy

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
)

func ptr[T any](v T) *T { return &v }

// devPolicy mirrors dev/policy.hujson: comments, trailing commas and all. The
// bridge must cope with the document an operator actually writes, not a
// normalised one.
const devPolicy = `{
  // Who is allowed to claim which tag.
  "tagOwners": {
    "tag:prod": ["ops@"],
  },
  "groups": {
    "group:eng": ["alice@", "bob@"],
    "group:ops": ["ops@"],
  },
  "hosts": {
    "office-lan": "10.0.0.0/24",
  },
  "acls": [
    // 0: engineers reach production over HTTPS only.
    {"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]},
    // 1: ops gets SSH to production.
    {"action": "accept", "src": ["group:ops"], "dst": ["tag:prod:22"]},
    // 2: everyone reaches their own devices.
    {"action": "accept", "src": ["autogroup:member"], "dst": ["autogroup:self:*"]},
  ],
}`

// tailnet mirrors what dev/up.sh seeds, in the shape internal/hs produces.
func tailnet() ([]types.User, []types.Node) {
	users := []types.User{
		{Name: "alice", DisplayName: "Alice", Email: "alice@headboard.test"},
		{Name: "bob", DisplayName: "Bob", Email: "bob@headboard.test"},
		{Name: "ops", DisplayName: "Ops", Email: "ops@headboard.test"},
	}
	users[0].ID, users[1].ID, users[2].ID = 1, 2, 3

	node := func(id uint64, name string, ip string, owner *types.User, tags ...string) types.Node {
		n := types.Node{
			ID:        types.NodeID(id),
			Hostname:  name,
			GivenName: name,
			IPv4:      ptr(netip.MustParseAddr(ip)),
			Tags:      types.Strings(tags),
			Hostinfo:  &tailcfg.Hostinfo{Hostname: name},
			IsOnline:  ptr(false),
		}

		if owner != nil {
			n.UserID = &owner.ID
			n.User = owner
		}

		return n
	}

	gw := node(4, "ops-gw", "100.64.0.4", &users[2])
	gw.Hostinfo.RoutableIPs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}
	gw.ApprovedRoutes = types.Prefixes{netip.MustParsePrefix("10.0.0.0/24")}

	return users, []types.Node{
		node(1, "alice-laptop", "100.64.0.1", &users[0]),
		node(2, "alice-phone", "100.64.0.2", &users[0]),
		node(3, "bob-desktop", "100.64.0.3", &users[1]),
		gw,
		// Tagged: no owner, exactly as internal/hs maps the sentinel.
		node(5, "prod-web", "100.64.0.5", nil, "tag:prod"),
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	users, nodes := tailnet()

	m, err := New(devPolicy, users, nodes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return m
}

func TestInboundResolvesToLabels(t *testing.T) {
	m := newTestManager(t)

	rules, err := m.Inbound(5) // prod-web
	if err != nil {
		t.Fatalf("Inbound: %v", err)
	}

	if len(rules) == 0 {
		t.Fatal("no inbound rules for prod-web")
	}

	// The engine emits raw address expressions — here the contiguous range
	// "100.64.0.1-100.64.0.3" for group:eng. A member reading their own
	// device page must see the devices, not the arithmetic.
	var sawLaptop, saw443 bool

	for _, r := range rules {
		for _, s := range r.Sources {
			if s.Kind == KindPrefix {
				t.Errorf("source %q was not resolved to any device", s.Raw)
			}

			for _, id := range s.NodeIDs {
				if id == 1 {
					sawLaptop = true
				}
			}
		}

		for _, d := range r.Dests {
			if d.Label == "prod-web" && d.Ports == "443" {
				saw443 = true
			}
		}
	}

	if !sawLaptop {
		t.Errorf("no source covers alice-laptop: %+v", rules)
	}

	if !saw443 {
		t.Errorf("no destination resolved to prod-web:443: %+v", rules)
	}
}

// Group expansion produces contiguous ranges rather than CIDRs, and netip
// cannot parse them. Leaving them unresolved would put "100.64.0.1-100.64.0.3"
// in front of a member on their own device page.
func TestRangeSourcesResolveToDevices(t *testing.T) {
	m := newTestManager(t)

	rules, err := m.Inbound(5)
	if err != nil {
		t.Fatalf("Inbound: %v", err)
	}

	var found bool

	for _, r := range rules {
		for _, s := range r.Sources {
			if !strings.Contains(s.Raw, "-") {
				continue
			}

			found = true

			if s.Kind != KindNodes {
				t.Errorf("range %q: Kind = %s, want %s", s.Raw, s.Kind, KindNodes)
			}

			if len(s.NodeIDs) != 3 {
				t.Errorf("range %q: covers %v, want the three group:eng devices", s.Raw, s.NodeIDs)
			}

			if s.Label == s.Raw {
				t.Errorf("range %q: label is still the raw range", s.Raw)
			}
		}
	}

	if !found {
		t.Skip("this policy did not produce a range source")
	}
}

func TestOutboundIsFilteredBySource(t *testing.T) {
	m := newTestManager(t)

	rules, err := m.Outbound(1) // alice-laptop
	if err != nil {
		t.Fatalf("Outbound: %v", err)
	}

	if len(rules) == 0 {
		t.Fatal("alice-laptop has no outbound rules")
	}

	// alice is in group:eng, so tag:prod:443 must appear. She is not in
	// group:ops, so :22 must not.
	var saw443, saw22 bool

	for _, r := range rules {
		for _, d := range r.Dests {
			if d.Label != "prod-web" {
				continue
			}

			switch d.Ports {
			case "443":
				saw443 = true
			case "22":
				saw22 = true
			}
		}
	}

	if !saw443 {
		t.Error("alice-laptop cannot reach prod-web:443, but group:eng grants it")
	}

	if saw22 {
		t.Error("alice-laptop reaches prod-web:22, but only group:ops has SSH")
	}
}

func TestPeersComeFromTheEngine(t *testing.T) {
	m := newTestManager(t)

	peers, err := m.Peers(1)
	if err != nil {
		t.Fatalf("Peers: %v", err)
	}

	names := map[string]bool{}
	for _, p := range peers {
		names[p.GivenName] = true
	}

	// alice-laptop can see prod-web (group:eng -> tag:prod:443) and
	// alice-phone (autogroup:self).
	for _, want := range []string{"prod-web", "alice-phone"} {
		if !names[want] {
			t.Errorf("alice-laptop cannot see %s: got %v", want, names)
		}
	}

	if names["alice-laptop"] {
		t.Error("alice-laptop lists itself as a peer")
	}
}

func TestSimulateMatchesPolicy(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		name        string
		src, dst    types.NodeID
		port        uint16
		wantAllowed bool
		wantSection string
		wantIndex   int
	}{
		{"eng reaches prod on 443", 1, 5, 443, true, "acls", 0},
		{"ops reaches prod on 22", 4, 5, 22, true, "acls", 1},
		// The rule that opens 443 must not be read as opening everything.
		{"eng does not reach prod on 22", 1, 5, 22, false, "", 0},
		{"eng does not reach prod on 80", 1, 5, 80, false, "", 0},
		{"own devices reach each other", 1, 2, 22, true, "acls", 2},
		{"alice does not reach bob", 1, 3, 22, false, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim, err := m.Simulate(tt.src, tt.dst, tt.port)
			if err != nil {
				t.Fatalf("Simulate: %v", err)
			}

			if sim.Allowed != tt.wantAllowed {
				t.Fatalf("Allowed: got %v, want %v", sim.Allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed {
				if sim.Because != nil {
					t.Errorf("Because: got %+v on a denied connection", sim.Because)
				}

				return
			}

			if sim.Because == nil {
				t.Fatal("Because: got nil, want the responsible policy entry")
			}

			if sim.Because.Section != tt.wantSection || sim.Because.Index != tt.wantIndex {
				t.Errorf("Because: got %s/%d, want %s/%d",
					sim.Because.Section, sim.Because.Index, tt.wantSection, tt.wantIndex)
			}

			wantPointer := "/" + tt.wantSection + "/" + itoa(tt.wantIndex)
			if sim.Because.Pointer != wantPointer {
				t.Errorf("Pointer: got %q, want %q", sim.Because.Pointer, wantPointer)
			}
		})
	}
}

func TestSimulateIPMatchesLiteralAndViaGrants(t *testing.T) {
	users, nodes := tailnet()

	agent := nodes[0]
	agent.ID = 6
	agent.Hostname = "agent"
	agent.GivenName = "agent"
	agent.IPv4 = ptr(netip.MustParseAddr("100.64.0.6"))
	agent.Tags = types.Strings{"tag:agent"}
	nodes[3].Tags = types.Strings{"tag:router"}
	nodes = append(nodes, agent)

	const grants = `{
	  "tagOwners": {
	    "tag:agent": ["ops@"],
	    "tag:router": ["ops@"],
	  },
	  "grants": [
	    {"src": ["tag:agent"], "dst": ["10.0.0.0/24"], "ip": ["22"]},
	  ],
	}`

	const viaGrants = `{
	  "tagOwners": {
	    "tag:agent": ["ops@"],
	    "tag:router": ["ops@"],
	  },
	  "grants": [
	    {"src": ["tag:agent"], "dst": ["10.0.0.0/24"], "ip": ["22"], "via": ["tag:router"]},
	  ],
	}`

	for _, tc := range []struct {
		name   string
		policy string
	}{
		{name: "literal grant", policy: grants},
		{name: "via subnet router", policy: viaGrants},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(tc.policy, users, nodes)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sim, err := m.SimulateIP(6, netip.MustParseAddr("10.0.0.25"), 22)
			if err != nil {
				t.Fatalf("SimulateIP: %v", err)
			}
			if !sim.Allowed || !sim.LiteralDestination {
				t.Fatalf("simulation = %+v, want an allowed literal destination", sim)
			}
			if sim.Because == nil || sim.Because.Pointer != "/grants/0" {
				t.Fatalf("because = %+v, want /grants/0", sim.Because)
			}
		})
	}

	m, err := New(grants, users, nodes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tc := range []struct {
		name string
		dst  netip.Addr
		port uint16
	}{
		{name: "outside subnet", dst: netip.MustParseAddr("10.1.0.25"), port: 22},
		{name: "wrong port", dst: netip.MustParseAddr("10.0.0.25"), port: 443},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim, err := m.SimulateIP(6, tc.dst, tc.port)
			if err != nil {
				t.Fatalf("SimulateIP: %v", err)
			}
			if sim.Allowed || sim.Because != nil {
				t.Fatalf("simulation = %+v, want denial without attribution", sim)
			}
		})
	}
}

// Attribution has to survive a rule order the compiled output does not
// preserve. The engine merges grants on the way to the wire format, so a naive
// "index of the matching FilterRule" would credit the wrong row here.
func TestAttributionSurvivesRuleMerging(t *testing.T) {
	users, nodes := tailnet()

	// Two entries with the same source and different destinations are
	// exactly the shape mergeFilterRules collapses.
	const merged = `{
	  "tagOwners": {"tag:prod": ["ops@"]},
	  "groups": {"group:eng": ["alice@"]},
	  "acls": [
	    {"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]},
	    {"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:8443"]},
	  ],
	}`

	m, err := New(merged, users, nodes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tc := range []struct {
		port  uint16
		index int
	}{{443, 0}, {8443, 1}} {
		sim, err := m.Simulate(1, 5, tc.port)
		if err != nil {
			t.Fatalf("Simulate(:%d): %v", tc.port, err)
		}

		if !sim.Allowed {
			t.Fatalf(":%d: got denied, want allowed", tc.port)
		}

		if sim.Because == nil || sim.Because.Index != tc.index {
			t.Errorf(":%d: attributed to %+v, want acls/%d", tc.port, sim.Because, tc.index)
		}
	}
}

func TestUpdateSkipsCosmeticChanges(t *testing.T) {
	users, nodes := tailnet()

	m, err := New(devPolicy, users, nodes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Online state and last-seen churn on every poll. Rebuilding the engine
	// for them would throw away the attribution cache and push a no-op
	// change to every browser.
	churned := make([]types.Node, len(nodes))
	copy(churned, nodes)
	churned[0].IsOnline = ptr(true)
	churned[0].LastSeen = ptr(time.Now())

	changed, err := m.Update(devPolicy, users, churned)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if changed {
		t.Error("Update reported a change for online/lastSeen churn")
	}

	// A tag change is policy-relevant and must rebuild.
	retagged := make([]types.Node, len(nodes))
	copy(retagged, nodes)
	retagged[4].Tags = types.Strings{"tag:ci"}

	changed, err = m.Update(devPolicy, users, retagged)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !changed {
		t.Error("Update reported no change after a tag was replaced")
	}
}

func TestUnknownNodeIsAnError(t *testing.T) {
	m := newTestManager(t)

	if _, err := m.Inbound(999); err == nil {
		t.Error("Inbound(999): got nil error")
	}

	if _, err := m.Outbound(999); err == nil {
		t.Error("Outbound(999): got nil error")
	}

	if _, err := m.Peers(999); err == nil {
		t.Error("Peers(999): got nil error")
	}

	if _, err := m.Simulate(999, 1, 22); err == nil {
		t.Error("Simulate(999, …): got nil error")
	}
}

// A tests-block destination must name one machine. Getting that wrong is not a
// red assertion: Headscale refuses the document while parsing it, so the whole
// policy stops loading. Grants make it easy to hit, because their destinations
// are subnets and `hosts` fills up with CIDR aliases that read like hosts.
func TestTestDestinationMustBeASingleHost(t *testing.T) {
	users, nodes := tailnet()

	policyWith := func(dst string) string {
		return `{
  "hosts": {"lan": "10.0.0.0/24", "printer": "10.0.0.9", "printer32": "10.0.0.9/32"},
  "grants": [{"src": ["ops@"], "dst": ["10.0.0.0/24"], "ip": ["*"]}],
  "tests": [{"src": "ops@", "accept": ["` + dst + `"]}],
}`
	}

	for _, dst := range []string{"10.0.0.0/24:22", "10.0.0.9/32:22", "lan:22"} {
		if _, err := New(policyWith(dst), users, nodes); err == nil {
			t.Errorf("%s: compiled, want a rejection", dst)
		} else if !strings.Contains(err.Error(), "must be a single host") {
			t.Errorf("%s: unexpected error: %v", dst, err)
		}
	}

	// A prefix is refused even at /32, but an alias holding one is not.
	for _, dst := range []string{"10.0.0.9:22", "printer:22", "printer32:22"} {
		if _, err := New(policyWith(dst), users, nodes); err != nil {
			t.Errorf("%s: rejected, want acceptance: %v", dst, err)
		}
	}
}

// Tailscale's autoApprovers carries a services map; Headscale's does not, and
// its unmarshaller rejects unknown members. The form used to offer a services
// section, which made every save after staging one fail — so the whole policy
// is refused, not the single entry. Keep this red until a Headscale bump adds
// the field on purpose.
func TestServicesAutoApproversAreRejected(t *testing.T) {
	users, nodes := tailnet()

	_, err := New(`{
  "tagOwners": {"tag:prod": ["ops@"]},
  "autoApprovers": {
    "routes": {"10.0.0.0/24": ["tag:prod"]},
    "services": {"svc:example": ["tag:prod"]},
  },
}`, users, nodes)
	if err == nil {
		t.Fatal("a policy with autoApprovers.services compiled, want a rejection")
	}

	if !strings.Contains(err.Error(), "services") {
		t.Errorf("error does not name the offending field: %v", err)
	}
}

func TestTagOwnershipScopesTaggedNodes(t *testing.T) {
	m := newTestManager(t)
	users, _ := tailnet()

	if !m.UserCanHaveTag(users[2].View(), "tag:prod") {
		t.Error("ops owns tag:prod per tagOwners, want true")
	}

	if m.UserCanHaveTag(users[0].View(), "tag:prod") {
		t.Error("alice does not own tag:prod, want false")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}

	return string(b)
}

// A dual-stack node contributes an IPv4 range and an IPv6 range to the same
// rule. Rendered verbatim that shows every device name twice.
func TestDualStackEndpointsAreMerged(t *testing.T) {
	users, nodes := tailnet()

	// Give every node a v6 address too, as a real tailnet has.
	for i := range nodes {
		v6 := netip.MustParseAddr("fd7a:115c:a1e0::" + itoa(int(nodes[i].ID)))
		nodes[i].IPv6 = &v6
	}

	m, err := New(devPolicy, users, nodes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rules, err := m.Inbound(5)
	if err != nil {
		t.Fatalf("Inbound: %v", err)
	}

	for _, r := range rules {
		seen := map[string]int{}

		for _, s := range r.Sources {
			seen[s.Label]++
		}

		for label, n := range seen {
			if n > 1 {
				t.Errorf("source %q appears %d times in one rule", label, n)
			}
		}

		dests := map[string]int{}

		for _, d := range r.Dests {
			dests[d.Label+":"+d.Ports]++
		}

		for label, n := range dests {
			if n > 1 {
				t.Errorf("destination %q appears %d times in one rule", label, n)
			}
		}
	}
}
