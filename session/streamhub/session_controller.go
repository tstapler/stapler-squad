package streamhub

import "errors"

// ErrSessionNotStarted is the sentinel a SessionController implementation
// returns from the methods below when the session simply hasn't finished
// starting yet (or is paused), as opposed to a dead/crashed controller.
// *session.Instance returns it during its cold-start window
// (session/instance_tmux.go). applyNegotiatedSize (hub.go) checks for it via
// errors.Is to skip-and-retry instead of tearing the whole hub down for a
// transient condition that resolves itself.
var ErrSessionNotStarted = errors.New("session not started or paused")

// SessionController is the narrow interface StreamHub depends on for
// resize/quiescence/capture/teardown, instead of the concrete
// *session.Instance type (Task 1.3.2a). session/streamhub never imports
// package session; *session.Instance satisfies SessionController
// structurally, which is what keeps package session's own dependency on
// session/streamhub (for StreamOwnershipLock, Story 3.1.2) a safe one-way
// edge rather than an import cycle — see plan.md's SessionController
// Pattern Decisions entry.
//
// Scoped to exactly the seven *session.Instance methods it mirrors:
// session/instance_tmux.go's ResizePTY (:587), CapturePaneContentRaw (:772),
// GetPaneCursorPosition (:792), StopControlMode (:727),
// SubscribeControlModeUpdates (:733), UnsubscribeControlModeUpdates (:738),
// and SetWindowSize (:752).
type SessionController interface {
	// SetWindowSize propagates a negotiated resize to the underlying tmux
	// session. StreamHub.applyNegotiatedSize is its sole caller.
	SetWindowSize(cols, rows int) error

	// ResizePTY resizes the terminal dimensions, mirroring
	// *session.Instance.ResizePTY's cols/rows nudge pattern. Part of the
	// interface per Task 1.3.2a; Epic 1.3's resize pipeline does not call it
	// (that call site is the attach-time handshake nudge in
	// server/services/connectrpc_websocket.go, left in place until a later
	// phase migrates it onto the hub).
	ResizePTY(cols, rows int) error

	// CapturePaneContentRaw captures the current visible pane content
	// without joining tmux's soft-wrapped lines (no -J) and with cursor
	// positioning codes intact. The hub calls this exactly once per resize,
	// after quiescence is reached, and once at attach time for a new
	// subscriber's catch-up snapshot; both call sites run the result through
	// prepareSnapshotContent/withCursorSync (snapshot_prepare.go) before
	// broadcasting. Deliberately not the joined CapturePaneContent variant:
	// -J strips the escape codes a snapshot's cursor-sync depends on and
	// collapses tmux's own wrap points, which is what made every post-resize
	// snapshot render staircased across the previous frame (2026-08-25
	// reflow bug — see snapshot_prepare.go's doc comment).
	CapturePaneContentRaw() (RawPaneContent, error)

	// GetPaneCursorPosition reports the tmux pane's current cursor
	// coordinates, used by withCursorSync (snapshot_prepare.go) to reposition
	// the client cursor after a snapshot renders.
	GetPaneCursorPosition() (x, y int, err error)

	// StopControlMode stops the control-mode stream. Called exactly once by
	// StreamHub.ForceTeardown.
	StopControlMode() error

	// SubscribeControlModeUpdates registers a consumer of raw control-mode
	// output for this session, returning a subscriber ID and a read-only
	// channel of raw frames. StreamHub.applyNegotiatedSize uses this to
	// detect quiescence after a resize.
	SubscribeControlModeUpdates() (string, <-chan []byte)

	// UnsubscribeControlModeUpdates removes a subscription by the ID
	// returned from SubscribeControlModeUpdates. Implementations must close
	// the corresponding channel so a range loop reading from it can exit
	// without leaking.
	UnsubscribeControlModeUpdates(id string)
}
