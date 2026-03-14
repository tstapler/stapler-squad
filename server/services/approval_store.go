package services

import (
	"fmt"
	"sync"
	"time"
)

// PermissionRequestPayload is the JSON payload from Claude Code's PermissionRequest HTTP hook.
type PermissionRequestPayload struct {
	SessionID      string                 `json:"session_id"`
	TranscriptPath string                 `json:"transcript_path"`
	Cwd            string                 `json:"cwd"`
	PermissionMode string                 `json:"permission_mode"`
	HookEventName  string                 `json:"hook_event_name"`
	ToolName       string                 `json:"tool_name"`
	ToolInput      map[string]interface{} `json:"tool_input"`
}

// ApprovalDecision is the user's response to a pending approval.
type ApprovalDecision struct {
	Behavior string // "allow" or "deny"
	Message  string // Optional reason shown to Claude on deny
}

// PendingApproval represents an in-flight hook approval waiting for a user decision.
type PendingApproval struct {
	ID              string
	SessionID       string // claude-squad session title (mapped from hook)
	ClaudeSessionID string // Claude Code's internal session_id
	ToolName        string
	ToolInput       map[string]interface{}
	Cwd             string
	PermissionMode  string
	CreatedAt       time.Time
	ExpiresAt       time.Time

	// decisionCh receives exactly one user decision. Buffered to allow non-blocking send.
	decisionCh chan ApprovalDecision
}

// ApprovalStore manages pending approval requests with thread-safe access.
type ApprovalStore struct {
	mu        sync.RWMutex
	pending   map[string]*PendingApproval // keyed by approval ID
	bySession map[string][]string         // session ID -> approval IDs
}

// NewApprovalStore creates a new empty ApprovalStore.
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		pending:   make(map[string]*PendingApproval),
		bySession: make(map[string][]string),
	}
}

// Create adds a new pending approval to the store and initializes its decision channel.
func (s *ApprovalStore) Create(a *PendingApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.ID == "" {
		return fmt.Errorf("approval ID is required")
	}
	if _, exists := s.pending[a.ID]; exists {
		return fmt.Errorf("approval %s already exists", a.ID)
	}

	a.decisionCh = make(chan ApprovalDecision, 1)
	s.pending[a.ID] = a
	s.bySession[a.SessionID] = append(s.bySession[a.SessionID], a.ID)
	return nil
}

// Get retrieves a pending approval by ID.
func (s *ApprovalStore) Get(id string) (*PendingApproval, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.pending[id]
	return a, ok
}

// GetBySession returns all pending approvals for a given session.
func (s *ApprovalStore) GetBySession(sessionID string) []*PendingApproval {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.bySession[sessionID]
	result := make([]*PendingApproval, 0, len(ids))
	for _, id := range ids {
		if a, ok := s.pending[id]; ok {
			result = append(result, a)
		}
	}
	return result
}

// ListAll returns all currently pending approvals.
func (s *ApprovalStore) ListAll() []*PendingApproval {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*PendingApproval, 0, len(s.pending))
	for _, a := range s.pending {
		result = append(result, a)
	}
	return result
}

// Resolve sends a decision to the pending approval and removes it from the store.
// Returns an error if the approval doesn't exist or was already resolved.
func (s *ApprovalStore) Resolve(id string, decision ApprovalDecision) error {
	s.mu.Lock()
	a, ok := s.pending[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("approval %s not found or already resolved", id)
	}
	delete(s.pending, id)
	s.removeFromSessionIndex(a.SessionID, id)
	s.mu.Unlock()

	// Send decision non-blocking (channel is buffered with capacity 1)
	select {
	case a.decisionCh <- decision:
		return nil
	default:
		return fmt.Errorf("approval %s channel full (already resolved)", id)
	}
}

// Remove removes an approval from the store without sending a decision.
// The pending HTTP handler will detect context cancellation or its own timeout.
func (s *ApprovalStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.pending[id]
	if !ok {
		return
	}
	delete(s.pending, id)
	s.removeFromSessionIndex(a.SessionID, id)
}

// CancelSession denies all pending approvals for a session (e.g., on restart).
func (s *ApprovalStore) CancelSession(sessionID string) []string {
	s.mu.Lock()
	ids := s.bySession[sessionID]
	var approvals []*PendingApproval
	for _, id := range ids {
		if a, ok := s.pending[id]; ok {
			approvals = append(approvals, a)
			delete(s.pending, id)
		}
	}
	delete(s.bySession, sessionID)
	s.mu.Unlock()

	var cancelled []string
	for _, a := range approvals {
		cancelled = append(cancelled, a.ID)
		select {
		case a.decisionCh <- ApprovalDecision{Behavior: "deny", Message: "Session restarted"}:
		default:
		}
	}
	return cancelled
}

// CleanupExpired removes approvals past their ExpiresAt and denies them with a timeout message.
// Returns the IDs of expired approvals.
func (s *ApprovalStore) CleanupExpired() []string {
	now := time.Now()
	s.mu.Lock()

	var expired []*PendingApproval
	for id, a := range s.pending {
		if now.After(a.ExpiresAt) {
			expired = append(expired, a)
			delete(s.pending, id)
			s.removeFromSessionIndex(a.SessionID, id)
		}
	}
	s.mu.Unlock()

	var ids []string
	for _, a := range expired {
		ids = append(ids, a.ID)
		select {
		case a.decisionCh <- ApprovalDecision{Behavior: "deny", Message: "Approval timed out. Please respond in the terminal."}:
		default:
		}
	}
	return ids
}

// removeFromSessionIndex removes an approval ID from the bySession index.
// Must be called with mu held.
func (s *ApprovalStore) removeFromSessionIndex(sessionID, approvalID string) {
	ids := s.bySession[sessionID]
	newIDs := ids[:0]
	for _, id := range ids {
		if id != approvalID {
			newIDs = append(newIDs, id)
		}
	}
	if len(newIDs) == 0 {
		delete(s.bySession, sessionID)
	} else {
		s.bySession[sessionID] = newIDs
	}
}
