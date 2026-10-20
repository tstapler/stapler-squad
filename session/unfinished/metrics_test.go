package unfinished

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// TestRegisterMetrics_NoError is a smoke check that instrument registration
// against the OTel global (no-op, in tests) meter succeeds — the one thing
// that would break silently if an instrument name were invalid or an
// observable gauge were registered with a description/unit OTel rejects.
func TestRegisterMetrics_NoError(t *testing.T) {
	t.Parallel()
	if err := RegisterMetrics(); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

// The worker must count one cycle per task, including a task whose repo has
// no scannable worktrees (the common idle case): the counter is a denominator
// for "scanner CPU per scan", so skipped-empty cycles still count.
func TestScannerWorker_ShouldCountOneCyclePerTask(t *testing.T) {
	// Not parallel: asserts a delta on a process-global counter other
	// scanner tests also bump, so it checks ">=" rather than "==".
	s := NewScannerWithReader(nil, nil, &GoGitVCSReader{})
	repo := t.TempDir()
	s.inFlight.Store(repo, struct{}{})
	s.scanQueue <- scanTask{repoPath: repo}
	close(s.scanQueue) // worker drains the one task, then returns: no goroutine, no sleep

	before := ScanCyclesTotal()
	s.worker(context.Background())

	if got := ScanCyclesTotal(); got < before+1 {
		t.Fatalf("ScanCyclesTotal = %d, want >= %d after one worker task", got, before+1)
	}
	if _, stillInFlight := s.inFlight.Load(repo); stillInFlight {
		t.Fatal("worker left the repo marked in-flight")
	}
}

// ---------------------------------------------------------------------------
// Story 5.1.1: unfinished.repostate.* series
// ---------------------------------------------------------------------------

// collectSeries registers the repostate catalog on a fresh SDK meter fed by
// collect and returns every observed value keyed "name" or "name{attr}".
func collectSeries(t *testing.T, collect func() repostateInputs) map[string]int64 {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	require.NoError(t, registerRepostateMetrics(mp.Meter("test"), collect))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			name := strings.TrimPrefix(m.Name, repostateMetricPrefix)
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range d.DataPoints {
					out[seriesKey(name, dp.Attributes)] = dp.Value
				}
			case metricdata.Gauge[int64]:
				for _, dp := range d.DataPoints {
					out[seriesKey(name, dp.Attributes)] = dp.Value
				}
			}
		}
	}
	return out
}

func seriesKey(name string, attrs attribute.Set) string {
	if attrs.Len() == 0 {
		return name
	}
	iter := attrs.Iter()
	iter.Next()
	return fmt.Sprintf("%s{%s}", name, iter.Attribute().Value.AsString())
}

// expectedSeriesKeys lists every series key the catalog must export.
func expectedSeriesKeys() []string {
	var keys []string
	for _, spec := range repostateCatalog {
		switch spec.attr {
		case "":
			keys = append(keys, spec.name)
		case "cache":
			for _, c := range repostateCaches {
				keys = append(keys, fmt.Sprintf("%s{%s}", spec.name, c))
			}
		case "field":
			for f := range numParityFields {
				keys = append(keys, fmt.Sprintf("%s{%s}", spec.name, f))
			}
		case "kind":
			keys = append(keys, spec.name+"{scratch}", spec.name+"{incremental}")
		case "impl":
			keys = append(keys, spec.name+"{stock}", spec.name+"{fork}")
		}
	}
	return keys
}

func TestMetrics_ShouldExportAllRepostateSeriesAsZero_WhenFlagOffOrRepoStateUnallocated(t *testing.T) {
	t.Parallel()
	// A flags-off reader with no RepoState, and no scanner: nothing is allocated.
	reader := &GoGitVCSReader{}
	reader.scanSkip.flags = &Flags{}
	collect := func() repostateInputs {
		in := newRepostateInputs()
		in.repoStates, in.repoBytes = reader.repoStateTotals()
		in.skip = reader.ScanSkipStats()
		return in
	}

	series := collectSeries(t, collect)

	keys := expectedSeriesKeys()
	require.NotEmpty(t, keys)
	for _, k := range keys {
		v, ok := series[k]
		assert.True(t, ok, "series %s is absent; an off cache must report 0, not disappear", k)
		if k == "commit_parents_reader{stock}" {
			assert.EqualValues(t, 1, v)
			continue
		}
		assert.Zero(t, v, "series %s", k)
	}
	assert.Len(t, series, len(keys), "no series beyond the catalog")
}

func TestMetrics_ShouldExposeFillLostCasDecodeReprobeRollbackAndCasRetries(t *testing.T) {
	t.Parallel()
	in := newRepostateInputs()
	in.graph = repostate.GraphStats{FillLostCAS: 1, FillLostDecode: 2, Reprobe: 3, Rollback: 4, CASRetries: 5, GraphFull: 6, FullRebuild: 14}
	in.caches["graph"] = repostate.CacheStats{Hits: 7, Misses: 8, Evictions: 9, Coalesced: 10, Entries: 11, Bytes: 12}
	in.parity.Mismatch[parityShortstat] = 13

	series := collectSeries(t, func() repostateInputs { return in })

	want := map[string]int64{
		"fill_lost_cas_total":              1,
		"fill_lost_decode_total":           2,
		"reprobe_total":                    3,
		"fill_rollback_total":              4,
		"cas_retries_total":                5,
		"graph_full_fallbacks_total":       6,
		"graph_full_rebuilds_total":        14,
		"cache_hits_total{graph}":          7,
		"cache_misses_total{graph}":        8,
		"cache_evictions_total{graph}":     9,
		"cache_coalesced_total{graph}":     10,
		"cache_entries{graph}":             11,
		"cache_bytes{graph}":               12,
		"parity_mismatch_total{shortstat}": 13,
	}
	for k, v := range want {
		assert.EqualValues(t, v, series[k], k)
	}
	assert.Zero(t, series["cache_hits_total{reach}"], "a cache with no data reads 0")
}

func TestMetrics_ShouldRecordScanCycleSeconds_WhenWorkerTaskCompletes(t *testing.T) {
	// Not parallel: scanCycleSeconds is a process-global instrument pointer.
	prev := scanCycleSeconds.Load()
	t.Cleanup(func() { scanCycleSeconds.Store(prev) })
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	require.NoError(t, registerRepostateMetrics(mp.Meter("test"), newRepostateInputs))

	before, _ := ScanCycleDurations()
	recordScanCycleDuration(1500 * time.Millisecond)
	after, total := ScanCycleDurations()
	require.Equal(t, before+1, after)
	require.GreaterOrEqual(t, total, 1500*time.Millisecond)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	var count uint64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != repostateMetricPrefix+"scan_cycle_seconds" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			for _, dp := range h.DataPoints {
				count += dp.Count
			}
		}
	}
	assert.EqualValues(t, 1, count, "the histogram saw the recorded cycle")
}

func TestScanCycleLog_ShouldEmitOneDebugLineAndNothingAtDefaultLevel(t *testing.T) {
	t.Parallel()
	run := func(debug bool) []string {
		f := newSigFake("/a", "/b")
		s := NewScannerWithReader(nil, nil, f)
		var lines []string
		s.debugLog = func(msg string, args ...any) {
			line := msg
			for i := 0; i+1 < len(args); i += 2 {
				line += fmt.Sprintf(" %v=%v", args[i], args[i+1])
			}
			lines = append(lines, line)
		}
		s.debugOn = func() bool { return debug }
		scanFake(s)
		return lines
	}

	assert.Empty(t, run(false), "default level: the scan loop logs nothing")
	lines := run(true)
	require.Len(t, lines, 1)
	for _, key := range []string{"repo=/repo", "worktrees=2", "duration=", "decodes=", "graph_rows_filled="} {
		assert.Contains(t, lines[0], key)
	}
}
