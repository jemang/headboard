package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/auth"
)

type renameInput struct {
	ID   uint64 `path:"id"`
	Body struct {
		Name string `json:"name" minLength:"1" maxLength:"63"`
	}
}

type tagsInput struct {
	ID   uint64 `path:"id"`
	Body struct {
		Tags []string `json:"tags"`
	}
}

type routesInput struct {
	ID   uint64 `path:"id"`
	Body struct {
		// Routes is the complete approved set, not an addition. Sending
		// an empty array revokes every route, which is how one is
		// withdrawn.
		Routes []string `json:"routes"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		// Renaming your own device is self-service; renaming anyone
		// else's needs the device capability. Both land here.
		huma.Register(api, huma.Operation{
			OperationID: "renameDevice",
			Method:      http.MethodPost,
			Path:        "/api/devices/{id}/rename",
			Summary:     "Rename a device",
			Tags:        []string{"devices"},
		}, func(ctx context.Context, in *renameInput) (*deviceOutput, error) {
			p, node, err := deviceForWrite(ctx, deps, in.ID)
			if err != nil {
				return nil, err
			}

			updated, err := deps.Mutator.RenameNode(ctx, node.ID, in.Body.Name)
			if err != nil {
				return nil, upstream(err, "could not rename the device")
			}

			finish(ctx, deps, p, "device.rename", "device", int64(node.ID),
				toDevice(node, p), toDevice(updated, p))

			return &deviceOutput{Body: toDevice(updated, p)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "setDeviceTags",
			Method:      http.MethodPut,
			Path:        "/api/devices/{id}/tags",
			Summary:     "Replace a device's tags",
			Description: "Tags are an identity, not a label: a tagged node has no owning user. " +
				"Members cannot set them.",
			Tags: []string{"devices"},
		}, func(ctx context.Context, in *tagsInput) (*deviceOutput, error) {
			p, err := require(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			node, err := nodeByID(deps, in.ID)
			if err != nil {
				return nil, err
			}

			for _, tag := range in.Body.Tags {
				if !strings.HasPrefix(tag, "tag:") {
					return nil, huma.Error422UnprocessableEntity(
						"tags must start with \"tag:\", got " + tag)
				}
			}

			updated, err := deps.Mutator.SetTags(ctx, node.ID, in.Body.Tags)
			if err != nil {
				return nil, upstream(err, "could not set tags")
			}

			finish(ctx, deps, p, "device.tags", "device", int64(node.ID),
				node.Tags, in.Body.Tags)

			return &deviceOutput{Body: toDevice(updated, p)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "approveDeviceRoutes",
			Method:      http.MethodPut,
			Path:        "/api/devices/{id}/routes",
			Summary:     "Replace the routes a device may advertise",
			Tags:        []string{"devices"},
		}, func(ctx context.Context, in *routesInput) (*deviceOutput, error) {
			p, err := require(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			node, err := nodeByID(deps, in.ID)
			if err != nil {
				return nil, err
			}

			before := toDevice(node, p)

			updated, err := deps.Mutator.ApproveRoutes(ctx, node.ID, in.Body.Routes)
			if err != nil {
				return nil, upstream(err, "could not approve routes")
			}

			finish(ctx, deps, p, "device.routes", "device", int64(node.ID),
				before.ApprovedRoutes, in.Body.Routes)

			return &deviceOutput{Body: toDevice(updated, p)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "expireDevice",
			Method:      http.MethodPost,
			Path:        "/api/devices/{id}/expire",
			Summary:     "Expire a device's key, forcing it to sign in again",
			Tags:        []string{"devices"},
		}, func(ctx context.Context, in *deviceInput) (*deviceOutput, error) {
			p, err := require(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			node, err := nodeByID(deps, in.ID)
			if err != nil {
				return nil, err
			}

			updated, err := deps.Mutator.ExpireNode(ctx, node.ID)
			if err != nil {
				return nil, upstream(err, "could not expire the device")
			}

			finish(ctx, deps, p, "device.expire", "device", int64(node.ID), nil, nil)

			return &deviceOutput{Body: toDevice(updated, p)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "deleteDevice",
			Method:      http.MethodDelete,
			Path:        "/api/devices/{id}",
			Summary:     "Remove a device from the tailnet",
			Tags:        []string{"devices"},
		}, func(ctx context.Context, in *deviceInput) (*struct{}, error) {
			p, node, err := deviceForWrite(ctx, deps, in.ID)
			if err != nil {
				return nil, err
			}

			// The whole record goes in the audit entry: after a delete
			// there is nothing left to look the device up in.
			before := toDevice(node, p)

			if err := deps.Mutator.DeleteNode(ctx, node.ID); err != nil {
				return nil, upstream(err, "could not delete the device")
			}

			finish(ctx, deps, p, "device.delete", "device", int64(node.ID), before, nil)

			return nil, nil
		})
	})
}

// deviceForWrite allows a member to act on their own device and an admin to act
// on any. It is used for the two genuinely self-service actions.
func deviceForWrite(ctx context.Context, deps Deps, id uint64) (auth.Principal, types.Node, error) {
	p, node, _, err := deviceFor(ctx, deps, id, auth.CapViewSelf)
	if err != nil {
		return auth.Principal{}, types.Node{}, err
	}

	if p.Can(auth.CapManageDevices) || p.OwnsHeadscaleUser(node.UserID) {
		return p, node, nil
	}

	return auth.Principal{}, types.Node{}, huma.Error403Forbidden(
		"you can only change your own devices")
}

func nodeByID(deps Deps, id uint64) (types.Node, error) {
	snap, err := currentSnapshot(deps)
	if err != nil {
		return types.Node{}, err
	}

	for _, n := range snap.Nodes {
		if uint64(n.ID) == id {
			return n, nil
		}
	}

	return types.Node{}, huma.Error404NotFound("no such device")
}

// finish records the change and refreshes the snapshot so the result is visible
// immediately rather than at the next poll.
func finish(ctx context.Context, deps Deps, p auth.Principal, action, targetType string, targetID int64, before, after any) {
	audit(ctx, deps, p, action, targetType, targetID, before, after)

	if deps.Tailnet != nil {
		deps.Tailnet.Invalidate()
	}
}

// upstream turns a Headscale error into something a browser can show without
// leaking the admin API key or the internal URL.
func upstream(err error, msg string) error {
	deps := huma.Error502BadGateway(msg, err)

	return deps
}
