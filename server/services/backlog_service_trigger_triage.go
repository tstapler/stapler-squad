package services

// backlog_service_trigger_triage.go — TriggerTriage/CancelTriage, the RPC handlers that
// kick off (and cancel) headless triage for a backlog item, plus the helpers used
// exclusively by them. Split out of backlog_service_triage.go (2026-09-12,
// complexity x churn hotspot — see docs/reference/hotspot-ranking.md): TriggerTriage was
// that file's single most complex function (517 lines, sync RPC-handler setup wrapping
// one large async goroutine body — the same shape session_service.go's StreamTerminal
// had). Pure file-level move, no behavior change.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
)

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

// sanitizeTriageTitle turns an LLM-supplied HeadlessTriageResult.Title into a
// value safe to use as a filepath.Join path segment, a commit message
// fragment, and branch-name input. ParseHeadlessTriageResult
// (session/backlog_triage.go) unmarshals Title straight from the triage LLM's
// JSON output and never sanitizes it — used raw, a crafted title such as
// "../../../etc/passwd" resolves outside triageWorkDir when joined into
// PlanArtifactsPath, which readPlanFile (session/backlog_review.go) later
// opens: an arbitrary-file-read primitive if a backlog item's content can
// steer the triage LLM's output (this repo already treats triage-time prompt
// injection as a realistic threat, not hypothetical). slugify already strips
// everything but lowercase alnum/hyphen, which rules out ".." and any path
// separator, so it doubles as the sanitizer here; callers must use this
// return value (not result.Title) everywhere the title reaches a path,
// commit message, or branch name. Falls back to a short itemID-derived slug
// when the title sanitizes away to nothing (empty, or all punctuation/symbols).
func sanitizeTriageTitle(title, itemID string) string {
	if s := slugify(title); s != "" {
		return s
	}
	return "item-" + itemID[:min(len(itemID), 8)]
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
		log.WarningLog().Printf("[TriggerTriage] failed to rename triage worktree branch for item=%s to %q: %v", itemID, finalBranch, renameErr)
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
		log.WarningLog().Printf("[TriggerTriage] failed to clean up provisional triage worktree for item=%s: %v", itemID, cleanupErr)
	}
}

// testTriageCompleteHook, when non-nil, is invoked with the item's ID as the
// last thing TriggerTriage's background goroutine does on every exit path.
// Production code never sets this; it exists so tests can wait for the
// goroutine to actually finish instead of polling item status, which flips
// before the goroutine's trailing writes land. Callers must filter by item
// ID themselves — this is one shared hook across all tests, and a leftover
// goroutine from an unmigrated test can still fire it. Modeled on
// backlog_service_events.go's testAfterSubscribeHook.
//
// WARNING: this is one package-level variable shared by the whole test
// binary. Do not add t.Parallel() to a test that sets this hook unless it
// also filters by item ID and tolerates callbacks from concurrently running
// tests — see session_service_test.go's newRateLimitHiddenTestFixture for the
// kind of cross-subtest crosstalk a shared-state test double can cause under
// t.Parallel().
var (
	// testTriageCompleteHookMu guards concurrent read/write of the hook from
	// a test goroutine (setter) and a still-running TriggerTriage goroutine
	// (reader) — required under -race even though a stale read would often
	// be harmless in practice.
	testTriageCompleteHookMu sync.Mutex
	testTriageCompleteHook   func(itemID string)
)

func setTestTriageCompleteHook(hook func(itemID string)) {
	testTriageCompleteHookMu.Lock()
	defer testTriageCompleteHookMu.Unlock()
	testTriageCompleteHook = hook
}

func callTestTriageCompleteHook(itemID string) {
	testTriageCompleteHookMu.Lock()
	hook := testTriageCompleteHook
	testTriageCompleteHookMu.Unlock()
	if hook != nil {
		hook(itemID)
	}
}

// TriggerTriage kicks off a headless triage planning call for a backlog item.
// Returns immediately after creating an ItemSession; actual triage runs in a goroutine.
// +api: backlog:trigger-triage
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity relocated verbatim by the backlog_service_triage.go split (sdd:fix-hotspot, 2026-09-12); reducing it is a separate follow-up, not a file move
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
			log.WarningLog().Printf("[TriggerTriage] item %s moved past ready before triage reset (race with work-session spawn); aborting re-triage", req.Msg.ItemId)
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
	if mkErr := os.MkdirAll(artifactAbsPath, 0o750); mkErr != nil {
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

	log.InfoLog().Printf("[PipelineEngine] item=%s stage=triage mode=%q", item.ID, session.ResolvedModeLabel(item.PipelineMode))

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

	log.InfoLog().Printf("[TriggerTriage] headless triage started item=%s session=%s path=%s", item.ID, triageSessionUUID, artifactAbsPath)

	// 9. Drive triage asynchronously so the RPC returns immediately.
	itemID := item.ID
	itemRepoPath := item.RepoPath
	isID := is.ID
	iteration := nextIteration
	triageStarted = true
	go func() {
		// Registered first so it runs last (defers are LIFO) — fires only after
		// every other defer in this goroutine (semaphore release, cancel) has
		// run, on every exit path. See testTriageCompleteHook's doc comment.
		defer callTestTriageCompleteHook(itemID)

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

		// Shape A (session.LivenessKindDurationBudget), keyed session.BacklogStatusIdea —
		// the same key reconcileOrphanedTriageItems' staleness gate resolves against
		// (session/backlog_lifecycle_triage.go), so ExpectedDuration and StalenessThreshold
		// can never independently drift apart for the same item (BUG-055's actual
		// invariant — Epic 1.4, Story 1.4.2). BacklogStatusIdea is used rather than
		// item.Status directly: by this point item.Status may still hold its pre-3b-transition
		// value ("ready") in this closure's captured struct even though the item's real status
		// is idea for the duration of this call — an unresolved "ready" key would fall through
		// to NoTimeoutLiveness (0 ExpectedDuration), an immediate-timeout regression. Nil-guarded:
		// an unwired/unresolvable engine falls back to the literal triageCallBudget constant,
		// byte-for-byte unchanged from before.
		callBudget := triageCallBudget
		if s.livenessEngine != nil {
			if def, defErr := s.livenessEngine.LivenessFor(session.BacklogStatusIdea, session.PipelineMode(item.PipelineMode)); defErr == nil && !def.IsNoTimeout() {
				callBudget = def.ExpectedDuration
			}
		}
		triageCtx, cancel := context.WithTimeout(s.shutdownCtx, callBudget)
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
		// Anchor to the main repo root and branch from its default branch tip,
		// exactly like session.CreateBacklogWorktree does for the real work
		// session — not from itemRepoPath's ambient HEAD. Without this, an item
		// filed by an agent that passed its own in-progress worktree as repo_path
		// forked triage (and, via retitleTriageWorktreeToFinalBranch below, the
		// item's eventual work branch too) from that agent's feature branch
		// instead of main. item.BaseBranch is the deliberate opt-out of that
		// default: when set, triage forks from that named branch instead (still
		// origin-first, falling back to local). Best-effort at every step: any
		// resolution failure falls back to the pre-fix behavior (branch from
		// ambient HEAD), and any worktree-creation failure falls back to running
		// triage directly in itemRepoPath, both unchanged from before — a bad
		// BaseBranch is still caught loudly later, when the real work session is
		// spawned via CreateBacklogWorktree, which hard-errors instead.
		triageRepoRoot, mainRepoErr := session.ResolveMainRepoRoot(itemRepoPath)
		if mainRepoErr != nil {
			triageRepoRoot = itemRepoPath
		}
		triageBranchName := config.LoadConfig().BranchPrefix + git.SanitizeBranchName("triage-"+itemID)
		var triageBaseSHA string
		var baseErr error
		if item.BaseBranch != "" {
			triageBaseSHA, baseErr = git.ResolveExplicitBranchSHA(triageRepoRoot, item.BaseBranch)
		} else {
			_, triageBaseSHA, baseErr = git.ResolveWorktreeBaseCommit(triageRepoRoot)
		}
		var wt *git.GitWorktree
		var wtErr error
		if baseErr == nil && triageBaseSHA != "" {
			wt, _, wtErr = git.NewGitWorktreeFromCommitSHA(triageRepoRoot, "triage-"+itemID, triageBranchName, triageBaseSHA)
		} else {
			wt, _, wtErr = git.NewGitWorktree(triageRepoRoot, "triage-"+itemID)
		}
		if wtErr != nil {
			log.WarningLog().Printf("[TriggerTriage] failed to create isolated worktree for item=%s, running triage directly in repo_path: %v", itemID, wtErr)
		} else if setupErr := wt.Setup(); setupErr != nil {
			log.WarningLog().Printf("[TriggerTriage] failed to set up isolated worktree for item=%s, running triage directly in repo_path: %v", itemID, setupErr)
		} else {
			triageWorktree = wt
			triageWorkDir = wt.GetWorktreePath()
		}

		callStart := time.Now()
		var triageCostUSD float64
		raw, callErr := s.headlessPool.CallBlocking(triageCtx,
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
			func(usd float64) { triageCostUSD = usd },
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

		// Persisted unconditionally, before the success/failure branches below: the
		// LLM call already incurred this cost whether or not it errored or produced
		// parseable output, so it must not be lost down either failure path.
		if triageCostUSD > 0 {
			if costErr := s.storage.UpdateItemSessionCost(cleanupCtx, isID, triageCostUSD); costErr != nil {
				log.WarningLog().Printf("[TriggerTriage] failed to persist cost item=%s: %v", itemID, costErr)
			}
		}

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
			log.ErrorLog().Printf("[TriggerTriage] headless triage failed item=%s elapsed=%s errType=%s capture=%s: %v",
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
			log.ErrorLog().Printf("[TriggerTriage] parse result failed item=%s elapsed=%s rawLen=%d capture=%s: %v",
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

		// result.Title is LLM-controlled (ParseHeadlessTriageResult never
		// sanitizes it) and reaches a commit message, a branch name, and — below
		// — a filepath.Join path segment (PlanArtifactsPath), so it must be
		// sanitized once, up front, and every downstream use must go through
		// sanitizedTitle rather than the raw field. See sanitizeTriageTitle's
		// doc comment for the path-traversal primitive this closes.
		//
		// result.Title itself is overwritten with sanitizedTitle below, not just
		// read from it: result is JSON-marshaled and persisted as the item's
		// TriageResult a few lines down, and triageShortTitle/backlogWorkBranchSlug
		// later read Title back out of that persisted JSON to recompute the
		// work-session branch name. If the persisted Title stayed raw, that
		// spawn-time branch computation would diverge from the sanitizedTitle
		// already used above to create the worktree/branch here — reintroducing
		// the worktree/branch drift regression backlogWorkBranchSlug's doc
		// comment says was already fixed once.
		sanitizedTitle := sanitizeTriageTitle(result.Title, itemID)
		result.Title = sanitizedTitle

		// Commit whatever the triage prompt wrote (project_plans/<name>/ for SDD
		// mode; nothing for default mode, which writes to artifactAbsPath instead
		// — CommitChanges no-ops when the worktree isn't dirty) so the docs
		// survive past this goroutine instead of sitting uncommitted indefinitely
		// (the exact gap docs/how-to/commit-sdd-planning-artifacts.md already
		// names). Only when triageWorktree is non-nil — the itemRepoPath fallback
		// path must never auto-commit into a repo this code didn't create.
		if triageWorktree != nil {
			if commitErr := triageWorktree.CommitChanges(fmt.Sprintf("chore(sdd): planning artifacts for %s", sanitizedTitle)); commitErr != nil {
				log.WarningLog().Printf("[TriggerTriage] failed to commit triage artifacts item=%s worktree=%s: %v", itemID, triageWorkDir, commitErr)
			}
			retitleTriageWorktreeToFinalBranch(itemID, itemRepoPath, sanitizedTitle, triageWorktree)
		}

		// persistCtx gives post-git persistence writes their own fresh timeout
		// budget, started after CommitChanges/retitle's ~30s-capable git
		// subprocesses run rather than before — reusing cleanupCtx here produced
		// an intermittent "expected: ready, actual: idea" flake under
		// -race -count=5 when those git ops alone ate its budget. See
		// TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession's
		// "Second root cause" comment for the full history.
		persistCtx, persistCancel := context.WithTimeout(context.Background(), s.triageCleanupTimeout)
		defer persistCancel()

		payloadJSON, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			log.ErrorLog().Printf("[TriggerTriage] marshal triage result item=%s: %v", itemID, marshalErr)
			_ = s.storage.UpdateItemSessionEnded(persistCtx, isID, time.Now())
			return
		}
		// persistFailures accumulates which of the post-triage persistence steps below
		// failed. Each step already logs its own error to the log file (operator-invisible
		// in real time); if any step fails, notifyTriagePersistFailure below additionally
		// surfaces a single operator-facing notification so a failure here is never silent.
		var persistFailures []string

		if updateErr := s.storage.UpdateItemSessionTriageResult(persistCtx, isID, string(payloadJSON)); updateErr != nil {
			log.ErrorLog().Printf("[TriggerTriage] persist triage result item=%s: %v", itemID, updateErr)
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
			pap = filepath.Join(triageWorkDir, "project_plans", sanitizedTitle, "implementation")
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
		if _, updateErr := s.storage.UpdateBacklogItem(persistCtx, itemID, update, nil); updateErr != nil {
			log.ErrorLog().Printf("[TriggerTriage] update plan_artifacts_path item=%s: %v", itemID, updateErr)
			persistFailures = append(persistFailures, "saving the plan artifacts path")
		}

		// Close out the ItemSession and release triageInFlight together, BEFORE the
		// status transition to Ready below — not after it, and not after the optional
		// auto-spawn further down. ended_at and triageInFlight are updated in the same
		// spot deliberately: both are inputs to the orphan-liveness check (IsTriageLive
		// / tombstoneOrphanTriageSessions above), so moving one without the other would
		// let a concurrent reconciliation sweep see "ended_at nil, not live" and
		// wrongly tombstone a session that's simply between here and its final log
		// line.
		//
		// This pair used to run AFTER the status transition ("the item's status
		// already flipped to Ready, so a caller polling on status can legitimately
		// re-trigger triage now"), which was itself the fix for the original
		// TestTriggerTriage_RefineWithFeedback CI flake (auto-spawn's I/O stretching
		// the window). But that left a second, narrower window open: a caller polling
		// storage directly (not through IsTriageLive) can observe Ready the instant
		// TransitionBacklogItemStatus returns, race ahead of this goroutine's own next
		// line, and hit a spurious AlreadyExists from the triageInFlight guard at 3a-i
		// above -- confirmed as the root cause of a full-test-suite-only (never
		// isolated) flake in TestTriggerTriage_RefineWithFeedback_ClearsRejectionReason,
		// where DB lock contention under -p made that window wide enough to hit
		// reliably. Clearing triageInFlight before the status write closes it: by the
		// time Ready is observable to any caller, this item is already re-triggerable.
		_ = s.storage.UpdateItemSessionEnded(persistCtx, isID, time.Now())
		s.triageInFlight.Delete(itemID)

		precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusIdea)}
		statusAdvanced := true
		if _, transErr := s.storage.TransitionBacklogItemStatus(persistCtx, itemID, //nolint:silenttransition surfaced a few lines below via notifyTriagePersistFailure once persistFailures is fully collected
			session.BacklogStatusReady, precondition, session.TriggeredBySystem); transErr != nil {
			log.ErrorLog().Printf("[TriggerTriage] status transition idea→ready item=%s: %v", itemID, transErr)
			persistFailures = append(persistFailures, "advancing the item to Ready")
			statusAdvanced = false
		}

		if len(persistFailures) > 0 {
			s.notifyTriagePersistFailure(persistCtx, itemID, item.Title, persistFailures, statusAdvanced)
		}

		// Opt-in: skip the manual "Spawn Session" click when the item is configured to
		// auto-spawn. Autonomous: true bypasses the planning-approval gate the same way
		// AutoReopenForPRFix's spawn already does — a human never gets to review the plan
		// first, which is the whole point of this toggle (default false; existing manual
		// flow is unchanged unless explicitly opted in).
		if statusAdvanced && item.AutoSpawnSession {
			if _, spawnErr := s.SpawnSessionFromItem(persistCtx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
				ItemId:     itemID,
				Autonomous: true,
			})); spawnErr != nil {
				log.WarningLog().Printf("[TriggerTriage] auto-spawn session item=%s: %v", itemID, spawnErr)
			} else {
				log.InfoLog().Printf("[TriggerTriage] auto-spawned work session item=%s (auto_spawn_session=true)", itemID)
			}
		}

		log.InfoLog().Printf("[TriggerTriage] headless triage complete item=%s elapsed=%s suggestions=%d tasks=%d",
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
