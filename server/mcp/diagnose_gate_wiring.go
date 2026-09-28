package mcp

// diagnose_gate_wiring.go -- Epic 4.1's NudgeGate pipeline assembly for the
// steer_session/write_to_session/resume_session MCP write handlers. Lives in
// server/mcp, not session/diagnose or server/services, because this is the
// one package that can import both without a cycle: session/diagnose defines
// NudgeGate/NudgeGateCheck but must not depend on server/services, and
// server/services's NewNudgeCapGateCheck deliberately returns a loose-typed
// closure for exactly this reason -- see nudge_cap_store.go's header comment
// and NudgeGateCheck's own doc comment (session/diagnose/nudge_gate.go).

import (
	"context"
	"errors"
	"sync"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// nudgeGateEvaluator is the narrow interface the steer/write/resume handlers
// depend on -- satisfied by *diagnose.NudgeGate. A seam so tests can inject a
// scripted pipeline result without standing up real flag/idle/identity/cap
// infrastructure; production wiring (NewCore, server.go) always passes a real
// *diagnose.NudgeGate built by NewDiagnoseNudgeGate.
type nudgeGateEvaluator interface {
	Evaluate(ctx context.Context, input diagnose.GateInput) (bool, *diagnose.SafetyGateReason)
}

// instanceUUIDSnapshot adapts a plain session-UUID string to
// tmux.InstanceIdentitySnapshot, satisfying the interface with no dependency
// on *session.Instance. Used by the NudgeGate pipeline's own identity check
// (evaluateIdentity), which has only GateInput's plain strings to work with.
type instanceUUIDSnapshot string

func (u instanceUUIDSnapshot) SnapshotSessionUUID() string { return string(u) }

// liveInstanceUUIDSnapshot adapts a live *session.Instance to
// tmux.InstanceIdentitySnapshot, re-reading Snapshot().UUID on every call
// (never a raw field access, per .claude/rules/instance-lock-free-reads.md).
// Used by verifyBeforeWrite's final, literal-last-step-before-write
// re-check -- as opposed to instanceUUIDSnapshot's frozen string, this one is
// live specifically so a UUID change between gate evaluation and the write
// itself (the class of drift ADR-002 exists to catch) is observed fresh.
type liveInstanceUUIDSnapshot struct{ inst *session.Instance }

func (l liveInstanceUUIDSnapshot) SnapshotSessionUUID() string { return l.inst.Snapshot().UUID }

// idleGateRegistry tracks one diagnose.IdleGate per target session UUID, so
// the idle-settle window's state (how long the session has been continuously
// idle) persists across repeated steer_session/write_to_session/
// resume_session polls for the same session rather than resetting on every
// call. Left to grow for the scope of Stories 4.1.1-4.1.3 -- no eviction
// hook on session end yet; a follow-up should wire one (e.g. into the
// session-archive/end path) if the map's size becomes a concern in practice.
type idleGateRegistry struct {
	gates sync.Map // map[string]*diagnose.IdleGate, keyed by session UUID
}

func newIdleGateRegistry() *idleGateRegistry {
	return &idleGateRegistry{}
}

// get returns the IdleGate for sessionUUID, constructing one with
// windowSeconds on first use. An in-progress settle window for an
// already-registered session keeps its original window even if config
// changes afterward -- IdleGate has no live-reconfigure method.
func (r *idleGateRegistry) get(sessionUUID string, windowSeconds int) *diagnose.IdleGate {
	if v, ok := r.gates.Load(sessionUUID); ok {
		return v.(*diagnose.IdleGate)
	}
	fresh := diagnose.NewIdleGate(windowSeconds, nil)
	actual, _ := r.gates.LoadOrStore(sessionUUID, fresh)
	return actual.(*diagnose.IdleGate)
}

// NewDiagnoseNudgeGate constructs the real flag/idle/identity/cap pipeline
// (diagnose.NewNudgeGate's fixed order) for the steer/write/resume MCP write
// handlers (Epic 4.1). cfgFn is read fresh on every check
// (feedback_rollout_flags_live_settable_no_env_vars) -- pass config.LoadConfig
// in production. storage may be nil (e.g. the stdio fallback transport,
// ADR-001, mirrors NewCore's other storage-optional handling elsewhere) --
// the cap check then fails closed (SafetyGateReasonNudgeCapReached) rather
// than calling into a nil *session.Storage.
func NewDiagnoseNudgeGate(cfgFn func() *config.Config, storage *session.Storage, idleGates *idleGateRegistry) *diagnose.NudgeGate {
	flagCheck := func(_ context.Context, _ diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
		cfg := cfgFn()
		if cfg == nil || !cfg.GetFeatureFlagWithDefault(config.DiagnoseNudgeExecutionFeatureFlag, false) {
			return false, diagnose.SafetyGateReasonNudgeExecutionDisabled
		}
		return true, ""
	}

	idleCheck := func(_ context.Context, input diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
		var nudgeCfg config.DiagnoseNudgeConfig
		if cfg := cfgFn(); cfg != nil {
			nudgeCfg = cfg.DiagnoseNudge
		}
		gate := idleGates.get(input.SessionUUID, nudgeCfg.IdleSettleWindowSecondsOrDefault())
		ok, reason := gate.Evaluate(input.Now, input.Program, input.Status, input.StatusContext)
		if !ok {
			return false, *reason
		}
		return true, ""
	}

	capCheck := newNudgeCapCheck(storage, cfgFn)

	return diagnose.NewNudgeGate(flagCheck, idleCheck, evaluateIdentity, capCheck)
}

// evaluateIdentity is the NudgeGate pipeline's own identity check (ADR-002):
// it runs alongside flag/idle/cap, using only GateInput's plain fields, so a
// flag/idle/cap failure short-circuits before this check's tmux I/O is even
// attempted. Because both sides of the comparison come from the same
// GateInput snapshot (built moments earlier, at handler entry -- see
// buildGateInput), this check can only meaningfully fail on the tmux-marker
// side; the Instance-UUID side is authoritative only for the SEPARATE final
// pre-write re-check (verifyBeforeWrite), which re-reads the live Instance
// against that same frozen expectation right before the write itself -- see
// that function's doc comment and NudgeGate's own doc comment for why both
// calls are required, not redundant.
func evaluateIdentity(ctx context.Context, input diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
	_, err := tmux.VerifyIdentityImmediatelyBeforeWrite(ctx, instanceUUIDSnapshot(input.SessionUUID), input.Socket, input.PaneName, input.SessionUUID)
	if err == nil {
		return true, ""
	}
	return false, identityMismatchReason(err)
}

// identityMismatchReason maps a verifyIdentityImmediatelyBeforeWrite error to
// its SafetyGateReason, falling back to IdentityMismatchInstance for any
// error shape other than *tmux.IdentityMismatchError (defensive; that
// facade's documented contract is to return only that type or nil).
func identityMismatchReason(err error) diagnose.SafetyGateReason {
	var mismatch *tmux.IdentityMismatchError
	if errors.As(err, &mismatch) {
		return mismatch.Reason
	}
	return diagnose.SafetyGateReasonIdentityMismatchInstance
}

// newNudgeCapCheck adapts services.NewNudgeCapGateCheck's loose
// func(ctx, itemID) shape to diagnose.NudgeGateCheck -- see NudgeGateCheck's
// doc comment (session/diagnose/nudge_gate.go) for why the adapter must live
// here rather than in either package it bridges.
func newNudgeCapCheck(storage *session.Storage, cfgFn func() *config.Config) diagnose.NudgeGateCheck {
	if storage == nil {
		return func(context.Context, diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
			return false, diagnose.SafetyGateReasonNudgeCapReached
		}
	}
	capStore := services.NewNudgeCapStore(storage)
	raw := services.NewNudgeCapGateCheck(capStore, cfgFn)
	return func(ctx context.Context, input diagnose.GateInput) (bool, diagnose.SafetyGateReason) {
		return raw(ctx, input.ItemID)
	}
}

// buildGateInput assembles a diagnose.GateInput from inst -- the target
// session about to be written to -- at the moment of the write attempt.
//
// ItemID keys the nudge cap/cooldown check (server/services.NudgeCapStore).
// Stories 4.1.1-4.1.3 gate steer_session/write_to_session/resume_session,
// which are generic session tools with no backlog-item argument of their
// own -- there is no itemID available at this call site the way there is on
// the backlog-item-scoped MCP tools (see tools_backlog.go's resolveItemLink).
// So each TARGET SESSION gets its own independent cap/cooldown budget (its
// own UUID doubling as ItemID) rather than every session touched through
// these tools sharing one global bucket, which the NudgeCapRecord schema's
// NotEmpty item_id column would make a hard requirement violation for an
// empty itemID anyway. A caller acting on behalf of a real backlog item (the
// diagnose-and-nudge dispatch path) can be captured per-item once Story
// 4.1.4's DiagnosticSessionUUID->DiagnoseDispatch->ItemID resolution lands --
// out of this story's scope; flagged for that worker.
func buildGateInput(inst *session.Instance) diagnose.GateInput {
	snap := inst.Snapshot()
	return diagnose.GateInput{
		ItemID:        snap.UUID,
		SessionUUID:   snap.UUID,
		Now:           time.Now(),
		Program:       snap.Program,
		Status:        inst.GetDetectedStatus(),
		StatusContext: inst.GetDetectedContext(),
		Socket:        snap.TmuxServerSocket,
		PaneName:      inst.GetTmuxSessionName(),
	}
}

// diagnoseOutcomeStore is the narrow surface Story 5.2.2's outcome-notify
// call sites need from services.DiagnoseDispatchStore -- resolving the
// CALLING session's own UUID (never the target session being written to,
// same distinction checkDuplicateWriteGuard's doc comment makes) to an
// in-flight DiagnoseDispatch row.
type diagnoseOutcomeStore interface {
	FindPendingByDiagnosticSessionUUID(ctx context.Context, diagnosticSessionUUID string) (services.DiagnoseDispatchRecord, bool, error)
}

// diagnoseOutcomeRecorder is the narrow surface Story 5.2.2's outcome-notify
// call sites need from *services.DiagnoseDispatcher -- satisfied by its
// RecordDiagnoseOutcome wrapper around the unexported notifyDiagnoseEvent
// (see that function's doc comment for why the wrapper exists).
type diagnoseOutcomeRecorder interface {
	RecordDiagnoseOutcome(ctx context.Context, dispatchID, itemID string, outcome diagnose.DiagnoseOutcome)
}

// diagnoseOutcomeHooks bundles diagnoseOutcomeStore and diagnoseOutcomeRecorder
// into one value so evaluateNudgeGate/verifyBeforeWrite/checkDuplicateWriteGuard
// and the write handlers that call them take one extra parameter, not two --
// mirroring nudgeGateEvaluator/diagnoseDispatchWriteGuard's existing "one
// narrow interface per gate concern" shape. The zero value (both fields nil)
// makes recordDiagnoseOutcomeIfDispatched a no-op, matching nudgeGate/
// dispatchWriteGuard's own nil-skips-entirely convention for a server
// configuration with storage == nil (e.g. the stdio fallback transport).
type diagnoseOutcomeHooks struct {
	store    diagnoseOutcomeStore
	recorder diagnoseOutcomeRecorder
}

// recordDiagnoseOutcomeIfDispatched resolves the CALLING session's own UUID
// (read from ctx, not the target session being acted upon) to an in-flight
// DiagnoseDispatch row and, if found, records buildOutcome()'s outcome
// against it via hooks.recorder -- Story 5.2.2's fix for notifyDiagnoseEvent's
// documented gap: before this, only handleDispatchFailure's DispatchFailed
// branch ever reached it. Mirrors checkDuplicateWriteGuard's exact
// resolve-caller-not-target pattern and three-way skip contract: a no-op (no
// error, no notification) when hooks.store/hooks.recorder are nil (dispatch
// store unwired, e.g. the stdio fallback transport with storage == nil),
// when the caller has no STAPLER_SESSION_UUID in context (manual/external
// MCP client), or when the caller's session has no PENDING DiagnoseDispatch
// row at all -- covering both "not a diagnose dispatch" (Tyler manually
// calling these tools) and "already completed" (idempotency: a second
// outcome-producing call within the same dispatch must not re-notify -- see
// FindPendingByDiagnosticSessionUUID's doc comment).
//
// A lookup error fails OPEN (skip, log, return) rather than closed, unlike
// checkDuplicateWriteGuard's fail-closed behavior -- this is a best-effort
// visibility notification, not a write-safety gate, so a storage hiccup here
// must never block or corrupt the write/tool-call outcome that already
// happened.
//
// buildOutcome is called lazily, only once a matching Pending row is
// actually found, so callers needn't construct a DiagnoseOutcome on the
// (common) no-op path.
func recordDiagnoseOutcomeIfDispatched(ctx context.Context, hooks diagnoseOutcomeHooks, buildOutcome func() diagnose.DiagnoseOutcome) {
	if hooks.store == nil || hooks.recorder == nil {
		return
	}
	callerUUID, ok := sessionUUIDFromContext(ctx)
	if !ok {
		return
	}
	record, found, err := hooks.store.FindPendingByDiagnosticSessionUUID(ctx, callerUUID)
	if err != nil {
		log.Error("diagnose outcome recorder: lookup failed", "caller_session_uuid", callerUUID, "error", err)
		return
	}
	if !found {
		return
	}
	hooks.recorder.RecordDiagnoseOutcome(ctx, record.ID, record.ItemID, buildOutcome())
}

// recordSkippedSafetyGateOutcome records a SkippedSafetyGate outcome for the
// calling session (Story 5.2.2, gap item 4): the safety-gate rejection
// itself (flag/idle/identity/cap via evaluateNudgeGate, the final pre-write
// identity re-check via verifyBeforeWrite, or the dispatch-level
// duplicate-write guard via checkDuplicateWriteGuard) is the ONE thing worth
// recording -- these three are the only producers of a gateFailureResult in
// this file, each called at most once per handler invocation before its
// write (an earlier rejection short-circuits the handler, so a later gate is
// never reached), so this can never double-notify for a single call. Across
// separate calls, FindPendingByDiagnosticSessionUUID's Pending-only filter
// (recordDiagnoseOutcomeIfDispatched) is what prevents a double-notify: once
// any outcome-producing call (including this one) completes the dispatch
// row, every later call for the same dispatch becomes a silent no-op.
func recordSkippedSafetyGateOutcome(ctx context.Context, hooks diagnoseOutcomeHooks, reason diagnose.SafetyGateReason) {
	recordDiagnoseOutcomeIfDispatched(ctx, hooks, func() diagnose.DiagnoseOutcome {
		return diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindSkippedSafetyGate, GateReason: &reason}
	})
}

// gateFailureResult builds the MCP tool error result for a NudgeGate/final
// identity-recheck failure. The error code IS the raw SafetyGateReason value
// (e.g. "nudge_execution_disabled") -- Story 4.1.1's AC requires the reason
// to be visibly named, never a silent no-op.
func gateFailureResult(reason diagnose.SafetyGateReason) *mcpgo.CallToolResult {
	return errResult(string(reason), reason.String(),
		"The nudge-safety gate blocked this write; see the error code for which safety check failed.")
}

// verifyBeforeWrite performs ADR-002's mandatory final identity re-check:
// call this as the literal last statement before the underlying
// SendKeys/SubmitContentWithEnter/RunWithResume/Resume call, with no
// intervening I/O -- Go cannot enforce that automatically (see
// tmux.verifyIdentityImmediatelyBeforeWrite's doc comment and
// validation.md's explicit caveat); it is enforced by code review, not a
// runtime assertion. expectedUUID must be the value captured once, at
// handler entry (inst.Snapshot().UUID right after resolving inst) -- NOT
// re-derived here -- so this check can actually catch the Instance drifting
// to a different session between resolution and write, which a
// freshly-re-derived expectation could never observe. Returns a ready-to-
// return MCP error result on failure, nil on success.
func verifyBeforeWrite(ctx context.Context, inst *session.Instance, expectedUUID string, hooks diagnoseOutcomeHooks) *mcpgo.CallToolResult {
	snap := inst.Snapshot()
	_, err := tmux.VerifyIdentityImmediatelyBeforeWrite(ctx, liveInstanceUUIDSnapshot{inst: inst}, snap.TmuxServerSocket, inst.GetTmuxSessionName(), expectedUUID)
	if err == nil {
		return nil
	}
	reason := identityMismatchReason(err)
	recordSkippedSafetyGateOutcome(ctx, hooks, reason)
	return gateFailureResult(reason)
}

// evaluateNudgeGate runs gate.Evaluate (the flag/idle/identity/cap pipeline)
// for inst and returns the session UUID that must be threaded into the
// matching verifyNudgeIdentity call(s) later in the same handler invocation
// -- callers must capture this return value once and reuse it, never
// re-derive it, so the final pre-write check can detect Instance drift (see
// verifyBeforeWrite's doc comment). A nil gate (see terminalHandlers/
// lifecycleHandlers.nudgeGate's doc comment) skips evaluation entirely,
// returning ("", nil).
func evaluateNudgeGate(ctx context.Context, gate nudgeGateEvaluator, inst *session.Instance, hooks diagnoseOutcomeHooks) (expectedUUID string, errRes *mcpgo.CallToolResult) {
	if gate == nil {
		return "", nil
	}
	expectedUUID = inst.Snapshot().UUID
	if ok, reason := gate.Evaluate(ctx, buildGateInput(inst)); !ok {
		recordSkippedSafetyGateOutcome(ctx, hooks, *reason)
		return "", gateFailureResult(*reason)
	}
	return expectedUUID, nil
}

// verifyNudgeIdentity is verifyBeforeWrite's nil-gate-safe wrapper -- callers
// use this (not verifyBeforeWrite directly) so a nil gate consistently skips
// both halves of the safety gate, matching evaluateNudgeGate's skip.
func verifyNudgeIdentity(ctx context.Context, gate nudgeGateEvaluator, inst *session.Instance, expectedUUID string, hooks diagnoseOutcomeHooks) *mcpgo.CallToolResult {
	if gate == nil {
		return nil
	}
	return verifyBeforeWrite(ctx, inst, expectedUUID, hooks)
}

// checkNudgeGate combines evaluateNudgeGate and verifyNudgeIdentity for a
// handler with a single write call site immediately following (writeToSession,
// resumeSession) -- steerSession has two write call sites gated by different
// branches, so it calls evaluateNudgeGate and verifyNudgeIdentity separately
// instead (see its own gate-insertion points).
func checkNudgeGate(ctx context.Context, gate nudgeGateEvaluator, inst *session.Instance, hooks diagnoseOutcomeHooks) *mcpgo.CallToolResult {
	expectedUUID, errRes := evaluateNudgeGate(ctx, gate, inst, hooks)
	if errRes != nil {
		return errRes
	}
	return verifyNudgeIdentity(ctx, gate, inst, expectedUUID, hooks)
}

// diagnoseDispatchWriteGuard is the narrow interface Story 4.1.4g's
// dispatch-level duplicate-write guard needs -- satisfied by
// services.DiagnoseDispatchStore. A seam so tests can inject a scripted
// result without a real ent-backed store; production wiring (NewCore,
// server.go) sets a real *services.entDiagnoseDispatchStore (via
// services.NewDiagnoseDispatchStore) when storage is non-nil.
type diagnoseDispatchWriteGuard interface {
	FindByDiagnosticSessionUUID(ctx context.Context, diagnosticSessionUUID string) (dispatchID string, found bool, err error)
	CheckAndSetWriteAttempted(ctx context.Context, dispatchID string) (alreadyAttempted bool, err error)
}

// checkDuplicateWriteGuard implements Task 4.1.4g: a SEPARATE, additive guard
// from NudgeGate.Evaluate's flag/idle/identity/cap pipeline -- it has no
// dispatchID in GateInput and does not consult or consume the nudge cap.
// Resolves the CALLING session's own UUID (the diagnostic agent's own
// session, read from context -- NOT inst, the target session being written
// to) to its DiagnoseDispatch row via DiagnosticSessionUUID, then rejects a
// second write attempt for that dispatch outright, regardless of remaining
// cap headroom. This is the structural backstop the adversarial re-review's
// residual CONCERNS finding asked for (adversarial-review.md's Concern 1
// "RESOLVED by Story 4.1.4" update): the nudge cap alone permits a second
// write within the same dispatch whenever cap > 1, but this guard does not.
//
// Skips entirely (returns nil, not an error) in three cases, all deliberate
// per Task 4.1.4g's carve-out: guard is nil (dispatch store unwired, e.g. the
// stdio fallback transport with storage == nil); the caller has no
// STAPLER_SESSION_UUID in context (e.g. a manual/external MCP client); or the
// caller's session UUID has no matching DiagnoseDispatch row at all (e.g.
// Tyler manually steering a session -- not a diagnose dispatch, nothing to
// guard). An unexpected store error fails closed, like every other
// safety-critical check in this file -- ambiguity about whether a duplicate
// write already happened must resolve to "don't write", not "assume it's
// fine".
func checkDuplicateWriteGuard(ctx context.Context, guard diagnoseDispatchWriteGuard, hooks diagnoseOutcomeHooks) *mcpgo.CallToolResult {
	if guard == nil {
		return nil
	}
	callerUUID, ok := sessionUUIDFromContext(ctx)
	if !ok {
		return nil
	}

	dispatchID, found, err := guard.FindByDiagnosticSessionUUID(ctx, callerUUID)
	if err != nil {
		log.Error("diagnose dispatch write guard: lookup failed, failing closed", "caller_session_uuid", callerUUID, "error", err)
		return gateFailureResult(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch)
	}
	if !found {
		return nil
	}

	alreadyAttempted, err := guard.CheckAndSetWriteAttempted(ctx, dispatchID)
	if err != nil {
		log.Error("diagnose dispatch write guard: check-and-set failed, failing closed", "dispatch_id", dispatchID, "error", err)
		return gateFailureResult(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch)
	}
	if alreadyAttempted {
		recordSkippedSafetyGateOutcome(ctx, hooks, diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch)
		return gateFailureResult(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch)
	}
	return nil
}
