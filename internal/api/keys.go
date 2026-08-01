package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/hs"
)

// API keys mint all-access credentials and pre-auth keys may enrol a device
// without approval. Both need explicit administrator handling.

type preAuthKeysOutput struct {
	Body struct {
		Keys []hs.PreAuthKey `json:"keys"`
	}
}

type revokeActivePreAuthKeysOutput struct {
	Body struct {
		Expired []string `json:"expired"`
		Failed  []string `json:"failed"`
	}
}

type apiKeysOutput struct {
	Body struct {
		Keys []apiKey `json:"keys"`
	}
}

// apiKey augments the Headscale response with a public safety marker. It
// deliberately contains only the configured key's prefix, never its secret.
type apiKey struct {
	hs.APIKey
	Protected bool `json:"protected,omitempty"`
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
			Description: "Legacy automatic-enrolment credentials. Secrets are not returned by a list; " +
				"Headscale stores a hash.",
			Tags: []string{"keys"},
		}, func(ctx context.Context, _ *struct{}) (*preAuthKeysOutput, error) {
			if _, err := require(ctx, auth.CapManageKeys); err != nil {
				return nil, err
			}

			keys, err := deps.Mutator.ListPreAuthKeys(ctx, 0)
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
			Summary:     "Pre-auth key creation is disabled",
			Description: "Devices must be explicitly approved through a registration request.",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if _, err := require(ctx, auth.CapManageKeys); err != nil {
				return nil, err
			}

			return nil, huma.Error403Forbidden(
				"automatic device enrolment is disabled; submit a registration request for approval")
		})

		huma.Register(api, huma.Operation{
			OperationID: "revokeActivePreAuthKeys",
			Method:      http.MethodPost,
			Path:        "/api/preauth-keys/revoke-active",
			Summary:     "Revoke every active pre-auth key",
			Tags:        []string{"keys"},
		}, func(ctx context.Context, _ *struct{}) (*revokeActivePreAuthKeysOutput, error) {
			p, err := require(ctx, auth.CapManageKeys)
			if err != nil {
				return nil, err
			}

			keys, err := deps.Mutator.ListPreAuthKeys(ctx, 0)
			if err != nil {
				return nil, upstream(err, "could not list pre-auth keys")
			}

			expired, failed := expireActivePreAuthKeys(ctx, keys, time.Now(), deps.Mutator.ExpirePreAuthKey)
			out := &revokeActivePreAuthKeysOutput{}
			out.Body.Expired = expired
			out.Body.Failed = failed

			if len(expired) > 0 {
				finish(ctx, deps, p, "preauthkey.revoke_active", "preauthkey", 0, nil, out.Body)
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
			out.Body.Keys = make([]apiKey, 0, len(keys))

			for _, key := range keys {
				out.Body.Keys = append(out.Body.Keys, apiKey{
					APIKey:    key,
					Protected: isProtectedAPIKey(key.Prefix, deps.HeadscaleAPIKeyPrefix),
				})
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
			if isProtectedAPIKey(in.Prefix, deps.HeadscaleAPIKeyPrefix) {
				return nil, huma.Error409Conflict(
					"cannot revoke Headboard's own API key; replace its server configuration first")
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
			if isProtectedAPIKey(in.Prefix, deps.HeadscaleAPIKeyPrefix) {
				return nil, huma.Error409Conflict(
					"cannot delete Headboard's own API key; replace its server configuration first")
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

func activePreAuthKeys(keys []hs.PreAuthKey, now time.Time) []hs.PreAuthKey {
	active := make([]hs.PreAuthKey, 0, len(keys))
	for _, key := range keys {
		if key.Expiry != nil && !key.Expiry.After(now) {
			continue
		}
		active = append(active, key)
	}

	return active
}

func expireActivePreAuthKeys(ctx context.Context, keys []hs.PreAuthKey, now time.Time, expire func(context.Context, string) error) ([]string, []string) {
	active := activePreAuthKeys(keys, now)
	expired := make([]string, 0, len(active))
	failed := make([]string, 0)

	for _, key := range active {
		if err := expire(ctx, key.ID); err != nil {
			failed = append(failed, key.ID)
			continue
		}
		expired = append(expired, key.ID)
	}

	return expired, failed
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

const apiKeyPrefixLength = 12

// keyPrefix is the public part of an API key: what identifies it for
// revocation. Everything after it is the secret.
func keyPrefix(key string) string { return hs.APIKeyPrefix(key) }

// isProtectedAPIKey identifies the credential Headboard itself uses. Prefixes
// are public, so matching them does not expose the configured secret.
func isProtectedAPIKey(prefix, protectedPrefix string) bool {
	return protectedPrefix != "" && hs.APIKeyPrefix(prefix) == protectedPrefix
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
