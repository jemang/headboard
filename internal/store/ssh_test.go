package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jemang/headboard/internal/store"
)

func rev(t *testing.T, st *store.Store) int64 {
	t.Helper()

	r, err := st.SSHRevision(t.Context())
	if err != nil {
		t.Fatalf("SSHRevision: %v", err)
	}

	return r
}

func owner(t *testing.T, st *store.Store) int64 {
	t.Helper()

	u, err := st.UpsertLogin(t.Context(), identity("https://idp", "o", "o@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	return u.ID
}

func TestSSHKeysAddListDeleteBumpRevision(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)
	r0 := rev(t, st)

	k, err := st.AddSSHKey(ctx, store.SSHKey{HeadscaleUserID: 3, PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:a", Label: "laptop"}, by)
	if err != nil {
		t.Fatalf("AddSSHKey: %v", err)
	}
	if rev(t, st) != r0+1 {
		t.Error("add did not bump the revision")
	}

	if _, err := st.AddSSHKey(ctx, store.SSHKey{HeadscaleUserID: 4, PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:a"}, by); !errors.Is(err, store.ErrDuplicateSSHKey) {
		t.Errorf("duplicate fingerprint err = %v", err)
	}

	uid := uint64(3)
	keys, err := st.ListSSHKeys(ctx, &uid)
	if err != nil || len(keys) != 1 || keys[0].Label != "laptop" {
		t.Fatalf("ListSSHKeys = %+v, %v", keys, err)
	}

	if _, err := st.DeleteSSHKey(ctx, k.ID); err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
	if _, err := st.DeleteSSHKey(ctx, k.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete err = %v", err)
	}
}

func TestSetSSHRowReplacesAndEmptyDeletes(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()

	if err := st.SetSSHRow(ctx, "app", "sysadmin", []uint64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSSHRow(ctx, "app", "sysadmin", []uint64{2}); err != nil {
		t.Fatal(err)
	}

	got, _ := st.ListSSHAssignments(ctx)
	if len(got) != 1 || got[0].HeadscaleUserID != 2 {
		t.Fatalf("assignments = %+v", got)
	}

	if err := st.SetSSHRow(ctx, "app", "sysadmin", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ListSSHAssignments(ctx); len(got) != 0 {
		t.Fatalf("empty list left %+v", got)
	}
}

func TestDeleteSSHUserRemovesKeysAndAssignments(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)

	st.AddSSHKey(ctx, store.SSHKey{HeadscaleUserID: 5, PublicKey: "k", Fingerprint: "SHA256:5"}, by)
	st.SetSSHRow(ctx, "app", "deploy", []uint64{5, 6})

	if err := st.DeleteSSHUser(ctx, 5); err != nil {
		t.Fatal(err)
	}

	keys, _ := st.ListSSHKeys(ctx, nil)
	assigns, _ := st.ListSSHAssignments(ctx)
	if len(keys) != 0 || len(assigns) != 1 || assigns[0].HeadscaleUserID != 6 {
		t.Fatalf("keys = %+v assigns = %+v", keys, assigns)
	}
}

func TestApproveSSHTargetRejectsSecondHostForSameTarget(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)

	a, err := st.CreateSSHAgent(ctx, "pve1", "hash1")
	if err != nil {
		t.Fatal(err)
	}

	t1 := store.SSHTarget{HostName: "app", AgentID: a.ID, TargetRef: "105", ApprovedIP: "192.0.2.10"}
	if err := st.ApproveSSHTarget(ctx, t1, by); err != nil {
		t.Fatal(err)
	}

	t2 := store.SSHTarget{HostName: "app-alias", AgentID: a.ID, TargetRef: "105", ApprovedIP: "192.0.2.10"}
	if err := st.ApproveSSHTarget(ctx, t2, by); !errors.Is(err, store.ErrTargetTaken) {
		t.Fatalf("second host err = %v", err)
	}
}

func TestReapproveClearsSuspension(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)
	a, _ := st.CreateSSHAgent(ctx, "pve1", "hash1")
	tg := store.SSHTarget{HostName: "app", AgentID: a.ID, TargetRef: "105", ApprovedIP: "192.0.2.10"}
	st.ApproveSSHTarget(ctx, tg, by)

	if err := st.RecordSSHReport(ctx, a.ID, store.SSHReport{Mode: "proxmox", Raw: json.RawMessage(`{}`), Mismatched: []string{"105"}}); err != nil {
		t.Fatal(err)
	}
	ts, _ := st.ListSSHTargets(ctx)
	if !ts[0].Suspended {
		t.Fatal("mismatch report did not suspend")
	}

	st.ApproveSSHTarget(ctx, tg, by)
	ts, _ = st.ListSSHTargets(ctx)
	if ts[0].Suspended {
		t.Fatal("re-approval did not clear the suspension")
	}
}

func TestRecordSSHReportOnlyBumpsOnRealChange(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	a, _ := st.CreateSSHAgent(ctx, "pve1", "hash1")
	applied := []store.SSHManaged{{TargetRef: "105", UnixUser: "deploy"}}

	r0 := rev(t, st)
	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Mode: "proxmox", Raw: json.RawMessage(`{}`), Applied: applied})
	if rev(t, st) != r0+1 {
		t.Fatal("first applied report did not bump")
	}

	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Mode: "proxmox", Raw: json.RawMessage(`{}`), Applied: applied})
	if rev(t, st) != r0+1 {
		t.Fatal("an identical report bumped the revision")
	}

	m, _ := st.ListSSHManaged(ctx)
	if len(m) != 1 || m[0].AgentID != a.ID {
		t.Fatalf("managed = %+v", m)
	}

	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Mode: "proxmox", Raw: json.RawMessage(`{}`), Cleared: applied})
	if m, _ := st.ListSSHManaged(ctx); len(m) != 0 {
		t.Fatalf("cleared row kept: %+v", m)
	}

	agents, _ := st.ListSSHAgents(ctx)
	if agents[0].LastSeenAt == nil || agents[0].Mode != "proxmox" {
		t.Fatalf("agent = %+v", agents[0])
	}
}

func TestSSHAgentTokenLookupRotateDelete(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)

	a, _ := st.CreateSSHAgent(ctx, "pve1", "old")
	if _, err := st.CreateSSHAgent(ctx, "pve1", "other"); !errors.Is(err, store.ErrAgentNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}

	if err := st.RotateSSHAgentToken(ctx, a.ID, "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SSHAgentByTokenHash(ctx, "old"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old token still works: %v", err)
	}
	if got, err := st.SSHAgentByTokenHash(ctx, "new"); err != nil || got.ID != a.ID {
		t.Fatalf("new token = %+v, %v", got, err)
	}

	st.ApproveSSHTarget(ctx, store.SSHTarget{HostName: "app", AgentID: a.ID, TargetRef: "105", ApprovedIP: "1.2.3.4"}, by)
	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Raw: json.RawMessage(`{}`), Applied: []store.SSHManaged{{TargetRef: "105", UnixUser: "deploy"}}})

	// What it returns is read in the same transaction as the delete, so the
	// revoke's cleanup commands cannot miss a block reported a moment before.
	gone, err := st.DeleteSSHAgent(ctx, a.ID)
	if err != nil || len(gone) != 1 || gone[0].UnixUser != "deploy" {
		t.Fatalf("deleted managed = %+v, %v", gone, err)
	}
	ts, _ := st.ListSSHTargets(ctx)
	m, _ := st.ListSSHManaged(ctx)
	if len(ts) != 0 || len(m) != 0 {
		t.Fatalf("agent delete left targets %+v managed %+v", ts, m)
	}
}

func TestSSHStateReadsEverythingTogether(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	by := owner(t, st)
	a, _ := st.CreateSSHAgent(ctx, "pve1", "h")
	st.AddSSHKey(ctx, store.SSHKey{HeadscaleUserID: 1, PublicKey: "k", Fingerprint: "SHA256:1"}, by)
	st.SetSSHRow(ctx, "app", "deploy", []uint64{1})
	st.ApproveSSHTarget(ctx, store.SSHTarget{HostName: "app", AgentID: a.ID, TargetRef: "105", ApprovedIP: "1.2.3.4"}, by)
	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Raw: json.RawMessage(`{}`), Applied: []store.SSHManaged{{TargetRef: "105", UnixUser: "deploy"}}})

	s, err := st.SSHState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Keys) != 1 || len(s.Assignments) != 1 || len(s.Targets) != 1 || len(s.Agents) != 1 || len(s.Managed) != 1 {
		t.Fatalf("state = %+v", s)
	}
}

// An agent leaves only once every block it wrote is gone: deleting it earlier
// would drop Headboard's record of keys still sitting on servers.
func TestRetireSSHAgentDeletesOnlyOnceNothingRemains(t *testing.T) {
	st := openStore(t)
	ctx := t.Context()
	a, _ := st.CreateSSHAgent(ctx, "pve1", "hash1")
	other, _ := st.CreateSSHAgent(ctx, "pve2", "hash2")
	deploy := store.SSHManaged{TargetRef: "105", UnixUser: "deploy"}
	ops := store.SSHManaged{TargetRef: "106", UnixUser: "ops"}
	st.RecordSSHReport(ctx, a.ID, store.SSHReport{Raw: json.RawMessage(`{}`), Applied: []store.SSHManaged{deploy, ops}})
	st.RecordSSHReport(ctx, other.ID, store.SSHReport{Raw: json.RawMessage(`{}`), Applied: []store.SSHManaged{deploy}})

	left, err := st.RetireSSHAgent(ctx, a.ID, []store.SSHManaged{deploy})
	if err != nil || len(left) != 1 || left[0].UnixUser != "ops" {
		t.Fatalf("left = %+v, %v", left, err)
	}
	if _, err := st.SSHAgentByTokenHash(ctx, "hash1"); err != nil {
		t.Fatalf("agent deleted while ops remained: %v", err)
	}

	left, err = st.RetireSSHAgent(ctx, a.ID, []store.SSHManaged{ops})
	if err != nil || len(left) != 0 {
		t.Fatalf("left = %+v, %v", left, err)
	}
	if _, err := st.SSHAgentByTokenHash(ctx, "hash1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("agent still there: %v", err)
	}

	m, _ := st.ListSSHManaged(ctx)
	if len(m) != 1 || m[0].AgentID != other.ID {
		t.Fatalf("another agent's rows were touched: %+v", m)
	}
}

func TestDeleteSSHTargetReportsAMissingBinding(t *testing.T) {
	st := openStore(t)
	if err := st.DeleteSSHTarget(t.Context(), "never-bound"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
