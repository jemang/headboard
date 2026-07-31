package hs

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

// loadNodes decodes a captured /api/v1/node response and maps every node,
// keyed by given name.
func loadNodes(t *testing.T, name string) map[string]types.Node {
	t.Helper()

	var resp struct {
		Nodes []v1Node `json:"nodes"`
	}

	readFixture(t, name, &resp)

	out := make(map[string]types.Node, len(resp.Nodes))

	for _, n := range resp.Nodes {
		node, err := mapNode(n)
		if err != nil {
			t.Fatalf("mapNode(%s): %v", n.GivenName, err)
		}

		out[n.GivenName] = node
	}

	return out
}

func readFixture(t *testing.T, name string, out any) {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
}

func TestMapNodeFromCapturedAPIResponse(t *testing.T) {
	nodes := loadNodes(t, "nodes.json")

	if len(nodes) != 5 {
		t.Fatalf("nodes: got %d, want 5", len(nodes))
	}

	t.Run("user-owned node keeps its owner", func(t *testing.T) {
		n := nodes["alice-laptop"]

		if n.ID != 1 {
			t.Errorf("ID: got %d, want 1", n.ID)
		}

		if n.UserID == nil {
			t.Fatal("UserID: got nil, want alice's id")
		}

		if *n.UserID != 1 {
			t.Errorf("UserID: got %d, want 1", *n.UserID)
		}

		if n.User == nil || n.User.Name != "alice" {
			t.Errorf("User: got %+v, want alice", n.User)
		}

		if n.GivenName != "alice-laptop" || n.Hostname != "alice-laptop" {
			t.Errorf("names: got hostname=%q given=%q", n.Hostname, n.GivenName)
		}
	})

	t.Run("addresses split by family", func(t *testing.T) {
		n := nodes["alice-laptop"]

		if n.IPv4 == nil || n.IPv4.String() != "100.64.0.1" {
			t.Errorf("IPv4: got %v, want 100.64.0.1", n.IPv4)
		}

		if n.IPv6 == nil || n.IPv6.String() != "fd7a:115c:a1e0::1" {
			t.Errorf("IPv6: got %v, want fd7a:115c:a1e0::1", n.IPv6)
		}

		if got := len(n.IPs()); got != 2 {
			t.Errorf("IPs(): got %d, want 2", got)
		}
	})

	// The wire format renders tagged nodes as owned by a sentinel user rather
	// than by nobody. Carrying that through would put a phantom
	// "tagged-devices" account in the member list and hand every tagged node
	// to whoever matched that id.
	t.Run("tagged node has no owner", func(t *testing.T) {
		n := nodes["prod-web"]

		if n.UserID != nil {
			t.Errorf("UserID: got %d, want nil for a tagged node", *n.UserID)
		}

		if n.User != nil {
			t.Errorf("User: got %+v, want nil for a tagged node", n.User)
		}

		if len(n.Tags) != 1 || n.Tags[0] != "tag:prod" {
			t.Errorf("Tags: got %v, want [tag:prod]", n.Tags)
		}
	})

	t.Run("sentinel user id never reaches the model", func(t *testing.T) {
		for name, n := range nodes {
			if n.UserID != nil && *n.UserID == types.TaggedDevicesUserID {
				t.Errorf("%s: UserID is the tagged-devices sentinel (%d)", name, types.TaggedDevicesUserID)
			}
		}
	})

	t.Run("approved routes parse", func(t *testing.T) {
		n := nodes["ops-gw"]

		want := []string{"0.0.0.0/0", "10.0.0.0/24", "::/0"}
		if len(n.ApprovedRoutes) != len(want) {
			t.Fatalf("ApprovedRoutes: got %v, want %v", n.ApprovedRoutes, want)
		}

		for i, w := range want {
			if n.ApprovedRoutes[i].String() != w {
				t.Errorf("ApprovedRoutes[%d]: got %s, want %s", i, n.ApprovedRoutes[i], w)
			}
		}
	})

	// A never-set expiry arrives as 0001-01-01T00:00:00Z, not null. Keeping
	// the zero value would make IsExpired report every node as expired — which
	// is exactly what `headscale nodes list` does in its table.
	t.Run("zero expiry means never expires", func(t *testing.T) {
		for name, n := range nodes {
			if n.Expiry != nil {
				t.Errorf("%s: Expiry: got %v, want nil for an unset expiry", name, n.Expiry)
			}

			if n.IsExpired() {
				t.Errorf("%s: IsExpired() = true for a node that never had an expiry", name)
			}
		}
	})

	t.Run("online state and last seen survive", func(t *testing.T) {
		n := nodes["alice-laptop"]

		if n.IsOnline == nil || *n.IsOnline {
			t.Errorf("IsOnline: got %v, want false", n.IsOnline)
		}

		if n.LastSeen == nil {
			t.Error("LastSeen: got nil, want the captured timestamp")
		}
	})

	t.Run("keys parse", func(t *testing.T) {
		n := nodes["alice-laptop"]

		if n.MachineKey.String() == "" {
			t.Error("MachineKey: empty after mapping")
		}

		if n.NodeKey.String() == "" {
			t.Error("NodeKey: empty after mapping")
		}
	})
}

// The synthesised Hostinfo is the one piece of the mapping with no
// counterpart in the response, and NodeView.SubnetRoutes()/IsExitNode() are
// the only consumers. Skip it and subnet routers lose their filter rules
// without any error surfacing.
func TestMapNodeSynthesisesHostinfoFromAvailableRoutes(t *testing.T) {
	nodes := loadNodes(t, "nodes_subnet_router.json")

	n, ok := nodes["ops-gw"]
	if !ok {
		t.Fatal("fixture is missing ops-gw")
	}

	if n.Hostinfo == nil {
		t.Fatal("Hostinfo: got nil, want a synthesised value")
	}

	want := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/24"),
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("::/0"),
	}

	if len(n.Hostinfo.RoutableIPs) != len(want) {
		t.Fatalf("RoutableIPs: got %v, want %v", n.Hostinfo.RoutableIPs, want)
	}

	for i, w := range want {
		if n.Hostinfo.RoutableIPs[i] != w {
			t.Errorf("RoutableIPs[%d]: got %s, want %s", i, n.Hostinfo.RoutableIPs[i], w)
		}
	}

	view := n.View()

	// SubnetRoutes is the intersection of advertised and approved, with the
	// exit-node prefixes excluded.
	subnets := view.SubnetRoutes()
	if len(subnets) != 1 || subnets[0].String() != "10.0.0.0/24" {
		t.Errorf("SubnetRoutes(): got %v, want [10.0.0.0/24]", subnets)
	}

	if !view.IsExitNode() {
		t.Error("IsExitNode(): got false, want true — 0.0.0.0/0 and ::/0 are advertised and approved")
	}

	if n.Expiry == nil {
		t.Error("Expiry: got nil, want the fixture's future expiry")
	}

	if n.IsExpired() {
		t.Error("IsExpired(): got true for an expiry in 2027")
	}
}

func TestMapUserFromCapturedAPIResponse(t *testing.T) {
	var resp struct {
		Users []v1User `json:"users"`
	}

	readFixture(t, "users.json", &resp)

	if len(resp.Users) != 3 {
		t.Fatalf("users: got %d, want 3", len(resp.Users))
	}

	users := make(map[string]types.User, len(resp.Users))

	for i := range resp.Users {
		u, err := mapUser(&resp.Users[i])
		if err != nil {
			t.Fatalf("mapUser(%s): %v", resp.Users[i].Name, err)
		}

		users[u.Name] = u
	}

	alice := users["alice"]

	if alice.ID != 1 {
		t.Errorf("ID: got %d, want 1", alice.ID)
	}

	if alice.DisplayName != "Alice" || alice.Email != "alice@headboard.test" {
		t.Errorf("profile: got display=%q email=%q", alice.DisplayName, alice.Email)
	}

	// Users created through the CLI have no OIDC identity. T4 matches
	// sessions on this column, so "absent" must stay distinguishable from
	// "empty string" — otherwise every CLI user collides on "".
	if alice.ProviderIdentifier.Valid {
		t.Errorf("ProviderIdentifier: got valid=%v value=%q, want invalid for a CLI-created user",
			alice.ProviderIdentifier.Valid, alice.ProviderIdentifier.String)
	}

	if got := alice.Username(); got != "alice@headboard.test" {
		t.Errorf("Username(): got %q", got)
	}
}

func TestMapNodeRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		node v1Node
	}{
		{"non-numeric id", v1Node{ID: "nope"}},
		{"bad ip", v1Node{ID: "1", IPAddresses: []string{"not-an-ip"}}},
		{"bad approved route", v1Node{ID: "1", ApprovedRoutes: []string{"10.0.0.0"}}},
		{"bad available route", v1Node{ID: "1", AvailableRoutes: []string{"10.0.0.0"}}},
		{"bad machine key", v1Node{ID: "1", MachineKey: "mkey:zzzz"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mapNode(tt.node); err == nil {
				t.Fatal("got nil error, want a mapping failure")
			}
		})
	}
}
