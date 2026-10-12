package agent

import (
	"testing"
	"time"
)

// A container whose shell never returns must cost one timeout, not the whole
// cycle: Discover waits for every container before any sync happens. The
// background sleep keeps stdout open after sh is killed, as lxc-attach can.
func TestRunnerGivesUpOnAHungCommand(t *testing.T) {
	r := ExecRunner{Timeout: 200 * time.Millisecond}

	start := time.Now()
	_, err := r.Run(t.Context(), "", "sh", "-c", "sleep 30 & sleep 30")

	if err == nil {
		t.Fatal("a hung command reported success")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Run returned after %v", took)
	}
}

// A container's root can swap any binary the agent calls for one that never
// stops printing. Four containers are read at once on the Proxmox host itself.
func TestRunnerRefusesOversizedOutput(t *testing.T) {
	r := ExecRunner{Timeout: 10 * time.Second}

	start := time.Now()
	out, err := r.Run(t.Context(), "", "yes")

	if err == nil {
		t.Fatal("unbounded output reported success")
	}
	if len(out) > maxOutput {
		t.Fatalf("kept %d bytes", len(out))
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("Run returned after %v", took)
	}
}

func TestRunnerTruncatesStderrInErrors(t *testing.T) {
	_, err := ExecRunner{}.Run(t.Context(), "", "sh", "-c", "head -c 100000 /dev/zero | tr '\\0' x >&2; exit 1")

	if err == nil || len(err.Error()) > 1024 {
		t.Fatalf("error is %d bytes", len(err.Error()))
	}
}
