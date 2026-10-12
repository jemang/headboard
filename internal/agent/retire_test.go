package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jemang/headboard/internal/sshsync"
)

type runningHost struct {
	*fakeHost
	up []sshsync.Discovered
}

func (r runningHost) Discover(context.Context) ([]sshsync.Discovered, error) { return r.up, nil }

// retireServer plays Headboard: it hands out what the agent wrote and retires
// it once every one of those comes back cleared.
func retireServer(t *testing.T, wrote []sshsync.Pair, got *[]sshsync.Pair) *Client {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ssh/agent/managed":
			json.NewEncoder(w).Encode(map[string]any{"managed": wrote})
		case "/api/ssh/agent/retire":
			var in struct{ Cleared []sshsync.Pair }
			json.NewDecoder(r.Body).Decode(&in)
			*got = in.Cleared
			left := []sshsync.Pair{}
			for _, p := range wrote {
				found := false
				for _, c := range in.Cleared {
					found = found || c == p
				}
				if !found {
					left = append(left, p)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"retired": len(left) == 0, "remaining": left})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return NewClient(srv.URL, "hbk_x", srv.Client())
}

func TestRetireClearsEveryBlockThenLeaves(t *testing.T) {
	h := runningHost{
		fakeHost: &fakeHost{files: map[string]string{
			"deploy": "mine\n" + sshsync.BeginMarker + "\nk\n" + sshsync.EndMarker + "\n",
		}},
		up: running,
	}
	var cleared []sshsync.Pair
	c := retireServer(t, []sshsync.Pair{{Ref: "105", UnixUser: "deploy"}}, &cleared)

	out, err := Retire(t.Context(), c, h)
	if err != nil || !out.Retired {
		t.Fatalf("out = %+v, %v", out, err)
	}
	if h.files["deploy"] != "mine\n"+sshsync.BeginMarker+"\n"+sshsync.EndMarker+"\n" {
		t.Fatalf("block not cleared: %q", h.files["deploy"])
	}
	if len(cleared) != 1 {
		t.Fatalf("server told %+v", cleared)
	}
}

// A container that is stopped cannot be cleared; the agent must not leave
// while its keys are still in there.
func TestRetireStaysWhenAContainerCannotBeCleared(t *testing.T) {
	h := runningHost{fakeHost: &fakeHost{files: map[string]string{}}, up: running}
	var cleared []sshsync.Pair
	c := retireServer(t, []sshsync.Pair{{Ref: "105", UnixUser: "deploy"}, {Ref: "300", UnixUser: "ops"}}, &cleared)

	out, err := Retire(t.Context(), c, h)
	if err != nil || out.Retired || len(out.Remaining) != 1 || out.Remaining[0].Ref != "300" || len(out.Warnings) == 0 {
		t.Fatalf("out = %+v, %v", out, err)
	}
}
