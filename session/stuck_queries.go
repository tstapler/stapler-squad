package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
)

// FindOpenStuckStates returns every BacklogStuckState row that is currently
// open (resolved_at IS NULL) and not currently snoozed (snoozed_until IS NULL
// OR snoozed_until is in the past), eager-loading the parent item so the
// projection carries title/status/pr_number/pr_url for rendering without a
// second round trip.
func (r *EntRepository) FindOpenStuckStates(ctx context.Context) ([]backlogdto.OpenStuckStateData, error) {
	now := time.Now()
	rows, err := r.Client.BacklogStuckState.Query().
		Where(
			backlogstuckstate.ResolvedAtIsNil(),
			backlogstuckstate.Or(
				backlogstuckstate.SnoozedUntilIsNil(),
				backlogstuckstate.SnoozedUntilLT(now),
			),
		).
		WithItem().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query open stuck states: %w", err)
	}

	result := make([]backlogdto.OpenStuckStateData, 0, len(rows))
	for _, row := range rows {
		data := backlogdto.OpenStuckStateData{
			ID:                     row.ID.String(),
			ItemID:                 row.ItemID.String(),
			Reason:                 domain.StuckReason(row.Reason),
			FirstDetectedAt:        row.FirstDetectedAt,
			LastCheckedAt:          row.LastCheckedAt,
			NotifiedAt:             row.NotifiedAt,
			Context:                row.Context,
			RemediationAttempts:    row.RemediationAttempts,
			NextRemediationAt:      row.NextRemediationAt,
			GraceBootTime:          row.GraceBootTime,
			DiagnoseNudgeCount:     row.DiagnoseNudgeCount,
			DiagnoseNextEligibleAt: row.DiagnoseNextEligibleAt,
		}
		if item := row.Edges.Item; item != nil {
			data.ItemTitle = item.Title
			data.ItemStatus = domain.BacklogStatus(item.Status)
			data.PrNumber = item.PrNumber
			data.PrURL = item.PrURL
			data.PlanArtifactsPath = item.PlanArtifactsPath
		}
		result = append(result, data)
	}
	return result, nil
}

// FindSnoozedStuckItemIDs returns the item IDs whose open row of reason is snoozed right
// now. FindOpenStuckStates hides those rows, so a gate that must outlive a snooze reads
// them here.
func (r *EntRepository) FindSnoozedStuckItemIDs(ctx context.Context, reason domain.StuckReason) ([]string, error) {
	rows, err := r.Client.BacklogStuckState.Query().
		Where(
			backlogstuckstate.ResolvedAtIsNil(),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.SnoozedUntilGT(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query snoozed stuck states: %w", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ItemID.String())
	}
	return ids, nil
}

// ListStuckStatesForItem returns every BacklogStuckState row for itemID,
// open or resolved, ordered oldest-first. Unlike FindOpenStuckStates this
// applies no resolved_at/snoozed_until filter — it's a single-item history
// read, not a sweep's work queue.
func (r *EntRepository) ListStuckStatesForItem(ctx context.Context, itemID string) ([]backlogdto.StuckStateHistoryData, error) {
	id, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id: %w", err)
	}
	rows, err := r.Client.BacklogStuckState.Query().
		Where(backlogstuckstate.ItemIDEQ(id)).
		Order(ent.Asc(backlogstuckstate.FieldFirstDetectedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query stuck states for item %s: %w", itemID, err)
	}
	result := make([]backlogdto.StuckStateHistoryData, 0, len(rows))
	for _, row := range rows {
		result = append(result, backlogdto.StuckStateHistoryData{
			ID:                  row.ID.String(),
			Reason:              domain.StuckReason(row.Reason),
			FirstDetectedAt:     row.FirstDetectedAt,
			LastCheckedAt:       row.LastCheckedAt,
			ResolvedAt:          row.ResolvedAt,
			Context:             row.Context,
			RemediationAttempts: row.RemediationAttempts,
			DiagnoseNudgeCount:  row.DiagnoseNudgeCount,
		})
	}
	return result, nil
}
