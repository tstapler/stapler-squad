package hookipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const defaultClientTimeout = 100 * time.Millisecond

type Client struct {
	endpoint HookEndpoint
	client   *http.Client
}

type ClientOption func(*Client) error

func WithClientTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) error {
		if timeout <= 0 {
			return errors.New("hookipc: client timeout must be positive")
		}
		c.client.Timeout = timeout
		return nil
	}
}

func NewClient(endpoint HookEndpoint, options ...ClientOption) (*Client, error) {
	if endpoint.SocketPath == "" || endpoint.InstanceFingerprint == "" || endpoint.ProtocolVersion == 0 {
		return nil, errors.New("hookipc: complete endpoint is required")
	}
	dialer := &net.Dialer{Timeout: defaultClientTimeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", endpoint.SocketPath)
		},
		DisableCompression:  true,
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &Client{
		endpoint: endpoint,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultClientTimeout,
		},
	}
	for _, option := range options {
		if err := option(client); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) Classify(ctx context.Context, envelope ClassificationEnvelope) (ClassificationReply, error) {
	if err := envelope.Validate(c.endpoint); err != nil {
		return ClassificationReply{}, err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return ClassificationReply{}, fmt.Errorf("hookipc: encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+classifyPath, bytes.NewReader(body))
	if err != nil {
		return ClassificationReply{}, fmt.Errorf("hookipc: create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return ClassificationReply{}, fmt.Errorf("hookipc: classify request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return ClassificationReply{}, fmt.Errorf("hookipc: classify response status %d", response.StatusCode)
	}
	var reply ClassificationReply
	decoder := json.NewDecoder(io.LimitReader(response.Body, defaultMaxBodySize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reply); err != nil {
		return ClassificationReply{}, fmt.Errorf("hookipc: decode response: %w", err)
	}
	if err := reply.Validate(c.endpoint, envelope.RequestID); err != nil {
		return ClassificationReply{}, err
	}
	return reply, nil
}
