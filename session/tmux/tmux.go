package tmux

import "github.com/linkdata/deadlock"

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
)

const ProgramClaude = "claude"

const ProgramAider = "aider"
const ProgramGemini = "gemini"

// defaultAttachCols/Rows are the initial PTY dimensions used before the real browser
// size is known. They are also the fallback if no client has ever connected.
const defaultAttachCols = 220
const defaultAttachRows = 50

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
	// attachCmd is the tmux attach-session process that owns the PTY
	// CRITICAL: Must be killed when closing PTY to prevent orphaned processes
	attachCmd *exec.Cmd
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
	detachMutex deadlock.Mutex
	detaching   bool

	// registry is an optional SessionExistenceChecker backed by the server-level
	// control-mode event stream. When healthy, DoesSessionExist queries it directly
	// instead of forking a tmux subprocess. Nil means use the exec fallback.
	registry SessionExistenceChecker
	// registryExplicit is set to true by WithRegistry so that newTmuxSessionWithSocket
	// knows to skip the GetServerRegistry call.  Without this flag, GetServerRegistry
	// would always start a reconnect loop even when the caller intends to use nil.
	registryExplicit bool

	// registryKey is the key used to register this session's circuit breaker executor
	// in the global registry. Stored here so Close() can unregister it on teardown.
	registryKey string

	// lastKnownCols/Rows hold the terminal dimensions from the most recent successful
	// resize. Used to pre-size new PTY attach connections (via attach-session -x/-y)
	// so the tmux session immediately reflects the correct browser window size on
	// reconnect rather than starting at the tmux default of 80×24.
	lastKnownCols atomic.Int32
	lastKnownRows atomic.Int32

	// Session existence caching to avoid repeated list-sessions calls
	existsCacheMutex deadlock.RWMutex
	existsCache      bool
	existsCacheTime  time.Time
	existsCacheTTL   time.Duration

	// Control mode streaming infrastructure (replaces pipe-pane + FIFO)
	controlModeCmd         *exec.Cmd              // tmux -C attach process
	controlModeStdout      io.ReadCloser          // stdout pipe for control mode notifications
	controlModeStdin       io.WriteCloser         // stdin pipe for control mode commands
	controlModeDone        chan struct{}          // Signal channel for control mode termination
	controlModeSubscribers map[string]chan []byte // WebSocket clients subscribed to control mode updates
	controlModeSubMu       deadlock.RWMutex       // Protects controlModeSubscribers, controlModeExited, and pendingCmds
	controlModeExited      bool                   // True after readControlModeOutput exits; new subscribers get pre-closed channel

	// Control mode command dispatch — priority queue
	// A dedicated sender goroutine owns the stdin write path so that high-priority
	// requests (interactive user input) always jump ahead of low-priority ones
	// (background polling, resize, capture-pane). The goroutine drains highPriSendCh
	// before touching normPriSendCh.
	highPriSendCh  chan cmSendReq   // user send-keys — processed before normPriSendCh
	normPriSendCh  chan cmSendReq   // background commands (polling, resize, capture-pane)
	cmSenderExited chan struct{}    // closed when runCMSender exits; lets StopControlMode know stdin is safe to close
	cmdSendMu      deadlock.Mutex   // guards stdin-close in StopControlMode vs sender goroutine writes
	pendingCmds    []chan cmdResult // FIFO of pending response channels; protected by controlModeSubMu
	cmdBodyBuf     strings.Builder  // body accumulator between %begin and %end; reader goroutine only
	curCmdCh       chan cmdResult   // current in-flight response channel; reader goroutine only
	inCmdResp      bool             // true while inside a %begin/%end block; reader goroutine only

	// Exit detection: fired when the session exits unexpectedly (not via StopControlMode).
	// onExit is called at most once per TmuxSession lifetime (guarded by onExitOnce).
	// intentionalStop distinguishes operator-initiated StopControlMode() from crashes.
	// Must not be called while stateMutex (on the owning Instance) is held.
	onExit          func(reason string)
	onExitOnce      sync.Once
	intentionalStop atomic.Bool
}

// windowSize represents terminal dimensions from external sources (like BubbleTea)
type windowSize struct {
	cols int
	rows int
}

const TmuxPrefix = "staplersquad_"
const LegacyTmuxPrefix = "claudesquad_"

// Timeout and interval constants for session lifecycle operations.
const (
	sessionExistsTimeout        = 3 * time.Second
	sessionExistsNoCacheTimeout = 5 * time.Second
	existsCacheDefaultTTL       = 5 * time.Second // registry fast-path is push-based; this is only the subprocess fallback
	sessionCreateTimeout        = 10 * time.Second
	sessionPollInitialDelay     = 5 * time.Millisecond
)

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// recoveryMu and recoveryInFlight guard against concurrent tmux server recovery attempts.
// When the server dies all sessions detect the failure simultaneously; only one should
// run EnsureServerRunning + ResetAll + CreateKeepaliveSession.
var (
	recoveryMu       deadlock.Mutex
	recoveryInFlight bool
)

// ToStaplerSquadTmuxName converts a string to a valid tmux session name with the default prefix
func ToStaplerSquadTmuxName(str string) string {
	return toStaplerSquadTmuxNameWithPrefix(str, TmuxPrefix)
}

func toStaplerSquadTmuxNameWithPrefix(str string, prefix string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_") // tmux replaces all . with _
	str = strings.ReplaceAll(str, ":", "_") // colons are special in tmux (session:window.pane)
	return fmt.Sprintf("%s%s", prefix, str)
}

// serverNotRunning returns true if the combined output of a failed tmux command
// indicates the tmux server process is not running (as opposed to a session not found).
func serverNotRunning(output []byte) bool {
	s := strings.ToLower(string(output))
	return strings.Contains(s, "no server running") || strings.Contains(s, "error connecting to")
}

// SetOnExitCallback registers a function called when the session exits unexpectedly.
// The callback fires at most once per TmuxSession lifetime (guarded by sync.Once).
// It is NOT called when StopControlMode() is the cause of the exit.
// The callback must not be called while the owning Instance's stateMutex is held.
func (t *TmuxSession) SetOnExitCallback(fn func(reason string)) {
	t.onExit = fn
}

// GetSanitizedName returns the tmux session name as it appears in `tmux list-sessions`.
// Used for bulk reconciliation against ListAllSessions output.
func (t *TmuxSession) GetSanitizedName() string {
	return t.sanitizedName
}

// ResetExitOnce resets the exit callback so it can fire again after a session restart.
// Also clears intentionalStop so the next StopControlMode() correctly guards the callback.
// Call this before reusing a TmuxSession object for a restarted session.
func (t *TmuxSession) ResetExitOnce() {
	t.onExitOnce = sync.Once{}
	t.intentionalStop.Store(false)
}

// IsServerDown returns true if the tmux server is not running for the given socket.
// Returns false if the server state cannot be determined (treats unknown as up to
// avoid false-positive zombie recovery suppression).
func IsServerDown(serverSocket string) bool {
	return checkServerNotRunning(serverSocket)
}

// ErrServerDown is returned by ListAllSessions when the tmux server is not running.
// Callers should treat this as "no sessions are alive" without attempting recovery.
var ErrServerDown = errors.New("tmux server not running")

// ListAllSessions returns the set of all currently live tmux session names.
// Uses serverSocket for isolation if non-empty (same -L flag semantics as TmuxSession).
// Does NOT go through the per-session existence cache - intended for bulk reconciliation.
// Returns ErrServerDown when the tmux server is not running.
func ListAllSessions(serverSocket string) (map[string]bool, error) {
	args := prependSocket(serverSocket, []string{"list-sessions", "-F", "#{session_name}"})
	listCtx, listCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer listCancel()
	cmd := exec.CommandContext(listCtx, Binary(), args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		// Collect stderr for server-down detection
		combinedOutput := []byte(err.Error())
		if exitErr, ok := err.(*exec.ExitError); ok {
			combinedOutput = append(combinedOutput, exitErr.Stderr...)
		}
		if serverNotRunning(combinedOutput) {
			return nil, ErrServerDown
		}
		return nil, fmt.Errorf("ListAllSessions: %w", err)
	}

	sessions := make(map[string]bool)
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" {
			sessions[name] = true
		}
	}
	return sessions, nil
}

// checkServerNotRunning runs tmux list-sessions directly (bypassing any circuit breaker)
// and returns true if the server is not running.
func checkServerNotRunning(serverSocket string) bool {
	args := prependSocket(serverSocket, []string{"list-sessions"})
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer checkCancel()
	cmd := exec.CommandContext(checkCtx, Binary(), args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	return err != nil && serverNotRunning(out)
}

// prependSocket prepends "-L <socket>" to args when socket is non-empty.
// This lets package-level tmux functions target an isolated server socket
// (used in tests) without modifying the args slice in place.
func prependSocket(socket string, args []string) []string {
	if socket == "" {
		return args
	}
	return append([]string{"-L", socket}, args...)
}

// EnsureServerRunning starts the tmux server if it is not already running.
// Uses exec.Command directly so it always runs regardless of circuit breaker state.
func EnsureServerRunning(serverSocket string) error {
	if !checkServerNotRunning(serverSocket) {
		return nil // server is already running
	}
	args := prependSocket(serverSocket, []string{"start-server"})
	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()
	cmd := exec.CommandContext(startCtx, Binary(), args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux start-server failed: %w (output: %s)", err, out)
	}
	log.InfoLog.Printf("[tmux] server started successfully")
	return nil
}

// ensureServerRunning is a package-level variable holding the function called by
// recoverFromServerFailure. Tests can replace it to inject a controlled failure
// without depending on real tmux socket behavior.
var ensureServerRunning = EnsureServerRunning

// onServerRecovered is called after a successful tmux server recovery.
// Wired by the server layer to notify connected clients. Safe to leave nil.
var onServerRecovered func()

// SetServerRecoveryCallback registers a function called after successful server recovery.
// Thread-safe: the callback executes outside the recoveryMu lock, in a goroutine.
func SetServerRecoveryCallback(fn func()) {
	onServerRecovered = fn
}

// SetExitEmpty sets the tmux server-level exit-empty option.
// When enabled=false, the server stays alive even when all sessions are closed.
// Requires the server to already be running.
func SetExitEmpty(serverSocket string, enabled bool) error {
	value := "off"
	if enabled {
		value = "on"
	}
	args := prependSocket(serverSocket, []string{"set-option", "-g", "exit-empty", value})
	optCtx, optCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer optCancel()
	cmd := exec.CommandContext(optCtx, Binary(), args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux set-option exit-empty %s failed: %w (output: %s)", value, err, out)
	}
	return nil
}

// CreateKeepaliveSession creates a hidden tmux session that keeps the server alive.
// The session runs an idle shell and is intentionally never cleaned up by stapler-squad.
// As long as this session exists, the tmux server cannot exit due to having no sessions.
func CreateKeepaliveSession(serverSocket string) error {
	keepaliveName := TmuxPrefix + "keepalive"

	// Check if already exists
	hasArgs := prependSocket(serverSocket, []string{"has-session", "-t", keepaliveName})
	hasCtx, hasCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer hasCancel()
	hasCmd := exec.CommandContext(hasCtx, Binary(), hasArgs...)
	hasCmd.WaitDelay = 2 * time.Second
	if hasCmd.Run() == nil {
		return nil // already exists
	}

	// Create a detached session with an idle shell
	newArgs := prependSocket(serverSocket, []string{"new-session", "-d", "-s", keepaliveName})
	newCtx, newCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer newCancel()
	cmd := exec.CommandContext(newCtx, Binary(), newArgs...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create keepalive session: %w (output: %s)", err, out)
	}
	log.InfoLog.Printf("[tmux] keepalive session '%s' created", keepaliveName)
	return nil
}

// CleanupFunc represents a cleanup function that should be deferred
type CleanupFunc func() error

// tmuxCircuitBreakerConfig returns a circuit breaker config tuned for tmux commands.
// The circuit breaker's sole purpose is detecting tmux server unavailability so the
// application stops hammering a dead server. It must NOT trip on per-target errors
// (e.g. "can't find pane", "session not found") because those are session-level
// failures — the server is healthy and the caller handles them directly.
// Applying serverNotRunning to all command classes prevents premature trips from
// capture-pane or display-message failures when a watched session has simply exited.
func tmuxCircuitBreakerConfig() executor.CircuitBreakerConfig {
	defaults := executor.DefaultCircuitBreakerConfig()
	return executor.CircuitBreakerConfig{
		FailureThreshold:   defaults.FailureThreshold,
		RecoveryTimeout:    defaults.RecoveryTimeout,
		MaxRecoveryTimeout: defaults.MaxRecoveryTimeout,
		IsFailure: func(commandClass string, output []byte, err error) bool {
			if err == nil {
				return false
			}
			// Only trip the breaker when the tmux server itself is down.
			// Per-target errors (pane/session not found, invalid flag, etc.)
			// are not server failures and must not open the circuit.
			return serverNotRunning(output)
		},
	}
}

// NewTmuxSession creates a new TmuxSession with the given name and program.
// The executor is wrapped with a CircuitBreakerExecutor for resilience.
func NewTmuxSession(name string, program string) *TmuxSession {
	baseExec := executor.MakeTimeoutExecutor(5 * time.Second)
	cbExec := executor.NewCircuitBreakerExecutor(baseExec, tmuxCircuitBreakerConfig())
	key := "tmux-" + name
	executor.GetGlobalRegistry().Register(key, cbExec)
	s := newTmuxSession(name, program, MakePtyFactory(), cbExec, TmuxPrefix)
	s.registryKey = key
	return s
}

// NewTmuxSessionWithPrefix creates a new TmuxSession with a custom prefix for process isolation.
// The executor is wrapped with a CircuitBreakerExecutor for resilience.
func NewTmuxSessionWithPrefix(name string, program string, prefix string) *TmuxSession {
	baseExec := executor.MakeTimeoutExecutor(5 * time.Second)
	cbExec := executor.NewCircuitBreakerExecutor(baseExec, tmuxCircuitBreakerConfig())
	key := "tmux-" + name
	executor.GetGlobalRegistry().Register(key, cbExec)
	s := newTmuxSession(name, program, MakePtyFactory(), cbExec, prefix)
	s.registryKey = key
	return s
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
// prefix: session name prefix (e.g., "staplersquad_test_")
func NewTmuxSessionWithServerSocket(name string, program string, prefix string, serverSocket string, opts ...TmuxSessionOption) *TmuxSession {
	baseExec := executor.MakeExecutor()
	cbExec := executor.NewCircuitBreakerExecutor(baseExec, tmuxCircuitBreakerConfig())
	key := "tmux-" + name
	executor.GetGlobalRegistry().Register(key, cbExec)
	s := newTmuxSessionWithSocket(name, program, MakePtyFactory(), cbExec, prefix, serverSocket, opts...)
	s.registryKey = key
	return s
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
// WithRegistry(nil) is passed so DoesSessionExist() uses cmdExec (the mock) instead of the
// global TmuxServerRegistry, which connects to real tmux and would bypass the mock executor.
func NewTmuxSessionWithDeps(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor) *TmuxSession {
	return newTmuxSessionWithSocket(name, program, ptyFactory, cmdExec, TmuxPrefix, "", WithRegistry(nil))
}

func newTmuxSession(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor, prefix string) *TmuxSession {
	return newTmuxSessionWithSocket(name, program, ptyFactory, cmdExec, prefix, "")
}

// TmuxSessionOption is a functional option for TmuxSession construction.
type TmuxSessionOption func(*TmuxSession)

// WithRegistry injects a SessionExistenceChecker; used in tests to avoid
// the global GetServerRegistry accessor. Passing nil suppresses the
// automatic GetServerRegistry call so no reconnect loop is started.
func WithRegistry(r SessionExistenceChecker) TmuxSessionOption {
	return func(t *TmuxSession) {
		t.registry = r
		t.registryExplicit = true
	}
}

// newTmuxSessionWithSocket creates a TmuxSession with both prefix and server socket isolation
func newTmuxSessionWithSocket(name string, program string, ptyFactory PtyFactory, cmdExec executor.Executor, prefix string, serverSocket string, opts ...TmuxSessionOption) *TmuxSession {
	s := &TmuxSession{
		sanitizedName:    toStaplerSquadTmuxNameWithPrefix(name, prefix),
		program:          program,
		serverSocket:     serverSocket,
		ptyFactory:       ptyFactory,
		cmdExec:          cmdExec,
		bannerFilter:     NewBannerFilter(),         // Initialize banner filter for terminal output filtering
		externalResizeCh: make(chan windowSize, 10), // Buffered channel for resize events
		existsCacheTTL:   existsCacheDefaultTTL,
	}
	s.lastKnownCols.Store(defaultAttachCols)
	s.lastKnownRows.Store(defaultAttachRows)
	// Apply opts first so WithRegistry can set registryExplicit before we
	// call GetServerRegistry (which starts a background reconnect loop).
	for _, opt := range opts {
		opt(s)
	}
	// Inject the server-level registry only when no explicit registry was provided.
	// This prevents an unwanted reconnect loop when WithRegistry(nil) is passed for
	// isolated sockets that have no keepalive session.
	if !s.registryExplicit {
		s.registry = GetServerRegistry(serverSocket)
	}
	return s
}

// NewTmuxSessionFromExisting creates a TmuxSession that wraps an existing tmux session by its exact name.
// Unlike other constructors, this does NOT add any prefix to the session name - it uses the name exactly as provided.
// This is used for external sessions discovered via mux socket monitoring that already have tmux sessions.
//
// The session must already exist in tmux. Call AttachToExisting() after creation to establish the PTY connection.
func NewTmuxSessionFromExisting(exactSessionName string) *TmuxSession {
	baseExec := executor.MakeExecutor()
	cbExec := executor.NewCircuitBreakerExecutor(baseExec, tmuxCircuitBreakerConfig())
	key := "tmux-ext-" + exactSessionName
	executor.GetGlobalRegistry().Register(key, cbExec)
	s := &TmuxSession{
		sanitizedName:    exactSessionName, // Use exact name - no prefix transformation
		program:          "",               // Unknown - external session
		serverSocket:     "",               // Use default server
		ptyFactory:       MakePtyFactory(),
		cmdExec:          cbExec,
		registryKey:      key,
		bannerFilter:     NewBannerFilter(),
		externalResizeCh: make(chan windowSize, 10),
		existsCacheTTL:   existsCacheDefaultTTL,
	}
	s.lastKnownCols.Store(defaultAttachCols)
	s.lastKnownRows.Store(defaultAttachRows)
	return s
}

// AttachToExisting connects to an already-running tmux session and establishes the PTY connection.
// This is similar to RestoreWithWorkDir but assumes the session definitely exists.
// Returns an error if the session doesn't exist or PTY connection fails.
func (t *TmuxSession) AttachToExisting() error {
	// Verify the session exists
	if !t.DoesSessionExist() {
		return fmt.Errorf("tmux session '%s' does not exist", t.sanitizedName)
	}

	// Create PTY connection via tmux attach-session
	if t.ptmx == nil {
		ptmx, cmd, err := t.ptyFactory.Start(t.buildAttachCommand())
		if err != nil {
			return fmt.Errorf("failed to attach PTY to session '%s': %w", t.sanitizedName, err)
		}
		t.ptmx = ptmx
		t.attachCmd = cmd // CRITICAL: Save command so we can kill it on cleanup
		log.InfoLog.Printf("Successfully attached PTY to existing tmux session '%s' (pid=%d)", t.sanitizedName, cmd.Process.Pid)
	}

	// Set up status monitor
	t.monitor = newStatusMonitor()

	return nil
}

// buildTmuxCommand creates a tmux command with proper server isolation.
// If serverSocket is set, adds -L flag for complete server isolation.
// The returned command has no context; callers that need timeout protection
// should use exec.CommandContext directly or wrap with a TimeoutExecutor.
func (t *TmuxSession) buildTmuxCommand(args ...string) *exec.Cmd {
	var cmdArgs []string

	// Add server socket isolation if specified
	if t.serverSocket != "" {
		cmdArgs = append(cmdArgs, "-L", t.serverSocket)
	}

	// Add the actual tmux command arguments
	cmdArgs = append(cmdArgs, args...)

	// Use background context; callers supply their own timeout via the executor layer.
	return exec.CommandContext(context.Background(), Binary(), cmdArgs...)
}

// buildAttachCommand creates a tmux attach-session command for PTY operations.
// Note: -x/-y are NOT passed here; for attach-session -x means read-only mode
// (not width), and tmux infers dimensions from the PTY itself.
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
	// Use a no-cache check here to detect stale sessions from previous server runs.
	// The registry only tracks sessions from the current run, so a session left over
	// from a crashed/restarted server would not be in the registry and DoesSessionExist()
	// would return false, causing new-session to fail with "duplicate session".
	if t.DoesSessionExistNoCache() {
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

	// Create a new detached tmux session and start the program in it.
	// Pass -e CLAUDECODE= to unset CLAUDECODE in the child environment so that
	// nested Claude Code sessions are not blocked by the "nested session" guard.
	historyPath := fmt.Sprintf("%s/.stapler_squad_history", workDir)
	programWithHistory := fmt.Sprintf("env HISTFILE=%s %s", historyPath, t.program)
	cmd := t.buildTmuxCommand("new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE=", "-c", workDir, programWithHistory)

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

	// Invalidate cache so the poll loop gets a fresh check immediately.
	// The pre-creation DoesSessionExist() call above caches a "false" result,
	// and the 500ms cache TTL would otherwise cause the first 500ms of the
	// timeout window to be wasted on stale data.
	t.invalidateExistsCache()

	// Fast path: confirm session existence directly via list-sessions before entering
	// the poll loop. The push-based registry can lag behind tmux reality (the
	// %session-created event arrives asynchronously), so using the registry alone
	// causes poll-loop timeouts when the event is delayed. A single no-cache check
	// right after successful new-session avoids the 10s wait in the common case.
	if t.DoesSessionExistNoCache() {
		t.invalidateExistsCache()
	} else {
		// Fall back to the poll loop for the rare case where the session isn't
		// immediately visible (e.g. tmux server under heavy load).
		// sessionCreateTimeout gives enough headroom when the tmux server is under load
		// from multiple active sessions (ReviewQueuePoller, control-mode streaming, etc.).
		timeout := time.After(sessionCreateTimeout)
		sleepDuration := sessionPollInitialDelay
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
		// Session confirmed by poll loop, invalidate cache for fresh state
		t.invalidateExistsCache()
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
	log.InfoLog.Printf("Tmux session '%s' created successfully, launching: %s", t.sanitizedName, programWithHistory)
	return nil
}

// Restore attaches to an existing session and restores the window size
func (t *TmuxSession) Restore() error {
	return t.RestoreWithWorkDir("")
}

func (t *TmuxSession) RestoreWithWorkDir(workDir string) error {
	// First check if the session actually exists
	// Try multiple times with increasing delays to handle slow tmux startup or temporary unavailability
	const maxRetries = 5
	sessionExists := false
	for i := 0; i < maxRetries; i++ {
		if t.DoesSessionExist() {
			sessionExists = true
			break
		}
		if i < maxRetries-1 {
			// Wait before retrying (exponential backoff: 100ms, 200ms, 400ms, 800ms)
			delay := time.Duration(100*(1<<uint(i))) * time.Millisecond
			log.InfoLog.Printf("Tmux session '%s' not found on attempt %d/%d, waiting %v before retry", t.sanitizedName, i+1, maxRetries, delay)
			time.Sleep(delay)
			t.invalidateExistsCache() // Clear cache before retry
		}
	}

	if !sessionExists {
		// Session doesn't exist after multiple retries
		// CRITICAL: One final check without cache before recreating to prevent accidental destruction
		log.InfoLog.Printf("Tmux session '%s' not found after %d cached checks, performing final non-cached verification", t.sanitizedName, maxRetries)
		finalCheck := t.DoesSessionExistNoCache()

		if finalCheck {
			// Session actually exists - cache was stale or timing issue
			log.InfoLog.Printf("Found existing tmux session '%s' on final non-cached check (cache was stale), will reattach to preserve history", t.sanitizedName)
			// Continue with PTY attachment below (session exists, just wasn't detected earlier)
		} else {
			// Session truly doesn't exist after all checks - safe to create new one
			log.WarningLog.Printf("Tmux session '%s' doesn't exist after %d attempts plus final verification, creating new session instead of restoring", t.sanitizedName, maxRetries)

			// Use the provided working directory, fall back to current directory if not provided
			if workDir == "" {
				var err error
				workDir, err = os.Getwd()
				if err != nil {
					log.WarningLog.Printf("Could not get working directory for session '%s': %v", t.sanitizedName, err)
					workDir = "."
				}
			}

			// Create a new detached tmux session directly (avoid recursive call to Start).
			// Pass -e CLAUDECODE= to unset CLAUDECODE in the child environment so that
			// nested Claude Code sessions are not blocked by the "nested session" guard.
			cmd := t.buildTmuxCommand("new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE=", "-c", workDir, t.program)
			err := t.cmdExec.Run(cmd)
			if err != nil {
				// Session creation failed - but it might be because the session already exists
				// (DoesSessionExist may have timed out and returned false incorrectly)
				// Invalidate cache and re-check before returning error
				t.invalidateExistsCache()
				if t.DoesSessionExist() {
					// Session actually exists - the initial check was wrong (likely timeout)
					// Continue with restore instead of returning error
					log.InfoLog.Printf("Tmux session '%s' already exists (initial check was incorrect), continuing with restore", t.sanitizedName)
				} else {
					return fmt.Errorf("failed to create tmux session '%s': %w", t.sanitizedName, err)
				}
			} else {
				log.InfoLog.Printf("Created new tmux session '%s' in directory '%s', launching: %s", t.sanitizedName, workDir, t.program)
				t.invalidateExistsCache() // Session was created, invalidate cache
				// new-session started the tmux server; reset this session's circuit breakers
				// so subsequent DoesSessionExist() calls can verify the session is running.
				if r, ok := t.cmdExec.(executor.Resettable); ok {
					r.Reset()
				}
			}
		}
	} else {
		log.InfoLog.Printf("Found existing tmux session '%s', will reattach to preserve history", t.sanitizedName)
	}

	// Session exists - create PTY connection for detached operations
	// This is needed for SetDetachedSize(), SendKeys(), and the Direct Claude Command Interface
	// We use tmux attach-session to get a PTY handle without actually attaching interactively
	if t.ptmx == nil {
		const ptyMaxRetries = 3
		var lastPTYErr error
		for attempt := 0; attempt < ptyMaxRetries; attempt++ {
			if attempt > 0 {
				delay := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
				log.InfoLog.Printf("Retrying PTY attach for session '%s' (attempt %d/%d, waiting %v)", t.sanitizedName, attempt+1, ptyMaxRetries, delay)
				time.Sleep(delay)
			}
			ptmx, attachCmd, err := t.ptyFactory.Start(t.buildAttachCommand())
			if err != nil {
				lastPTYErr = err
				continue
			}
			t.ptmx = ptmx
			t.attachCmd = attachCmd // CRITICAL: track so it can be killed on cleanup
			log.InfoLog.Printf("Successfully restored PTY connection for tmux session '%s'", t.sanitizedName)
			lastPTYErr = nil
			break
		}
		if lastPTYErr != nil {
			// Graceful degradation - session can still be viewed via tmux capture-pane,
			// but PTY-based operations (resizing, SendKeys, controller) will be unavailable.
			log.WarningLog.Printf("PTY initialization failed for session '%s' after %d attempts: %v", t.sanitizedName, ptyMaxRetries, lastPTYErr)
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
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool, content string) {
	content, err := t.CapturePaneContent()
	if err != nil {
		log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		return false, false, ""
	}

	// Filter out the tmux status line (bottom line with clock) before checking for updates
	// The status line updates every second and causes false positive update detection
	contentWithoutStatusLine := t.filterStatusLine(content)

	// Only set hasPrompt for claude and aider. Use these strings to check for a prompt.
	hasPrompt = t.detectPromptInContent(contentWithoutStatusLine)

	if !bytes.Equal(t.monitor.hash(contentWithoutStatusLine), t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = t.monitor.hash(contentWithoutStatusLine)
		return true, hasPrompt, content
	}
	return false, hasPrompt, content
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

	// Use centralized PTY + attach process cleanup
	errs = append(errs, t.closePTYAndAttachCmd()...)

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
	defer func() {
		t.detaching = false
	}()

	// Use centralized PTY + attach process cleanup
	cleanupErrs := t.closePTYAndAttachCmd()
	if len(cleanupErrs) > 0 {
		// Check if this is a "file already closed" error, which can happen due to race conditions
		isFatalError := false
		for _, cleanupErr := range cleanupErrs {
			if !strings.Contains(cleanupErr.Error(), "file already closed") {
				isFatalError = true
				break
			}
		}

		if isFatalError {
			// This is a fatal error. We can't detach if we can't close the PTY properly.
			msg := fmt.Sprintf("error closing attach pty session: %v", cleanupErrs)
			log.ErrorLog.Println(msg)
			panic(msg)
		} else {
			// All errors are "file already closed" - expected in race conditions
			log.InfoLog.Printf("PTY already closed during detach (expected in concurrent scenarios)")
		}
	}

	// Attach goroutines should die on EOF due to the ptmx closing. Call
	// t.Restore to set a new t.ptmx.
	if restoreErr := t.Restore(); restoreErr != nil {
		// This is a fatal error. Our invariant that a started TmuxSession always has a valid ptmx is violated.
		msg := fmt.Sprintf("error restoring session after detach: %v", restoreErr)
		log.ErrorLog.Println(msg)
		panic(msg)
	}

	// Cancel goroutines created by Attach.
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	if t.wg != nil {
		t.wg.Wait()
		t.wg = nil
	}

	t.ctx = nil

	// Safely close attachCh only if it exists and isn't already closed
	// This is done after goroutines finish tearing down to match original behavior.
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
}

// closePTYAndAttachCmd closes the PTY file descriptor and kills the attach process.
// CRITICAL: This must be called whenever closing a PTY to prevent orphaned tmux attach processes.
// Returns a slice of errors encountered during cleanup (for aggregation in calling functions).
func (t *TmuxSession) closePTYAndAttachCmd() []error {
	var errs []error

	// Close PTY file descriptor
	if t.ptmx != nil {
		if err := t.ptmx.Close(); err != nil {
			// Only log error if it's not "file already closed" (common in concurrent scenarios)
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}
		}
		t.ptmx = nil
	}

	// CRITICAL: Kill the tmux attach-session process to prevent PTY leak
	// Closing the PTY FD does NOT kill the process - it keeps running and consuming PTYs
	if t.attachCmd != nil && t.attachCmd.Process != nil {
		log.InfoLog.Printf("Killing orphaned tmux attach process for '%s' (pid=%d)", t.sanitizedName, t.attachCmd.Process.Pid)
		if err := t.attachCmd.Process.Kill(); err != nil {
			// Process may already be dead, only log if it's a real error
			if !strings.Contains(err.Error(), "process already finished") && !strings.Contains(err.Error(), "no such process") {
				errs = append(errs, fmt.Errorf("error killing attach process: %w", err))
			}
		}
		// Wait for process to be reaped to avoid zombies
		_ = t.attachCmd.Wait()
		t.attachCmd = nil
	}

	return errs
}

// Close terminates the tmux session and cleans up resources
func (t *TmuxSession) Close() error {
	var errs []error

	// Use centralized PTY + attach process cleanup
	errs = append(errs, t.closePTYAndAttachCmd()...)

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

	// Unregister circuit breaker from global registry to prevent stale entries
	// accumulating in ResetAll() iteration across long-lived processes.
	if t.registryKey != "" {
		executor.GetGlobalRegistry().Unregister(t.registryKey)
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
	// First resize the PTY using the existing method
	if err := t.updateWindowSize(cols, rows); err != nil {
		log.WarningLog.Printf("Failed to resize PTY for session '%s': %v", t.sanitizedName, err)
	}

	// Also resize the tmux window itself to ensure the dimensions are applied.
	colsStr := fmt.Sprintf("%d", cols)
	rowsStr := fmt.Sprintf("%d", rows)
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		if _, cmErr := t.sendCMCommand(ctx,
			"resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr); cmErr != nil {
			if log.DebugLog != nil {
				log.DebugLog.Printf("SetWindowSize CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
			}
			cmd := t.buildTmuxCommand("resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr)
			if err := t.cmdExec.Run(cmd); err != nil {
				log.ErrorLog.Printf("tmux resize-window failed for '%s': %v", t.sanitizedName, err)
				return fmt.Errorf("failed to resize tmux window: %w", err)
			}
		}
	} else {
		cmd := t.buildTmuxCommand("resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr)
		if err := t.cmdExec.Run(cmd); err != nil {
			log.ErrorLog.Printf("tmux resize-window failed for '%s': %v", t.sanitizedName, err)
			return fmt.Errorf("failed to resize tmux window: %w", err)
		}
	}

	// Store requested dimensions for future PTY attach connections (via attach-session -x/-y).
	t.lastKnownCols.Store(int32(cols))
	t.lastKnownRows.Store(int32(rows))
	return nil
}

// recoverFromServerFailure attempts to restart the tmux server and reset circuit breakers.
// It also recreates the keepalive session to prevent the server from dying again.
// For isolated servers (serverSocket != ""), no keepalive is created — the caller
// manages the server's lifecycle directly (e.g., test harnesses).
//
// Only one recovery attempt runs at a time across all sessions. Concurrent callers
// return immediately if a recovery is already in progress (the first caller handles it).
func recoverFromServerFailure(serverSocket, caller string) {
	recoveryMu.Lock()
	if recoveryInFlight {
		recoveryMu.Unlock()
		log.InfoLog.Printf("[tmux] server recovery already in progress, skipping from %s", caller)
		return
	}
	recoveryInFlight = true
	recoveryMu.Unlock()
	defer func() {
		recoveryMu.Lock()
		recoveryInFlight = false
		recoveryMu.Unlock()
	}()

	if restartErr := ensureServerRunning(serverSocket); restartErr == nil {
		log.InfoLog.Printf("[tmux] server restarted from %s, resetting circuit breakers", caller)
		executor.GetGlobalRegistry().ResetAll()
		if onServerRecovered != nil {
			go onServerRecovered()
		}
		if serverSocket == "" {
			if keepErr := CreateKeepaliveSession(serverSocket); keepErr != nil {
				log.WarningLog.Printf("[tmux] failed to recreate keepalive session: %v", keepErr)
			}
		}
	} else {
		// Log at ERROR level: if the server cannot be restarted, all session operations
		// will continue to fail until the user manually intervenes.
		// Note: individual user sessions are NOT automatically re-created after recovery;
		// they will be restarted on the next user interaction (e.g., Resume() call).
		log.ErrorLog.Printf("[tmux] failed to restart tmux server from %s: %v", caller, restartErr)
	}
}

// listSessionsRaw runs "tmux list-sessions -F #{session_name}" using the provided context.
// On circuit-breaker open it falls back to a direct exec.CombinedOutput call so that
// existence checks always work regardless of breaker state.
// Returns raw combined output and the first error encountered.
func (t *TmuxSession) listSessionsRaw(ctx context.Context) ([]byte, error) {
	var cmdArgs []string
	if t.serverSocket != "" {
		cmdArgs = []string{"-L", t.serverSocket, "list-sessions", "-F", "#{session_name}"}
	} else {
		cmdArgs = []string{"list-sessions", "-F", "#{session_name}"}
	}
	cmd := exec.CommandContext(ctx, Binary(), cmdArgs...)
	output, err := t.cmdExec.CombinedOutput(cmd)
	// If the circuit breaker is open, fall back to direct exec.
	// "No sessions" (exit 1 when server running but empty) can cause false circuit
	// breaker trips; the fallback ensures checks always work regardless of breaker state.
	if errors.Is(err, executor.ErrCircuitOpen) {
		cmd = exec.CommandContext(ctx, Binary(), cmdArgs...)
		output, err = cmd.CombinedOutput()
	}
	return output, err
}

func (t *TmuxSession) DoesSessionExist() bool {
	if t == nil {
		return false
	}

	// Fast path: use the push-based registry when it is healthy.
	// This avoids an exec.Command fork entirely.
	if t.registry != nil && t.registry.IsHealthy() {
		return t.registry.SessionExists(t.sanitizedName)
	}

	// Check cache first (read lock)
	t.existsCacheMutex.RLock()
	if time.Since(t.existsCacheTime) < t.existsCacheTTL {
		cached := t.existsCache
		t.existsCacheMutex.RUnlock()
		return cached
	}
	t.existsCacheMutex.RUnlock()

	// Cache expired or not set, get fresh data (write lock).
	// IMPORTANT: do NOT call recoverFromServerFailure while this lock is held —
	// recovery runs subprocess calls that can take seconds and would stall all
	// concurrent callers of DoesSessionExist on the same session.
	t.existsCacheMutex.Lock()

	// Double-check cache hasn't been updated by another goroutine
	if time.Since(t.existsCacheTime) < t.existsCacheTTL {
		result := t.existsCache
		t.existsCacheMutex.Unlock()
		return result
	}

	// Use list-sessions to get actual running sessions for reliable checking.
	// sessionExistsTimeout is sized to be more resilient under high system load.
	ctx, cancel := context.WithTimeout(context.Background(), sessionExistsTimeout)
	defer cancel()

	output, err := t.listSessionsRaw(ctx)

	// Check if error is due to timeout
	if ctx.Err() == context.DeadlineExceeded {
		log.WarningLog.Printf("Timeout checking if tmux session exists: %s", t.sanitizedName)
		t.existsCache = false
		t.existsCacheTime = time.Now()
		t.existsCacheMutex.Unlock()
		return false
	}

	if err != nil {
		// Detect server failure before releasing the lock so we can record the cache state,
		// then release and call recovery outside the lock (recovery is slow — subprocess calls).
		needsRecovery := t.serverSocket == "" && serverNotRunning(output)
		t.existsCache = false
		t.existsCacheTime = time.Now()
		t.existsCacheMutex.Unlock()
		if needsRecovery {
			recoverFromServerFailure(t.serverSocket, "DoesSessionExist")
		}
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

	// Update cache and release lock
	t.existsCache = exists
	t.existsCacheTime = time.Now()
	t.existsCacheMutex.Unlock()
	return exists
}

// invalidateExistsCache clears the session existence cache to force a fresh check
func (t *TmuxSession) invalidateExistsCache() {
	t.existsCacheMutex.Lock()
	defer t.existsCacheMutex.Unlock()
	t.existsCacheTime = time.Time{} // Zero time forces cache miss
}

// DoesSessionExistNoCache checks if session exists WITHOUT using cache.
// This is used for critical validation before session creation to ensure we have
// the most up-to-date information about session existence.
func (t *TmuxSession) DoesSessionExistNoCache() bool {
	if t == nil {
		return false
	}

	// Direct check without cache — use a longer timeout for critical validation.
	ctx, cancel := context.WithTimeout(context.Background(), sessionExistsNoCacheTimeout)
	defer cancel()

	output, err := t.listSessionsRaw(ctx)
	if err != nil {
		log.WarningLog.Printf("DoesSessionExistNoCache: tmux list-sessions failed: %v", err)
		// Only attempt auto-recovery for the default server (not isolated test servers).
		if t.serverSocket == "" && serverNotRunning(output) {
			recoverFromServerFailure(t.serverSocket, "DoesSessionExistNoCache")
		}
		return false
	}

	// Parse and log ALL sessions for debugging
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	log.InfoLog.Printf("DoesSessionExistNoCache: checking for '%s' in tmux sessions: %v", t.sanitizedName, sessions)

	for _, session := range sessions {
		if session == t.sanitizedName {
			return true
		}
	}

	return false
}

// RefreshClient sends a refresh signal to the tmux client,
// forcing the process running inside to redraw at current dimensions.
// This is critical after resizing to update cursor positions and line wrapping.
func (t *TmuxSession) RefreshClient() error {
	// CM path: send refresh-client over the existing control mode connection.
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		if _, cmErr := t.sendCMCommand(ctx, "refresh-client", "-t", t.sanitizedName); cmErr == nil {
			return nil
		}
		// Fall through to subprocess path if CM fails (open question: refresh-client
		// targeting the CM connection itself may behave differently).
		if log.DebugLog != nil {
			log.DebugLog.Printf("RefreshClient CM path failed for '%s'; falling back to subprocess", t.sanitizedName)
		}
	}

	// Method 1: Use refresh-client command (preferred)
	cmd := t.buildTmuxCommand("refresh-client", "-t", t.sanitizedName)
	if err := t.cmdExec.Run(cmd); err != nil {
		log.WarningLog.Printf("refresh-client failed for '%s', trying alternative: %v", t.sanitizedName, err)

		// Method 2: Send SIGWINCH via kill command
		cmd = t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName, "#{pane_pid}")
		output, err := t.cmdExec.Output(cmd)
		if err != nil {
			return fmt.Errorf("failed to get pane PID: %w", err)
		}

		panePID := strings.TrimSpace(string(output))
		winchCtx, winchCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer winchCancel()
		killCmd := exec.CommandContext(winchCtx, "kill", "-WINCH", panePID)
		killCmd.WaitDelay = 2 * time.Second
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to send SIGWINCH: %w", err)
		}
	}

	return nil
}

// cmEnabled returns true when the CM command dispatch path should be attempted.
func (t *TmuxSession) cmEnabled() bool {
	return cmCommandsEnabled.Load() && t.normPriSendCh != nil
}

// cmEnabledForBackground returns true when CM is available AND the normal-priority
// send queue has room. Background ops skip CM when the queue is backed up — they
// fall back to subprocess so that the queue stays clear for high-priority user input.
func (t *TmuxSession) cmEnabledForBackground() bool {
	return t.cmEnabled() && len(t.normPriSendCh) == 0
}

// cmCtx returns a 3-second context for use with sendCMCommand.
func cmCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

// CapturePaneContent captures the content of the tmux pane.
// When STAPLER_SQUAD_CM_COMMANDS=true and control mode is running, the query is sent
// over the control mode stdin pipe (zero new subprocesses); otherwise falls back to subprocess.
func (t *TmuxSession) CapturePaneContent() (string, error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx, "capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
		if cmErr == nil {
			return sanitizeUTF8String([]byte(body)), nil
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("CapturePaneContent CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	recordSpawn(time.Now())
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		recordFailure(time.Now())
		// Invalidate cache so TmuxAlive() returns false on the next call without
		// waiting for the 5-second TTL. This prevents repeated ERROR-level subprocess
		// failures when a session has died and the registry hasn't caught up yet.
		t.invalidateExistsCache()
		if log.WarningLog != nil {
			log.WarningLog.Printf("Failed to capture pane content for session '%s': %v", t.sanitizedName, err)
		}
		return "", fmt.Errorf("error capturing pane content for session '%s': %v", t.sanitizedName, err)
	}
	return sanitizeUTF8String(output), nil
}

// CapturePaneContentRaw captures the pane content with ANSI codes preserved and WITHOUT joining wrapped lines.
// This is essential for hybrid streaming where we need to preserve exact cursor positioning.
// The -J flag (join wrapped lines) strips cursor positioning codes, breaking TUI rendering.
func (t *TmuxSession) CapturePaneContentRaw() (string, error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx, "capture-pane", "-p", "-e", "-t", t.sanitizedName)
		if cmErr == nil {
			return sanitizeUTF8String([]byte(body)), nil
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("CapturePaneContentRaw CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-t", t.sanitizedName)
	recordSpawn(time.Now())
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		recordFailure(time.Now())
		t.invalidateExistsCache()
		if log.WarningLog != nil {
			log.WarningLog.Printf("Failed to capture raw pane content for session '%s': %v", t.sanitizedName, err)
		}
		return "", fmt.Errorf("error capturing raw pane content: %v", err)
	}
	return sanitizeUTF8String(output), nil
}

// CapturePaneContentWithOptions captures the pane content with additional options.
// start and end specify the starting and ending line numbers (use "-" for the start/end of history).
func (t *TmuxSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx, "capture-pane", "-p", "-e", "-J",
			"-S", start, "-E", end, "-t", t.sanitizedName)
		if cmErr == nil {
			return sanitizeUTF8String([]byte(body)), nil
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("CapturePaneContentWithOptions CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-S", start, "-E", end, "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux pane content with options: %v", err)
	}
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
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx,
			"display-message", "-p", "-t", t.sanitizedName, "'#{cursor_x} #{cursor_y}'")
		if cmErr == nil {
			var cx, cy int
			if _, parseErr := fmt.Sscanf(strings.TrimSpace(body), "%d %d", &cx, &cy); parseErr == nil {
				return cx, cy, nil
			}
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("GetCursorPosition CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{cursor_x} #{cursor_y}")

	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get cursor position for session '%s': %w", t.sanitizedName, err)
	}

	var cursorX, cursorY int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d %d", &cursorX, &cursorY)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse cursor position '%s': %w", string(output), err)
	}

	return cursorX, cursorY, nil
}

// GetPaneDimensions returns the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
//
// When STAPLER_SQUAD_CM_COMMANDS=true and control mode is running, the query is sent
// over the existing control mode stdin pipe (zero new subprocesses). Otherwise it falls
// back to the original subprocess path.
func (t *TmuxSession) GetPaneDimensions() (width, height int, err error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx,
			"display-message", "-p", "-t", t.sanitizedName, "'#{pane_width} #{pane_height}'")
		if cmErr == nil {
			var paneWidth, paneHeight int
			if _, parseErr := fmt.Sscanf(strings.TrimSpace(body), "%d %d", &paneWidth, &paneHeight); parseErr == nil {
				return paneWidth, paneHeight, nil
			}
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("GetPaneDimensions CM path failed for '%s': %v; falling back to subprocess", t.sanitizedName, cmErr)
		}
	}

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
	lsCtx, lsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer lsCancel()
	var cmd *exec.Cmd
	if serverSocket != "" {
		cmd = exec.CommandContext(lsCtx, Binary(), "-L", serverSocket, "ls")
	} else {
		cmd = exec.CommandContext(lsCtx, Binary(), "ls")
	}
	cmd.WaitDelay = 2 * time.Second
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
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		var killCmd *exec.Cmd
		if serverSocket != "" {
			killCmd = exec.CommandContext(killCtx, Binary(), "-L", serverSocket, "kill-session", "-t", match)
		} else {
			killCmd = exec.CommandContext(killCtx, Binary(), "kill-session", "-t", match)
		}
		killCmd.WaitDelay = 2 * time.Second
		runErr := cmdExec.Run(killCmd)
		killCancel()
		if runErr != nil {
			return fmt.Errorf("failed to kill tmux session %s: %v", match, runErr)
		}
	}
	return nil
}

// sanitizeUTF8String converts raw bytes to valid UTF-8 string, replacing invalid sequences
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
			result.WriteString("")
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

// GetPaneCurrentPath returns the current working directory of the tmux pane.
// This is used by CaptureCurrentState to persist cwd before shutdown for cold restore.
func (t *TmuxSession) GetPaneCurrentPath() (string, error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx,
			"display-message", "-p", "-t", t.sanitizedName, "#{pane_current_path}")
		if cmErr == nil {
			return strings.TrimSpace(body), nil
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("GetPaneCurrentPath CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_current_path}")

	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get pane path for session '%s': %w", t.sanitizedName, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetPanePID returns the PID of the foreground process in the pane.
// This is used by HistoryLinker to correlate open files with session records.
func (t *TmuxSession) GetPanePID() (int32, error) {
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		body, cmErr := t.sendCMCommand(ctx,
			"display-message", "-p", "-t", t.sanitizedName, "#{pane_pid}")
		if cmErr == nil {
			pidStr := strings.TrimSpace(body)
			pid, parseErr := strconv.ParseInt(pidStr, 10, 32)
			if parseErr == nil {
				return int32(pid), nil
			}
		}
		if log.DebugLog != nil {
			log.DebugLog.Printf("GetPanePID CM path failed for '%s': %v; falling back", t.sanitizedName, cmErr)
		}
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_pid}")

	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get pane PID for session '%s': %w", t.sanitizedName, err)
	}

	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid pane PID %q for session '%s': %w", pidStr, t.sanitizedName, err)
	}

	return int32(pid), nil
}
