package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/hs"
)

// Reassigning a device to another user.
//
// Headscale v0.29.3 has no way to change a node's owner (MoveNode was removed
// upstream). A machine that signs in again as a different user gets a second,
// new node with a new IP, and the old node stays behind. So a reassign is that
// re-registration plus removing the old node — in that order, so the device is
// never left with no node at all.
//
// It goes through the registration-approval path rather than a pre-auth key:
// Headboard deliberately does not mint keys.

type reassignInput struct {
	ID   uint64 `path:"id"`
	Body struct {
		// AuthID is the hskey-authreq-… value the device printed after
		// `tailscale up --force-reauth`.
		AuthID string `json:"authId" minLength:"14"`

		// User is the Headscale user that will own the device.
		User string `json:"user" minLength:"1"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "reassignDevice",
			Method:      http.MethodPost,
			Path:        "/api/devices/{id}/reassign",
			Summary:     "Move a device to another user by re-registering it",
			Description: "Registers the device's pending sign-in for the new user, checks it came " +
				"from the same machine, then removes the old node. The device gets a new IP.",
			Tags: []string{"devices"},
		}, func(ctx context.Context, in *reassignInput) (*deviceOutput, error) {
			p, err := requireSudo(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			if err := validAuthID(in.Body.AuthID); err != nil {
				return nil, err
			}

			_, old, _, err := deviceFor(ctx, deps, in.ID, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			before := toDevice(old, p)

			node, err := reassignNode(ctx, deps.Mutator, old, in.Body.AuthID, in.Body.User)
			if err != nil {
				return nil, err
			}

			after := toDevice(node, p)

			finish(ctx, deps, p, "device.reassign", "device", int64(old.ID), old.GivenName, before, after)

			return &deviceOutput{Body: after}, nil
		})
	})
}

// reassignNode registers authID for user and, once the new node is confirmed to
// be the same machine, deletes old. A registration from any other machine is
// undone: a mistyped auth id must not cost the device its access.
func reassignNode(ctx context.Context, m hs.Mutator, old types.Node, authID, user string) (types.Node, error) {
	if len(old.Tags) > 0 {
		return types.Node{}, huma.Error422UnprocessableEntity(
			"a tagged device is owned by its tags, not a user; change its tags instead")
	}

	if old.User != nil && old.User.Name == user {
		return types.Node{}, huma.Error422UnprocessableEntity("the device already belongs to " + user)
	}

	node, err := m.RegisterNode(ctx, authID, user)
	if err != nil {
		return types.Node{}, upstream(err, "could not register the device for "+user)
	}

	if old.MachineKey.IsZero() || node.MachineKey != old.MachineKey {
		if err := m.DeleteNode(ctx, node.ID); err != nil {
			return types.Node{}, upstream(err,
				"that sign-in came from a different machine and was registered for "+user+
					"; remove the new device by hand")
		}

		return types.Node{}, huma.Error409Conflict(
			"that sign-in came from a different machine, so nothing was changed; " +
				"run the command on the device being moved and use the id it prints")
	}

	if err := m.DeleteNode(ctx, old.ID); err != nil {
		return types.Node{}, upstream(err,
			"the device was registered for "+user+" but the old device is still listed; remove it by hand")
	}

	return node, nil
}
