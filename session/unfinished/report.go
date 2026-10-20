package unfinished

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// commitParentsImpl names the CommitParentsReader in use. Only the stock
// reader exists: the fork reader (ssqfork build tag) was never built.
const commitParentsImpl = "stock"

// CacheReport is one cache's counters in Report.
type CacheReport struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Coalesced int64 `json:"coalesced"`
	Bytes     int64 `json:"bytes"`
	Entries   int64 `json:"entries"`
}

// Report is the read-only view of every scanner counter, served by
// /actuator/unfinished-scanner. Every field is read from atomics, so building
// it takes no lock and needs no OpenTelemetry.
type Report struct {
	CommitParentsReader  string                 `json:"commit_parents_reader"`
	Flags                map[string]bool        `json:"flags"`
	ShadowVerifyEvery    int                    `json:"shadow_verify_every"`
	ScanCyclesTotal      int64                  `json:"scan_cycles_total"`
	ScanCycleSecondsSum  float64                `json:"scan_cycle_seconds_sum"`
	DecodedCommitsTotal  int64                  `json:"decoded_commits_total"`
	StatCallsTotal       int64                  `json:"stat_calls_total"`
	StatCallsPerScan     int64                  `json:"stat_calls_per_scan"`
	RepoStates           int64                  `json:"repostates"`
	RepoStateBytes       int64                  `json:"repostate_bytes"`
	Caches               map[string]CacheReport `json:"caches"`
	ReachBuilds          map[string]uint64      `json:"reach_builds"`
	CASRetriesTotal      int64                  `json:"cas_retries_total"`
	GraphFullFallbacks   uint64                 `json:"graph_full_fallbacks_total"`
	GraphFullRebuilds    uint64                 `json:"graph_full_rebuilds_total"`
	RedundancyUnproven   map[string]int64       `json:"mergebase_redundancy_unproven_total"`
	GraphExtension       map[string]any         `json:"graph_extension"`
	DirtyFullWalkTotal   map[string]int64       `json:"dirty_full_walk_total"`
	ParityMismatchTotal  map[string]int64       `json:"parity_mismatch_total"`
	ParityTransientTotal map[string]int64       `json:"parity_transient_total"`
	ShadowVerify         map[string]int64       `json:"shadow_verify"`
	ScanSkip             ScanSkipStats          `json:"scan_skip"`
	SignatureSkip        map[string]int64       `json:"signature_skip"`
}

// BuildReport snapshots every counter. With no reader or scanner registered
// every number is 0 and the cache keys are still present.
func BuildReport() Report {
	in := collectRepostateInputs()
	f := EffectiveFlags()
	r := Report{
		CommitParentsReader: commitParentsImpl,
		Flags: map[string]bool{
			"shared_commit_state":    f.SharedCommitState,
			"diff_counter":           f.DiffCounter,
			"scan_skip":              f.ScanSkip,
			"watch_worktree_gitdirs": f.WatchWorktreeGitdirs,
			"scan_skip_prelude":      f.ScanSkipPrelude,
			"scan_skip_dirty_gen":    f.ScanSkipDirtyGen,
		},
		ScanCyclesTotal:     ScanCyclesTotal(),
		DecodedCommitsTotal: in.decoded,
		StatCallsTotal:      in.statCalls,
		StatCallsPerScan:    perScan(in.statCalls, ScanCyclesTotal()),
		RepoStates:          in.repoStates,
		RepoStateBytes:      in.repoBytes,
		Caches:              make(map[string]CacheReport, len(repostateCaches)+2),
		ReachBuilds:         map[string]uint64{"scratch": in.reachBuilds.Scratch, "incremental": in.reachBuilds.Incremental},
		CASRetriesTotal:     in.casRetries(),
		GraphFullFallbacks:  in.graph.GraphFull,
		GraphFullRebuilds:   in.graph.FullRebuild,
		RedundancyUnproven: map[string]int64{
			redundancyOutcomeNames[redundancyBound]:      in.redundancy[redundancyBound],
			redundancyOutcomeNames[redundancyPrunedMiss]: in.redundancy[redundancyPrunedMiss],
		},
		GraphExtension: map[string]any{
			"count": in.graphExt.Count, "over_100ms": in.graphExt.SlowOver100ms,
			"seconds_sum": in.graphExt.Total.Seconds(),
		},
		DirtyFullWalkTotal:   map[string]int64{},
		ParityMismatchTotal:  map[string]int64{},
		ParityTransientTotal: map[string]int64{},
		ScanSkip:             in.skip,
		SignatureSkip: map[string]int64{
			"checks": in.scan.SignatureChecks, "hits": in.scan.SignatureSkips,
			"misses": in.scan.SignatureChecks - in.scan.SignatureSkips,
		},
		ShadowVerify: map[string]int64{
			"sampled": in.parity.Sampled, "skipped_pressure": in.parity.SkippedPressure,
			"oracle_errors": in.parity.OracleErrors,
		},
	}
	_, total := ScanCycleDurations()
	r.ScanCycleSecondsSum = total.Seconds()
	for _, c := range repostateCaches {
		r.Caches[c] = cacheReport(in.caches[c])
	}
	r.Caches["index"] = CacheReport{Hits: in.skip.IndexHits, Misses: in.skip.IndexMisses}
	r.Caches["untracked"] = CacheReport{Hits: in.skip.DirsSkipped, Misses: in.skip.DirsWalked}
	for reason := ReasonOverflow; reason < numFullWalkReasons; reason++ {
		r.DirtyFullWalkTotal[reason.String()] = DirtyFullWalkTotal(reason)
	}
	for field := range numParityFields {
		r.ParityMismatchTotal[field.String()] = in.parity.Mismatch[field]
		r.ParityTransientTotal[field.String()] = in.parity.Transient[field]
	}
	if s := currentScanner.Load(); s != nil {
		r.ShadowVerifyEvery = s.shadowEvery()
	}
	return r
}

func cacheReport(c repostate.CacheStats) CacheReport {
	return CacheReport{Hits: c.Hits, Misses: c.Misses, Evictions: c.Evictions,
		Coalesced: c.Coalesced, Bytes: c.Bytes, Entries: int64(c.Entries)}
}

// LogStartup emits the single INFO line naming the parents reader, the
// effective flags and the shadow-verify rate. Call it once at server startup;
// never from the scan loop (hot-poll log rule).
func LogStartup() { logStartup(log.Info) }

func logStartup(info func(msg string, args ...any)) {
	f := EffectiveFlags()
	every := 0
	if s := currentScanner.Load(); s != nil {
		every = s.shadowEvery()
	}
	info("unfinished scanner configured",
		"commit_parents_reader", commitParentsImpl,
		"shared_commit_state", f.SharedCommitState,
		"diff_counter", f.DiffCounter,
		"scan_skip", f.ScanSkip,
		"watch_worktree_gitdirs", f.WatchWorktreeGitdirs,
		"shadow_verify_every", every)
}
