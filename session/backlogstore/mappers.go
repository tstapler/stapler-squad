package backlogstore

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// reviewVerdictToSummary maps an *ent.ReviewVerdict to a ReviewVerdictSummary DTO.
// Returns nil when rv is nil so callers can check the pointer directly.
func reviewVerdictToSummary(rv *ent.ReviewVerdict) *backlogdto.ReviewVerdictSummary {
	if rv == nil {
		return nil
	}
	return &backlogdto.ReviewVerdictSummary{
		ID:                rv.ID.String(),
		OverallOutcome:    rv.OverallOutcome,
		PerCriterion:      rv.PerCriterion,
		Summary:           rv.Summary,
		DiffHash:          rv.DiffHash,
		DiffTokenCount:    rv.DiffTokenCount,
		DiffTruncated:     rv.DiffTruncated,
		ReviewedCommitSha: rv.ReviewedCommitSha,
		OverrideBy:        rv.OverrideBy,
		OverrideReason:    rv.OverrideReason,
		OverrideAt:        rv.OverrideAt,
		CreatedAt:         rv.CreatedAt,
	}
}

// itemSessionToSummary maps an *ent.ItemSession to an ItemSessionSummary DTO.
// The BacklogItem edge must be eagerly loaded (WithBacklogItem) for BacklogItemID to be populated.
// The ReviewVerdict edge must be eagerly loaded (WithReviewVerdict) for ReviewVerdict to be non-nil.
func ItemSessionToSummary(is *ent.ItemSession) backlogdto.ItemSessionSummary {
	var backlogItemID string
	if is.Edges.BacklogItem != nil {
		backlogItemID = is.Edges.BacklogItem.ID.String()
	}

	// Parse TriageResultSummary from the triage_result JSON column.
	var triageResultSummary string
	if is.TriageResult != "" {
		var tr struct {
			Summary string `json:"summary"`
		}
		_ = json.Unmarshal([]byte(is.TriageResult), &tr)
		triageResultSummary = tr.Summary
	}

	// OverallOutcome from the linked ReviewVerdict (empty when no verdict exists yet).
	var overallOutcome string
	if is.Edges.ReviewVerdict != nil {
		overallOutcome = is.Edges.ReviewVerdict.OverallOutcome
	}

	return backlogdto.ItemSessionSummary{
		ID:                       is.ID.String(),
		BacklogItemID:            backlogItemID,
		SessionUUID:              is.SessionUUID,
		Role:                     is.SessionRole,
		BranchName:               is.BranchName,
		AcSnapshot:               domain.AcCriteriaJSON(is.AcSnapshot),
		PipelineModeSnapshot:     is.PipelineModeSnapshot,
		PipelineModeSnapshotHash: is.PipelineModeSnapshotHash,
		ResolvedProgram:          is.ResolvedProgram,
		ResolvedModel:            is.ResolvedModel,
		ExecutorSnapshotHash:     is.ExecutorSnapshotHash,
		ConfiguredProgram:        is.ConfiguredProgram,
		ExecutorFallbackReason:   is.ExecutorFallbackReason,
		BaseCommitSha:            is.BaseCommitSha,
		LastCommitSha:            is.LastCommitSha,
		LastCommitMessage:        is.LastCommitMessage,
		CommitCountSinceSpawn:    is.CommitCountSinceSpawn,
		StartedAt:                is.StartedAt,
		EndedAt:                  is.EndedAt,
		EndReason:                is.EndReason,
		FailureCapturePath:       is.FailureCapturePath,
		ErrorDetail:              is.ErrorDetail,
		LastCommitAt:             is.LastCommitAt,
		LastFileTouchAt:          is.LastFileTouchAt,
		LastProgressAt:           is.LastProgressAt,
		CreatedAt:                is.CreatedAt,
		EstimatedCostUsd:         is.EstimatedCostUsd,
		CostPriced:               is.CostPriced,
		TriageResult:             is.TriageResult,
		TriageResultSummary:      triageResultSummary,
		DiagnoseResult:           is.DiagnoseResult,
		VerificationNotes:        is.VerificationNotes,
		DiffTruncated:            is.DiffTruncated,
		OverallOutcome:           overallOutcome,
		ReviewVerdict:            reviewVerdictToSummary(is.Edges.ReviewVerdict),
		ClaimantHostID:           is.ClaimantHostID,
	}
}

// BacklogStatusEventToData maps an *ent.BacklogStatusEvent to a BacklogStatusEventData DTO.
func BacklogStatusEventToData(e *ent.BacklogStatusEvent) backlogdto.BacklogStatusEventData {
	return backlogdto.BacklogStatusEventData{
		ID:                         e.ID.String(),
		FromStatus:                 e.FromStatus,
		ToStatus:                   e.ToStatus,
		TriggeredBy:                e.TriggeredBy,
		Note:                       e.Note,
		StageNameSnapshot:          e.StageNameSnapshot,
		AllowedTransitionsSnapshot: e.AllowedTransitionsSnapshot,
		CreatedAt:                  e.CreatedAt,
	}
}

// progressNoteToData maps an *ent.BacklogProgressNote to a ProgressNoteData DTO.
func progressNoteToData(n *ent.BacklogProgressNote) backlogdto.ProgressNoteData {
	return backlogdto.ProgressNoteData{
		ID:             n.ID.String(),
		CriterionIndex: n.CriterionIndex,
		Note:           n.Note,
		Status:         n.Status,
		CreatedAt:      n.CreatedAt,
	}
}

// ActivityNoteToData maps an *ent.BacklogActivityNote to an ActivityNoteData DTO.
func ActivityNoteToData(n *ent.BacklogActivityNote) backlogdto.ActivityNoteData {
	return backlogdto.ActivityNoteData{
		ID:                 n.ID.String(),
		Message:            n.Message,
		AuthorSessionUUID:  n.AuthorSessionUUID,
		AuthorSessionTitle: n.AuthorSessionTitle,
		CreatedAt:          n.CreatedAt,
	}
}

func BacklogItemToData(item *ent.BacklogItem) backlogdto.BacklogItemData {
	data := backlogdto.BacklogItemData{
		ID:                              item.ID.String(),
		PublicIDRaw:                     item.PublicID,
		Title:                           item.Title,
		Description:                     item.Description,
		AcceptanceCriteria:              domain.AcCriteriaJSON(item.AcceptanceCriteria),
		Priority:                        item.Priority,
		Status:                          item.Status,
		RepoPath:                        item.RepoPath,
		BaseBranch:                      item.BaseBranch,
		SkipReviewGate:                  item.SkipReviewGate,
		SkipPlanning:                    item.SkipPlanning,
		AutoSpawnSession:                item.AutoSpawnSession,
		AutoCreatePR:                    item.AutoCreatePr,
		AutoApprovePlan:                 item.AutoApprovePlan,
		PipelineMode:                    item.PipelineMode,
		Category:                        item.Category,
		PlanApproved:                    item.PlanApproved,
		PlanApprovedAt:                  item.PlanApprovedAt,
		QueuedAt:                        item.QueuedAt,
		QueuedAutonomous:                item.QueuedAutonomous,
		PlanArtifactsPath:               item.PlanArtifactsPath,
		PlanRejectionReason:             item.PlanRejectionReason,
		PlanRejectedAt:                  item.PlanRejectedAt,
		Notes:                           item.Notes,
		ExternalID:                      item.ExternalID,
		ExternalURL:                     item.ExternalURL,
		Labels:                          item.Labels,
		ArchivedAt:                      item.ArchivedAt,
		PrURL:                           item.PrURL,
		PrNumber:                        item.PrNumber,
		ShippedCheckConclusion:          item.ShippedCheckConclusion,
		ShippedApprovedCount:            item.ShippedApprovedCount,
		ShippedChangesReqCount:          item.ShippedChangesReqCount,
		ShippedSnapshotAt:               item.ShippedSnapshotAt,
		PrFeedbackAddressedAt:           item.PrFeedbackAddressedAt,
		GitHubSyncedIssueUpdatedAt:      item.GithubSyncedIssueUpdatedAt,
		ClaimingHostID:                  item.ClaimingHostID,
		GitHubLastProgressCommentStatus: item.GithubLastProgressCommentStatus,
		UserModifiedFields:              item.UserModifiedFields,
		ShippedFileStats:                item.ShippedFileStats,
		WorkflowSuggestions:             item.WorkflowSuggestions,
		ShippedSnapshotCaptureFailed:    item.ShippedSnapshotCaptureFailed,
		ReworkCapOverride:               item.ReworkCapOverride,
		IsIntegrationChild:              item.IsIntegrationChild,
		IntegrationChildID:              uuidPtrString(item.IntegrationChildID),
		IntegrationFixCount:             item.IntegrationFixCount,
		CostBudgetThresholdUsd:          item.CostBudgetThresholdUsd,
		NextWorkflowID:                  item.NextWorkflowID,
		ChainFired:                      item.ChainFired,
		ChainedAt:                       item.ChainedAt,
		TriggeredByChainDepth:           item.TriggeredByChainDepth,
		CreatedAt:                       item.CreatedAt,
		UpdatedAt:                       item.UpdatedAt,
	}
	// Resolve source ID from the eager-loaded edge when available.
	if item.Edges.Source != nil {
		data.SourceID = item.Edges.Source.ID.String()
		data.SourcePluginID = item.Edges.Source.PluginID
	}
	// Propagate eagerly-loaded status events when present.
	if item.Edges.StatusEvents != nil {
		data.StatusEvents = make([]backlogdto.BacklogStatusEventData, len(item.Edges.StatusEvents))
		for i, ev := range item.Edges.StatusEvents {
			data.StatusEvents[i] = BacklogStatusEventToData(ev)
		}
	}
	// Propagate eagerly-loaded progress notes when present (see StatusEvents above).
	if item.Edges.ProgressNotes != nil {
		data.ProgressNotes = make([]backlogdto.ProgressNoteData, len(item.Edges.ProgressNotes))
		for i, n := range item.Edges.ProgressNotes {
			data.ProgressNotes[i] = progressNoteToData(n)
		}
	}
	// Propagate eagerly-loaded activity notes when present (see StatusEvents above).
	if item.Edges.ActivityNotes != nil {
		data.ActivityNotes = make([]backlogdto.ActivityNoteData, len(item.Edges.ActivityNotes))
		for i, n := range item.Edges.ActivityNotes {
			data.ActivityNotes[i] = ActivityNoteToData(n)
		}
	}
	// Propagate eagerly-loaded item sessions when present. Note: the
	// ReviewVerdict sub-edge must ALSO have been loaded on each item.Edges.ItemSessions[i]
	// (e.g. via .WithItemSessions(func(q) { q.WithReviewVerdict() })) for
	// gateVerdict/gateVerdictSummary/triageStatus — which the frontend derives
	// entirely from ItemSessions (web-app/src/lib/hooks/useBacklogService.ts's
	// mapBacklogItem) — to come through non-empty. Most callers that need this
	// populated correctly go through AttachItemSessionsForPublish instead of
	// relying on this edge being loaded on the entity passed in here.
	if item.Edges.ItemSessions != nil {
		data.ItemSessions = make([]backlogdto.ItemSessionSummary, len(item.Edges.ItemSessions))
		for i, is := range item.Edges.ItemSessions {
			s := ItemSessionToSummary(is)
			s.BacklogItemID = data.ID
			data.ItemSessions[i] = s
		}
	}
	return data
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
