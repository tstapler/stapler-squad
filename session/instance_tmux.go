package session

// instance_tmux.go contains tmux session creation, terminal I/O, PTY access, and
// control-mode delegation methods. All methods delegate to i.processManager.

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	ptyPkg "github.com/creack/pty"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// clampWinsizeDim clamps a terminal dimension (columns or rows) coming from
// a resize request (ultimately client-controlled, e.g. a browser's terminal
// widget) to the range representable by pty.Winsize's uint16 fields
// ([1, 65535]) before the narrowing int->uint16 conversion, so an
// out-of-range or negative value can't silently wrap around into a bogus
// terminal size.
func clampWinsizeDim(v int) uint16 {
	switch {
	case v < 1:
		return 1
	case v > math.MaxUint16:
		return math.MaxUint16
	default:
		return uint16(v)
	}
}

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

// defaultPromptFileCleanupDelay is how long promptArg waits before removing a
// temp-file-backed prompt (see promptArg), unless overridden per-instance via
// promptFileCleanupDelayOverride. The file only needs to survive long enough
// for the shell tmux spawns to evaluate the `$(cat ...)` command substitution,
// which happens as part of exec'ing the claude command -- effectively
// immediately after `tmux new-session` returns successfully. The delay is
// generous purely to tolerate a slow/loaded box; it is not a correctness
// requirement, so tests may shrink it (per-instance) to avoid a real sleep.
const defaultPromptFileCleanupDelay = 30 * time.Second

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

// yoloFlagByAgent maps a supported agent's basename to the CLI flag that
// bypasses its tool/permission-approval prompts entirely. This is a separate
// mechanism from AutoYes's --permission-mode injection in buildClaudeCommand
// -- see Instance.AutoApprove's doc comment.
var yoloFlagByAgent = map[string]string{
	"claude": "--dangerously-skip-permissions",
	"aider":  "--yes-always", // NOT "--yes" -- verified against the installed
	// aider binary's --help (v0.78.0): --yes-always is the current flag.
}

// yoloFlagFor returns the yolo/auto-approve flag for the agent detected in
// program's whitespace-delimited tokens (basename match, mirroring isClaude),
// or "" if the agent has no known flag.
func yoloFlagFor(program string) string {
	for _, token := range strings.Fields(program) {
		if flag, ok := yoloFlagByAgent[filepath.Base(token)]; ok {
			return flag
		}
	}
	return ""
}

// AutoApproveSupported reports whether program is a recognized agent that
// AutoApprove can inject a bypass flag for.
func AutoApproveSupported(program string) bool {
	return yoloFlagFor(program) != ""
}

// pm returns the process manager, lazy-initializing with the default TmuxBackend if nil.
// This mirrors the old embedded-struct behaviour where a zero-value TmuxProcessManager
// was always valid for method calls (session == nil → IsAlive()/HasSession() return false).
// Tests that create bare &Instance{} structs rely on this guarantee.
//
// pm() itself has no error return (its ~80 call sites across the package all
// assume a non-nil ProcessManager, matching the zero-value-friendly guarantee
// above), so an unrecognized i.Backend here — which should never happen in
// practice; i.Backend is only ever set to a known constant — is logged
// loudly via log.Error (not silently swallowed) and then recovered by
// constructing the guaranteed-valid BackendTmux explicitly, preserving the
// non-nil contract every caller of pm() depends on. Construction paths that
// DO have an error return (NewInstance, fromInstanceData) propagate
// NewProcessManager's error directly instead of going through this
// fallback — see backend_factory.go's ErrUnrecognizedBackend.
func (i *Instance) pm() ProcessManager {
	i.pmMu.Lock()
	defer i.pmMu.Unlock()
	if i.processManager == nil {
		mgr, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: i.Backend})
		if err != nil {
			log.Error("unrecognized process manager backend; falling back to BackendTmux", "session", i.Title, "backend", i.Backend, "err", err)
			mgr, _ = NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})
		}
		i.processManager = mgr
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
		// AutoApprove is injected inside buildClaudeCommand, before the
		// trailing "--" prompt separator -- see that function's comment for
		// why appending it here (after the separator) would be silently
		// swallowed as inert positional text instead of a real flag.
		cmd = i.buildClaudeCommand(p.base, claudeSessionID)
	case plainProgram:
		// shellQuoteFields (not one whole-string shellQuote) preserves legitimate multi-word
		// Program values like "sleep 300" that rely on shell word-splitting, while still
		// preventing a metacharacter-bearing token -- e.g. a preset's argv[0] of "true; touch
		// /tmp/pwned" -- from terminating the command and injecting a second one.
		cmd = shellQuoteFields(p.cmd)
		if i.AutoApprove {
			if flag := yoloFlagFor(i.Program); flag != "" {
				cmd = cmd + " " + flag
				log.ForSession(i.Title).Debug("auto-approve flag injected", "program", i.Program, "flag", flag)
			}
		}
	default:
		panic(fmt.Sprintf("unknown programKind %T", p))
	}
	if flags := shellQuoteFields(i.CLIFlags); flags != "" {
		cmd = cmd + " " + flags
	}
	// ExtraArgs elements are appended after CLIFlags, each independently shell-quoted as one
	// unit — never whitespace-split — so a multi-word element (e.g. a remote-exec fragment
	// like "cd ~/repo && exec claude") survives intact instead of being re-split into several
	// argv positions.
	for _, a := range i.ExtraArgs {
		cmd = cmd + " " + shellQuote(a)
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

// shellQuoteFields splits s on whitespace and shell-quotes each resulting token
// independently, joining them back with single spaces. Used wherever a caller-supplied
// string must be safe against embedded shell metacharacters while still preserving
// multi-word values that rely on ordinary shell word-splitting (e.g. cli_flags,
// or a plainProgram value like "sleep 300"). Returns "" for an empty/whitespace-only s.
func shellQuoteFields(s string) string {
	fields := strings.Fields(s)
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = shellQuote(f)
	}
	return strings.Join(quoted, " ")
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
	if i.AutoApprove {
		// Must be appended here, before the "--" prompt separator below --
		// once "--" is emitted, claude treats every subsequent token as
		// positional, so appending this after the separator (e.g. in
		// buildLaunchCommand, post-switch) would be silently ignored as
		// prompt text rather than parsed as a real flag. Verified empirically
		// that Claude CLI accepts this alongside AutoYes's --permission-mode
		// bypassPermissions on the same command line without error (both
		// bypass in the same direction; harmless if both are set).
		if flag := yoloFlagFor(base); flag != "" {
			parts = append(parts, flag)
			log.ForSession(i.Title).Debug("auto-approve flag injected", "program", i.Program, "flag", flag)
		}
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
// promptFileDir resolves config.Config.PromptCacheDirOrDefault()
// (~/.stapler-squad/prompt-cache) and ensures it exists. Falls back to "" (the
// OS default temp dir, via os.CreateTemp's own behavior) if resolution or
// creation fails, mirroring promptArg's existing degrade-to-inline pattern of
// never letting a filesystem hiccup here block session launch.
func promptFileDir() string {
	dir, err := config.LoadConfig().PromptCacheDirOrDefault()
	if err != nil {
		log.Warn("promptFileDir: failed to resolve prompt cache dir, using OS temp dir", "err", err)
		return ""
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Warn("promptFileDir: failed to create prompt cache dir, using OS temp dir", "dir", dir, "err", err)
		return ""
	}
	return dir
}

func (i *Instance) promptArg() string {
	if len(i.Prompt) < maxInlinePromptBytes {
		return shellQuote(i.Prompt)
	}
	f, err := os.CreateTemp(promptFileDir(), "prompt-*.txt")
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
	i.promptFilePath.Store(&path)
	// This timer is a fallback safety net, not the primary cleanup path:
	// Instance.Destroy() removes the file immediately via cleanupPromptFile.
	// The timer still exists for cases Destroy never runs (process crash,
	// or a session that's replaced by a later promptArg call before it is
	// ever destroyed).
	delay := defaultPromptFileCleanupDelay
	if i.promptFileCleanupDelayOverride > 0 {
		delay = i.promptFileCleanupDelayOverride
	}
	go func() {
		time.Sleep(delay)
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warn("promptArg: failed to clean up temp prompt file", "path", path, "err", rmErr)
		}
	}()
	return fmt.Sprintf(`"$(cat %s)"`, shellQuote(path))
}

// cleanupPromptFile removes the most recently created temp-file-backed prompt
// for this instance, if any. Called from Instance.Destroy() so the file is
// removed promptly at session teardown rather than relying solely on
// promptFileCleanupDelay's background timer.
func (i *Instance) cleanupPromptFile() {
	pathPtr := i.promptFilePath.Swap(nil)
	if pathPtr == nil {
		return
	}
	if rmErr := os.Remove(*pathPtr); rmErr != nil && !os.IsNotExist(rmErr) {
		log.Warn("cleanupPromptFile: failed to remove temp prompt file", "path", *pathPtr, "err", rmErr)
	}
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

	// runner threads i.ExecutionTarget through TmuxSession construction (ssh-remote-workspaces
	// Phase 4, Task 4.2.1d) -- tmux.LocalRunner{} for LocalTarget (the default, identical to
	// every construction site's pre-Phase-4 behavior) or the dialed *tmux.SSHRunner for a
	// remote target, so this session's tmux subprocess calls run on the same host the
	// CreateSession mode-specific block (server/services/session_service.go) already created
	// the remote tmux session on.
	runner := i.executionTarget().Runner()
	var session *tmux.TmuxSession
	if i.TmuxServerSocket != "" {
		session = tmux.NewTmuxSessionWithServerSocket(i.Title, enrichedProgram, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil), tmux.WithCommandRunner(runner))
	} else {
		session = tmux.NewTmuxSessionWithPrefix(i.Title, enrichedProgram, tmuxPrefix, tmux.WithCommandRunner(runner))
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
// TmuxAlive intentionally does not special-case Hibernated or Crashed here (unlike
// Paused/Stopped): both rely on their tmux session having actually been killed
// (Hibernate()/MarkCrashed) to make !i.pm().HasSession() true. If that kill ever
// fails, TmuxAlive() can still report true for either status -- ReviewQueuePoller's
// reconcileSessions Hibernated-but-alive and Crashed-but-alive cases exist as the
// safety net for exactly that scenario.
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
	dead, _, _ := i.PaneExitInfo()
	return dead
}

// PaneExitInfo reports whether the wrapped program's pane has exited
// (PaneProcessDead), along with its exit code and signal (empty string if
// none) when available. Used by SessionHealthChecker to distinguish a normal
// completion (exit code 0, no signal) from a genuine crash. code/signal are
// zero-valued when dead is false.
func (i *Instance) PaneExitInfo() (dead bool, code int, signal string) {
	if !i.TmuxAlive() {
		return false, 0, ""
	}
	tb, ok := i.pm().(*TmuxBackend)
	if !ok {
		return false, 0, ""
	}
	tm := tb.TmuxManager()
	if tm == nil {
		return false, 0, ""
	}
	code, signal, dead = tm.PaneExitStatus()
	return dead, code, signal
}

// GetPTYReader returns the PTY file handle for the tmux session.
func (i *Instance) GetPTYReader() (*os.File, error) {
	if !i.started.Load() {
		return nil, fmt.Errorf("session not started")
	}
	return i.pm().GetPTY()
}

// IsRemote reports whether this instance's tmux session runs on a remote
// host (ssh-remote-workspaces Phase 4 Epic 4.2's ExecutionTarget), exposed
// for server/services call sites (a different package, so the unexported
// executionTarget() accessor isn't reachable there) that need to branch on
// remoteness -- e.g. StreamTerminal's raw-PTY fallback (Task 4.4.1d). This
// is the same single mechanism (IsRemote()) every in-package branch already
// uses; server/services must never type-switch on ExecutionTarget/Instance
// fields instead (architecture-review.md Blocker 1).
func (i *Instance) IsRemote() bool {
	return i.executionTarget().IsRemote()
}

// GetPTYSession returns a live, resizable terminal connection for this
// instance's raw (non-control-mode) PTY-attach path --
// server/services/session_service.go's StreamTerminal raw-PTY fallback
// (ssh-remote-workspaces Phase 4, Task 4.4.1d). For a local instance this is
// exactly GetPTYReader()'s *os.File (which already satisfies
// tmux.PtySession: *os.File has Read/Write/Close, and localPTYSession below
// adds Resize via pty.Setsize). For a remote instance (IsRemote()) it opens
// a fresh SSH-backed PTY attached to the SAME remote tmux session via
// tmux.SSHPtyFactory (RequestPty + "tmux ... attach-session ..."), so a
// remote session's raw-PTY consumers see byte-identical behavior to a local
// one, differing only in transport underneath -- matching Story 4.4.1's
// acceptance criteria for streamViaControlMode, applied here to this
// separate raw-PTY code path. cols/rows seed the PTY's initial size (no
// terminal dimensions flow through StreamTerminal's handshake the way
// streamViaControlMode's CurrentPaneRequest does, so callers pass their own
// best-known size; tmux.defaultAttachCols/Rows-equivalent values are a
// reasonable default when none is known yet).
func (i *Instance) GetPTYSession(ctx context.Context, cols, rows int) (tmux.PtySession, error) {
	if !i.executionTarget().IsRemote() {
		ptyFile, err := i.GetPTYReader()
		if err != nil {
			return nil, err
		}
		return &localPTYSession{File: ptyFile}, nil
	}

	remote, ok := i.executionTarget().(RemoteExecutionTarget)
	if !ok {
		return nil, fmt.Errorf("session: IsRemote() true but ExecutionTarget is %T, not RemoteExecutionTarget", i.executionTarget())
	}
	sshRunner, ok := remote.Runner().(*tmux.SSHRunner)
	if !ok {
		return nil, fmt.Errorf("session: remote target's CommandRunner is %T, not *tmux.SSHRunner", remote.Runner())
	}
	tmuxSession := i.GetTmuxSession()
	if tmuxSession == nil {
		return nil, fmt.Errorf("session: no tmux session to attach to")
	}

	// Declared as the tmux.RemotePtyFactory interface (not the concrete
	// *tmux.SSHPtyFactory NewSSHPtyFactory returns) so this call site is
	// actually written against the interface, matching how PtyFactory
	// itself is consumed elsewhere in this codebase (TmuxSession's
	// ptyFactory field is PtyFactory-typed despite Pty{} being its only real
	// implementation) -- see RemotePtyFactory's doc comment in
	// session/tmux/pty.go for why the interface exists at all.
	var factory tmux.RemotePtyFactory = tmux.NewSSHPtyFactory(sshRunner)

	// WrapRemoteCommand applies the same $TMUX-unset/$TERM-forced treatment
	// startRemoteControlMode applies to the remote "tmux -C attach-session"
	// -- necessary here too since "tmux attach-session" (no -C) is even more
	// sensitive to a stale/absent $TERM (it renders the pane directly,
	// unlike control mode's structured text protocol).
	runName, runArgs := tmux.WrapRemoteCommand(tmux.Binary(), tmuxSession.AttachArgs())
	ws := &ptyPkg.Winsize{Rows: clampWinsizeDim(rows), Cols: clampWinsizeDim(cols)}
	return factory.StartPty(ctx, ws, "", runName, runArgs...)
}

// localPTYSession adapts *os.File to tmux.PtySession for GetPTYSession's
// local branch -- *os.File already has Read/Write/Close; Resize is the only
// method it's missing, implemented via the same pty.Setsize the local
// control-mode/raw-attach paths already use elsewhere in this codebase
// (session/tmux/tmux.go's updateWindowSize).
type localPTYSession struct {
	*os.File
}

func (l *localPTYSession) Resize(cols, rows int) error {
	return ptyPkg.Setsize(l.File, &ptyPkg.Winsize{
		Rows: clampWinsizeDim(rows),
		Cols: clampWinsizeDim(cols),
	})
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
		return "", streamhub.ErrSessionNotStarted
	}
	return i.pm().CapturePaneContent()
}

// terminalResyncExecGateFastLaneFlagName mirrors server/services'
// terminalResyncExecGateFastLaneFlagName (feature_flag_service.go). It can't
// be shared directly — server/services imports session, so the reverse
// import would cycle — so the literal is duplicated here; keep both in sync
// if the flag is ever renamed.
const terminalResyncExecGateFastLaneFlagName = "terminal:resync-exec-gate-fast-lane"

// CapturePaneContentPriority captures the current visible tmux pane content
// via the resync exec-gate fast lane (Epic 4.2) when the instance is
// tmux-backed, falling back to the plain CapturePaneContent() call for
// non-tmux-backed instances (there is no fast lane to reach there, and the
// plain call is a correct, if unoptimized, behavior).
func (i *Instance) CapturePaneContentPriority() (string, error) {
	if !i.started.Load() || i.Status == Paused {
		return "", streamhub.ErrSessionNotStarted
	}
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		return tb.TmuxManager().CapturePaneContentPriority()
	}
	i.logFastLaneAssertionFailure("CapturePaneContentPriority")
	return i.pm().CapturePaneContent()
}

// CapturePaneContentRawPriority mirrors CapturePaneContentPriority but
// returns the unjoined variant — see CapturePaneContentRaw's doc comment for
// why a terminal-rendering caller needs this instead of the joined form. The
// non-tmux fallback is CapturePaneContentRaw, not CapturePaneContent, for the
// same reason.
//
// ctx is caller-supplied, not manufactured here — see
// session/tmux/exec_gate.go's runFastLaneSubprocess doc comment: a resync
// operation calls this alongside RefreshTmuxClientPriority/
// GetPaneDimensionsPriority in sequence, and all of them must share the same
// overall deadline rather than each getting an independent fresh one.
func (i *Instance) CapturePaneContentRawPriority(ctx context.Context) (streamhub.RawPaneContent, error) {
	if !i.started.Load() || i.Status == Paused {
		return "", streamhub.ErrSessionNotStarted
	}
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		content, err := tb.TmuxManager().CapturePaneContentRawPriority(ctx)
		return streamhub.RawPaneContent(content), err
	}
	i.logFastLaneAssertionFailure("CapturePaneContentRawPriority")
	content, err := i.pm().CapturePaneContentRaw()
	return streamhub.RawPaneContent(content), err
}

// RefreshTmuxClientPriority forces the tmux client to refresh via the resync
// exec-gate fast lane (Epic 4.2) when the instance is tmux-backed, falling
// back to the plain RefreshTmuxClient() call otherwise. See
// CapturePaneContentPriority's doc comment for the fallback rationale, and
// CapturePaneContentRawPriority's for why ctx is caller-supplied.
func (i *Instance) RefreshTmuxClientPriority(ctx context.Context) error {
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		return tb.TmuxManager().RefreshClientPriority(ctx)
	}
	i.logFastLaneAssertionFailure("RefreshTmuxClientPriority")
	return i.pm().RefreshClient()
}

// GetPaneDimensionsPriority mirrors GetPaneDimensions but routes its
// subprocess fallback through the resync exec-gate fast lane when the
// instance is tmux-backed — see TmuxSession.GetPaneDimensionsPriority's doc
// comment. ctx is caller-supplied, for the same shared-deadline reason as
// CapturePaneContentRawPriority.
func (i *Instance) GetPaneDimensionsPriority(ctx context.Context) (width, height int, err error) {
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		return tb.TmuxManager().GetPaneDimensionsPriority(ctx)
	}
	i.logFastLaneAssertionFailure("GetPaneDimensionsPriority")
	return i.pm().GetPaneDimensions()
}

// logFastLaneAssertionFailure logs, at debug level, that a Priority() call
// fell back to its non-priority sibling because i.processManager wasn't a
// *TmuxBackend. It's a no-op unless terminal:resync-exec-gate-fast-lane is
// on: with the flag off, falling back is the expected no-op path and would
// just be noise. With the flag on, this distinguishes "flag on but this
// instance genuinely can't reach the fast lane" (e.g. a non-tmux backend)
// from "flag off, fast lane never attempted" — see pre-mortem.md P2 #4: a
// silent fallback here would otherwise mask a broken type assertion behind
// an unmoved top-line metric.
func (i *Instance) logFastLaneAssertionFailure(method string) {
	if !config.LoadConfig().GetFeatureFlag(terminalResyncExecGateFastLaneFlagName) {
		return
	}
	log.Debug("fast-lane priority call fell back to non-priority path: instance is not tmux-backed",
		"method", method, "instanceID", i.UUID, "title", i.Title)
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (streamhub.RawPaneContent, error) {
	if !i.started.Load() || i.Status == Paused {
		return "", streamhub.ErrSessionNotStarted
	}

	content, err := i.pm().CapturePaneContentRaw()
	return streamhub.RawPaneContent(content), err
}

// CapturePaneContentRawContext mirrors CapturePaneContentRaw but honors ctx:
// it delegates to CapturePaneContentRawPriority, which already races the
// underlying capture against ctx.Done() via the resync exec-gate fast lane
// (session/tmux/exec_gate.go's runFastLaneSubprocess) for tmux-backed
// instances. Satisfies streamhub.SessionController for
// StreamHub.applyNegotiatedSize.
func (i *Instance) CapturePaneContentRawContext(ctx context.Context) (streamhub.RawPaneContent, error) {
	return i.CapturePaneContentRawPriority(ctx)
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

// StartControlMode starts the control mode stream on the underlying tmux
// session.
//
// Story 3.1.2 (correctness gap fix): this is the actual point where
// control-mode ownership is acquired, so every caller — today's two RPC
// handler entry points (streamViaControlMode, streamViaHub) plus any future
// direct caller — is protected, not just the ones that remember to call
// streamhub.AcquireOwnershipLock themselves first. Before this fix, the
// ownership lock was only acquired from server/services/connectrpc_websocket.go,
// so a caller reaching StartControlMode by any other path bypassed mutual
// exclusion entirely. StartControlMode does not assert a specific expected
// StreamPath the way the RPC handlers' own ResolveExpecting calls do — it is
// legitimately called unconditionally by both the legacy and hub-owned
// paths (the underlying subprocess start is refcounted either way, see
// streamViaHub's comment at its own StartControlMode call site) — but it
// still runs inside AcquireOwnershipLock's real critical section
// (AcquireAndResolve), so a concurrent HubRegistry.GetOrCreate for the same
// session genuinely blocks on it rather than racing to resolve first.
func (i *Instance) StartControlMode() error {
	name := i.GetTmuxSessionName()
	if name == "" {
		// No tmux session identity yet (uninitialized instance, or a
		// non-tmux backend such as NativeProcessManager on Windows) — there
		// is no session name to key an ownership lock on, and every such
		// instance sharing the same "" key would otherwise be serialized
		// against each other for no reason. Fall back to the pre-3.1.2
		// unconditional call.
		return i.pm().StartControlMode()
	}
	return streamhub.AcquireOwnershipLock(name).AcquireAndResolve(effectiveStreamHubFlag(), func(streamhub.StreamPath) error {
		return i.pm().StartControlMode()
	})
}

// effectiveStreamHubFlag mirrors server/services' useStreamHub(): the same
// STAPLER_SQUAD_USE_STREAM_HUB env var + config.ResolveGlobalStreamHubDefault
// gate, duplicated here rather than imported (server/services must not be
// imported by package session — the reverse of the one-way dependency this
// project relies on) so this package's own ownership-lock choke point in
// StartControlMode observes the identical effective flag value. Both call
// sites route through config.ResolveGlobalStreamHubDefault, the single
// source of truth for the gating decision itself, so they cannot diverge in
// what "true" requires — only this thin env-var-read wrapper is repeated.
func effectiveStreamHubFlag() bool {
	requested := os.Getenv("STAPLER_SQUAD_USE_STREAM_HUB") == "true"
	effective, err := config.ResolveGlobalStreamHubDefault(config.LoadConfig(), requested)
	if err != nil {
		log.Error("streamhub: refusing to enable global default", "error", err)
		return false
	}
	return effective
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
//
// Guards on i.started/i.Status the same way CapturePaneContent and its
// siblings do, returning streamhub.ErrSessionNotStarted instead of falling
// through to i.pm().SetWindowSize. HasSession() alone is not a readiness
// signal: LoadInstances()'s reconciliation wires the *tmux.TmuxSession object
// (instance_serialization.go) synchronously, well before the async
// Start()/RestoreWithWorkDir() call that actually installs the PTY, so a
// resize landing in that window used to reach tmux.TmuxSession.SetWindowSize
// and fail with a raw "PTY is not initialized" error that
// StreamHub.applyNegotiatedSize's errors.Is(err, ErrSessionNotStarted)
// skip-and-retry branch (hub.go) couldn't recognize, plus logged a spurious
// WARN on every restart.
func (i *Instance) SetWindowSize(cols, rows int) error {
	if !i.started.Load() || i.Status == Paused {
		return streamhub.ErrSessionNotStarted
	}
	if i.pm().HasSession() {
		return i.pm().SetWindowSize(cols, rows)
	}
	return nil
}

// SetWindowSizeContext mirrors SetWindowSize but returns as soon as ctx is
// canceled or its deadline expires, instead of only ever waiting out
// SetWindowSize's own internal fixed timeout — the underlying tmux command
// path (session/tmux/tmux.go's cmCtx) has no caller-overridable context, so
// this wraps the blocking call in a goroutine and races it against ctx,
// following the same pattern as CapturePaneContentRawPriority for the
// resync fast lane. StreamHub.applyNegotiatedSize is the sole caller, via
// the streamhub.SessionController interface.
func (i *Instance) SetWindowSizeContext(ctx context.Context, cols, rows int) error {
	done := make(chan error, 1)
	go func() { done <- i.SetWindowSize(cols, rows) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	return i.pm().RefreshClient()
}

// EnterKeySequence is the byte sequence that submits a line of input to an
// interactive terminal session, matching what a real terminal sends for a
// physical Enter keypress in raw/cbreak mode (TapEnter, elsewhere in this
// package, writes the same 0x0D byte directly to the PTY). Raw-mode TUIs —
// including the Claude Code CLI's Ink-based interface running inside these
// tmux panes — read '\r' as submit; a bare '\n' is not recognized as Enter,
// so text sent with only a trailing '\n' is written into the pane but never
// actually submitted and sits unactioned in the input buffer indefinitely
// (BUG-047). Every call site that appends an "Enter" to text sent via
// SendKeys must use this constant (via BuildSubmittableInput, where
// applicable) rather than hand-rolling its own terminator.
const EnterKeySequence = "\r"

// BuildSubmittableInput appends EnterKeySequence to input when pressEnter is
// true, producing the exact string that must be handed to SendKeys for the
// receiving program to treat it as a submitted line rather than unsubmitted
// text sitting in the input buffer. Centralizing this (rather than each
// caller appending its own terminator) is what BUG-047 was missing: three of
// six SendKeys-with-enter call sites had independently picked '\n' instead of
// '\r'.
func BuildSubmittableInput(input string, pressEnter bool) string {
	if pressEnter {
		return input + EnterKeySequence
	}
	return input
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
