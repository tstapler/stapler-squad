package diagnose

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// GateInput carries the per-attempt context a NudgeGateCheck needs to decide
// whether one nudge write may proceed. Story 3.4.1 started this minimal (only
// ItemID/SessionUUID, exercised by stub/spy checks); Phase 4 wiring
// (server/mcp/diagnose_gate_wiring.go, the package that constructs
// NewNudgeGate with real checks) extended it below with the fields the real
// idle/identity checks need. Existing fields must not be repurposed.
type GateInput struct {
	// ItemID is the backlog item being considered for a nudge. Consumed by
	// the cap check (server/services.NewNudgeCapGateCheck's itemID param).
	ItemID string
	// SessionUUID is the target session's own recorded UUID -- the value the
	// identity check re-verifies against, mirroring
	// verifyIdentityImmediatelyBeforeWrite's expectedSessionUUID parameter
	// (session/tmux/write_gate_ownership.go).
	SessionUUID string

	// --- Phase 4 additions: consumed by the idle and identity checks ---

	// Now is the observation time for the idle-settle-window check
	// (IdleGate.Evaluate). Passed explicitly, not read via time.Now() inside
	// the check itself, so the check stays deterministic under test --
	// mirrors IdleGate.Evaluate's own convention.
	Now time.Time
	// Program, Status, and StatusContext are the target session's currently
	// detected program/status/status-context triple, forwarded verbatim to
	// IdleGate.Evaluate's identically-named parameters.
	Program       string
	Status        detection.DetectedStatus
	StatusContext string
	// Socket and PaneName identify the target session's tmux pane, forwarded
	// to verifyIdentityImmediatelyBeforeWrite's identically-named parameters.
	Socket   string
	PaneName string
}

// NudgeGateCheck is one link in the NudgeGate pipeline: it decides whether a
// nudge write may proceed based on input, returning (false, reason) to block
// it. A check must never propagate an internal error out through this
// signature -- there is no error return, by design, mirroring
// NewNudgeCapGateCheck's precedent (server/services/nudge_cap_store.go): any
// ambiguity a check encounters (a store error, a transport failure reading a
// tmux marker, etc.) must resolve to (false, someReason) inside the check
// itself, never bubble up as "the pipeline doesn't know what to do." Success
// is reported as (true, "") -- the zero value of SafetyGateReason.
//
// NOTE for Phase 4 wiring: NewNudgeCapGateCheck currently returns
// func(ctx context.Context, itemID string) (bool, SafetyGateReason) -- a
// loose-itemID shape, not this GateInput-based one. It is NOT structurally
// assignable to NudgeGateCheck as defined here and needs a thin adapter at
// the call site, e.g.:
//
//	func adaptCapCheck(capCheck func(context.Context, string) (bool, diagnose.SafetyGateReason)) diagnose.NudgeGateCheck {
//		return func(ctx context.Context, input diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
//			return capCheck(ctx, input.ItemID)
//		}
//	}
//
// session/diagnose intentionally does not define this adapter itself (it
// would have nothing to adapt to without importing server/services, which
// cycles -- see nudge_cap_store.go's own header comment).
type NudgeGateCheck func(ctx context.Context, input GateInput) (bool, SafetyGateReason)

// NudgeGate is the ordered, fail-closed pipeline of nudge-safety checks a
// write must pass: flag, then idle, then identity, then cap -- exactly the
// order the plan's Domain Glossary and Story 3.4.1 require. It holds no
// dependencies of its own; NewNudgeGate takes the four checks as already-
// constructed closures, so this package never needs to import anything that
// builds a real check (avoiding the server/services import-cycle problem
// documented on NudgeGateCheck and in nudge_cap_store.go).
//
// # Why the identity check runs here AND again immediately before the write
//
// This pipeline's identity check and the caller's final pre-write
// verifyIdentityImmediatelyBeforeWrite call (added in a later Phase 4 story,
// at the actual write call site) are deliberately the SAME function, called
// twice -- not redundant dead code. ADR-002 requires identity to be
// re-verified with NO I/O between the check and the write it guards; this
// pipeline's identity check runs earlier, alongside the other three checks,
// so a flag/idle/cap failure short-circuits before any identity I/O is even
// attempted. But because other work (constructing the write payload, etc.)
// may happen between this pipeline returning and the write actually firing,
// ADR-002 also requires a second, literal-last-step-before-write identity
// check at the call site itself. Both calls are idempotent and read-only, so
// calling the same check twice is cheap and correct -- it is the only way to
// satisfy both "identity is one of the ordered checks" and "identity is
// re-verified immediately before write" at once.
type NudgeGate struct {
	checks []NudgeGateCheck
}

// NewNudgeGate constructs a NudgeGate running flagCheck, idleCheck,
// identityCheck, then capCheck, in that fixed order. Each parameter is a
// plain NudgeGateCheck value -- the real flag/idle/identity/cap closures are
// constructed elsewhere (Phase 4 wiring, in a package that can import both
// session/diagnose and server/services) and passed in here by value.
func NewNudgeGate(flagCheck, idleCheck, identityCheck, capCheck NudgeGateCheck) *NudgeGate {
	return &NudgeGate{
		checks: []NudgeGateCheck{flagCheck, idleCheck, identityCheck, capCheck},
	}
}

// Evaluate runs the pipeline's checks in order, short-circuiting on the
// first one that returns false: later checks -- including identity, even
// when it isn't the failing one -- are never evaluated. Returns (true, nil)
// only when every check passes; otherwise returns (false, &reason) naming
// the specific check that failed. This is the fail-closed contract: any
// single check's false result, for any reason (including one representing
// an internal error the check couldn't resolve any other way), aborts the
// whole pipeline -- there is no default-allow path.
func (g *NudgeGate) Evaluate(ctx context.Context, input GateInput) (bool, *SafetyGateReason) {
	for _, check := range g.checks {
		ok, reason := check(ctx, input)
		if !ok {
			return false, &reason
		}
	}
	return true, nil
}
