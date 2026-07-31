package hs

import (
	"database/sql"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"gorm.io/gorm"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// The wire types below mirror what grpc-gateway emits for the protobuf messages
// in headscale.proto. They are deliberately separate from types.Node/types.User:
// the JSON shape is the server's public contract, the Go structs are its
// internals, and the two disagree in ways that matter (see mapNode).
//
// Numbers are strings because protobuf-JSON renders uint64 that way.

type v1Node struct {
	ID              string        `json:"id"`
	MachineKey      string        `json:"machineKey"`
	NodeKey         string        `json:"nodeKey"`
	DiscoKey        string        `json:"discoKey"`
	IPAddresses     []string      `json:"ipAddresses"`
	Name            string        `json:"name"`
	User            *v1User       `json:"user"`
	LastSeen        *time.Time    `json:"lastSeen"`
	Expiry          *time.Time    `json:"expiry"`
	PreAuthKey      *v1PreAuthKey `json:"preAuthKey"`
	CreatedAt       time.Time     `json:"createdAt"`
	RegisterMethod  string        `json:"registerMethod"`
	GivenName       string        `json:"givenName"`
	Online          bool          `json:"online"`
	ApprovedRoutes  []string      `json:"approvedRoutes"`
	AvailableRoutes []string      `json:"availableRoutes"`
	SubnetRoutes    []string      `json:"subnetRoutes"`
	Tags            []string      `json:"tags"`
}

type v1User struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"createdAt"`
	DisplayName   string    `json:"displayName"`
	Email         string    `json:"email"`
	ProviderID    string    `json:"providerId"`
	Provider      string    `json:"provider"`
	ProfilePicURL string    `json:"profilePicUrl"`
}

type v1PreAuthKey struct {
	ID         string     `json:"id"`
	Key        string     `json:"key"`
	User       *v1User    `json:"user"`
	Reusable   bool       `json:"reusable"`
	Ephemeral  bool       `json:"ephemeral"`
	Used       bool       `json:"used"`
	Expiration *time.Time `json:"expiration"`
	CreatedAt  *time.Time `json:"createdAt"`
	ACLTags    []string   `json:"aclTags"`
}

// mapUser converts a wire user into the Headscale type the policy engine reads.
func mapUser(u *v1User) (types.User, error) {
	if u == nil {
		return types.User{}, nil
	}

	id, err := strconv.ParseUint(u.ID, 10, 64)
	if err != nil {
		return types.User{}, fmt.Errorf("user %q: id: %w", u.Name, err)
	}

	return types.User{
		Model: gorm.Model{
			ID:        uint(id),
			CreatedAt: u.CreatedAt,
		},
		Name:        u.Name,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		// Empty on the wire means "no OIDC identity" — users created with
		// `headscale users create` have none. Keep that distinct from the
		// empty string, because T4 matches sessions on this column.
		ProviderIdentifier: sql.NullString{
			String: u.ProviderID,
			Valid:  u.ProviderID != "",
		},
		Provider:      u.Provider,
		ProfilePicURL: u.ProfilePicURL,
	}, nil
}

// mapNode converts a wire node into the Headscale type the policy engine reads.
//
// Three conversions here are not obvious, and every downstream feature is wrong
// without them:
//
//   - The API has no hostinfo field, but NodeView.SubnetRoutes() and
//     IsExitNode() both read Hostinfo.RoutableIPs. availableRoutes is
//     synthesised into a minimal Hostinfo, or subnet routers silently lose
//     their filter rules.
//   - Tagged nodes are rendered with the sentinel user "tagged-devices"
//     (types.TaggedDevicesUserID). Headscale's own model represents them with a
//     nil UserID, and ownership checks depend on that.
//   - A never-set expiry serialises as "0001-01-01T00:00:00Z" rather than null.
//     types.Node.IsExpired treats both nil and zero as "not expired"; the
//     headscale CLI's table does not, which is why `headscale nodes list`
//     prints Expired=yes for every freshly seeded node. Follow the type, not
//     the CLI.
func mapNode(n v1Node) (types.Node, error) {
	id, err := strconv.ParseUint(n.ID, 10, 64)
	if err != nil {
		return types.Node{}, fmt.Errorf("node %q: id: %w", n.Name, err)
	}

	node := types.Node{
		ID:             types.NodeID(id),
		Hostname:       n.Name,
		GivenName:      n.GivenName,
		RegisterMethod: n.RegisterMethod,
		Tags:           types.Strings(n.Tags),
		CreatedAt:      n.CreatedAt,
		IsOnline:       &n.Online,
	}

	for _, raw := range n.IPAddresses {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return types.Node{}, fmt.Errorf("node %q: ip %q: %w", n.Name, raw, err)
		}

		if addr.Is4() {
			node.IPv4 = &addr
		} else {
			node.IPv6 = &addr
		}
	}

	approved, err := parsePrefixes(n.Name, "approvedRoutes", n.ApprovedRoutes)
	if err != nil {
		return types.Node{}, err
	}

	node.ApprovedRoutes = approved

	available, err := parsePrefixes(n.Name, "availableRoutes", n.AvailableRoutes)
	if err != nil {
		return types.Node{}, err
	}

	node.Hostinfo = &tailcfg.Hostinfo{
		Hostname:    n.Name,
		RoutableIPs: available,
	}

	if err := parseKeys(&node, n); err != nil {
		return types.Node{}, err
	}

	if t := nonZeroTime(n.Expiry); t != nil {
		node.Expiry = t
	}

	if t := nonZeroTime(n.LastSeen); t != nil {
		node.LastSeen = t
	}

	if err := attachOwner(&node, n); err != nil {
		return types.Node{}, err
	}

	if n.PreAuthKey != nil {
		node.AuthKey = &types.PreAuthKey{
			Key:        n.PreAuthKey.Key,
			Reusable:   n.PreAuthKey.Reusable,
			Ephemeral:  n.PreAuthKey.Ephemeral,
			Used:       n.PreAuthKey.Used,
			Tags:       n.PreAuthKey.ACLTags,
			CreatedAt:  n.PreAuthKey.CreatedAt,
			Expiration: n.PreAuthKey.Expiration,
		}
	}

	return node, nil
}

// attachOwner sets UserID/User, collapsing the tagged-devices sentinel back to
// the nil owner Headscale's own model uses for tagged nodes.
func attachOwner(node *types.Node, n v1Node) error {
	if n.User == nil {
		return nil
	}

	user, err := mapUser(n.User)
	if err != nil {
		return err
	}

	if user.ID == types.TaggedDevicesUserID {
		return nil
	}

	node.UserID = &user.ID
	node.User = &user

	return nil
}

// parseKeys fills the node's key material. Keys are informational for the
// policy engine but the admin UI shows them, and a parse failure means the wire
// format changed under us — which is worth failing loudly for.
func parseKeys(node *types.Node, n v1Node) error {
	if n.MachineKey != "" {
		if err := node.MachineKey.UnmarshalText([]byte(n.MachineKey)); err != nil {
			return fmt.Errorf("node %q: machineKey: %w", n.Name, err)
		}
	}

	if n.NodeKey != "" {
		if err := node.NodeKey.UnmarshalText([]byte(n.NodeKey)); err != nil {
			return fmt.Errorf("node %q: nodeKey: %w", n.Name, err)
		}
	}

	if n.DiscoKey != "" {
		var disco key.DiscoPublic
		if err := disco.UnmarshalText([]byte(n.DiscoKey)); err != nil {
			return fmt.Errorf("node %q: discoKey: %w", n.Name, err)
		}

		node.DiscoKey = disco
	}

	return nil
}

func parsePrefixes(node, field string, raw []string) (types.Prefixes, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	out := make(types.Prefixes, 0, len(raw))

	for _, r := range raw {
		p, err := netip.ParsePrefix(r)
		if err != nil {
			return nil, fmt.Errorf("node %q: %s %q: %w", node, field, r, err)
		}

		out = append(out, p)
	}

	return out, nil
}

// nonZeroTime maps protobuf's zero timestamp to nil. Callers treat nil as
// "unset", which is what a zero timestamp means on the wire.
func nonZeroTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}

	return t
}
