package tymux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

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
}

// NewTymuxGRPCSession constructs a tymuxGRPCSession using the given rpcTransport,
// returned as a TymuxManager since tymuxGRPCSession itself is unexported.
func NewTymuxGRPCSession(transport rpcTransport) TymuxManager {
	return &tymuxGRPCSession{transport: transport}
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

// cacheFromSession stores sess's id/pane/cwd and marks the session live —
// the common tail of Start/RestoreWithWorkDir once a *v1.Session is in hand.
func (s *tymuxGRPCSession) cacheFromSession(sess *v1.Session) error {
	pane := firstPane(sess)
	if pane == nil {
		return fmt.Errorf("tymux: session %q has no pane", sess.GetId())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionID = sess.GetId()
	s.paneID = pane.GetId()
	s.cwd = pane.GetCwd()
	s.liveness = v1.Liveness_LIVENESS_LIVE
	s.closed = false
	return nil
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
		return classifyRPCError("Close", err)
	}
	return nil
}

// IsAlive reports whether the session is currently live. Task 2.2.1c: the
// cached liveness field only stays fresh for free once Epic 2.3's standing
// Attach stream exists to push Exited events into it; until then (i.e.
// today), every call here falls back to a CapturePane-based read so the
// answer reflects tymuxd's current state rather than a stale cache — except
// once Close() has run, which short-circuits to false without a further RPC
// against a session this process itself just killed.
func (s *tymuxGRPCSession) IsAlive() bool {
	s.mu.RLock()
	paneID := s.paneID
	closed := s.closed
	s.mu.RUnlock()
	if paneID == "" || closed {
		return false
	}
	snap, err := s.capturePane(context.Background(), 0)
	if err != nil {
		return false
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

func (s *tymuxGRPCSession) SendKeys(keys string) (int, error)  { return 0, ErrNotImplemented }
func (s *tymuxGRPCSession) TapEnter() error                    { return ErrNotImplemented }
func (s *tymuxGRPCSession) SendPromptWithEnter(p string) error { return ErrNotImplemented }
func (s *tymuxGRPCSession) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return ErrNotImplemented
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

func (s *tymuxGRPCSession) SetWindowSize(cols, rows int) error { return ErrNotImplemented }
func (s *tymuxGRPCSession) SetDetachedSize(w, h int, title string) error {
	return ErrNotImplemented
}
func (s *tymuxGRPCSession) RefreshClient() error { return ErrNotImplemented }

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
func (s *tymuxGRPCSession) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (s *tymuxGRPCSession) UnsubscribeFromControlModeUpdates(id string) {}

// --- Attach ---

func (s *tymuxGRPCSession) Attach() (chan struct{}, error) { return nil, ErrNotImplemented }
func (s *tymuxGRPCSession) DetachSafely() error            { return ErrNotImplemented }

// --- Exit notifications ---

func (s *tymuxGRPCSession) SetOnExitCallback(fn func(string)) {}
func (s *tymuxGRPCSession) ResetExitOnce()                    {}

// compile-time check that *tymuxGRPCSession satisfies TymuxManager.
var _ TymuxManager = (*tymuxGRPCSession)(nil)
