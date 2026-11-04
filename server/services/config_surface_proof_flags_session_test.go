package services

// Story 5.1.3: runtime proofs for feature flags whose effect lives in the
// session and terminal services. Each flips the flag through
// FeatureFlagService.UpdateFeatureFlag (wired the way SessionService wires it)
// and observes the production consumer's answer. Helpers carry the bkf prefix.

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
)

func TestConfigSurfaceProof_HiddenSessionReadonlyGuards_ChangesWriteToHiddenSession(t *testing.T) {
	e := newGuardsEnv(t)
	hidden := e.addSession("bkf-guards-hidden", true, "")
	ts := newLeaseTerminalService(t, hidden.inst)
	ts.SetGuardsFlag(e.fix.svc.guards)

	assertConfigChangesBehavior(t, func(setting any) any {
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

	assertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, flagUpdateCtx(t.Context(), e.fix.svc.featureFlagSvc, hiddenSessionReplyFlagName, setting.(bool)))
		replies++
		return e.replyOK("1", "bkf-reply-"+string(rune('a'+replies))).Outcome
	}, false, true)
}

func TestConfigSurfaceProof_TerminalWriteLease_ChangesLeaseExclusivity(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	inst := &session.Instance{UUID: "bkf-lease"}

	assertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, flagUpdateCtx(t.Context(), svc.featureFlagSvc, terminalWriteLeaseFlagName, setting.(bool)))
		first, ok := inst.TryTerminalWriteLease(session.LeaseWriterOther)
		require.True(t, ok)
		defer first.Release()
		second, secondOK := inst.TryTerminalWriteLease(session.LeaseWriterOther)
		second.Release()
		return secondOK
	}, true, false)
}

func TestConfigSurfaceProof_HiddenSessionGate_ChangesDeliveryGateCache(t *testing.T) {
	svc := newGatedFlagService(t)
	svc.DeliveryGate().Stats().SetWriterRunning(true)

	assertConfigChangesBehavior(t, func(setting any) any {
		require.NoError(t, gateFlagUpdate(svc, t.Context(), setting.(bool)))
		return svc.DeliveryGate().Flags().Enabled()
	}, false, true)
}

func TestConfigSurfaceProof_TerminalResyncFlags_ChangeResyncOptions(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, field := range map[string]func(ResyncOptions) bool{
		terminalResyncSkipStaleDimensionSlowpathFlagName: func(o ResyncOptions) bool { return o.SkipStaleDimensionSlowPath },
		terminalResyncExecGateFastLaneFlagName:           func(o ResyncOptions) bool { return o.UseFastLane },
		terminalResyncCorrelationIDFlagName:              func(o ResyncOptions) bool { return o.EchoResyncID },
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return field(currentResyncOptions())
			}, false, true)
		})
	}
}

func TestConfigSurfaceProof_TerminalResyncCompression_ChangesEnvelopeFlag(t *testing.T) {
	set := bkfFlagWriter(t)
	largeContent := bytes.Repeat([]byte("resync-payload-filler-"), 100)

	assertConfigChangesBehavior(t, func(setting any) any {
		set(terminalResyncCompressionFlagName, setting.(bool))
		stream, clientConn, cleanup := createTestWebSocketPair(t)
		defer cleanup()
		target := &fakePanePTY{captureContent: string(largeContent), cols: 80, rows: 24}
		onCurrentPane := func(ctx context.Context, r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return handleCurrentPaneRequest(ctx, "bkf-session", target, r, currentResyncOptionsFor(testWriter()))
		}
		var settling atomic.Bool
		handleCurrentPaneRequestFrame(stream, "bkf-session", &sessionv1.CurrentPaneRequest{}, onCurrentPane, &settling)
		return readEnvelopeFromClient(t, clientConn).Flags&protocol.CompressedFlag != 0
	}, false, true)
}

func TestConfigSurfaceProof_TerminalAppScrollbackForwardingClaude_ChangesScrollbackPath(t *testing.T) {
	set := bkfFlagWriter(t)
	inst := session.NewStartedInstanceForTest(t, "bkf-scroll", &replyPM{})
	inst.Program = "claude"

	assertConfigChangesBehavior(t, func(setting any) any {
		set(config.FeatureAppScrollForwardingClaude, setting.(bool))
		result, err := scrollbackResultForRequest(scrollbackRequestParams{
			writer:          testWriter(),
			instance:        inst,
			subscriberCount: -1,
			fallback:        func(string, string) (string, error) { return "tmux-native", nil },
		})
		require.NoError(t, err)
		return result.AppScroll != nil
	}, false, true)
}
