// Package hs talks to Headscale's REST API and converts its responses into the
// Headscale Go types that the policy engine consumes.
//
// The split matters: Headboard imports headscale/hscontrol/policy/v2 as a
// library to answer "which rules apply to this device", and that engine reads
// types.Node/types.User, not JSON. Everything Headboard reports about
// reachability is therefore only as correct as the mapping in model.go.
//
// The interface exists so the v0.30 client (native REST plus a Tailscale-shaped
// /api/v2) can drop in without touching handlers or UI.
package hs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
)

// ErrNotFound is returned when Headscale answers 404.
var ErrNotFound = errors.New("not found")

// Client is the read surface of Headscale's API.
//
// Reads and writes are separate interfaces on purpose: the poller and the
// policy bridge only ever read, and taking the narrow interface makes that
// checkable rather than a matter of trust.
type Client interface {
	// Version reports the Headscale server's own version. Headboard is
	// compiled against one version's policy engine, so a mismatch is a
	// correctness problem rather than a cosmetic one.
	Version(ctx context.Context) (Version, error)

	// ListNodes returns every node, already mapped for the policy engine.
	ListNodes(ctx context.Context) ([]types.Node, error)

	// ListUsers returns every user, already mapped for the policy engine.
	ListUsers(ctx context.Context) ([]types.User, error)

	// Policy returns the ACL policy as stored: raw HuJSON, comments intact.
	// Headboard patches this text rather than regenerating it.
	Policy(ctx context.Context) (Policy, error)
}

// Mutator is the write surface. Only handlers that have checked a capability
// take it.
type Mutator interface {
	RenameNode(ctx context.Context, id types.NodeID, name string) (types.Node, error)
	SetTags(ctx context.Context, id types.NodeID, tags []string) (types.Node, error)
	ApproveRoutes(ctx context.Context, id types.NodeID, routes []string) (types.Node, error)
	ExpireNode(ctx context.Context, id types.NodeID) (types.Node, error)
	DeleteNode(ctx context.Context, id types.NodeID) error

	ApproveRegistration(ctx context.Context, authID string) error
	RejectRegistration(ctx context.Context, authID string) error
	RegisterNode(ctx context.Context, authID, user string) (types.Node, error)

	CreateUser(ctx context.Context, name, displayName, email string) (types.User, error)
	RenameUser(ctx context.Context, id uint, newName string) (types.User, error)
	DeleteUser(ctx context.Context, id uint) error

	ListPreAuthKeys(ctx context.Context, userID uint) ([]PreAuthKey, error)
	CreatePreAuthKey(ctx context.Context, userID uint, reusable, ephemeral bool, expiry time.Time, tags []string) (PreAuthKey, error)
	ExpirePreAuthKey(ctx context.Context, id string) error

	ListAPIKeys(ctx context.Context) ([]APIKey, error)
	CreateAPIKey(ctx context.Context, expiry time.Time) (string, error)
	ExpireAPIKey(ctx context.Context, prefix string) error
	DeleteAPIKey(ctx context.Context, prefix string) error

	SetPolicy(ctx context.Context, hujson string) (Policy, error)
	CheckPolicy(ctx context.Context, hujson string) error
}

// Version is Headscale's build information, from its unauthenticated /version.
type Version struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// Policy is the ACL document exactly as Headscale stores it.
type Policy struct {
	// HuJSON is the raw policy text, including comments and formatting.
	HuJSON string

	// UpdatedAt is when Headscale last accepted a write, when it reports
	// one. Used as a staleness hint; the write guard compares content
	// hashes rather than trusting this.
	UpdatedAt string
}

// APIError is a non-2xx response from Headscale.
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("headscale %s %s: %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func (e *APIError) Is(target error) bool {
	return target == ErrNotFound && e.Status == 404
}
