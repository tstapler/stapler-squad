// Package resize provides a small tmux-behavior-aware helper for forcing a
// terminal redraw across a resize. It has no dependency on the concrete tmux
// package or on any streaming subsystem, so it can be shared by every caller
// that resizes a tmux pane (server/services' control-mode streaming paths and
// streamhub's negotiated-resize pipeline) without those callers depending on
// each other or on tmux directly.
package resize

// WithForcedRedraw calls setSize once at (cols-1, rows) — best-effort, its
// error is ignored — then again at the real (cols, rows), returning that call's
// error.
//
// tmux's resize-window command is a no-op, and critically does not send
// SIGWINCH, when the pane is already at the requested size. Without this
// nudge, a same-size resize never causes the inner program to repaint — which
// happens reliably on the first reconnect after a service restart with
// --tmux-keep-server: the tmux server (and its panes' sizes) survive the
// restart, and a client-side dimension cache (e.g. localStorage) reliably
// re-sends those same already-correct dimensions, so the resize that should
// force a fresh draw is a no-op instead, leaving the pane blank.
//
// This consolidates three independently hand-rolled copies of the identical
// dance (server/services/connectrpc_websocket.go's streamViaControlMode and
// streamShellViaControlMode, and streamhub.applyNegotiatedSize) into one
// place: a future caller that resizes a tmux pane gets the fix automatically
// instead of needing to remember to hand-roll it again.
func WithForcedRedraw(setSize func(cols, rows int) error, cols, rows int) error {
	if cols > 1 {
		_ = setSize(cols-1, rows)
	}
	return setSize(cols, rows)
}
