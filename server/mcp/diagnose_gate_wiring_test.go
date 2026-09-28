package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// fakeDiagnoseOutcomeStore is a scripted diagnoseOutcomeStore for Story
// 5.2.2's outcome-notify tests -- returns a fixed record/found/err regardless
// of the UUID passed, and records every lookup's argument for assertion.
type fakeDiagnoseOutcomeStore struct {
	record services.DiagnoseDispatchRecord
	found  bool
	err    error
	calls  []string
}

func (f *fakeDiagnoseOutcomeStore) FindPendingByDiagnosticSessionUUID(_ context.Context, diagnosticSessionUUID string) (services.DiagnoseDispatchRecord, bool, error) {
	f.calls = append(f.calls, diagnosticSessionUUID)
	return f.record, f.found, f.err
}

// fakeDiagnoseOutcomeRecorderCall records one RecordDiagnoseOutcome call's
// arguments.
type fakeDiagnoseOutcomeRecorderCall struct {
	dispatchID, itemID string
	outcome            diagnose.DiagnoseOutcome
}

// fakeDiagnoseOutcomeRecorder is a scripted diagnoseOutcomeRecorder that
// records every RecordDiagnoseOutcome call for assertion, standing in for a
// real *services.DiagnoseDispatcher.
type fakeDiagnoseOutcomeRecorder struct {
	calls []fakeDiagnoseOutcomeRecorderCall
}

func (f *fakeDiagnoseOutcomeRecorder) RecordDiagnoseOutcome(_ context.Context, dispatchID, itemID string, outcome diagnose.DiagnoseOutcome) {
	f.calls = append(f.calls, fakeDiagnoseOutcomeRecorderCall{dispatchID: dispatchID, itemID: itemID, outcome: outcome})
}

// anySafeIdleStatusContext returns one of diagnose.SafeIdleStatusContexts'
// keys -- any of them, since IsSafeSteerStatus treats them equivalently --
// for tests that need a real "safe idle" StatusContext value.
func anySafeIdleStatusContext() string {
	for k := range diagnose.SafeIdleStatusContexts {
		return k
	}
	panic("diagnose.SafeIdleStatusContexts is empty")
}

// safeIdleGateInput builds a GateInput reporting a safe-idle Claude session
// at time now -- shared by the tests below that only vary sessionUUID/
// paneName/now, to avoid repeating the same five-field literal.
func safeIdleGateInput(sessionUUID, paneName string, now time.Time) diagnose.GateInput {
	return diagnose.GateInput{
		SessionUUID:   sessionUUID,
		Now:           now,
		Program:       "claude",
		Status:        detection.StatusIdle,
		StatusContext: anySafeIdleStatusContext(),
		PaneName:      paneName,
	}
}

func TestNewDiagnoseNudgeGate_ShouldReturnNudgeExecutionDisabled_WhenFlagOff(t *testing.T) {
	cfgFn := func() *config.Config { return &config.Config{} } // FeatureFlags nil -> flag defaults false
	gate := NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry())

	ok, reason := gate.Evaluate(context.Background(), diagnose.GateInput{SessionUUID: "s1"})

	require.False(t, ok)
	require.NotNil(t, reason)
	require.Equal(t, diagnose.SafetyGateReasonNudgeExecutionDisabled, *reason)
}

func TestNewDiagnoseNudgeGate_ShouldReturnNotIdle_WhenFlagOnButStatusNeverObservedIdle(t *testing.T) {
	cfgFn := func() *config.Config {
		return &config.Config{FeatureFlags: map[string]bool{config.DiagnoseNudgeExecutionFeatureFlag: true}}
	}
	gate := NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry())

	ok, reason := gate.Evaluate(context.Background(), diagnose.GateInput{
		SessionUUID: "s1",
		Now:         time.Now(),
		Program:     "claude",
		Status:      detection.StatusProcessing,
	})

	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonNotIdle, *reason)
}

func TestNewDiagnoseNudgeGate_ShouldPassAllFourChecks_WhenFlagOnIdleSustainedIdentityMatchesAndCapAvailable(t *testing.T) {
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=s1", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	storage := newTestBacklogStorage(t)
	cfgFn := func() *config.Config {
		return &config.Config{
			FeatureFlags:  map[string]bool{config.DiagnoseNudgeExecutionFeatureFlag: true},
			DiagnoseNudge: config.DiagnoseNudgeConfig{IdleSettleWindowSeconds: 1},
		}
	}
	gate := NewDiagnoseNudgeGate(cfgFn, storage, newIdleGateRegistry())

	base := time.Now()
	input := safeIdleGateInput("s1", "stapler-e6c2a88e-work", base)
	input.ItemID = "s1"

	// First observation starts the settle window -- IdleGate never passes on
	// its very first poll (see IdleGate.Evaluate's doc comment).
	ok, reason := gate.Evaluate(context.Background(), input)
	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonNotIdle, *reason)

	// Second observation, after the configured window elapses, passes idle,
	// identity (fake tmux marker matches), and cap (fresh item, under cap).
	input.Now = base.Add(2 * time.Second)
	ok, reason = gate.Evaluate(context.Background(), input)
	require.True(t, ok)
	require.Nil(t, reason)
}

func TestNewDiagnoseNudgeGate_ShouldReturnIdentityMismatchTmuxMarker_WhenIdlePassesButMarkerDiffers(t *testing.T) {
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	cfgFn := func() *config.Config {
		return &config.Config{
			FeatureFlags:  map[string]bool{config.DiagnoseNudgeExecutionFeatureFlag: true},
			DiagnoseNudge: config.DiagnoseNudgeConfig{IdleSettleWindowSeconds: 1},
		}
	}
	gate := NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry())

	base := time.Now()
	input := safeIdleGateInput("s1", "stapler-mismatch", base)
	gate.Evaluate(context.Background(), input) // starts the settle window

	input.Now = base.Add(2 * time.Second)
	ok, reason := gate.Evaluate(context.Background(), input)

	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker, *reason)
}

func TestNewDiagnoseNudgeGate_ShouldFailClosedOnCap_WhenStorageNil(t *testing.T) {
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=s1", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	cfgFn := func() *config.Config {
		return &config.Config{
			FeatureFlags:  map[string]bool{config.DiagnoseNudgeExecutionFeatureFlag: true},
			DiagnoseNudge: config.DiagnoseNudgeConfig{IdleSettleWindowSeconds: 1},
		}
	}
	gate := NewDiagnoseNudgeGate(cfgFn, nil /* storage nil */, newIdleGateRegistry())

	base := time.Now()
	input := safeIdleGateInput("s1", "stapler-cap-nil-storage", base)
	gate.Evaluate(context.Background(), input) // starts the settle window

	input.Now = base.Add(2 * time.Second)
	ok, reason := gate.Evaluate(context.Background(), input)

	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonNudgeCapReached, *reason)
}

func TestIdleGateRegistry_Get_ShouldReturnSameInstance_ForRepeatedCallsWithSameSessionUUID(t *testing.T) {
	r := newIdleGateRegistry()

	g1 := r.get("s1", 60)
	g2 := r.get("s1", 60)
	g3 := r.get("s2", 60)

	require.Same(t, g1, g2)
	require.NotSame(t, g1, g3)
}

func TestEvaluateIdentity_ShouldReturnTrue_WhenTmuxMarkerMatchesSessionUUID(t *testing.T) {
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=abc", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	ok, reason := evaluateIdentity(context.Background(), diagnose.GateInput{SessionUUID: "abc", PaneName: "p1"})

	require.True(t, ok)
	require.Empty(t, reason)
}

func TestEvaluateIdentity_ShouldReturnTmuxMarkerMismatch_WhenMarkerDiffersFromSessionUUID(t *testing.T) {
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=different", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	ok, reason := evaluateIdentity(context.Background(), diagnose.GateInput{SessionUUID: "abc", PaneName: "p1"})

	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker, reason)
}

func TestNewNudgeCapCheck_ShouldFailClosed_WhenStorageIsNil(t *testing.T) {
	check := newNudgeCapCheck(nil, func() *config.Config { return &config.Config{} })

	ok, reason := check(context.Background(), diagnose.GateInput{ItemID: "item1"})

	require.False(t, ok)
	require.Equal(t, diagnose.SafetyGateReasonNudgeCapReached, reason)
}

func TestNewNudgeCapCheck_ShouldEnforceCap_WhenStorageIsReal(t *testing.T) {
	storage := newTestBacklogStorage(t)
	cfgFn := func() *config.Config {
		return &config.Config{DiagnoseNudge: config.DiagnoseNudgeConfig{MaxNudgesPerItem: 1}}
	}
	check := newNudgeCapCheck(storage, cfgFn)

	ok1, _ := check(context.Background(), diagnose.GateInput{ItemID: "item1"})
	ok2, reason2 := check(context.Background(), diagnose.GateInput{ItemID: "item1"})

	require.True(t, ok1, "first nudge for a fresh item must be allowed")
	require.False(t, ok2, "second nudge must be blocked by the cap of 1")
	require.Equal(t, diagnose.SafetyGateReasonNudgeCapReached, reason2)
}

func TestBuildGateInput_ShouldReflectInstanceSnapshotFields(t *testing.T) {
	inst := newGatedTestInstance(t, "gate-input-test", "claude")

	input := buildGateInput(inst)

	snap := inst.Snapshot()
	require.Equal(t, snap.UUID, input.SessionUUID)
	require.Equal(t, snap.UUID, input.ItemID)
	require.Equal(t, "claude", input.Program)
	require.NotEmpty(t, input.PaneName)
	require.WithinDuration(t, time.Now(), input.Now, 5*time.Second)
}

func TestGateFailureResult_ShouldContainRawSafetyGateReasonString(t *testing.T) {
	res := gateFailureResult(diagnose.SafetyGateReasonNudgeExecutionDisabled)

	m := parseResult(t, res)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	require.Equal(t, "nudge_execution_disabled", errObj["code"])
}

// --- Story 5.2.2: outcome-notify wiring for create_backlog_item/
// post_backlog_update/a successful nudge write/a safety-gate rejection ---

// TestRecordDiagnoseOutcomeIfDispatched_ShouldRecordOutcome_WhenCallerHasPendingDispatchRow
// is the "successful-nudge-from-diagnostic-session -> Nudged outcome
// recorded" case: recordDiagnoseOutcomeIfDispatched is the exact function
// writeToSession/steerSession/resumeSession call right after a successful
// write, and create_backlog_item/post_backlog_update call after their own
// storage writes -- this exercises it directly since standing up a real PTY
// write to reach writeToSession/steerSession's own success branch isn't
// possible in this package's test environment (see e.g.
// TestSteerSession_ShouldReachSendKeys_WhenGateAndFinalIdentityRecheckBothPass's
// own "a real PTY would make this the true happy path" early return).
func TestRecordDiagnoseOutcomeIfDispatched_ShouldRecordOutcome_WhenCallerHasPendingDispatchRow(t *testing.T) {
	store := &fakeDiagnoseOutcomeStore{found: true, record: services.DiagnoseDispatchRecord{ID: "dispatch-1", ItemID: "item-1"}}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-1-abc")

	recordDiagnoseOutcomeIfDispatched(ctx, hooks, func() diagnose.DiagnoseOutcome {
		return diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}
	})

	require.Equal(t, []string{"headless-diagnose-item-1-abc"}, store.calls, "must resolve the CALLING session's own UUID, not any target session")
	require.Len(t, recorder.calls, 1)
	require.Equal(t, "dispatch-1", recorder.calls[0].dispatchID)
	require.Equal(t, "item-1", recorder.calls[0].itemID)
	require.Equal(t, diagnose.DiagnoseOutcomeKindNudged, recorder.calls[0].outcome.Kind)
}

// TestRecordDiagnoseOutcomeIfDispatched_ShouldNotRecord_WhenCallerHasNoDispatchRow
// is the "a Tyler-manual call (no DiagnoseDispatch row) -> completely
// unaffected, no notification fired" case.
func TestRecordDiagnoseOutcomeIfDispatched_ShouldNotRecord_WhenCallerHasNoDispatchRow(t *testing.T) {
	store := &fakeDiagnoseOutcomeStore{found: false}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "tylers-manual-session-uuid")

	buildCalled := false
	recordDiagnoseOutcomeIfDispatched(ctx, hooks, func() diagnose.DiagnoseOutcome {
		buildCalled = true
		return diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}
	})

	require.Empty(t, recorder.calls, "a caller with no matching Pending DiagnoseDispatch row must never be notified")
	require.False(t, buildCalled, "buildOutcome must only run once a matching row is actually found")
}

// TestRecordDiagnoseOutcomeIfDispatched_ShouldNoOp_WhenNoSessionUUIDInContext
// covers a manual/external MCP client with no STAPLER_SESSION_UUID set at
// all (as opposed to the above, which has a UUID but no matching row).
func TestRecordDiagnoseOutcomeIfDispatched_ShouldNoOp_WhenNoSessionUUIDInContext(t *testing.T) {
	store := &fakeDiagnoseOutcomeStore{found: true, record: services.DiagnoseDispatchRecord{ID: "dispatch-1", ItemID: "item-1"}}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}

	recordDiagnoseOutcomeIfDispatched(context.Background(), hooks, func() diagnose.DiagnoseOutcome {
		return diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}
	})

	require.Empty(t, store.calls, "must not even attempt a lookup with no caller session UUID")
	require.Empty(t, recorder.calls)
}

// TestRecordDiagnoseOutcomeIfDispatched_ShouldNoOp_WhenHooksZeroValue covers
// an unwired server configuration (e.g. the stdio fallback transport with
// storage == nil), matching nudgeGate/dispatchWriteGuard's own nil-skips
// convention.
func TestRecordDiagnoseOutcomeIfDispatched_ShouldNoOp_WhenHooksZeroValue(t *testing.T) {
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-1-abc")

	require.NotPanics(t, func() {
		recordDiagnoseOutcomeIfDispatched(ctx, diagnoseOutcomeHooks{}, func() diagnose.DiagnoseOutcome {
			t.Fatal("buildOutcome must never run when hooks is the zero value")
			return diagnose.DiagnoseOutcome{}
		})
	})
}

// TestRecordDiagnoseOutcomeIfDispatched_ShouldFailOpen_WhenStoreLookupErrors
// covers the deliberate fail-OPEN choice this helper makes (unlike
// checkDuplicateWriteGuard's fail-closed behavior): a storage hiccup here
// must skip the notification, not block the write/tool-call outcome that
// already happened.
func TestRecordDiagnoseOutcomeIfDispatched_ShouldFailOpen_WhenStoreLookupErrors(t *testing.T) {
	store := &fakeDiagnoseOutcomeStore{err: errors.New("db unavailable")}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-1-abc")

	require.NotPanics(t, func() {
		recordDiagnoseOutcomeIfDispatched(ctx, hooks, func() diagnose.DiagnoseOutcome {
			return diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}
		})
	})
	require.Empty(t, recorder.calls, "a lookup error must fail open (skip notification), not propagate or panic")
}

// TestRecordSkippedSafetyGateOutcome_ShouldRecordWithGateReason covers gap
// item 4: a safety-gate rejection of a diagnostic session's write is itself
// an outcome worth recording.
func TestRecordSkippedSafetyGateOutcome_ShouldRecordWithGateReason(t *testing.T) {
	store := &fakeDiagnoseOutcomeStore{found: true, record: services.DiagnoseDispatchRecord{ID: "dispatch-2", ItemID: "item-2"}}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-2-abc")

	recordSkippedSafetyGateOutcome(ctx, hooks, diagnose.SafetyGateReasonNotIdle)

	require.Len(t, recorder.calls, 1)
	require.Equal(t, diagnose.DiagnoseOutcomeKindSkippedSafetyGate, recorder.calls[0].outcome.Kind)
	require.NotNil(t, recorder.calls[0].outcome.GateReason)
	require.Equal(t, diagnose.SafetyGateReasonNotIdle, *recorder.calls[0].outcome.GateReason)
}

// TestEvaluateNudgeGate_ShouldRecordSkippedSafetyGateOutcome_WhenGateRejectsDiagnosticSessionWrite
// covers the real call site: evaluateNudgeGate's own rejection branch must
// invoke the outcome hooks, not just return the error result.
func TestEvaluateNudgeGate_ShouldRecordSkippedSafetyGateOutcome_WhenGateRejectsDiagnosticSessionWrite(t *testing.T) {
	inst := newGatedTestInstance(t, "gate-rejects-session", "claude")
	store := &fakeDiagnoseOutcomeStore{found: true, record: services.DiagnoseDispatchRecord{ID: "dispatch-3", ItemID: "item-3"}}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-3-abc")

	gate := fakeNudgeGateEvaluator{ok: false, reason: diagnose.SafetyGateReasonNotIdle}
	_, errRes := evaluateNudgeGate(ctx, gate, inst, hooks)

	require.NotNil(t, errRes)
	require.Len(t, recorder.calls, 1)
	require.Equal(t, diagnose.DiagnoseOutcomeKindSkippedSafetyGate, recorder.calls[0].outcome.Kind)
	require.Equal(t, diagnose.SafetyGateReasonNotIdle, *recorder.calls[0].outcome.GateReason)
}

// TestEvaluateNudgeGate_ShouldNotRecordOutcome_WhenGatePasses guards against
// over-eager recording: a passing gate must never call the recorder.
func TestEvaluateNudgeGate_ShouldNotRecordOutcome_WhenGatePasses(t *testing.T) {
	inst := newGatedTestInstance(t, "gate-passes-session", "claude")
	store := &fakeDiagnoseOutcomeStore{found: true, record: services.DiagnoseDispatchRecord{ID: "dispatch-4", ItemID: "item-4"}}
	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: store, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), "headless-diagnose-item-4-abc")

	gate := fakeNudgeGateEvaluator{ok: true}
	_, errRes := evaluateNudgeGate(ctx, gate, inst, hooks)

	require.Nil(t, errRes)
	require.Empty(t, recorder.calls)
}

// TestCheckDuplicateWriteGuard_ShouldRecordSkippedSafetyGateOutcome_WhenSecondWriteRejected
// covers the OTHER real call site: checkDuplicateWriteGuard's own rejection
// (a second write attempt for the same dispatch) must also record the
// outcome, and the task's "don't double-notify" concern resolves structurally
// via FindPendingByDiagnosticSessionUUID's Pending-only filter (a completed
// dispatch's row is simply never found again) -- covered at the store layer
// by TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnNotFound_WhenRowAlreadyCompleted.
func TestCheckDuplicateWriteGuard_ShouldRecordSkippedSafetyGateOutcome_WhenSecondWriteRejected(t *testing.T) {
	inst := newGatedTestInstance(t, "dup-guard-outcome-session", "claude")
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-outcome-test"
	storage := newTestBacklogStorage(t)
	capStore := services.NewNudgeCapStore(storage)
	dispatchStore := services.NewDiagnoseDispatchStore(storage)

	ok, err := capStore.CheckAndReserve(context.Background(), itemID, 2, 0)
	require.NoError(t, err)
	require.True(t, ok)
	dispatchID, err := dispatchStore.Record(context.Background(), services.DiagnoseDispatchRequest{
		ItemID: itemID, TargetSessionUUID: itemID, DiagnosticSessionUUID: callerUUID,
	})
	require.NoError(t, err)

	recorder := &fakeDiagnoseOutcomeRecorder{}
	hooks := diagnoseOutcomeHooks{store: dispatchStore, recorder: recorder}
	ctx := WithSessionUUID(context.Background(), callerUUID)

	// First call reserves WriteAttemptedAt and passes (no rejection).
	require.Nil(t, checkDuplicateWriteGuard(ctx, dispatchStore, hooks))
	require.Empty(t, recorder.calls, "the first (accepted) write attempt must not itself be recorded as a gate skip")

	// Second call for the same dispatch is rejected -- and must be recorded.
	errRes := checkDuplicateWriteGuard(ctx, dispatchStore, hooks)
	require.NotNil(t, errRes)
	require.Len(t, recorder.calls, 1)
	require.Equal(t, dispatchID, recorder.calls[0].dispatchID)
	require.Equal(t, diagnose.DiagnoseOutcomeKindSkippedSafetyGate, recorder.calls[0].outcome.Kind)
	require.Equal(t, diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch, *recorder.calls[0].outcome.GateReason)
}
