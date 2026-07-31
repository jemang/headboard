package hs_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jemang/headboard/internal/hs"
)

// The fixtures prove the mapping; this proves the fixtures still describe the
// server. It runs only when pointed at a Headscale, so `go test ./...` stays
// hermetic:
//
//	env $(grep -v '^#' dev/.env.dev | xargs) go test ./internal/hs/ -run Live
//
// Read-only by construction — every call here is a GET.
func liveClient(t *testing.T) *hs.HTTP {
	t.Helper()

	url, key := os.Getenv("HEADSCALE_URL"), os.Getenv("HEADSCALE_API_KEY")
	if url == "" || key == "" {
		t.Skip("set HEADSCALE_URL and HEADSCALE_API_KEY to run live tests")
	}

	return hs.New(url, key, 10*time.Second)
}

func TestLiveRoundTrip(t *testing.T) {
	c := liveClient(t)
	ctx := t.Context()

	nodes, err := c.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	if len(nodes) == 0 {
		t.Fatal("ListNodes returned nothing — is the instance seeded?")
	}

	for _, n := range nodes {
		if n.ID == 0 {
			t.Errorf("%s: ID did not survive mapping", n.GivenName)
		}

		if n.IPv4 == nil && n.IPv6 == nil {
			t.Errorf("%s: no addresses after mapping", n.GivenName)
		}

		// Tagged and user-owned are the only two shapes; a node that is
		// neither means the sentinel collapse dropped a real owner.
		if n.UserID == nil && len(n.Tags) == 0 {
			t.Errorf("%s: neither owned nor tagged after mapping", n.GivenName)
		}

		if n.Hostinfo == nil {
			t.Errorf("%s: Hostinfo was not synthesised", n.GivenName)
		}
	}

	users, err := c.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	if len(users) == 0 {
		t.Fatal("ListUsers returned nothing")
	}

	for _, u := range users {
		if u.ID == 0 || u.Name == "" {
			t.Errorf("user %d/%q: incomplete after mapping", u.ID, u.Name)
		}
	}

	pol, err := c.Policy(ctx)
	if err != nil {
		t.Fatalf("Policy: %v", err)
	}

	if pol.HuJSON == "" {
		t.Error("Policy: empty — the dev instance should have a seeded policy")
	}
}

func TestLiveVersionProbe(t *testing.T) {
	c := liveClient(t)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	p, err := hs.CheckVersion(t.Context(), c, "v0.29.3", log)
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}

	if p.Server == "" {
		t.Fatal("Server version: empty")
	}

	if !p.Match {
		t.Errorf("version mismatch: server %s, compiled against %s", p.Server, p.CompiledAgainst)
	}
}

func TestLiveUnauthorizedIsSurfaced(t *testing.T) {
	url := os.Getenv("HEADSCALE_URL")
	if url == "" {
		t.Skip("set HEADSCALE_URL to run live tests")
	}

	c := hs.New(url, "hskey-api-definitely-not-valid", 10*time.Second)

	if _, err := c.ListNodes(t.Context()); err == nil {
		t.Fatal("ListNodes with a bogus key: got nil error, want a failure")
	}

	// The version probe is unauthenticated on purpose, so startup can tell
	// "cannot reach Headscale" apart from "the API key is wrong".
	if _, err := c.Version(t.Context()); err != nil {
		t.Errorf("Version with a bogus key: got %v, want success", err)
	}
}
