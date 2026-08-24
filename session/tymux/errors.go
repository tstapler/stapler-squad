package tymux

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
)

// ErrTymuxdUnreachable indicates an RPC could not reach a tymuxd daemon at
// all — connection refused, or nothing listening on the configured
// socket/address — as opposed to a request that reached tymuxd but was
// rejected there (a live daemon telling us "no" is a different failure mode
// than no daemon to ask).
//
// Story 2.2.6 / adversarial-review.md Blocker: stapler-squad does not start
// or supervise tymuxd itself (a deliberate scope decision — no
// ensureServerRunning-equivalent, unlike BackendTmux's
// recoverFromServerFailure). It assumes an out-of-band, already-running
// daemon. Callers match this sentinel with errors.Is to distinguish "daemon
// not started/crashed/misconfigured" from "daemon rejected the request" —
// research/ux.md:218-224's "should not present as the same 'reconnecting'
// transient state."
var ErrTymuxdUnreachable = errors.New("tymux: tymuxd unreachable")

// classifyRPCError wraps an error returned by an rpcTransport call so a
// caller can test errors.Is(err, ErrTymuxdUnreachable) whenever the failure
// happened at the transport level (Connect-Go's connect.CodeUnavailable —
// dial/connect failure) rather than as an ordinary RPC-level rejection from
// a daemon that was reachable. The underlying Connect-Go error's message is
// preserved via %w, never discarded (research/ux.md:224: "should not swallow
// the underlying connect error string"). Returns nil unchanged.
func classifyRPCError(op string, err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeUnavailable {
		return fmt.Errorf("tymux: %s: %w: %w", op, ErrTymuxdUnreachable, err)
	}
	return fmt.Errorf("tymux: %s: %w", op, err)
}
