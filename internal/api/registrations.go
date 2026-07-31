package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
)

// Pending node registrations.
//
// Headscale v0.29.3 has no endpoint that lists them: the API offers approve,
// reject and register, each by auth id, and nothing to enumerate. The id
// reaches an administrator out of band — the Tailscale client prints a URL
// containing it, and the person registering the device passes it on.
//
// So this is an "approve this id" form rather than the inbox the plan
// described. Building a list would mean either polling an endpoint that does
// not exist or having Headboard sit in the registration path, which it does
// not. Headscale main's /api/v2 is expected to expose the queue.

type registrationInput struct {
	Body struct {
		// AuthID is the hskey-authreq-… value from the registration URL.
		AuthID string `json:"authId" minLength:"14"`

		// User assigns the device to a Headscale user. Leave empty to
		// approve a registration that already names one.
		User string `json:"user,omitempty"`
	}
}

type registrationOutput struct {
	Body struct {
		Approved bool    `json:"approved"`
		Device   *Device `json:"device,omitempty"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "approveRegistration",
			Method:      http.MethodPost,
			Path:        "/api/registrations/approve",
			Summary:     "Approve a pending device registration by its auth id",
			Description: "Headscale exposes no way to list pending registrations on v0.29.x, " +
				"so the id has to be supplied. It is the hskey-authreq-… value from the URL " +
				"the Tailscale client prints.",
			Tags: []string{"registrations"},
		}, func(ctx context.Context, in *registrationInput) (*registrationOutput, error) {
			p, err := require(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			if err := validAuthID(in.Body.AuthID); err != nil {
				return nil, err
			}

			out := &registrationOutput{}
			out.Body.Approved = true

			// Registering against a named user and approving an
			// existing request are different calls upstream.
			if in.Body.User != "" {
				node, err := deps.Mutator.RegisterNode(ctx, in.Body.AuthID, in.Body.User)
				if err != nil {
					return nil, upstream(err, "could not register the device")
				}

				device := toDevice(node, p)
				out.Body.Device = &device
			} else if err := deps.Mutator.ApproveRegistration(ctx, in.Body.AuthID); err != nil {
				return nil, upstream(err, "could not approve the registration")
			}

			finish(ctx, deps, p, "registration.approve", "registration", 0,
				nil, map[string]string{"authId": redactAuthID(in.Body.AuthID), "user": in.Body.User})

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "rejectRegistration",
			Method:      http.MethodPost,
			Path:        "/api/registrations/reject",
			Summary:     "Reject a pending device registration",
			Tags:        []string{"registrations"},
		}, func(ctx context.Context, in *registrationInput) (*registrationOutput, error) {
			p, err := require(ctx, auth.CapManageDevices)
			if err != nil {
				return nil, err
			}

			if err := validAuthID(in.Body.AuthID); err != nil {
				return nil, err
			}

			if err := deps.Mutator.RejectRegistration(ctx, in.Body.AuthID); err != nil {
				return nil, upstream(err, "could not reject the registration")
			}

			finish(ctx, deps, p, "registration.reject", "registration", 0,
				nil, map[string]string{"authId": redactAuthID(in.Body.AuthID)})

			return &registrationOutput{}, nil
		})
	})
}

// authIDPrefix and authIDLength mirror types.AuthID, which does not export a
// validator. Checking here turns a typo into a clear message instead of an
// opaque upstream error.
const (
	authIDPrefix = "hskey-authreq-"
	authIDLength = 38
)

func validAuthID(id string) error {
	if !strings.HasPrefix(id, authIDPrefix) {
		return huma.Error422UnprocessableEntity(
			"an auth id starts with " + authIDPrefix)
	}

	if len(id) != authIDLength {
		return huma.Error422UnprocessableEntity(
			"an auth id is 38 characters long")
	}

	return nil
}

// redactAuthID keeps enough of the id to correlate an audit entry with a
// registration, without writing a usable credential into the log.
func redactAuthID(id string) string {
	if len(id) <= len(authIDPrefix)+6 {
		return authIDPrefix + "…"
	}

	return id[:len(authIDPrefix)+6] + "…"
}
