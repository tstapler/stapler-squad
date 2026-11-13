package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestDriverAttempt_WaitingForOperatorDoesNotEscalate(t *testing.T) {
	t.Parallel()
	for _, status := range []detection.DetectedStatus{
		detection.StatusNeedsApproval, detection.StatusInputRequired,
		detection.StatusIdle, detection.StatusReady, detection.StatusSuccess,
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
			old := now.Add(-time.Hour)
			inst := &Instance{Title: t.Name(), Program: "pi", Status: domain.Active, InitialPromptSentAt: old}
			inst.SetLastMeaningfulOutput(old)
			source := NewPiStatusSource(inst.Title, nil)
			source.SetStatusForTest(status)
			manager := NewInstanceStatusManager()
			manager.RegisterPiStatusSource(inst.Title, source)
			inst.SetStatusManager(manager)
			attempt := newDriverAttempt(inst, "task", old)
			cont, ret := handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now)
			if cont || ret || inst.Snapshot().Status != domain.Active {
				t.Fatalf("waiting status %v must not fail or skip approval handling", status)
			}
			if idle := attempt.idleDuration(now, old); idle != 0 {
				t.Fatalf("waiting must reset the inactivity floor for resumed work, got %v", idle)
			}
		})
	}
}

func TestDriverAttempt_BacklogApprovalWaitResetsBudget(t *testing.T) {
	t.Parallel()
	for _, status := range []detection.DetectedStatus{detection.StatusNeedsApproval, detection.StatusInputRequired} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
			old := now.Add(-time.Hour)
			inst := &Instance{Title: t.Name(), Program: "pi", Status: domain.Active, Tags: []string{TagBacklogWork}, InitialPromptSentAt: old}
			inst.SetLastMeaningfulOutput(old)
			source := NewPiStatusSource(inst.Title, nil)
			source.SetStatusForTest(status)
			manager := NewInstanceStatusManager()
			manager.RegisterPiStatusSource(inst.Title, source)
			inst.SetStatusManager(manager)
			attempt := newDriverAttempt(inst, "task", old)
			attempt.nudgeSentAt = old
			cont, ret := handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now)
			if cont || ret || !attempt.nudgeSentAt.IsZero() || attempt.idleDuration(now, old) != 0 {
				t.Fatal("approval waiting must pause both retry and nudge budgets")
			}
			source.SetStatusForTest(detection.StatusProcessing)
			_, ret = handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now.Add(time.Minute))
			if ret {
				t.Fatal("resumed work must receive a fresh inactivity budget")
			}
			inst.RemoveTag(TagBacklogWork)
			inst.SetProcessManagerForTest(&livenessPM{alive: false}) // interactive: only a dead pane fails
			late := now.Add(driverInactivityTimeout + time.Second)
			handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, late) // first dead probe
			_, ret = handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, late.Add(deadPaneConfirmWindow))
			if !ret {
				t.Fatal("genuine inactivity after resuming must still fail")
			}
		})
	}
}

func TestDriverAttempt_BacklogIdleStillEscalates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	inst := &Instance{Title: t.Name(), Program: "pi", Status: domain.Active, Tags: []string{TagBacklogWork}, InitialPromptSentAt: old}
	inst.SetLastMeaningfulOutput(old)
	source := NewPiStatusSource(inst.Title, nil)
	source.SetStatusForTest(detection.StatusIdle)
	manager := NewInstanceStatusManager()
	manager.RegisterPiStatusSource(inst.Title, source)
	inst.SetStatusManager(manager)
	attempt := newDriverAttempt(inst, "task", old)
	cont, ret := handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now)
	if !cont || ret || inst.Snapshot().Status != domain.Active || attempt.nudgeSentAt.IsZero() {
		t.Fatal("idle backlog work must retain its inactivity nudge policy")
	}
}

func TestDriverAttempt_StalePersistedPromptDoesNotImmediatelyEscalate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	inst := &Instance{Title: "fresh-driver-stale-activity", Status: domain.Active, InitialPromptSentAt: old}
	inst.SetLastMeaningfulOutput(old)
	attempt := newDriverAttempt(inst, "task", now)
	cont, ret := handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now.Add(2*time.Second))
	if cont || ret || inst.Snapshot().Status != domain.Active {
		t.Fatalf("fresh driver immediately escalated stale persisted activity: continue=%v return=%v status=%v", cont, ret, inst.Snapshot().Status)
	}
}

func TestDriverAttempt_InactivityBaseline(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                                   string
		startedAt, promptAt, outputAt, nudgeAt time.Time
		backlog, archived, wantEscalation      bool
	}{
		{name: "fresh attempt", startedAt: now, promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour)},
		{name: "inactivity boundary", startedAt: now.Add(-driverInactivityTimeout), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour)},
		{name: "nudge delay boundary", startedAt: now.Add(-driverBacklogNudgeDelay), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), backlog: true},
		{name: "nudge grace boundary", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), nudgeAt: now.Add(-driverBacklogNudgeGrace), backlog: true},
		{name: "eventual inactivity (dead pane)", startedAt: now.Add(-driverInactivityTimeout - time.Second), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), wantEscalation: true},
		{name: "newer output wins", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Minute)},
		{name: "new prompt wins", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Minute), outputAt: now.Add(-time.Hour)},
		{name: "newer nudge wins", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), nudgeAt: now.Add(-time.Minute), backlog: true},
		{name: "output after nudge wins", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Minute), nudgeAt: now.Add(-driverBacklogNudgeGrace - time.Second), backlog: true},
		{name: "nudge grace expires", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), nudgeAt: now.Add(-driverBacklogNudgeGrace - time.Second), backlog: true, wantEscalation: true},
		{name: "archived inactivity stays retired", startedAt: now.Add(-time.Hour), promptAt: now.Add(-time.Hour), outputAt: now.Add(-time.Hour), archived: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{Title: tc.name, Status: domain.Active, InitialPromptSentAt: tc.promptAt}
			inst.SetProcessManagerForTest(&livenessPM{alive: false}) // interactive escalation requires a dead pane
			if tc.backlog {
				inst.Tags = []string{TagBacklogWork}
			}
			if tc.archived {
				inst.ArchivedAt = &now
			}
			inst.SetLastMeaningfulOutput(tc.outputAt)
			attempt := newDriverAttempt(inst, "task", tc.startedAt)
			attempt.nudgeSentAt = tc.nudgeAt
			tickAt := now
			if !tc.backlog { // interactive failure needs the pane dead across the confirm window
				handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, now)
				tickAt = now.Add(deadPaneConfirmWindow)
			}
			cont, ret := handleInactivityTick(context.Background(), inst, "", RetryPolicy{}, nil, &attempt, tickAt)
			if cont || ret != (tc.wantEscalation || tc.archived) {
				t.Errorf("loop signal=(%v,%v), want (false,%v)", cont, ret, tc.wantEscalation || tc.archived)
			}
			if failed := inst.Snapshot().Status == domain.PermanentlyFailed; failed != tc.wantEscalation {
				t.Fatalf("escalated=%v, want %v", failed, tc.wantEscalation)
			}
		})
	}
}

func TestDriverAttempt_PromptDedupeIndependentOfRestart(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, time.November, 3, 10, 0, 0, 0, time.UTC)
	inst := &Instance{InitialPromptSentAt: old}
	for _, startedAt := range []time.Time{old.Add(time.Hour), old.Add(2 * time.Hour)} {
		attempt := newDriverAttempt(inst, "task", startedAt)
		if !attempt.sentInitial || !attempt.initialPromptSentAt.Equal(old) || !attempt.startedAt.Equal(startedAt) {
			t.Fatalf("restart must retain dedupe and refresh activity floor: %+v", attempt)
		}
		if !inst.GetInitialPromptSentAt().Equal(old) {
			t.Fatal("attempt changed persisted prompt timestamp")
		}
	}
	unsent := newDriverAttempt(&Instance{}, "task", old)
	if unsent.sentInitial {
		t.Fatal("unsent prompt must still be delivered")
	}
	noPrompt := newDriverAttempt(&Instance{}, "", old)
	if !noPrompt.sentInitial || !noPrompt.initialPromptSentAt.Equal(old) {
		t.Fatal("empty prompt must skip delivery")
	}
}
