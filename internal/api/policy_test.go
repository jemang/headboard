package api

import (
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

func TestPolicyBodyForDecodesWrittenPolicy(t *testing.T) {
	body, err := policyBodyFor(`{
		"groups": {"group:eng": ["alice@"]},
		"acls": [{"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]}]
	}`, []types.User{{Name: "alice"}})
	if err != nil {
		t.Fatalf("policyBodyFor: %v", err)
	}

	if body.Schema == nil || len(body.Schema.ACLs) != 1 {
		t.Fatalf("schema ACLs = %#v, want the written rule", body.Schema)
	}
	if body.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
}
