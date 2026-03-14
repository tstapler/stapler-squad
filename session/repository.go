package session

import (
	"context"
	"time"
)

// Repository defines the interface for session persistence operations.
// This abstraction allows multiple storage backends (SQLite, JSON, etc.)
// while maintaining a consistent API for session management.
type Repository interface {
	// Create inserts a new session into storage
	Create(ctx context.Context, data InstanceData) error

	// Update modifies an existing session in storage
	Update(ctx context.Context, data InstanceData) error

	// Delete removes a session from storage by title
	Delete(ctx context.Context, title string) error

	// Get retrieves a single session by title with full child data
	// For selective loading, use GetWithOptions instead
	Get(ctx context.Context, title string) (*InstanceData, error)

	// GetWithOptions retrieves a single session with selective child data loading
	// Use LoadOptions presets (LoadMinimal, LoadSummary, LoadFull) or custom options
	GetWithOptions(ctx context.Context, title string, options LoadOptions) (*InstanceData, error)

	// List retrieves all sessions with summary child data (no diff content)
	// For selective loading, use ListWithOptions instead
	List(ctx context.Context) ([]InstanceData, error)

	// ListWithOptions retrieves all sessions with selective child data loading
	// Use LoadOptions presets (LoadMinimal, LoadSummary, LoadFull) or custom options
	ListWithOptions(ctx context.Context, options LoadOptions) ([]InstanceData, error)

	// ListByStatus retrieves sessions filtered by status with summary child data
	// For selective loading, use ListByStatusWithOptions instead
	ListByStatus(ctx context.Context, status Status) ([]InstanceData, error)

	// ListByStatusWithOptions retrieves sessions filtered by status with selective loading
	ListByStatusWithOptions(ctx context.Context, status Status, options LoadOptions) ([]InstanceData, error)

	// ListByTag retrieves sessions with a specific tag with summary child data
	// For selective loading, use ListByTagWithOptions instead
	ListByTag(ctx context.Context, tag string) ([]InstanceData, error)

	// ListByTagWithOptions retrieves sessions with a specific tag with selective loading
	ListByTagWithOptions(ctx context.Context, tag string, options LoadOptions) ([]InstanceData, error)

	// UpdateTimestamps efficiently updates only timestamp fields for a session
	// This is optimized for frequent updates from WebSocket terminal streaming
	UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error

	// Close performs cleanup and releases resources
	Close() error

	// --- New Session-based methods (Phase 2 of schema normalization) ---
	// These methods use the new domain-driven Session type with optional contexts.
	// They are preferred over InstanceData methods for new code.

	// GetSession retrieves a session using the new Session domain model.
	// Use ContextOptions to control which optional contexts are loaded.
	// Returns nil if session not found.
	GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error)

	// ListSessions retrieves all sessions using the new Session domain model.
	// Use ContextOptions to control which optional contexts are loaded.
	ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error)

	// CreateSession creates a new session from the Session domain model.
	CreateSession(ctx context.Context, session *Session) error

	// UpdateSession updates an existing session using the Session domain model.
	UpdateSession(ctx context.Context, session *Session) error
}

// RepositoryOption is a function that configures a repository
type RepositoryOption func(interface{}) error

// WithDatabasePath sets the database file path for the repository
func WithDatabasePath(path string) RepositoryOption {
	return func(r interface{}) error {
		if entRepo, ok := r.(*EntRepository); ok {
			entRepo.dbPath = path
			return nil
		}
		return nil // No-op for unsupported repository types
	}
}
