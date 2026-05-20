package tokens

import "encoding/json"

// jsonlEntry is the outer envelope for each line in a Claude JSONL file.
// Only fields relevant to token parsing are extracted.
type jsonlEntry struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	UUID      string          `json:"uuid"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// jsonlMessage is the "message" field of an assistant entry.
type jsonlMessage struct {
	ID         string           `json:"id"`
	Role       string           `json:"role"`
	Model      string           `json:"model"`
	Content    []jsonlContent   `json:"content"`
	Usage      *jsonlUsage      `json:"usage"`
	StopReason string           `json:"stop_reason"`
}

// jsonlUsage contains the token counts for a message.
type jsonlUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// jsonlContent is one element in the content array.
type jsonlContent struct {
	Type string `json:"type"`
	// For tool_use blocks
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// For text blocks
	Text string `json:"text"`
	// For tool_result blocks
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// jsonlUserMessage is the "message" field of a user entry.
type jsonlUserMessage struct {
	Role    string         `json:"role"`
	Content []jsonlContent `json:"content"`
}

