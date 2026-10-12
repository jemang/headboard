package api

import (
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/store"
)

func TestNewAgentTokenIsPrefixedRandomAndHashed(t *testing.T) {
	raw1, hash1, err := newAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	raw2, _, _ := newAgentToken()

	if !strings.HasPrefix(raw1, "hbk_") || len(raw1) < 40 || raw1 == raw2 {
		t.Fatalf("tokens %q %q", raw1, raw2)
	}
	if hash1 != hashAgentToken(raw1) || strings.Contains(hash1, raw1) || len(hash1) != 64 {
		t.Fatalf("hash %q", hash1)
	}
}

func TestBearerToken(t *testing.T) {
	if tok, ok := bearerToken("Bearer hbk_abc"); !ok || tok != "hbk_abc" {
		t.Fatalf("got %q %v", tok, ok)
	}
	for _, h := range []string{"", "hbk_abc", "Bearer abc", "Basic hbk_abc"} {
		if _, ok := bearerToken(h); ok {
			t.Errorf("accepted %q", h)
		}
	}
}

func TestAgentViewsCountBindingsAndPendingClears(t *testing.T) {
	s := fixtureState(t)

	v := agentViews(s, "1.0.0")
	if len(v) != 1 || v[0].Bindings != 1 || v[0].PendingClears != 1 || v[0].VersionMatch {
		t.Fatalf("views = %+v", v)
	}
	if len(v[0].Discovered) != 2 {
		t.Fatalf("discovered = %+v", v[0].Discovered)
	}
}

func TestCleanupCommandsQuoteNothingFromGuests(t *testing.T) {
	cmds := cleanupCommands("proxmox", []store.SSHManaged{
		{AgentID: 7, TargetRef: "105", UnixUser: "sysadmin"},
		{AgentID: 8, TargetRef: "200", UnixUser: "deploy"},
	}, 7)

	want := `pct exec 105 -- sh -c 'f=~sysadmin/.ssh/authorized_keys; grep -qx "# END headboard" "$f" && sed -i "/^# BEGIN headboard/,/^# END headboard$/d" "$f"'`
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("cmds = %q", cmds)
	}

	local := cleanupCommands("local", []store.SSHManaged{{AgentID: 7, TargetRef: "self", UnixUser: "deploy"}}, 7)
	if local[0] != `f=~deploy/.ssh/authorized_keys; grep -qx "# END headboard" "$f" && sed -i "/^# BEGIN headboard/,/^# END headboard$/d" "$f"` {
		t.Fatalf("local = %q", local)
	}
}

func TestCleanupCommandsSkipUnsafeRefs(t *testing.T) {
	cmds := cleanupCommands("proxmox", []store.SSHManaged{
		{AgentID: 7, TargetRef: "1; reboot", UnixUser: "deploy"},
		{AgentID: 7, TargetRef: "self", UnixUser: "deploy"},
	}, 7)
	if len(cmds) != 0 {
		t.Fatalf("unsafe or wrong-mode refs produced commands: %q", cmds)
	}
}
