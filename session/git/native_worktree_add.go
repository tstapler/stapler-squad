package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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

// nativeSetupNewWorktree is the pure-Go, go-git-based replacement for setupNewWorktree's
// subprocess `git worktree add -b <branch> <path> <commit>` (Epic 2.1, Stories 2.1.1 and
// 2.1.2), dispatched from setupNewWorktree via useNativeWorktree (Task 2.1.3b). It writes
// real git's exact `.git/worktrees/<name>/` admin-file set in crash-safe order (ADR-001)
// — LockedMarker first, GitdirFile before CommondirFile, then HEAD, then the worktree's
// own WorktreeRedirectFile — then populates the working tree via go-git's existing
// Worktree.Checkout (Story 2.1.2) rather than hand-rolling an index writer, removing
// LockedMarker last, only on success (Task 2.1.1d). Any earlier failure leaves
// LockedMarker in place — matching real git's own "a failed Add stays locked/prunable"
// behavior — nothing here rolls back partial admin-file state on error.
func (g *GitWorktree) nativeSetupNewWorktree() error {
	repo, err := OpenRepo(g.repoPath)
	if err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to open repository %q: %w", g.repoPath, err)
	}

	baseCommitSHA, err := g.resolveNativeAddBaseCommit(repo)
	if err != nil {
		return err
	}

	// Real git names a worktree's admin dir after the target directory's own basename,
	// not the branch name — confirmed against git 2.53.0's `git worktree add -b <branch>
	// <path>` output (the admin dir under .git/worktrees/ takes <path>'s leaf component).
	adminDir, err := AllocateAdminDirName(g.repoPath, filepath.Base(g.worktreePath))
	if err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to allocate admin dir: %w", err)
	}

	if err := writeNativeWorktreeAdminFiles(adminDir, g.worktreePath, g.branchName); err != nil {
		return err
	}
	if err := writeNativeWorktreeRedirectFile(g.worktreePath, adminDir); err != nil {
		return err
	}
	if err := checkoutNativeWorktree(g.worktreePath, g.branchName, baseCommitSHA); err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(adminDir, "locked")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nativeSetupNewWorktree: failed to remove locked marker after successful checkout: %w", err)
	}

	return nil
}

// resolveNativeAddBaseCommit returns g.baseCommitSHA if already set (a caller that
// pre-selected a specific base commit, e.g. NewGitWorktreeFromCommitSHA — mirroring
// legacySetupNewWorktree's identical precedent), otherwise resolves and caches repo's
// current HEAD commit. Errors if the resolved/pre-set SHA doesn't exist in repo, so a
// bad base commit fails before any admin file is written.
func (g *GitWorktree) resolveNativeAddBaseCommit(repo *git.Repository) (string, error) {
	sha := g.baseCommitSHA
	if sha == "" {
		head, err := repo.Head()
		if err != nil {
			return "", fmt.Errorf("nativeSetupNewWorktree: failed to resolve HEAD of %q: %w", g.repoPath, err)
		}
		sha = head.Hash().String()
	}

	if _, err := repo.CommitObject(plumbing.NewHash(sha)); err != nil {
		return "", fmt.Errorf("nativeSetupNewWorktree: base commit %q does not exist: %w", sha, err)
	}

	g.baseCommitSHA = sha
	return sha, nil
}

// writeNativeWorktreeAdminFiles writes LockedMarker, GitdirFile, CommondirFile, and HEAD
// into adminDir in real git's crash-safe order (Tasks 2.1.1a-b): locked first (content
// "initializing"), then gitdir (absolute path to worktreePath's own WorktreeRedirectFile),
// then commondir (always "../.."), then HEAD (a symbolic ref to branchName — the branch
// itself doesn't need to exist yet; checkoutNativeWorktree creates it).
func writeNativeWorktreeAdminFiles(adminDir, worktreePath, branchName string) error {
	w := NewAdminFileWriter(adminDir)

	if err := w.WriteFile("locked", []byte("initializing")); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to write locked marker in %q: %w", adminDir, err)
	}
	if err := w.WriteFile("gitdir", []byte(filepath.Join(worktreePath, ".git")+"\n")); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to write gitdir file in %q: %w", adminDir, err)
	}
	if err := w.WriteFile("commondir", []byte("../..\n")); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to write commondir file in %q: %w", adminDir, err)
	}
	if err := w.WriteFile("HEAD", []byte("ref: "+plumbing.NewBranchReferenceName(branchName).String()+"\n")); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to write HEAD file in %q: %w", adminDir, err)
	}
	return nil
}

// writeNativeWorktreeRedirectFile creates worktreePath and writes its own top-level `.git`
// file (WorktreeRedirectFile, Task 2.1.1c) pointing back at adminDir. Per ADR-001's
// Update, this goes through AdminFileWriter too — a bare os.WriteFile is not acceptable
// here: a torn write on this specific file is invisible to real git's own `worktree list
// --porcelain` prunability check, which inspects the admin-dir side, not this file.
func writeNativeWorktreeRedirectFile(worktreePath, adminDir string) error {
	if err := os.MkdirAll(worktreePath, 0o750); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to create worktree directory %q: %w", worktreePath, err)
	}
	w := NewAdminFileWriter(worktreePath)
	if err := w.WriteFile(".git", []byte(gitdirFileLinePrefix+adminDir+"\n")); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to write worktree redirect file in %q: %w", worktreePath, err)
	}
	return nil
}

// checkoutNativeWorktree populates worktreePath's working tree and stage-0 index by
// reusing go-git's own, already-correct Worktree.Checkout (Story 2.1.2) rather than
// hand-rolling an index writer — the clean-add path needs no bespoke tree-population
// logic at all.
func checkoutNativeWorktree(worktreePath, branchName, baseCommitSHA string) error {
	repo, err := openWorktreeRepo(worktreePath)
	if err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to open new worktree %q: %w", worktreePath, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: failed to get worktree handle for %q: %w", worktreePath, err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{
		Hash:   plumbing.NewHash(baseCommitSHA),
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	}); err != nil {
		return fmt.Errorf("nativeSetupNewWorktree: checkout failed for branch %q at %q: %w", branchName, baseCommitSHA, err)
	}
	return nil
}

// nativeUnlockWorktree is the pure-Go replacement for setupFromExistingBranch's `git
// worktree unlock` subprocess call (Story 2.1.4): it clears a stale LockedMarker left
// behind by an interrupted Add so a subsequent force-remove+re-add of worktreePath isn't
// refused. Removing an absent marker is a no-op, not an error, matching today's "ignore
// error if not locked" handling of the subprocess call it replaces. Deletion via
// os.Remove is already an atomic unlink, so no AdminFileWriter temp+rename step applies
// here — that primitive exists for content writes, not removals.
func nativeUnlockWorktree(repoPath, worktreePath string) error {
	adminDir, err := worktreeAdminDirFor(repoPath, worktreePath)
	if err != nil {
		return fmt.Errorf("nativeUnlockWorktree: failed to resolve admin dir for %q: %w", worktreePath, err)
	}

	if err := os.Remove(filepath.Join(adminDir, "locked")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nativeUnlockWorktree: failed to remove locked marker in %q: %w", adminDir, err)
	}
	return nil
}

// worktreeAdminDirFor resolves worktreePath's WorktreeAdminDir the same way
// resolveWorktreeIndexPath (Task 1.2.2b) locates it — by reading worktreePath's own
// `.git` redirect file — falling back to the deterministic repoPath/.git/worktrees/
// <basename> convention AllocateAdminDirName uses when that redirect file itself is
// unreadable. The fallback matters here specifically: nativeUnlockWorktree's whole
// purpose is clearing a marker left by an interrupted Add, which can leave worktreePath's
// own `.git` file missing or malformed.
func worktreeAdminDirFor(repoPath, worktreePath string) (string, error) {
	if indexPath, err := resolveWorktreeIndexPath(worktreePath); err == nil {
		return filepath.Dir(indexPath), nil
	}
	return filepath.Join(repoPath, ".git", "worktrees", filepath.Base(worktreePath)), nil
}
