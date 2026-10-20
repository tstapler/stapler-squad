package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var soakNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// readyLedger is a ledger that satisfies every READY criterion for class
// operator at soakNow: 8 days of history, Group 1 edges at 25 samples each.
func readyLedger() ShadowLedger {
	cl := newClassLedger()
	cl.Calls["CanTransition|transition"] = 300
	cl.ConfigSeen[ConfigSeeded] = 300
	cl.FirstSampleAt = soakNow.Add(-8 * 24 * time.Hour)
	for _, e := range systemRequiredCore[:4] {
		cl.Edges[EdgeKey(e.From, e.To)] = EdgeStats{TransitionSamples: 25}
	}
	return ShadowLedger{
		Version: shadowLedgerVersion, BinaryStarts: []time.Time{soakNow.Add(-8 * 24 * time.Hour)},
		Classes: map[string]*ClassLedger{string(ClassOperator): cl},
	}
}

func evalOperator(l ShadowLedger, mutate func(*SoakParams)) Verdict {
	p := DefaultSoakParams(ClassOperator, soakNow)
	p.ReproPath = "/cfg/workflow-engine-shadow-ledger.json"
	if mutate != nil {
		mutate(&p)
	}
	return EvaluateSoak(ClassOperator, l, p)
}

func TestEvaluateSoak_IsReady_WhenEveryCriterionMet(t *testing.T) {
	t.Parallel()
	v := evalOperator(readyLedger(), nil)
	require.Equal(t, VerdictReady, v.Status, v.Reasons)
	require.Zero(t, v.ExitCode)
	require.Equal(t, ConfigSeeded, v.ConfigWindow)
}

func TestEvaluateSoak_NotReady_WhenReadPathDivergenceOnSeededConfig_ButReadSamplesDoNotCountTowardFloor(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(ClassOperator)]
	cl.Divergence["AllowedTransitions|graph|seeded|read"] = 1
	v := evalOperator(l, nil)
	require.Equal(t, VerdictNotReady, v.Status)
	require.Equal(t, int64(1), v.Alarms["graph"])
	require.Contains(t, strings.Join(v.Reasons, "\n"), "/cfg/workflow-engine-shadow-ledger.json", "prints the repro path")

	// Read calls alone never reach the floor.
	l = readyLedger()
	cl = l.Classes[string(ClassOperator)]
	cl.Calls = map[string]int64{"AllowedTransitions|read": 10000}
	v = evalOperator(l, nil)
	require.Equal(t, int64(0), v.TransitionSamples)
	require.Contains(t, strings.Join(v.Reasons, "\n"), "floor")
}

func TestEvaluateSoak_LabelsWindowEdited_AndNeedsAckForDivergence_WhenConfigEditedInsideWindow(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(ClassOperator)]
	cl.ConfigSeen[ConfigEdited] = 5
	cl.Divergence["CanTransition|graph|edited|transition"] = 5
	cl.Edges["done->refining"] = EdgeStats{TransitionSamples: 5, Divergences: 5}

	v := evalOperator(l, nil)
	require.Equal(t, ConfigEdited, v.ConfigWindow, "a window spanning an edit is never seeded")
	require.Equal(t, VerdictNeedsOwnerAck, v.Status)

	v = evalOperator(l, func(p *SoakParams) { p.Acks.DivergentEdges = []string{"done->refining"} })
	require.Equal(t, VerdictReady, v.Status, v.Reasons)
}

func TestEvaluateSoak_ListsStatefulEdgeUnverified_AndExcludesItFromEvidence(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(ClassOperator)]
	cl.Edges["review->done"] = EdgeStats{TransitionSamples: 400, StatefulSkips: 400}
	cl.Calls["ValidateGates|transition"] = 400

	v := evalOperator(l, nil)
	require.Equal(t, VerdictNeedsOwnerAck, v.Status)
	require.Contains(t, strings.Join(v.Unverified, "\n"), "review->done UNVERIFIED in shadow (not_compared{stateful_gate}) skipped=400")
	require.Equal(t, int64(300), v.TransitionSamples, "stateful-edge samples are not evidence")

	v = evalOperator(l, func(p *SoakParams) { p.Acks.StatefulEdges = []string{"review->done"} })
	require.Equal(t, VerdictReady, v.Status, v.Reasons)
	require.NotEmpty(t, v.Unverified, "still listed after acknowledgement")
}

func TestSoakStatus_PrintsReviewDueDate_FromEarliestBinaryStartPlus14Days(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	l := readyLedger()
	l.BinaryStarts = []time.Time{first, first.Add(24 * time.Hour), first.Add(48 * time.Hour)}

	v := evalOperator(l, func(p *SoakParams) { p.Now = first.Add(3 * 24 * time.Hour) })

	require.Equal(t, "REVIEW DUE 2026-11-03", v.Review)
	require.Contains(t, v.Text(), "REVIEW DUE 2026-11-03")
	require.NotContains(t, v.Text(), "OVERDUE")
}

func TestSoakStatus_PrintsReviewOverdue_WithoutChangingExitCode_WhenDatePassedAndNoClassEnforced(t *testing.T) {
	t.Parallel()
	first := soakNow.Add(-20 * 24 * time.Hour)
	ready := readyLedger()
	ready.BinaryStarts = []time.Time{first}
	ready.Classes[string(ClassOperator)].FirstSampleAt = first

	v := evalOperator(ready, nil)
	require.Equal(t, VerdictReady, v.Status)
	require.Zero(t, v.ExitCode, "advisory only: a READY stays READY")
	require.Contains(t, v.Review, "REVIEW OVERDUE: no class is enforced; run the Outcome review")

	enforced := evalOperator(ready, func(p *SoakParams) { p.AnyClassEnforced = true })
	require.NotContains(t, enforced.Review, "OVERDUE")

	notReady := readyLedger()
	notReady.BinaryStarts = []time.Time{first}
	notReady.Classes[string(ClassOperator)].Divergence["CanTransition|graph|seeded|transition"] = 2
	v = evalOperator(notReady, nil)
	require.Equal(t, 1, v.ExitCode, "the overdue line never changes a failing exit code")
	require.Contains(t, v.Review, "OVERDUE")
}

func TestEvaluateSoak_ReturnsSameVerdictAsSoakStatusCommand_ForEveryClass_AndUnavailableOnLedgerError(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	for _, class := range []CallerClass{ClassOperator, ClassAgent, ClassSystem} {
		p := DefaultSoakParams(class, soakNow)
		direct := EvaluateSoak(class, l, p)
		viaSource := EvaluateSoakFromSource(class, func() (ShadowLedger, error) { return l, nil }, p)
		require.Equal(t, direct, viaSource, class)
		require.Equal(t, direct.Text(), viaSource.Text())
	}
	require.Equal(t, 14*24*time.Hour, DefaultSoakParams(ClassSystem, soakNow).Window)
	require.Equal(t, 7*24*time.Hour, DefaultSoakParams(ClassAgent, soakNow).Window)

	v := EvaluateSoakFromSource(ClassOperator, func() (ShadowLedger, error) { return readyLedger(), errors.New("disk gone") }, DefaultSoakParams(ClassOperator, soakNow))
	require.Equal(t, VerdictUnavailable, v.Status)
	require.Equal(t, 2, v.ExitCode)
	require.Equal(t, 2, OverallSoakExit([]Verdict{v}))
}

func TestOverallSoakExit_IsOneForNoVerdictsAndForAnyNotReady(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1, OverallSoakExit(nil))
	require.Equal(t, 1, OverallSoakExit([]Verdict{{ExitCode: 0}, {ExitCode: 1}}))
	require.Equal(t, 0, OverallSoakExit([]Verdict{{ExitCode: 0}}))
}
