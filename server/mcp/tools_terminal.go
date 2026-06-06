package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

const (
	maxOutputLines = 200
	maxOutputBytes = 10 * 1024 // 10 KB
	maxInputBytes  = 4096

	writeRateLimitPerSec = 1.0
)

// terminalHandlers implements terminal I/O and VCS MCP tools.
type terminalHandlers struct {
	store      session.InstanceStore
	scrollback *scrollback.ScrollbackManager
	writeLim   *tokenBucket // per-session rate limiter for write_to_session
}

// ReadSessionOutputResult is the response type for read_session_output.
type ReadSessionOutputResult struct {
	MCPResult
	Output     string `json:"output,omitempty"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

func registerTerminalTools(s *mcpserver.MCPServer, th *terminalHandlers) {
	s.AddTool(
		mcpgo.NewTool("read_session_output",
			mcpgo.WithDescription("Read recent terminal output from a Stapler Squad session. Returns the last N lines of scrollback, ANSI codes stripped by default. Always check the 'truncated' field — if true, there is earlier output not shown. Use the 'lines' parameter (max 200) to retrieve more context."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("lines",
				mcpgo.Description("Number of lines to return from the end of scrollback (default 50, max 200)"),
				mcpgo.DefaultNumber(50),
				mcpgo.Min(1),
				mcpgo.Max(200),
			),
			mcpgo.WithBoolean("strip_ansi",
				mcpgo.Description("Strip ANSI escape sequences (default true)"),
				mcpgo.DefaultBool(true),
			),
		),
		th.readSessionOutput,
	)

	s.AddTool(
		mcpgo.NewTool("write_to_session",
			mcpgo.WithDescription("Send text input to a running session's terminal. Input is written directly to the session's PTY and reaches the running program (claude, shell, etc.) unfiltered. This tool is fire-and-forget: it returns immediately without waiting for the input to be processed. Use run_command for most cases; use this only when you need to send input without waiting. Rate limited to 1 call per second per session."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithString("input",
				mcpgo.Description("Text to send to the terminal (max 4096 bytes)"),
				mcpgo.Required(),
			),
			mcpgo.WithBoolean("press_enter",
				mcpgo.Description("Append newline after input (default true)"),
				mcpgo.DefaultBool(true),
			),
		),
		th.writeToSession,
	)

	s.AddTool(
		mcpgo.NewTool("send_control",
			mcpgo.WithDescription("Send a control character to a running session. Use key=\"C\" to interrupt (Ctrl+C) a hung or running process, key=\"D\" for EOF/exit, key=\"Z\" to suspend, key=\"L\" to clear screen. Returns immediately. Follow with read_session_output to confirm effect."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithString("key",
				mcpgo.Description("Control key to send: C (Ctrl+C interrupt), D (Ctrl+D EOF), Z (Ctrl+Z suspend), L (Ctrl+L clear screen)"),
				mcpgo.Required(),
				mcpgo.Enum("C", "D", "Z", "L"),
			),
		),
		th.sendControl,
	)

	s.AddTool(
		mcpgo.NewTool("wait_for_output",
			mcpgo.WithDescription("Wait until a pattern appears in the session's terminal output, or until timeout. Returns the matched line and recent output. On timeout, returns matched=false with the last-seen output — this is an expected outcome, not an error. Use run_command for most command execution."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithString("pattern",
				mcpgo.Description("Substring to match in output lines"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("timeout_seconds",
				mcpgo.Description("How long to wait in seconds (default 30, max 60)"),
				mcpgo.DefaultNumber(30),
				mcpgo.Min(1),
				mcpgo.Max(60),
			),
		),
		th.waitForOutput,
	)

	s.AddTool(
		mcpgo.NewTool("steer_session",
			mcpgo.WithDescription("Send a semantic steering message to a running Stapler Squad agent session. Unlike write_to_session, steer_session always appends a newline so the agent receives the message as a complete input. Use this to redirect an agent mid-task (e.g. 'Focus on the authentication module first'). Not rate-limited — this is a high-level semantic operation."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithString("message",
				mcpgo.Description("Steering message to send to the agent (max 4096 bytes)"),
				mcpgo.Required(),
			),
		),
		th.steerSession,
	)

	s.AddTool(
		mcpgo.NewTool("run_command",
			mcpgo.WithDescription("Send a shell command to a running session and wait for output. Combines write + wait + read in one call. Waits until output stops changing or timeout_seconds elapses, then returns the captured output. Use this for most command execution; use write_to_session + wait_for_output only when you need finer control. Input reaches the PTY unfiltered."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
			mcpgo.WithString("command",
				mcpgo.Description("Shell command to run (max 4096 bytes); newline appended automatically"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("timeout_seconds",
				mcpgo.Description("How long to wait for output to stabilize (default 30, max 120)"),
				mcpgo.DefaultNumber(30),
				mcpgo.Min(1),
				mcpgo.Max(120),
			),
			mcpgo.WithNumber("lines",
				mcpgo.Description("Lines of output to return (default 50, max 200)"),
				mcpgo.DefaultNumber(50),
				mcpgo.Min(1),
				mcpgo.Max(200),
			),
		),
		th.runCommand,
	)
}

// ---- read_session_output ----

func (th *terminalHandlers) readSessionOutput(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	lines := 50
	if v, ok := args["lines"].(float64); ok && v > 0 {
		lines = int(v)
		if lines > maxOutputLines {
			lines = maxOutputLines
		}
	}

	stripANSI := true
	if v, ok := args["strip_ansi"].(bool); ok {
		stripANSI = v
	}

	// Verify session exists.
	instances, err := th.store.LoadInstances()
	if err != nil {
		return errResult(ErrInternalError, "failed to load sessions", ""), nil
	}
	found := false
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			found = true
			break
		}
	}
	if !found {
		return errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID), "Use list_sessions to find available sessions"), nil
	}

	raw, err := th.scrollback.GetRecentBytes(sessionID, maxOutputBytes)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("failed to read scrollback: %v", err), ""), nil
	}

	if stripANSI {
		raw = stripANSI_(raw)
	}

	allLines := splitLines(raw)
	totalLines := len(allLines)
	truncated := false

	if totalLines > lines {
		allLines = allLines[totalLines-lines:]
		truncated = true
	}

	output := strings.Join(toStringSlice(allLines), "\n")
	if truncated {
		output = fmt.Sprintf("[... %d lines omitted. Call read_session_output with lines=200 to see earlier output ...]\n", totalLines-lines) + output
	}

	return okResult(ReadSessionOutputResult{
		MCPResult:  MCPResult{Success: true},
		Output:     output,
		TotalLines: totalLines,
		Truncated:  truncated,
	}), nil
}

// ---- write_to_session ----

// WriteSessionResult is the response for write_to_session.
type WriteSessionResult struct {
	MCPResult
	BytesWritten int `json:"bytes_written"`
}

func (th *terminalHandlers) writeToSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	input, ok := args["input"].(string)
	if !ok {
		return errResult(ErrInvalidArgument, "input is required", ""), nil
	}
	if len(input) > maxInputBytes {
		return errResult("INPUT_TOO_LONG", fmt.Sprintf("input exceeds %d bytes", maxInputBytes), "Reduce input length"), nil
	}

	pressEnter := true
	if v, ok := args["press_enter"].(bool); ok {
		pressEnter = v
	}

	if !th.writeLim.allow(sessionID) {
		return errResult("RATE_LIMITED", "write_to_session is limited to 1 call per second per session", "Wait 1 second before retrying"), nil
	}

	inst, errResult_ := th.findInstance(sessionID)
	if errResult_ != nil {
		return errResult_, nil
	}

	text := input
	if pressEnter {
		text += "\n"
	}

	// Wrap SendKeys in a goroutine with a 5-second timeout to prevent PTY write deadlock.
	// Use the request context so caller cancellation propagates; add a hard 5s cap.
	errCh := make(chan error, 1)
	go func() { errCh <- inst.SendKeys(text) }()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	select {
	case err := <-errCh:
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("send keys failed: %v", err), "Check that the session is running and not paused"), nil
		}
	case <-ctx.Done():
		return errResult("PTY_WRITE_TIMEOUT", "timed out writing to session PTY", "The session may be blocked. Use send_control with key=C to interrupt"), nil
	}

	return okResult(WriteSessionResult{
		MCPResult:    MCPResult{Success: true},
		BytesWritten: len(text),
	}), nil
}

// ---- send_control ----

var controlChars = map[string]string{
	"C": "\x03", // Ctrl+C
	"D": "\x04", // Ctrl+D
	"Z": "\x1a", // Ctrl+Z
	"L": "\x0c", // Ctrl+L
}

var controlNames = map[string]string{
	"C": "^C",
	"D": "^D",
	"Z": "^Z",
	"L": "^L",
}

// SendControlResult is the response for send_control.
type SendControlResult struct {
	MCPResult
	Sent string `json:"sent"`
}

func (th *terminalHandlers) sendControl(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	key, ok := args["key"].(string)
	if !ok || key == "" {
		return errResult(ErrInvalidArgument, "key is required (C, D, Z, or L)", ""), nil
	}

	char, ok := controlChars[key]
	if !ok {
		return errResult(ErrInvalidArgument, fmt.Sprintf("unknown key %q; must be C, D, Z, or L", key), ""), nil
	}

	inst, errResult_ := th.findInstance(sessionID)
	if errResult_ != nil {
		return errResult_, nil
	}

	errCh := make(chan error, 1)
	go func() { errCh <- inst.SendKeys(char) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case err := <-errCh:
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("send control failed: %v", err), ""), nil
		}
	case <-ctx.Done():
		return errResult("PTY_WRITE_TIMEOUT", "timed out writing control character", ""), nil
	}

	return okResult(SendControlResult{
		MCPResult: MCPResult{Success: true},
		Sent:      controlNames[key],
	}), nil
}

// ---- wait_for_output ----

// WaitForOutputResult is the response for wait_for_output.
type WaitForOutputResult struct {
	MCPResult
	Matched     bool   `json:"matched"`
	MatchedLine string `json:"matched_line,omitempty"`
	Output      string `json:"output"`
	Truncated   bool   `json:"truncated"`
}

func (th *terminalHandlers) waitForOutput(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return errResult(ErrInvalidArgument, "pattern is required", ""), nil
	}

	timeoutSecs := 30
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSecs = int(v)
		if timeoutSecs > 60 {
			timeoutSecs = 60
		}
	}

	// Verify session exists.
	instances, err := th.store.LoadInstances()
	if err != nil {
		return errResult(ErrInternalError, "failed to load sessions", ""), nil
	}
	found := false
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			found = true
			break
		}
	}
	if !found {
		return errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID), "Use list_sessions to find available sessions"), nil
	}

	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	patternLower := strings.ToLower(pattern)

	for {
		raw, readErr := th.scrollback.GetRecentBytes(sessionID, maxOutputBytes)
		if readErr == nil {
			stripped := stripANSI_(raw)
			allLines := splitLines(stripped)

			displayLines := allLines
			truncated := false
			if len(allLines) > 50 {
				displayLines = allLines[len(allLines)-50:]
				truncated = true
			}

			// Search for pattern (case-insensitive substring match).
			var matchedLine string
			matched := false
			for _, lineBytes := range allLines {
				if strings.Contains(strings.ToLower(string(lineBytes)), patternLower) {
					matchedLine = string(lineBytes)
					matched = true
					break
				}
			}

			if matched {
				output := strings.Join(toStringSlice(displayLines), "\n")
				if truncated {
					output = "[... earlier output omitted ...]\n" + output
				}
				return okResult(WaitForOutputResult{
					MCPResult:   MCPResult{Success: true},
					Matched:     true,
					MatchedLine: matchedLine,
					Output:      output,
					Truncated:   truncated,
				}), nil
			}
		}

		if time.Now().After(deadline) {
			// Return timeout result with last-seen output.
			raw, _ := th.scrollback.GetRecentBytes(sessionID, maxOutputBytes)
			stripped := stripANSI_(raw)
			allLines := splitLines(stripped)
			displayLines := allLines
			truncated := false
			if len(allLines) > 50 {
				displayLines = allLines[len(allLines)-50:]
				truncated = true
			}
			output := strings.Join(toStringSlice(displayLines), "\n")
			return okResult(WaitForOutputResult{
				MCPResult: MCPResult{Success: true, Error: &MCPError{
					Code:    "WAIT_TIMEOUT",
					Message: fmt.Sprintf("pattern %q not found within %d seconds", pattern, timeoutSecs),
				}},
				Matched:   false,
				Output:    output,
				Truncated: truncated,
			}), nil
		}

		<-ticker.C
	}
}

// ---- run_command ----

// RunCommandResult is the response for run_command.
type RunCommandResult struct {
	MCPResult
	Output       string `json:"output"`
	Truncated    bool   `json:"truncated"`
	TimedOut     bool   `json:"timed_out"`
	LastSequence uint64 `json:"last_sequence"`
}

func (th *terminalHandlers) runCommand(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return errResult(ErrInvalidArgument, "command is required", ""), nil
	}
	if len(command) > maxInputBytes {
		return errResult("INPUT_TOO_LONG", fmt.Sprintf("command exceeds %d bytes", maxInputBytes), "Reduce command length"), nil
	}

	timeoutSecs := 30
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSecs = int(v)
		if timeoutSecs > 120 {
			timeoutSecs = 120
		}
	}

	lines := 50
	if v, ok := args["lines"].(float64); ok && v > 0 {
		lines = int(v)
		if lines > maxOutputLines {
			lines = maxOutputLines
		}
	}

	inst, errResult_ := th.findInstance(sessionID)
	if errResult_ != nil {
		return errResult_, nil
	}

	// Send the command.
	sendErrCh := make(chan error, 1)
	go func() { sendErrCh <- inst.SendKeys(command + "\n") }()

	sendCtx, sendCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sendCancel()

	select {
	case err := <-sendErrCh:
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("send command failed: %v", err), "Check that the session is running and not paused"), nil
		}
	case <-sendCtx.Done():
		return errResult("PTY_WRITE_TIMEOUT", "timed out writing command to session PTY", ""), nil
	}

	// Poll until output stops changing for 2 consecutive seconds or timeout expires.
	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var prevChecksum uint64
	stableCount := 0
	timedOut := false

	for {
		<-ticker.C

		raw, _ := th.scrollback.GetRecentBytes(sessionID, maxOutputBytes)
		cs := bytesChecksum(raw)

		if cs == prevChecksum {
			stableCount++
			if stableCount >= 2 {
				break
			}
		} else {
			stableCount = 0
			prevChecksum = cs
		}

		if time.Now().After(deadline) {
			timedOut = true
			break
		}
	}

	// Read final output.
	raw, _ := th.scrollback.GetRecentBytes(sessionID, maxOutputBytes)
	stripped := stripANSI_(raw)
	allLines := splitLines(stripped)
	totalLines := len(allLines)
	truncated := false

	if totalLines > lines {
		allLines = allLines[totalLines-lines:]
		truncated = true
	}

	// Get last sequence from a single recent entry.
	var lastSeq uint64
	if entries, err := th.scrollback.GetScrollback(sessionID, 0, 10000); err == nil && len(entries) > 0 {
		lastSeq = entries[len(entries)-1].Sequence
	}

	output := strings.Join(toStringSlice(allLines), "\n")
	if truncated {
		output = fmt.Sprintf("[... %d lines omitted ...]\n", totalLines-lines) + output
	}

	return okResult(RunCommandResult{
		MCPResult:    MCPResult{Success: true},
		Output:       output,
		Truncated:    truncated,
		TimedOut:     timedOut,
		LastSequence: lastSeq,
	}), nil
}

// ---- steer_session ----

// SteerSessionResult is the response for steer_session.
type SteerSessionResult struct {
	MCPResult
	SessionID string `json:"session_id"`
	CharsSent int    `json:"chars_sent"`
	Result    string `json:"result,omitempty"` // set when using --resume subprocess
	Method    string `json:"method"`           // "send_keys" or "resume_subprocess"
}

func (th *terminalHandlers) steerSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	message, ok := args["message"].(string)
	if !ok || message == "" {
		return errResult(ErrInvalidArgument, "message is required", ""), nil
	}
	if len(message) > maxInputBytes {
		return errResult("MESSAGE_TOO_LONG", fmt.Sprintf("message exceeds %d bytes", maxInputBytes), "Reduce message length"), nil
	}

	// Strip null bytes to avoid silent corruption via the tmux send-keys path.
	message = strings.ReplaceAll(message, "\x00", "")
	if message == "" {
		return errResult(ErrInvalidArgument, "message must be non-empty after sanitization", ""), nil
	}

	inst, errResult_ := th.findInstance(sessionID)
	if errResult_ != nil {
		return errResult_, nil
	}

	// For OneShot sessions that have completed (Stopped) and have a conversation UUID,
	// use claude --resume subprocess instead of PTY send-keys.
	uuid := inst.GetClaudeConversationUUID()
	if inst.OneShot && uuid != "" && inst.GetEffectiveStatus() == session.Stopped {
		resumeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := inst.RunWithResume(resumeCtx, message)
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("resume subprocess failed: %v", err), "Ensure claude CLI is available and session_id is valid"), nil
		}
		return okResult(SteerSessionResult{
			MCPResult: MCPResult{Success: true},
			SessionID: sessionID,
			CharsSent: len(message),
			Result:    result,
			Method:    "resume_subprocess",
		}), nil
	}

	// Fallback: send via PTY send-keys (interactive sessions or sessions without UUID).
	text := message + "\r"

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- inst.SendKeys(text) }()

	select {
	case err := <-errCh:
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("send keys failed: %v", err), "Check that the session is running and not paused"), nil
		}
	case <-sendCtx.Done():
		return errResult("PTY_WRITE_TIMEOUT", "timed out writing to session PTY", "The session may be blocked. Use send_control with key=C to interrupt"), nil
	}

	return okResult(SteerSessionResult{
		MCPResult: MCPResult{Success: true},
		SessionID: sessionID,
		CharsSent: len(message),
		Method:    "send_keys",
	}), nil
}

// ---- helpers ----

// findInstance loads all sessions and returns the one matching sessionID.
// Returns an error result if not found or load fails.
func (th *terminalHandlers) findInstance(sessionID string) (*session.Instance, *mcpgo.CallToolResult) {
	instances, err := th.store.LoadInstances()
	if err != nil {
		return nil, errResult(ErrInternalError, "failed to load sessions", "")
	}
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			return inst, nil
		}
	}
	return nil, errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID), "Use list_sessions to find available sessions")
}

// stripANSI_ is an alias so we can call stripANSI without shadowing the function
// name inside the package (strip is defined in ansi.go).
func stripANSI_(b []byte) []byte { return stripANSI(b) }

// toStringSlice converts a slice of byte slices to a slice of strings.
func toStringSlice(lines [][]byte) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = string(l)
	}
	return out
}

// bytesChecksum returns a simple FNV-like checksum used for output-stability detection.
func bytesChecksum(b []byte) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}
