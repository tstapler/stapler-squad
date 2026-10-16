package unfinished

import (
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// refSnapshotReader is the optional VCSReader capability behind the per-Burst
// prelude (Story 4.1.1): resolve the base ref, every worktree HEAD and the
// shallow stamp once, then run phase 2 of AheadBehind from those hashes.
type refSnapshotReader interface {
	ResolveRefSnapshot(repoPath, base string, worktrees []string) *RefSnapshot
	AheadBehindResolved(worktreePath, base string, head, baseHash plumbing.Hash) (ahead, behind int, err error)
}

// WorktreeRefs is what one Burst observed about one worktree.
type WorktreeRefs struct {
	Head plumbing.Hash
	// Err is set when HEAD could not be resolved; callers then take the plain
	// path, which reports the real error.
	Err error
	// Index is the stat signature of the worktree's index file (zero if absent).
	Index statSig
	// DirtyGen is the watcher generation, read only under ScanSkipDirtyGen.
	DirtyGen uint64
}

// RefSnapshot is the immutable resolution of the base ref, every worktree HEAD
// and the repo-level ShallowStamp for one Burst. It lives for exactly one
// Burst: a base-ref move can produce no watched event (Tier A watches only
// .git/worktrees/<name>/), so every Burst, tick-initiated or not, re-resolves.
type RefSnapshot struct {
	Base     string
	BaseHash plumbing.Hash
	// BaseErr is set when Base could not be resolved.
	BaseErr   error
	Shallow   repostate.ShallowStamp
	Worktrees map[string]WorktreeRefs
}

// repoScanRecord is the per-repo scan state. snap is non-nil only while a Burst
// is running; a .git event drops it so the rest of that Burst re-resolves.
type repoScanRecord struct {
	snap atomic.Pointer[RefSnapshot]
}

// scanStateStats counts prelude activity; all fields are lock-free.
type scanStateStats struct {
	snapshotsBuilt     atomic.Int64
	snapshotsDropped   atomic.Int64
	reusedAcrossBursts atomic.Int64 // must stay 0: a snapshot outlived its Burst
}

// ScanStateStats is a snapshot of the prelude counters.
type ScanStateStats struct {
	SnapshotsBuilt, SnapshotsDropped, ReusedAcrossBursts int64
}

// ScanStateStats returns the prelude counters.
func (s *Scanner) ScanStateStats() ScanStateStats {
	return ScanStateStats{
		SnapshotsBuilt:     s.scanStats.snapshotsBuilt.Load(),
		SnapshotsDropped:   s.scanStats.snapshotsDropped.Load(),
		ReusedAcrossBursts: s.scanStats.reusedAcrossBursts.Load(),
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
	return EffectiveFlags().ScanSkipPrelude
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
func (s *Scanner) aheadBehind(snap *RefSnapshot, worktreePath, base string) (int, int, error) {
	if snap != nil && snap.Base == base && snap.BaseErr == nil {
		if r, ok := s.reader.(refSnapshotReader); ok {
			if refs, ok := snap.Worktrees[worktreePath]; ok && refs.Err == nil {
				return r.AheadBehindResolved(worktreePath, base, refs.Head, snap.BaseHash)
			}
		}
	}
	return s.reader.AheadBehind(worktreePath, base)
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
