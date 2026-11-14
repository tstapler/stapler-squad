package session

import (
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
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

type PiAdapter struct{}

func NewPiAdapter() *PiAdapter {
	return &PiAdapter{}
}

func (a *PiAdapter) Name() string {
	return "pi"
}

func (a *PiAdapter) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "pi")
}

func (a *PiAdapter) Import(ctx context.Context, inst *instance.Instance) ([]instance.CanonicalTurn, error) {
	sessionID := inst.PiSessionID()

	if sessionID == "" {
		sessionID = inst.GetClaudeConversationUUID()
	}
	if sessionID == "" {
		return nil, fmt.Errorf("no pi session ID found")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	piSessionPath := filepath.Join(home, ".pi", "sessions", sessionID+".jsonl")
	file, err := os.Open(piSessionPath)
	if err != nil {
		return nil, fmt.Errorf("pi session file not found at %s: %w", piSessionPath, err)
	}
	defer file.Close()

	reader := instance.NewPiEventReader(file)
	var turns []instance.CanonicalTurn
	turnIdx := 0

	for {
		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if toolEv, ok := event.(instance.PiToolExecutionStartEvent); ok {
			turn := instance.CanonicalTurn{
				Role: instance.RoleAssistant,
				Blocks: []instance.CanonicalBlock{
					instance.NewToolUseBlock(toolEv.ToolCallID, toolEv.ToolName, toolEv.Args),
				},
				Timestamp: time.Now(),
				TurnIndex: turnIdx,
			}
			turns = append(turns, turn)
			turnIdx++
		} else if resultEv, ok := event.(instance.PiToolExecutionEndEvent); ok {
			var textParts []string
			for _, c := range resultEv.Result.Content {
				if c.Text != "" {
					textParts = append(textParts, c.Text)
				}
			}
			turn := instance.CanonicalTurn{
				Role: instance.RoleUser,
				Blocks: []instance.CanonicalBlock{
					instance.NewToolResultBlock(resultEv.ToolCallID, resultEv.ToolName, strings.Join(textParts, "\n"), resultEv.IsError),
				},
				Timestamp: time.Now(),
				TurnIndex: turnIdx,
			}
			turns = append(turns, turn)
			turnIdx++
		}
	}

	return turns, nil
}

func (a *PiAdapter) Export(ctx context.Context, turns []instance.CanonicalTurn, inst *instance.Instance) error {
	sessionID := inst.PiSessionID()

	if sessionID == "" {
		sessionID = uuid.New().String()
		inst.WithPiSession(func(d *domain.PiSessionData) { d.SessionID = sessionID })

		inst.WithClaudeSession(func(d *domain.ClaudeSessionData) { d.ConversationUUID = sessionID })
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	piDir := filepath.Join(home, ".pi", "sessions")
	if err := os.MkdirAll(piDir, 0700); err != nil {
		return fmt.Errorf("failed to create pi session dir: %w", err)
	}

	path := filepath.Join(piDir, sessionID+".jsonl")
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create pi session file: %w", err)
	}
	defer file.Close()

	sessEv := instance.PiSessionEvent{
		Type:      "session",
		Version:   3,
		ID:        sessionID,
		Timestamp: time.Now().Format(time.RFC3339),
		CWD:       inst.ActiveDir(),
	}
	if b, err := json.Marshal(sessEv); err == nil {
		_, _ = file.Write(b)
		_, _ = file.Write([]byte("\n"))
	}

	for _, turn := range turns {
		for _, block := range turn.Blocks {
			switch block.Kind {
			case instance.BlockKindToolUse:
				ev := instance.PiToolExecutionStartEvent{
					Type:       "tool_execution_start",
					ToolCallID: block.ToolID,
					ToolName:   block.ToolName,
					Args:       block.ToolArgs,
				}
				if b, err := json.Marshal(ev); err == nil {
					_, _ = file.Write(b)
					_, _ = file.Write([]byte("\n"))
				}
			case instance.BlockKindToolResult:
				ev := instance.PiToolExecutionEndEvent{
					Type:       "tool_execution_end",
					ToolCallID: block.ToolResultID,
					ToolName:   block.ToolName,
					IsError:    block.ToolResultIsError,
					Result: instance.PiToolExecutionResult{
						Content: []instance.PiToolExecutionResultContent{
							{Type: "text", Text: block.ToolResultContent},
						},
					},
				}
				if b, err := json.Marshal(ev); err == nil {
					_, _ = file.Write(b)
					_, _ = file.Write([]byte("\n"))
				}
			}
		}
	}

	log.Info("PiAdapter: exported session history", "session", inst.Title, "session_id", sessionID, "turns", len(turns))
	return nil
}
