package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
)

func reportJSON(t *testing.T, r sshsync.Report) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureState(t *testing.T) sshState {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	seen := now.Add(-time.Minute)

	return sshState{
		now:     now,
		servers: []sshsync.Server{{Name: "app", IP: "192.0.2.10"}, {Name: "web", IP: "198.51.100.5"}},
		users:   map[uint64]string{1: "alice", 2: "bob"},
		targets: []store.SSHTarget{{HostName: "app", AgentID: 7, TargetRef: "105", ApprovedIP: "192.0.2.10", ApprovedLabel: "app-1"}},
		assigns: []store.SSHAssignment{
			{HostName: "app", UnixUser: "sysadmin", HeadscaleUserID: 1},
			{HostName: "app", UnixUser: "sysadmin", HeadscaleUserID: 9},
			{HostName: "gone", UnixUser: "deploy", HeadscaleUserID: 1},
		},
		keys: []store.SSHKey{{HeadscaleUserID: 1, PublicKey: "ssh-ed25519 AAAA1"}},
		agents: []store.SSHAgent{{ID: 7, Name: "pve1", LastSeenAt: &seen, LastReport: reportJSON(t, sshsync.Report{
			Discovered: []sshsync.Discovered{
				{Ref: "105", Hostname: "app-1", IPs: []string{"192.0.2.10"}},
				{Ref: "120", Hostname: "web-1", IPs: []string{"198.51.100.5"}},
			},
			Warnings: []sshsync.Warning{{Ref: "105", UnixUser: "sysadmin", Message: "unix user does not exist"}},
		})}},
		managed: []store.SSHManaged{{AgentID: 7, TargetRef: "105", UnixUser: "staging"}},
	}
}

func TestBuildAccessShowsBindingRowsWarningsAndPendingClears(t *testing.T) {
	v := buildAccess(fixtureState(t))

	if len(v.Servers) != 2 || v.Servers[0].Name != "app" {
		t.Fatalf("servers = %+v", v.Servers)
	}

	app := v.Servers[0]
	if app.Binding == nil || app.Binding.State != sshsync.StateActive || app.Binding.AgentName != "pve1" {
		t.Fatalf("binding = %+v", app.Binding)
	}
	if len(app.Rows) != 1 || len(app.Rows[0].Users) != 2 {
		t.Fatalf("rows = %+v", app.Rows)
	}
	if !app.Rows[0].Users[1].Missing || app.Rows[0].Users[0].Keys != 1 {
		t.Fatalf("row users = %+v", app.Rows[0].Users)
	}
	if len(app.Rows[0].Warnings) != 1 {
		t.Fatalf("warnings = %+v", app.Rows[0].Warnings)
	}
	if len(app.PendingClears) != 1 || app.PendingClears[0] != "staging" {
		t.Fatalf("pending = %+v", app.PendingClears)
	}
}

func TestBuildAccessSuggestsUnboundServer(t *testing.T) {
	v := buildAccess(fixtureState(t))

	web := v.Servers[1]
	if web.Binding != nil || len(web.Suggestions) != 1 || web.Suggestions[0].TargetRef != "120" || web.Suggestions[0].Ambiguous {
		t.Fatalf("web = %+v", web)
	}
}

func TestBuildAccessListsOrphanHosts(t *testing.T) {
	v := buildAccess(fixtureState(t))

	if len(v.Orphans) != 1 || v.Orphans[0].HostName != "gone" {
		t.Fatalf("orphans = %+v", v.Orphans)
	}
}

func TestApproveRefusesAmbiguousIP(t *testing.T) {
	s := fixtureState(t)
	seen := s.now.Add(-time.Minute)
	s.agents = append(s.agents, store.SSHAgent{ID: 8, Name: "pve2", LastSeenAt: &seen, LastReport: reportJSON(t, sshsync.Report{
		Discovered: []sshsync.Discovered{{Ref: "300", IPs: []string{"198.51.100.5"}}},
	})})

	if _, err := approvalCandidate(s, "web", 7, "120"); err == nil {
		t.Fatal("approved an IP that two targets report")
	}

	v := buildAccess(s)
	if len(v.Servers[1].Suggestions) != 2 || !v.Servers[1].Suggestions[0].Ambiguous {
		t.Fatalf("suggestions = %+v", v.Servers[1].Suggestions)
	}
}

func TestApproveRequiresMatchingDiscoveredTarget(t *testing.T) {
	s := fixtureState(t)

	if _, err := approvalCandidate(s, "web", 7, "105"); err == nil {
		t.Fatal("approved a target whose IP does not match the host")
	}
	if _, err := approvalCandidate(s, "nope", 7, "120"); err == nil {
		t.Fatal("approved a host that is not a server")
	}
	d, err := approvalCandidate(s, "web", 7, "120")
	if err != nil || d.Hostname != "web-1" {
		t.Fatalf("candidate = %+v, %v", d, err)
	}
}

func TestApproveRefusesUnsafeRef(t *testing.T) {
	s := fixtureState(t)
	s.agents[0].LastReport = reportJSON(t, sshsync.Report{
		Discovered: []sshsync.Discovered{{Ref: "120; reboot", IPs: []string{"198.51.100.5"}}},
	})

	if _, err := approvalCandidate(s, "web", 7, "120; reboot"); err == nil {
		t.Fatal("approved a ref that is not a VMID or self")
	}
}

func TestStaleAgentReportIsIgnored(t *testing.T) {
	s := fixtureState(t)
	old := s.now.Add(-time.Hour)
	s.agents[0].LastSeenAt = &old

	if v := buildAccess(s); len(v.Servers[1].Suggestions) != 0 {
		t.Fatalf("offline agent still suggests: %+v", v.Servers[1].Suggestions)
	}
	if _, err := approvalCandidate(s, "web", 7, "120"); err == nil {
		t.Fatal("approved from an offline agent's old report")
	}
	if v := agentViews(s, ""); v[0].Online {
		t.Fatal("agent last seen an hour ago shown as online")
	}
}

func TestSuspendedBindingSaysWhetherReapproveCanWork(t *testing.T) {
	s := fixtureState(t)
	s.servers[0].IP = "192.0.2.99"

	if b := buildAccess(s).Servers[0].Binding; b == nil || b.State != sshsync.StateSuspended || b.Reapprovable {
		t.Fatalf("binding = %+v, want suspended and not re-approvable", b)
	}

	seen := s.now.Add(-time.Minute)
	s.agents[0].LastSeenAt = &seen
	s.agents[0].LastReport = reportJSON(t, sshsync.Report{Discovered: []sshsync.Discovered{{Ref: "105", Hostname: "app-1", IPs: []string{"192.0.2.99"}}}})

	if b := buildAccess(s).Servers[0].Binding; !b.Reapprovable {
		t.Fatalf("binding = %+v, want re-approvable once the agent reports the new IP", b)
	}
}

// Suspension freezes a server on purpose, but a removal made meanwhile has not
// landed. The card must say so, or the admin believes the person is revoked.
func TestSuspendedServerListsTheClearsWaitingOnIt(t *testing.T) {
	s := fixtureState(t)
	s.servers[0].IP = "192.0.2.99"

	app := buildAccess(s).Servers[0]
	if app.Binding == nil || app.Binding.State != sshsync.StateSuspended {
		t.Fatalf("binding = %+v", app.Binding)
	}
	if len(app.PendingClears) != 1 || app.PendingClears[0] != "staging" {
		t.Fatalf("pendingClears = %q, want the frozen clear", app.PendingClears)
	}
}

// A store miss is a 404, not a server error: unbinding a server that was never
// bound, deleting a key twice.
func TestStatusForMapsNotFoundTo404(t *testing.T) {
	if got := status(statusFor(store.ErrNotFound, "no such binding")); got != http.StatusNotFound {
		t.Fatalf("ErrNotFound → %d", got)
	}
	if got := status(statusFor(errors.New("disk full"), "x")); got != http.StatusInternalServerError {
		t.Fatalf("other error → %d", got)
	}
}

func TestBuildAccessListsPeopleWithKeyCounts(t *testing.T) {
	v := buildAccess(fixtureState(t))

	want := []SSHPerson{{ID: 1, Name: "alice", Keys: 1}, {ID: 2, Name: "bob", Keys: 0}}
	if len(v.People) != len(want) {
		t.Fatalf("people = %+v", v.People)
	}
	for i := range want {
		if v.People[i] != want[i] {
			t.Fatalf("people[%d] = %+v, want %+v", i, v.People[i], want[i])
		}
	}
}
