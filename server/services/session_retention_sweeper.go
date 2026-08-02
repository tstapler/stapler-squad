package services

import (
	"context"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// sessionRetentionSweepInterval is how often the retention sweep runs. Matches the
// coarse-grained cadence of other calendar-day-scale retention sweeps in this codebase
// (see server/analytics/retention.go) rather than the minute-scale cadence used by
// HibernationSweeper, since eligibility here only changes once a day at the finest.
const sessionRetentionSweepInterval = 1 * time.Hour

// SessionRetentionSweeper periodically deletes archived sessions that have passed
// the configured retention window, reusing SessionService.DeleteSession's existing
// tmux/worktree/DB cleanup path so deletion logic lives in exactly one place.
//
// Scope (deliberate, see project_plans/session-retention-cleanup): only sessions with
// ArchivedAt set are considered. Sessions that reached a terminal Stopped status without
// ever being archived are NOT swept by this pass — there is no reliable "how long has
// this been stopped" timestamp to anchor a retention window on for that case (UpdatedAt
// is touched by more than just the stop transition). Follow-up, not built here.
type SessionRetentionSweeper struct {
	storage *session.Storage
	cfg     *config.Config
	svc     *SessionService
}

// NewSessionRetentionSweeper constructs a sweeper. svc is used to perform the actual
// deletion via its existing DeleteSession RPC handler, so tmux/worktree/DB cleanup
// logic is not duplicated here.
func NewSessionRetentionSweeper(storage *session.Storage, cfg *config.Config, svc *SessionService) *SessionRetentionSweeper {
	return &SessionRetentionSweeper{storage: storage, cfg: cfg, svc: svc}
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled.
func (s *SessionRetentionSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(sessionRetentionSweepInterval)
	defer ticker.Stop()

	log.Info("session retention sweeper started",
		"retention_days", s.cfg.SessionRetention.RetentionDaysOrDefault(),
		"check_interval", sessionRetentionSweepInterval)

	// Run immediately on start rather than waiting for the first tick.
	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("session retention sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep finds archived sessions past the retention window and deletes the ones that
// pass safety checks. A session that fails a safety check is skipped for this cycle
// (not an error) — it will be reconsidered on the next tick once it becomes safe.
func (s *SessionRetentionSweeper) sweep(ctx context.Context) {
	if !s.cfg.SessionRetention.EnabledOrDefault() {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -s.cfg.SessionRetention.RetentionDaysOrDefault())

	dataSlice, err := s.storage.ListInstanceData()
	if err != nil {
		log.Error("session retention sweeper: failed to list instances", "err", err)
		return
	}

	for _, d := range dataSlice {
		if d.ArchivedAt == nil || !d.ArchivedAt.Before(cutoff) {
			continue
		}

		if safe, reason := s.sessionSafeToDelete(ctx, d); !safe {
			log.Debug("session retention sweeper: skipping session, not safe to delete",
				"session", d.Title, "reason", reason)
			continue
		}

		log.Info("session retention sweeper: deleting expired archived session",
			"session", d.Title, "archived_at", d.ArchivedAt, "retention_days", s.cfg.SessionRetention.RetentionDaysOrDefault())

		req := connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: d.Title})
		if _, err := s.svc.DeleteSession(ctx, req); err != nil {
			log.Warn("session retention sweeper: failed to delete session", "session", d.Title, "err", err)
		}
	}
}

// sessionSafeToDelete applies the required safety checks to raw stored instance data:
// no open/unmerged PR, no uncommitted worktree changes, and (for backlog-linked
// sessions) no sibling round still pointing at the same shared worktree. All must pass.
func (s *SessionRetentionSweeper) sessionSafeToDelete(ctx context.Context, d session.InstanceData) (safe bool, reason string) {
	// PR check: GitHubPRStatusTerminal is maintained by PRStatusPoller and is only
	// true once the PR is merged or closed — reuse it rather than making a fresh
	// GitHub API call from the sweep.
	if d.GitHubPRNumber != 0 && !d.GitHubPRStatusTerminal {
		return false, "open or unmerged PR"
	}

	// Worktree-dependent checks: only meaningful when a worktree is actually recorded
	// and still on disk. Gate on Worktree.WorktreePath rather than the IsWorktree field —
	// ToInstanceData() only populates Worktree (guarded by gitManager.HasWorktree()),
	// it never sets IsWorktree, so that field cannot be trusted to round-trip through
	// storage. A directory-mode session, or a worktree already cleaned up (e.g. by a
	// prior Pause), has nothing on disk for either check below to worry about.
	if d.Worktree.WorktreePath == "" {
		return true, ""
	}
	if _, statErr := os.Stat(d.Worktree.WorktreePath); statErr != nil {
		return true, ""
	}

	// Dirty-worktree check.
	wt := git.NewGitWorktreeFromStorage(d.Worktree.RepoPath, d.Worktree.WorktreePath, d.Title, d.Worktree.BranchName, d.Worktree.BaseCommitSHA)
	if wt != nil {
		dirty, dirtyErr := wt.IsDirty()
		if dirtyErr != nil {
			return false, "worktree dirty-check failed: " + dirtyErr.Error()
		}
		if dirty {
			return false, "uncommitted worktree changes"
		}
	}

	// Shared-worktree check: backlog rework/reopen reuses the same deterministic
	// branch (and therefore the same worktree directory — see
	// session/git.findExistingWorktreeForBranch) across rounds, so an old archived
	// round's session can point at the exact directory a newer, still-active round's
	// session is using. Destroying it out from under that sibling would be exactly the
	// kind of collateral damage this sweep must not cause. Mirrors the sibling lookup
	// backlog_service.go's cleanupItemWorktreesExcept already does for the same reason.
	if d.UUID != "" {
		is, err := s.storage.GetItemSessionBySessionUUID(ctx, d.UUID)
		if err == nil && is.BacklogItemID != "" {
			siblings, listErr := s.storage.ListItemSessions(ctx, is.BacklogItemID)
			if listErr != nil {
				return false, "failed to list sibling item sessions: " + listErr.Error()
			}
			for _, sib := range siblings {
				if sib.SessionUUID == "" || sib.SessionUUID == d.UUID {
					continue
				}
				sibWorktree, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, sib.SessionUUID)
				if wtErr == nil && sibWorktree.WorktreePath != "" && sibWorktree.WorktreePath == d.Worktree.WorktreePath {
					return false, "worktree shared with another session on the same backlog item"
				}
			}
		}
	}

	return true, ""
}
