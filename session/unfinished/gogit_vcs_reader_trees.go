package unfinished

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// resolveHeadTreeHashes returns the name->blob-hash map for repo's current HEAD
// tree, using entry.headTreeCache when it's still valid for the current HEAD.
// Both hasUncommittedGoGitPhase and diffShortstatUncached need the identical
// map for the same HEAD commit, and HEAD moves far less often than either is
// called, so the two callers share this cache instead of each walking the
// HEAD tree on every invocation.
//
// Callers must already hold entry.mu. Returns (nil, nil) if there is no HEAD
// yet (unborn branch / empty repo) — callers should treat that the same as
// an empty tree, not as an error.
func resolveHeadTreeHashes(g *GoGitVCSReader, entry *cachedRepo, repo *git.Repository) (map[string]plumbing.Hash, error) {
	headRef, headErr := repo.Head()
	if headErr != nil {
		if errors.Is(headErr, plumbing.ErrReferenceNotFound) {
			return nil, nil //nolint:nilnil // documented sentinel: no HEAD yet (unborn branch) is a valid, non-error "empty tree" result — see the doc comment above.
		}
		return nil, fmt.Errorf("head: %w", headErr)
	}
	headHash := headRef.Hash()
	if entry.headTreeHash == headHash && entry.headTreeCache != nil {
		return entry.headTreeCache, nil
	}
	headCommit, cerr := repo.CommitObject(headHash)
	if cerr != nil {
		return nil, fmt.Errorf("head commit: %w", cerr)
	}
	headTree, terr := headCommit.Tree()
	if terr != nil {
		return nil, fmt.Errorf("head tree: %w", terr)
	}

	// Walk the tree without loading blob content — only tree-entry hashes are needed.
	// object.FileIter.Next() calls GetBlob() per entry (loads full content); TreeWalker
	// loads only tree objects, which are orders of magnitude smaller.
	headHashes := make(map[string]plumbing.Hash, len(headTree.Entries))
	tw := object.NewTreeWalker(headTree, true, nil)
	defer tw.Close()
	for {
		name, te, twErr := tw.Next()
		if errors.Is(twErr, io.EOF) {
			break
		}
		if twErr != nil {
			return nil, fmt.Errorf("walk head tree: %w", twErr)
		}
		if te.Mode == filemode.Dir {
			continue
		}
		headHashes[name] = te.Hash
	}
	if entry.headTreeHash != headHash {
		// HEAD moved (or this is the first population): the name->hash
		// mapping above is stale, but blobCache is keyed by content hash, not
		// by name/HEAD, so it stays valid and is intentionally left alone
		// (see the blobCache field comment — PerfFix-1).
		//
		// The gitignore matcher only needs rebuilding if a .gitignore file's
		// content actually changed -- most commits don't touch one, but
		// unconditionally invalidating on every HEAD move forced a full
		// gitignore.ReadPatterns recursive filesystem walk per commit
		// regardless (profiled at 55-82% of a CPU core on a machine with
		// frequent commit activity across many worktrees). headTreeCache
		// (old) and headHashes (new) already have every tracked file's path
		// and blob hash from the walk above, so checking just the
		// .gitignore-named entries for a real change is free by comparison.
		if entry.headTreeCache == nil || gitignoreEntriesChanged(entry.headTreeCache, headHashes) {
			entry.untrackedMatcherBuilt = false // F6: force a gitignore matcher rebuild too.
			if entry.headTreeCache != nil {
				// Only count an invalidation once the matcher has actually been
				// built at least once (headTreeCache != nil implies a prior
				// populate) — the first-ever build below isn't "invalidated by
				// a HEAD move," it's just startup.
				atomic.AddInt64(&g.gitignoreMatcherInvalidations, 1)
			}
		}
	}
	entry.headTreeHash = headHash
	entry.headTreeCache = headHashes
	return headHashes, nil
}
