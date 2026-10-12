package agent

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jemang/headboard/internal/sshsync"
)

type scripted struct {
	mu      sync.Mutex
	calls   [][]string
	stdins  []string
	respond func(argv []string, stdin string) (string, error)
}

func (s *scripted) Run(_ context.Context, stdin string, argv ...string) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, argv)
	s.stdins = append(s.stdins, stdin)
	s.mu.Unlock()

	return s.respond(argv, stdin)
}

func TestProxmoxDiscoverListsRunningContainers(t *testing.T) {
	r := &scripted{respond: func(argv []string, _ string) (string, error) {
		switch strings.Join(argv, " ") {
		case "/usr/sbin/pct list":
			return "VMID       Status     Lock         Name\n105        running                 app-1\n106        stopped                 old\n", nil
		case "/usr/sbin/pct exec 105 -- sh -c " + discoverScript:
			return "app-1\n1: lo    inet 127.0.0.1/8 scope host lo\n2: eth0    inet 192.0.2.10/24 brd 192.0.2.255 scope global eth0\n", nil
		}
		t.Fatalf("unexpected %q", argv)
		return "", nil
	}}

	got, err := Proxmox{R: r}.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Ref != "105" || got[0].Hostname != "app-1" || !reflect.DeepEqual(got[0].IPs, []string{"192.0.2.10"}) {
		t.Fatalf("Discover = %+v", got)
	}
}

func TestProxmoxExecRefusesNonNumericRef(t *testing.T) {
	r := &scripted{respond: func([]string, string) (string, error) { return "", nil }}

	if _, err := (Proxmox{R: r}).Exec(t.Context(), "105; reboot", "", "hostname"); err == nil || len(r.calls) != 0 {
		t.Fatal("ran pct with a non-numeric ref")
	}
}

func TestProxmoxDiscoverUsesOneExecPerContainer(t *testing.T) {
	r := &scripted{respond: func(argv []string, _ string) (string, error) {
		if strings.Join(argv, " ") == "/usr/sbin/pct list" {
			return "VMID Status Lock Name\n105 running x\n106 running y\n", nil
		}
		return "app-" + argv[2] + "\n1: lo inet 127.0.0.1/8 scope host lo\n2: eth0 inet 192.0.2." + argv[2] + "/24 brd x\n", nil
	}}

	got, err := Proxmox{R: r}.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 3 {
		t.Fatalf("calls = %d (%q), want pct list + one exec per container", len(r.calls), r.calls)
	}
	want := []sshsync.Discovered{
		{Ref: "105", Hostname: "app-105", IPs: []string{"192.0.2.105"}},
		{Ref: "106", Hostname: "app-106", IPs: []string{"192.0.2.106"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover = %+v", got)
	}
}

// Hostname and addresses come from the container and go into every report.
func TestProxmoxDiscoverBoundsWhatAContainerClaims(t *testing.T) {
	var many strings.Builder
	for i := range 300 {
		many.WriteString("inet 192.0.2." + strconv.Itoa(i%250) + "/24\n")
	}

	r := &scripted{respond: func(argv []string, _ string) (string, error) {
		if strings.Join(argv, " ") == "/usr/sbin/pct list" {
			return "VMID Status Lock Name\n105 running x\n", nil
		}
		return strings.Repeat("h", 5000) + "\n" + many.String(), nil
	}}

	got, err := Proxmox{R: r}.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Hostname) > maxHostname || len(got[0].IPs) > maxIPs {
		t.Fatalf("hostname %d bytes, %d IPs", len(got[0].Hostname), len(got[0].IPs))
	}
}
