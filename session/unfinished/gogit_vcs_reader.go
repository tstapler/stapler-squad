package unfinished

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// diffStatCacheTTL is how long a DiffShortstat result is reused before re-scanning.
// The scanner runs every 30s per repo; a 30s TTL avoids redundant wt.Status() calls
// when multiple goroutines or scan cycles hit the same worktree path.
const diffStatCacheTTL = 30 * time.Second

// maxUntrackedFiles is the hard cap on untracked files walked per repo. Repos with
// large build-artifact trees (e.g. Android/Bazel projects with 200K+ .class/.dex files)
// would otherwise trigger an OOM by calling os.ReadFile on every file.
// Declared as var (not const) so tests can lower it without creating 10K files.
var maxUntrackedFiles = 10_000

// maxUntrackedFileSize is the per-file read limit for untracked line-counting.
// Files larger than this (binaries, JARs, APKs, etc.) have their file count
// incremented but their line count skipped. Also used for blob and working-tree
// file reads in applyDiff.
// Declared as var (not const) so tests can use smaller thresholds.
var maxUntrackedFileSize = int64(512 * 1024) // 512 KB

// errStopWalk is returned by walkUntrackedRec to signal early termination when
// the maxUntrackedFiles cap has been reached.
var errStopWalk = errors.New("stop untracked walk")

type diffStatEntry struct {
	result DiffStat
	expiry time.Time
}

// repoCacheMaxEntries is the maximum number of repositories held in repoCache
// before eviction runs. Sized for typical multi-repo workspaces; adjust higher
// only if scanning > 100 repos simultaneously.
const repoCacheMaxEntries = 100

// repoCacheTTL is the duration after which an unaccessed repo entry is evicted.
// 30 minutes covers the scanner's 30s cycle comfortably while bounding memory.
const repoCacheTTL = 30 * time.Minute

// cachedRepo holds an open go-git Repository and a per-repo mutex that
// serialises concurrent VCS reads. go-git's packfile reader is not safe for
// concurrent use; holding this mutex for the duration of a VCS operation
// eliminates the internal packfile-reader contention that was the #1 profiling
// hotspot (3.8B cycles, 36K events).
type cachedRepo struct {
	repo         *git.Repository
	mu           sync.Mutex
	accessedAtNs int64 // atomic UnixNano; updated on every cache hit
}

// pruneRepoCache evicts entries not accessed within repoCacheTTL, then trims
// the oldest entries if the cache still exceeds repoCacheMaxEntries.
// Designed to be called infrequently (e.g. when openRepoEntry detects overflow).
func (g *GoGitVCSReader) pruneRepoCache() {
	cutoff := time.Now().Add(-repoCacheTTL).UnixNano()
	type liveEntry struct {
		key          string
		accessedAtNs int64
	}
	var live []liveEntry
	g.repoCache.Range(func(k, v any) bool {
		entry := v.(*cachedRepo)
		ts := atomic.LoadInt64(&entry.accessedAtNs)
		if ts < cutoff {
			g.repoCache.Delete(k)
			atomic.AddInt64(&g.repoCacheSize, -1)
		} else {
			live = append(live, liveEntry{k.(string), ts})
		}
		return true
	})

	// LRU trim: if still over cap after TTL pass, evict coldest entries.
	if len(live) > repoCacheMaxEntries {
		slices.SortFunc(live, func(a, b liveEntry) int {
			return cmp.Compare(a.accessedAtNs, b.accessedAtNs)
		})
		for _, e := range live[:len(live)-repoCacheMaxEntries] {
			g.repoCache.Delete(e.key)
			atomic.AddInt64(&g.repoCacheSize, -1)
		}
	}
}

// GoGitVCSReader implements VCSReader using the go-git library.
// No subprocesses are spawned; all operations run in-process.
// Prefer this in environments where spawning git subprocesses is undesirable
// or where index.lock contention is a concern.
//
// All fields are zero-value safe — GoGitVCSReader{} is valid without a constructor.
type GoGitVCSReader struct {
	// repoCache caches open go-git Repository handles keyed by absolute path.
	// Values are *cachedRepo; LoadOrStore ensures only one entry per path wins.
	// Entries are evicted after repoCacheTTL of inactivity (pruneRepoCache).
	repoCache sync.Map // map[string]*cachedRepo

	// repoCacheSize tracks the approximate entry count atomically so eviction
	// can be triggered without a full Range scan on every cache miss.
	repoCacheSize int64

	// diffStatCache caches DiffShortstat results keyed by absolute worktreePath.
	// Values are diffStatEntry (stored by value; no mutation after Store).
	// Races on a cache miss are benign: last writer wins; both compute the same value.
	diffStatCache sync.Map // map[string]diffStatEntry
}

var _ VCSReader = (*GoGitVCSReader)(nil)

func (g *GoGitVCSReader) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	repo, err := g.openWorktree(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo %s: %w", repoPath, err)
	}

	// Main worktree.
	main := WorktreeInfo{Path: repoPath}
	if head, err := repo.Head(); err == nil {
		main.HEAD = head.Hash().String()
		if head.Name().IsBranch() {
			main.Branch = head.Name().Short()
		} else {
			main.IsDetached = true
		}
	}
	worktrees := []WorktreeInfo{main}

	// Linked worktrees live in $GIT_COMMON_DIR/worktrees/<name>/.
	// Use gitCommonDir to handle the case where repoPath is itself a linked worktree.
	worktreesDir := filepath.Join(gitCommonDir(repoPath), "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return worktrees, nil // no linked worktrees — not an error
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(worktreesDir, entry.Name())

		// gitdir file contains the absolute path to the worktree's .git file.
		gitdirData, err := os.ReadFile(filepath.Join(base, "gitdir"))
		if err != nil {
			continue
		}
		// Strip the trailing "/.git" to get the worktree path.
		wtPath := filepath.Dir(strings.TrimSpace(string(gitdirData)))

		wt := WorktreeInfo{Path: wtPath}

		// Read HEAD: either "ref: refs/heads/<branch>" or a bare SHA.
		headData, err := os.ReadFile(filepath.Join(base, "HEAD"))
		if err == nil {
			headStr := strings.TrimSpace(string(headData))
			const refPrefix = "ref: refs/heads/"
			if strings.HasPrefix(headStr, refPrefix) {
				wt.Branch = strings.TrimPrefix(headStr, refPrefix)
			} else {
				wt.IsDetached = true
				wt.HEAD = headStr
			}
		}

		if _, err := os.Stat(filepath.Join(base, "locked")); err == nil {
			wt.IsLocked = true
		}
		if _, err := os.Stat(filepath.Join(base, "gitdir")); err == nil {
			// Check prune flag.
			if _, err := os.Stat(wtPath); os.IsNotExist(err) {
				wt.IsPrunable = true
			}
		}

		worktrees = append(worktrees, wt)
	}
	return worktrees, nil
}

func (g *GoGitVCSReader) ResolveDefaultBranch(repoPath string) string {
	repo, err := g.openWorktree(repoPath)
	if err != nil {
		return ""
	}

	// Try refs/remotes/origin/HEAD first.
	if ref, err := repo.Reference("refs/remotes/origin/HEAD", true); err == nil {
		name := ref.Name().Short() // e.g. "origin/main"
		if name != "" {
			return name
		}
	}

	// Fall back to well-known remote tracking refs, then local.
	for _, candidate := range []string{
		"refs/remotes/origin/main", "refs/remotes/origin/master",
		"refs/remotes/origin/develop", "refs/remotes/origin/trunk",
		"refs/heads/main", "refs/heads/master",
		"refs/heads/develop", "refs/heads/trunk",
	} {
		if _, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			// Return the short name callers expect (e.g. "origin/main").
			short := plumbing.ReferenceName(candidate).Short()
			return short
		}
	}
	return ""
}

// HasUncommitted reports whether the worktree has any staged or unstaged changes.
//
// Strategy (no subprocess, low allocations):
//  1. Staged changes: compare index entry hashes against HEAD tree hashes — O(n)
//     hash comparisons, zero file I/O.
//  2. Working-tree changes: stat each tracked file and compare mtime/size against
//     the index record — O(n) stat calls, no file reads.
//
// This avoids the 1.85 GB allocation caused by wt.Status(), which hashes every
// modified file in full.
func (g *GoGitVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return false, fmt.Errorf("open repo %s: %w", worktreePath, err)
	}

	// Phase 1: go-git operations — hold the per-repo mutex for the full phase.
	// The mutex is NOT deferred so we can release it before the OS-only phase.
	entry.mu.Lock()
	repo := entry.repo

	idx, err := repo.Storer.Index()
	if err != nil {
		entry.mu.Unlock()
		return false, fmt.Errorf("read index: %w", err)
	}

	// --- staged changes: index vs HEAD (go-git, needs lock) ---
	headRef, headErr := repo.Head()
	if headErr == nil {
		headCommit, cerr := repo.CommitObject(headRef.Hash())
		if cerr != nil {
			entry.mu.Unlock()
			return false, fmt.Errorf("head commit: %w", cerr)
		}
		headTree, terr := headCommit.Tree()
		if terr != nil {
			entry.mu.Unlock()
			return false, fmt.Errorf("head tree: %w", terr)
		}

		// Walk the tree without loading blob content — only tree-entry hashes are needed.
		// object.FileIter.Next() calls GetBlob() per entry (loads full content); TreeWalker
		// loads only tree objects, which are orders of magnitude smaller.
		headHashes := make(map[string]plumbing.Hash, len(headTree.Entries))
		tw := object.NewTreeWalker(headTree, true, nil)
		for {
			name, te, twErr := tw.Next()
			if errors.Is(twErr, io.EOF) {
				break
			}
			if twErr != nil {
				tw.Close()
				entry.mu.Unlock()
				return false, fmt.Errorf("walk head tree: %w", twErr)
			}
			if te.Mode == filemode.Dir {
				continue
			}
			headHashes[name] = te.Hash
		}
		tw.Close()

		indexNames := make(map[string]bool, len(idx.Entries))
		for _, idxEntry := range idx.Entries {
			if idxEntry.Stage != 0 { // merge conflict stage → dirty
				entry.mu.Unlock()
				return true, nil
			}
			indexNames[idxEntry.Name] = true
			if h, ok := headHashes[idxEntry.Name]; !ok || h != idxEntry.Hash {
				entry.mu.Unlock()
				return true, nil // new or modified staged file
			}
		}
		for name := range headHashes {
			if !indexNames[name] {
				entry.mu.Unlock()
				return true, nil // staged deletion
			}
		}
	} else if !errors.Is(headErr, plumbing.ErrReferenceNotFound) {
		entry.mu.Unlock()
		return false, fmt.Errorf("head: %w", headErr)
	}

	// Capture index entries needed for the OS phase as plain value types.
	type trackedFile struct {
		name       string
		size       uint32
		modifiedAt time.Time
	}
	tracked := make([]trackedFile, len(idx.Entries))
	for i, idxEntry := range idx.Entries {
		tracked[i] = trackedFile{idxEntry.Name, idxEntry.Size, idxEntry.ModifiedAt}
	}
	entry.mu.Unlock() // release before all OS calls — no go-git access below

	// Phase 2: filesystem stat + directory walk — no go-git, no lock needed.
	indexed := make(map[string]bool, len(tracked))
	for _, tf := range tracked {
		indexed[tf.name] = true
		info, serr := os.Lstat(filepath.Join(worktreePath, tf.name))
		if serr != nil {
			if os.IsNotExist(serr) {
				return true, nil // tracked file deleted
			}
			continue
		}
		if info.Size() != int64(tf.size) ||
			!info.ModTime().Truncate(time.Second).Equal(tf.modifiedAt.Truncate(time.Second)) {
			return true, nil
		}
	}

	// Detect untracked files: walk working tree, skip .git directory.
	hasUntracked, err := hasUntrackedFiles(worktreePath, indexed)
	if err != nil {
		return false, err
	}
	return hasUntracked, nil
}

// hasUntrackedFiles reports whether any file under root is absent from the indexed set.
// It skips the .git directory and respects the .gitignore convention by not reading
// .gitignore files (callers that need full .gitignore support should use wt.Status()).
// For the mtime-stat approach this is a best-effort check sufficient for typical use.
func hasUntrackedFiles(root string, indexed map[string]bool) (bool, error) {
	return hasUntrackedFilesRec(root, root, indexed)
}

func hasUntrackedFilesRec(root, dir string, indexed map[string]bool) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if de.IsDir() {
			found, err := hasUntrackedFilesRec(root, full, indexed)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		} else if !indexed[rel] {
			return true, nil // untracked file
		}
	}
	return false, nil
}

// AheadBehind returns the number of commits by which worktreePath's HEAD is
// ahead of and behind the given base ref.
//
// Strategy (no subprocess): find the merge base with a BFS over each side,
// then count commits between each tip and the merge base. This bounds the
// walk to the diverged portion of history rather than the full reachable set.
func (g *GoGitVCSReader) AheadBehind(worktreePath, base string) (int, int, error) {
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return 0, 0, fmt.Errorf("open repo %s: %w", worktreePath, err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	repo := entry.repo

	headRef, err := repo.Head()
	if err != nil {
		return 0, 0, fmt.Errorf("head: %w", err)
	}

	baseHash, err := resolveRef(repo, base)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve base %q: %w", base, err)
	}

	if headRef.Hash() == baseHash {
		return 0, 0, nil
	}

	mb, err := findMergeBase(repo, headRef.Hash(), baseHash)
	if err != nil {
		return 0, 0, fmt.Errorf("merge base: %w", err)
	}

	ahead, err := countCommitsTo(repo, headRef.Hash(), mb)
	if err != nil {
		return 0, 0, err
	}
	behind, err := countCommitsTo(repo, baseHash, mb)
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

func (g *GoGitVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	repo := entry.repo

	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}

	baseHash, err := resolveRef(repo, base)
	if err != nil {
		return nil, err
	}

	// Collect commits reachable from HEAD but not from base.
	baseReachable, err := reachableSet(repo, baseHash)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var msgs []string
	err = iter.ForEach(func(c *object.Commit) error {
		if baseReachable[c.Hash] {
			return storer.ErrStop
		}
		if len(msgs) < max {
			// Mimic `git log --oneline`: short hash + first line of message.
			msgs = append(msgs, c.Hash.String()[:7]+" "+firstLine(c.Message))
		}
		return nil
	})
	return msgs, err
}

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
	result, err := g.diffShortstatUncached(worktreePath)
	if err == nil {
		g.diffStatCache.Store(worktreePath, diffStatEntry{
			result: result,
			expiry: time.Now().Add(diffStatCacheTTL),
		})
	}
	return result, err
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

	// Build HEAD tree hash map using TreeWalker — loads only tree objects, not blobs.
	headHashes := make(map[string]plumbing.Hash, len(idx.Entries))
	if headRef, headErr := repo.Head(); headErr == nil {
		if headCommit, cerr := repo.CommitObject(headRef.Hash()); cerr == nil {
			if headTree, terr := headCommit.Tree(); terr == nil {
				tw := object.NewTreeWalker(headTree, true, nil)
				for {
					name, te, twErr := tw.Next()
					if errors.Is(twErr, io.EOF) {
						break
					}
					if twErr != nil {
						break
					}
					if te.Mode == filemode.Dir {
						continue
					}
					headHashes[name] = te.Hash
				}
				tw.Close()
			}
		}
	}

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
		info, serr := os.Lstat(wtPath)
		if serr != nil {
			if os.IsNotExist(serr) {
				unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, true})
			}
			continue
		}
		if info.Size() != int64(m.size) ||
			!info.ModTime().Truncate(time.Second).Equal(m.modifiedAt.Truncate(time.Second)) {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
		}
	}

	// ── Phase 3: process each changed file inline — one blob at a time ───────
	// Loading a blob → reading working-tree file → computing diff → discarding,
	// before loading the next blob, keeps peak live memory proportional to the
	// largest single changed file rather than the sum of all changed-file blobs.
	var d DiffStat

	readBlobUnderLock := func(hash plumbing.Hash) []byte {
		if hash == (plumbing.Hash{}) {
			return nil
		}
		entry.mu.Lock()
		blob, berr := entry.repo.BlobObject(hash)
		if berr != nil {
			entry.mu.Unlock()
			return nil
		}
		// Skip blobs larger than maxUntrackedFileSize to avoid reading large
		// binaries or auto-generated files (e.g. package-lock.json, JARs).
		if blob.Size > maxUntrackedFileSize {
			entry.mu.Unlock()
			return nil
		}
		r, rerr := blob.Reader()
		if rerr != nil {
			entry.mu.Unlock()
			return nil
		}
		data, _ := io.ReadAll(r)
		_ = r.Close()
		entry.mu.Unlock()
		return data
	}

	applyDiff := func(t changeTarget) {
		d.Files++
		headData := readBlobUnderLock(t.headHash)
		if t.isDeleted {
			d.Deletions += countBytesLines(headData)
			return
		}
		wtData, ok := readFileIfSmall(filepath.Join(worktreePath, t.name))
		if !ok {
			d.Deletions += countBytesLines(headData)
			return
		}
		ins, del := linesDiffBytes(headData, wtData)
		d.Insertions += ins
		d.Deletions += del
		// headData and wtData are freed after this call returns.
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
	_ = walkUntracked(worktreePath, indexedNames, func(absPath string) {
		d.Files++
		if data, ok := readFileIfSmall(absPath); ok {
			d.Insertions += countBytesLines(data)
		}
	})

	return d, nil
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
	data, err := os.ReadFile(path) //nolint:gosec
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

// lcsLength computes the length of the longest common subsequence of two line slices.
// Uses O(n*m) DP — acceptable for typical source files.
func lcsLength(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
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
// Uses O(min(n,m)) space rolling DP, same algorithm as lcsLength.
func lcsLengthBytes(a, b [][]byte) int {
	if len(a) == 0 || len(b) == 0 {
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

// walkUntracked calls fn for every file under root that is not in indexed.
// Skips the .git directory; does not read .gitignore (best-effort for the scanner).
// Stops after maxUntrackedFiles files to avoid unbounded walks in large build trees.
func walkUntracked(root string, indexed map[string]struct{}, fn func(absPath string)) error {
	count := 0
	err := walkUntrackedRec(root, root, indexed, &count, fn)
	if errors.Is(err, errStopWalk) {
		return nil
	}
	return err
}

func walkUntrackedRec(root, dir string, indexed map[string]struct{}, count *int, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if de.IsDir() {
			if err := walkUntrackedRec(root, full, indexed, count, fn); err != nil {
				return err
			}
		} else if _, tracked := indexed[rel]; !tracked {
			*count++
			if *count > maxUntrackedFiles {
				return errStopWalk
			}
			fn(full)
		}
	}
	return nil
}

// gitCommonDir returns the path to the common git directory (the main .git dir),
// resolving through the .git file in linked worktrees.
func gitCommonDir(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	data, err := os.ReadFile(gitPath)
	if err != nil {
		// .git is a directory (or missing).
		return gitPath
	}
	// .git is a file: "gitdir: /abs/path/to/.git/worktrees/<name>\n"
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return gitPath
	}
	wtGitDir := strings.TrimPrefix(line, prefix)
	// Each per-worktree gitdir contains a "commondir" file pointing to the main .git.
	if cdData, err := os.ReadFile(filepath.Join(wtGitDir, "commondir")); err == nil {
		commondir := strings.TrimSpace(string(cdData))
		if !filepath.IsAbs(commondir) {
			commondir = filepath.Join(wtGitDir, commondir)
		}
		return commondir
	}
	return filepath.Dir(wtGitDir)
}

// openRepoEntry returns the cached *cachedRepo for path, opening it if needed.
// Uses LoadOrStore so that only one *cachedRepo is ever stored per path even
// when multiple goroutines race on the first access.
// Access timestamps are updated atomically on every hit so pruneRepoCache can
// evict cold entries without interrupting concurrent readers.
func (g *GoGitVCSReader) openRepoEntry(path string) (*cachedRepo, error) {
	now := time.Now().UnixNano()
	if v, ok := g.repoCache.Load(path); ok {
		entry := v.(*cachedRepo)
		atomic.StoreInt64(&entry.accessedAtNs, now)
		return entry, nil
	}

	// Trigger eviction before adding a new entry if the cache is large.
	if atomic.LoadInt64(&g.repoCacheSize) >= repoCacheMaxEntries {
		g.pruneRepoCache()
		// Re-check after eviction — another goroutine may have stored this path.
		if v, ok := g.repoCache.Load(path); ok {
			entry := v.(*cachedRepo)
			atomic.StoreInt64(&entry.accessedAtNs, now)
			return entry, nil
		}
	}

	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("plain open %s: %w", path, err)
	}
	entry := &cachedRepo{repo: repo, accessedAtNs: now}
	actual, loaded := g.repoCache.LoadOrStore(path, entry)
	if !loaded {
		atomic.AddInt64(&g.repoCacheSize, 1)
	}
	return actual.(*cachedRepo), nil
}

// openWorktree opens a git repo that may be a linked worktree (has a .git file
// rather than a .git directory). It returns the cached *git.Repository.
// Callers that perform heavy VCS work should use openRepoEntry directly so
// they can hold the per-repo mutex for the duration of their operation.
func (g *GoGitVCSReader) openWorktree(path string) (*git.Repository, error) {
	entry, err := g.openRepoEntry(path)
	if err != nil {
		return nil, err
	}
	return entry.repo, nil
}

// resolveRef resolves a short ref name (e.g. "origin/main") to a commit hash.
func resolveRef(repo *git.Repository, name string) (plumbing.Hash, error) {
	// Try as a full or short reference name.
	for _, candidate := range []string{
		name,
		"refs/remotes/" + name,
		"refs/heads/" + name,
	} {
		if ref, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			return ref.Hash(), nil
		}
	}
	// Try as a literal hash.
	h := plumbing.NewHash(name)
	if !h.IsZero() {
		return h, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("cannot resolve ref %q", name)
}

// reachableSet returns the set of all commits reachable from start.
func reachableSet(repo *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool, 64)
	iter, err := repo.Log(&git.LogOptions{From: start})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if err := iter.ForEach(func(c *object.Commit) error {
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return nil, err
	}
	return seen, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return strings.TrimSpace(s)
}

// mergeBaseBFSLimit caps the number of commits visited per side in findMergeBase.
// For a 10K-commit repo the unbounded walk allocates ~640 KB; bounding at 2000
// limits that to ~128 KB while still covering typical branch divergences.
const mergeBaseBFSLimit = 2000

// findMergeBase returns the most-recent common ancestor of h1 and h2 using BFS.
// It first marks ancestors of h1 (up to mergeBaseBFSLimit), then walks ancestors
// of h2 until it finds one that is already marked. If no merge base is found
// within the depth limit, a sentinel error is returned.
func findMergeBase(repo *git.Repository, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}

	// Mark ancestors of h1 (bounded).
	anc := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q := []plumbing.Hash{h1}
	visited := 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if anc[h] {
			continue
		}
		anc[h] = true
		visited++
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue // object missing from shallow clone or pack — skip
		}
		q = append(q, c.ParentHashes...)
	}

	// Walk from h2 breadth-first; first ancestor in anc is the nearest merge base.
	seen := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q = []plumbing.Hash{h2}
	visited = 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[0]
		q = q[1:]
		if seen[h] {
			continue
		}
		seen[h] = true
		visited++
		if anc[h] {
			return h, nil
		}
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue
		}
		q = append(q, c.ParentHashes...)
	}
	return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", mergeBaseBFSLimit)
}

// countCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive).
func countCommitsTo(repo *git.Repository, start, stop plumbing.Hash) (int, error) {
	seen := make(map[plumbing.Hash]bool, 32)
	q := []plumbing.Hash{start}
	n := 0
	for len(q) > 0 {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if seen[h] || h == stop {
			continue
		}
		seen[h] = true
		n++
		c, err := repo.CommitObject(h)
		if err != nil {
			return 0, err
		}
		for _, p := range c.ParentHashes {
			if !seen[p] && p != stop {
				q = append(q, p)
			}
		}
	}
	return n, nil
}
