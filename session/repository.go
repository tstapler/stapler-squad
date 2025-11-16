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

	// Get retrieves a single session by title
	Get(ctx context.Context, title string) (*InstanceData, error)

	// List retrieves all sessions from storage
	List(ctx context.Context) ([]InstanceData, error)

	// ListByStatus retrieves sessions filtered by status
	ListByStatus(ctx context.Context, status Status) ([]InstanceData, error)

	// ListByTag retrieves sessions that have a specific tag
	ListByTag(ctx context.Context, tag string) ([]InstanceData, error)

	// UpdateTimestamps efficiently updates only timestamp fields for a session
	// This is optimized for frequent updates from WebSocket terminal streaming
	UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error

	// Close performs cleanup and releases resources
	Close() error
}

// RepositoryOption is a function that configures a repository
type RepositoryOption func(interface{}) error

// WithDatabasePath sets the database file path for SQLite repositories
func WithDatabasePath(path string) RepositoryOption {
	return func(r interface{}) error {
		if sqliteRepo, ok := r.(*SQLiteRepository); ok {
			sqliteRepo.dbPath = path
			return nil
		}
		if entRepo, ok := r.(*EntRepository); ok {
			entRepo.dbPath = path
			return nil
		}
		return nil // No-op for unsupported repository types
	}
}

// WithMigrationMode enables migration mode for dual-write scenarios
func WithMigrationMode(enabled bool) RepositoryOption {
	return func(r interface{}) error {
		if sqliteRepo, ok := r.(*SQLiteRepository); ok {
			sqliteRepo.migrationMode = enabled
			return nil
		}
		return nil
	}
}
