package unfinished

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

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
	// probe is what the last Burst scanned, so enqueueRepo can ask "did any
	// worktree's signature change?" without a scan.
	probe     atomic.Pointer[burstProbe]
	worktrees sync.Map // map[string]*worktreeScanRecord (key = worktreePath)
}

// burstProbe is the base ref and worktree set of the last Burst.
type burstProbe struct {
	base  string
	paths []string
}

// worktreeScanRecord is the per-worktree signature state. Both pointers are
// replaced wholesale, never mutated.
type worktreeScanRecord struct {
	last      atomic.Pointer[ScanSignature] // signature at the last scan
	committed atomic.Pointer[committedState]
}

// ScanSignature is what one Burst observed about one worktree: its HEAD, the
// base ref, the index stat signature, the watcher generation, and the repo's
// ShallowStamp. Two equal signatures mean "nothing a scan reads from git moved".
type ScanSignature struct {
	Head, Base plumbing.Hash
	Index      statSig
	DirtyGen   uint64
	Shallow    repostate.ShallowStamp
}

// committedKey is the part of a ScanSignature that determines the
// committed-state fields (see scanResultCoverage). The index signature and
// DirtyGen decide only whether a worktree counts as "changed" (ordering and
// the enqueue skip); they never license reusing committed state.
type committedKey struct {
	Head, Base plumbing.Hash
	Shallow    repostate.ShallowStamp
}

func (sg ScanSignature) committed() committedKey {
	return committedKey{Head: sg.Head, Base: sg.Base, Shallow: sg.Shallow}
}

// committedState is a worktree's last committed-state result, valid while the
// committedKey is equal and it is younger than one tick.
type committedState struct {
	key           committedKey
	ahead, behind int
	messages      []string
	storedAt      time.Time
}

// fieldClass says which inputs a ScanResult field depends on.
type fieldClass uint8

const (
	// classCommitted fields are a function of (HEAD, base, ShallowStamp) and are
	// reused while that key is unchanged.
	classCommitted fieldClass = iota + 1
	// classRefDerived fields come from the base ref (covered by its hash).
	classRefDerived
	// classWorkingTree fields depend on files on disk and are always recomputed;
	// the tracked-file stat pass never runs conditionally (ADR-006 hard rule).
	classWorkingTree
	// classIdentity fields are static per worktree or stamped per scan.
	classIdentity
)

// scanResultCoverage assigns every ScanResult field its dependency class. A
// test fails when ScanResult gains a field that is not declared here.
//
// LastModified is os.Stat(worktree).ModTime() and DefaultBranch is
// ResolveDefaultBranch(repo), both read on every scan, so neither is reused.
var scanResultCoverage = map[string]fieldClass{
	"AheadCount":     classCommitted,
	"BehindCount":    classCommitted,
	"AheadMessages":  classCommitted,
	"DefaultBranch":  classRefDerived,
	"HasUncommitted": classWorkingTree,
	"ChangedFiles":   classWorkingTree,
	"LinesAdded":     classWorkingTree,
	"LinesRemoved":   classWorkingTree,
	"LastModified":   classWorkingTree,
	"Status":         classWorkingTree,
	"ErrorMsg":       classWorkingTree,
	"RepoPath":       classIdentity,
	"Branch":         classIdentity,
	"WorktreePath":   classIdentity,
	"RepoName":       classIdentity,
	"DisplayPath":    classIdentity,
	"SessionIDs":     classIdentity,
	"ScanTime":       classIdentity,
}

func (r *repoScanRecord) worktree(path string) *worktreeScanRecord {
	if v, ok := r.worktrees.Load(path); ok {
		w, _ := v.(*worktreeScanRecord)
		return w
	}
	v, _ := r.worktrees.LoadOrStore(path, &worktreeScanRecord{})
	w, _ := v.(*worktreeScanRecord)
	return w
}

// signatureOf assembles worktreePath's signature from snap; ok is false when
// HEAD or the base did not resolve (the scan then takes the plain path).
func (snap *RefSnapshot) signatureOf(worktreePath string) (ScanSignature, bool) {
	if snap == nil || snap.BaseErr != nil {
		return ScanSignature{}, false
	}
	refs, ok := snap.Worktrees[worktreePath]
	if !ok || refs.Err != nil {
		return ScanSignature{}, false
	}
	return ScanSignature{Head: refs.Head, Base: snap.BaseHash, Index: refs.Index, DirtyGen: refs.DirtyGen, Shallow: snap.Shallow}, true
}

func (s *Scanner) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// signatureMaxAge bounds committed-state reuse to one tick: a change that
// leaves every signature component equal is corrected within a tick.
func (s *Scanner) signatureMaxAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tickInterval
}

// fillCommittedState sets the committed-state fields of result. When the
// worktree's committed key is unchanged and younger than one tick it reuses the
// stored values and skips AheadBehind and CommitMessages; HasUncommitted and
// the diff are the caller's and always run.
func (s *Scanner) fillCommittedState(result *ScanResult, repoPath, worktreePath, base string) {
	snap := s.burstSnapshot(repoPath)
	sig, haveSig := snap.signatureOf(worktreePath)
	var wrec *worktreeScanRecord
	if haveSig {
		wrec = s.scanRecord(repoPath).worktree(worktreePath)
		s.scanStats.signatureChecks.Add(1)
		if cs := wrec.committed.Load(); cs != nil && cs.key == sig.committed() && s.now().Sub(cs.storedAt) < s.signatureMaxAge() {
			s.scanStats.signatureSkips.Add(1)
			result.AheadCount, result.BehindCount = cs.ahead, cs.behind
			result.AheadMessages = slices.Clone(cs.messages)
			wrec.last.Store(&sig)
			return
		}
	}
	complete := false
	ahead, behind, aErr := s.aheadBehind(snap, worktreePath, base)
	if aErr == nil {
		result.AheadCount = ahead
		result.BehindCount = behind
		complete = true
		if ahead > 0 {
			msgs, mErr := s.reader.CommitMessages(worktreePath, base, 5)
			if mErr == nil {
				result.AheadMessages = msgs
			} else {
				complete = false
			}
		}
	}
	if haveSig {
		wrec.last.Store(&sig)
		if complete {
			wrec.committed.Store(&committedState{key: sig.committed(), ahead: ahead, behind: behind, messages: slices.Clone(result.AheadMessages), storedAt: s.now()})
		}
	}
}

// changedFirst orders worktrees so those whose signature changed since their
// last scan come first (stable within each group), so a fresh commit is
// reported before the unchanged siblings are looked at.
func (s *Scanner) changedFirst(repoPath string, wts []WorktreeInfo) []WorktreeInfo {
	snap := s.burstSnapshot(repoPath)
	if snap == nil {
		return wts
	}
	rec := s.scanRecord(repoPath)
	out := make([]WorktreeInfo, 0, len(wts))
	var unchanged []WorktreeInfo
	for _, wt := range wts {
		if rec.signatureChanged(snap, wt.Path) {
			out = append(out, wt)
		} else {
			unchanged = append(unchanged, wt)
		}
	}
	return append(out, unchanged...)
}

// signatureChanged reports whether worktreePath's signature in snap differs
// from the one recorded at its last scan. A worktree never scanned, or whose
// refs did not resolve, counts as changed.
func (r *repoScanRecord) signatureChanged(snap *RefSnapshot, worktreePath string) bool {
	sig, ok := snap.signatureOf(worktreePath)
	if !ok {
		return true
	}
	v, ok := r.worktrees.Load(worktreePath)
	if !ok {
		return true
	}
	w, _ := v.(*worktreeScanRecord)
	last := w.last.Load()
	return last == nil || *last != sig
}

// siblingSignatureChanged is enqueueRepo's check behind its "any worktree cache
// is fresh, skip the repo" rule: a fresh sibling must not hide a worktree whose
// HEAD, base, index or watcher generation moved. It resolves refs (a handful of
// stats and file reads) and only runs when that rule would otherwise skip.
func (s *Scanner) siblingSignatureChanged(repoPath string) bool {
	if !s.preludeEnabled() {
		return false
	}
	r, ok := s.reader.(refSnapshotReader)
	if !ok {
		return false
	}
	v, ok := s.scanRecords.Load(repoPath)
	if !ok {
		return false
	}
	rec, _ := v.(*repoScanRecord)
	if rec == nil {
		return false
	}
	probe := rec.probe.Load()
	if probe == nil {
		return false
	}
	snap := r.ResolveRefSnapshot(repoPath, probe.base, probe.paths)
	for _, p := range probe.paths {
		if rec.signatureChanged(snap, p) {
			return true
		}
	}
	return false
}

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

// currentScanner is the process's most recently constructed Scanner, read by
// the metrics callback (the production process has exactly one).
var currentScanner atomic.Pointer[Scanner]

// logScanCycle starts the per-Burst debug line and returns the function that
// emits it: repo, worktrees, duration, decodes and graph rows filled. Disabled
// (the default level) it costs one level check and no counter reads.
func (s *Scanner) logScanCycle(repoPath string, worktrees int) func() {
	if !s.debugEnabled() {
		return func() {}
	}
	started := s.now()
	decodes0, rows0 := cycleCounters()
	return func() {
		decodes1, rows1 := cycleCounters()
		s.logDebug("scan cycle",
			"repo", repoPath, "worktrees", worktrees,
			"duration", s.now().Sub(started),
			"decodes", decodes1-decodes0, "graph_rows_filled", rows1-rows0)
	}
}
