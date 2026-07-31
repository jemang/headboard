package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// MeBody describes the signed-in caller. The SPA fetches this once on load to
// decide what to render.
type MeBody struct {
	User store.User `json:"user"`

	// Capabilities lets the UI hide what the caller cannot use. It is a
	// courtesy, not a control — every endpoint checks for itself.
	Capabilities []auth.Capability `json:"capabilities"`

	// Linked is false when no Headscale user matched this identity, which
	// means the member portal has nothing to show until an admin links it.
	Linked bool `json:"linked"`

	// Local is true for a password account. The UI uses it to decide whether
	// to offer a password form or point at the identity provider.
	Local bool `json:"local"`
}

type meOutput struct {
	Body MeBody
}

// AuthStatusBody tells an anonymous browser how it may sign in.
type AuthStatusBody struct {
	Authenticated bool `json:"authenticated"`

	// LocalEnabled is always true: a password account is created on first
	// run, so there is always a way in. It is reported rather than assumed
	// so the login screen has one shape of answer to render from.
	LocalEnabled bool `json:"localEnabled"`

	OIDCEnabled bool   `json:"oidcEnabled"`
	Issuer      string `json:"issuer,omitempty"`
	LoginURL    string `json:"loginUrl,omitempty"`
}

type authStatusOutput struct {
	Body AuthStatusBody
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "me",
			Method:      http.MethodGet,
			Path:        "/api/me",
			Summary:     "The signed-in account and what it may do",
			Tags:        []string{"auth"},
		}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
			p, err := require(ctx, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			return &meOutput{Body: MeBody{
				User:         p.User,
				Capabilities: auth.Capabilities(p.User.Role),
				Linked:       p.User.Linked(),
				Local:        p.User.Local(),
			}}, nil
		})

		// Unauthenticated on purpose: the login screen has to render
		// before anyone is signed in, and it needs to know whether an
		// identity provider is configured at all.
		huma.Register(api, huma.Operation{
			OperationID: "authStatus",
			Method:      http.MethodGet,
			Path:        "/api/auth/status",
			Summary:     "Whether login is configured and whether this browser is signed in",
			Tags:        []string{"auth"},
		}, func(ctx context.Context, _ *struct{}) (*authStatusOutput, error) {
			body := AuthStatusBody{LocalEnabled: true, OIDCEnabled: deps.OIDCEnabled}

			if body.OIDCEnabled {
				body.Issuer = deps.OIDCIssuer
				body.LoginURL = "/auth/oidc"
			}

			if _, ok := auth.PrincipalFrom(ctx); ok {
				body.Authenticated = true
			}

			return &authStatusOutput{Body: body}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "changePassword",
			Method:      http.MethodPost,
			Path:        "/api/me/password",
			Summary:     "Change your own password",
			Description: "Local accounts only. An account that signs in through an " +
				"identity provider has no password here to change.",
			Tags: []string{"auth"},
		}, func(ctx context.Context, in *struct {
			Body struct {
				Current string `json:"current"`
				New     string `json:"new" minLength:"12"`
			}
		},
		) (*struct{ Body MeBody }, error) {
			p, err := require(ctx, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			if !p.User.Local() {
				return nil, huma.Error400BadRequest(
					"this account signs in through an identity provider, so it has no password here")
			}

			// The current password is required even though the caller is
			// already authenticated: it is what stops a borrowed session
			// from locking the real owner out.
			if !auth.VerifyPassword(p.User.PasswordHash, in.Body.Current) {
				return nil, huma.Error401Unauthorized("that is not your current password")
			}

			hash, err := auth.HashPassword(in.Body.New)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity("that password cannot be used", err)
			}

			if err := deps.Store.SetPassword(ctx, p.User.ID, hash); err != nil {
				return nil, statusFor(err, "could not change the password")
			}

			finish(ctx, deps, p, "account.password", "user", p.User.ID, nil, nil)

			return &struct{ Body MeBody }{Body: MeBody{
				User:         p.User,
				Capabilities: auth.Capabilities(p.User.Role),
				Linked:       p.User.Linked(),
				Local:        p.User.Local(),
			}}, nil
		})
	})
}
