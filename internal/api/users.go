package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
)

// TailnetUser is a Headscale user — the owner of devices. Distinct from a
// Headboard account, which is an OIDC identity with a role.
type TailnetUser struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`

	// ProviderID is Headscale's provider_identifier. Empty means the user
	// was created with the CLI and can never be matched to an OIDC login
	// automatically — the admin console has to link it by hand.
	ProviderID string `json:"providerId,omitempty"`

	Devices int `json:"devices"`

	// LinkedAccountID is the Headboard account claiming this user, if any.
	LinkedAccountID *int64 `json:"linkedAccountId,omitempty"`
}

type usersOutput struct {
	Body struct {
		Users []TailnetUser `json:"users"`
	}
}

type createUserInput struct {
	Body struct {
		Name        string `json:"name" minLength:"1" maxLength:"63"`
		DisplayName string `json:"displayName,omitempty"`
		Email       string `json:"email,omitempty"`
	}
}

type userIDInput struct {
	ID uint `path:"id"`
}

type renameUserInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name string `json:"name" minLength:"1" maxLength:"63"`
	}
}

type userOutput struct {
	Body TailnetUser
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listTailnetUsers",
			Method:      http.MethodGet,
			Path:        "/api/users",
			Summary:     "Headscale users, with device counts and account links",
			Tags:        []string{"users"},
		}, func(ctx context.Context, _ *struct{}) (*usersOutput, error) {
			if _, err := require(ctx, auth.CapViewAll); err != nil {
				return nil, err
			}

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}

			// One pass to count devices per owner, rather than a scan
			// per user.
			counts := make(map[uint]int, len(snap.Users))
			for _, n := range snap.Nodes {
				if n.UserID != nil {
					counts[*n.UserID]++
				}
			}

			links := make(map[int64]int64)

			if accounts, err := deps.Store.ListUsers(ctx); err == nil {
				for _, a := range accounts {
					if a.HeadscaleUserID != nil {
						links[*a.HeadscaleUserID] = a.ID
					}
				}
			}

			out := &usersOutput{}
			out.Body.Users = []TailnetUser{}

			for _, u := range snap.Users {
				tu := TailnetUser{
					ID:          u.ID,
					Name:        u.Name,
					DisplayName: u.DisplayName,
					Email:       u.Email,
					Devices:     counts[u.ID],
				}

				if u.ProviderIdentifier.Valid {
					tu.ProviderID = u.ProviderIdentifier.String
				}

				if id, ok := links[int64(u.ID)]; ok {
					tu.LinkedAccountID = &id
				}

				out.Body.Users = append(out.Body.Users, tu)
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "createTailnetUser",
			Method:      http.MethodPost,
			Path:        "/api/users",
			Summary:     "Create a Headscale user",
			Description: "A user created here has no OIDC provider identifier, so it will not " +
				"match a login automatically and must be linked from the accounts page.",
			Tags: []string{"users"},
		}, func(ctx context.Context, in *createUserInput) (*userOutput, error) {
			p, err := require(ctx, auth.CapManageUsers)
			if err != nil {
				return nil, err
			}

			user, err := deps.Mutator.CreateUser(ctx, in.Body.Name, in.Body.DisplayName, in.Body.Email)
			if err != nil {
				return nil, upstream(err, "could not create the user")
			}

			finish(ctx, deps, p, "user.create", "user", int64(user.ID), nil, in.Body)

			return &userOutput{Body: TailnetUser{
				ID: user.ID, Name: user.Name,
				DisplayName: user.DisplayName, Email: user.Email,
			}}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "renameTailnetUser",
			Method:      http.MethodPost,
			Path:        "/api/users/{id}/rename",
			Summary:     "Rename a Headscale user",
			Tags:        []string{"users"},
		}, func(ctx context.Context, in *renameUserInput) (*userOutput, error) {
			p, err := require(ctx, auth.CapManageUsers)
			if err != nil {
				return nil, err
			}

			user, err := deps.Mutator.RenameUser(ctx, in.ID, in.Body.Name)
			if err != nil {
				return nil, upstream(err, "could not rename the user")
			}

			finish(ctx, deps, p, "user.rename", "user", int64(in.ID), nil, in.Body)

			return &userOutput{Body: TailnetUser{
				ID: user.ID, Name: user.Name,
				DisplayName: user.DisplayName, Email: user.Email,
			}}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "deleteTailnetUser",
			Method:      http.MethodDelete,
			Path:        "/api/users/{id}",
			Summary:     "Delete a Headscale user",
			Description: "Refused while the user still owns devices: deleting one upstream " +
				"removes their machines from the tailnet with no way back.",
			Tags: []string{"users"},
		}, func(ctx context.Context, in *userIDInput) (*struct{}, error) {
			p, err := require(ctx, auth.CapManageUsers)
			if err != nil {
				return nil, err
			}

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}

			var owned int

			for _, n := range snap.Nodes {
				if n.UserID != nil && *n.UserID == in.ID {
					owned++
				}
			}

			// Headscale would cascade the delete. Making that explicit
			// here means nobody removes a person and silently takes
			// their laptops off the network.
			if owned > 0 {
				return nil, huma.Error409Conflict(
					"this user still owns devices; remove or reassign them first")
			}

			if err := deps.Mutator.DeleteUser(ctx, in.ID); err != nil {
				return nil, upstream(err, "could not delete the user")
			}

			finish(ctx, deps, p, "user.delete", "user", int64(in.ID), nil, nil)

			return nil, nil
		})
	})
}
