package tokens

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// maxScannerTokenSize is the maximum line size for the bufio.Scanner.
	// 10MB handles the longest possible JSONL lines with large base64-encoded content.
	maxScannerTokenSize = 10 * 1024 * 1024
)

// Parser parses Claude Code JSONL transcript files into ParseResult values.
type Parser struct{}

// NewParser creates a new Parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile reads a JSONL transcript file and returns an aggregated ParseResult.
// Malformed or truncated lines are skipped without returning an error.
// The caller must not retain message content — ParseResult only holds aggregates.
func (p *Parser) ParseFile(filePath string) (*ParseResult, error) {
	f, err := os.Open(filePath) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	result, err := p.ParseReader(f)
	if err != nil {
		return nil, err
	}

	result.FileModTime = stat.ModTime()
	result.ParsedAt = time.Now()

	// Extract the session UUID from the file name.
	base := filepath.Base(filePath)
	if strings.HasSuffix(base, ".jsonl") {
		result.SessionUUID = strings.TrimSuffix(base, ".jsonl")
	}

	// Extract project path from the directory name.
	dir := filepath.Base(filepath.Dir(filePath))
	result.ProjectPath = decodeProjectDirName(dir)

	return result, nil
}

// ParseReader parses JSONL from an io.Reader.
// Suitable for tests that pass in strings via strings.NewReader.
func (p *Parser) ParseReader(r io.Reader) (*ParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxScannerTokenSize), maxScannerTokenSize)

	result := &ParseResult{
		ToolUsage: make(map[string]ToolTokenStats),
	}

	modelCounts := make(map[string]int)
	humanTurnIndex := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines.
			continue
		}

		// The outer envelope uses "type" field: "assistant", "user", etc.
		// Some older entries may use "role" instead.
		entryType := entry.Type
		if entryType == "" {
			entryType = entry.Role
		}

		switch entryType {
		case "assistant":
			p.processAssistantEntry(entry, result, modelCounts)
		case "user":
			p.processUserEntry(entry, result, humanTurnIndex)
			humanTurnIndex++
		}
	}

	// Ignore scanner errors for partial writes (EOF mid-line).
	_ = scanner.Err()

	// Determine primary model (most frequently used).
	result.PrimaryModel = primaryModel(modelCounts)

	// Build sorted deduplicated Models list.
	result.Models = sortedKeys(modelCounts)

	return result, nil
}

// processAssistantEntry extracts token counts and tool usage from an assistant turn.
func (p *Parser) processAssistantEntry(entry jsonlEntry, result *ParseResult, modelCounts map[string]int) {
	if len(entry.Message) == 0 {
		return
	}

	var msg jsonlMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	// Only process entries with role == "assistant" at the message level too.
	if msg.Role != "" && msg.Role != "assistant" {
		return
	}

	turn := TurnStats{
		Model: msg.Model,
	}

	// Parse timestamp from outer entry.
	if entry.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
			turn.Timestamp = t
		} else if t2, err2 := time.Parse(time.RFC3339Nano, entry.Timestamp); err2 == nil {
			turn.Timestamp = t2
		}
	}

	if msg.Usage != nil {
		turn.Input = msg.Usage.InputTokens
		turn.Output = msg.Usage.OutputTokens
		turn.CacheCreation = msg.Usage.CacheCreationInputTokens
		turn.CacheRead = msg.Usage.CacheReadInputTokens

		result.TotalInput += turn.Input
		result.TotalOutput += turn.Output
		result.CacheCreation += turn.CacheCreation
		result.CacheRead += turn.CacheRead
	}

	// Extract tool use names from content.
	for _, c := range msg.Content {
		if c.Type != "tool_use" || c.Name == "" {
			continue
		}
		turn.ToolNames = append(turn.ToolNames, c.Name)

		stat := result.ToolUsage[c.Name]
		stat.ToolName = c.Name
		stat.CallCount++
		// Extract MCP server name: mcp__<server>__<tool>
		if strings.HasPrefix(c.Name, "mcp__") {
			parts := strings.SplitN(c.Name, "__", 3)
			if len(parts) >= 2 {
				stat.MCPServer = parts[1]
			}
		}
		result.ToolUsage[c.Name] = stat
	}

	if msg.Model != "" {
		modelCounts[msg.Model]++
	}

	result.MessageCount++
	result.TurnTimeline = append(result.TurnTimeline, turn)
}

// processUserEntry detects skill activations and /commands in user turns.
func (p *Parser) processUserEntry(entry jsonlEntry, result *ParseResult, turnIndex int) {
	if len(entry.Message) == 0 {
		return
	}

	var msg jsonlUserMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	activations := detectSkillActivations(msg.Content, turnIndex)
	result.SkillActivations = append(result.SkillActivations, activations...)
}

// primaryModel returns the model with the highest count, or empty string.
func primaryModel(counts map[string]int) string {
	best := ""
	bestCount := 0
	for model, count := range counts {
		if count > bestCount || (count == bestCount && model > best) {
			best = model
			bestCount = count
		}
	}
	return best
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// decodeProjectDirName reverse-engineers the project directory name back to a path.
// Claude encodes paths by replacing every non-alphanumeric char with '-'.
// This is a best-effort decode: '-' maps to '/' as a first guess for leading '-'.
// Example: "-Users-alice-myproject" → "/Users/alice/myproject"
func decodeProjectDirName(dirName string) string {
	if !strings.HasPrefix(dirName, "-") {
		return dirName
	}
	// Replace '-' with '/' for path separators.
	// This loses information (original '-' in path names are gone) but is good enough.
	return "/" + strings.ReplaceAll(dirName[1:], "-", "/")
}
