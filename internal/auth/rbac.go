package auth

import (
	"context"

	"github.com/jemang/headboard/internal/store"
)

// Capability is a thing a person can do. Handlers name a capability rather than
// a role, so adding a role later does not mean auditing every handler.
type Capability string

const (
	// CapViewSelf covers a member's own devices and the rules affecting
	// them. Everyone who can log in has it.
	CapViewSelf Capability = "view:self"

	// CapViewAll covers every device and user in the tailnet.
	CapViewAll Capability = "view:all"

	// CapManageDevices covers rename, tags, routes, expiry and deletion of
	// any device, plus approving pending registrations.
	CapManageDevices Capability = "manage:devices"

	// CapManagePolicy covers reading and writing the ACL document.
	CapManagePolicy Capability = "manage:policy"

	// CapManageUsers covers Headscale users, Headboard roles and account
	// links.
	CapManageUsers Capability = "manage:users"

	// CapManageKeys covers pre-auth keys and API keys, which mint
	// credentials.
	CapManageKeys Capability = "manage:keys"

	// CapViewAudit covers the audit log.
	CapViewAudit Capability = "view:audit"

	// CapManageOwners covers promoting and demoting owners.
	CapManageOwners Capability = "manage:owners"
)

// capabilities is the whole authorisation model, in one readable table.
//
// network-admin deliberately has policy and device control but no user
// management: it is the role for someone who runs the network without being
// able to grant themselves more access. auditor is read-only on purpose,
// including the audit log.
var capabilities = map[store.Role]map[Capability]bool{
	store.RoleOwner: {
		CapViewSelf: true, CapViewAll: true, CapManageDevices: true,
		CapManagePolicy: true, CapManageUsers: true, CapManageKeys: true,
		CapViewAudit: true, CapManageOwners: true,
	},
	store.RoleAdmin: {
		CapViewSelf: true, CapViewAll: true, CapManageDevices: true,
		CapManagePolicy: true, CapManageUsers: true, CapManageKeys: true,
		CapViewAudit: true,
	},
	store.RoleNetworkAdmin: {
		CapViewSelf: true, CapViewAll: true, CapManageDevices: true,
		CapManagePolicy: true, CapViewAudit: true,
	},
	store.RoleAuditor: {
		CapViewSelf: true, CapViewAll: true, CapViewAudit: true,
	},
	store.RoleMember: {
		CapViewSelf: true,
	},
}

// Can reports whether a role has a capability.
func Can(role store.Role, cap Capability) bool {
	return capabilities[role][cap]
}

// Capabilities lists everything a role may do, for the UI to hide what it
// cannot use. Hiding is a courtesy; the server checks regardless.
func Capabilities(role store.Role) []Capability {
	granted := capabilities[role]

	out := make([]Capability, 0, len(granted))

	for _, c := range allCapabilities {
		if granted[c] {
			out = append(out, c)
		}
	}

	return out
}

// allCapabilities fixes the order Capabilities returns, so the API response is
// stable between requests.
var allCapabilities = []Capability{
	CapViewSelf, CapViewAll, CapManageDevices, CapManagePolicy,
	CapManageUsers, CapManageKeys, CapViewAudit, CapManageOwners,
}

// Principal is the authenticated caller.
type Principal struct {
	User store.User
}

// Can reports whether the caller has a capability.
func (p Principal) Can(cap Capability) bool { return Can(p.User.Role, cap) }

// OwnsHeadscaleUser reports whether the caller is the Headscale user that owns
// a device. Used by member-scoped endpoints to answer "is this mine".
func (p Principal) OwnsHeadscaleUser(id *uint) bool {
	if id == nil || p.User.HeadscaleUserID == nil {
		return false
	}

	return int64(*id) == *p.User.HeadscaleUserID
}

type principalKey struct{}

// WithPrincipal attaches the caller to a context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom reads the caller back out.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)

	return p, ok
}
