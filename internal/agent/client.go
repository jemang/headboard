package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jemang/headboard/internal/sshsync"
)

// ErrUnauthorized means the token was revoked or rotated. The agent stops;
// keys already written stay, because revoking an agent is not revoking access.
var ErrUnauthorized = errors.New("agent token rejected")

type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(base, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	c := *hc
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("refusing a redirect to a non-https URL")
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}

		return nil
	}

	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &c}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	return c.http.Do(req)
}

// Sync asks for the desired state. changed is false on 304, in which case
// resp is empty and newETag equals etag.
func (c *Client) Sync(ctx context.Context, etag string) (resp sshsync.SyncResponse, newETag string, changed bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/ssh/agent/sync", nil)
	if err != nil {
		return resp, etag, false, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	res, err := c.do(req)
	if err != nil {
		return resp, etag, false, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusNotModified:
		return resp, etag, false, nil
	case http.StatusUnauthorized:
		return resp, etag, false, ErrUnauthorized
	case http.StatusOK:
	default:
		return resp, etag, false, fmt.Errorf("sync: %s", res.Status)
	}

	if err := decodeLimited(res.Body, &resp); err != nil {
		return resp, etag, false, fmt.Errorf("sync: %w", err)
	}

	if resp.Protocol != sshsync.Protocol {
		return sshsync.SyncResponse{}, etag, false,
			fmt.Errorf("Headboard speaks protocol %d, this agent %d; reinstall the agent", resp.Protocol, sshsync.Protocol)
	}

	return resp, res.Header.Get("ETag"), true, nil
}

func (c *Client) Report(ctx context.Context, r sshsync.Report) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/ssh/agent/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.StatusCode >= 300:
		return fmt.Errorf("report: %s", res.Status)
	}

	return nil
}

// maxJSON bounds what the agent reads from Headboard's JSON answers. A real
// sync is a few kilobytes per server.
var maxJSON int64 = 16 << 20

// decodeLimited refuses a body past maxJSON instead of decoding a truncated
// prefix of it.
func decodeLimited(r io.Reader, into any) error {
	body, err := io.ReadAll(io.LimitReader(r, maxJSON+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxJSON {
		return fmt.Errorf("answer is larger than %d bytes", maxJSON)
	}

	return json.Unmarshal(body, into)
}
