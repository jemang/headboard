package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Runner runs a command by argv. Nothing here goes through a shell with
// interpolated values; the only shell scripts are the fixed ones in apply.go,
// which receive their inputs as positional arguments.
type Runner interface {
	Run(ctx context.Context, stdin string, argv ...string) (string, error)
}

// ExecRunner bounds every command: one hung container must not stall
// discovery, and with it every sync on the host. Timeout 0 means 30 s.
type ExecRunner struct {
	Timeout time.Duration
}

func (r ExecRunner) Run(ctx context.Context, stdin string, argv ...string) (string, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// pct is a Perl wrapper around lxc-attach: killing pct alone can leave its
	// child holding stdout, so kill the whole group, and stop waiting for the
	// pipes shortly after.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	out := &capped{max: maxOutput}
	errb := &capped{max: maxStderr}
	cmd.Stdout, cmd.Stderr = out, errb

	err := cmd.Run()
	if out.over {
		return "", fmt.Errorf("%s: output passed %d bytes; refused", argv[0], maxOutput)
	}
	if err != nil {
		return out.String(), fmt.Errorf("%s: %w: %s", argv[0], err, strings.TrimSpace(errb.String()))
	}

	return out.String(), nil
}

// The largest stdout a command may produce: far above any authorized_keys or
// discovery output, far below what four containers at once can do to a
// Proxmox host's memory.
const maxOutput = 1 << 20

// Only the start of stderr is kept, for the warning shown to admins.
const maxStderr = 512

var errTooMuchOutput = errors.New("output limit reached")

// capped keeps up to max bytes. Past that, stdout fails the write so the
// command dies on a broken pipe; stderr is just cut short. The buffer is a
// field, not embedded: an embedded bytes.Buffer brings ReadFrom, and io.Copy
// would use it to read everything past Write.
type capped struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *capped) String() string { return c.buf.String() }

func (c *capped) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	if len(p) <= room {
		return c.buf.Write(p)
	}

	c.buf.Write(p[:max(room, 0)])
	if c.max == maxStderr {
		return len(p), nil
	}

	c.over = true

	return 0, errTooMuchOutput
}
