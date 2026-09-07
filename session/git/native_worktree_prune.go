package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// nativeWorktreePrune is the pure-Go replacement for `git worktree prune` (Epic 2.4,
// Story 2.4.1). It reuses nativeListWorktrees' Prunable classification rather than
// reimplementing prunability logic here, so List and Prune can never disagree about what
// counts as prunable — removing exactly the admin dirs classified Prunable=true and
// leaving every locked or live entry untouched.
func nativeWorktreePrune(repoPath string) error {
	entries, err := nativeListWorktrees(repoPath)
	if err != nil {
		return fmt.Errorf("nativeWorktreePrune: failed to list worktrees for %q: %w", repoPath, err)
	}

	for _, entry := range entries {
		if !entry.Prunable {
			continue
		}
		adminDir := filepath.Join(repoPath, ".git", "worktrees", entry.Name)
		if err := os.RemoveAll(adminDir); err != nil {
			return fmt.Errorf("nativeWorktreePrune: failed to remove admin dir %q: %w", adminDir, err)
		}
	}

	return nil
}
