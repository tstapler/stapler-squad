package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

// livenessPM reports a fixed pane liveness; every other ProcessManager call
// panics via the nil embedded interface, so a test fails loudly if the stall
// path starts touching the pane.
type livenessPM struct {
	ProcessManager
	alive bool
}

func (p *livenessPM) IsAlive() bool               { return p.alive }
func (p *livenessPM) HasLiveSessionNoCache() bool { return p.alive }

// stallNotifier records NotifySession calls; Notify (the PF path) is recorded
// separately so a test can assert which one fired.
type stallNotifier struct {
	mu       sync.Mutex
	sessions []string
	failures []string
}

func (n *stallNotifier) Notify(_, title, _ string, _ int32, _, _ bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = append(n.failures, title)
}

func (n *stallNotifier) NotifySession(_, title, _ string, _ int32, _, _ bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sessions = append(n.sessions, title)
}

func (n *stallNotifier) counts() (sessions, failures int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.sessions), len(n.failures)
}

var stallTestNow = time.Date(2026, time.November, 5, 13, 0, 0, 0, time.UTC)

// retryEverything mirrors the production default that produced the 2026-11-05
// permanent failures: enabled, one attempt, every reason eligible.
func retryEverything() RetryPolicy {
	return RetryPolicy{
		Enabled:     true,
		MaxAttempts: 1,
		RetryOn:     []string{"crashed", "stalled", "tmux_exited"},
	}
}

type stallFixture struct {
	inst     *Instance
	attempt  driverAttempt
	notifier *stallNotifier
}

// stallScenario describes what the driver can observe about a session. The
// zero value is a live interactive pane with an active status controller.
type stallScenario struct {
	status        detection.DetectedStatus
	controllerOff bool // no status controller: detected status reads Unknown
	paneDead      bool
	autonomous    bool          // backlog work session
	quiet         time.Duration // how long before stallTestNow output last arrived
}

func newStallFixture(t *testing.T, sc stallScenario) *stallFixture {
	t.Helper()
	last := stallTestNow.Add(-sc.quiet)
	inst := &Instance{Title: t.Name(), Program: "pi", Status: domain.Active, InitialPromptSentAt: last}
	inst.RetryMaxAttempts = retryEverything().MaxAttempts // driver start snapshots this
	if sc.autonomous {
		inst.Tags = []string{TagBacklogWork}
	}
	inst.processManager = &livenessPM{alive: !sc.paneDead}
	inst.SetLastMeaningfulOutput(last)
	manager := NewInstanceStatusManager()
	if !sc.controllerOff {
		source := NewPiStatusSource(inst.Title, nil)
		source.status.Store(int32(sc.status))
		manager.RegisterPiStatusSource(inst.Title, source)
	}
	inst.SetStatusManager(manager)
	n := &stallNotifier{}
	inst.SetNotifier(n)
	return &stallFixture{inst: inst, attempt: newDriverAttempt(inst, "task", last), notifier: n}
}

func (f *stallFixture) tick(policy RetryPolicy, now time.Time) (cont, ret bool) {
	return handleInactivityTick(context.Background(), f.inst, "", policy, nil, &f.attempt, now)
}

// The observed 2026-11-05 sequence: a live pane doing real work (Executing)
// with no meaningful output, stalled twice. It used to schedule a kill+resume
// restart on the first stall and mark the session permanently failed on the
// second.
func TestInteractiveStall_LiveWorkingPaneIsNeverRestartedOrFailed(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusExecuting})
	policy := retryEverything()

	// Two quiet periods, one stall budget past each last-output reference.
	for round, now := range []time.Time{
		stallTestNow.Add(3*time.Hour + time.Second),
		stallTestNow.Add(6*time.Hour + time.Second),
	} {
		f.tick(policy, now)
		rs := f.inst.RetrySnapshot()
		if rs.RetryAttempt != 0 || !rs.NextRetryAt.IsZero() {
			t.Fatalf("round %d: live working pane was scheduled for a restart: %+v", round, rs)
		}
		if got := f.inst.Snapshot().Status; got == domain.PermanentlyFailed {
			t.Fatalf("round %d: live interactive session was marked permanently failed", round)
		}
	}
	if _, failures := f.notifier.counts(); failures != 0 {
		t.Fatalf("gave-up notification fired for a live session: %d", failures)
	}
}

// The same sequence with the status controller gone: restartForRetry's
// Restart path logged "failed to restart controller" for perf, tymux and mcp,
// so the second stall read Unknown at an idle prompt and escaped the
// operator-wait exemption.
func TestInteractiveStall_ControllerlessLivePaneIsNotFailed(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{controllerOff: true})
	f.inst.RetryAttempt = 1 // first stall already consumed the only attempt

	f.tick(retryEverything(), stallTestNow.Add(3*time.Hour))

	if got := f.inst.Snapshot().Status; got == domain.PermanentlyFailed {
		t.Fatal("controllerless live pane was marked permanently failed")
	}
	if _, failures := f.notifier.counts(); f.attempt.stallFlags != 1 || failures != 0 {
		t.Fatalf("want one quiet note and no gave-up notification, got %d/%d", f.attempt.stallFlags, failures)
	}
}

func TestDecideStall(t *testing.T) {
	t.Parallel()
	const budget = 30 * time.Minute
	statuses := []detection.DetectedStatus{
		detection.StatusUnknown, detection.StatusProcessing, detection.StatusExecuting,
		detection.StatusCompacting, detection.StatusWaitingForAgent, detection.StatusError,
		detection.StatusTestsFailing,
	}
	for _, status := range statuses {
		interactiveBudget := budget
		if isWorkingStatus(status) {
			interactiveBudget = workingStallMultiplier * budget
		}
		for _, tc := range []struct {
			name       string
			autonomous bool
			paneAlive  bool
			idle       time.Duration
			want       stallVerdict
		}{
			{"interactive live inside driver timeout", false, true, driverInactivityTimeout, stallWithinBudget},
			{"interactive live just under budget", false, true, interactiveBudget, stallWithinBudget},
			{"interactive live past budget", false, true, interactiveBudget + time.Second, stallNeedsAttention},
			{"interactive dead inside driver timeout", false, false, driverInactivityTimeout, stallWithinBudget},
			{"interactive dead past driver timeout", false, false, driverInactivityTimeout + time.Second, stallPaneDead},
			{"autonomous live inside grace", true, true, driverInactivityTimeout, stallWithinBudget},
			{"autonomous live past grace", true, true, driverInactivityTimeout + time.Second, stallRestart},
			{"autonomous dead past grace", true, false, driverInactivityTimeout + time.Second, stallRestart},
		} {
			t.Run(status.String()+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				e := stallEvidence{
					Autonomous: tc.autonomous, Status: status, PaneAlive: tc.paneAlive, Idle: tc.idle,
					Budget: budget,
				}
				if tc.autonomous {
					e.Budget = driverInactivityTimeout
				}
				if got := decideStall(e); got != tc.want {
					t.Fatalf("decideStall(%+v) = %v, want %v", e, got, tc.want)
				}
			})
		}
	}
}

// Attempt counters never change the interactive verdict: a quiet live pane is
// not retried at attempt 0, at the cap, or beyond it.
func TestInteractiveStall_AttemptCountersAreIrrelevantForLivePanes(t *testing.T) {
	t.Parallel()
	for _, attemptsUsed := range []int{0, 1, 5} {
		f := newStallFixture(t, stallScenario{status: detection.StatusProcessing})
		f.inst.RetryAttempt = attemptsUsed
		f.tick(retryEverything(), stallTestNow.Add(3*time.Hour))
		rs := f.inst.RetrySnapshot()
		if f.inst.Snapshot().Status == domain.PermanentlyFailed || rs.RetryAttempt != attemptsUsed || !rs.NextRetryAt.IsZero() {
			t.Fatalf("attempts=%d: retry state touched for a live pane: %+v", attemptsUsed, rs)
		}
	}
}

func TestInteractiveStall_FlagIsEdgeTriggeredAndRearmsOnOutput(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusExecuting})
	policy := retryEverything()
	start := stallTestNow.Add(2*interactiveStallBudget(policy) + time.Minute)

	f.tick(policy, start)
	f.tick(policy, start.Add(driverPollInterval))
	f.tick(policy, start.Add(2*driverPollInterval))
	if f.attempt.stallFlags != 1 {
		t.Fatalf("quiet flag must fire once per episode, fired %d times", f.attempt.stallFlags)
	}

	// Output resumes (still past the driver timeout, inside the budget), then
	// goes quiet again: a new episode may flag once more.
	f.inst.SetLastMeaningfulOutput(start.Add(-driverInactivityTimeout - time.Minute))
	f.tick(policy, start)
	f.tick(policy, start.Add(2*interactiveStallBudget(policy)+time.Minute))
	if f.attempt.stallFlags != 2 {
		t.Fatalf("a fresh quiet episode must re-flag, total notes %d", f.attempt.stallFlags)
	}
}

func TestInteractiveStall_WorkingStatusGetsLongerBudget(t *testing.T) {
	t.Parallel()
	policy := retryEverything()
	pastBudget := stallTestNow.Add(interactiveStallBudget(policy) + time.Minute)

	working := newStallFixture(t, stallScenario{status: detection.StatusExecuting})
	working.tick(policy, pastBudget)
	if working.attempt.stallFlags != 0 {
		t.Fatal("a working pane must not be flagged at the base budget")
	}

	unknown := newStallFixture(t, stallScenario{status: detection.StatusUnknown})
	unknown.tick(policy, pastBudget)
	if unknown.attempt.stallFlags != 1 {
		t.Fatal("a non-working quiet pane must be flagged at the base budget")
	}
}

func TestInteractiveStall_ConfiguredBudgetIsHonored(t *testing.T) {
	t.Parallel()
	policy := retryEverything()
	policy.InteractiveStallTimeout = 12 * time.Minute
	f := newStallFixture(t, stallScenario{status: detection.StatusUnknown})
	f.tick(policy, stallTestNow.Add(11*time.Minute))
	if f.attempt.stallFlags != 0 {
		t.Fatal("flagged before the configured budget")
	}
	f.tick(policy, stallTestNow.Add(12*time.Minute+time.Second))
	if f.attempt.stallFlags != 1 {
		t.Fatal("not flagged after the configured budget")
	}
}

// A genuinely dead pane keeps the failure path. RetryOn excludes tmux_exited so
// the verdict does not depend on the process-boot restart-grace window
// (serverStartTime), which a parallel test cannot control.
func TestInteractiveStall_DeadPaneStillFailsThroughRetryPolicy(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusUnknown, paneDead: true})
	policy := retryEverything()
	policy.RetryOn = []string{"crashed"}

	later := stallTestNow.Add(driverInactivityTimeout + time.Minute)
	f.tick(policy, later)
	if f.inst.Snapshot().Status == domain.PermanentlyFailed {
		t.Fatal("failed on the first dead probe")
	}
	_, ret := f.tick(policy, later.Add(deadPaneConfirmWindow))

	if !ret || f.inst.Snapshot().Status != domain.PermanentlyFailed {
		t.Fatal("dead pane must still be marked permanently failed")
	}
	if _, failures := f.notifier.counts(); failures != 1 {
		t.Fatalf("want one gave-up notification, got %d", failures)
	}
}

// Backlog sessions keep stall -> retry -> permanently failed, whatever the
// detected status says.
func TestAutonomousStall_RetainsRestartSemantics(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusExecuting, autonomous: true})
	policy := retryEverything()
	f.attempt.nudgeSentAt = stallTestNow
	later := stallTestNow.Add(driverBacklogNudgeGrace + time.Second)

	cont, ret := f.tick(policy, later)
	rs := f.inst.RetrySnapshot()
	if !cont || ret || rs.RetryAttempt != 1 || rs.LastFailureReason != "stalled" {
		t.Fatalf("autonomous first stall: cont=%v ret=%v state=%+v", cont, ret, rs)
	}
	f.attempt.nudgeSentAt = stallTestNow
	_, ret = f.tick(policy, later.Add(time.Minute))
	if !ret || f.inst.Snapshot().Status != domain.PermanentlyFailed {
		t.Fatal("autonomous second stall must still mark permanently failed")
	}
}

func TestInteractiveStall_OperatorWaitStillExempt(t *testing.T) {
	t.Parallel()
	for _, status := range []detection.DetectedStatus{detection.StatusIdle, detection.StatusSuccess, detection.StatusNeedsApproval} {
		f := newStallFixture(t, stallScenario{status: status})
		f.tick(retryEverything(), stallTestNow.Add(24*time.Hour))
		if f.attempt.stallFlags != 0 {
			t.Fatalf("status %v waiting on the operator was flagged as quiet", status)
		}
	}
}

func TestResolveRetryPolicy_InteractiveStallTimeout(t *testing.T) {
	t.Parallel()
	global := config.RetryPolicyConfig{}
	if got := resolveRetryPolicy(global, nil).InteractiveStallTimeout; got != 30*time.Minute {
		t.Fatalf("default = %v, want 30m", got)
	}
	global.InteractiveStallMinutes = 45
	if got := resolveRetryPolicy(global, nil).InteractiveStallTimeout; got != 45*time.Minute {
		t.Fatalf("global = %v, want 45m", got)
	}
	override := &config.RetryPolicyConfig{InteractiveStallMinutes: 5}
	if got := resolveRetryPolicy(global, override).InteractiveStallTimeout; got != 5*time.Minute {
		t.Fatalf("override = %v, want 5m", got)
	}
}

// One failed liveness probe is not a failure; two consecutive ones are, and an
// alive probe in between resets the count.
func TestInteractiveStall_TransientDeadProbeDoesNotFail(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusUnknown, paneDead: true})
	policy := retryEverything()
	policy.RetryOn = []string{"crashed"}
	later := stallTestNow.Add(driverInactivityTimeout + time.Minute)
	pm := f.inst.processManager.(*livenessPM)

	f.tick(policy, later) // dead since `later`
	pm.alive = true
	f.tick(policy, later.Add(deadPaneConfirmWindow-time.Second)) // alive: resets the window
	pm.alive = false
	f.tick(policy, later.Add(deadPaneConfirmWindow)) // dead again, window restarts here
	if _, ret := f.tick(policy, later.Add(deadPaneConfirmWindow+time.Second)); ret || f.inst.Snapshot().Status == domain.PermanentlyFailed {
		t.Fatal("dead probes separated by an alive probe must not fail the session")
	}
	if _, ret := f.tick(policy, later.Add(2*deadPaneConfirmWindow)); !ret {
		t.Fatal("a pane dead for the whole confirm window must fail the session")
	}
}

// Operator-wait status flapping resets the stall budget and must not re-fire
// the flag; only new output re-arms it.
func TestInteractiveStall_StatusFlappingDoesNotRefireFlag(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t, stallScenario{status: detection.StatusExecuting})
	policy := retryEverything()
	source, ok := f.inst.GetStatusManager().piSources.Load(f.inst.Title)
	if !ok {
		t.Fatal("fixture has no status source")
	}
	set := func(s detection.DetectedStatus) { source.status.Store(int32(s)) }
	now := stallTestNow.Add(4 * time.Hour)

	f.tick(policy, now)
	for i := 1; i <= 6; i++ {
		set(detection.StatusIdle) // operator wait: budget resets
		f.tick(policy, now.Add(time.Duration(i)*time.Minute))
		set(detection.StatusExecuting)
		f.tick(policy, now.Add(time.Duration(i)*time.Minute+interactiveStallBudget(policy)*workingStallMultiplier+time.Minute))
	}
	if f.attempt.stallFlags != 1 {
		t.Fatalf("flapping status re-fired the flag %d times", f.attempt.stallFlags)
	}
}

// The interactive driver must outlive its longest stall budget, or the quiet
// flag can never fire (the 25m safety-net deadline once ended it first).
func TestDriverTotalDeadline_InteractiveOutlivesStallBudget(t *testing.T) {
	t.Parallel()
	start := stallTestNow
	budget := interactiveStallBudget(RetryPolicy{}) * workingStallMultiplier
	if d := driverTotalDeadline(&Instance{Title: "interactive"}, start); !d.IsZero() && d.Sub(start) <= budget {
		t.Fatalf("interactive deadline %v must be unbounded or exceed the stall budget %v", d.Sub(start), budget)
	}
	if got := driverTotalDeadline(&Instance{Title: "backlog", Tags: []string{TagBacklogWork}}, start); got != start.Add(driverTotalTimeout) {
		t.Fatalf("autonomous deadline = %v, want the %v safety net", got.Sub(start), driverTotalTimeout)
	}
}

// Through the real gate: with an interactive deadline an already-elapsed
// wall-clock budget does not end the driver; an autonomous one does.
func TestDriverTickGate_InteractiveIgnoresElapsedDeadline(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "gate", Status: domain.Active}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	stop := make(chan struct{})
	if _, _, exit := driverTickGate(inst, stop, ticker, driverTotalDeadline(inst, time.Now().Add(-2*driverTotalTimeout))); exit {
		t.Fatal("interactive driver exited on the total deadline")
	}
	inst.Tags = []string{TagBacklogWork}
	if _, _, exit := driverTickGate(inst, stop, ticker, driverTotalDeadline(inst, time.Now().Add(-2*driverTotalTimeout))); !exit {
		t.Fatal("autonomous driver must still stop at the total deadline")
	}
}
