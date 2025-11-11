package tmux

import (
	"bytes"
	"claude-squad/executor"
	"claude-squad/log"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
)

const ProgramClaude = "claude"

const ProgramAider = "aider"
const ProgramGemini = "gemini"

// TmuxSession represents a managed tmux session
type TmuxSession struct {
	// Initialized by NewTmuxSession
	//
	// The name of the tmux session and the sanitized name used for tmux commands.
	sanitizedName string
	program       string
	// serverSocket is the tmux server socket name for isolation (used with -L flag)
	// If empty, uses the default tmux server. For complete isolation (e.g., testing),
	// set to a unique value like "test" or "teatest_123" to create separate tmux servers.
	serverSocket string
	// ptyFactory is used to create a PTY for the tmux session.
	ptyFactory PtyFactory
	// cmdExec is used to execute commands in the tmux session.
	cmdExec executor.Executor

	// Initialized by Start or Restore
	//
	// ptmx is a PTY is running the tmux attach command. This can be resized to change the
	// stdout dimensions of the tmux pane. On detach, we close it and set a new one.
	// This should never be nil.
	ptmx *os.File
	// monitor monitors the tmux pane content and sends signals to the UI when it's status changes
	monitor *statusMonitor
	// bannerFilter detects and filters tmux status line banners from terminal output
	bannerFilter *BannerFilter

	// Initialized by Attach
	// Deinitilaized by Detach
	//
	// Channel to be closed at the very end of detaching. Used to signal callers.
	attachCh chan struct{}
	// While attached, we use some goroutines to manage the window size and stdin/stdout. This stuff
	// is used to terminate them on Detach. We don't want them to outlive the attached window.
	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup
	// External window size channel for IntelliJ terminal compatibility
	externalResizeCh chan windowSize

	// Detach synchronization to prevent race conditions
	detachMutex sync.Mutex
	detaching   bool

	// Session existence caching to avoid repeated list-sessions calls
	existsCacheMutex sync.RWMutex
	existsCache      bool
	existsCacheTime  time.Time
	existsCacheTTL   time.Duration
}

// windowSize represents terminal dimensions from external sources (like BubbleTea)
type windowSize struct {
	cols int
	rows int
}

const TmuxPrefix = "claudesquad_"

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// ToClaudeSquadTmuxName converts a string to a valid tmux session name with the default prefix
func ToClaudeSquadTmuxName(str string) string {
	return toClaudeSquadTmuxNameWithPrefix(str, TmuxPrefix)
}

// toClaudeSquadTmuxName is the internal version for backward compatibility
func toClaudeSquadTmuxName(str string) string {
	return ToClaudeSquadTmuxName(str)
}

func toClaudeSquadTmuxNameWithPrefix(str string, prefix string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_") // tmux replaces all . with _
	return fmt.Sprintf("%s%s", prefix, str)
}

// CleanupFunc represents a cleanup function that should be deferred
type CleanupFunc func() error

// NewTmuxSession creates a new TmuxSession with the given name and program.
func NewTmuxSession(name string, program string) *TmuxSession {
	return newTmuxSession(name, program, MakePtyFactory(), executor.MakeExecutor(), TmuxPrefix)
}

// NewTmuxSessionWithPrefix creates a new TmuxSession with a custom prefix for process isolation.
func NewTmuxSessionWithPrefix(name string, program string, prefix string) *TmuxSession {
	return newTmuxSession(name, program, MakePtyFactory(), executor.MakeExecutor(), prefix)
}

// NewTmuxSessionWithCleanup creates a new TmuxSession and returns it along with a cleanup function.
// Usage: session, cleanup := NewTmuxSessionWithCleanup(name, program); defer cleanup()
func NewTmuxSessionWithCleanup(name string, program string) (*TmuxSession, CleanupFunc) {
	session := NewTmuxSession(name, program)
	cleanup := CleanupFunc(func() error {
		return session.Close()
	})
	return session, cleanup
}

// NewTmuxSessionWithPrefixAndCleanup creates a new TmuxSession with custom prefix and cleanup function.
// Usage: session, cleanup := NewTmuxSessionWithPrefixAndCleanup(name, program, prefix); defer cleanup()
func NewTmuxSessionWithPrefixAndCleanup(name string, program string, prefix string) (*TmuxSession, CleanupFunc) {
	session := NewTmuxSessionWithPrefix(name, program, prefix)
	cleanup := CleanupFunc(func() error {
		return session.Close()
	})
	return session, cleanup
}

// NewTmuxSessionWithServerSocket creates a new TmuxSession with complete server isolation.
// This uses the tmux -L flag to create a completely separate tmux server, providing
// true isolation from other tmux sessions. Use this for testing or when you need
// complete separation from production tmux sessions.
//
// serverSocket: unique socket name (e.g., "test", "teatest_123", "isolated")
// prefix: session name prefix (e.g., "claudesquad_test_")
func NewTmuxSessionWithServerSocket(name string, program string, prefix string, serverSocket string) *TmuxSession {
	return newTmuxSessionWithSocket(name, program, MakePtyFactory(), executor.MakeExecutor(), prefix, serverSocket)
}

// NewTmuxSessionWithServerSocketAndCleanup creates a TmuxSession with server isolation and cleanup.
// Usage: session, cleanup := NewTmuxSessionWithServerSocketAndCleanup(name, program, prefix, socket); defer cleanup()
func NewTmuxSessionWithServerSocketAndCleanup(name string, program string, prefix string, serverSocket string) (*TmuxSession, CleanupFunc) {
	session := NewTmuxSessionWithServerSocket(name, program, prefix, serverSocket)
	cleanup := CleanupFunc(func() error {
		return session.Close()
	})
	return session, cleanup
}

// NewTmuxSessionWithDeps creates a new TmuxSession with provided dependencies for testing.
func NewTmuxSessionWithDeps(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor) *TmuxSession {
	return newTmuxSession(name, program, ptyFactory, cmdExec, TmuxPrefix)
}

func newTmuxSession(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor, prefix string) *TmuxSession {
	return newTmuxSessionWithSocket(name, program, ptyFactory, cmdExec, prefix, "")
}

// newTmuxSessionWithSocket creates a TmuxSession with both prefix and server socket isolation
func newTmuxSessionWithSocket(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor, prefix string, serverSocket string) *TmuxSession {
	return &TmuxSession{
		sanitizedName:    toClaudeSquadTmuxNameWithPrefix(name, prefix),
		program:          program,
		serverSocket:     serverSocket,
		ptyFactory:       ptyFactory,
		cmdExec:          cmdExec,
		bannerFilter:     NewBannerFilter(),          // Initialize banner filter for terminal output filtering
		externalResizeCh: make(chan windowSize, 10), // Buffered channel for resize events
		existsCacheTTL:   500 * time.Millisecond,    // Cache session existence for 500ms
	}
}

// buildTmuxCommand creates a tmux command with proper server isolation.
// If serverSocket is set, adds -L flag for complete server isolation.
func (t *TmuxSession) buildTmuxCommand(args ...string) *exec.Cmd {
	var cmdArgs []string

	// Add server socket isolation if specified
	if t.serverSocket != "" {
		cmdArgs = append(cmdArgs, "-L", t.serverSocket)
	}

	// Add the actual tmux command arguments
	cmdArgs = append(cmdArgs, args...)

	return exec.Command("tmux", cmdArgs...)
}

// buildAttachCommand creates a tmux attach-session command for PTY operations
func (t *TmuxSession) buildAttachCommand() *exec.Cmd {
	return t.buildTmuxCommand("attach-session", "-t", t.sanitizedName)
}

// Start creates and starts a new tmux session, then attaches to it. Program is the command to run in
// the session (ex. claude). workdir is the git worktree directory.
func (t *TmuxSession) Start(workDir string) error {
	return t.start(workDir, false, nil)
}

// StartWithCleanup creates and starts a new tmux session and returns a cleanup function.
// Usage: cleanup, err := session.StartWithCleanup(workDir); if err == nil { defer cleanup() }
func (t *TmuxSession) StartWithCleanup(workDir string) (CleanupFunc, error) {
	cleanup := CleanupFunc(func() error {
		return t.Close()
	})
	err := t.start(workDir, true, &cleanup)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// start is the internal implementation for Start and StartWithCleanup
func (t *TmuxSession) start(workDir string, setupCleanup bool, cleanup *CleanupFunc) error {
	// Check if the session already exists
	if t.DoesSessionExist() {
		// Session already exists - we can reuse it
		log.InfoLog.Printf("Tmux session '%s' already exists, reusing existing session", t.sanitizedName)

		// Set up cleanup if requested
		if setupCleanup && cleanup != nil {
			*cleanup = func() error {
				return t.Close()
			}
		}

		return nil
	}

	// Create a new detached tmux session and start the program in it
	cmd := t.buildTmuxCommand("new-session", "-d", "-s", t.sanitizedName, "-c", workDir, t.program)

	// Use cmdExec.Run() instead of pty.Start() for detached session creation
	// since detached sessions don't need PTY attachment during creation
	err := t.cmdExec.Run(cmd)
	if err != nil {
		// Cleanup any partially created session if any exists.
		if t.DoesSessionExist() {
			cleanupCmd := t.buildTmuxCommand("kill-session", "-t", t.sanitizedName)
			if cleanupErr := t.cmdExec.Run(cleanupCmd); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			t.invalidateExistsCache() // Session was killed, invalidate cache
		}
		// If we have a cleanup function pointer, set it to nil since startup failed
		if setupCleanup && cleanup != nil {
			*cleanup = func() error { return nil }
		}
		return fmt.Errorf("error starting tmux session: %w", err)
	}

	// Poll for session existence with exponential backoff
	timeout := time.After(2 * time.Second)
	sleepDuration := 5 * time.Millisecond
	for !t.DoesSessionExist() {
		select {
		case <-timeout:
			if cleanupErr := t.Close(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			return fmt.Errorf("timed out waiting for tmux session %s: %v", t.sanitizedName, err)
		default:
			time.Sleep(sleepDuration)
			// Exponential backoff up to 50ms max
			if sleepDuration < 50*time.Millisecond {
				sleepDuration *= 2
			}
		}
	}

	// Session exists now, invalidate cache to ensure fresh state
	t.invalidateExistsCache()

	// Set history limit to enable scrollback (default is 2000, we'll use 10000 for more history)
	historyCmd := t.buildTmuxCommand("set-option", "-t", t.sanitizedName, "history-limit", "10000")
	if err := t.cmdExec.Run(historyCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to set history-limit for session %s: %v", t.sanitizedName, err)
	}

	// Set up monitoring for session status tracking
	t.monitor = newStatusMonitor()

	// Set up cleanup if requested
	if setupCleanup && cleanup != nil {
		*cleanup = func() error {
			return t.Close()
		}
	}

	// Session is created and ready - let the user handle any program-specific interactions
	log.InfoLog.Printf("Tmux session '%s' created successfully, program '%s' starting", t.sanitizedName, t.program)
	return nil
}

// Restore attaches to an existing session and restores the window size
func (t *TmuxSession) Restore() error {
	return t.RestoreWithWorkDir("")
}

func (t *TmuxSession) RestoreWithWorkDir(workDir string) error {
	// First check if the session actually exists
	if !t.DoesSessionExist() {
		// Session doesn't exist, we need to create it instead of trying to attach
		log.WarningLog.Printf("Tmux session '%s' doesn't exist, creating new session instead of restoring", t.sanitizedName)

		// Use the provided working directory, fall back to current directory if not provided
		if workDir == "" {
			var err error
			workDir, err = os.Getwd()
			if err != nil {
				log.WarningLog.Printf("Could not get working directory for session '%s': %v", t.sanitizedName, err)
				workDir = "."
			}
		}

		// Create a new detached tmux session directly (avoid recursive call to Start)
		cmd := t.buildTmuxCommand("new-session", "-d", "-s", t.sanitizedName, "-c", workDir, t.program)
		err := t.cmdExec.Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to create tmux session '%s': %w", t.sanitizedName, err)
		}
		log.InfoLog.Printf("Created new tmux session '%s' in directory '%s'", t.sanitizedName, workDir)
		t.invalidateExistsCache() // Session was created, invalidate cache
	}

	// Session exists - create PTY connection for detached operations
	// This is needed for SetDetachedSize(), SendKeys(), and the Direct Claude Command Interface
	// We use tmux attach-session to get a PTY handle without actually attaching interactively
	if t.ptmx == nil {
		ptmx, _, err := t.ptyFactory.Start(t.buildAttachCommand())
		if err != nil {
			// Graceful degradation - log warning but allow session to continue
			// Session can still be viewed via tmux capture-pane, just won't support
			// PTY-based operations like resizing or command sending
			log.WarningLog.Printf("PTY initialization failed for session '%s': %v (session will work with limited functionality)", t.sanitizedName, err)
			// Continue without PTY - operations that require it will fail gracefully
		} else {
			t.ptmx = ptmx
			log.InfoLog.Printf("Successfully restored PTY connection for tmux session '%s'", t.sanitizedName)
		}
	}

	t.monitor = newStatusMonitor()
	return nil
}

type statusMonitor struct {
	// Store hashes to save memory.
	prevOutputHash []byte
}

func newStatusMonitor() *statusMonitor {
	return &statusMonitor{}
}

// hash hashes the string.
func (m *statusMonitor) hash(s string) []byte {
	h := sha256.New()
	// TODO: this allocation sucks since the string is probably large. Ideally, we hash the string directly.
	h.Write([]byte(s))
	return h.Sum(nil)
}

// TapEnter sends an enter keystroke to the tmux pane.
func (t *TmuxSession) TapEnter() error {
	_, err := t.ptmx.Write([]byte{0x0D})
	if err != nil {
		return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
	}
	return nil
}

// TapDAndEnter sends 'D' followed by an enter keystroke to the tmux pane.
func (t *TmuxSession) TapDAndEnter() error {
	_, err := t.ptmx.Write([]byte{0x44, 0x0D})
	if err != nil {
		return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
	}
	return nil
}

func (t *TmuxSession) SendKeys(keys string) (int, error) {
	return t.ptmx.Write([]byte(keys))
}

// GetPTY returns the PTY file descriptor for reading terminal output.
// This provides direct access to the PTY master for terminal streaming.
// Returns an error if the PTY is not initialized.
func (t *TmuxSession) GetPTY() (*os.File, error) {
	if t.ptmx == nil {
		return nil, fmt.Errorf("PTY not initialized - session may not be started")
	}
	return t.ptmx, nil
}

// HasUpdated checks if the tmux pane content has changed since the last tick. It also returns true if
// the tmux pane has a prompt for aider or claude code.
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool) {
	content, err := t.CapturePaneContent()
	if err != nil {
		log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		return false, false
	}

	// Filter out the tmux status line (bottom line with clock) before checking for updates
	// The status line updates every second and causes false positive update detection
	contentWithoutStatusLine := t.filterStatusLine(content)

	// Only set hasPrompt for claude and aider. Use these strings to check for a prompt.
	hasPrompt = t.detectPromptInContent(contentWithoutStatusLine)

	if !bytes.Equal(t.monitor.hash(contentWithoutStatusLine), t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = t.monitor.hash(contentWithoutStatusLine)
		return true, hasPrompt
	}
	return false, hasPrompt
}

// filterStatusLine removes the tmux status line (last line) from the content
// The status line typically contains session info and a clock that updates every second
// This uses sophisticated detection to identify actual status lines rather than blindly removing the last line
func (t *TmuxSession) filterStatusLine(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return content
	}

	lastLine := lines[len(lines)-1]

	// Check if the last line looks like a tmux status line
	// Tmux status lines typically have:
	// 1. Session name at the start (our sanitizedName)
	// 2. A time/date stamp (various formats: HH:MM, HH:MM:SS, MMM DD, etc.)
	// 3. Often contain ANSI color codes
	// 4. Are relatively short (< 200 chars typically)

	// Quick length check - status lines are usually short
	if len(lastLine) > 200 {
		return content // Last line too long to be a status line
	}

	// Check for session name in the line (strong indicator)
	hasSessionName := strings.Contains(lastLine, t.sanitizedName)

	// Check for time patterns (HH:MM or HH:MM:SS format)
	// Matches: "12:34", "23:59:59", "1:23", etc.
	timePattern := regexp.MustCompile(`\b([0-2]?[0-9]):([0-5][0-9])(:[0-5][0-9])?\b`)
	hasTime := timePattern.MatchString(lastLine)

	// Check for date patterns (common formats: "Jan 15", "2025-01-15", "15 Jan", etc.)
	datePattern := regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\b|\b\d{4}-\d{2}-\d{2}\b|\b\d{1,2}\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\b`)
	hasDate := datePattern.MatchString(lastLine)

	// Check for ANSI color codes (ESC sequences like \x1b[...m)
	hasColorCodes := strings.Contains(lastLine, "\x1b[")

	// Decision logic: Remove last line if it looks like a status line
	// Strong indicators: session name + (time OR date)
	// Weak indicators: just time/date without session name (could be program output)
	isStatusLine := false

	if hasSessionName && (hasTime || hasDate) {
		// Very likely a status line - has session name and timestamp
		isStatusLine = true
	} else if hasTime && hasDate && hasColorCodes {
		// Likely a status line - has both time and date with colors
		isStatusLine = true
	} else if hasTime && hasColorCodes && len(lastLine) < 100 {
		// Possibly a status line - has time, colors, and is short
		// This catches cases where session name might be styled/truncated
		isStatusLine = true
	}

	if isStatusLine {
		// Remove the status line
		return strings.Join(lines[:len(lines)-1], "\n")
	}

	// Not a status line, keep the original content
	return content
}

// detectPromptInContent checks if the given content contains a prompt from the configured program
func (t *TmuxSession) detectPromptInContent(content string) bool {
	if t.program == ProgramClaude {
		// Claude Code approval dialogs have a distinctive pattern:
		// An arrow selector (❯) followed by numbered options (1., 2., 3.)
		// This is more reliable than checking for specific text that might change.
		//
		// Example:
		// ❯ 1. Yes
		//   2. Yes, allow all edits during this session (shift+tab)
		//   3. No, and tell Claude what to do differently (esc)

		// Check for the arrow selector with numbered option pattern
		// Look for: arrow (❯) followed by number and period (1., 2., etc.) on subsequent lines
		if strings.Contains(content, "❯") {
			// If we have the arrow, check for numbered options nearby
			// Split into lines and look for the pattern
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				if strings.Contains(line, "❯") {
					// Found the arrow, check next few lines for numbered options
					for j := i; j < i+5 && j < len(lines); j++ {
						trimmed := strings.TrimSpace(lines[j])
						// Check for numbered options (1., 2., 3., etc.)
						if len(trimmed) > 0 && (trimmed[0] >= '1' && trimmed[0] <= '9') &&
						   len(trimmed) > 1 && trimmed[1] == '.' {
							return true
						}
					}
				}
			}
		}

		// Fallback: Check for legacy patterns in case the UI changes
		return strings.Contains(content, "No, and tell Claude what to do differently") ||
			strings.Contains(content, "Yes, allow all edits during this session")
	} else if strings.HasPrefix(t.program, ProgramAider) {
		return strings.Contains(content, "(Y)es/(N)o/(D)on't ask again")
	} else if strings.HasPrefix(t.program, ProgramGemini) {
		return strings.Contains(content, "Yes, allow once")
	}
	return false
}

func (t *TmuxSession) Attach() (chan struct{}, error) {
	t.attachCh = make(chan struct{})

	t.wg = &sync.WaitGroup{}
	t.wg.Add(1)
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// The first goroutine should terminate when the ptmx is closed. We use the
	// waitgroup to wait for it to finish.
	// The 2nd one returns when you press escape to Detach. It doesn't need to be
	// in the waitgroup because is the goroutine doing the Detaching; it waits for
	// all the other ones.
	go func() {
		defer t.wg.Done()
		_, _ = io.Copy(os.Stdout, t.ptmx)
		// When io.Copy returns, it means the connection was closed
		// This could be due to normal detach or Ctrl-D
		// Check if the context is done to determine if it was a normal detach
		select {
		case <-t.ctx.Done():
			// Normal detach, do nothing
		default:
			// If context is not done, it was likely an abnormal termination (Ctrl-D)
			// Gracefully handle the unexpected termination by calling DetachSafely
			// This will properly close the attachCh and clean up resources
			go func() {
				if err := t.DetachSafely(); err != nil {
					log.ErrorLog.Printf("Error during safe detach after session termination: %v", err)
				}
			}()
		}
	}()

	go func() {
		// Close the channel after 50ms
		timeoutCh := make(chan struct{})
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(timeoutCh)
		}()

		// Read input from stdin and check for Ctrl+q
		buf := make([]byte, 32)
		for {
			nr, err := os.Stdin.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}

			// Nuke the first bytes of stdin, up to 64, to prevent tmux from reading it.
			// When we attach, there tends to be terminal control sequences like ?[?62c0;95;0c or
			// ]10;rgb:f8f8f8. The control sequences depend on the terminal (warp vs iterm). We should use regex ideally
			// but this works well for now. Log this for debugging.
			//
			// There seems to always be control characters, but I think it's possible for there not to be. The heuristic
			// here can be: if there's characters within 50ms, then assume they are control characters and nuke them.
			select {
			case <-timeoutCh:
			default:
				log.InfoLog.Printf("nuked first stdin: %s", buf[:nr])
				continue
			}

			// Check for Ctrl+q (ASCII 17)
			if nr == 1 && buf[0] == 17 {
				// Detach from the session
				t.Detach()
				return
			}

			// Forward other input to tmux
			_, _ = t.ptmx.Write(buf[:nr])
		}
	}()

	t.monitorWindowSize()
	return t.attachCh, nil
}

// DetachSafely disconnects from the current tmux session without panicking
func (t *TmuxSession) DetachSafely() error {
	// Use mutex to prevent concurrent detach operations
	t.detachMutex.Lock()
	defer t.detachMutex.Unlock()

	// Check if we're already detaching or detached
	if t.detaching || t.attachCh == nil {
		return nil // Already detaching or detached
	}

	// Mark as detaching to prevent concurrent operations
	t.detaching = true
	defer func() {
		t.detaching = false
	}()

	var errs []error

	// Close the attached pty session only if it's not already closed.
	if t.ptmx != nil {
		// Attempt to close PTY, but ignore "file already closed" errors
		if err := t.ptmx.Close(); err != nil {
			// Only log error if it's not "file already closed"
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("error closing attach pty session: %w", err))
			}
		}
		t.ptmx = nil
	}

	// Clean up attach state safely
	if t.attachCh != nil {
		// Use a select with default to avoid blocking on an already-closed channel
		select {
		case <-t.attachCh:
			// Channel is already closed, nothing to do
		default:
			// Channel is open, safe to close
			close(t.attachCh)
		}
		t.attachCh = nil
	}

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.wg != nil {
		t.wg.Wait()
		t.wg = nil
	}

	t.ctx = nil

	if len(errs) > 0 {
		return fmt.Errorf("errors during detach: %v", errs)
	}
	return nil
}

// Detach disconnects from the current tmux session. It panics if detaching fails. At the moment, there's no
// way to recover from a failed detach.
func (t *TmuxSession) Detach() {
	// Use mutex to prevent concurrent detach operations
	t.detachMutex.Lock()
	defer t.detachMutex.Unlock()

	// Check if we're already detaching or detached
	if t.detaching || t.attachCh == nil {
		return // Already detaching or detached
	}

	// Mark as detaching to prevent concurrent operations
	t.detaching = true

	// TODO: control flow is a bit messy here. If there's an error,
	// I'm not sure if we get into a bad state. Needs testing.
	defer func() {
		// Safely close attachCh only if it exists and isn't already closed
		if t.attachCh != nil {
			// Use a select with default to avoid blocking on an already-closed channel
			select {
			case <-t.attachCh:
				// Channel is already closed, nothing to do
			default:
				// Channel is open, safe to close
				close(t.attachCh)
			}
			t.attachCh = nil
		}
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		if t.wg != nil {
			t.wg.Wait()
			t.wg = nil
		}
		t.ctx = nil
		t.detaching = false
	}()

	// Close the attached pty session.
	err := t.ptmx.Close()
	if err != nil {
		// Check if this is a "file already closed" error, which can happen due to race conditions
		if strings.Contains(err.Error(), "file already closed") {
			// This is expected in race conditions, just log and continue with restore
			log.InfoLog.Printf("PTY already closed during detach (expected in concurrent scenarios)")
		} else {
			// This is a fatal error. We can't detach if we can't close the PTY. It's better to just panic and have the
			// user re-invoke the program than to ruin their terminal pane.
			msg := fmt.Sprintf("error closing attach pty session: %v", err)
			log.ErrorLog.Println(msg)
			panic(msg)
		}
	}
	t.ptmx = nil

	// Attach goroutines should die on EOF due to the ptmx closing. Call
	// t.Restore to set a new t.ptmx.
	if err = t.Restore(); err != nil {
		// This is a fatal error. Our invariant that a started TmuxSession always has a valid ptmx is violated.
		msg := fmt.Sprintf("error restoring session after detach: %v", err)
		log.ErrorLog.Println(msg)
		panic(msg)
	}

	// Cancel goroutines created by Attach.
	if t.cancel != nil {
		t.cancel()
	}
	if t.wg != nil {
		t.wg.Wait()
	}
}

// Close terminates the tmux session and cleans up resources
func (t *TmuxSession) Close() error {
	var errs []error

	if t.ptmx != nil {
		if err := t.ptmx.Close(); err != nil {
			// Only log error if it's not "file already closed" (common in concurrent scenarios)
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}
		}
		t.ptmx = nil
	}

	// Check if session exists before trying to kill it
	if t.DoesSessionExist() {
		cmd := t.buildTmuxCommand("kill-session", "-t", t.sanitizedName)
		if err := t.cmdExec.Run(cmd); err != nil {
			// Check if this is the common "session not found" error
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				// Exit code 1 usually means session doesn't exist or was already killed
				log.InfoLog.Printf("Tmux session '%s' was already killed or doesn't exist", t.sanitizedName)
			} else {
				errs = append(errs, fmt.Errorf("error killing tmux session: %w", err))
			}
		} else {
			log.InfoLog.Printf("Successfully killed tmux session: %s", t.sanitizedName)
		}
		t.invalidateExistsCache() // Session was killed, invalidate cache
	} else {
		log.InfoLog.Printf("Tmux session '%s' doesn't exist, no need to kill", t.sanitizedName)
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple errors occurred during cleanup:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return errors.New(errMsg)
}

// SetDetachedSize set the width and height of the session while detached. This makes the
// tmux output conform to the specified shape.
func (t *TmuxSession) SetDetachedSize(width, height int) error {
	return t.updateWindowSize(width, height)
}

// updateWindowSize updates the window size of the PTY.
func (t *TmuxSession) updateWindowSize(cols, rows int) error {
	// Check if PTY is valid before attempting to resize
	if t.ptmx == nil {
		return fmt.Errorf("PTY is not initialized")
	}

	// Get the file descriptor value
	fd := int(t.ptmx.Fd())

	// Check if file descriptor is valid (not closed)
	if fd < 0 {
		return fmt.Errorf("PTY file descriptor is invalid")
	}

	// Additional check: try a simple stat on the file descriptor to verify it's still valid
	// This is more portable than platform-specific ioctl calls
	if _, err := os.Stat(fmt.Sprintf("/dev/fd/%d", fd)); err != nil {
		// If we can't stat the FD, it's likely closed or invalid
		return fmt.Errorf("PTY file descriptor is closed or invalid: %v", err)
	}

	return pty.Setsize(t.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
		X:    0,
		Y:    0,
	})
}

// SetWindowSize allows external callers (like web UI) to set terminal dimensions.
// This is particularly useful for web terminal integration where the browser controls the size.
// This method executes the resize immediately by calling both PTY and tmux resize commands.
func (t *TmuxSession) SetWindowSize(cols, rows int) error {
	log.InfoLog.Printf("🔧 SetWindowSize called for session '%s': target %dx%d", t.sanitizedName, cols, rows)

	// Get current dimensions for comparison
	currentWidth, currentHeight, _ := t.GetPaneDimensions()
	log.InfoLog.Printf("📏 Current tmux pane dimensions for '%s': %dx%d", t.sanitizedName, currentWidth, currentHeight)

	// First resize the PTY using the existing method
	if err := t.updateWindowSize(cols, rows); err != nil {
		// Log warning but don't fail - PTY resize might not be critical
		log.WarningLog.Printf("⚠️ Failed to resize PTY for session '%s': %v", t.sanitizedName, err)
	} else {
		log.InfoLog.Printf("✅ PTY resized successfully for session '%s'", t.sanitizedName)
	}

	// Also resize the tmux window itself to ensure the dimensions are applied
	// This ensures the tmux pane reflects the new size
	log.InfoLog.Printf("🔧 Running tmux resize-window command for '%s' to %dx%d", t.sanitizedName, cols, rows)
	cmd := t.buildTmuxCommand("resize-window", "-t", t.sanitizedName, "-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows))
	if err := t.cmdExec.Run(cmd); err != nil {
		log.ErrorLog.Printf("❌ tmux resize-window failed for '%s': %v", t.sanitizedName, err)
		return fmt.Errorf("failed to resize tmux window: %w", err)
	}

	// Verify the resize actually worked
	newWidth, newHeight, err := t.GetPaneDimensions()
	if err != nil {
		log.WarningLog.Printf("⚠️ Could not verify resize for '%s': %v", t.sanitizedName, err)
	} else {
		log.InfoLog.Printf("📏 New tmux pane dimensions for '%s': %dx%d (expected %dx%d)",
			t.sanitizedName, newWidth, newHeight, cols, rows)
		if newWidth != cols || newHeight != rows {
			log.ErrorLog.Printf("❌ Dimension mismatch after resize for '%s': got %dx%d, expected %dx%d",
				t.sanitizedName, newWidth, newHeight, cols, rows)
		}
	}

	log.InfoLog.Printf("✅ Resized tmux session '%s' to %dx%d", t.sanitizedName, cols, rows)
	return nil
}

func (t *TmuxSession) DoesSessionExist() bool {
	if t == nil {
		return false
	}

	// Check cache first (read lock)
	t.existsCacheMutex.RLock()
	if time.Since(t.existsCacheTime) < t.existsCacheTTL {
		cached := t.existsCache
		t.existsCacheMutex.RUnlock()
		return cached
	}
	t.existsCacheMutex.RUnlock()

	// Cache expired or not set, get fresh data (write lock)
	t.existsCacheMutex.Lock()
	defer t.existsCacheMutex.Unlock()

	// Double-check cache hasn't been updated by another goroutine
	if time.Since(t.existsCacheTime) < t.existsCacheTTL {
		return t.existsCache
	}

	// Use list-sessions to get actual running sessions for reliable checking
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux")
	// Add server socket isolation if specified
	if t.serverSocket != "" {
		cmd = exec.CommandContext(ctx, "tmux", "-L", t.serverSocket, "list-sessions", "-F", "#{session_name}")
	} else {
		cmd = exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	}
	output, err := t.cmdExec.Output(cmd)

	// Check if error is due to timeout
	if ctx.Err() == context.DeadlineExceeded {
		log.WarningLog.Printf("Timeout checking if tmux session exists: %s", t.sanitizedName)
		t.existsCache = false
		t.existsCacheTime = time.Now()
		return false
	}

	if err != nil {
		// If tmux list-sessions fails, there are no sessions
		t.existsCache = false
		t.existsCacheTime = time.Now()
		return false
	}

	// Parse the output to check if our session exists
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	exists := false
	for _, session := range sessions {
		if session == t.sanitizedName {
			exists = true
			break
		}
	}

	// Update cache
	t.existsCache = exists
	t.existsCacheTime = time.Now()
	return exists
}

// invalidateExistsCache clears the session existence cache to force a fresh check
func (t *TmuxSession) invalidateExistsCache() {
	t.existsCacheMutex.Lock()
	defer t.existsCacheMutex.Unlock()
	t.existsCacheTime = time.Time{} // Zero time forces cache miss
}

// CapturePaneContent captures the content of the tmux pane
func (t *TmuxSession) CapturePaneContent() (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		// Log detailed error information for debugging
		if log.ErrorLog != nil {
			log.ErrorLog.Printf("Failed to capture pane content for session '%s': %v", t.sanitizedName, err)
			log.ErrorLog.Printf("Tmux command: %s", cmd.String())
		}
		return "", fmt.Errorf("error capturing pane content for session '%s': %v", t.sanitizedName, err)
	}
	// Convert bytes to valid UTF-8 string, replacing invalid sequences
	// This prevents downstream parsing errors while preserving ANSI sequences
	return sanitizeUTF8String(output), nil
}

// CapturePaneContentWithOptions captures the pane content with additional options
// start and end specify the starting and ending line numbers (use "-" for the start/end of history)
func (t *TmuxSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-S", start, "-E", end, "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux pane content with options: %v", err)
	}
	// Convert bytes to valid UTF-8 string, replacing invalid sequences
	// This prevents downstream parsing errors while preserving ANSI sequences
	return sanitizeUTF8String(output), nil
}

// HasMeaningfulContent checks if the terminal output contains meaningful content
// (excluding tmux status banners). This is used to determine if the session has
// produced actual output versus just tmux status line updates.
func (t *TmuxSession) HasMeaningfulContent(content string) bool {
	if t.bannerFilter == nil {
		// No banner filter available, assume all content is meaningful
		return len(strings.TrimSpace(content)) > 0
	}
	return t.bannerFilter.HasMeaningfulContent(content)
}

// FilterBanners removes tmux status banners from terminal output.
// This is useful for processing terminal output while excluding tmux status lines.
func (t *TmuxSession) FilterBanners(content string) (filteredContent string, bannersRemoved int) {
	if t.bannerFilter == nil {
		// No banner filter available, return content as-is
		return content, 0
	}
	return t.bannerFilter.FilterBannersFromText(content)
}

// GetCursorPosition returns the current cursor position in the tmux pane.
// Returns cursor X (column) and Y (row) coordinates, both 0-based.
func (t *TmuxSession) GetCursorPosition() (x, y int, err error) {
	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{cursor_x} #{cursor_y}")

	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get cursor position for session '%s': %w", t.sanitizedName, err)
	}

	// Parse "x y" format
	var cursorX, cursorY int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d %d", &cursorX, &cursorY)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse cursor position '%s': %w", string(output), err)
	}

	return cursorX, cursorY, nil
}

// GetPaneDimensions returns the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (t *TmuxSession) GetPaneDimensions() (width, height int, err error) {
	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_width} #{pane_height}")

	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get pane dimensions for session '%s': %w", t.sanitizedName, err)
	}

	// Parse "width height" format
	var paneWidth, paneHeight int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d %d", &paneWidth, &paneHeight)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse pane dimensions '%s': %w", string(output), err)
	}

	return paneWidth, paneHeight, nil
}

// CleanupSessions kills all tmux sessions that start with "session-" on the default server
func CleanupSessions(cmdExec executor.Executor) error {
	return CleanupSessionsOnServer(cmdExec, "")
}

// CleanupSessionsOnServer kills all tmux sessions that start with "session-" on a specific server
// serverSocket: socket name for server isolation, empty string for default server
func CleanupSessionsOnServer(cmdExec executor.Executor, serverSocket string) error {
	// First try to list sessions
	var cmd *exec.Cmd
	if serverSocket != "" {
		cmd = exec.Command("tmux", "-L", serverSocket, "ls")
	} else {
		cmd = exec.Command("tmux", "ls")
	}
	output, err := cmdExec.Output(cmd)

	// If there's an error and it's because no server is running, that's fine
	// Exit code 1 typically means no sessions exist
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // No sessions to clean up
		}
		return fmt.Errorf("failed to list tmux sessions: %v", err)
	}

	re := regexp.MustCompile(fmt.Sprintf(`%s.*:`, TmuxPrefix))
	matches := re.FindAllString(string(output), -1)
	for i, match := range matches {
		matches[i] = match[:strings.Index(match, ":")]
	}

	for _, match := range matches {
		log.InfoLog.Printf("cleaning up session: %s", match)
		var killCmd *exec.Cmd
		if serverSocket != "" {
			killCmd = exec.Command("tmux", "-L", serverSocket, "kill-session", "-t", match)
		} else {
			killCmd = exec.Command("tmux", "kill-session", "-t", match)
		}
		if err := cmdExec.Run(killCmd); err != nil {
			return fmt.Errorf("failed to kill tmux session %s: %v", match, err)
		}
	}
	return nil
}

// sanitizeUTF8String converts raw bytes to valid UTF-8 string, preserving ANSI escape sequences
// This prevents xterm.js parsing errors from invalid byte sequences while maintaining
// terminal formatting and color information
func sanitizeUTF8String(rawBytes []byte) string {
	if len(rawBytes) == 0 {
		return ""
	}

	var result strings.Builder
	inEscape := false

	for i := 0; i < len(rawBytes); {
		// Start of ANSI escape sequence
		if rawBytes[i] == '\x1b' {
			inEscape = true
			result.WriteByte(rawBytes[i])
			i++
			continue
		}

		// Inside ANSI escape sequence - preserve all bytes
		if inEscape {
			b := rawBytes[i]
			result.WriteByte(b)
			// End of escape sequence (letter terminates most ANSI sequences)
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				inEscape = false
			}
			i++
			continue
		}

		// Outside escape sequences - handle UTF-8 and control characters
		r, size := utf8.DecodeRune(rawBytes[i:])

		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte - replace with replacement character
			result.WriteString("�")
			i++
		} else if r < 32 {
			// Control character - allow common terminal chars
			switch r {
			case '\t', '\n', '\r':
				result.WriteRune(r) // Keep tab, newline, carriage return
			case 7, 8:
				result.WriteRune(r) // Keep bell (BEL) and backspace (BS)
			default:
				// Replace other control characters with space to prevent parsing issues
				result.WriteByte(' ')
			}
			i += size
		} else {
			// Valid UTF-8 character
			result.WriteRune(r)
			i += size
		}
	}

	return result.String()
}
