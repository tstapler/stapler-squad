package backlogstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// ParseDiagnoseResult decodes an ItemSessionSummary.DiagnoseResult column; ok is false when empty or invalid.
func ParseDiagnoseResult(raw string) (backlogdto.DiagnoseResultData, bool) {
	var d backlogdto.DiagnoseResultData
	if raw == "" || json.Unmarshal([]byte(raw), &d) != nil {
		return backlogdto.DiagnoseResultData{}, false
	}
	return d, true
}

// UpdateItemSessionDiagnoseResult stores the structured diagnose outcome on an ItemSession.
func (b *Backlog) UpdateItemSessionDiagnoseResult(ctx context.Context, id string, result backlogdto.DiagnoseResultData) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal diagnose_result: %w", err)
	}
	if _, err := b.client.ItemSession.UpdateOneID(parsedID).SetDiagnoseResult(string(payload)).Save(ctx); err != nil {
		return fmt.Errorf("failed to update diagnose_result on item session %s: %w", id, err)
	}
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionDiagnoseResult: owning item lookup for %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated, UpdatedFields: []string{"diagnoseResult"}})
	}
	return nil
}

// AppendWorkflowSuggestion adds a suggestion to the item atomically and returns its id.
func (b *Backlog) AppendWorkflowSuggestion(ctx context.Context, itemID string, sug backlogdto.WorkflowSuggestionData) (string, error) {
	sug.SuggestionID = uuid.NewString()
	if sug.CreatedAt.IsZero() {
		sug.CreatedAt = time.Now().UTC()
	}
	err := b.mutateWorkflowSuggestions(ctx, itemID, func(list []backlogdto.WorkflowSuggestionData) ([]backlogdto.WorkflowSuggestionData, error) {
		active := 0
		for _, e := range list {
			if e.Dismissed {
				continue
			}
			if e.Recommendation == sug.Recommendation {
				return nil, fmt.Errorf("%w: %q", backlogdto.ErrDuplicateWorkflowSuggestion, sug.Recommendation)
			}
			active++
		}
		if active >= backlogdto.MaxActiveWorkflowSuggestions {
			return nil, fmt.Errorf("%w (max %d); dismiss some first", backlogdto.ErrTooManyWorkflowSuggestions, backlogdto.MaxActiveWorkflowSuggestions)
		}
		return append(list, sug), nil
	})
	if err != nil {
		return "", err
	}
	return sug.SuggestionID, nil
}

// DismissWorkflowSuggestion marks a suggestion dismissed; ErrNotFound if the id is unknown.
func (b *Backlog) DismissWorkflowSuggestion(ctx context.Context, itemID, suggestionID string) error {
	return b.mutateWorkflowSuggestions(ctx, itemID, func(list []backlogdto.WorkflowSuggestionData) ([]backlogdto.WorkflowSuggestionData, error) {
		for i := range list {
			if list[i].SuggestionID == suggestionID {
				list[i].Dismissed = true
				return list, nil
			}
		}
		return nil, fmt.Errorf("workflow suggestion %s not found on item %s: %w", suggestionID, itemID, domain.ErrNotFound)
	})
}

// mutateWorkflowSuggestions read-modify-writes the JSON column in one transaction.
func (b *Backlog) mutateWorkflowSuggestions(ctx context.Context, itemID string, mutate func([]backlogdto.WorkflowSuggestionData) ([]backlogdto.WorkflowSuggestionData, error)) error {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin workflow-suggestion tx: %w", err)
	}
	item, err := tx.BacklogItem.Get(ctx, parsedItemID)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("backlog item %s not found: %w", itemID, domain.ErrNotFound)
	}
	updated, err := mutate(ParseWorkflowSuggestions(item.WorkflowSuggestions))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	payload, err := json.Marshal(updated)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("marshal workflow_suggestions: %w", err)
	}
	if err := tx.BacklogItem.UpdateOneID(parsedItemID).SetWorkflowSuggestions(string(payload)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("save workflow_suggestions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow_suggestions: %w", err)
	}
	if data, getErr := b.GetBacklogItem(ctx, parsedItemID.String()); getErr == nil {
		b.PublishItemChanged(ctx, data, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated, UpdatedFields: []string{"workflowSuggestions"}})
	}
	return nil
}

// ParseWorkflowSuggestions decodes the workflow_suggestions column; a malformed value yields nil.
func ParseWorkflowSuggestions(raw string) []backlogdto.WorkflowSuggestionData {
	if raw == "" {
		return nil
	}
	var out []backlogdto.WorkflowSuggestionData
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.WarningLog().Printf("[ParseWorkflowSuggestions] invalid JSON: %v", err)
		return nil
	}
	return out
}
