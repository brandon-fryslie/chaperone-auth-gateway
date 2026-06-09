package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client is a typed control-plane client over the daemon's unix socket. It is the
// counterpart of Server and shares the protocol types, so the two ends cannot
// drift ([LAW:one-source-of-truth]). vf4.4's MCP server is a thin client of this.
//
// If no daemon is listening, every call fails LOUDLY with a clear message — never
// a silent zero value or fallback ([LAW:no-silent-failure]).
type Client struct {
	socketPath string
	http       *http.Client
}

// NewClient builds a client that dials the control socket at socketPath. The HTTP
// host in request URLs is a fixed placeholder; the dialer always connects to the
// socket, ignoring it.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		http: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// Grant activates a pairing. A non-nil error carries the daemon's verbatim
// rejection (e.g. "grant rejected: no approved pairing ...") so the caller can
// surface it without re-deciding.
func (c *Client) Grant(ctx context.Context, req GrantRequest) (GrantResult, error) {
	var res GrantResult
	err := c.do(ctx, http.MethodPost, PathGrant, req, &res)
	return res, err
}

// Revoke removes the active grant for a host. Absent host is a soft success
// (Revoked=false), matching the server's idempotent semantics.
func (c *Client) Revoke(ctx context.Context, req RevokeRequest) (RevokeResult, error) {
	var res RevokeResult
	err := c.do(ctx, http.MethodPost, PathRevoke, req, &res)
	return res, err
}

// List returns the live active set (references only).
func (c *Client) List(ctx context.Context) (ListResult, error) {
	var res ListResult
	err := c.do(ctx, http.MethodGet, PathList, nil, &res)
	return res, err
}

// ListGrantable returns the approved universe the agent may ask from.
func (c *Client) ListGrantable(ctx context.Context) (ListGrantableResult, error) {
	var res ListGrantableResult
	err := c.do(ctx, http.MethodGet, PathListGrantable, nil, &res)
	return res, err
}

// do performs one request/response round-trip. reqBody is JSON-encoded when
// non-nil; a non-2xx response is turned into the daemon's verbatim error.
func (c *Client) do(ctx context.Context, method, path string, reqBody, out any) error {
	var body *bytes.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("control: encode request: %w", err)
		}
		body = bytes.NewReader(buf)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://chaperone"+path, body)
	if err != nil {
		return fmt.Errorf("control: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("control plane not reachable at %s (is the chaperone daemon running?): %w", c.socketPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		var e errorBody
		if decErr := json.NewDecoder(resp.Body).Decode(&e); decErr == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("control: request failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("control: decode response: %w", err)
	}
	return nil
}
