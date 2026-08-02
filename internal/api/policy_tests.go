package api

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/acl"
	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/policy"
)

// policyDraftInput carries an optional pending change, so the tests runner and
// the simulator can answer questions about a policy that has not been saved.
//
// That ordering is the whole point: Headscale runs the tests block as a write
// boundary and refuses the save when one fails, and an ACL mistake locks people
// out of machines. Both of those are far cheaper to discover before the write.
type policyDraftInput struct {
	Body struct {
		// HuJSON replaces the whole document, for the raw editor.
		HuJSON string `json:"hujson,omitempty"`

		// Ops patches it instead, for the form.
		Ops []acl.Op `json:"ops,omitempty"`

		// SHA256 is required only when a draft is supplied, because the
		// draft is computed against the stored document.
		SHA256 string `json:"sha256,omitempty"`

		Source uint64 `json:"src,omitempty"`
		Dest   uint64 `json:"dst,omitempty"`

		DestinationIP string `json:"destinationIP,omitempty"`
		Port          uint16 `json:"port,omitempty"`
	}
}

type simulationDestination struct {
	nodeID  types.NodeID
	addr    netip.Addr
	literal bool
}

func parseSimulationDestination(dst uint64, destinationIP string) (simulationDestination, error) {
	destinationIP = strings.TrimSpace(destinationIP)

	if (dst == 0) == (destinationIP == "") {
		return simulationDestination{}, errors.New("choose one destination")
	}

	if dst != 0 {
		return simulationDestination{nodeID: types.NodeID(dst)}, nil
	}

	if strings.Contains(destinationIP, "/") {
		return simulationDestination{}, errors.New("enter one IPv4 or IPv6 address, not a CIDR")
	}

	addr, err := netip.ParseAddr(destinationIP)
	if err != nil {
		return simulationDestination{}, errors.New("enter a valid IPv4 or IPv6 address")
	}

	return simulationDestination{addr: addr, literal: true}, nil
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "runPolicyTests",
			Method:      http.MethodPost,
			Path:        "/api/policy/tests",
			Summary:     "Run the policy's own tests and sshTests blocks",
			Description: "Send no draft to test the stored policy, or ops/hujson to test a " +
				"pending change against the live tailnet before saving it.",
			Tags: []string{"policy"},
		}, func(ctx context.Context, in *policyDraftInput) (*struct{ Body policy.TestRun }, error) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			manager, draft, err := draftManager(ctx, deps, in)
			if err != nil {
				return nil, err
			}

			run, err := manager.RunTests(draft)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity("the policy could not be tested", err)
			}

			return &struct{ Body policy.TestRun }{Body: run}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "simulateConnection",
			Method:      http.MethodPost,
			Path:        "/api/policy/simulate",
			Summary:     "Can this device reach a device or IP address on this port?",
			Description: "Device destinations use their own filter, so rules using " +
				"autogroup:self are included. Literal IP answers are policy-only. Answers for a draft policy when one is sent.",
			Tags: []string{"policy"},
		}, func(ctx context.Context, in *policyDraftInput) (*struct{ Body policy.Simulation }, error) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			if in.Body.Source == 0 {
				return nil, huma.Error422UnprocessableEntity("choose a source")
			}

			if in.Body.Port == 0 {
				return nil, huma.Error422UnprocessableEntity("choose a port")
			}

			destination, err := parseSimulationDestination(in.Body.Dest, in.Body.DestinationIP)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}

			manager, draft, err := draftManager(ctx, deps, in)
			if err != nil {
				return nil, err
			}

			// A draft is simulated against a manager built from it, so
			// the answer describes the policy about to be saved rather
			// than the one currently in force.
			if draft != "" {
				manager, err = manager.WithPolicy(draft)
				if err != nil {
					return nil, huma.Error422UnprocessableEntity(
						"the draft policy does not compile", err)
				}
			}

			var sim policy.Simulation
			if destination.literal {
				sim, err = manager.SimulateIP(types.NodeID(in.Body.Source), destination.addr, in.Body.Port)
			} else {
				sim, err = manager.Simulate(types.NodeID(in.Body.Source), destination.nodeID, in.Body.Port)
			}
			if err != nil {
				if errors.Is(err, policy.ErrUnknownNode) {
					return nil, huma.Error404NotFound("no such device", err)
				}

				return nil, huma.Error500InternalServerError("could not simulate", err)
			}

			return &struct{ Body policy.Simulation }{Body: sim}, nil
		})
	})
}

// draftManager returns the snapshot's policy manager and the draft document to
// evaluate, which is empty when the caller sent no pending change.
func draftManager(ctx context.Context, deps Deps, in *policyDraftInput) (*policy.Manager, string, error) {
	snap, err := currentSnapshot(deps)
	if err != nil {
		return nil, "", err
	}

	if in.Body.HuJSON == "" && len(in.Body.Ops) == 0 {
		return snap.Manager, "", nil
	}

	write := &policyWriteInput{}
	write.Body.HuJSON = in.Body.HuJSON
	write.Body.Ops = in.Body.Ops
	write.Body.SHA256 = in.Body.SHA256

	_, next, err := applyChange(ctx, deps, write)
	if err != nil {
		return nil, "", err
	}

	return snap.Manager, next, nil
}
