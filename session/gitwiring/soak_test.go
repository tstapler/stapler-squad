package gitwiring

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gogitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func shadowRefs() backend.CohortMap {
	return backend.CohortMap{}.With(backend.CohortRefs, backend.BackendShadow)
}

func commitFixture(t *testing.T) backend.Local {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogitlib.PlainInit(dir, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("f.txt")
	require.NoError(t, err)
	_, err = wt.Commit("init", &gogitlib.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}})
	require.NoError(t, err)
	return backend.Local{Root: backend.RepoRoot(dir)}
}

// counterDelta reports how much a counter moved while fn ran, summed over matching labels.
func counterDelta(t *testing.T, name string, match map[string]string, fn func()) int64 {
	t.Helper()
	sum := func() int64 {
		var total int64
	points:
		for _, p := range readLive(context.Background()).Counters {
			if p.Name != name {
				continue
			}
			for k, v := range match {
				if p.Labels[k] != v {
					continue points
				}
			}
			total += p.Value
		}
		return total
	}
	before := sum()
	fn()
	return sum() - before
}

func TestShadowRefsEndToEndReturnsCLIAnswerAndCountsWithoutMismatch(t *testing.T) {
	loc := commitFixture(t)
	dir := t.TempDir()
	b := newProduction(shadowRefs(), NewLocalRunner(), dir)
	t.Cleanup(stopSoak)
	cliOnly := NewRouter(backend.CohortMap{}, NewLocalRunner())
	ctx := context.Background()

	wantSHA, err := cliOnly.ResolveRef(ctx, loc, "HEAD")
	require.NoError(t, err)
	wantBranch, err := cliOnly.CurrentBranch(ctx, loc)
	require.NoError(t, err)

	const calls = 5
	goroutines := runtime.NumGoroutine()
	var gotSHA backend.CommitSHA
	var gotBranch backend.BranchName
	shadowCalls := counterDelta(t, backend.MetricShadowCallsTotal, nil, func() {
		for i := 0; i < calls; i++ {
			gotSHA, err = b.ResolveRef(ctx, loc, "HEAD")
			require.NoError(t, err)
			gotBranch, err = b.CurrentBranch(ctx, loc)
			require.NoError(t, err)
		}
	})
	assert.Equal(t, wantSHA, gotSHA)
	assert.Equal(t, wantBranch, gotBranch)
	assert.EqualValues(t, 2*calls, shadowCalls)
	assert.LessOrEqual(t, runtime.NumGoroutine(), goroutines+2, "shadow reads must not leave goroutines behind")

	live := readLive(ctx)
	for _, p := range live.Counters {
		if p.Name == backend.MetricShadowMismatchTotal {
			assert.NotEqual(t, "real", p.Labels["class"], "agreeing repo must not produce a real mismatch: %+v", p)
		}
	}
	var sampled bool
	for _, h := range live.Histograms {
		sampled = sampled || (h.Name == backend.MetricShadowDurationMS && h.Count > 0)
	}
	assert.True(t, sampled, "shadow durations must be recorded for the p50 criterion")

	stopSoak()
	assert.FileExists(t, filepath.Join(dir, soakLedgerFile))
}

func TestShadowSpawnsOnlyTheCLIOncePerCall(t *testing.T) {
	loc := commitFixture(t)
	b := newProduction(shadowRefs(), NewLocalRunner(), t.TempDir())
	t.Cleanup(stopSoak)
	spawns := counterDelta(t, backend.MetricCLISpawnTotal, map[string]string{"operation": string(backend.OpCurrentBranch)}, func() {
		for i := 0; i < 4; i++ {
			_, err := b.CurrentBranch(context.Background(), loc)
			require.NoError(t, err)
		}
	})
	assert.EqualValues(t, 4, spawns, "in-process shadow side must spawn nothing")
}

func TestUnconfiguredRouterSpawnsExactlyAsBeforeAndBuildsNoShadowMachinery(t *testing.T) {
	local := &fakeLocal{}
	dir := t.TempDir()
	discovery := counterDelta(t, backend.MetricCLISpawnTotal, map[string]string{"operation": string(backend.OpGetConfig)}, func() {
		b := newProduction(backend.CohortMap{}, local, dir)
		got, err := b.CurrentBranch(context.Background(), backend.Local{Root: "/r"})
		require.NoError(t, err)
		assert.Equal(t, backend.BranchName("main"), got)
	})
	assert.Len(t, local.calls, 1)
	assert.Zero(t, discovery, "an all-cli install must not run system config discovery")
	assert.Nil(t, soakActive)
	assert.NoFileExists(t, filepath.Join(dir, soakLedgerFile))
	assert.NoDirExists(t, filepath.Join(dir, "shadow-mismatches"))
}

type disagreeingGoGit struct{ backend.Backend }

func (disagreeingGoGit) CurrentBranch(context.Context, backend.RepoLocation) (backend.BranchName, error) {
	return "not-main", nil
}

type recorderSpy struct{ recs []backend.MismatchRecord }

func (r *recorderSpy) Record(rec backend.MismatchRecord) { r.recs = append(r.recs, rec) }

func TestShadowDisagreementNeverChangesTheAnswerButIsRecorded(t *testing.T) {
	local := &fakeLocal{}
	spy := &recorderSpy{}
	b := NewRouter(shadowRefs(), local,
		WithInProcess(disagreeingGoGit{}, unknownSystemConfigProbe{}), WithMismatchRecorder(spy), WithMeter(soakMeter()))
	real := counterDelta(t, backend.MetricShadowMismatchTotal, map[string]string{"class": "real"}, func() {
		got, err := b.CurrentBranch(context.Background(), backend.Local{Root: "/r"})
		require.NoError(t, err)
		assert.Equal(t, backend.BranchName("main"), got, "the CLI answer is authoritative")
	})
	assert.EqualValues(t, 1, real)
	require.Len(t, spy.recs, 1)
	assert.Equal(t, backend.MismatchReal, spy.recs[0].Class)
}

func TestWithoutInProcessEveryShadowCohortRoutesToTheCLI(t *testing.T) {
	local := &fakeLocal{}
	b := NewRouter(shadowRefs(), local)
	shadow := counterDelta(t, backend.MetricShadowCallsTotal, nil, func() {
		_, err := b.CurrentBranch(context.Background(), backend.Local{Root: "/r"})
		require.NoError(t, err)
	})
	assert.Zero(t, shadow)
	assert.Len(t, local.calls, 1)
}

func TestLedgerSurvivesRestartAndIsDiscardedOnLeavingShadow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, soakLedgerFile)
	since := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	st := &soakState{path: path, base: ledger{Since: since, Sample: soakSample{
		Counters: []counterPoint{{Name: backend.MetricShadowCallsTotal, Labels: map[string]string{"operation": "ResolveRef"}, Value: 7}},
	}}}
	st.flush(context.Background())

	loaded := loadLedger(path)
	assert.True(t, since.Equal(loaded.Since))
	require.NotEmpty(t, loaded.Sample.Counters)
	assert.GreaterOrEqual(t, loaded.Sample.Counters[0].Value, int64(7), "the persisted base is kept; live counts of this test process are added on top")

	startSoak(backend.CohortMap{}, dir) // leaving shadow ends the window
	assert.NoFileExists(t, path)
}

func TestMergeSamplesAddsCountersAndHistograms(t *testing.T) {
	l := map[string]string{"operation": "Log"}
	a := soakSample{
		Counters:   []counterPoint{{Name: "c", Labels: l, Value: 2}},
		Histograms: []histPoint{{Name: "h", Labels: l, Bounds: []float64{1}, Buckets: []uint64{1, 0}, Count: 1}},
	}
	b := soakSample{
		Counters:   []counterPoint{{Name: "c", Labels: l, Value: 3}, {Name: "d", Value: 1}},
		Histograms: []histPoint{{Name: "h", Labels: l, Bounds: []float64{1}, Buckets: []uint64{0, 2}, Count: 2}},
	}
	m := mergeSamples(a, b)
	require.Len(t, m.Counters, 2)
	assert.EqualValues(t, 5, m.Counters[0].Value)
	require.Len(t, m.Histograms, 1)
	assert.Equal(t, []uint64{1, 2}, m.Histograms[0].Buckets)
	assert.EqualValues(t, 3, m.Histograms[0].Count)
}

func TestHistogramP50(t *testing.T) {
	h := histPoint{Bounds: []float64{1, 2, 4}, Buckets: []uint64{0, 10, 0, 0}, Count: 10}
	v, ok := h.p50()
	require.True(t, ok)
	assert.InDelta(t, 1.5, v, 1e-9)

	_, ok = histPoint{Bounds: []float64{1}, Buckets: []uint64{0, 0}}.p50()
	assert.False(t, ok)

	v, _ = histPoint{Bounds: []float64{1, 5}, Buckets: []uint64{0, 0, 4}, Count: 4}.p50()
	assert.Equal(t, 5.0, v, "a median past the last bound reports the last bound")
}

func soakFixture(calls, real int64, cliMs, goMs float64, hours float64) (SoakReport, backend.Cohort) {
	cohorts := shadowRefs()
	now := time.Now()
	var counters []counterPoint
	var hists []histPoint
	for _, op := range backend.AllOperations() {
		if op.Mutating() || backend.CohortOf(op) != backend.CohortRefs {
			continue
		}
		l := map[string]string{"operation": string(op)}
		counters = append(counters, counterPoint{Name: backend.MetricShadowCallsTotal, Labels: l, Value: calls})
		if real > 0 {
			counters = append(counters, counterPoint{Name: backend.MetricShadowMismatchTotal, Labels: map[string]string{"operation": string(op), "class": "real"}, Value: real})
		}
		for impl, ms := range map[string]float64{"cli": cliMs, "gogit": goMs} {
			buckets := make([]uint64, len(backend.ShadowDurationBucketsMS)+1)
			for i, bound := range backend.ShadowDurationBucketsMS {
				if ms <= bound {
					buckets[i] = uint64(calls)
					break
				}
			}
			hists = append(hists, histPoint{Name: backend.MetricShadowDurationMS, Labels: map[string]string{"operation": string(op), "implementation": impl},
				Bounds: backend.ShadowDurationBucketsMS, Buckets: buckets, Count: uint64(calls)})
		}
	}
	l := ledger{Since: now.Add(-time.Duration(hours * float64(time.Hour))), Sample: soakSample{Counters: counters, Histograms: hists}}
	return buildReport(now, cohorts, l, true), backend.CohortRefs
}

func TestVerdictReadyOnlyWhenEveryCriterionHolds(t *testing.T) {
	rep, _ := soakFixture(600, 0, 10, 0.5, 8*24)
	assert.True(t, rep.Verdict["refs"].Ready, "%v", rep.Verdict["refs"].Blockers)
	assert.Contains(t, rep.Text(), "criteria met")
}

func TestVerdictBlockers(t *testing.T) {
	tests := []struct {
		name               string
		calls, real        int64
		cliMs, goMs, hours float64
		want               string
	}{
		{"window too short", 600, 0, 10, 0.5, 24, "window is"},
		{"too few calls", 499, 0, 10, 0.5, 200, "need 500"},
		{"real mismatch", 600, 1, 10, 0.5, 200, "real mismatch"},
		{"gogit slower", 600, 0, 0.5, 10, 200, "slower than cli"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep, _ := soakFixture(tc.calls, tc.real, tc.cliMs, tc.goMs, tc.hours)
			v := rep.Verdict["refs"]
			assert.False(t, v.Ready)
			assert.Contains(t, joinAll(v.Blockers), tc.want)
		})
	}
}

func joinAll(ss []string) string {
	out := ""
	for _, s := range ss {
		out += s + "\n"
	}
	return out
}

func TestUncalledOperationHoldsTheWindowOpen(t *testing.T) {
	rep := buildReport(time.Now(), shadowRefs(), ledger{Since: time.Now().Add(-30 * 24 * time.Hour)}, true)
	assert.False(t, rep.Verdict["refs"].Ready)
	assert.Contains(t, joinAll(rep.Verdict["refs"].Blockers), "ResolveRef: 0 shadow calls")
}

func TestCapabilityDetectErrorRatioBlocksAtTheCeiling(t *testing.T) {
	rep, _ := soakFixture(600, 0, 10, 0.5, 200)
	l := ledger{Since: time.Now().Add(-200 * time.Hour), Sample: soakSample{Counters: []counterPoint{
		{Name: backend.MetricCapabilityDetectTotal, Labels: map[string]string{"outcome": "ok"}, Value: 98},
		{Name: backend.MetricCapabilityDetectTotal, Labels: map[string]string{"outcome": "detect_error", "sub_reason": "parse_error"}, Value: 2},
	}}}
	l.Sample = mergeSamples(l.Sample, soakSample{})
	got := buildReport(time.Now(), shadowRefs(), l, true)
	assert.EqualValues(t, 2, got.Capability.SubReasons["parse_error"])
	assert.InDelta(t, 0.02, got.Capability.DetectErrorRatio, 1e-9)
	assert.Contains(t, joinAll(got.Verdict["refs"].Blockers), "detect_error ratio")
	assert.NotNil(t, rep)
}

func TestReportWithNoShadowCohortSaysNothingIsSoaking(t *testing.T) {
	rep := buildReport(time.Now(), backend.CohortMap{}, ledger{}, false)
	assert.Contains(t, rep.Text(), "nothing is being soaked")
	assert.Empty(t, rep.Verdict)
}

// Differential check against real git: across the simple refs reads on a fresh repository, the
// in-process side must agree with the CLI (zero real mismatches), whatever it is asked.
func TestShadowRefsReadsAgreeWithRealGit(t *testing.T) {
	loc := commitFixture(t)
	b := newProduction(shadowRefs(), NewLocalRunner(), t.TempDir())
	t.Cleanup(stopSoak)
	ctx := context.Background()
	real := counterDelta(t, backend.MetricShadowMismatchTotal, map[string]string{"class": "real"}, func() {
		_, _ = b.CurrentBranch(ctx, loc)
		_, _ = b.HeadRef(ctx, loc)
		_, _ = b.ResolveRef(ctx, loc, "HEAD")
		_, _ = b.ResolveRef(ctx, loc, "refs/heads/does-not-exist")
		_, _ = b.RefExists(ctx, loc, "HEAD")
		_, _ = b.RefExists(ctx, loc, "refs/heads/does-not-exist")
		_, _ = b.GitDir(ctx, loc)
		_, _ = b.CommonDir(ctx, loc)
		_, _ = b.RepoRoot(ctx, loc)
	})
	assert.Zero(t, real, "go-git disagreed with git on a plain repository")
}

func TestNewProductionDiscardsStaleLedgerWhenNothingIsShadowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, soakLedgerFile)
	require.NoError(t, os.WriteFile(path, []byte(`{"since":"2026-01-01T00:00:00Z"}`), 0o600))
	newProduction(backend.CohortMap{}, &fakeLocal{}, dir)
	assert.NoFileExists(t, path)
}

func TestLedgerForADifferentShadowedSetIsNotReused(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-30 * 24 * time.Hour).UTC()
	st := &soakState{path: filepath.Join(dir, soakLedgerFile), base: ledger{Since: old, Shadowed: []string{"diffstatus"}}}
	st.flush(context.Background())

	startSoak(shadowRefs(), dir)
	t.Cleanup(stopSoak)
	soakMu.Lock()
	got := soakActive.base
	soakMu.Unlock()
	assert.Equal(t, []string{"refs"}, got.Shadowed)
	assert.WithinDuration(t, time.Now(), got.Since, time.Minute, "a new cohort mix starts a new window")
}
