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
// Story 2.2.6's original doc comment here claimed stapler-squad does not
// start or supervise tymuxd itself; that scope decision is superseded by
// project_plans/tymux-bundled-integration/decisions/ADR-003-supersede-story-2-2-6-no-supervision.md —
// stapler-squad now supervises tymuxd (start-if-not-running, health-check)
// whenever the tymux backend is in use. This sentinel's own scope is
// unchanged by that: it is produced in exactly one place, classifyRPCError
// below, and means "an RPC against an already-in-use session's transport
// hit a transport-level Unavailable" -- e.g. a previously-healthy tymuxd
// that has since crashed or become unreachable mid-session. It is NOT
// produced by session/tymux/supervise.go's EnsureDaemonRunning: that
// function's own failure paths (ErrTymuxdPortSquatted below, or a plain
// "did not become healthy" error for a supervision-start failure before any
// session ever attaches) are distinct error values -- errors.Is(err,
// ErrTymuxdUnreachable) is false for both. Callers match THIS sentinel with
// errors.Is to distinguish "an in-flight session's daemon connection dropped"
// from "daemon rejected the request" — research/ux.md:218-224's "should not
// present as the same 'reconnecting' transient state." A caller that wants
// to detect a supervision-start failure specifically should check
// EnsureDaemonRunning's returned error (or errors.Is(err,
// ErrTymuxdPortSquatted) for the port-squat case) instead.
var ErrTymuxdUnreachable = errors.New("tymux: tymuxd unreachable")

// ErrTymuxdPortSquatted indicates EnsureDaemonRunning (session/tymux/supervise.go)
// found something already listening at a DaemonConfig.Addr that never answers
// a ListSessions RPC correctly, even after every spawn-and-retry attempt —
// i.e. a non-tymuxd process (or an incompatible/misbehaving tymuxd) has taken
// the port. Task 2.1.2d: this is a distinct failure mode from "nothing is
// listening yet, so spawn one" and is surfaced loudly rather than silently
// proceeding with a session pointed at an unverified daemon
// (research/pitfalls.md §2/§4's shared mitigation).
var ErrTymuxdPortSquatted = errors.New("tymux: tymuxd port squatted by another process")

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
