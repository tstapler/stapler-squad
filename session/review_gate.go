package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// headlessReviewUUIDPrefix is prepended to all synthetic ItemSession UUIDs created by
// the headless review gate path (both real verdicts and degraded UNVERIFIABLE ones).
// Mirrors backlog_service_triage.go's headlessReReviewUUIDPrefix for the re-review path.
const headlessReviewUUIDPrefix = "headless-review-"

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
	getNotifier     func() Notifier
	sessionCreator  ReviewGateSpawner

	// capabilityCheck gates the first codebase-read call per process lifetime (Story
	// 2.2.6). Defaults to headless.DefaultCapabilitySelfCheck (shared with
	// TriggerReReview so a failure discovered via either call site short-circuits
	// the other) but is a field — not a hardcoded package-var reference — so tests
	// can inject a fresh instance instead of fighting the singleton's sync.Once.
	capabilityCheck *headless.CodebaseReadCapabilitySelfCheck
}

// NewReviewGateRunner constructs a ReviewGateRunner.
// getPool, getAutoReopener, and getNotifier are getter functions (typically method
// values from BacklogLifecycleListener) so the runner sees the latest values when
// dynamic setters are called after construction.
func NewReviewGateRunner(
	storage *Storage,
	getPool func() *headless.Pool,
	getAutoReopener func() AutoReopenSpawner,
	getNotifier func() Notifier,
	sessionCreator ReviewGateSpawner,
) *ReviewGateRunner {
	return &ReviewGateRunner{
		storage:         storage,
		getPool:         getPool,
		getAutoReopener: getAutoReopener,
		getNotifier:     getNotifier,
		sessionCreator:  sessionCreator,
		capabilityCheck: headless.DefaultCapabilitySelfCheck,
	}
}

// SetCapabilityCheck overrides the codebase-read capability self-check instance.
// Exposed for tests, which need a fresh (non-shared) instance to avoid the
// package-level singleton's sync.Once making later tests observe an earlier test's
// cached result. Production callers should rely on the default.
func (r *ReviewGateRunner) SetCapabilityCheck(c *headless.CodebaseReadCapabilitySelfCheck) {
	r.capabilityCheck = c
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
	// worktreeDiffErr is set only when we positively know a worktree/base-commit exists
	// for this session but the diff still couldn't be computed (e.g. a stale/corrupted
	// base_commit_sha pointing at a pruned or otherwise nonexistent git object). That is
	// an infrastructure failure, not "no changes were made" — see the block below that
	// blocks the review instead of silently handing the reviewer an empty diff.
	var worktreeDiffErr error
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
			// item.RepoPath's own checked-out HEAD is not the work branch's tip, so an
			// explicit branch ref is required here — implicit HEAD would diff against
			// whatever the shared main checkout happens to have, not the agent's work.
			diff, truncated, diffErr = GetGitDiffRef(ctx, item.RepoPath, wt.BaseCommitSHA, wt.BranchName)
			if diffErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff (repo fallback) item=%s: %v", item.ID, diffErr)
				worktreeDiffErr = diffErr
			}
		}
	} else {
		var diffErr error
		diff, truncated, diffErr = GetGitDiff(ctx, item.RepoPath, is.LastCommitSha)
		if diffErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff item=%s: %v", item.ID, diffErr)
		}
	}
	// Auto-repair: a broken base_commit_sha (stale/corrupted/garbage-collected — the
	// exact failure found via manual QA on item ae1e2070) is recoverable in the common
	// case, because the branch itself is still reachable from repoPath's object store
	// even when the recorded SHA is not. Recompute the merge-base and retry once before
	// giving up; this lets a genuinely-complete review proceed on a real diff instead of
	// unconditionally blocking (or worse, silently returning an empty one).
	if worktreeDiffErr != nil && wt.BranchName != "" {
		if recoveredSHA, recoverErr := RecoverBaseCommitSHA(ctx, item.RepoPath, wt.BranchName); recoverErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate RecoverBaseCommitSHA item=%s: %v", item.ID, recoverErr)
		} else if recoveredDiff, recoveredTruncated, retryErr := GetGitDiffRef(ctx, item.RepoPath, recoveredSHA, wt.BranchName); retryErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate retry with recovered base %s item=%s: %v", recoveredSHA, item.ID, retryErr)
		} else if strings.TrimSpace(recoveredDiff) == "" {
			// A recovered base that produces an empty diff is indistinguishable from
			// "nothing changed" and just as unsafe to hand the reviewer as the original
			// failure — e.g. when the recovered merge-base collapses to headRef itself
			// (no divergence from repoPath's checked-out branch). Do not treat this as a
			// successful repair; fall through to the explicit-block path below instead of
			// silently manufacturing a misleading empty-but-"valid" diff.
			log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate recovered base %s item=%s produced an empty diff — not trusting it, falling through to block", recoveredSHA, item.ID)
		} else {
			log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate auto-repaired broken base_commit_sha item=%s recovered=%s (recorded=%s)", item.ID, recoveredSHA, wt.BaseCommitSHA)
			diff, truncated = recoveredDiff, recoveredTruncated
			worktreeDiffErr = nil
			if r.getNotifier != nil {
				if n := r.getNotifier(); n != nil {
					n.Notify(item.ID,
						"Review auto-repaired a broken diff",
						fmt.Sprintf("%s — the recorded base commit was missing/corrupted; recomputed it from the branch and continued the review normally. The stored value should still be corrected so this doesn't repeat every run.", item.Title),
						8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
						2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
					)
				}
			}
		}
	}
	if uncommittedWarning != "" {
		diff = uncommittedWarning + diff
	}

	// A worktree/base-commit was recorded for this session but the diff could not be
	// computed against it even after the repo-fallback attempt AND the auto-repair
	// retry above — most likely the branch itself is gone too, or repoPath has no
	// merge-base with it. Proceeding here would hand the reviewer an empty diff
	// indistinguishable from "no changes made," producing a false UNVERIFIABLE/FAIL
	// verdict that masks the real (infrastructure) problem and, via the auto-reopen
	// loop, can spin forever without ever fixing the underlying cause. Block the review
	// with a verdict that says so explicitly instead.
	if worktreeDiffErr != nil {
		summary := fmt.Sprintf("Review blocked: could not compute a diff for this session (%v). "+
			"The recorded base commit may be missing or corrupted — this needs investigation, not rework.", worktreeDiffErr)
		diffFailIS, createErr := r.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "diff-error-" + uuid.New().String(),
			SessionRole: SessionRoleReview,
			AcSnapshot:  is.AcSnapshot,
		}, ReviewVerdictData{
			OverallOutcome: ReviewVerdictFail,
			Summary:        summary,
		})
		if createErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (diff error) item=%s: %v", item.ID, createErr)
			return
		}
		if updateErr := r.storage.UpdateItemSessionEnded(ctx, diffFailIS.ID, time.Now()); updateErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate UpdateItemSessionEnded (diff error) item=%s: %v", item.ID, updateErr)
		}
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate diff computation failed for item %s — review blocked, FAIL verdict recorded", item.ID)
		if r.getNotifier != nil {
			if n := r.getNotifier(); n != nil {
				n.Notify(item.ID,
					"Review blocked — diff computation failed",
					fmt.Sprintf("%s — recorded base commit may be missing or corrupted. Needs investigation.", item.Title),
					7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
					3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
				)
			}
		}
		// Feed into the same auto-reopen/cap-and-notify machinery used for real FAIL
		// verdicts, so a persistently broken worktree still surfaces to a human via
		// notifyReworkCapHit after maxAutoReworkIterations instead of looping silently.
		if reopener := r.getAutoReopener(); reopener != nil {
			go func() {
				if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
					log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (diff error) item=%s: %v", item.ID, err)
				}
			}()
		}
		return
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
		if r.getNotifier != nil {
			if n := r.getNotifier(); n != nil {
				n.Notify(item.ID,
					"Review blocked by security check",
					fmt.Sprintf("%s — override required to proceed.", item.Title),
					7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
					3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
				)
			}
		}
		return
	}

	// Deserialize AC snapshot, overlaying any live Note/Status written by
	// report_progress after the ItemSession's AcSnapshot was captured at spawn
	// time — otherwise the reviewer sees a stale snapshot missing self-reported
	// progress notes. See MergeLiveCriterionNotes.
	acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
	liveAC, _ := ParseAcCriteria(item.AcceptanceCriteria)
	if len(acSnapshot) == 0 {
		acSnapshot = liveAC
	} else {
		acSnapshot = MergeLiveCriterionNotes(acSnapshot, liveAC)
	}

	prompt := BuildReviewPrompt(item, acSnapshot, diff, truncated, is.ID, is.VerificationNotes)

	pool := r.getPool()
	if pool != nil {
		// Headless path: call LLM directly without spawning a tmux session.
		// codebaseWorkDir prefers the session's dedicated worktree (freshest, matches
		// the diff/PR branch); falls back to the item's shared repo checkout when no
		// worktree is recorded for this session (directory-mode sessions).
		codebaseWorkDir := wt.WorktreePath
		if codebaseWorkDir == "" {
			codebaseWorkDir = item.RepoPath
		}
		systemPrompt, callOpts, callTimeout, reviewPath := BuildReviewCallOptions(diff, codebaseWorkDir)
		// callStart is recorded immediately before the headless call sequence
		// (capability self-check, then CallBlocking) so Epic 2.5's duration_ms=
		// observability logging reflects the real cost of this review attempt,
		// including a first-in-process capability self-check when one runs.
		callStart := time.Now()

		// Story 2.2.6b: before the FIRST real codebase-read call in this process's
		// lifetime, verify the claude CLI/config actually grants WorkDir+AllowedTools+
		// PermissionMode read access — the same empirical fact
		// TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess checks in CI. A
		// failure here means every AllowedTools/PermissionMode-bearing call would
		// silently produce zero real evidence, so skip the real call entirely and
		// record UNVERIFIABLE directly — mirrors the codebase-read-timeout branch's
		// shape below (same cleanupCtx pattern, same auto-reopen wiring).
		if reviewPath == "codebase-read" && !r.capabilityCheck.Ensure(ctx, pool) {
			reviewPath = "codebase-read-degraded"
			summary := "Review UNVERIFIABLE: codebase-read capability self-check failed — this process's claude CLI/config does not appear to grant WorkDir+AllowedTools+PermissionMode read access, so no real codebase-read call was attempted."
			capIS, createErr := RecordDegradedReviewVerdict(r.storage, item.ID, is.AcSnapshot, headlessReviewUUIDPrefix, summary)
			if createErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate RecordDegradedReviewVerdict (capability self-check failed) item=%s: %v", item.ID, createErr)
				return
			}
			log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s, path=%s, duration_ms=%d)", item.ID, capIS.ID, ReviewVerdictUnverifiable, reviewPath, time.Since(callStart).Milliseconds())
			if reopener := r.getAutoReopener(); reopener != nil {
				go func() {
					if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
						log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (capability self-check failed) item=%s: %v", item.ID, err)
					}
				}()
			}
			return
		}

		reviewCtx, reviewCancel := context.WithTimeout(ctx, callTimeout)
		defer reviewCancel()

		headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated, is.VerificationNotes)
		reviewResult, callCostUSD, callErr := pool.CallBlocking(reviewCtx, headless.FeatureKeyReview, systemPrompt, headlessPrompt, callOpts)
		if callErr != nil {
			// Story 2.2.4b: a timeout on the codebase-read path is an infrastructure
			// signal (hung/degraded tool access), not evidence the criteria failed —
			// degrade to UNVERIFIABLE instead of taking the normal FAIL path below.
			if reviewPath == "codebase-read" && (errors.Is(reviewCtx.Err(), context.DeadlineExceeded) || errors.Is(reviewCtx.Err(), context.Canceled)) {
				// A parent-context cancellation (e.g. process shutdown mid-call) is an
				// infrastructure signal just like a deadline — ADR-001's rationale for
				// degrading to UNVERIFIABLE rather than FAIL applies equally to both.
				reviewPath = "codebase-read-degraded"
				summary := fmt.Sprintf("Review UNVERIFIABLE: codebase-read call timed out or was cancelled after %s (%v) — could not independently verify criteria against the codebase.", callTimeout, reviewCtx.Err())
				timeoutIS, createErr := RecordDegradedReviewVerdict(r.storage, item.ID, is.AcSnapshot, headlessReviewUUIDPrefix, summary)
				if createErr != nil {
					log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate RecordDegradedReviewVerdict (codebase-read timeout) item=%s: %v", item.ID, createErr)
					return
				}
				log.WarningLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s, path=%s, duration_ms=%d)", item.ID, timeoutIS.ID, ReviewVerdictUnverifiable, reviewPath, time.Since(callStart).Milliseconds())
				if reopener := r.getAutoReopener(); reopener != nil {
					go func() {
						if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
							log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (codebase-read timeout) item=%s: %v", item.ID, err)
						}
					}()
				}
				return
			}
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate headless.CallBlocking item=%s: %v", item.ID, callErr)
			// Record a FAIL verdict so the item is not stuck in review with no actionable result.
			// cleanupCtx is a separate, freshly-derived context (not ctx, which may itself be
			// close to its own deadline/cancellation — e.g. exactly the case where callErr came
			// back as a context error) so this write succeeds even then — same rationale as the
			// codebase-read timeout branch above.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			failUUID := headlessReviewUUIDPrefix + uuid.New().String()
			failIS, createErr := r.storage.CreateItemSessionWithVerdict(cleanupCtx, ItemSessionData{
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
			} else if updateErr := r.storage.UpdateItemSessionEnded(cleanupCtx, failIS.ID, time.Now()); updateErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate UpdateItemSessionEnded (headless fail) item=%s: %v", item.ID, updateErr)
			}
			return
		}

		overall, perCriterion, summary := ParseHeadlessVerdictResult(reviewResult)
		toolReads := ParseHeadlessToolReads(reviewResult)
		overall, perCriterion, summary, reviewPath = DegradeIfUnverified(reviewPath, overall, perCriterion, summary, toolReads, codebaseWorkDir)
		// reviewPath now carries the final path label ("diff", "codebase-read-verified",
		// or "codebase-read-degraded"), logged below via Epic 2.5's path=/duration_ms=
		// observability fields.
		perCriterionJSON, _ := json.Marshal(perCriterion)

		// Update AC statuses on the item to reflect what was verified.
		applyVerdictsToACs(ctx, r.storage, item, acSnapshot, perCriterion)

		// Create a synthetic ItemSession and its ReviewVerdict atomically so there
		// is never a dangling session with no verdict if the verdict write fails.
		reviewSessionUUID := headlessReviewUUIDPrefix + uuid.New().String()
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

		log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s, path=%s, duration_ms=%d)", item.ID, reviewIS.ID, overall, reviewPath, time.Since(callStart).Milliseconds())

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
