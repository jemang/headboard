package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// require returns the caller, or an error the handler should return unchanged.
//
// Every protected handler starts with this. Authorisation is never inferred
// from the shape of a route or from what the UI chose to render — the server
// decides, per request, from the capability table.
func require(ctx context.Context, cap auth.Capability) (auth.Principal, error) {
	p, err := requireAuthenticated(ctx)
	if err != nil {
		return auth.Principal{}, err
	}

	if !p.User.Active() {
		return auth.Principal{}, admissionDenied(p.User.Admission)
	}

	if !p.Can(cap) {
		return auth.Principal{}, huma.Error403Forbidden("your role does not allow this")
	}

	return p, nil
}

// requireAuthenticated returns a signed-in principal without granting access
// to Headboard resources. It is reserved for safe status endpoints.
func requireAuthenticated(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, huma.Error401Unauthorized("not signed in")
	}

	return p, nil
}

func admissionDenied(admission store.AdmissionState) error {
	if admission == store.AdmissionPending {
		return huma.Error403Forbidden("account approval is pending")
	}

	return huma.Error403Forbidden("account access was rejected")
}

// requireLinked is for member-scoped endpoints. An identity that logged in but
// was never linked to a Headscale user owns no devices, and saying so plainly
// beats returning an empty list that looks like "you have no devices".
func requireLinked(ctx context.Context, cap auth.Capability) (auth.Principal, error) {
	p, err := require(ctx, cap)
	if err != nil {
		return auth.Principal{}, err
	}

	if !p.User.Linked() {
		return auth.Principal{}, huma.Error409Conflict(
			"your account is not linked to a headscale user yet; an administrator has to link it")
	}

	return p, nil
}

// statusFor maps a store error onto an HTTP status without leaking the query.
func statusFor(err error, notFound string) error {
	if err == nil {
		return nil
	}

	return huma.NewError(http.StatusInternalServerError, notFound, err)
}
