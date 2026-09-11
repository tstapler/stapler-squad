package session

// liveness_cache_test.go — unit/integration tests for livenessCache and
// CachingLivenessEngine (Epic 1.3, Story 1.3.1 of
// project_plans/backlog-custom-workflow-stages). Test names are taken
// verbatim from implementation/validation.md's "Story 1.3.1" rows.

import (
	"bytes"
	stdlog "log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tslog "github.com/tstapler/stapler-squad/log"
)

// TestLivenessCache_should_ReturnLockFreeMiss_When_NoRowMatchesStageAndMode
// covers Story 1.3.1's happy path: Get("idea","sdd") on an empty (never
// loaded) cache returns a miss without touching writeMu — proven here by
// never acquiring writeMu at all (an empty livenessCache{} has never had
// Load/Invalidate called on it, so writeMu is provably untouched;
// Get itself is documented to never touch it regardless).
func TestLivenessCache_should_ReturnLockFreeMiss_When_NoRowMatchesStageAndMode(t *testing.T) {
	c := &livenessCache{}

	rd, ok := c.Get("idea", PipelineMode("sdd"))

	assert.False(t, ok)
	assert.Equal(t, resolvedLivenessDefinition{}, rd)
}

// TestCachingLivenessEngine_should_FallBackToModeLessRowWithoutWarnLog_When_StageModeRowAbsentButStageOnlyRowExists
// covers Story 1.3.1's error/fallback path: given a cache containing only
// ("idea", nil) with ExpectedDuration=40m, LivenessFor(BacklogStatusIdea,
// "sdd") returns the ("idea", nil) row's definition (40m), not
// DefaultLivenessEngine's 30m default, and logs no Warn — a mode-less
// fallback is not a failure.
func TestCachingLivenessEngine_should_FallBackToModeLessRowWithoutWarnLog_When_StageModeRowAbsentButStageOnlyRowExists(t *testing.T) {
	def, err := NewLivenessDefinition(LivenessKindDurationBudget,
		WithExpectedDuration(40*time.Minute),
		WithStalenessMargin(5*time.Minute),
	)
	require.NoError(t, err)

	cache := &livenessCache{}
	seeded := map[string]resolvedLivenessDefinition{
		livenessCacheKey("idea", PipelineModeDefault): {LivenessDefinition: *def},
	}
	cache.ptr.Store(&seeded)

	engine := &CachingLivenessEngine{
		repo:            nil, // never touched: cache is pre-seeded, no Load/Invalidate call in this test
		cache:           cache,
		embeddedDefault: NewDefaultLivenessEngine(),
	}

	var buf bytes.Buffer
	orig := tslog.SetWarningLogForTest(stdlog.New(&buf, "WARNING: ", 0))
	t.Cleanup(func() { tslog.SetWarningLogForTest(orig) })

	got, err := engine.LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))

	require.NoError(t, err)
	assert.Equal(t, LivenessKindDurationBudget, got.Kind)
	assert.Equal(t, 40*time.Minute, got.ExpectedDuration)
	assert.Equal(t, 45*time.Minute, got.StalenessThreshold())
	assert.NotEqual(t, defaultTriageExpectedDuration, got.ExpectedDuration,
		"must resolve the mode-less override, not DefaultLivenessEngine's 30m built-in")
	assert.Empty(t, buf.String(), "a mode-less fallback must not log a Warn line")
}

// TestCachingLivenessEngine_should_FallBackToDefaultEngineWithWarnLog_When_NeitherStageModeNorStageOnlyRowExists
// (Integration, per validation.md) covers Story 1.3.1's full-fallback path
// against a real ent/DB-backed cache load: with the stage_liveness_definitions
// table empty, LivenessFor(BacklogStatusIdea, "sdd") falls back to
// DefaultLivenessEngine's built-in idea-stage definition and logs exactly one
// [LivenessEngine] Warn line naming the unresolved stage/mode.
func TestCachingLivenessEngine_should_FallBackToDefaultEngineWithWarnLog_When_NeitherStageModeNorStageOnlyRowExists(t *testing.T) {
	repo := NewTestEntRepository(t)
	livenessRepo := NewEntLivenessRepository(repo.GetEntClient())

	engine, err := NewCachingLivenessEngine(livenessRepo)
	require.NoError(t, err)

	var buf bytes.Buffer
	orig := tslog.SetWarningLogForTest(stdlog.New(&buf, "WARNING: ", 0))
	t.Cleanup(func() { tslog.SetWarningLogForTest(orig) })

	got, err := engine.LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))

	require.NoError(t, err)
	want, wantErr := NewDefaultLivenessEngine().LivenessFor(BacklogStatusIdea, PipelineMode("sdd"))
	require.NoError(t, wantErr)
	assert.Equal(t, want, got, "expected the identical DefaultLivenessEngine built-in definition")

	logged := buf.String()
	lines := 0
	for _, line := range bytes.Split(bytes.TrimRight([]byte(logged), "\n"), []byte("\n")) {
		if len(line) > 0 {
			lines++
		}
	}
	assert.Equal(t, 1, lines, "expected exactly one Warn line, got log output: %q", logged)
	assert.Contains(t, logged, "[LivenessEngine]")
	assert.Contains(t, logged, "idea")
	assert.Contains(t, logged, "sdd")
}
