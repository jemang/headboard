package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
)

// SSHKeyView is a stored public key as the admin UI shows it.
type SSHKeyView struct {
	ID              int64     `json:"id"`
	HeadscaleUserID uint64    `json:"headscaleUserId"`
	Type            string    `json:"type"`
	Fingerprint     string    `json:"fingerprint"`
	Label           string    `json:"label"`
	CreatedAt       time.Time `json:"createdAt"`
}

func sshKeyView(k store.SSHKey) SSHKeyView {
	return SSHKeyView{
		ID:              k.ID,
		HeadscaleUserID: k.HeadscaleUserID,
		Type:            strings.SplitN(k.PublicKey, " ", 2)[0],
		Fingerprint:     k.Fingerprint,
		Label:           k.Label,
		CreatedAt:       k.CreatedAt,
	}
}

func tailnetUserExists(users []types.User, id uint) bool {
	for _, u := range users {
		if u.ID == id {
			return true
		}
	}

	return false
}

type sshKeysOutput struct {
	Body struct {
		Keys []SSHKeyView `json:"keys"`
	}
}

type addSSHKeyInput struct {
	ID   uint `path:"id"`
	Body struct {
		PublicKey string `json:"publicKey" minLength:"1" maxLength:"16384"`
		Label     string `json:"label,omitempty" maxLength:"64"`
	}
}

type sshKeyOutput struct {
	Body SSHKeyView
}

type sshKeyIDInput struct {
	ID int64 `path:"id"`
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listSSHKeys",
			Method:      http.MethodGet,
			Path:        "/api/users/{id}/ssh-keys",
			Summary:     "SSH public keys of a Headscale user",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *userIDInput) (*sshKeysOutput, error) {
			if _, err := require(ctx, auth.CapManageSSH); err != nil {
				return nil, err
			}

			id := uint64(in.ID)
			keys, err := deps.Store.ListSSHKeys(ctx, &id)
			if err != nil {
				return nil, statusFor(err, "could not list keys")
			}

			out := &sshKeysOutput{}
			out.Body.Keys = make([]SSHKeyView, 0, len(keys))
			for _, k := range keys {
				out.Body.Keys = append(out.Body.Keys, sshKeyView(k))
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "addSSHKey",
			Method:      http.MethodPost,
			Path:        "/api/users/{id}/ssh-keys",
			Summary:     "Add an SSH public key to a Headscale user",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *addSSHKeyInput) (*sshKeyOutput, error) {
			p, err := require(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}
			if !tailnetUserExists(snap.Users, in.ID) {
				return nil, huma.Error404NotFound("no such user")
			}

			key, err := sshsync.ParseKey(in.Body.PublicKey)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}

			label, err := sshsync.CleanLabel(in.Body.Label)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}

			k, err := deps.Store.AddSSHKey(ctx, store.SSHKey{
				HeadscaleUserID: uint64(in.ID),
				PublicKey:       key.Line,
				Fingerprint:     key.Fingerprint,
				Label:           label,
			}, p.User.ID)
			if errors.Is(err, store.ErrDuplicateSSHKey) {
				return nil, huma.Error409Conflict(err.Error())
			}
			if err != nil {
				return nil, statusFor(err, "could not add the key")
			}

			audit(ctx, deps, p, "ssh_key.add", "ssh_key", k.ID, nil, sshKeyView(k))

			return &sshKeyOutput{Body: sshKeyView(k)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "deleteSSHKey",
			Method:      http.MethodDelete,
			Path:        "/api/ssh-keys/{id}",
			Summary:     "Remove an SSH public key",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *sshKeyIDInput) (*struct{}, error) {
			p, err := require(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			k, err := deps.Store.DeleteSSHKey(ctx, in.ID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, huma.Error404NotFound("no such key")
			}
			if err != nil {
				return nil, statusFor(err, "could not remove the key")
			}

			audit(ctx, deps, p, "ssh_key.delete", "ssh_key", k.ID, sshKeyView(k), nil)

			return nil, nil
		})
	})
}
