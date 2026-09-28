package tmux

import (
	"context"
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// SessionIdentity is the immutable pairing of a session's UUID and the owner
// marker written into its tmux pane, used to re-verify identity right before
// a nudge write (see ADR-002). Identity comparison lives here so it isn't
// re-derived ad hoc at each nudge call site.
type SessionIdentity struct {
	SessionUUID     string
	TmuxOwnerMarker string
}

// Matches reports whether both the session UUID and the tmux owner marker
// agree with expectedSessionUUID. On mismatch it names which check failed --
// the UUID check runs first, so a SessionUUID mismatch is reported even if
// TmuxOwnerMarker also disagrees.
func (id SessionIdentity) Matches(expectedSessionUUID string) (bool, diagnose.SafetyGateReason) {
	if id.SessionUUID != expectedSessionUUID {
		return false, diagnose.SafetyGateReasonIdentityMismatchInstance
	}
	if id.TmuxOwnerMarker != expectedSessionUUID {
		return false, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker
	}
	return true, ""
}

// sessionOwnerMarkerEnvVar is the tmux-pane environment marker stamped at
// session creation, identifying which session UUID a pane belongs to. Named
// distinctly from any shared constant in session/orphan_sweep.go or
// session/workspace_peers.go (which read-back the same env var name for their
// own, unrelated purposes) -- see ADR-002's Consequences section: this is
// accepted, documented duplication rather than a shared import, since this
// package cannot depend on the session package (see
// InstanceIdentitySnapshot's doc comment) and the plan explicitly declines to
// depend on the unmerged ce71ad1a tmux-ownership branch.
const sessionOwnerMarkerEnvVar = "STAPLER_SESSION_UUID"

// readSessionOwnerMarker reads the STAPLER_SESSION_UUID environment marker
// stamped on the tmux pane/session named paneName, via the same
// `tmux show-environment`-style read-back mechanism
// session/orphan_sweep.go's ReconcileOrphanedTmuxSessions and
// session/workspace_peers.go's LiveTmuxSessionUUIDs already use elsewhere in
// this codebase. Returns a non-nil error -- never a default/empty-string
// success -- when the pane is gone, the marker was never set, or the marker
// was explicitly removed from the environment (tmux prefixes a removed
// variable's show-environment line with "-", per tmux(1)).
func readSessionOwnerMarker(ctx context.Context, socket, paneName string) (string, error) {
	if paneName == "" {
		return "", fmt.Errorf("readSessionOwnerMarker: paneName must not be empty")
	}

	args := prependSocket(socket, []string{"show-environment", "-t", paneName, sessionOwnerMarkerEnvVar})
	out, err := runGated(ctx, socket, func() ([]byte, error) {
		return safeexec.CommandContext(ctx, ResolveClientForSocket(socket), args...).Output()
	})
	if err != nil {
		return "", fmt.Errorf("readSessionOwnerMarker: pane %q: %w", paneName, err)
	}

	line := strings.TrimSpace(string(out))
	if strings.HasPrefix(line, "-") {
		return "", fmt.Errorf("readSessionOwnerMarker: pane %q: marker %s was removed from the pane environment", paneName, sessionOwnerMarkerEnvVar)
	}
	marker := strings.TrimPrefix(line, sessionOwnerMarkerEnvVar+"=")
	if marker == "" || marker == line {
		return "", fmt.Errorf("readSessionOwnerMarker: pane %q: marker %s is absent or malformed (raw output %q)", paneName, sessionOwnerMarkerEnvVar, line)
	}
	return marker, nil
}

// InstanceIdentitySnapshot is the minimal identity surface
// verifyIdentityImmediatelyBeforeWrite needs from a session Instance: its own
// recorded session UUID, read via Instance.Snapshot() (never a raw field),
// per .claude/rules/instance-lock-free-reads.md.
//
// This is a narrow interface, not *session.Instance directly, even though
// ADR-002 and the implementation plan name the parameter as such: package
// session imports package session/tmux (this package) at 30+ call sites, so
// session/tmux importing session.Instance back would create an import cycle
// and fail `go build ./...`. The real nudge-write call site (server/mcp,
// which can import both session and session/tmux with no cycle) satisfies
// this interface with a one-line adapter around inst.Snapshot().UUID.
type InstanceIdentitySnapshot interface {
	// SnapshotSessionUUID returns the instance's own recorded session UUID.
	SnapshotSessionUUID() string
}

// IdentityMismatchError reports which of verifyIdentityImmediatelyBeforeWrite's
// two checks failed. Callers identify which one via errors.Is against
// ErrIdentityMismatchInstance / ErrIdentityMismatchTmuxMarker below, rather
// than string-matching Error() or type-asserting this struct's fields.
type IdentityMismatchError struct {
	Reason diagnose.SafetyGateReason
	// Cause is the underlying read failure when Reason is
	// IdentityMismatchTmuxMarker and readSessionOwnerMarker itself errored
	// (pane gone, marker absent/removed) -- nil for a plain value mismatch
	// with no read error.
	Cause error
}

// Error implements the error interface.
func (e *IdentityMismatchError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("verifyIdentityImmediatelyBeforeWrite: %s: %v", e.Reason, e.Cause)
	}
	return fmt.Sprintf("verifyIdentityImmediatelyBeforeWrite: %s", e.Reason)
}

// Unwrap exposes Cause to errors.Is/errors.As chains.
func (e *IdentityMismatchError) Unwrap() error { return e.Cause }

// Is reports whether target is an *IdentityMismatchError with the same
// Reason, ignoring Cause -- so errors.Is(err, ErrIdentityMismatchTmuxMarker)
// identifies which check failed regardless of whether that check failed via
// an explicit mismatch or a read error.
func (e *IdentityMismatchError) Is(target error) bool {
	t, ok := target.(*IdentityMismatchError)
	return ok && t.Reason == e.Reason
}

// ErrIdentityMismatchInstance and ErrIdentityMismatchTmuxMarker are sentinel
// values for errors.Is checks against verifyIdentityImmediatelyBeforeWrite's
// returned error -- see IdentityMismatchError.Is.
var (
	ErrIdentityMismatchInstance   = &IdentityMismatchError{Reason: diagnose.SafetyGateReasonIdentityMismatchInstance}
	ErrIdentityMismatchTmuxMarker = &IdentityMismatchError{Reason: diagnose.SafetyGateReasonIdentityMismatchTmuxMarker}
)

// verifyIdentityImmediatelyBeforeWrite is the facade ADR-002 requires
// immediately before any nudge write (SendKeysWithTimeout/
// SubmitContentWithEnter): it performs the Instance-Snapshot() check and the
// tmux-pane-marker check back-to-back, with NO I/O between them, inside this
// single un-splittable function body -- not two call sites a future edit
// could separate. Go cannot verify "no I/O between two lines" automatically
// (see ADR-002's Decision section and validation.md's explicit caveat on
// this file's own test suite); this structure is the enforcement, not a
// convention callers must remember.
//
// Both reads always happen, even if the first already disagrees with
// expectedSessionUUID -- the two checks answer different questions (does the
// Go heap object still believe it's session X; does the OS-level tmux pane
// still carry session X's marker) and neither subsumes the other, so a
// caller building a full SessionIdentity always gets both values.
//
// Returns (SessionIdentity{}, non-nil) on ANY ambiguity: an Instance-UUID
// mismatch, a tmux marker read failure, or a tmux marker mismatch -- fail
// closed, never "verification skipped, proceed anyway." Use errors.Is against
// ErrIdentityMismatchInstance / ErrIdentityMismatchTmuxMarker to identify
// which check failed.
func verifyIdentityImmediatelyBeforeWrite(ctx context.Context, inst InstanceIdentitySnapshot, socket, paneName, expectedSessionUUID string) (SessionIdentity, error) {
	instanceUUID := inst.SnapshotSessionUUID()
	tmuxMarker, markerErr := readSessionOwnerMarker(ctx, socket, paneName)

	if instanceUUID != expectedSessionUUID {
		return SessionIdentity{}, &IdentityMismatchError{Reason: diagnose.SafetyGateReasonIdentityMismatchInstance}
	}
	if markerErr != nil {
		return SessionIdentity{}, &IdentityMismatchError{Reason: diagnose.SafetyGateReasonIdentityMismatchTmuxMarker, Cause: markerErr}
	}

	identity := SessionIdentity{SessionUUID: instanceUUID, TmuxOwnerMarker: tmuxMarker}
	if ok, reason := identity.Matches(expectedSessionUUID); !ok {
		return SessionIdentity{}, &IdentityMismatchError{Reason: reason}
	}
	return identity, nil
}

// VerifyIdentityImmediatelyBeforeWrite is the exported entry point for
// verifyIdentityImmediatelyBeforeWrite (Phase 4 wiring, server/mcp): a thin,
// no-I/O passthrough, so the fail-closed, single-function-body contract
// documented on the unexported function above still holds -- there is
// nothing for a future edit to insert between this wrapper and the real
// check. Call sites are server/mcp/diagnose_gate_wiring.go (the NudgeGate
// pipeline's own identity check) and the steer_session/write_to_session/
// resume_session handlers' final pre-write re-check (ADR-002).
func VerifyIdentityImmediatelyBeforeWrite(ctx context.Context, inst InstanceIdentitySnapshot, socket, paneName, expectedSessionUUID string) (SessionIdentity, error) {
	return verifyIdentityImmediatelyBeforeWrite(ctx, inst, socket, paneName, expectedSessionUUID)
}
