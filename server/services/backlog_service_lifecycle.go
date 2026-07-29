package services

// backlog_service_lifecycle.go — state-mutation RPC handlers for BacklogService.
// These handlers create, update, archive, delete, or transition backlog items and sources.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
)

// resolveStuckOnManualTransition immediately resolves the durable stuck
// reasons that a manual TransitionBacklogItemStatus call makes obsolete,
// rather than waiting for the self-heal sweep's next tick (Task 2.1.5b —
// "manual re-review/transition"). Best-effort: errors are logged, never
// returned. Mirrors (but does not replace) the reconcile self-heal sweep in
// session/backlog_lifecycle.go, which remains the correctness backstop for
// any transition path this handler doesn't cover.
func resolveStuckOnManualTransition(ctx context.Context, storage *session.Storage, itemID string, to session.BacklogStatus) {
	if storage == nil {
		return
	}
	var reasons []domain.StuckReason
	switch to {
	case session.BacklogStatusInProgress:
		// Leaving review/pr_pending for rework — the cap-hit and
		// review-abandonment conditions no longer apply.
		reasons = []domain.StuckReason{domain.StuckReasonReworkCap, domain.StuckReasonAbandonedReview}
	case session.BacklogStatusDone, session.BacklogStatusArchived:
		// Terminal — every reason is obsolete.
		reasons = domain.AllStuckReasons
	case session.BacklogStatusPRPending:
		reasons = []domain.StuckReason{domain.StuckReasonAbandonedReview, domain.StuckReasonStaleWork}
	default:
		return
	}
	for _, reason := range reasons {
		if _, err := storage.ResolveStuck(ctx, itemID, reason); err != nil {
			log.WarningLog.Printf("[TransitionBacklogItemStatus] ResolveStuck(%s) item=%s: %v", reason, itemID, err)
		}
	}
}

// encryptAndMergeToken produces a token config JSON string suitable for storage.
// If key is non-nil the token is AES-GCM encrypted and the result is
// `{"token":"<ciphertext>","encrypted":true}`. Otherwise the token is stored
// unencrypted (backwards-compat). The returned string can be stored as-is when
// the existing config is empty. When existingConfig is non-empty the token JSON
// is merged into it (token fields win). Returns the merged JSON or an error.
func encryptAndMergeToken(cfg *config.Config, token, existingConfig string) (string, error) {
	var tokenJSON string
	if cfg != nil {
		key, err := cfg.GetOrCreateEncryptionKey()
		if err != nil {
			return "", fmt.Errorf("get encryption key: %w", err)
		}
		encrypted, err := session.EncryptToken(key, token)
		if err != nil {
			return "", fmt.Errorf("encrypt token: %w", err)
		}
		tokenJSON = fmt.Sprintf(`{"token":%q,"encrypted":true}`, encrypted)
	} else {
		// No config available; store unencrypted (backwards compatibility).
		tokenJSON = fmt.Sprintf(`{"token":%q}`, token)
	}

	if existingConfig == "" {
		return tokenJSON, nil
	}

	// Merge token fields into the existing config JSON.
	var cfgMap map[string]interface{}
	if err := json.Unmarshal([]byte(existingConfig), &cfgMap); err != nil {
		return "", fmt.Errorf("unmarshal existing config: %w", err)
	}
	var tokMap map[string]interface{}
	if err := json.Unmarshal([]byte(tokenJSON), &tokMap); err != nil {
		return "", fmt.Errorf("unmarshal token json: %w", err)
	}
	for k, v := range tokMap {
		cfgMap[k] = v
	}
	merged, err := json.Marshal(cfgMap)
	if err != nil {
		return "", fmt.Errorf("marshal merged config: %w", err)
	}
	return string(merged), nil
}

// acCriteriaToJSON serializes proto AcCriterion slice to JSON string for storage.
func acCriteriaToJSON(protoAC []*sessionv1.AcCriterion) (session.AcCriteriaJSON, error) {
	if len(protoAC) == 0 {
		return "", nil
	}
	criteria := make([]session.AcCriterion, len(protoAC))
	for i, c := range protoAC {
		criteria[i] = session.AcCriterion{
			Index:  int(c.Index),
			Text:   c.Text,
			Status: session.AcStatus(c.Status),
		}
	}
	b, err := json.Marshal(criteria)
	if err != nil {
		return "", err
	}
	return session.AcCriteriaJSON(b), nil
}

// defaultPipelineModeForNewItem resolves the pipeline_mode value a brand-new
// item should be created with. requested is CreateBacklogItemRequest's raw
// *string field: nil means the caller omitted pipeline_mode entirely (every
// non-UI caller — BacklogItemForm.tsx always sends the field, even as ""; see
// project_plans/backlog-sdd-default-pipeline/research/expressiveness-and-
// design.md §4), and is the only case this function ever overrides. Any
// explicit value — including an explicit empty string — is always respected
// verbatim, so this never overrides a caller's deliberate choice to use the
// flat default pipeline.
//
// Reads the flag via a fresh config.LoadConfig() rather than s.cfg, matching
// every other live feature-flag read in this codebase (server/server.go's
// interceptor, session/instance_vnc.go, session/instance_cdp.go) — a flag
// toggled via UpdateFeatureFlag persists through its own LoadConfig/SaveConfig
// round-trip, not through BacklogService's constructor-injected cfg pointer.
// Only ever runs at item creation, so it can never retroactively change an
// already-created item's stored pipeline_mode.
func defaultPipelineModeForNewItem(requested *string) string {
	if requested != nil {
		return *requested
	}
	if !config.LoadConfig().GetFeatureFlag(sddDefaultPipelineFlagName) {
		return ""
	}
	return session.DefaultSDDPipelineModeSlug
}

// --- CreateBacklogItem ---

// CreateBacklogItem adds a new item to the backlog.
// +api: backlog:create-item
func (s *BacklogService) CreateBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateBacklogItemRequest],
) (*connect.Response[sessionv1.CreateBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	category := ""
	if req.Msg.Category != nil {
		category = *req.Msg.Category
	}
	if !session.IsValidBacklogCategory(category) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid category %q", category))
	}

	acJSON, err := acCriteriaToJSON(req.Msg.AcceptanceCriteria)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid acceptance_criteria: %w", err))
	}

	priority := int(req.Msg.Priority)
	if priority == 0 {
		priority = session.DefaultBacklogPriority
	}

	repoPath, err := s.resolveRepoPathInput(req.Msg.RepoPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	data := session.BacklogItemData{
		Title:              req.Msg.Title,
		Description:        req.Msg.Description,
		AcceptanceCriteria: acJSON,
		Priority:           priority,
		Status:             string(session.BacklogStatusIdea),
		RepoPath:           repoPath,
		SkipReviewGate:     req.Msg.SkipReviewGate,
		SkipPlanning:       req.Msg.SkipPlanning,
		AutoSpawnSession:   req.Msg.AutoSpawnSession,
		AutoCreatePR:       req.Msg.AutoCreatePr,
		PipelineMode:       defaultPipelineModeForNewItem(req.Msg.PipelineMode),
		Category:           category,
		Notes:              req.Msg.Notes,
	}

	created, err := s.storage.CreateBacklogItem(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create backlog item: %w", err))
	}

	triageTriggered := false
	if !req.Msg.SkipTriage && created.RepoPath != "" && s.headlessPool != nil {
		// 30s gates only the synchronous path (item lookup + ItemSession creation).
		// The headless LLM call itself runs in a goroutine under shutdownCtx (30-min cap).
		triageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, triageErr := s.TriggerTriage(triageCtx,
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: created.ID}))
		if triageErr != nil {
			log.WarningLog.Printf("[CreateBacklogItem] auto-triage failed for item %s: %v", created.ID, triageErr)
			// Do not fail the create; log and continue
		} else {
			triageTriggered = true
		}
	}

	return connect.NewResponse(&sessionv1.CreateBacklogItemResponse{
		Item:            backlogItemToProto(created, s.buildCostLookup()),
		TriageTriggered: triageTriggered,
	}), nil
}

// --- UpdateBacklogItem ---

// UpdateBacklogItem modifies the properties of an existing backlog item.
// +api: backlog:update-item
func (s *BacklogService) UpdateBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateBacklogItemRequest],
) (*connect.Response[sessionv1.UpdateBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	acJSON, err := acCriteriaToJSON(req.Msg.AcceptanceCriteria)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid acceptance_criteria: %w", err))
	}

	update := session.BacklogItemUpdate{}
	if req.Msg.Title != "" {
		title := req.Msg.Title
		update.Title = &title
	}
	if req.Msg.Description != "" {
		desc := req.Msg.Description
		update.Description = &desc
	}
	if acJSON != "" {
		update.AcceptanceCriteria = &acJSON
	}
	if req.Msg.Priority != 0 {
		prio := int(req.Msg.Priority)
		update.Priority = &prio
	}
	if req.Msg.RepoPath != "" {
		rp, resolveErr := s.resolveRepoPathInput(req.Msg.RepoPath)
		if resolveErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, resolveErr)
		}
		update.RepoPath = &rp
	}
	skipRG := req.Msg.SkipReviewGate
	update.SkipReviewGate = &skipRG
	skipP := req.Msg.SkipPlanning
	update.SkipPlanning = &skipP
	autoSpawn := req.Msg.AutoSpawnSession
	update.AutoSpawnSession = &autoSpawn
	autoCreatePR := req.Msg.AutoCreatePr
	update.AutoCreatePR = &autoCreatePR
	// PipelineMode is presence-gated (optional string on the wire): only set
	// update.PipelineMode when the field was explicitly present on the
	// request, so an omitted pipeline_mode never clobbers the item's existing
	// mode back to "". This is deliberately NOT an unconditional wrap like
	// SkipReviewGate/SkipPlanning/AutoSpawnSession above — see Story 1.4.4.
	if req.Msg.PipelineMode != nil {
		update.PipelineMode = req.Msg.PipelineMode
	}
	// Category is presence-gated the same way as PipelineMode above: only set
	// update.Category when the field was explicitly present on the request,
	// so an omitted category never clobbers the item's existing category back
	// to "" (uncategorized).
	if req.Msg.Category != nil {
		if !session.IsValidBacklogCategory(*req.Msg.Category) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid category %q", *req.Msg.Category))
		}
		update.Category = req.Msg.Category
	}
	// ReworkCapOverride is presence-gated the same way as PipelineMode above:
	// only set when the client explicitly sent it, so an omitted field never
	// clobbers the item's existing override back to "unlimited" (0).
	if req.Msg.ReworkCapOverride != nil {
		override := int(*req.Msg.ReworkCapOverride)
		update.ReworkCapOverride = &override
	}
	if req.Msg.Notes != "" {
		notes := req.Msg.Notes
		update.Notes = &notes
	}

	var precondition *session.BacklogItemPrecondition
	if req.Msg.ExpectedStatus != "" || req.Msg.ExpectedUpdatedAt != nil {
		precondition = &session.BacklogItemPrecondition{
			ExpectedStatus: req.Msg.ExpectedStatus,
		}
		if req.Msg.ExpectedUpdatedAt != nil {
			t := req.Msg.ExpectedUpdatedAt.AsTime()
			precondition.ExpectedUpdatedAt = &t
		}
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, precondition)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, err)
		}
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateBacklogItemResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
}

// --- ArchiveBacklogItem ---

// ArchiveBacklogItem soft-deletes an item by setting its archived_at timestamp.
// +api: backlog:archive-item
func (s *BacklogService) ArchiveBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveBacklogItemRequest],
) (*connect.Response[sessionv1.ArchiveBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// Push work branches before archiving so changes are durable.
	if sessions, lsErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId); lsErr == nil {
		s.commitAndPushItemWorktrees(ctx, sessions)
	}

	archived, err := s.storage.ArchiveBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to archive backlog item: %w", err))
	}

	// Best-effort: clean up git worktrees for work sessions on archive.
	if sessions, lsErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId); lsErr == nil {
		s.cleanupItemWorktrees(ctx, sessions)
	}

	return connect.NewResponse(&sessionv1.ArchiveBacklogItemResponse{
		Item: backlogItemToProto(archived, s.buildCostLookup()),
	}), nil
}

// --- DeleteBacklogItem ---

// DeleteBacklogItem permanently removes an item and all its child records.
// +api: backlog:delete-item
func (s *BacklogService) DeleteBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteBacklogItemRequest],
) (*connect.Response[sessionv1.DeleteBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	if err := s.storage.DeleteBacklogItem(ctx, req.Msg.ItemId); err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.DeleteBacklogItemResponse{}), nil
}

// --- TransitionBacklogItemStatus ---

// resolveLatestWorkCommit returns the true current tip commit of the work
// session identified by sessionUUID — never ItemSessionSummary.LastCommitSha,
// which is only ever seeded once at session spawn with the pre-work base SHA
// (see the UpdateItemSessionGitActivity calls in backlog_service_triage.go/
// backlog_service_sync.go, all of which pass baseSHA) and is never updated
// afterward as the agent commits real work. Treating that field as "the
// agent's latest commit" made isCodeShippedToMain trivially true for nearly
// any item, because a branch's own base commit is — by construction — always
// an ancestor of main. Confirmed live 2026-07-21: archived items 693c2700,
// 61684863, and 40cf8885 were all approved as "shipped" by this gate despite
// each having real, unmerged work and no PR ever opened. Mirrors
// session.BacklogLifecycleListener's identical fix for
// reconcileBouncingItems' equivalent no-PR fallback
// (session/backlog_lifecycle.go).
//
// Prefers the session's own worktree HEAD; falls back to resolving the
// branch's tip directly in repoPath if the worktree directory is gone, since
// worktrees of the same repo share one object store. Returns "" if neither
// resolves.
func (s *BacklogService) resolveLatestWorkCommit(ctx context.Context, sessionUUID, repoPath string) string {
	wt, err := s.storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	if err != nil {
		log.WarningLog.Printf("resolveLatestWorkCommit: no worktree data for session %s: %v", sessionUUID, err)
		return ""
	}
	if wt.WorktreePath != "" {
		if info, statErr := os.Stat(wt.WorktreePath); statErr == nil && info.IsDir() {
			if sha, headErr := session.GetGitHeadSHA(wt.WorktreePath); headErr == nil && sha != "" {
				return sha
			}
		}
	}
	if wt.BranchName == "" {
		return ""
	}
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--verify", wt.BranchName)
	cmd.Dir = repoPath
	out, revErr := cmd.Output()
	if revErr != nil {
		log.WarningLog.Printf("resolveLatestWorkCommit: rev-parse %s in %s: %v", wt.BranchName, repoPath, revErr)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isCodeShippedToMain reports whether itemID's most recent work session's
// current tip commit (resolveLatestWorkCommit, NOT the session's stale
// LastCommitSha field — see that function's doc comment) has actually landed
// on main — locally or via a merged PR pushed to origin. Returns true when
// there is nothing to verify (no work session ever ran, or no commit could be
// resolved for it) or the commit is confirmed on main; false when there IS a
// commit that could not be confirmed shipped, or the check itself failed
// (fails closed — callers must not silently mark an item done just because
// verification was unavailable).
//
// This is the single check shared by every path that can transition an item to
// done — the RPC handler, TriggerReReview's and SubmitManualReview's auto-transition
// on a PASS verdict — so "approved" (a review verdict) and "shipped" (code actually on
// main) stay two independently-enforced gates everywhere, not just at the one call
// site a human happens to go through.
func (s *BacklogService) isCodeShippedToMain(ctx context.Context, itemID, repoPath, logPrefix string) bool {
	itemSessions, err := s.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog.Printf("[%s] isCodeShippedToMain: failed to load item sessions for item %s: %v", logPrefix, itemID, err)
		return false
	}
	var lastWorkSessionUUID string
	for _, is := range itemSessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep overwriting
		// so this ends up holding the *most recent* work session.
		if is.Role == session.SessionRoleWork {
			lastWorkSessionUUID = is.SessionUUID
		}
	}
	if lastWorkSessionUUID == "" {
		return true // no work session ever ran — nothing to verify
	}
	lastCommitSha := s.resolveLatestWorkCommit(ctx, lastWorkSessionUUID, repoPath)
	if lastCommitSha == "" {
		return true // nothing resolvable — nothing to verify
	}
	onMain, mainErr := git.IsCommitOnMain(repoPath, prFixMainBranch, lastCommitSha)
	if mainErr != nil {
		log.WarningLog.Printf("[%s] isCodeShippedToMain: failed to verify commit %s on main for item %s: %v", logPrefix, lastCommitSha, itemID, mainErr)
		return false
	}
	return onMain
}

// TransitionBacklogItemStatus moves an item through the status state machine.
// +api: backlog:transition-status
func (s *BacklogService) TransitionBacklogItemStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.TransitionBacklogItemStatusRequest],
) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// Load current item to check CanTransitionBacklog.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	from := session.BacklogStatus(item.Status)
	to := session.BacklogStatus(req.Msg.TargetStatus)

	if !s.engine.CanTransition(from, to) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid transition from %q to %q", from, to))
	}

	// Load the most recent ReviewVerdict for this item so TransitionGuard can
	// evaluate the review→done guard (ErrVerdictRequired).
	overallOutcome, verdictErr := s.storage.GetMostRecentReviewVerdictForItem(ctx, req.Msg.ItemId)
	if verdictErr != nil {
		log.WarningLog.Printf("[TransitionBacklogItemStatus] failed to load review verdict for item %s: %v", req.Msg.ItemId, verdictErr)
		// Non-fatal: proceed with empty outcome; TransitionGuard will block review→done if needed.
	}

	// Check for unshipped worktree code when transitioning to done. A PrURL alone
	// is not proof the code shipped — that PR may still be open, may have been
	// closed without merging, or may have been reverted — so verify the most
	// recent work session's commit is actually an ancestor of main (locally or on
	// origin) rather than trusting the cached PrURL field. This is a distinct
	// gate from OverallOutcome above: a PASS verdict says the code is good, not
	// that it has landed on main.
	var hasUnshippedCode bool
	if to == session.BacklogStatusDone {
		hasUnshippedCode = !s.isCodeShippedToMain(ctx, req.Msg.ItemId, item.RepoPath, "TransitionBacklogItemStatus")
	}

	// Run transition guard for business rules.
	guardInput := session.BacklogItemTransitionInput{
		Status:            from,
		AcCriteria:        item.AcceptanceCriteria,
		PlanApproved:      item.PlanApproved,
		SkipPlanning:      item.SkipPlanning,
		PlanArtifactsPath: item.PlanArtifactsPath,
		OverallOutcome:    overallOutcome,
		OverrideReason:    req.Msg.OverrideReason,
		HasUnshippedCode:  hasUnshippedCode,
	}
	if guardErr := s.engine.ValidateGates(guardInput, to); guardErr != nil {
		if errors.Is(guardErr, session.ErrACRequired) ||
			errors.Is(guardErr, session.ErrPlanRequired) ||
			errors.Is(guardErr, session.ErrPlanArtifactsRequired) ||
			errors.Is(guardErr, session.ErrVerdictRequired) ||
			errors.Is(guardErr, session.ErrCodeNotOnMain) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, guardErr)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, guardErr)
	}

	var precondition *session.BacklogItemPrecondition
	if req.Msg.ExpectedStatus != "" || req.Msg.ExpectedUpdatedAt != nil {
		precondition = &session.BacklogItemPrecondition{
			ExpectedStatus: req.Msg.ExpectedStatus,
		}
		if req.Msg.ExpectedUpdatedAt != nil {
			t := req.Msg.ExpectedUpdatedAt.AsTime()
			precondition.ExpectedUpdatedAt = &t
		}
	}

	// Push work branches before marking done so changes are durable before the
	// status changes and the worktree is removed.
	if to == session.BacklogStatusDone {
		if sessions, lsErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId); lsErr == nil {
			s.commitAndPushItemWorktrees(ctx, sessions)
		}
	}

	updated, err := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId, to, precondition, session.TriggeredByUser)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to transition backlog item: %w", err))
	}
	resolveStuckOnManualTransition(ctx, s.storage, req.Msg.ItemId, to)

	// Best-effort: clean up git worktrees and archive work sessions on terminal
	// transitions, so they stop accumulating in the default session list once
	// their item is done/archived (see docs/tasks/workflow-history-and-archiving.md
	// — this reuses that epic's ArchivedAt mechanism, extended to backlog work
	// sessions which it originally excluded).
	if to == session.BacklogStatusDone || to == session.BacklogStatusArchived {
		if sessions, lsErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId); lsErr == nil {
			s.cleanupItemWorktrees(ctx, sessions)
			s.archiveItemWorkSessions(ctx, sessions)
		}
	}

	// Backward to idea/refining: reset planning approval so triage must re-run.
	// Best-effort — a warning is logged but the transition itself is already committed.
	if to == session.BacklogStatusIdea || to == session.BacklogStatusRefining {
		planApproved := false
		planArtifactsPath := ""
		if upd, resetErr := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, session.BacklogItemUpdate{
			PlanApproved:      &planApproved,
			PlanArtifactsPath: &planArtifactsPath,
		}, nil); resetErr != nil {
			log.WarningLog.Printf("[TransitionBacklogItemStatus] failed to reset planning state for item %s: %v", req.Msg.ItemId, resetErr)
		} else {
			updated = upd
		}
	}

	return connect.NewResponse(&sessionv1.TransitionBacklogItemStatusResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
}

// --- ApprovePlan ---

// ApprovePlan marks the planning artifacts for an item as approved.
// +api: backlog:approve-plan
func (s *BacklogService) ApprovePlan(
	ctx context.Context,
	req *connect.Request[sessionv1.ApprovePlanRequest],
) (*connect.Response[sessionv1.ApprovePlanResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	if item.PlanArtifactsPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no plan artifacts found — run TriggerTriage first"))
	}
	if _, statErr := os.Stat(item.PlanArtifactsPath); statErr != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("plan artifacts path %q does not exist on disk — re-run TriggerTriage", item.PlanArtifactsPath))
	}

	now := time.Now()
	approved := true
	update := session.BacklogItemUpdate{
		PlanApproved:   &approved,
		PlanApprovedAt: &now,
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to approve plan: %w", err))
	}

	return connect.NewResponse(&sessionv1.ApprovePlanResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
}

// --- ItemSource handlers ---

// CreateItemSource registers a new external plugin source.
// +api: backlog:create-source
func (s *BacklogService) CreateItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateItemSourceRequest],
) (*connect.Response[sessionv1.CreateItemSourceResponse], error) {
	if s.sourceBackend == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	data := session.ItemSourceData{
		PluginID:    req.Msg.PluginId,
		DisplayName: req.Msg.DisplayName,
		Enabled:     true,
		Config:      req.Msg.ConfigJson,
	}
	if req.Msg.Token != "" {
		data.TokenConfigured = true
		merged, mergeErr := encryptAndMergeToken(s.cfg, req.Msg.Token, data.Config)
		if mergeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, mergeErr)
		}
		data.Config = merged
	}

	created, err := s.sourceBackend.CreateItemSource(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.CreateItemSourceResponse{
		Source: itemSourceToProto(created),
	}), nil
}

// UpdateItemSource modifies configuration for an existing item source.
// +api: backlog:update-source
func (s *BacklogService) UpdateItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateItemSourceRequest],
) (*connect.Response[sessionv1.UpdateItemSourceResponse], error) {
	if s.sourceBackend == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	update := session.ItemSourceUpdate{}
	if req.Msg.DisplayName != "" {
		dn := req.Msg.DisplayName
		update.DisplayName = &dn
	}
	enabled := req.Msg.Enabled
	update.Enabled = &enabled
	if req.Msg.Token != "" {
		// UpdateItemSource replaces the config wholesale (no prior config to merge).
		tokenJSON, mergeErr := encryptAndMergeToken(s.cfg, req.Msg.Token, "")
		if mergeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, mergeErr)
		}
		update.Config = &tokenJSON
	}

	updated, err := s.sourceBackend.UpdateItemSource(ctx, req.Msg.SourceId, update)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateItemSourceResponse{
		Source: itemSourceToProto(updated),
	}), nil
}

// DeleteItemSource removes an external item source registration.
// +api: backlog:delete-source
func (s *BacklogService) DeleteItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteItemSourceRequest],
) (*connect.Response[sessionv1.DeleteItemSourceResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	if err := s.storage.DeleteItemSource(ctx, req.Msg.SourceId); err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.DeleteItemSourceResponse{}), nil
}

// --- Session-linked handlers ---

// SpawnSessionFromItem creates a new AI agent session for a backlog item.
// +api: backlog:spawn-session

// OverrideVerdict manually overrides a review verdict for an item session.
// +api: backlog:override-verdict
func (s *BacklogService) OverrideVerdict(
	ctx context.Context,
	req *connect.Request[sessionv1.OverrideVerdictRequest],
) (*connect.Response[sessionv1.OverrideVerdictResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Validate override reason.
	if req.Msg.OverrideReason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("override_reason is required"))
	}

	// 2. Load the ItemSession by entity UUID to get the linked BacklogItem ID.
	is, err := s.storage.GetItemSession(ctx, req.Msg.ItemSessionId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("item session %q not found", req.Msg.ItemSessionId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get item session: %w", err))
	}

	// Get the linked BacklogItem ID from the ItemSessionSummary.
	itemID := is.BacklogItemID
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("item session %q has no linked backlog item", req.Msg.ItemSessionId))
	}

	// 3. Determine outcome based on to_status.
	outcome := session.ReviewVerdictPass
	if req.Msg.ToStatus == string(session.BacklogStatusInProgress) {
		outcome = session.ReviewVerdictFail
	}

	// 4. Save/upsert the ReviewVerdict with override fields.
	now := time.Now()
	if verdictErr := s.storage.SaveReviewVerdict(ctx, is.ID, session.ReviewVerdictData{
		OverallOutcome: outcome,
		Summary:        fmt.Sprintf("Manual override: %s", req.Msg.OverrideReason),
		OverrideBy:     "user",
		OverrideReason: req.Msg.OverrideReason,
		OverrideAt:     &now,
	}); verdictErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save review verdict: %w", verdictErr))
	}

	// 5. Transition item to target status if valid (validate via state machine).
	var updatedItem *session.BacklogItemData
	if req.Msg.ToStatus != "" {
		toStatus := session.BacklogStatus(req.Msg.ToStatus)
		currentItem, currentErr := s.storage.GetBacklogItem(ctx, itemID)
		if currentErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load item for transition: %w", currentErr))
		}
		from := session.BacklogStatus(currentItem.Status)
		if !session.CanTransitionBacklog(from, toStatus) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("cannot transition item from %q to %q", from, toStatus))
		}
		updated, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, toStatus, nil, session.TriggeredByUser) //nolint:silenttransition the fallback reload a few lines below returns the item's true post-transition state in the RPC response, so the caller sees the failure implicitly rather than a false "success"
		if transErr != nil {
			log.ErrorLog.Printf("[OverrideVerdict] failed to transition item %s to %s: %v", itemID, toStatus, transErr)
		} else {
			updatedItem = updated
		}
	}

	// Fall back to loading item if transition was skipped or failed.
	if updatedItem == nil {
		updatedItem, err = s.storage.GetBacklogItem(ctx, itemID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item: %w", err))
		}
	}

	return connect.NewResponse(&sessionv1.OverrideVerdictResponse{
		Item: backlogItemToProto(updatedItem, s.buildCostLookup()),
	}), nil
}

// SubmitManualReview allows a user to submit a review verdict directly,
// without running an AI review session.
// +api: backlog:submit-manual-review
func (s *BacklogService) SubmitManualReview(
	ctx context.Context,
	req *connect.Request[sessionv1.SubmitManualReviewRequest],
) (*connect.Response[sessionv1.SubmitManualReviewResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if req.Msg.Summary == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("summary is required"))
	}

	overall := session.ReviewOutcome(req.Msg.OverallOutcome)
	if !overall.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("overall_outcome must be PASS, FAIL, PARTIAL, or UNVERIFIABLE; got %q", req.Msg.OverallOutcome))
	}

	// Load item to get AC criteria.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// Build per-criterion verdicts: use provided ones or synthesize from overall.
	var cvs []session.CriterionVerdict
	if len(req.Msg.PerCriterionVerdicts) > 0 {
		for _, pv := range req.Msg.PerCriterionVerdicts {
			cvs = append(cvs, session.CriterionVerdict{
				CriterionIndex: int(pv.CriterionIndex),
				Outcome:        session.ReviewOutcome(pv.Outcome),
				Evidence:       pv.Evidence,
			})
		}
	} else {
		// Synthesize one verdict per AC using the overall outcome.
		criteria, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
		for _, ac := range criteria {
			cvs = append(cvs, session.CriterionVerdict{
				CriterionIndex: ac.Index,
				Outcome:        overall,
				Evidence:       fmt.Sprintf("Manual review: %s", req.Msg.Summary),
			})
		}
	}

	perCriterionJSON, _ := json.Marshal(cvs)
	now := time.Now()

	// Create a synthetic review ItemSession + verdict atomically.
	syntheticUUID := "manual-review-" + req.Msg.ItemId[:8] + "-" + fmt.Sprintf("%d", now.UnixNano())
	is, createErr := s.storage.CreateItemSessionWithVerdict(ctx, session.ItemSessionData{
		ItemID:      req.Msg.ItemId,
		SessionUUID: syntheticUUID,
		SessionRole: session.SessionRoleReview,
		AcSnapshot:  item.AcceptanceCriteria,
	}, session.ReviewVerdictData{
		OverallOutcome: overall,
		PerCriterion:   string(perCriterionJSON),
		Summary:        req.Msg.Summary,
		OverrideBy:     "user",
		OverrideReason: "manual review",
		OverrideAt:     &now,
	})
	if createErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save manual review verdict: %w", createErr))
	}
	if endErr := s.storage.UpdateItemSessionEnded(ctx, is.ID, now); endErr != nil { //nolint:silenttransition bookkeeping timestamp only; the PASS/done transition below (which does notify on failure) is what actually gates forward progress here
		log.WarningLog.Printf("[SubmitManualReview] UpdateItemSessionEnded: %v", endErr)
	}

	// If PASS, transition item to done (only from review status) — but only once
	// the most recent work commit is verified on main. A PASS verdict says the
	// code is good, not that it has shipped; a manual review here must not
	// silently mark an item done for code that's still sitting in an open PR.
	// The item's "Ship PR" action (backlog_service_ship.go) is the intended
	// recovery path once left here (docs/tasks/backlog-feature-improvement.md,
	// 2026-07-18 update).
	if overall == session.ReviewVerdictPass {
		if item.Status == string(session.BacklogStatusReview) {
			if !s.isCodeShippedToMain(ctx, req.Msg.ItemId, item.RepoPath, "SubmitManualReview") {
				log.InfoLog.Printf("[SubmitManualReview] item=%s PASS verdict but code not verified on main — leaving in review for manual transition/override", req.Msg.ItemId)
			} else {
				precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReview)}
				if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId, session.BacklogStatusDone, precondition, session.TriggeredByUser); transErr != nil {
					log.WarningLog.Printf("[SubmitManualReview] PASS but transition to done failed: %v", transErr)
					// Same shape as TriggerReReview's PASS->done path: code is
					// confirmed shipped to main but the item is left stuck in review.
					s.notifyTransitionFailed(req.Msg.ItemId, item.Title, "a manual PASS verdict was submitted and code was confirmed shipped to main, but the item's transition to done failed", transErr)
				}
			}
		}
	}

	// Reload item to return updated state.
	updated, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.SubmitManualReviewResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
}
