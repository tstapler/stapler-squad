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
// Appends a one-time "other active sessions in this workspace" nudge (AC5) when peers
// exist — best-effort: detection/lookup failures are logged and swallowed rather than
// blocking session creation, since this is a convenience nudge, not required context.
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
// on any detection/lookup failure or when repoPath is empty. Delegates to
// session.WorkspacePeersBlockForPath, shared with SessionService.CreateSession so the two
// callers can't drift on how the nudge is built.
func (s *BacklogService) workspacePeersBlockFor(ctx context.Context, repoPath string) string {
	return session.WorkspacePeersBlockForPath(ctx, s.storage, repoPath)
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

// headlessTriageUUIDPrefix is prepended to all synthetic ItemSession UUIDs created by the
// headless triage path. The orphan guard uses this prefix to identify sessions that have no
// live tmux process and can be safely tombstoned on re-trigger.
// headlessReReviewUUIDPrefix is the equivalent prefix for headless re-review sessions.
const (
	headlessTriageUUIDPrefix   = "headless-triage-"
	headlessReReviewUUIDPrefix = "headless-re-review-"
)

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
// Returns the same errors storage.TransitionBacklogItemStatus returns
// (ErrPreconditionFailed, etc.) on success of the guard checks, or the raw
// domain sentinel error (ErrPlanRequired, ErrACRequired, ...) if a guard
// fails — un-wrapped in connect terms so each call site keeps doing its own
// connect.NewError translation, matching this file's existing style.
func (s *BacklogService) transitionWithGuard(ctx context.Context, item *session.BacklogItemData, to session.BacklogStatus, precondition *session.BacklogItemPrecondition, triggeredBy string) (*session.BacklogItemData, error) {
	from := session.BacklogStatus(item.Status)
	if !s.engine.CanTransition(from, to) {
		return nil, fmt.Errorf("invalid transition from %q to %q", from, to)
	}
	guardInput := session.BacklogItemTransitionInput{
		Status:            from,
		AcCriteria:        item.AcceptanceCriteria,
		PlanApproved:      item.PlanApproved,
		SkipPlanning:      item.SkipPlanning,
		PlanArtifactsPath: item.PlanArtifactsPath,
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
	updated, err := s.transitionWithGuard(ctx, item, session.BacklogStatusQueued, precondition, triggeredBy)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, fmt.Errorf("item status changed concurrently — retry the spawn: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to queue item: %w", err))
	}
	return updated, nil
}

// DequeueNextQueuedItems implements session.QueueDequeuer. It claims and spawns as
// many queued items as there are free WIP slots, oldest-queued (FIFO) first. Called
// from BacklogLifecycleListener.onSessionExited (immediate dequeue the moment a slot
// frees up) and the periodic ReconcileStuck sweep (safety net for a missed hook or a
// concurrency limit raised while items were queued) — see session/backlog_lifecycle.go.
//
// Each candidate is claimed via a SQL-level compare-and-swap (queued->in_progress,
// ExpectedStatus=queued) before spawning, so concurrent callers (this method running
// from both the exit hook and the sweep, or multiple server processes sharing one DB)
// cannot double-claim the SAME item — see TransitionBacklogItemStatus's doc comment.
// That per-item CAS alone does not prevent two concurrent calls to this method from
// each computing their own freeSlots from an unsynchronized snapshot and jointly
// claiming DIFFERENT queued items past the cap, so dequeueMu additionally serializes
// the whole method body, making this method single-flight system-wide (PR #199
// review F2 — the exact "uncontrolled concurrency overshoot" class of bug the WIP
// cap feature exists to prevent).
//
// The claim itself now goes through transitionWithGuard (PR #199 review F4), so an
// item without an approved plan (SkipPlanning=false, PlanApproved=false) cannot be
// claimed at all — defense-in-depth against F3, on top of SpawnSessionFromItem's own
// planning gate now running before the WIP-cap queue gate.
//
// If the claim succeeds but the spawn itself fails (missing repo_path, stale plan
// approval, SessionCreator error), the item is rolled back to queued rather than left
// stranded in_progress with no session.
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

	queued, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusQueued)},
	})
	if err != nil {
		return fmt.Errorf("list queued items: %w", err)
	}
	sort.Slice(queued, func(i, j int) bool {
		ai, aj := queued[i].QueuedAt, queued[j].QueuedAt
		if ai == nil || aj == nil {
			return aj == nil && ai != nil
		}
		return ai.Before(*aj)
	})

	spawned := 0
	for _, item := range queued {
		if spawned >= freeSlots {
			break
		}
		claimed, claimErr := s.transitionWithGuard(ctx, &item,
			session.BacklogStatusInProgress,
			&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusQueued), Note: "dequeued: WIP slot freed"},
			session.TriggeredBySystem)
		if claimErr != nil {
			switch {
			case errors.Is(claimErr, session.ErrPreconditionFailed):
				// Expected under concurrent claims (another process's dequeue
				// sweep, or a manual un-queue) — not worth logging.
			case errors.Is(claimErr, session.ErrPlanRequired), errors.Is(claimErr, session.ErrPlanArtifactsRequired):
				// Defense-in-depth (PR #199 review F2/F3): should be unreachable
				// now that SpawnSessionFromItem's planning gate runs before the
				// WIP-cap gate that queues an item, but refuse the claim rather
				// than silently spawning an unapproved item if this is ever hit
				// (e.g. a future call site regression, or a pre-existing queued
				// row from before that ordering fix).
				log.WarningLog.Printf("[DequeueNextQueuedItems] claim blocked by planning gate item=%s: %v — leaving queued", item.ID, claimErr)
			default:
				log.WarningLog.Printf("[DequeueNextQueuedItems] claim failed item=%s: %v", item.ID, claimErr)
			}
			continue
		}

		resp, spawnErr := s.spawnSessionAfterGates(ctx, claimed, true, item.QueuedAutonomous)
		if spawnErr != nil {
			log.WarningLog.Printf("[DequeueNextQueuedItems] spawn failed for dequeued item=%s: %v; rolling back to queued", item.ID, spawnErr)
			if _, rbErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusQueued,
				&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress), Note: "dequeue spawn failed"},
				session.TriggeredBySystem); rbErr != nil {
				log.ErrorLog.Printf("[DequeueNextQueuedItems] rollback to queued failed item=%s: %v", item.ID, rbErr)
			}
			continue
		}
		spawned++
		log.InfoLog.Printf("[DequeueNextQueuedItems] dequeued and spawned item=%s session=%s", item.ID, resp.Msg.SessionUuid)
	}
	return nil
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
	if hasActiveWorkSession(priorSessions) {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a work session is already active for this item; wait for it to finish or kill it first"))
	}

	// 8. Build agent prompt. Routed through PipelineEngine (Epic 1.5, Story 1.5.5) so a
	// non-default PipelineMode changes what inst.Prompt / AutonomousDriver's goal sees.
	prompt := s.initialPromptFor(ctx, item, priorSessions)

	// 9. Generate session title.
	// On reopen, append a revision number (r2, r3…) based on how many work sessions
	// already exist so the session list shows distinct, human-readable names.
	repoName := slugify(filepath.Base(item.RepoPath))
	baseTitle := repoName + "-" + triageShortTitle(priorSessions, item.Title)
	title := buildRevisionTitle(baseTitle, isReopen, priorSessions)

	// 10. Create a dedicated git worktree for this work session. The branch slug is
	// derived from baseTitle (NOT title) so rework/reopen iterations reuse the same
	// "backlog/<item>" branch instead of minting a new one per -rN revision — the
	// worktree setup path already detects and reuses an existing branch (see
	// git.GitWorktree.Setup), so this just needs a stable slug across reopens.
	// Falls back to a plain directory session if the repo is not git-managed (or
	// worktree creation fails for any other reason — e.g. a bare clone, a detached
	// HEAD, or disk quota hit).
	// Files must be written to the session path BEFORE spawning.
	// worktreeMu guards concurrent spawns from interleaving writes to the same path.
	worktreePath, useWorktree, resolveErr := resolveSessionPath(item.RepoPath, slugify(baseTitle))
	if resolveErr != nil {
		return nil, resolveErr
	}

	if wErr := s.writeSessionFiles(item, priorSessions, worktreePath); wErr != nil {
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
	// all commits the agent makes (not just HEAD~1..HEAD at review time).
	if baseSHA, shaErr := session.GetGitHeadSHA(worktreePath); shaErr == nil && baseSHA != "" {
		_ = s.storage.UpdateItemSessionGitActivity(ctx, is.ID, baseSHA, "", time.Now(), 0)
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
	for _, ps := range priorSessions {
		if ps.Role == session.SessionRoleWork && ps.EndedAt == nil {
			return true
		}
	}
	return false
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
// session ourselves would just trade one bug for a worse one. All this adds
// is a notification once the SAME staleness computation and threshold
// review_queue_determiner.go already uses (Instance.
// GetTimeSinceLastMeaningfulOutput vs
// session.DefaultReviewQueuePollerConfig().StalenessThreshold — reused
// directly rather than inventing a second definition of "stale") confirms
// the blocking session isn't just idle-but-thinking.
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
func (s *BacklogService) notifyIfActiveWorkSessionStale(itemID, itemTitle string, sessions []session.ItemSessionSummary) {
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
	idle, live := s.sessionStopper.TimeSinceLastMeaningfulOutput(active.SessionUUID)
	if !live {
		return
	}
	threshold := session.DefaultReviewQueuePollerConfig().StalenessThreshold
	if idle <= threshold {
		return
	}
	log.WarningLog.Printf("[AutoReopenAfterFailedReview] item %s reopen blocked by active work session %s that is itself stale (%s since last meaningful output, threshold %s)",
		itemID, active.SessionUUID, idle.Round(time.Second), threshold)
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

// hasActiveReviewSession reports whether any of the provided ItemSessions is an
// open (not yet ended) review-role session. Mirrors hasActiveWorkSession; used by
// AutoRespawnReview to avoid double-spawning a review pass that is already running.
func hasActiveReviewSession(priorSessions []session.ItemSessionSummary) bool {
	for _, ps := range priorSessions {
		if ps.Role == session.SessionRoleReview && ps.EndedAt == nil {
			return true
		}
	}
	return false
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
// directory. Returns the resolved path, whether a worktree was used, and any error.
func resolveSessionPath(repoPath, slug string) (worktreePath string, useWorktree bool, err error) {
	wt, wtErr := session.CreateBacklogWorktree(repoPath, slug)
	if wtErr == nil {
		return wt, true, nil
	}
	log.WarningLog.Printf("[SpawnSessionFromItem] worktree creation failed (%v), falling back to directory mode", wtErr)
	dirPath, pathErr := session.ResolveSessionPath(repoPath)
	if pathErr != nil {
		return "", false, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_path: %w", pathErr))
	}
	if dirErr := session.EnsureDirectorySessionPath(dirPath); dirErr != nil {
		return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to prepare session directory: %w", dirErr))
	}
	return dirPath, false, nil
}

// writeSessionFiles writes the backlog slash-command files and context file to the session
// directory. The write is serialized under worktreeMu to prevent concurrent write races.
func (s *BacklogService) writeSessionFiles(item *session.BacklogItemData, priorSessions []session.ItemSessionSummary, worktreePath string) error {
	s.worktreeMu.Lock()
	defer s.worktreeMu.Unlock()
	if wErr := session.WriteSlashCommands(s.pipelineEngine, item, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
	}
	if wErr := session.WriteBacklogContextFile(item, priorSessions, worktreePath); wErr != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
	}
	return nil
}

// AutoReopenAfterFailedReview implements session.AutoReopenSpawner.
// It transitions the item from review back to in_progress and spawns a new
// work session so the review→rework cycle runs without manual intervention.
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

	// The work session for this round may still be alive (it stays running and
	// polls get_backlog_item after request_review — see taskProtocolBlock step 8).
	// Spawning a new one would fail on the hasActiveWorkSession guard anyway and
	// strand the item with only the manual "Reopen for Revision" path; reusing the
	// live session instead keeps its conversation (and prompt cache) intact.
	if hasActiveWorkSession(sessions) {
		log.InfoLog.Printf("[AutoReopenAfterFailedReview] item %s already has an active work session; leaving it in place to pick up the verdict instead of respawning", itemID)
		s.notifyIfActiveWorkSessionStale(itemID, item.Title, sessions)
		return nil
	}

	// Circuit breaker: if the last two verdicts failed for the identical reason,
	// another rework attempt won't change anything either — stop before burning
	// through the (possibly much larger) rework cap and park the item for
	// automated or human remediation instead. Checked ahead of the cap so a
	// fast-looping infrastructure fault (e.g. a broken worktree diff) can't spend
	// the whole cap in minutes.
	recentVerdicts, verdictErr := s.storage.GetRecentReviewVerdictSummaries(ctx, itemID, 2)
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
	if hasActiveWorkSession(sessions) {
		log.InfoLog.Printf("[AutoRespawnAutonomousWork] item %s already has an active work session; skipping respawn", itemID)
		return nil
	}

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

	// Kill the stale tmux pane only (Instance.KillSession, NOT Instance.Kill),
	// keeping the worktree intact so any in-progress but uncommitted work
	// survives for the next work session to pick up. Best-effort: even if the
	// kill fails (session already gone, tmux server hiccup), still tombstone
	// the DB row and respawn below rather than leaving the item stranded on a
	// pure kill failure.
	if s.sessionStopper != nil {
		if killErr := s.sessionStopper.KillTmuxPaneOnly(ctx, active.SessionUUID); killErr != nil {
			log.WarningLog.Printf("[RemediateStaleWorkSession] item=%s session=%s: kill failed (continuing): %v", itemID, active.SessionUUID, killErr)
		}
	}

	now := time.Now()
	if endErr := s.storage.UpdateItemSessionEnded(ctx, active.ID, now); endErr != nil {
		return fmt.Errorf("end stale work session %s: %w", active.ID, endErr)
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
	if hasActiveWorkSession(sessions) {
		log.InfoLog.Printf("[AutoReopenForPRFix] item %s already has an active work session; leaving pr_pending alone", itemID)
		return nil
	}

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
	if hasActiveWorkSession(sessions) || hasActiveReviewSession(sessions) {
		log.InfoLog.Printf("[AutoRespawnReview] item %s already has an active session; skipping respawn", itemID)
		return nil
	}

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

	// 3a. Orphan-aware guard: if an open triage session exists, check whether it is
	// genuinely still running. Headless sessions are always orphaned if not ended
	// (no live tmux session to check) — tombstone them and allow re-trigger.
	existingSessions, listErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if listErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list triage sessions: %w", listErr))
	}
	if err := s.tombstoneOrphanTriageSessions(ctx, req.Msg.ItemId, item.Status, existingSessions); err != nil {
		return nil, err
	}

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
		triagePrompt = session.BuildHeadlessRetriagePrompt(item, artifactAbsPath, priorResult, feedback)
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
	go func() {
		// Acquire concurrency semaphore (max 8 concurrent triage calls).
		select {
		case s.triageSem <- struct{}{}:
		case <-s.shutdownCtx.Done():
			// cleanupCtx is a separate context for DB writes that must complete even
			// after shutdownCtx is cancelled. Passing shutdownCtx here would cause the
			// write to fail immediately with context.Canceled.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		defer func() { <-s.triageSem }()

		triageCtx, cancel := context.WithTimeout(s.shutdownCtx, 30*time.Minute)
		defer cancel()

		raw, _, callErr := s.headlessPool.CallBlocking(triageCtx,
			headless.FeatureKeyTriage,
			headless.HeadlessTriageSystemPrompt(),
			triagePrompt,
			headless.CallOptions{WorkDir: itemRepoPath},
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

		if callErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] headless triage failed item=%s: %v", itemID, callErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}

		result, parseErr := session.ParseHeadlessTriageResult(raw)
		if parseErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] parse result failed item=%s: %v", itemID, parseErr)
			_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
			return
		}
		result.Iteration = iteration
		result.Feedback = feedback

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

		pap := artifactAbsPath
		update := session.BacklogItemUpdate{PlanArtifactsPath: &pap}
		applyTriageACToUpdate(&result, &update)
		if _, updateErr := s.storage.UpdateBacklogItem(cleanupCtx, itemID, update, nil); updateErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] update plan_artifacts_path item=%s: %v", itemID, updateErr)
			persistFailures = append(persistFailures, "saving the plan artifacts path")
		}

		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusIdea)}
		statusAdvanced := true
		if _, transErr := s.storage.TransitionBacklogItemStatus(cleanupCtx, itemID,
			session.BacklogStatusReady, precondition, session.TriggeredBySystem); transErr != nil {
			log.ErrorLog.Printf("[TriggerTriage] status transition idea→ready item=%s: %v", itemID, transErr)
			persistFailures = append(persistFailures, "advancing the item to Ready")
			statusAdvanced = false
		}

		if len(persistFailures) > 0 {
			s.notifyTriagePersistFailure(cleanupCtx, itemID, item.Title, persistFailures, statusAdvanced)
		}

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

		_ = s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())
		log.InfoLog.Printf("[TriggerTriage] headless triage complete item=%s suggestions=%d tasks=%d",
			itemID, len(result.Suggestions), len(result.Tasks))
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
		})
		if createErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review verdict: %w", createErr))
		}
		if endErr := s.storage.UpdateItemSessionEnded(cleanupCtx, is.ID, time.Now()); endErr != nil {
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
		_ = s.storage.UpdateItemSessionGitActivity(ctx, is.ID, baseSHA, "", time.Now(), 0)
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
		if err := s.storage.UpdateItemSessionEnded(ctx, is.ID, now); err != nil {
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
		// Headless triage sessions have no live in-memory instance; treat as orphaned.
		// Sessions older than maxTriageSessionAge are also treated as orphaned to prevent
		// a hung or leaked session from blocking re-trigger indefinitely.
		isHeadless := strings.HasPrefix(is.SessionUUID, headlessTriageUUIDPrefix)
		isStale := time.Since(is.CreatedAt) > maxTriageSessionAge
		notLive := isHeadless || isStale || s.sessionStopper == nil || !s.sessionStopper.IsSessionLive(is.SessionUUID)
		statusAdvanced := itemStatus != string(session.BacklogStatusIdea)
		if notLive || statusAdvanced {
			_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, time.Now())
			continue
		}
		return connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("triage session already running for item %s", itemID))
	}
	return nil
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

// applyTriageACToUpdate re-indexes and status-normalises the AC criteria from a triage
// result, then writes the serialized JSON into the provided update struct.
func applyTriageACToUpdate(result *session.HeadlessTriageResult, update *session.BacklogItemUpdate) {
	if len(result.AcceptanceCriteria) == 0 {
		return
	}
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
	if diffBaseSHA == "" && workSession.LastCommitSha != "" {
		diffBaseSHA = workSession.LastCommitSha
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
