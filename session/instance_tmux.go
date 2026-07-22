package session

// instance_tmux.go contains tmux session creation, terminal I/O, PTY access, and
// control-mode delegation methods. All methods delegate to i.processManager.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// maxInlinePromptBytes bounds how large i.Prompt can be before it is embedded
// directly (shell-quoted) into the tmux new-session command string, versus
// written to a temp file and referenced via command substitution (see
// promptArg). tmux's client/server protocol caps the entire new-session
// command around 16KB -- measured empirically: `tmux new-session -d -s <name>
// <command>` succeeds with a ~16KB command string and fails client-side with
// "command too long" once the string crosses somewhere between 16000 and
// 16500 bytes -- and that budget also has to cover the rest of the claude
// invocation (--mcp-config JSON, --allowedTools, --permission-mode, etc.) plus
// tmux's own session-name/workdir/env args, not just the prompt. 4KB leaves
// generous headroom for all of that.
//
// This is the exact failure that broke backlog review-gate spawns: a large
// description plus many acceptance criteria (each carrying a verbose
// self-reported implementation note) pushed the review prompt alone past
// tmux's limit, so `tmux new-session` rejected the command outright with
// "command too long" on every single attempt, and
// BacklogLifecycle.ReconcileStuckReviewGates kept re-spawning the identical,
// permanently-doomed command every ~8 minutes with no operator-visible signal
// beyond a repeating log line.
const maxInlinePromptBytes = 4096

// promptFileCleanupDelay is how long promptArg waits before removing a
// temp-file-backed prompt (see promptArg). The file only needs to survive
// long enough for the shell tmux spawns to evaluate the `$(cat ...)` command
// substitution, which happens as part of exec'ing the claude command --
// effectively immediately after `tmux new-session` returns successfully. The
// delay is generous purely to tolerate a slow/loaded box; it is not a
// correctness requirement, so tests may shrink it to avoid a real sleep.
var promptFileCleanupDelay = 30 * time.Second

// programKind is a sealed sum type over the kinds of launchable programs.
// Holding a claudeProgram is proof that isClaude() returned true — downstream
// code needs no further guards. Parse once at the boundary, trust internally.
type programKind interface{ sealedProgramKind() }

type claudeProgram struct{ base string }
type plainProgram struct{ cmd string }

func (claudeProgram) sealedProgramKind() {}
func (plainProgram) sealedProgramKind()  {}

// classifyProgram parses a raw program string into its kind.
// Call this once; pass the result where program type matters.
func classifyProgram(program string) programKind {
	if isClaude(program) {
		return claudeProgram{base: program}
	}
	return plainProgram{cmd: program}
}

// isClaude reports whether the program command invokes the claude binary.
// It checks each whitespace-delimited token's basename to avoid false positives
// from env wrappers (e.g. "env -u VAR claude") and to reject similar names
// like "claude-squad" or "myclaudeapp".
func isClaude(program string) bool {
	for _, token := range strings.Fields(program) {
		if filepath.Base(token) == "claude" {
			return true
		}
	}
	return false
}

// pm returns the process manager, lazy-initializing with the default TmuxBackend if nil.
// This mirrors the old embedded-struct behaviour where a zero-value TmuxProcessManager
// was always valid for method calls (session == nil → IsAlive()/HasSession() return false).
// Tests that create bare &Instance{} structs rely on this guarantee.
func (i *Instance) pm() ProcessManager {
	i.pmMu.Lock()
	defer i.pmMu.Unlock()
	if i.processManager == nil {
		i.processManager = NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})
	}
	return i.processManager
}

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string for external or uninitialized sessions.
func (i *Instance) GetTmuxSessionName() string {
	return i.pm().GetSessionIdentifier()
}

// buildLaunchCommand constructs the final command string used to launch the program
// in tmux. It parses the program once into a programKind sum type and delegates:
// non-claude programs are returned unchanged; claude programs get flag injection.
func (i *Instance) buildLaunchCommand(claudeSessionID string) string {
	var cmd string
	switch p := classifyProgram(i.Program).(type) {
	case claudeProgram:
		cmd = i.buildClaudeCommand(p.base, claudeSessionID)
	case plainProgram:
		cmd = p.cmd
	default:
		panic(fmt.Sprintf("unknown programKind %T", p))
	}
	for _, f := range strings.Fields(i.CLIFlags) {
		cmd = cmd + " " + shellQuote(f)
	}
	return cmd
}

// shellQuote POSIX-single-quotes s for safe interpolation into a shell command
// string. tmux launches the assembled command through a shell, and Go's %q
// produces double quotes, which do NOT suppress backtick/$(...)/$VAR
// expansion. Single quotes do: the only special case is a literal single
// quote, escaped by closing the quoted string, emitting \', and reopening.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildClaudeCommand assembles the full claude invocation with all instance flags.
// It is only called when the program is proven to be claude (via programKind),
// so no isClaude guards are needed here.
func (i *Instance) buildClaudeCommand(base, claudeSessionID string) string {
	parts := []string{base}
	if claudeSessionID != "" {
		// claudeSessionID traces back to the client-supplied resume_id RPC field
		// (CreateSessionRequest) with no format validation, so it needs the same
		// shell-quoting as the other interpolated flag values.
		parts = append(parts, "--resume", shellQuote(claudeSessionID))
	}
	if i.MCPServerURL != "" {
		flag, val := i.claudeMCPConfigArgs()
		parts = append(parts, flag, val)
	}
	if i.AppendSystemPrompt != "" {
		parts = append(parts, "--append-system-prompt", shellQuote(i.AppendSystemPrompt))
	}
	if i.AllowedTools != "" {
		parts = append(parts, "--allowedTools", shellQuote(i.AllowedTools))
	}
	if i.PermissionMode != "" {
		parts = append(parts, "--permission-mode", shellQuote(i.PermissionMode))
	}
	if i.AutoYes {
		parts = append(parts, "--permission-mode", PermissionModeBypassPermissions)
	}
	if i.OneShot {
		parts = append(parts, "-p", "--output-format", "json")
	}
	if i.Prompt != "" && (claudeSessionID == "" || i.OneShot) {
		// "--" stops claude from parsing a prompt that begins with "--" (e.g. the
		// backlog prompt's "--- BACKLOG ITEM DATA ---") as CLI flags.
		parts = append(parts, "--", i.promptArg())
	}
	return strings.Join(parts, " ")
}

// promptArg returns the shell syntax used to supply i.Prompt as the trailing
// positional argument to claude. Short prompts are embedded directly
// (shell-quoted), as before. Prompts at or above maxInlinePromptBytes are
// written to a temp file and referenced via a `"$(cat '<path>')"` command
// substitution instead.
//
// This distinction matters because tmux's new-session command-length limit
// (see maxInlinePromptBytes) applies to the literal command string handed to
// tmux, which tmux inspects before any shell ever runs -- so a large prompt
// embedded inline blows that budget outright, regardless of how carefully it
// is quoted. Routing it through a file keeps the string tmux sees short; the
// substitution -- and the real prompt content -- is only expanded later, by
// the shell tmux spawns to actually run the command, which is not subject to
// tmux's own limit. This was verified empirically: a `tmux new-session`
// command referencing a 20KB file via `$(cat ...)` succeeds and the spawned
// process receives the full, unmodified 20KB content, while the same 20KB
// embedded inline is rejected outright with "command too long".
//
// On temp-file write failure this falls back to the inline (shell-quoted)
// form so a filesystem hiccup degrades to the pre-existing behavior (which
// works fine for prompts under the tmux limit) rather than silently dropping
// the prompt.
//
// Caveat: POSIX command substitution strips ALL trailing newlines from its
// output, so if i.Prompt ends in one or more "\n" characters, the claude
// process receives the prompt with that trailing whitespace removed (content
// is otherwise byte-identical). This is semantically inert for an LLM prompt
// and is a world apart from the bug being fixed here (the entire tail of the
// prompt being dropped or the spawn failing outright), so it's accepted
// rather than worked around with a fragile shell trim-guard hack.
func (i *Instance) promptArg() string {
	if len(i.Prompt) < maxInlinePromptBytes {
		return shellQuote(i.Prompt)
	}
	f, err := os.CreateTemp("", "stapler-squad-prompt-*.txt")
	if err != nil {
		log.Warn("promptArg: failed to create temp file for large prompt, embedding inline", "err", err, "promptBytes", len(i.Prompt))
		return shellQuote(i.Prompt)
	}
	path := f.Name()
	if _, writeErr := f.WriteString(i.Prompt); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		log.Warn("promptArg: failed to write temp prompt file, embedding inline", "err", writeErr, "promptBytes", len(i.Prompt))
		return shellQuote(i.Prompt)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(path)
		log.Warn("promptArg: failed to close temp prompt file, embedding inline", "err", closeErr, "promptBytes", len(i.Prompt))
		return shellQuote(i.Prompt)
	}
	// Capture the delay here, on the calling goroutine, rather than reading
	// promptFileCleanupDelay inside the spawned goroutine below: tests mutate
	// that package var (see withShortPromptFileCleanupDelay) and restore it in
	// t.Cleanup, which races with a still-sleeping cleanup goroutine from an
	// earlier promptArg call reading the same var concurrently. Capturing a
	// local copy means the background goroutine never touches the shared var,
	// so only the (sequential, non-parallel) test/production goroutine ever
	// accesses it.
	delay := promptFileCleanupDelay
	go func() {
		time.Sleep(delay)
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warn("promptArg: failed to clean up temp prompt file", "path", path, "err", rmErr)
		}
	}()
	return fmt.Sprintf(`"$(cat %s)"`, shellQuote(path))
}

// claudeMCPConfigArgs returns the --mcp-config flag and its shell-quoted JSON value.
// Uses the Streamable HTTP transport (type "http") pointing at MCPServerURL, with the
// session UUID passed as a request header. The server middleware at /mcp extracts
// X-Stapler-Session-UUID and injects it into the request context for tool handlers.
// Both "http" and "streamable-http" are accepted by the Claude CLI for --mcp-config.
func (i *Instance) claudeMCPConfigArgs() (string, string) {
	cfg := fmt.Sprintf(
		`{"mcpServers":{"stapler-squad":{"type":"http","url":%q,"headers":{"X-Stapler-Session-UUID":%q}}}}`,
		i.MCPServerURL, i.UUID,
	)
	return "--mcp-config", shellQuote(cfg)
}

// initTmuxSession creates (or reuses) the tmux.TmuxSession object without starting it.
func (i *Instance) initTmuxSession() {
	if i.pm().HasSession() {
		log.Info("reusing existing tmux session", "session", i.Title)
		return
	}
	var claudeSessionID string
	if i.claudeSession != nil {
		claudeSessionID = i.claudeSession.ConversationUUID
	}
	enrichedProgram := i.buildLaunchCommand(claudeSessionID)
	i.LaunchCommand = enrichedProgram
	log.Info("creating tmux session", "session", i.Title, "program", enrichedProgram)

	tmuxPrefix := i.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}

	var session *tmux.TmuxSession
	if i.TmuxServerSocket != "" {
		session = tmux.NewTmuxSessionWithServerSocket(i.Title, enrichedProgram, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil))
	} else {
		session = tmux.NewTmuxSessionWithPrefix(i.Title, enrichedProgram, tmuxPrefix)
	}
	if i.UUID != "" {
		session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})
	}
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		tb.TmuxManager().SetSession(session)
	}
}

// KillSession terminates the tmux session only (leaves worktree intact).
func (i *Instance) KillSession() error {
	if i.pm().HasSession() {
		if err := i.pm().Close(); err != nil {
			return fmt.Errorf("failed to close tmux session: %w", err)
		}
	}
	return nil
}

// KillSessionKeepWorktree terminates tmux session but preserves worktree for recovery scenarios.
func (i *Instance) KillSessionKeepWorktree() error {
	return i.KillSession()
}

// KillExternalSession terminates an external mux session by killing its tmux session.
// This only works for external sessions that were started via ssq-mux with tmux integration.
// Returns an error if this is not an external instance or lacks tmux session name.
func (i *Instance) KillExternalSession() error {
	if i.InstanceType != InstanceTypeExternal {
		return fmt.Errorf("not an external instance")
	}
	if i.ExternalMetadata == nil || i.ExternalMetadata.TmuxSessionName == "" {
		return fmt.Errorf("no tmux session name available (session may not support destroy)")
	}

	// Stop the controller if running
	i.StopController()

	// Kill the tmux session
	killCtx, killCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer killCancel()
	//nolint:tmuxsocketscope targets a user-created external session; no isolated variant exists to target
	cmd := safeexec.CommandContext(killCtx, tmux.Binary(), "kill-session", "-t", i.ExternalMetadata.TmuxSessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill tmux session '%s': %w", i.ExternalMetadata.TmuxSessionName, err)
	}

	return nil
}

// HasUpdated reports whether terminal content has changed since the last check.
// Returns (updated, hasPrompt) and side-effects terminal timestamps on change.
func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started.Load() || i.Status == Paused {
		return false, false
	}

	// Check if the tmux session is still alive
	if !i.TmuxAlive() {
		return false, false
	}

	var content string
	updated, hasPrompt, content = i.pm().HasUpdated()

	// Update timestamps when content has actually changed.
	// HasUpdated returns the already-captured content, so no second CapturePaneContent call needed.
	if updated && content != "" {
		i.UpdateTerminalTimestamps(content, false)
	}

	return updated, hasPrompt
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started.Load() || !i.AutoYes {
		return
	}
	if err := i.pm().TapEnter(); err != nil {
		log.Error("error tapping enter", "err", err)
	}
}

// Attach attaches to the tmux session and returns a done channel.
func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started.Load() {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.pm().Attach()
}

// SetPreviewSize sets the detached terminal dimensions for preview rendering.
func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started.Load() || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.pm().SetDetachedSize(width, height, i.Title)
}

// trackRestartRate records a restart timestamp and logs a warning when the
// session has restarted more than 5 times in the last 5 minutes (crash loop).
func (i *Instance) trackRestartRate() {
	const window = 5 * time.Minute
	const threshold = 5

	now := time.Now()
	i.restartMu.Lock()
	defer i.restartMu.Unlock()

	i.restartCount++

	// Drop timestamps outside the window.
	cutoff := now.Add(-window)
	kept := i.recentRestartTimes[:0]
	for _, t := range i.recentRestartTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	i.recentRestartTimes = append(kept, now)

	if int64(len(i.recentRestartTimes)) >= threshold {
		log.Warn("restart storm detected, possible crash loop", "session", i.Title, "count", len(i.recentRestartTimes), "window", window.Seconds(), "total", i.restartCount)
	}
}

// TmuxSessionExists reports whether the underlying tmux session is currently alive.
// Used at startup to reconcile stale Stopped status against live tmux sessions.
func (i *Instance) TmuxSessionExists() bool {
	return i.pm().IsAlive()
}

// TmuxAlive returns true if the tmux session is alive. This is a sanity check before attaching.
func (i *Instance) TmuxAlive() bool {
	if i.Status == Paused || i.Status == Stopped || !i.started.Load() || !i.pm().HasSession() {
		return false
	}
	return i.pm().IsAlive()
}

// PaneProcessDead reports whether the tmux session is alive (TmuxAlive()==true)
// but the wrapped program running in the pane has already exited. remain-on-exit
// keeps the tmux session/pane around as a "Pane is dead (signal N, ...)"
// placeholder after the wrapped program is killed (e.g. OOM SIGKILL) or crashes,
// rather than tearing the session down -- so TmuxAlive() alone reports this
// session as healthy forever. Health checks must consult this in addition to
// TmuxAlive() to detect that failure mode. Returns false for non-tmux backends
// (e.g. native process manager), which have no equivalent placeholder state.
func (i *Instance) PaneProcessDead() bool {
	if !i.TmuxAlive() {
		return false
	}
	tb, ok := i.pm().(*TmuxBackend)
	if !ok {
		return false
	}
	tm := tb.TmuxManager()
	if tm == nil {
		return false
	}
	_, _, dead := tm.PaneExitStatus()
	return dead
}

// GetPTYReader returns the PTY file handle for the tmux session.
func (i *Instance) GetPTYReader() (*os.File, error) {
	if !i.started.Load() {
		return nil, fmt.Errorf("session not started")
	}
	return i.pm().GetPTY()
}

// WriteToPTY writes data to the PTY, sending input to the terminal session.
// This is used for forwarding client input to the tmux session.
func (i *Instance) WriteToPTY(data []byte) (int, error) {
	if !i.started.Load() {
		return 0, fmt.Errorf("session not started")
	}
	return i.pm().SendKeys(string(data))
}

// ResizePTY resizes the terminal dimensions.
// This is used when clients resize their terminal windows.
func (i *Instance) ResizePTY(cols, rows int) error {
	if !i.started.Load() {
		return fmt.Errorf("session not started")
	}
	if err := i.pm().SetWindowSize(cols, rows); err != nil {
		return fmt.Errorf("failed to resize terminal: %w", err)
	}
	return nil
}

// CapturePaneContent captures the current visible tmux pane content.
// This is a simple wrapper around TmuxSession.CapturePaneContent() for compatibility
// with the terminal WebSocket handlers.
func (i *Instance) CapturePaneContent() (string, error) {
	if !i.started.Load() || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}
	return i.pm().CapturePaneContent()
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (string, error) {
	if !i.started.Load() || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	return i.pm().CapturePaneContentRaw()
}

// GetCurrentPaneContent captures the current visible tmux pane content.
// Delegates to processManager.CaptureViewport.
func (i *Instance) GetCurrentPaneContent(lines int) (string, error) {
	content, err := i.pm().CaptureViewport(lines)
	if err != nil {
		return "", fmt.Errorf("failed to capture current pane content: %w", err)
	}
	return content, nil
}

// GetPaneCursorPosition gets the current cursor position in the tmux pane.
// Returns cursor X (column) and Y (row) coordinates, both 0-based.
func (i *Instance) GetPaneCursorPosition() (x, y int, err error) {
	return i.pm().GetCursorPosition()
}

// GetPaneDimensions gets the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (i *Instance) GetPaneDimensions() (width, height int, err error) {
	return i.pm().GetPaneDimensions()
}

// GetScrollbackHistory captures scrollback history from tmux using line ranges.
// Uses tmux's native scrollback capabilities instead of stored sequences.
// startLine and endLine follow tmux conventions: negative numbers go back from current position,
// use "-" for the start/end of history.
func (i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error) {
	return i.pm().CapturePaneContentWithOptions(startLine, endLine)
}

// SendPrompt sends a prompt to the tmux session. Delegates to processManager.SendPromptWithEnter.
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started.Load() {
		return fmt.Errorf("instance not started")
	}
	return i.pm().SendPromptWithEnter(prompt)
}

// GetTmuxSession returns the underlying tmux session for direct access.
// Returns nil if the session hasn't been started yet or if the backend is not tmux.
func (i *Instance) GetTmuxSession() *tmux.TmuxSession {
	tb, ok := i.pm().(*TmuxBackend)
	if !ok {
		return nil
	}
	return tb.TmuxManager().Session()
}

// ---- SessionStreamer delegation methods ----
// These methods satisfy the services.SessionStreamer interface without exposing
// the concrete *tmux.TmuxSession type to the server layer.

// StartControlMode starts the control mode stream on the underlying tmux session.
func (i *Instance) StartControlMode() error {
	return i.pm().StartControlMode()
}

// StopControlMode stops the control mode stream.
func (i *Instance) StopControlMode() error {
	return i.pm().StopControlMode()
}

// SubscribeControlModeUpdates returns a subscriber ID and a read-only output channel.
// Returns a pre-closed channel if the tmux session is not available.
func (i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte) {
	return i.pm().SubscribeToControlModeUpdates()
}

// UnsubscribeControlModeUpdates removes a subscriber by ID.
func (i *Instance) UnsubscribeControlModeUpdates(id string) {
	i.pm().UnsubscribeFromControlModeUpdates(id)
}

// SetTmuxSession sets the tmux session for testing purposes.
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		tb.TmuxManager().SetSession(session)
	}
	i.started.Store(session != nil)
}

// SetWindowSize propagates window size changes to the tmux session.
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work.
func (i *Instance) SetWindowSize(cols, rows int) error {
	if i.pm().HasSession() {
		return i.pm().SetWindowSize(cols, rows)
	}
	return nil
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	return i.pm().RefreshClient()
}

// SendKeys sends keys to the tmux session.
func (i *Instance) SendKeys(keys string) error {
	if !i.started.Load() || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	_, err := i.pm().SendKeys(keys)
	return err
}

// SendInputViaControlMode sends raw bytes through the existing control mode connection,
// avoiding the subprocess spawn overhead and timeout risk of exec.CommandContext.
func (i *Instance) SendInputViaControlMode(ctx context.Context, data []byte) error {
	if !i.started.Load() || i.Status == Paused {
		return fmt.Errorf("cannot send input to instance that has not been started or is paused")
	}
	return i.pm().SendInputViaControlMode(ctx, data)
}

// GetPanePID returns the PID of the foreground process in the tmux pane.
// The DoesSessionExist guard is omitted here: TmuxSession.GetPanePID already uses
// the CM fast path (no subprocess) and falls back to display-message which returns
// an error if the session is gone. Avoiding a separate list-sessions call per instance
// keeps this cheap for HistoryLinker.ScanAll, which calls this sequentially (not
// fanned out) per session anyway -- and the display-message subprocess fallback is
// itself gated (session/tmux's exec gate), so there's no need for a second guard
// here even if that ever changes.
func (i *Instance) GetPanePID() (int32, error) {
	return i.pm().GetPanePID()
}
