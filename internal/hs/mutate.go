package hs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
)

// Mutations against Headscale. Every one of these is an admin action, so each
// caller is expected to have checked a capability and to record an audit entry:
// Headscale keeps no history of its own.

// RenameNode changes a node's given name.
func (h *HTTP) RenameNode(ctx context.Context, id types.NodeID, name string) (types.Node, error) {
	// The new name is a path segment in this API, so it has to be escaped
	// rather than interpolated.
	path := fmt.Sprintf("/api/v1/node/%d/rename/%s", id, url.PathEscape(name))

	return h.nodeCall(ctx, http.MethodPost, path, nil)
}

// SetTags replaces a node's tags. Passing an empty slice clears them.
func (h *HTTP) SetTags(ctx context.Context, id types.NodeID, tags []string) (types.Node, error) {
	if tags == nil {
		tags = []string{}
	}

	body := map[string]any{"tags": tags}

	return h.nodeCall(ctx, http.MethodPost, fmt.Sprintf("/api/v1/node/%d/tags", id), body)
}

// ApproveRoutes replaces the set of routes a node may advertise. It is a
// replacement, not an addition: passing an empty slice revokes everything,
// which is how a route is withdrawn.
func (h *HTTP) ApproveRoutes(ctx context.Context, id types.NodeID, routes []string) (types.Node, error) {
	if routes == nil {
		routes = []string{}
	}

	body := map[string]any{"routes": routes}

	return h.nodeCall(ctx, http.MethodPost, fmt.Sprintf("/api/v1/node/%d/approve_routes", id), body)
}

// ExpireNode expires a node's key, forcing the device to re-authenticate.
func (h *HTTP) ExpireNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	return h.nodeCall(ctx, http.MethodPost, fmt.Sprintf("/api/v1/node/%d/expire", id), nil)
}

// DeleteNode removes a node from the tailnet.
func (h *HTTP) DeleteNode(ctx context.Context, id types.NodeID) error {
	return h.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/node/%d", id), nil, nil)
}

// ApproveRegistration completes a pending node registration.
//
// There is no endpoint to *list* pending registrations on v0.29.3 — only
// approve, reject and register, each by id. The id comes from the URL the
// Tailscale client prints, so it reaches an admin out of band.
func (h *HTTP) ApproveRegistration(ctx context.Context, authID string) error {
	return h.do(ctx, http.MethodPost, "/api/v1/auth/approve",
		map[string]any{"authId": authID}, nil)
}

// RejectRegistration refuses a pending node registration.
func (h *HTTP) RejectRegistration(ctx context.Context, authID string) error {
	return h.do(ctx, http.MethodPost, "/api/v1/auth/reject",
		map[string]any{"authId": authID}, nil)
}

// RegisterNode completes a pending registration against a user, which is what
// an admin does when approving a device on someone's behalf.
func (h *HTTP) RegisterNode(ctx context.Context, authID, user string) (types.Node, error) {
	var resp struct {
		Node v1Node `json:"node"`
	}

	body := map[string]any{"authId": authID, "user": user}

	if err := h.do(ctx, http.MethodPost, "/api/v1/auth/register", body, &resp); err != nil {
		return types.Node{}, err
	}

	return mapNode(resp.Node)
}

// CreateUser adds a Headscale user.
func (h *HTTP) CreateUser(ctx context.Context, name, displayName, email string) (types.User, error) {
	var resp struct {
		User v1User `json:"user"`
	}

	body := map[string]any{"name": name}
	if displayName != "" {
		body["displayName"] = displayName
	}

	if email != "" {
		body["email"] = email
	}

	if err := h.do(ctx, http.MethodPost, "/api/v1/user", body, &resp); err != nil {
		return types.User{}, err
	}

	user, err := mapUser(&resp.User)

	return user, err
}

// RenameUser changes a Headscale username.
func (h *HTTP) RenameUser(ctx context.Context, id uint, newName string) (types.User, error) {
	var resp struct {
		User v1User `json:"user"`
	}

	path := fmt.Sprintf("/api/v1/user/%d/rename/%s", id, url.PathEscape(newName))

	if err := h.do(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return types.User{}, err
	}

	user, err := mapUser(&resp.User)

	return user, err
}

// DeleteUser removes a Headscale user.
func (h *HTTP) DeleteUser(ctx context.Context, id uint) error {
	return h.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/user/%d", id), nil, nil)
}

// PreAuthKey is a registration credential.
type PreAuthKey struct {
	ID        string     `json:"id"`
	Key       string     `json:"key,omitempty"`
	User      string     `json:"user,omitempty"`
	UserID    uint       `json:"userId,omitempty"`
	Reusable  bool       `json:"reusable"`
	Ephemeral bool       `json:"ephemeral"`
	Used      bool       `json:"used"`
	Tags      []string   `json:"tags,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	Expiry    *time.Time `json:"expiry,omitempty"`
}

// ListPreAuthKeys returns pre-auth keys, filtered to one user when userID is
// non-zero.
//
// The filter is applied here rather than upstream: v0.29.3's GET
// /api/v1/preauthkey takes no parameters and always returns every key. A
// member-scoped list therefore has to be narrowed after the fact.
func (h *HTTP) ListPreAuthKeys(ctx context.Context, userID uint) ([]PreAuthKey, error) {
	var resp struct {
		PreAuthKeys []v1PreAuthKey `json:"preAuthKeys"`
	}

	if err := h.get(ctx, "/api/v1/preauthkey", &resp); err != nil {
		return nil, err
	}

	out := make([]PreAuthKey, 0, len(resp.PreAuthKeys))

	for i := range resp.PreAuthKeys {
		k := mapPreAuthKey(resp.PreAuthKeys[i])

		if userID != 0 && k.UserID != userID {
			continue
		}

		out = append(out, k)
	}

	return out, nil
}

// CreatePreAuthKey mints a registration credential. The secret is returned
// once and never again — Headscale stores only a hash.
//
// The user is a numeric id, not a name: v0.29.3 declares the field as uint64
// and rejects a username with a proto decoding error.
func (h *HTTP) CreatePreAuthKey(ctx context.Context, userID uint, reusable, ephemeral bool, expiry time.Time, tags []string) (PreAuthKey, error) {
	body := map[string]any{
		"user":      strconv.FormatUint(uint64(userID), 10),
		"reusable":  reusable,
		"ephemeral": ephemeral,
	}

	if !expiry.IsZero() {
		body["expiration"] = expiry.UTC().Format(time.RFC3339)
	}

	if len(tags) > 0 {
		body["aclTags"] = tags
	}

	var resp struct {
		PreAuthKey v1PreAuthKey `json:"preAuthKey"`
	}

	if err := h.do(ctx, http.MethodPost, "/api/v1/preauthkey", body, &resp); err != nil {
		return PreAuthKey{}, err
	}

	return mapPreAuthKey(resp.PreAuthKey), nil
}

// ExpirePreAuthKey invalidates a pre-auth key by its id.
//
// The id, not the secret: Headscale stores only a hash of the key itself, so
// the secret is not a usable handle once it has been issued.
func (h *HTTP) ExpirePreAuthKey(ctx context.Context, id string) error {
	return h.do(ctx, http.MethodPost, "/api/v1/preauthkey/expire",
		map[string]any{"id": id}, nil)
}

// APIKey is a Headscale admin credential. The secret is never returned by a
// list — only the prefix, which is what identifies it for revocation.
type APIKey struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Expiration *time.Time `json:"expiration,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	LastSeen   *time.Time `json:"lastSeen,omitempty"`
}

// ListAPIKeys returns every Headscale API key.
func (h *HTTP) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var resp struct {
		APIKeys []APIKey `json:"apiKeys"`
	}

	if err := h.get(ctx, "/api/v1/apikey", &resp); err != nil {
		return nil, err
	}

	return resp.APIKeys, nil
}

// CreateAPIKey mints an admin credential. The returned secret is all-access and
// is shown once.
func (h *HTTP) CreateAPIKey(ctx context.Context, expiry time.Time) (string, error) {
	body := map[string]any{}
	if !expiry.IsZero() {
		body["expiration"] = expiry.UTC().Format(time.RFC3339)
	}

	var resp struct {
		APIKey string `json:"apiKey"`
	}

	if err := h.do(ctx, http.MethodPost, "/api/v1/apikey", body, &resp); err != nil {
		return "", err
	}

	return resp.APIKey, nil
}

// ExpireAPIKey revokes an API key by prefix.
func (h *HTTP) ExpireAPIKey(ctx context.Context, prefix string) error {
	return h.do(ctx, http.MethodPost, "/api/v1/apikey/expire",
		map[string]any{"prefix": prefix}, nil)
}

// DeleteAPIKey removes an API key by prefix.
func (h *HTTP) DeleteAPIKey(ctx context.Context, prefix string) error {
	return h.do(ctx, http.MethodDelete, "/api/v1/apikey/"+url.PathEscape(prefix), nil, nil)
}

// SetPolicy writes the ACL document. Requires policy.mode = database on the
// server; in file mode Headscale rejects the write.
func (h *HTTP) SetPolicy(ctx context.Context, hujson string) (Policy, error) {
	var resp struct {
		Policy    string `json:"policy"`
		UpdatedAt string `json:"updatedAt"`
	}

	body := map[string]any{"policy": hujson}

	if err := h.do(ctx, http.MethodPut, "/api/v1/policy", body, &resp); err != nil {
		return Policy{}, err
	}

	return Policy{HuJSON: resp.Policy, UpdatedAt: resp.UpdatedAt}, nil
}

// CheckPolicy validates a document without storing it.
func (h *HTTP) CheckPolicy(ctx context.Context, hujson string) error {
	return h.do(ctx, http.MethodPost, "/api/v1/policy/check",
		map[string]any{"policy": hujson}, nil)
}

func mapPreAuthKey(k v1PreAuthKey) PreAuthKey {
	out := PreAuthKey{
		ID:        k.ID,
		Key:       k.Key,
		Reusable:  k.Reusable,
		Ephemeral: k.Ephemeral,
		Used:      k.Used,
		Tags:      k.ACLTags,
		CreatedAt: k.CreatedAt,
		Expiry:    k.Expiration,
	}

	if k.User != nil {
		out.User = k.User.Name

		if id, err := strconv.ParseUint(k.User.ID, 10, 64); err == nil {
			out.UserID = uint(id)
		}
	}

	return out
}

func (h *HTTP) nodeCall(ctx context.Context, method, path string, body any) (types.Node, error) {
	var resp struct {
		Node v1Node `json:"node"`
	}

	if err := h.do(ctx, method, path, body, &resp); err != nil {
		return types.Node{}, err
	}

	return mapNode(resp.Node)
}

// do performs a request with an optional JSON body and an optional decoded
// response.
func (h *HTTP) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("headscale %s %s: encoding body: %w", method, path, err)
		}

		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("headscale %s %s: %w", method, path, err)
	}

	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("headscale %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

		return &APIError{
			Status: resp.StatusCode,
			Method: method,
			Path:   path,
			Body:   strings.TrimSpace(string(raw)),
		}
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)

		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("headscale %s %s: decoding response: %w", method, path, err)
	}

	return nil
}

// ParseNodeID converts a path parameter into a node id.
func ParseNodeID(s string) (types.NodeID, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid node id %q", s)
	}

	return types.NodeID(id), nil
}
