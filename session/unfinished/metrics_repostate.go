package unfinished

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// The unfinished.repostate.* series (Observability Plan; Prometheus spells the
// prefix unfinished_repostate_). Series are per cache, never per repo: a repo
// label would be unbounded cardinality. A cache whose flag is off, or whose
// RepoState is not allocated, reports 0 rather than disappearing, so soak
// comparisons have no gaps.

const repostateMetricPrefix = "unfinished.repostate."

// repostateCaches is the fixed cache label set. Caches whose stories have not
// landed (reach, aheadbehind, behind, messages, trees, matcherparse) report 0.
var repostateCaches = []string{"graph", "reach", "aheadbehind", "behind", "messages", "trees", "diffmemo", "matcherparse"}

// repostateInputs is one read of every source the series come from.
type repostateInputs struct {
	caches       map[string]repostate.CacheStats
	graph        repostate.GraphStats
	repoStates   int64
	repoBytes    int64
	graphCommits int64
	graphEpoch   int64
	decoded      int64
	scan         ScanStateStats
	skip         ScanSkipStats
	parity       ParityStats
	tierBArmed   int64
	internerSize int64
}

func newRepostateInputs() repostateInputs {
	in := repostateInputs{caches: make(map[string]repostate.CacheStats, len(repostateCaches))}
	for _, c := range repostateCaches {
		in.caches[c] = repostate.CacheStats{}
	}
	return in
}

// graphProvider and graphCacheStatser are satisfied by RepoState and Graph once
// they expose their graph; until then every graph series reads 0.
type (
	graphProvider     interface{ Graph() *repostate.Graph }
	graphCacheStatser interface{ CacheStats() repostate.CacheStats }
	graphEpocher      interface{ Load() *repostate.Generation }
)

// collectRepostateInputs reads the live reader and scanner; with neither (or
// with shared_commit_state off) the result is all zero.
func collectRepostateInputs() repostateInputs {
	in := newRepostateInputs()
	if r := currentReader.Load(); r != nil {
		in.repoStates, in.repoBytes = r.repoStateTotals()
		r.forEachRepoState(func(rs *repostate.RepoState) { in.addGraph(rs) })
		in.skip = r.ScanSkipStats()
	}
	m := DiffMemoStats()
	in.caches["diffmemo"] = repostate.CacheStats{Hits: m.Hits, Misses: m.Misses, Evictions: m.Evictions}
	in.decoded = DecodedCommitsTotal()
	if s := currentScanner.Load(); s != nil {
		in.scan = s.ScanStateStats()
		in.parity = s.ParityStats()
	}
	return in
}

func (in *repostateInputs) addGraph(rs *repostate.RepoState) {
	gp, ok := any(rs).(graphProvider)
	if !ok {
		return
	}
	g := gp.Graph()
	if g == nil {
		return
	}
	st := g.Stats()
	in.graph.FillLostCAS += st.FillLostCAS
	in.graph.FillLostDecode += st.FillLostDecode
	in.graph.Reprobe += st.Reprobe
	in.graph.Rollback += st.Rollback
	in.graph.CASRetries += st.CASRetries
	in.graph.GraphFull += st.GraphFull
	if cs, ok := any(g).(graphCacheStatser); ok {
		c := cs.CacheStats()
		cur := in.caches["graph"]
		cur.Hits += c.Hits
		cur.Misses += c.Misses
		cur.Evictions += c.Evictions
		cur.Entries += c.Entries
		cur.Bytes += c.Bytes
		in.caches["graph"] = cur
		in.graphCommits += int64(c.Entries)
	}
	if ge, ok := any(g).(graphEpocher); ok {
		if gen := ge.Load(); gen != nil {
			in.graphEpoch = max(in.graphEpoch, int64(gen.Epoch())) //nolint:gosec // epoch counts bumps, nowhere near int64
		}
	}
}

// decodedCommitsTotal counts full commit decodes in production paths.
var decodedCommitsTotal atomic.Int64

// RecordDecodedCommits adds n commit decodes to unfinished.repostate.decoded_commits_total.
func RecordDecodedCommits(n int64) { decodedCommitsTotal.Add(n) }

// DecodedCommitsTotal returns the commit-decode count since process start.
func DecodedCommitsTotal() int64 { return decodedCommitsTotal.Load() }

// cycleCounters are the two process-wide counters the scan-cycle debug line
// reports as per-Burst deltas.
func cycleCounters() (decodes, graphRows int64) {
	decodes = DecodedCommitsTotal()
	if r := currentReader.Load(); r != nil {
		in := newRepostateInputs()
		r.forEachRepoState(func(rs *repostate.RepoState) { in.addGraph(rs) })
		graphRows = in.caches["graph"].Misses
	}
	return decodes, graphRows
}

// scanCycleSeconds is the scan_cycle_seconds histogram; durations recorded
// before RegisterMetrics are kept only in the cumulative counters below.
var (
	scanCycleSeconds atomic.Pointer[metric.Float64Histogram]
	scanCycleCount   atomic.Int64
	scanCycleSumNs   atomic.Int64
)

func recordScanCycleDuration(d time.Duration) {
	scanCycleCount.Add(1)
	scanCycleSumNs.Add(int64(d))
	if h := scanCycleSeconds.Load(); h != nil {
		(*h).Record(context.Background(), d.Seconds())
	}
}

// ScanCycleDurations returns the number of recorded scan cycles and their summed duration.
func ScanCycleDurations() (count int64, total time.Duration) {
	return scanCycleCount.Load(), time.Duration(scanCycleSumNs.Load())
}

// seriesKind distinguishes monotonic counters from gauges.
type seriesKind uint8

const (
	kindCounter seriesKind = iota
	kindGauge
)

// seriesSpec is one catalog entry; attr names its single attribute ("" none).
type seriesSpec struct {
	name, desc, attr string
	kind             seriesKind
}

// repostateCatalog lists every series by instrument; points() must produce a
// value for each one on every read.
var repostateCatalog = []seriesSpec{
	{"cache_hits_total", "Cache hits per repostate cache", "cache", kindCounter},
	{"cache_misses_total", "Cache misses per repostate cache", "cache", kindCounter},
	{"cache_evictions_total", "Cache evictions per repostate cache", "cache", kindCounter},
	{"cache_coalesced_total", "Callers that shared another caller's computation, per cache", "cache", kindCounter},
	{"cache_bytes", "Bytes held per repostate cache", "cache", kindGauge},
	{"cache_entries", "Entries held per repostate cache", "cache", kindGauge},
	{"graph_full_fallbacks_total", "Walks that fell back to the plain walk because the graph arena was full", "", kindCounter},
	{"fill_lost_cas_total", "Graph row fills that lost the CAS race", "", kindCounter},
	{"fill_lost_decode_total", "Graph row fills whose decode was discarded after losing", "", kindCounter},
	{"reprobe_total", "Truncated graph rows re-probed after a ShallowStamp change", "", kindCounter},
	{"fill_rollback_total", "Graph row fills rolled back", "", kindCounter},
	{"cas_retries_total", "CAS retries in the lock-free graph arena", "", kindCounter},
	{"decoded_commits_total", "Full commit decodes", "", kindCounter},
	{"graph_epoch", "Largest GraphEpoch across live repos", "", kindGauge},
	{"graph_commits", "Commits interned across live graphs", "", kindGauge},
	{"repostates", "Live RepoState count", "", kindGauge},
	{"repostate_bytes", "Summed RepoState bytes", "", kindGauge},
	{"path_interner_bytes", "Path interner bytes (interner not built: always 0)", "", kindGauge},
	{"dirty_tierb_armed", "Tier B watchers armed (Tier B is not built: always 0)", "", kindGauge},
	{"refsnapshot_reused_across_bursts_total", "RefSnapshots that outlived their Burst (must stay 0)", "", kindCounter},
	{"scan_signature_checks_total", "Committed-state lookups that had a ScanSignature", "", kindCounter},
	{"scan_signature_skips_total", "Lookups served from the stored committed state (skip engagement)", "", kindCounter},
	{"scanskip_index_hits_total", "HasUncommitted served the parsed index from cache", "", kindCounter},
	{"scanskip_index_misses_total", "HasUncommitted decoded the index", "", kindCounter},
	{"scanskip_dirs_skipped_total", "Untracked walk directories served from the verdict cache", "", kindCounter},
	{"scanskip_dirs_walked_total", "Untracked walk directories read from disk", "", kindCounter},
	{"shadow_verify_sampled_total", "Scans the shadow-verify sampler ran an oracle for", "", kindCounter},
	{"shadow_verify_skipped_pressure_total", "Sampled scans skipped under severe memory pressure", "", kindCounter},
	{"shadow_verify_oracle_errors_total", "Oracle reads that failed (comparisons that did not happen)", "", kindCounter},
	{"parity_mismatch_total", "Stable disagreements between the new path and the legacy oracle", "field", kindCounter},
	{"parity_transient_total", "Disagreements discarded because the oracle disagreed with itself", "field", kindCounter},
	{"commit_parents_reader", "Active CommitParentsReader implementation (1 for the active impl)", "impl", kindGauge},
}

// seriesPoint is one observed value of a catalog series.
type seriesPoint struct {
	name, attrVal string
	value         int64
}

// points flattens in into one point per catalog series (and attribute value).
func (in repostateInputs) points() []seriesPoint {
	var pts []seriesPoint
	add := func(name, attr string, v int64) { pts = append(pts, seriesPoint{name, attr, v}) }
	for _, c := range repostateCaches {
		cs := in.caches[c]
		add("cache_hits_total", c, cs.Hits)
		add("cache_misses_total", c, cs.Misses)
		add("cache_evictions_total", c, cs.Evictions)
		add("cache_coalesced_total", c, cs.Coalesced)
		add("cache_bytes", c, cs.Bytes)
		add("cache_entries", c, int64(cs.Entries))
	}
	g := in.graph
	add("graph_full_fallbacks_total", "", int64(g.GraphFull))  //nolint:gosec // counters never approach int64
	add("fill_lost_cas_total", "", int64(g.FillLostCAS))       //nolint:gosec // counters never approach int64
	add("fill_lost_decode_total", "", int64(g.FillLostDecode)) //nolint:gosec // counters never approach int64
	add("reprobe_total", "", int64(g.Reprobe))                 //nolint:gosec // counters never approach int64
	add("fill_rollback_total", "", int64(g.Rollback))          //nolint:gosec // counters never approach int64
	add("cas_retries_total", "", int64(g.CASRetries))          //nolint:gosec // counters never approach int64
	add("decoded_commits_total", "", in.decoded)
	add("graph_epoch", "", in.graphEpoch)
	add("graph_commits", "", in.graphCommits)
	add("repostates", "", in.repoStates)
	add("repostate_bytes", "", in.repoBytes)
	add("path_interner_bytes", "", in.internerSize)
	add("dirty_tierb_armed", "", in.tierBArmed)
	add("refsnapshot_reused_across_bursts_total", "", in.scan.ReusedAcrossBursts)
	add("scan_signature_checks_total", "", in.scan.SignatureChecks)
	add("scan_signature_skips_total", "", in.scan.SignatureSkips)
	add("scanskip_index_hits_total", "", in.skip.IndexHits)
	add("scanskip_index_misses_total", "", in.skip.IndexMisses)
	add("scanskip_dirs_skipped_total", "", in.skip.DirsSkipped)
	add("scanskip_dirs_walked_total", "", in.skip.DirsWalked)
	add("shadow_verify_sampled_total", "", in.parity.Sampled)
	add("shadow_verify_skipped_pressure_total", "", in.parity.SkippedPressure)
	add("shadow_verify_oracle_errors_total", "", in.parity.OracleErrors)
	for f := range numParityFields {
		add("parity_mismatch_total", f.String(), in.parity.Mismatch[f])
		add("parity_transient_total", f.String(), in.parity.Transient[f])
	}
	add("commit_parents_reader", "stock", 1)
	add("commit_parents_reader", "fork", 0)
	return pts
}

// registerRepostateMetrics creates every catalog instrument and the
// scan_cycle_seconds histogram on meter and wires one callback that observes
// all of them from collect.
func registerRepostateMetrics(meter metric.Meter, collect func() repostateInputs) error {
	hist, err := meter.Float64Histogram(repostateMetricPrefix+"scan_cycle_seconds",
		metric.WithDescription("Wall time of one scan task (scanRepo + publishResults), for per-scan normalisation of profiles"),
		metric.WithUnit("s"))
	if err != nil {
		return err
	}
	scanCycleSeconds.Store(&hist)

	instruments := make(map[string]metric.Int64Observable, len(repostateCatalog))
	observables := make([]metric.Observable, 0, len(repostateCatalog))
	for _, spec := range repostateCatalog {
		name := repostateMetricPrefix + spec.name
		var inst metric.Int64Observable
		if spec.kind == kindCounter {
			inst, err = meter.Int64ObservableCounter(name, metric.WithDescription(spec.desc))
		} else {
			inst, err = meter.Int64ObservableGauge(name, metric.WithDescription(spec.desc))
		}
		if err != nil {
			return err
		}
		instruments[spec.name] = inst
		observables = append(observables, inst)
	}
	attrKey := make(map[string]string, len(repostateCatalog))
	for _, spec := range repostateCatalog {
		attrKey[spec.name] = spec.attr
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, p := range collect().points() {
			inst := instruments[p.name]
			if key := attrKey[p.name]; key != "" {
				o.ObserveInt64(inst, p.value, metric.WithAttributes(attribute.String(key, p.attrVal)))
			} else {
				o.ObserveInt64(inst, p.value)
			}
		}
		return nil
	}, observables...)
	return err
}
