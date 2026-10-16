package unfinished

import (
	"context"
	"errors"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

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

// repoKeyOf is the registry's canonical repository key for entry's storer.
func repoKeyOf(entry *cachedRepo) (repostate.RepoKey, bool) {
	k, ok := entry.repo.Storer.(commonDirKeyer)
	if !ok {
		return "", false
	}
	return repostate.RepoKey(k.CommonDirKey()), true
}

// commitMessagesShared answers CommitMessages from the shared reach set and
// message memos. handled is false when the flag is off, the storer has no
// shared state, or the graph fell back (full arena, severe pressure, a missing
// commit): the caller then runs the legacy phases, which report their own errors.
func (g *GoGitVCSReader) commitMessagesShared(entry *cachedRepo, head, base plumbing.Hash, max int) (msgs []string, handled bool, err error) {
	rs := g.repoState(entry)
	if rs == nil {
		return nil, false, nil
	}
	repo, ok := repoKeyOf(entry)
	if !ok {
		return nil, false, nil
	}
	ctx := context.Background()
	ord, shallow := g.observeShallow(entry, rs)
	src := newGraphSource(entry.repo.Storer, shallow)
	reach, err := rs.Reach(ctx, src, ord, repostate.ReachKey{Repo: repo, Base: base, Cap: maxReachableSetCommits})
	if err != nil {
		return nil, !errors.Is(err, repostate.ErrFallback), err
	}
	in := repostate.MessageInputs{Src: src, Ord: ord, Reach: reach, Line: commitFirstLine(entry)}
	msgs, err = rs.CommitMessages(ctx, in, repostate.MessageKey{Repo: repo, Head: head, Base: base, Max: max, Cap: maxReachableSetCommits})
	if err != nil {
		return nil, !errors.Is(err, repostate.ErrFallback), err
	}
	return msgs, true, nil
}

// commitFirstLine reads a commit's first message line straight from the storer,
// which self-locks (like graphSource), so it never takes entry.mu.
func commitFirstLine(entry *cachedRepo) repostate.LineFunc {
	return func(h plumbing.Hash) (string, error) {
		c, err := object.GetCommit(entry.repo.Storer, h)
		if err != nil {
			return "", err
		}
		return firstLine(c.Message), nil
	}
}
