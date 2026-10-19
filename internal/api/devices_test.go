package api

import (
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
)

func TestRulesUnavailableShowsTheCompileErrorToWhoeverCanFixIt(t *testing.T) {
	snap := &tailnet.Snapshot{PolicyErr: `multiple users found: token "alice@" found: [ 1: alice, 3: alice,  ]`}

	admin := auth.Principal{User: store.User{Role: store.RoleNetworkAdmin}}
	if err := rulesUnavailable(admin, snap); !strings.Contains(err.Error(), `token "alice@"`) {
		t.Errorf("a policy admin got %q, want the compile error", err)
	}

	// The error names other people's accounts, and a member cannot fix it.
	member := auth.Principal{User: store.User{Role: store.RoleMember}}
	if err := rulesUnavailable(member, snap); strings.Contains(err.Error(), "alice") {
		t.Errorf("a member was shown the compile error: %q", err)
	}
}

func TestRulesUnavailableWithNoPolicyDoesNotClaimItFailedToCompile(t *testing.T) {
	admin := auth.Principal{User: store.User{Role: store.RoleOwner}}

	err := rulesUnavailable(admin, &tailnet.Snapshot{})
	if strings.Contains(err.Error(), "does not compile") {
		t.Errorf("an empty policy was reported as broken: %q", err)
	}
}
