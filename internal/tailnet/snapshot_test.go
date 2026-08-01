package tailnet

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/hs"
)

func ptr[T any](v T) *T { return &v }

const testPolicy = `{
  "tagOwners": {"tag:prod": ["ops@"]},
  "groups": {"group:eng": ["alice@"]},
  "acls": [{"action": "accept", "src": ["group:eng"], "dst": ["tag:prod:443"]}],
}`

// fakeClient serves whatever the test sets, and counts calls.
type fakeClient struct {
	mu     sync.Mutex
	nodes  []types.Node
	users  []types.User
	policy string
	err    error
	calls  int
}

func (f *fakeClient) set(fn func(*fakeClient)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fn(f)
}

func (f *fakeClient) Version(context.Context) (hs.Version, error) {
	return hs.Version{Version: "v0.29.3"}, nil
}

func (f *fakeClient) ListNodes(context.Context) ([]types.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	if f.err != nil {
		return nil, f.err
	}

	return f.nodes, nil
}

func (f *fakeClient) ListUsers(context.Context) ([]types.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}

	return f.users, nil
}

func (f *fakeClient) Policy(context.Context) (hs.Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return hs.Policy{}, f.err
	}

	return hs.Policy{HuJSON: f.policy}, nil
}

func newFake() *fakeClient {
	users := []types.User{{Name: "alice"}, {Name: "ops"}}
	users[0].ID, users[1].ID = 1, 2

	laptop := types.Node{
		ID: 1, Hostname: "alice-laptop", GivenName: "alice-laptop",
		IPv4:   ptr(netip.MustParseAddr("100.64.0.1")),
		UserID: &users[0].ID, User: &users[0], IsOnline: ptr(false),
	}

	web := types.Node{
		ID: 2, Hostname: "prod-web", GivenName: "prod-web",
		IPv4: ptr(netip.MustParseAddr("100.64.0.2")),
		Tags: types.Strings{"tag:prod"}, IsOnline: ptr(false),
	}

	return &fakeClient{nodes: []types.Node{laptop, web}, users: users, policy: testPolicy}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// waitFor polls a condition rather than sleeping a fixed amount, so the test
// does not depend on how fast the machine is.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestWatcherFetchesImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	// A long interval: if the first fetch waited for a tick, this fails.
	w := New(f, time.Hour, quiet())

	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	snap, err := w.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}

	if len(snap.Nodes) != 2 || len(snap.Users) != 2 {
		t.Errorf("snapshot has %d nodes and %d users, want 2 and 2", len(snap.Nodes), len(snap.Users))
	}

	if snap.Manager == nil {
		t.Fatal("no policy manager was compiled")
	}

	// The snapshot must be usable, not just present.
	if _, err := snap.Manager.Inbound(2); err != nil {
		t.Errorf("Inbound on the snapshot's manager: %v", err)
	}

	if snap.PolicySHA256 == "" {
		t.Error("PolicySHA256 is empty; the ACL write guard depends on it")
	}
}

// A Headscale blip must show as stale data, not as an empty console — the
// console is where an operator would go to find out what is wrong.
func TestPollFailureKeepsPreviousSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, 20*time.Millisecond, quiet())

	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	before, _ := w.Current()

	f.set(func(c *fakeClient) { c.err = errors.New("headscale is down") })

	callsAt := func() int {
		f.mu.Lock()
		defer f.mu.Unlock()

		return f.calls
	}

	start := callsAt()
	waitFor(t, "two failed polls", func() bool { return callsAt() >= start+2 })

	after, err := w.Current()
	if err != nil {
		t.Fatalf("Current returned an error while a snapshot exists: %v", err)
	}

	if after == nil || after.Revision != before.Revision {
		t.Errorf("the failed poll replaced the snapshot: %+v", after)
	}
}

func TestConnectionStatusReportsStaleAfterPollFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, 20*time.Millisecond, quiet())
	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		snap, err := w.Current()
		return snap != nil && err == nil
	})

	f.set(func(c *fakeClient) { c.err = errors.New("headscale is down") })
	waitFor(t, "a failed poll", func() bool {
		w.mu.RLock()
		defer w.mu.RUnlock()
		return w.err != nil
	})

	status := w.ConnectionStatus()
	if status.State != "stale" || status.LastSynced == nil {
		t.Fatalf("ConnectionStatus = %+v, want stale with a last sync", status)
	}
}

// The whole console re-renders on a revision bump, so a tick where nothing
// meaningful moved must not produce one.
func TestRevisionOnlyMovesOnRealChange(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, 15*time.Millisecond, quiet())

	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	first, _ := w.Current()

	// Several polls with identical data.
	time.Sleep(100 * time.Millisecond)

	same, _ := w.Current()
	if same.Revision != first.Revision {
		t.Errorf("revision moved from %d to %d with no change", first.Revision, same.Revision)
	}

	// The compiled engine should be reused rather than rebuilt.
	if same.Manager != first.Manager {
		t.Error("the policy engine was rebuilt even though nothing it reads changed")
	}

	// A node going online is a real change for the device list.
	f.set(func(c *fakeClient) {
		nodes := make([]types.Node, len(c.nodes))
		copy(nodes, c.nodes)
		nodes[0].IsOnline = ptr(true)
		c.nodes = nodes
	})

	waitFor(t, "the revision to move", func() bool {
		s, _ := w.Current()

		return s.Revision > first.Revision
	})
}

func TestSubscribersReceiveChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, 15*time.Millisecond, quiet())

	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	events, unsubscribe := w.Subscribe()
	defer unsubscribe()

	f.set(func(c *fakeClient) {
		nodes := make([]types.Node, len(c.nodes))
		copy(nodes, c.nodes)
		nodes[0].GivenName = "renamed-laptop"
		c.nodes = nodes
	})

	select {
	case snap := <-events:
		if snap.Nodes[0].GivenName != "renamed-laptop" {
			t.Errorf("subscriber got %q, want renamed-laptop", snap.Nodes[0].GivenName)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber received nothing after a change")
	}

	unsubscribe()

	if n := w.Subscribers(); n != 0 {
		t.Errorf("subscribers = %d after unsubscribing, want 0", n)
	}

	// Unsubscribing twice must not panic on a closed channel.
	unsubscribe()
}

// A browser that stops reading must not stall the poller for everyone else.
func TestSlowSubscriberDoesNotBlockThePoller(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, 10*time.Millisecond, quiet())

	go w.Run(ctx)

	_, unsubscribe := w.Subscribe() // never read from
	defer unsubscribe()

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	for i := range 5 {
		name := "rename-" + string(rune('a'+i))

		f.set(func(c *fakeClient) {
			nodes := make([]types.Node, len(c.nodes))
			copy(nodes, c.nodes)
			nodes[0].GivenName = name
			c.nodes = nodes
		})

		waitFor(t, "the poller to keep running", func() bool {
			s, _ := w.Current()

			return s.Nodes[0].GivenName == name
		})
	}
}

// A mutation must be visible immediately, not up to a poll interval later.
func TestInvalidateRefreshesNow(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	w := New(f, time.Hour, quiet())

	go w.Run(ctx)

	waitFor(t, "the first snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	f.set(func(c *fakeClient) {
		nodes := make([]types.Node, len(c.nodes))
		copy(nodes, c.nodes)
		nodes[0].GivenName = "just-renamed"
		c.nodes = nodes
	})

	w.Invalidate()

	waitFor(t, "the out-of-band refresh", func() bool {
		s, _ := w.Current()

		return s.Nodes[0].GivenName == "just-renamed"
	})

	// Repeated invalidations must coalesce rather than queue up.
	for range 10 {
		w.Invalidate()
	}
}

// An unparseable policy is exactly when someone needs the console most.
func TestBrokenPolicyStillYieldsASnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	f := newFake()
	f.policy = `{ "acls": [ {"action": "accept", "src": ["group:nope"], "dst": ["*:*"]} ] }`

	w := New(f, time.Hour, quiet())

	go w.Run(ctx)

	waitFor(t, "a snapshot", func() bool {
		s, _ := w.Current()

		return s != nil
	})

	snap, _ := w.Current()

	if len(snap.Nodes) != 2 {
		t.Errorf("the device list was lost when the policy failed to compile: %d nodes", len(snap.Nodes))
	}

	if snap.Manager != nil {
		t.Error("a policy that references an undefined group produced a manager")
	}
}
