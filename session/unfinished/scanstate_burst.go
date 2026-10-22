package unfinished

import (
	"context"
	"sync/atomic"
)

// Per-Burst prelude lifecycle (begin/end/drop snapshot), ScanStateStats and the scan-cycle log line.

// scanStateStats counts prelude activity; all fields are lock-free.
type scanStateStats struct {
	snapshotsBuilt     atomic.Int64
	snapshotsDropped   atomic.Int64
	reusedAcrossBursts atomic.Int64 // must stay 0: a snapshot outlived its Burst
	signatureChecks    atomic.Int64 // committed-state lookups that had a signature
	signatureSkips     atomic.Int64 // lookups served from the stored committed state
}

// ScanStateStats is a snapshot of the prelude counters.
type ScanStateStats struct {
	SnapshotsBuilt, SnapshotsDropped, ReusedAcrossBursts int64
	// SignatureChecks and SignatureSkips give the skip engagement fraction
	// (row SG8 of the soak table): Skips / Checks.
	SignatureChecks, SignatureSkips int64
}

// ScanStateStats returns the prelude counters.
func (s *Scanner) ScanStateStats() ScanStateStats {
	return ScanStateStats{
		SnapshotsBuilt:     s.scanStats.snapshotsBuilt.Load(),
		SnapshotsDropped:   s.scanStats.snapshotsDropped.Load(),
		ReusedAcrossBursts: s.scanStats.reusedAcrossBursts.Load(),
		SignatureChecks:    s.scanStats.signatureChecks.Load(),
		SignatureSkips:     s.scanStats.signatureSkips.Load(),
	}
}

// flagView is the optional reader hook exposing its flag snapshot (tests
// override it per reader).
type flagView interface{ effectiveFlags() Flags }

// preludeEnabled reports whether the scan_skip prelude (RefSnapshot and
// ScanSignature) is on, which needs scan_skip and shared_commit_state.
func (s *Scanner) preludeEnabled() bool {
	if v, ok := s.reader.(flagView); ok {
		return v.effectiveFlags().ScanSkipPrelude
	}
	return s.flags.ScanSkipPrelude
}

type pressureReader interface{ UnderSeverePressure() bool }

func (s *Scanner) scanRecord(repoPath string) *repoScanRecord {
	if v, ok := s.scanRecords.Load(repoPath); ok {
		rec, _ := v.(*repoScanRecord)
		return rec
	}
	v, _ := s.scanRecords.LoadOrStore(repoPath, &repoScanRecord{})
	rec, _ := v.(*repoScanRecord)
	return rec
}

// beginBurst builds the Burst's RefSnapshot. It reads only refs and two stats:
// no graph is filled, so nothing is pre-ingested. It builds nothing under
// severe pressure, and the Burst then takes the plain path.
func (s *Scanner) beginBurst(repoPath, defaultBranch string, worktrees []string) {
	if defaultBranch == "" || !s.preludeEnabled() {
		return
	}
	r, ok := s.reader.(refSnapshotReader)
	if !ok {
		return
	}
	if p, ok := s.reader.(pressureReader); ok && p.UnderSeverePressure() {
		return
	}
	rec := s.scanRecord(repoPath)
	if rec.snap.Swap(nil) != nil {
		s.scanStats.reusedAcrossBursts.Add(1)
	}
	rec.snap.Store(r.ResolveRefSnapshot(repoPath, defaultBranch, worktrees))
	rec.probe.Store(&burstProbe{base: defaultBranch, paths: worktrees})
	s.scanStats.snapshotsBuilt.Add(1)
}

// endBurst discards the Burst's snapshot; it is never reused by a later Burst.
func (s *Scanner) endBurst(repoPath string) {
	if v, ok := s.scanRecords.Load(repoPath); ok {
		if rec, _ := v.(*repoScanRecord); rec != nil {
			rec.snap.Store(nil)
		}
	}
}

// dropRefSnapshot is called on a .git event: a snapshot taken before the event
// no longer describes the repo.
func (s *Scanner) dropRefSnapshot(repoPath string) {
	if v, ok := s.scanRecords.Load(repoPath); ok {
		if rec, _ := v.(*repoScanRecord); rec != nil && rec.snap.Swap(nil) != nil {
			s.scanStats.snapshotsDropped.Add(1)
		}
	}
}

// burstSnapshot returns the live snapshot of repoPath's Burst, or nil.
func (s *Scanner) burstSnapshot(repoPath string) *RefSnapshot {
	if v, ok := s.scanRecords.Load(repoPath); ok {
		if rec, _ := v.(*repoScanRecord); rec != nil {
			return rec.snap.Load()
		}
	}
	return nil
}

// aheadBehind runs phase 2 from the Burst's resolved hashes when the snapshot
// still covers worktreePath, and the reader's full call otherwise.
func (s *Scanner) aheadBehind(ctx context.Context, snap *RefSnapshot, worktreePath, base string) (int, int, error) {
	if snap != nil && snap.Base == base && snap.BaseErr == nil {
		if r, ok := s.reader.(refSnapshotReader); ok {
			if refs, ok := snap.Worktrees[worktreePath]; ok && refs.Err == nil {
				return r.AheadBehindResolved(ctx, worktreePath, base, refs.Head, snap.BaseHash)
			}
		}
	}
	return s.reader.AheadBehind(ctx, worktreePath, base)
}

// worktreePaths lists the paths of the worktrees a Burst will scan.
func worktreePaths(wts []WorktreeInfo) []string {
	paths := make([]string, 0, len(wts))
	for _, wt := range wts {
		if wt.IsBare || wt.IsDetached || wt.IsPrunable || wt.Branch == "" {
			continue
		}
		paths = append(paths, wt.Path)
	}
	return paths
}

// logScanCycle starts the per-Burst debug line and returns the function that
// emits it: repo, worktrees, duration, decodes and graph rows filled. Disabled
// (the default level) it costs one level check and no counter reads.
func (s *Scanner) logScanCycle(repoPath string, worktrees int) func() {
	if !s.debugEnabled() {
		return func() {}
	}
	started := s.now()
	decodes0, rows0 := s.cycleCounters()
	return func() {
		decodes1, rows1 := s.cycleCounters()
		s.logDebug("scan cycle",
			"repo", repoPath, "worktrees", worktrees,
			"duration", s.now().Sub(started),
			"decodes", decodes1-decodes0, "graph_rows_filled", rows1-rows0)
	}
}
