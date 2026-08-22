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
			log.WarningLog().Printf("[TransitionBacklogItemStatus] ResolveStuck(%s) item=%s: %v", reason, itemID, err)
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

	triageTriggered := s.MaybeTriggerTriage(ctx, created.ID, req.Msg.SkipTriage, created.RepoPath)

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

	// Loaded once, up front: needed both to value-diff Title/Description/Priority
	// against the request (see touchedFields below — a presence-only check would
	// falsely mark them user-modified on nearly every edit, since the only
	// frontend edit form always resubmits the current title verbatim) and to
	// merge into the existing UserModifiedFields raw string.
	existing, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load backlog item: %w", err))
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

	// touchedFields is a VALUE-DIFF against the existing item's current values,
	// not a bare presence check — see plan.md Epic 0.3 Task 0.3.2b's pre-mortem
	// P1 #2 correction. The only frontend edit form always resubmits the
	// current Title verbatim regardless of which field the user actually
	// changed, so a presence-only check would falsely mark Title as
	// user-modified on nearly every edit.
	var touchedFields []string
	if req.Msg.Title != "" && req.Msg.Title != existing.Title {
		touchedFields = append(touchedFields, "title")
	}
	if req.Msg.Description != "" && req.Msg.Description != existing.Description {
		touchedFields = append(touchedFields, "description")
	}
	if req.Msg.Priority != 0 && int(req.Msg.Priority) != existing.Priority {
		touchedFields = append(touchedFields, "priority")
	}
	if len(touchedFields) > 0 {
		merged, mergeErr := session.MergeUserModifiedFields(existing.UserModifiedFields, touchedFields...)
		if mergeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to merge user modified fields: %w", mergeErr))
		}
		update.UserModifiedFields = &merged
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

	// pr_url/pr_number are presence-gated and must be set together — this is
	// the manual "associate an existing PR after the fact" escape hatch, for
	// items whose real fix landed via an out-of-band worktree with no
	// item_sessions link (so report_pr_created was never callable for them).
	// Validated up front, before any write, so a bad pair never lands a
	// partial update.
	if (req.Msg.PrUrl != nil) != (req.Msg.PrNumber != nil) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("pr_url and pr_number must both be set or both be unset"))
	}
	var associatePR bool
	var prURL string
	var prNumber int
	if req.Msg.PrUrl != nil && req.Msg.PrNumber != nil {
		associatePR = true
		prURL = *req.Msg.PrUrl
		prNumber = int(*req.Msg.PrNumber)
		// Same cheap, no-network cross-check reportPRCreated does
		// (server/mcp/tools_backlog.go) before ever touching storage: a
		// typo'd URL/number pair fails fast here.
		//
		// Known, deliberately-scoped-out gap (code review, security pass):
		// unlike reportPRCreated, this manual path does not verify pr_url's
		// owner/repo matches the item's own repo, nor call GitHub to
		// confirm the PR exists — an operator could associate a PR from an
		// unrelated project, and the automated ReconcilePRPending sweep
		// would then walk the item to "done" on that unrelated PR's merge.
		// A live GitHub check was already explicitly scoped out of v1
		// (pitfalls.md §3 — the branch-match check the agent path can do,
		// but this out-of-band, no-session path structurally cannot). A
		// local repo-name cross-check would close part of this without a
		// network call, but is new capability (resolving the item's git
		// remote), not a quick fix — tracked as a follow-up.
		ref, parseErr := session.ParseGitHubURL(prURL)
		if parseErr != nil || ref.Type != session.GitHubRefTypePR {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("pr_url is not a recognizable GitHub PR URL: %v", parseErr))
		}
		if ref.PRNumber != prNumber {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("pr_url references PR #%d but pr_number=%d was given — these must match", ref.PRNumber, prNumber))
		}
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

	if associatePR {
		// Routed through the shared primary-write path — not a hand-rolled
		// field write — so the status transition (review -> pr_pending) and
		// the PR fields land as one atomic UPDATE, same as report_pr_created.
		// A split write here would reopen the BUG-040 pr_pending_no_pr class
		// of bug (session/storage.go's SetBacklogItemPRAndTransition doc
		// comment). That atomicity is scoped to THIS write alone, not the
		// whole request: this is a second, separate write from the general
		// field update above (own precondition, own failure mode) — if a
		// caller combines regular field changes with pr_url/pr_number and
		// this second write fails, the first write's changes stay
		// committed (no rollback). Today's only caller (ManualOverrideSection)
		// never combines them meaningfully (its non-PR fields are
		// currentFlags()'s idempotent no-ops), so this has no live impact,
		// but a future caller relying on whole-request atomicity across
		// both would be surprised — flagged in code review, not silently
		// left unstated.
		// nil guard: this RPC never calls GitHub (see the scoped-out-gap
		// comment above) and so has no way to verify override_reason/merged
		// state/author for a reassignment. SetBacklogItemPRAndTransition
		// centrally enforces that a nil guard cannot reassign an
		// already-pr_pending item to a different PR — this operator path
		// still supports first-time association (review -> pr_pending) and
		// the idempotent same-PR-number no-op, same as before this change;
		// it can no longer silently swap an already-tracked PR (including a
		// merged one) with zero verification.
		note := fmt.Sprintf("Manually associated with PR #%d by operator", prNumber)
		if setErr := s.storage.SetBacklogItemPRAndTransition(ctx, updated, prURL, prNumber, note, nil); setErr != nil {
			if errors.Is(setErr, session.ErrPRReassignmentNotAllowed) {
				return nil, connect.NewError(connect.CodeFailedPrecondition,
					fmt.Errorf("cannot associate PR: item %q already has PR #%d tracked (status pr_pending) — this manual override endpoint does not support reassigning an already-tracked PR to a different one (no GitHub verification is performed here); use report_pr_created from a work session instead: %w", req.Msg.ItemId, updated.PrNumber, setErr))
			}
			if errors.Is(setErr, session.ErrPreconditionFailed) {
				current, reloadErr := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
				status := "unknown"
				if reloadErr == nil && current != nil {
					status = current.Status
				}
				return nil, connect.NewError(connect.CodeFailedPrecondition,
					fmt.Errorf("cannot associate PR: item must be in %q or %q status to link a PR, but is currently %q", session.BacklogStatusReview, session.BacklogStatusPRPending, status))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to associate PR: %w", setErr))
		}
		reloaded, reloadErr := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
		if reloadErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item after PR association: %w", reloadErr))
		}
		updated = reloaded
		s.notifyManualOverride(updated.ID, updated.Title, fmt.Sprintf("PR #%d (%s) manually linked by operator — item moved to pr_pending.", prNumber, prURL))
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

// --- UnarchiveBacklogItem ---

// UnarchiveBacklogItem clears archived_at and restores the item to "idea".
// It does not attempt to recreate worktrees deleted at archive time.
// +api: backlog:unarchive-item
func (s *BacklogService) UnarchiveBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.UnarchiveBacklogItemRequest],
) (*connect.Response[sessionv1.UnarchiveBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	unarchived, err := s.storage.UnarchiveBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unarchive backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.UnarchiveBacklogItemResponse{
		Item: backlogItemToProto(unarchived, s.buildCostLookup()),
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

// --- AddBacklogItemDependency ---

// AddBacklogItemDependency marks BlockedItemId as depending on (blocked by)
// BlockerItemId, so DequeueNextQueuedItems skips it until the blocker
// resolves (reaches done or archived status, or is deleted).
// +api: backlog:add-item-dependency
func (s *BacklogService) AddBacklogItemDependency(
	ctx context.Context,
	req *connect.Request[sessionv1.AddBacklogItemDependencyRequest],
) (*connect.Response[sessionv1.AddBacklogItemDependencyResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	edge := session.BacklogItemDependencyEdge{
		BlockerID: req.Msg.BlockerItemId,
		BlockedID: req.Msg.BlockedItemId,
	}
	if err := s.storage.AddBacklogItemDependency(ctx, edge); err != nil {
		if errors.Is(err, session.ErrDependencyCycle) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("dependency would create a cycle: %w", err))
		}
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add backlog item dependency: %w", err))
	}

	updated, err := s.storage.GetBacklogItem(ctx, req.Msg.BlockedItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item after adding dependency: %w", err))
	}

	return connect.NewResponse(&sessionv1.AddBacklogItemDependencyResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
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
		log.WarningLog().Printf("resolveLatestWorkCommit: no worktree data for session %s: %v", sessionUUID, err)
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
		log.WarningLog().Printf("resolveLatestWorkCommit: rev-parse %s in %s: %v", wt.BranchName, repoPath, revErr)
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
		log.WarningLog().Printf("[%s] isCodeShippedToMain: failed to load item sessions for item %s: %v", logPrefix, itemID, err)
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
		log.WarningLog().Printf("[%s] isCodeShippedToMain: failed to verify commit %s on main for item %s: %v", logPrefix, lastCommitSha, itemID, mainErr)
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
	// evaluate the review→done guard (ErrVerdictRequired) and the
	// review/pr_pending→ready guard (ErrVerdictClearRequiredForReady).
	overallOutcome, verdictErr := s.storage.GetMostRecentReviewVerdictForItem(ctx, req.Msg.ItemId)
	if verdictErr != nil {
		log.WarningLog().Printf("[TransitionBacklogItemStatus] failed to load review verdict for item %s: %v", req.Msg.ItemId, verdictErr)
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

	var hasUnresolvedBlockers bool
	if to == session.BacklogStatusInProgress {
		hasUnresolvedBlockers, err = s.hasUnresolvedBlockers(ctx, req.Msg.ItemId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check unresolved blockers: %w", err))
		}
	}

	// Run transition guard for business rules.
	guardInput := session.BacklogItemTransitionInput{
		Status:                from,
		AcCriteria:            item.AcceptanceCriteria,
		PlanApproved:          item.PlanApproved,
		SkipPlanning:          item.SkipPlanning,
		PlanArtifactsPath:     item.PlanArtifactsPath,
		OverallOutcome:        overallOutcome,
		OverrideReason:        req.Msg.OverrideReason,
		HasUnshippedCode:      hasUnshippedCode,
		HasUnresolvedBlockers: hasUnresolvedBlockers,
	}
	if guardErr := s.engine.ValidateGates(guardInput, to); guardErr != nil {
		if errors.Is(guardErr, session.ErrACRequired) ||
			errors.Is(guardErr, session.ErrPlanRequired) ||
			errors.Is(guardErr, session.ErrPlanArtifactsRequired) ||
			errors.Is(guardErr, session.ErrVerdictRequired) ||
			errors.Is(guardErr, session.ErrCodeNotOnMain) ||
			errors.Is(guardErr, session.ErrVerdictClearRequiredForReady) ||
			errors.Is(guardErr, session.ErrUnresolvedBlockers) {
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

	// override_reason set means this was a manual operator override (bypassing
	// TransitionGuard, per session.TransitionGuard's override handling above),
	// not a routine automated transition — make it visible, not a silent write.
	if req.Msg.OverrideReason != "" {
		note := fmt.Sprintf("Manually overridden by operator: %s -> %s (%s)", from, to, req.Msg.OverrideReason)
		if noteErr := s.storage.AppendProgressNote(ctx, req.Msg.ItemId, -1, note, string(to)); noteErr != nil {
			log.WarningLog().Printf("[TransitionBacklogItemStatus] failed to append override progress note for item %s: %v", req.Msg.ItemId, noteErr)
		}
		s.notifyManualOverride(updated.ID, updated.Title, fmt.Sprintf("status manually overridden %s -> %s: %s", from, to, req.Msg.OverrideReason))
	}

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
		rejectionReason := ""
		if upd, resetErr := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, session.BacklogItemUpdate{
			PlanApproved:        &planApproved,
			PlanArtifactsPath:   &planArtifactsPath,
			PlanRejectionReason: &rejectionReason,
			ClearPlanRejectedAt: true,
		}, nil); resetErr != nil {
			log.WarningLog().Printf("[TransitionBacklogItemStatus] failed to reset planning state for item %s: %v", req.Msg.ItemId, resetErr)
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
	clearedReason := ""
	update := session.BacklogItemUpdate{
		PlanApproved:        &approved,
		PlanApprovedAt:      &now,
		PlanRejectionReason: &clearedReason,
		ClearPlanRejectedAt: true,
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to approve plan: %w", err))
	}

	return connect.NewResponse(&sessionv1.ApprovePlanResponse{
		Item: backlogItemToProto(updated, s.buildCostLookup()),
	}), nil
}

// --- RejectPlan ---

// maxRejectReasonLength caps the free-text rejection reason. No RPC in this
// server currently enforces a request-size cap (see grep for WithReadMaxBytes
// across server/ — a known, pre-existing, repo-wide gap), but this is a new
// mutating write path, so it gets an explicit cap rather than waiting on that
// broader fix. Matches session.MaxNoteLength/MaxSteerMessageLength's value; a
// local constant is used here since reject-reason isn't the same domain
// concept as either of those and doesn't warrant coupling to them.
const maxRejectReasonLength = 10000

// RejectPlan records a rejection reason for the item's current plan
// artifacts and clears any existing approval. Does not itself trigger
// regeneration — see project_plans/plan-approval-ux/decisions/ADR-002.
// +api: backlog:reject-plan
func (s *BacklogService) RejectPlan(
	ctx context.Context,
	req *connect.Request[sessionv1.RejectPlanRequest],
) (*connect.Response[sessionv1.RejectPlanResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	reason := strings.TrimSpace(req.Msg.Reason)
	if reason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("reason is required"))
	}
	if len(reason) > maxRejectReasonLength {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("reason exceeds maximum length of %d bytes", maxRejectReasonLength))
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

	now := time.Now()
	approvalReset := false
	update := session.BacklogItemUpdate{
		PlanRejectionReason: &reason,
		PlanRejectedAt:      &now,
		PlanApproved:        &approvalReset,
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reject plan: %w", err))
	}

	return connect.NewResponse(&sessionv1.RejectPlanResponse{
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
	fwd := req.Msg.ForwardSyncEnabled
	update.ForwardSyncEnabled = &fwd
	bwd := req.Msg.BackwardSyncEnabled
	update.BackwardSyncEnabled = &bwd
	label := req.Msg.ForwardSyncCloseLabel
	update.ForwardSyncCloseLabel = &label
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
		// EntRepository.UpdateItemSource re-wraps ent's *ent.NotFoundError as
		// session.ErrNotFound before returning (session/ent_repository_backlog.go),
		// so the check here must match against that sentinel — ent.IsNotFound
		// would never match since the original *ent.NotFoundError is not preserved
		// in the wrap chain.
		if errors.Is(err, session.ErrNotFound) {
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
		DiffHash:       s.storage.ComputeCurrentDiffHash(ctx, itemID),
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
		// CAS-protected like every other transition write path (see
		// TransitionBacklogItemStatus above) — a nil precondition here would
		// let this unconditional write silently clobber a transition that
		// happened concurrently between the GetBacklogItem read above and
		// this write.
		updatedAt := currentItem.UpdatedAt
		precondition := &session.BacklogItemPrecondition{
			ExpectedStatus:    string(from),
			ExpectedUpdatedAt: &updatedAt,
		}
		updated, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, toStatus, precondition, session.TriggeredByUser) //nolint:silenttransition the fallback reload a few lines below returns the item's true post-transition state in the RPC response, so the caller sees the failure implicitly rather than a false "success"
		if transErr != nil {
			log.ErrorLog().Printf("[OverrideVerdict] failed to transition item %s to %s: %v", itemID, toStatus, transErr)
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
		DiffHash:       s.storage.ComputeCurrentDiffHash(ctx, req.Msg.ItemId),
		OverrideBy:     "user",
		OverrideReason: "manual review",
		OverrideAt:     &now,
	})
	if createErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save manual review verdict: %w", createErr))
	}
	if endErr := s.storage.UpdateItemSessionEnded(ctx, is.ID, now); endErr != nil { //nolint:silenttransition bookkeeping timestamp only; the PASS/done transition below (which does notify on failure) is what actually gates forward progress here
		log.WarningLog().Printf("[SubmitManualReview] UpdateItemSessionEnded: %v", endErr)
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
				log.InfoLog().Printf("[SubmitManualReview] item=%s PASS verdict but code not verified on main — leaving in review for manual transition/override", req.Msg.ItemId)
			} else {
				precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReview)}
				if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId, session.BacklogStatusDone, precondition, session.TriggeredByUser); transErr != nil {
					log.WarningLog().Printf("[SubmitManualReview] PASS but transition to done failed: %v", transErr)
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
