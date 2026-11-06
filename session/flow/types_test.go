package flow

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

var fixedNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func TestFlowWindow_should_BuildThirtyDayWindowAndRejectZeroDays_When_Constructed(t *testing.T) {
	t.Parallel()
	clock := FixedClock{T: fixedNow}

	w, err := NewFlowWindow(clock, 30)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), w.Start())
	assert.Equal(t, fixedNow, w.End())

	_, err = NewFlowWindow(clock, 0)
	assert.Error(t, err)
	_, err = NewFlowWindow(clock, -3)
	assert.Error(t, err)
}

func TestFlowWindow_should_NormalizeNonUTCClockToUTC_When_Constructed(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("x", -7*3600)
	w, err := NewFlowWindow(FixedClock{T: fixedNow.In(loc)}, 1)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, w.End().Location())
	assert.Equal(t, fixedNow, w.End())
}

func TestFlowWindow_should_BuildAllTimeWindowAndRejectZeroDays_When_AllTimeRequested(t *testing.T) {
	t.Parallel()
	clock := FixedClock{T: fixedNow}

	w, err := NewAllTimeWindow(clock)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), w.Start())
	assert.Equal(t, fixedNow, w.End())

	_, err = NewFlowWindow(clock, 0)
	assert.Error(t, err)

	allKey := WindowSpec{AllTime: true}.CacheKey()
	for _, days := range []int{1, 7, 30, 90, 365} {
		assert.NotEqual(t, allKey, WindowSpec{Days: days}.CacheKey(), "days=%d", days)
	}
}

func TestFlowWindow_should_RejectNonPositiveInterval_When_BoundsGiven(t *testing.T) {
	t.Parallel()
	_, err := NewFlowWindowBounds(fixedNow, fixedNow)
	assert.Error(t, err)
	_, err = NewFlowWindowBounds(fixedNow, fixedNow.Add(-time.Hour))
	assert.Error(t, err)
	w, err := NewFlowWindowBounds(fixedNow.Add(-time.Hour), fixedNow)
	require.NoError(t, err)
	assert.True(t, w.Contains(fixedNow.Add(-time.Hour)), "start is inclusive")
	assert.False(t, w.Contains(fixedNow), "end is exclusive")
}

func TestWindowSpec_should_BuildClockFreeCacheKey_When_OffsetsDiffer(t *testing.T) {
	t.Parallel()
	a := WindowSpec{Days: 30}.CacheKey()
	b := WindowSpec{Days: 30, OffsetDays: 30}.CacheKey()
	assert.NotEqual(t, a, b)
	assert.Equal(t, a, WindowSpec{Days: 30}.CacheKey())
}

func TestWindowSpec_should_ResolveWindows_When_AllTimeOffsetOrPlain(t *testing.T) {
	t.Parallel()
	clock := FixedClock{T: fixedNow}

	prev, err := WindowSpec{Days: 30, OffsetDays: 30}.Window(clock)
	require.NoError(t, err)
	assert.Equal(t, fixedNow.AddDate(0, 0, -60), prev.Start())
	assert.Equal(t, fixedNow.AddDate(0, 0, -30), prev.End())

	all, err := WindowSpec{AllTime: true}.Window(clock)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), all.Start())

	_, err = WindowSpec{}.Window(clock)
	assert.Error(t, err)
}

func TestDefaultConfig_should_HoldPlanDefaultsAndDerivePad_When_ThrashGapChanges(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.Equal(t, 15*time.Minute, cfg.ThrashGap)
	assert.Equal(t, 6, cfg.ThrashMinFlips)
	assert.Equal(t, time.Hour, cfg.ThrashMergeGap)
	assert.Equal(t, 3, cfg.DeadlockMinRun)
	assert.Equal(t, 3, cfg.DeadlockRepeatCriterion)
	assert.Equal(t, 5, cfg.LowSampleN)
	assert.Equal(t, time.Hour, cfg.StalledAfter)
	assert.Equal(t, 2*time.Hour+30*time.Minute, cfg.CollapsePad())

	cfg.ThrashGap = 30 * time.Minute
	assert.Equal(t, 4*time.Hour, cfg.CollapsePad())
}

func TestSpan_should_RejectIllegalCombinations_When_CountBelowMinFlipsOrFirstAfterLast(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	first := fixedNow.Add(-time.Hour)
	ok := ThrashSpanParams{
		Item:      "i1",
		Pair:      FlipPair{A: domain.BacklogStatusReady, B: domain.BacklogStatusInProgress},
		Count:     6,
		First:     first,
		Last:      first.Add(10 * time.Minute),
		NetStatus: domain.BacklogStatusReady,
		Cause:     CauseDequeueSpawnFailed,
	}

	s, err := NewThrashSpan(cfg, ok)
	require.NoError(t, err)
	assert.Equal(t, 6, s.Count())
	assert.Equal(t, CauseDequeueSpawnFailed, s.Cause())

	low := ok
	low.Count = 5
	_, err = NewThrashSpan(cfg, low)
	assert.Error(t, err, "count below ThrashMinFlips")

	rev := ok
	rev.First, rev.Last = ok.Last, ok.First
	_, err = NewThrashSpan(cfg, rev)
	assert.Error(t, err, "first after last")

	noItem := ok
	noItem.Item = ""
	_, err = NewThrashSpan(cfg, noItem)
	assert.Error(t, err, "empty item")
}

func TestRealSpan_should_RejectEmptyItemOrStatus_When_Constructed(t *testing.T) {
	t.Parallel()
	_, err := NewRealSpan("", domain.BacklogStatusReady, domain.BacklogStatusQueued, fixedNow)
	assert.Error(t, err)
	_, err = NewRealSpan("i", "", domain.BacklogStatusQueued, fixedNow)
	assert.Error(t, err)
	_, err = NewRealSpan("i", domain.BacklogStatusReady, domain.BacklogStatusQueued, time.Time{})
	assert.Error(t, err)

	s, err := NewRealSpan("i", domain.BacklogStatusReady, domain.BacklogStatusQueued, fixedNow.In(time.FixedZone("x", 3600)))
	require.NoError(t, err)
	assert.Equal(t, fixedNow, s.At())
	assert.Equal(t, time.UTC, s.At().Location())
}

// TestSpanImplementationsHandled enumerates every Span implementation; adding a
// third implementation without extending SpanImplementations (and so this
// switch) fails here.
func TestSpanImplementationsHandled(t *testing.T) {
	t.Parallel()
	impls := SpanImplementations(fixedNow)
	require.Len(t, impls, 2)
	var real, thrash int
	for _, s := range impls {
		switch s.(type) {
		case RealSpan:
			real++
		case ThrashSpan:
			thrash++
		default:
			t.Fatalf("unhandled Span implementation %T", s)
		}
		assert.NotEmpty(t, s.Item())
	}
	assert.Equal(t, 1, real)
	assert.Equal(t, 1, thrash)
}

func TestCause_should_NameEveryValue_When_Stringified(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, c := range AllCauses() {
		name := c.String()
		assert.NotEmpty(t, name)
		assert.False(t, seen[name], "duplicate %q", name)
		seen[name] = true
	}
	assert.Len(t, seen, 5)
	assert.Equal(t, "unknown", Cause(99).String())
}

func TestSystemClock_should_ReturnUTC_When_Called(t *testing.T) {
	t.Parallel()
	assert.Equal(t, time.UTC, SystemClock{}.Now().Location())
}

func TestFlowImports_should_ExcludeEntSessionServerProto_When_GoListDepsScanned(t *testing.T) {
	t.Parallel()
	//nolint:norawexec short-lived `go list` in a test; no long-running process to reap.
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	require.NoError(t, err, string(out))
	forbidden := []string{
		"entgo.io/",
		"connectrpc.com/",
		"github.com/tstapler/stapler-squad/server",
		"github.com/tstapler/stapler-squad/session/ent",
		"github.com/tstapler/stapler-squad/proto",
	}
	for _, dep := range strings.Fields(string(out)) {
		assert.NotEqual(t, "github.com/tstapler/stapler-squad/session", dep, "flow must not import session")
		for _, bad := range forbidden {
			assert.False(t, strings.HasPrefix(dep, bad), "forbidden transitive dependency %s", dep)
		}
	}
}
