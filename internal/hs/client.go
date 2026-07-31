package hs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
)

// maxErrorBody bounds how much of a failed response is kept for the error
// message. Headscale's error bodies are small; a hostile or misconfigured proxy
// in front of it might not be.
const maxErrorBody = 4 << 10

// HTTP is the v0.29.x REST client.
type HTTP struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

var (
	_ Client  = (*HTTP)(nil)
	_ Mutator = (*HTTP)(nil)
)

// New returns a client for the Headscale at baseURL, authenticating with an
// admin API key. The key never leaves this process.
func New(baseURL, apiKey string, timeout time.Duration) *HTTP {
	return &HTTP{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
	}
}

// Version reads Headscale's unauthenticated /version. It is deliberately not
// under /api/v1: the probe has to work even when the API key is wrong, so the
// startup error can distinguish "unreachable" from "unauthorised".
func (h *HTTP) Version(ctx context.Context) (Version, error) {
	var v Version
	if err := h.get(ctx, "/version", &v); err != nil {
		return Version{}, err
	}

	return v, nil
}

func (h *HTTP) ListNodes(ctx context.Context) ([]types.Node, error) {
	var resp struct {
		Nodes []v1Node `json:"nodes"`
	}

	if err := h.get(ctx, "/api/v1/node", &resp); err != nil {
		return nil, err
	}

	nodes := make([]types.Node, 0, len(resp.Nodes))

	for _, n := range resp.Nodes {
		node, err := mapNode(n)
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (h *HTTP) ListUsers(ctx context.Context) ([]types.User, error) {
	var resp struct {
		Users []v1User `json:"users"`
	}

	if err := h.get(ctx, "/api/v1/user", &resp); err != nil {
		return nil, err
	}

	users := make([]types.User, 0, len(resp.Users))

	for i := range resp.Users {
		user, err := mapUser(&resp.Users[i])
		if err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	return users, nil
}

func (h *HTTP) Policy(ctx context.Context) (Policy, error) {
	var resp struct {
		Policy    string `json:"policy"`
		UpdatedAt string `json:"updatedAt"`
	}

	if err := h.get(ctx, "/api/v1/policy", &resp); err != nil {
		return Policy{}, err
	}

	return Policy{HuJSON: resp.Policy, UpdatedAt: resp.UpdatedAt}, nil
}

func (h *HTTP) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("headscale GET %s: %w", path, err)
	}

	req.Header.Set("Accept", "application/json")

	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("headscale GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

		return &APIError{
			Status: resp.StatusCode,
			Method: http.MethodGet,
			Path:   path,
			Body:   strings.TrimSpace(string(body)),
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("headscale GET %s: decoding response: %w", path, err)
	}

	return nil
}
