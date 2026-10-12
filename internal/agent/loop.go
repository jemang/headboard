package agent

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jemang/headboard/internal/sshsync"
)

// Host is where targets live: a Proxmox host's containers, or this machine.
type Host interface {
	Mode() string
	Discover(ctx context.Context) ([]sshsync.Discovered, error)
	// Exec runs argv inside target ref with stdin, returning stdout.
	Exec(ctx context.Context, ref, stdin string, argv ...string) (string, error)
}

type Result struct {
	Applied    []sshsync.Pair
	Cleared    []sshsync.Pair
	Mismatched []string
	Warnings   []sshsync.Warning
}

type Applier func(ctx context.Context, h Host, discovered []sshsync.Discovered, targets []sshsync.Target) Result

type Loop struct {
	Client    *Client
	Host      Host
	Apply     Applier
	Version   string
	Log       *slog.Logger
	Interval  time.Duration
	FullEvery time.Duration
}

// Once runs one cycle and returns the ETag to send next time. When the report
// fails after files were written, the old ETag is returned so the next cycle
// re-syncs and re-reports instead of getting a 304 and never recording it.
func (l *Loop) Once(ctx context.Context, etag string, force bool) (string, error) {
	discovered, err := l.Host.Discover(ctx)
	if err != nil {
		return etag, err
	}

	ask := etag
	if force {
		ask = ""
	}

	resp, next, changed, err := l.Client.Sync(ctx, ask)
	if err != nil {
		return etag, err
	}

	rep := sshsync.Report{Mode: l.Host.Mode(), Version: l.Version, Protocol: sshsync.Protocol, Discovered: discovered}

	if changed {
		r := l.Apply(ctx, l.Host, discovered, resp.Targets)
		rep.Applied, rep.Cleared, rep.Mismatched, rep.Warnings = r.Applied, r.Cleared, r.Mismatched, r.Warnings

		for _, w := range r.Warnings {
			l.Log.Warn("target skipped", "ref", w.Ref, "unix_user", w.UnixUser, "reason", w.Message)
		}
	}

	if err := l.Client.Report(ctx, rep); err != nil {
		return etag, err
	}

	// Skipped work has no server-side change to wake it, so keep asking until
	// the target is reachable again rather than waiting for the hourly sync.
	if len(rep.Warnings) > 0 {
		return etag, nil
	}

	return next, nil
}

// Run polls until ctx ends or the token is rejected.
func (l *Loop) Run(ctx context.Context) error {
	etag := ""
	var lastFull time.Time
	wait := l.Interval

	for {
		force := time.Since(lastFull) >= l.FullEvery
		started := time.Now()

		tag, err := l.Once(ctx, etag, force)

		switch {
		case errors.Is(err, ErrUnauthorized):
			l.Log.Error("agent token rejected; stopping. Keys already written stay in place.")

			return nil
		case err != nil:
			wait = min(wait*2, 5*time.Minute)
			l.Log.Warn("sync failed; files left as they are", "err", err, "retry_in", wait)
		default:
			etag = tag
			wait = l.Interval
			if force {
				lastFull = time.Now()
			}
		}

		// Healthy polls are spaced start to start: on a Proxmox host a cycle of
		// pct calls can itself take half the interval, and waiting a full
		// interval after it would double the time a revocation takes to land.
		pause := wait
		if err == nil {
			pause = max(0, l.Interval-time.Since(started))
		}

		jitter := time.Duration(rand.Int64N(int64(l.Interval/5) + 1))

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pause + jitter):
		}
	}
}
