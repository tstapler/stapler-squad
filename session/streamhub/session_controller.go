package streamhub

// SessionController is the narrow interface StreamHub depends on for
// resize/quiescence/capture/teardown, instead of the concrete
// *session.Instance type (Task 1.3.2a). session/streamhub never imports
// package session; *session.Instance satisfies SessionController
// structurally, which is what keeps package session's own dependency on
// session/streamhub (for StreamOwnershipLock, Story 3.1.2) a safe one-way
// edge rather than an import cycle — see plan.md's SessionController
// Pattern Decisions entry.
//
// Scoped to exactly the six *session.Instance methods it mirrors:
// session/instance_tmux.go's ResizePTY (:587), CapturePaneContent (:600),
// StopControlMode (:727), SubscribeControlModeUpdates (:733),
// UnsubscribeControlModeUpdates (:738), and SetWindowSize (:752).
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

	// CapturePaneContent captures the current visible pane content. The hub
	// calls this exactly once per resize, after quiescence is reached, and
	// broadcasts the result to every attached subscriber.
	CapturePaneContent() (string, error)

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
