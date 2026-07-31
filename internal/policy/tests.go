package policy

import (
	"encoding/json"
	"fmt"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/types/views"
)

// Assertion is one claim a tests-block entry makes, reported on its own.
//
// A `tests` entry can assert several destinations at once and an `sshTests`
// entry asserts every listed user against every listed destination, so one
// entry usually produces several assertions. Reporting them individually is the
// point: "test 2 failed" sends an operator back to the document to work out
// which of its six destinations was wrong.
type Assertion struct {
	// Section is "tests" or "sshTests", and Index the entry's position, so
	// the editor can deep-link to the row that made the claim.
	Section string `json:"section"`
	Index   int    `json:"index"`
	Pointer string `json:"pointer"`

	// Kind is accept, deny or check.
	Kind string `json:"kind"`

	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Proto string `json:"proto,omitempty"`

	// User is the login an sshTests entry asserts. Empty for ACL tests,
	// whose assertions are about reachability rather than identity.
	User string `json:"user,omitempty"`

	Passed bool `json:"passed"`

	// Error carries the engine's reason when an assertion could not be
	// evaluated at all — an unresolvable src, a malformed destination —
	// which is a different failure from the assertion being false.
	Error string `json:"error,omitempty"`
}

// TestRun is the outcome of evaluating a policy's tests and sshTests blocks.
type TestRun struct {
	// Ran is false when the document asserts nothing, which the UI reports
	// as "no tests" rather than as a pass.
	Ran bool `json:"ran"`

	AllPassed  bool        `json:"allPassed"`
	Assertions []Assertion `json:"assertions"`
}

// RunTests evaluates a policy's own tests and sshTests blocks.
//
// Pass the empty string to test the live policy, or a draft to test one before
// it is saved. Testing the draft is the useful case: Headscale's SetPolicy runs
// the tests block as a write boundary and rejects the whole write when one
// fails, so an operator who cannot see which assertion failed is left guessing
// at a save that simply will not go through.
//
// Evaluation is Headscale's. The engine exports RunTests only as an error over
// the whole block, so this narrows a failure by re-running each assertion in a
// document holding that assertion alone. That is exact rather than approximate:
// the tests block does not contribute to the compiled filter, so removing the
// other assertions cannot change the answer for the one that remains.
func (m *Manager) RunTests(hujsonText string) (TestRun, error) {
	m.mu.RLock()
	live, users, slice, livePM := m.hujson, m.users, m.views, m.pm
	m.mu.RUnlock()

	if hujsonText == "" {
		hujsonText = live
	}

	doc, err := decodeForAttribution(hujsonText)
	if err != nil {
		return TestRun{}, err
	}

	assertions, err := assertionsIn(doc)
	if err != nil {
		return TestRun{}, err
	}

	if len(assertions) == 0 {
		return TestRun{AllPassed: true, Assertions: []Assertion{}}, nil
	}

	// Compile the document as a whole before anything else, so a policy
	// that does not build reports one clear error instead of the same
	// error repeated once per assertion.
	pm := livePM

	if hujsonText != live || pm == nil {
		pm, err = policyv2.NewPolicyManager([]byte(hujsonText), users, slice)
		if err != nil {
			return TestRun{}, fmt.Errorf("compiling policy: %w", err)
		}
	}

	// The common case is a block that passes, and that costs one call per
	// section. Only a section that failed is narrowed down assertion by
	// assertion, and a failure in one section does not slow the other.
	aclOK := pm.RunTests() == nil
	sshOK := pm.RunSSHTests() == nil

	run := TestRun{Ran: true, AllPassed: true, Assertions: assertions}

	for i := range run.Assertions {
		a := &run.Assertions[i]

		if (a.Section == sectionTests && aclOK) || (a.Section == sectionSSHTests && sshOK) {
			a.Passed = true

			continue
		}

		m.evaluateAssertion(doc, a, users, slice)

		if !a.Passed {
			run.AllPassed = false
		}
	}

	return run, nil
}

const (
	sectionTests    = "tests"
	sectionSSHTests = "sshTests"
)

// testEntry mirrors policyv2.PolicyTest. It is decoded here rather than through
// the engine's own type because that type's UnmarshalJSON resolves aliases
// eagerly, and a test naming a group that no longer exists must still be
// listed — with its failure — rather than taking the whole run down.
type testEntry struct {
	Src    string   `json:"src"`
	Proto  string   `json:"proto,omitempty"`
	Accept []string `json:"accept,omitempty"`
	Deny   []string `json:"deny,omitempty"`
}

type sshTestEntry struct {
	Src    string   `json:"src"`
	Dst    []string `json:"dst"`
	Accept []string `json:"accept,omitempty"`
	Deny   []string `json:"deny,omitempty"`
	Check  []string `json:"check,omitempty"`
}

// assertionsIn expands both test blocks into individual claims.
func assertionsIn(doc map[string]json.RawMessage) ([]Assertion, error) {
	out := []Assertion{}

	if raw, ok := doc[sectionTests]; ok {
		var entries []testEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("decoding tests: %w", err)
		}

		for i, e := range entries {
			for _, kind := range []string{"accept", "deny"} {
				for _, dst := range pick(kind, e.Accept, e.Deny, nil) {
					out = append(out, Assertion{
						Section: sectionTests,
						Index:   i,
						Pointer: fmt.Sprintf("/%s/%d", sectionTests, i),
						Kind:    kind,
						Src:     e.Src,
						Dst:     dst,
						Proto:   e.Proto,
					})
				}
			}
		}
	}

	if raw, ok := doc[sectionSSHTests]; ok {
		var entries []sshTestEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("decoding sshTests: %w", err)
		}

		for i, e := range entries {
			for _, kind := range []string{"accept", "deny", "check"} {
				for _, user := range pick(kind, e.Accept, e.Deny, e.Check) {
					for _, dst := range e.Dst {
						out = append(out, Assertion{
							Section: sectionSSHTests,
							Index:   i,
							Pointer: fmt.Sprintf("/%s/%d", sectionSSHTests, i),
							Kind:    kind,
							Src:     e.Src,
							Dst:     dst,
							User:    user,
						})
					}
				}
			}
		}
	}

	return out, nil
}

func pick(kind string, accept, deny, check []string) []string {
	switch kind {
	case "accept":
		return accept
	case "deny":
		return deny
	default:
		return check
	}
}

// evaluateAssertion re-runs one assertion in a document containing only it.
func (m *Manager) evaluateAssertion(
	doc map[string]json.RawMessage,
	a *Assertion,
	users []types.User,
	slice views.Slice[types.NodeView],
) {
	isolated, err := isolateAssertion(doc, *a)
	if err != nil {
		a.Error = err.Error()

		return
	}

	pm, err := policyv2.NewPolicyManager(isolated, users, slice)
	if err != nil {
		// The document compiled as a whole, so a failure here is about
		// this assertion's own shape — an unknown tag in an sshTests
		// dst, for instance, which Headscale rejects at parse time.
		a.Error = err.Error()

		return
	}

	if a.Section == sectionTests {
		err = pm.RunTests()
	} else {
		err = pm.RunSSHTests()
	}

	if err != nil {
		a.Error = err.Error()

		return
	}

	a.Passed = true
}

// isolateAssertion returns the document with both test blocks replaced by the
// single assertion under examination.
//
// Everything else is kept verbatim, including ssh and acls: sshTests are
// evaluated against the compiled SSH policy and tests against the compiled
// filter, so dropping either section would change the answer.
func isolateAssertion(doc map[string]json.RawMessage, a Assertion) ([]byte, error) {
	out := make(map[string]json.RawMessage, len(doc))

	for k, v := range doc {
		out[k] = v
	}

	delete(out, sectionTests)
	delete(out, sectionSSHTests)

	var entry any

	if a.Section == sectionTests {
		e := testEntry{Src: a.Src, Proto: a.Proto}
		if a.Kind == "accept" {
			e.Accept = []string{a.Dst}
		} else {
			e.Deny = []string{a.Dst}
		}

		entry = e
	} else {
		e := sshTestEntry{Src: a.Src, Dst: []string{a.Dst}}

		switch a.Kind {
		case "accept":
			e.Accept = []string{a.User}
		case "deny":
			e.Deny = []string{a.User}
		default:
			e.Check = []string{a.User}
		}

		entry = e
	}

	list, err := json.Marshal([]any{entry})
	if err != nil {
		return nil, fmt.Errorf("marshalling assertion: %w", err)
	}

	out[a.Section] = list

	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshalling policy: %w", err)
	}

	return b, nil
}

// WithPolicy returns a manager over the same tailnet but a different policy
// document, for answering "what would this draft do" before it is saved.
func (m *Manager) WithPolicy(hujsonText string) (*Manager, error) {
	m.mu.RLock()
	live, users, nodes := m.hujson, m.users, m.nodes
	m.mu.RUnlock()

	if hujsonText == "" || hujsonText == live {
		return m, nil
	}

	return New(hujsonText, users, nodes)
}
