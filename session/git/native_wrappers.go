package git

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/tstapler/stapler-squad/session/git/native"
)

// Thin wrappers over session/git/native's observability helpers so existing call sites
// in this package keep their original names.

const (
	outcomeSuccess       = native.OutcomeSuccess
	outcomeError         = native.OutcomeError
	implementationNative = native.ImplementationNative
)

func spanOutcome(err error) string { return native.SpanOutcome(err) }

func withOperationAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return native.WithOperationAttrs(ctx, attrs...)
}

func withOperationSpan(ctx context.Context, op string, fn func() (implementation, outcome string, err error)) error {
	return native.WithOperationSpan(ctx, op, fn)
}

// --- worktree add/list/prune/remove/admin-file wrappers (Story 1.1.0a group ii) ---

// NativeWorktreeEntry is kept so existing callers (session/worktree_consistency_sweep.go)
// do not change.
type NativeWorktreeEntry = native.WorktreeEntry

// AdminFileWriter is kept as an alias of native.AdminFileWriter.
type AdminFileWriter = native.AdminFileWriter

// NewAdminFileWriter forwards to native.NewAdminFileWriter.
func NewAdminFileWriter(dir string) *AdminFileWriter { return native.NewAdminFileWriter(dir) }

// ListWorktrees is the exported entry point for on-disk worktree truth.
func ListWorktrees(repoPath string) ([]NativeWorktreeEntry, error) {
	return native.ListWorktrees(repoPath)
}

func nativeListWorktrees(repoPath string) ([]NativeWorktreeEntry, error) {
	return native.ListWorktrees(repoPath)
}

func nativeRemoveWorktree(repoPath, worktreePath string) error {
	return native.RemoveWorktree(repoPath, worktreePath)
}

func nativeUnlockWorktree(repoPath, worktreePath string) error {
	return native.UnlockWorktree(repoPath, worktreePath)
}

func nativeWorktreePrune(repoPath string) error { return native.PruneWorktrees(repoPath) }

func resolveWorktreeIndexPath(path string) (string, error) {
	return native.ResolveWorktreeIndexPath(path)
}

func openWorktreeRepo(path string) (*git.Repository, error) { return native.OpenRepo(path) }

// nativeSetupNewWorktree runs native.SetupNewWorktree for g and caches the resolved base
// commit SHA on g, as the pre-extraction method did.
func (g *GitWorktree) nativeSetupNewWorktree() error {
	sha, err := native.SetupNewWorktree(native.SetupParams{
		RepoPath:      g.repoPath,
		WorktreePath:  g.worktreePath,
		BranchName:    g.branchName,
		BaseCommitSHA: g.baseCommitSHA,
	})
	if sha != "" {
		g.baseCommitSHA = sha
	}
	return err
}

// --- dirty-check wrappers (Story 1.1.0a group iii) ---

type (
	gitignoreFSCache  = native.GitignoreFSCache
	headTreeHashCache = native.HeadTreeHashCache
)

func newCachedFilesystem(fs billy.Filesystem, cache *gitignoreFSCache) billy.Filesystem {
	return native.NewCachedFilesystem(fs, cache)
}

func worktreeIsDirtyFast(path string, cache *gitignoreFSCache, headCache *headTreeHashCache) (bool, error) {
	return native.IsDirtyFast(path, cache, headCache)
}

func cachedHeadTreeHashes(repo *git.Repository, cache *headTreeHashCache) (map[string]plumbing.Hash, error) {
	return native.CachedHeadTreeHashes(repo, cache)
}

func worktreeStagedDirty(idx *index.Index, headHashes map[string]plumbing.Hash) bool {
	return native.StagedDirty(idx, headHashes)
}

// IsDirtyUncached reports whether the worktree has uncommitted changes, bypassing
// IsDirtyWithHint's own TTL cache but still reusing g.gitignoreFS/g.headTreeCache, the
// per-GitWorktree allocation-avoidance caches native.IsDirtyFast needs. Those two are
// safe to share without reintroducing staleness: headTreeCache is keyed by HEAD's own
// commit hash and gitignoreFS has its own invalidation (InvalidateDirtyCache). Used by
// session.WorktreeChangeDetector's periodic tick, which needs a fresh per-tick answer.
func (g *GitWorktree) IsDirtyUncached() (bool, error) {
	return native.IsDirtyFast(g.GetWorktreePath(), &g.gitignoreFS, &g.headTreeCache)
}
