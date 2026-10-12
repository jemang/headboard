package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
)

// SSH keys and logins are credentials. Only owner and admin hold manage:ssh;
// every other role must be refused by the server, whatever the UI hides.
func TestSSHEndpointsRefuseRolesWithoutManageSSH(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	_, api := humatest.New(t)
	for _, fn := range registrations {
		fn(api, Deps{Store: st})
	}

	// Bodies are valid, so a refusal can only come from the role check.
	calls := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/ssh/access", nil},
		{http.MethodPut, "/api/ssh/access/app/deploy", map[string]any{"userIds": []int{1}}},
		{http.MethodPost, "/api/ssh/targets/app", map[string]any{"agentId": 1, "targetRef": "105"}},
		{http.MethodDelete, "/api/ssh/targets/app", nil},
		{http.MethodDelete, "/api/ssh/access/app", nil},
		{http.MethodGet, "/api/ssh/agents", nil},
		{http.MethodPost, "/api/ssh/agents", map[string]any{"name": "pve9"}},
		{http.MethodPost, "/api/ssh/agents/1/rotate", nil},
		{http.MethodDelete, "/api/ssh/agents/1", nil},
		{http.MethodGet, "/api/users/1/ssh-keys", nil},
		{http.MethodPost, "/api/users/1/ssh-keys", map[string]any{"publicKey": "ssh-ed25519 AAAA", "label": "x"}},
		{http.MethodDelete, "/api/ssh-keys/1", nil},
	}

	for _, role := range []store.Role{store.RoleNetworkAdmin, store.RoleAuditor, store.RoleMember} {
		ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{ID: 9, Role: role, Admission: store.AdmissionActive}})
		for _, c := range calls {
			args := []any{}
			if c.body != nil {
				args = append(args, c.body)
			}
			resp := api.DoCtx(ctx, c.method, c.path, args...)
			if resp.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: got %d, want 403 (route missing?)", role, c.method, c.path, resp.Code)
			}
		}
	}
}

// One agent's report must never touch another agent's records, even when it
// names the same container and user.
func TestAnAgentCannotClearAnotherAgentsBlocks(t *testing.T) {
	h, rawA, st := agentFixture(t)
	a := &agentReportInput{Authorization: "Bearer " + rawA}
	a.Body = sshsync.Report{Mode: "proxmox", Applied: []sshsync.Pair{{Ref: "105", UnixUser: "sysadmin"}}}
	if _, err := h.report(t.Context(), a); err != nil {
		t.Fatal(err)
	}

	rawB, hashB, _ := newAgentToken()
	if _, err := st.CreateSSHAgent(t.Context(), "pve2", hashB); err != nil {
		t.Fatal(err)
	}
	b := &agentReportInput{Authorization: "Bearer " + rawB}
	b.Body = sshsync.Report{Mode: "proxmox", Cleared: []sshsync.Pair{{Ref: "105", UnixUser: "sysadmin"}}}
	if _, err := h.report(t.Context(), b); err != nil {
		t.Fatal(err)
	}

	m, _ := st.ListSSHManaged(t.Context())
	if len(m) != 1 || m[0].UnixUser != "sysadmin" {
		t.Fatalf("agent B cleared agent A's record: %+v", m)
	}
}

// Rotation is how a leaked token is killed; the old one must stop at once.
func TestARotatedTokenIsRefusedBySync(t *testing.T) {
	h, raw, st := agentFixture(t)
	a, _ := st.SSHAgentByTokenHash(t.Context(), hashAgentToken(raw))
	_, hash, _ := newAgentToken()
	if err := st.RotateSSHAgentToken(t.Context(), a.ID, hash); err != nil {
		t.Fatal(err)
	}

	if _, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw}); status(err) != http.StatusUnauthorized {
		t.Fatalf("old token after rotate: %v", err)
	}
}
