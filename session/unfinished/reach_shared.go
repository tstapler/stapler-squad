package unfinished

import (
	"context"
	"errors"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// reachSet answers "is this commit already reachable from base" for
// CommitMessages: a repostate bitset on the shared path, the legacy map off it.
type reachSet interface{ Contains(plumbing.Hash) bool }

// hashSet adapts the legacy reachableSet map.
type hashSet map[plumbing.Hash]bool

func (s hashSet) Contains(h plumbing.Hash) bool { return s[h] }

var (
	_ reachSet = hashSet(nil)
	_ reachSet = (*repostate.ReachBitset)(nil)
)

// reachMembership is the call-site switch behind shared_commit_state: flag on
// resolves the base's reach set once per (repository, base) on the shared id
// graph; flag off, a non-gogitstore storer or any graph fallback (full arena,
// severe pressure, a missing commit, which the legacy walk reports itself)
// runs the legacy per-worktree set.
func (g *GoGitVCSReader) reachMembership(entry *cachedRepo, worktreePath string, baseHash plumbing.Hash) (reachSet, error) {
	if rs := g.repoState(entry); rs != nil {
		if keyer, ok := entry.repo.Storer.(commonDirKeyer); ok {
			ord, shallow := g.observeShallow(entry, rs)
			want := repostate.ReachKey{Repo: repostate.RepoKey(keyer.CommonDirKey()), Base: baseHash, Cap: maxReachableSetCommits}
			b, err := rs.Reach(context.Background(), newGraphSource(entry.repo.Storer, shallow), ord, want)
			if err == nil {
				return b, nil
			}
			if !errors.Is(err, repostate.ErrFallback) {
				return nil, err
			}
		}
	}
	set, err := g.cachedReachableSet(entry, worktreePath, baseHash)
	if err != nil {
		return nil, err
	}
	return hashSet(set), nil
}
