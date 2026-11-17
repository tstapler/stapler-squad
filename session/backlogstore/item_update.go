package backlogstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// UpdateBacklogItem modifies an existing backlog item with optional precondition check.
func (b *Backlog) UpdateBacklogItem(ctx context.Context, id string, update backlogdto.BacklogItemUpdate, precondition *backlogdto.BacklogItemPrecondition) (*backlogdto.BacklogItemData, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	// One transaction: the criteria read-merge-write below must not race a concurrent waiver.
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin backlog item update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Fetch current item for precondition check.
	current, err := tx.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	if precondition != nil {
		if precondition.ExpectedStatus != "" && current.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", domain.ErrPreconditionFailed, precondition.ExpectedStatus, current.Status)
		}
		if precondition.ExpectedUpdatedAt != nil && !current.UpdatedAt.Equal(*precondition.ExpectedUpdatedAt) {
			return nil, fmt.Errorf("%w: updated_at mismatch", domain.ErrPreconditionFailed)
		}
	}

	var clearedWaivers []domain.AcCriterion
	u := tx.BacklogItem.UpdateOneID(parsedID)
	if update.Title != nil {
		u.SetTitle(*update.Title)
	}
	if update.Description != nil {
		u.SetDescription(*update.Description)
	}
	if update.AcceptanceCriteria != nil {
		merged, cleared, rejectErr := b.reconcileAcUpdate(ctx, tx, current, *update.AcceptanceCriteria)
		if rejectErr != nil {
			if commitErr := tx.Commit(); commitErr != nil { // keep the rejection's audit row
				return nil, errors.Join(rejectErr, commitErr)
			}
			return nil, rejectErr
		}
		u.SetAcceptanceCriteria(string(merged))
		clearedWaivers = cleared
	}
	applyBacklogItemConfigUpdate(u, update)
	applyBacklogItemPlanAndPRUpdate(u, update)
	applyBacklogItemShippedAndChainUpdate(u, update)

	item, err := u.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update backlog item %s: %w", id, err)
	}
	for _, c := range clearedWaivers {
		if err := b.CreateAcAuditEvent(ctx, tx.AcAuditEvent, backlogdto.AcAuditEventData{
			ItemID: parsedID.String(), ItemPublicID: item.PublicID, CriterionIndex: c.Index,
			PreviousStatus: string(domain.AcStatusWaived), Reason: c.WaiverReason,
			Outcome: "waiver_cleared:criterion_removed_or_text_changed",
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit backlog item update: %w", err)
	}
	result := BacklogItemToData(item)

	// Best-effort publish: never blocks or fails the update itself.
	b.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:          backlogdto.ChangeItemUpdated,
		UpdatedFields: updatedFieldsFromBacklogItemUpdate(update),
	})

	result.ClearedAcWaivers = clearedWaivers
	return &result, nil
}

// reconcileAcUpdate applies the waiver invariant to a whole-list criteria rewrite inside tx and
// returns the list to store plus the waivers that did not survive (to be audited on commit). A
// refused rewrite writes its own audit row on tx before returning the error.
func (b *Backlog) reconcileAcUpdate(ctx context.Context, tx *ent.Tx, current *ent.BacklogItem, incomingJSON domain.AcCriteriaJSON) (domain.AcCriteriaJSON, []domain.AcCriterion, error) {
	incoming, err := domain.ParseAcCriteria(incomingJSON)
	if err != nil {
		return "", nil, fmt.Errorf("invalid acceptance criteria: %w", err)
	}
	existing, _ := domain.ParseAcCriteria(domain.AcCriteriaJSON(current.AcceptanceCriteria)) // an unreadable stored list pins nothing
	merged, cleared, err := backlogdto.ReconcileAcWaivers(existing, incoming)
	if err != nil {
		if auditErr := b.CreateAcAuditEvent(ctx, tx.AcAuditEvent, backlogdto.AcAuditEventData{
			ItemID: current.ID.String(), ItemPublicID: current.PublicID, NewStatus: string(domain.AcStatusWaived),
			Outcome: "rejected:" + backlogdto.ErrAcWaiverViaUpdate.Error(),
		}); auditErr != nil {
			return "", nil, errors.Join(err, auditErr)
		}
		return "", nil, err
	}
	if len(merged) == 0 {
		return incomingJSON, cleared, nil
	}
	out, err := domain.SerializeAcCriteria(merged)
	if err != nil {
		return "", nil, fmt.Errorf("serialize acceptance criteria: %w", err)
	}
	return out, cleared, nil
}

// updatedFieldsFromBacklogItemUpdate computes which fields a BacklogItemUpdate
// actually sets (non-nil pointer params), for the ChangeItemUpdated publish
// hook (Story 2.2.1). Field names use lowerCamelCase to match the JSON/proto
// naming convention used elsewhere on the wire. A call with every field left
// nil (a no-op update) returns an empty, non-nil slice rather than fabricating
// any field name.
func updatedFieldsFromBacklogItemUpdate(update backlogdto.BacklogItemUpdate) []string {
	fields := make([]string, 0, 24)
	fields = appendConfigUpdatedFields(update, fields)
	fields = appendPlanAndPRUpdatedFields(update, fields)
	return appendShippedAndChainUpdatedFields(update, fields)
}

func applyBacklogItemConfigUpdate(u *ent.BacklogItemUpdateOne, update backlogdto.BacklogItemUpdate) {
	if update.Priority != nil {
		u.SetPriority(*update.Priority)
	}
	if update.RepoPath != nil {
		u.SetRepoPath(*update.RepoPath)
	}
	if update.BaseBranch != nil {
		u.SetBaseBranch(*update.BaseBranch)
	}
	if update.SkipReviewGate != nil {
		u.SetSkipReviewGate(*update.SkipReviewGate)
	}
	if update.SkipPlanning != nil {
		u.SetSkipPlanning(*update.SkipPlanning)
	}
	if update.AutoSpawnSession != nil {
		u.SetAutoSpawnSession(*update.AutoSpawnSession)
	}
	if update.AutoCreatePR != nil {
		u.SetAutoCreatePr(*update.AutoCreatePR)
	}
	if update.AutoApprovePlan != nil {
		u.SetAutoApprovePlan(*update.AutoApprovePlan)
	}
	if update.PipelineMode != nil {
		u.SetPipelineMode(*update.PipelineMode)
	}
	if update.Category != nil {
		u.SetCategory(*update.Category)
	}
	if update.Notes != nil {
		u.SetNotes(*update.Notes)
	}
	if update.PlanApproved != nil {
		u.SetPlanApproved(*update.PlanApproved)
	}
	if update.PlanApprovedAt != nil {
		u.SetPlanApprovedAt(*update.PlanApprovedAt)
	}
	if update.QueuedAt != nil {
		u.SetQueuedAt(*update.QueuedAt)
	}
	if update.QueuedAutonomous != nil {
		u.SetQueuedAutonomous(*update.QueuedAutonomous)
	}
}

func applyBacklogItemPlanAndPRUpdate(u *ent.BacklogItemUpdateOne, update backlogdto.BacklogItemUpdate) {
	if update.PlanArtifactsPath != nil {
		u.SetPlanArtifactsPath(*update.PlanArtifactsPath)
	}
	if update.PlanRejectionReason != nil {
		u.SetPlanRejectionReason(*update.PlanRejectionReason)
	}
	if update.ClearPlanRejectedAt {
		u.ClearPlanRejectedAt()
	} else if update.PlanRejectedAt != nil {
		u.SetPlanRejectedAt(*update.PlanRejectedAt)
	}
	if update.PrURL != nil {
		u.SetPrURL(*update.PrURL)
	}
	if update.PrNumber != nil {
		u.SetPrNumber(*update.PrNumber)
	}
	if update.ShippedCheckConclusion != nil {
		u.SetShippedCheckConclusion(*update.ShippedCheckConclusion)
	}
	if update.ShippedApprovedCount != nil {
		u.SetShippedApprovedCount(*update.ShippedApprovedCount)
	}
	if update.ShippedChangesReqCount != nil {
		u.SetShippedChangesReqCount(*update.ShippedChangesReqCount)
	}
	if update.ShippedSnapshotAt != nil {
		u.SetShippedSnapshotAt(*update.ShippedSnapshotAt)
	}
	if update.ClearPrFeedbackAddressedAt {
		u.ClearPrFeedbackAddressedAt()
	} else if update.PrFeedbackAddressedAt != nil {
		u.SetPrFeedbackAddressedAt(*update.PrFeedbackAddressedAt)
	}
}

func applyBacklogItemShippedAndChainUpdate(u *ent.BacklogItemUpdateOne, update backlogdto.BacklogItemUpdate) {
	if update.ShippedFileStats != nil {
		u.SetShippedFileStats(*update.ShippedFileStats)
	}
	if update.ShippedSnapshotCaptureFailed != nil {
		u.SetShippedSnapshotCaptureFailed(*update.ShippedSnapshotCaptureFailed)
	}
	if update.ReworkCapOverride != nil {
		u.SetReworkCapOverride(*update.ReworkCapOverride)
	}
	if update.CostBudgetThresholdUsd != nil {
		u.SetCostBudgetThresholdUsd(*update.CostBudgetThresholdUsd)
	}
	if update.ExternalURL != nil {
		u.SetExternalURL(*update.ExternalURL)
	}
	if update.Labels != nil {
		u.SetLabels(*update.Labels)
	}
	if update.ClearGitHubSyncedIssueUpdatedAt {
		u.ClearGithubSyncedIssueUpdatedAt()
	} else if update.GitHubSyncedIssueUpdatedAt != nil {
		u.SetGithubSyncedIssueUpdatedAt(*update.GitHubSyncedIssueUpdatedAt)
	}
	if update.GitHubLastProgressCommentStatus != nil {
		u.SetGithubLastProgressCommentStatus(*update.GitHubLastProgressCommentStatus)
	}
	if update.UserModifiedFields != nil {
		u.SetUserModifiedFields(*update.UserModifiedFields)
	}
	if update.ClearNextWorkflowID {
		u.ClearNextWorkflowID()
	} else if update.NextWorkflowID != nil {
		u.SetNextWorkflowID(*update.NextWorkflowID)
	}
	if update.ChainFired != nil {
		u.SetChainFired(*update.ChainFired)
	}
	if update.ClearChainedAt {
		u.ClearChainedAt()
	} else if update.ChainedAt != nil {
		u.SetChainedAt(*update.ChainedAt)
	}
	if update.TriggeredByChainDepth != nil {
		u.SetTriggeredByChainDepth(*update.TriggeredByChainDepth)
	}
}

func appendConfigUpdatedFields(update backlogdto.BacklogItemUpdate, fields []string) []string {
	if update.Title != nil {
		fields = append(fields, "title")
	}
	if update.Description != nil {
		fields = append(fields, "description")
	}
	if update.AcceptanceCriteria != nil {
		fields = append(fields, "acceptanceCriteria")
	}
	if update.Priority != nil {
		fields = append(fields, "priority")
	}
	if update.RepoPath != nil {
		fields = append(fields, "repoPath")
	}
	if update.BaseBranch != nil {
		fields = append(fields, "baseBranch")
	}
	if update.SkipReviewGate != nil {
		fields = append(fields, "skipReviewGate")
	}
	if update.SkipPlanning != nil {
		fields = append(fields, "skipPlanning")
	}
	if update.AutoSpawnSession != nil {
		fields = append(fields, "autoSpawnSession")
	}
	if update.AutoCreatePR != nil {
		fields = append(fields, "autoCreatePR")
	}
	if update.AutoApprovePlan != nil {
		fields = append(fields, "autoApprovePlan")
	}
	if update.PipelineMode != nil {
		fields = append(fields, "pipelineMode")
	}
	if update.Category != nil {
		fields = append(fields, "category")
	}
	if update.Notes != nil {
		fields = append(fields, "notes")
	}
	if update.PlanApproved != nil {
		fields = append(fields, "planApproved")
	}
	if update.PlanApprovedAt != nil {
		fields = append(fields, "planApprovedAt")
	}
	if update.QueuedAt != nil {
		fields = append(fields, "queuedAt")
	}
	if update.QueuedAutonomous != nil {
		fields = append(fields, "queuedAutonomous")
	}
	if update.PlanArtifactsPath != nil {
		fields = append(fields, "planArtifactsPath")
	}
	if update.PlanRejectionReason != nil {
		fields = append(fields, "planRejectionReason")
	}
	return fields
}

func appendPlanAndPRUpdatedFields(update backlogdto.BacklogItemUpdate, fields []string) []string {
	if update.PlanRejectedAt != nil || update.ClearPlanRejectedAt {
		fields = append(fields, "planRejectedAt")
	}
	if update.PrURL != nil {
		fields = append(fields, "prUrl")
	}
	if update.PrNumber != nil {
		fields = append(fields, "prNumber")
	}
	if update.ShippedCheckConclusion != nil {
		fields = append(fields, "shippedCheckConclusion")
	}
	if update.ShippedApprovedCount != nil {
		fields = append(fields, "shippedApprovedCount")
	}
	if update.ShippedChangesReqCount != nil {
		fields = append(fields, "shippedChangesReqCount")
	}
	if update.ShippedSnapshotAt != nil {
		fields = append(fields, "shippedSnapshotAt")
	}
	if update.PrFeedbackAddressedAt != nil || update.ClearPrFeedbackAddressedAt {
		fields = append(fields, "prFeedbackAddressedAt")
	}
	if update.ShippedFileStats != nil {
		fields = append(fields, "shippedFileStats")
	}
	if update.ShippedSnapshotCaptureFailed != nil {
		fields = append(fields, "shippedSnapshotCaptureFailed")
	}
	if update.ReworkCapOverride != nil {
		fields = append(fields, "reworkCapOverride")
	}
	if update.CostBudgetThresholdUsd != nil {
		fields = append(fields, "costBudgetThresholdUsd")
	}
	if update.ExternalURL != nil {
		fields = append(fields, "externalUrl")
	}
	if update.Labels != nil {
		fields = append(fields, "labels")
	}
	return fields
}

func appendShippedAndChainUpdatedFields(update backlogdto.BacklogItemUpdate, fields []string) []string {
	if update.GitHubSyncedIssueUpdatedAt != nil || update.ClearGitHubSyncedIssueUpdatedAt {
		fields = append(fields, "gitHubSyncedIssueUpdatedAt")
	}
	if update.GitHubLastProgressCommentStatus != nil {
		fields = append(fields, "githubLastProgressCommentStatus")
	}
	if update.UserModifiedFields != nil {
		fields = append(fields, "userModifiedFields")
	}
	if update.NextWorkflowID != nil || update.ClearNextWorkflowID {
		fields = append(fields, "nextWorkflowId")
	}
	if update.ChainFired != nil {
		fields = append(fields, "chainFired")
	}
	if update.ChainedAt != nil || update.ClearChainedAt {
		fields = append(fields, "chainedAt")
	}
	if update.TriggeredByChainDepth != nil {
		fields = append(fields, "triggeredByChainDepth")
	}
	return fields
}
