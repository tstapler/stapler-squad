package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
)

type GeminiAdapter struct{}

func NewGeminiAdapter() *GeminiAdapter {
	return &GeminiAdapter{}
}

func (a *GeminiAdapter) Name() string {
	return "gemini"
}

func (a *GeminiAdapter) CanHandle(program string) bool {
	p := strings.ToLower(program)
	// Match standalone gemini CLI, excluding agy/antigravity (handled by AgyAdapter)
	return strings.Contains(p, "gemini") && !strings.Contains(p, "agy") && !strings.Contains(p, "antigravity")
}

type rawGeminiMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

func (a *GeminiAdapter) Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error) {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		inst.tryExtractConversationUUID()
		uuidStr = inst.GetClaudeConversationUUID()
	}
	if uuidStr == "" {
		return nil, fmt.Errorf("no gemini conversation UUID found")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	geminiHistoryPath := filepath.Join(home, ".gemini", "cli", "history", uuidStr+".jsonl")
	if _, err := os.Stat(geminiHistoryPath); os.IsNotExist(err) {
		geminiHistoryPath = filepath.Join(home, ".gemini", "history", uuidStr+".jsonl")
		if _, err := os.Stat(geminiHistoryPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("gemini history file not found for UUID %s", uuidStr)
		}
	}

	file, err := os.Open(geminiHistoryPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var turns []CanonicalTurn
	reader := bufio.NewReader(file)
	turnIdx := 0

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read Gemini history: %w", err)
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr != "" {
			var raw rawGeminiMessage
			if jsonErr := json.Unmarshal([]byte(lineStr), &raw); jsonErr == nil {
				role := RoleUser
				if raw.Role == "model" || raw.Role == "assistant" {
					role = RoleAssistant
				}

				tTime, parseErr := time.Parse(time.RFC3339, raw.Timestamp)
				if parseErr != nil {
					tTime = time.Now()
				}

				turn := CanonicalTurn{
					Role: role,
					Blocks: []CanonicalBlock{
						NewTextBlock(raw.Content),
					},
					Timestamp: tTime,
					TurnIndex: turnIdx,
				}
				if valErr := turn.Validate(); valErr == nil {
					turns = append(turns, turn)
					turnIdx++
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	return turns, nil
}

func (a *GeminiAdapter) Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		uuidStr = uuid.New().String()
	}

	inst.claudeSessionMu.Lock()
	if inst.claudeSession == nil {
		inst.claudeSession = &ClaudeSessionData{}
	}
	inst.claudeSession.ConversationUUID = uuidStr
	inst.claudeSessionMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	geminiHistoryDir := filepath.Join(home, ".gemini", "cli", "history")
	if err := os.MkdirAll(geminiHistoryDir, 0700); err != nil {
		return fmt.Errorf("failed to create gemini history dir: %w", err)
	}

	historyFilePath := filepath.Join(geminiHistoryDir, uuidStr+".jsonl")
	file, err := os.Create(historyFilePath)
	if err != nil {
		return fmt.Errorf("failed to create gemini history file: %w", err)
	}
	defer file.Close()

	for _, turn := range turns {
		roleStr := "user"
		if turn.Role == RoleAssistant {
			roleStr = "model"
		}

		var textParts []string
		for _, b := range turn.Blocks {
			if b.Kind == BlockKindText {
				textParts = append(textParts, b.Text)
			}
		}

		msg := rawGeminiMessage{
			Role:      roleStr,
			Content:   strings.Join(textParts, "\n"),
			Timestamp: turn.Timestamp.Format(time.RFC3339),
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal gemini message: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("failed to write gemini message: %w", err)
		}
		if _, err := file.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write gemini newline: %w", err)
		}
	}

	log.Info("GeminiAdapter: exported session history", "session", inst.Title, "uuid", uuidStr, "turns", len(turns))
	return nil
}
