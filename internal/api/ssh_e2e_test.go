package api

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jemang/headboard/internal/agent"
	"github.com/jemang/headboard/internal/sshsync"
)

type oneTarget struct{}

func (oneTarget) Mode() string { return "proxmox" }
func (oneTarget) Discover(context.Context) ([]sshsync.Discovered, error) {
	return []sshsync.Discovered{{Ref: "105", Hostname: "app-1", IPs: []string{"192.0.2.10"}}}, nil
}
func (oneTarget) Exec(context.Context, string, string, ...string) (string, error) { return "", nil }

func TestAgentRoundTripRecordsWriteThenClear(t *testing.T) {
	h, raw, st := agentFixture(t)

	router := chi.NewRouter()
	h.register(humachi.New(router, Config("test")))
	srv := httptest.NewTLSServer(router)
	defer srv.Close()

	var sent []sshsync.Target
	l := &agent.Loop{
		Client: agent.NewClient(srv.URL, raw, srv.Client()),
		Host:   oneTarget{},
		Apply: func(_ context.Context, _ agent.Host, _ []sshsync.Discovered, ts []sshsync.Target) agent.Result {
			sent = ts
			r := agent.Result{}
			for _, tg := range ts {
				for u := range tg.Users {
					r.Applied = append(r.Applied, sshsync.Pair{Ref: tg.Ref, UnixUser: u})
				}
				for _, u := range tg.Clear {
					r.Cleared = append(r.Cleared, sshsync.Pair{Ref: tg.Ref, UnixUser: u})
				}
			}
			return r
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tag, err := l.Once(t.Context(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || len(sent[0].Users["sysadmin"]) != 1 {
		t.Fatalf("first sync sent %+v", sent)
	}

	if err := st.SetSSHRow(t.Context(), "app", "sysadmin", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := l.Once(t.Context(), tag, false); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || len(sent[0].Clear) != 1 || sent[0].Clear[0] != "sysadmin" {
		t.Fatalf("second sync sent %+v", sent)
	}

	if m, _ := st.ListSSHManaged(t.Context()); len(m) != 0 {
		t.Fatalf("clear not acknowledged: %+v", m)
	}
}
