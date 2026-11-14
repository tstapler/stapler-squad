package services

// Story 5.1.3: runtime proofs for feature flags whose effect lives in the
// session and terminal services. Each flips the flag through
// FeatureFlagService.UpdateFeatureFlag (wired the way SessionService wires it)
// and observes the production consumer's answer. Helpers carry the bkf prefix.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestConfigSurfaceProof_HiddenSessionReadonlyGuards_ChangesWriteToHiddenSession(t *testing.T) {
	e := newGuardsEnv(t)
	hidden := e.addSession("bkf-guards-hidden", true, "")
	ts := newLeaseTerminalService(t, hidden.inst)
	ts.SetGuardsFlag(e.fix.svc.guards)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		e.setGuards(setting.(bool))
		_, err := ts.WriteToSession(t.Context(), writeToSessionReq("bkf-guards-hidden"))
		return err == nil
	}, true, false)
}

func TestConfigSurfaceProof_HiddenSessionReply_ChangesReplyOutcome(t *testing.T) {
	e := newReplyEnv(t)
	e.fix.svc.featureFlagSvc.SetAudit(e.sink, map[string]FlagAuditPolicy{hiddenSessionReplyFlagName: replyFlagAuditPolicy})
	e.fix.svc.wireReplyFlag()
	replies := 0

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, flagUpdateCtx(t.Context(), e.fix.svc.featureFlagSvc, hiddenSessionReplyFlagName, setting.(bool)))
		replies++
		return e.replyOK("1", "bkf-reply-"+string(rune('a'+replies))).Outcome
	}, false, true)
}

func TestConfigSurfaceProof_TerminalWriteLease_ChangesLeaseExclusivity(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	inst := &instance.Instance{UUID: "bkf-lease"}

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, flagUpdateCtx(t.Context(), svc.featureFlagSvc, terminalWriteLeaseFlagName, setting.(bool)))
		first, ok := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
		require.True(t, ok)
		defer first.Release()
		second, secondOK := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
		second.Release()
		return secondOK
	}, true, false)
}

func TestConfigSurfaceProof_HiddenSessionGate_ChangesDeliveryGateCache(t *testing.T) {
	svc := newGatedFlagService(t)
	svc.DeliveryGate().Stats().SetWriterRunning(true)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, gateFlagUpdate(svc, t.Context(), setting.(bool)))
		return svc.DeliveryGate().Flags().Enabled()
	}, false, true)
}
