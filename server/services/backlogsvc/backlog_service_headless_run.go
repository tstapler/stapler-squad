package backlogsvc

// backlog_service_headless_run.go — the headless re-review run (TriggerReReview) and the
// call-error helpers shared by the headless triage and re-review call sites. Moved out of
// backlog_service_triage.go unchanged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// classifyHeadlessCallError buckets a headless.CallBlocking error into a coarse category
// for log-line grepping, so a future incident (e.g. a repeat of the 2026-07-24 stuck-triage
// investigation) can answer "how often does each failure mode happen" from log history alone,
// without re-deriving it by hand from raw error text and process timing.
//
//   - "pool_saturated": the call never got a shot at running — it waited for a
//     concurrency-pool slot until headless.ErrPoolSaturated's short queue-wait
//     cap elapsed (session/headless/caller.go). Checked before "timeout" so a
//     burst of concurrent calls doesn't look like a genuine LLM hang.
//   - "idle": the call started but produced no new stream-json output line for
//     headless.idleTimeout (session/headless/pool.go) — checked before
//     "timeout" so a genuinely stalled call isn't indistinguishable from a
//     legitimately long-but-active one.
//   - "fanout_ceiling": an sdd-mode call crossed its configured turn/subagent ceiling
//     (headless.ErrFanoutCeilingExceeded, ADR-029). Checked before "timeout" so an abort
//     near the budget tail keeps its distinct reason.
//   - "cost_ceiling": estimated spend crossed headless.CallOptions.MaxCostUSD/MaxTokens
//     (headless.ErrCostCeilingExceeded) — a busy-but-wasteful call, caught regardless
//     of elapsed time. Checked before "timeout" so it is never mislabeled as one.
//   - "timeout": ctx deadline exceeded, or elapsed is within 5s of budget (covers a
//     hang whose error got wrapped/lost before reaching context.DeadlineExceeded). With
//     idle detection now the primary defense against a truly stuck call, this bucket
//     should mostly mean "actively producing output for the full budget," not "hung."
//   - "shutdown": server shutdown context cancelled mid-call, not a call failure.
//   - "claude_not_found": the claude binary itself is missing from PATH — an environment
//     problem, not a per-call one.
//   - "subprocess_start_error": the claude subprocess itself failed to start (os.Pipe()
//     or cmd.Start() failing — fd exhaustion, ENOMEM, ENOENT, EACCES) — see
//     headless.ErrSubprocessStart. raw is always "" for this bucket: the failure happens
//     before any output is produced, so there is nothing for captureHeadlessFailure to
//     persist.
//   - "other": anything else, including the claude subprocess running and exiting
//     non-zero. There is deliberately no dedicated bucket for that today: the OS-level
//     exit code is captured in the process audit log (executor/audit.go) but is never
//     threaded back through ProcessRunner.Run/headless.Pool.call to the caller — the
//     only per-call "failure" signal callBlocking's first-call path actually inspects is
//     the CLI's own JSON is_error field, which is a distinct, app-level signal from the
//     OS exit code. (An earlier version of this classifier had a "process_error" bucket
//     matching headless.ErrLLMError/ErrUsageError/ErrInterrupted sentinels that no
//     production code path ever constructed — dead code, removed. Wiring up real
//     exit-code translation would require restructuring every error-return branch in
//     Pool.call to consult the subprocess's actual exit status before sending; if that's
//     ever worth doing, do it as its own change rather than reintroducing unused
//     sentinels.)
//
// budget is the caller's own call timeout (e.g. triageCallBudget for TriggerTriage,
// callTimeout for TriggerReReview) — the same classifier is shared across both headless
// call sites, so it takes budget as a parameter rather than hardcoding one.
//
// Callers MUST log the underlying error ("error", callErr) in the same structured log
// call as errType — the bucket alone is undiagnosable — and persist the ItemSession end
// via endItemSessionForCallError so "other" also records error_detail.
func classifyHeadlessCallError(err error, elapsed, budget time.Duration) string {
	switch {
	case errors.Is(err, headless.ErrPoolSaturated):
		return "pool_saturated"
	case errors.Is(err, headless.ErrIdleTimeout):
		return "idle"
	case errors.Is(err, headless.ErrFanoutCeilingExceeded):
		return backlog.TriageEndReasonFanoutCeiling
	case errors.Is(err, headless.ErrCostCeilingExceeded):
		return backlog.TriageEndReasonCostCeiling
	case errors.Is(err, context.DeadlineExceeded), budget-elapsed < 5*time.Second:
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "shutdown"
	case errors.Is(err, headless.ErrClaudeNotFound):
		return "claude_not_found"
	case errors.Is(err, headless.ErrSubprocessStart):
		return "subprocess_start_error"
	default:
		return backlog.TriageEndReasonOther
	}
}

// maxErrorDetailRunes bounds ItemSession.error_detail.
const maxErrorDetailRunes = 500

// truncateErrorDetail returns err.Error() cut to at most maxRunes runes (never splitting a
// multi-byte rune), or "" for a nil error.
func truncateErrorDetail(err error, maxRunes int) string {
	if err == nil {
		return ""
	}
	s := services.RedactErrorDetail(strings.Join(strings.Fields(err.Error()), " ")) // one line: it is embedded in a one-line stuck context
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// endItemSessionForCallError closes an ItemSession after a failed headless call,
// recording error_detail only for the unclassified "other" bucket. Best-effort.
func (s *BacklogService) endItemSessionForCallError(ctx context.Context, itemSessionID, errType string, callErr error) {
	var err error
	if errType == backlog.TriageEndReasonOther {
		err = s.storage.UpdateItemSessionEndedWithDetail(ctx, itemSessionID, time.Now(), errType, truncateErrorDetail(callErr, maxErrorDetailRunes))
	} else {
		err = s.storage.UpdateItemSessionEndedWithReason(ctx, itemSessionID, time.Now(), errType)
	}
	if err != nil {
		log.Warn("failed to record headless call failure on item session", "itemSession", itemSessionID, "errType", errType, "error", err)
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
		log.Warn("[captureHeadlessFailure] resolve capture dir failed", "error", dirErr)
		return ""
	}
	path, writeErr := session.WriteHeadlessFailureCapture(dir, sessionUUID, raw, session.DefaultHeadlessFailureCaptureMaxBytes)
	if writeErr != nil {
		log.Warn("[captureHeadlessFailure] write capture file failed", "session", sessionUUID, "error", writeErr)
		return ""
	}
	return path
}

// TriggerReReview re-runs the review gate for a backlog item.
// +api: backlog:trigger-re-review
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity relocated verbatim from backlog_service_triage.go; reducing it is a separate follow-up, not a file move
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
		if ent.IsNotFound(err) || errors.Is(err, domain.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Validate item is in review status.
	if item.Status != string(domain.BacklogStatusReview) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to re-trigger review, got %q", domain.BacklogStatusReview, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering re-review"))
	}

	// 3b. One in-flight review per item, shared with the lifecycle listener's
	// spawn paths. Held for the whole (synchronous) re-review.
	if s.reviewGuard != nil {
		release, ok := s.reviewGuard.TryReserve(item.ID, nil)
		if !ok {
			log.Info("[TriggerReReview] skipping: review already in flight", "item", item.ID)
			return nil, connect.NewError(connect.CodeAlreadyExists, errReviewInFlight)
		}
		defer release()
	}

	// 4. Find the most recent review and work ItemSessions for this item.
	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	mostRecentReviewSession, mostRecentWorkSession := services.FindMostRecentSessions(sessions)

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
				log.Warn("[TriggerReReview] branch drift blocked review", "item", item.ID, "branch", wt.BranchName, "summary", blockedSummary)
				is, createErr := pipeline.RecordDegradedReviewVerdict(s.storage, item.ID, domain.AcCriteriaJSON(acSnapshotJSON), services.HeadlessReReviewUUIDPrefix, blockedSummary)
				if createErr != nil {
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review branch-drift-blocked verdict: %w", createErr))
				}
				log.Info("[TriggerReReview] branch drift blocked — verdict recorded", "item", item.ID, "session", is.ID)
				if s.eventBus != nil {
					// itemID as sessionID — see comment in notifyReworkCapHit above.
					s.eventBus.Publish(events.NewNotificationEvent(
						item.ID, "", uuid.New().String(),
						int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
						services.DerivePriority(true, true), // urgent, important — needs investigation, not rework
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
	workSessionDiff, diffTruncated := s.getWorkSessionDiffTruncated(ctx, item.RepoPath, mostRecentWorkSession)

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

	attachedImages := pipeline.AttachedImagesSection(item.Description)
	if attachedImages != "" {
		attachedImages = "\n" + attachedImages
	}
	reReviewPrompt := fmt.Sprintf(`You are re-reviewing a backlog item that previously entered the review state.

# Item: %s

## Description
%s
%s%s
## Acceptance Criteria (at time of work session)
`, item.Title, item.Description, attachedImages, priorVerdictSection)

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
`, pipeline.SanitizeDiff(workSessionDiff), item.ID)

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
			is, createErr := pipeline.RecordDegradedReviewVerdict(s.storage, item.ID, domain.AcCriteriaJSON(acSnapshotJSON), services.HeadlessReReviewUUIDPrefix, blockedSummary)
			if createErr != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review blocked verdict: %w", createErr))
			}
			log.Error("[TriggerReReview] codebase-read work dir does not exist — review blocked, UNVERIFIABLE verdict recorded", "dir", codebaseWorkDir, "item", item.ID, "session", is.ID)
			if s.eventBus != nil {
				// itemID as sessionID — see comment in notifyReworkCapHit above.
				s.eventBus.Publish(events.NewNotificationEvent(
					item.ID, "", uuid.New().String(),
					int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
					services.DerivePriority(true, true), // urgent, important — needs investigation, not rework
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

		var extras pipeline.ReviewContextExtras
		if workSessionDiff == "" {
			// sessions was already loaded above (step 4) — reuse it rather than a second
			// ListItemSessions round trip.
			extras.PriorSessions = sessions
			if notes, notesErr := s.storage.ListProgressNotesForItem(ctx, item.ID); notesErr != nil {
				log.Warn("[TriggerReReview] ListProgressNotesForItem (context extras) failed", "item", item.ID, "error", notesErr)
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
					log.Warn("[TriggerReReview] WriteReviewTranscriptFile failed", "item", item.ID, "error", transcriptErr)
				} else {
					extras.TranscriptRelPath = relPath
				}
			}
		}

		headlessPrompt := s.reviewPromptFor(item, acSnapshot, workSessionDiff, false, verificationNotes, extras)
		systemPrompt, callOpts, callTimeout, reviewPath := pipeline.BuildReviewCallOptions(workSessionDiff, codebaseWorkDir)

		// Resolve this stage's configured (program, model) executor (Epic 2.3,
		// Story 2.3.3), mirroring TriggerTriage's identical resolution. As there:
		// ComputeExecutorHash MUST hash the RAW reviewExecModel (e.g.
		// "family:opus"), never the ResolveModel-resolved concrete ID that goes
		// into callOpts.Model below — see ComputeExecutorHash's doc comment.
		var reviewExecProgram, reviewExecModel string
		if s.pipelineEngine != nil {
			reviewExecProgram, reviewExecModel = s.pipelineEngine.ExecutorFor(item, workflowengine.StageRoleReview)
		}
		reviewExecutorHash := pipeline.ComputeExecutorHash(reviewExecProgram, reviewExecModel)
		reviewExecModel = pinnedStageModel(workflowengine.StageRoleReview, reviewExecProgram, reviewExecModel)
		reviewCaller, reviewConfiguredProgram, reviewFallbackReason := s.resolveHeadlessCaller(reviewExecProgram, item.ID, "review")
		reviewResolvedModel, reviewModelErr := session.ResolveModel(s.modelFamilies, reviewExecModel)
		if reviewModelErr != nil {
			log.Warn("[PipelineEngine] failed to resolve review model family alias, using empty model", "item", item.ID, "model", reviewExecModel, "err", reviewModelErr)
			reviewResolvedModel = ""
		}
		callOpts.Model = reviewResolvedModel
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
			is, createErr := pipeline.RecordDegradedReviewVerdict(s.storage, item.ID, domain.AcCriteriaJSON(acSnapshotJSON), services.HeadlessReReviewUUIDPrefix, capSummary)
			if createErr != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review capability-self-check verdict: %w", createErr))
			}
			log.Warn("[TriggerReReview] headless re-review complete", "item", item.ID, "outcome", domain.ReviewVerdictUnverifiable, "path", reviewPath, "duration_ms", time.Since(callStart).Milliseconds())
			return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
				ItemSession: itemSessionToProto(is, s.buildCostLookup()),
			}), nil
		}

		reviewCtx, reviewCancel := context.WithTimeout(ctx, callTimeout)
		defer reviewCancel()

		var callCostUSD float64
		// callCostPriced defaults true, matching cost_priced's own schema default,
		// so a call that fails before the sink ever fires reads as "no cost
		// incurred" rather than as an untrustworthy $0 — see the identical
		// rationale on TriggerTriage's triageCostPriced.
		callCostPriced := true
		var reviewConversationID string
		callOpts.OnConversationID = func(id string) { reviewConversationID = id }
		reviewResult, callErr := reviewCaller.CallBlocking(
			reviewCtx, headless.FeatureKeyReview, systemPrompt, headlessPrompt, callOpts,
			func(usd float64, priced bool) {
				callCostUSD = usd
				callCostPriced = priced
				logBudgetWarningIfCrossed(item.ID, "review", item.CostBudgetThresholdUsd, sessions, usd)
			},
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
				is, createErr := pipeline.RecordDegradedReviewVerdict(s.storage, item.ID, domain.AcCriteriaJSON(acSnapshotJSON), services.HeadlessReReviewUUIDPrefix, timeoutSummary)
				if createErr != nil {
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review timeout verdict: %w", createErr))
				}
				log.Warn("[TriggerReReview] headless re-review complete", "item", item.ID, "outcome", domain.ReviewVerdictUnverifiable, "path", reviewPath, "duration_ms", time.Since(callStart).Milliseconds())
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
			capturePath := s.captureHeadlessFailure(services.HeadlessReReviewUUIDPrefix+uuid.New().String(), reviewResult)
			log.Error("[TriggerReReview] headless re-review call failed", "item", item.ID, "errType", errType, "capture", capturePath, "error", callErr)
			failCleanupCtx, failCleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if failIS, failCreateErr := s.storage.CreateItemSession(failCleanupCtx, backlogdto.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: services.HeadlessReReviewUUIDPrefix + uuid.New().String(),
				SessionRole: instance.SessionRoleReview,
				AcSnapshot:  domain.AcCriteriaJSON(acSnapshotJSON),
			}); failCreateErr != nil {
				log.Warn("[TriggerReReview] failed to record audit ItemSession for failed call", "item", item.ID, "error", failCreateErr)
			} else {
				s.endItemSessionForCallError(failCleanupCtx, failIS.ID, errType, callErr)
				if capturePath != "" {
					_ = s.storage.UpdateItemSessionFailureCapture(failCleanupCtx, failIS.ID, capturePath)
				}
			}
			failCleanupCancel()
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("headless re-review call failed: %w", callErr))
		}

		overall, perCriterion, reviewSummary := pipeline.ParseHeadlessVerdictResult(reviewResult)
		toolReads := pipeline.ParseHeadlessToolReads(reviewResult)
		overall, perCriterion, reviewSummary, reviewPath = pipeline.DegradeIfUnverified(reviewPath, overall, perCriterion, reviewSummary, toolReads, codebaseWorkDir, item.PlanArtifactsPath)
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

		reviewSessionUUID := services.HeadlessReReviewUUIDPrefix + uuid.New().String()
		is, createErr := s.storage.CreateItemSessionWithVerdict(cleanupCtx, backlogdto.ItemSessionData{
			ItemID:                 item.ID,
			SessionUUID:            reviewSessionUUID,
			ConversationUUID:       reviewConversationID,
			SessionRole:            instance.SessionRoleReview,
			AcSnapshot:             domain.AcCriteriaJSON(acSnapshotJSON),
			EstimatedCostUsd:       callCostUSD,
			CostUnpriced:           !callCostPriced,
			ResolvedProgram:        reviewExecProgram,
			ResolvedModel:          reviewResolvedModel,
			ExecutorSnapshotHash:   reviewExecutorHash,
			ConfiguredProgram:      reviewConfiguredProgram,
			ExecutorFallbackReason: reviewFallbackReason,
		}, backlogdto.ReviewVerdictData{
			OverallOutcome:    overall,
			PerCriterion:      string(perCriterionJSON),
			Summary:           reviewSummary,
			DiffHash:          s.storage.ComputeCurrentDiffHash(cleanupCtx, item.ID),
			ReviewedCommitSha: s.storage.ComputeReviewedCommitSha(cleanupCtx, item.ID),
			DiffTruncated:     diffTruncated,
		})
		if createErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save headless re-review verdict: %w", createErr))
		}
		if endErr := s.storage.UpdateItemSessionEnded(cleanupCtx, is.ID, time.Now()); endErr != nil { //nolint:silenttransition bookkeeping timestamp only; the PASS/done transition a few lines below (which does notify on failure) is what actually gates forward progress here
			log.Warn("[TriggerReReview] UpdateItemSessionEnded failed", "error", endErr)
		}

		log.Info("[TriggerReReview] headless re-review complete", "item", item.ID, "outcome", overall, "path", reviewPath, "duration_ms", time.Since(callStart).Milliseconds())

		// A fresh review verdict now exists — resolve any open abandoned_review
		// row immediately (Task 2.1.5b) rather than waiting for the self-heal
		// sweep's next tick.
		if _, resolveErr := s.storage.ResolveStuck(ctx, item.ID, domain.StuckReasonAbandonedReview); resolveErr != nil {
			log.Warn("[TriggerReReview] ResolveStuck(abandoned_review) failed", "item", item.ID, "error", resolveErr)
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
		if overall == domain.ReviewVerdictPass {
			if !s.isCodeShippedToMain(ctx, item.ID, item.RepoPath, "TriggerReReview") {
				log.Info("[TriggerReReview] PASS verdict but code not verified on main — leaving in review for manual transition/override", "item", item.ID)
			} else {
				precondition := &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReview)}
				if _, transErr := s.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
					Caller: domain.CallerTriggerReReview, ItemID: item.ID, To: domain.BacklogStatusDone,
					Precondition: precondition, TriggeredBy: instance.TriggeredBySystem,
				}); transErr != nil {
					log.Warn("[TriggerReReview] PASS but transition to done failed", "error", transErr)
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
		log.Info("[TriggerReReview] triggered but no SessionCreator available", "item", item.ID)
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
	// Only allowed to kill a pane owned by one of this item's own prior sessions
	// (ce71ad1a) -- a stale pane under this exact title belonging to an unrelated
	// item is refused, not silently killed.
	if s.sessionStopper != nil {
		if err := s.sessionStopper.KillTmuxSessionByTitle(ctx, title, services.NonEmptySessionUUIDs(sessions)...); errors.Is(err, services.ErrTmuxKillRefused) {
			log.Warn("re-review: stale tmux session not owned by this item, leaving it", "title", title, "err", err)
		}
	}

	// Archive the prior review round's Instance before spawning its replacement —
	// unlike spawnSessionAfterGates' isReopen path, this tmux-backed re-review spawn
	// had no archive-on-supersede call at all, leaving every abandoned-review round
	// (AutoRespawnReview's repeated TriggerReReview calls) as a live Active row that
	// LoadInstances cold-restores on every future restart.
	if mostRecentReviewSession != nil {
		s.archiveItemWorkSessions(ctx, []backlogdto.ItemSessionSummary{*mostRecentReviewSession})
	}

	inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, item.RepoPath, services.SessionSpawnOptions{
		Title:  title,
		Prompt: reReviewPrompt,
		Tags:   []string{"backlog:review"},
		// review-stage program threading is out of Epic 2.4's scope, so ProgramOverride stays "".
		OneShot: !useAutonomous,
		Hidden:  true,
	})
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
	}
	inst.SetCategory(instance.CategoryBacklog)
	if useAutonomous {
		s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
	}

	// 11. Create ItemSession with role=review.
	is, err := s.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID:        item.ID,
		SessionUUID:   inst.UUID,
		SessionRole:   instance.SessionRoleReview,
		AcSnapshot:    domain.AcCriteriaJSON(acSnapshotJSON),
		DiffTruncated: diffTruncated,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create re-review item session: %w", err))
	}

	// Capture the pre-review HEAD SHA so diffs against base..HEAD work correctly.
	s.stampBaseCommit(ctx, baseCommitStamp{Caller: "TriggerReReview", Item: item, Session: is, Dir: item.RepoPath})

	log.Info("[TriggerReReview] spawned re-review session", "session", inst.UUID, "item", item.ID)

	// A review session is active again — resolve any open abandoned_review row
	// immediately (Task 2.1.5b) rather than waiting for the self-heal sweep's
	// next tick.
	if _, resolveErr := s.storage.ResolveStuck(ctx, item.ID, domain.StuckReasonAbandonedReview); resolveErr != nil {
		log.Warn("[TriggerReReview] ResolveStuck(abandoned_review) failed", "item", item.ID, "error", resolveErr)
	}

	return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}
