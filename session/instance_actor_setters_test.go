package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestSetAutoApprove_should_NotRestart_When_StatusPaused(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = domain.Paused

	persisted := false
	err := inst.SetAutoApprove(true, func() error {
		persisted = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, inst.AutoApprove)
	assert.True(t, persisted, "persist must run even when no restart occurs")
}

func TestSetAutoApprove_should_ReturnErrorAndSkipRestart_When_PersistFails(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = domain.Active
	inst.MarkStarted()
	persistErr := errors.New("disk full")

	err := inst.SetAutoApprove(true, func() error {
		return persistErr
	})

	require.ErrorIs(t, err, persistErr)
	// Matches the documented ordering (set -> persist -> restart): the in-memory
	// field is already flipped by the time persist runs, but a persist failure
	// must short-circuit before any restart is attempted.
	assert.True(t, inst.AutoApprove)
}

// TestSetAutoApprove_RestartError_PersistAlreadyRan mirrors
// TestSwitchProgram_RestartError_PersistAlreadyRan (session/instance_program_test.go):
// a restart failure must not roll back or hide the already-persisted field change, so
// the caller (UpdateSession) and the persisted store agree on the new value even though
// the session didn't actually restart.
func TestSetAutoApprove_RestartError_PersistAlreadyRan(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = domain.Active
	inst.MarkStarted()
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	persistedValue := false
	persistRan := false
	err := inst.SetAutoApprove(true, func() error {
		persistRan = true
		persistedValue = inst.AutoApprove
		return nil
	})

	require.Error(t, err, "Restart should fail with no working directory configured")
	assert.True(t, persistRan, "persist callback must run before the restart attempt")
	assert.True(t, persistedValue, "persist callback must observe the new value, i.e. run after the field is set")
	assert.True(t, inst.AutoApprove, "AutoApprove field must reflect the change even though restart failed")
}

// TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls is the AC7
// regression guard: SetAutoApprove and SwitchProgram must serialize through the shared
// restartTriggerMu so a racing program-switch and auto-approve toggle can't both observe
// Status == Active and double-restart the same tmux session. Proven deterministically (not
// just via -race) by blocking SetAutoApprove mid-persist and asserting a concurrent
// SwitchProgram call cannot proceed until the lock is released.
func TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = domain.Active
	inst.MarkStarted()
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	entered := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = inst.SetAutoApprove(true, func() error {
			close(entered)
			<-release
			return nil
		})
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("SetAutoApprove never reached persist callback")
	}

	done := make(chan struct{})
	go func() {
		_, _, _ = inst.SwitchProgram(context.Background(), "aider", nil)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("SwitchProgram completed while SetAutoApprove held restartTriggerMu — the two setters are not sharing a lock")
	case <-time.After(100 * time.Millisecond):
		// Expected: SwitchProgram is blocked waiting on restartTriggerMu.
	}

	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SwitchProgram never completed after SetAutoApprove released restartTriggerMu")
	}
}

func TestInstance_SetPinned_should_SetPinnedTrue_When_ToggledOn(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)

	require.NoError(t, inst.SetPinned(true))
	assert.True(t, inst.Snapshot().Pinned)

	require.NoError(t, inst.SetPinned(true), "pinning twice must be idempotent")
	require.NoError(t, inst.SetPinned(false))
	require.NoError(t, inst.SetPinned(false), "unpinning twice must be idempotent")
	assert.False(t, inst.Snapshot().Pinned)
}

func TestInstance_SetPinned_should_RejectPin_When_SessionIsArchived(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	now := time.Now()
	inst.SetArchivedAt(&now)

	err := inst.SetPinned(true)

	assert.ErrorIs(t, err, instance.ErrCannotPinArchivedSession)
	assert.False(t, inst.Snapshot().Pinned)
}

func TestInstance_SetArchivedAt_should_ClearPinned_When_Archiving(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	require.NoError(t, inst.SetPinned(true))

	now := time.Now()
	inst.SetArchivedAt(&now)
	assert.False(t, inst.Snapshot().Pinned)

	inst.SetArchivedAt(nil)
	assert.False(t, inst.Snapshot().Pinned, "unarchiving must not re-pin")
}

// --- Story 3.3.1: reclassifyTagsLocked fixpoint hook ---

func TestReclassifyTagsLocked_should_ApplyMatchingSeedRuleTagSynchronously_When_BranchSetterCalled(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Branch = "main"
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetGitHubResolution(domain.GitHubResolution{Path: inst.Path, Branch: "bugfix/pr-poller"})

	assert.Contains(t, inst.GetTags(), "Bugfix")
	assert.Equal(t, "seed-bugfix", inst.RuleTagProvenance["Bugfix"])
}

func TestReclassifyTagsLocked_should_RetractRuleOwnedTag_When_ConditionNoLongerHolds(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{"Important"}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	// Prime: apply the rule via a mutating setter (SetGitHubResolution with an unchanged
	// branch — Branch stays "bugfix/x", Path is set for the first time).
	inst.SetGitHubResolution(domain.GitHubResolution{Path: inst.Path, Branch: "bugfix/x"})
	require.Contains(t, inst.GetTags(), "Bugfix")

	// Rename off the matching branch.
	inst.SetGitHubResolution(domain.GitHubResolution{Path: inst.Path, Branch: "main"})

	tags := inst.GetTags()
	assert.NotContains(t, tags, "Bugfix", "rule-owned tag must be retracted once its condition no longer holds")
	assert.Contains(t, tags, "Important", "an unrelated user tag must never be touched by retraction")
}

func TestReclassifyTagsLocked_should_NotReAddSuppressedTag_When_RuleConditionMatchesAgain(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.SuppressedRuleTags = map[string]bool{"Bugfix": true}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetGitHubResolution(domain.GitHubResolution{Path: inst.Path, Branch: "bugfix/x"})

	assert.NotContains(t, inst.GetTags(), "Bugfix", "a suppressed tag must never be re-added by the fixpoint")
}

func TestReclassifyTagsLocked_should_PreserveLLMProvenancedTag_When_UnrelatedSetterRuns(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Tags = []string{"Feature"}
	inst.RuleTagProvenance = map[string]string{"Feature": instance.LlmSentinelRuleID}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil) // no sync rule can match "Feature" -- and none has ID "llm"
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	assert.Contains(t, inst.GetTags(), "Feature", "an LLM-provenanced tag must survive the sync retraction loop")
	assert.Equal(t, instance.LlmSentinelRuleID, inst.RuleTagProvenance["Feature"])
}

func TestReclassifyTagsLocked_should_RetractTag_When_OwningRuleDeletedViaCRUD(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{"Bugfix"}
	inst.RuleTagProvenance = map[string]string{"Bugfix": "seed-bugfix"}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	// Simulate a CRUD DeleteTaggingRule: the rule with ID "seed-bugfix" no longer exists.
	engine.ReplaceRules(nil)

	inst.SetProgram("aider")

	assert.NotContains(t, inst.GetTags(), "Bugfix", "a tag whose owning rule was deleted must be retracted, not orphaned")
	_, hasProvenance := inst.RuleTagProvenance["Bugfix"]
	assert.False(t, hasProvenance)
}

// --- Story 3.4.1: Unclassified coexistence rule ---

func TestDropUnclassifiedIfOtherTagsPresentLocked_should_RemoveUnclassified_When_RealTagAdded(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.Tags = []string{instance.UnclassifiedTag}
	inst.RuleTagProvenance = map[string]string{instance.UnclassifiedTag: instance.LlmSentinelRuleID}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	tags := inst.GetTags()
	assert.Contains(t, tags, "Bugfix")
	assert.NotContains(t, tags, instance.UnclassifiedTag)
}

func TestDropUnclassifiedIfOtherTagsPresentLocked_should_LeaveUnclassified_When_NoOtherTagPresent(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Tags = []string{instance.UnclassifiedTag}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil)
	inst.SetTaggingEngine(engine)

	inst.SetProgram("aider")

	assert.Equal(t, []string{instance.UnclassifiedTag}, inst.GetTags())
}

// --- Story 2.3.3 addendum: fire-count recording from reclassifyTagsLocked ---

// fakeTagFireRecorder records every RecordTaggingRuleFire call for assertion, satisfying
// session.TagFireRecorder structurally (no import needed the other way — see that
// interface's doc comment).
type fakeTagFireRecorder struct {
	fired []string
}

func (f *fakeTagFireRecorder) RecordTaggingRuleFire(ruleID string) {
	f.fired = append(f.fired, ruleID)
}

func TestReclassifyTagsLocked_should_RecordTagFire_When_RuleMatchesRegardlessOfSuppression(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Branch = "bugfix/x"
	inst.SuppressedRuleTags = map[string]bool{"Bugfix": true}
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{fixturetest.BugfixSeedRule()})
	inst.SetTaggingEngine(engine)
	recorder := &fakeTagFireRecorder{}
	inst.SetTagFireRecorder(recorder)

	inst.SetProgram("aider")

	assert.Equal(t, []string{"seed-bugfix"}, recorder.fired, "the rule fired (and must be recorded) even though \"Bugfix\" is suppressed and never actually applied")
	assert.NotContains(t, inst.GetTags(), "Bugfix", "sanity check: the tag itself must indeed be suppressed")
}

func TestInstance_SetPinned_should_BumpUpdatedAt_OnlyWhenValueChanges(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	before := inst.Snapshot().UpdatedAt

	require.NoError(t, inst.SetPinned(true))
	afterPin := inst.Snapshot().UpdatedAt
	assert.True(t, afterPin.After(before), "pin must bump UpdatedAt so clients don't dedupe it away")

	require.NoError(t, inst.SetPinned(true))
	assert.Equal(t, afterPin, inst.Snapshot().UpdatedAt, "no-op pin must not bump UpdatedAt")
}

// autonomousInstance is a live-actor instance with autonomy on mid-run.
func autonomousInstance(t *testing.T) *instance.Instance {
	t.Helper()
	inst := minimalInstance(t)
	li := instance.NewLiveInstance(inst)
	t.Cleanup(li.Stop)
	inst.SetAutonomousMode(true, "")
	inst.SetAutonomousTurn(4, 20)
	return inst
}

func wantOutcome(reason domain.StopReason) string {
	switch reason {
	case domain.StopReasonDone:
		return "done"
	case domain.StopReasonCancelledExternal:
		return ""
	default:
		return "stuck"
	}
}

func TestSetAutonomousStopped_OutcomeByReason(t *testing.T) {
	t.Parallel()
	reasons := append([]domain.StopReason{""}, domain.AllStopReasons...)
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			t.Parallel()
			inst := autonomousInstance(t)

			require.NoError(t, inst.SetAutonomousStopped(reason))

			got := inst.Snapshot().Autonomous
			assert.False(t, got.AutonomousMode)
			assert.Zero(t, got.AutonomousTurn)
			assert.Zero(t, got.AutonomousMaxTurns)
			assert.Equal(t, wantOutcome(reason), got.AutonomousOutcome)
			assert.Contains(t, []string{"", "done", "stuck"}, got.AutonomousOutcome)
			assert.Equal(t, reason, got.AutonomousStopReason)
		})
	}
}

func TestSetAutonomousStoppedQuiet_OutcomeAlwaysEmpty(t *testing.T) {
	t.Parallel()
	reasons := append([]domain.StopReason{""}, domain.AllStopReasons...)
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			t.Parallel()
			inst := autonomousInstance(t)

			require.NoError(t, inst.SetAutonomousStoppedQuiet(reason))

			got := inst.Snapshot().Autonomous
			assert.False(t, got.AutonomousMode)
			assert.Equal(t, "", got.AutonomousOutcome)
			assert.Equal(t, reason, got.AutonomousStopReason)
		})
	}
}

func TestSetAutonomousStopObserved_KeepsModeAndOutcome(t *testing.T) {
	t.Parallel()
	inst := autonomousInstance(t)
	inst.SetAutonomousMode(true, "stuck")
	inst.SetAutonomousTurn(4, 20)

	require.NoError(t, inst.SetAutonomousStopObserved(domain.StopReasonStartupTimeout))

	got := inst.Snapshot().Autonomous
	assert.True(t, got.AutonomousMode, "an observed-only stop keeps autonomy on")
	assert.Equal(t, "stuck", got.AutonomousOutcome, "outcome is untouched")
	assert.Zero(t, got.AutonomousTurn)
	assert.Zero(t, got.AutonomousMaxTurns)
	assert.Equal(t, domain.StopReasonStartupTimeout, got.AutonomousStopReason)

	inst.SetAutonomousMode(true, "")
	assert.Equal(t, domain.StopReason(""), inst.Snapshot().Autonomous.AutonomousStopReason, "a new run clears the reason")
}

func TestInstanceSnapshot_CarriesAutonomousStopReason(t *testing.T) {
	t.Parallel()
	inst := autonomousInstance(t)
	assert.Equal(t, domain.StopReason(""), inst.Snapshot().Autonomous.AutonomousStopReason)

	require.NoError(t, inst.SetAutonomousStopObserved(domain.StopReasonSubmitFailed))
	assert.Equal(t, domain.StopReasonSubmitFailed, inst.Snapshot().Autonomous.AutonomousStopReason)

	require.NoError(t, inst.SetAutonomousStopped(domain.StopReasonMaxTurns))
	assert.Equal(t, domain.StopReasonMaxTurns, inst.Snapshot().Autonomous.AutonomousStopReason)

	require.NoError(t, inst.SetAutonomousStoppedQuiet(domain.StopReasonCancelledShutdown))
	assert.Equal(t, domain.StopReasonCancelledShutdown, inst.Snapshot().Autonomous.AutonomousStopReason)
}

func TestAutonomousSetters_NoRaceWithSnapshotReaders(t *testing.T) {
	t.Parallel()
	inst := autonomousInstance(t)
	const workers, iterations = 8, 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				_ = inst.SetAutonomousStopped(domain.StopReasonMaxTurns)
				_ = inst.SetAutonomousStopObserved(domain.StopReasonStartupTimeout)
				_ = inst.SetAutonomousStoppedQuiet(domain.StopReasonCancelledShutdown)
				inst.SetAutonomousMode(true, "")
			}
		}()
		go func() {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				_ = inst.Snapshot().Autonomous
			}
		}()
	}
	wg.Wait()
}

func TestAutonomousSetters_ReturnErrorWhenActorDone(t *testing.T) {
	t.Parallel()
	setters := map[string]func(*instance.Instance) error{
		"Stopped": func(i *instance.Instance) error { return i.SetAutonomousStopped(domain.StopReasonOrchestratorError) },
		"StopObserved": func(i *instance.Instance) error {
			return i.SetAutonomousStopObserved(domain.StopReasonOrchestratorError)
		},
		"StoppedQuiet": func(i *instance.Instance) error {
			return i.SetAutonomousStoppedQuiet(domain.StopReasonOrchestratorError)
		},
	}
	for name, set := range setters {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inst := minimalInstance(t)
			inst.AutonomousMode = true
			inst.PublishSnapshotForTest()
			li := instance.NewLiveInstance(inst)
			li.CancelForTest()
			t.Cleanup(li.Stop)
			before := inst.Snapshot().Autonomous

			err := set(inst)

			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, before, inst.Snapshot().Autonomous, "a failed write must not change the snapshot")
		})
	}

	t.Run("nil actor writes directly", func(t *testing.T) {
		t.Parallel()
		inst := minimalInstance(t)
		for _, set := range setters {
			assert.NoError(t, set(inst))
		}
	})
}
