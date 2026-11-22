package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// recordingNudgeGate records every verify call and returns a scripted error.
type recordingNudgeGate struct {
	err     error
	targets []*instance.Instance
}

func (g *recordingNudgeGate) VerifyNudgeSafeToWrite(_ context.Context, target *instance.Instance) error {
	g.targets = append(g.targets, target)
	return g.err
}

func nudgeGateFixture(t *testing.T, gate NudgeGate) (*diagnoseHandlers, *instance.Instance) {
	t.Helper()
	target := instance.NewStartedInstanceForTest(t, "nudge-gate-target", &recordingPM{})
	live := &fakeLiveInstanceFinder{instances: map[string]*instance.Instance{"target-1": target}}
	return &diagnoseHandlers{storage: newTestBacklogStorage(t), live: live, nudgeGate: gate}, target
}

func TestResolveNudgeTarget_should_PassTheLiveTargetToTheGateAndReturnIt_When_TheGateAllows(t *testing.T) {
	t.Parallel()
	gate := &recordingNudgeGate{}
	dh, target := nudgeGateFixture(t, gate)

	got, res := dh.resolveNudgeTarget(context.Background(), nudgeSessionArgs{targetSessionID: "target-1"})

	assert.Nil(t, res)
	assert.Same(t, target, got)
	require.Len(t, gate.targets, 1)
	assert.Same(t, target, gate.targets[0])
}

func TestResolveNudgeTarget_should_RefuseWithPermissionDenied_When_TheGateRefuses(t *testing.T) {
	t.Parallel()
	gate := &recordingNudgeGate{err: errors.New("pane is not idle")}
	dh, _ := nudgeGateFixture(t, gate)

	got, res := dh.resolveNudgeTarget(context.Background(), nudgeSessionArgs{targetSessionID: "target-1"})

	require.NotNil(t, res)
	assert.Nil(t, got)
	errObj := parseResult(t, res)["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
	assert.Contains(t, errObj["message"], "pane is not idle")
}

func TestResolveNudgeTarget_should_ReportInternalError_When_TheGateErrorsBecauseTheRequestWasCancelled(t *testing.T) {
	t.Parallel()
	gate := &recordingNudgeGate{err: errors.New("settle wait aborted")}
	dh, _ := nudgeGateFixture(t, gate)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, res := dh.resolveNudgeTarget(ctx, nudgeSessionArgs{targetSessionID: "target-1"})

	require.NotNil(t, res)
	assert.Nil(t, got)
	errObj := parseResult(t, res)["error"].(map[string]interface{})
	assert.Equal(t, ErrInternalError, errObj["code"])
	assert.Contains(t, errObj["message"], "context canceled")
}

func TestResolveNudgeTarget_should_FailClosed_When_NoGateIsWired(t *testing.T) {
	t.Parallel()
	dh, _ := nudgeGateFixture(t, nil)

	got, res := dh.resolveNudgeTarget(context.Background(), nudgeSessionArgs{targetSessionID: "target-1"})

	require.NotNil(t, res)
	assert.Nil(t, got)
	errObj := parseResult(t, res)["error"].(map[string]interface{})
	assert.Equal(t, ErrInternalError, errObj["code"])
}

func TestNewDiagnoseHandlers_should_CarryTheConfiguredNudgeGate(t *testing.T) {
	t.Parallel()
	gate := &recordingNudgeGate{}

	dh := newDiagnoseHandlers(coreDeps{cfg: newCoreConfig([]CoreOption{WithNudgeGate(gate)})}, nil)

	assert.Same(t, gate, dh.nudgeGate)
}

func TestWithNudgeGate_should_ExposeTheGateThroughTheOptions(t *testing.T) {
	t.Parallel()
	gate := &recordingNudgeGate{}

	assert.Same(t, gate, NudgeGateFromOptions(WithNudgeGate(gate)))
	assert.True(t, NudgeGateFromOptions() == nil)
}

func TestRecordDiagnoseWriteFailure_should_LabelEachErrorByItsKind(t *testing.T) {
	// Swaps the process-wide recorder per case, so it cannot run in parallel.
	cases := []struct {
		name                      string
		err                       error
		wantUncertain, wantFailed int64
	}{
		{"bare uncertain sentinel", domain.ErrNudgeSubmitUncertain, 1, 0},
		{"wrapped uncertain", fmt.Errorf("%w: boom", domain.ErrNudgeSubmitUncertain), 1, 0},
		{"plain failure", errors.New("plain failure"), 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := session.NewNudgeOutcomeRecorder("", nil)
			defer session.SwapNudgeOutcomeRecorder(rec)()
			target := instance.NewStartedInstanceForTest(t, "outcome-target", &recordingPM{})

			recordDiagnoseWriteFailure(target, tc.err)

			got := map[session.NudgeOutcomeLabel]int64{}
			for _, row := range rec.Snapshot().Totals {
				got[row.Outcome] += row.Value
			}
			assert.Equal(t, tc.wantUncertain, got[session.NudgeOutcomeUncertain])
			assert.Equal(t, tc.wantFailed, got[session.NudgeOutcomeFailed])
		})
	}
}

// ctxHonouringNudgeGate fails with the context error, as the real gate's settle wait does.
type ctxHonouringNudgeGate struct{}

func (ctxHonouringNudgeGate) VerifyNudgeSafeToWrite(ctx context.Context, _ *instance.Instance) error {
	return ctx.Err()
}
