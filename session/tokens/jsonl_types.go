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
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []jsonlContent `json:"content"`
	Usage      *jsonlUsage    `json:"usage"`
	StopReason string         `json:"stop_reason"`
}

// jsonlUsage contains the token counts for a message.
type jsonlUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// jsonlContent is one element in the content array of an assistant message.
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

// jsonlUserContent is a minimal content element for user message parsing.
// It intentionally omits the Input field (present on tool_use blocks in assistant
// messages) to avoid allocating the large tool-input JSON payload — the source of
// the ~237 KB per-call allocation reported in PerfFix-4. User entries never
// contain tool_use blocks, so Input is always absent and safe to skip.
type jsonlUserContent struct {
	Type string `json:"type"`
	// For text blocks
	Text string `json:"text"`
	// For tool_result blocks
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// jsonlUserMessage is the "message" field of a user entry.
// It uses jsonlUserContent instead of jsonlContent to avoid allocating
// the large Input json.RawMessage field that appears only in assistant messages.
type jsonlUserMessage struct {
	Role    string             `json:"role"`
	Content []jsonlUserContent `json:"content"`
}
