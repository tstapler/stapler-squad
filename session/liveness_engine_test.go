package session

import (
	"bytes"
	stdlog "log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tslog "github.com/tstapler/stapler-squad/log"
)

// TestDefaultLivenessEngine_should_ReturnThirtyFiveMinuteThreshold_When_StageIsIdeaDefaultMode
// is the Story 1.2.1 zero-regression assertion for Shape A: the idea stage's
// duration-budget-plus-margin definition must derive to exactly 35m, today's
// maxHeadlessTriageSessionStaleness (session/backlog_lifecycle_triage.go).
func TestDefaultLivenessEngine_should_ReturnThirtyFiveMinuteThreshold_When_StageIsIdeaDefaultMode(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	def, err := engine.LivenessFor(BacklogStatusIdea, PipelineModeDefault)

	require.NoError(t, err)
	assert.Equal(t, LivenessKindDurationBudget, def.Kind)
	assert.Equal(t, 35*time.Minute, def.StalenessThreshold())
	assert.Equal(t, maxHeadlessTriageSessionStaleness, def.StalenessThreshold())
}

// TestDefaultLivenessEngine_should_ReturnTwoHourHeartbeat_When_StageIsInProgressDefaultMode
// is Story 1.2.1's second named acceptance criterion (Shape B): the
// in_progress stage's heartbeat definition must match today's
// maxWorkSessionStaleness (session/backlog_lifecycle_stale.go).
func TestDefaultLivenessEngine_should_ReturnTwoHourHeartbeat_When_StageIsInProgressDefaultMode(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	def, err := engine.LivenessFor(BacklogStatusInProgress, PipelineModeDefault)

	require.NoError(t, err)
	assert.Equal(t, LivenessKindHeartbeat, def.Kind)
	assert.Equal(t, 2*time.Hour, def.MaxNoProgressDuration)
	assert.Equal(t, maxWorkSessionStaleness, def.MaxNoProgressDuration)
}

// TestDefaultLivenessEngine_should_ReturnCycleFrequencyThreeInTwentyFourHours_When_StageIsReviewDefaultMode
// covers Shape C (bouncing), keyed to BacklogStatusReview per this file's
// documented judgment call (see NewDefaultLivenessEngine's comment). Values
// must match today's bounceThreshold/bounceLookback (session/stuck_decisions.go).
func TestDefaultLivenessEngine_should_ReturnCycleFrequencyThreeInTwentyFourHours_When_StageIsReviewDefaultMode(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	def, err := engine.LivenessFor(BacklogStatusReview, PipelineModeDefault)

	require.NoError(t, err)
	assert.Equal(t, LivenessKindCycleFrequency, def.Kind)
	assert.Equal(t, 3, def.CycleThreshold)
	assert.Equal(t, bounceThreshold, def.CycleThreshold)
	assert.Equal(t, 24*time.Hour, def.CycleLookback)
	assert.Equal(t, bounceLookback, def.CycleLookback)
}

// TestDefaultLivenessEngine_should_ReturnNoTimeoutSentinel_When_StageHasNoTimeoutConcept
// covers plan_not_approved and blocked_by_dependency (both DequeueNextQueuedItems
// gates anchored to BacklogStatusQueued, per architecture.md §1): stages with
// no timeout concept must resolve to the NoTimeoutLiveness sentinel, not an
// error and not one of the three real LivenessKind shapes.
func TestDefaultLivenessEngine_should_ReturnNoTimeoutSentinel_When_StageHasNoTimeoutConcept(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	def, err := engine.LivenessFor(BacklogStatusQueued, PipelineModeDefault)

	require.NoError(t, err)
	assert.True(t, def.IsNoTimeout())
	assert.Equal(t, NoTimeoutLiveness, def)
	assert.Empty(t, def.Kind)
}

// TestDefaultLivenessEngine_should_ReturnNoTimeoutSentinel_When_StageIsCompletelyUnmapped
// is the general fail-closed case behind the named test above: ANY stage
// without a table entry — not just the two named no-timeout reasons — must
// resolve to the sentinel rather than an error, so an unrecognized/future
// custom stage can never produce a zero/infinite threshold or abort a sweep.
func TestDefaultLivenessEngine_should_ReturnNoTimeoutSentinel_When_StageIsCompletelyUnmapped(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	def, err := engine.LivenessFor(BacklogStatus("some_future_custom_stage"), PipelineModeDefault)

	require.NoError(t, err)
	assert.True(t, def.IsNoTimeout())
}

// TestDefaultLivenessEngine_should_IgnorePipelineMode_When_NoOverrideTableExists
// documents that DefaultLivenessEngine has no per-mode overrides of its own —
// a non-default mode still resolves to the identical built-in value. Per-mode
// overrides are Epic 1.3's CachingLivenessEngine, backed by
// stage_liveness_definitions rows.
func TestDefaultLivenessEngine_should_IgnorePipelineMode_When_NoOverrideTableExists(t *testing.T) {
	engine := NewDefaultLivenessEngine()

	defDefault, err := engine.LivenessFor(BacklogStatusIdea, PipelineModeDefault)
	require.NoError(t, err)
	defSDD, err := engine.LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))
	require.NoError(t, err)

	assert.Equal(t, defDefault, defSDD)
}

// countNonEmptyLines counts non-empty newline-delimited lines in a captured
// log buffer — the same idiom liveness_cache_test.go uses to assert "exactly
// one Warn line," extracted here so both Story 1.6.1 tests below can share it.
func countNonEmptyLines(s string) int {
	if s == "" {
		return 0
	}
	lines := 0
	for _, line := range bytes.Split(bytes.TrimRight([]byte(s), "\n"), []byte("\n")) {
		if len(line) > 0 {
			lines++
		}
	}
	return lines
}

// TestLivenessFor_should_EmitExactlyOneWarnLine_When_FallingBackToDefaultEngine
// is Story 1.6.1's named happy-path observability test: a single
// CachingLivenessEngine.LivenessFor call for a stage/mode with neither an
// exact nor a mode-less cache row emits exactly one [LivenessEngine] Warn
// line (not zero — the fallback must be visible — and not more than one).
func TestLivenessFor_should_EmitExactlyOneWarnLine_When_FallingBackToDefaultEngine(t *testing.T) {
	engine := &CachingLivenessEngine{
		repo:            nil, // never touched: cache is deliberately empty, no Load/Invalidate call
		cache:           &livenessCache{},
		embeddedDefault: NewDefaultLivenessEngine(),
	}

	var buf bytes.Buffer
	orig := tslog.SetWarningLogForTest(stdlog.New(&buf, "WARNING: ", 0))
	t.Cleanup(func() { tslog.SetWarningLogForTest(orig) })

	got, err := engine.LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))

	require.NoError(t, err)
	assert.Equal(t, LivenessKindDurationBudget, got.Kind)
	logged := buf.String()
	assert.Equal(t, 1, countNonEmptyLines(logged), "expected exactly one Warn line, got: %q", logged)
	assert.Contains(t, logged, "[LivenessEngine]")
	assert.Contains(t, logged, "stage=idea")
	assert.Contains(t, logged, "mode=sdd")
}

// TestLivenessFor_should_NotEmitDuplicateWarnLines_When_CalledRepeatedlyForSameUnresolvedStageAndMode
// is Story 1.6.1's named error-path observability test: LivenessFor has no
// internal retry loop around its cache-miss fallback, so N repeated calls for
// the identical unresolved (stage, mode) pair must log exactly N Warn lines —
// one per call, never a multiple of N (no hidden internal retry re-logging)
// and never fewer (no first-call-only de-duplication silently swallowing
// later fallbacks).
func TestLivenessFor_should_NotEmitDuplicateWarnLines_When_CalledRepeatedlyForSameUnresolvedStageAndMode(t *testing.T) {
	engine := &CachingLivenessEngine{
		repo:            nil, // never touched: cache is deliberately empty, no Load/Invalidate call
		cache:           &livenessCache{},
		embeddedDefault: NewDefaultLivenessEngine(),
	}

	var buf bytes.Buffer
	orig := tslog.SetWarningLogForTest(stdlog.New(&buf, "WARNING: ", 0))
	t.Cleanup(func() { tslog.SetWarningLogForTest(orig) })

	const calls = 3
	for i := 0; i < calls; i++ {
		_, err := engine.LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))
		require.NoError(t, err)
	}

	logged := buf.String()
	assert.Equal(t, calls, countNonEmptyLines(logged), "expected exactly one Warn line per call, got: %q", logged)
}
