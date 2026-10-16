package unfinished

import (
	"github.com/tstapler/stapler-squad/session/unfinished/gogitstore"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// repoStateBudgetBytes is the per-repository byte budget above which
// PruneToMemoryBudget starts dropping RepoState tiers (ADR-007); it matches
// the ~96 MB per-repo budget in the design.
const repoStateBudgetBytes = 96 << 20

// extensionHolder is satisfied by *gogitstore.WorktreeStorer.
type extensionHolder interface {
	Extension(newFn func() gogitstore.Extension) gogitstore.Extension
}

// repoState returns the shared per-repository state hung on entry's store, or
// nil when shared_commit_state is off (the extension slot is then never
// touched) or the entry's storer is not a gogitstore one.
func (g *GoGitVCSReader) repoState(entry *cachedRepo) *repostate.RepoState {
	if !EffectiveFlags().SharedCommitState {
		return nil
	}
	h, ok := entry.repo.Storer.(extensionHolder)
	if !ok {
		return nil
	}
	ext := h.Extension(func() gogitstore.Extension {
		return repostate.NewRepoState(repostate.Options{PressureFunc: g.UnderSeverePressure, MaxGraphCommits: g.repoStateMaxGraphCommits}, repoStateBudgetBytes)
	})
	rs, _ := ext.(*repostate.RepoState)
	return rs
}

// forEachRepoState calls fn for every live RepoState in the registry.
func (g *GoGitVCSReader) forEachRepoState(fn func(*repostate.RepoState)) {
	g.gogitstoreRegistry().ExtensionsDo(func(e gogitstore.Extension) {
		if rs, ok := e.(*repostate.RepoState); ok {
			fn(rs)
		}
	})
}

// pruneRepoStates frees RepoState memory in ADR-007 order; severe drops all.
func (g *GoGitVCSReader) pruneRepoStates() {
	severe := g.UnderSeverePressure()
	g.forEachRepoState(func(rs *repostate.RepoState) { rs.Prune(severe) })
}

// dropAllRepoStates empties every RepoState (ClearCache).
func (g *GoGitVCSReader) dropAllRepoStates() {
	g.forEachRepoState((*repostate.RepoState).DropAll)
}

// repoStateTotals returns the live RepoState count and their summed bytes.
func (g *GoGitVCSReader) repoStateTotals() (count int64, bytes int64) {
	g.forEachRepoState(func(rs *repostate.RepoState) {
		count++
		bytes += rs.Bytes()
	})
	return count, bytes
}
