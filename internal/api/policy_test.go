package api

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

func TestParseSimulationDestination(t *testing.T) {
	tests := []struct {
		name          string
		dst           uint64
		destinationIP string
		wantNode      types.NodeID
		wantAddr      netip.Addr
		wantLiteral   bool
		wantErr       string
	}{
		{name: "device", dst: 7, wantNode: 7},
		{name: "literal IPv4", destinationIP: "13.0.0.25", wantAddr: netip.MustParseAddr("13.0.0.25"), wantLiteral: true},
		{name: "CIDR", destinationIP: "13.0.0.0/24", wantErr: "enter one IPv4 or IPv6 address, not a CIDR"},
		{name: "malformed address", destinationIP: "not-an-ip", wantErr: "enter a valid IPv4 or IPv6 address"},
		{name: "missing destination", wantErr: "choose one destination"},
		{name: "ambiguous destination", dst: 7, destinationIP: "13.0.0.25", wantErr: "choose one destination"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSimulationDestination(tt.dst, tt.destinationIP)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSimulationDestination: %v", err)
			}
			if got.nodeID != tt.wantNode || got.addr != tt.wantAddr || got.literal != tt.wantLiteral {
				t.Fatalf("destination = %+v, want node=%d addr=%s literal=%t", got, tt.wantNode, tt.wantAddr, tt.wantLiteral)
			}
		})
	}
}

func TestPolicyBodyForDecodesWrittenPolicy(t *testing.T) {
	body, err := policyBodyFor(`{
		"groups": {"group:eng": ["alice@"]},
		"acls": [{"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]}],
		"grants": [{"src": ["group:eng"], "dst": ["13.0.0.0/24"], "ip": ["tcp:443"]}]
	}`, []types.User{{Name: "alice"}})
	if err != nil {
		t.Fatalf("policyBodyFor: %v", err)
	}

	if body.Schema == nil || len(body.Schema.ACLs) != 1 {
		t.Fatalf("schema ACLs = %#v, want the written rule", body.Schema)
	}
	if body.Schema == nil || len(body.Schema.Grants) != 1 || body.Schema.Grants[0].IP[0] != "tcp:443" {
		t.Fatalf("schema grants = %#v, want the written grant", body.Schema)
	}
	if !slices.Contains(body.Sections, "acls") || !slices.Contains(body.Sections, "grants") {
		t.Fatalf("sections = %v, want acls and grants", body.Sections)
	}
	if body.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
}

func TestPolicyBodyForPreservesEmptySectionPresence(t *testing.T) {
	empty, err := policyBodyFor(`{"grants": []}`, nil)
	if err != nil {
		t.Fatalf("policyBodyFor empty grants: %v", err)
	}
	if !slices.Contains(empty.Sections, "grants") {
		t.Fatalf("sections = %v, want grants", empty.Sections)
	}

	absent, err := policyBodyFor(`{"groups": {}}`, nil)
	if err != nil {
		t.Fatalf("policyBodyFor absent grants: %v", err)
	}
	if slices.Contains(absent.Sections, "grants") {
		t.Fatalf("sections = %v, do not want grants", absent.Sections)
	}
}
