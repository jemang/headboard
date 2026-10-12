package api

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
)

//go:embed ssh_agent_install.sh
var installScript []byte

// snapshotSource is the part of *tailnet.Watcher the agent API needs, so
// tests can supply a snapshot without polling a Headscale.
type snapshotSource interface {
	Current() (*tailnet.Snapshot, error)
	ConnectionStatus() tailnet.ConnectionStatus
}

type sshAgentAPI struct {
	store     *store.Store
	snapshots snapshotSource

	// boot is random per process. The snapshot revision restarts at 1 with
	// the process, so without it an agent could be told 304 for an ETag
	// minted before a restart while the state underneath changed.
	boot string
}

type agentSyncInput struct {
	Authorization string `header:"Authorization"`
	IfNoneMatch   string `header:"If-None-Match"`
}

type agentSyncOutput struct {
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	Body         sshsync.SyncResponse
}

type agentReportInput struct {
	Authorization string `header:"Authorization"`
	Body          sshsync.Report
}

func (h sshAgentAPI) agent(ctx context.Context, header string) (store.SSHAgent, error) {
	raw, ok := bearerToken(header)
	if !ok {
		return store.SSHAgent{}, huma.Error401Unauthorized("agent token required")
	}

	a, err := h.store.SSHAgentByTokenHash(ctx, hashAgentToken(raw))
	if errors.Is(err, store.ErrNotFound) {
		return store.SSHAgent{}, huma.Error401Unauthorized("agent token is not valid")
	}
	if err != nil {
		return store.SSHAgent{}, huma.Error500InternalServerError("could not check the token", err)
	}

	return a, nil
}

// sync serves what the calling agent must hold. A stale or unreadable
// snapshot is a 503, never an empty answer, so an outage cannot read as
// "revoke everything".
func (h sshAgentAPI) sync(ctx context.Context, in *agentSyncInput) (*agentSyncOutput, error) {
	agent, err := h.agent(ctx, in.Authorization)
	if err != nil {
		return nil, err
	}

	if h.snapshots.ConnectionStatus().State != "connected" {
		return nil, huma.Error503ServiceUnavailable("headscale state is not current; try again")
	}

	snap, err := h.snapshots.Current()
	if err != nil || snap == nil {
		return nil, huma.Error503ServiceUnavailable("headscale state is not available yet")
	}

	rev, err := h.store.SSHRevision(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not read the revision", err)
	}

	etag := fmt.Sprintf(`"%d.%d.%s"`, rev, snap.Revision, h.boot)
	if in.IfNoneMatch == etag {
		return nil, huma.Status304NotModified()
	}

	s, err := loadSSHState(ctx, h.store, snap)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("could not read the policy hosts", err)
	}
	if err := s.implausible(); err != nil {
		return nil, huma.Error503ServiceUnavailable("refusing to revoke every server", err)
	}

	out := &agentSyncOutput{ETag: etag, CacheControl: "no-store"}
	out.Body = sshsync.SyncResponse{
		Rev:      etag[1 : len(etag)-1],
		Protocol: sshsync.Protocol,
		Targets:  s.desired(agent.ID),
	}

	return out, nil
}

func pairs(ps []sshsync.Pair) []store.SSHManaged {
	out := make([]store.SSHManaged, 0, len(ps))
	for _, p := range ps {
		if sshsync.ValidUnixUser(p.UnixUser) && sshsync.ValidRef(p.Ref) {
			out = append(out, store.SSHManaged{TargetRef: p.Ref, UnixUser: p.UnixUser})
		}
	}

	return out
}

func (h sshAgentAPI) report(ctx context.Context, in *agentReportInput) (*struct{}, error) {
	agent, err := h.agent(ctx, in.Authorization)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("could not encode the report")
	}

	// Bindings this report will suspend, read first so the audit names only
	// real changes, not a mismatch repeated on every poll.
	var suspending []string
	if len(in.Body.Mismatched) > 0 {
		targets, err := h.store.ListSSHTargets(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("could not read bindings", err)
		}
		for _, t := range targets {
			if t.AgentID == agent.ID && !t.Suspended && slices.Contains(in.Body.Mismatched, t.TargetRef) {
				suspending = append(suspending, t.HostName)
			}
		}
	}

	err = h.store.RecordSSHReport(ctx, agent.ID, store.SSHReport{
		Mode:       in.Body.Mode,
		Version:    in.Body.Version,
		Raw:        raw,
		Applied:    pairs(in.Body.Applied),
		Cleared:    pairs(in.Body.Cleared),
		Mismatched: in.Body.Mismatched,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("could not record the report", err)
	}

	if len(suspending) > 0 {
		h.agentAudit(ctx, agent, "ssh_target.suspend", map[string]any{"servers": suspending, "refs": in.Body.Mismatched})
	}

	return nil, nil
}

// agentAudit records a change an agent made, under "agent:<name>". A failed
// audit write is not a failed report: the agent has already acted.
func (h sshAgentAPI) agentAudit(ctx context.Context, agent store.SSHAgent, action string, after any) {
	_ = h.store.Audit(ctx, store.AuditEntry{
		ActorLabel:  "agent:" + agent.Name,
		Action:      action,
		TargetType:  "ssh_agent",
		TargetLabel: agent.Name,
		TargetID:    fmt.Sprint(agent.ID),
		After:       mustJSON(after),
	})
}

type agentAuthInput struct {
	Authorization string `header:"Authorization"`
}

type agentManagedOutput struct {
	Body struct {
		Managed []sshsync.Pair `json:"managed"`
	}
}

type agentRetireInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		Cleared []sshsync.Pair `json:"cleared" maxItems:"10000"`
	}
}

type agentRetireOutput struct {
	Body struct {
		Retired   bool           `json:"retired"`
		Remaining []sshsync.Pair `json:"remaining"`
	}
}

func toPairs(ms []store.SSHManaged) []sshsync.Pair {
	out := make([]sshsync.Pair, 0, len(ms))
	for _, m := range ms {
		out = append(out, sshsync.Pair{Ref: m.TargetRef, UnixUser: m.UnixUser})
	}

	return out
}

// managed lists every block the calling agent has written and not yet
// cleared, which is what uninstall must remove before it leaves.
func (h sshAgentAPI) managed(ctx context.Context, in *agentAuthInput) (*agentManagedOutput, error) {
	agent, err := h.agent(ctx, in.Authorization)
	if err != nil {
		return nil, err
	}

	all, err := h.store.ListSSHManaged(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not read managed blocks", err)
	}

	var mine []store.SSHManaged
	for _, m := range all {
		if m.AgentID == agent.ID {
			mine = append(mine, m)
		}
	}

	out := &agentManagedOutput{}
	out.Body.Managed = toPairs(mine)

	return out, nil
}

// retire records what an uninstalling agent cleared and deletes the agent
// once nothing it wrote remains. Until then it says what is left and the
// agent stays, token and all.
func (h sshAgentAPI) retire(ctx context.Context, in *agentRetireInput) (*agentRetireOutput, error) {
	agent, err := h.agent(ctx, in.Authorization)
	if err != nil {
		return nil, err
	}

	left, err := h.store.RetireSSHAgent(ctx, agent.ID, pairs(in.Body.Cleared))
	if err != nil {
		return nil, huma.Error500InternalServerError("could not retire the agent", err)
	}

	if len(left) == 0 {
		h.agentAudit(ctx, agent, "ssh_agent.retire", map[string]any{"cleared": in.Body.Cleared})
	}

	out := &agentRetireOutput{}
	out.Body.Retired = len(left) == 0
	out.Body.Remaining = toPairs(left)

	return out, nil
}

func (h sshAgentAPI) register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "sshAgentSync",
		Method:      http.MethodGet,
		Path:        "/api/ssh/agent/sync",
		Summary:     "What the calling agent must hold (agent token)",
		Tags:        []string{"ssh-agent"},
	}, h.sync)

	huma.Register(api, huma.Operation{
		OperationID:  "sshAgentReport",
		Method:       http.MethodPost,
		Path:         "/api/ssh/agent/report",
		Summary:      "What the calling agent found, wrote and cleared (agent token)",
		Tags:         []string{"ssh-agent"},
		MaxBodyBytes: 64 << 10,
	}, h.report)

	huma.Register(api, huma.Operation{
		OperationID: "sshAgentManaged",
		Method:      http.MethodGet,
		Path:        "/api/ssh/agent/managed",
		Summary:     "Every block the calling agent has written (agent token)",
		Tags:        []string{"ssh-agent"},
	}, h.managed)

	huma.Register(api, huma.Operation{
		OperationID:  "sshAgentRetire",
		Method:       http.MethodPost,
		Path:         "/api/ssh/agent/retire",
		Summary:      "Record an uninstalling agent's last clears and delete it once none remain (agent token)",
		Tags:         []string{"ssh-agent"},
		MaxBodyBytes: 256 << 10,
	}, h.retire)
}

func installScriptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(installScript)
	}
}

// agentBinaryHandler serves the agent binaries bundled into the image. Only
// the two known names are ever opened; the path parameter is not a path.
func agentBinaryHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arch := chi.URLParam(r, "arch")
		if arch != "amd64" && arch != "arm64" {
			http.NotFound(w, r)

			return
		}

		path := filepath.Join(dir, "headboard-agent-linux-"+arch)
		if _, err := os.Stat(path); err != nil {
			http.Error(w, "agent binaries are not bundled in this build", http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path)
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		if deps.Tailnet == nil {
			return
		}

		boot := make([]byte, 6)
		_, _ = rand.Read(boot)

		sshAgentAPI{store: deps.Store, snapshots: deps.Tailnet, boot: hex.EncodeToString(boot)}.register(api)
	})
}
