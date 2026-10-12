package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
)

func newAgentToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}

	raw = "hbk_" + base64.RawURLEncoding.EncodeToString(b)

	return raw, hashAgentToken(raw), nil
}

func hashAgentToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(sum[:])
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}

	tok := strings.TrimSpace(header[len(prefix):])

	return tok, strings.HasPrefix(tok, "hbk_")
}

type SSHAgentView struct {
	ID            int64                `json:"id"`
	Name          string               `json:"name"`
	Mode          string               `json:"mode"`
	Version       string               `json:"version"`
	VersionMatch  bool                 `json:"versionMatch"`
	Online        bool                 `json:"online"`
	LastSeenAt    *time.Time           `json:"lastSeenAt,omitempty"`
	Discovered    []sshsync.Discovered `json:"discovered"`
	Warnings      []sshsync.Warning    `json:"warnings"`
	Bindings      int                  `json:"bindings"`
	PendingClears int                  `json:"pendingClears"`
}

func agentViews(s sshState, version string) []SSHAgentView {
	out := make([]SSHAgentView, 0, len(s.agents))

	for _, a := range s.agents {
		r := agentReport(a)
		v := SSHAgentView{
			ID: a.ID, Name: a.Name, Mode: a.Mode, Version: a.Version,
			VersionMatch: a.Version == version, Online: s.online(a), LastSeenAt: a.LastSeenAt,
			Discovered: r.Discovered, Warnings: r.Warnings,
		}
		if v.Discovered == nil {
			v.Discovered = []sshsync.Discovered{}
		}
		if v.Warnings == nil {
			v.Warnings = []sshsync.Warning{}
		}

		for _, t := range s.targets {
			if t.AgentID == a.ID {
				v.Bindings++
			}
		}
		for _, t := range s.desired(a.ID) {
			v.PendingClears += len(t.Clear)
		}

		out = append(out, v)
	}

	return out
}

// cleanupCommands is what an admin runs by hand after force-revoking an agent
// that still had blocks written. Only validated values appear in it: refs are
// re-checked here, because a ref a compromised agent reported would otherwise
// become shell text pasted into a root terminal.
func cleanupCommands(mode string, managed []store.SSHManaged, agentID int64) []string {
	var out []string

	for _, m := range managed {
		if m.AgentID != agentID || !sshsync.ValidUnixUser(m.UnixUser) || !sshsync.ValidRef(m.TargetRef) {
			continue
		}
		if (mode == "proxmox") != (m.TargetRef != "self") {
			continue
		}

		// Guarded by the end marker: without it the sed range runs to end of
		// file and takes every hand-added key with it.
		sed := fmt.Sprintf(`f=~%s/.ssh/authorized_keys; grep -qx "# END headboard" "$f" && sed -i "/^# BEGIN headboard/,/^# END headboard$/d" "$f"`, m.UnixUser)
		if mode == "proxmox" {
			out = append(out, fmt.Sprintf(`pct exec %s -- sh -c '%s'`, m.TargetRef, sed))
		} else {
			out = append(out, sed)
		}
	}

	return out
}

type sshAgentsOutput struct {
	Body struct {
		Agents []SSHAgentView `json:"agents"`
	}
}

type createSSHAgentInput struct {
	Body struct {
		Name string `json:"name" minLength:"1" maxLength:"63" pattern:"^[A-Za-z0-9][A-Za-z0-9._-]*$"`
	}
}

type sshAgentTokenOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Agent *SSHAgentView `json:"agent,omitempty"`
		Token string        `json:"token"`
	}
}

type sshAgentIDInput struct {
	ID int64 `path:"id"`
}

type revokeSSHAgentInput struct {
	ID    int64 `path:"id"`
	Force bool  `query:"force"`
}

type revokeSSHAgentOutput struct {
	Body struct {
		Cleanup []string `json:"cleanup"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		state := func(ctx context.Context) (sshState, error) {
			snap, err := currentSnapshot(deps)
			if err != nil {
				return sshState{}, err
			}

			s, err := loadSSHState(ctx, deps.Store, snap)
			if err != nil {
				return sshState{}, huma.Error503ServiceUnavailable("could not read the policy hosts", err)
			}

			return s, nil
		}

		huma.Register(api, huma.Operation{
			OperationID: "listSSHAgents",
			Method:      http.MethodGet,
			Path:        "/api/ssh/agents",
			Summary:     "SSH key agents and what they last reported",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, _ *struct{}) (*sshAgentsOutput, error) {
			if _, err := require(ctx, auth.CapManageSSH); err != nil {
				return nil, err
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}

			out := &sshAgentsOutput{}
			out.Body.Agents = agentViews(s, deps.Version)

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "createSSHAgent",
			Method:      http.MethodPost,
			Path:        "/api/ssh/agents",
			Summary:     "Create an agent; the token is returned once",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *createSSHAgentInput) (*sshAgentTokenOutput, error) {
			p, err := require(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			raw, hash, err := newAgentToken()
			if err != nil {
				return nil, huma.Error500InternalServerError("could not generate a token", err)
			}

			a, err := deps.Store.CreateSSHAgent(ctx, in.Body.Name, hash)
			if errors.Is(err, store.ErrAgentNameTaken) {
				return nil, huma.Error409Conflict(err.Error())
			}
			if err != nil {
				return nil, statusFor(err, "could not create the agent")
			}

			// The token is deliberately absent from the audit entry.
			audit(ctx, deps, p, "ssh_agent.create", "ssh_agent", a.ID, nil, map[string]string{"name": a.Name})

			out := &sshAgentTokenOutput{CacheControl: "no-store"}
			out.Body.Agent = &SSHAgentView{ID: a.ID, Name: a.Name, Discovered: []sshsync.Discovered{}, Warnings: []sshsync.Warning{}}
			out.Body.Token = raw

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "rotateSSHAgent",
			Method:      http.MethodPost,
			Path:        "/api/ssh/agents/{id}/rotate",
			Summary:     "Replace an agent's token; the old one stops working at once",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *sshAgentIDInput) (*sshAgentTokenOutput, error) {
			p, err := require(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			raw, hash, err := newAgentToken()
			if err != nil {
				return nil, huma.Error500InternalServerError("could not generate a token", err)
			}

			err = deps.Store.RotateSSHAgentToken(ctx, in.ID, hash)
			if errors.Is(err, store.ErrNotFound) {
				return nil, huma.Error404NotFound("no such agent")
			}
			if err != nil {
				return nil, statusFor(err, "could not rotate the token")
			}

			audit(ctx, deps, p, "ssh_agent.rotate", "ssh_agent", in.ID, nil, nil)

			out := &sshAgentTokenOutput{CacheControl: "no-store"}
			out.Body.Token = raw

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "revokeSSHAgent",
			Method:      http.MethodDelete,
			Path:        "/api/ssh/agents/{id}",
			Summary:     "Revoke an agent; refused while it still has bindings or pending clears unless forced",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *revokeSSHAgentInput) (*revokeSSHAgentOutput, error) {
			p, err := require(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}

			var agent *SSHAgentView
			for _, v := range agentViews(s, deps.Version) {
				if v.ID == in.ID {
					agent = &v
				}
			}
			if agent == nil {
				return nil, huma.Error404NotFound("no such agent")
			}

			if !in.Force && (agent.Bindings > 0 || agent.PendingClears > 0) {
				return nil, huma.Error409Conflict(
					"unbind this agent's servers and wait until its pending clears are done, or force the revoke")
			}

			gone, err := deps.Store.DeleteSSHAgent(ctx, in.ID)
			if err != nil {
				return nil, statusFor(err, "could not revoke the agent")
			}

			// Built from the rows the delete removed, not the earlier read: a
			// report landing in between must not leave a block out.
			out := &revokeSSHAgentOutput{}
			out.Body.Cleanup = cleanupCommands(agent.Mode, gone, in.ID)
			if out.Body.Cleanup == nil {
				out.Body.Cleanup = []string{}
			}

			audit(ctx, deps, p, "ssh_agent.revoke", "ssh_agent", in.ID, map[string]any{"name": agent.Name, "force": in.Force},
				map[string]any{"cleanup": out.Body.Cleanup})

			return out, nil
		})
	})
}
