package backlogdto

import (
	"errors"
	"time"
)

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
