package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// maxAllocateAdminDirNameRetries bounds AllocateAdminDirName's EEXIST-retry loop. Real
// git's own add_worktree loop (builtin/worktree.c) is bounded only by unsigned int
// overflow — effectively unbounded in practice. This project caps it at a much smaller,
// practical number instead: an unbounded loop under a live concurrency-stress test
// would hang rather than fail loudly, and >100 real collisions on one base name past a
// bounded rollout window is itself a sign something else is wrong.
const maxAllocateAdminDirNameRetries = 100

// AllocateAdminDirName picks and creates a `.git/worktrees/<name>/` directory
// (WorktreeAdminDir, plan.md's Domain Glossary) under repoPath, using the same
// allocation strategy real git's own add_worktree uses: a direct os.Mkdir attempt, and
// on EEXIST a numeric suffix appended directly onto name with no separator —
// "<name>1", "<name>2", ... incrementing on each further collision — until an attempt
// succeeds. Confirmed against builtin/worktree.c's add_worktree
// (`while (mkdir(...)) { counter++; strbuf_addf(&sb_repo, "%d", counter); }`) at
// git/git@9321f5936a11d43a666244b4a1737f231469ef7b:
// https://github.com/git/git/blob/9321f5936a11d43a666244b4a1737f231469ef7b/builtin/worktree.c#L507-L514
//
// This is deliberately NOT the Lstat-then-MkdirAll approach go-git v6-alpha's own
// worktree Add uses: that is a confirmed TOCTOU race, since two concurrent callers can
// both pass the Lstat existence check before either calls Mkdir. A direct os.Mkdir is
// atomic at the filesystem level, so two callers racing the same name can never both
// succeed for the same path (plan.md's Pattern Decisions table).
func AllocateAdminDirName(repoPath, name string) (string, error) {
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")
	if err := os.MkdirAll(worktreesDir, 0o777); err != nil {
		return "", fmt.Errorf("AllocateAdminDirName: failed to create %q: %w", worktreesDir, err)
	}

	candidate := filepath.Join(worktreesDir, name)
	for attempt := 0; attempt <= maxAllocateAdminDirNameRetries; attempt++ {
		err := os.Mkdir(candidate, 0o777)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("AllocateAdminDirName: failed to create %q: %w", candidate, err)
		}
		candidate = filepath.Join(worktreesDir, fmt.Sprintf("%s%d", name, attempt+1))
	}

	return "", fmt.Errorf("AllocateAdminDirName: exhausted %d suffix attempts for %q under %q", maxAllocateAdminDirNameRetries, name, worktreesDir)
}
