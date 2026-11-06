package unfinished

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// parsedIndex is the immutable, shareable result of decoding one worktree's
// index and comparing it with HEAD, keyed by the index's stat signature
// (Story 4.2.2). It removes the O(n) decode, the staged-vs-HEAD compare and
// the per-call tracked-file copies when nothing changed. It never removes the
// per-file stat pass: an in-place edit of a tracked file changes no index
// bytes (ADR-006 hard rule). Readers must not mutate tracked or indexed.
type parsedIndex struct {
	sig        statSig
	observedAt time.Time // clock reading taken before the signature stat
	headTree   plumbing.Hash

	// stagedDirty is true when the index differs from HEAD (conflict, staged
	// add/modify, staged delete); tracked and indexed are then nil.
	stagedDirty bool
	tracked     []trackedFile
	indexed     map[string]struct{}

	accessedAtNs atomic.Int64
}

// trackedGen is the tracked-set generation handed to the untracked-walk
// cache: the signature observed before the index was read, so a rewrite after
// the read cannot be paired with a stale tracked set. The walk adds the entry
// count of the set it uses to the cache key.
type trackedGen struct {
	sig        statSig
	observedAt time.Time
	ok         bool
}

func (p *parsedIndex) gen() trackedGen {
	return trackedGen{sig: p.sig, observedAt: p.observedAt, ok: !p.stagedDirty && p.sig != (statSig{})}
}

// currentTrackedGen stats the worktree's index now. Call it before reading
// the index so the signature can only predate the data it describes.
func (g *GoGitVCSReader) currentTrackedGen(worktreePath string) trackedGen {
	if !g.effectiveFlags().ScanSkip {
		return trackedGen{}
	}
	observedAt := g.scanSkip.now()
	sig, ok := statPathSig(filepath.Join(worktreeGitDir(worktreePath), "index"))
	return trackedGen{sig: sig, observedAt: observedAt, ok: ok}
}

// hasUncommittedIndexPhaseCached is hasUncommittedGoGitPhase with the parsed
// index cached by IndexStatSig. force (a DirtyGen full walk) bypasses the
// cached copy but still refreshes it. Acquires and releases entry.mu.
func (g *GoGitVCSReader) hasUncommittedIndexPhaseCached(entry *cachedRepo, worktreePath string, force bool) (*parsedIndex, gitignore.Matcher, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	repo := entry.repo
	headHashes, headErr := resolveHeadTreeHashes(g, entry, repo)
	if headErr != nil {
		return nil, nil, headErr
	}
	matcher := g.getOrBuildUntrackedMatcher(entry, worktreePath)
	var headTree plumbing.Hash
	if headHashes != nil {
		headTree = entry.headTreeHash
	}

	st := &g.scanSkip
	observedAt := st.now()
	sig, sigOK := statPathSig(filepath.Join(worktreeGitDir(worktreePath), "index"))
	if sigOK && !force {
		if pi, ok := st.indexCache.Load(worktreePath); ok && pi.sig == sig && pi.headTree == headTree {
			if sig.trustedAt(pi.observedAt) && observedAt.Sub(pi.observedAt) < scanSkipMaxAge {
				st.stats.indexHits.Add(1)
				pi.accessedAtNs.Store(observedAt.UnixNano())
				return pi, matcher, nil
			}
			st.stats.indexRacySkips.Add(1)
		}
	}
	st.stats.indexMisses.Add(1)

	idx, idxErr := repo.Storer.Index()
	st.stats.indexDecodes.Add(1)
	if idxErr != nil {
		return nil, nil, fmt.Errorf("read index: %w", idxErr)
	}
	pi := &parsedIndex{sig: sig, observedAt: observedAt, headTree: headTree}
	pi.accessedAtNs.Store(observedAt.UnixNano())
	if headHashes != nil { // nil means no HEAD yet (unborn branch): nothing staged to compare
		pi.stagedDirty = stagedDiffersFromHead(idx.Entries, headHashes)
	}
	if !pi.stagedDirty {
		pi.tracked = make([]trackedFile, len(idx.Entries))
		pi.indexed = make(map[string]struct{}, len(idx.Entries))
		for i, e := range idx.Entries {
			pi.tracked[i] = trackedFile{name: e.Name, size: e.Size, modifiedAt: e.ModifiedAt, skipWorktree: e.SkipWorktree}
			pi.indexed[e.Name] = struct{}{}
		}
	}
	if sigOK {
		st.indexCache.Store(worktreePath, pi)
	}
	return pi, matcher, nil
}

// stagedDiffersFromHead reports whether the index differs from HEAD: a
// conflict stage, a staged add or modify, or a staged deletion. Same
// verdicts as the legacy compare in hasUncommittedGoGitPhase.
func stagedDiffersFromHead(entries []*index.Entry, headHashes map[string]plumbing.Hash) bool {
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Stage != 0 {
			return true
		}
		names[e.Name] = true
		if h, ok := headHashes[e.Name]; !ok || h != e.Hash {
			return true
		}
	}
	for name := range headHashes {
		if !names[name] {
			return true
		}
	}
	return false
}
