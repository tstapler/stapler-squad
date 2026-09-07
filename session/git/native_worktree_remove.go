package git

import (
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/log"
)

// nativeRemoveWorktree is the pure-Go replacement for removeLocked's subprocess `git
// worktree remove -f` + admin-file cleanup (Epic 2.2, Story 2.2.1), dispatched from
// removeLocked via useNativeWorktree (Task 2.2.2a). It removes worktreePath's working
// directory and its WorktreeAdminDir but never touches refs/heads/<branch> — branch
// deletion is not this function's job, matching Cleanup()'s existing doc comment and
// guarding against the previously-fixed "stop_session silently deletes the git branch"
// bug class (Story 2.2.1's "As a" line).
func nativeRemoveWorktree(repoPath, worktreePath string) error {
	adminDir, err := worktreeAdminDirFor(repoPath, worktreePath)
	if err != nil {
		return fmt.Errorf("nativeRemoveWorktree: failed to resolve admin dir for %q: %w", worktreePath, err)
	}

	// Explicit Stat before removal (Task 2.2.1b) rather than relying on os.RemoveAll's
	// own no-op-on-missing-path behavior: mirrors removeLocked's existing
	// "always verify liveness via a real filesystem stat" pattern so a working
	// directory already deleted out from under git (an external `rm -rf`) is a
	// recognized, logged no-op rather than an implicit side effect of RemoveAll.
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("nativeRemoveWorktree: failed to remove working tree %q: %w", worktreePath, err)
		}
	} else if os.IsNotExist(statErr) {
		log.Info("nativeRemoveWorktree: worktree directory does not exist", "path", worktreePath)
	} else {
		return fmt.Errorf("nativeRemoveWorktree: failed to stat working tree %q: %w", worktreePath, statErr)
	}

	if err := os.RemoveAll(adminDir); err != nil {
		return fmt.Errorf("nativeRemoveWorktree: failed to remove admin dir %q: %w", adminDir, err)
	}

	return nil
}
