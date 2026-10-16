package unfinished

import (
	"errors"
	"fmt"
	"io/fs"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// trackedFile holds the index metadata for a single tracked file needed by the
// OS stat phase of HasUncommitted.
type trackedFile struct {
	name       string
	size       uint32
	modifiedAt time.Time
	// skipWorktree marks a sparse-checkout entry; set only by the scan_skip index cache.
	skipWorktree bool
}

// hasUncommittedGoGitPhase runs the go-git index phase of HasUncommitted.
// Acquires and releases entry.mu via defer. Returns tracked file slice + dirty flag.
// MUST NOT be called with entry.mu already held — Go mutexes are not reentrant.
//
// Returns:
//   - tracked: slice of index entries needed for the OS stat phase (lock released before caller uses these)
//   - dirty: true if dirty was determined from go-git alone
//   - dirtyKnown: true if dirty result is definitive (caller should skip OS phase)
//   - matcher: cached gitignore matcher, built under entry.mu here since Phase 3
//     (the untracked-files walk) runs without the lock held
//   - err: any error encountered
func (g *GoGitVCSReader) hasUncommittedGoGitPhase(entry *cachedRepo, worktreePath string) (tracked []trackedFile, dirty bool, dirtyKnown bool, matcher gitignore.Matcher, err error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	repo := entry.repo

	idx, idxErr := repo.Storer.Index()
	if idxErr != nil {
		return nil, false, false, nil, fmt.Errorf("read index: %w", idxErr)
	}

	// --- staged changes: index vs HEAD (go-git, needs lock) ---
	headHashes, headErr := resolveHeadTreeHashes(g, entry, repo)
	if headErr != nil {
		return nil, false, false, nil, headErr
	}
	matcher = g.getOrBuildUntrackedMatcher(entry, worktreePath)
	if headHashes != nil { // nil means no HEAD yet (unborn branch) — nothing staged to compare
		indexNames := make(map[string]bool, len(idx.Entries))
		for _, idxEntry := range idx.Entries {
			if idxEntry.Stage != 0 { // merge conflict stage → dirty
				return nil, true, true, matcher, nil
			}
			indexNames[idxEntry.Name] = true
			if h, ok := headHashes[idxEntry.Name]; !ok || h != idxEntry.Hash {
				return nil, true, true, matcher, nil // new or modified staged file
			}
		}
		for name := range headHashes {
			if !indexNames[name] {
				return nil, true, true, matcher, nil // staged deletion
			}
		}
	}

	// Capture index entries needed for the OS phase as plain value types.
	// entry.mu is released by defer when this function returns — OS stat work
	// must happen in the caller after this function returns.
	result := make([]trackedFile, len(idx.Entries))
	for i, idxEntry := range idx.Entries {
		result[i] = trackedFile{name: idxEntry.Name, size: idxEntry.Size, modifiedAt: idxEntry.ModifiedAt}
	}
	return result, false, false, matcher, nil
}

func (g *GoGitVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	if v, ok := g.hasUncommittedCache.Load(worktreePath); ok {
		if e := v.(hasUncommittedEntry); time.Now().Before(e.expiry) {
			return e.result, nil
		}
	}

	// After the cache check: the guard allocates, and a hit must stay alloc-free.
	defer g.guardInvalidation(worktreePath)()
	result, sfErr := sfDo(&g.hasUncommittedSF, worktreePath, func() (bool, error) {
		entry, err := g.openRepoEntry(worktreePath)
		if err != nil {
			return false, fmt.Errorf("open repo %s: %w", worktreePath, err)
		}

		if g.effectiveFlags().ScanSkip {
			return g.hasUncommittedSkip(entry, worktreePath)
		}

		// Phase 1: go-git index phase — entry.mu is held only inside this call.
		// The gitignore matcher is also built/cached here (under entry.mu) so
		// Phase 3 below can consume it without needing the lock itself.
		tracked, dirty, dirtyKnown, matcher, err := g.hasUncommittedGoGitPhase(entry, worktreePath)
		if err != nil {
			return false, err
		}
		if dirtyKnown {
			g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
				result: dirty, expiry: time.Now().Add(diffStatCacheTTL),
			})
			return dirty, nil
		}

		// Phase 2: OS stat walk — entry.mu is NOT held here (released by hasUncommittedGoGitPhase).
		indexedMap := make(map[string]struct{}, len(tracked))
		stat := newDirStatter(worktreePath)
		defer stat.close()
		for _, tf := range tracked {
			indexedMap[tf.name] = struct{}{}
			g.noteStat(tf.name)
			size, mtime, serr := stat.lstat(tf.name)
			if serr != nil {
				if errors.Is(serr, fs.ErrNotExist) {
					r := true // tracked file deleted
					g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
						result: r, expiry: time.Now().Add(diffStatCacheTTL),
					})
					return r, nil
				}
				continue
			}
			if size != int64(tf.size) || !mtime.Equal(tf.modifiedAt) {
				r := true
				g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
					result: r, expiry: time.Now().Add(diffStatCacheTTL),
				})
				return r, nil
			}
		}

		// Phase 3: untracked files walk — no lock held. matcher was built under
		// entry.mu in Phase 1 above and is safe to read here (a plain local copy).
		r, err := hasUntrackedFiles(worktreePath, indexedMap, matcher)
		if err != nil {
			return false, err
		}
		g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
			result: r, expiry: time.Now().Add(diffStatCacheTTL),
		})
		return r, nil
	})
	if sfErr != nil {
		return false, sfErr
	}
	return result, nil
}

// getOrBuildUntrackedMatcher returns the cached gitignore matcher for entry,
// rebuilding it if resolveHeadTreeHashes invalidated it since the last build.
// MUST be called with entry.mu already held (mirrors the caching contract of
// entry.headTreeCache above — both are entry-scoped fields protected by entry.mu).
func (g *GoGitVCSReader) getOrBuildUntrackedMatcher(entry *cachedRepo, worktreePath string) gitignore.Matcher {
	if entry.untrackedMatcherBuilt {
		return entry.untrackedMatcher
	}

	// Check the decoupled cache before paying for a rebuild: entry may be a
	// freshly (re)opened cachedRepo whose predecessor was evicted from
	// repoCache purely for memory pressure, with HEAD never having moved.
	if cached, ok := g.matcherCache.Load(worktreePath); ok {
		mc, _ := cached.(*matcherCacheEntry)
		if mc != nil && mc.headTreeHash == entry.headTreeHash {
			atomic.StoreInt64(&mc.accessedAtNs, time.Now().UnixNano())
			entry.untrackedMatcher = mc.matcher
			entry.untrackedMatcherBuilt = true
			return entry.untrackedMatcher
		}
	}

	atomic.AddInt64(&g.gitignoreMatcherRebuilds, 1)
	var m gitignore.Matcher
	if patterns, ignErr := gitignore.ReadPatterns(osfs.New(worktreePath), nil); ignErr == nil {
		m = gitignore.NewMatcher(patterns)
	}
	entry.untrackedMatcher = m
	entry.untrackedMatcherBuilt = true
	g.matcherCache.Store(worktreePath, &matcherCacheEntry{
		matcher:      m,
		headTreeHash: entry.headTreeHash,
		accessedAtNs: time.Now().UnixNano(),
	})
	return m
}
