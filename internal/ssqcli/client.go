package ssqcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 64 << 20

// Client posts Connect-protocol JSON unary requests.
type Client struct {
	BaseURL string
	Token   string
	Timeout time.Duration
	HTTP    *http.Client
}

// Call POSTs body to <BaseURL>/api<procedure> and returns the raw JSON reply.
func (c *Client) Call(ctx context.Context, procedure string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	url := strings.TrimRight(c.BaseURL, "/") + "/api" + procedure
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Connect-Timeout-Ms", strconv.FormatInt(c.Timeout.Milliseconds(), 10))
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		// Connect RPCs never redirect; following one could replay the token and body elsewhere.
		hc = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out after %s (server hung or overloaded; raise --timeout to wait longer)", procedure, c.Timeout)
		}
		return nil, fmt.Errorf("cannot reach %s: %w (is stapler-squad running? set --url or STAPLER_SQUAD_URL)", c.BaseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out after %s while reading the response", procedure, c.Timeout)
		}
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("%s response exceeds %d MiB", procedure, maxResponseBytes>>20)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeError(procedure, resp.StatusCode, data)
	}
	return data, nil
}

func decodeError(procedure string, status int, data []byte) error {
	const authHint = " (set --token, STAPLER_SQUAD_TOKEN, or run on the host that owns the local token file)"
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil && e.Code != "" {
		hint := ""
		if e.Code == "unauthenticated" || status == http.StatusUnauthorized {
			hint = authHint
		}
		return fmt.Errorf("%s failed: %s: %s%s", procedure, e.Code, e.Message, hint)
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("%s failed: HTTP 401 unauthorized%s", procedure, authHint)
	}
	return fmt.Errorf("%s failed: HTTP %d: %s", procedure, status, strings.TrimSpace(string(data)))
}
