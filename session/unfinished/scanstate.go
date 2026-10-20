package unfinished

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// Scan-state data model: RefSnapshot, per-repo/worktree scan records, ScanSignature and field coverage.

// refSnapshotReader is the optional VCSReader capability behind the per-Burst
// prelude (Story 4.1.1): resolve the base ref, every worktree HEAD and the
// shallow stamp once, then run phase 2 of AheadBehind from those hashes.
type refSnapshotReader interface {
	ResolveRefSnapshot(repoPath, base string, worktrees []string) *RefSnapshot
	AheadBehindResolved(ctx context.Context, worktreePath, base string, head, baseHash plumbing.Hash) (ahead, behind int, err error)
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

// signatureMaxAge bounds committed-state reuse to scanSkipAgeTicks ticks: a
// change that leaves every signature component equal is corrected within that
// bound. At one tick it equalled the scan cadence, so no scheduled scan ever
// reused (0 of 234 in the S2 A/B); refs are re-resolved every Burst, so the
// bound only backstops what the signature cannot see.
func (s *Scanner) signatureMaxAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return scanSkipAgeTicks * s.tickInterval
}
