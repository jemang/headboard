package sshsync

import (
	"reflect"
	"testing"
)

var (
	servers = []Server{{Name: "app", IP: "192.0.2.10"}}
	users   = map[uint64]string{1: "alice", 2: "bob"}
	keys    = []StoredKey{
		{HeadscaleUserID: 1, Line: "ssh-ed25519 AAAA1", Label: "laptop"},
		{HeadscaleUserID: 1, Line: "ssh-ed25519 AAAA2", Label: "desktop"},
		{HeadscaleUserID: 2, Line: "ssh-ed25519 AAAA3"},
	}
	bound = []Binding{{HostName: "app", AgentID: 7, TargetRef: "105", ApprovedIP: "192.0.2.10"}}
)

func TestDesiredWritesEveryKeyOfEveryAssignedPerson(t *testing.T) {
	assigns := []Assignment{
		{HostName: "app", UnixUser: "sysadmin", HeadscaleUserID: 1},
		{HostName: "app", UnixUser: "sysadmin", HeadscaleUserID: 2},
		{HostName: "app", UnixUser: "deploy", HeadscaleUserID: 2},
	}

	got := Desired(7, servers, users, bound, assigns, keys, nil)

	want := []Target{{
		Ref:      "105",
		ExpectIP: "192.0.2.10",
		Users: map[string][]string{
			"deploy": {"ssh-ed25519 AAAA3 bob"},
			"sysadmin": {
				"ssh-ed25519 AAAA1 alice laptop",
				"ssh-ed25519 AAAA2 alice desktop",
				"ssh-ed25519 AAAA3 bob",
			},
		},
		Clear: []string{},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Desired =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDesiredIgnoresOtherAgents(t *testing.T) {
	assigns := []Assignment{{HostName: "app", UnixUser: "deploy", HeadscaleUserID: 1}}
	if got := Desired(8, servers, users, bound, assigns, keys, nil); len(got) != 0 {
		t.Fatalf("agent 8 got %+v", got)
	}
}

func TestDesiredDropsDeletedUsers(t *testing.T) {
	assigns := []Assignment{{HostName: "app", UnixUser: "deploy", HeadscaleUserID: 9}}
	managed := []Managed{{AgentID: 7, TargetRef: "105", UnixUser: "deploy"}}

	got := Desired(7, servers, users, bound, assigns, keys, managed)

	if len(got) != 1 || len(got[0].Users) != 0 || !reflect.DeepEqual(got[0].Clear, []string{"deploy"}) {
		t.Fatalf("deleted user's row was not cleared: %+v", got)
	}
}

func TestDesiredKeepsRowWithNoKeysAsEmptyBlock(t *testing.T) {
	assigns := []Assignment{{HostName: "app", UnixUser: "staging", HeadscaleUserID: 2}}

	got := Desired(7, servers, users, bound, assigns, []StoredKey{}, nil)

	if lines, ok := got[0].Users["staging"]; !ok || lines == nil || len(lines) != 0 {
		t.Fatalf("staging = %#v, want present and empty", got[0].Users)
	}
}

func TestDesiredClearsWhenHostLeavesPolicy(t *testing.T) {
	managed := []Managed{{AgentID: 7, TargetRef: "105", UnixUser: "sysadmin"}}

	got := Desired(7, nil, users, bound, nil, keys, managed)

	want := []Target{{Ref: "105", Users: map[string][]string{}, Clear: []string{"sysadmin"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Desired = %+v, want %+v", got, want)
	}
}

func TestDesiredClearsUnboundTarget(t *testing.T) {
	managed := []Managed{{AgentID: 7, TargetRef: "105", UnixUser: "sysadmin"}}

	got := Desired(7, servers, users, nil, nil, keys, managed)

	if len(got) != 1 || got[0].ExpectIP != "" || !reflect.DeepEqual(got[0].Clear, []string{"sysadmin"}) {
		t.Fatalf("Desired = %+v", got)
	}
}

func TestDesiredFreezesSuspendedBinding(t *testing.T) {
	moved := []Server{{Name: "app", IP: "192.0.2.99"}}
	assigns := []Assignment{{HostName: "app", UnixUser: "sysadmin", HeadscaleUserID: 1}}
	managed := []Managed{{AgentID: 7, TargetRef: "105", UnixUser: "sysadmin"}}

	if got := Desired(7, moved, users, bound, assigns, keys, managed); len(got) != 0 {
		t.Fatalf("suspended binding produced %+v", got)
	}

	flagged := []Binding{{HostName: "app", AgentID: 7, TargetRef: "105", ApprovedIP: "192.0.2.10", Suspended: true}}
	if got := Desired(7, servers, users, flagged, assigns, keys, managed); len(got) != 0 {
		t.Fatalf("agent-flagged binding produced %+v", got)
	}
}

func TestStateOf(t *testing.T) {
	ips := map[string]string{"app": "192.0.2.10"}
	b := bound[0]

	if s := StateOf(b, ips); s != StateActive {
		t.Errorf("state = %s", s)
	}
	if s := StateOf(b, map[string]string{"app": "10.0.0.1"}); s != StateSuspended {
		t.Errorf("moved state = %s", s)
	}
	if s := StateOf(b, map[string]string{}); s != StateOrphan {
		t.Errorf("missing state = %s", s)
	}
}

func TestDesiredSendsApprovedHostname(t *testing.T) {
	labelled := []Binding{{HostName: "app", AgentID: 7, TargetRef: "105", ApprovedIP: "192.0.2.10", ApprovedLabel: "app-1"}}

	got := Desired(7, servers, users, labelled, nil, keys, nil)

	if len(got) != 1 || got[0].ExpectLabel != "app-1" {
		t.Fatalf("Desired = %+v", got)
	}
}
