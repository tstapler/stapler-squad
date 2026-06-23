package artifacts

import "encoding/json"

// artifactEntry is a minimal copy of the JSONL envelope fields the scanner needs.
// Deliberately does NOT embed session/tokens types to avoid coupling to that package's
// performance-optimized omissions (jsonlUserContent omits Input to avoid 237KB allocations).
type artifactEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type artifactMessage struct {
	Role    string                 `json:"role"`
	Content []artifactContentBlock `json:"content"`
}

type artifactContentBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // string or []textBlock (tool_result)
	// tool_use fields:
	Name  string          `json:"name"`  // "Bash", "Write", "Edit", etc.
	Input json.RawMessage `json:"input"` // {"command":"...", "description":"..."} for Bash
}

// bashInput holds the Bash tool_use input fields we care about.
type bashInput struct {
	Command string `json:"command"`
}
