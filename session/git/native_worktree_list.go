package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NativeWorktreeEntry describes one worktree found under a repo's `.git/worktrees/`
// admin directory (plan.md's Domain Glossary), as produced by nativeListWorktrees — the
// pure-Go replacement for parsing `git worktree list --porcelain` output.
type NativeWorktreeEntry struct {
	// Name is the admin dir's own name under .git/worktrees/ (WorktreeAdminDir's leaf
	// component), not necessarily the branch name — AllocateAdminDirName names it after
	// the target directory's basename, and a collision can suffix it further.
	Name string
	// WorktreePath is the linked worktree's working-directory path, read from the admin
	// dir's GitdirFile ("<WorktreePath>/.git").
	WorktreePath string
	// BranchRef is the full ref name (e.g. "refs/heads/feature-x") read from the admin
	// dir's HEAD file, or the raw content of HEAD when it isn't a symbolic ref (detached
	// HEAD) — empty if HEAD is missing or unreadable.
	BranchRef string
	// Locked reports whether the admin dir's LockedMarker ("locked") is present.
	Locked bool
	// Prunable is this project's deliberately scoped-down classification (stack.md
	// §2.2): true when WorktreePath no longer exists on disk and the entry isn't
	// Locked. No mtime grace period, unlike real git's own should_prune_worktree.
	Prunable bool
}

// nativeListWorktrees is the pure-Go replacement for parsing `git worktree list
// --porcelain` output (Epic 2.3, Story 2.3.1): it reads repoPath's `.git/worktrees/`
// admin directory directly and classifies each entry's liveness per this project's
// scoped-down prunability rule. A repo with no linked worktrees yet (no
// `.git/worktrees/` directory) returns an empty, non-error result — only a genuine
// read failure (e.g. permission denied) is an error.
func nativeListWorktrees(repoPath string) ([]NativeWorktreeEntry, error) {
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")
	dirEntries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("nativeListWorktrees: failed to read %q: %w", worktreesDir, err)
	}

	entries := make([]NativeWorktreeEntry, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}
		adminDir := filepath.Join(worktreesDir, dirEntry.Name())
		entry, err := buildNativeWorktreeEntry(dirEntry.Name(), adminDir)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// buildNativeWorktreeEntry reads adminDir's GitdirFile, HEAD, and LockedMarker to build
// one NativeWorktreeEntry (Task 2.3.1a), then classifies it per Task 2.3.1b's rule:
// Prunable = (WorktreePath missing) && !Locked.
func buildNativeWorktreeEntry(name, adminDir string) (NativeWorktreeEntry, error) {
	worktreePath, err := readWorktreePathFromGitdirFile(adminDir)
	if err != nil {
		return NativeWorktreeEntry{}, err
	}

	locked := fileExistsOnDisk(filepath.Join(adminDir, "locked"))
	prunable := !locked && !dirExistsOnDisk(worktreePath)

	return NativeWorktreeEntry{
		Name:         name,
		WorktreePath: worktreePath,
		BranchRef:    readWorktreeHEADRef(adminDir),
		Locked:       locked,
		Prunable:     prunable,
	}, nil
}

// readWorktreePathFromGitdirFile reads adminDir's GitdirFile — an absolute path to the
// linked worktree's own WorktreeRedirectFile, "<WorktreePath>/.git" — and returns the
// WorktreePath directory it names.
func readWorktreePathFromGitdirFile(adminDir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
	if err != nil {
		return "", fmt.Errorf("nativeListWorktrees: failed to read gitdir file in %q: %w", adminDir, err)
	}
	gitFilePath := strings.TrimSpace(string(content))
	if gitFilePath == "" {
		return "", fmt.Errorf("nativeListWorktrees: gitdir file in %q is empty", adminDir)
	}
	return filepath.Dir(gitFilePath), nil
}

// readWorktreeHEADRef reads adminDir's HEAD file and returns the ref it names (stripping
// the "ref: " prefix), or its raw trimmed content for a detached HEAD. Returns "" if HEAD
// is missing or unreadable — an admin dir mid-Add (Task 2.1.1b writes HEAD after
// GitdirFile/CommondirFile) can legitimately lack it, and a missing branch ref must not
// abort the whole listing the way a missing GitdirFile does.
func readWorktreeHEADRef(adminDir string) string {
	content, err := os.ReadFile(filepath.Join(adminDir, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(content))
	return strings.TrimPrefix(line, "ref: ")
}

// fileExistsOnDisk reports whether path exists and is a regular file (not a directory).
func fileExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExistsOnDisk reports whether path exists and is a directory — the directory-exists-
// only prunability check this project deliberately scopes down to (stack.md §2.2's
// explicit scope-cut: no mtime grace period).
func dirExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
