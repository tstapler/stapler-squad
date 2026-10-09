package git

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/go-git/go-git/v5"
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
