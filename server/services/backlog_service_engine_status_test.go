package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

var statNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// statReadyLedger satisfies every READY criterion for class operator at statNow
// and carries `graph` repro records at the given ages.
func statReadyLedger(reproAges ...time.Duration) session.ShadowLedger {
	cl := &session.ClassLedger{
		Calls:         map[string]int64{"CanTransition|transition": 300},
		Divergence:    map[string]int64{},
		NotCompared:   map[string]int64{},
		Edges:         map[string]session.EdgeStats{},
		ConfigSeen:    map[string]int64{session.ConfigSeeded: 300},
		FirstSampleAt: statNow.Add(-8 * 24 * time.Hour),
	}
	for _, e := range session.ExpectedEdgesForClass(session.ClassOperator) {
		cl.Edges[session.EdgeKey(e.From, e.To)] = session.EdgeStats{TransitionSamples: 25}
	}
	l := session.ShadowLedger{
		BinaryStarts: []time.Time{statNow.Add(-8 * 24 * time.Hour)},
		Classes:      map[string]*session.ClassLedger{string(session.ClassOperator): cl},
	}
	for _, age := range reproAges {
		l.Repro = append(l.Repro, session.ReproRecord{At: statNow.Add(-age), Class: string(session.ClassOperator), Divergence: "graph"})
	}
	return l
}

// statEditedDivergentLedger is a ready ledger with an unacknowledged divergence
// on one edge inside an edited-config window.
func statEditedDivergentLedger() session.ShadowLedger {
	l := statReadyLedger()
	cl := l.Classes[string(session.ClassOperator)]
	cl.ConfigSeen[session.ConfigEdited] = 5
	e := session.ExpectedEdgesForClass(session.ClassOperator)[0]
	key := session.EdgeKey(e.From, e.To)
	st := cl.Edges[key]
	st.Divergences = 1
	cl.Edges[key] = st
	cl.Divergence["CanTransition|graph|edited|transition"] = 1
	return l
}

func statModes(op, agent, system session.EngineMode) func(session.CallerClass) session.EngineMode {
	return func(c session.CallerClass) session.EngineMode {
		switch c {
		case session.ClassOperator:
			return op
		case session.ClassAgent:
			return agent
		}
		return system
	}
}

func statSource(ledger func() (session.ShadowLedger, error), modes func(session.CallerClass) session.EngineMode) *WorkflowEngineStatusSource {
	return &WorkflowEngineStatusSource{
		ModeFor:          modes,
		Ledger:           ledger,
		InvalidatorWired: func() bool { return true },
		GateWired:        func() bool { return true },
		FallbackActive:   func() bool { return false },
		ConfigDiff: func(context.Context) (session.ConfigDiff, []session.StrandedItem, error) {
			return session.ConfigDiff{Label: session.ConfigEdited}, []session.StrandedItem{{}, {}}, nil
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

func statLedgerOf(l session.ShadowLedger) func() (session.ShadowLedger, error) {
	return func() (session.ShadowLedger, error) { return l, nil }
}

func TestGetWorkflowEngineStatus_ReportsShadowForEveryClass_WithDivergenceAndWiring(t *testing.T) {
	t.Parallel()
	ledger := statReadyLedger(time.Hour, 2*time.Hour, 3*time.Hour, 30*time.Hour)
	msg, err := statCall(t, statSource(statLedgerOf(ledger), statModes(session.ModeShadow, session.ModeShadow, session.ModeShadow)))
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
	src := statSource(statLedgerOf(statReadyLedger()), statModes(session.ModeEnforce, session.ModeShadow, session.ModeShadow))
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
		"no recorder": {ModeFor: statModes(session.ModeOff, session.ModeOff, session.ModeOff)},
		"no modes":    {Ledger: statLedgerOf(session.ShadowLedger{})},
	} {
		_, err := statCall(t, src)
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), name)
	}
}

func TestGetWorkflowEngineStatus_ClassReadiness_FollowsSoakVerdict(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		ledger func() (session.ShadowLedger, error)
		acks   func() session.SoakAcks
		want   sessionv1.WorkflowEngineSoakVerdict
	}{
		"ready": {statLedgerOf(statReadyLedger(time.Hour)), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_READY},
		"not ready": {statLedgerOf(session.ShadowLedger{}), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NOT_READY},
		"needs owner ack": {statLedgerOf(statEditedDivergentLedger()), nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NEEDS_OWNER_ACK},
		"ledger error": {func() (session.ShadowLedger, error) { return session.ShadowLedger{}, errors.New("disk") }, nil,
			sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_UNAVAILABLE},
	}
	for name, tc := range cases {
		src := statSource(tc.ledger, statModes(session.ModeShadow, session.ModeShadow, session.ModeShadow))
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
	l.Repro = append(l.Repro, session.ReproRecord{At: statNow.Add(-time.Hour), Class: string(session.ClassAgent), Divergence: "graph"})
	msg, err := statCall(t, statSource(statLedgerOf(l), statModes(session.ModeShadow, session.ModeShadow, session.ModeShadow)))
	require.NoError(t, err)
	require.Equal(t, int64(3), msg.Divergence_24H)
	require.Equal(t, int64(2), msg.ClassReadiness["operator"].Divergence_24H)
	require.Equal(t, int64(1), msg.ClassReadiness["agent"].Divergence_24H)
	require.Equal(t, int64(0), msg.ClassReadiness["system"].Divergence_24H)
}

func TestGetWorkflowEngineStatus_ReviewFields(t *testing.T) {
	t.Parallel()
	start := statNow.Add(-8 * 24 * time.Hour)
	due := start.Add(session.SoakReviewAfter)
	shadow := statModes(session.ModeShadow, session.ModeShadow, session.ModeShadow)
	cases := map[string]struct {
		now      time.Time
		modes    func(session.CallerClass) session.EngineMode
		ledger   func() (session.ShadowLedger, error)
		state    sessionv1.WorkflowEngineReviewState
		overdue  bool
		wantDate bool
	}{
		"before the date": {now: due.Add(-time.Hour), modes: shadow, ledger: statLedgerOf(statReadyLedger()),
			state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_NOT_DUE, wantDate: true},
		"after, none enforced": {now: due.Add(time.Hour), modes: shadow, ledger: statLedgerOf(statReadyLedger()),
			state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_OVERDUE, overdue: true, wantDate: true},
		"after, one enforced": {now: due.Add(time.Hour), modes: statModes(session.ModeEnforce, session.ModeShadow, session.ModeShadow),
			ledger: statLedgerOf(statReadyLedger()), state: sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_ENFORCED, wantDate: true},
		"ledger error": {now: due.Add(time.Hour), modes: shadow,
			ledger: func() (session.ShadowLedger, error) { return session.ShadowLedger{}, errors.New("disk") },
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
