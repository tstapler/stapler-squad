package unfinished

import (
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// parentSource supplies commit parents and committer times to the merge-base
// and counting walks. parentMemo is the per-call implementation.
type parentSource interface {
	info(h plumbing.Hash) (commitInfo, error)
	get(h plumbing.Hash) ([]plumbing.Hash, error)
}

var _ parentSource = (*parentMemo)(nil)

// aheadBehindLocked computes (ahead, behind) for entry's HEAD against key.base,
// reusing or storing the TTL cache entry. The caller holds entry.mu across the
// whole call (the legacy path; the shared path will call aheadBehindFrom
// without it).
func (g *GoGitVCSReader) aheadBehindLocked(entry *cachedRepo, key aheadBehindKey) (int, int, error) {
	head, baseHash, err := resolveAheadBehindRefs(entry, key.base)
	if err != nil {
		return 0, 0, err
	}
	if head == baseHash {
		g.aheadBehindCache.Store(key, aheadBehindEntry{headHash: head, baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
		return 0, 0, nil
	}
	// A TTL-expired entry whose head/base hashes still match the current
	// ones means neither ref moved since the last computation — the BFS
	// result can't have changed, so reuse it instead of re-running
	// findMergeBase + two countCommitsTo walks (the actual expensive work;
	// resolving HEAD/base above is cheap by comparison).
	if e, ok := g.aheadBehindCache.Load(key); ok && e.headHash == head && e.baseHash == baseHash {
		e.expiry = time.Now().Add(diffStatCacheTTL)
		g.aheadBehindCache.Store(key, e)
		return e.ahead, e.behind, nil
	}
	ahead, behind, err := aheadBehindFrom(newParentMemo(entry.repo), head, baseHash)
	if err != nil {
		return 0, 0, err
	}
	g.aheadBehindCache.Store(key, aheadBehindEntry{ahead: ahead, behind: behind, headHash: head, baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
	return ahead, behind, nil
}

// resolveAheadBehindRefs is phase 1: HEAD and base as hashes. The caller holds
// entry.mu; no commit graph is walked here.
func resolveAheadBehindRefs(entry *cachedRepo, base string) (head, baseHash plumbing.Hash, err error) {
	headRef, headErr := entry.repo.Head()
	if headErr != nil {
		return head, baseHash, fmt.Errorf("head: %w", headErr)
	}
	baseHash, baseErr := resolveRef(entry.repo, base)
	if baseErr != nil {
		return head, baseHash, fmt.Errorf("resolve base %q: %w", base, baseErr)
	}
	return headRef.Hash(), baseHash, nil
}

// aheadBehindFrom is phase 2: a pure function of the commit graph behind src.
// It reads nothing from any cachedRepo.
func aheadBehindFrom(src parentSource, head, base plumbing.Hash) (ahead, behind int, err error) {
	mb, mbErr := findMergeBase(src, head, base)
	if mbErr != nil {
		return 0, 0, fmt.Errorf("merge base: %w", mbErr)
	}
	if ahead, err = countCommitsTo(src, head, mb); err != nil {
		return 0, 0, err
	}
	if behind, err = countCommitsTo(src, base, mb); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// resolveCommitMessageRefs snapshots HEAD and base as plain hash values under a
// brief entry.mu, so the walks that follow never hold it.
func resolveCommitMessageRefs(entry *cachedRepo, base string) (head, baseHash plumbing.Hash, err error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	headRef, headErr := entry.repo.Head()
	if headErr != nil {
		return head, baseHash, headErr
	}
	baseHash, err = resolveRef(entry.repo, base)
	return headRef.Hash(), baseHash, err
}

// commitMessagesFrom lists up to max "short-hash subject" lines from head back
// to the first commit already in baseReachable. go-git's packfile reader needs
// entry.mu, so it is held for the walk.
func commitMessagesFrom(entry *cachedRepo, head plumbing.Hash, baseReachable map[plumbing.Hash]bool, max int) ([]string, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	iter, err := entry.repo.Log(&git.LogOptions{From: head})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var result []string
	err = iter.ForEach(func(c *object.Commit) error {
		if baseReachable[c.Hash] {
			return storer.ErrStop
		}
		if len(result) < max {
			// Mimic `git log --oneline`: short hash + first line of message.
			result = append(result, c.Hash.String()[:7]+" "+firstLine(c.Message))
		}
		return nil
	})
	return result, err
}
