package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
)

type fakeSnapshots struct {
	snap  *tailnet.Snapshot
	state string
}

func (f fakeSnapshots) Current() (*tailnet.Snapshot, error) { return f.snap, nil }
func (f fakeSnapshots) ConnectionStatus() tailnet.ConnectionStatus {
	return tailnet.ConnectionStatus{State: f.state}
}

func agentFixture(t *testing.T) (sshAgentAPI, string, *store.Store) {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	raw, hash, _ := newAgentToken()
	a, _ := st.CreateSSHAgent(t.Context(), "pve1", hash)
	owner, _ := st.UpsertLogin(t.Context(), store.User{OIDCIssuer: "i", OIDCSubject: "s", Email: "o@x"})
	st.ApproveSSHTarget(t.Context(), store.SSHTarget{HostName: "app", AgentID: a.ID, TargetRef: "105", ApprovedIP: "192.0.2.10"}, owner.ID)
	st.SetSSHRow(t.Context(), "app", "sysadmin", []uint64{1})
	st.AddSSHKey(t.Context(), store.SSHKey{HeadscaleUserID: 1, PublicKey: "ssh-ed25519 AAAA1", Fingerprint: "SHA256:1"}, owner.ID)

	alice := types.User{Name: "alice"}
	alice.ID = 1
	snap := &tailnet.Snapshot{
		Users:    []types.User{alice},
		Policy:   hs.Policy{HuJSON: `{"hosts": {"app": "192.0.2.10"}}`},
		Revision: 3,
	}

	return sshAgentAPI{store: st, snapshots: fakeSnapshots{snap: snap, state: "connected"}}, raw, st
}

func status(err error) int {
	var se huma.StatusError
	if errors.As(err, &se) {
		return se.GetStatus()
	}
	return 0
}

func TestSyncReturnsDesiredStateAndETag(t *testing.T) {
	h, raw, _ := agentFixture(t)

	out, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw})
	if err != nil {
		t.Fatal(err)
	}
	if out.ETag == "" || out.Body.Protocol != sshsync.Protocol || len(out.Body.Targets) != 1 {
		t.Fatalf("out = %+v", out)
	}
	if got := out.Body.Targets[0].Users["sysadmin"]; len(got) != 1 || got[0] != "ssh-ed25519 AAAA1 alice" {
		t.Fatalf("sysadmin = %q", got)
	}

	_, err = h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw, IfNoneMatch: out.ETag})
	if status(err) != http.StatusNotModified {
		t.Fatalf("same ETag err = %v", err)
	}
}

func TestSyncRejectsBadToken(t *testing.T) {
	h, _, _ := agentFixture(t)

	for _, hdr := range []string{"", "Bearer hbk_wrong", "Bearer nothbk"} {
		if _, err := h.sync(t.Context(), &agentSyncInput{Authorization: hdr}); status(err) != http.StatusUnauthorized {
			t.Errorf("%q err = %v", hdr, err)
		}
	}
}

func TestSyncRefusesStaleSnapshot(t *testing.T) {
	h, raw, _ := agentFixture(t)
	f := h.snapshots.(fakeSnapshots)
	f.state = "stale"
	h.snapshots = f

	if _, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw}); status(err) != http.StatusServiceUnavailable {
		t.Fatalf("stale err = %v", err)
	}
}

func TestSyncRefusesUnparseablePolicy(t *testing.T) {
	h, raw, _ := agentFixture(t)
	f := h.snapshots.(fakeSnapshots)
	f.snap = &tailnet.Snapshot{Policy: hs.Policy{HuJSON: `{"hosts": `}}
	h.snapshots = f

	if _, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw}); status(err) != http.StatusServiceUnavailable {
		t.Fatalf("broken policy err = %v", err)
	}
}

func TestReportRecordsAppliedAndDropsInvalidUnixUsers(t *testing.T) {
	h, raw, st := agentFixture(t)

	in := &agentReportInput{Authorization: "Bearer " + raw}
	in.Body = sshsync.Report{
		Mode:    "proxmox",
		Version: "1.0.0",
		Applied: []sshsync.Pair{{Ref: "105", UnixUser: "sysadmin"}, {Ref: "105", UnixUser: "../root"}},
	}
	if _, err := h.report(t.Context(), in); err != nil {
		t.Fatal(err)
	}

	m, _ := st.ListSSHManaged(t.Context())
	if len(m) != 1 || m[0].UnixUser != "sysadmin" {
		t.Fatalf("managed = %+v", m)
	}
}

func TestAgentBinaryHandlerServesOnlyKnownArches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "headboard-agent-linux-amd64"), []byte("ELF"), 0o644)

	r := chi.NewRouter()
	r.Get("/api/ssh/agent/bin/{arch}", agentBinaryHandler(dir))

	for path, want := range map[string]int{
		"/api/ssh/agent/bin/amd64":             http.StatusOK,
		"/api/ssh/agent/bin/arm64":             http.StatusNotFound,
		"/api/ssh/agent/bin/..%2Fetc%2Fpasswd": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestInstallScriptRefusesHTTP(t *testing.T) {
	rec := httptest.NewRecorder()
	installScriptHandler()(rec, httptest.NewRequest(http.MethodGet, "/api/ssh/agent/install.sh", nil))

	body := rec.Body.String()
	for _, want := range []string{"https://*)", "--proto '=https'", "/dev/tty", "systemctl enable headboard-agent"} {
		if !strings.Contains(body, want) {
			t.Errorf("installer lacks %q", want)
		}
	}
}

func TestReportDropsUnsafeRefs(t *testing.T) {
	h, raw, st := agentFixture(t)

	in := &agentReportInput{Authorization: "Bearer " + raw}
	in.Body = sshsync.Report{Mode: "proxmox", Applied: []sshsync.Pair{{Ref: "105'; reboot; '", UnixUser: "deploy"}}}
	if _, err := h.report(t.Context(), in); err != nil {
		t.Fatal(err)
	}

	if m, _ := st.ListSSHManaged(t.Context()); len(m) != 0 {
		t.Fatalf("managed = %+v", m)
	}
}

// snap.Revision restarts at 1 with the process, so an ETag built only from
// revisions can repeat across a restart and hide a change behind a 304.
func TestSyncETagDiffersAcrossRestarts(t *testing.T) {
	h, raw, _ := agentFixture(t)
	before, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw})
	if err != nil {
		t.Fatal(err)
	}

	restarted := h
	restarted.boot = "other-boot"
	_, err = restarted.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw, IfNoneMatch: before.ETag})
	if status(err) == http.StatusNotModified {
		t.Fatal("a restarted Headboard answered 304 to an ETag from before the restart")
	}
}

func TestInstallScriptRestoresEchoOnInterrupt(t *testing.T) {
	script := string(installScript)
	trap := strings.Index(script, "trap 'stty echo </dev/tty")
	off := strings.Index(script, "stty -echo")
	if trap < 0 || trap > off {
		t.Fatal("echo is turned off before a trap restores it on Ctrl-C")
	}
}

func TestInstallScriptTakesTokenFromEnvironment(t *testing.T) {
	script := string(installScript)
	env := strings.Index(script, `token=${HEADBOARD_AGENT_TOKEN:-}`)
	prompt := strings.Index(script, "Agent token (hbk_...)")
	if env < 0 || prompt < 0 || env > prompt {
		t.Fatal("installer must read HEADBOARD_AGENT_TOKEN before falling back to the prompt")
	}
}

// Re-running the installer is how an agent is upgraded or re-tokened, so it
// must restart a unit that is already running, not only start a stopped one.
func TestInstallScriptRestartsTheAgent(t *testing.T) {
	if !strings.Contains(string(installScript), "systemctl restart headboard-agent") {
		t.Fatal("installer does not restart an already running agent")
	}
}

// A valid policy that lost its hosts block, or a Headscale answering with no
// users, would turn every binding into a clear. That is far likelier to be a
// mistake than an intent, so sync refuses rather than revoke every server.
func TestSyncRefusesAnEmptyWorldThatWouldRevokeEverything(t *testing.T) {
	for name, mutate := range map[string]func(*tailnet.Snapshot){
		"no hosts": func(s *tailnet.Snapshot) { s.Policy = hs.Policy{HuJSON: `{"acls": []}`} },
		"no users": func(s *tailnet.Snapshot) { s.Users = nil },
	} {
		t.Run(name, func(t *testing.T) {
			h, raw, _ := agentFixture(t)
			snap := *h.snapshots.(fakeSnapshots).snap
			mutate(&snap)
			h.snapshots = fakeSnapshots{snap: &snap, state: "connected"}

			_, err := h.sync(t.Context(), &agentSyncInput{Authorization: "Bearer " + raw})
			if status(err) != http.StatusServiceUnavailable {
				t.Fatalf("err = %v, want 503", err)
			}
		})
	}
}

// Uninstall asks what it wrote, clears it, then retires. The agent and its
// token go only when nothing it wrote is left on any server.
func TestAgentRetiresOnlyAfterClearingEverythingItWrote(t *testing.T) {
	h, raw, st := agentFixture(t)
	auth := "Bearer " + raw

	rep := &agentReportInput{Authorization: auth}
	rep.Body = sshsync.Report{Mode: "proxmox", Applied: []sshsync.Pair{{Ref: "105", UnixUser: "sysadmin"}, {Ref: "106", UnixUser: "ops"}}}
	if _, err := h.report(t.Context(), rep); err != nil {
		t.Fatal(err)
	}

	got, err := h.managed(t.Context(), &agentAuthInput{Authorization: auth})
	if err != nil || len(got.Body.Managed) != 2 {
		t.Fatalf("managed = %+v, %v", got, err)
	}

	in := &agentRetireInput{Authorization: auth}
	in.Body.Cleared = []sshsync.Pair{{Ref: "105", UnixUser: "sysadmin"}}
	out, err := h.retire(t.Context(), in)
	if err != nil || out.Body.Retired || len(out.Body.Remaining) != 1 || out.Body.Remaining[0].Ref != "106" {
		t.Fatalf("partial retire = %+v, %v", out, err)
	}

	in.Body.Cleared = []sshsync.Pair{{Ref: "106", UnixUser: "ops"}}
	out, err = h.retire(t.Context(), in)
	if err != nil || !out.Body.Retired {
		t.Fatalf("retire = %+v, %v", out, err)
	}

	if _, err := h.sync(t.Context(), &agentSyncInput{Authorization: auth}); status(err) != http.StatusUnauthorized {
		t.Fatalf("token still works after retire: %v", err)
	}
	if ts, _ := st.ListSSHTargets(t.Context()); len(ts) != 0 {
		t.Fatalf("bindings left behind: %+v", ts)
	}
	if e, _ := st.ListAudit(t.Context(), store.AuditFilter{Action: "ssh_agent.retire"}); len(e) != 1 || e[0].ActorLabel != "agent:pve1" {
		t.Fatalf("retire audit = %+v", e)
	}
}

// The token is written into a key=value config, so a newline in it would add
// a line of its own (a second url=, say). The installer's check is run here
// for real, cut out of the script by its markers.
func TestInstallScriptAcceptsOnlyWellFormedTokens(t *testing.T) {
	script := string(installScript)
	start, end := strings.Index(script, "# token-check:begin"), strings.Index(script, "# token-check:end")
	if start < 0 || end < start {
		t.Fatal("token check markers missing")
	}
	check := script[start:end]

	for token, ok := range map[string]bool{
		"hbk_e1FVwJ2n-_X9":                 true,
		"hbk_":                             false,
		"hbk_ab\nurl=https://evil.example": false,
		"hbk_ab cd":                        false,
		"nope":                             false,
	} {
		err := exec.Command("sh", "-c", "token=$1\n"+check+"echo ok", "sh", token).Run()
		if (err == nil) != ok {
			t.Errorf("token %q: accepted = %v, want %v", token, err == nil, ok)
		}
	}
}

// Written through a temp file and installed 0600 in one step, so the token is
// never in a file other users can read, even for a moment or on a re-run.
func TestInstallScriptWritesTheConfigPrivately(t *testing.T) {
	if !strings.Contains(string(installScript), "install -m 0600 \"$cfg\" /etc/headboard-agent/config") {
		t.Fatal("config is not installed with mode 0600")
	}
}

// Guest-reported text is stored and shown to admins; the schema bounds each
// field rather than leaning on the 64 KiB body cap alone.
func TestReportSchemaBoundsGuestReportedFields(t *testing.T) {
	h, raw, _ := agentFixture(t)
	_, api := humatest.New(t)
	h.register(api)

	report := func(extra map[string]any) map[string]any {
		b := map[string]any{"mode": "proxmox", "version": "1.0.0", "protocol": sshsync.Protocol}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}

	ok := api.Post("/api/ssh/agent/report", "Authorization: Bearer "+raw, report(map[string]any{
		"discovered": []any{map[string]any{"ref": "105", "hostname": "app-1", "ips": []string{"192.0.2.10"}}},
		"warnings":   []any{map[string]any{"ref": "105", "message": "fine"}},
	}))
	if ok.Code != http.StatusNoContent {
		t.Fatalf("a well-formed report got %d: %s", ok.Code, ok.Body.String())
	}

	ips := make([]string, 40)
	for i := range ips {
		ips[i] = "192.0.2.1"
	}
	for name, extra := range map[string]map[string]any{
		"hostname": {"discovered": []any{map[string]any{"ref": "105", "hostname": strings.Repeat("h", 300)}}},
		"ips":      {"discovered": []any{map[string]any{"ref": "105", "hostname": "a", "ips": ips}}},
		"message":  {"warnings": []any{map[string]any{"ref": "105", "message": strings.Repeat("m", 3000)}}},
	} {
		resp := api.Post("/api/ssh/agent/report", "Authorization: Bearer "+raw, report(extra))
		if resp.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, want 422", name, resp.Code)
		}
	}
}

// An agent suspending a binding changes who can log in; it is recorded like
// an admin's change, under the agent's name.
func TestReportedMismatchIsAudited(t *testing.T) {
	h, raw, st := agentFixture(t)

	in := &agentReportInput{Authorization: "Bearer " + raw}
	in.Body = sshsync.Report{Mode: "proxmox", Mismatched: []string{"105"}}
	if _, err := h.report(t.Context(), in); err != nil {
		t.Fatal(err)
	}

	entries, _ := st.ListAudit(t.Context(), store.AuditFilter{})
	found := false
	for _, e := range entries {
		found = found || (e.Action == "ssh_target.suspend" && e.ActorLabel == "agent:pve1" && strings.Contains(string(e.After), "app"))
	}
	if !found {
		t.Fatalf("no suspension audit entry in %+v", entries)
	}
}
