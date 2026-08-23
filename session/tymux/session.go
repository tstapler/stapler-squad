package tymux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ErrNotImplemented is returned by tymuxGRPCSession methods that a later
// epic (2.3+, standing Attach stream / control-mode) has not yet
// implemented.
var ErrNotImplemented = errors.New("tymux: not implemented")

// ErrNotSupportedOnTymuxBackend is returned by GetPTY/GetPanePID (Story
// 2.2.5, Pattern Decisions): tymux has no local PTY file descriptor or OS
// pane PID to hand back — every terminal I/O path goes over gRPC, not a
// local file/process the caller could read or signal directly. Returning
// this explicit, typed error (never a bare nil/zero value or a panic) lets
// a generic ProcessManager caller distinguish "not supported by this
// backend" from "supported but currently unavailable."
var ErrNotSupportedOnTymuxBackend = errors.New("not supported by the tymux backend")

// errSessionNotStarted is returned by methods that need a live pane_id
// (capture/cursor/dimensions) when Start/RestoreWithWorkDir hasn't run yet.
var errSessionNotStarted = errors.New("tymux: session not started")

// tymuxGRPCSession is the real TymuxManager implementation, backed by a live
// tymuxd daemon over gRPC via rpcTransport.
type tymuxGRPCSession struct {
	transport rpcTransport

	// mu guards every field below — Start/Close/RestoreWithWorkDir write
	// them, capture/introspection methods and IsAlive/HasSession read them,
	// and nothing here assumes single-threaded callers.
	mu sync.RWMutex

	// sessionID/paneID are tymux's own UUIDs for the session and its single
	// (v1.0, non-split) pane, cached from CreateSession/ReviveSession's
	// response — GetSessionIdentifier() needs no RPC (architecture.md §1).
	sessionID string
	paneID    string

	// cwd is the pane's spawn-time working directory (Pane.cwd, Epic 1.5),
	// cached once at Start()/RestoreWithWorkDir() time — Story 2.2.4:
	// GetCurrentWorkingDirectory() never re-queries it.
	cwd string

	// liveness is a local cache of the last-observed Liveness, updated here
	// via a CapturePane-based fallback read (Task 2.2.1c) since the standing
	// Attach stream that will keep it fresh for free doesn't exist until
	// Epic 2.3.
	liveness v1.Liveness

	// closed is set once Close() has issued KillSession, short-circuiting
	// IsAlive() without a further RPC against a session we already killed.
	closed bool

	// stream/cancelAttach/streamDone back the standing Attach stream
	// (Epic 2.3, Story 2.3.1): opened once in cacheFromSession — the
	// common tail of Start/RestoreWithWorkDir — and kept alive for the
	// whole session's lifetime, independent of any one browser tab.
	// cancelAttach is the context.CancelFunc DetachSafely() invokes for a
	// full cancellation (not a half-close); streamDone is closed by the
	// reader goroutine (stream.go's readAttachLoop) when the stream ends
	// for any reason, and is what Attach() hands back to callers.
	stream       attachStream
	cancelAttach context.CancelFunc
	streamDone   chan struct{}

	// fanout is the local multi-subscriber broadcast (Story 2.3.2) that
	// SubscribeToControlModeUpdates hands subscriber channels out from —
	// one upstream standing stream, N independently-paced local
	// subscribers, rather than one Attach call per subscriber.
	fanout *ClientFanout

	// outputGapCount is a per-session running total of OutputGap events
	// received on the standing stream (Observability Plan: "output_gap
	// receipt count"), logged on each occurrence (stream.go's
	// handleAttachEvent) alongside the process-wide
	// tymux_attach_stream_reconnects_total{output_gap} metric.
	outputGapCount atomic.Int64

	// exitCallback is invoked at most once, by whichever of
	// readAttachLoop's Exited handling (stream.go's deliverExit) or
	// SetOnExitCallback observes the exit second — the check-before-and-
	// after-registration race pane.rs:311-322's wait_exit() resolves via
	// check/notify/check, mirrored here via exited/exitFired instead of
	// Go's sync.Once (which can't dynamically pick between "the callback
	// registered so far" and "the callback that shows up later").
	exitCallback func(string)

	// exited/exitReason record whether/why the standing stream's Exited
	// event has been observed; exitFired guards the one-shot delivery.
	// ResetExitOnce (Task 2.4.2a) clears all three so a reused session
	// object can observe a later exit again, mirroring TmuxSession's
	// ResetExitOnce (tmux.go:373-382).
	exited     bool
	exitReason string
	exitFired  bool

	// closing is set by Close()/DetachSafely() before cancelling the
	// standing stream's context (Task 2.5.1a) — the signal
	// readAttachLoop checks when Receive() errors, to distinguish a
	// deliberate detach/close (no reconnect) from an unexpected drop
	// (network blip, daemon restart — triggers ReconnectLoop, Story
	// 2.5.1/2.5.2). atomic.Bool rather than a field under mu because
	// readAttachLoop's reader goroutine must be able to check it without
	// risking a lock a Close()-in-progress call might already hold.
	closing atomic.Bool

	// abortReconnect is closed exactly once, by Close()/DetachSafely()
	// (Task 2.5.1a), to interrupt any in-progress ReconnectLoop backoff
	// wait or in-flight reattach dial immediately — without this,
	// teardownStandingStream's <-done wait could block until an
	// unrelated retry schedule finished on its own. Created once in
	// NewTymuxGRPCSession, not per-stream, since it must outlive any
	// individual stream generation (a backoff wait between attempts has
	// no live stream to hang cancellation off of) and Close() is
	// terminal (this object is never reused after Close(), matching
	// KillSession's own irreversibility).
	abortReconnect chan struct{}

	// reconnect{MaxAttempts,BaseDelay,MaxDelay} bound ReconnectLoop's
	// jittered exponential backoff (Task 2.5.2a). Struct fields (not
	// package consts) so tests can shrink them for fast, deterministic
	// exercises of the retry path; NewTymuxGRPCSession seeds production
	// defaults.
	reconnectMaxAttempts int
	reconnectBaseDelay   time.Duration
	reconnectMaxDelay    time.Duration

	// reconnecting/reconnectAttempt/reconnectCause/reconnectSince back
	// ReconnectState() (Task 2.5.2e) — set at the start of each
	// ReconnectLoop attempt and cleared once the shared resync path
	// (Task 2.5.2b) completes after a successful reattach, so a future
	// UI (ux.md Surface 2) can read "is this session reconnecting right
	// now, attempt N, since when" without reverse-engineering it from
	// the aggregate tymux_attach_stream_reconnects_total metric.
	reconnecting     bool
	reconnectAttempt int
	reconnectCause   string
	reconnectSince   time.Time

	// backendRestarted/backendRestartedAt back Story 2.5.3's distinct
	// "backend restarted, session state may be lost" state (Task
	// 2.5.3b): set the moment ReconnectLoop's daemon-restart detection
	// (Task 2.5.3a) fires and a ReviveSession call is about to run;
	// cleared by cacheFromSession so a later Start()/RestoreWithWorkDir()
	// on a reused object doesn't carry a stale flag from a previous
	// generation. Deliberately not folded into IsAlive()/exited — "still
	// alive" and "alive, but it's a fresh replacement process, not the
	// one you started" are different facts a caller needs to tell apart.
	backendRestarted   bool
	backendRestartedAt time.Time
}

// Default ReconnectLoop backoff bounds (Task 2.5.2a) — a jittered
// exponential schedule from defaultReconnectBaseDelay up to
// defaultReconnectMaxDelay, giving up after defaultReconnectMaxAttempts
// (~45.5s worst-case total wait pre-jitter: attempts 2-8 wait
// 0.5+1+2+4+8+15+15s, per reconnectBackoffDelay's base*2^(attempt-1)-capped-
// at-max schedule; jitter only ever shortens each wait), matching the
// "backend unavailable" transient-vs-permanent split ux.md §4 calls for
// rather than retrying forever.
const (
	defaultReconnectMaxAttempts = 8
	defaultReconnectBaseDelay   = 250 * time.Millisecond
	defaultReconnectMaxDelay    = 15 * time.Second
)

// NewTymuxGRPCSession constructs a tymuxGRPCSession using the given rpcTransport,
// returned as a TymuxManager since tymuxGRPCSession itself is unexported.
func NewTymuxGRPCSession(transport rpcTransport) TymuxManager {
	return &tymuxGRPCSession{
		transport:            transport,
		fanout:               NewClientFanout(),
		abortReconnect:       make(chan struct{}),
		reconnectMaxAttempts: defaultReconnectMaxAttempts,
		reconnectBaseDelay:   defaultReconnectBaseDelay,
		reconnectMaxDelay:    defaultReconnectMaxDelay,
	}
}

// --- Lifecycle ---

// validateWorkDir rejects an empty or nonexistent working directory before
// ever reaching tymuxd, mirroring session/tmux/tmux.go's own
// validateWorkDir (tmux.go:1196-1212) check-shape exactly (Task 2.2.1a). It
// reuses tmux.ErrWorkDirMissing (rather than inventing a second sentinel)
// so a caller-side errors.Is(err, tmux.ErrWorkDirMissing) check behaves
// identically regardless of which backend is in play.
func validateWorkDir(workDir string) error {
	if workDir == "" {
		return fmt.Errorf("tymux: working directory not set: %w", tmux.ErrWorkDirMissing)
	}
	info, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("tymux: working directory %q is not accessible: %w: %w", workDir, tmux.ErrWorkDirMissing, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("tymux: working directory %q is not a directory: %w", workDir, tmux.ErrWorkDirMissing)
	}
	return nil
}

// firstPane walks a Session's window/layout tree to its first (v1.0: only)
// pane. tymux's wire shape nests the pane under Session.Windows[].Layout
// (leaf Pane, or a binary Split to recurse into) rather than exposing it
// directly on Session, since a window can in general hold a split layout —
// see tymux.pb.go's Layout/Split/LayoutChild.
func firstPane(session *v1.Session) *v1.Pane {
	if session == nil {
		return nil
	}
	for _, w := range session.GetWindows() {
		if p := paneFromLayout(w.GetLayout()); p != nil {
			return p
		}
	}
	return nil
}

func paneFromLayout(l *v1.Layout) *v1.Pane {
	if l == nil {
		return nil
	}
	if p := l.GetPane(); p != nil {
		return p
	}
	if split := l.GetSplit(); split != nil {
		for _, child := range split.GetChildren() {
			if p := paneFromLayout(child.GetLayout()); p != nil {
				return p
			}
		}
	}
	return nil
}

// cacheFromSession stores sess's id/pane/cwd, marks the session live, and
// opens the standing Attach stream (Story 2.3.1) — the common tail of
// Start/RestoreWithWorkDir once a *v1.Session is in hand.
func (s *tymuxGRPCSession) cacheFromSession(sess *v1.Session) error {
	pane := firstPane(sess)
	if pane == nil {
		return fmt.Errorf("tymux: session %q has no pane", sess.GetId())
	}
	s.mu.Lock()
	s.sessionID = sess.GetId()
	s.paneID = pane.GetId()
	s.cwd = pane.GetCwd()
	s.liveness = v1.Liveness_LIVENESS_LIVE
	s.closed = false
	// Fresh generation: reset Story 2.5.1/2.5.3's per-generation state so
	// a reused session object (e.g. Start() after an earlier Close(), or
	// a second RestoreWithWorkDir()) doesn't carry a stale closing flag,
	// an already-closed abortReconnect channel, or a backend-restarted
	// flag from a previous generation.
	s.backendRestarted = false
	s.backendRestartedAt = time.Time{}
	s.abortReconnect = make(chan struct{})
	s.mu.Unlock()
	s.closing.Store(false)

	log.Info("tymux: session ready", "session_id", sess.GetId(), "pane_id", pane.GetId(), "cwd", pane.GetCwd())

	return s.openStandingStream(pane.GetId())
}

// beginClosing is the single place Close()/DetachSafely() (Task 2.5.1a)
// mark this generation as deliberately ending: it flips closing so
// readAttachLoop's next Receive() error is treated as "no reconnect,"
// and closes abortReconnect exactly once (guarded by the closing
// CompareAndSwap, since Close()/DetachSafely() must tolerate being
// called more than once) to interrupt any in-progress ReconnectLoop
// backoff wait or in-flight reattach dial immediately.
func (s *tymuxGRPCSession) beginClosing() {
	if s.closing.CompareAndSwap(false, true) {
		s.mu.RLock()
		abort := s.abortReconnect
		s.mu.RUnlock()
		close(abort)
	}
}

// Start creates a real tymux session rooted at dir. dir also becomes the
// session's tymux-side Name — the only stable string Start and
// RestoreWithWorkDir share across a stapler-squad process restart (the
// constructor takes no separate session-name parameter), so
// RestoreWithWorkDir's ListSessions lookup (Task 2.2.1b) can find this
// session again by matching on it. This does mean two sessions started
// against the identical directory aren't distinguishable by name alone —
// acceptable for v1 (mirrors the plan's other Unresolved-Questions-style
// scope calls); a future epic can widen this if it becomes a real
// constraint.
func (s *tymuxGRPCSession) Start(dir string) error {
	if err := validateWorkDir(dir); err != nil {
		return err
	}
	resp, err := s.transport.CreateSession(context.Background(), connect.NewRequest(&v1.CreateSessionRequest{
		Name: dir,
		Cwd:  dir,
	}))
	if err != nil {
		return classifyRPCError("Start", err)
	}
	return s.cacheFromSession(resp.Msg)
}

// RestoreWithWorkDir reconciles this session against tymuxd after a
// stapler-squad process restart: ListSessions to find the session by the
// name Start() gave it (workDir, see Start's doc comment), ReviveSession if
// tymux reports it Dead, or a no-op if it's already Live — matching
// architecture.md §1's mapping exactly (Task 2.2.1b). If no matching
// session is found at all (never created, or tymuxd itself was restarted
// and lost it), falls back to creating a fresh one via Start, mirroring
// TmuxSession.RestoreWithWorkDir's own "recreate if truly gone" fallback.
func (s *tymuxGRPCSession) RestoreWithWorkDir(workDir string) error {
	if err := validateWorkDir(workDir); err != nil {
		return err
	}
	ctx := context.Background()
	listResp, err := s.transport.ListSessions(ctx, connect.NewRequest(&v1.ListSessionsRequest{}))
	if err != nil {
		return classifyRPCError("RestoreWithWorkDir", err)
	}
	var found *v1.Session
	for _, sess := range listResp.Msg.GetSessions() {
		if sess.GetName() == workDir {
			found = sess
			break
		}
	}
	if found == nil {
		return s.Start(workDir)
	}
	if found.GetLiveness() == v1.Liveness_LIVENESS_DEAD {
		reviveResp, err := s.transport.ReviveSession(ctx, connect.NewRequest(&v1.ReviveSessionRequest{
			SessionId: found.GetId(),
		}))
		if err != nil {
			return classifyRPCError("RestoreWithWorkDir", err)
		}
		if reviveResp.Msg.GetSession() != nil {
			found = reviveResp.Msg.GetSession()
		}
	}
	return s.cacheFromSession(found)
}

// Close kills the tymux session via KillSession. A Close before any Start
// is a no-op (nothing to kill) rather than an error, matching the
// zero-value-friendly shape other ProcessManager implementations use.
func (s *tymuxGRPCSession) Close() error {
	s.mu.RLock()
	sessionID := s.sessionID
	s.mu.RUnlock()
	log.Info("tymux: session closing", "session_id", sessionID)
	// Task 2.5.1a: mark this generation as deliberately ending BEFORE
	// tearing anything down, so readAttachLoop's Receive() error (caused
	// by teardownStandingStream's own cancellation below) is recognized
	// as a deliberate close, not a drop — and so any in-progress
	// ReconnectLoop backoff/dial is interrupted immediately rather than
	// leaving teardownStandingStream's <-done wait blocked.
	s.beginClosing()
	s.teardownStandingStream()
	if sessionID == "" {
		return nil
	}
	_, err := s.transport.KillSession(context.Background(), connect.NewRequest(&v1.KillSessionRequest{
		SessionId: sessionID,
	}))
	s.mu.Lock()
	s.closed = true
	s.liveness = v1.Liveness_LIVENESS_DEAD
	s.mu.Unlock()
	if err != nil {
		log.Warn("tymux: KillSession failed during Close", "session_id", sessionID, "err", err)
		return classifyRPCError("Close", err)
	}
	log.Info("tymux: session closed", "session_id", sessionID)
	return nil
}

// IsAlive reports whether the session is currently live. Task 2.2.1c: the
// cached liveness field only stays fresh for free once Epic 2.3's standing
// Attach stream exists to push Exited events into it; until then (i.e.
// today), every call here falls back to a CapturePane-based read so the
// answer reflects tymuxd's current state rather than a stale cache — except
// once Close() has run, which short-circuits to false without a further RPC
// against a session this process itself just killed.
//
// A failed CapturePane RPC is not the daemon confirming the pane is dead —
// it only means this particular request didn't get an answer, whether
// because tymuxd is unreachable (ErrTymuxdUnreachable, errors.go) or some
// other RPC-level failure. Collapsing that to a flat false would make a
// transient network blip on a genuinely-live session indistinguishable from
// a real death, exactly the distinction ErrTymuxdUnreachable exists to
// preserve (see errors.go's doc comment, research/ux.md:218-224). So on any
// capturePane error, log it and fall back to the last cached liveness
// rather than reporting dead; only a real (non-error) response saying
// LIVENESS_DEAD counts as a genuine confirmation.
func (s *tymuxGRPCSession) IsAlive() bool {
	s.mu.RLock()
	paneID := s.paneID
	closed := s.closed
	cachedLiveness := s.liveness
	sessionID := s.sessionID
	s.mu.RUnlock()
	if paneID == "" || closed {
		return false
	}
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		if errors.Is(err, ErrTymuxdUnreachable) {
			log.Warn("tymux: IsAlive: tymuxd unreachable, falling back to cached liveness", "session_id", sessionID, "err", err)
		} else {
			log.Warn("tymux: IsAlive: CapturePane failed, falling back to cached liveness", "session_id", sessionID, "err", err)
		}
		return cachedLiveness == v1.Liveness_LIVENESS_LIVE
	}
	s.mu.Lock()
	s.liveness = snap.GetLiveness()
	s.mu.Unlock()
	return snap.GetLiveness() == v1.Liveness_LIVENESS_LIVE
}

// --- Identification ---

// GetSessionIdentifier returns tymux's own session_id UUID, cached locally
// from Start/RestoreWithWorkDir — no RPC (architecture.md §1).
func (s *tymuxGRPCSession) GetSessionIdentifier() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// --- Existence / state ---

// HasSession reports whether Start/RestoreWithWorkDir has run and cached a
// pane, independent of whether that pane is currently live — mirroring
// TmuxProcessManager.HasSession's "has a session been initialized" meaning,
// not IsAlive's "is it alive right now."
func (s *tymuxGRPCSession) HasSession() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paneID != ""
}

// --- Working directory ---

// GetCurrentWorkingDirectory returns the cwd cached at Start()/
// RestoreWithWorkDir() time (Story 2.2.4) — no RPC. Returns "" (not an
// error) before any Start, matching NativeProcessManager/TmuxBackend's own
// "no session yet" convention.
func (s *tymuxGRPCSession) GetCurrentWorkingDirectory() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd, nil
}

// --- Terminal I/O ---

// GetPTY always returns ErrNotSupportedOnTymuxBackend — see that var's doc
// comment (Story 2.2.5).
func (s *tymuxGRPCSession) GetPTY() (*os.File, error) { return nil, ErrNotSupportedOnTymuxBackend }

// SendKeys, TapEnter, SendPromptWithEnter, and SendInputViaControlMode
// (Story 2.3.3) all collapse onto one AttachRequest{Input(...)} send over
// the standing stream — no per-call RPC, per architecture.md §1's noted
// simplification (flagged there as a parity risk against
// TmuxProcessManager's two-step SendKeys-then-sleep-then-TapEnter shape,
// to watch for in Epic 3's validation, not solved here).
func (s *tymuxGRPCSession) SendKeys(keys string) (int, error) {
	if err := s.sendOnStream(&v1.AttachRequest{
		Payload: &v1.AttachRequest_Input{Input: []byte(keys)},
	}); err != nil {
		return 0, err
	}
	return len(keys), nil
}

func (s *tymuxGRPCSession) TapEnter() error {
	return s.sendOnStream(&v1.AttachRequest{
		Payload: &v1.AttachRequest_Input{Input: []byte{0x0D}},
	})
}

func (s *tymuxGRPCSession) SendPromptWithEnter(p string) error {
	return s.sendOnStream(&v1.AttachRequest{
		Payload: &v1.AttachRequest_Input{Input: append([]byte(p), 0x0D)},
	})
}

func (s *tymuxGRPCSession) SendInputViaControlMode(ctx context.Context, data []byte) error {
	// sendOnStream/the standing stream's Send have no context parameter of
	// their own to cancel against, so honor the caller's cancellation/
	// timeout here before writing — otherwise ctx is accepted but has zero
	// effect, silently sending on an already-cancelled/expired context.
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.sendOnStream(&v1.AttachRequest{
		Payload: &v1.AttachRequest_Input{Input: data},
	})
}

// --- Terminal state ---

// capturePane is the shared CapturePane call every read-only introspection
// method (capture/cursor/dimensions/IsAlive) funnels through.
func (s *tymuxGRPCSession) capturePane(ctx context.Context, scrollbackOffset uint32) (*v1.PaneSnapshot, error) {
	s.mu.RLock()
	paneID := s.paneID
	s.mu.RUnlock()
	if paneID == "" {
		return nil, errSessionNotStarted
	}
	resp, err := s.transport.CapturePane(ctx, connect.NewRequest(&v1.CapturePaneRequest{
		PaneId:           paneID,
		ScrollbackOffset: scrollbackOffset,
	}))
	if err != nil {
		return nil, classifyRPCError("CapturePane", err)
	}
	return resp.Msg, nil
}

// rowsToPlainText concatenates each row's Cell.text (one grapheme cluster
// per cell), newline-joined — the ANSI-free join both CapturePaneContentRaw
// and CaptureViewport/CapturePaneContentWithOptions need.
func rowsToPlainText(rows []*v1.Row) string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		var b strings.Builder
		for _, c := range row.GetCells() {
			b.WriteString(c.GetText())
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// CapturePaneContent renders the live screen with ANSI/SGR attributes
// preserved (Task 2.2.2d). CellsToSGR's body lands in Epic 2.6; wiring the
// call site here just makes the package compile end-to-end in the meantime.
func (s *tymuxGRPCSession) CapturePaneContent() (string, error) {
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return "", err
	}
	return CellsToSGR(snap.GetGrid())
}

// CapturePaneContentRaw returns the live screen's plain text (Cell.text
// only, no ANSI) — Task 2.2.2a.
func (s *tymuxGRPCSession) CapturePaneContentRaw() (string, error) {
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return "", err
	}
	return rowsToPlainText(snap.GetGrid()), nil
}

// parseScrollbackOffset adapts a tmux capture-pane -S/-E style line argument
// (a literal offset, or "-"/"" meaning "as far as available"/"the live
// screen") onto tymux's single non-negative scrollback_offset. Anything
// that isn't a clean non-negative integer falls back to 0 (the live
// screen) rather than erroring — CapturePaneContentWithOptions is a
// best-effort adapter, not an exact tmux-semantics replay (architecture.md
// §1: tymux's offset is "a single integer, not a range").
func parseScrollbackOffset(v string) uint32 {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return uint32(n)
}

// CapturePaneContentWithOptions adapts tmux's start/end line-range capture
// onto one CapturePane call at the scrollback_offset startLine parses to
// (Task 2.2.2c) — tymux's CapturePaneRequest takes a single
// scrollback_offset, not a range, so this reads one full screen at that
// offset rather than an arbitrary startLine..endLine span; endLine is
// accepted (matching ProcessManager's signature) but not further
// interpreted, since a single CapturePane response is already exactly one
// rows x cols screen.
func (s *tymuxGRPCSession) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	snap, err := s.capturePane(context.Background(), parseScrollbackOffset(startLine))
	if err != nil {
		return "", err
	}
	return rowsToPlainText(snap.GetGrid()), nil
}

// CaptureViewport returns the live screen (scrollback_offset=0), tailed to
// at most lines rows (Task 2.2.2b). lines <= 0 returns the whole screen.
func (s *tymuxGRPCSession) CaptureViewport(lines int) (string, error) {
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return "", err
	}
	grid := snap.GetGrid()
	if lines > 0 && lines < len(grid) {
		grid = grid[len(grid)-lines:]
	}
	return rowsToPlainText(grid), nil
}

// GetCursorPosition returns (x, y) = (cursor_col, cursor_row), read from a
// PaneSnapshot with no dedicated RPC (Task 2.2.3a) — matching
// TmuxSession.GetCursorPosition's (cursor_x, cursor_y) column/row
// convention.
func (s *tymuxGRPCSession) GetCursorPosition() (x, y int, err error) {
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return 0, 0, err
	}
	return int(snap.GetCursorCol()), int(snap.GetCursorRow()), nil
}

// GetPaneDimensions returns (width, height) = (cols, rows), read from a
// PaneSnapshot with no dedicated RPC (Task 2.2.3a).
func (s *tymuxGRPCSession) GetPaneDimensions() (width, height int, err error) {
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return 0, 0, err
	}
	return int(snap.GetCols()), int(snap.GetRows()), nil
}

// SetWindowSize sends a Resize on the standing stream (Task 2.4.1a) — no
// separate RPC, and it succeeds even with no active Attach() caller from
// stapler-squad's own UI layer, since the standing stream (Epic 2.3) is
// always open once a session has started.
func (s *tymuxGRPCSession) SetWindowSize(cols, rows int) error {
	return s.sendResize(cols, rows)
}

// SetDetachedSize sends the same Resize as SetWindowSize (Task 2.4.1a);
// instanceTitle is accepted for TymuxManager parity but unused — tymux has
// no server-side per-title tag on the wire (architecture.md §1), unlike
// TmuxSession's local-only use of it.
func (s *tymuxGRPCSession) SetDetachedSize(width, height int, instanceTitle string) error {
	return s.sendResize(width, height)
}

// RefreshClient is a no-op returning nil (Task 2.4.1b) — no server RPC
// needed, since tymux's structured PaneSnapshot makes client-side re-render
// sufficient (architecture.md §1).
func (s *tymuxGRPCSession) RefreshClient() error { return nil }

// --- Process metadata ---

// GetPanePID always returns ErrNotSupportedOnTymuxBackend — see that var's
// doc comment (Story 2.2.5).
func (s *tymuxGRPCSession) GetPanePID() (int32, error) { return 0, ErrNotSupportedOnTymuxBackend }

// --- Content helpers ---

func (s *tymuxGRPCSession) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return false, false, ""
}
func (s *tymuxGRPCSession) FilterBanners(content string) (string, int) { return content, 0 }
func (s *tymuxGRPCSession) HasMeaningfulContent(content string) bool   { return false }

// --- Streaming (control mode) ---

func (s *tymuxGRPCSession) StartControlMode() error { return ErrNotImplemented }
func (s *tymuxGRPCSession) StopControlMode() error  { return ErrNotImplemented }

// SubscribeToControlModeUpdates/UnsubscribeFromControlModeUpdates (Story
// 2.3.2) delegate to ClientFanout — every subscriber shares the one
// standing stream's output rather than opening its own Attach call.
func (s *tymuxGRPCSession) SubscribeToControlModeUpdates() (string, chan []byte) {
	return s.fanout.Subscribe()
}
func (s *tymuxGRPCSession) UnsubscribeFromControlModeUpdates(id string) {
	s.fanout.Unsubscribe(id)
}

// --- Attach ---

// Attach returns the channel that closes when the standing stream ends
// (Task 2.3.4a), for callers wanting the interactive-TUI-attach "wait
// until detached/disconnected" concept.
func (s *tymuxGRPCSession) Attach() (chan struct{}, error) {
	s.mu.RLock()
	done := s.streamDone
	s.mu.RUnlock()
	if done == nil {
		return nil, errSessionNotStarted
	}
	return done, nil
}

// DetachSafely fully cancels the standing Attach call's context (Task
// 2.3.4b) — matching the proto's documented detach contract
// (tymux.proto:52-58): "Detaching means fully cancelling this call...
// Half-closing only the send side does not stop the receive side or end
// the attach." It does not close/reopen the stream itself; a subsequent
// Start/RestoreWithWorkDir opens a fresh one via cacheFromSession.
func (s *tymuxGRPCSession) DetachSafely() error {
	s.mu.RLock()
	cancel := s.cancelAttach
	s.mu.RUnlock()
	if cancel == nil {
		return errSessionNotStarted
	}
	// Task 2.5.1a: same ordering as Close() — mark deliberate before
	// cancelling, so readAttachLoop treats the resulting stream-end as a
	// detach, not a drop, and doesn't hand off to ReconnectLoop.
	s.beginClosing()
	cancel()
	return nil
}

// --- Exit notifications ---

// SetOnExitCallback stores fn, then — under the same lock — checks whether
// the pane already exited before this call and, if so and no callback has
// fired yet, claims the fire-once slot and invokes fn once (Task 2.4.2a).
// This is the "after" half of the check-before-and-after pattern:
// deliverExit (stream.go) is the "before" half, run by readAttachLoop when
// Exited arrives ahead of any registration. Whichever of the two observes
// exitFired == false first under mu wins the single delivery; the other is
// a no-op. Mirrors pane.rs:311-322's wait_exit() shape (check, then
// register/notify, then check again) adapted to Go's synchronous,
// non-blocking registration API.
func (s *tymuxGRPCSession) SetOnExitCallback(fn func(string)) {
	s.mu.Lock()
	s.exitCallback = fn
	reason := s.exitReason
	fire := fn != nil && s.exited && !s.exitFired
	if fire {
		s.exitFired = true
	}
	s.mu.Unlock()
	if fire {
		log.Info("tymux: exit callback fired", "session_id", s.GetSessionIdentifier(), "reason", reason, "registered_after_exit", true)
		fn(reason)
	}
}

// ResetExitOnce clears the observed-exit state so a later exit (e.g. after
// this session object is reused for a restarted session) can fire the
// registered callback again — mirroring TmuxSession.ResetExitOnce
// (tmux.go:373-382). It does not clear exitCallback itself, matching the
// tmux backend's ResetExitOnce, which resets its sync.Once but leaves
// t.onExit in place.
func (s *tymuxGRPCSession) ResetExitOnce() {
	s.mu.Lock()
	s.exited = false
	s.exitReason = ""
	s.exitFired = false
	s.mu.Unlock()
}

// --- Reconnect state (Epic 2.5) ---

// ReconnectState reports whether ReconnectLoop is currently retrying this
// session's standing stream, which attempt it's on, and what triggered it
// ("error" for a transport drop, "output_gap" is never a reconnecting
// state since OutputGap resyncs in place without reopening Attach — see
// resync.go). Task 2.5.2e: closes the gap Phase 4's Product/UX triad
// review found — the aggregate tymux_attach_stream_reconnects_total
// metric can't answer "is *this* session reconnecting right now," and a
// future UI implementing ux.md Surface 2 (reconnect indicator) needs a
// per-session answer to that, not backend archaeology.
func (s *tymuxGRPCSession) ReconnectState() (reconnecting bool, attempt int, cause string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reconnecting, s.reconnectAttempt, s.reconnectCause
}

// BackendRestarted reports whether tymuxd was detected to have restarted
// out from under this session's standing stream (Story 2.5.3) — i.e. the
// currently-live pane is a fresh ReviveSession-spawned replacement
// process, not the one Start()/RestoreWithWorkDir() originally attached
// to, and any in-flight work in the original process (if it survived at
// all as an orphan) is not recovered. Task 2.5.3b: deliberately not
// folded into IsAlive()/SetOnExitCallback — "still alive" and "alive, but
// it's not the same process anymore" are different facts a caller needs
// to tell apart. since is the zero time if no restart has been observed
// in this generation.
func (s *tymuxGRPCSession) BackendRestarted() (restarted bool, since time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backendRestarted, s.backendRestartedAt
}

// compile-time check that *tymuxGRPCSession satisfies TymuxManager.
var _ TymuxManager = (*tymuxGRPCSession)(nil)
