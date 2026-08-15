package tmux

import "github.com/linkdata/deadlock"

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"golang.org/x/sync/singleflight"
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
	// attachCmdWaitOnce guards attachCmd.Wait() so it is called exactly once
	// across closePTYAndAttachCmd and the diagnostic goroutine in RestoreWithWorkDir.
	// Reset to a new *sync.Once each time a new attachCmd is assigned.
	attachCmdWaitOnce *sync.Once
	// ptmxMu guards the PTY triple: ptmx, attachCmd, and attachCmdWaitOnce, which must
	// always be read/written together (all three describe one attach "generation").
	// Leaf lock: never acquire ptmxMu while already holding controlModeSubMu,
	// controlModeStartMu, cmdSendMu, or recoveryMu. detachMutex IS held across ptmxMu
	// critical sections in both Detach() and DetachSafely() (both call
	// closePTYAndAttachCmd(), and Detach() also calls Restore() -> RestoreWithWorkDir(),
	// while still holding detachMutex) -- that nesting is fine because the order is
	// always detachMutex (outer) -> ptmxMu (inner/leaf) and never reversed; ptmxMu's own
	// critical sections never call back into detachMutex. Cross-package callers reach
	// GetPTY() via TmuxProcessManager/TmuxBackend/Instance.GetPTYReader() without holding
	// any Instance-level lock across the call (Instance.pmMu only guards lazy-init of the
	// process-manager reference, not calls through it) -- and since ptmxMu never calls back
	// into any Instance-level lock, no reversed-order deadlock is structurally possible
	// regardless of what the caller holds.
	ptmxMu deadlock.Mutex
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

	// extraEnv holds KEY=VALUE pairs injected via -e flags in the new-session command.
	extraEnv []string

	// registryKey is the key used to register this session's circuit breaker executor
	// in the global registry. Stored here so Close() can unregister it on teardown.
	registryKey string

	// lastKnownCols/Rows hold the terminal dimensions from the most recent successful
	// resize. Used to pre-size new PTY attach connections (via attach-session -x/-y)
	// so the tmux session immediately reflects the correct browser window size on
	// reconnect rather than starting at the tmux default of 80×24.
	lastKnownCols atomic.Int32
	lastKnownRows atomic.Int32

	// Session existence caching — lock-free via atomic.Value snapshot.
	// existsSF coalesces concurrent subprocess calls so only one list-sessions
	// runs at a time; no lock is held during the subprocess.
	existsCache    atomic.Value       // stores existsCacheState; zero value = cache invalid
	existsSF       singleflight.Group //nolint:exhaustruct
	existsCacheTTL time.Duration      // read-only after construction

	// noCacheSF coalesces concurrent DoesSessionExistNoCache callers (health
	// checker, hibernation sweeper, session-create retry loop, bulk restore,
	// etc.) into a single in-flight subprocess. NoCache intentionally skips
	// existsCache/existsSF for freshness, but with no coalescing at all here,
	// every concurrent caller spawned its own tmux subprocess against an
	// already-serialized single-threaded server — this preserves the
	// always-fresh contract (no TTL) while still collapsing duplicates.
	noCacheSF singleflight.Group //nolint:exhaustruct

	// Control mode streaming infrastructure (replaces pipe-pane + FIFO)
	controlModeCmd         *exec.Cmd              // tmux -C attach process
	controlModeStdout      io.ReadCloser          // stdout pipe for control mode notifications
	controlModeStdin       io.WriteCloser         // stdin pipe for control mode commands
	controlModeDone        chan struct{}          // Signal channel for control mode termination
	controlModeSubscribers map[string]chan []byte // WebSocket clients subscribed to control mode updates
	controlModeSubMu       sync.RWMutex           // Protects controlModeSubscribers, controlModeExited, pendingCmds, and controlModeRefCount
	controlModeExited      bool                   // True after readControlModeOutput exits; new subscribers get pre-closed channel
	controlModeStartMu     sync.Mutex             // Serializes Start/Stop so only one process starts at a time
	controlModeRefCount    int                    // Number of active Start/Stop pairs; protected by controlModeSubMu

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
	// Must not be called while mu (on the owning Instance) is held.
	onExit          func(reason string)
	onExitOnce      sync.Once
	intentionalStop atomic.Bool

	// ExtraEnv holds additional KEY=VALUE pairs to pass as -e flags to tmux new-session.
	// Used to inject per-session environment variables such as DISPLAY for VNC support.
	ExtraEnv []string
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
	sessionCreateTimeoutDefault = 10 * time.Second
	// killSessionTimeout bounds Close()'s "tmux kill-session" subprocess so a
	// hung tmux server can't block callers (e.g. Instance.Destroy(), and in
	// turn SessionService.DeleteSession's cleanup goroutine) indefinitely.
	// Matches KillTmuxSessionByTitle's existing 5s cap in server/services.
	killSessionTimeout      = 5 * time.Second
	sessionPollInitialDelay = 5 * time.Millisecond
	// sessionPollMaxDelay bounds the poll loop's exponential backoff below.
	// Previously capped at 50ms, which -- once ramped up after ~4 doublings
	// (~75ms) -- spawns a real `tmux list-sessions` subprocess (via
	// DoesSessionExistNoCache) roughly every 50ms for the rest of the
	// sessionCreateTimeout window: up to ~600 subprocess forks over a 30s
	// CI-widened timeout. Each of those is itself gated CPU/scheduling work
	// competing with the very tmux-server fork/exec it's waiting on, on the
	// same CPU-starved runner diagnosed as this loop's root timeout cause
	// (see sessionCreateTimeout's doc comment) -- i.e. the poll loop was
	// worsening the exact contention it exists to tolerate. 250ms cuts the
	// worst-case spawn count roughly 5x while staying invisible in the
	// common case, where the session is already visible within the first
	// few (5/10/20/40/80/160ms) low-delay iterations.
	sessionPollMaxDelay = 250 * time.Millisecond
)

// sessionCreateTimeout is sessionCreateTimeoutDefault unless overridden via
// STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS (unset/invalid -> default, no
// production behavior change). Exists for CI specifically: even with a
// per-test isolated tmux -L server (NewTmuxSessionWithServerSocket), a brand
// new server still has to fork and become responsive within this budget, and
// a fully-loaded CI runner (many packages' -race suites competing for CPU)
// can occasionally exceed 10s just on scheduling delay, not lock contention
// -- confirmed via TestCommitImportExternalSession_PersistsAndLinksAndSuspends_
// When_StartAndSuspendSucceed failing deterministically twice in the same CI
// run (Makefile's own coverage-then-verbose-rerun fallback) even after socket
// isolation removed cross-test contention as a cause. A var (not const) so
// it's computed once at package init, ponytail: global var read at init,
// per-process override only -- add a setter if a future caller needs to vary
// it mid-process.
var sessionCreateTimeout = func() time.Duration {
	if raw := os.Getenv("STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return sessionCreateTimeoutDefault
}()

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// existsCacheState is the immutable snapshot stored in TmuxSession.existsCache.
type existsCacheState struct {
	exists bool
	time   time.Time
}

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

// SessionName is a sanitized tmux session identifier — the only string form that is
// safe to pass to tmux's "-t" flag. The field is unexported so the only way to obtain
// one outside this package is NewSessionName: holding a SessionName proves the raw
// title has already been through sanitization, so callers never need to re-derive or
// re-sanitize it themselves.
//
// This exists because tmux session names were historically re-derived ad hoc at each
// call site (creation, streaming, approval matching) using slightly different logic,
// which silently drifted out of sync whenever a title contained whitespace (see #162:
// a session was created as "staplersquad_CareerGrowth" but addressed as
// "staplersquad_Career Growth", making it permanently uncontrollable).
type SessionName struct{ value string }

// NewSessionName sanitizes a raw instance title into the tmux session name that was
// (or will be) used to create the session. Always call this instead of concatenating
// prefix+title — it is the single source of truth for the sanitization rules.
func NewSessionName(title, prefix string) SessionName {
	return SessionName{value: toStaplerSquadTmuxNameWithPrefix(title, prefix)}
}

func (n SessionName) String() string { return n.value }

// serverNotRunning returns true if the combined output of a failed tmux command
// indicates the tmux server process is not running (as opposed to a session not found).
// "server exited unexpectedly" is produced by tmux when the client connects to a stale
// socket whose server process is already dead — this is a server-level failure, not a
// per-session one. recoverFromServerFailure re-verifies with a fresh list-sessions before
// restarting, so a transient false positive on a single session won't trigger a restart.
func serverNotRunning(output []byte) bool {
	s := strings.ToLower(string(output))
	return strings.Contains(s, "no server running") ||
		strings.Contains(s, "error connecting to") ||
		strings.Contains(s, "server exited unexpectedly")
}

// SetOnExitCallback registers a function called when the session exits unexpectedly.
// The callback fires at most once per TmuxSession lifetime (guarded by sync.Once).
// It is NOT called when StopControlMode() is the cause of the exit.
// The callback must not be called while the owning Instance's mu is held.
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
// Resets control mode refcount/cmd/exited so a stale dead process left by a prior crash
// does not corrupt the next Start/Stop cycle.
// Call this before reusing a TmuxSession object for a restarted session.
func (t *TmuxSession) ResetExitOnce() {
	t.onExitOnce = sync.Once{}
	t.intentionalStop.Store(false)
	t.controlModeSubMu.Lock()
	t.controlModeRefCount = 0
	t.controlModeCmd = nil
	t.controlModeExited = false
	t.controlModeSubMu.Unlock()
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
	out, err := runGated(listCtx, serverSocket, func() ([]byte, error) {
		return safeexec.CommandContext(listCtx, Binary(), args...).Output()
	})
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
	out, err := runGated(checkCtx, serverSocket, func() ([]byte, error) {
		return safeexec.CommandContext(checkCtx, Binary(), args...).CombinedOutput()
	})
	return err != nil && serverNotRunning(out)
}

// testSocketOnce lazily computes the per-process isolated socket name for test
// binaries, computed once and reused for the lifetime of the process so that all
// isolated tmux calls within one `go test` binary land on the same server.
var testSocketOnce = sync.OnceValue(func() string {
	return fmt.Sprintf("test-isolated-%d", os.Getpid())
})

// Socket identifies which tmux server a command targets. The zero value ("")
// means the real, shared default server. The only way to obtain a non-trivial
// Socket is through ResolveSocket -- holding one proves resolution (including
// test-mode isolation) already happened, so callers building tmux argv via
// Args never need to re-derive or re-check isolation themselves.
//
// This is a plain newtype, not an opaque struct: many callers legitimately
// need the socket name as a string too (struct fields for UI display, log
// lines, equality checks against ""), and forcing a conversion at every one
// of those sites would fight the pattern instead of guiding it. Args is the
// one sanctioned way to turn a Socket into a tmux command's argv; see the
// tmuxsocketscope lint pass for the structural check that every tmux
// invocation's args flow through it (or ResolveSocket/prependSocket) instead
// of a hand-rolled "-L" literal.
type Socket string

// Args prepends "-L <socket>" to args when s is a non-default socket, and
// returns args unchanged for the default server (matching production
// behavior: an unscoped call targets the real shared socket, exactly as
// before this isolation mechanism existed).
func (s Socket) Args(args ...string) []string {
	if s == "" {
		return args
	}
	return append([]string{"-L", string(s)}, args...)
}

// String returns the socket name, or "" for the default server.
func (s Socket) String() string { return string(s) }

// ResolveSocket is the single choke point between "the socket a caller asked for"
// and "the socket a tmux command actually targets." An explicit non-empty socket
// always passes through unchanged (real per-worktree/per-test isolation, or a
// caller intentionally targeting a specific server, is always honored). An empty
// socket -- historically "the real shared default socket" everywhere in this
// package, including in code that enumerates or kills ALL sessions on it
// (ReconcileOrphanedTmuxSessions, batchPaneActivity, health checks) -- resolves to
// a per-process isolated socket inside a `go test` binary instead.
//
// Before this existed, "empty string" meant the real default socket unconditionally,
// so ANY test that ended up calling a tmux-touching code path (not just tests that
// intentionally exercise tmux) could enumerate and kill every real session on a
// developer's machine, including sessions from an entirely separate, currently
// running production stapler-squad process. That happened repeatedly in production
// incidents traced to nothing more than a `go test ./server/...` run elsewhere on
// the same machine. Every function below that builds a tmux invocation from a raw
// socket string must resolve it through here first -- there is intentionally no
// second, competing way to decide "which socket does this command target."
//
// This is deliberately NOT gated behind an explicit opt-in flag on the destructive
// functions themselves (the previous fix for this class of bug): a flag can be
// forgotten at any new call site. Resolving centrally, once, at the boundary where a
// caller-supplied socket turns into a real tmux invocation means every existing and
// future caller is isolated automatically, with no per-call-site action required.
func ResolveSocket(explicit string) Socket {
	if explicit != "" {
		return Socket(explicit)
	}
	if config.IsTestMode() {
		return Socket(testSocketOnce())
	}
	return ""
}

// prependSocket prepends "-L <socket>" to args after resolving socket through
// ResolveSocket. This lets package-level tmux functions target an isolated server
// socket (used in tests) without modifying the args slice in place.
func prependSocket(socket string, args []string) []string {
	return ResolveSocket(socket).Args(args...)
}

// TmuxServerReady is a zero-size proof token returned by EnsureServerRunning.
// BuildRuntimeDeps requires it as its first parameter to enforce that the tmux
// server is running before any sessions are loaded — preventing cold-restore of
// processes that are still alive inside tmux.
type TmuxServerReady struct{}

// startServerSucceededDespiteError reports whether a failed start-server attempt
// should be treated as success because a follow-up check shows a server is now
// (or still) actually running -- see EnsureServerRunning's doc comment on the
// check-race this recovers from. isNotRunning is checkServerNotRunning, passed
// in so tests can simulate the race deterministically instead of depending on
// real tmux subprocess timing.
func startServerSucceededDespiteError(isNotRunning func() bool) bool {
	return !isNotRunning()
}

// serverStartAttempts/serverStartBackoffStart/serverStartBackoffMax bound how
// long EnsureServerRunning retries a failed start-server call before
// surfacing the error. A single recheck-after-failure (the original fix)
// assumed the transient "server exited unexpectedly" condition clears in one
// shot; a full `make test` run proved that assumption wrong -- under
// sustained heavy system load (many concurrent tmux servers spawned across
// the whole suite) the condition can outlast a short fixed retry window, and
// the underlying start-server invocation can also genuinely need re-issuing,
// not just re-checked. So each attempt below re-runs start-server itself
// (not just the recheck), with exponential backoff between attempts --
// mirroring exec_gate.go's doubling-backoff pattern for the same class of
// system-load-dependent contention.
//
// A first version of these constants (5 attempts, 50ms-400ms backoff, ~750ms
// total wait) still failed under a real `make build && make test` run:
// isolated/targeted runs of this exact retry path passed repeatedly, but the
// contention window under genuine full-suite load (many packages spawning
// tmux servers concurrently) outlasted 750ms. These wider bounds (8 attempts,
// 100ms-3s backoff, ~9.1s worst-case total wait) trade a longer failure path
// for actually riding out that contention -- this function is only on the
// hot path when a server genuinely isn't running yet, so extra latency here
// is paid rarely and only in the case that used to fail outright.
const (
	serverStartAttempts     = 8
	serverStartBackoffStart = 100 * time.Millisecond
	serverStartBackoffMax   = 3 * time.Second
)

// serverStartAttempt runs one start-server invocation and reports whether the
// server ended up running -- either because the call itself succeeded, or
// because a follow-up check shows the server is now (or still) actually up
// despite a reported error (see EnsureServerRunning's doc comment on the
// check-race this recovers from). startServer and isNotRunning are injected
// so tests can simulate this deterministically instead of depending on real
// tmux subprocess timing.
func serverStartAttempt(startServer func() ([]byte, error), isNotRunning func() bool) (ok bool, out []byte, err error) {
	out, err = startServer()
	if err == nil {
		return true, out, nil
	}
	if startServerSucceededDespiteError(isNotRunning) {
		return true, out, err
	}
	return false, out, err
}

// ensureServerRunningWithRetry retries serverStartAttempt up to attempts
// times with exponential backoff (capped at backoffMax) between tries, to
// ride out a transient start-server failure that outlasts a single retry
// under sustained heavy system load. Returns the last attempt's output/error
// when every attempt fails.
func ensureServerRunningWithRetry(startServer func() ([]byte, error), isNotRunning func() bool, attempts int, backoffStart, backoffMax time.Duration) (out []byte, err error) {
	backoff := backoffStart
	for i := 0; i < attempts; i++ {
		ok, o, e := serverStartAttempt(startServer, isNotRunning)
		out, err = o, e
		if ok {
			return out, nil
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
	}
	return out, err
}

// EnsureServerRunning starts the tmux server if it is not already running.
// Uses exec.Command directly so it always runs regardless of circuit breaker state.
// Returns a TmuxServerReady token that callers must pass to BuildRuntimeDeps.
func EnsureServerRunning(serverSocket string) (TmuxServerReady, error) {
	if !checkServerNotRunning(serverSocket) {
		return TmuxServerReady{}, nil // server is already running
	}
	args := prependSocket(serverSocket, []string{"start-server"})
	startServer := func() ([]byte, error) {
		startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer startCancel()
		return runGated(startCtx, serverSocket, func() ([]byte, error) {
			return safeexec.CommandContext(startCtx, Binary(), args...).CombinedOutput()
		})
	}
	// Under heavy concurrent tmux usage, the list-sessions check above can itself
	// transiently report "server exited unexpectedly" against a socket that
	// actually has a live server (a racy connect, not a real absence) -- which
	// sends us down this path to start a server that's already running, and the
	// start-server call then hits the same transient failure. Since this
	// function's actual contract is "a server is running when this returns", not
	// "this call is the one that started it", each retry re-checks before
	// surfacing an error: if a server is now (or still) up, that's success.
	out, err := ensureServerRunningWithRetry(startServer, func() bool { return checkServerNotRunning(serverSocket) }, serverStartAttempts, serverStartBackoffStart, serverStartBackoffMax)
	if err != nil {
		return TmuxServerReady{}, fmt.Errorf("tmux start-server failed after %d attempts: %w (output: %s)", serverStartAttempts, err, out)
	}
	log.Info("[tmux] server started successfully")

	// Set the server-wide default so every session created on this server -- including
	// any path that doesn't explicitly set it per-session -- keeps its pane around when
	// the wrapped program exits instead of tmux silently destroying the whole session.
	remainArgs := prependSocket(serverSocket, []string{"set-option", "-g", "remain-on-exit", "on"})
	remainCtx, remainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer remainCancel()
	if out, err := runGated(remainCtx, serverSocket, func() ([]byte, error) {
		return safeexec.CommandContext(remainCtx, Binary(), remainArgs...).CombinedOutput()
	}); err != nil {
		log.Warn("[tmux] failed to set global remain-on-exit default", "err", err, "output", string(out))
	}

	return TmuxServerReady{}, nil
}

// KillOrphanedControlModeClients terminates every control-mode ("-C") client already
// attached to the tmux server at the moment this is called. Safe only at process
// startup, before any session has (re)started its own control mode: a freshly-started
// process cannot have spawned a control-mode client yet, so any control-mode client
// already attached is necessarily a leftover from a previous process instance that
// --tmux-keep-server intentionally kept alive across the restart (see
// docs/bugs/open/BUG-042-orphaned-control-mode-clients-overload-tmux-server.md).
// Left unreconciled, these accumulate one per restart and eventually crash the tmux
// server outright. Plain (non-control-mode) attach-session clients are left alone --
// those can be real interactive users and are not this app's to manage.
func KillOrphanedControlModeClients(serverSocket string) (int, error) {
	args := prependSocket(serverSocket, []string{"list-clients", "-F", "#{client_pid} #{client_control_mode}"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, Binary(), args...).Output()
	if err != nil {
		// No server running yet, or no clients at all -- nothing to clean up.
		return 0, nil
	}

	killed := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "1" {
			continue // not a control-mode client
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Kill(); err == nil {
			killed++
		}
	}
	if killed > 0 {
		log.Info("[tmux] killed orphaned control-mode clients from a prior process instance", "count", killed)
	}
	return killed, nil
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
	out, err := runGated(optCtx, serverSocket, func() ([]byte, error) {
		return safeexec.CommandContext(optCtx, Binary(), args...).CombinedOutput()
	})
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
	hasErr := runGatedErr(hasCtx, serverSocket, func() error {
		return safeexec.CommandContext(hasCtx, Binary(), hasArgs...).Run()
	})
	if hasErr == nil {
		return nil // already exists
	}

	// Create a detached session with an idle shell
	newArgs := prependSocket(serverSocket, []string{"new-session", "-d", "-s", keepaliveName})
	newCtx, newCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer newCancel()
	out, err := runGated(newCtx, serverSocket, func() ([]byte, error) {
		return safeexec.CommandContext(newCtx, Binary(), newArgs...).CombinedOutput()
	})
	if err != nil {
		return fmt.Errorf("failed to create keepalive session: %w (output: %s)", err, out)
	}
	log.Info("[tmux] keepalive session created", "session", keepaliveName)
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

	// For isolated server sockets (used in tests), append the socket name to the registry key
	// to prevent registration conflicts when multiple tests create sessions with the same name.
	key := "tmux-" + name
	if serverSocket != "" {
		key += "-" + serverSocket
	}

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
	// Resolve once, here, at construction -- not per-command. Every TmuxSession's
	// serverSocket is isolated automatically inside a test binary regardless of what
	// the caller passed (see ResolveSocket), so no session-creation call site anywhere
	// needs to remember to ask for isolation.
	serverSocket = ResolveSocket(serverSocket).String()
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
	// In test mode, skip this entirely to avoid flaky control-mode connections.
	if !s.registryExplicit && !config.IsTestMode() {
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
	return NewTmuxSessionFromExistingWithServerSocket(exactSessionName, "")
}

// NewTmuxSessionFromExistingWithServerSocket is like NewTmuxSessionFromExisting but targets an
// isolated tmux server socket (e.g. a shell session's TmuxServerSocket) instead of the default
// server. serverSocket is resolved through ResolveSocket, so test-mode isolation still applies
// when the caller passes "".
func NewTmuxSessionFromExistingWithServerSocket(exactSessionName string, serverSocket string) *TmuxSession {
	baseExec := executor.MakeExecutor()
	cbExec := executor.NewCircuitBreakerExecutor(baseExec, tmuxCircuitBreakerConfig())
	key := "tmux-ext-" + exactSessionName
	executor.GetGlobalRegistry().Register(key, cbExec)
	s := &TmuxSession{
		sanitizedName:    exactSessionName, // Use exact name - no prefix transformation
		program:          "",               // Unknown - external session
		serverSocket:     ResolveSocket(serverSocket).String(),
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

	// Create PTY connection via tmux attach-session.
	// Check-then-act: not atomic with respect to a concurrent AttachToExisting(),
	// RestoreWithWorkDir(), or Close() call on the same session -- each individual
	// field access below is memory-safe via ptmxMu, but two concurrent callers can
	// still both observe nil and both install a PTY. Known limitation, not a data
	// race (go test -race cannot catch it); tracked separately as backlog item
	// 0f4b1300-d667-437b-b51f-89d81a668693.
	if t.lockedPTMX() == nil {
		ptmx, cmd, err := t.ptyFactory.Start(t.buildAttachCommand())
		if err != nil {
			return fmt.Errorf("failed to attach PTY to session '%s': %w", t.sanitizedName, err)
		}
		t.setPTYTriple(ptmx, cmd, new(sync.Once)) // CRITICAL: Save command so we can kill it on cleanup
		log.Info("successfully attached PTY to existing tmux session", "session", t.sanitizedName, "pid", cmd.Process.Pid)
	}

	// Set up status monitor
	t.monitor = newStatusMonitor()

	return nil
}

// SetExtraEnv sets additional KEY=VALUE environment variable pairs to inject via
// tmux new-session -e flags. Must be called before Start().
func (t *TmuxSession) SetExtraEnv(env []string) {
	t.extraEnv = env
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
	return safeexec.CommandContext(context.Background(), Binary(), cmdArgs...)
}

// buildAttachCommand creates a tmux attach-session command for PTY operations.
// Note: -x/-y are NOT passed here; for attach-session -x means read-only mode
// (not width), and tmux infers dimensions from the PTY itself.
// TERM must be set explicitly: when the service runs headless (systemd, no
// controlling terminal), TERM is absent from the environment, which causes
// tmux to fail terminal initialization and exit immediately.
func (t *TmuxSession) buildAttachCommand() *exec.Cmd {
	cmd := t.buildTmuxCommand("attach-session", "-t", t.sanitizedName)
	if os.Getenv("TERM") == "" {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}
	return cmd
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

// setRemainOnExit keeps the pane around when its program exits instead of tmux's
// default of destroying the whole session. Without this, an unexpected exit of the
// wrapped program (OS-killed, crashed, or otherwise) silently erases the session --
// including any output that would explain why it exited -- and the only trace left
// behind is "session doesn't exist" on the next check. Called after every path that
// creates a session (fresh start, and the "recreate after not found" restore fallback).
func (t *TmuxSession) setRemainOnExit() {
	remainCmd := t.buildTmuxCommand("set-option", "-t", t.sanitizedName, "remain-on-exit", "on")
	if err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(remainCmd)
	}); err != nil {
		log.Warn("failed to set remain-on-exit for session", "session", t.sanitizedName, "err", err)
	}
}

// ErrWorkDirMissing indicates a session's working directory is unset or no
// longer exists on disk (e.g. a pruned git worktree). Callers can match on it
// with errors.Is to distinguish a permanent failure — the session should be
// failed with a clear status, not silently retried against a guessed directory.
var ErrWorkDirMissing = errors.New("session working directory missing")

// validateWorkDir rejects an empty or nonexistent working directory instead of
// letting a caller silently fall back to a guessed directory (e.g. os.Getwd(),
// which for a long-running server process is often $HOME) — see start() and
// RestoreWithWorkDir()'s recreate path.
func validateWorkDir(workDir string) error {
	if workDir == "" {
		return fmt.Errorf("working directory not set: %w", ErrWorkDirMissing)
	}
	info, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("working directory %q is not accessible: %w: %w", workDir, ErrWorkDirMissing, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %q is not a directory: %w", workDir, ErrWorkDirMissing)
	}
	return nil
}

// preconfigureServerBeforeSession sets exit-empty off and remain-on-exit on,
// in ONE chained tmux invocation, before the session in start() below is
// created -- closing a race that's always present but usually wins locally
// and loses under CI's slower, -race-instrumented syscalls:
// t.setRemainOnExit() only runs AFTER the new-session command returns AND
// existence is confirmed, but a fast-exiting program (e.g. Program="true",
// which exits in microseconds) can already have destroyed the session --
// and, since it was the server's only session, killed the server itself --
// before that point, especially on a brand-new socket with no pre-existing
// server to inherit options from.
//
// MUST be one chained invocation (`cmd1 \; cmd2 \; cmd3`), not separate
// commands run back-to-back -- confirmed empirically (PR #445): a tmux
// server that reaches zero sessions exits near-instantly by default
// (exit-empty defaults to on), so `start-server` followed by a SEPARATE
// `set-option` invocation already finds "no server running" -- the server
// that start-server just reported success for is already gone by the time
// the next process connects. Chaining into one invocation means the server
// never has a gap where it's both running and unprotected: start-server
// brings it up, exit-empty off keeps a zero-session server alive,
// remain-on-exit on then protects the session new-session (in start()) is
// about to create from destruction the instant its program exits. `;` here
// is tmux's own command-separator syntax (parsed by tmux itself from
// distinct argv elements), not a shell operator -- no shell is involved via
// exec.Cmd, so no escaping is needed or applicable.
//
// Best-effort: log and continue on failure, matching every other
// non-essential tmux option set in start() (history-limit,
// setRemainOnExit itself) -- a failure here degrades to the pre-existing
// (racy) behavior, not a hard Start() failure.
func (t *TmuxSession) preconfigureServerBeforeSession() {
	preconfigureCmd := t.buildTmuxCommand("start-server", ";", "set-option", "-g", "exit-empty", "off", ";", "set-option", "-g", "remain-on-exit", "on")
	if err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(preconfigureCmd)
	}); err != nil {
		log.Warn("failed to pre-configure tmux server before session creation", "session", t.sanitizedName, "serverSocket", t.serverSocket, "err", err)
	}
}

// start is the internal implementation for Start and StartWithCleanup
func (t *TmuxSession) start(workDir string, setupCleanup bool, cleanup *CleanupFunc) error {
	// Use a no-cache check here to detect stale sessions from previous server runs.
	// The registry only tracks sessions from the current run, so a session left over
	// from a crashed/restarted server would not be in the registry and DoesSessionExist()
	// would return false, causing new-session to fail with "duplicate session".
	if t.DoesSessionExistNoCache() {
		// Session already exists - we can reuse it
		log.Info("tmux session already exists, reusing", "session", t.sanitizedName)

		// Set up cleanup if requested
		if setupCleanup && cleanup != nil {
			*cleanup = func() error {
				return t.Close()
			}
		}

		return nil
	}

	if err := validateWorkDir(workDir); err != nil {
		return fmt.Errorf("cannot start tmux session %s: %w", t.sanitizedName, err)
	}

	t.preconfigureServerBeforeSession()

	// Create a new detached tmux session and start the program in it.
	// Pass -e CLAUDECODE= to unset CLAUDECODE in the child environment so that
	// nested Claude Code sessions are not blocked by the "nested session" guard.
	historyPath := fmt.Sprintf("%s/.stapler_squad_history", workDir)
	programWithHistory := fmt.Sprintf("env HISTFILE=%s %s", historyPath, t.program)
	newSessionArgs := []string{"new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE="}
	for _, kv := range t.ExtraEnv {
		newSessionArgs = append(newSessionArgs, "-e", kv)
	}
	for _, kv := range t.extraEnv {
		newSessionArgs = append(newSessionArgs, "-e", kv)
	}
	newSessionArgs = append(newSessionArgs, "-c", workDir, programWithHistory)
	cmd := t.buildTmuxCommand(newSessionArgs...)

	// Use cmdExec.Run() instead of pty.Start() for detached session creation
	// since detached sessions don't need PTY attachment during creation.
	//
	// stderr goes to a scratch FILE, not a pipe/buffer: `tmux new-session -d`
	// forks a detached server that inherits these fds, so a buffer-based
	// capture (CombinedOutput, bytes.Buffer) blocks forever waiting for EOF
	// that the still-running server never sends. A file has no such wait.
	var stderrOutput string
	stderrFile, tmpErr := os.CreateTemp("", "tmux-new-session-stderr-*")
	if tmpErr == nil {
		cmd.Stderr = stderrFile
		defer os.Remove(stderrFile.Name())
		defer stderrFile.Close()
	}
	err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(cmd)
	})
	if stderrFile != nil {
		if data, readErr := os.ReadFile(stderrFile.Name()); readErr == nil {
			stderrOutput = strings.TrimSpace(string(data))
		}
	}
	if err != nil {
		// Cleanup any partially created session if any exists.
		if t.DoesSessionExist() {
			cleanupCmd := t.buildTmuxCommand("kill-session", "-t", t.sanitizedName)
			cleanupErr := runGatedErr(context.Background(), t.serverSocket, func() error {
				return t.cmdExec.Run(cleanupCmd)
			})
			if cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			t.invalidateExistsCache() // Session was killed, invalidate cache
		}
		// If we have a cleanup function pointer, set it to nil since startup failed
		if setupCleanup && cleanup != nil {
			*cleanup = func() error { return nil }
		}
		if stderrOutput != "" {
			return fmt.Errorf("error starting tmux session: %s (%w)", stderrOutput, err)
		}
		return fmt.Errorf("error starting tmux session: %w", err)
	}

	// `tmux new-session -d` reported success (exit 0) at this point -- confirmed
	// unambiguously via PID and socket file existence, not just the exit code,
	// since a false-positive exit 0 with a not-yet-listening server is exactly
	// the failure mode under investigation for PR #445's CI-only flake (see
	// DoesSessionExistNoCache's expanded error log for the other half of this
	// diagnostic pair).
	log.Info("tmux new-session command succeeded", "session", t.sanitizedName, "serverSocket", t.serverSocket, "stderr", stderrOutput)

	// Invalidate cache so the poll loop gets a fresh check immediately.
	// The pre-creation DoesSessionExist() call above caches a "false" result,
	// and the 5s cache TTL would otherwise cause the first 5s of the
	// timeout window to be wasted on stale data.
	t.invalidateExistsCache()

	// Fast path: confirm session existence directly via list-sessions before entering
	// the poll loop. The push-based registry can lag behind tmux reality (the
	// %session-created event arrives asynchronously), so using the registry alone
	// causes poll-loop timeouts when the event is delayed. A single no-cache check
	// right after successful new-session avoids the 10s wait in the common case.
	if t.DoesSessionExistNoCache() {
		// Proactively update the registry so DoesSessionExist() returns true
		// immediately — the async %session-created event may not have arrived yet.
		if notifier, ok := t.registry.(interface{ NotifySessionCreated(string) }); ok {
			notifier.NotifySessionCreated(t.sanitizedName)
		}
		t.invalidateExistsCache()
		// The registry is push-based: %session-created arrives asynchronously and may
		// not be reflected yet. Wait briefly so that DoesSessionExist() (which takes
		// the registry fast path when healthy) is consistent the moment Start() returns.
		// Without this, callers see false immediately after a successful Start().
		if t.registry != nil && t.registry.IsHealthy() {
			registryDeadline := time.Now().Add(2 * time.Second)
			for !t.registry.SessionExists(t.sanitizedName) {
				if time.Now().After(registryDeadline) {
					log.Warn("registry lagged for session after creation; continuing", "session", t.sanitizedName)
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.invalidateExistsCache()
		}
	} else {
		// Fall back to the poll loop for the rare case where the session isn't
		// immediately visible (e.g. tmux server under heavy load).
		// sessionCreateTimeout gives enough headroom when the tmux server is under load
		// from multiple active sessions (ReviewQueuePoller, control-mode streaming, etc.).
		timeout := time.After(sessionCreateTimeout)
		sleepDuration := sessionPollInitialDelay
		for !t.DoesSessionExistNoCache() {
			select {
			case <-timeout:
				if cleanupErr := t.Close(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
				return fmt.Errorf("timed out waiting for tmux session %s: %v", t.sanitizedName, err)
			default:
				time.Sleep(sleepDuration)
				// Exponential backoff up to sessionPollMaxDelay.
				if sleepDuration < sessionPollMaxDelay {
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
	if err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(historyCmd)
	}); err != nil {
		log.Warn("failed to set history-limit for session", "session", t.sanitizedName, "err", err)
	}

	t.setRemainOnExit()

	// Set up monitoring for session status tracking
	t.monitor = newStatusMonitor()

	// Set up cleanup if requested
	if setupCleanup && cleanup != nil {
		*cleanup = func() error {
			return t.Close()
		}
	}

	// Session is created and ready - let the user handle any program-specific interactions
	log.Info("tmux session created successfully", "session", t.sanitizedName, "program", programWithHistory)
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
	// ponytail: caller already ran DoesSessionExistNoCache() and got false — cache is stale, flush it.
	t.invalidateExistsCache()
	sessionExists := false
	for i := 0; i < maxRetries; i++ {
		if t.DoesSessionExist() {
			sessionExists = true
			break
		}
		if i < maxRetries-1 {
			// Wait before retrying (exponential backoff: 100ms, 200ms, 400ms, 800ms)
			delay := time.Duration(100*(1<<uint(i))) * time.Millisecond
			log.Info("tmux session not found, retrying", "session", t.sanitizedName, "attempt", i+1, "maxRetries", maxRetries, "delay", delay)
			time.Sleep(delay)
			t.invalidateExistsCache() // Clear cache before retry
		}
	}

	if !sessionExists {
		// Session doesn't exist after multiple retries
		// CRITICAL: One final check without cache before recreating to prevent accidental destruction
		log.Info("tmux session not found, performing final non-cached verification", "session", t.sanitizedName, "cachedChecks", maxRetries)
		finalCheck := t.DoesSessionExistNoCache()

		if finalCheck {
			// Session actually exists - cache was stale or timing issue
			log.Info("found existing tmux session on final non-cached check (cache was stale), will reattach", "session", t.sanitizedName)
			// Continue with PTY attachment below (session exists, just wasn't detected earlier)
		} else {
			// Session truly doesn't exist after all checks - safe to create new one
			log.Warn("tmux session doesn't exist after all attempts, creating new session instead of restoring", "session", t.sanitizedName, "attempts", maxRetries)

			// ponytail: never guess a directory here (e.g. os.Getwd(), which for a
			// long-running server process is often $HOME) — a wrong guess silently
			// reconnects the session to the wrong workspace. Fail loudly instead so
			// the caller can surface a clear status to the user.
			if err := validateWorkDir(workDir); err != nil {
				return fmt.Errorf("cannot recreate tmux session %s: %w", t.sanitizedName, err)
			}

			// Create a new detached tmux session directly (avoid recursive call to Start).
			// Pass -e CLAUDECODE= to unset CLAUDECODE in the child environment so that
			// nested Claude Code sessions are not blocked by the "nested session" guard.
			restoreArgs := make([]string, 0, 6+2*len(t.ExtraEnv)+2*len(t.extraEnv)+3)
			restoreArgs = append(restoreArgs, "new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE=")
			for _, kv := range t.ExtraEnv {
				restoreArgs = append(restoreArgs, "-e", kv)
			}
			for _, kv := range t.extraEnv {
				restoreArgs = append(restoreArgs, "-e", kv)
			}
			restoreArgs = append(restoreArgs, "-c", workDir, t.program)
			cmd := t.buildTmuxCommand(restoreArgs...)
			err := runGatedErr(context.Background(), t.serverSocket, func() error {
				return t.cmdExec.Run(cmd)
			})
			if err != nil {
				// Session creation failed - but it might be because the session already exists
				// (DoesSessionExist may have timed out and returned false incorrectly)
				// Invalidate cache and re-check before returning error
				t.invalidateExistsCache()
				if t.DoesSessionExist() {
					// Session actually exists - the initial check was wrong (likely timeout)
					// Continue with restore instead of returning error
					log.Info("tmux session already exists (initial check was incorrect), continuing with restore", "session", t.sanitizedName)
				} else {
					return fmt.Errorf("failed to create tmux session '%s': %w", t.sanitizedName, err)
				}
			} else {
				log.Info("created new tmux session", "session", t.sanitizedName, "dir", workDir, "program", t.program)
				t.invalidateExistsCache() // Session was created, invalidate cache
				// new-session started the tmux server; reset this session's circuit breakers
				// so subsequent DoesSessionExist() calls can verify the session is running.
				if r, ok := t.cmdExec.(executor.Resettable); ok {
					r.Reset()
				}
				t.setRemainOnExit()
			}
		}
	} else {
		log.Info("found existing tmux session, will reattach to preserve history", "session", t.sanitizedName)
	}

	// Session exists - create PTY connection for detached operations
	// This is needed for SetDetachedSize(), SendKeys(), and the Direct Claude Command Interface
	// We use tmux attach-session to get a PTY handle without actually attaching interactively.
	// Always close any existing PTY before creating a new one: the old attach-session may have
	// exited (returning EIO on reads) but left t.ptmx non-nil, which would cause the new
	// response stream to immediately get EIO. Closing and reopening guarantees a live connection.
	// Check-then-act: see AttachToExisting's identical comment above (backlog item
	// 0f4b1300-d667-437b-b51f-89d81a668693) -- not atomic against a concurrent
	// AttachToExisting()/RestoreWithWorkDir()/Close() call on the same session.
	if t.lockedPTMX() != nil {
		_ = t.closePTYAndAttachCmd()
	}
	if t.lockedPTMX() == nil {
		const ptyMaxRetries = 3
		var lastPTYErr error
		for attempt := 0; attempt < ptyMaxRetries; attempt++ {
			if attempt > 0 {
				delay := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
				log.Info("retrying PTY attach for session", "session", t.sanitizedName, "attempt", attempt+1, "maxRetries", ptyMaxRetries, "delay", delay)
				time.Sleep(delay)
			}
			// Use StartWithSize so the PTY has non-zero dimensions before tmux attach-session
			// forks. Without this, running headless (systemd with no controlling terminal)
			// produces a 0×0 PTY; tmux reads that size at client startup and immediately
			// disconnects, causing EIO within ~1ms of the response stream starting.
			ws := &pty.Winsize{
				Rows: uint16(t.lastKnownRows.Load()),
				Cols: uint16(t.lastKnownCols.Load()),
			}
			ptmx, attachCmd, err := t.ptyFactory.StartWithSize(t.buildAttachCommand(), ws)
			if err != nil {
				lastPTYErr = err
				continue
			}
			waitOnce := new(sync.Once)
			t.setPTYTriple(ptmx, attachCmd, waitOnce) // CRITICAL: track attachCmd so it can be killed on cleanup
			log.Info("successfully restored PTY connection for tmux session", "session", t.sanitizedName)
			// Diagnostic: watch for unexpected early exit of the attach process.
			// Uses the same sync.Once as closePTYAndAttachCmd so Wait is called exactly once.
			go func(cmd *exec.Cmd, name string, once *sync.Once) {
				var err error
				once.Do(func() { err = cmd.Wait() })
				log.Info("attach-session process exited", "session", name, "exitErr", err)
			}(attachCmd, t.sanitizedName, waitOnce)
			lastPTYErr = nil
			break
		}
		if lastPTYErr != nil {
			// Graceful degradation - session can still be viewed via tmux capture-pane,
			// but PTY-based operations (resizing, SendKeys, controller) will be unavailable.
			log.Warn("PTY initialization failed for session after all attempts", "session", t.sanitizedName, "attempts", ptyMaxRetries, "err", lastPTYErr)
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
	_, _ = io.WriteString(h, s)
	return h.Sum(nil)
}

// TapEnter sends an enter keystroke to the tmux pane.
// writeToPTY looks up the current PTY via GetPTY() and writes data to it,
// wrapping the not-initialized error in the given message on failure.
func (t *TmuxSession) writeToPTY(data []byte, errMsg string) (int, error) {
	file, err := t.GetPTY()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", errMsg, err)
	}
	return file.Write(data)
}

func (t *TmuxSession) TapEnter() error {
	_, err := t.writeToPTY([]byte{0x0D}, "error sending enter keystroke to PTY")
	return err
}

// TapDAndEnter sends 'D' followed by an enter keystroke to the tmux pane.
func (t *TmuxSession) TapDAndEnter() error {
	_, err := t.writeToPTY([]byte{0x44, 0x0D}, "error sending enter keystroke to PTY")
	return err
}

func (t *TmuxSession) SendKeys(keys string) (int, error) {
	file, err := t.GetPTY()
	if err != nil {
		return 0, err
	}
	return file.Write([]byte(keys))
}

// GetPTY returns the PTY file descriptor for reading terminal output.
// This provides direct access to the PTY master for terminal streaming.
// Returns an error if the PTY is not initialized.
func (t *TmuxSession) GetPTY() (*os.File, error) {
	file := t.lockedPTMX()
	if file == nil {
		return nil, fmt.Errorf("PTY not initialized - session may not be started")
	}
	return file, nil
}

// HasUpdated checks if the tmux pane content has changed since the last tick. It also returns true if
// the tmux pane has a prompt for aider or claude code.
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool, content string) {
	content, err := t.CapturePaneContent()
	if err != nil {
		log.Error("error capturing pane content in status monitor", "session", t.sanitizedName, "err", err)
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
	// Snapshot once, not re-fetched per-loop like the stdin-forward goroutine below:
	// io.Copy blocks on one reader for its whole (blocking) call, matching the pre-fix
	// code's implicit one-time argument evaluation. This intentionally does NOT pick up
	// a mid-session Restore()/setPTYTriple() swap -- that asymmetry with the write side
	// is a pre-existing limitation (not introduced by ptmxMu), tracked for a possible
	// future fix in project_plans/tmux-ptmx-race-fix/implementation/pre-mortem.md finding #5.
	ptmxForCopy := t.lockedPTMX()
	go func() {
		defer t.wg.Done()
		_, _ = io.Copy(os.Stdout, ptmxForCopy)
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
					log.Error("error during safe detach after session termination", "session", t.sanitizedName, "err", err)
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
				log.Info("nuked first stdin", "bytes", string(buf[:nr]))
				continue
			}

			// Check for Ctrl+q (ASCII 17)
			if nr == 1 && buf[0] == 17 {
				// Detach from the session
				t.Detach()
				return
			}

			// Forward other input to tmux. Re-snapshot on every iteration (not hoisted)
			// so a mid-session Restore() swap transparently redirects to the new PTY.
			if file := t.lockedPTMX(); file != nil {
				_, _ = file.Write(buf[:nr])
			}
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

	var errs []error //nolint:prealloc // size unknown before calling closePTYAndAttachCmd

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
			log.Error("error closing attach pty session", "session", t.sanitizedName, "errs", cleanupErrs)
			panic(msg)
		} else {
			// All errors are "file already closed" - expected in race conditions
			log.Info("PTY already closed during detach (expected in concurrent scenarios)", "session", t.sanitizedName)
		}
	}

	// Attach goroutines should die on EOF due to the ptmx closing. Call
	// t.Restore to set a new t.ptmx.
	if restoreErr := t.Restore(); restoreErr != nil {
		// This is a fatal error. Our invariant that a started TmuxSession always has a valid ptmx is violated.
		msg := fmt.Sprintf("error restoring session after detach: %v", restoreErr)
		log.Error("error restoring session after detach", "session", t.sanitizedName, "err", restoreErr)
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

// lockedPTMX returns the current ptmx pointer (possibly nil) under ptmxMu.
func (t *TmuxSession) lockedPTMX() *os.File {
	t.ptmxMu.Lock()
	defer t.ptmxMu.Unlock()
	return t.ptmx // allow-direct-ptmx-access
}

// setPTYTriple atomically installs a new PTY triple (one attach generation).
func (t *TmuxSession) setPTYTriple(file *os.File, cmd *exec.Cmd, waitOnce *sync.Once) {
	t.ptmxMu.Lock()
	defer t.ptmxMu.Unlock()
	t.ptmx = file                  // allow-direct-ptmx-access
	t.attachCmd = cmd              // allow-direct-ptmx-access
	t.attachCmdWaitOnce = waitOnce // allow-direct-ptmx-access
}

// clearPTYTriple atomically captures and clears the PTY triple, returning the
// captured values so the caller can run blocking cleanup (Close/Kill/Wait) outside
// the lock. Safe to call even if the triple is already nil (returns nils). Clears
// all three fields unconditionally -- pre-fix, closePTYAndAttachCmd only nilled
// attachCmd/attachCmdWaitOnce inside the `attachCmd.Process != nil` branch, so an
// attachCmd with a nil Process (shouldn't happen via the normal write sites, which
// only install a triple after ptyFactory.Start/StartWithSize succeeds) would have
// been left stale. Clearing unconditionally here closes that edge case rather than
// reproducing it.
func (t *TmuxSession) clearPTYTriple() (file *os.File, cmd *exec.Cmd, waitOnce *sync.Once) {
	t.ptmxMu.Lock()
	defer t.ptmxMu.Unlock()
	file, cmd, waitOnce = t.ptmx, t.attachCmd, t.attachCmdWaitOnce // allow-direct-ptmx-access
	t.ptmx, t.attachCmd, t.attachCmdWaitOnce = nil, nil, nil       // allow-direct-ptmx-access
	return file, cmd, waitOnce
}

// closePTYAndAttachCmd closes the PTY file descriptor and kills the attach process.
// CRITICAL: This must be called whenever closing a PTY to prevent orphaned tmux attach processes.
// Returns a slice of errors encountered during cleanup (for aggregation in calling functions).
//
// Snapshots the PTY triple under ptmxMu then runs blocking cleanup (Close/Kill/Wait) on the
// locals outside the lock, so ptmxMu is never held across I/O. One accepted side effect: only
// the first concurrent caller ever receives a non-nil snapshot, so concurrent
// closePTYAndAttachCmd callers now serialize into one real cleanup plus fast no-ops instead of
// each racing .Close()/.Kill() independently -- see TestClosePTYAndAttachCmd_OnlyFirstConcurrentCallerPerformsCleanup.
func (t *TmuxSession) closePTYAndAttachCmd() []error {
	var errs []error
	file, cmd, waitOnce := t.clearPTYTriple()

	// Close PTY file descriptor
	if file != nil {
		if err := file.Close(); err != nil {
			// Only log error if it's not "file already closed" (common in concurrent scenarios)
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}
		}
	}

	// CRITICAL: Kill the tmux attach-session process to prevent PTY leak
	// Closing the PTY FD does NOT kill the process - it keeps running and consuming PTYs
	if cmd != nil && cmd.Process != nil {
		log.Info("killing orphaned tmux attach process", "session", t.sanitizedName, "pid", cmd.Process.Pid)
		if err := cmd.Process.Kill(); err != nil {
			// Process may already be dead, only log if it's a real error
			if !strings.Contains(err.Error(), "process already finished") && !strings.Contains(err.Error(), "no such process") {
				errs = append(errs, fmt.Errorf("error killing attach process: %w", err))
			}
		}
		// Wait for process to be reaped to avoid zombies. Use waitOnce so this
		// call is safe even when RestoreWithWorkDir's diagnostic goroutine also calls Wait.
		if waitOnce != nil {
			waitOnce.Do(func() { _ = cmd.Wait() })
		} else {
			_ = cmd.Wait()
		}
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
		// buildTmuxCommand binds cmd to context.Background(), so runGatedErr's
		// ctx only bounds the exec-gate slot acquisition, not the subprocess
		// itself. Run through a TimeoutExecutor here so a hung tmux server
		// genuinely gets killed after killSessionTimeout, matching
		// KillTmuxSessionByTitle's non-cosmetic 5s cap, instead of leaving
		// Close() (and its callers, e.g. DeleteSession's cleanup goroutine)
		// blocked indefinitely.
		killExec := executor.MakeTimeoutExecutor(killSessionTimeout)
		gatedErr := runGatedErr(context.Background(), t.serverSocket, func() error {
			return killExec.Run(cmd)
		})
		if err := gatedErr; err != nil {
			// Check if this is the common "session not found" error
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				// Exit code 1 usually means session doesn't exist or was already killed
				log.Info("tmux session was already killed or doesn't exist", "session", t.sanitizedName)
			} else {
				errs = append(errs, fmt.Errorf("error killing tmux session: %w", err))
			}
		} else {
			log.Info("successfully killed tmux session", "session", t.sanitizedName)
		}
		t.invalidateExistsCache() // Session was killed, invalidate cache
	} else {
		log.Info("tmux session doesn't exist, no need to kill", "session", t.sanitizedName)
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
	file := t.lockedPTMX()
	if file == nil {
		return fmt.Errorf("PTY is not initialized")
	}

	// Get the file descriptor value
	fd := int(file.Fd())

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

	return pty.Setsize(file, &pty.Winsize{
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
		log.Warn("failed to resize PTY for session", "session", t.sanitizedName, "err", err)
	}

	// Also resize the tmux window itself to ensure the dimensions are applied.
	colsStr := fmt.Sprintf("%d", cols)
	rowsStr := fmt.Sprintf("%d", rows)
	if t.cmEnabledForBackground() {
		ctx, cancel := cmCtx()
		defer cancel()
		if _, cmErr := t.sendCMCommand(ctx,
			"resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr); cmErr != nil {
			log.Debug("SetWindowSize CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
			cmd := t.buildTmuxCommand("resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr)
			if err := runGatedErr(context.Background(), t.serverSocket, func() error {
				return t.cmdExec.Run(cmd)
			}); err != nil {
				log.Error("tmux resize-window failed", "session", t.sanitizedName, "err", err)
				return fmt.Errorf("failed to resize tmux window: %w", err)
			}
		}
	} else {
		cmd := t.buildTmuxCommand("resize-window", "-t", t.sanitizedName, "-x", colsStr, "-y", rowsStr)
		if err := runGatedErr(context.Background(), t.serverSocket, func() error {
			return t.cmdExec.Run(cmd)
		}); err != nil {
			log.Error("tmux resize-window failed", "session", t.sanitizedName, "err", err)
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
		log.Info("[tmux] server recovery already in progress, skipping", "caller", caller)
		return
	}
	recoveryInFlight = true
	recoveryMu.Unlock()
	defer func() {
		recoveryMu.Lock()
		recoveryInFlight = false
		recoveryMu.Unlock()
	}()

	if _, restartErr := ensureServerRunning(serverSocket); restartErr == nil {
		log.Info("[tmux] server restarted, resetting circuit breakers", "caller", caller)
		executor.GetGlobalRegistry().ResetAll()
		if onServerRecovered != nil {
			go onServerRecovered()
		}
		if serverSocket == "" {
			if keepErr := CreateKeepaliveSession(serverSocket); keepErr != nil {
				log.Warn("[tmux] failed to recreate keepalive session", "err", keepErr)
			}
		}
	} else {
		// Log at ERROR level: if the server cannot be restarted, all session operations
		// will continue to fail until the user manually intervenes.
		// Note: individual user sessions are NOT automatically re-created after recovery;
		// they will be restarted on the next user interaction (e.g., Resume() call).
		log.Error("[tmux] failed to restart tmux server", "caller", caller, "err", restartErr)
	}
}

// listSessionsRaw runs "tmux list-sessions -F #{session_name}" using the provided context.
// On circuit-breaker open it falls back to a direct exec.CombinedOutput call so that
// existence checks always work regardless of breaker state.
// Returns raw combined output and the first error encountered.
func (t *TmuxSession) listSessionsRaw(ctx context.Context) ([]byte, error) {
	// Gated here (not by DoesSessionExist/DoesSessionExistNoCache's callers) so
	// a singleflight-coalesced burst of concurrent callers consumes exactly one
	// exec-gate slot, not one per caller.
	return runGated(ctx, t.serverSocket, func() ([]byte, error) {
		cmdArgs := Socket(t.serverSocket).Args("list-sessions", "-F", "#{session_name}")
		cmd := safeexec.CommandContext(ctx, Binary(), cmdArgs...)
		output, err := t.cmdExec.CombinedOutput(cmd)
		// If the circuit breaker is open, fall back to direct exec.
		// "No sessions" (exit 1 when server running but empty) can cause false circuit
		// breaker trips; the fallback ensures checks always work regardless of breaker state.
		if errors.Is(err, executor.ErrCircuitOpen) {
			cmd = safeexec.CommandContext(ctx, Binary(), cmdArgs...)
			output, err = cmd.CombinedOutput()
		}
		return output, err
	})
}

func (t *TmuxSession) DoesSessionExist() bool {
	if t == nil {
		return false
	}

	// Fast path: use the push-based registry when it is healthy and it confirms
	// the session exists. If the registry returns false, it may be lagging behind
	// tmux reality (e.g. the %session-created event has not been delivered yet),
	// so fall through to the cache/subprocess path for an authoritative answer.
	if t.registry != nil && t.registry.IsHealthy() {
		if t.registry.SessionExists(t.sanitizedName) {
			return true
		}
		// Registry returned false — do not trust it blindly; fall through.
	}

	// Fast path: lock-free atomic load.
	if v := t.existsCache.Load(); v != nil {
		state := v.(existsCacheState)
		if !state.time.IsZero() && time.Since(state.time) < t.existsCacheTTL {
			return state.exists
		}
	}

	// Slow path: coalesce concurrent misses via singleflight.
	// No lock is held during the subprocess — fixes the previous anti-pattern
	// of holding existsCacheMutex across listSessionsRaw (a subprocess call).
	v, _, _ := t.existsSF.Do("", func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(context.Background(), sessionExistsTimeout)
		defer cancel()
		output, err := t.listSessionsRaw(ctx)

		if ctx.Err() == context.DeadlineExceeded {
			log.Warn("timeout checking if tmux session exists", "session", t.sanitizedName)
			t.existsCache.Store(existsCacheState{exists: false, time: time.Now()})
			return false, nil
		}

		if err != nil {
			needsRecovery := t.serverSocket == "" && serverNotRunning(output)
			t.existsCache.Store(existsCacheState{exists: false, time: time.Now()})
			if needsRecovery {
				recoverFromServerFailure(t.serverSocket, "DoesSessionExist")
			}
			return false, nil
		}

		sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
		exists := false
		for _, session := range sessions {
			if session == t.sanitizedName {
				exists = true
				break
			}
		}
		t.existsCache.Store(existsCacheState{exists: exists, time: time.Now()})
		return exists, nil
	})
	return v.(bool)
}

// invalidateExistsCache clears the session existence cache to force a fresh check.
func (t *TmuxSession) invalidateExistsCache() {
	t.existsCache.Store(existsCacheState{}) // zero time = cache invalid
}

// DoesSessionExistNoCache checks if session exists WITHOUT using the
// time-based cache. This is used for critical validation before session
// creation to ensure we have the most up-to-date information about session
// existence.
//
// Concurrent callers (health checker, hibernation sweeper, the session-create
// retry loop, bulk instance restore, etc.) are coalesced via noCacheSF into a
// single in-flight subprocess — this keeps the "always fresh" contract (no
// TTL) while eliminating the redundant concurrent tmux subprocess spawns that
// used to hit an already-serialized single-threaded tmux server one-for-one
// with callers.
func (t *TmuxSession) DoesSessionExistNoCache() bool {
	if t == nil {
		return false
	}

	v, _, _ := t.noCacheSF.Do("", func() (interface{}, error) {
		// Direct check without cache — use a longer timeout for critical validation.
		ctx, cancel := context.WithTimeout(context.Background(), sessionExistsNoCacheTimeout)
		defer cancel()

		output, err := t.listSessionsRaw(ctx)
		if err != nil {
			// output is included: the Go error alone (e.g. "exit status 1") never
			// carries tmux's own stderr text (e.g. "no server running on <socket>",
			// "error connecting to <socket> (No such file or directory)"), which is
			// the one piece of evidence that actually distinguishes "server never
			// came up" from "server up but genuinely doesn't have this session yet"
			// -- see the incident this comment documents in PR #445.
			log.Warn("DoesSessionExistNoCache: tmux list-sessions failed", "session", t.sanitizedName, "serverSocket", t.serverSocket, "err", err, "output", string(output))
			// Only attempt auto-recovery for the default server (not isolated test servers).
			if t.serverSocket == "" && serverNotRunning(output) {
				recoverFromServerFailure(t.serverSocket, "DoesSessionExistNoCache")
			}
			return false, nil
		}

		// Parse and log ALL sessions for debugging
		sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
		log.Info("DoesSessionExistNoCache: checking for session in tmux sessions", "session", t.sanitizedName, "sessions", sessions)

		for _, session := range sessions {
			if session == t.sanitizedName {
				return true, nil
			}
		}
		return false, nil
	})
	return v.(bool)
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
		log.Debug("RefreshClient CM path failed, falling back to subprocess", "session", t.sanitizedName)
	}

	// Method 1: Use refresh-client command (preferred)
	cmd := t.buildTmuxCommand("refresh-client", "-t", t.sanitizedName)
	refreshErr := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(cmd)
	})
	if err := refreshErr; err != nil {
		log.Warn("refresh-client failed, trying alternative", "session", t.sanitizedName, "err", err)

		// Method 2: Send SIGWINCH via kill command
		cmd = t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName, "#{pane_pid}")
		output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
			return t.cmdExec.Output(cmd)
		})
		if err != nil {
			return fmt.Errorf("failed to get pane PID: %w", err)
		}

		panePID := strings.TrimSpace(string(output))
		winchCtx, winchCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer winchCancel()
		killCmd := safeexec.CommandContext(winchCtx, "kill", "-WINCH", panePID)
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to send SIGWINCH: %w", err)
		}
	}

	return nil
}

// cmEnabledForBackground returns true when CM is available AND the normal-priority
// send queue has room. Background ops skip CM when the queue is backed up — they
// fall back to subprocess so that the queue stays clear for high-priority user input.
func (t *TmuxSession) cmEnabledForBackground() bool {
	if !cmCommandsEnabled.Load() {
		return false
	}
	t.controlModeSubMu.RLock()
	ch := t.normPriSendCh
	t.controlModeSubMu.RUnlock()
	return ch != nil && len(ch) == 0
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
		log.Debug("CapturePaneContent CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	recordSpawn(time.Now())
	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
	if err != nil {
		recordFailure(time.Now())
		// Invalidate cache so TmuxAlive() returns false on the next call without
		// waiting for the 5-second TTL. This prevents repeated ERROR-level subprocess
		// failures when a session has died and the registry hasn't caught up yet.
		t.invalidateExistsCache()
		logArgs := []any{"session", t.sanitizedName, "err", err}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			logArgs = append(logArgs, "stderr", strings.TrimSpace(string(exitErr.Stderr)))
		}
		log.Warn("failed to capture pane content for session", logArgs...)
		return "", fmt.Errorf("error capturing pane content for session '%s': %v", t.sanitizedName, err)
	}
	return sanitizeUTF8String(output), nil
}

// CapturePaneContentPriority mirrors CapturePaneContent but routes the
// subprocess call through the resync exec-gate fast lane (runGatedFastLane)
// instead of the default pool (runGated), so resync-triggered captures don't
// queue behind ordinary tmux exec traffic on the same server socket
// (Epic 4.2, terminal:resync-exec-gate-fast-lane). It does not use the
// control-mode path, unlike CapturePaneContent — resync callers need the
// isolation the subprocess gate provides, and control mode has no gate to
// isolate against.
func (t *TmuxSession) CapturePaneContentPriority() (string, error) {
	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	recordSpawn(time.Now())
	output, err := runGatedFastLane(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
	if err != nil {
		recordFailure(time.Now())
		t.invalidateExistsCache()
		logArgs := []any{"session", t.sanitizedName, "err", err}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			logArgs = append(logArgs, "stderr", strings.TrimSpace(string(exitErr.Stderr)))
		}
		log.Warn("failed to capture pane content for session (fast lane)", logArgs...)
		return "", fmt.Errorf("error capturing pane content for session '%s': %v", t.sanitizedName, err)
	}
	return sanitizeUTF8String(output), nil
}

// RefreshClientPriority mirrors RefreshClient's Method 1 subprocess path but
// routes through the resync exec-gate fast lane (runGatedFastLane) instead of
// the default pool (Epic 4.2). Unlike RefreshClient, it does not attempt the
// control-mode path or the SIGWINCH fallback (Method 2) — resync's priority
// caller wants a bounded, fast-lane-only refresh, not the full fallback
// chain.
func (t *TmuxSession) RefreshClientPriority() error {
	cmd := t.buildTmuxCommand("refresh-client", "-t", t.sanitizedName)
	return runGatedErrFastLane(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(cmd)
	})
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
		log.Debug("CapturePaneContentRaw CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-t", t.sanitizedName)
	recordSpawn(time.Now())
	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
	if err != nil {
		recordFailure(time.Now())
		t.invalidateExistsCache()
		log.Warn("failed to capture raw pane content for session", "session", t.sanitizedName, "err", err)
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
		log.Debug("CapturePaneContentWithOptions CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("capture-pane", "-p", "-e", "-J", "-S", start, "-E", end, "-t", t.sanitizedName)
	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
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
		log.Debug("GetCursorPosition CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{cursor_x} #{cursor_y}")

	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
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
		log.Debug("GetPaneDimensions CM path failed, falling back to subprocess", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_width} #{pane_height}")

	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
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
	// Resolve once, here -- not per-command below. Without this, an empty
	// serverSocket always meant the real, shared default tmux socket
	// unconditionally, including inside a `go test` binary, letting this
	// enumerate-and-kill-by-prefix function target sessions belonging to a
	// separate, currently-running stapler-squad process. See ResolveSocket's
	// doc comment for the incident history this closes.
	socket := ResolveSocket(serverSocket)

	// First try to list sessions
	lsCtx, lsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer lsCancel()
	output, err := runGated(lsCtx, serverSocket, func() ([]byte, error) {
		cmd := safeexec.CommandContext(lsCtx, Binary(), socket.Args("ls")...)
		return cmdExec.Output(cmd)
	})

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
		log.Info("cleaning up session", "session", match)
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		runErr := runGatedErr(killCtx, serverSocket, func() error {
			killCmd := safeexec.CommandContext(killCtx, Binary(), socket.Args("kill-session", "-t", match)...)
			return cmdExec.Run(killCmd)
		})
		killCancel()
		if runErr != nil {
			return fmt.Errorf("failed to kill tmux session %s: %v", match, runErr)
		}
	}
	return nil
}

// sanitizerInitialCap is the pre-allocated capacity for each pooled strings.Builder.
// Sized to cover typical tmux pane output without reallocation.
const sanitizerInitialCap = 4096

// sanitizerPool reuses strings.Builder allocations across sanitizeUTF8String calls.
var sanitizerPool = sync.Pool{New: func() any {
	b := new(strings.Builder)
	b.Grow(sanitizerInitialCap)
	return b
}}

// sanitizeUTF8String converts raw bytes to valid UTF-8 string, replacing invalid sequences
// This prevents xterm.js parsing errors from invalid byte sequences while maintaining
// terminal formatting and color information
func sanitizeUTF8String(rawBytes []byte) string {
	if len(rawBytes) == 0 {
		return ""
	}

	result := sanitizerPool.Get().(*strings.Builder)
	result.Reset()
	// result.String() below copies bytes into a new string before defer fires, so
	// returning the copy and then returning result to the pool is safe.
	defer sanitizerPool.Put(result)
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
		log.Debug("GetPaneCurrentPath CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_current_path}")

	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
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
		log.Debug("GetPanePID CM path failed, falling back", "session", t.sanitizedName, "err", cmErr)
	}

	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_pid}")

	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
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

// ExitStatus reports the wrapped program's exit code and signal for a dead pane,
// via tmux's #{pane_dead_status}/#{pane_dead_signal} (populated by remain-on-exit).
// Returns ok=false if the pane is still alive, the session is already gone, or the
// pane never went through a dead state (nothing to report). Callers should read this
// as early as possible after detecting an exit -- the pane is destroyed the moment
// anything issues kill-session/respawn-pane against it, and this data goes with it.
func (t *TmuxSession) ExitStatus() (code int, signal string, ok bool) {
	cmd := t.buildTmuxCommand("display-message", "-p", "-t", t.sanitizedName,
		"#{pane_dead_status}\t#{pane_dead_signal}")
	output, err := runGated(context.Background(), t.serverSocket, func() ([]byte, error) {
		return t.cmdExec.Output(cmd)
	})
	if err != nil {
		return 0, "", false
	}
	parts := strings.SplitN(strings.TrimRight(string(output), "\n"), "\t", 2)
	statusStr := strings.TrimSpace(parts[0])
	if statusStr == "" {
		// Empty means the pane is still alive (or the format variables aren't
		// supported by this tmux version).
		return 0, "", false
	}
	code, err = strconv.Atoi(statusStr)
	if err != nil {
		return 0, "", false
	}
	if len(parts) > 1 {
		signal = strings.TrimSpace(parts[1])
	}
	return code, signal, true
}
