package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// DefaultTimeout bounds each request, including reading the body.
	DefaultTimeout = 2 * time.Second
	// DefaultMaxResponseBytes caps a response body; 200 events of 2000-char
	// bodies fit comfortably.
	DefaultMaxResponseBytes = 8 << 20
	// maxErrorMessage truncates server-supplied error text kept in errors.
	maxErrorMessage = 200
)

// Config configures a Client. Only URL and ConsumerID are required.
type Config struct {
	URL        string
	TokenPath  string // empty means DefaultTokenPath
	ConsumerID string
	Timeout    time.Duration // per request; zero means DefaultTimeout
	MaxBytes   int64         // response cap; zero means DefaultMaxResponseBytes
	Resolver   Resolver      // nil means the system resolver; tests inject one
}

// Client talks to one loopback inbox as one consumer. It is safe for
// concurrent use.
type Client struct {
	base       *url.URL
	tokenPath  string
	consumerID string
	timeout    time.Duration
	maxBytes   int64
	http       *http.Client
}

// NewClient refuses non-loopback URLs before anything touches the network or
// the token file.
func NewClient(cfg Config) (*Client, error) {
	base, err := ParseLoopbackURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	c := &Client{
		base:       base,
		tokenPath:  cfg.TokenPath,
		consumerID: cfg.ConsumerID,
		timeout:    cfg.Timeout,
		maxBytes:   cfg.MaxBytes,
	}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	if c.maxBytes <= 0 {
		c.maxBytes = DefaultMaxResponseBytes
	}
	c.http = &http.Client{
		Transport: &http.Transport{
			// No Proxy: the token must not detour through HTTP(S)_PROXY.
			DialContext: LoopbackDialer(cfg.Resolver, nil),
		},
		// A redirect could carry the token elsewhere; hand back the 3xx instead.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c, nil
}

// Watch subscribes and asks the inbox to ensure webhook coverage. An inbox
// that predates /v1/watch yields ErrWatchUnsupported.
func (c *Client) Watch(ctx context.Context, repo string, pr int) (WatchResult, error) {
	var out WatchResult
	err := c.do(ctx, http.MethodPost, "/v1/watch", nil, c.subscribeBody(repo, pr), true, &out)
	return out, err
}

// Subscribe registers interest in one PR without touching webhooks.
func (c *Client) Subscribe(ctx context.Context, repo string, pr int) (Subscription, error) {
	var out struct {
		Subscription Subscription `json:"subscription"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/subscriptions", nil, c.subscribeBody(repo, pr), true, &out)
	return out.Subscription, err
}

// Subscriptions lists this consumer's active subscriptions.
func (c *Client) Subscriptions(ctx context.Context) ([]Subscription, error) {
	var out struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	q := url.Values{"consumer_id": {c.consumerID}}
	err := c.do(ctx, http.MethodGet, "/v1/subscriptions", q, nil, true, &out)
	return out.Subscriptions, err
}

// Events returns pending items, oldest first. Reads are non-destructive.
func (c *Client) Events(ctx context.Context, limit int) ([]Item, error) {
	var out struct {
		Items []Item `json:"items"`
	}
	q := url.Values{"consumer_id": {c.consumerID}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	err := c.do(ctx, http.MethodGet, "/v1/events", q, nil, true, &out)
	return out.Items, err
}

// Ack takes the inbox item ID (Item.ID), not the event ID. It is idempotent.
func (c *Client) Ack(ctx context.Context, itemID string) error {
	body := map[string]string{"consumer_id": c.consumerID}
	return c.do(ctx, http.MethodPost, "/v1/events/"+url.PathEscape(itemID)+"/ack", nil, body, true, nil)
}

// Unsubscribe takes the subscription ID and reports whether it was still active.
func (c *Client) Unsubscribe(ctx context.Context, subscriptionID string) (bool, error) {
	var out struct {
		Removed bool `json:"removed"`
	}
	body := map[string]string{"consumer_id": c.consumerID}
	err := c.do(ctx, http.MethodDelete, "/v1/subscriptions/"+url.PathEscape(subscriptionID), nil, body, true, &out)
	return out.Removed, err
}

// Health is unauthenticated, so it never reads the token file.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.do(ctx, http.MethodGet, "/v1/health", nil, nil, false, &out)
	return out, err
}

func (c *Client) subscribeBody(repo string, pr int) map[string]any {
	return map[string]any{"consumer_id": c.consumerID, "repository": repo, "pull_request": pr}
}

// do runs one request. The token is re-read here, so each call picks up a
// rotated file, and it goes only into the Authorization header.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, auth bool, out any) error {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%w: %s %s: %v", ErrDecode, method, path, err)
		}
		payload = bytes.NewReader(raw)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	u := *c.base
	u.Path = path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), payload)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %v", ErrTransport, method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		token, err := ReadToken(c.tokenPath)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error text holds the request URL, which never contains the token.
		return fmt.Errorf("%w: %s %s: %v", ErrTransport, method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %s %s: reading body: %v", ErrTransport, method, path, err)
	}
	if int64(len(data)) > c.maxBytes {
		return fmt.Errorf("%w: %s %s: response exceeds %d bytes", ErrDecode, method, path, c.maxBytes)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(method, path, resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: %s %s: %v", ErrDecode, method, path, err)
	}
	return nil
}

func statusError(method, path string, status int, data []byte) error {
	var kind error
	switch {
	case status == http.StatusUnauthorized:
		kind = ErrUnauthorized
	case status >= 500:
		kind = Err5xx
	case status >= 300 && status < 400:
		kind = ErrTransport
	case status == http.StatusNotFound && path == "/v1/watch":
		kind = ErrWatchUnsupported
	default:
		kind = ErrRejected
	}
	msg := serverMessage(data)
	if msg == "" {
		return fmt.Errorf("%w: %s %s: status %d", kind, method, path, status)
	}
	return fmt.Errorf("%w: %s %s: status %d: %s", kind, method, path, status, msg)
}

// serverMessage extracts error.message from the documented error shape; any
// other body (a proxy page) is dropped rather than echoed.
func serverMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	msg := e.Error.Message
	if len(msg) > maxErrorMessage {
		msg = msg[:maxErrorMessage]
	}
	return msg
}
