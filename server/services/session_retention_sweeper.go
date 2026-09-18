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

	// Worktree must be eager-loaded here: baseSafeToDelete's dirty-worktree check and
	// sessionSafeToDelete's shared-worktree check both gate on d.Worktree.WorktreePath,
	// which ListInstanceData (LoadMinimal) never populates -- that silently bypassed both
	// safety checks (see git history for the regression this fixes).
	dataSlice, err := s.storage.ListInstanceDataWithWorktree()
	if err != nil {
		log.Error("session retention sweeper: failed to list instances", "err", err)
		return
	}

	// Snapshot by UUID once per cycle so the shared-worktree convergence check (below)
	// can evaluate a sibling's own base eligibility without a second storage read per
	// sibling per candidate — and so every candidate this cycle sees the same consistent
	// view of every other session's state.
	byUUID := make(map[string]session.InstanceData, len(dataSlice))
	for _, d := range dataSlice {
		if d.UUID != "" {
			byUUID[d.UUID] = d
		}
	}

	for _, d := range dataSlice {
		if d.ArchivedAt == nil || !d.ArchivedAt.Before(cutoff) {
			continue
		}

		if safe, reason := s.sessionSafeToDelete(ctx, d, byUUID, cutoff); !safe {
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

// baseSafeToDelete applies the safety checks that depend only on d itself: past the
// retention window, no open/unmerged PR, no uncommitted worktree changes. It deliberately
// does NOT check worktree sharing with siblings — sessionSafeToDelete calls this both for
// the top-level candidate and, in its own sibling loop, to test each sibling's own
// eligibility — the sharing check must not recurse into itself.
func (s *SessionRetentionSweeper) baseSafeToDelete(d session.InstanceData, cutoff time.Time) (safe bool, reason string) {
	if d.ArchivedAt == nil || !d.ArchivedAt.Before(cutoff) {
		return false, "not archived, or not yet past the retention window"
	}

	// PR check: GitHubPRStatusTerminal is maintained by PRStatusPoller and is only
	// true once the PR is merged or closed — reuse it rather than making a fresh
	// GitHub API call from the sweep.
	if d.GitHubPRNumber != 0 && !d.GitHubPRStatusTerminal {
		return false, "open or unmerged PR"
	}

	// Dirty-worktree check: only meaningful when a worktree is actually recorded and
	// still on disk. Gate on Worktree.WorktreePath rather than the IsWorktree field —
	// ToInstanceData() only populates Worktree (guarded by gitManager.HasWorktree()),
	// it never sets IsWorktree, so that field cannot be trusted to round-trip through
	// storage. A directory-mode session, or a worktree already cleaned up (e.g. by a
	// prior Pause), has nothing on disk to worry about.
	if d.Worktree.WorktreePath == "" {
		return true, ""
	}
	if _, statErr := os.Stat(d.Worktree.WorktreePath); statErr != nil {
		return true, ""
	}
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

	return true, ""
}

// sessionSafeToDelete applies baseSafeToDelete plus the shared-worktree convergence
// check: backlog rework/reopen reuses the same deterministic branch (and therefore the
// same worktree directory — see session/git.findExistingWorktreeForBranch) across
// rounds, so an old archived round's session can point at the exact directory another
// round's session is using.
//
// Naively skipping deletion whenever ANY sibling still references the same path is
// wrong: for an item reopened N>=2 times, every round shares the identical path, so
// each round's row would find some other row still referencing it and block on it
// forever — even long after every round is individually archived, past retention,
// clean, and PR-terminal (see PR #303 review). Instead, a sibling only blocks deletion
// while IT is not itself independently eligible (baseSafeToDelete) — i.e. still
// genuinely in use (never archived, too recent, dirty, or has an open PR). Once every
// round sharing a path independently reaches eligibility, none of them find a blocking
// sibling anymore, and the whole group converges to deletable on the same sweep pass:
// each gets deleted via the normal per-session svc.DeleteSession call in sweep()'s loop.
// DeleteSession's own worktree cleanup (Destroy()) runs asynchronously, though, so more
// than one sibling in the group can be mid-flight against the identical physical
// directory at once — one sibling's in-flight `git worktree remove` can transiently make
// another's own dirty-check in baseSafeToDelete error out, causing that one to be
// skipped for just this cycle rather than deleted alongside its siblings. That's a safe,
// conservative outcome (not a correctness bug): the worktree it was guarding is gone
// moments later, so it converges on the very next tick instead of this exact one. See
// the *_ConvergesWhenAllSiblingsBecomeEligible test, which polls across sweep() calls
// rather than asserting a strict single-pass guarantee for this reason.
//
// Mirrors the sibling-lookup pattern backlog_service.go's cleanupItemWorktreesExcept
// already uses for the same underlying reason (shared worktree across reopens).
func (s *SessionRetentionSweeper) sessionSafeToDelete(ctx context.Context, d session.InstanceData, byUUID map[string]session.InstanceData, cutoff time.Time) (safe bool, reason string) {
	safe, reason = s.baseSafeToDelete(d, cutoff)
	if !safe {
		return false, reason
	}
	if d.Worktree.WorktreePath == "" {
		return true, ""
	}
	if _, statErr := os.Stat(d.Worktree.WorktreePath); statErr != nil {
		return true, ""
	}
	if d.UUID == "" {
		return true, ""
	}

	is, err := s.storage.GetItemSessionBySessionUUID(ctx, d.UUID)
	if err != nil || is.BacklogItemID == "" {
		return true, "" // not backlog-linked (or lookup failed) -- no sibling to share with
	}
	siblings, listErr := s.storage.ListItemSessions(ctx, is.BacklogItemID)
	if listErr != nil {
		return false, "failed to list sibling item sessions: " + listErr.Error()
	}
	for _, sib := range siblings {
		if sib.SessionUUID == "" || sib.SessionUUID == d.UUID {
			continue
		}
		sibWorktree, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, sib.SessionUUID)
		if wtErr != nil || sibWorktree.WorktreePath == "" || sibWorktree.WorktreePath != d.Worktree.WorktreePath {
			continue // sibling doesn't share this exact worktree path -- irrelevant
		}
		sibData, ok := byUUID[sib.SessionUUID]
		if !ok {
			// Sibling's session row no longer exists in storage (e.g. already deleted
			// earlier in this same sweep pass) -- nothing left to protect.
			continue
		}
		if sibSafe, _ := s.baseSafeToDelete(sibData, cutoff); !sibSafe {
			return false, "worktree shared with another session on the same backlog item that is not yet independently eligible for deletion"
		}
	}

	return true, ""
}
