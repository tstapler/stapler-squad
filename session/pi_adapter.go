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

// piScannerInitialBufferSize and piScannerMaxBufferSize size the bufio.Scanner
// used by PiEventReader. pi's `--mode json` output streams large
// `message_update` envelopes (assistant text/thinking/tool-call deltas plus a
// usage block) on a single line; the default bufio.Scanner buffer (64KB) is
// too small for these in practice, so both bounds are raised well above it
// per research/stack.md's note on large message_update deltas.
const (
	piScannerInitialBufferSize = 64 * 1024
	piScannerMaxBufferSize     = 1024 * 1024
)

// PiEvent is a minimal discriminator used to peek a JSONL line's `type`
// field before re-decoding the same line into a concrete event struct. This
// mirrors the peek-then-decode pattern used elsewhere in the codebase for
// discriminated-union-shaped JSON (see session/sshremote's
// PermissionRequestPayload handling).
type PiEvent struct {
	Type string `json:"type"`
}

// PiSessionEvent is the first line of a pi `--mode json` transcript,
// describing the session itself.
//
// Verified against pi 0.84.4 on 2026-09-02 (session/detection/testdata/pi/basic_session.jsonl):
// {"type":"session","version":3,"id":"...","timestamp":"...","cwd":"..."}
type PiSessionEvent struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

// PiAgentStartEvent marks the start of agent processing. Observed shape is
// just {"type":"agent_start"} with no additional fields.
type PiAgentStartEvent struct {
	Type string `json:"type"`
}

// PiAgentSettledEvent marks that the agent has fully settled (no more work
// pending). Observed shape is just {"type":"agent_settled"}.
type PiAgentSettledEvent struct {
	Type string `json:"type"`
}

// PiTurnStartEvent and PiTurnEndEvent bracket a single conversational turn.
// PiTurnEndEvent carries the same message envelope shape as message_end plus
// a toolResults array; both are captured loosely as json.RawMessage since
// Epic 5.2's status inference does not need to parse turn contents.
type PiTurnStartEvent struct {
	Type string `json:"type"`
}

type PiTurnEndEvent struct {
	Type        string          `json:"type"`
	Message     json.RawMessage `json:"message,omitempty"`
	ToolResults json.RawMessage `json:"toolResults,omitempty"`
}

// PiMessageStartEvent and PiMessageEndEvent bracket a message (user,
// assistant, or toolResult role). The message body's shape varies by role
// and is not needed by Epic 5.2's status inference, so it's kept as
// json.RawMessage.
type PiMessageStartEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
}

type PiMessageEndEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
}

// PiAssistantMessageEvent captures only the nested `assistantMessageEvent`
// envelope's own `type` (e.g. "toolcall_start", "text_delta",
// "thinking_end", ...). Deeper per-variant fields (delta text, contentIndex,
// etc.) are intentionally left unparsed since Epic 5.2's status inference
// cares about message_update's outer envelope, not this nested detail.
type PiAssistantMessageEvent struct {
	Type string `json:"type"`
}

// PiMessageUpdateEvent is a single streaming delta within an in-progress
// assistant message. `Usage` and the nested `AssistantMessageEvent` are kept
// loosely typed since only the outer event type matters for status
// inference.
type PiMessageUpdateEvent struct {
	Type                  string                  `json:"type"`
	Usage                 json.RawMessage         `json:"usage,omitempty"`
	AssistantMessageEvent PiAssistantMessageEvent `json:"assistantMessageEvent"`
}

// PiToolExecutionStartEvent fires when pi begins executing a tool call.
type PiToolExecutionStartEvent struct {
	Type       string          `json:"type"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Args       json.RawMessage `json:"args,omitempty"`
}

// PiToolExecutionResultContent is one entry of a tool_execution_end event's
// result.content array.
type PiToolExecutionResultContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PiToolExecutionResult is the `result` object nested in a
// tool_execution_end event.
type PiToolExecutionResult struct {
	Content []PiToolExecutionResultContent `json:"content"`
}

// PiToolExecutionEndEvent fires when pi finishes executing a tool call,
// matched to its start via ToolCallID.
type PiToolExecutionEndEvent struct {
	Type       string                `json:"type"`
	ToolCallID string                `json:"toolCallId"`
	ToolName   string                `json:"toolName"`
	Result     PiToolExecutionResult `json:"result"`
	IsError    bool                  `json:"isError"`
}

// PiAgentEndEvent marks the end of a full agent turn cycle. It carries the
// entire conversation (`messages`) plus cost/usage data; Epic 5.2's status
// inference only needs to recognize this event's type, so `Messages` is kept
// as json.RawMessage rather than deeply typed.
type PiAgentEndEvent struct {
	Type      string          `json:"type"`
	Messages  json.RawMessage `json:"messages,omitempty"`
	WillRetry bool            `json:"willRetry,omitempty"`
}

// piUnrecognizedTypeError is returned by PiEventReader.Next when a line's
// `type` discriminator doesn't match any known pi event, so callers can
// count/log it (per Story 6.1.1) instead of the reader silently dropping the
// line or panicking.
type piUnrecognizedTypeError struct {
	eventType string
}

func (e *piUnrecognizedTypeError) Error() string {
	return fmt.Sprintf("pi_adapter: unrecognized event type %q", e.eventType)
}

// PiEventReader reads a pi `--mode json` transcript line-by-line, decoding
// each line into its concrete typed event. It mirrors ClaudeAdapter's
// JSONL-reading shape (session/claude_adapter.go), but uses bufio.Scanner
// with a raised buffer instead of bufio.NewReader.ReadBytes, since pi's
// large message_update lines can exceed the default 64KB scanner buffer.
//
// PiEventReader deliberately uses bufio.ScanLines (the default split
// function), which splits only on '\n' (optionally preceded by '\r'). This
// is a regression guard for PITFALL-2: some "line-aware" splitters treat
// Unicode line separators such as U+2028 as line breaks too, which would
// mis-split a JSON string value containing a literal U+2028 character into
// two garbled, non-JSON lines. bufio.ScanLines does not do this.
type PiEventReader struct {
	scanner *bufio.Scanner
}

// NewPiEventReader constructs a PiEventReader over r.
func NewPiEventReader(r io.Reader) *PiEventReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, piScannerInitialBufferSize), piScannerMaxBufferSize)
	return &PiEventReader{scanner: scanner}
}

// piEventDecoders maps each known pi event `type` discriminator to a decode
// function. Keeping this as a table (rather than a long switch in Next)
// keeps Next's cognitive complexity low despite the large number of event
// types.
var piEventDecoders = map[string]func(line []byte) (any, error){
	"session": func(line []byte) (any, error) {
		var ev PiSessionEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"agent_start": func(line []byte) (any, error) {
		var ev PiAgentStartEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"agent_settled": func(line []byte) (any, error) {
		var ev PiAgentSettledEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"turn_start": func(line []byte) (any, error) {
		var ev PiTurnStartEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"turn_end": func(line []byte) (any, error) {
		var ev PiTurnEndEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"message_start": func(line []byte) (any, error) {
		var ev PiMessageStartEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"message_end": func(line []byte) (any, error) {
		var ev PiMessageEndEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"message_update": func(line []byte) (any, error) {
		var ev PiMessageUpdateEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"tool_execution_start": func(line []byte) (any, error) {
		var ev PiToolExecutionStartEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"tool_execution_end": func(line []byte) (any, error) {
		var ev PiToolExecutionEndEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
	"agent_end": func(line []byte) (any, error) {
		var ev PiAgentEndEvent
		err := json.Unmarshal(line, &ev)
		return ev, err
	},
}

// Next reads and decodes the next event line. It returns io.EOF (wrapped by
// nothing) when the underlying reader is exhausted. Blank lines are skipped.
// A line whose `type` discriminator is unrecognized returns a nil event and
// a *piUnrecognizedTypeError (the raw line is not retained); callers can use
// errors.As to detect and count this case rather than treating it as fatal.
func (r *PiEventReader) Next() (event any, err error) {
	for r.scanner.Scan() {
		line := r.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var peek PiEvent
		if unmarshalErr := json.Unmarshal(line, &peek); unmarshalErr != nil {
			return nil, fmt.Errorf("pi_adapter: failed to parse event line: %w", unmarshalErr)
		}

		decode, known := piEventDecoders[peek.Type]
		if !known {
			return nil, &piUnrecognizedTypeError{eventType: peek.Type}
		}

		lineCopy := append([]byte(nil), line...)
		ev, decodeErr := decode(lineCopy)
		if decodeErr != nil {
			return nil, fmt.Errorf("pi_adapter: failed to decode %s event: %w", peek.Type, decodeErr)
		}
		return ev, nil
	}

	if scanErr := r.scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("pi_adapter: scanner error: %w", scanErr)
	}

	return nil, io.EOF
}

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

func (a *PiAdapter) Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error) {
	sessionID := ""
	inst.piSessionMu.Lock()
	if inst.piSession != nil {
		sessionID = inst.piSession.SessionID
	}
	inst.piSessionMu.Unlock()

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

	reader := NewPiEventReader(file)
	var turns []CanonicalTurn
	turnIdx := 0

	for {
		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if toolEv, ok := event.(PiToolExecutionStartEvent); ok {
			turn := CanonicalTurn{
				Role: RoleAssistant,
				Blocks: []CanonicalBlock{
					NewToolUseBlock(toolEv.ToolCallID, toolEv.ToolName, toolEv.Args),
				},
				Timestamp: time.Now(),
				TurnIndex: turnIdx,
			}
			turns = append(turns, turn)
			turnIdx++
		} else if resultEv, ok := event.(PiToolExecutionEndEvent); ok {
			var textParts []string
			for _, c := range resultEv.Result.Content {
				if c.Text != "" {
					textParts = append(textParts, c.Text)
				}
			}
			turn := CanonicalTurn{
				Role: RoleUser,
				Blocks: []CanonicalBlock{
					NewToolResultBlock(resultEv.ToolCallID, resultEv.ToolName, strings.Join(textParts, "\n"), resultEv.IsError),
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

func (a *PiAdapter) Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error {
	sessionID := ""
	inst.piSessionMu.Lock()
	if inst.piSession != nil {
		sessionID = inst.piSession.SessionID
	}
	inst.piSessionMu.Unlock()

	if sessionID == "" {
		sessionID = uuid.New().String()
		inst.piSessionMu.Lock()
		if inst.piSession == nil {
			inst.piSession = &PiSessionData{}
		}
		inst.piSession.SessionID = sessionID
		inst.piSessionMu.Unlock()

		inst.claudeSessionMu.Lock()
		if inst.claudeSession == nil {
			inst.claudeSession = &ClaudeSessionData{}
		}
		inst.claudeSession.ConversationUUID = sessionID
		inst.claudeSessionMu.Unlock()
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

	sessEv := PiSessionEvent{
		Type:      "session",
		Version:   3,
		ID:        sessionID,
		Timestamp: time.Now().Format(time.RFC3339),
		CWD:       inst.GetWorkingDirectory(),
	}
	if b, err := json.Marshal(sessEv); err == nil {
		_, _ = file.Write(b)
		_, _ = file.Write([]byte("\n"))
	}

	for _, turn := range turns {
		for _, block := range turn.Blocks {
			switch block.Kind {
			case BlockKindToolUse:
				ev := PiToolExecutionStartEvent{
					Type:       "tool_execution_start",
					ToolCallID: block.ToolID,
					ToolName:   block.ToolName,
					Args:       block.ToolArgs,
				}
				if b, err := json.Marshal(ev); err == nil {
					_, _ = file.Write(b)
					_, _ = file.Write([]byte("\n"))
				}
			case BlockKindToolResult:
				ev := PiToolExecutionEndEvent{
					Type:       "tool_execution_end",
					ToolCallID: block.ToolResultID,
					ToolName:   block.ToolName,
					IsError:    block.ToolResultIsError,
					Result: PiToolExecutionResult{
						Content: []PiToolExecutionResultContent{
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
