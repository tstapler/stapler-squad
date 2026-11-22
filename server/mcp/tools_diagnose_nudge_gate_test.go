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

func TestRecordDiagnoseWriteFailure_should_CountUncertainApartFromFailed(t *testing.T) {
	// Swaps the process-wide recorder, so it cannot run in parallel.
	rec := session.NewNudgeOutcomeRecorder("", nil)
	defer session.SwapNudgeOutcomeRecorder(rec)()
	target := instance.NewStartedInstanceForTest(t, "outcome-target", &recordingPM{})

	recordDiagnoseWriteFailure(target, fmt.Errorf("%w: boom", domain.ErrNudgeSubmitUncertain))
	recordDiagnoseWriteFailure(target, errors.New("plain failure"))

	got := map[session.NudgeOutcomeLabel]int64{}
	for _, row := range rec.Snapshot().Totals {
		got[row.Outcome] += row.Value
	}
	assert.Equal(t, int64(1), got[session.NudgeOutcomeUncertain])
	assert.Equal(t, int64(1), got[session.NudgeOutcomeFailed])
}

// ctxHonouringNudgeGate fails with the context error, as the real gate's settle wait does.
type ctxHonouringNudgeGate struct{}

func (ctxHonouringNudgeGate) VerifyNudgeSafeToWrite(ctx context.Context, _ *instance.Instance) error {
	return ctx.Err()
}
