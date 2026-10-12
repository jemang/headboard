package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/jemang/headboard/internal/sshsync"
)

const pct = "/usr/sbin/pct"

var vmid = regexp.MustCompile(`^[0-9]{1,9}$`)

// Proxmox reaches each running container through pct, which enters the
// container's namespaces, so paths resolve inside the guest.
type Proxmox struct{ R Runner }

func (Proxmox) Mode() string { return "proxmox" }

func (p Proxmox) Exec(ctx context.Context, ref, stdin string, argv ...string) (string, error) {
	if !vmid.MatchString(ref) {
		return "", errors.New("container id must be numeric")
	}

	return p.R.Run(ctx, stdin, append([]string{pct, "exec", ref, "--"}, argv...)...)
}

// discoverScript prints the hostname, then the IPv4 lines, so one pct exec
// per container answers both: each pct call starts a Perl process.
const discoverScript = `hostname; ip -4 -o addr show`

// discoverParallel bounds how many containers are inspected at once.
const discoverParallel = 4

func (p Proxmox) Discover(ctx context.Context) ([]sshsync.Discovered, error) {
	out, err := p.R.Run(ctx, "", pct, "list")
	if err != nil {
		return nil, err
	}

	var refs []string
	for _, line := range strings.Split(out, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "running" && vmid.MatchString(f[0]) {
			refs = append(refs, f[0])
		}
	}

	found := make([]sshsync.Discovered, len(refs))
	sem := make(chan struct{}, discoverParallel)
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			d := sshsync.Discovered{Ref: ref}
			if out, err := p.Exec(ctx, ref, "", "sh", "-c", discoverScript); err == nil {
				host, rest, _ := strings.Cut(out, "\n")
				d.Hostname = truncate(strings.TrimSpace(host), maxHostname)
				d.IPs = parseIPv4s(rest)
				if len(d.IPs) > maxIPs {
					d.IPs = d.IPs[:maxIPs]
				}
			}
			found[i] = d
		}()
	}

	wg.Wait()

	return found, nil
}

// What a container may claim about itself. Both values come from inside it
// and go into every report the agent sends.
const (
	maxHostname = 253
	maxIPs      = 32
)

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}

	return s
}

var inet = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)

func parseIPv4s(out string) []string {
	var ips []string

	for _, m := range inet.FindAllStringSubmatch(out, -1) {
		if !strings.HasPrefix(m[1], "127.") {
			ips = append(ips, m[1])
		}
	}

	return ips
}

// Local manages only the machine it runs on.
type Local struct {
	R        Runner
	Addrs    func() ([]net.Addr, error)
	Hostname func() (string, error)
}

func (Local) Mode() string { return "local" }

func (l Local) Exec(ctx context.Context, ref, stdin string, argv ...string) (string, error) {
	if ref != "self" {
		return "", errors.New("a local agent only manages itself")
	}

	return l.R.Run(ctx, stdin, argv...)
}

func (l Local) Discover(context.Context) ([]sshsync.Discovered, error) {
	d := sshsync.Discovered{Ref: "self"}
	d.Hostname, _ = l.Hostname()

	addrs, err := l.Addrs()
	if err != nil {
		return nil, err
	}

	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
			d.IPs = append(d.IPs, n.IP.String())
		}
	}

	return []sshsync.Discovered{d}, nil
}

func DetectHost(r Runner) Host {
	if _, err := os.Stat(pct); err == nil {
		return Proxmox{R: r}
	}

	return Local{R: r, Addrs: net.InterfaceAddrs, Hostname: os.Hostname}
}
