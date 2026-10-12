package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/jemang/headboard/internal/sshsync"
)

// Retirement is what uninstall learned on the way out.
type Retirement struct {
	Retired   bool
	Cleared   []sshsync.Pair
	Remaining []sshsync.Pair
	Warnings  []sshsync.Warning
}

// Retire removes every block this agent wrote, then asks Headboard to delete
// it. Headboard refuses while anything it wrote is left, so a stopped
// container or a failed clear keeps the agent, its token and the record of
// those keys. Keys added by hand outside the block are never touched.
func Retire(ctx context.Context, c *Client, h Host) (Retirement, error) {
	wrote, err := c.managedPairs(ctx)
	if err != nil {
		return Retirement{}, err
	}

	discovered, err := h.Discover(ctx)
	if err != nil {
		return Retirement{}, err
	}

	byRef := map[string][]string{}
	for _, p := range wrote {
		byRef[p.Ref] = append(byRef[p.Ref], p.UnixUser)
	}

	targets := make([]sshsync.Target, 0, len(byRef))
	for ref, users := range byRef {
		sort.Strings(users)
		targets = append(targets, sshsync.Target{Ref: ref, Users: map[string][]string{}, Clear: users})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Ref < targets[j].Ref })

	r := Apply(ctx, h, discovered, targets)

	out := Retirement{Cleared: r.Cleared, Warnings: r.Warnings}
	out.Retired, out.Remaining, err = c.retire(ctx, r.Cleared)

	return out, err
}

func (c *Client) managedPairs(ctx context.Context) ([]sshsync.Pair, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/ssh/agent/managed", nil)
	if err != nil {
		return nil, err
	}

	var body struct {
		Managed []sshsync.Pair `json:"managed"`
	}
	if err := c.call(req, &body); err != nil {
		return nil, fmt.Errorf("managed: %w", err)
	}

	return body.Managed, nil
}

func (c *Client) retire(ctx context.Context, cleared []sshsync.Pair) (bool, []sshsync.Pair, error) {
	raw, err := json.Marshal(map[string]any{"cleared": cleared})
	if err != nil {
		return false, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/ssh/agent/retire", bytes.NewReader(raw))
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var body struct {
		Retired   bool           `json:"retired"`
		Remaining []sshsync.Pair `json:"remaining"`
	}
	if err := c.call(req, &body); err != nil {
		return false, nil, fmt.Errorf("retire: %w", err)
	}

	return body.Retired, body.Remaining, nil
}

func (c *Client) call(req *http.Request, into any) error {
	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%s", res.Status)
	}

	return decodeLimited(res.Body, into)
}
