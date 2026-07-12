package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// ReviewGateRunner encapsulates the spawnReviewGate logic into a testable value type.
// BacklogLifecycleListener holds one as a field and delegates to it.
//
// getPool and getAutoReopener are getter functions rather than stored values so that
// the runner always observes the latest pool / reopener even when
// SetHeadlessPool / SetAutoReopener are called after construction.
type ReviewGateRunner struct {
	storage         *Storage
	getPool         func() *headless.Pool
	getAutoReopener func() AutoReopenSpawner
	sessionCreator  ReviewGateSpawner
}

// NewReviewGateRunner constructs a ReviewGateRunner.
// getPool and getAutoReopener are getter functions (typically method values from
// BacklogLifecycleListener) so the runner sees the latest values when dynamic
// setters are called after construction.
func NewReviewGateRunner(
	storage *Storage,
	getPool func() *headless.Pool,
	getAutoReopener func() AutoReopenSpawner,
	sessionCreator ReviewGateSpawner,
) *ReviewGateRunner {
	return &ReviewGateRunner{
		storage:         storage,
		getPool:         getPool,
		getAutoReopener: getAutoReopener,
		sessionCreator:  sessionCreator,
	}
}

// Run executes the review gate for a backlog item session.
// ctx should be the listener's shutdownCtx so long-running calls are cancelled
// on shutdown.
// onPass is called when the headless review outcome is PASS; typically
// BacklogLifecycleListener.pushAndCreatePR.
func (r *ReviewGateRunner) Run(
	ctx context.Context,
	item *BacklogItemData,
	is ItemSessionSummary,
	onPass func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary),
) {
	// Short-circuit: items with SkipReviewGate bypass the review mechanism entirely.
	if item.SkipReviewGate {
		return
	}

	// Precondition: repo_path must be set or we have nothing to review.
	if item.RepoPath == "" {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate item=%s has no repo path set; skipping review gate", item.ID)
		return
	}

	// Get the committed diff from the session's dedicated worktree (preferred)
	// or fall back to the item's repo path (directory-mode / worktree gone).
	var diff string
	var truncated bool
	var uncommittedWarning string
	wt, wtErr := r.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
	if wtErr == nil && wt.WorktreePath != "" {
		// Belt-and-suspenders layer 2: warn if the worktree still has uncommitted changes.
		// request_review (layer 1) should have caught this, but flag it here too so the
		// reviewer prompt is aware and the verdict reflects the incomplete state.
		if dirty, dirtyErr := IsWorktreeDirty(ctx, wt.WorktreePath); dirtyErr == nil && dirty {
			log.InfoLog.Printf("[BacklogLifecycle] review gate: item=%s has uncommitted changes in worktree — diff will be incomplete", item.ID)
			uncommittedWarning = "[WARNING: worktree has uncommitted changes — the following diff may be incomplete]\n"
		}
		var diffErr error
		diff, truncated, diffErr = GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
		if diffErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff (worktree) item=%s: %v; falling back to repo", item.ID, diffErr)
			diff, truncated, diffErr = GetGitDiff(ctx, item.RepoPath, wt.BaseCommitSHA)
			if diffErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff (repo fallback) item=%s: %v", item.ID, diffErr)
			}
		}
	} else {
		var diffErr error
		diff, truncated, diffErr = GetGitDiff(ctx, item.RepoPath, is.LastCommitSha)
		if diffErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff item=%s: %v", item.ID, diffErr)
		}
	}
	if uncommittedWarning != "" {
		diff = uncommittedWarning + diff
	}

	// Security check — block if secrets detected.
	if secErr := RunPreGateSecurityCheck(diff); secErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate security check blocked item=%s: %v", item.ID, secErr)
		// Record a failed review ItemSession with a FAIL verdict so the gate verdict
		// is visible in the UI and operators can act (override or re-review).
		summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)
		secIS, secCreateErr := r.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "review-blocked-" + item.ID,
			SessionRole: SessionRoleReview,
		}, ReviewVerdictData{
			OverallOutcome: ReviewVerdictFail,
			Summary:        summary,
		})
		if secCreateErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (security block) item=%s: %v", item.ID, secCreateErr)
			return
		}
		if updateErr := r.storage.UpdateItemSessionEnded(ctx, secIS.ID, time.Now()); updateErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate UpdateItemSessionEnded (security block) item=%s: %v", item.ID, updateErr)
		}
		log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate security check blocked for item %s — FAIL verdict recorded", item.ID)
		return
	}

	// Deserialize AC snapshot.
	acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
	if len(acSnapshot) == 0 {
		acSnapshot, _ = ParseAcCriteria(item.AcceptanceCriteria)
	}

	prompt := BuildReviewPrompt(item, acSnapshot, diff, truncated, is.ID)

	pool := r.getPool()
	if pool != nil {
		// Headless path: call LLM directly without spawning a tmux session.
		// Use JSON-output prompts because headless claude -p has no tool access.
		reviewCtx, reviewCancel := context.WithTimeout(ctx, headless.DefaultCallTimeout)
		defer reviewCancel()

		headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated)
		reviewResult, callCostUSD, callErr := pool.CallBlockingWithCost(reviewCtx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt)
		if callErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate headless.CallBlocking item=%s: %v", item.ID, callErr)
			// Record a FAIL verdict so the item is not stuck in review with no actionable result.
			failUUID := "headless-review-" + uuid.New().String()
			failIS, createErr := r.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: failUUID,
				SessionRole: SessionRoleReview,
				AcSnapshot:  is.AcSnapshot,
			}, ReviewVerdictData{
				OverallOutcome: ReviewVerdictFail,
				Summary:        fmt.Sprintf("Review failed: %v", callErr),
			})
			if createErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (headless fail) item=%s: %v", item.ID, createErr)
			} else if updateErr := r.storage.UpdateItemSessionEnded(ctx, failIS.ID, time.Now()); updateErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate UpdateItemSessionEnded (headless fail) item=%s: %v", item.ID, updateErr)
			}
			return
		}

		overall, perCriterion, summary := ParseHeadlessVerdictResult(reviewResult)
		perCriterionJSON, _ := json.Marshal(perCriterion)

		// Update AC statuses on the item to reflect what was verified.
		applyVerdictsToACs(ctx, r.storage, item, acSnapshot, perCriterion)

		// Create a synthetic ItemSession and its ReviewVerdict atomically so there
		// is never a dangling session with no verdict if the verdict write fails.
		reviewSessionUUID := "headless-review-" + uuid.New().String()
		reviewIS, createErr := r.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
			ItemID:           item.ID,
			SessionUUID:      reviewSessionUUID,
			SessionRole:      SessionRoleReview,
			AcSnapshot:       is.AcSnapshot,
			EstimatedCostUsd: callCostUSD,
		}, ReviewVerdictData{
			OverallOutcome: overall,
			PerCriterion:   string(perCriterionJSON),
			Summary:        summary,
		})
		if createErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (headless) item=%s: %v", item.ID, createErr)
			return
		}
		if updateErr := r.storage.UpdateItemSessionEnded(ctx, reviewIS.ID, time.Now()); updateErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate UpdateItemSessionEnded (headless) item=%s: %v", item.ID, updateErr)
		}

		log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s)", item.ID, reviewIS.ID, overall)

		// Auto-reopen: if verdict is FAIL or PARTIAL, immediately transition the item
		// back to in_progress and spawn a new work session so the review→rework cycle
		// is fully automated without requiring manual intervention.
		if reopener := r.getAutoReopener(); (overall == ReviewVerdictFail || overall == ReviewVerdictPartial || overall == ReviewVerdictUnverifiable) && reopener != nil {
			go func() {
				if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
					log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview item=%s: %v", item.ID, err)
				} else {
					log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate auto-reopened item %s for rework (verdict %s)", item.ID, overall)
				}
			}()
		}

		// On PASS: push the branch, create a PR, and move to pr_pending so the work
		// is visible on GitHub and a human (or the reconciler) can merge it to done.
		// Falls back to direct done transition when no worktree is available.
		if overall == ReviewVerdictPass {
			onPass(ctx, item, is)
		}
		return
	}

	// Legacy path: spawn a tmux review session via sessionCreator.
	if r.sessionCreator == nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate item=%s: no review mechanism configured", item.ID)
		return
	}

	reviewInst, spawnErr := r.sessionCreator.SpawnReviewSession(ctx, item, is.ID, prompt)
	if spawnErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate SpawnReviewSession item=%s: %v", item.ID, spawnErr)
		return
	}

	// Create ItemSession linking the new review session to the backlog item.
	if _, createErr := r.storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewInst.UUID,
		SessionRole: SessionRoleReview,
		AcSnapshot:  is.AcSnapshot,
	}); createErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSession item=%s review=%s: %v", item.ID, reviewInst.UUID, createErr)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate spawned review session %s for item %s", reviewInst.UUID, item.ID)
}
