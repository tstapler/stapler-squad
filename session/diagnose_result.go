package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DiagnoseResultData is the JSON stored in item_sessions.diagnose_result.
type DiagnoseResultData struct {
	Outcome     string    `json:"outcome"`
	Summary     string    `json:"summary"`
	FiledItemID string    `json:"filed_item_id,omitempty"`
	DiagnosedAt time.Time `json:"diagnosed_at"`
}

// Workflow suggestion action types accepted by suggest_workflow_action.
var WorkflowSuggestionActionTypes = []string{"transition_status", "set_config", "operator_note"}

// WorkflowSuggestionData is one entry of backlog_items.workflow_suggestions.
type WorkflowSuggestionData struct {
	SuggestionID   string    `json:"suggestion_id"`
	ActionType     string    `json:"action_type"`
	Recommendation string    `json:"recommendation"`
	SessionUUID    string    `json:"session_uuid"`
	CreatedAt      time.Time `json:"created_at"`
	Dismissed      bool      `json:"dismissed"`
}

// MaxActiveWorkflowSuggestions caps non-dismissed suggestions per item.
const MaxActiveWorkflowSuggestions = 20

var (
	ErrTooManyWorkflowSuggestions  = errors.New("too many active workflow suggestions")
	ErrDuplicateWorkflowSuggestion = errors.New("duplicate workflow suggestion")
)

// ParseDiagnoseResult decodes an ItemSessionSummary.DiagnoseResult column; ok is false when empty or invalid.
func ParseDiagnoseResult(raw string) (DiagnoseResultData, bool) {
	var d DiagnoseResultData
	if raw == "" || json.Unmarshal([]byte(raw), &d) != nil {
		return DiagnoseResultData{}, false
	}
	return d, true
}

// ParseWorkflowSuggestions decodes the workflow_suggestions column; a malformed value yields nil.
func ParseWorkflowSuggestions(raw string) []WorkflowSuggestionData {
	if raw == "" {
		return nil
	}
	var out []WorkflowSuggestionData
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.WarningLog().Printf("[ParseWorkflowSuggestions] invalid JSON: %v", err)
		return nil
	}
	return out
}

// UpdateItemSessionDiagnoseResult stores the structured diagnose outcome on an ItemSession.
func (r *EntRepository) UpdateItemSessionDiagnoseResult(ctx context.Context, id string, result DiagnoseResultData) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal diagnose_result: %w", err)
	}
	if _, err := r.Client.ItemSession.UpdateOneID(parsedID).SetDiagnoseResult(string(payload)).Save(ctx); err != nil {
		return fmt.Errorf("failed to update diagnose_result on item session %s: %w", id, err)
	}
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionDiagnoseResult: owning item lookup for %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, BacklogItemChange{Kind: ChangeItemUpdated, UpdatedFields: []string{"diagnoseResult"}})
	}
	return nil
}

// AppendWorkflowSuggestion adds a suggestion to the item atomically and returns its id.
func (r *EntRepository) AppendWorkflowSuggestion(ctx context.Context, itemID string, sug WorkflowSuggestionData) (string, error) {
	sug.SuggestionID = uuid.NewString()
	if sug.CreatedAt.IsZero() {
		sug.CreatedAt = time.Now().UTC()
	}
	err := r.mutateWorkflowSuggestions(ctx, itemID, func(list []WorkflowSuggestionData) ([]WorkflowSuggestionData, error) {
		active := 0
		for _, e := range list {
			if e.Dismissed {
				continue
			}
			if e.Recommendation == sug.Recommendation {
				return nil, fmt.Errorf("%w: %q", ErrDuplicateWorkflowSuggestion, sug.Recommendation)
			}
			active++
		}
		if active >= MaxActiveWorkflowSuggestions {
			return nil, fmt.Errorf("%w (max %d); dismiss some first", ErrTooManyWorkflowSuggestions, MaxActiveWorkflowSuggestions)
		}
		return append(list, sug), nil
	})
	if err != nil {
		return "", err
	}
	return sug.SuggestionID, nil
}

// DismissWorkflowSuggestion marks a suggestion dismissed; ErrNotFound if the id is unknown.
func (r *EntRepository) DismissWorkflowSuggestion(ctx context.Context, itemID, suggestionID string) error {
	return r.mutateWorkflowSuggestions(ctx, itemID, func(list []WorkflowSuggestionData) ([]WorkflowSuggestionData, error) {
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
func (r *EntRepository) mutateWorkflowSuggestions(ctx context.Context, itemID string, mutate func([]WorkflowSuggestionData) ([]WorkflowSuggestionData, error)) error {
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	tx, err := r.Client.Tx(ctx)
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
	if data, getErr := r.GetBacklogItem(ctx, parsedItemID.String()); getErr == nil {
		r.publishItemChanged(ctx, data, BacklogItemChange{Kind: ChangeItemUpdated, UpdatedFields: []string{"workflowSuggestions"}})
	}
	return nil
}
