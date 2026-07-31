package policy

import (
	"strings"
	"testing"
)

// withBlocks appends extra top-level sections to the dev policy, so every test
// here runs against the same acls an operator would actually have.
func withBlocks(t *testing.T, blocks string) string {
	t.Helper()

	// devPolicy ends in a trailing comma, which is legal HuJSON but would
	// double up against the comma these blocks start with.
	body := strings.TrimSuffix(strings.TrimSpace(devPolicy), "}")
	body = strings.TrimSuffix(strings.TrimRight(body, " \n\t"), ",")

	return body + blocks + "\n}"
}

func runner(t *testing.T, policyText string) TestRun {
	t.Helper()

	users, nodes := tailnet()

	m, err := New(devPolicy, users, nodes)
	if err != nil {
		t.Fatalf("building manager: %v", err)
	}

	run, err := m.RunTests(policyText)
	if err != nil {
		t.Fatalf("running tests: %v", err)
	}

	return run
}

func TestADocumentWithoutTestsReportsThatRatherThanPassing(t *testing.T) {
	run := runner(t, devPolicy)

	if run.Ran {
		t.Error("a document with no tests block should report Ran = false")
	}

	if len(run.Assertions) != 0 {
		t.Errorf("expected no assertions, got %d", len(run.Assertions))
	}
}

func TestEveryDestinationInATestBecomesItsOwnAssertion(t *testing.T) {
	run := runner(t, withBlocks(t, `,
  "tests": [
    {"src": "alice@", "accept": ["tag:prod:443"], "deny": ["tag:prod:22"]},
  ]`))

	if !run.Ran || !run.AllPassed {
		t.Fatalf("expected a passing run, got ran=%v passed=%v", run.Ran, run.AllPassed)
	}

	if len(run.Assertions) != 2 {
		t.Fatalf("expected 2 assertions (one accept, one deny), got %d", len(run.Assertions))
	}

	for _, a := range run.Assertions {
		if !a.Passed {
			t.Errorf("%s %s -> %s failed: %s", a.Kind, a.Src, a.Dst, a.Error)
		}

		if a.Pointer != "/tests/0" {
			t.Errorf("assertion should deep-link to its entry, got %q", a.Pointer)
		}
	}
}

// The whole point of the runner: Headscale reports "test(s) failed" for the
// block, and the UI has to say which destination was wrong.
func TestOneFailingDestinationDoesNotFailItsSiblings(t *testing.T) {
	run := runner(t, withBlocks(t, `,
  "tests": [
    // alice reaches :443 but not :22, so the second destination is a lie.
    {"src": "alice@", "accept": ["tag:prod:443", "tag:prod:22"]},
  ]`))

	if run.AllPassed {
		t.Fatal("expected the run to fail")
	}

	if len(run.Assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(run.Assertions))
	}

	byDst := map[string]Assertion{}
	for _, a := range run.Assertions {
		byDst[a.Dst] = a
	}

	if got := byDst["tag:prod:443"]; !got.Passed {
		t.Errorf("alice does reach :443, but it was reported as failing: %s", got.Error)
	}

	if got := byDst["tag:prod:22"]; got.Passed {
		t.Error("alice cannot reach :22, so that assertion must fail")
	}
}

func TestADenyThatIsActuallyAllowedFails(t *testing.T) {
	run := runner(t, withBlocks(t, `,
  "tests": [
    {"src": "alice@", "deny": ["tag:prod:443"]},
  ]`))

	if run.AllPassed {
		t.Fatal("alice reaches :443, so denying it must fail")
	}

	if run.Assertions[0].Kind != "deny" {
		t.Errorf("expected a deny assertion, got %q", run.Assertions[0].Kind)
	}
}

func TestSSHTestsExpandPerUserAndDestination(t *testing.T) {
	run := runner(t, withBlocks(t, `,
  "ssh": [
    {"action": "accept", "src": ["group:ops"], "dst": ["tag:prod"], "users": ["root"]},
  ],
  "sshTests": [
    {"src": "ops@", "dst": ["tag:prod"], "accept": ["root"], "deny": ["alice"]},
  ]`))

	if !run.Ran {
		t.Fatal("expected the sshTests block to run")
	}

	if len(run.Assertions) != 2 {
		t.Fatalf("expected one assertion per user, got %d", len(run.Assertions))
	}

	for _, a := range run.Assertions {
		if a.Section != "sshTests" {
			t.Errorf("expected section sshTests, got %q", a.Section)
		}

		if a.User == "" {
			t.Error("an sshTests assertion is about a login user; User must be set")
		}

		if !a.Passed {
			t.Errorf("%s %s/%s -> %s failed: %s", a.Kind, a.Src, a.User, a.Dst, a.Error)
		}
	}
}

// An assertion the engine cannot evaluate is a different kind of wrong from an
// assertion that is false, and the UI shows the reason.
func TestAnUnresolvableSourceReportsItsReason(t *testing.T) {
	run := runner(t, withBlocks(t, `,
  "tests": [
    {"src": "group:nobody", "accept": ["tag:prod:443"]},
  ]`))

	if run.AllPassed {
		t.Fatal("a test naming an undefined group cannot pass")
	}

	if run.Assertions[0].Error == "" {
		t.Error("expected the engine's reason to be reported")
	}
}

// A draft is tested against the live tailnet, which is what makes it possible
// to catch a lockout before saving.
func TestADraftIsTestedWithoutTouchingTheLivePolicy(t *testing.T) {
	users, nodes := tailnet()

	m, err := New(devPolicy, users, nodes)
	if err != nil {
		t.Fatalf("building manager: %v", err)
	}

	draft := withBlocks(t, `,
  "tests": [
    {"src": "alice@", "accept": ["tag:prod:22"]},
  ]`)

	run, err := m.RunTests(draft)
	if err != nil {
		t.Fatalf("running tests: %v", err)
	}

	if run.AllPassed {
		t.Error("the draft asserts something false and should fail")
	}

	if m.hujson != devPolicy {
		t.Error("testing a draft must not replace the live policy")
	}
}
