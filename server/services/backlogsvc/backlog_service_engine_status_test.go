package backlogsvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

var statNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// statReadyLedger satisfies every READY criterion for class operator at statNow
// and carries `graph` repro records at the given ages.
func statReadyLedger(reproAges ...time.Duration) workflowengine.ShadowLedger {
	cl := &workflowengine.ClassLedger{
		Calls:         map[string]int64{"CanTransition|transition": 300},
		Divergence:    map[string]int64{},
		NotCompared:   map[string]int64{},
		Edges:         map[string]workflowengine.EdgeStats{},
		ConfigSeen:    map[string]int64{workflowengine.ConfigSeeded: 300},
		FirstSampleAt: statNow.Add(-8 * 24 * time.Hour),
	}
	for _, e := range workflowengine.ExpectedEdgesForClass(domain.ClassOperator) {
		cl.Edges[workflowengine.EdgeKey(e.From, e.To)] = workflowengine.EdgeStats{TransitionSamples: 25}
	}
	l := workflowengine.ShadowLedger{
		BinaryStarts: []time.Time{statNow.Add(-8 * 24 * time.Hour)},
		Classes:      map[string]*workflowengine.ClassLedger{string(domain.ClassOperator): cl},
	}
	for _, age := range reproAges {
		l.Repro = append(l.Repro, workflowengine.ReproRecord{At: statNow.Add(-age), Class: string(domain.ClassOperator), Divergence: "graph"})
	}
	return l
}

// statEditedDivergentLedger is a ready ledger with an unacknowledged divergence
// on one edge inside an edited-config window.
func statEditedDivergentLedger() workflowengine.ShadowLedger {
	l := statReadyLedger()
	cl := l.Classes[string(domain.ClassOperator)]
	cl.ConfigSeen[workflowengine.ConfigEdited] = 5
	e := workflowengine.ExpectedEdgesForClass(domain.ClassOperator)[0]
	key := workflowengine.EdgeKey(e.From, e.To)
	st := cl.Edges[key]
	st.Divergences = 1
	cl.Edges[key] = st
	cl.Divergence["CanTransition|graph|edited|transition"] = 1
	return l
}

func statModes(op, agent, system workflowengine.EngineMode) func(domain.CallerClass) workflowengine.EngineMode {
	return func(c domain.CallerClass) workflowengine.EngineMode {
		switch c {
		case domain.ClassOperator:
			return op
		case domain.ClassAgent:
			return agent
		}
		return system
	}
}

func statSource(ledger func() (workflowengine.ShadowLedger, error), modes func(domain.CallerClass) workflowengine.EngineMode) *WorkflowEngineStatusSource {
	return &WorkflowEngineStatusSource{
		ModeFor:          modes,
		Ledger:           ledger,
		InvalidatorWired: func() bool { return true },
		GateWired:        func() bool { return true },
		FallbackActive:   func() bool { return false },
		ConfigDiff: func(context.Context) (workflowengine.ConfigDiff, []workflowengine.StrandedItem, error) {
			return workflowengine.ConfigDiff{Label: workflowengine.ConfigEdited}, []workflowengine.StrandedItem{{}, {}}, nil
		},
		Now: func() time.Time { return statNow },
	}
}

func statCall(t *testing.T, src *WorkflowEngineStatusSource) (*sessionv1.GetWorkflowEngineStatusResponse, error) {
	t.Helper()
	svc := &BacklogService{}
	svc.SetWorkflowEngineStatusSource(src)
	resp, err := svc.GetWorkflowEngineStatus(context.Background(), connect.NewRequest(&sessionv1.GetWorkflowEngineStatusRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func statLedgerOf(l workflowengine.ShadowLedger) func() (workflowengine.ShadowLedger, error) {
	return func() (workflowengine.ShadowLedger, error) { return l, nil }
}

func TestGetWorkflowEngineStatus_ReportsShadowForEveryClass_WithDivergenceAndWiring(t *testing.T) {
	t.Parallel()
	ledger := statReadyLedger(time.Hour, 2*time.Hour, 3*time.Hour, 30*time.Hour)
	msg, err := statCall(t, statSource(statLedgerOf(ledger), statModes(workflowengine.ModeShadow, workflowengine.ModeShadow, workflowengine.ModeShadow)))
	require.NoError(t, err)

	for _, c := range []string{"operator", "agent", "system"} {
		require.Equal(t, sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_SHADOW, msg.ClassModes[c], c)
	}
	require.Equal(t, int64(3), msg.Divergence_24H, "the 30h-old record is outside the window")
	require.Equal(t, sessionv1.WorkflowEngineConfigProvenance_WORKFLOW_ENGINE_CONFIG_PROVENANCE_EDITED, msg.ConfigProvenance)
	require.True(t, msg.InvalidatorWired)
	require.True(t, msg.GateWired)
	require.False(t, msg.FallbackActive)
}

func TestGetWorkflowEngineStatus_ReportsEachClassSeparately_AndGateUnwired(t *testing.T) {
	t.Parallel()
	src := statSource(statLedgerOf(statReadyLedger()), statModes(workflowengine.ModeEnforce, workflowengine.ModeShadow, workflowengine.ModeShadow))
	src.GateWired = func() bool { return false }
	msg, err := statCall(t, src)
	require.NoError(t, err)

	require.Equal(t, sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_ENFORCE, msg.ClassModes["operator"])
	require.Equal(t, sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_SHADOW, msg.ClassModes["agent"])
	require.Equal(t, sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_SHADOW, msg.ClassModes["system"])
	require.False(t, msg.GateWired)
}

func TestGetWorkflowEngineStatus_ReturnsError_WhenSourceOrRecorderUnwired(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]*WorkflowEngineStatusSource{
		"no source":   nil,
		"no recorder": {ModeFor: statModes(workflowengine.ModeOff, workflowengine.ModeOff, workflowengine.ModeOff)},
		"no modes":    {Ledger: statLedgerOf(workflowengine.ShadowLedger{})},
	} {
		_, err := statCall(t, src)
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), name)
	}
}

func TestGetWorkflowEngineStatus_ClassReadiness_FollowsSoakVerdict(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		ledger func() (workflowengine.ShadowLedger, error)
		acks   func() workflowengine.SoakAcks
		want   sessionv1.WorkflowEngineSoakVerdict
	}{
		"ready": {statLedgerOf(statReadyLedger(time.Hour)), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_READY},
		"not ready": {statLedgerOf(workflowengine.ShadowLedger{}), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NOT_READY},
		"needs owner ack": {statLedgerOf(statEditedDivergentLedger()), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NEEDS_OWNER_ACK},
		"ledger error": {func() (workflowengine.ShadowLedger, error) { return workflowengine.ShadowLedger{}, errors.New("disk") }, nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_UNAVAILABLE},
	}
	for name, tc := range cases {
		src := statSource(tc.ledger, statModes(workflowengine.ModeShadow, workflowengine.ModeShadow, workflowengine.ModeShadow))
		src.Acks = tc.acks
		msg, err := statCall(t, src)
		require.NoError(t, err, name)
		op := msg.ClassReadiness["operator"]
		require.Equal(t, tc.want, op.SoakVerdict, name)
		require.Equal(t, int64(2), op.StrandedItemCount, name)
	}
}

func TestGetWorkflowEngineStatus_ClassReadiness_CountsDivergencePerClass(t *testing.T) {
	t.Parallel()
	l := statReadyLedger(time.Hour, time.Hour)
	l.Repro = append(l.Repro, workflowengine.ReproRecord{At: statNow.Add(-time.Hour), Class: string(domain.ClassAgent), Divergence: "graph"})
	msg, err := statCall(t, statSource(statLedgerOf(l), statModes(workflowengine.ModeShadow, workflowengine.ModeShadow, workflowengine.ModeShadow)))
	require.NoError(t, err)
	require.Equal(t, int64(3), msg.Divergence_24H)
	require.Equal(t, int64(2), msg.ClassReadiness["operator"].Divergence_24H)
	require.Equal(t, int64(1), msg.ClassReadiness["agent"].Divergence_24H)
	require.Equal(t, int64(0), msg.ClassReadiness["system"].Divergence_24H)
}

func TestGetWorkflowEngineStatus_ReviewFields(t *testing.T) {
	t.Parallel()
	start := statNow.Add(-8 * 24 * time.Hour)
	due := start.Add(workflowengine.SoakReviewAfter)
	shadow := statModes(workflowengine.ModeShadow, workflowengine.ModeShadow, workflowengine.ModeShadow)
	cases := map[string]struct {
		now      time.Time
		modes    func(domain.CallerClass) workflowengine.EngineMode
		ledger   func() (workflowengine.ShadowLedger, error)
		state    sessionv1.WorkflowEngineReviewState
		overdue  bool
		wantDate bool
	}{
		"before the date": {now: due.Add(-time.Hour), modes: shadow, ledger: statLedgerOf(statReadyLedger()),
			state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_NOT_DUE, wantDate: true},
		"after, none enforced": {now: due.Add(time.Hour), modes: shadow, ledger: statLedgerOf(statReadyLedger()),
			state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_OVERDUE, overdue: true, wantDate: true},
		"after, one enforced": {now: due.Add(time.Hour), modes: statModes(workflowengine.ModeEnforce, workflowengine.ModeShadow, workflowengine.ModeShadow),
			ledger: statLedgerOf(statReadyLedger()), state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_ENFORCED, wantDate: true},
		"ledger error": {now: due.Add(time.Hour), modes: shadow,
			ledger: func() (workflowengine.ShadowLedger, error) { return workflowengine.ShadowLedger{}, errors.New("disk") },
			state:  sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_UNAVAILABLE},
	}
	for name, tc := range cases {
		src := statSource(tc.ledger, tc.modes)
		src.Now = func() time.Time { return tc.now }
		msg, err := statCall(t, src)
		require.NoError(t, err, name)
		require.Equal(t, tc.state, msg.ReviewState, name)
		require.Equal(t, tc.overdue, msg.ReviewOverdue, name)
		if tc.wantDate {
			require.True(t, due.Equal(msg.ReviewDueAt.AsTime()), name)
		} else {
			require.Nil(t, msg.ReviewDueAt, name)
		}
	}
}
