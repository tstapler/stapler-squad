package unfinished

import (
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// racyWindow is git's racy-timestamp rule: a stat signature only proves
// "unchanged" when its newest timestamp is older than the moment it was
// observed by more than this, because a second write inside one timestamp
// granule leaves (mtime, size, inode) equal (ADR-006 addendum, Story 4.2.1).
const racyWindow = 2 * time.Second

// scanSkipMaxAge bounds how long any scan_skip verdict is trusted: a few
// backstop ticks (see scanSkipAgeTicks), so a change that leaves every cache
// key equal is corrected within that bound (ADR-006).
const scanSkipMaxAge = fullWalkMaxAge

// statSig is the stat signature a scan_skip cache compares: (mtime, ctime,
// size, inode). ctime cannot be set from user space, so it exposes
// mtime-preserving writes (rsync -a, tar x, os.Chtimes).
type statSig struct {
	mtimeNs, ctimeNs, size int64
	ino                    uint64
}

// newest is the later of mtime and ctime, in unix nanoseconds.
func (s statSig) newest() int64 { return max(s.mtimeNs, s.ctimeNs) }

// trustedAt reports whether an equal signature observed at observedAt may be
// trusted (racy rule).
func (s statSig) trustedAt(observedAt time.Time) bool {
	return observedAt.UnixNano()-s.newest() > int64(racyWindow)
}

// ScanSkipStats is a snapshot of the scan_skip engagement counters. A cache
// that the racy guard keeps disabled shows zero hits, which is why every cache
// test asserts a hit counter above zero.
type ScanSkipStats struct {
	IndexHits       int64 // HasUncommitted served the parsed index from cache
	IndexMisses     int64 // index decoded (cold, changed, HEAD moved, or forced)
	IndexRacySkips  int64 // an equal signature was refused by the racy guard
	IndexDecodes    int64 // index.Decode calls made by HasUncommitted
	DirsSkipped     int64 // untracked walk: directories served from the verdict cache
	DirsWalked      int64 // untracked walk: directories read from disk
	DirsRacyRefused int64 // untracked walk: verdicts not stored or not trusted (racy)
}

// scanSkipCounters backs ScanSkipStats.
type scanSkipCounters struct {
	indexHits, indexMisses, indexRacySkips, indexDecodes atomic.Int64
	dirsSkipped, dirsWalked, dirsRacyRefused             atomic.Int64
}

// scanSkipState holds every scan_skip cache of one reader. The zero value is
// ready to use and every field is lock-free.
type scanSkipState struct {
	// clock is the injected time source (nil means time.Now); tests inject it
	// to step past the racy window without sleeping.
	clock func() time.Time
	// flags, when non-nil, overrides EffectiveFlags for this reader (tests).
	flags *Flags

	indexCache lazyMap[string, *parsedIndex]
	untracked  lazyMap[string, *untrackedCache]
	dirtyGens  lazyMap[string, *DirtyGen]
	stats      scanSkipCounters
	refs       refResolveCounters
	// oracle is the lazily built legacy-path reader of the shadow-verify sampler.
	oracle atomic.Pointer[GoGitVCSReader]
	// oracleUsers counts sampled runs holding the oracle; -1 while the last
	// releaser clears it (see OracleBegin/OracleDone).
	oracleUsers atomic.Int32
	// oracleGate is closed when the clear that set oracleUsers to -1 ends, waking
	// OracleBegin waiters without spinning. A waiter that loads a stale (closed)
	// gate just loops once more.
	oracleGate atomic.Pointer[chan struct{}]
	// clearOracle overrides the oracle's ClearCache (tests inject a panic).
	clearOracle func(*GoGitVCSReader)

	// passSF coalesces HasUncommittedPass callers; its result type differs
	// from hasUncommittedSF's, so it needs its own group.
	passSF singleflight.Group //nolint:exhaustruct
}

func (s *scanSkipState) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// effectiveFlags is this reader's flag view; a reader nobody gave flags runs
// with every rollback unit off.
func (g *GoGitVCSReader) effectiveFlags() Flags {
	if f := g.scanSkip.flags; f != nil {
		return *f
	}
	return Flags{}
}

// setFlags fixes the reader's flags. Call it before the reader is shared.
func (g *GoGitVCSReader) setFlags(f Flags) { g.scanSkip.flags = &f }

// ScanSkipStats returns the engagement counters.
func (g *GoGitVCSReader) ScanSkipStats() ScanSkipStats {
	c := &g.scanSkip.stats
	return ScanSkipStats{
		IndexHits:       c.indexHits.Load(),
		IndexMisses:     c.indexMisses.Load(),
		IndexRacySkips:  c.indexRacySkips.Load(),
		IndexDecodes:    c.indexDecodes.Load(),
		DirsSkipped:     c.dirsSkipped.Load(),
		DirsWalked:      c.dirsWalked.Load(),
		DirsRacyRefused: c.dirsRacyRefused.Load(),
	}
}

// pruneScanSkip drops idle scan_skip entries; severe pressure drops them all.
// Signature-validated entries rebuild on the next scan, so dropping is safe.
func (g *GoGitVCSReader) pruneScanSkip(severe bool) {
	cutoff := g.scanSkip.now().Add(-repoCacheTTL).UnixNano()
	g.scanSkip.indexCache.Range(func(k string, v *parsedIndex) bool {
		if severe || v.accessedAtNs.Load() < cutoff {
			g.scanSkip.indexCache.Delete(k)
		}
		return true
	})
	g.scanSkip.untracked.Range(func(k string, v *untrackedCache) bool {
		if severe || v.accessedAtNs.Load() < cutoff {
			g.scanSkip.untracked.Delete(k)
		}
		return true
	})
}

// forgetScanSkip drops one worktree's scan_skip state (worktree gone).
func (g *GoGitVCSReader) forgetScanSkip(worktreePath string) {
	g.scanSkip.indexCache.Delete(worktreePath)
	g.scanSkip.untracked.Delete(worktreePath)
	g.scanSkip.dirtyGens.Delete(worktreePath)
}
