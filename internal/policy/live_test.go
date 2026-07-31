package policy_test

import (
	"os"
	"testing"
	"time"

	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/policy"
)

// The unit tests build the tailnet by hand. This one takes it from a running
// Headscale through internal/hs, which is the only way to know the mapper and
// the policy engine agree on the same data:
//
//	env $(grep -v '^#' dev/.env.dev | xargs) go test ./internal/policy/ -run Live
//
// Read-only — every call is a GET.
func TestLivePolicyBridge(t *testing.T) {
	url, key := os.Getenv("HEADSCALE_URL"), os.Getenv("HEADSCALE_API_KEY")
	if url == "" || key == "" {
		t.Skip("set HEADSCALE_URL and HEADSCALE_API_KEY to run live tests")
	}

	c := hs.New(url, key, 10*time.Second)
	ctx := t.Context()

	nodes, err := c.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	users, err := c.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	pol, err := c.Policy(ctx)
	if err != nil {
		t.Fatalf("Policy: %v", err)
	}

	m, err := policy.New(pol.HuJSON, users, nodes)
	if err != nil {
		t.Fatalf("building policy manager from live data: %v", err)
	}

	var checked int

	for _, n := range nodes {
		inbound, err := m.Inbound(n.ID)
		if err != nil {
			t.Fatalf("Inbound(%s): %v", n.GivenName, err)
		}

		outbound, err := m.Outbound(n.ID)
		if err != nil {
			t.Fatalf("Outbound(%s): %v", n.GivenName, err)
		}

		if _, err := m.Peers(n.ID); err != nil {
			t.Fatalf("Peers(%s): %v", n.GivenName, err)
		}

		// Whatever the policy says, no endpoint should reach the UI as an
		// unresolved address expression when the address belongs to a
		// node in this tailnet.
		for _, r := range append(inbound, outbound...) {
			for _, s := range r.Sources {
				if s.Kind == policy.KindPrefix && s.Label == "" {
					t.Errorf("%s: source %q produced an empty label", n.GivenName, s.Raw)
				}
			}
		}

		checked++
	}

	if checked == 0 {
		t.Fatal("no nodes to check — is the dev instance seeded?")
	}

	t.Logf("policy bridge drove %d live nodes", checked)
}
