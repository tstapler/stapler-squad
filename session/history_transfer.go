package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ConversationID represents a validated Claude/Antigravity conversation UUID.
type ConversationID string

// ParseConversationID parses and validates a raw string as a ConversationID.
func ParseConversationID(s string) (ConversationID, error) {
	if !isValidUUID(s) {
		return "", fmt.Errorf("invalid conversation ID format: %q", s)
	}
	return ConversationID(s), nil
}

// WorkspacePath represents a cleaned, resolved workspace root path.
type WorkspacePath string

// NewWorkspacePath resolves symlinks and cleans a path to guarantee a single canonical representation.
func NewWorkspacePath(s string) (WorkspacePath, error) {
	if s == "" {
		return "", fmt.Errorf("workspace path cannot be empty")
	}
	resolved, err := filepath.EvalSymlinks(s)
	if err != nil {
		resolved = s
	}
	return WorkspacePath(filepath.Clean(resolved)), nil
}

// PortSessionHistory translates and syncs history between Claude Code and Antigravity CLI.
func PortSessionHistory(ctx context.Context, oldProgram, newProgram string, i *Instance) error {
	var srcAdapter, dstAdapter HistoryAdapter

	claude := NewClaudeAdapter()
	agy := NewAgyAdapter()

	if claude.CanHandle(oldProgram) {
		srcAdapter = claude
	} else if agy.CanHandle(oldProgram) {
		srcAdapter = agy
	}

	if claude.CanHandle(newProgram) {
		dstAdapter = claude
	} else if agy.CanHandle(newProgram) {
		dstAdapter = agy
	}

	if srcAdapter == nil || dstAdapter == nil || srcAdapter.Name() == dstAdapter.Name() {
		return nil
	}

	// 1. Import turns to canonical format from source adapter
	turns, err := srcAdapter.Import(ctx, i)
	if err != nil {
		return fmt.Errorf("failed to import session history from %s: %w", srcAdapter.Name(), err)
	}

	// 2. Export turns in target adapter format
	if err := dstAdapter.Export(ctx, turns, i); err != nil {
		return fmt.Errorf("failed to export session history to %s: %w", dstAdapter.Name(), err)
	}

	// 3. Perform post-switch steps like history.jsonl mapping
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	uuidStr := i.GetClaudeConversationUUID()
	workspace := i.GetWorkingDirectory()

	if dstAdapter.Name() == "agy" {
		// Append history.jsonl for Antigravity
		agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
		if err := os.MkdirAll(filepath.Dir(agyHistoryPath), 0700); err == nil {
			historyEntry := map[string]interface{}{
				"display":        fmt.Sprintf("Ported from Claude: %s", i.Title),
				"timestamp":      time.Now().UnixMilli(),
				"workspace":      workspace,
				"conversationId": uuidStr,
			}
			historyData, _ := json.Marshal(historyEntry)
			if f, err := os.OpenFile(agyHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.Write(historyData)
				f.Write([]byte("\n"))
				f.Close()
			}
		}
	} else if dstAdapter.Name() == "claude" {
		// Append history.jsonl for Claude
		claudeHistoryPath := filepath.Join(home, ".claude", "history.jsonl")
		if err := os.MkdirAll(filepath.Dir(claudeHistoryPath), 0700); err == nil {
			historyEntry := map[string]interface{}{
				"display":        fmt.Sprintf("Ported from Antigravity: %s", i.Title),
				"pastedContents": map[string]interface{}{},
				"timestamp":      time.Now().UnixMilli(),
				"project":        workspace,
				"sessionId":      uuidStr,
			}
			historyData, _ := json.Marshal(historyEntry)
			if hf, err := os.OpenFile(claudeHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				hf.Write(historyData)
				hf.Write([]byte("\n"))
				hf.Close()
			}
		}

		// Update Claude session data on the Instance
		i.stateMutex.Lock()
		if i.claudeSession == nil {
			i.claudeSession = &ClaudeSessionData{}
		}
		i.claudeSession.ConversationUUID = uuidStr
		i.claudeSession.ProjectName = i.Title
		i.claudeSession.LastAttached = time.Now()
		if i.claudeSession.Metadata == nil {
			i.claudeSession.Metadata = make(map[string]string)
		}
		i.claudeSession.Metadata["working_dir"] = workspace
		cb := i.claudeSessionIDSavedCallback
		i.stateMutex.Unlock()

		if cb != nil {
			cb()
		}
	}

	log.Info("PortSessionHistory: ported session history using canonical formats", "from", srcAdapter.Name(), "to", dstAdapter.Name(), "session", i.Title)
	return nil
}

// Shim functions to keep backward compatibility with existing tests and server code.
func portClaudeToAgy(i *Instance) error {
	return PortSessionHistory(context.Background(), "claude", "agy", i)
}

func portAgyToClaude(i *Instance) error {
	return PortSessionHistory(context.Background(), "agy", "claude", i)
}

type UnifiedTurn interface {
	unifiedTurn()
}

type SkippedTurn struct{}
func (SkippedTurn) unifiedTurn() {}

type shimUserMessage struct{}
func (shimUserMessage) unifiedTurn() {}

type shimAssistantMessage struct{}
func (shimAssistantMessage) unifiedTurn() {}

func ParseClaudeTurn(line []byte) (UnifiedTurn, error) {
	var raw struct {
		Type    string `json:"type"`
		Message *struct {
			Role string `json:"role"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}
	if raw.Message == nil {
		return SkippedTurn{}, nil
	}
	if raw.Type == "user" {
		return shimUserMessage{}, nil
	}
	if raw.Type == "assistant" {
		return shimAssistantMessage{}, nil
	}
	return SkippedTurn{}, nil
}
