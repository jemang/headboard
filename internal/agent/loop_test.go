package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jemang/headboard/internal/sshsync"
)

type nopHost struct{}

func (nopHost) Mode() string { return "local" }
func (nopHost) Discover(context.Context) ([]sshsync.Discovered, error) {
	return []sshsync.Discovered{{Ref: "self"}}, nil
}
func (nopHost) Exec(context.Context, string, string, ...string) (string, error) { return "", nil }

func TestOnceKeepsOldETagWhenReportFails(t *testing.T) {
	var reports atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			reports.Add(1)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("ETag", `"2.1"`)
		json.NewEncoder(w).Encode(sshsync.SyncResponse{Protocol: sshsync.Protocol})
	}))
	defer srv.Close()

	applied := 0
	l := &Loop{
		Client: NewClient(srv.URL, "hbk_ok", srv.Client()),
		Host:   nopHost{},
		Apply: func(context.Context, Host, []sshsync.Discovered, []sshsync.Target) Result {
			applied++
			return Result{}
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tag, err := l.Once(t.Context(), `"1.1"`, false)
	if err == nil || tag != `"1.1"` || applied != 1 || reports.Load() != 1 {
		t.Fatalf("tag %q err %v applied %d reports %d", tag, err, applied, reports.Load())
	}
}

func TestOnceSkipsApplyOn304(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer srv.Close()

	applied := 0
	l := &Loop{
		Client: NewClient(srv.URL, "hbk_ok", srv.Client()),
		Host:   nopHost{},
		Apply: func(context.Context, Host, []sshsync.Discovered, []sshsync.Target) Result {
			applied++
			return Result{}
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tag, err := l.Once(t.Context(), `"1.1"`, false)
	if err != nil || tag != `"1.1"` || applied != 0 {
		t.Fatalf("tag %q err %v applied %d", tag, err, applied)
	}
}

// A skipped target (stopped LXC, missing user, failed exec) gets no retry from
// the server, so the agent must ask again rather than sit on a 304.
func TestOnceKeepsOldETagWhenApplySkipsWork(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("ETag", `"3.1"`)
			json.NewEncoder(w).Encode(sshsync.SyncResponse{Protocol: sshsync.Protocol})
		}
	}))
	defer srv.Close()

	l := &Loop{
		Client: NewClient(srv.URL, "hbk_ok", srv.Client()),
		Host:   nopHost{},
		Apply: func(context.Context, Host, []sshsync.Discovered, []sshsync.Target) Result {
			return Result{Warnings: []sshsync.Warning{{Ref: "105", Message: "target is not running"}}}
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tag, err := l.Once(t.Context(), `"1.1"`, false)
	if err != nil || tag != `"1.1"` {
		t.Fatalf("tag %q err %v, want the old ETag kept for a retry", tag, err)
	}
}

type slowHost struct{ nopHost }

func (slowHost) Discover(context.Context) ([]sshsync.Discovered, error) {
	time.Sleep(80 * time.Millisecond)
	return []sshsync.Discovered{{Ref: "self"}}, nil
}

// The interval is measured start to start: a slow cycle must not add its own
// duration to the wait, or a 30 s interval becomes 60 s on a busy host.
func TestRunPollsStartToStart(t *testing.T) {
	var syncs atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			syncs.Add(1)
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer srv.Close()

	l := &Loop{
		Client:    NewClient(srv.URL, "hbk_ok", srv.Client()),
		Host:      slowHost{},
		Apply:     func(context.Context, Host, []sshsync.Discovered, []sshsync.Target) Result { return Result{} },
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Interval:  100 * time.Millisecond,
		FullEvery: time.Hour,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 620*time.Millisecond)
	defer cancel()
	l.Run(ctx)

	if n := syncs.Load(); n < 5 {
		t.Fatalf("%d polls in 620 ms with a 100 ms interval and 80 ms cycles; the wait is not subtracting the cycle time", n)
	}
}
