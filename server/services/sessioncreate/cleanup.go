package sessioncreate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
)

// CleanupPartialCreation idempotently tears down whatever subset of
// {tmux session, git worktree/clone directory} an instance has actually
// acquired, and is a no-op (returns nil) for any resource that never came
// into existence. It generalizes DeleteSession's original live-instance
// cleanup (previously a direct instance.Destroy() call) so Cancel (Epic 3.2)
// and Retry (Epic 3.3) can reuse the same already-solved edge cases instead
// of re-deriving them.
//
// instance.Destroy() alone is insufficient here: Destroy() short-circuits
// via its `!i.started.Load()` guard and returns nil without touching either
// resource whenever the instance never reached a fully-started Active state
// (session/instance.go's Destroy) — exactly the state a creation pipeline
// leaves an instance in when it fails or is cancelled after acquiring a
// worktree but before tmux startup completes. For a fully-started instance,
// this defers to Destroy() so DeleteSession's existing guarantees (VNC/CDP
// teardown, diff-stats capture, EventStopped, the destroyChainTimeout bound)
// are preserved unchanged.
//
// Both KillSession() and CleanupWorktree() are independently idempotent —
// they check HasSession()/HasWorktree() before doing anything — so calling
// this twice, or on an instance with zero, one, or both resources present,
// is always safe.
//
// Known accepted limitation (BUG-072, see plan.md's Domain Glossary): the
// underlying cleanup work (git worktree remove) is not itself
// context-bounded, so a hung removal can outlive a caller's timeout as an
// untracked goroutine. Not fixed here — this helper inherits that property
// from DeleteSession's existing chain rather than worsening it.
func CleanupPartialCreation(instance *instancepkg.Instance, force bool) error {
	if instance.Started() {
		return destroyInstance(instance, force)
	}

	// BUG-099: this branch never called Destroy(), so it never called
	// StopSessionDriver — leaving a SessionDriver goroutine the async
	// creation pipeline was still racing to start completely unstoppable.
	// Call it unconditionally, mirroring Destroy()'s own top-of-function
	// call, so any StartSessionDriver arriving after this point refuses to
	// start (driverDestroyed). See docs/bugs/fixed/BUG-099-*.md.
	instancepkg.StopSessionDriver(instance)

	// Re-check Started() immediately after StopSessionDriver: if a
	// concurrent Instance.Start() completed while we were stopping the
	// driver, the instance is now fully started and must go through
	// Destroy()'s full teardown (VNC/CDP, diff-stats, EventStopped) instead
	// of falling through to the defense-in-depth guard below, which is only
	// safe for an instance that never finished starting.
	if instance.Started() {
		return destroyInstance(instance, force)
	}

	// Task 3.1.1d: defense-in-depth liveness guard. instance.Started() is
	// only flipped true at the very end of Instance.Start() (session/instance.go's
	// startLocked, i.started.Store(true)) -- well after the worktree is set up
	// and the tmux session is actually launched (i.pm().Start(startPath)). A
	// crash in that window leaves an instance that reports Started()==false
	// (and whose persisted Status a caller like Cancel/Retry reads as
	// Creating/Failed) even though both resources genuinely came up healthy.
	// Re-check liveness immediately before each destructive step so that
	// narrow window doesn't cost real, working session state -- see
	// pre-mortem.md failure #2 (P1). This does not apply to the
	// instance.Started() branch above (DeleteSession's normal path for a
	// fully-started session): that branch must still unconditionally destroy
	// a live session the user explicitly asked to delete.
	var errs []error
	if instance.TmuxSessionExists() {
		log.Warn("CleanupPartialCreation: tmux session is alive despite instance.Started()==false; skipping kill (defense-in-depth guard)",
			"session", instance.Title)
	} else if err := instance.KillSession(); err != nil {
		errs = append(errs, fmt.Errorf("failed to kill tmux session: %w", err))
	}

	if worktreePath := instance.GetEffectiveRootDir(); instance.HasGitWorktree() && worktreeLooksAlive(worktreePath) {
		log.Warn("CleanupPartialCreation: worktree looks alive/healthy despite instance.Started()==false; skipping removal (defense-in-depth guard)",
			"session", instance.Title, "path", worktreePath)
	} else if err := removeWorktreeUnlessDirty(instance, force); err != nil {
		errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
	}
	return errors.Join(errs...)
}

// destroyInstance is Destroy, or ForceDestroy when the caller explicitly asked to discard
// uncommitted work in the session's worktree.
func destroyInstance(instance *instancepkg.Instance, force bool) error {
	if force {
		return instance.ForceDestroy()
	}
	return instance.Destroy()
}

// removeWorktreeUnlessDirty removes the worktree, refusing (session.ErrWorktreeDirty) when it
// holds uncommitted changes unless force is set.
func removeWorktreeUnlessDirty(instance *instancepkg.Instance, force bool) error {
	if !force {
		if err := instance.CheckWorktreeRemovable(); err != nil {
			return err
		}
	}
	return instance.CleanupWorktree()
}

// worktreeLooksAlive is the "cheap git-status check" backstop Task 3.1.1d
// calls for: it confirms the directory exists and is a functioning git
// checkout, mirroring session/git's own IsDirtyWithHint check
// (`git status --porcelain`) rather than reinventing a second one.
//
// go-git was tried first per .claude/rules/prefer-go-git-over-subshells.md,
// but PlainOpenWithOptions(path, {DetectDotGit: true}).Head() reliably
// returns "reference not found" for a real `git worktree add` checkout
// (verified against a fresh worktree: go-git resolves the linked .git file
// and commondir correctly enough to open the repo, but not to resolve HEAD
// through it) -- worktrees are exactly what every resource this guard checks
// is. That's the specific, documented failure mode the fallback rule asks
// for, so this shells out instead, scoped to a short timeout like every
// other git subprocess call in this file (e.g. the origin/HEAD..HEAD count
// below).
func worktreeLooksAlive(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := gitprovider.Backend().IsDirty(ctx, backend.Local{Root: backend.RepoRoot(path)}, backend.IntentDestructive)
	return err == nil
}
