package unfinished

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/tstapler/stapler-squad/log"
)

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
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return DiffStat{}, err
	}

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

	// Classify index entries: staged-changed (index hash ≠ HEAD hash) vs stable.
	type indexMeta struct {
		name       string
		indexHash  plumbing.Hash
		headHash   plumbing.Hash // zero if not in HEAD
		size       uint32
		modifiedAt time.Time
		staged     bool
	}
	metas := make([]indexMeta, 0, len(idx.Entries))
	indexedNames := make(map[string]struct{}, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.Stage != 0 {
			continue
		}
		indexedNames[e.Name] = struct{}{}
		headHash := headHashes[e.Name]
		metas = append(metas, indexMeta{
			name:       e.Name,
			indexHash:  e.Hash,
			headHash:   headHash,
			size:       e.Size,
			modifiedAt: e.ModifiedAt,
			staged:     e.Hash != headHash,
		})
	}

	// Staged deletions: files in HEAD but absent from index.
	type stagedDel struct {
		name     string
		headHash plumbing.Hash
	}
	var stagedDels []stagedDel
	for name, headHash := range headHashes {
		if _, inIdx := indexedNames[name]; !inIdx {
			stagedDels = append(stagedDels, stagedDel{name, headHash})
		}
	}

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
	type changeTarget struct {
		name      string
		headHash  plumbing.Hash // zero = new file not in HEAD
		isDeleted bool          // no working-tree file exists
	}

	stagedTargets := make([]changeTarget, 0, len(stagedDels))
	for _, m := range metas {
		if m.staged {
			stagedTargets = append(stagedTargets, changeTarget{m.name, m.headHash, false})
		}
	}
	for _, sd := range stagedDels {
		stagedTargets = append(stagedTargets, changeTarget{sd.name, sd.headHash, true})
	}

	unstagedTargets := make([]changeTarget, 0)
	for _, m := range metas {
		if m.staged {
			continue
		}
		wtPath := filepath.Join(worktreePath, m.name)
		if g.statFn != nil {
			g.statFn(m.name)
		}
		info, serr := os.Lstat(wtPath)
		if serr != nil {
			if os.IsNotExist(serr) {
				unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, true})
			}
			continue
		}
		sizeDiffers := info.Size() != int64(m.size)
		mtimeDiffers := !info.ModTime().Truncate(time.Second).Equal(m.modifiedAt.Truncate(time.Second))
		if sizeDiffers || mtimeDiffers {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}

		// Racy-clean case: size matches the index entry AND the working-tree
		// mtime (truncated to second) equals the index entry's recorded mtime.
		// stat alone cannot prove the file is unchanged in the racy window —
		// a file rewritten with identical size within the same wall-clock
		// second as the index write looks clean by stat but may differ in
		// content (the classic "racy git" problem). Outside that window
		// (file mtime clearly predates the index write), stat alone is
		// sufficient proof of no change, exactly as real git treats it — so
		// only pay for a content hash when the file could actually be racy.
		if !fileNeedsContentCheck(info.ModTime(), indexMtime) {
			continue // stat-clean and outside the racy window: confirmed unchanged
		}
		if info.Size() > maxUntrackedFileSize {
			// Too large to hash within the cap; conservatively treat as changed
			// rather than reading it into memory, consistent with how large
			// modified tracked files are handled elsewhere (applyDiff skips the
			// read but still counts the file via readFileIfSmall returning false).
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}
		wtData, ok := readFileIfSmall(wtPath)
		if !ok {
			// Unreadable or raced past the cap; treat as changed conservatively.
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}
		if plumbing.ComputeHash(plumbing.BlobObject, wtData) != m.indexHash {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
		}
	}

	// ── Phase 3: batch-read all needed blobs under a single lock hold ────────
	// Collecting all hashes first and reading them in one critical section
	// eliminates the N lock-acquire/release cycles (one per changed file) that
	// was the #2 mutex hotspot: 9.87B cycles, 1641 events in pprof profiling.
	// Peak memory is bounded by the sum of changed-file blobs (M << N typical),
	// which is acceptable since blobs > maxUntrackedFileSize are still skipped.
	allTargets := append(stagedTargets, unstagedTargets...) //nolint:gocritic // intentional ephemeral append
	blobMap := make(map[plumbing.Hash][]byte, len(allTargets))
	func() {
		// Deferred unlock (rather than an explicit entry.mu.Unlock() at the
		// end of this block): a panic partway through the loop below would
		// otherwise leave entry.mu permanently held, deadlocking every future
		// operation on this repo entry.
		entry.mu.Lock()
		defer entry.mu.Unlock()
		for _, t := range allTargets {
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
			blob, berr := entry.repo.BlobObject(t.headHash)
			if berr != nil {
				continue
			}
			// Skip blobs larger than maxUntrackedFileSize to avoid reading large
			// binaries or auto-generated files (e.g. package-lock.json, JARs).
			if blob.Size > maxUntrackedFileSize {
				continue
			}
			r, rerr := blob.Reader()
			if rerr != nil {
				continue
			}
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			_ = r.Close()
			data := bytes.Clone(buf.Bytes())
			blobMap[t.headHash] = data
			atomic.AddInt64(&g.blobCacheMisses, 1)
			atomic.AddInt64(&g.blobCacheMissNanos, int64(time.Since(missStart)))
			entry.blobCache.put(t.headHash, data, g.effectiveBlobCacheMaxBytes())
		}
	}()

	var d DiffStat

	applyDiff := func(t changeTarget) {
		d.Files++
		headData := blobMap[t.headHash] // nil for zero hash or skipped blobs
		if t.isDeleted {
			d.Deletions += countBytesLines(headData)
			return
		}
		wtData, ok := readFileIfSmall(filepath.Join(worktreePath, t.name))
		if !ok {
			d.Deletions += countBytesLines(headData)
			return
		}
		// F7: surface when the LCS cap forces a "fully replaced" approximation
		// instead of an exact diff, so this is discoverable without reading code.
		if oldLines, newLines := countBytesLines(headData), countBytesLines(wtData); oldLines > maxLCSLines || newLines > maxLCSLines {
			log.DebugLog().Printf("[GoGitVCSReader] LCS cap hit for %s (old=%d new=%d lines, cap=%d): reporting a fully-replaced approximation instead of an exact line diff", t.name, oldLines, newLines, maxLCSLines)
		}
		ins, del := linesDiffBytes(headData, wtData)
		d.Insertions += ins
		d.Deletions += del
	}

	for _, t := range stagedTargets {
		applyDiff(t)
	}
	for _, t := range unstagedTargets {
		applyDiff(t)
	}

	// Untracked files: count lines as pure insertions (no HEAD version).
	// Limited to maxUntrackedFiles to avoid OOM on repos with large build-artifact trees.
	// Files over maxUntrackedFileSize are counted but not read for line counting.
	// gitignore-matched paths (node_modules/, build output, etc.) are skipped
	// entirely rather than walked and read — this was previously the single
	// largest allocator in the app (~163GB cum in one profiling session),
	// since ignored build trees were read just like any other untracked file.
	// untrackedMatcher was resolved in Phase 1 above (cached on entry, F6).
	_ = walkUntracked(worktreePath, indexedNames, untrackedMatcher, func(absPath string) {
		d.Files++
		if data, ok := readFileIfSmall(absPath); ok {
			d.Insertions += countBytesLines(data)
		}
	})

	return d, nil
}

// fileNeedsContentCheck reports whether a stat-clean file (size and mtime both
// matching the index entry) still requires a content-hash fallback to rule
// out the racy-git case. A file is only racy if its own mtime is not clearly
// before the index was last written — the same window real git uses — so a
// file whose mtime predates the index write is safe to trust from stat alone.
// An unknown indexMtime (zero value) is treated conservatively as racy.
func fileNeedsContentCheck(fileMtime, indexMtime time.Time) bool {
	if indexMtime.IsZero() {
		return true
	}
	return !fileMtime.Truncate(time.Second).Before(indexMtime.Truncate(time.Second))
}

// readFileIfSmall reads the file at path only if its size is ≤ maxUntrackedFileSize.
// Returns (data, true) on success; (nil, false) when the file is too large or unreadable.
// This is the single safe wrapper for all content reads in diffShortstatUncached —
// callers should use this instead of os.ReadFile to prevent OOM on large build artifacts.
func readFileIfSmall(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.Size() > maxUntrackedFileSize {
		return nil, false
	}
	// path is walked from repoPath by walkUntrackedFiles (an internal directory
	// walk over the session's own git worktree), not user/RPC input.
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from an internal directory walk over the worktree, not user input
	if err != nil {
		return nil, false
	}
	return data, true
}

// LinesDiff returns inserted and deleted line counts between old and new using LCS.
// Exported so tests can exercise the algorithm directly.
func LinesDiff(old, newContent string) (insertions, deletions int) {
	oldLines := splitLines(old)
	newLines := splitLines(newContent)
	lcs := lcsLength(oldLines, newLines)
	return len(newLines) - lcs, len(oldLines) - lcs
}

// maxLCSLines caps the input size to the O(n*m) LCS DP below. A file well
// under maxUntrackedFileSize in bytes can still contain hundreds of
// thousands of lines (e.g. mostly-blank or single-character lines), and
// n×m cells at that scale turns a shortstat poll into minutes of CPU on one
// goroutine. Beyond this cap, lcsLength/lcsLengthBytes return 0, which the
// existing len(new)-lcs / len(old)-lcs formulas turn into a "fully replaced"
// approximation — bounded and cheap instead of exact and unbounded.
// Cost at the cap: 20,000 × 20,000 = 4×10^8 DP-cell comparisons — on the
// order of a few seconds of CPU per file at this size, even though the
// rolling two-row implementation keeps memory at O(min(n,m)) (~160 KB of
// ints), not O(n*m); it's the CPU time, not memory, that this cap bounds.
// Declared as var (not const) so tests can lower it without generating huge fixtures.
var maxLCSLines = 20_000

// lcsLength computes the length of the longest common subsequence of two line slices.
// Uses O(n*m) DP — acceptable for typical source files, capped by maxLCSLines.
func lcsLength(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > maxLCSLines || len(b) > maxLCSLines {
		return 0
	}
	// Use two rows to keep memory O(min(n,m)).
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return prev[len(b)]
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Drop the empty string that results from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// countBytesLines counts the number of lines in data by counting '\n' bytes,
// treating a trailing newline as not creating an extra empty line.
func countBytesLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++ // last line without terminating newline
	}
	return n
}

// linesDiffBytes returns LCS-based insertion and deletion counts between old and new,
// operating directly on []byte to avoid string conversion overhead.
func linesDiffBytes(old, newData []byte) (insertions, deletions int) {
	a := splitLinesBytes(old)
	b := splitLinesBytes(newData)
	lcs := lcsLengthBytes(a, b)
	return len(b) - lcs, len(a) - lcs
}

// splitLinesBytes splits data into lines as subslices of data (no allocation per line).
func splitLinesBytes(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lcsLengthBytes computes the LCS length of two byte-slice-of-lines sequences.
// Uses O(min(n,m)) space rolling DP, same algorithm as lcsLength, capped by maxLCSLines.
func lcsLengthBytes(a, b [][]byte) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > maxLCSLines || len(b) > maxLCSLines {
		return 0
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if bytes.Equal(a[i-1], b[j-1]) {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return prev[len(b)]
}
