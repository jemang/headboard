package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// maxBinary bounds a download so a runaway answer cannot fill the disk the
// agent lives on. The agent itself is about 9 MB.
var maxBinary int64 = 200 << 20

// Update replaces the binary at exe with the one Headboard serves for arch.
// check runs the downloaded file before it is swapped in, so a truncated or
// wrong download never replaces a working agent. changed is false when the
// served binary is the one already installed.
func Update(ctx context.Context, c *Client, arch, exe string, check func(path string) error) (changed bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/ssh/agent/bin/"+arch, nil)
	if err != nil {
		return false, err
	}

	res, err := c.do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download: %s", res.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".headboard-agent-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())

	n, err := io.Copy(tmp, io.LimitReader(res.Body, maxBinary+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBinary {
		err = fmt.Errorf("binary is larger than %d bytes", maxBinary)
	}
	if err != nil {
		return false, fmt.Errorf("download: %w", err)
	}

	same, err := sameFile(exe, tmp.Name())
	if err != nil || same {
		return false, err
	}

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return false, err
	}
	if err := check(tmp.Name()); err != nil {
		return false, fmt.Errorf("downloaded agent failed its check: %w", err)
	}

	// Same directory, so the rename is atomic: the old binary is replaced
	// whole or not at all.
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return false, err
	}

	return true, nil
}

func sameFile(a, b string) (bool, error) {
	x, err := os.ReadFile(a)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	y, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}

	return bytes.Equal(x, y), nil
}
