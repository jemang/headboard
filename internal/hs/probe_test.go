package hs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

type fakeClient struct {
	version Version
	err     error
}

func (f fakeClient) Version(context.Context) (Version, error) { return f.version, f.err }
func (fakeClient) ListNodes(context.Context) ([]types.Node, error) {
	return nil, errors.New("not used")
}
func (fakeClient) ListUsers(context.Context) ([]types.User, error) {
	return nil, errors.New("not used")
}
func (fakeClient) Policy(context.Context) (Policy, error) { return Policy{}, errors.New("not used") }

func TestCheckVersion(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name            string
		server          string
		compiledAgainst string
		want            bool
	}{
		{"identical", "v0.29.3", "v0.29.3", true},
		// Patch releases do not change policy semantics, so a patch drift
		// is not worth alarming an operator about.
		{"patch drift is fine", "v0.29.7", "v0.29.3", true},
		{"missing v prefix still compares", "0.29.3", "v0.29.3", true},
		// v0.30 rewrote the API and dropped gRPC; the policy engine moved
		// with it.
		{"minor drift is not", "v0.30.0", "v0.29.3", false},
		{"major drift is not", "v1.0.0", "v0.29.3", false},
		{"pre-release compares on its base version", "v0.29.4-beta.1", "v0.29.3", true},
		{"pre-release of a different minor still mismatches", "v0.30.0-beta.1", "v0.29.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := CheckVersion(t.Context(), fakeClient{version: Version{Version: tt.server}}, tt.compiledAgainst, log)
			if err != nil {
				t.Fatalf("CheckVersion: %v", err)
			}

			if p.Match != tt.want {
				t.Errorf("Match: got %v, want %v (server %s, compiled %s)",
					p.Match, tt.want, tt.server, tt.compiledAgainst)
			}

			if p.Server != tt.server {
				t.Errorf("Server: got %q, want %q", p.Server, tt.server)
			}
		})
	}
}

func TestCheckVersionPropagatesProbeFailure(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	want := errors.New("connection refused")

	if _, err := CheckVersion(t.Context(), fakeClient{err: want}, "v0.29.3", log); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
