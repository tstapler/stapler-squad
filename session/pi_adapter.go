package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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

		lineCopy := append([]byte(nil), line...)

		switch peek.Type {
		case "session":
			var ev PiSessionEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode session event: %w", decodeErr)
			}
			return ev, nil
		case "agent_start":
			var ev PiAgentStartEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode agent_start event: %w", decodeErr)
			}
			return ev, nil
		case "agent_settled":
			var ev PiAgentSettledEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode agent_settled event: %w", decodeErr)
			}
			return ev, nil
		case "turn_start":
			var ev PiTurnStartEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode turn_start event: %w", decodeErr)
			}
			return ev, nil
		case "turn_end":
			var ev PiTurnEndEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode turn_end event: %w", decodeErr)
			}
			return ev, nil
		case "message_start":
			var ev PiMessageStartEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode message_start event: %w", decodeErr)
			}
			return ev, nil
		case "message_end":
			var ev PiMessageEndEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode message_end event: %w", decodeErr)
			}
			return ev, nil
		case "message_update":
			var ev PiMessageUpdateEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode message_update event: %w", decodeErr)
			}
			return ev, nil
		case "tool_execution_start":
			var ev PiToolExecutionStartEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode tool_execution_start event: %w", decodeErr)
			}
			return ev, nil
		case "tool_execution_end":
			var ev PiToolExecutionEndEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode tool_execution_end event: %w", decodeErr)
			}
			return ev, nil
		case "agent_end":
			var ev PiAgentEndEvent
			if decodeErr := json.Unmarshal(lineCopy, &ev); decodeErr != nil {
				return nil, fmt.Errorf("pi_adapter: failed to decode agent_end event: %w", decodeErr)
			}
			return ev, nil
		default:
			return nil, &piUnrecognizedTypeError{eventType: peek.Type}
		}
	}

	if scanErr := r.scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("pi_adapter: scanner error: %w", scanErr)
	}

	return nil, io.EOF
}
