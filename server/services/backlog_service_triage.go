package services

// backlog_service_triage.go — session spawning and triage/review orchestration handlers
// for BacklogService. Covers the full lifecycle of headless triage, review, and re-review.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
)

// initialPromptFor returns s.pipelineEngine.InitialPromptFor(...) when pipelineEngine
// is wired, or the default session.BuildTokenBudgetedPrompt otherwise — mirrors
// session.CachingPipelineEngine's own default-mode fallback for the nil-engine case
// (many tests construct a BacklogService without one). Used by SpawnSessionFromItem
// (Epic 1.5, Story 1.5.5) to build the prompt handed to inst.Prompt / AutonomousDriver.
//
// Appends a one-time "other active sessions in this workspace" nudge, when the
// workspacePeersNudgeFlagName feature flag is enabled and peers exist — best-effort:
// detection/lookup failures are logged and swallowed rather than blocking session creation,
// since this is a convenience nudge, not required context.
func (s *BacklogService) initialPromptFor(ctx context.Context, item *session.BacklogItemData, priorSessions []session.ItemSessionSummary) string {
	var prompt string
	if s.pipelineEngine == nil {
		prompt = session.BuildTokenBudgetedPrompt(item, priorSessions)
	} else {
		prompt = s.pipelineEngine.InitialPromptFor(item, priorSessions)
	}
	return prompt + s.workspacePeersBlockFor(ctx, item.RepoPath)
}

// workspacePeersBlockFor returns the rendered workspace-peers nudge for repoPath, or ""
// when the workspacePeersNudgeFlagName feature flag is off (default), on any
// detection/lookup failure, or when repoPath is empty. Delegates to
// session.WorkspacePeersBlockForPath, shared with SessionService.CreateSession so the two
// callers can't drift on how the nudge is built.
func (s *BacklogService) workspacePeersBlockFor(ctx context.Context, repoPath string) string {
	return workspacePeersBlockFor(ctx, s.storage, repoPath)
}

// triagePromptFor returns s.pipelineEngine.TriagePromptFor(...) when pipelineEngine is
// wired, or the default session.BuildHeadlessTriagePrompt otherwise — mirrors
// session.CachingPipelineEngine's own default-mode fallback for the nil-engine case.
// Used by TriggerTriage's FIRST-triage branch only (Epic 1.5, Story 1.5.3); the
// retriage branch always calls session.BuildHeadlessRetriagePrompt directly and is
// NOT routed through PipelineEngine — "refine the existing plan" is mode-independent
// (research/architecture.md §3).
func (s *BacklogService) triagePromptFor(item *session.BacklogItemData, artifactAbsPath string) string {
	if s.pipelineEngine == nil {
		return session.BuildHeadlessTriagePrompt(item, artifactAbsPath)
	}
	return s.pipelineEngine.TriagePromptFor(item, artifactAbsPath)
}

// reviewPromptFor returns s.pipelineEngine.ReviewPromptFor(...) when pipelineEngine is
// wired, or the default session.BuildHeadlessReviewPrompt otherwise — mirrors
// session.CachingPipelineEngine's own default-mode fallback for the nil-engine case.
// Used by TriggerReReview (Epic 1.5, Story 1.5.4); the equivalent seam for
// ReviewGateRunner.Run lives in session/review_gate.go's own reviewPromptFor method.
func (s *BacklogService) reviewPromptFor(item *session.BacklogItemData, acSnapshot []session.AcCriterion, diff string, diffTruncated bool, verificationNotes string, extras session.ReviewContextExtras) string {
	if s.pipelineEngine == nil {
		return session.BuildHeadlessReviewPrompt(item, acSnapshot, diff, diffTruncated, verificationNotes, extras)
	}
	return s.pipelineEngine.ReviewPromptFor(item, acSnapshot, diff, diffTruncated, verificationNotes, extras)
}

// effectiveReworkCap returns item's own per-item rework-cap override if set
// (BacklogItemData.ReworkCapOverride), otherwise the global default
// (config.Config.MaxAutoReworkIterationsOrDefault). 0 on the override means
// "unlimited retries for this item" — represented as math.MaxInt so every
// count comparison (workCount/reviewCount >= reworkCap) never trips.
func (s *BacklogService) effectiveReworkCap(item *session.BacklogItemData) int {
	if item != nil && item.ReworkCapOverride != nil {
		if *item.ReworkCapOverride == 0 {
			return math.MaxInt
		}
		return *item.ReworkCapOverride
	}
	return s.maxAutoReworkIterations()
}

// recentReviewHadVerdict returns up to n bools, most-recent-first, one per
// review-role ItemSession in sessions — true if that session ever had a
// ReviewVerdict row attached. sessions must be ordered oldest-first, as
// Storage.ListItemSessions returns (and as AutoReopenAfterFailedReview already
// has in hand for its work-session cap check, so this needs no extra query).
// Feeds session.IsRepeatedNoVerdictFailure.
func recentReviewHadVerdict(sessions []session.ItemSessionSummary, n int) []bool {
	out := make([]bool, 0, n)
	for i := len(sessions) - 1; i >= 0 && len(out) < n; i-- {
		if sessions[i].Role != session.SessionRoleReview {
			continue
		}
		out = append(out, sessions[i].ReviewVerdict != nil)
	}
	return out
}

// recentWorkSessionFileLists returns up to n changed-file lists (most recent
// first), one per completed (EndedAt != nil) work-role ItemSession in
// sessions — computed lazily via go-git tree comparison of each session's
// BaseCommitSha/LastCommitSha (git.FileStatsBetween; no git subshell, see
// .claude/rules/prefer-go-git-over-subshells.md). sessions must be ordered
// oldest-first, as Storage.ListItemSessions returns (mirrors
// recentReviewHadVerdict's contract above). Feeds
// session.IsTestOnlyReworkCycle.
//
// Only completed sessions are considered — reading a still-in-progress
// session's commit range risks racing an in-flight write to the same
// worktree (see validation.md's async-race edge case). A session with a
// git-diff error, or missing SHAs, still occupies a rework-cycle slot with
// an empty file list rather than being skipped outright: IsTestOnlyReworkCycle
// treats an empty list as "no signal, don't guess" for that attempt, which is
// the correct behavior for an attempt whose file data genuinely couldn't be
// computed — silently skipping it instead would let the cycle compare across
// a gap it never actually observed.
func recentWorkSessionFileLists(repoPath string, sessions []session.ItemSessionSummary, n int) [][]string {
	out := make([][]string, 0, n)
	for i := len(sessions) - 1; i >= 0 && len(out) < n; i-- {
		is := sessions[i]
		if is.Role != session.SessionRoleWork || is.EndedAt == nil {
			continue
		}
		var files []string
		if is.BaseCommitSha != "" && is.LastCommitSha != "" {
			if stats, err := git.FileStatsBetween(repoPath, is.BaseCommitSha, is.LastCommitSha); err != nil {
				log.WarningLog.Printf("[recentWorkSessionFileLists] session=%s FileStatsBetween: %v", is.ID, err)
			} else {
				for _, fs := range stats {
					files = append(files, fs.Path)
				}
			}
		}
		out = append(out, files)
	}
	return out
}

// notifyReworkCapHit publishes an operator-facing notification when the auto-rework
// loop (review→rework or PR-fix→rework) hits reworkCap (see effectiveReworkCap) and
// leaves an item stranded for manual action. No-op if no event bus is wired.
//
// Story 2.1.2: also writes a durable rework_cap BacklogStuckState row (threshold
// 0 — the cap hit is a discrete, definitive event, marked the moment it's hit)
// so the cap-hit is restart-surviving and notify-once is DB-backed, not lost on
// a missed toast. The durable write is additive to the notification, not a
// gate: a MarkStuck/MarkStuckNotified failure is logged but must never
// suppress the notification itself.
func (s *BacklogService) notifyReworkCapHit(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, capContext string, reworkCap int) {
	if s.storage != nil {
		applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonReworkCap, currentStatus,
			fmt.Sprintf("hit the %d-iteration rework cap %s. Increase the cap in Settings → Defaults, or click \"Reopen for Revision\" to try one more round manually.", reworkCap, capContext))
		if err != nil {
			log.WarningLog.Printf("[notifyReworkCapHit] MarkStuck item=%s: %v", itemID, err)
		} else if applied {
			if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonReworkCap); notifyErr != nil {
				log.WarningLog.Printf("[notifyReworkCapHit] MarkStuckNotified item=%s: %v", itemID, notifyErr)
			}
		}
	}

	if s.eventBus == nil {
		return
	}
	// itemID is passed as sessionID (not just metadata) so the notification subscriber's
	// coalescing key (sessionID:notificationType) differentiates between different backlog
	// items — see the comment on EventBusNotifier.Notify in backlog_notifier.go for the
	// full explanation of the bug this avoids.
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		"Auto-rework cap reached",
		fmt.Sprintf("%s — hit the %d-iteration rework cap %s. Left for manual review.", itemTitle, reworkCap, capContext),
		map[string]string{"item_id": itemID},
	))
}

// notifyRepeatedFailure publishes an operator-facing notification and durable
// BacklogStuckState row (reused StuckReasonBouncing — same "non-converging
// cycle with no PASS verdict" semantics as the periodic bounce sweep, just
// tripped immediately on two identical verdicts instead of waiting for
// bounceThreshold cycles within bounceLookback) when session.IsRepeatedFailure
// stops the auto-reopen loop. Mirrors notifyReworkCapHit's structure: a
// MarkStuck/MarkStuckNotified failure is logged but never suppresses the
// notification itself.
func (s *BacklogService) notifyRepeatedFailure(ctx context.Context, itemID, itemTitle string, currentStatus session.BacklogStatus, failureSummary string) {
	if s.storage != nil {
		applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, currentStatus,
			fmt.Sprintf("stopped auto-rework — the last two attempts failed the same way: %q. Fix the underlying issue, then click \"Reopen for Revision\".", failureSummary))
		if err != nil {
			log.WarningLog.Printf("[notifyRepeatedFailure] MarkStuck item=%s: %v", itemID, err)
		} else if applied {
			if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonBouncing); notifyErr != nil {
				log.WarningLog.Printf("[notifyRepeatedFailure] MarkStuckNotified item=%s: %v", itemID, notifyErr)
			}
		}
	}

	if s.eventBus == nil {
		return
	}
	// itemID as sessionID — see comment in notifyReworkCapHit above.
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		"Auto-rework stopped — repeated failure",
		fmt.Sprintf("%s — the last two attempts failed the same way, so auto-rework stopped instead of retrying. Left for manual review.", itemTitle),
		map[string]string{"item_id": itemID},
	))
}

// notifyLikelyFlaky writes a durable, purely-informational StuckReasonLikelyFlaky
// row (session/domain/backlog.go) when session.IsFlakyVerdictFlipFlop or
// session.IsTestOnlyReworkCycle matches this item's recent review history —
// behavioral evidence the review outcome may be non-deterministic rather than a
// real pass/fail signal (see plan.md's option (c): a UI/stuck-state hint, not a
// gate). Unlike notifyRepeatedFailure/notifyReworkCapHit, this never stops the
// caller and publishes no operator-facing notification — it's evaluated as a
// side observation alongside whatever AutoReopenAfterFailedReview decides next,
// specifically so a misfiring heuristic can never newly stall an item that would
// otherwise proceed. A MarkStuck/MarkStuckNotified failure is only logged,
// matching every other MarkStuck call site's own convention.
func (s *BacklogService) notifyLikelyFlaky(ctx context.Context, itemID string, currentStatus session.BacklogStatus, recentVerdicts []session.ReviewVerdictSummary, sessions []session.ItemSessionSummary, repoPath string) {
	if s.storage == nil {
		return
	}
	flip := session.IsFlakyVerdictFlipFlop(recentVerdicts)
	testOnly := false
	if repoPath != "" {
		testOnly = session.IsTestOnlyReworkCycle(recentWorkSessionFileLists(repoPath, sessions, session.TestOnlyReworkMinAttempts))
	}
	if !flip && !testOnly {
		return
	}

	reason := "the last two review verdicts landed on different outcomes for what looks like the identical diff"
	switch {
	case testOnly && flip:
		reason = "the last two review verdicts flip-flopped on an identical diff, and rework has been touching only test files"
	case testOnly:
		reason = "the last two rework attempts touched only test files — possibly chasing a flaky test rather than fixing the underlying code"
	}

	applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonLikelyFlaky, currentStatus,
		fmt.Sprintf("possibly flaky — verify before assuming: %s.", reason))
	if err != nil {
		log.WarningLog.Printf("[notifyLikelyFlaky] MarkStuck item=%s: %v", itemID, err)
	} else if applied {
		if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonLikelyFlaky); notifyErr != nil {
			log.WarningLog.Printf("[notifyLikelyFlaky] MarkStuckNotified item=%s: %v", itemID, notifyErr)
		}
	}
}

// notifyBlockedByDependency writes a durable StuckReasonBlockedByDependency row
// (session/domain/backlog.go) when DequeueNextQueuedItems finds an item still
// has unresolved blockers (AC3) — purely informational, like notifyLikelyFlaky,
// so the item detail view can render a BlockerChip instead of leaving the
// operator to guess why a queued/ready item never gets claimed. A MarkStuck/
// MarkStuckNotified failure is only logged, matching every other MarkStuck call
// site's own convention.
func (s *BacklogService) notifyBlockedByDependency(ctx context.Context, itemID string, currentStatus session.BacklogStatus) {
	if s.storage == nil {
		return
	}
	blockerIDs, err := s.storage.UnresolvedBlockerIDs(ctx, itemID)
	if err != nil {
		log.WarningLog.Printf("[notifyBlockedByDependency] UnresolvedBlockerIDs item=%s: %v", itemID, err)
		return
	}
	if len(blockerIDs) == 0 {
		// Blockers resolved between the batched check and here — nothing to report.
		return
	}

	message := fmt.Sprintf("blocked by unresolved dependency: %s", strings.Join(blockerIDs, ", "))
	applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonBlockedByDependency, currentStatus, message)
	if err != nil {
		log.WarningLog.Printf("[notifyBlockedByDependency] MarkStuck item=%s: %v", itemID, err)
	} else if applied {
		if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonBlockedByDependency); notifyErr != nil {
			log.WarningLog.Printf("[notifyBlockedByDependency] MarkStuckNotified item=%s: %v", itemID, notifyErr)
		}
	}
}

// notifySpawnAndRollbackFailed publishes an operator-facing notification and durable
// BacklogStuckState row (StuckReasonSpawnFailed) when AutoReopenAfterFailedReview's
// SpawnSessionFromItem call fails AND the subsequent scoped rollback to "review" also
// fails — previously this left the item silently stranded at in_progress with no work
// session and no visible error anywhere (BUG-030). Mirrors notifyReworkCapHit's
// structure: a MarkStuck/MarkStuckNotified failure is logged but never suppresses the
// notification itself.
func (s *BacklogService) notifySpawnAndRollbackFailed(ctx context.Context, itemID, itemTitle string, spawnErr, rollbackErr error) {
	if s.storage != nil {
		applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonSpawnFailed, session.BacklogStatusInProgress,
			fmt.Sprintf("a rework session failed to spawn (%v) and the automatic rollback to review also failed (%v) — the item is in_progress with no active session. Click \"Reopen for Revision\" or \"Run Autonomously\" to retry.", spawnErr, rollbackErr))
		if err != nil {
			log.WarningLog.Printf("[notifySpawnAndRollbackFailed] MarkStuck item=%s: %v", itemID, err)
		} else if applied {
			if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonSpawnFailed); notifyErr != nil {
				log.WarningLog.Printf("[notifySpawnAndRollbackFailed] MarkStuckNotified item=%s: %v", itemID, notifyErr)
			}
		}
	}

	if s.eventBus == nil {
		return
	}
	// itemID as sessionID — see comment in notifyReworkCapHit above.
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Rework failed to start",
		fmt.Sprintf("%s — a rework session failed to spawn and the automatic rollback also failed. The item is stranded in_progress with no active session; needs manual action.", itemTitle),
		map[string]string{"item_id": itemID},
	))
}

// notifyTriagePersistFailure publishes an operator-facing notification when one or more of
// the post-triage persistence steps (saving the triage result, saving the plan artifacts
// path, or transitioning the item to Ready) fails. These failures previously only reached
// the log file — never the operator — so an item could complete triage successfully and
// still sit stuck at 'idea' forever with no signal. No-op if no event bus is wired.
func (s *BacklogService) notifyTriagePersistFailure(ctx context.Context, itemID, itemTitle string, failures []string, statusAdvanced bool) {
	if s.eventBus == nil {
		return
	}
	title := "Triage completed but a save step failed"
	body := fmt.Sprintf("%s — triage ran successfully, but failed: %s.", itemTitle, strings.Join(failures, "; "))
	if !statusAdvanced {
		body += " The item is still at 'idea' — retry manually or re-trigger triage."
	}
	// itemID as sessionID — see comment in notifyReworkCapHit above.
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		title, body,
		map[string]string{"item_id": itemID},
	))
}

// notifyTransitionFailed publishes an operator-facing notification when a backlog
// item's status-transition (or session-bookkeeping) write fails AFTER its side
// effects have already happened — e.g. a work session was already spawned and
// persisted, or code was already confirmed shipped to main — leaving the item's
// status silently out of sync with reality. Previously these failures only
// reached the log file, invisible to the operator and to every stuck-item
// sweep (the recurring "silent status-transition failure" bug shape: BUG-030,
// BUG-040, BUG-041, BUG-046, BUG-048, and the sibling call sites fixed
// alongside this helper's introduction — see
// docs/tasks/backlog-feature-improvement.md's 2026-07-27 update).
//
// Mirrors notifyTriagePersistFailure's shape (notification-only, no durable
// BacklogStuckState row) rather than notifyReworkCapHit's — there is no single
// good StuckReason bucket for "a routine write failed after the fact"; the
// value here is making the failure visible immediately, not classifying it.
// No-op if no event bus is wired.
func (s *BacklogService) notifyTransitionFailed(itemID, itemTitle, failureContext string, writeErr error) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Status update failed after work completed",
		fmt.Sprintf("%s — %s: %v. The item's status may not reflect reality; check manually.", itemTitle, failureContext, writeErr),
		map[string]string{"item_id": itemID},
	))
}

// notifyManualOverride publishes an operator-facing notification when a human
// manually associates a PR or forces a status transition on a backlog item.
// Unlike notifyTransitionFailed above, this fires on the SUCCESS path — a
// manual override is exactly the kind of edge-case state change other
// sessions/agents and reconciliation sweeps will read and act on without
// surrounding narrative (a work session polling get_backlog_item after a
// force-transition has no way to know why it jumped). No-op if no event bus
// is wired.
func (s *BacklogService) notifyManualOverride(itemID, itemTitle, message string) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
		"Manual override applied",
		fmt.Sprintf("%s — %s", itemTitle, message),
		map[string]string{"item_id": itemID},
	))
}

// headlessTriageUUIDPrefix is prepended to all synthetic ItemSession UUIDs created by the
// headless triage path. The orphan guard uses this prefix to identify sessions that have no
// live tmux process and can be safely tombstoned on re-trigger.
// headlessReReviewUUIDPrefix is the equivalent prefix for headless re-review sessions.
const (
	headlessTriageUUIDPrefix   = "headless-triage-"
	headlessReReviewUUIDPrefix = "headless-re-review-"
)

// triageCallBudget bounds a single headless triage LLM call (TriggerTriage's own
// triageCtx). session.maxHeadlessTriageSessionStaleness (session/backlog_lifecycle.go)
// — the periodic sweep's threshold for treating a still-open triage session as
// dead — MUST stay strictly greater than this, with enough margin that a call
// finishing at (or timing out at) its own full budget has already ended by the
// time the sweep's next tick considers it stale; otherwise the sweep and this
// call's own natural completion/timeout race on every slow call. See BUG-055.
const triageCallBudget = 30 * time.Minute

// The auto-rework iteration cap bounds how many automated work sessions can be
// spawned for a single backlog item by the auto-reopen loop. When this ceiling
// is hit, the item stays in review so a human can inspect it rather than
// spinning indefinitely on a persistent FAIL verdict.
//
// Configurable via config.Config.MaxAutoReworkIterationsOrDefault() (Settings →
// Defaults, default 3) — call sites read s.maxAutoReworkIterations(), not a
// constant. That helper (not s.cfg directly) is required: cfg is a live,
// shared *config.Config instance DefaultsService.UpdateGlobalDefaults can
// write to concurrently (see cfgMu's doc comment on the BacklogService
// struct), so reads must go through the mutex-guarded accessor.

// The backlog work-item concurrency cap is configurable via
// config.Config.MaxConcurrentBacklogWorkItemsOrDefault() (Settings → Defaults,
// default 2) — call sites read s.maxConcurrentBacklogWorkItems(), not a
// constant, for the same cfgMu-guarded-accessor reason described above.
// Fresh spawns beyond the cap are queued (BacklogStatusQueued) instead of
// rejected; reopen/revision spawns for an item that's already in_progress
// don't count against it, since they don't add a new concurrent item.
//
// Added 2026-07-12 after a kernel OOM caused by too many concurrent agent
// sessions (backlog-spawned and otherwise) exhausting system memory.

// defaultTriageCleanupTimeout bounds the DB writes TriggerTriage's goroutine makes
// after its headless LLM call returns (persist result, update plan_artifacts_path,
// transition idea->ready, mark session ended). See BacklogService.triageCleanupTimeout
// for why this needed to become configurable rather than a global.
const defaultTriageCleanupTimeout = 10 * time.Second

// maxTriageSessionAge is the maximum age of an open triage ItemSession before it is
// treated as orphaned in the re-trigger guard. This prevents a hung or leaked session
// from blocking re-trigger indefinitely.
const maxTriageSessionAge = 2 * time.Hour

// prFixMainBranch is the branch AutoReopenForPRFix syncs a PR's branch against before
// respawning a fix session. This repo's convention is "main" (see CLAUDE.md).
const prFixMainBranch = "main"

// slugify converts s to a lowercase hyphen-delimited slug safe for file paths.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// triageShortTitle extracts the triage-suggested short title from the most recent
// completed triage ItemSession, falling back to a truncated slug of itemTitle.
func triageShortTitle(sessions []session.ItemSessionSummary, itemTitle string) string {
	for i := len(sessions) - 1; i >= 0; i-- {
		s := sessions[i]
		if s.Role != string(session.SessionRoleTriage) || s.TriageResult == "" {
			continue
		}
		var r session.HeadlessTriageResult
		if err := json.Unmarshal([]byte(s.TriageResult), &r); err == nil && r.Title != "" {
			return r.Title
		}
	}
	// Fallback: first 4 words of the slug.
	parts := strings.SplitN(slugify(itemTitle), "-", 5)
	if len(parts) > 4 {
		parts = parts[:4]
	}
	return strings.Join(parts, "-")
}

// backlogWorkBranchSlug is the single source of truth for the deterministic
// worktree/branch slug a backlog item's work session uses, given its repo path
// and a short title identifying it (the triage-suggested title, or
// triageShortTitle's item.Title-derived fallback). session.CreateBacklogWorktree
// (session/instance_worktree.go) prefixes this with "backlog/" to get the real
// git branch.
//
// This function exists because the formula was previously duplicated —
// spawnSessionAfterGates computed it inline for a real spawn, and
// retitleTriageWorktreeToFinalBranch (below) independently recomputed it ahead
// of time so the triage worktree could be renamed onto the same branch before
// spawn ever runs. The two silently drifted once already (see
// TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession,
// which failed against that duplicated version — a fresh worktree was created
// from main instead of reusing triage's). Every caller that needs this slug
// MUST go through this function, not reimplement the formula.
func backlogWorkBranchSlug(repoPath, title string) string {
	repoName := slugify(filepath.Base(repoPath))
	return slugify(repoName + "-" + title)
}

// retitleTriageWorktreeToFinalBranch moves wt's branch from its provisional
// "triage-<item-id>" name onto the exact "backlog/<repo>-<title>" branch
// spawnSessionAfterGates will independently compute and look for once this item
// reaches a real work session (via backlogWorkBranchSlug — see its doc comment
// for why both sides must share that one function); title comes from
// triageShortTitle, which picks up this exact title from the triage result
// this goroutine is about to persist. So the eventual work session reuses this
// same worktree, and its already-committed planning docs, instead of starting
// fresh from main.
//
// Best-effort: any failure (including the target branch already being checked
// out elsewhere — a stale leftover from an earlier run, most likely) just
// leaves wt on its provisional branch, logged but non-fatal. The committed docs
// are never lost either way, only not picked up automatically —
// spawnSessionAfterGates falls back to creating its own worktree off main, same
// as if this had never run.
func retitleTriageWorktreeToFinalBranch(itemID, repoPath, title string, wt *git.GitWorktree) {
	if title == "" {
		return
	}
	finalBranch := session.BacklogBranchPrefix + backlogWorkBranchSlug(repoPath, title)

	if renameErr := wt.RenameBranch(finalBranch); renameErr != nil {
		log.WarningLog.Printf("[TriggerTriage] failed to rename triage worktree branch for item=%s to %q: %v", itemID, finalBranch, renameErr)
	}
}

// cleanupProvisionalTriageWorktree removes a triage worktree that was created
// for this run but never reached the commit+rename step (LLM call failed,
// result parsing failed) — otherwise it's an orphaned triage-<itemID>
// worktree/branch that nothing ever reuses or removes. Once
// retitleTriageWorktreeToFinalBranch has run, the worktree is promoted for
// reuse and must not be cleaned up here.
func cleanupProvisionalTriageWorktree(itemID string, wt *git.GitWorktree) {
	if wt == nil {
		return
	}
	if cleanupErr := wt.Cleanup(); cleanupErr != nil {
		log.WarningLog.Printf("[TriggerTriage] failed to clean up provisional triage worktree for item=%s: %v", itemID, cleanupErr)
	}
}

func (s *BacklogService) SpawnSessionFromItem(
	ctx context.Context,
	req *connect.Request[sessionv1.SpawnSessionFromItemRequest],
) (*connect.Response[sessionv1.SpawnSessionFromItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// A spawn is user-initiated unless the caller explicitly marks it Autonomous
	// (the autonomous driver spawning its own follow-up sessions).
	triggeredBy := session.TriggeredByUser
	if req.Msg.Autonomous {
		triggeredBy = session.TriggeredBySystem
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 1b. Atomic check-and-set: only one SpawnSessionFromItem call for this item may be
	// in flight at a time. Without this, two concurrent calls (e.g. AutoReopenAfterFailedReview
	// / AutoRespawnAutonomousWork / AutoReopenForPRFix all funnel here, and any of them can
	// race a manual retrigger or a periodic reconciliation sweep) can both pass the
	// hasActiveWorkSession guard below (step 8b) before either has written its new
	// ItemSession row, producing two concurrent work sessions for one item — see
	// spawnInFlight's doc comment on the BacklogService struct for the live incident this
	// closes. Released via defer so every return path (including early gate failures below)
	// frees the item for the next attempt.
	if _, alreadyInFlight := s.spawnInFlight.LoadOrStore(item.ID, struct{}{}); alreadyInFlight {
		log.InfoLog.Printf("[SpawnSessionFromItem] spawn already in flight for item=%s; rejecting concurrent attempt", item.ID)
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a session spawn is already in progress for this item; wait for it to finish"))
	}
	defer s.spawnInFlight.Delete(item.ID)

	// 2. If force=true, clear any in-flight sessions and reset status so the normal
	// path below can proceed. Handles both in_progress (stop work session) and review
	// (stop review session + transition back to in_progress so restart begins from
	// the work phase where the git worktree and slash commands are set up).
	if req.Msg.Force && (item.Status == string(session.BacklogStatusInProgress) ||
		item.Status == string(session.BacklogStatusReview)) {
		var forceErr error
		item, forceErr = s.forceResetItem(ctx, item, triggeredBy)
		if forceErr != nil {
			return nil, forceErr
		}
	}

	// 3. Validate status. Allow ready (first spawn) or in_progress (re-spawn after reopen).
	isReopen := item.Status == string(session.BacklogStatusInProgress)
	if item.Status != string(session.BacklogStatusReady) && !isReopen {
		log.InfoLog.Printf("[SpawnSessionFromItem] status gate blocked spawn item=%s status=%s autonomous=%v", item.ID, item.Status, req.Msg.Autonomous)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to spawn a session, got %q — use TriggerTriage to advance from %q",
				session.BacklogStatusReady, session.BacklogStatusInProgress, item.Status, item.Status))
	}

	// 3b. Planning gate (only for fresh spawns; on reopen planning is already approved).
	// Autonomous mode bypasses the gate — the driver handles its own planning loop.
	// Deliberately runs BEFORE the WIP-cap gate below: an item without an approved
	// plan must be rejected outright here, never queued — a queued item skips this
	// RPC entirely on dequeue (DequeueNextQueuedItems calls spawnSessionAfterGates
	// directly), so queueing an unapproved-plan item would let it reach a real
	// spawned session with no planning check at all (PR #199 review F2/F3).
	if !isReopen && !item.SkipPlanning && !item.PlanApproved && !req.Msg.Autonomous {
		log.InfoLog.Printf("[SpawnSessionFromItem] planning gate blocked spawn item=%s status=%s autonomous=false", item.ID, item.Status)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("run TriggerTriage and approve the plan before spawning, or use 'Run Autonomously' to skip the planning gate"))
	}

	// 4. WIP limit gate (only for fresh spawns; a reopen doesn't add a new concurrent
	// item — it's already counted as in_progress). Not bypassed by Autonomous: the
	// point is to cap total concurrent agent load regardless of how a spawn was
	// triggered. At the cap, the item is queued (BacklogStatusQueued) rather than
	// rejected — BacklogLifecycleListener.onSessionExited and the periodic
	// ReconcileStuck sweep dequeue it once a slot frees up (DequeueNextQueuedItems).
	if !isReopen {
		liveCount, wipErr := s.countLiveBacklogWorkSessions(ctx)
		if wipErr != nil {
			log.WarningLog.Printf("[SpawnSessionFromItem] WIP count query failed item=%s: %v; allowing spawn", item.ID, wipErr)
		} else if wipCap := s.maxConcurrentBacklogWorkItems(); liveCount >= wipCap {
			log.InfoLog.Printf("[SpawnSessionFromItem] WIP limit hit item=%s live=%d cap=%d — queueing", item.ID, liveCount, wipCap)
			if _, queueErr := s.queueBacklogItem(ctx, item, req.Msg.Autonomous); queueErr != nil {
				return nil, queueErr
			}
			return connect.NewResponse(&sessionv1.SpawnSessionFromItemResponse{Queued: true}), nil
		}
	}

	return s.spawnSessionAfterGates(ctx, item, isReopen, req.Msg.Autonomous)
}

// transitionWithGuard runs the domain transition-guard checks — structural
// CanTransition plus the business-rule ValidateGates (e.g. ErrPlanRequired for
// queued->in_progress) — before delegating to storage.TransitionBacklogItemStatus.
// These are the exact two checks TransitionBacklogItemStatus's generic RPC
// handler (backlog_service_lifecycle.go) always applies; queueBacklogItem and
// DequeueNextQueuedItems's dequeue claim previously called
// storage.TransitionBacklogItemStatus directly — a pure CAS with no guard at
// all — which let an unapproved-plan item reach a real spawned session via
// ready->queued->in_progress with the planning gate never once evaluated
// (PR #199 review F3, structural root cause F4). Every status-mutating call
// site outside the generic RPC handler should route through this helper so a
// future call site can't reintroduce the same bug class.
//
// hasUnresolvedBlockers is a per-item convenience wrapper around
// storage.UnresolvedBlockerItemIDs for call sites transitioning a single item
// (queueBacklogItem, TransitionBacklogItemStatus). DequeueNextQueuedItems
// instead batches this lookup across all candidates up front to avoid an N+1
// query per dequeue sweep — see its own call to UnresolvedBlockerItemIDs.
func (s *BacklogService) hasUnresolvedBlockers(ctx context.Context, itemID string) (bool, error) {
	unresolved, err := s.storage.UnresolvedBlockerItemIDs(ctx, []string{itemID})
	if err != nil {
		return false, err
	}
	return unresolved[itemID], nil
}

// Returns the same errors storage.TransitionBacklogItemStatus returns
// (ErrPreconditionFailed, etc.) on success of the guard checks, or the raw
// domain sentinel error (ErrPlanRequired, ErrACRequired, ...) if a guard
// fails — un-wrapped in connect terms so each call site keeps doing its own
// connect.NewError translation, matching this file's existing style.
//
// hasUnresolvedBlockers is supplied by the caller rather than computed here
// so DequeueNextQueuedItems can batch the underlying query once across all
// candidates instead of once per claim (see its own call to
// storage.UnresolvedBlockerItemIDs before the claim loop).
func (s *BacklogService) transitionWithGuard(ctx context.Context, item *session.BacklogItemData, to session.BacklogStatus, precondition *session.BacklogItemPrecondition, triggeredBy string, hasUnresolvedBlockers bool) (*session.BacklogItemData, error) {
	from := session.BacklogStatus(item.Status)
	if !s.engine.CanTransition(from, to) {
		return nil, fmt.Errorf("invalid transition from %q to %q", from, to)
	}
	guardInput := session.BacklogItemTransitionInput{
		Status:                from,
		AcCriteria:            item.AcceptanceCriteria,
		PlanApproved:          item.PlanApproved,
		SkipPlanning:          item.SkipPlanning,
		PlanArtifactsPath:     item.PlanArtifactsPath,
		HasUnresolvedBlockers: hasUnresolvedBlockers,
	}
	if guardErr := s.engine.ValidateGates(guardInput, to); guardErr != nil {
		return nil, guardErr
	}
	return s.storage.TransitionBacklogItemStatus(ctx, item.ID, to, precondition, triggeredBy)
}

// queueBacklogItem transitions item from ready to queued after a fresh spawn hit
// the concurrency cap. queued_at (FIFO dequeue order) and the autonomous flag the
// original request carried are written BEFORE the status transition so no reader
// ever observes status=queued with queue metadata still unset.
func (s *BacklogService) queueBacklogItem(ctx context.Context, item *session.BacklogItemData, autonomous bool) (*session.BacklogItemData, error) {
	now := time.Now()
	if _, err := s.storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{
		QueuedAt:         &now,
		QueuedAutonomous: &autonomous,
	}, nil); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to record queue metadata: %w", err))
	}
	// A spawn is user-initiated unless the caller explicitly marks it Autonomous
	// (the autonomous driver spawning its own follow-up sessions).
	triggeredBy := session.TriggeredByUser
	if autonomous {
		triggeredBy = session.TriggeredBySystem
	}
	precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReady), Note: "WIP cap hit"}
	// Blocker guard only applies to the queued/ready -> in_progress transition, so
	// queueing (-> Queued) always passes false rather than querying.
	updated, err := s.transitionWithGuard(ctx, item, session.BacklogStatusQueued, precondition, triggeredBy, false)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, fmt.Errorf("item status changed concurrently — retry the spawn: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to queue item: %w", err))
	}
	return updated, nil
}

// DequeueNextQueuedItems implements session.QueueDequeuer. It claims and spawns as
// many queued items as there are free WIP slots, highest-priority (P1) first, oldest
// (by queued time, or created time for a "ready" candidate that was never explicitly
// queued) as the tiebreaker. When autoSpawnReadyItemsEnabled (default true —
// config.Config.AutoSpawnReadyItemsOrDefault) is set, "ready" items are eligible
// candidates too, not just ones already sitting in "queued" — this is what makes
// auto-implementation the default: an item reaching "ready" no longer needs either a
// human to click "Spawn Session" or an explicit AutoSpawnSession flag to eventually
// get worked, it just needs a free WIP slot and the highest priority among what's
// waiting. Called from BacklogLifecycleListener.onSessionExited (immediate dequeue
// the moment a slot frees up) and the periodic ReconcileStuck sweep (safety net for a
// missed hook, a concurrency limit raised while items were waiting, or an item that
// reached "ready" between ticks) — see session/backlog_lifecycle.go.
//
// Each candidate is claimed via a SQL-level compare-and-swap (queued->in_progress or
// ready->in_progress, ExpectedStatus set to whichever status the candidate was found
// in) before spawning, so concurrent callers (this method running from both the exit
// hook and the sweep, or multiple server processes sharing one DB) cannot double-claim
// the SAME item — see TransitionBacklogItemStatus's doc comment. That per-item CAS
// alone does not prevent two concurrent calls to this method from each computing
// their own freeSlots from an unsynchronized snapshot and jointly claiming DIFFERENT
// candidates past the cap, so dequeueMu additionally serializes the whole method
// body, making this method single-flight system-wide (PR #199 review F2 — the exact
// "uncontrolled concurrency overshoot" class of bug the WIP cap feature exists to
// prevent).
//
// The claim itself now goes through transitionWithGuard (PR #199 review F4), so an
// item without an approved plan (SkipPlanning=false, PlanApproved=false) cannot be
// claimed at all — defense-in-depth against F3, on top of SpawnSessionFromItem's own
// planning gate now running before the WIP-cap queue gate. This is also what makes a
// "ready" candidate safe to auto-claim directly: ready->in_progress carries the exact
// same ErrPlanRequired/ErrPlanArtifactsRequired guard as queued->in_progress (see
// domain.TransitionGuard), so an unapproved-plan item is silently skipped (left at
// ready) rather than auto-spawned without review.
//
// If the claim succeeds but the spawn itself fails (missing repo_path, stale plan
// approval, SessionCreator error), the item is rolled back to whichever status it was
// claimed from (queued or ready) rather than left stranded in_progress with no session.
func (s *BacklogService) DequeueNextQueuedItems(ctx context.Context) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}
	s.dequeueMu.Lock()
	defer s.dequeueMu.Unlock()

	liveCount, err := s.countLiveBacklogWorkSessions(ctx)
	if err != nil {
		return fmt.Errorf("count live work sessions: %w", err)
	}
	freeSlots := s.maxConcurrentBacklogWorkItems() - liveCount
	if freeSlots <= 0 {
		return nil
	}

	statuses := []string{string(session.BacklogStatusQueued)}
	if s.autoSpawnReadyItemsEnabled() {
		statuses = append(statuses, string(session.BacklogStatusReady))
	}
	candidates, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: statuses,
	})
	if err != nil {
		return fmt.Errorf("list queued/ready items: %w", err)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority // P1 (1) before P5 (5)
		}
		return effectiveQueueTime(candidates[i]).Before(effectiveQueueTime(candidates[j]))
	})

	candidateIDs := make([]string, 0, len(candidates))
	for _, item := range candidates {
		candidateIDs = append(candidateIDs, item.ID)
	}
	// Batched once across all candidates (not per-claim) to avoid an N+1 query in
	// this loop — see transitionWithGuard's doc comment.
	unresolvedBlockers, err := s.storage.UnresolvedBlockerItemIDs(ctx, candidateIDs)
	if err != nil {
		return fmt.Errorf("check unresolved blockers: %w", err)
	}

	spawned := 0
	for _, item := range candidates {
		if spawned >= freeSlots {
			break
		}
		fromStatus := session.BacklogStatus(item.Status)
		claimed, claimErr := s.transitionWithGuard(ctx, &item,
			session.BacklogStatusInProgress,
			&session.BacklogItemPrecondition{ExpectedStatus: string(fromStatus), Note: "dequeued: WIP slot freed"},
			session.TriggeredBySystem,
			unresolvedBlockers[item.ID])
		if claimErr != nil {
			switch {
			case errors.Is(claimErr, session.ErrPreconditionFailed):
				// Expected under concurrent claims (another process's dequeue
				// sweep, or a manual un-queue) — not worth logging.
			case errors.Is(claimErr, session.ErrUnresolvedBlockers):
				// Expected steady state: item is legitimately blocked and will be
				// retried on a later sweep once its blocker reaches done — not a
				// bug, so not worth a warning-level log every sweep. Still surface
				// it durably (AC3) so the item detail view can render a BlockerChip
				// instead of leaving the operator to guess why it's stalled.
				s.notifyBlockedByDependency(ctx, item.ID, fromStatus)
			case errors.Is(claimErr, session.ErrPlanRequired), errors.Is(claimErr, session.ErrPlanArtifactsRequired):
				// Defense-in-depth (PR #199 review F2/F3): should be unreachable
				// now that SpawnSessionFromItem's planning gate runs before the
				// WIP-cap queue gate, but refuse the claim rather than silently
				// spawning an unapproved item if this is ever hit (e.g. a future
				// call site regression, or a pre-existing queued/ready row from
				// before that ordering fix).
				log.WarningLog.Printf("[DequeueNextQueuedItems] claim blocked by planning gate item=%s status=%s: %v — leaving as-is", item.ID, fromStatus, claimErr)
			default:
				log.WarningLog.Printf("[DequeueNextQueuedItems] claim failed item=%s status=%s: %v", item.ID, fromStatus, claimErr)
			}
			continue
		}

		resp, spawnErr := s.spawnSessionAfterGates(ctx, claimed, true, item.QueuedAutonomous)
		if spawnErr != nil {
			log.WarningLog.Printf("[DequeueNextQueuedItems] spawn failed for dequeued item=%s: %v; rolling back to %s", item.ID, spawnErr, fromStatus)
			if _, rbErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, fromStatus,
				&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress), Note: "dequeue spawn failed"},
				session.TriggeredBySystem); rbErr != nil {
				log.ErrorLog.Printf("[DequeueNextQueuedItems] rollback to %s failed item=%s: %v", fromStatus, item.ID, rbErr)
				// The same silent-stranding shape notifySpawnAndRollbackFailed was
				// built for (BUG-030) — that fix only wired this helper into
				// AutoReopenAfterFailedReview's own spawn+rollback path, missing this
				// sibling one. The item is left claimed (in_progress) with no live
				// session and no visible error anywhere.
				s.notifySpawnAndRollbackFailed(ctx, item.ID, item.Title, spawnErr, rbErr)
			}
			continue
		}
		spawned++
		log.InfoLog.Printf("[DequeueNextQueuedItems] dequeued and spawned item=%s (was %s, priority=%d) session=%s", item.ID, fromStatus, item.Priority, resp.Msg.SessionUuid)
	}
	return nil
}

// effectiveQueueTime is the timestamp DequeueNextQueuedItems' priority-tiebreaker sort
// uses: QueuedAt for a genuinely queued item, or CreatedAt for a "ready" candidate that
// was never explicitly queued (QueuedAt is nil for those) — so older work still wins
// ties within the same priority tier regardless of which status it's waiting in.
func effectiveQueueTime(item session.BacklogItemData) time.Time {
	if item.QueuedAt != nil {
		return *item.QueuedAt
	}
	return item.CreatedAt
}

// spawnSessionAfterGates performs the actual session spawn for item once all gating
// checks (status, WIP cap, planning approval) have passed. Used by SpawnSessionFromItem
// (fresh spawn / manual reopen) and by DequeueNextQueuedItems — in the dequeue case
// isReopen is always true, since the item's status has already been CAS-transitioned to
// in_progress by the caller before this runs, and step 13 below must not re-transition it.
func (s *BacklogService) spawnSessionAfterGates(
	ctx context.Context,
	item *session.BacklogItemData,
	isReopen bool,
	autonomous bool,
) (*connect.Response[sessionv1.SpawnSessionFromItemResponse], error) {
	// 4b. Planning-gate defense-in-depth (PR #199 review F2/F3). SpawnSessionFromItem's
	// own planning gate (step 3b) only runs on that RPC's direct call path;
	// DequeueNextQueuedItems claims a queued item via transitionWithGuard (which itself
	// now enforces this — F4) and then calls this method directly, with no other gate in
	// between. Re-checking here means an unapproved-plan item can never reach a real
	// spawned session no matter which call site reaches this function, now or in the
	// future. Skipped when autonomous=true (the driver runs its own planning loop) —
	// this matches SpawnSessionFromItem's own gate and means it never fires for
	// AutoReopenAfterFailedReview/AutoReopenForPRFix, which always pass autonomous=true.
	if !item.SkipPlanning && !item.PlanApproved && !autonomous {
		log.InfoLog.Printf("[spawnSessionAfterGates] planning gate blocked spawn item=%s status=%s autonomous=false", item.ID, item.Status)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("run TriggerTriage and approve the plan before spawning, or use 'Run Autonomously' to skip the planning gate"))
	}

	// 5. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before spawning a session"))
	}

	// 6. Require SessionCreator before doing any DB writes.
	// degraded: sessionCreator unavailable — return CodeUnimplemented so callers can detect the gap.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// 7. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

	// 8. Load prior sessions for context.
	priorSessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] failed to load prior sessions for item %s: %v", item.ID, err)
		priorSessions = nil
	}

	// 8a. Tombstone any work session that never reached its normal completion path
	// (crash, kill, server restart mid-session) before checking 8b below — otherwise a
	// single dead session blocks every future spawn attempt for this item forever. Found
	// live: AutoReopenForPRFix retried every ~60s against the same dead session for
	// hours, bouncing the item in_progress<->pr_pending with no progress (see
	// docs/tasks/backlog-feature-improvement.md).
	s.tombstoneOrphanWorkSessions(ctx, item.ID, priorSessions)

	// 8a2. Close the tmux pane of every already-ended work-session round before
	// spawning the next one. Each rework round gets its own "-rN" title (see
	// buildRevisionTitle) so the session list stays readable across rounds, but
	// nothing previously closed a finished round's tmux pane — it sat around
	// indefinitely as an idle "[exited]" pane, accumulating with every rework
	// cycle. KillTmuxPaneOnly (not StopSessionByUUID/Instance.Kill) leaves the
	// worktree alone, since rework rounds share one worktree/branch.
	s.killEndedWorkSessionPanes(ctx, priorSessions)

	// 8b. Guard against spawning a duplicate work session when one is already active.
	if active := findActiveWorkSession(priorSessions); active != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, s.activeWorkSessionBlockedError(active))
	}

	// 8. Build agent prompt. Routed through PipelineEngine (Epic 1.5, Story 1.5.5) so a
	// non-default PipelineMode changes what inst.Prompt / AutonomousDriver's goal sees.
	prompt := s.initialPromptFor(ctx, item, priorSessions)

	// 9. Generate session title.
	// On reopen, append a revision number (r2, r3…) based on how many work sessions
	// already exist so the session list shows distinct, human-readable names.
	shortTitle := triageShortTitle(priorSessions, item.Title)
	baseTitle := slugify(filepath.Base(item.RepoPath)) + "-" + shortTitle
	title := buildRevisionTitle(baseTitle, isReopen, priorSessions)

	// 10. Create a dedicated git worktree for this work session. The branch slug
	// comes from backlogWorkBranchSlug(item.RepoPath, shortTitle) — the same
	// single formula TriggerTriage's retitleTriageWorktreeToFinalBranch
	// independently computes ahead of time (see its doc comment for why both
	// sides must share it) — so rework/reopen iterations reuse the same
	// "backlog/<item>" branch instead of minting a new one per -rN revision, and
	// so a triage worktree already committed on that branch gets reused here
	// instead of a fresh one from main. The worktree setup path already detects
	// and reuses an existing branch (see git.GitWorktree.Setup).
	// Falls back to a plain directory session if the repo is not git-managed (or
	// worktree creation fails for any other reason — e.g. a bare clone, a detached
	// HEAD, or disk quota hit).
	// Files must be written to the session path BEFORE spawning.
	// worktreeMu guards concurrent spawns from interleaving writes to the same path. It
	// also has to cover resolveSessionPath's check-then-create window: two concurrent
	// spawns for the same backlog item compute the identical deterministic branch name
	// (backlogWorkBranchSlug) and can otherwise both pass CreateBacklogWorktree's
	// branch-existence check before either creates the branch, and the loser hits a
	// "branch already exists" failure (session/git/worktree_ops.go's setupNewWorktree
	// self-heals that specific error, but serializing here closes the race at the
	// source instead of just recovering from it after the fact).
	s.worktreeMu.Lock()
	worktreePath, useWorktree, resolveErr := resolveSessionPath(item.RepoPath, backlogWorkBranchSlug(item.RepoPath, shortTitle))
	if resolveErr != nil {
		s.worktreeMu.Unlock()
		return nil, resolveErr
	}

	wErr := s.writeSessionFilesLocked(item, priorSessions, worktreePath)
	s.worktreeMu.Unlock()
	if wErr != nil {
		return nil, wErr
	}

	// 11. Spawn session first so we have the real UUID before creating the ItemSession record.
	spawnTags := []string{session.TagBacklogWork}
	if isReopen {
		spawnTags = append(spawnTags, session.TagBacklogRevision)
	}
	if autonomous {
		spawnTags = append(spawnTags, session.TagAutonomous)
	}
	var inst *session.Instance
	if useWorktree {
		inst, err = s.sessionCreator.CreateWorktreeSession(ctx, title, item.RepoPath, worktreePath, prompt,
			spawnTags, false, false)
	} else {
		inst, err = s.sessionCreator.CreateDirectorySession(ctx, title, worktreePath, prompt,
			spawnTags, false, false)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn session: %w", err))
	}
	inst.SetCategory(session.CategoryBacklog)

	// Steering hook: git-drift-check fires the same branch-drift detection BUG-044's
	// review-gate precondition uses, but after every git commit/push instead of only
	// at review time — so an autonomous session notices and can self-correct while
	// still working. Scoped STRICTLY to autonomous=true (AutonomousDriver-run, no
	// human attached) — never injected for a manual spawn, the generic create_session
	// MCP tool, or a human-initiated "Reopen for Revision" (which does not pass
	// autonomous=true; see BacklogItemDetail.tsx's handleGateReopen). The worktree
	// path (and its "backlog/<item>" branch) is reused across reopen cycles, so on
	// every spawn we reconcile in BOTH directions — inject when autonomous, actively
	// remove when not — rather than only ever adding: without the removal side, a
	// worktree that was ever spawned autonomously once would keep this hook wired
	// into a later manual session on the same worktree forever.
	driftHook := []HookName{HookGitDriftCheck}
	if autonomous {
		if hookErr := InjectHooksConfig(inst.GetEffectiveRootDir(), inst.Title, driftHook); hookErr != nil {
			log.WarningLog.Printf("[SpawnSessionFromItem] git-drift-check hook injection failed item=%s session=%s: %v", item.ID, inst.UUID, hookErr)
		}
	} else if hookErr := RemoveHooksConfig(inst.GetEffectiveRootDir(), driftHook); hookErr != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] git-drift-check hook removal failed item=%s session=%s: %v", item.ID, inst.UUID, hookErr)
	}

	// Persist the instance (and its Worktree row, with BaseCommitSha) synchronously now
	// rather than waiting for the next periodic SaveInstances sweep. The review gate looks
	// up worktree data by session UUID as soon as request_review fires from inside the
	// spawned session; without this, a fast work session can request review before the
	// worktree row exists, causing the review gate to fall back to an unreliable diff.
	if saveErr := s.storage.SaveInstances([]*session.Instance{inst}); saveErr != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] failed to persist instance immediately after spawn item=%s session=%s: %v", item.ID, inst.UUID, saveErr)
	}

	if autonomous {
		if s.autonomousStarter != nil {
			log.InfoLog.Printf("[SpawnSessionFromItem] starting autonomous driver item=%s session=%s", item.ID, inst.UUID)
			s.autonomousStarter.StartAutonomousDriverForInstance(inst)
		} else {
			log.WarningLog.Printf("[SpawnSessionFromItem] autonomous=true but no driver starter wired item=%s session=%s — session will need manual approval", item.ID, inst.UUID)
		}
	}

	// 12. Create ItemSession with the real session UUID (avoids "<pending>" orphan records on failure).
	// Snapshot the resolved PipelineMode slug + content hash at the moment this session
	// first starts (Epic 1.6) — see pipelineEngine's field doc comment on BacklogService
	// for why the hash lookup is nil-guarded (Epic 1.5 has not yet wired a real engine
	// into the constructor; item.PipelineMode itself is always recorded regardless).
	var pipelineModeSnapshotHash string
	if s.pipelineEngine != nil {
		pipelineModeSnapshotHash, _ = s.pipelineEngine.ContentHashFor(session.PipelineMode(item.PipelineMode))
	}
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:                   item.ID,
		SessionUUID:              inst.UUID,
		SessionRole:              session.SessionRoleWork,
		AcSnapshot:               acSnapshot,
		PipelineModeSnapshot:     item.PipelineMode,
		PipelineModeSnapshotHash: pipelineModeSnapshotHash,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 12b. Capture the pre-work HEAD SHA so the review gate can diff base..HEAD across
	// all commits the agent makes (not just HEAD~1..HEAD at review time). This goes in
	// BaseCommitSha, never LastCommitSha — see SetItemSessionBaseCommit's doc comment.
	if baseSHA, shaErr := session.GetGitHeadSHA(worktreePath); shaErr == nil && baseSHA != "" {
		_ = s.storage.SetItemSessionBaseCommit(ctx, is.ID, baseSHA)
		inst.SetDirBaseSHA(baseSHA)
	}

	// 12c. On reopen, clean up git worktrees from prior work sessions now that the
	// new session is safely persisted. Best-effort only — errors are logged, not returned.
	// worktreePath itself is exempted: step 10 reuses the same "backlog/<item>" worktree
	// across reopens (same branch slug every revision), so priorSessions still contains a
	// worktree row pointing at this exact path — cleaning it up here would delete the
	// directory the session spawned above just started using.
	if isReopen {
		s.cleanupItemWorktreesExcept(ctx, priorSessions, worktreePath)
		// Archive the superseded prior-round work session(s) now that the new
		// session has replaced them — otherwise every rework round piles up a
		// fresh work session that's never cleaned up until the item eventually
		// reaches done/archived (see docs/tasks/workflow-history-and-archiving.md;
		// this is the fix for items that bounce through many rework rounds while
		// still open).
		s.archiveItemWorkSessions(ctx, priorSessions)
	}

	// 13. Transition item to in_progress. No-op for isReopen: a manual reopen is
	// already in_progress, and a dequeue claim already CAS'd the item to in_progress
	// before calling this helper.
	if !isReopen {
		// A spawn is user-initiated unless the caller explicitly marks it
		// Autonomous (the autonomous driver spawning its own follow-up sessions).
		triggeredBy := session.TriggeredByUser
		if autonomous {
			triggeredBy = session.TriggeredBySystem
		}
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil, triggeredBy); transErr != nil {
			log.ErrorLog.Printf("[SpawnSessionFromItem] failed to transition item to in_progress: %v", transErr)
			// The work session and worktree above are already created and
			// persisted — a live session is now running for this item while its
			// status still says otherwise (also invisible to
			// countLiveBacklogWorkSessions' WIP-cap accounting, which only counts
			// in_progress/review items).
			s.notifyTransitionFailed(item.ID, item.Title, "a work session was spawned but the item's transition to in_progress failed", transErr)
		}
	}

	return connect.NewResponse(&sessionv1.SpawnSessionFromItemResponse{
		SessionUuid: inst.UUID,
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// forceResetItem stops any in-flight work or review sessions for the item, and — if
// the item is currently in review — transitions it back to in_progress. Used when
// SpawnSessionFromItem is called with Force=true so the caller can re-spawn cleanly.
func (s *BacklogService) forceResetItem(ctx context.Context, item *session.BacklogItemData, triggeredBy string) (*session.BacklogItemData, error) {
	earlyPrior, _ := s.storage.ListItemSessions(ctx, item.ID)
	for _, ps := range earlyPrior {
		if ps.EndedAt != nil {
			continue
		}
		if ps.Role != string(session.SessionRoleWork) && ps.Role != string(session.SessionRoleReview) {
			continue
		}
		if s.sessionStopper != nil {
			_ = s.sessionStopper.StopSessionByUUID(ctx, ps.SessionUUID)
		}
		_ = s.storage.UpdateItemSessionEnded(ctx, ps.ID, time.Now())
	}
	if item.Status == string(session.BacklogStatusReview) {
		updated, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil, triggeredBy)
		if transErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reset item to in_progress for restart: %w", transErr))
		}
		return updated, nil
	}
	return item, nil
}

// countLiveBacklogWorkSessions counts backlog items that currently have an active
// (unended) work-session agent running, across both "in_progress" and "review" status —
// not just "in_progress". AutoReopenAfterFailedReview intentionally leaves a work session
// alive (polling for a review verdict) after the item's status flips back to "review", so
// counting "in_progress" items alone undercounts real concurrent agent load and lets the
// WIP cap (maxConcurrentBacklogWorkItems) be silently exceeded — see
// docs/tasks/backlog-feature-improvement.md's "WIP limit now undercounts live sessions"
// finding, tied to the 2026-07-12 OOM incident the cap exists to prevent.
func (s *BacklogService) countLiveBacklogWorkSessions(ctx context.Context) (int, error) {
	candidates, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusInProgress), string(session.BacklogStatusReview)},
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range candidates {
		if item.Status == string(session.BacklogStatusInProgress) {
			count++
			continue
		}
		// review status only counts toward the cap if a work session is still
		// actually running (the case AutoReopenAfterFailedReview's live-session
		// reuse makes invisible to a naive in_progress-only count).
		sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[countLiveBacklogWorkSessions] list sessions failed item=%s: %v; assuming no active session", item.ID, sessErr)
			continue
		}
		if hasActiveWorkSession(sessions) {
			count++
		}
	}
	return count, nil
}

// hasActiveWorkSession reports whether any of the provided ItemSessions is an
// open (not yet ended) work-role session.
func hasActiveWorkSession(priorSessions []session.ItemSessionSummary) bool {
	return findActiveWorkSession(priorSessions) != nil
}

// findActiveWorkSession returns the open (not yet ended) work-role
// ItemSession, if any — the same match hasActiveWorkSession checks for, but
// returning the session itself so a caller blocked by it (spawnSessionAfterGates'
// 8b guard) can enrich its error with a progress signal instead of a bare
// "already active."
func findActiveWorkSession(priorSessions []session.ItemSessionSummary) *session.ItemSessionSummary {
	for i := range priorSessions {
		if priorSessions[i].Role == session.SessionRoleWork && priorSessions[i].EndedAt == nil {
			return &priorSessions[i]
		}
	}
	return nil
}

// activeWorkSessionBlockedError builds the CodeAlreadyExists error for
// spawnSessionAfterGates' 8b guard, enriched with the same progress signal
// notifyIfActiveWorkSessionStale already computes for the review-reopen path
// (via the shared respawnBlockedActiveProgressSignal/workSessionStaleness
// helpers below) — without this, a caller blocked here has no way to tell
// "still working" from "silently stuck" short of manually cross-referencing a
// session's last-activity timestamp, a full diff pull, and a live tmux check
// (see docs/tasks/backlog-feature-improvement.md's 2026-07-31 entry:
// discovered live while manually unsticking backlog item 04089969). Reuses
// maxReworkBlockStaleness (15min) as the same "stalled" threshold that path
// already established, rather than inventing a second one.
func (s *BacklogService) activeWorkSessionBlockedError(active *session.ItemSessionSummary) error {
	base := fmt.Sprintf("a work session (%s) is already active for this item; wait for it to finish or kill it first", active.SessionUUID)
	return fmt.Errorf("%s — %s", base, s.respawnBlockedActiveProgressSignal(active.SessionUUID))
}

// maxReworkBlockStaleness is the threshold notifyIfActiveWorkSessionStale (and
// its resolve-side counterpart, ResolveReworkBlockedStaleIfRecovered) compare
// idle-since-last-meaningful-output against when deciding whether a work
// session blocking a rework attempt is stale enough to durably flag. 15
// minutes — see ADR-001-staleness-threshold-recalibration.md for the full
// rationale. Intentionally distinct from both maxWorkSessionStaleness (2h,
// session/backlog_lifecycle.go — tuned for a quietly-running in_progress
// item, a less urgent scenario) and the Review Queue's own
// ReviewQueuePollerConfig.StalenessThreshold (5min after this same change —
// a low-stakes "might be worth a look" badge). Used by both the mark side
// (this function) and the resolve side so they agree on one number.
const maxReworkBlockStaleness = 15 * time.Minute

// workSessionStaleness reports how long sessionUUID has been idle (no
// meaningful output) and whether it currently exceeds maxReworkBlockStaleness.
// Shared by the three call sites that all need this exact computation —
// activeWorkSessionBlockedError (the blocked-spawn error, enriched with a
// progress signal), notifyIfActiveWorkSessionStale (the mark side of the
// rework-blocked-stale notification), and ResolveReworkBlockedStaleIfRecovered
// (its resolve-side counterpart) — so the "idle vs. threshold" comparison
// can't drift between them. live mirrors TimeSinceLastMeaningfulOutput's own
// liveness flag; idle and stale are meaningless when live is false. Returns
// (0, false, false) when no sessionStopper is wired — callers that need to
// distinguish "no stopper" from "stopper says not live" (activeWorkSessionBlockedError
// does, for its own distinct error message) must still check s.sessionStopper == nil
// themselves before calling this.
func (s *BacklogService) workSessionStaleness(sessionUUID string) (idle time.Duration, live, stale bool) {
	if s.sessionStopper == nil {
		return 0, false, false
	}
	idle, live = s.sessionStopper.TimeSinceLastMeaningfulOutput(sessionUUID)
	return idle, live, live && idle > maxReworkBlockStaleness
}

// respawnBlockedActiveProgressSignal renders the same "still active" vs.
// "likely stalled" vs. "progress signal unavailable" text
// activeWorkSessionBlockedError already gives spawnSessionAfterGates' 8b
// guard, built from the shared workSessionStaleness computation. Factored out
// so notifyRespawnBlockedByActiveSession doesn't duplicate that three-way
// branch — the same duplication-avoidance activeWorkSessionBlockedError
// itself already applies by calling workSessionStaleness rather than
// recomputing idle/live/stale locally.
func (s *BacklogService) respawnBlockedActiveProgressSignal(activeSessionUUID string) string {
	if s.sessionStopper == nil {
		return "progress signal unavailable: sessionStopper not wired"
	}
	idle, live, stale := s.workSessionStaleness(activeSessionUUID)
	if !live {
		return "progress signal unavailable: session not currently tracked live"
	}
	if stale {
		return fmt.Sprintf("likely stalled: no output in %s (stale threshold %s)", idle.Round(time.Second), maxReworkBlockStaleness)
	}
	return fmt.Sprintf("still active: output %s ago", idle.Round(time.Second))
}

// notifyRespawnBlockedByActiveSession closes the audit-trail gap where
// AutoRespawnAutonomousWork, AutoReopenForPRFix, and AutoRespawnReview's
// findActiveWorkSession/findActiveReviewSession guards previously only
// log.InfoLog.Printf'd the skip and returned nil — zero operator-visible
// signal and no audit record, strictly worse than spawnSessionAfterGates' own
// 8b guard (activeWorkSessionBlockedError), which at least returns a
// progress-enriched error to its synchronous caller (see
// docs/tasks/backlog-feature-improvement.md, 2026-07-31/2026-08-03 updates).
//
// Mirrors notifyReworkCapHit's structure (MarkStuck + MarkStuckNotified +
// eventBus notification), reusing the dedicated
// domain.StuckReasonRespawnBlockedActive reason so the item's stuck-reason
// history can tell "blocked because a session is already running" apart from
// rework_cap/bouncing/rework_blocked_stale. Reuses
// respawnBlockedActiveProgressSignal (built on the same workSessionStaleness
// helper activeWorkSessionBlockedError/notifyIfActiveWorkSessionStale/
// ResolveReworkBlockedStaleIfRecovered already share) so an operator checking
// a skipped item here gets the same "still active" vs. "likely stalled"
// distinction instead of a bare log line.
//
// Deliberately NOT gated on staleness (unlike notifyIfActiveWorkSessionStale)
// — these three call sites previously had zero signal even for a healthy,
// still-progressing block, which is the gap being closed here. Deliberately
// NOT gated on MarkStuck's `applied` return either, matching
// notifyReworkCapHit/notifySpawnAndRollbackFailed's existing precedent: those
// helpers already re-publish a notification on every call rather than only
// once per open row, and callers of this helper are naturally rate-limited
// the same way (each fires once per reconcile-triggered respawn attempt, not
// on a tight poll loop).
func (s *BacklogService) notifyRespawnBlockedByActiveSession(ctx context.Context, caller, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID string) {
	progress := s.respawnBlockedActiveProgressSignal(activeSessionUUID)
	log.InfoLog.Printf("[%s] item %s already has an active session %s; skipping respawn — %s", caller, itemID, activeSessionUUID, progress)

	if s.storage != nil {
		applied, err := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive, currentStatus,
			fmt.Sprintf("%s skipped auto-respawn — session %s already active (%s)", caller, activeSessionUUID, progress))
		if err != nil {
			log.WarningLog.Printf("[%s] MarkStuck(respawn_blocked_active) item=%s: %v", caller, itemID, err)
		} else if applied {
			if _, notifyErr := s.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonRespawnBlockedActive); notifyErr != nil {
				log.WarningLog.Printf("[%s] MarkStuckNotified(respawn_blocked_active) item=%s: %v", caller, itemID, notifyErr)
			}
		}
	}

	// Publishes unconditionally, even when MarkStuck's `applied` came back
	// false (e.g. the item's status changed out from under this call between
	// the guard check and here) or MarkStuck errored outright — a known,
	// pre-existing inconsistency this helper inherits from
	// notifyReworkCapHit/notifySpawnAndRollbackFailed (both do the same),
	// not something to "fix" here in isolation without touching those
	// siblings too.
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
		"Auto-respawn skipped — session already active",
		fmt.Sprintf("%s — automatic respawn (%s) was skipped because a session is already active: %s.", itemTitle, caller, progress),
		map[string]string{"item_id": itemID},
	))
}

// resolveRespawnBlockedActiveLogged clears an open
// StuckReasonRespawnBlockedActive row for itemID, logging (not returning) any
// storage error — mirrors resolveReworkBlockedStaleLogged. Called by all
// three notifyRespawnBlockedByActiveSession callers once their active-session
// guard passes (the block has cleared), so the row doesn't outlive the
// condition it records.
func (s *BacklogService) resolveRespawnBlockedActiveLogged(ctx context.Context, caller, itemID string) {
	if s.storage == nil {
		return
	}
	if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive); err != nil {
		log.WarningLog.Printf("[%s] ResolveStuck(respawn_blocked_active) item=%s: %v", caller, itemID, err)
	}
}

// notifyIfActiveWorkSessionStale closes the "zero operator signal" half of a
// live gap: AutoReopenAfterFailedReview's hasActiveWorkSession guard treats
// any work session with EndedAt == nil as "in flight" and skips reopening
// so the live agent can pick up the verdict itself (see the guard's own
// comment above). That check is purely liveness-based — it says nothing
// about whether the session is actually making progress. A session can be
// technically alive (tmux pane exists, DB row open) for hours with zero
// real output, and this guard has no way to tell the difference, so the
// item silently sits stuck with nothing surfaced to the operator. Confirmed
// live 2026-07-20 on backlog item 9264efe7: session
// stapler-squad-fix-backlog-status-audit-trail-r15 reported Active with a
// current last_activity_at, while review_queue_determiner.go's own,
// independently-computed staleness detector flagged the same session
// "STALENESS DETECTED ... 6h 35m since last meaningful output" on every
// reconciliation tick.
//
// This function does NOT change the reopen decision — a live session is
// never stopped, killed, or bypassed here, regardless of how stale it is.
// This repo has a deliberate policy against force-stopping a slow-but-alive
// agent (see docs/tasks/backlog-feature-improvement.md's StuckReasonStaleWork
// discussion and the stop_session-deletes-branch incident) — killing the
// session ourselves would just trade one bug for a worse one. The staleness
// check uses its own dedicated threshold, maxReworkBlockStaleness (15min —
// see ADR-001-staleness-threshold-recalibration.md), not the Review Queue's
// StalenessThreshold — the two were previously conflated, which caused a
// single slow LLM turn to routinely misfire this gate.
//
// In addition to the notification below, this function durably marks the
// item StuckReasonReworkBlockedStale (session/domain/backlog.go) via
// MarkStuck so the state survives past the one-shot toast — see that
// constant's doc comment for the full mark/resolve lifecycle.
//
// Best-effort and silent by design when it can't observe anything: no
// sessionStopper/eventBus wired, no active work session found (shouldn't
// happen — the caller already confirmed hasActiveWorkSession), or the
// session isn't currently tracked live (ok == false) all skip quietly,
// leaving the existing reconcileBouncingItems/reconcileStaleWorkSessions
// sweeps as the fallback signal, same as before this function existed.
//
// Naturally rate-limited without extra dedup bookkeeping: this only runs
// from inside AutoReopenAfterFailedReview, which itself is gated by
// autoReopenWithBackoffGate's RemediationDue backoff (minimum 30 minutes
// between attempts) once the item has been marked "bouncing" — the exact
// state this bug report describes.
func (s *BacklogService) notifyIfActiveWorkSessionStale(ctx context.Context, itemID, itemTitle string, sessions []session.ItemSessionSummary) {
	if s.sessionStopper == nil || s.eventBus == nil {
		return
	}
	var active *session.ItemSessionSummary
	for i := range sessions {
		if sessions[i].Role == session.SessionRoleWork && sessions[i].EndedAt == nil {
			active = &sessions[i]
			break
		}
	}
	if active == nil {
		return
	}
	idle, _, stale := s.workSessionStaleness(active.SessionUUID)
	if !stale {
		return
	}
	log.WarningLog.Printf("[AutoReopenAfterFailedReview] item %s reopen blocked by active work session %s that is itself stale (%s since last meaningful output, threshold %s)",
		itemID, active.SessionUUID, idle.Round(time.Second), maxReworkBlockStaleness)

	// Durably mark the item, best-effort: a storage error or a status
	// precondition mismatch (item moved off review between read and write)
	// must never block or skip the notification below — that publish is the
	// one pre-existing behavior this addition must not regress.
	if applied, markErr := s.storage.MarkStuck(ctx, itemID, domain.StuckReasonReworkBlockedStale, session.BacklogStatusReview,
		fmt.Sprintf("active work session %s idle %s since last meaningful output", active.SessionUUID, idle.Round(time.Second))); markErr != nil {
		log.WarningLog.Printf("[AutoReopenAfterFailedReview] item %s MarkStuck(rework_blocked_stale) error: %v", itemID, markErr)
	} else if !applied {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s MarkStuck(rework_blocked_stale) skipped — status precondition no longer holds", itemID)
	}

	// itemID as sessionID — see comment in notifyReworkCapHit above.
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		"Rework blocked by a stale-but-alive session",
		fmt.Sprintf("%s — a failed review can't reopen for another rework attempt because its active work session hasn't produced output in over %s. The session is still running, so it will not be stopped automatically; check it manually, or use \"Reopen for Revision\" once you've confirmed it's actually stuck.", itemTitle, idle.Round(time.Second)),
		map[string]string{"item_id": itemID},
	))
}

// ResolveReworkBlockedStaleIfRecovered implements
// session.ReworkBlockStaleResolver — the resolve-side counterpart to
// notifyIfActiveWorkSessionStale above, called from
// BacklogLifecycleListener.reconcileReworkBlockedStaleResolution once per
// open StuckReasonReworkBlockedStale row per reconcile tick. Re-checks the
// item's active work session's current staleness and clears the row
// (storage.ResolveStuck) if any of three conditions hold: the session is
// producing output again (recovered), it no longer has an active work
// session, or the item has left review status — the last two are
// belt-and-suspenders alongside selfHealStuck's own status-anchored clear,
// matching reconcileStaleWorkSessions' identical justification for its own
// resolve pass (same-status clears are invisible to status-anchored
// self-heal). No-op (nil error) if still stale. Best-effort: a ResolveStuck
// error is logged and swallowed here (not returned) so one item's storage
// hiccup can't abort the tick for every other open row — mirroring
// resolveStuckLogged's established style.
func (s *BacklogService) ResolveReworkBlockedStaleIfRecovered(ctx context.Context, itemID string) error {
	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if item.Status != string(session.BacklogStatusReview) {
		s.resolveReworkBlockedStaleLogged(ctx, itemID)
		return nil
	}

	sessions, sessErr := s.storage.ListItemSessions(ctx, itemID)
	if sessErr != nil {
		return fmt.Errorf("list sessions: %w", sessErr)
	}
	var active *session.ItemSessionSummary
	for i := range sessions {
		if sessions[i].Role == session.SessionRoleWork && sessions[i].EndedAt == nil {
			active = &sessions[i]
			break
		}
	}
	if active == nil {
		s.resolveReworkBlockedStaleLogged(ctx, itemID)
		return nil
	}

	if s.sessionStopper == nil {
		return nil
	}
	_, _, stale := s.workSessionStaleness(active.SessionUUID)
	if !stale {
		s.resolveReworkBlockedStaleLogged(ctx, itemID)
	}
	return nil
}

// resolveReworkBlockedStaleLogged clears an open StuckReasonReworkBlockedStale
// row for itemID, logging (not returning) any storage error — see
// ResolveReworkBlockedStaleIfRecovered's doc comment for why this must never
// abort the caller's reconcile tick.
func (s *BacklogService) resolveReworkBlockedStaleLogged(ctx context.Context, itemID string) {
	if _, resolveErr := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonReworkBlockedStale); resolveErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] ResolveReworkBlockedStaleIfRecovered ResolveStuck item=%s: %v", itemID, resolveErr)
	}
}

// findActiveReviewSession returns the open (not yet ended) review-role
// ItemSession, if any — the review-role counterpart of findActiveWorkSession.
// Used by AutoRespawnReview to avoid double-spawning a review pass that is
// already running, and to give its skip branch the session UUID it needs to
// enrich notifyRespawnBlockedByActiveSession with a progress signal instead
// of a bare "already active."
func findActiveReviewSession(priorSessions []session.ItemSessionSummary) *session.ItemSessionSummary {
	for i := range priorSessions {
		if priorSessions[i].Role == session.SessionRoleReview && priorSessions[i].EndedAt == nil {
			return &priorSessions[i]
		}
	}
	return nil
}

// HasActiveReviewSession reports whether any of the provided ItemSessions is an
// open (not yet ended) review-role session. Mirrors hasActiveWorkSession; used by
// AutoRespawnReview to avoid double-spawning a review pass that is already running,
// and by server/mcp's request_review handler to refuse re-routing a pr_pending item
// out from under a running reviewer (FR2).
func HasActiveReviewSession(priorSessions []session.ItemSessionSummary) bool {
	return findActiveReviewSession(priorSessions) != nil
}

// buildRevisionTitle returns the session title for a backlog work session. On reopen
// (isReopen=true) it appends "-rN" where N is one past the existing work-session count.
func buildRevisionTitle(baseTitle string, isReopen bool, priorSessions []session.ItemSessionSummary) string {
	if !isReopen {
		return baseTitle
	}
	workCount := 0
	for _, s := range priorSessions {
		if s.Role == string(session.SessionRoleWork) {
			workCount++
		}
	}
	return fmt.Sprintf("%s-r%d", baseTitle, workCount+1)
}

// resolveSessionPath determines the file-system path for a new work session.
// It first tries to create a git worktree; if that fails it falls back to a plain
// directory session ONLY when repoPath is not git-managed at all. Returns the
// resolved path, whether a worktree was used, and any error.
//
// BUG-057: a worktree-creation failure on a repo that IS git-managed (disk
// quota, a detached HEAD, a locked ref — anything past the "not a git repo"
// check) used to fall back to session.ResolveSessionPath(repoPath), which
// returns repoPath itself unscoped. CreateDirectorySession would then spawn
// the session directly in the live checkout — editing and committing against
// the real working tree of whatever repo the backlog item targets, main
// included — instead of failing loudly. Confirmed: this is the shape that
// left session-resume-fix work committed directly on stapler-squad's own
// main branch outside a worktree.
func resolveSessionPath(repoPath, slug string) (worktreePath string, useWorktree bool, err error) {
	wt, wtErr := session.CreateBacklogWorktree(repoPath, slug)
	if wtErr == nil {
		return wt, true, nil
	}

	resolvedRepo, pathErr := session.ResolveSessionPath(repoPath)
	if pathErr != nil {
		return "", false, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_path: %w", pathErr))
	}

	if git.IsGitRepo(resolvedRepo) {
		// No special-case fallback for a zero-commit repo: findGitRepoRoot
		// (session/git/util.go, called by both GitWorktree constructors before
		// Setup runs) auto-creates an initial commit for exactly that case, so
		// CreateBacklogWorktree already succeeds instead of erroring — see
		// TestResolveSessionPath_should_CreateWorktree_When_RepoHasNoInitialCommit.
		// Any other git-managed worktree failure must hard-fail per BUG-057, not
		// silently fall back to an unscoped directory session.
		log.ErrorLog.Printf("[SpawnSessionFromItem] worktree creation failed for git-managed repo %s (%v)", resolvedRepo, wtErr)
		return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create git worktree: %w", wtErr))
	}

	log.WarningLog.Printf("[SpawnSessionFromItem] %s is not git-managed, falling back to directory mode (%v)", resolvedRepo, wtErr)
	if dirErr := session.EnsureDirectorySessionPath(resolvedRepo); dirErr != nil {
		return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to prepare session directory: %w", dirErr))
	}
	return resolvedRepo, false, nil
}

// writeSessionFilesLocked writes the backlog slash-command files and context file to the
// session directory. Callers must hold s.worktreeMu — see spawnSessionAfterGates, which
// locks it around this call and the preceding resolveSessionPath to close the
// check-then-create worktree race described there.
func (s *BacklogService) writeSessionFilesLocked(item *session.BacklogItemData, priorSessions []session.ItemSessionSummary, worktreePath string) error {
	if wErr := session.WriteSlashCommands(s.pipelineEngine, item, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
	}
	if wErr := session.WriteBacklogContextFile(item, priorSessions, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
	}
	return nil
}

// AutoReopenAfterFailedReview implements session.AutoReopenSpawner.
// It always transitions the item from review back to in_progress (required
// for request_review to become callable again — see its hardcoded
// ExpectedStatus precondition), then either spawns a new work session or, if
// one is already alive for this item, leaves it in place to continue and
// re-request review — so the review→rework cycle runs without manual
// intervention either way.
func (s *BacklogService) AutoReopenAfterFailedReview(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	// Load item to check current status and obtain updated_at for the precondition.
	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}

	// Iteration cap: count prior work sessions so we don't spin forever on a
	// persistent FAIL verdict. Fail-safe: if the DB query errors we cannot know
	// the true count, so we bail rather than risk an unbounded loop.
	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}

	// Tombstone any work session confirmed dead before checking liveness,
	// mirroring AutoRespawnAutonomousWork's and AutoReopenForPRFix's identical
	// guard (see AutoReopenForPRFix's doc comment for the incident this
	// precaution exists for). Without this, hasActiveWorkSession below is
	// purely DB-liveness (Role == Work && EndedAt == nil) — if the work
	// session is ALSO a zombie (not just the reviewer), it would be treated as
	// "active," the item would transition to in_progress with no live agent,
	// and nothing would spawn a replacement until the separate staleness
	// sweep catches it later.
	s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)

	// The work session for this round may still be alive (it stays running and
	// polls get_backlog_item after request_review — see taskProtocolBlock step 8).
	// Spawning a new one would fail on the hasActiveWorkSession guard anyway and
	// would throw away its conversation (and prompt cache), so spawning is still
	// skipped below when this is true — but the transition to in_progress must
	// NOT be skipped: request_review's own precondition hardcodes
	// ExpectedStatus: in_progress (server/mcp/tools_backlog.go), so without this
	// transition the live work session can never call request_review again no
	// matter how many times it fixes the noted gaps — it fails identically
	// forever with "concurrent modification detected: expected status
	// in_progress, got review". Confirmed live 2026-08-03 on backlog item
	// 40a243b0: the review session that recorded the FAIL verdict died before
	// its exit ever reached handleReviewSessionExited;
	// reconcileUnprocessedReviewVerdicts' crash-recovery sweep correctly
	// force-processed the FAIL verdict and called into this function, which
	// used to return here without transitioning — permanently wedging the item
	// in "review" with its live work session unable to make any further
	// progress. See TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress.
	//
	// notifyIfActiveWorkSessionStale still runs here, before the transition,
	// while the BacklogStatusReview precondition it durably marks against
	// still holds — it only surfaces a stale-but-alive session to the
	// operator and does not gate the transition below.
	activeWork := hasActiveWorkSession(sessions)
	if activeWork {
		s.notifyIfActiveWorkSessionStale(ctx, itemID, item.Title, sessions)
	}

	// Circuit breaker: if the last two verdicts failed for the identical reason,
	// another rework attempt won't change anything either — stop before burning
	// through the (possibly much larger) rework cap and park the item for
	// automated or human remediation instead. Checked ahead of the cap so a
	// fast-looping infrastructure fault (e.g. a broken worktree diff) can't spend
	// the whole cap in minutes.
	recentVerdicts, verdictErr := s.storage.GetRecentReviewVerdictSummaries(ctx, itemID, 2)

	// Story 3.2.1: purely informational — evaluated unconditionally, on every
	// call, regardless of whether a circuit breaker below trips and returns
	// early. Never gates the reopen/park decision itself (see
	// StuckReasonLikelyFlaky's doc comment); a likely_flaky row can and often
	// will co-occur with e.g. a bouncing row on the same underlying evidence
	// (see plan.md's correlated-signal note).
	s.notifyLikelyFlaky(ctx, itemID, session.BacklogStatus(item.Status), recentVerdicts, sessions, item.RepoPath)

	if verdictErr != nil {
		log.WarningLog.Printf("[AutoReopenAfterFailedReview] item %s GetRecentReviewVerdictSummaries: %v", itemID, verdictErr)
	} else if session.IsRepeatedFailure(recentVerdicts) {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s failed the same way twice in a row; leaving in review for remediation instead of reopening", itemID)
		s.notifyRepeatedFailure(ctx, itemID, item.Title, session.BacklogStatus(item.Status), recentVerdicts[0].Summary)
		return nil
	}

	// Circuit breaker, no-verdict shape: GetRecentReviewVerdictSummaries above
	// queries itemsession.HasReviewVerdict(), so a review session that crashed,
	// was killed, or hit its turn cap before ever calling submit_review_verdict
	// is invisible to the check above — the IsRepeatedFailure comparison above
	// never even sees it, so it can never trip on this failure shape no matter
	// how many times it repeats. sessions (already fetched above for the work
	// session cap check) has the review-role entries with ReviewVerdict
	// eagerly loaded, so no extra query is needed. See
	// session.IsRepeatedNoVerdictFailure's doc comment for the live bounce
	// loop (78 cycles in 24h) this closes.
	if session.IsRepeatedNoVerdictFailure(recentReviewHadVerdict(sessions, 2)) {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s: the last two review sessions both exited without ever writing a verdict; leaving in review for remediation instead of reopening", itemID)
		s.notifyRepeatedFailure(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "review session exited without ever writing a verdict")
		return nil
	}

	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	if reworkCap := s.effectiveReworkCap(item); workCount >= reworkCap {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s has %d work sessions (cap %d); leaving in review for manual action", itemID, workCount, reworkCap)
		s.notifyReworkCapHit(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "after a failed review verdict", reworkCap)
		return nil
	}

	// Transition review → in_progress with a precondition to guard against races
	// (e.g. concurrent manual reopen firing at the same time).
	updatedAt := item.UpdatedAt
	precondition := &session.BacklogItemPrecondition{
		ExpectedStatus:    string(session.BacklogStatusReview),
		ExpectedUpdatedAt: &updatedAt,
		Note:              "auto-reopened after failed review verdict",
	}
	inProgress, transitionErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusInProgress, precondition, session.TriggeredBySystem)
	if transitionErr != nil {
		return fmt.Errorf("transition to in_progress: %w", transitionErr)
	}

	// The item just left review for in_progress — resolve any open rework_cap
	// or abandoned_review rows immediately (Task 2.1.5b) rather than waiting
	// for the self-heal sweep's next tick.
	if _, resolveErr := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonReworkCap); resolveErr != nil {
		log.WarningLog.Printf("[AutoReopenAfterFailedReview] ResolveStuck(rework_cap) item=%s: %v", itemID, resolveErr)
	}
	if _, resolveErr := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonAbandonedReview); resolveErr != nil {
		log.WarningLog.Printf("[AutoReopenAfterFailedReview] ResolveStuck(abandoned_review) item=%s: %v", itemID, resolveErr)
	}

	if activeWork {
		// Reuse the live work session instead of spawning a new one — spawning
		// would fail against SpawnSessionFromItem's own hasActiveWorkSession gate
		// anyway, and would discard the live session's conversation/prompt
		// cache. The item is now back at in_progress, so that session's next
		// request_review call succeeds instead of failing the precondition
		// check forever.
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s transitioned to in_progress; reusing its active work session instead of respawning", itemID)
		return nil
	}

	_, spawnErr := s.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	if spawnErr != nil {
		// Roll back: item should stay in review rather than stranded in in_progress
		// with no active session. ReconcileStuckItems is an eventual fallback, but
		// an explicit rollback provides faster recovery.
		//
		// The rollback precondition is tied to the in_progress row *this call*
		// just wrote (ExpectedUpdatedAt: inProgress.UpdatedAt), not applied
		// unconditionally. An unconditional rollback (precondition: nil) would
		// blindly overwrite whatever status the item is in by the time the
		// rollback runs — including a "done" reached in the meantime by a
		// completely different, legitimate path (the live work session shipping
		// on its own). That is exactly what happened live on 2026-07-20 to
		// backlog item 0fd4a940 (PR #176): SpawnSessionFromItem failed after the
		// item had already shipped, and the unconditional rollback silently
		// dragged an already-done item back to "review" with no audit note,
		// kicking off a stale-verdict reprocessing cascade. Scoping the
		// precondition here means the rollback only fires if nothing else has
		// touched the item since this function's own in_progress write landed.
		rollbackPrecondition := &session.BacklogItemPrecondition{
			ExpectedStatus:    string(session.BacklogStatusInProgress),
			ExpectedUpdatedAt: &inProgress.UpdatedAt,
		}
		if _, rollbackErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReview, rollbackPrecondition, session.TriggeredBySystem); rollbackErr != nil {
			log.ErrorLog.Printf("[AutoReopenAfterFailedReview] rollback to review failed for item %s: %v", itemID, rollbackErr)
			// The item is now stranded in_progress with no active session and no
			// visible error anywhere else (BUG-030) — a log line nobody reads.
			// Mark it durably stuck so the reconciliation sweep and the operator
			// both see it, instead of it sitting invisible forever.
			s.notifySpawnAndRollbackFailed(ctx, itemID, item.Title, spawnErr, rollbackErr)
		}
		return fmt.Errorf("spawn session: %w", spawnErr)
	}
	return nil
}

// AutoRespawnAutonomousWork implements the AutonomousStuckRespawner interface
// consumed by AutonomousOrchestrationService. It gives an in_progress item a
// fresh autonomous work-session turn budget after a work session hits its
// turn cap without a DONE signal, instead of forcing the item through a
// review cycle against known-incomplete work (see onAutonomousDriverComplete's
// SessionRoleWork case in autonomous_orchestration_service.go, and
// docs/tasks/backlog-feature-improvement.md, 2026-07-19 update, for the
// bounce loop this closes). No status transition is needed — the item is
// already in_progress — so this mirrors AutoReopenAfterFailedReview's guard
// and cap checks without the review→in_progress transition step.
func (s *BacklogService) AutoRespawnAutonomousWork(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if session.BacklogStatus(item.Status) != session.BacklogStatusInProgress {
		// Already moved on (a human acted manually, or another reconciler beat
		// us to it) — nothing to do.
		return nil
	}

	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}

	// Tombstone any work session confirmed dead before checking liveness,
	// mirroring AutoReopenForPRFix's identical guard — the driver-complete
	// callback that triggered this call already ended the session record, but
	// a race with another respawn attempt is still possible.
	s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
	if active := findActiveWorkSession(sessions); active != nil {
		s.notifyRespawnBlockedByActiveSession(ctx, "AutoRespawnAutonomousWork", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
		return nil
	}
	s.resolveRespawnBlockedActiveLogged(ctx, "AutoRespawnAutonomousWork", itemID)

	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	if reworkCap := s.effectiveReworkCap(item); workCount >= reworkCap {
		log.InfoLog.Printf("[AutoRespawnAutonomousWork] item %s has %d work sessions (cap %d); leaving in_progress for manual action", itemID, workCount, reworkCap)
		s.notifyReworkCapHit(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "after repeatedly hitting the autonomous turn cap without finishing", reworkCap)
		return nil
	}

	_, spawnErr := s.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	if spawnErr != nil {
		return fmt.Errorf("spawn session: %w", spawnErr)
	}
	log.InfoLog.Printf("[AutoRespawnAutonomousWork] item %s respawned with a fresh turn budget", itemID)
	return nil
}

// RemediateStaleWorkSession implements session.StaleWorkRemediator, consumed
// by BacklogLifecycleListener's remediateStaleWorkWithBackoffGate
// (session/backlog_lifecycle.go). It closes out a work session that has gone
// stale (no progress reported for over session.maxWorkSessionStaleness) even
// though the underlying tmux session and pane process are still alive
// (session.Instance.TmuxAlive/PaneProcessDead) — a genuinely stale session is
// NOT a zombie the generic tmux health check would ever catch: the agent
// inside finished its own work and is idle at an interactive prompt waiting
// on a human, rather than crashed or hung (live repro 2026-07-20, item
// 9264efe7-b4c2-455a-9e2a-ab0196a63ecd, rework suffix -r14 — 14 prior rework
// rounds with nothing ever unsticking it, since detection existed but no
// remediation action did). Trusts the caller's staleness signal plus
// RemediationDue's own backoff gate rather than adding a second, possibly-
// conflicting liveness heuristic here — see StaleWorkRemediator's doc
// comment in session/backlog_lifecycle.go.
//
// Ends the stale ItemSession and delegates the actual respawn to
// AutoRespawnAutonomousWork, which already implements exactly the "in_progress
// item, no active work session, needs a fresh turn budget" case this
// produces — including the rework-cap check, so a stale-work loop is bounded
// by whichever of the rework cap or MaxRemediationAttempts (session/
// backlog_remediation.go) is tighter, never solely by a rework cap an
// operator may have set to 0 (unlimited) for a different reason.
//
// BUG-064: UpdateItemSessionEnded runs BEFORE KillTmuxPaneOnly, deliberately
// — not just cosmetically — ordered this way. Killing the tmux pane fires
// the Instance's EventStopped lifecycle notification, which
// session.BacklogLifecycleListener.onSessionExited (session/
// backlog_lifecycle.go) handles in its own goroutine by unconditionally
// transitioning any in_progress item straight to "review" the moment a work
// ItemSession's EndedAt is observed set. Live evidence (backlog item
// 2d7fac56, 2026-08-06): that goroutine's transition raced ahead of this
// function's own AutoRespawnAutonomousWork call below — which no-ops once
// item.Status is no longer in_progress ("already moved on ... nothing to
// do") — so the stale session was killed but no fresh work session was ever
// spawned, and the item silently went straight back into review carrying the
// exact same (already twice-PARTIAL) diff instead of getting a fresh turn
// budget. Ending the session here first, before the kill, guarantees
// (program-order happens-before, not a race) that whenever onSessionExited's
// goroutine eventually runs, it observes EndedAt already non-nil for this
// session and skips its own transition (see onSessionExited's own guard),
// leaving this function's AutoRespawnAutonomousWork call as the sole decider
// of what happens next.
func (s *BacklogService) RemediateStaleWorkSession(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if session.BacklogStatus(item.Status) != session.BacklogStatusInProgress {
		// Already moved on (a human acted manually, or another reconciler beat
		// us to it) — nothing to remediate.
		return nil
	}

	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions: %w", sessErr)
	}
	var active *session.ItemSessionSummary
	for i := range sessions {
		if sessions[i].Role == session.SessionRoleWork && sessions[i].EndedAt == nil {
			active = &sessions[i]
			break
		}
	}
	if active == nil {
		// The stale session already ended between when the sweep queued this
		// remediation and now (a concurrent respawn, or the agent finally
		// wrapped up on its own) — AutoRespawnAutonomousWork's own
		// hasActiveWorkSession/rework-cap guards decide whether a fresh
		// session is still warranted.
		return s.AutoRespawnAutonomousWork(ctx, itemID)
	}

	// End the ItemSession row BEFORE killing the pane (BUG-064 — see doc
	// comment above): this ordering is what lets onSessionExited's
	// already-ended guard close the race with AutoRespawnAutonomousWork below.
	now := time.Now()
	if endErr := s.storage.UpdateItemSessionEnded(ctx, active.ID, now); endErr != nil {
		return fmt.Errorf("end stale work session %s: %w", active.ID, endErr)
	}

	// Kill the stale tmux pane only (Instance.KillSession, NOT Instance.Kill),
	// keeping the worktree intact so any in-progress but uncommitted work
	// survives for the next work session to pick up. Best-effort: even if the
	// kill fails (session already gone, tmux server hiccup), still respawn
	// below rather than leaving the item stranded on a pure kill failure.
	if s.sessionStopper != nil {
		if killErr := s.sessionStopper.KillTmuxPaneOnly(ctx, active.SessionUUID); killErr != nil {
			log.WarningLog.Printf("[RemediateStaleWorkSession] item=%s session=%s: kill failed (continuing): %v", itemID, active.SessionUUID, killErr)
		}
	}
	log.InfoLog.Printf("[RemediateStaleWorkSession] item=%s ended stale work session=%s (session_uuid=%s), respawning", itemID, active.ID, active.SessionUUID)

	return s.AutoRespawnAutonomousWork(ctx, itemID)
}

// AutoReopenForPRFix implements session.PRFixSpawner. It transitions the item
// from pr_pending back to in_progress and spawns a new autonomous work session
// pre-loaded with the CI/review failure context so the agent can fix and push.
func (s *BacklogService) AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if session.BacklogStatus(item.Status) != session.BacklogStatusPRPending {
		return fmt.Errorf("item %s is not pr_pending (got %s)", itemID, item.Status)
	}

	// Reuse the same iteration cap as the review rework cycle.
	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}

	// Tombstone any work session confirmed dead (crashed/killed without reaching its
	// completion path), then check for one still genuinely active. Skip entirely — no
	// status transition at all — if a fix is already in flight: previously this
	// transitioned pr_pending->in_progress unconditionally every tick, discovered the
	// spawn was blocked by an active session, and rolled back to pr_pending, churning
	// two BacklogStatusEvent rows every ~60s indefinitely even while a legitimate
	// multi-hour autonomous session was still working on the fix. Found live: an item's
	// activity history showed continuous pr_pending<->in_progress cycling with no
	// progress while its 4-hour-old autonomous work session was, in fact, still active
	// (see docs/tasks/backlog-feature-improvement.md).
	s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
	if active := findActiveWorkSession(sessions); active != nil {
		s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
		return nil
	}
	s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)

	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	if reworkCap := s.effectiveReworkCap(item); workCount >= reworkCap {
		log.InfoLog.Printf("[AutoReopenForPRFix] item %s has %d work sessions (cap %d); leaving in pr_pending for manual action", itemID, workCount, reworkCap)
		s.notifyReworkCapHit(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "while fixing PR #"+fmt.Sprint(item.PrNumber), reworkCap)
		return nil
	}

	updatedAt := item.UpdatedAt
	precondition := &session.BacklogItemPrecondition{
		ExpectedStatus:    string(session.BacklogStatusPRPending),
		ExpectedUpdatedAt: &updatedAt,
		Note:              "auto-reopened for PR fix (CI/review)",
	}
	if _, err := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusInProgress, precondition, session.TriggeredBySystem); err != nil {
		return fmt.Errorf("transition to in_progress: %w", err)
	}

	// The item just left pr_pending for in_progress — resolve any open
	// rework_cap, pr_ready_unmerged, or push_failed rows immediately (Task
	// 2.1.5b) rather than waiting for the self-heal sweep's next tick.
	for _, reason := range []domain.StuckReason{domain.StuckReasonReworkCap, domain.StuckReasonPRReadyUnmerged, domain.StuckReasonPushFailed} {
		if _, resolveErr := s.storage.ResolveStuck(ctx, itemID, reason); resolveErr != nil {
			log.WarningLog.Printf("[AutoReopenForPRFix] ResolveStuck(%s) item=%s: %v", reason, itemID, resolveErr)
		}
	}

	// Best-effort: sync the currently open PR's branch with main before handing the
	// fix off to a new session. This is preventive rather than reactive — a CI
	// failure caused by drift from main (rather than the PR's own diff) gets
	// resolved here directly by pushing the merge, and a conflict discovered now
	// becomes part of the fix context instead of being silently left for a later,
	// harder-to-diagnose collision (the PR #157 pattern: a branch drifted from main
	// with nobody proactively resyncing it until it hit a hard conflict). Never
	// blocks the spawn — any failure here is logged and swallowed.
	if syncNote := s.syncPRBranchWithMain(ctx, itemID, sessions); syncNote != "" {
		fixContext = syncNote + "\n\n" + fixContext
	}

	// Prepend the PR failure context to the item's notes so the spawned session
	// prompt includes it. Restore original notes after spawning.
	originalNotes := item.Notes
	prFixNote := fmt.Sprintf("[PR Fix context - PR #%d (%s)]\n%s", item.PrNumber, item.PrURL, fixContext)
	combinedNotes := prFixNote
	if originalNotes != "" {
		combinedNotes = prFixNote + "\n\n---\n\n" + originalNotes
	}
	if _, noteErr := s.storage.UpdateBacklogItem(ctx, itemID, session.BacklogItemUpdate{
		Notes: &combinedNotes,
	}, nil); noteErr != nil {
		log.WarningLog.Printf("[AutoReopenForPRFix] set fix notes item=%s: %v", itemID, noteErr)
	}

	_, spawnErr := s.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))

	// Restore original notes regardless of spawn outcome.
	if _, noteErr := s.storage.UpdateBacklogItem(ctx, itemID, session.BacklogItemUpdate{
		Notes: &originalNotes,
	}, nil); noteErr != nil {
		log.WarningLog.Printf("[AutoReopenForPRFix] restore notes item=%s: %v", itemID, noteErr)
	}

	if spawnErr != nil {
		// Roll back to pr_pending so the reconciler can retry.
		if _, rollbackErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusPRPending, nil, session.TriggeredBySystem); rollbackErr != nil {
			log.ErrorLog.Printf("[AutoReopenForPRFix] rollback to pr_pending failed for item %s: %v", itemID, rollbackErr)
			// The caller below only ever learns about spawnErr — a rollback
			// failure here leaves the item stranded at in_progress with no work
			// session, same shape notifySpawnAndRollbackFailed was built for
			// (BUG-030).
			s.notifySpawnAndRollbackFailed(ctx, itemID, item.Title, spawnErr, rollbackErr)
		}
		return fmt.Errorf("spawn session: %w", spawnErr)
	}

	log.InfoLog.Printf("[AutoReopenForPRFix] item %s → in_progress for PR fix session", itemID)
	return nil
}

// AutoRespawnReview implements session.ReviewRespawner. It re-triggers the review gate
// for a backlog item abandoned in review with no active session — closing the gap where
// StuckReasonAbandonedReview was previously only detected and notified, never acted on,
// which let real backlog items sit stuck for days (docs/tasks/backlog-feature-improvement.md).
//
// Unlike AutoReopenAfterFailedReview/AutoReopenForPRFix, this does NOT transition the
// item's status: the item is already "review" (TriggerReReview requires exactly that
// status) and the underlying work may well already be complete — the whole point of
// re-review is to find out, not to force another work session. See TriggerReReview for
// why this is likely the right respawn mechanism over spawning a fresh work session: a
// live audit found several abandoned-review items with nearly all acceptance criteria
// already marked complete, just never actually reviewed.
//
// Deliberately NOT gated by maxConcurrentBacklogWorkItems: that cap bounds concurrent
// "in_progress" items, and this path never transitions the item out of "review" (a
// manual TriggerReReview call doesn't check that cap either — this preserves existing
// behavior rather than introducing a new restriction). Concurrency is instead bounded by
// the caller (markAbandonedReview), which dispatches under l.reviewSem — the same
// limiter ReconcileStuck's sibling review-gate-respawn path already uses.
func (s *BacklogService) AutoRespawnReview(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if session.BacklogStatus(item.Status) != session.BacklogStatusReview {
		// Already moved on by the time this async call runs (e.g. a review gate
		// was re-spawned by ReconcileStuck's FindReviewItemsWithoutGate path, or a
		// human acted manually) — nothing to do.
		return nil
	}

	sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
	if sessErr != nil {
		return fmt.Errorf("list sessions for cap check: %w", sessErr)
	}

	// Re-check liveness immediately before acting: the caller (markAbandonedReview)
	// dispatches this asynchronously under a semaphore, so time may have passed
	// since the detector query that found the item abandoned. Tombstone any work
	// session confirmed dead first, mirroring AutoReopenForPRFix's identical guard.
	s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
	active := findActiveWorkSession(sessions)
	if active == nil {
		active = findActiveReviewSession(sessions)
	}
	if active != nil {
		s.notifyRespawnBlockedByActiveSession(ctx, "AutoRespawnReview", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
		return nil
	}
	s.resolveRespawnBlockedActiveLogged(ctx, "AutoRespawnReview", itemID)

	// Cap on *review* sessions, not work sessions: this path never spawns a work
	// session, so the work-session counters AutoReopenAfterFailedReview/
	// AutoReopenForPRFix use would never trip here. Without a cap of its own, an
	// item whose underlying work is genuinely incomplete (verdict never PASSes)
	// would re-review forever, once per abandoned_review occurrence. Reuses the
	// same threshold and notifyReworkCapHit pattern as the other two rework loops
	// for consistency rather than inventing a new constant.
	reviewCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleReview {
			reviewCount++
		}
	}
	if reworkCap := s.effectiveReworkCap(item); reviewCount >= reworkCap {
		log.InfoLog.Printf("[AutoRespawnReview] item %s has %d review sessions (cap %d); leaving in review for manual action", itemID, reviewCount, reworkCap)
		s.notifyReworkCapHit(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "while abandoned in review with no active session", reworkCap)
		return nil
	}

	if _, reviewErr := s.TriggerReReview(ctx, connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID})); reviewErr != nil {
		return fmt.Errorf("trigger re-review: %w", reviewErr)
	}
	log.InfoLog.Printf("[AutoRespawnReview] item %s re-review triggered", itemID)
	return nil
}

// AutoRespawnTriage implements session.TriageRespawner. It re-triggers triage for an
// idea-status item whose most recent triage session orphaned — closing the gap where
// StuckReasonOrphanedTriage was previously only detected and notified, never acted on
// (its own doc comment in session/backlog_lifecycle.go used to read "no resolve pass
// needed here... once the item leaves 'idea'", which was true for resolution but left
// nothing driving the item TOWARD leaving idea in the first place). Confirmed live
// 2026-07-27 (docs/tasks/backlog-feature-improvement.md): items 4f03de7b and 505fb733
// sat stuck in "idea" for 2 days, only recovering once a human noticed the one-time
// notification and manually re-triggered triage.
//
// Delegates entirely to TriggerTriage, which already tombstones any still-open triage
// session and handles the ready->idea/idea status guard itself — reconcileOrphanedTriageItems
// (the caller's caller, via the backoff gate) already ended the orphaned session before
// marking the item stuck, so by the time this runs there is normally nothing left for
// TriggerTriage's own tombstone step to do; it is still safe to call unconditionally.
//
// Generalized 2026-08-03 (docs/tasks/backlog-feature-improvement.md, item be676dab) to
// also handle a queued item: TriggerTriage only ever accepts idea/ready, so a queued item
// gated on plan approval with no usable triage result first needs the same reset-to-idea
// step the manual "Return to Triage" escape hatch performs (ActionsSection.tsx's
// send_back_idea action, session/domain's queued->idea "backward: re-triage from scratch"
// transition) before triage can run again — this mirrors the manual recovery already
// performed for be676dab exactly, just automated.
func (s *BacklogService) AutoRespawnTriage(ctx context.Context, itemID string) error {
	if s.storage == nil {
		return fmt.Errorf("storage not available")
	}

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}

	switch session.BacklogStatus(item.Status) {
	case session.BacklogStatusIdea:
		// Already in the state TriggerTriage requires — fall through below.
	case session.BacklogStatusQueued:
		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusQueued)}
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusIdea, precondition, session.TriggeredBySystem); transErr != nil {
			return fmt.Errorf("reset queued item to idea before retriage: %w", transErr)
		}
	default:
		// Already moved on by the time this async call runs (e.g. a human already
		// re-triggered triage manually, or the item was otherwise resolved) —
		// nothing to do. Mirrors AutoRespawnReview's identical staleness guard.
		return nil
	}

	if _, triageErr := s.TriggerTriage(ctx, connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID})); triageErr != nil {
		return fmt.Errorf("trigger triage: %w", triageErr)
	}
	log.InfoLog.Printf("[AutoRespawnTriage] item %s triage re-triggered", itemID)
	return nil
}

// IsTriageLive reports whether this process itself still has a headless triage call
// genuinely in flight for itemID, per the triageInFlight field's doc comment. This is
// the single source of truth both tombstoneOrphanTriageSessions (in this file) and
// session.BacklogLifecycleListener's periodic staleness sweep (reconcileOrphanedTriageItems,
// via the TriageRespawner interface this method satisfies) consult — see BUG-055: before
// this method existed, that sweep had its own separate, liveness-blind staleness-only gate
// that could tombstone a call genuinely still running past maxHeadlessTriageSessionStaleness.
func (s *BacklogService) IsTriageLive(itemID string) bool {
	_, live := s.triageInFlight.Load(itemID)
	return live
}

// syncPRBranchWithMain merges prFixMainBranch into the worktree of item's most recent
// work session — the branch behind the currently open, failing PR — and pushes the
// merge when it brings in new commits, so the live PR is resynced with main before the
// fix session starts. It is best-effort: any failure (no worktree found, fetch/merge
// error, push error) is logged and swallowed, never blocking the fix spawn. Returns a
// note describing what happened for AutoReopenForPRFix to prepend to the fix context,
// or "" when there's nothing worth telling the spawned session (no worktree to sync,
// or the branch was already up to date with main).
func (s *BacklogService) syncPRBranchWithMain(ctx context.Context, itemID string, sessions []session.ItemSessionSummary) string {
	_, workSession := findMostRecentSessions(sessions)
	if workSession == nil {
		return ""
	}
	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		log.InfoLog.Printf("[AutoReopenForPRFix] syncPRBranchWithMain item=%s: no worktree to sync (%v)", itemID, wtErr)
		return ""
	}

	result, mergeErr := git.MergeMainIntoWorktree(wt.WorktreePath, prFixMainBranch)
	if mergeErr != nil {
		log.WarningLog.Printf("[AutoReopenForPRFix] merge %s into item=%s branch=%s: %v", prFixMainBranch, itemID, wt.BranchName, mergeErr)
		return ""
	}

	switch {
	case result.Conflicted:
		log.InfoLog.Printf("[AutoReopenForPRFix] item=%s: merging %s into %s produced conflicts in %v", itemID, prFixMainBranch, wt.BranchName, result.ConflictedFiles)
		return fmt.Sprintf("[Branch sync] Merging %q into this PR's branch (%s) produced conflicts in:\n- %s\n\nThe merge was aborted so the worktree is clean; resolving these conflicts against %s is part of this fix.",
			prFixMainBranch, wt.BranchName, strings.Join(result.ConflictedFiles, "\n- "), prFixMainBranch)
	case result.Merged:
		g := git.NewGitWorktreeFromStorage(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)
		if pushErr := g.PushBranch(); pushErr != nil {
			log.WarningLog.Printf("[AutoReopenForPRFix] push merged %s into item=%s branch=%s: %v", prFixMainBranch, itemID, wt.BranchName, pushErr)
			// The fix session that reads this note gets its own fresh worktree
			// (SpawnSessionFromItem always creates a new one on reopen), not this
			// one — so the note must be actionable from anywhere, not just "push
			// it": name the branch and give the exact command against the shared
			// repo checkout, whose .git the now-deleted worktree's branch ref
			// still lives in (worktree cleanup never deletes branches).
			return fmt.Sprintf("[Branch sync] Merged the latest %q into this PR's branch (%s) locally, but could not push it to origin (%v). "+
				"The merge commit is not lost — push it from the shared repo checkout before continuing: `git -C %s push origin %s`.",
				prFixMainBranch, wt.BranchName, pushErr, wt.RepoPath, wt.BranchName)
		}
		log.InfoLog.Printf("[AutoReopenForPRFix] item=%s: merged and pushed %s into %s", itemID, prFixMainBranch, wt.BranchName)
		return fmt.Sprintf("[Branch sync] Merged the latest %q into this PR's branch (%s) and pushed it — the branch is now up to date with %s.", prFixMainBranch, wt.BranchName, prFixMainBranch)
	default: // UpToDate
		return ""
	}
}

// classifyHeadlessCallError buckets a headless.CallBlocking error into a coarse category
// for log-line grepping, so a future incident (e.g. a repeat of the 2026-07-24 stuck-triage
// investigation) can answer "how often does each failure mode happen" from log history alone,
// without re-deriving it by hand from raw error text and process timing.
//
//   - "timeout": ctx deadline exceeded, or elapsed is within 5s of budget (covers a
//     hang whose error got wrapped/lost before reaching context.DeadlineExceeded).
//   - "shutdown": server shutdown context cancelled mid-call, not a call failure.
//   - "process_error": the claude subprocess ran and exited non-zero (bad prompt, LLM
//     refusal, usage error) — see headless.ErrLLMError/ErrUsageError/ErrInterrupted.
//   - "claude_not_found": the claude binary itself is missing from PATH — an environment
//     problem, not a per-call one.
//   - "other": anything else (e.g. a storage/DB error surfaced through the same path).
//
// budget is the caller's own call timeout (e.g. triageCallBudget for TriggerTriage,
// callTimeout for TriggerReReview) — the same classifier is shared across both headless
// call sites, so it takes budget as a parameter rather than hardcoding one.
func classifyHeadlessCallError(err error, elapsed, budget time.Duration) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), budget-elapsed < 5*time.Second:
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "shutdown"
	case errors.Is(err, headless.ErrClaudeNotFound):
		return "claude_not_found"
	case errors.Is(err, headless.ErrLLMError), errors.Is(err, headless.ErrUsageError), errors.Is(err, headless.ErrInterrupted):
		return "process_error"
	default:
		return "other"
	}
}

// captureHeadlessFailure best-effort writes raw (the accumulated stdout of a
// headless triage/review call that errored or failed to parse) to a durable
// file under s.cfg.HeadlessFailureCaptureDirOrDefault() and returns its
// absolute path, or "" if there was nothing to capture or the write failed.
// Deliberately swallow-and-log rather than propagate: a failure to persist
// diagnostic data about a failure must never mask or block handling of the
// original failure itself. See session.WriteHeadlessFailureCapture's doc
// comment for why this exists (log rotation + the ~200-byte preview
// previously logged on parse failure are not enough to diagnose a failed
// call after the fact).
func (s *BacklogService) captureHeadlessFailure(sessionUUID, raw string) string {
	if raw == "" {
		return ""
	}
	dir, dirErr := s.cfg.HeadlessFailureCaptureDirOrDefault()
	if dirErr != nil {
		log.WarningLog.Printf("[captureHeadlessFailure] resolve capture dir: %v", dirErr)
		return ""
	}
	path, writeErr := session.WriteHeadlessFailureCapture(dir, sessionUUID, raw, session.DefaultHeadlessFailureCaptureMaxBytes)
	if writeErr != nil {
		log.WarningLog.Printf("[captureHeadlessFailure] write capture file session=%s: %v", sessionUUID, writeErr)
		return ""
	}
	return path
}

// MaybeTriggerTriage is the single "should this newly created item get
// auto-triaged" decision, shared by every backlog-item creation entry point:
// RPC CreateBacklogItem and ImportGitHubIssue (backlog_service_lifecycle.go,
// backlog_service_sync.go), and the create_backlog_item/import_github_issue
// MCP tools (server/mcp/tools_backlog.go). Before this helper existed, the
// MCP tools called storage.CreateBacklogItem directly and skipped this gate
// entirely — every backlog item self-filed by an agent session via those
// tools sat in "idea" with zero triage attempts until (at best) a human
// noticed and manually re-triggered triage, since reconcileOrphanedTriageItems
// (session/backlog_lifecycle.go) only ever detects items that already have a
// prior triage-role ItemSession and cannot originate the first attempt.
//
// Mirrors the RPC handlers' existing inline gate exactly: skip if the caller
// asked to, if the item has no repo_path (nothing to run triage against), or
// if no headless pool is wired (e.g. claude binary unavailable). Best-effort
// — a failure to trigger is logged and never fails item creation. Returns
// whether triage was actually triggered, for callers that surface it back to
// the client (e.g. CreateBacklogItemResponse.TriageTriggered).
func (s *BacklogService) MaybeTriggerTriage(ctx context.Context, itemID string, skipTriage bool, repoPath string) bool {
	if skipTriage || repoPath == "" || s.headlessPool == nil {
		return false
	}
	// 30s gates only the synchronous path (item lookup + ItemSession creation).
	// The headless LLM call itself runs in a goroutine under shutdownCtx (30-min cap).
	triageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := s.TriggerTriage(triageCtx, connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
	if err != nil {
		log.WarningLog.Printf("[MaybeTriggerTriage] auto-triage failed for item %s: %v", itemID, err)
		return false
	}
	return true
}

// TriggerTriage kicks off a headless triage planning call for a backlog item.
// Returns immediately after creating an ItemSession; actual triage runs in a goroutine.
// +api: backlog:trigger-triage
func (s *BacklogService) TriggerTriage(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerTriageRequest],
) (*connect.Response[sessionv1.TriggerTriageResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Status guard — triage is only valid for idea or ready items.
	if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to trigger triage, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering triage"))
	}

	// 3z. repo_path must be an absolute, existing directory. Without this check, a
	// bare slug (e.g. "stapler-squad" instead of
	// "/home/tstapler/Programming/stapler-squad") reaches the headless LLM
	// subprocess's WorkDir unchanged (see the goroutine's CallOptions.WorkDir below),
	// and os/exec has a well-documented quirk: when Cmd.Dir doesn't exist, the
	// fork/exec error names the EXECUTABLE path, not the directory — e.g.
	// "fork/exec /home/tstapler/.local/bin/claude: no such file or directory" — which
	// looks exactly like the claude binary is missing even though the binary is fine
	// and the real problem is the bogus working directory (BUG-062). Validating here,
	// synchronously and before any ItemSession/artifact-dir creation, means every
	// caller (this RPC, MaybeTriggerTriage, and any future creation path that reuses
	// it) gets an immediate, correctly-attributed rejection instead of a doomed
	// goroutine that fails 0-1s later with a misleading error.
	if !filepath.IsAbs(item.RepoPath) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("repo_path %q is not an absolute path", item.RepoPath))
	}
	if fi, statErr := os.Stat(item.RepoPath); statErr != nil || !fi.IsDir() {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("repo_path %q does not exist or is not a directory", item.RepoPath))
	}

	// 3a. Orphan-aware guard: if an open triage session exists, check whether it is
	// genuinely still running — via s.triageInFlight for a headless call (this
	// process's own in-memory liveness record, see that field's doc comment) or via
	// sessionStopper for a live tmux session — and tombstone it only if it is not.
	existingSessions, listErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if listErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list triage sessions: %w", listErr))
	}
	if err := s.tombstoneOrphanTriageSessions(ctx, req.Msg.ItemId, item.Status, existingSessions); err != nil {
		return nil, err
	}

	// 3a-i. Atomic check-and-set, closing the TOCTOU window between the check above
	// (which only sees already-persisted session rows) and this item's new
	// ItemSession row being created below: two concurrent TriggerTriage calls for the
	// same item (a manual "Retry now" racing the periodic reconciliation sweep, say)
	// could otherwise both pass the check above before either has written its row.
	// Mirrors spawnInFlight's identical guard on SpawnSessionFromItem above. Cleared
	// via triageStarted below if this call returns before actually launching the
	// goroutine, or via the goroutine's own defer once it does launch.
	if _, alreadyInFlight := s.triageInFlight.LoadOrStore(req.Msg.ItemId, struct{}{}); alreadyInFlight {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("triage session already running for item %s", req.Msg.ItemId))
	}
	triageStarted := false
	defer func() {
		if !triageStarted {
			s.triageInFlight.Delete(req.Msg.ItemId)
		}
	}()

	// 3b. If re-triggering on a "ready" item, move it back to "idea".
	// Use a precondition so a concurrent work-session spawn (ready→in_progress) that
	// races with this re-triage doesn't drag the item backwards to idea.
	if item.Status == string(session.BacklogStatusReady) {
		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReady)}
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId,
			session.BacklogStatusIdea, precondition, session.TriggeredByUser); transErr != nil {
			log.WarningLog.Printf("[TriggerTriage] item %s moved past ready before triage reset (race with work-session spawn); aborting re-triage", req.Msg.ItemId)
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("item %s was already moved past ready — a work session may have just started; retry after it completes", req.Msg.ItemId))
		}
	}

	// 3c. Feedback-driven refine: find the most recent completed triage result to
	// revise. Refining requires one to exist — feedback on an item with no completed
	// triage falls back to a confusing fresh run, so reject explicitly instead.
	feedback := strings.TrimSpace(req.Msg.Feedback)
	priorResult, havePrior := findPriorTriageResult(existingSessions)
	if feedback != "" && !havePrior {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no completed triage result to refine for item %s — trigger initial triage first", req.Msg.ItemId))
	}
	nextIteration := priorResult.Iteration + 1

	// 4. Build artifact dir path under ~/.stapler-squad/triage-artifacts/<item-id>/
	//    so triage workers don't write into the item's git repo.
	triageBase, triageBaseErr := s.cfg.TriageArtifactDirOrDefault()
	if triageBaseErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to resolve triage artifact dir: %w", triageBaseErr))
	}
	artifactAbsPath := filepath.Join(triageBase, item.ID)

	// 5. Create artifact dir.
	if mkErr := os.MkdirAll(artifactAbsPath, 0o755); mkErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create artifact dir %s: %w", artifactAbsPath, mkErr))
	}

	// 6. Require headless pool.
	if s.headlessPool == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("headless pool not available — ensure claude binary is installed"))
	}

	// 7. Build triage prompt — a fresh triage, or a feedback-driven refine of the
	// most recent completed result. The retriage (feedback != "") branch deliberately
	// stays on BuildHeadlessRetriagePrompt directly and is NOT routed through
	// PipelineEngine — "refine the existing plan" is mode-independent
	// (research/architecture.md §3). Only the first-triage branch is routed through
	// the engine (Epic 1.5, Story 1.5.3).
	var triagePrompt string
	if feedback != "" {
		if req.Msg.ChatMode {
			triagePrompt = session.BuildHeadlessChatRetriagePrompt(item, artifactAbsPath, priorResult, feedback)
		} else {
			triagePrompt = session.BuildHeadlessRetriagePrompt(item, artifactAbsPath, priorResult, feedback)
		}
	} else {
		triagePrompt = s.triagePromptFor(item, artifactAbsPath)
	}

	log.InfoLog.Printf("[PipelineEngine] item=%s stage=triage mode=%q", item.ID, session.ResolvedModeLabel(item.PipelineMode))

	// 8. Create ItemSession synchronously before goroutine (prevents TOCTOU on orphan guard).
	// Snapshot the resolved PipelineMode slug + content hash — see the comment on the
	// equivalent SpawnSessionFromItem call site above for the nil-guard rationale.
	triageSessionUUID := headlessTriageUUIDPrefix + uuid.New().String()
	var triagePipelineModeSnapshotHash string
	if s.pipelineEngine != nil {
		triagePipelineModeSnapshotHash, _ = s.pipelineEngine.ContentHashFor(session.PipelineMode(item.PipelineMode))
	}
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:                   item.ID,
		SessionUUID:              triageSessionUUID,
		SessionRole:              session.SessionRoleTriage,
		AcSnapshot:               item.AcceptanceCriteria,
		PipelineModeSnapshot:     item.PipelineMode,
		PipelineModeSnapshotHash: triagePipelineModeSnapshotHash,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create triage item session: %w", err))
	}

	log.InfoLog.Printf("[TriggerTriage] headless triage started item=%s session=%s path=%s", item.ID, triageSessionUUID, artifactAbsPath)

	// 9. Drive triage asynchronously so the RPC returns immediately.
	itemID := item.ID
	itemRepoPath := item.RepoPath
	isID := is.ID
	iteration := nextIteration
	triageStarted = true
	go func() {
		// Clears the triageInFlight entry set at 3a-i above no matter how this
		// goroutine exits, so the item is never left permanently un-retriggerable.
		defer s.triageInFlight.Delete(itemID)

		// Acquire concurrency semaphore (max 8 concurrent triage calls).
		select {
		case s.triageSem <- struct{}{}:
		case <-s.shutdownCtx.Done():
			// cleanupCtx is a separate context for DB writes that must complete even
			// after shutdownCtx is cancelled. Passing shutdownCtx here would cause the
			// write to fail immediately with context.Canceled.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			// EndReason="shutdown" (not the plain UpdateItemSessionEnded) — BUG-065.
			// This item never even reached the semaphore (8 concurrent slots, see
			// triageSem above); it was still queued when our own graceful shutdown
			// fired. reconcileOrphanedTriageItems' shutdown carve-out (session/backlog_lifecycle.go)
			// only recognizes EndReason=="shutdown" to respawn for free with no
			// stuck-notification; leaving this blank made every queued-but-never-started
			// item indistinguishable from a genuine unclassified triage failure.
			_ = s.storage.UpdateItemSessionEndedWithReason(cleanupCtx, isID, time.Now(), "shutdown")
			return
		}
		defer func() { <-s.triageSem }()

		triageCtx, cancel := context.WithTimeout(s.shutdownCtx, triageCallBudget)
		defer cancel()

		// Run triage in a dedicated worktree, not itemRepoPath directly. SDD-mode
		// triage prompts (session/pipeline_mode_seed.go's sddTriagePromptTemplate)
		// deliberately write project_plans/<name>/ relative to CWD rather than to
		// artifactAbsPath above — by design, so those docs land in the target repo
		// and travel with the eventual PR (see that file's design-rationale
		// comment). But itemRepoPath is routinely a shared or actively-used
		// checkout (an app-managed mirror other sessions touch, or — for items
		// created interactively with repo_path defaulted to the calling session's
		// own cwd — a developer's live working directory), so writing uncommitted
		// planning docs directly into it pollutes whatever else is happening
		// there. A worktree gives the research phase the same real repo content
		// (git worktrees share history/objects with the main checkout) while
		// isolating the writes; falls back to itemRepoPath directly if worktree
		// creation fails (e.g. itemRepoPath isn't a git repo at all — some items
		// legitimately target a plain directory).
		triageWorkDir := itemRepoPath
		var triageWorktree *git.GitWorktree
		if wt, _, wtErr := git.NewGitWorktree(itemRepoPath, "triage-"+itemID); wtErr != nil {
			log.WarningLog.Printf("[TriggerTriage] failed to create isolated worktree for item=%s, running triage directly in repo_path: %v", itemID, wtErr)
		} else if setupErr := wt.Setup(); setupErr != nil {
			log.WarningLog.Printf("[TriggerTriage] failed to set up isolated worktree for item=%s, running triage directly in repo_path: %v", itemID, setupErr)
		} else {
			triageWorktree = wt
			triageWorkDir = wt.GetWorktreePath()
		}

		callStart := time.Now()
		raw, _, callErr := s.headlessPool.CallBlocking(triageCtx,
			headless.FeatureKeyTriage,
			headless.HeadlessTriageSystemPrompt(),
			triagePrompt,
			// WorkDir-only, no PermissionMode — matches the empirically-verified
			// precedent from ADR-001 (project_plans/backlog-already-implemented),
			// re-confirmed live against the real CLI: a WorkDir-bearing claude -p
			// call with no --permission-mode flag grants real Write/Bash access via
			// Claude Code's own auto-mode default (defaultMode: "auto" in
			// ~/.claude/settings.json) with zero permission_denials and no hang. An
			// earlier version of this comment claimed a missing PermissionMode
			// causes a silent hang; that was a misdiagnosis — the 2026-07-24 stuck
			// sessions correlate with a concurrent memory-exhaustion/zombie-subprocess
			// incident (swap 100% full, orphaned claude -p processes from 4-18h
			// earlier), not a permission-mode gap. Do not add bypassPermissions here
			// without a fresh empirical repro, per ADR-001's own "don't trust
			// unverified CLI-behavior assumptions" precedent.
			headless.CallOptions{WorkDir: triageWorkDir},
		)

		// cleanupCtx outlives shutdownCtx so DB writes succeed even during graceful
		// shutdown. Created HERE, after CallBlocking returns, not before
		// it: the LLM call above routinely takes 7-15 minutes (4 parallel research
		// subagents), so a cleanupCtx created before it would have its 10s budget
		// already expired by the time these persistence calls run below — every
		// successful triage would silently fail to ever mark the item ready. This
		// was a live, 100%-reproducible bug: see the backlog cross-platform audit.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), s.triageCleanupTimeout)
		defer cleanupCancel()

		callElapsed := time.Since(callStart)
		if callErr != nil {
			// elapsed=<duration> lets a future incident distinguish "died fast"
			// (config/parse/process error) from "ran the full 30m budget" (a real
			// hang or an upstream call that never returns) at a glance in the log,
			// without needing to cross-reference session start/end timestamps by
			// hand — exactly the reconstruction this session had to do manually for
			// the 2026-07-24 stuck-triage incident. errType classifies the error
			// into a few high-signal buckets so a grep over historical logs can
			// answer "how often do we hit each failure mode" without parsing %v text.
			errType := classifyHeadlessCallError(callErr, callElapsed, triageCallBudget)
			capturePath := s.captureHeadlessFailure(triageSessionUUID, raw)
			log.ErrorLog.Printf("[TriggerTriage] headless triage failed item=%s elapsed=%s errType=%s capture=%s: %v",
				itemID, callElapsed.Round(time.Second), errType, capturePath, callErr)
			_ = s.storage.UpdateItemSessionEndedWithReason(cleanupCtx, isID, time.Now(), errType)
			if capturePath != "" {
				_ = s.storage.UpdateItemSessionFailureCapture(cleanupCtx, isID, capturePath)
			}
			cleanupProvisionalTriageWorktree(itemID, triageWorktree)
			return
		}

		result, parseErr := session.ParseHeadlessTriageResult(raw)
		if parseErr != nil {
			capturePath := s.captureHeadlessFailure(triageSessionUUID, raw)
			log.ErrorLog.Printf("[TriggerTriage] parse result failed item=%s elapsed=%s rawLen=%d capture=%s: %v",
				itemID, callElapsed.Round(time.Second), len(raw), capturePath, parseErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			if capturePath != "" {
				_ = s.storage.UpdateItemSessionFailureCapture(cleanupCtx, isID, capturePath)
			}
			cleanupProvisionalTriageWorktree(itemID, triageWorktree)
			return
		}
		result.Iteration = iteration
		result.Feedback = feedback

		// Commit whatever the triage prompt wrote (project_plans/<name>/ for SDD
		// mode; nothing for default mode, which writes to artifactAbsPath instead
		// — CommitChanges no-ops when the worktree isn't dirty) so the docs
		// survive past this goroutine instead of sitting uncommitted indefinitely
		// (the exact gap .claude/rules/sdd-planning-artifacts-commit.md already
		// names). Only when triageWorktree is non-nil — the itemRepoPath fallback
		// path must never auto-commit into a repo this code didn't create.
		if triageWorktree != nil {
			if commitErr := triageWorktree.CommitChanges(fmt.Sprintf("chore(sdd): planning artifacts for %s", result.Title)); commitErr != nil {
				log.WarningLog.Printf("[TriggerTriage] failed to commit triage artifacts item=%s worktree=%s: %v", itemID, triageWorkDir, commitErr)
			}
			retitleTriageWorktreeToFinalBranch(itemID, itemRepoPath, result.Title, triageWorktree)
		}

		payloadJSON, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] marshal triage result item=%s: %v", itemID, marshalErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		// persistFailures accumulates which of the post-triage persistence steps below
		// failed. Each step already logs its own error to the log file (operator-invisible
		// in real time); if any step fails, notifyTriagePersistFailure below additionally
		// surfaces a single operator-facing notification so a failure here is never silent.
		var persistFailures []string

		if updateErr := s.storage.UpdateItemSessionTriageResult(cleanupCtx, isID, string(payloadJSON)); updateErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] persist triage result item=%s: %v", itemID, updateErr)
			persistFailures = append(persistFailures, "saving the triage result")
		}

		// SDD mode writes plan.md under project_plans/<name>/implementation/, not
		// flat under artifactAbsPath like the default pipeline's prompt does —
		// readPlanFile (session/backlog_review.go) only ever looks for
		// <PlanArtifactsPath>/plan.md, so this must point at the implementation/
		// subdirectory in triageWorkDir, or review/context-building silently
		// finds no plan content for every SDD-mode item (true even before this
		// change, since artifactAbsPath never held SDD's output). Keyed off
		// triageWorkDir, not triageWorktree != nil: the fallback path (worktree
		// setup failed) still runs SDD triage directly in itemRepoPath — via
		// triageWorkDir == itemRepoPath — and still needs pap to find it there.
		pap := artifactAbsPath
		if item.PipelineMode == session.DefaultSDDPipelineModeSlug {
			pap = filepath.Join(triageWorkDir, "project_plans", result.Title, "implementation")
		}
		approvalReset := false
		clearedReason := ""
		update := session.BacklogItemUpdate{
			PlanArtifactsPath:   &pap,
			PlanApproved:        &approvalReset,
			PlanRejectionReason: &clearedReason,
			ClearPlanRejectedAt: true,
		}
		applyTriageResultToUpdate(&result, &update)
		if _, updateErr := s.storage.UpdateBacklogItem(cleanupCtx, itemID, update, nil); updateErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] update plan_artifacts_path item=%s: %v", itemID, updateErr)
			persistFailures = append(persistFailures, "saving the plan artifacts path")
		}

		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusIdea)}
		statusAdvanced := true
		if _, transErr := s.storage.TransitionBacklogItemStatus(cleanupCtx, itemID, //nolint:silenttransition surfaced a few lines below via notifyTriagePersistFailure once persistFailures is fully collected
			session.BacklogStatusReady, precondition, session.TriggeredBySystem); transErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] status transition idea→ready item=%s: %v", itemID, transErr)
			persistFailures = append(persistFailures, "advancing the item to Ready")
			statusAdvanced = false
		}

		if len(persistFailures) > 0 {
			s.notifyTriagePersistFailure(cleanupCtx, itemID, item.Title, persistFailures, statusAdvanced)
		}

		// Close out the ItemSession and release triageInFlight together, right here —
		// not after the optional auto-spawn below. The item's status already flipped to
		// Ready above, so a caller polling on status (or a human clicking "retry") can
		// legitimately re-trigger triage now; leaving triageInFlight held through
		// auto-spawn's own I/O (SpawnSessionFromItem creates a worktree, etc.) only
		// stretched a window where a well-timed retry got a spurious AlreadyExists —
		// exactly what made TestTriggerTriage_RefineWithFeedback flaky in CI. ended_at
		// and triageInFlight are updated in the same spot deliberately: both are inputs
		// to the orphan-liveness check (IsTriageLive / tombstoneOrphanTriageSessions
		// above), so moving one without the other would let a concurrent reconciliation
		// sweep see "ended_at nil, not live" and wrongly tombstone a session that's
		// simply between here and its final log line.
		_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
		s.triageInFlight.Delete(itemID)

		// Opt-in: skip the manual "Spawn Session" click when the item is configured to
		// auto-spawn. Autonomous: true bypasses the planning-approval gate the same way
		// AutoReopenForPRFix's spawn already does — a human never gets to review the plan
		// first, which is the whole point of this toggle (default false; existing manual
		// flow is unchanged unless explicitly opted in).
		if statusAdvanced && item.AutoSpawnSession {
			if _, spawnErr := s.SpawnSessionFromItem(cleanupCtx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
				ItemId:     itemID,
				Autonomous: true,
			})); spawnErr != nil {
				log.WarningLog.Printf("[TriggerTriage] auto-spawn session item=%s: %v", itemID, spawnErr)
			} else {
				log.InfoLog.Printf("[TriggerTriage] auto-spawned work session item=%s (auto_spawn_session=true)", itemID)
			}
		}

		log.InfoLog.Printf("[TriggerTriage] headless triage complete item=%s elapsed=%s suggestions=%d tasks=%d",
			itemID, callElapsed.Round(time.Second), len(result.Suggestions), len(result.Tasks))
	}()

	return connect.NewResponse(&sessionv1.TriggerTriageResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// CancelTriage stops a running triage session for a backlog item.
// +api: backlog:cancel-triage
func (s *BacklogService) CancelTriage(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelTriageRequest],
) (*connect.Response[sessionv1.CancelTriageResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	existingSessions, err := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
	}

	cancelled := false
	now := time.Now()
	for _, is := range existingSessions {
		if is.Role != string(session.SessionRoleTriage) || is.EndedAt != nil {
			continue
		}
		if s.sessionStopper != nil {
			_ = s.sessionStopper.StopSessionByUUID(ctx, is.SessionUUID)
		}
		_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, now)
		cancelled = true
	}

	return connect.NewResponse(&sessionv1.CancelTriageResponse{Cancelled: cancelled}), nil
}

// TriggerReReview re-runs the review gate for a backlog item.
// +api: backlog:trigger-re-review
func (s *BacklogService) TriggerReReview(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerReReviewRequest],
) (*connect.Response[sessionv1.TriggerReReviewResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Validate item is in review status.
	if item.Status != string(session.BacklogStatusReview) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to re-trigger review, got %q", session.BacklogStatusReview, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering re-review"))
	}

	// 4. Find the most recent review and work ItemSessions for this item.
	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	mostRecentReviewSession, mostRecentWorkSession := findMostRecentSessions(sessions)

	// 5. Note: We don't need to delete the old verdict; a new one will overwrite it when the re-review
	// session submits its findings via the MCP tool.

	// 5b. Deserialize AC snapshot (from most recent work session or item AC) — needed
	// ahead of step 6 so the branch-drift precondition below (5c) can record a blocked
	// verdict against it without a second lookup.
	acSnapshot := resolveACSnapshot(mostRecentWorkSession, item.AcceptanceCriteria)
	acSnapshotJSON, _ := json.Marshal(acSnapshot)

	// 5c. Precondition of review, not a best-effort side effect of the reactive PR-fix
	// path (BUG-044): this is the entry point AutoRespawnReview uses to re-review an
	// item abandoned in review — exactly the path that let backlog item 693c2700's
	// branch drift 289 commits behind main across repeated abandoned-review cycles
	// before ever being caught. Checked/synced here, before any diff is computed, so a
	// clean auto-sync never even reaches the reviewer, and a real conflict blocks with
	// an explicit, actionable reason instead of a misleading "no related work" verdict.
	if mostRecentWorkSession != nil {
		if wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, mostRecentWorkSession.SessionUUID); wtErr == nil && wt.WorktreePath != "" && wt.BranchName != "" {
			if ok, blockedSummary := git.EnsureBranchSyncedWithMain(wt.WorktreePath, wt.BranchName, prFixMainBranch, git.DefaultBranchDriftThreshold); !ok {
				log.WarningLog.Printf("[TriggerReReview] branch drift blocked review item=%s branch=%s: %s", item.ID, wt.BranchName, blockedSummary)
				is, createErr := session.RecordDegradedReviewVerdict(s.storage, item.ID, session.AcCriteriaJSON(acSnapshotJSON), headlessReReviewUUIDPrefix, blockedSummary)
				if createErr != nil {
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review branch-drift-blocked verdict: %w", createErr))
				}
				log.InfoLog.Printf("[TriggerReReview] branch drift blocked for item %s — verdict recorded (session %s)", item.ID, is.ID)
				if s.eventBus != nil {
					// itemID as sessionID — see comment in notifyReworkCapHit above.
					s.eventBus.Publish(events.NewNotificationEvent(
						item.ID, "", uuid.New().String(),
						int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
						int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
						"Review blocked — branch drifted too far behind main",
						fmt.Sprintf("%s — the branch could not be automatically synced with main. See the item's review history for the conflict details.", item.Title),
						map[string]string{"item_id": item.ID},
					))
				}
				return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
					ItemSession: itemSessionToProto(is, s.buildCostLookup()),
				}), nil
			}
		}
	}

	// 6. Get git diff from the most recent work session's worktree using its base SHA.
	// Fall back to item.RepoPath / HEAD~1 only for directory-mode sessions. Read AFTER
	// the drift precondition above (5c) so a just-synced branch's diff reflects the
	// merge rather than the stale pre-sync state.
	workSessionDiff := s.getWorkSessionDiff(ctx, item.RepoPath, mostRecentWorkSession)

	verificationNotes := ""
	if mostRecentWorkSession != nil {
		verificationNotes = mostRecentWorkSession.VerificationNotes
	}

	// 8. Build re-review prompt. acSnapshotJSON was already computed in step 5c above.
	priorVerdictSection := ""
	if mostRecentReviewSession != nil && mostRecentReviewSession.ReviewVerdict != nil {
		rv := mostRecentReviewSession.ReviewVerdict
		priorVerdictSection = fmt.Sprintf("\n## Prior Review Verdict\nOutcome: %s\nSummary: %s\n", rv.OverallOutcome, rv.Summary)
	}

	reReviewPrompt := fmt.Sprintf(`You are re-reviewing a backlog item that previously entered the review state.

# Item: %s

## Description
%s
%s
## Acceptance Criteria (at time of work session)
`, item.Title, item.Description, priorVerdictSection)

	for _, ac := range acSnapshot {
		reReviewPrompt += fmt.Sprintf("%d. %s (status: %s)\n", ac.Index, ac.Text, ac.Status)
	}

	reReviewPrompt += fmt.Sprintf(`
## Recent Changes
The work session made the following changes to the codebase:

%s

## Your Task
Perform a comprehensive review and submit your verdict using the submit_review_verdict MCP tool:
- Assess each acceptance criterion listed above
- Evaluate the diff against the requirements
- For each criterion provide: criterion_index, outcome (PASS/FAIL/PARTIAL), evidence

Call submit_review_verdict with:
  item_id: "%s"
  summary: "<overall summary of your findings>"
  verdicts: [{"criterion_index": N, "outcome": "PASS|FAIL|PARTIAL", "evidence": "<specific evidence>"}]

Do not modify the code. Only write the review verdict.
`, session.SanitizeDiff(workSessionDiff), item.ID)

	// 9. Headless path — preferred when a headless pool is configured.
	// This avoids needing tmux and runs the review inline via LLM call.
	if s.headlessPool != nil {
		codebaseWorkDir, codebaseWorkDirExists := s.resolveCodebaseWorkDir(ctx, item.RepoPath, mostRecentWorkSession)

		// codebaseWorkDir only matters on the empty-diff path — BuildReviewCallOptions
		// never grants directory access when a real diff exists. Block here, before ever
		// building a prompt or spending a headless call, when that directory doesn't
		// exist on disk: handing the reviewer Read/Grep/Glob access scoped to a
		// nonexistent directory produces zero real evidence, which it then (correctly,
		// given what it was shown) reports as "no diff exists" — a false FAIL that masks
		// real work sitting on the branch. See resolveCodebaseWorkDir's doc comment for
		// the confirmed live incident this guards against. Same failure class
		// ReviewGateRunner.Run (session/review_gate.go) blocks on an unrecoverable diff.
		if workSessionDiff == "" && !codebaseWorkDirExists {
			blockedSummary := fmt.Sprintf("Review blocked: no diff could be computed and the codebase-read fallback directory (%s) does not exist on disk. The recorded worktree may have been cleaned up without its DB row being updated — this needs investigation, not rework.", codebaseWorkDir)
			is, createErr := session.RecordDegradedReviewVerdict(s.storage, item.ID, session.AcCriteriaJSON(acSnapshotJSON), headlessReReviewUUIDPrefix, blockedSummary)
			if createErr != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review blocked verdict: %w", createErr))
			}
			log.ErrorLog.Printf("[TriggerReReview] codebase-read work dir %s does not exist for item %s — review blocked, UNVERIFIABLE verdict recorded (session %s)", codebaseWorkDir, item.ID, is.ID)
			if s.eventBus != nil {
				// itemID as sessionID — see comment in notifyReworkCapHit above.
				s.eventBus.Publish(events.NewNotificationEvent(
					item.ID, "", uuid.New().String(),
					int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
					int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
					"Review blocked — codebase directory missing",
					fmt.Sprintf("%s — no diff could be computed and the fallback review directory is gone. Needs investigation.", item.Title),
					map[string]string{"item_id": item.ID},
				))
			}
			return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
				ItemSession: itemSessionToProto(is, s.buildCostLookup()),
			}), nil
		}

		// Additional context (prior review attempts, full notes history, item goal/status
		// history, searchable session transcript) is only gathered on the empty-diff
		// codebase-read path — see session.ReviewContextExtras. Every fetch here is
		// best-effort/log-and-continue: none of it is required for the re-review to
		// proceed.
		// transcriptCleanup removes the review transcript file written into
		// codebaseWorkDir below (if any). Defaults to a no-op; both the explicit call
		// right after CallBlocking returns AND the deferred call are kept, mirroring
		// ReviewGateRunner.Run's identical pattern (session/review_gate.go) — see the
		// explicit call site below for the full rationale. Unlike ReviewGateRunner.Run,
		// TriggerReReview has no onPass-equivalent call after the review completes (it
		// persists the ItemSession+verdict and returns the RPC response directly; no
		// git commit/push happens in this function), so the ordering bug Fix B in the
		// review-gate path fixed does not currently reproduce here. The early call is
		// still added for defense-in-depth and consistency, so a future change that
		// adds a post-review action to this function does not silently reintroduce it.
		transcriptCleanup := func() {}
		defer func() { transcriptCleanup() }()

		var extras session.ReviewContextExtras
		if workSessionDiff == "" {
			// sessions was already loaded above (step 4) — reuse it rather than a second
			// ListItemSessions round trip.
			extras.PriorSessions = sessions
			if notes, notesErr := s.storage.ListProgressNotesForItem(ctx, item.ID); notesErr != nil {
				log.WarningLog.Printf("[TriggerReReview] ListProgressNotesForItem (context extras) item=%s: %v", item.ID, notesErr)
			} else {
				extras.ProgressNotes = notes
			}
			// item was loaded via storage.GetBacklogItem above, which always eagerly
			// loads StatusEvents — no extra fetch needed here.
			extras.ItemDescription = item.Description
			extras.StatusEvents = item.StatusEvents
			if sm := s.getScrollbackManager(); sm != nil && mostRecentWorkSession != nil {
				relPath, cleanup, transcriptErr := session.WriteReviewTranscriptFile(sm, mostRecentWorkSession.SessionUUID, codebaseWorkDir, session.DefaultReviewTranscriptMaxBytes)
				transcriptCleanup = cleanup
				if transcriptErr != nil {
					log.WarningLog.Printf("[TriggerReReview] WriteReviewTranscriptFile item=%s: %v", item.ID, transcriptErr)
				} else {
					extras.TranscriptRelPath = relPath
				}
			}
		}

		headlessPrompt := s.reviewPromptFor(item, acSnapshot, workSessionDiff, false, verificationNotes, extras)
		systemPrompt, callOpts, callTimeout, reviewPath := session.BuildReviewCallOptions(workSessionDiff, codebaseWorkDir)
		// callStart is recorded immediately before the headless call sequence
		// (capability self-check, then CallBlocking) so Epic 2.5's duration_ms=
		// observability logging reflects the real cost of this re-review attempt,
		// including a first-in-process capability self-check when one runs.
		callStart := time.Now()

		// Story 2.2.6c: before the FIRST real codebase-read call in this process's
		// lifetime, verify the claude CLI/config actually grants WorkDir+AllowedTools+
		// PermissionMode read access — shares headless.DefaultCapabilitySelfCheck (via
		// s.capabilityCheck) with ReviewGateRunner so a failure discovered via either
		// call site short-circuits the other. A failure here means every
		// AllowedTools/PermissionMode-bearing call would silently produce zero real
		// evidence, so skip the real call entirely and record UNVERIFIABLE directly —
		// mirrors the codebase-read-timeout branch's shape below.
		if reviewPath == "codebase-read" && !s.capabilityCheck.Ensure(ctx, s.headlessPool) {
			reviewPath = "codebase-read-degraded"
			capSummary := "Review UNVERIFIABLE: codebase-read capability self-check failed — this process's claude CLI/config does not appear to grant WorkDir+AllowedTools+PermissionMode read access, so no real codebase-read call was attempted."
			is, createErr := session.RecordDegradedReviewVerdict(s.storage, item.ID, session.AcCriteriaJSON(acSnapshotJSON), headlessReReviewUUIDPrefix, capSummary)
			if createErr != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review capability-self-check verdict: %w", createErr))
			}
			log.WarningLog.Printf("[TriggerReReview] headless re-review complete for item %s (outcome %s, path=%s, duration_ms=%d)", item.ID, session.ReviewVerdictUnverifiable, reviewPath, time.Since(callStart).Milliseconds())
			return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
				ItemSession: itemSessionToProto(is, s.buildCostLookup()),
			}), nil
		}

		reviewCtx, reviewCancel := context.WithTimeout(ctx, callTimeout)
		defer reviewCancel()

		reviewResult, callCostUSD, callErr := s.headlessPool.CallBlocking(
			reviewCtx, headless.FeatureKeyReview, systemPrompt, headlessPrompt, callOpts,
		)

		// Explicit, immediate cleanup as soon as the transcript file is no longer
		// needed — see the identical call in ReviewGateRunner.Run
		// (session/review_gate.go) for the full rationale. Kept here even though
		// TriggerReReview currently has no post-review commit/push action, for
		// consistency and so this function stays safe if one is ever added.
		transcriptCleanup()

		if callErr != nil {
			// Story 2.2.4c: a timeout OR a parent-context cancellation on the codebase-read
			// path is an infrastructure signal (hung/degraded tool access, or e.g. process
			// shutdown mid-call), not evidence the criteria failed — degrade to UNVERIFIABLE
			// instead of the normal error path below. ADR-001's rationale for timeouts
			// applies equally to cancellation.
			if reviewPath == "codebase-read" && (errors.Is(reviewCtx.Err(), context.DeadlineExceeded) || errors.Is(reviewCtx.Err(), context.Canceled)) {
				reviewPath = "codebase-read-degraded"
				timeoutSummary := fmt.Sprintf("Review UNVERIFIABLE: codebase-read call timed out or was cancelled after %s (%v) — could not independently verify criteria against the codebase.", callTimeout, reviewCtx.Err())
				is, createErr := session.RecordDegradedReviewVerdict(s.storage, item.ID, session.AcCriteriaJSON(acSnapshotJSON), headlessReReviewUUIDPrefix, timeoutSummary)
				if createErr != nil {
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review timeout verdict: %w", createErr))
				}
				log.WarningLog.Printf("[TriggerReReview] headless re-review complete for item %s (outcome %s, path=%s, duration_ms=%d)", item.ID, session.ReviewVerdictUnverifiable, reviewPath, time.Since(callStart).Milliseconds())
				return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
					ItemSession: itemSessionToProto(is, s.buildCostLookup()),
				}), nil
			}
			// Persist a best-effort audit record of the failed call before returning the
			// RPC error: unlike TriggerTriage, no ItemSession exists yet for this attempt
			// at this point (one is normally only created alongside the verdict below), so
			// without this the failure would be visible only in the ephemeral log line —
			// exactly the gap captureHeadlessFailure exists to close. Never blocks or
			// changes the returned error: every step here is best-effort/log-and-continue.
			errType := classifyHeadlessCallError(callErr, time.Since(callStart), callTimeout)
			capturePath := s.captureHeadlessFailure(headlessReReviewUUIDPrefix+uuid.New().String(), reviewResult)
			log.ErrorLog.Printf("[TriggerReReview] headless re-review call failed item=%s errType=%s capture=%s: %v", item.ID, errType, capturePath, callErr)
			failCleanupCtx, failCleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if failIS, failCreateErr := s.storage.CreateItemSession(failCleanupCtx, session.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: headlessReReviewUUIDPrefix + uuid.New().String(),
				SessionRole: session.SessionRoleReview,
				AcSnapshot:  session.AcCriteriaJSON(acSnapshotJSON),
			}); failCreateErr != nil {
				log.WarningLog.Printf("[TriggerReReview] failed to record audit ItemSession for failed call item=%s: %v", item.ID, failCreateErr)
			} else {
				_ = s.storage.UpdateItemSessionEndedWithReason(failCleanupCtx, failIS.ID, time.Now(), errType)
				if capturePath != "" {
					_ = s.storage.UpdateItemSessionFailureCapture(failCleanupCtx, failIS.ID, capturePath)
				}
			}
			failCleanupCancel()
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("headless re-review call failed: %w", callErr))
		}

		overall, perCriterion, reviewSummary := session.ParseHeadlessVerdictResult(reviewResult)
		toolReads := session.ParseHeadlessToolReads(reviewResult)
		overall, perCriterion, reviewSummary, reviewPath = session.DegradeIfUnverified(reviewPath, overall, perCriterion, reviewSummary, toolReads, codebaseWorkDir)
		// reviewPath now carries the final path label ("diff", "codebase-read-verified",
		// or "codebase-read-degraded"), logged below via Epic 2.5's path=/duration_ms=
		// observability fields.
		perCriterionJSON, _ := json.Marshal(perCriterion)

		// cleanupCtx is a separate, freshly-derived context (not ctx, which may itself be
		// close to its own deadline by the time a long-but-successful re-review call
		// returns — e.g. an RPC deadline or the caller's own bounding timeout, even though
		// the call itself already succeeded within reviewCtx's own budget). Same rationale
		// as ReviewGateRunner.Run's success-path cleanupCtx and RecordDegradedReviewVerdict's
		// cleanupCtx above: persistence is a separate, short, always-should-succeed
		// operation that must not be held hostage by the review call's context lifetime.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		reviewSessionUUID := headlessReReviewUUIDPrefix + uuid.New().String()
		is, createErr := s.storage.CreateItemSessionWithVerdict(cleanupCtx, session.ItemSessionData{
			ItemID:           item.ID,
			SessionUUID:      reviewSessionUUID,
			SessionRole:      session.SessionRoleReview,
			AcSnapshot:       session.AcCriteriaJSON(acSnapshotJSON),
			EstimatedCostUsd: callCostUSD,
		}, session.ReviewVerdictData{
			OverallOutcome: overall,
			PerCriterion:   string(perCriterionJSON),
			Summary:        reviewSummary,
			DiffHash:       s.storage.ComputeCurrentDiffHash(cleanupCtx, item.ID),
		})
		if createErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review verdict: %w", createErr))
		}
		if endErr := s.storage.UpdateItemSessionEnded(cleanupCtx, is.ID, time.Now()); endErr != nil { //nolint:silenttransition bookkeeping timestamp only; the PASS/done transition a few lines below (which does notify on failure) is what actually gates forward progress here
			log.WarningLog.Printf("[TriggerReReview] UpdateItemSessionEnded: %v", endErr)
		}

		log.InfoLog.Printf("[TriggerReReview] headless re-review complete for item %s (outcome %s, path=%s, duration_ms=%d)", item.ID, overall, reviewPath, time.Since(callStart).Milliseconds())

		// A fresh review verdict now exists — resolve any open abandoned_review
		// row immediately (Task 2.1.5b) rather than waiting for the self-heal
		// sweep's next tick.
		if _, resolveErr := s.storage.ResolveStuck(ctx, item.ID, domain.StuckReasonAbandonedReview); resolveErr != nil {
			log.WarningLog.Printf("[TriggerReReview] ResolveStuck(abandoned_review) item=%s: %v", item.ID, resolveErr)
		}

		// On PASS, auto-transition to done rather than leaving the item sitting in
		// review awaiting a manual "Approve — Mark Done" click — matches the
		// behavior of the tmux-driven submit_review_verdict MCP tool and
		// SubmitManualReview, both of which already auto-transition on PASS.
		// Best-effort: verdict is already persisted regardless of transition outcome.
		//
		// Gated on isCodeShippedToMain: a PASS verdict says the code is good, not
		// that it has actually landed on main, and this path (unlike the RPC
		// handler) has no override_reason escape hatch — if it can't verify, it
		// must leave the item in review rather than silently mark it done. The
		// item's "Ship PR" action (backlog_service_ship.go) is the intended
		// recovery path once left here (docs/tasks/backlog-feature-improvement.md,
		// 2026-07-18 update).
		if overall == session.ReviewVerdictPass {
			if !s.isCodeShippedToMain(ctx, item.ID, item.RepoPath, "TriggerReReview") {
				log.InfoLog.Printf("[TriggerReReview] item=%s PASS verdict but code not verified on main — leaving in review for manual transition/override", item.ID)
			} else {
				precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReview)}
				if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, precondition, session.TriggeredBySystem); transErr != nil {
					log.WarningLog.Printf("[TriggerReReview] PASS but transition to done failed: %v", transErr)
					// Code is confirmed shipped to main — the item is left stuck in
					// review with nothing further to trigger a retry.
					s.notifyTransitionFailed(item.ID, item.Title, "code was confirmed shipped to main but the item's transition to done failed", transErr)
				}
			}
		}

		return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
			ItemSession: itemSessionToProto(is, s.buildCostLookup()),
		}), nil
	}

	// 10. Spawn re-review session — AutonomousDriver mode if available, oneShot fallback.
	if s.sessionCreator == nil {
		log.InfoLog.Printf("[TriggerReReview] triggered for item %s but no SessionCreator available", item.ID)
		return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
			ItemSession: &sessionv1.ItemSession{
				Id:          item.ID,
				SessionRole: "re-review-triggered",
			},
		}), nil
	}

	slug := slugify(item.Title)
	title := "re-review:" + slug
	useAutonomous := s.autonomousStarter != nil

	// Kill any stale tmux session with this title so the new session gets a fresh
	// pane and the autonomous driver can deliver its prompt without attaching to an
	// old, idle session that was left behind from a previous (possibly crashed) attempt.
	if s.sessionStopper != nil {
		_ = s.sessionStopper.KillTmuxSessionByTitle(ctx, title)
	}

	inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
		[]string{"backlog:review"}, !useAutonomous /*oneShot*/, true /*hidden*/)
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
	}
	inst.SetCategory(session.CategoryBacklog)
	if useAutonomous {
		s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
	}

	// 11. Create ItemSession with role=review.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
		AcSnapshot:  session.AcCriteriaJSON(acSnapshotJSON),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create re-review item session: %w", err))
	}

	// Capture the pre-review HEAD SHA so diffs against base..HEAD work correctly.
	if baseSHA, shaErr := session.GetGitHeadSHA(item.RepoPath); shaErr == nil && baseSHA != "" {
		_ = s.storage.SetItemSessionBaseCommit(ctx, is.ID, baseSHA)
	}

	log.InfoLog.Printf("[TriggerReReview] spawned re-review session %s for item %s", inst.UUID, item.ID)

	// A review session is active again — resolve any open abandoned_review row
	// immediately (Task 2.1.5b) rather than waiting for the self-heal sweep's
	// next tick.
	if _, resolveErr := s.storage.ResolveStuck(ctx, item.ID, domain.StuckReasonAbandonedReview); resolveErr != nil {
		log.WarningLog.Printf("[TriggerReReview] ResolveStuck(abandoned_review) item=%s: %v", item.ID, resolveErr)
	}

	return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// tombstoneOrphanWorkSessions marks any open (not-yet-ended) work-role ItemSession as
// ended if it is confirmed dead (no live tracked session). Called before
// hasActiveWorkSession's guard in SpawnSessionFromItem so a work session that never
// reached its normal completion path (crash, kill, server restart mid-session) doesn't
// block every future spawn attempt for the item forever — the same class of gap as
// tombstoneOrphanTriageSessions below, but for work sessions. Conservative: if
// sessionStopper isn't wired, liveness is unknown, so nothing is tombstoned (same "assume
// alive" policy as reconcileStuckReviewItems' zombie-session check). Mutates sessions'
// EndedAt in place so callers checking the same slice immediately after see the tombstone.
func (s *BacklogService) tombstoneOrphanWorkSessions(ctx context.Context, itemID string, sessions []session.ItemSessionSummary) {
	if s.sessionStopper == nil {
		return
	}
	var freed []session.ItemSessionSummary
	for i := range sessions {
		is := &sessions[i]
		if is.Role != string(session.SessionRoleWork) || is.EndedAt != nil {
			continue
		}
		if s.sessionStopper.IsSessionLive(is.SessionUUID) {
			continue // genuinely still running
		}
		now := time.Now()
		if err := s.storage.UpdateItemSessionEnded(ctx, is.ID, now); err != nil { //nolint:silenttransition best-effort tombstone sweep; continue skips only this session, retried every call rather than silently proceeding as if it succeeded
			log.WarningLog.Printf("[tombstoneOrphanWorkSessions] item=%s session=%s: %v", itemID, is.ID, err)
			continue
		}
		log.InfoLog.Printf("[tombstoneOrphanWorkSessions] item=%s tombstoned dead work session=%s (created %s)", itemID, is.ID, is.CreatedAt)
		is.EndedAt = &now
		freed = append(freed, *is)
	}
	// Prune the worktree for every session just tombstoned here, rather than leaving it
	// on disk until the item is reopened/re-triaged — a dead work session's directory
	// otherwise lingers indefinitely and can later be found "missing" by a session that
	// still references it.
	if len(freed) > 0 {
		s.cleanupItemWorktrees(ctx, freed)
	}
}

// killEndedWorkSessionPanes closes the tmux pane for every already-ended work
// session in the given list. Best-effort and nil-safe (no-op if sessionStopper
// isn't wired) — called right before spawning a new rework round so a
// finished round's pane doesn't linger forever. Uses KillTmuxPaneOnly, not
// StopSessionByUUID, since rework rounds share one worktree/branch across
// their "-rN" revisions (see buildRevisionTitle) and StopSessionByUUID's
// Instance.Kill also runs CleanupWorktree.
func (s *BacklogService) killEndedWorkSessionPanes(ctx context.Context, sessions []session.ItemSessionSummary) {
	if s.sessionStopper == nil {
		return
	}
	for _, is := range sessions {
		if is.Role != string(session.SessionRoleWork) || is.EndedAt == nil {
			continue
		}
		if err := s.sessionStopper.KillTmuxPaneOnly(ctx, is.SessionUUID); err != nil {
			log.WarningLog.Printf("[killEndedWorkSessionPanes] session=%s: %v", is.SessionUUID, err)
		}
	}
}

// tombstoneOrphanTriageSessions marks any open triage ItemSessions that are no longer
// live as ended. Returns CodeAlreadyExists if a live triage session is genuinely running.
func (s *BacklogService) tombstoneOrphanTriageSessions(ctx context.Context, itemID, itemStatus string, sessions []session.ItemSessionSummary) error {
	for _, is := range sessions {
		if is.Role != string(session.SessionRoleTriage) || is.EndedAt != nil {
			continue
		}
		// A headless triage session has no live tmux instance to query — check this
		// process's own triageInFlight record instead (see that field's doc comment;
		// BUG-054). A non-headless (tmux-backed) session falls back to sessionStopper
		// as before. Sessions older than maxTriageSessionAge are treated as orphaned
		// regardless, to prevent a genuinely hung or leaked call from blocking
		// re-trigger indefinitely.
		isHeadless := strings.HasPrefix(is.SessionUUID, headlessTriageUUIDPrefix)
		isStale := time.Since(is.CreatedAt) > maxTriageSessionAge
		var live bool
		if isHeadless {
			live = s.IsTriageLive(itemID)
		} else {
			live = s.sessionStopper != nil && s.sessionStopper.IsSessionLive(is.SessionUUID)
		}
		notLive := isStale || !live
		statusAdvanced := itemStatus != string(session.BacklogStatusIdea)
		if notLive || statusAdvanced {
			// BUG-065: attribute this tombstone to "shutdown" — matching
			// classifyHeadlessCallError's bucket name, which
			// reconcileOrphanedTriageItems' shutdown carve-out (session/backlog_lifecycle.go)
			// keys off to respawn for free with no stuck-notification — when the row
			// can ONLY be explained by our own process having restarted since it was
			// created, not a genuine failure. See shouldAttributeTombstoneToShutdown's
			// doc comment for the exact criteria.
			reason := ""
			if shouldAttributeTombstoneToShutdown(isHeadless, isStale, live, is.CreatedAt, serverStartTime) {
				reason = "shutdown"
			}
			if reason != "" {
				_ = s.storage.UpdateItemSessionEndedWithReason(ctx, is.ID, time.Now(), reason)
			} else {
				_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, time.Now())
			}
			continue
		}
		return connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("triage session already running for item %s", itemID))
	}
	return nil
}

// shouldAttributeTombstoneToShutdown reports whether an open triage session
// tombstoneOrphanTriageSessions is about to close can only be explained by
// this process's own restart, rather than a genuine failure — BUG-065. Pure
// and side-effect-free (bootTime passed explicitly, not read as a package
// var) so the decision table is directly testable, same rationale as
// evaluateRemediation (session/backlog_remediation.go) and
// classifyHeadlessCallError above.
//
// s.triageInFlight (backing IsTriageLive, hence the `live` param) is a fresh
// in-memory map every boot: it can never truthfully vouch for a session this
// process didn't itself start, so "!live" alone is ambiguous between "dead"
// and "we just don't remember it." createdAt.Before(bootTime) resolves that
// ambiguity — a session created before this process even started can only be
// a leftover from a prior (now-dead) instance. Two conditions must ALSO hold
// to avoid misattributing genuinely unrelated tombstones:
//   - !isStale: an isStale row is independently explained by
//     maxTriageSessionAge (genuinely old/hung), regardless of which process
//     created it — don't relabel that as "our restart's fault".
//   - isHeadless: a non-headless (tmux-backed) session's liveness comes from
//     sessionStopper, not triageInFlight, so createdAt-vs-bootTime carries no
//     signal about our own process lifecycle for it.
func shouldAttributeTombstoneToShutdown(isHeadless, isStale, live bool, createdAt, bootTime time.Time) bool {
	return isHeadless && !isStale && !live && createdAt.Before(bootTime)
}

// findPriorTriageResult returns the most recent successfully-parsed triage result from
// the provided sessions, along with a boolean indicating whether one was found.
func findPriorTriageResult(sessions []session.ItemSessionSummary) (session.HeadlessTriageResult, bool) {
	for i := len(sessions) - 1; i >= 0; i-- {
		is := sessions[i]
		if is.Role != string(session.SessionRoleTriage) || is.TriageResult == "" {
			continue
		}
		var result session.HeadlessTriageResult
		if jsonErr := json.Unmarshal([]byte(is.TriageResult), &result); jsonErr == nil {
			return result, true
		}
	}
	return session.HeadlessTriageResult{}, false
}

// applyTriageResultToUpdate re-indexes and status-normalises the AC criteria from a
// triage result, then writes the serialized JSON into the provided update struct.
// Also applies the LLM's assessed priority and item category, when it provided valid
// ones — this is what makes triage assign labels/priority rather than leaving every
// item at DefaultBacklogPriority forever, which is otherwise indistinguishable from
// "genuinely assessed as normal" and defeats priority-ordered auto-spawn (see
// DequeueNextQueuedItems). Each field is independently optional: a missing or invalid
// value leaves the item's existing priority/category untouched rather than
// clobbering it with a zero value — same convention AcceptanceCriteria already uses
// here (an empty result means "no assessment", not "clear the existing value").
func applyTriageResultToUpdate(result *session.HeadlessTriageResult, update *session.BacklogItemUpdate) {
	if len(result.AcceptanceCriteria) > 0 {
		// Re-index to ensure 0-based contiguous indices regardless of what the model output.
		for i := range result.AcceptanceCriteria {
			result.AcceptanceCriteria[i].Index = i
			if result.AcceptanceCriteria[i].Status == "" {
				result.AcceptanceCriteria[i].Status = "pending"
			}
		}
		if acJSON, marshalErr := session.SerializeAcCriteria(result.AcceptanceCriteria); marshalErr == nil {
			update.AcceptanceCriteria = &acJSON
		}
	}

	if result.Priority >= 1 && result.Priority <= 5 {
		p := result.Priority
		update.Priority = &p
	}

	// IsValidBacklogCategory also accepts "" (its own "uncategorized" convention),
	// which must NOT be treated as a real assessment here — an omitted
	// item_category means "no assessment", not "clear the existing category".
	if result.ItemCategory != "" && session.IsValidBacklogCategory(result.ItemCategory) {
		c := result.ItemCategory
		update.Category = &c
	}
}

// findMostRecentSessions returns the most recently created review and work ItemSessions
// from the provided list. Either return value may be nil if no session of that role exists.
func findMostRecentSessions(sessions []session.ItemSessionSummary) (reviewSession, workSession *session.ItemSessionSummary) {
	for i := range sessions {
		is := &sessions[i]
		switch is.Role {
		case session.SessionRoleReview:
			if reviewSession == nil || is.CreatedAt.After(reviewSession.CreatedAt) {
				reviewSession = is
			}
		case session.SessionRoleWork:
			if workSession == nil || is.CreatedAt.After(workSession.CreatedAt) {
				workSession = is
			}
		}
	}
	return
}

// resolveCodebaseWorkDir returns the directory the headless codebase-read review call
// (BuildReviewCallOptions' empty-diff branch) should be granted Read/Grep/Glob access
// to, and whether that directory is safe to use for that purpose. Prefers the work
// session's dedicated worktree path (freshest, matches the session's actual branch).
// Falls back to repoPath only when there is no work session at all to fall back
// from — the one case where repoPath is genuinely the only directory available, not a
// stand-in for the item's own (missing) state.
//
// The existence check on the worktree path exists because the DB-recorded worktree row
// can outlive the worktree directory itself (e.g. cleanup deleted the directory without
// pruning the row) — see the confirmed live incident on the "Backlog History feature
// Broken" item (PR #173): get_session_diff reported "worktree path does not exist" for
// a session whose worktree row still resolved successfully.
//
// BUG-045 (confirmed live on item 693c2700, PR #216): when a work session exists but its
// worktree data cannot be resolved at all (the underlying session/worktree row itself
// was reaped, or the storage lookup otherwise fails), this function used to silently
// fall back to repoPath and report it as "exists" — repoPath obviously exists, since for
// every backlog item in this project it resolves to the single shared main checkout the
// human operator (and any concurrent Claude Code session) actively works in day to day.
// Granting the reviewer live Read/Grep/Glob access to that directory hands it whatever
// unrelated, uncommitted work happens to be sitting there at that exact moment, as if it
// were the item's own diff — producing a plausible-sounding but completely wrong verdict
// (item 693c2700's review reported FAIL describing an entirely unrelated tmux fix that
// happened to be stashed in the operator's checkout, not any of the item's real work).
// A work session with no resolvable worktree now refuses the codebase-read fallback
// outright (dir is still returned, for logging, but exists is always false) — mirroring
// ReviewGateRunner.Run's (session/review_gate.go) refusal to hand the reviewer a diff it
// could not positively compute. The caller must check exists before proceeding into
// codebase-read mode.
func (s *BacklogService) resolveCodebaseWorkDir(ctx context.Context, repoPath string, workSession *session.ItemSessionSummary) (dir string, exists bool) {
	if workSession == nil {
		// No work session at all for this item — repoPath is the only directory
		// available, and there is nothing item-specific it could be masking.
		info, statErr := os.Stat(repoPath)
		return repoPath, statErr == nil && info.IsDir()
	}
	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		// A work session exists but its dedicated worktree cannot be resolved. Refuse
		// the repoPath fallback rather than risk granting the reviewer live access to
		// the shared main checkout's current, arbitrary working-tree state (BUG-045).
		return repoPath, false
	}
	info, statErr := os.Stat(wt.WorktreePath)
	return wt.WorktreePath, statErr == nil && info.IsDir()
}

// getWorkSessionDiff returns the git diff for the given work session. It prefers the
// session's dedicated worktree path and base SHA; falls back to the item's repo when
// the worktree directory is gone (commits remain accessible via the shared object store).
func (s *BacklogService) getWorkSessionDiff(ctx context.Context, repoPath string, workSession *session.ItemSessionSummary) string {
	if workSession == nil {
		return ""
	}
	diffDir := repoPath
	diffBaseSHA := ""
	diffHeadRef := ""
	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID)
	if wtErr == nil && wt.WorktreePath != "" {
		// Try the dedicated worktree first.
		diff, _, diffErr := session.GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
		if diffErr == nil {
			return diff
		}
		// Worktree path is gone — fall through to repo fallback using the same base SHA
		// and an explicit branch ref: repoPath's own checked-out HEAD is not the work
		// branch's tip, so diffing against implicit HEAD would compare against whatever
		// the shared main checkout happens to have, not the agent's actual work.
		log.WarningLog.Printf("[TriggerReReview] GetGitDiff in worktree failed (path gone?): %v; falling back to repo", diffErr)
		diffBaseSHA = wt.BaseCommitSHA
		diffHeadRef = wt.BranchName
	}
	// Fallback: diff in the main repo between base and last commit. Git worktrees
	// share the object store, so commits from any worktree are reachable here.
	// BaseCommitSha, not LastCommitSha: this is the *base* of the diff. It only
	// ever worked because the two were historically the same value — spawn wrote
	// the base SHA into LastCommitSha and nothing ever refreshed it (BUG-047).
	if diffBaseSHA == "" && workSession.BaseCommitSha != "" {
		diffBaseSHA = workSession.BaseCommitSha
	}
	diff, _, diffErr := session.GetGitDiffRef(ctx, diffDir, diffBaseSHA, diffHeadRef)
	if diffErr == nil {
		return diff
	}
	log.WarningLog.Printf("[TriggerReReview] GetGitDiff fallback in %s failed: %v", diffDir, diffErr)

	// Auto-repair: mirror ReviewGateRunner.Run's recovery (session/review_gate.go) for a
	// stale/corrupted base_commit_sha — the same failure mode found via manual QA on item
	// ae1e2070 and fixed there first. Only attemptable when a branch ref is known; recompute
	// the merge-base of repoPath's own checked-out HEAD against the branch and retry once
	// before giving up on what may just be a recoverable infrastructure hiccup rather than
	// "no changes were made".
	if diffHeadRef != "" {
		if recoveredSHA, recoverErr := session.RecoverBaseCommitSHA(ctx, diffDir, diffHeadRef); recoverErr != nil {
			log.WarningLog.Printf("[TriggerReReview] RecoverBaseCommitSHA in %s ref=%s failed: %v", diffDir, diffHeadRef, recoverErr)
		} else if recoveredDiff, _, retryErr := session.GetGitDiffRef(ctx, diffDir, recoveredSHA, diffHeadRef); retryErr != nil {
			log.WarningLog.Printf("[TriggerReReview] retry with recovered base %s in %s failed: %v", recoveredSHA, diffDir, retryErr)
		} else if strings.TrimSpace(recoveredDiff) == "" {
			// A recovered base that produces an empty diff is indistinguishable from
			// "nothing changed" and just as unsafe to trust as the original failure — see
			// the identical guard in ReviewGateRunner.Run. Fall through and return "" below
			// rather than treating this as a successful repair.
			log.WarningLog.Printf("[TriggerReReview] recovered base %s ref=%s produced an empty diff — not trusting it", recoveredSHA, diffHeadRef)
		} else {
			log.InfoLog.Printf("[TriggerReReview] auto-repaired broken base commit ref=%s recovered=%s (recorded=%s)", diffHeadRef, recoveredSHA, diffBaseSHA)
			return recoveredDiff
		}
	}
	return ""
}

// resolveACSnapshot returns the acceptance criteria to use for a re-review. It prefers
// the snapshot captured at work-session start; falls back to the item's current AC.
func resolveACSnapshot(workSession *session.ItemSessionSummary, itemAC session.AcCriteriaJSON) []session.AcCriterion {
	live, _ := session.ParseAcCriteria(itemAC)
	if workSession != nil && workSession.AcSnapshot != "" {
		if ac, _ := session.ParseAcCriteria(workSession.AcSnapshot); len(ac) > 0 {
			return session.MergeLiveCriterionNotes(ac, live)
		}
	}
	return live
}
