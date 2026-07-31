// Package spike guards Headboard's load-bearing assumption: that Headscale's
// policy engine can be driven as a library, from data Headboard can obtain over
// the REST API, to produce per-node effective rules.
//
// If this test stops compiling or passing after a Headscale version bump, the
// effective-rules and simulator features are broken and the bump must not ship.
// It lives outside internal/policy on purpose — it tests the upstream contract,
// not Headboard's wrapper around it.
package spike

import (
	"net/netip"
	"testing"
	"time"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/views"
)

func ptr[T any](v T) *T { return &v }

// policyWithComments is written the way an operator would write it: HuJSON with
// comments and trailing commas. Headboard must round-trip this untouched.
const policyWithComments = `{
  // Who owns which tags.
  "tagOwners": {
    "tag:prod": ["bob@"],
  },
  "groups": {
    "group:eng": ["alice@"],
  },
  "acls": [
    // Engineers reach production over HTTPS only.
    {"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]},
    // Everyone reaches their own devices.
    {"action": "accept", "src": ["autogroup:member"], "dst": ["autogroup:self:*"]},
  ],
}`

// tailnet builds a two-node tailnet: alice's laptop, and a bob-owned server
// tagged tag:prod that advertises an approved subnet route.
func tailnet() (users []types.User, laptop, server types.Node, nodes views.Slice[types.NodeView]) {
	users = []types.User{
		{Name: "alice", DisplayName: "Alice", Email: "alice@example.com"},
		{Name: "bob", DisplayName: "Bob", Email: "bob@example.com"},
	}
	users[0].ID = 1
	users[1].ID = 2

	laptop = types.Node{
		ID:        1,
		Hostname:  "alice-laptop",
		GivenName: "alice-laptop",
		IPv4:      ptr(netip.MustParseAddr("100.64.0.1")),
		UserID:    ptr(uint(1)),
		User:      &users[0],
		Expiry:    ptr(time.Now().Add(90 * 24 * time.Hour)),
	}

	server = types.Node{
		ID:        2,
		Hostname:  "prod-web",
		GivenName: "prod-web",
		IPv4:      ptr(netip.MustParseAddr("100.64.0.2")),
		UserID:    ptr(uint(2)),
		User:      &users[1],
		Tags:      types.Strings{"tag:prod"},
		// The REST API exposes availableRoutes but not Hostinfo, so
		// internal/hs synthesises this. SubnetRoutes() and IsExitNode()
		// read it, so the shape matters.
		Hostinfo: &tailcfg.Hostinfo{
			RoutableIPs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
		},
		ApprovedRoutes: types.Prefixes{netip.MustParsePrefix("10.0.0.0/24")},
	}

	nodes = views.SliceOf([]types.NodeView{laptop.View(), server.View()})

	return users, laptop, server, nodes
}

func TestPolicyEngineDrivesFromAPIShapedData(t *testing.T) {
	users, laptop, server, nodes := tailnet()

	pm, err := policyv2.NewPolicyManager([]byte(policyWithComments), users, nodes)
	if err != nil {
		t.Fatalf("NewPolicyManager: %v", err)
	}

	t.Run("inbound: who can reach prod-web", func(t *testing.T) {
		rules, err := pm.FilterForNode(server.View())
		if err != nil {
			t.Fatalf("FilterForNode: %v", err)
		}

		if len(rules) != 1 {
			t.Fatalf("want 1 rule, got %d: %+v", len(rules), rules)
		}

		// group:eng expanded to alice's IP, tag:prod:443 to the server's.
		if got, want := rules[0].SrcIPs, []string{"100.64.0.1"}; len(got) != 1 || got[0] != want[0] {
			t.Errorf("SrcIPs = %v, want %v", got, want)
		}

		if len(rules[0].DstPorts) != 1 {
			t.Fatalf("want 1 dst, got %+v", rules[0].DstPorts)
		}

		dst := rules[0].DstPorts[0]
		if dst.IP != "100.64.0.2" || dst.Ports.First != 443 || dst.Ports.Last != 443 {
			t.Errorf("dst = %+v, want 100.64.0.2:443", dst)
		}
	})

	t.Run("outbound: what alice-laptop can reach", func(t *testing.T) {
		_, matchers := pm.Filter()

		var found int
		for _, m := range matchers {
			if m.SrcsContainsIPs(laptop.View().IPs()...) {
				found++
			}
		}

		if found == 0 {
			t.Error("no global rule lists alice-laptop as a source")
		}
	})

	t.Run("simulator: alice-laptop reaches prod-web", func(t *testing.T) {
		_, matchers := pm.Filter()

		var reachable bool
		for _, m := range matchers {
			if m.SrcsContainsIPs(laptop.View().IPs()...) && m.DestsContainsIP(server.View().IPs()...) {
				reachable = true
			}
		}

		if !reachable {
			t.Error("want alice-laptop -> prod-web reachable under group:eng -> tag:prod:443")
		}
	})

	t.Run("peer map", func(t *testing.T) {
		peers := pm.BuildPeerMap(nodes)

		if got := len(peers[laptop.ID]); got != 1 {
			t.Errorf("alice-laptop sees %d peers, want 1", got)
		}
	})

	// Tag ownership scopes tagged nodes in the member portal: a tagged node has
	// no owning user, so visibility follows tagOwners instead.
	t.Run("tag ownership", func(t *testing.T) {
		if !pm.UserCanHaveTag(users[1].View(), "tag:prod") {
			t.Error("bob owns tag:prod per tagOwners, want true")
		}

		if pm.UserCanHaveTag(users[0].View(), "tag:prod") {
			t.Error("alice does not own tag:prod, want false")
		}
	})
}
