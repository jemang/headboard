package auth

import (
	"database/sql"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/store"
)

func TestCapabilityTable(t *testing.T) {
	tests := []struct {
		role store.Role
		cap  Capability
		want bool
	}{
		{store.RoleOwner, CapManageOwners, true},
		// Only owners may touch owners, otherwise an admin can promote
		// themselves past whoever set the system up.
		{store.RoleAdmin, CapManageOwners, false},

		{store.RoleAdmin, CapManageUsers, true},
		// network-admin runs the network without being able to grant
		// itself more access.
		{store.RoleNetworkAdmin, CapManagePolicy, true},
		{store.RoleNetworkAdmin, CapManageDevices, true},
		{store.RoleNetworkAdmin, CapManageUsers, false},
		{store.RoleNetworkAdmin, CapManageKeys, false},

		{store.RoleAuditor, CapViewAll, true},
		{store.RoleAuditor, CapViewAudit, true},
		{store.RoleAuditor, CapManageDevices, false},
		{store.RoleAuditor, CapManagePolicy, false},

		{store.RoleMember, CapViewSelf, true},
		{store.RoleMember, CapViewAll, false},
		{store.RoleMember, CapManageDevices, false},

		// An unknown role must grant nothing, so a bad database value
		// fails closed.
		{store.Role("nonsense"), CapViewSelf, false},
	}

	for _, tt := range tests {
		if got := Can(tt.role, tt.cap); got != tt.want {
			t.Errorf("Can(%s, %s) = %v, want %v", tt.role, tt.cap, got, tt.want)
		}
	}
}

func TestOwnsHeadscaleUser(t *testing.T) {
	linked := int64(7)
	p := Principal{User: store.User{HeadscaleUserID: &linked}}

	seven, eight := uint(7), uint(8)

	if !p.OwnsHeadscaleUser(&seven) {
		t.Error("linked account does not recognise its own headscale user")
	}

	if p.OwnsHeadscaleUser(&eight) {
		t.Error("linked account claims a headscale user it is not linked to")
	}

	// A tagged node has no owner. Treating nil as a match would hand every
	// tagged device to whoever asked first.
	if p.OwnsHeadscaleUser(nil) {
		t.Error("nil owner (a tagged node) was treated as owned")
	}

	unlinked := Principal{User: store.User{}}
	if unlinked.OwnsHeadscaleUser(&seven) {
		t.Error("unlinked account claimed ownership")
	}
}

// An absolute return_to would turn the login endpoint into an open redirect:
// send someone /auth/login?return_to=https://evil.example and they land there
// carrying whatever trust they place in the Headboard domain.
func TestSafeReturnTo(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{"/devices", "/devices"},
		{"/acl?tab=rules", "/acl?tab=rules"},
		{"https://evil.example/", "/"},
		{"http://evil.example/x", "/"},
		{"//evil.example/x", "/"},
		{"javascript:alert(1)", "/"},
		{"devices", "/"},
	}

	for _, tt := range tests {
		if got := safeReturnTo(tt.in); got != tt.want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatchHeadscaleUser(t *testing.T) {
	oidcUser := types.User{
		Name: "alice",
		ProviderIdentifier: sql.NullString{
			String: "http://127.0.0.1:9998/oidc/alice-sub",
			Valid:  true,
		},
	}
	oidcUser.ID = 1

	// Created with `headscale users create`: no provider identifier at all.
	cliUser := types.User{Name: "bob"}
	cliUser.ID = 2

	users := []types.User{oidcUser, cliUser}

	t.Run("matches on issuer and subject", func(t *testing.T) {
		got, ok := matchHeadscaleUser("http://127.0.0.1:9998/oidc", "alice-sub", users)
		if !ok || got.Name != "alice" {
			t.Fatalf("got %+v ok=%v, want alice", got, ok)
		}
	})

	t.Run("trailing slash on the issuer still matches", func(t *testing.T) {
		if _, ok := matchHeadscaleUser("http://127.0.0.1:9998/oidc/", "alice-sub", users); !ok {
			t.Error("a trailing slash on the issuer broke the match")
		}
	})

	// The whole point of matching on provider identifier rather than email
	// is that a different subject is a different person, even at the same
	// issuer.
	t.Run("different subject does not match", func(t *testing.T) {
		if _, ok := matchHeadscaleUser("http://127.0.0.1:9998/oidc", "mallory-sub", users); ok {
			t.Error("a different subject matched")
		}
	})

	t.Run("different issuer does not match", func(t *testing.T) {
		if _, ok := matchHeadscaleUser("https://other.example", "alice-sub", users); ok {
			t.Error("the same subject at a different issuer matched")
		}
	})

	// CLI-created users have an empty identifier. If empty matched empty,
	// the first person to log in would be handed someone else's devices.
	t.Run("empty provider identifier never matches", func(t *testing.T) {
		if _, ok := matchHeadscaleUser("", "", users); ok {
			t.Error("an empty identity matched a CLI-created user")
		}
	})
}

func TestRoleRank(t *testing.T) {
	if store.RoleOwner.Rank() <= store.RoleAdmin.Rank() {
		t.Error("owner does not outrank admin")
	}

	if store.Role("nonsense").Valid() {
		t.Error("an unknown role reported itself valid")
	}

	for _, r := range []store.Role{
		store.RoleOwner, store.RoleAdmin, store.RoleNetworkAdmin,
		store.RoleAuditor, store.RoleMember,
	} {
		if !r.Valid() {
			t.Errorf("%s reported itself invalid", r)
		}
	}
}

// Redirect targets are app paths, but return_to arrives from the query string
// and may already carry the deployment prefix. Adding it twice sends a person
// to /manage/manage/acl, which the SPA renders as a blank page.
func TestAppPathDoesNotDoubleThePrefix(t *testing.T) {
	for _, tc := range []struct{ base, in, want string }{
		{"", "/acl", "/acl"},
		{"/manage", "/acl", "/manage/acl"},
		{"/manage", "/", "/manage/"},
		{"/manage", "/manage/acl", "/manage/acl"},
		{"/manage", "/manage", "/manage"},
		{"/manage", "/management/acl", "/manage/management/acl"},
	} {
		a := &Auth{cfg: Config{BasePath: tc.base}}
		if got := a.appPath(tc.in); got != tc.want {
			t.Errorf("appPath(base=%q, %q) = %q, want %q", tc.base, tc.in, got, tc.want)
		}
	}
}
