package session

import (
	"encoding/json"
	"regexp"
)

// Default secret patterns for the inhibition engine.
var defaultSecretPatterns = []*regexp.Regexp{
	// OpenAI / Anthropic / Generic API Keys
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),
	// AWS Access Key ID
	regexp.MustCompile(`(AKIA[0-9A-Z]{16})`),
	// Bearer tokens
	regexp.MustCompile(`(?i)(Bearer\s+[a-zA-Z0-9._\-]+)`),
	// Generic secret/password/token assignments (e.g., password=foo, token: "bar")
	regexp.MustCompile(`(?i)(api_key|apikey|secret|password|passwd|auth_token|access_token)\s*[:=]\s*["']?([^\s"']+)["']?`),
}

const redactedPlaceholder = "[REDACTED_CREDENTIAL]"

// InhibitionEngine filters and redacts sensitive data from canonical turns.
type InhibitionEngine struct {
	patterns []*regexp.Regexp
}

// NewInhibitionEngine creates a new InhibitionEngine with default security patterns.
func NewInhibitionEngine() *InhibitionEngine {
	return &InhibitionEngine{
		patterns: defaultSecretPatterns,
	}
}

// SanitizeString redacts any sensitive credentials or secrets found in text.
func (ie *InhibitionEngine) SanitizeString(s string) string {
	if s == "" {
		return ""
	}
	result := s
	for _, pattern := range ie.patterns {
		result = pattern.ReplaceAllString(result, redactedPlaceholder)
	}
	return result
}

// SanitizeTurn applies inhibition rules to all blocks within a CanonicalTurn.
func (ie *InhibitionEngine) SanitizeTurn(turn CanonicalTurn) CanonicalTurn {
	sanitizedBlocks := make([]CanonicalBlock, 0, len(turn.Blocks))
	for _, block := range turn.Blocks {
		sanitizedBlock := block

		switch block.Kind {
		case BlockKindText, BlockKindThinking:
			sanitizedBlock.Text = ie.SanitizeString(block.Text)

		case BlockKindToolResult:
			sanitizedBlock.ToolResultContent = ie.SanitizeString(block.ToolResultContent)

		case BlockKindToolUse:
			if len(block.ToolArgs) > 0 {
				var rawArgs map[string]interface{}
				if err := json.Unmarshal(block.ToolArgs, &rawArgs); err == nil {
					ie.sanitizeMap(rawArgs)
					if reencoded, err := json.Marshal(rawArgs); err == nil {
						sanitizedBlock.ToolArgs = reencoded
					}
				}
			}
		}

		sanitizedBlocks = append(sanitizedBlocks, sanitizedBlock)
	}

	turn.Blocks = sanitizedBlocks
	return turn
}

// SanitizeTurns applies inhibition rules to a slice of CanonicalTurns.
func (ie *InhibitionEngine) SanitizeTurns(turns []CanonicalTurn) []CanonicalTurn {
	sanitized := make([]CanonicalTurn, len(turns))
	for i, t := range turns {
		sanitized[i] = ie.SanitizeTurn(t)
	}
	return sanitized
}

// sanitizeMap recursively redacts string values within a nested JSON map.
func (ie *InhibitionEngine) sanitizeMap(m map[string]interface{}) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			m[k] = ie.SanitizeString(val)
		case map[string]interface{}:
			ie.sanitizeMap(val)
		case []interface{}:
			ie.sanitizeSlice(val)
		}
	}
}

// sanitizeSlice recursively redacts string values within a JSON slice.
func (ie *InhibitionEngine) sanitizeSlice(s []interface{}) {
	for i, v := range s {
		switch val := v.(type) {
		case string:
			s[i] = ie.SanitizeString(val)
		case map[string]interface{}:
			ie.sanitizeMap(val)
		case []interface{}:
			ie.sanitizeSlice(val)
		}
	}
}
