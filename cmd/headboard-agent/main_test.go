package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jemang/headboard/internal/agent"
)

// Running update from a copy elsewhere must still upgrade the binary the
// service runs, or it reports success while the service keeps the old one.
func TestUpdateTargetsTheInstalledBinary(t *testing.T) {
	saved := agent.Installed
	t.Cleanup(func() { agent.Installed = saved })

	agent.Installed.Exe = filepath.Join(t.TempDir(), "headboard-agent")
	if _, err := updateTarget(); err == nil {
		t.Fatal("a missing installed binary was accepted as the target")
	}

	if err := os.WriteFile(agent.Installed.Exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := updateTarget()
	if err != nil || got != agent.Installed.Exe {
		t.Fatalf("target = %q, %v; want %q", got, err, agent.Installed.Exe)
	}
}
