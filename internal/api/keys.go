package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/tailnet"
)

// Pre-auth keys and API keys both mint credentials, so both are behind
// CapManageKeys — except a member minting a key for their own user, which is
// self-service and scoped to them.

type preAuthKeysOutput struct {
	Body struct {
		Keys []hs.PreAuthKey `json:"keys"`
	}
}

type createPreAuthKeyInput struct {
	Body struct {
		// User the key registers devices as. Optional: a member who
		// omits it gets their own. Huma treats a field without
		// omitempty as required, and requiring it here would force
		// members to name themselves.
		User string `json:"user,omitempty"`

		Reusable  bool `json:"reusable,omitempty"`
		Ephemeral bool `json:"ephemeral,omitempty"`

		// ExpiresIn is a duration such as "24h" or "720h". Empty leaves
		// Headscale's default.
		ExpiresIn string `json:"expiresIn,omitempty" example:"24h"`

		// Tags make every device registered with this key a tagged node,
		// owned by the tag rather than by the user.
		Tags []string `json:"tags,omitempty"`
	}
}

type createPreAuthKeyOutput struct {
	Body struct {
		Key hs.PreAuthKey `json:"key"`

		// Command is ready to paste on the device being enrolled.
		Command string `json:"command"`

		// LoginServer is the address inside that command, reported on its
		// own so the UI can show which host a device is being pointed at.
		LoginServer string `json:"loginServer"`

		// LoginServerProblem is set when that address looks like one only
		// Headboard can reach. Advisory — Headboard cannot know what a
		// device can resolve — but the alternative is a command that copies
		// cleanly and fails on someone else's machine.
		LoginServerProblem string `json:"loginServerProblem,omitempty"`

		// Warning states plainly that the secret is not recoverable.
		Warning string `json:"warning"`
	}
}

type apiKeysOutput struct {
	Body struct {
		Keys []hs.APIKey `json:"keys"`
	}
}

type createAPIKeyInput struct {
	Body struct {
		ExpiresIn string `json:"expiresIn,omitempty" example:"2160h"`
	}
}

type createAPIKeyOutput struct {
	Body struct {
		Key     string `json:"key"`
		Warning string `json:"warning"`
	}
}

type apiKeyPrefixInput struct {
	Prefix string `path:"prefix"`
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listPreAuthKeys",
			Method:      http.MethodGet,
			Path:        "/api/preauth-keys",
			Summary:     "Pre-auth keys",
			Description: "Members see only their own. Secrets are not returned by a list; " +
				"Headscale stores a hash.",
			Tags: []string{"keys"},
		}, func(ctx context.Context, _ *struct{}) (*preAuthKeysOutput, error) {
			p, err := require(ctx, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			var userID uint

			if !p.Can(auth.CapManageKeys) {
				u, err := ownHeadscaleUser(ctx, deps, p)
				if err != nil {
					return nil, err
				}

				userID = u.ID
			}

			keys, err := deps.Mutator.ListPreAuthKeys(ctx, userID)
			if err != nil {
				return nil, upstream(err, "could not list pre-auth keys")
			}

			out := &preAuthKeysOutput{}
			out.Body.Keys = keys

			if out.Body.Keys == nil {
				out.Body.Keys = []hs.PreAuthKey{}
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "createPreAuthKey",
			Method:      http.MethodPost,
			Path:        "/api/preauth-keys",
			Summary:     "Mint a pre-auth key",
			Description: "Returns the secret exactly once. Members may only mint keys for " +
				"themselves, and may not tag them: a tagged key produces devices they would " +
				"not own.",
			Tags: []string{"keys"},
		}, func(ctx context.Context, in *createPreAuthKeyInput) (*createPreAuthKeyOutput, error) {
			p, err := require(ctx, auth.CapViewSelf)
			if err != nil {
				return nil, err
			}

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}

			user := in.Body.User

			if !p.Can(auth.CapManageKeys) {
				own, err := ownHeadscaleUser(ctx, deps, p)
				if err != nil {
					return nil, err
				}

				if user != "" && user != own.Name {
					return nil, huma.Error403Forbidden(
						"you can only create keys for your own user")
				}

				user = own.Name

				// A tagged key mints nodes owned by the tag, which
				// takes them outside the creator's own devices.
				if len(in.Body.Tags) > 0 {
					return nil, huma.Error403Forbidden(
						"only an administrator can create tagged keys")
				}
			}

			if user == "" {
				return nil, huma.Error422UnprocessableEntity("user is required")
			}

			// The API takes a numeric user id, so the name has to be
			// resolved against the snapshot first.
			userID, ok := userIDByName(snap, user)
			if !ok {
				return nil, huma.Error404NotFound("no such headscale user: " + user)
			}

			expiry, err := parseExpiry(in.Body.ExpiresIn)
			if err != nil {
				return nil, err
			}

			for _, tag := range in.Body.Tags {
				if !strings.HasPrefix(tag, "tag:") {
					return nil, huma.Error422UnprocessableEntity(
						"tags must start with \"tag:\", got " + tag)
				}
			}

			key, err := deps.Mutator.CreatePreAuthKey(ctx, userID,
				in.Body.Reusable, in.Body.Ephemeral, expiry, in.Body.Tags)
			if err != nil {
				return nil, upstream(err, "could not create the pre-auth key")
			}

			// The audit entry records that a key was minted and for
			// whom — never the secret itself.
			finish(ctx, deps, p, "preauthkey.create", "user", 0, nil, map[string]any{
				"user":      user,
				"reusable":  in.Body.Reusable,
				"ephemeral": in.Body.Ephemeral,
				"tags":      in.Body.Tags,
			})

			out := &createPreAuthKeyOutput{}
			out.Body.Key = key
			out.Body.Command = enrolCommand(deps, key.Key)
			out.Body.LoginServer = loginServer(deps)
			out.Body.Warning = "This key is shown once. Headscale stores only a hash, so it cannot be retrieved again."

			if problem := unreachableLoginServer(out.Body.LoginServer); problem != "" {
				out.Body.LoginServerProblem = problem
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "expirePreAuthKey",
			Method:      http.MethodPost,
			Path:        "/api/preauth-keys/expire",
			Summary:     "Expire a pre-auth key",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, in *struct {
			Body struct {
				// ID identifies the key. Headscale stores only a
				// hash of the secret, so the secret itself is not
				// a handle once issued.
				ID string `json:"id"`
			}
		},
		) (*struct{}, error) {
			p, err := require(ctx, auth.CapManageKeys)
			if err != nil {
				return nil, err
			}

			if err := deps.Mutator.ExpirePreAuthKey(ctx, in.Body.ID); err != nil {
				return nil, upstream(err, "could not expire the key")
			}

			finish(ctx, deps, p, "preauthkey.expire", "preauthkey", 0, nil,
				map[string]string{"id": in.Body.ID})

			return nil, nil
		})

		// Headscale API keys are all-access admin credentials with no
		// read-only scope on v0.29.x, so this is owner/admin territory
		// and every action is loud in the audit log.
		huma.Register(api, huma.Operation{
			OperationID: "listAPIKeys",
			Method:      http.MethodGet,
			Path:        "/api/headscale-keys",
			Summary:     "Headscale API keys",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, _ *struct{}) (*apiKeysOutput, error) {
			if _, err := require(ctx, auth.CapManageKeys); err != nil {
				return nil, err
			}

			keys, err := deps.Mutator.ListAPIKeys(ctx)
			if err != nil {
				return nil, upstream(err, "could not list API keys")
			}

			out := &apiKeysOutput{}
			out.Body.Keys = keys

			if out.Body.Keys == nil {
				out.Body.Keys = []hs.APIKey{}
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "createAPIKey",
			Method:      http.MethodPost,
			Path:        "/api/headscale-keys",
			Summary:     "Mint a Headscale API key",
			Description: "The result is an all-access admin credential for the whole tailnet, " +
				"shown once.",
			Tags: []string{"keys"},
		}, func(ctx context.Context, in *createAPIKeyInput) (*createAPIKeyOutput, error) {
			p, err := require(ctx, auth.CapManageKeys)
			if err != nil {
				return nil, err
			}

			expiry, err := parseExpiry(in.Body.ExpiresIn)
			if err != nil {
				return nil, err
			}

			key, err := deps.Mutator.CreateAPIKey(ctx, expiry)
			if err != nil {
				return nil, upstream(err, "could not create the API key")
			}

			finish(ctx, deps, p, "apikey.create", "apikey", 0, nil,
				map[string]string{"prefix": keyPrefix(key)})

			out := &createAPIKeyOutput{}
			out.Body.Key = key
			out.Body.Warning = "This is an all-access admin credential for your whole tailnet. " +
				"It is shown once and cannot be retrieved again."

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "expireAPIKey",
			Method:      http.MethodPost,
			Path:        "/api/headscale-keys/{prefix}/expire",
			Summary:     "Revoke a Headscale API key",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, in *apiKeyPrefixInput) (*struct{}, error) {
			p, err := require(ctx, auth.CapManageKeys)
			if err != nil {
				return nil, err
			}

			if err := deps.Mutator.ExpireAPIKey(ctx, in.Prefix); err != nil {
				return nil, upstream(err, "could not expire the API key")
			}

			finish(ctx, deps, p, "apikey.expire", "apikey", 0, nil,
				map[string]string{"prefix": safePrefix(in.Prefix)})

			return nil, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "deleteAPIKey",
			Method:      http.MethodDelete,
			Path:        "/api/headscale-keys/{prefix}",
			Summary:     "Delete a Headscale API key",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, in *apiKeyPrefixInput) (*struct{}, error) {
			p, err := require(ctx, auth.CapManageKeys)
			if err != nil {
				return nil, err
			}

			if err := deps.Mutator.DeleteAPIKey(ctx, in.Prefix); err != nil {
				return nil, upstream(err, "could not delete the API key")
			}

			finish(ctx, deps, p, "apikey.delete", "apikey", 0, nil,
				map[string]string{"prefix": safePrefix(in.Prefix)})

			return nil, nil
		})
	})
}

// ownHeadscaleUser resolves the caller's Headscale user, which member-scoped
// key operations are limited to.
func ownHeadscaleUser(ctx context.Context, deps Deps, p auth.Principal) (types.User, error) {
	if !p.User.Linked() {
		return types.User{}, huma.Error409Conflict(
			"your account is not linked to a headscale user yet")
	}

	snap, err := currentSnapshot(deps)
	if err != nil {
		return types.User{}, err
	}

	u, ok := headscaleUser(snap, *p.User.HeadscaleUserID)
	if !ok {
		return types.User{}, huma.Error409Conflict(
			"the headscale user this account is linked to no longer exists")
	}

	return u, nil
}

func userIDByName(snap *tailnet.Snapshot, name string) (uint, bool) {
	for _, u := range snap.Users {
		if u.Name == name {
			return u.ID, true
		}
	}

	return 0, false
}

// parseExpiry turns a duration string into an absolute time, because that is
// what Headscale takes.
func parseExpiry(in string) (time.Time, error) {
	if in == "" {
		return time.Time{}, nil
	}

	d, err := time.ParseDuration(in)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(
			"expiresIn must be a duration such as 24h or 720h")
	}

	if d <= 0 {
		return time.Time{}, huma.Error422UnprocessableEntity("expiresIn must be positive")
	}

	return time.Now().Add(d), nil
}

// enrolCommand is what gets pasted on the device being added.
//
// It uses the public address rather than the one Headboard dials: those differ
// whenever Headscale is reached over an internal name, and a command naming
// `headscale:8080` is one a device cannot act on.
func enrolCommand(deps Deps, key string) string {
	if key == "" {
		return ""
	}

	return fmt.Sprintf("tailscale up --login-server %s --authkey %s",
		loginServer(deps), key)
}

// loginServer is the address devices enrol against.
func loginServer(deps Deps) string {
	if deps.HeadscalePublicURL != "" {
		return deps.HeadscalePublicURL
	}

	return deps.HeadscaleURL
}

// unreachableLoginServer reports whether the login server looks like an address
// only this process can resolve, and says why.
//
// A guess, deliberately: Headboard cannot know what a device can reach. It is
// worth guessing because the failure is otherwise silent — the command copies
// cleanly, and only fails on someone else's machine.
func unreachableLoginServer(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}

	host := u.Hostname()

	switch {
	case host == "localhost" || host == "127.0.0.1" || host == "::1":
		return "points at this server's own loopback address"
	case !strings.Contains(host, "."):
		// A bare label resolves inside a container network and nowhere
		// else — this is exactly the docker-compose service-name case.
		return "is a bare hostname, which only resolves on Headboard's own network"
	default:
		return ""
	}
}

// Headscale API keys are "hskey-api-" followed by a 12-character public prefix
// and a 64-character secret, all concatenated with no separator
// (hscontrol/db/api_key.go). There is nothing to split on, so the prefix has to
// be taken by length.
const (
	apiKeyMarker       = "hskey-api-"
	apiKeyPrefixLength = 12
	legacyPrefixLength = 7
)

// keyPrefix is the public part of an API key: what identifies it for
// revocation. Everything after it is the secret.
func keyPrefix(key string) string {
	body := strings.TrimPrefix(key, apiKeyMarker)

	n := apiKeyPrefixLength
	if len(body) < n {
		n = len(body)
	}

	return body[:n]
}

// safePrefix bounds a caller-supplied prefix before it is written anywhere.
//
// The expire and delete endpoints take a prefix from the browser, and pasting a
// whole key into that field is an easy mistake. Truncating here means a slip
// cannot put an all-access credential into the audit log, which is durable and
// readable by every auditor.
func safePrefix(in string) string {
	p := keyPrefix(in)

	if len(p) > apiKeyPrefixLength {
		return p[:apiKeyPrefixLength]
	}

	return p
}
