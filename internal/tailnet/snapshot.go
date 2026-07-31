// Package tailnet keeps a current picture of the Headscale tailnet.
//
// Headscale has no event stream, so this polls. Everything the UI reads comes
// from one snapshot taken at a point in time, which means a device list and the
// rules shown next to it always describe the same instant rather than two
// requests a second apart.
package tailnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/policy"
)

// Snapshot is one consistent read of the tailnet.
type Snapshot struct {
	Nodes []types.Node
	Users []types.User

	// Policy is the ACL document as stored, comments intact.
	Policy hs.Policy

	// PolicySHA256 identifies the exact text this snapshot was built from.
	// The ACL editor compares against it to refuse a write when another
	// admin changed the document underneath.
	PolicySHA256 string

	// Manager answers effective-rules questions for this snapshot.
	Manager *policy.Manager

	// Revision increments whenever anything changed. Browsers use it to
	// tell a real change from a keepalive.
	Revision uint64

	FetchedAt time.Time
}

// Watcher polls Headscale and fans changes out to subscribers.
type Watcher struct {
	client   hs.Client
	interval time.Duration
	log      *slog.Logger

	mu   sync.RWMutex
	snap *Snapshot
	err  error

	// refresh carries out-of-band refresh requests. Buffered to one: a
	// burst of mutations only needs one refresh after the last of them.
	refresh chan struct{}

	subsMu sync.Mutex
	subs   map[int]chan Snapshot
	nextID int
}

// New builds a watcher. Nothing is fetched until Run.
func New(client hs.Client, interval time.Duration, log *slog.Logger) *Watcher {
	return &Watcher{
		client:   client,
		interval: interval,
		log:      log,
		refresh:  make(chan struct{}, 1),
		subs:     make(map[int]chan Snapshot),
	}
}

// Run polls until the context is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	// Fetch immediately so the first request after startup does not have to
	// wait out a tick.
	w.poll(ctx)

	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.poll(ctx)
		case <-w.refresh:
			w.poll(ctx)
			// Reset the ticker so an out-of-band refresh does not
			// leave a full interval's worth of staleness behind it.
			t.Reset(w.interval)
		}
	}
}

// Invalidate asks for a refresh now rather than at the next tick. Mutating
// handlers call it so the change is visible immediately instead of up to one
// poll interval later.
func (w *Watcher) Invalidate() {
	select {
	case w.refresh <- struct{}{}:
	default:
		// A refresh is already queued; one is enough.
	}
}

// Current returns the latest snapshot, or the reason there is none.
func (w *Watcher) Current() (*Snapshot, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.snap == nil {
		return nil, w.err
	}

	return w.snap, nil
}

func (w *Watcher) poll(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, w.interval*2+5*time.Second)
	defer cancel()

	snap, err := w.fetch(ctx)
	if err != nil {
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()

		// Keep serving the previous snapshot. A Headscale blip should
		// show as stale data with a warning, not as an empty console.
		w.log.Warn("tailnet poll failed; serving the previous snapshot", "err", err)

		return
	}

	w.mu.Lock()

	var changed bool

	if prev := w.snap; prev == nil {
		snap.Revision = 1
		w.snap = snap
		changed = true
	} else {
		changed = w.applyLocked(prev, snap)
	}

	w.err = nil
	w.mu.Unlock()

	if changed {
		w.broadcast(*snap)
	}
}

// applyLocked installs a new snapshot over an existing one and reports whether
// anything a browser would care about changed. Caller holds the write lock and
// has already handled the first-snapshot case.
func (w *Watcher) applyLocked(prev, next *Snapshot) bool {
	changed := prev.PolicySHA256 != next.PolicySHA256 ||
		!sameNodes(prev.Nodes, next.Nodes) ||
		!sameUsers(prev.Users, next.Users)

	next.Revision = prev.Revision
	if changed {
		next.Revision++
	}

	// Reuse the compiled policy engine when nothing it reads changed.
	// Rebuilding recompiles every rule and throws away the attribution
	// cache, which is wasted work on a tick where only last-seen moved.
	if !changed {
		next.Manager = prev.Manager
	}

	w.snap = next

	return changed
}

func (w *Watcher) fetch(ctx context.Context) (*Snapshot, error) {
	nodes, err := w.client.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	users, err := w.client.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	pol, err := w.client.Policy(ctx)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Nodes:        nodes,
		Users:        users,
		Policy:       pol,
		PolicySHA256: SHA256(pol.HuJSON),
		FetchedAt:    time.Now(),
	}

	// An empty policy is legitimate on a fresh Headscale: there is simply
	// nothing to compile yet, and every node reaches every other node.
	if pol.HuJSON != "" {
		mgr, err := policy.New(pol.HuJSON, users, nodes)
		if err != nil {
			// A policy Headboard cannot compile is worth surfacing,
			// but it must not blank the device list — the ACL editor
			// is exactly where someone would go to fix it.
			w.log.Error("policy did not compile; effective rules are unavailable", "err", err)
		} else {
			snap.Manager = mgr
		}
	}

	return snap, nil
}

// SHA256 hashes policy text. Exported because the ACL write path compares
// against it to detect a concurrent edit.
func SHA256(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}
