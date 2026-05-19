package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	anthropicAPIURL = "https://api.anthropic.com/v1/messages"
	anthropicModel  = "claude-haiku-4-5-20251001"
	anthropicVersion = "2023-06-01"
)

// AnthropicAIClient implements AIClient using the Anthropic Messages API.
type AnthropicAIClient struct {
	apiKey string
	client *http.Client
	model  string
}

// NewAnthropicAIClient creates an AnthropicAIClient. Returns an error if apiKey is empty.
func NewAnthropicAIClient(apiKey string) (*AnthropicAIClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("AnthropicAIClient: apiKey must not be empty")
	}
	return &AnthropicAIClient{
		apiKey: apiKey,
		model:  anthropicModel,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// anthropicRequest is the JSON body sent to the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the minimal JSON shape returned by the Anthropic Messages API.
type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete sends systemPrompt and userPrompt to the Anthropic API and returns the response text.
// ctx cancellation aborts the outbound HTTP request.
func (c *AnthropicAIClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 2048,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("anthropic: decode response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		if apiResp.Error != nil {
			msg = fmt.Sprintf("status %d: %s: %s", resp.StatusCode, apiResp.Error.Type, apiResp.Error.Message)
		}
		return "", fmt.Errorf("anthropic: API error: %s", msg)
	}

	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty content in response")
	}
	return apiResp.Content[0].Text, nil
}
