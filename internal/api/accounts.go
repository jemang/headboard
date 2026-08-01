package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// Accounts are Headboard's own records — an OIDC identity, a role, and the
// Headscale user it maps to. They are distinct from Headscale users, which
// Headboard never stores.

type accountsOutput struct {
	Body struct {
		Accounts []store.User `json:"accounts"`
	}
}

type linkInput struct {
	ID   int64 `path:"id"`
	Body struct {
		// HeadscaleUserID is the Headscale user to link to. Null unlinks.
		HeadscaleUserID *int64 `json:"headscaleUserId"`
	}
}

type accountOutput struct {
	Body store.User
}

type roleInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Role store.Role `json:"role" enum:"owner,admin,network-admin,auditor,member"`
	}
}

type admissionInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Admission store.AdmissionState `json:"admission" enum:"active,rejected"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listAccounts",
			Method:      http.MethodGet,
			Path:        "/api/accounts",
			Summary:     "Headboard accounts, their roles and Headscale links",
			Tags:        []string{"accounts"},
		}, func(ctx context.Context, _ *struct{}) (*accountsOutput, error) {
			if _, err := require(ctx, auth.CapManageUsers); err != nil {
				return nil, err
			}

			accounts, err := deps.Store.ListUsers(ctx)
			if err != nil {
				return nil, statusFor(err, "could not list accounts")
			}

			out := &accountsOutput{}
			out.Body.Accounts = accounts

			return out, nil
		})

		// The link screen exists because Headscale users created with the
		// CLI carry no provider_identifier and can never match an OIDC
		// identity automatically. Without this, those people can log in
		// and see nothing, with no way to fix it.
		huma.Register(api, huma.Operation{
			OperationID: "linkAccount",
			Method:      http.MethodPut,
			Path:        "/api/accounts/{id}/headscale-user",
			Summary:     "Link a Headboard account to a Headscale user",
			Tags:        []string{"accounts"},
		}, func(ctx context.Context, in *linkInput) (*accountOutput, error) {
			p, err := require(ctx, auth.CapManageUsers)
			if err != nil {
				return nil, err
			}

			before, err := deps.Store.UserByID(ctx, in.ID)
			if err != nil {
				return nil, huma.Error404NotFound("no such account")
			}

			updated, err := deps.Store.LinkHeadscaleUser(ctx, in.ID, in.Body.HeadscaleUserID)

			if errors.Is(err, store.ErrLinkTaken) {
				return nil, huma.Error409Conflict(err.Error())
			}

			if err != nil {
				return nil, statusFor(err, "could not link account")
			}

			audit(ctx, deps, p, "account.link", "account", in.ID, before, updated)

			return &accountOutput{Body: updated}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "setAccountRole",
			Method:      http.MethodPut,
			Path:        "/api/accounts/{id}/role",
			Summary:     "Change a Headboard account's role",
			Tags:        []string{"accounts"},
		}, func(ctx context.Context, in *roleInput) (*accountOutput, error) {
			p, err := require(ctx, auth.CapManageUsers)
			if err != nil {
				return nil, err
			}

			if !in.Body.Role.Valid() {
				return nil, huma.Error422UnprocessableEntity("unknown role")
			}

			before, err := deps.Store.UserByID(ctx, in.ID)
			if err != nil {
				return nil, huma.Error404NotFound("no such account")
			}

			// Only an owner may create or remove owners, otherwise an
			// admin could promote themselves past the person who set
			// the system up.
			if (in.Body.Role == store.RoleOwner || before.Role == store.RoleOwner) &&
				!p.Can(auth.CapManageOwners) {
				return nil, huma.Error403Forbidden("only an owner can change owners")
			}

			// The last owner cannot be demoted. Losing every owner means
			// nobody can ever grant the role back.
			if before.Role == store.RoleOwner && in.Body.Role != store.RoleOwner {
				owners, err := deps.Store.CountOwners(ctx)
				if err != nil {
					return nil, statusFor(err, "could not count owners")
				}

				if owners <= 1 {
					return nil, huma.Error409Conflict(
						"this is the only owner; promote someone else first")
				}
			}

			updated, err := deps.Store.SetRole(ctx, in.ID, in.Body.Role)
			if err != nil {
				return nil, statusFor(err, "could not change role")
			}

			audit(ctx, deps, p, "account.role", "account", in.ID, before, updated)

			return &accountOutput{Body: updated}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "setAccountAdmission",
			Method:      http.MethodPut,
			Path:        "/api/accounts/{id}/admission",
			Summary:     "Approve or reject a Headboard account",
			Tags:        []string{"accounts"},
		}, func(ctx context.Context, in *admissionInput) (*accountOutput, error) {
			if in.Body.Admission != store.AdmissionActive && in.Body.Admission != store.AdmissionRejected {
				return nil, huma.Error422UnprocessableEntity("admission must be active or rejected")
			}

			updated, err := setAccountAdmission(ctx, deps, in.ID, in.Body.Admission)
			if err != nil {
				return nil, err
			}

			return &accountOutput{Body: updated}, nil
		})
	})
}

func setAccountAdmission(ctx context.Context, deps Deps, id int64, admission store.AdmissionState) (store.User, error) {
	p, err := require(ctx, auth.CapManageUsers)
	if err != nil {
		return store.User{}, err
	}

	if admission != store.AdmissionActive && admission != store.AdmissionRejected {
		return store.User{}, huma.Error422UnprocessableEntity("admission must be active or rejected")
	}

	before, err := deps.Store.UserByID(ctx, id)
	if err != nil {
		return store.User{}, huma.Error404NotFound("no such account")
	}

	updated, err := deps.Store.SetAdmission(ctx, id, admission)
	if err != nil {
		return store.User{}, statusFor(err, "could not change account admission")
	}

	audit(ctx, deps, p, "account.admission", "account", id, before.Admission, updated.Admission)

	return updated, nil
}

// audit records a mutation, best effort. A failed audit write must not undo a
// change that already happened — it is logged instead, and the gap is visible
// in the log rather than silently absent.
func audit(ctx context.Context, deps Deps, p auth.Principal, action, targetType string, targetID int64, before, after any) {
	entry := store.AuditEntry{
		ActorUserID: &p.User.ID,
		ActorLabel:  p.User.Email,
		Action:      action,
		TargetType:  targetType,
		TargetID:    itoa64(targetID),
		Before:      mustJSON(before),
		After:       mustJSON(after),
	}

	if err := deps.Store.Audit(ctx, entry); err != nil {
		deps.Log.Error("audit write failed", "action", action, "target", targetID, "err", err)
	}
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}

	return b
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}

	neg := v < 0
	if neg {
		v = -v
	}

	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}

	if neg {
		b = append([]byte{'-'}, b...)
	}

	return string(b)
}
