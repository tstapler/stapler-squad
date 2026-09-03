package session

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
)

// ReviewGateRunner encapsulates the spawnReviewGate logic into a testable value type.
// BacklogLifecycleListener holds one as a field and delegates to it.
//
// getAutoReopener, getNotifier, and getSessionCreator are getter functions rather
// than stored values so that the runner always observes the latest
// reopener/notifier/spawner even when SetAutoReopener / SetNotifier /
// SetSessionCreator are called after construction.
type ReviewGateRunner struct {
	storage           *Storage
	getAutoReopener   func() AutoReopenSpawner
	getNotifier       func() Notifier
	getSessionCreator func() ReviewGateSpawner

	// pipelineEngine resolves a custom PipelineMode's ReviewPromptTemplate for
	// the real review session prompt built in Run. May be nil (many tests and
	// some constructors predate PipelineEngine wiring) — reviewPromptFor falls
	// back to BuildReviewPrompt directly in that case, matching
	// BacklogService.reviewPromptFor's identical nil-safe pattern
	// (server/services/backlog_service_triage.go). Set once at construction,
	// never mutated afterward — mirrors BacklogLifecycleListener.pipelineEngine.
	pipelineEngine PipelineEngine
}

// NewReviewGateRunner constructs a ReviewGateRunner.
// getAutoReopener, getNotifier, and getSessionCreator are getter functions
// (typically method values from BacklogLifecycleListener) so the runner sees
// the latest values when dynamic setters are called after construction.
// pipelineEngine may be nil — see the field's doc comment for the fallback.
func NewReviewGateRunner(
	storage *Storage,
	getAutoReopener func() AutoReopenSpawner,
	getNotifier func() Notifier,
	getSessionCreator func() ReviewGateSpawner,
	pipelineEngine PipelineEngine,
) *ReviewGateRunner {
	return &ReviewGateRunner{
		storage:           storage,
		getAutoReopener:   getAutoReopener,
		getNotifier:       getNotifier,
		getSessionCreator: getSessionCreator,
		pipelineEngine:    pipelineEngine,
	}
}

// parseDuplicateRef reports whether notes — VerificationNotes as written by
// report_duplicate (server/mcp/tools_backlog.go), entries joined by
// "\n\n---\n\n" — ends in a "duplicate_ref=<ref> reason=<reason>" marker for the
// CURRENT attempt, and if so returns its ref.
//
// Only the last entry is inspected, never an earlier one. report_duplicate always
// appends its marker as the newest entry, so a duplicate claim's marker is always
// last; conversely, if the latest entry is something else (e.g. a later work
// session's request_review notes from a subsequent rework cycle), an older
// duplicate_ref= entry further back is stale and must not be resurrected — reading
// past it would treat that later, unrelated empty-diff session as a duplicate claim
// too and route it around the guard below — exactly the false-PASS regression this
// fix (backlog item e2373931) must not reintroduce.
func parseDuplicateRef(notes string) (ref string, ok bool) {
	const marker = "duplicate_ref="
	entries := strings.Split(notes, "\n\n---\n\n")
	last := strings.TrimSpace(entries[len(entries)-1])
	if !strings.HasPrefix(last, marker) {
		return "", false
	}
	rest := strings.TrimPrefix(last, marker)
	if idx := strings.Index(rest, " "); idx >= 0 {
		rest = rest[:idx]
	}
	rest = strings.TrimSpace(rest)
	return rest, rest != ""
}

// reviewPromptFor returns r.pipelineEngine.InteractiveReviewPromptFor(...) when
// pipelineEngine is wired, or the default BuildReviewPrompt otherwise — mirrors
// BacklogService.reviewPromptFor's identical nil-safe fallback pattern
// (server/services/backlog_service_triage.go) for the headless-review seam.
func (r *ReviewGateRunner) reviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, itemSessionID string, verificationNotes string) string {
	if r.pipelineEngine == nil {
		return BuildReviewPrompt(item, acSnapshot, diff, diffTruncated, itemSessionID, verificationNotes)
	}
	return r.pipelineEngine.InteractiveReviewPromptFor(item, acSnapshot, diff, diffTruncated, itemSessionID, verificationNotes)
}

// Run executes the review gate for a backlog item session.
// ctx should be the listener's shutdownCtx so long-running calls are cancelled
// on shutdown.
// onPass is retained for signature compatibility with existing callers
// (BacklogLifecycleListener.pushAndCreatePR) but is no longer invoked directly
// from Run: since review now always happens in a real, hidden session.Instance,
// the PASS/FAIL/PARTIAL/UNVERIFIABLE outcome is only known once that session
// exits and calls submit_review_verdict — handled by
// BacklogLifecycleListener.handleReviewSessionExited, not here.
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
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate item=%s has no repo path set; skipping review gate", item.ID)
		return
	}

	log.InfoLog().Printf("[PipelineEngine] item=%s stage=review mode=%q", item.ID, ResolvedModeLabel(item.PipelineMode))

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
		// Worktree-identity guard: if a directory still sits at the recorded worktree
		// path, confirm it actually has this session's own branch checked out before
		// touching it at all — including the branch-drift sync just below, which would
		// otherwise run against whatever unrelated repo/branch happens to be at that
		// path. (A missing directory is not itself a mismatch — see
		// WorktreeIdentityMismatch's doc comment for why.) Gated only on WorktreePath,
		// not also requiring BranchName: a row with a path but no recorded branch must
		// still go through this check (it reports "unverifiable" rather than being
		// silently skipped) — the branch-drift block just below assumes wt.BranchName
		// is meaningful and must never be reached with it empty. Worktree identity is
		// resolved by title-derived branch name, not item/session UUID
		// (findExistingWorktreeForBranch, session/git/worktree.go), so two items with
		// colliding sanitized titles can silently be handed the same worktree — the
		// "diff computed against the wrong worktree" failure class this guards against
		// (backlog item e7664cbf). Fail closed with a distinct verdict instead of
		// diffing (or auto-repairing against) whatever actually turns out to be there.
		if mismatchReason := WorktreeIdentityMismatch(wt.WorktreePath, wt.BranchName); mismatchReason != "" {
			summary := fmt.Sprintf("Review blocked: this session's recorded worktree (%s) %s. "+
				"The worktree path may have been reused or recreated for a different item — this needs investigation, not rework.",
				wt.WorktreePath, mismatchReason)
			log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate worktree identity mismatch item=%s worktree=%s branch=%s: %s", item.ID, wt.WorktreePath, wt.BranchName, mismatchReason)
			mismatchIS, createErr := recordTerminalReviewVerdict(r.storage, item.ID, is.AcSnapshot, "worktree-identity-"+uuid.New().String(), ReviewVerdictFail, summary)
			if createErr != nil {
				log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (worktree identity) item=%s: %v", item.ID, createErr)
				return
			}
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate worktree identity mismatch blocked review for item %s — FAIL verdict recorded (session %s)", item.ID, mismatchIS.ID)
			if r.getNotifier != nil {
				if n := r.getNotifier(); n != nil {
					n.Notify(item.ID,
						"Review blocked — worktree identity mismatch",
						fmt.Sprintf("%s — the session's recorded worktree %s. See the item's review history for details.", item.Title, mismatchReason),
						7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
						3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
					)
				}
			}
			// Feed into the same auto-reopen/cap-and-notify machinery every other terminal
			// block in this function uses — a recycled/misattributed worktree left by
			// worktree-management flakiness is exactly the kind of thing a rework session's
			// respawn can resolve, and the rework cap still protects against a persistently
			// broken case looping silently forever.
			if reopener := r.getAutoReopener(); reopener != nil {
				go func() {
					if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
						log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (worktree identity) item=%s: %v", item.ID, err)
					}
				}()
			}
			return
		}
		// Precondition of review, not a best-effort side effect of the reactive PR-fix
		// path (BUG-044): a branch left to drift unbounded from main eventually produces
		// a diff dominated by unrelated upstream commits rather than the item's own
		// work, which review then — correctly, given what it's shown — reports as
		// unrelated, misdiagnosing branch staleness as bad work (backlog item 693c2700).
		// Checked/synced here, before any diff is computed, so a clean auto-sync never
		// even reaches the reviewer, and a real conflict blocks with an explicit,
		// actionable reason instead of silently producing a misleading diff.
		if ok, blockedSummary := git.EnsureBranchSyncedWithMain(wt.WorktreePath, wt.BranchName, bounceMainBranch, git.DefaultBranchDriftThreshold); !ok {
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate branch drift blocked review item=%s branch=%s: %s", item.ID, wt.BranchName, blockedSummary)
			driftIS, createErr := recordTerminalReviewVerdict(r.storage, item.ID, is.AcSnapshot, "branch-drift-"+uuid.New().String(), ReviewVerdictFail, blockedSummary)
			if createErr != nil {
				log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (branch drift) item=%s: %v", item.ID, createErr)
				return
			}
			log.InfoLog().Printf("[BacklogLifecycle] spawnReviewGate branch drift blocked for item %s — FAIL verdict recorded (session %s)", item.ID, driftIS.ID)
			if r.getNotifier != nil {
				if n := r.getNotifier(); n != nil {
					n.Notify(item.ID,
						"Review blocked — branch drifted too far behind main",
						fmt.Sprintf("%s — the branch could not be automatically synced with main. See the item's review history for the conflict details.", item.Title),
						7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
						3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
					)
				}
			}
			// Feed into the same auto-reopen/cap-and-notify machinery every other terminal
			// block in this function uses — a conflict left by drift is exactly the kind of
			// thing a rework session can resolve, and the rework cap still protects against
			// an unresolvable case looping silently forever.
			if reopener := r.getAutoReopener(); reopener != nil {
				go func() {
					if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
						log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (branch drift) item=%s: %v", item.ID, err)
					}
				}()
			}
			return
		}
	}
	if wtErr == nil && wt.WorktreePath != "" {
		// Belt-and-suspenders layer 2: warn if the worktree still has uncommitted changes.
		// request_review (layer 1) should have caught this, but flag it here too so the
		// reviewer prompt is aware and the verdict reflects the incomplete state.
		if dirty, dirtyErr := IsWorktreeDirty(ctx, wt.WorktreePath); dirtyErr == nil && dirty {
			log.InfoLog().Printf("[BacklogLifecycle] review gate: item=%s has uncommitted changes in worktree — diff will be incomplete", item.ID)
			uncommittedWarning = "[WARNING: worktree has uncommitted changes — the following diff may be incomplete]\n"
		}
		var diffErr error
		diff, truncated, diffErr = GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
		if diffErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff (worktree) item=%s: %v; falling back to repo", item.ID, diffErr)
			// item.RepoPath's own checked-out HEAD is not the work branch's tip, so an
			// explicit branch ref is required here — implicit HEAD would diff against
			// whatever the shared main checkout happens to have, not the agent's work.
			diff, truncated, diffErr = GetGitDiffRef(ctx, item.RepoPath, wt.BaseCommitSHA, wt.BranchName)
			if diffErr != nil {
				log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff (repo fallback) item=%s: %v", item.ID, diffErr)
				worktreeDiffErr = diffErr
			}
		}
	} else {
		var diffErr error
		// Directory-mode session (no worktree row): the diff BASE is the session's
		// spawn-time HEAD. Read BaseCommitSha, falling back to LastCommitSha only
		// for rows written before the two were split — on those legacy rows the
		// bug being fixed meant both held the same base value. Using
		// LastCommitSha unconditionally here would now diff the session's tip
		// against itself and always produce an empty review diff, since that
		// field is live-refreshed (BUG-047).
		diffBase := is.BaseCommitSha
		if diffBase == "" {
			diffBase = is.LastCommitSha
		}
		diff, truncated, diffErr = GetGitDiff(ctx, item.RepoPath, diffBase)
		if diffErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff item=%s: %v", item.ID, diffErr)
			worktreeDiffErr = diffErr
		}
	}
	// Auto-repair: a broken base_commit_sha (stale/corrupted/garbage-collected — the
	// exact failure found via manual QA on item ae1e2070) is recoverable in the common
	// case, because the branch itself is still reachable from repoPath's object store
	// even when the recorded SHA is not. Recompute the merge-base and retry once before
	// giving up; this lets a genuinely-complete review proceed on a real diff instead of
	// unconditionally blocking (or worse, silently returning an empty one).
	if worktreeDiffErr != nil && wt.BranchName != "" && wt.WorktreePath != "" {
		// Recover against whichever directory is actually still on disk: prefer the
		// session's own worktree, but fall back to item.RepoPath when it's been torn
		// down — mirrors getWorkSessionDiff's identical recoverDir fallback
		// (server/services/backlog_service_triage.go). Without this, a torn-down
		// worktree combined with a corrupted/stale base_commit_sha made
		// RecoverBaseCommitSHA run `git merge-base` with cmd.Dir pointed at a
		// nonexistent directory — the command never even starts, so recovery is
		// abandoned and the review is wrongly hard-blocked, even though the branch's
		// commits remain reachable via the shared object store at item.RepoPath.
		recoverDir := item.RepoPath
		if info, statErr := os.Stat(wt.WorktreePath); statErr == nil && info.IsDir() {
			recoverDir = wt.WorktreePath
		}
		if recoveredSHA, recoverErr := RecoverBaseCommitSHA(ctx, recoverDir, wt.BranchName); recoverErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate RecoverBaseCommitSHA item=%s: %v", item.ID, recoverErr)
		} else if recoveredDiff, recoveredTruncated, retryErr := GetGitDiffRef(ctx, recoverDir, recoveredSHA, wt.BranchName); retryErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate retry with recovered base %s item=%s: %v", recoveredSHA, item.ID, retryErr)
		} else if strings.TrimSpace(recoveredDiff) == "" {
			// A recovered base that produces an empty diff is indistinguishable from
			// "nothing changed" and just as unsafe to hand the reviewer as the original
			// failure — e.g. when the recovered merge-base collapses to headRef itself
			// (no divergence from repoPath's checked-out branch). Do not treat this as a
			// successful repair; fall through to the explicit-block path below instead of
			// silently manufacturing a misleading empty-but-"valid" diff.
			log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate recovered base %s item=%s produced an empty diff — not trusting it, falling through to block", recoveredSHA, item.ID)
		} else {
			log.InfoLog().Printf("[BacklogLifecycle] spawnReviewGate auto-repaired broken base_commit_sha item=%s recovered=%s (recorded=%s)", item.ID, recoveredSHA, wt.BaseCommitSHA)
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
	// Captured before uncommittedWarning is prepended below, so this reflects only
	// the actual committed diff rather than the warning banner text.
	committedDiffEmpty := worktreeDiffErr == nil && strings.TrimSpace(diff) == ""

	if uncommittedWarning != "" {
		diff = uncommittedWarning + diff
	}

	// An empty committed diff is indistinguishable from "no real work happened" — the
	// same failure shape as the worktreeDiffErr case below, but for a diff that
	// computed successfully rather than one that errored. Without this check, a
	// no-op session (crashed before committing, or promoted to review by
	// ReconcileStuckItems purely because its work sessions ended, with zero commits)
	// would reach a real review session with nothing to review, risking a false
	// PASS/UNVERIFIABLE verdict that marks the item done despite no work having
	// shipped — see BUG-047/BUG-065 in backlog_lifecycle.go and
	// backlog_lifecycle_pr.go for the same class of bug in sibling reconciliation
	// paths. Blocking here, with the same FAIL-verdict + auto-reopen handling used
	// for every other guardrail in this function, routes a no-work item back
	// through AutoReopenAfterFailedReview to in_progress instead of letting it
	// silently pass review.
	//
	// Exception: report_duplicate (server/mcp/tools_backlog.go) deliberately routes an
	// item to review with zero committed diff — a duplicate claim has nothing to
	// commit by design — and writes a duplicate_ref= marker into VerificationNotes
	// before triggering this gate specifically so a real reviewer can confirm it
	// (backlog item e2373931). Without this carve-out, every report_duplicate call hit
	// this guard's hardcoded FAIL, and two calls in a row tripped IsRepeatedFailure and
	// permanently parked the item in review with no reviewer ever consulted. Detecting
	// the marker here and falling through to the normal SpawnReviewSession path below
	// is what fixes that: the reviewer's prompt already carries the duplicate_ref line
	// (writeVerificationEvidenceSection in backlog_review.go), and BuildReviewPrompt's
	// diff=="" branch already tells the reviewer to check the codebase itself. A
	// session with no duplicate_ref marker — the genuinely abandoned case this guard
	// exists for — is unaffected: isDup is false and the FAIL below still fires
	// exactly as before.
	_, isDup := parseDuplicateRef(is.VerificationNotes)
	if committedDiffEmpty && !isDup {
		summary := "Review blocked: no committed changes were found for this session. " +
			"There is nothing to review — the work session ended without shipping any commits."
		emptyDiffIS, createErr := recordTerminalReviewVerdict(r.storage, item.ID, is.AcSnapshot, "empty-diff-"+uuid.New().String(), ReviewVerdictFail, summary)
		if createErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (empty diff) item=%s: %v", item.ID, createErr)
			return
		}
		log.WarningLog().Printf("[BacklogLifecycle] spawnReviewGate empty diff for item %s — FAIL verdict recorded (session %s)", item.ID, emptyDiffIS.ID)
		if r.getNotifier != nil {
			if n := r.getNotifier(); n != nil {
				n.Notify(item.ID,
					"Review blocked — no changes to review",
					fmt.Sprintf("%s — the work session ended without any committed changes.", item.Title),
					7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
					3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
				)
			}
		}
		if reopener := r.getAutoReopener(); reopener != nil {
			go func() {
				if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
					log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (empty diff) item=%s: %v", item.ID, err)
				}
			}()
		}
		return
	}

	// A worktree/base-commit was recorded for this session but the diff could not be
	// computed against it even after the repo-fallback attempt AND the auto-repair
	// retry above — most likely the branch itself is gone too, or repoPath has no
	// merge-base with it. Proceeding here would hand the reviewer an empty diff
	// indistinguishable from "no changes made," producing a false UNVERIFIABLE/FAIL
	// verdict that masks the real (infrastructure) problem and, via the auto-reopen
	// loop, can spin forever without ever fixing the underlying cause. Block the review
	// with a verdict that says so explicitly instead.
	//
	// This is a synthetic, non-Instance-backed terminal verdict — a pre-flight
	// guardrail, not the review call itself — so it is recorded directly here rather
	// than via a spawned review session, same as before this file switched to
	// always spawning a real session for the actual review call.
	if worktreeDiffErr != nil {
		summary := fmt.Sprintf("Review blocked: could not compute a diff for this session (%v). "+
			"The recorded base commit may be missing or corrupted — this needs investigation, not rework.", worktreeDiffErr)
		diffFailIS, createErr := recordTerminalReviewVerdict(r.storage, item.ID, is.AcSnapshot, "diff-error-"+uuid.New().String(), ReviewVerdictFail, summary)
		if createErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (diff error) item=%s: %v", item.ID, createErr)
			return
		}
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate diff computation failed for item %s — review blocked, FAIL verdict recorded (session %s)", item.ID, diffFailIS.ID)
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
					log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (diff error) item=%s: %v", item.ID, err)
				}
			}()
		}
		return
	}

	// Security check — block if secrets detected.
	//
	// Same as the diff-error block above: this records a synthetic, non-Instance-backed
	// terminal verdict directly — it's a pre-flight guardrail, not the review call itself.
	if secErr := RunPreGateSecurityCheck(diff); secErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate security check blocked item=%s: %v", item.ID, secErr)
		// Record a failed review ItemSession with a FAIL verdict so the gate verdict
		// is visible in the UI and operators can act (override or re-review).
		summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)
		secIS, secCreateErr := recordTerminalReviewVerdict(r.storage, item.ID, is.AcSnapshot, "review-blocked-"+uuid.New().String(), ReviewVerdictFail, summary)
		if secCreateErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (security block) item=%s: %v", item.ID, secCreateErr)
			return
		}
		log.InfoLog().Printf("[BacklogLifecycle] spawnReviewGate security check blocked for item %s — FAIL verdict recorded (session %s)", item.ID, secIS.ID)
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
		// Feed into the same auto-reopen/cap-and-notify machinery every other
		// terminal FAIL/UNVERIFIABLE verdict block in this function uses (see the
		// diff-computation-blocked block above) — a secret left behind by a rework
		// session is exactly the kind of thing a subsequent rework attempt can fix,
		// and the maxAutoReworkIterations cap still protects against an unfixable
		// case looping silently forever.
		if reopener := r.getAutoReopener(); reopener != nil {
			go func() {
				if err := reopener.AutoReopenAfterFailedReview(ctx, item.ID); err != nil {
					log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate AutoReopenAfterFailedReview (security block) item=%s: %v", item.ID, err)
				}
			}()
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

	prompt := r.reviewPromptFor(item, acSnapshot, diff, truncated, is.ID, is.VerificationNotes)

	// Spawn a real, hidden, tagged review session.Instance so the review
	// participates in the same visibility/attention mechanism (idle/error/approval
	// detection) as every other session — see SpawnReviewSession. The verdict is
	// no longer computed synchronously here: the spawned session calls the
	// submit_review_verdict MCP tool, and BacklogLifecycleListener.
	// handleReviewSessionExited processes the outcome once that session exits.
	//
	// Note: prompt is built via reviewPromptFor, which routes through
	// PipelineEngine.InteractiveReviewPromptFor (tool-call/submit_review_verdict
	// style) rather than ReviewPromptFor/BuildHeadlessReviewPrompt. The latter
	// pair asks for a bare JSON object on stdout — correct for the still-headless
	// callers that use it (TriggerReReview in
	// server/services/backlog_service_triage.go), but wrong here: this session is
	// a real, tool-using Claude Code agent that must call the submit_review_verdict
	// MCP tool, not print JSON, so handleReviewSessionExited has a verdict to read
	// once it exits. PipelineModeDefault (or a nil pipelineEngine) still renders
	// BuildReviewPrompt directly, so this is behavior-preserving for every item
	// that hasn't opted into a custom PipelineMode.
	sessionCreator := r.getSessionCreator()
	if sessionCreator == nil {
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate item=%s: no review mechanism configured", item.ID)
		return
	}

	reviewInst, spawnErr := sessionCreator.SpawnReviewSession(ctx, item, is.ID, prompt)
	if spawnErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate SpawnReviewSession item=%s: %v", item.ID, spawnErr)
		return
	}

	// Create ItemSession linking the new review session to the backlog item.
	if _, createErr := r.storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewInst.UUID,
		SessionRole: SessionRoleReview,
		AcSnapshot:  is.AcSnapshot,
	}); createErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] spawnReviewGate CreateItemSession item=%s review=%s: %v", item.ID, reviewInst.UUID, createErr)
		return
	}

	log.InfoLog().Printf("[BacklogLifecycle] spawnReviewGate spawned review session %s for item %s", reviewInst.UUID, item.ID)
}

// WorktreeIdentityMismatch reports why worktreePath does not actually belong to
// branchName, or "" if it does — or if worktreePath doesn't exist on disk at all.
// A missing directory is deliberately NOT reported as a mismatch here: it's the
// same "worktree torn down, but its commits remain reachable via the shared
// object store" case the diff-recovery fallbacks in this package and
// server/services/backlog_service_triage.go's getWorkSessionDiff already handle
// gracefully — this check exists to catch a *present* directory that's actually
// the wrong one, not to duplicate or preempt that existing recovery path.
// Worktree paths are resolved by title-derived branch name rather than
// item/session UUID (findExistingWorktreeForBranch, session/git/worktree.go), so
// a recorded worktree row can point at a path that's since been reused,
// recreated, or handed to a different item while a directory still sits there.
// An empty branchName can never match a real checked-out branch, so it's always
// reported as unverifiable rather than silently skipping the check — a legacy
// row with a path but no recorded branch is exactly the case this guard exists
// to be cautious about, not a case to wave through. Exported: called from
// server/services' TriggerReReview diff/codebase-read paths too, which need the
// identical guard against the same title-collision hazard.
func WorktreeIdentityMismatch(worktreePath, branchName string) string {
	info, statErr := os.Stat(worktreePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return ""
		}
		return fmt.Sprintf("could not be verified: %v", statErr)
	}
	if !info.IsDir() {
		return "exists on disk but is not a directory"
	}
	if branchName == "" {
		return "has no recorded branch name to verify against"
	}
	actualBranch, branchErr := git.GetCurrentBranchName(worktreePath)
	if branchErr != nil {
		return fmt.Sprintf("could not be verified: %v", branchErr)
	}
	if actualBranch != branchName {
		return fmt.Sprintf("is checked out to %q, not the expected %q", actualBranch, branchName)
	}
	return ""
}
