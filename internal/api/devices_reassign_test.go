package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/types/key"

	"github.com/jemang/headboard/internal/hs"
)

// reassignMutator records what reassignNode asked Headscale to do. The embedded
// interface is nil: any call the test did not expect panics.
type reassignMutator struct {
	hs.Mutator

	registered  types.Node
	registerErr error
	deleteErr   map[types.NodeID]error
	deleted     []types.NodeID
}

func (m *reassignMutator) RegisterNode(context.Context, string, string) (types.Node, error) {
	return m.registered, m.registerErr
}

func (m *reassignMutator) DeleteNode(_ context.Context, id types.NodeID) error {
	if err := m.deleteErr[id]; err != nil {
		return err
	}

	m.deleted = append(m.deleted, id)

	return nil
}

func ownedNode(id types.NodeID, owner string, mk key.MachinePublic) types.Node {
	u := types.User{Name: owner}
	u.ID = uint(id) + 100

	return types.Node{ID: id, GivenName: "laptop", MachineKey: mk, UserID: &u.ID, User: &u}
}

const testAuthID = "hskey-authreq-abcdefghijklmnopqrstuvwx"

func TestReassignDeletesTheOldNodeOnceTheSameMachineRegistered(t *testing.T) {
	mk := key.NewMachine().Public()
	m := &reassignMutator{registered: ownedNode(9, "bob", mk)}

	got, err := reassignNode(t.Context(), m, ownedNode(1, "alice", mk), testAuthID, "bob")
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != 9 {
		t.Errorf("returned node %d, want the new one", got.ID)
	}

	if len(m.deleted) != 1 || m.deleted[0] != 1 {
		t.Errorf("deleted %v, want only the old node", m.deleted)
	}
}

// A mistyped auth id belongs to some other machine. Approving it must not cost
// the device being reassigned its access, and the stray registration is undone.
func TestReassignFromAnotherMachineRollsBackAndKeepsTheOldNode(t *testing.T) {
	m := &reassignMutator{registered: ownedNode(9, "bob", key.NewMachine().Public())}

	_, err := reassignNode(t.Context(), m, ownedNode(1, "alice", key.NewMachine().Public()), testAuthID, "bob")
	if err == nil || !strings.Contains(err.Error(), "different machine") {
		t.Fatalf("got %v, want a different-machine refusal", err)
	}

	if len(m.deleted) != 1 || m.deleted[0] != 9 {
		t.Errorf("deleted %v, want only the stray new node", m.deleted)
	}
}

func TestReassignThatFailsToRegisterDeletesNothing(t *testing.T) {
	m := &reassignMutator{registerErr: errors.New("no such auth id")}

	if _, err := reassignNode(t.Context(), m, ownedNode(1, "alice", key.NewMachine().Public()), testAuthID, "bob"); err == nil {
		t.Fatal("a failed registration reported success")
	}

	if len(m.deleted) != 0 {
		t.Errorf("deleted %v after a failed registration", m.deleted)
	}
}

func TestReassignSaysSoWhenTheOldNodeSurvives(t *testing.T) {
	mk := key.NewMachine().Public()
	m := &reassignMutator{
		registered: ownedNode(9, "bob", mk),
		deleteErr:  map[types.NodeID]error{1: errors.New("boom")},
	}

	_, err := reassignNode(t.Context(), m, ownedNode(1, "alice", mk), testAuthID, "bob")
	if err == nil || !strings.Contains(err.Error(), "old device is still listed") {
		t.Fatalf("got %v, want an error naming the leftover node", err)
	}
}

func TestReassignRefusesTaggedNodesAndTheCurrentOwner(t *testing.T) {
	mk := key.NewMachine().Public()

	tagged := types.Node{ID: 1, MachineKey: mk, Tags: types.Strings{"tag:prod"}}
	if _, err := reassignNode(t.Context(), &reassignMutator{}, tagged, testAuthID, "bob"); err == nil {
		t.Error("a tagged device was reassigned to a user")
	}

	if _, err := reassignNode(t.Context(), &reassignMutator{}, ownedNode(1, "alice", mk), testAuthID, "alice"); err == nil {
		t.Error("a device was reassigned to the user who already owns it")
	}
}
