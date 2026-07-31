package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/policy"
	"github.com/jemang/headboard/internal/tailnet"
)

// Device is a node as the UI renders it. It is a projection rather than
// types.Node: the wire shape should not change every time Headscale's internals
// do.
type Device struct {
	ID        uint64   `json:"id"`
	Name      string   `json:"name"`
	Hostname  string   `json:"hostname"`
	IPs       []string `json:"ips"`
	Tags      []string `json:"tags,omitempty"`
	Owner     string   `json:"owner,omitempty"`
	OwnerID   *uint    `json:"ownerId,omitempty"`
	Online    bool     `json:"online"`
	Expired   bool     `json:"expired"`
	Ephemeral bool     `json:"ephemeral"`

	LastSeen *time.Time `json:"lastSeen,omitempty"`
	Expiry   *time.Time `json:"expiry,omitempty"`

	// AdvertisedRoutes is what the device offers; ApprovedRoutes is what an
	// admin has allowed. The difference is the approval queue.
	AdvertisedRoutes []string `json:"advertisedRoutes,omitempty"`
	ApprovedRoutes   []string `json:"approvedRoutes,omitempty"`
	SubnetRoutes     []string `json:"subnetRoutes,omitempty"`
	ExitNode         bool     `json:"exitNode"`

	// Mine is true when the caller owns this device, so the UI can show the
	// self-service actions without a second request.
	Mine bool `json:"mine"`
}

type devicesOutput struct {
	Body struct {
		Devices []Device `json:"devices"`

		// Revision matches the SSE event that produced this list, so a
		// browser can tell whether it is already up to date.
		Revision uint64 `json:"revision"`

		// Stale is true when the last poll of Headscale failed and this
		// is the previous snapshot.
		Stale bool `json:"stale"`
	}
}

type deviceInput struct {
	ID uint64 `path:"id"`
}

type deviceOutput struct {
	Body Device
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listDevices",
			Method:      http.MethodGet,
			Path:        "/api/devices",
			Summary:     "Devices visible to the caller",
			Description: "Admins see the whole tailnet. Members see only their own devices, " +
				"scoped on the server rather than filtered in the browser.",
			Tags: []string{"devices"},
		}, func(ctx context.Context, _ *struct{}) (*devicesOutput, error) {
			p, err := require(ctx, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}

			out := &devicesOutput{}
			out.Body.Revision = snap.Revision
			out.Body.Devices = []Device{}

			for _, n := range snap.Nodes {
				if !visible(p, n, snap) {
					continue
				}

				out.Body.Devices = append(out.Body.Devices, toDevice(n, p))
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "getDevice",
			Method:      http.MethodGet,
			Path:        "/api/devices/{id}",
			Summary:     "One device",
			Tags:        []string{"devices"},
		}, func(ctx context.Context, in *deviceInput) (*deviceOutput, error) {
			p, node, _, err := deviceFor(ctx, deps, in.ID, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			return &deviceOutput{Body: toDevice(node, p)}, nil
		})
	})
}

// currentSnapshot returns the latest tailnet read, or an error a browser can
// act on.
func currentSnapshot(deps Deps) (*tailnet.Snapshot, error) {
	snap, err := deps.Tailnet.Current()
	if err != nil {
		return nil, huma.Error503ServiceUnavailable(
			"headscale is unreachable and no snapshot has been taken yet", err)
	}

	if snap == nil {
		return nil, huma.Error503ServiceUnavailable("still starting up")
	}

	return snap, nil
}

// visible decides whether a caller may see a node at all.
//
// Members are scoped here, on the server. A tagged node has no owning user, so
// visibility follows tag ownership: someone who owns tag:prod can see the
// devices carrying it.
func visible(p auth.Principal, n types.Node, snap *tailnet.Snapshot) bool {
	if p.Can(auth.CapViewAll) {
		return true
	}

	if p.OwnsHeadscaleUser(n.UserID) {
		return true
	}

	if len(n.Tags) == 0 || snap.Manager == nil || p.User.HeadscaleUserID == nil {
		return false
	}

	user, ok := headscaleUser(snap, *p.User.HeadscaleUserID)
	if !ok {
		return false
	}

	for _, tag := range n.Tags {
		if snap.Manager.UserCanHaveTag(user.View(), tag) {
			return true
		}
	}

	return false
}

func headscaleUser(snap *tailnet.Snapshot, id int64) (types.User, bool) {
	for _, u := range snap.Users {
		if int64(u.ID) == id {
			return u, true
		}
	}

	return types.User{}, false
}

// deviceFor resolves a device the caller is allowed to see.
func deviceFor(ctx context.Context, deps Deps, id uint64, cap auth.Capability) (auth.Principal, types.Node, *tailnet.Snapshot, error) {
	p, err := require(ctx, cap)
	if err != nil {
		return auth.Principal{}, types.Node{}, nil, err
	}

	snap, err := currentSnapshot(deps)
	if err != nil {
		return auth.Principal{}, types.Node{}, nil, err
	}

	for _, n := range snap.Nodes {
		if uint64(n.ID) != id {
			continue
		}

		if !visible(p, n, snap) {
			// Deliberately the same answer as a device that does not
			// exist: a member should not be able to probe which node
			// ids are real.
			return auth.Principal{}, types.Node{}, nil, huma.Error404NotFound("no such device")
		}

		return p, n, snap, nil
	}

	return auth.Principal{}, types.Node{}, nil, huma.Error404NotFound("no such device")
}

func toDevice(n types.Node, p auth.Principal) Device {
	view := n.View()

	d := Device{
		ID:        uint64(n.ID),
		Name:      n.GivenName,
		Hostname:  n.Hostname,
		Tags:      n.Tags,
		OwnerID:   n.UserID,
		Online:    n.IsOnline != nil && *n.IsOnline,
		Expired:   n.IsExpired(),
		Ephemeral: n.IsEphemeral(),
		LastSeen:  n.LastSeen,
		Expiry:    n.Expiry,
		ExitNode:  view.IsExitNode(),
		Mine:      p.OwnsHeadscaleUser(n.UserID),
	}

	for _, ip := range n.IPs() {
		d.IPs = append(d.IPs, ip.String())
	}

	if n.User != nil {
		d.Owner = n.User.Username()
	}

	if n.Hostinfo != nil {
		for _, r := range n.Hostinfo.RoutableIPs {
			d.AdvertisedRoutes = append(d.AdvertisedRoutes, r.String())
		}
	}

	for _, r := range n.ApprovedRoutes {
		d.ApprovedRoutes = append(d.ApprovedRoutes, r.String())
	}

	for _, r := range view.SubnetRoutes() {
		d.SubnetRoutes = append(d.SubnetRoutes, r.String())
	}

	return d
}

// effective rules are exposed per device so the member portal and the admin
// drawer can share one endpoint.
type rulesOutput struct {
	Body struct {
		Inbound  []policy.EffectiveRule `json:"inbound"`
		Outbound []policy.EffectiveRule `json:"outbound"`
		Peers    []policy.Peer          `json:"peers"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "deviceRules",
			Method:      http.MethodGet,
			Path:        "/api/devices/{id}/rules",
			Summary:     "The rules that actually apply to this device",
			Description: "Computed by Headscale's own policy engine, not re-implemented: " +
				"inbound is the reduced rule set this node receives, outbound is what it may " +
				"reach, and peers is the literal peer list it would be given.",
			Tags: []string{"devices"},
		}, func(ctx context.Context, in *deviceInput) (*rulesOutput, error) {
			_, node, snap, err := deviceFor(ctx, deps, in.ID, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			if snap.Manager == nil {
				return nil, huma.Error409Conflict(
					"the current policy does not compile, so effective rules are unavailable")
			}

			inbound, err := snap.Manager.Inbound(node.ID)
			if err != nil {
				return nil, statusFor(err, "could not compute inbound rules")
			}

			outbound, err := snap.Manager.Outbound(node.ID)
			if err != nil {
				return nil, statusFor(err, "could not compute outbound rules")
			}

			peers, err := snap.Manager.Peers(node.ID)
			if err != nil {
				return nil, statusFor(err, "could not compute peers")
			}

			out := &rulesOutput{}
			out.Body.Inbound = orEmptyRules(inbound)
			out.Body.Outbound = orEmptyRules(outbound)
			out.Body.Peers = orEmptyPeers(peers)

			return out, nil
		})
	})
}

// The UI distinguishes "no rules" from "not loaded", so an empty list must
// serialise as [] rather than null.
func orEmptyRules(r []policy.EffectiveRule) []policy.EffectiveRule {
	if r == nil {
		return []policy.EffectiveRule{}
	}

	return r
}

func orEmptyPeers(p []policy.Peer) []policy.Peer {
	if p == nil {
		return []policy.Peer{}
	}

	return p
}
