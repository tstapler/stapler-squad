package unfinished

import (
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/tstapler/stapler-squad/log"
)

// DiffShortstat orchestration, memo resolution and per-file diff application.

// DiffShortstat returns changed-file and line counts for the given worktree.
// Results are cached for diffStatCacheTTL (30s) to avoid repeated wt.Status()
// calls from concurrent scanner workers, which was the top mutex hotspot (537M
// cycles, 13,941 events in profiling).
func (g *GoGitVCSReader) DiffShortstat(worktreePath string) (DiffStat, error) {
	if v, ok := g.diffStatCache.Load(worktreePath); ok {
		if e := v.(diffStatEntry); time.Now().Before(e.expiry) {
			return e.result, nil
		}
	}
	// After the cache check: the guard allocates, and a hit must stay alloc-free.
	defer g.guardInvalidation(worktreePath)()
	result, err := sfDo(&g.diffStatSF, worktreePath, func() (DiffStat, error) {
		// diffShortstatUncached acquires entry.mu internally — do NOT add an outer lock here.
		// It does not call DiffShortstat, so there is no recursive sfDo path for this key.
		stat, uncachedErr := g.diffShortstatUncached(worktreePath)
		if uncachedErr != nil {
			return DiffStat{}, uncachedErr
		}
		g.diffStatCache.Store(worktreePath, diffStatEntry{
			result: stat,
			expiry: time.Now().Add(diffStatCacheTTL),
		})
		return stat, nil
	})
	if err != nil {
		return DiffStat{}, err
	}
	return result, nil
}

// diffShortstatUncached computes a DiffStat for worktreePath using index metadata
// to detect changed files, then loads HEAD blobs only for those files.
//
// This replaces the previous wt.Status() approach, which loaded ALL blob content
// into Go heap (MemoryObject) to hash every tracked file — allocating ~30 GB/hour
// cumulatively across active repos. The new approach:
//
//  1. Identifies staged changes by comparing index hashes with HEAD tree hashes
//     (TreeWalker, no blobs loaded).
//  2. Identifies unstaged changes by comparing index mtime/size with disk stat
//     (O(n) lstat calls, zero blob reads for unchanged files).
//  3. Loads HEAD blobs only for the M actually-changed files (M << N typical).
//  4. Computes line diffs on []byte directly, avoiding string conversion overhead.
func (g *GoGitVCSReader) diffShortstatUncached(worktreePath string) (DiffStat, error) {
	return g.diffShortstatUncachedPass(worktreePath, nil)
}

// diffShortstatUncachedPass is diffShortstatUncached taking the lstat results
// HasUncommittedPass already gathered in this scan (nil: stat every file here).
func (g *GoGitVCSReader) diffShortstatUncachedPass(worktreePath string, pass *statPass) (DiffStat, error) {
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return DiffStat{}, err
	}
	tg := g.currentTrackedGen(worktreePath) // before the index read: the signature may only predate it

	// ── Phase 1: go-git metadata reads under lock (no blob loading) ──────────
	entry.mu.Lock()
	repo := entry.repo

	idx, err := repo.Storer.Index()
	if err != nil {
		entry.mu.Unlock()
		return DiffStat{}, err
	}

	// Build HEAD tree hash map — reused across polls via entry.headTreeCache
	// (shared with hasUncommittedGoGitPhase) as long as HEAD hasn't moved,
	// since HEAD only changes on commit/checkout, far less often than the
	// poll cycle. resolveHeadTreeHashes returns (nil, nil) for an unborn
	// branch (no commits yet), which we treat as an empty HEAD tree below.
	//
	// NOTE: this now propagates HEAD-commit/HEAD-tree lookup errors as fatal,
	// whereas the pre-extraction code silently swallowed them and proceeded
	// with an empty headHashes map. Unifying to "always propagate" matches
	// hasUncommittedGoGitPhase's existing behavior and is intentional — no
	// test in this package or server/services relies on the old swallow
	// behavior (verified via `go test ./session/unfinished/... ./server/services/...`).
	headHashes, headErr := resolveHeadTreeHashes(g, entry, repo)
	if headErr != nil {
		entry.mu.Unlock()
		return DiffStat{}, headErr
	}
	if headHashes == nil {
		headHashes = make(map[string]plumbing.Hash, len(idx.Entries))
	}

	// F6: reuse the compiled gitignore matcher across polls; resolveHeadTreeHashes
	// clears untrackedMatcherBuilt whenever HEAD moves, so a stale matcher can
	// only persist for at most one HEAD move's worth of polls — an acceptable
	// trade-off for skipping a redundant .gitignore walk every 30s poll.
	untrackedMatcher := g.getOrBuildUntrackedMatcher(entry, worktreePath)

	metas, indexedNames, stagedDels := classifyIndex(idx, headHashes)

	entry.mu.Unlock() // release before OS operations

	// indexMtime anchors the racy-git window check below: a file is only
	// "racy" (stat can't prove it's unchanged) if its own mtime is not clearly
	// before the index was last written — the same test git itself uses,
	// rather than re-hashing every stat-clean file on every single poll.
	var indexMtime time.Time
	if fi, statErr := os.Stat(filepath.Join(worktreeGitDir(worktreePath), "index")); statErr == nil {
		indexMtime = fi.ModTime()
	}

	// ── Phase 2: build change target list (no lock, no blob reads) ───────────
	stagedTargets := stagedTargetList(metas, stagedDels)
	unstagedTargets := g.unstagedTargetList(worktreePath, metas, indexMtime, pass)

	// ── Phase 3: memo lookup, then batch-read the needed blobs ───────────────
	// With diff_counter on, files whose stat identity and HEAD blob are
	// unchanged since a prior diff are answered from the memo and cost neither
	// a blob read nor a file read.
	var d DiffStat
	var memo *fileDiffMemo
	if g.effectiveFlags().DiffCounter {
		memo = diffMemoInstance()
	}
	pending := make([]pendingTarget, 0, len(stagedTargets)+len(unstagedTargets))
	for _, ts := range [][]changeTarget{stagedTargets, unstagedTargets} {
		pending = g.resolveMemoHits(memo, worktreePath, ts, pending, &d)
	}
	blobMap := g.loadHeadBlobs(entry, pending)

	for _, p := range pending {
		g.applyDiff(&d, p, blobMap, memo, worktreePath)
	}

	// Untracked files: count lines as pure insertions (no HEAD version).
	// Limited to maxUntrackedFiles to avoid OOM on repos with large build-artifact trees.
	// Files over maxUntrackedFileSize are counted but not read for line counting.
	// gitignore-matched paths (node_modules/, build output, etc.) are skipped
	// entirely rather than walked and read — this was previously the single
	// largest allocator in the app (~163GB cum in one profiling session),
	// since ignored build trees were read just like any other untracked file.
	// untrackedMatcher was resolved in Phase 1 above (cached on entry, F6).
	files, insertions, walkErr := g.countUntracked(worktreePath, indexedNames, untrackedMatcher, tg)
	d.Files += files
	d.Insertions += insertions
	if walkErr != nil {
		// Keep the partial stat (an unreadable subdirectory must not blank the
		// whole badge) but say it is incomplete.
		log.WarningLog().Printf("[GoGitVCSReader] untracked walk of %s aborted, diff stat undercounts untracked files: %v", worktreePath, walkErr)
	}

	return d, nil
}

// countUntracked counts untracked files as pure insertions. On a walk error it
// returns what was counted before the walk aborted, plus the error.
func (g *GoGitVCSReader) countUntracked(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher, tg trackedGen) (files, insertions int, err error) {
	err = g.walkUntrackedSkip(worktreePath, indexed, matcher, tg, func(absPath string) {
		files++
		if n, ok := countFileLines(absPath); ok {
			insertions += n
		}
	})
	return files, insertions, err
}

// changeTarget is one file whose diff against HEAD must be counted.
type changeTarget struct {
	name      string
	headHash  plumbing.Hash // zero = new file not in HEAD
	isDeleted bool          // no working-tree file exists
}

// pendingTarget is a changeTarget the memo could not answer. keyed reports
// whether key is valid for storing the result afterwards.
type pendingTarget struct {
	changeTarget
	key   FileDiffMemoKey
	keyed bool
}

// resolveMemoHits adds memo-answered targets straight to d and appends the
// rest to pending. A nil memo (diff_counter off) passes everything through.
func (g *GoGitVCSReader) resolveMemoHits(memo *fileDiffMemo, root string, targets []changeTarget, pending []pendingTarget, d *DiffStat) []pendingTarget {
	for _, t := range targets {
		p := pendingTarget{changeTarget: t}
		if memo != nil && !t.isDeleted {
			if key, ok := memo.keyFor(filepath.Join(root, t.name), t.headHash); ok {
				if c, hit := memo.lookup(key); hit {
					d.Files++
					d.Insertions += c.Insertions
					d.Deletions += c.Deletions
					continue
				}
				p.key, p.keyed = key, true
			}
		}
		pending = append(pending, p)
	}
	return pending
}

// loadHeadBlobs reads every distinct HEAD blob the pending targets need under
// a single lock hold. Collecting all hashes first and reading them in one
// critical section eliminates the N lock-acquire/release cycles (one per
// changed file) that was the #2 mutex hotspot: 9.87B cycles, 1641 events in
// pprof profiling. Peak memory is bounded by the sum of changed-file blobs
// (M << N typical); blobs > maxUntrackedFileSize are skipped.
func (g *GoGitVCSReader) loadHeadBlobs(entry *cachedRepo, pending []pendingTarget) map[plumbing.Hash][]byte {
	blobMap := make(map[plumbing.Hash][]byte, len(pending))
	// Deferred unlock: a panic partway through the loop would otherwise leave
	// entry.mu permanently held, deadlocking every future operation on this repo.
	entry.mu.Lock()
	defer entry.mu.Unlock()
	for _, t := range pending {
		if t.headHash == (plumbing.Hash{}) {
			continue
		}
		if _, already := blobMap[t.headHash]; already {
			continue
		}
		// Blob content is keyed by its content-addressed hash, so a cache hit
		// is always correct — no invalidation needed, ever. This skips
		// re-decompressing the same HEAD blob from the packfile on every poll
		// for a file that stays modified across several consecutive polls.
		if cached, ok := entry.blobCache.get(t.headHash); ok {
			blobMap[t.headHash] = cached
			atomic.AddInt64(&g.blobCacheHits, 1)
			continue
		}
		missStart := time.Now()
		data, ok := readBlobExact(entry.repo, t.headHash)
		if !ok {
			continue
		}
		blobMap[t.headHash] = data
		atomic.AddInt64(&g.blobCacheMisses, 1)
		atomic.AddInt64(&g.blobCacheMissNanos, int64(time.Since(missStart)))
		entry.blobCache.put(t.headHash, data, g.effectiveBlobCacheMaxBytes())
	}
	return blobMap
}

// readBlobExact reads a blob into one right-sized slice (the object header
// carries the size, so no growth steps and no second clone). Blobs larger than
// maxUntrackedFileSize are skipped to avoid reading large binaries or
// auto-generated files (e.g. package-lock.json, JARs).
func readBlobExact(repo *git.Repository, h plumbing.Hash) ([]byte, bool) {
	blob, err := repo.BlobObject(h)
	if err != nil || blob.Size > maxUntrackedFileSize {
		return nil, false
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, false
	}
	defer r.Close()
	data := make([]byte, blob.Size)
	n, _ := io.ReadFull(r, data)
	return data[:n], true
}

// applyDiff folds one pending target's line counts into d, storing the result
// in the memo when the target is keyed.
func (g *GoGitVCSReader) applyDiff(d *DiffStat, t pendingTarget, blobMap map[plumbing.Hash][]byte, memo *fileDiffMemo, root string) {
	d.Files++
	headData := blobMap[t.headHash] // nil for zero hash or skipped blobs
	if t.isDeleted {
		d.Deletions += countBytesLines(headData)
		return
	}
	bp, ok := readSmallFilePooled(filepath.Join(root, t.name))
	if !ok {
		d.Deletions += countBytesLines(headData)
		return
	}
	defer releaseFileBuf(bp)
	wtData := *bp
	// F7: surface when the LCS cap forces a "fully replaced" approximation
	// instead of an exact diff, so this is discoverable without reading code.
	// A file has at most len(bytes) lines, so the byte-length test skips the
	// newline counts for every file that cannot reach the cap.
	if len(headData) > maxLCSLines || len(wtData) > maxLCSLines {
		if oldLines, newLines := countBytesLines(headData), countBytesLines(wtData); oldLines > maxLCSLines || newLines > maxLCSLines {
			log.DebugLog().Printf("[GoGitVCSReader] LCS cap hit for %s (old=%d new=%d lines, cap=%d): reporting a fully-replaced approximation instead of an exact line diff", t.name, oldLines, newLines, maxLCSLines)
		}
	}
	var ins, del int
	if memo != nil {
		memo.diffs.Add(1)
		ins, del = linesDiffMyers(headData, wtData)
		// A head blob that failed to load would be diffed as empty; never
		// memoize that, or a transient read error becomes a sticky wrong answer.
		if _, headLoaded := blobMap[t.headHash]; t.keyed && (headLoaded || t.headHash == (plumbing.Hash{})) {
			memo.store(t.key, EditCounts{Insertions: ins, Deletions: del})
		}
	} else {
		ins, del = linesDiffBytes(headData, wtData)
	}
	d.Insertions += ins
	d.Deletions += del
}
