package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// guardsEnv is a steerEnv whose service has the guards flag wired to the flag
// service with its audit policy, as newGatedSessionService does.
func newGuardsEnv(t *testing.T) *steerEnv {
	t.Helper()
	e := newSteerEnv(t)
	e.fix.svc.featureFlagSvc.SetAudit(e.sink, map[string]FlagAuditPolicy{
		hiddenSessionReadonlyGuardsFlagName: guardsFlagAuditPolicy,
	})
	e.fix.svc.wireGuardsFlag()
	return e
}

func (e *steerEnv) setGuards(on bool) {
	e.t.Helper()
	require.NoError(e.t, flagUpdateCtx(context.Background(), e.fix.svc.featureFlagSvc, hiddenSessionReadonlyGuardsFlagName, on))
}

func TestGuardsFlag_ShouldBeRegisteredDefaultOnGlobalOnlyAndApplyEveryFlipAtOnce(t *testing.T) {
	e := newGuardsEnv(t)
	require.True(t, featureFlagDefault(hiddenSessionReadonlyGuardsFlagName), "default on")
	for _, kf := range knownFeatureFlags {
		if kf.name == hiddenSessionReadonlyGuardsFlagName {
			assert.Empty(t, kf.scopes, "global only")
		}
	}
	assert.True(t, e.fix.svc.guards.GuardsEnabled())

	e.setGuards(false)
	assert.False(t, e.fix.svc.guards.GuardsEnabled(), "off applies at once, no reload")
	e.setGuards(true)
	assert.True(t, e.fix.svc.guards.GuardsEnabled())
}

func TestGuardsFlag_ShouldStartOffOnlyForAnExplicitPersistedFalse_AndKeepGuardsOnWhenConfigHasNoKey(t *testing.T) {
	e := newGuardsEnv(t)
	require.True(t, e.fix.svc.guards.GuardsEnabled(), "no persisted key: on")

	require.NoError(t, config.LoadConfig().SetFeatureFlag(hiddenSessionReadonlyGuardsFlagName, false))
	e.fix.svc.wireGuardsFlag()
	assert.False(t, e.fix.svc.guards.GuardsEnabled())

	require.NoError(t, config.LoadConfig().DeleteFeatureFlag(hiddenSessionReadonlyGuardsFlagName))
	e.fix.svc.wireGuardsFlag()
	assert.True(t, e.fix.svc.guards.GuardsEnabled(), "deleting the key returns to on")
}

// T-RO-24
func TestUnaryGuards_ShouldAcceptHiddenTargetWhenReadonlyGuardsFlagOffAndRejectWhenOn_WhileStreamDropsAndReplyStayUnaffected(t *testing.T) {
	e := newGuardsEnv(t)
	hidden := e.addSession("flag-hidden", true, "")
	ts := newLeaseTerminalService(t, hidden.inst)
	ts.SetGuardsFlag(e.fix.svc.guards)

	_, err := ts.WriteToSession(context.Background(), writeToSessionReq("flag-hidden"))
	requireReadOnlyRefusal(t, err, "flag on")

	e.setGuards(false)
	_, err = ts.WriteToSession(context.Background(), writeToSessionReq("flag-hidden"))
	require.NoError(t, err, "flag off: accepted as on main")
	assert.Equal(t, []string{"ls", session.EnterKeySequence}, hidden.pm.writes())
	assert.Equal(t, TerminalReadOnly, AccessFor(hidden.inst), "the stream drops are not behind the flag")

	e.setGuards(true)
	_, err = ts.WriteToSession(context.Background(), writeToSessionReq("flag-hidden"))
	requireReadOnlyRefusal(t, err, "flag back on")
}

func TestUnaryGuards_ShouldAcceptProgramAndAutoApproveChangesOnlyWhileTheFlagIsOff(t *testing.T) {
	e := newGuardsEnv(t)
	paused := e.addSession("flag-restart", true, "", func(i *session.Instance) { i.Status = session.Paused })
	yes := true
	req := func() *connect.Request[sessionv1.UpdateSessionRequest] {
		return connect.NewRequest(&sessionv1.UpdateSessionRequest{Id: "flag-restart", AutoApprove: &yes})
	}
	_, err := e.fix.svc.UpdateSession(context.Background(), req())
	requireReadOnlyRefusal(t, err, "on")
	assert.False(t, paused.inst.AutoApprove)

	e.setGuards(false)
	_, err = e.fix.svc.UpdateSession(context.Background(), req())
	if err != nil {
		assert.NotContains(t, err.Error(), "read-only", "off: the guard is out of the way")
	}
}

// T-RO-31
func TestUnaryGuards_ShouldKeepTypedPathAndAuditInBothFlagStates_AndAuditGuardBypassWithRateLimitedWarn_WhenFlagOffAndNonQualifyingHiddenSteer(t *testing.T) {
	e := newGuardsEnv(t)
	review := e.addSession("both-review", true, session.SessionRoleReview)
	diag := e.addSession("both-diagnose", true, session.SessionRoleDiagnose)

	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, "on"))
	e.setGuards(false)
	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, "off"), "the qualifying steer takes the typed path in both states")
	assert.Len(t, review.pm.writes(), 4)

	// A non-qualifying hidden target is allowed while off, audited as guard_bypass.
	require.NoError(t, e.steer(e.localCtx(), diag.inst.Title, "bypass"))
	assert.Equal(t, []string{"bypass", session.EnterKeySequence}, diag.pm.writes())
	assert.EqualValues(t, 1, e.count(steerOutcomeGuardBypass))
	assert.Equal(t, uint64(1), e.fix.svc.guardBypass.count.Load())

	lines := e.auditLines()
	assert.Len(t, steerLines(lines, auditKindBacklogSteer), 4, "two typed steers, a request and a result line each")
	bypass := steerLines(lines, auditKindGuardBypass)
	require.Len(t, bypass, 2)
	assert.Equal(t, auditPhaseRequest, bypass[0].Phase)
	assert.Equal(t, auditPhaseResult, bypass[1].Phase)
	assert.Equal(t, steerOutcomeSent, bypass[1].Outcome)
	assert.Equal(t, diag.inst.UUID, bypass[0].SessionUUID)
}

func TestGuardBypass_ShouldRefuseWithZeroWrites_WhenTheAuditSinkIsDown_AndStillSteerAQualifyingReviewOnTheFallbackRecord(t *testing.T) {
	e := newGuardsEnv(t)
	review := e.addSession("down-review", true, session.SessionRoleReview)
	diag := e.addSession("down-diagnose", true, session.SessionRoleDiagnose)
	e.setGuards(false)
	e.mfs.failWrite = errDiskFull

	err := e.steer(e.localCtx(), diag.inst.Title, "bypass")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Empty(t, diag.pm.writes(), "a bypass write is never allowed on a log line alone")

	require.NoError(t, e.steer(e.localCtx(), review.inst.Title, "review"), "guards off and sink down: the typed path goes through")
	assert.Len(t, review.pm.writes(), 2)
	assert.GreaterOrEqual(t, e.warn("steer_audit_degraded"), 1, "the degraded WARN record stands in for the audit line")
}

func TestGuardBypass_ShouldRateLimitTheWarnToOncePerMinuteAndCountEveryBypass(t *testing.T) {
	var g guardBypassState
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }

	assert.True(t, g.note(), "first bypass warns")
	now = now.Add(30 * time.Second)
	assert.False(t, g.note(), "within the minute: counted, not warned")
	now = now.Add(31 * time.Second)
	assert.True(t, g.note(), "a minute later warns again")
	assert.Equal(t, uint64(3), g.count.Load())
}

func TestGuardsFlag_ShouldShowOffStatusLineWithBypassCountInSettings(t *testing.T) {
	e := newGuardsEnv(t)
	assert.Empty(t, statusDetailOf(t, e.fix.svc.featureFlagSvc, hiddenSessionReadonlyGuardsFlagName))

	e.setGuards(false)
	assert.Equal(t, "Hidden-session write guards are OFF: 0 writes bypassed",
		statusDetailOf(t, e.fix.svc.featureFlagSvc, hiddenSessionReadonlyGuardsFlagName))

	diag := e.addSession("status-diagnose", true, session.SessionRoleDiagnose)
	require.NoError(t, e.steer(e.localCtx(), diag.inst.Title, "x"))
	assert.Equal(t, "Hidden-session write guards are OFF: 1 writes bypassed",
		statusDetailOf(t, e.fix.svc.featureFlagSvc, hiddenSessionReadonlyGuardsFlagName))
}

// Every flip is a flag_change line with the true previous value; the off flip
// is audited durably before it persists and is never refused for a sink fault.
func TestGuardsFlag_ShouldRecordFlagChangeLinesAndPersistOffWithOnlyTheFallbackRecord_WhenTheAuditSinkIsDown(t *testing.T) {
	e := newGuardsEnv(t)
	e.setGuards(false)
	e.setGuards(true)
	e.sink.Close() // drains the queued result lines

	var requested, results int
	for _, l := range steerLines(e.readLines(), auditKindFlagChg) {
		if l.Flag != hiddenSessionReadonlyGuardsFlagName {
			continue
		}
		if l.Phase == auditPhaseRequest {
			requested++
			continue
		}
		results++
		require.NotNil(t, l.Previous)
		require.NotNil(t, l.New)
		assert.NotEqual(t, *l.Previous, *l.New)
	}
	assert.Equal(t, 1, requested, "only the loosening flip writes a durable requested line first")
	assert.Equal(t, 2, results)

	e.mfs.failWrite = errDiskFull
	require.NoError(t, flagUpdateCtx(context.Background(), e.fix.svc.featureFlagSvc, hiddenSessionReadonlyGuardsFlagName, false),
		"off is the hatch for a sink fault: never refused")
	assert.False(t, e.fix.svc.guards.GuardsEnabled())
	assert.GreaterOrEqual(t, e.warn("flag_change_audit_degraded"), 1)
}
