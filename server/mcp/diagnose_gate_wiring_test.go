package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

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
