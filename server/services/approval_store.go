package services

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	SessionID       string // stapler-squad session title (mapped from hook)
	ClaudeSessionID string // Claude Code's internal session_id
	ToolName        string
	ToolInput       map[string]interface{}
	Cwd             string
	PermissionMode  string
	CreatedAt       time.Time
	ExpiresAt       time.Time

	// Orphaned is true for approvals loaded from disk after a server restart.
	// These have no live HTTP connection, so they cannot be resolved via the decision channel.
	Orphaned bool

	// decisionCh receives exactly one user decision. Buffered to allow non-blocking send.
	// Nil for orphaned approvals (loaded from disk after restart).
	decisionCh chan ApprovalDecision
}

// PersistedApproval is the JSON-serializable representation of a PendingApproval for disk storage.
type PersistedApproval struct {
	ID              string                 `json:"id"`
	SessionID       string                 `json:"session_id"`
	ClaudeSessionID string                 `json:"claude_session_id"`
	ToolName        string                 `json:"tool_name"`
	ToolInput       map[string]interface{} `json:"tool_input"`
	Cwd             string                 `json:"cwd"`
	PermissionMode  string                 `json:"permission_mode"`
	CreatedAt       time.Time              `json:"created_at"`
	ExpiresAt       time.Time              `json:"expires_at"`
	Orphaned        bool                   `json:"orphaned"`
}

// orphanedCleanupThreshold is the maximum age for orphaned approvals before they are cleaned up.
const orphanedCleanupThreshold = 4 * time.Hour

// ApprovalStore manages pending approval requests with thread-safe access.
type ApprovalStore struct {
	mu        sync.RWMutex
	pending   map[string]*PendingApproval // keyed by approval ID
	bySession map[string][]string         // session ID -> approval IDs
	filePath  string                      // path to pending_approvals.json (empty disables persistence)
}

// NewApprovalStore creates a new ApprovalStore.
// If filePath is non-empty, persisted approvals are loaded from disk and marked as orphaned.
func NewApprovalStore(filePath string) *ApprovalStore {
	s := &ApprovalStore{
		pending:   make(map[string]*PendingApproval),
		bySession: make(map[string][]string),
		filePath:  filePath,
	}
	if filePath != "" {
		if err := s.loadFromDisk(); err != nil {
			log.WarningLog.Printf("[ApprovalPersistence] Failed to load persisted approvals from %s: %v (starting with empty state)", filePath, err)
		}
	}
	return s
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

	s.persistToDiskLocked()
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

// GetApprovalMetadataBySession implements session.ApprovalMetadataProvider.
// Returns approval metadata for all pending approvals matching the given session ID.
func (s *ApprovalStore) GetApprovalMetadataBySession(sessionID string) []session.ApprovalMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.bySession[sessionID]
	result := make([]session.ApprovalMetadata, 0, len(ids))
	for _, id := range ids {
		if a, ok := s.pending[id]; ok {
			result = append(result, session.ApprovalMetadata{
				ApprovalID: a.ID,
				ToolName:   a.ToolName,
				ToolInput:  a.ToolInput,
				Cwd:        a.Cwd,
				Orphaned:   a.Orphaned,
			})
		}
	}
	return result
}

// Resolve sends a decision to the pending approval and removes it from the store.
// Returns an error if the approval doesn't exist or was already resolved.
// For orphaned approvals (loaded from disk after restart), the record is simply removed
// since there is no live HTTP connection to send the decision to.
func (s *ApprovalStore) Resolve(id string, decision ApprovalDecision) error {
	s.mu.Lock()
	a, ok := s.pending[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("approval %s not found or already resolved", id)
	}
	delete(s.pending, id)
	s.removeFromSessionIndex(a.SessionID, id)
	s.persistToDiskLocked()
	s.mu.Unlock()

	// Orphaned approvals have no live HTTP connection -- just remove the record.
	if a.Orphaned || a.decisionCh == nil {
		log.InfoLog.Printf("[ApprovalPersistence] Removed orphaned approval %s (session=%s)", id, a.SessionID)
		return nil
	}

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
	s.persistToDiskLocked()
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
	s.persistToDiskLocked()
	s.mu.Unlock()

	var cancelled []string
	for _, a := range approvals {
		cancelled = append(cancelled, a.ID)
		if a.Orphaned || a.decisionCh == nil {
			continue
		}
		select {
		case a.decisionCh <- ApprovalDecision{Behavior: "deny", Message: "Session restarted"}:
		default:
		}
	}
	return cancelled
}

// CleanupExpired removes approvals past their ExpiresAt and denies them with a timeout message.
// Also removes orphaned approvals older than orphanedCleanupThreshold (4 hours).
// Returns the IDs of cleaned-up approvals.
func (s *ApprovalStore) CleanupExpired() []string {
	now := time.Now()
	s.mu.Lock()

	var expired []*PendingApproval
	var orphanedCleaned []*PendingApproval
	for id, a := range s.pending {
		if !a.Orphaned && now.After(a.ExpiresAt) {
			expired = append(expired, a)
			delete(s.pending, id)
			s.removeFromSessionIndex(a.SessionID, id)
		} else if a.Orphaned && now.Sub(a.CreatedAt) > orphanedCleanupThreshold {
			orphanedCleaned = append(orphanedCleaned, a)
			delete(s.pending, id)
			s.removeFromSessionIndex(a.SessionID, id)
		}
	}
	if len(expired) > 0 || len(orphanedCleaned) > 0 {
		s.persistToDiskLocked()
	}
	s.mu.Unlock()

	var ids []string
	for _, a := range expired {
		ids = append(ids, a.ID)
		if a.decisionCh != nil {
			select {
			case a.decisionCh <- ApprovalDecision{Behavior: "deny", Message: "Approval timed out. Please respond in the terminal."}:
			default:
			}
		}
	}
	for _, a := range orphanedCleaned {
		ids = append(ids, a.ID)
		log.InfoLog.Printf("[ApprovalPersistence] Cleaned up orphaned approval %s (session=%s, age=%s)",
			a.ID, a.SessionID, now.Sub(a.CreatedAt).Round(time.Second))
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

// persistToDiskLocked writes all current pending approvals to disk using an atomic write pattern.
// Must be called with mu held (write lock). If filePath is empty, this is a no-op.
func (s *ApprovalStore) persistToDiskLocked() {
	if s.filePath == "" {
		return
	}

	persisted := make([]PersistedApproval, 0, len(s.pending))
	for _, a := range s.pending {
		persisted = append(persisted, PersistedApproval{
			ID:              a.ID,
			SessionID:       a.SessionID,
			ClaudeSessionID: a.ClaudeSessionID,
			ToolName:        a.ToolName,
			ToolInput:       a.ToolInput,
			Cwd:             a.Cwd,
			PermissionMode:  a.PermissionMode,
			CreatedAt:       a.CreatedAt,
			ExpiresAt:       a.ExpiresAt,
			Orphaned:        a.Orphaned,
		})
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		log.ErrorLog.Printf("[ApprovalPersistence] Failed to marshal approvals: %v", err)
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.ErrorLog.Printf("[ApprovalPersistence] Failed to create directory %s: %v", dir, err)
		return
	}

	// Atomic write: write to temp file, then rename
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.ErrorLog.Printf("[ApprovalPersistence] Failed to write temp file %s: %v", tmpPath, err)
		return
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		log.ErrorLog.Printf("[ApprovalPersistence] Failed to rename temp file to %s: %v", s.filePath, err)
		return
	}

	log.InfoLog.Printf("[ApprovalPersistence] Persisted %d approvals to %s", len(persisted), s.filePath)
}

// loadFromDisk loads persisted approvals from disk and marks them all as orphaned
// since their HTTP connections are gone after a server restart.
func (s *ApprovalStore) loadFromDisk() error {
	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No file yet -- that is fine on first boot.
			return nil
		}
		return fmt.Errorf("read %s: %w", s.filePath, err)
	}

	if len(data) == 0 {
		return nil
	}

	var persisted []PersistedApproval
	if err := json.Unmarshal(data, &persisted); err != nil {
		// Corrupt JSON -- log warning and start fresh (don't crash).
		log.WarningLog.Printf("[ApprovalPersistence] Corrupt JSON in %s, starting with empty state: %v", s.filePath, err)
		return nil
	}

	loaded := 0
	for _, p := range persisted {
		if p.ID == "" {
			continue
		}
		a := &PendingApproval{
			ID:              p.ID,
			SessionID:       p.SessionID,
			ClaudeSessionID: p.ClaudeSessionID,
			ToolName:        p.ToolName,
			ToolInput:       p.ToolInput,
			Cwd:             p.Cwd,
			PermissionMode:  p.PermissionMode,
			CreatedAt:       p.CreatedAt,
			ExpiresAt:       p.ExpiresAt,
			Orphaned:        true,       // Always mark as orphaned on load
			decisionCh:      nil,        // No live HTTP connection
		}
		s.pending[a.ID] = a
		s.bySession[a.SessionID] = append(s.bySession[a.SessionID], a.ID)
		loaded++
	}

	if loaded > 0 {
		log.InfoLog.Printf("[ApprovalPersistence] Loaded %d orphaned approvals from %s", loaded, s.filePath)
	}
	return nil
}

// GetFilePath returns the file path used for persistence (for testing/wiring).
func (s *ApprovalStore) GetFilePath() string {
	return s.filePath
}
