package unfinished

// White-box tests for the gogitstore-backed open path: sharing both the
// decoded-object cache and the parsed pack-index across every worktree of
// the same repo (keyed by common .git dir, not worktree path). See
// openRepoEntry/gogitstoreRegistry in gogit_vcs_reader.go and
// session/unfinished/gogitstore + session/unfinished/design/
// pluggable-gitstore.md for the underlying mechanism.

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/cache"
)

// TestPerRepoObjectCacheSize_IsSmallerThanGoGitDefault guards the actual
// point of this fix: go-git's own default (cache.DefaultMaxSize, 96MB) is
// far larger than what a scanner holding many repos "hot" concurrently
// should pay per repo. This test fails if a future edit silently reverts to
// (or exceeds) go-git's default.
func TestPerRepoObjectCacheSize_IsSmallerThanGoGitDefault(t *testing.T) {
	t.Parallel()
	if perRepoObjectCacheSize >= cache.DefaultMaxSize {
		t.Errorf("perRepoObjectCacheSize = %d, want strictly less than go-git's own default %d",
			perRepoObjectCacheSize, cache.DefaultMaxSize)
	}
}

// TestGogitstoreRegistry_SameInstanceAcrossCalls proves the registry is
// actually reused, not rebuilt per open — a fresh Registry per call would
// defeat cross-worktree sharing entirely (see gogitstore.Registry's own
// doc comment).
func TestGogitstoreRegistry_SameInstanceAcrossCalls(t *testing.T) {
	t.Parallel()
	g := &GoGitVCSReader{}

	first := g.gogitstoreRegistry()
	second := g.gogitstoreRegistry()

	if first != second {
		t.Error("gogitstoreRegistry() returned different *gogitstore.Registry pointers on two calls; want the same instance reused (sync.Once)")
	}
	if first.CacheMaxSize != int64(perRepoObjectCacheSize) {
		t.Errorf("Registry.CacheMaxSize = %d, want %d (perRepoObjectCacheSize)", first.CacheMaxSize, perRepoObjectCacheSize)
	}
}

// TestOpenRepoEntry_TwoWorktreesOfSameRepo_ShareOneIndexParse is the
// end-to-end proof: two real linked worktrees of one repo, opened through
// openRepoEntry exactly as the scanner does, must resolve to the same
// underlying gogitstore.SharedObjectStore (proven via Registry.Stats()
// reporting exactly one commondir entry) — the actual production wiring
// this change exists for, not just that gogitstore compiles.
func TestOpenRepoEntry_TwoWorktreesOfSameRepo_ShareOneIndexParse(t *testing.T) {
	t.Parallel()
	mainRepo := initRepoInternal(t)
	linkedPath := filepath.Join(filepath.Dir(mainRepo), "linked-worktree-gogitstore")
	gitRunInternal(t, mainRepo, "worktree", "add", linkedPath, "-b", "gogitstore-feature")

	g := &GoGitVCSReader{}

	if _, err := g.openRepoEntry(mainRepo); err != nil {
		t.Fatalf("openRepoEntry(mainRepo): %v", err)
	}
	if _, err := g.openRepoEntry(linkedPath); err != nil {
		t.Fatalf("openRepoEntry(linkedPath): %v", err)
	}

	stats := g.gogitstoreRegistry().Stats()
	if len(stats) != 1 {
		t.Errorf("gogitstoreRegistry().Stats() has %d commondir entries after opening 2 worktrees of 1 repo, want exactly 1 (shared)", len(stats))
	}
}
