package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// --- Epic 4.1 / Story 4.1.3: NudgeGate + IsArchived() insertion in resumeSession ---

// allSafetyGateReasonCodes lists every SafetyGateReason string -- used by the
// "all-pass" tests below to assert a result is NOT a gate-failure code,
// without needing to know exactly which non-gate error (if any) a given test
// double produces.
var allSafetyGateReasonCodes = []string{
	string(diagnose.SafetyGateReasonNotIdle),
	string(diagnose.SafetyGateReasonIdentityMismatchInstance),
	string(diagnose.SafetyGateReasonIdentityMismatchTmuxMarker),
	string(diagnose.SafetyGateReasonNudgeCapReached),
	string(diagnose.SafetyGateReasonNudgeCooldownActive),
	string(diagnose.SafetyGateReasonNudgeExecutionDisabled),
}

// TestResumeSession_ShouldAbortWithIdentityMismatchInstance_WhenSessionIsArchived
// is Story 4.1.3's named AC: an archived session is never resumed, and the
// check runs before any worktree/tmux state is recreated -- proven here by a
// nudgeGate double that would panic-via-unexpected-call assertions if it were
// ever consulted (it isn't; the IsArchived() check runs first and returns
// before the gate is reached).
func TestResumeSession_ShouldAbortWithIdentityMismatchInstance_WhenSessionIsArchived(t *testing.T) {
	inst := newGatedTestInstance(t, "archived-resume-session", "claude")
	inst.Status = session.Paused
	archivedAt := time.Now()
	inst.SetArchivedAt(&archivedAt)

	lh := newWorktreeGuardHandlers(t, inst)
	lh.nudgeGate = fakeNudgeGateEvaluator{ok: true}

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "an archived session must never be resumed")
	errObj, _ := m["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, string(diagnose.SafetyGateReasonIdentityMismatchInstance), errObj["code"])
}

// TestResumeSession_ShouldAbortWithSafetyGateReason_WhenNudgeExecutionFlagDisabled
// wires resumeSession to a REAL *diagnose.NudgeGate (mirroring REQ-5's
// steerSession test) to prove the flag gate is actually reachable for a
// non-archived, correctly-Paused session -- not just plumbed for a fake.
func TestResumeSession_ShouldAbortWithSafetyGateReason_WhenNudgeExecutionFlagDisabled(t *testing.T) {
	inst := newGatedTestInstance(t, "flag-off-resume-session", "claude")
	inst.Status = session.Paused

	lh := newWorktreeGuardHandlers(t, inst)
	cfgFn := func() *config.Config { return &config.Config{} } // flag defaults false
	lh.nudgeGate = NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry())

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.Equal(t, string(diagnose.SafetyGateReasonNudgeExecutionDisabled), errObj["code"])
}

// TestResumeSession_ShouldProceedPastGate_WhenNotArchivedAndGateAndIdentityBothPass
// is the "all-pass" scenario: not archived, Paused, gate passes (scripted
// true), and the real final identity re-check's fake tmux backend reports a
// matching marker -- the handler must proceed past the gate to the real
// inst.Resume() call. Whatever inst.Resume() itself does in this
// no-real-tmux-server unit test environment is not asserted; the point is
// that no SafetyGateReason code appears, proving Resume() was reached.
func TestResumeSession_ShouldProceedPastGate_WhenNotArchivedAndGateAndIdentityBothPass(t *testing.T) {
	inst := newGatedTestInstance(t, "resume-all-pass-session", "claude")
	inst.Status = session.Paused
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	lh := newWorktreeGuardHandlers(t, inst)
	lh.nudgeGate = fakeNudgeGateEvaluator{ok: true}

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		return // real environments where Resume() succeeds are the true happy path
	}
	errObj, _ := m["error"].(map[string]interface{})
	code, _ := errObj["code"].(string)
	require.NotContains(t, allSafetyGateReasonCodes, code,
		"resumeSession returned a gate-failure code after an all-pass gate; expected inst.Resume() to be reached")
}

// TestResumeSession_ShouldAbortWrite_WhenFinalIdentityRecheckFindsTmuxMarkerMismatch
// exercises the REAL tmux.VerifyIdentityImmediatelyBeforeWrite facade for
// resumeSession's final pre-write re-check, mirroring the steerSession/
// write_to_session coverage in tools_terminal_test.go.
func TestResumeSession_ShouldAbortWrite_WhenFinalIdentityRecheckFindsTmuxMarkerMismatch(t *testing.T) {
	inst := newGatedTestInstance(t, "resume-identity-mismatch-session", "claude")
	inst.Status = session.Paused
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	lh := newWorktreeGuardHandlers(t, inst)
	lh.nudgeGate = fakeNudgeGateEvaluator{ok: true}

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.Equal(t, string(diagnose.SafetyGateReasonIdentityMismatchTmuxMarker), errObj["code"])
}

// --- Story 4.1.4: ambiguous write-outcome classification + dispatch guard for resumeSession ---

// newResumableStructLiteralInstance builds a *session.Instance via a struct
// literal (not session.NewInstance/newGatedTestInstance) whose Snapshot()
// genuinely reports Paused: Snapshot() lazily builds its cached copy from the
// struct's CURRENT fields on first call (session/instance.go's Snapshot()),
// so a struct literal's fields are picked up correctly, whereas
// newGatedTestInstance's post-construction `inst.Status = Paused` mutation
// would not be (session.NewInstance already published a snapshot at
// construction time, before that assignment, per
// .claude/rules/instance-lock-free-reads.md). Setting no TmuxSession leaves
// pm() to lazily construct a real default TmuxBackend, which resolves the
// tmux binary via TMUX_BIN like production code -- letting this test drive a
// genuine "no such file or directory" exec failure out of inst.Resume(),
// rather than only a generic "can only resume paused instances" error.
func newResumableStructLiteralInstance(t *testing.T, title string) *session.Instance {
	t.Helper()
	return &session.Instance{
		Title:       title,
		UUID:        uuid.New().String(),
		Status:      session.Paused,
		Permissions: session.GetManagedPermissions(),
		Path:        t.TempDir(),
	}
}

// TestResumeSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingResumeFailsWithConnectionShapedError
// covers Task 4.1.4a/4.1.4d for resumeSession's underlying inst.Resume() call:
// pointing TMUX_BIN at a nonexistent binary makes the tmux exec fail with a
// "no such file or directory" error, which must classify as
// write_outcome_unknown. lh.nudgeGate/dispatchWriteGuard are left nil (skip)
// so this test isolates the post-gate write-outcome classification.
func TestResumeSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingResumeFailsWithConnectionShapedError(t *testing.T) {
	inst := newResumableStructLiteralInstance(t, "resume-connection-error-session")
	t.Setenv("TMUX_BIN", filepath.Join(t.TempDir(), "nonexistent-tmux-binary"))

	lh := newWorktreeGuardHandlers(t, inst)

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, writeOutcomeUnknownMarker, errObj["code"])
}

// TestResumeSession_ShouldLeaveCapSlotConsumed_NotRefunded_WhenUnderlyingResumeReturnsWriteOutcomeUnknown
// covers Task 4.1.4d's cap-non-refund AC for resumeSession -- same scope note
// as TestSteerSession_ShouldLeaveCapSlotConsumed_NotRefunded_...: this proves
// no code path in resumeSession touches NudgeCapStore based on the write's
// outcome.
func TestResumeSession_ShouldLeaveCapSlotConsumed_NotRefunded_WhenUnderlyingResumeReturnsWriteOutcomeUnknown(t *testing.T) {
	inst := newResumableStructLiteralInstance(t, "resume-cap-not-refunded-session")
	t.Setenv("TMUX_BIN", filepath.Join(t.TempDir(), "nonexistent-tmux-binary"))

	lh := newWorktreeGuardHandlers(t, inst)
	storage := newTestBacklogStorage(t)
	capStore := services.NewNudgeCapStore(storage)
	itemID := inst.Snapshot().UUID
	ok, err := capStore.CheckAndReserve(context.Background(), itemID, 2, 0)
	require.NoError(t, err)
	require.True(t, ok)

	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})
	result, err := lh.resumeSession(context.Background(), req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.Equal(t, writeOutcomeUnknownMarker, errObj["code"])

	rec, err := capStore.Get(context.Background(), itemID)
	require.NoError(t, err)
	require.Equal(t, 1, rec.NudgeCount, "a write_outcome_unknown result must not refund the already-reserved cap slot")
}

// newResumeDispatchGuardTestSetup mirrors
// newDispatchGuardTestSetup (tools_terminal_test.go) for resumeSession's
// lifecycleHandlers.
func newResumeDispatchGuardTestSetup(t *testing.T, inst *session.Instance) (*lifecycleHandlers, services.NudgeCapStore, services.DiagnoseDispatchStore) {
	t.Helper()
	lh := newWorktreeGuardHandlers(t, inst)
	storage := newTestBacklogStorage(t)
	dispatchStore := services.NewDiagnoseDispatchStore(storage)
	lh.nudgeGate = fakeNudgeGateEvaluator{ok: true}
	lh.dispatchWriteGuard = dispatchStore
	return lh, services.NewNudgeCapStore(storage), dispatchStore
}

// TestResumeSession_ShouldRejectSecondWriteAttempt_WhenDispatchAlreadyRecordedAWriteAttempt_EvenWithCapHeadroomRemaining
// mirrors the steerSession version (tools_terminal_test.go) for resumeSession.
// The fake tmux backend only answers show-environment (matching the
// identity re-check) and fails every other command -- so inst.Resume() itself
// fails generically on both calls, which keeps inst.Status Paused between
// them (Resume() only transitions to Active after its tmux calls succeed),
// letting the second call reach the guard instead of being rejected earlier
// by resumeSession's own "must be Paused" precondition check.
func TestResumeSession_ShouldRejectSecondWriteAttempt_WhenDispatchAlreadyRecordedAWriteAttempt_EvenWithCapHeadroomRemaining(t *testing.T) {
	inst := newGatedTestInstance(t, "resume-dispatch-guard-session", "claude")
	inst.Status = session.Paused
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)
	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-resume-guard-test"

	lh, capStore, dispatchStore := newResumeDispatchGuardTestSetup(t, inst)
	seedDispatchGuardPrecondition(t, capStore, dispatchStore, itemID, callerUUID)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})

	first, err := lh.resumeSession(ctx, req)
	require.NoError(t, err)
	if errObj, _ := parseResult(t, first)["error"].(map[string]interface{}); errObj != nil {
		require.NotEqual(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), errObj["code"])
	}
	require.Equal(t, session.Paused, inst.Status, "test precondition: Resume() must not have succeeded, so the second call still finds a Paused session")

	second, err := lh.resumeSession(ctx, req)
	require.NoError(t, err)
	secondResult := parseResult(t, second)
	require.False(t, secondResult["success"].(bool))
	errObj, _ := secondResult["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), errObj["code"])

	rec, err := capStore.Get(context.Background(), itemID)
	require.NoError(t, err)
	require.Equal(t, 1, rec.NudgeCount, "the dispatch-level guard's rejection must not consult or consume the nudge cap")
}

// TestResumeSession_ShouldNotApplyDuplicateWriteGuard_WhenCallerHasNoMatchingDiagnoseDispatchRow
// mirrors the steerSession version for resumeSession.
func TestResumeSession_ShouldNotApplyDuplicateWriteGuard_WhenCallerHasNoMatchingDiagnoseDispatchRow(t *testing.T) {
	inst := newGatedTestInstance(t, "resume-no-dispatch-row-session", "claude")
	inst.Status = session.Paused
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	lh, _, _ := newResumeDispatchGuardTestSetup(t, inst) // no dispatch rows recorded

	ctx := WithSessionUUID(context.Background(), "manual-caller-with-no-dispatch-row")
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})

	result, err := lh.resumeSession(ctx, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		return
	}
	errObj, _ := m["error"].(map[string]interface{})
	code, _ := errObj["code"].(string)
	require.NotEqual(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), code,
		"the guard must be skipped entirely when the caller has no matching DiagnoseDispatch row")
}

// TestResumeSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck
// mirrors TestSteerSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck
// (tools_terminal_test.go) for resume_session: checkDuplicateWriteGuard must
// run BEFORE verifyNudgeIdentity's final pre-write identity re-check. The
// FIRST call uses a matching tmux identity so it passes both checks and marks
// the dispatch's write attempted (inst.Resume() itself still fails generically
// here -- no real tmux server -- which keeps Status Paused for the second
// call, same precondition TestResumeSession_ShouldRejectSecondWriteAttempt...
// relies on). The SECOND call switches to a MISMATCHED tmux identity: if the
// guard genuinely runs first, it rejects before the identity re-check is ever
// reached. If the buggy ordering ever regresses, this test would instead
// observe an identity-mismatch code.
func TestResumeSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck(t *testing.T) {
	inst := newGatedTestInstance(t, "resume-guard-before-identity-session", "claude")
	inst.Status = session.Paused

	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-resume-order-test"
	lh, capStore, dispatchStore := newResumeDispatchGuardTestSetup(t, inst)
	seedDispatchGuardPrecondition(t, capStore, dispatchStore, itemID, callerUUID)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title})

	matchingTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", matchingTmux)
	_, err := lh.resumeSession(ctx, req)
	require.NoError(t, err)
	require.Equal(t, session.Paused, inst.Status, "test precondition: Resume() must not have succeeded, so the second call still finds a Paused session")

	mismatchedTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", mismatchedTmux)
	result, err := lh.resumeSession(ctx, req)
	require.NoError(t, err)
	assertDuplicateWriteGuardRejected(t, result)
}
