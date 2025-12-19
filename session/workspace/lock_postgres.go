package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

// PostgresAdvisoryLock implements distributed locking using PostgreSQL advisory locks.
// Advisory locks are application-level locks that don't conflict with row-level locks.
// They are ideal for distributed coordination across multiple application instances.
//
// Key characteristics:
// - Reentrant within the same database connection
// - Automatically released when the connection closes
// - No risk of deadlock with row-level operations
// - Efficient: no disk I/O, fully in-memory on the PostgreSQL server
type PostgresAdvisoryLock struct {
	db        *sql.DB
	namespace int64 // Namespace prefix to avoid conflicts with other applications

	// Track connections holding locks
	mu      sync.Mutex
	handles map[string]*postgresHandle
}

// postgresHandle represents an acquired PostgreSQL advisory lock
type postgresHandle struct {
	resource string
	key      int64
	conn     *sql.Conn // Dedicated connection for this lock
	released bool
	mu       sync.Mutex
	parent   *PostgresAdvisoryLock
}

// NewPostgresAdvisoryLock creates a new PostgreSQL advisory lock manager.
// The namespace should be unique to this application to avoid conflicts.
// Recommended: use a consistent hash of the application name.
func NewPostgresAdvisoryLock(db *sql.DB, namespace int64) *PostgresAdvisoryLock {
	return &PostgresAdvisoryLock{
		db:        db,
		namespace: namespace,
		handles:   make(map[string]*postgresHandle),
	}
}

// Acquire obtains an exclusive advisory lock for the given resource.
// The lock is held on a dedicated connection and released when Release() is called
// or when the connection closes.
func (l *PostgresAdvisoryLock) Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error) {
	// Create timeout context
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Check if we already hold this lock
	l.mu.Lock()
	if existing, exists := l.handles[resource]; exists && !existing.released {
		l.mu.Unlock()
		// Reentrant - return existing handle
		return existing, nil
	}
	l.mu.Unlock()

	// Hash resource to int64 key for advisory lock
	key := l.hashResource(resource)

	// Get a dedicated connection for this lock
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, &LockError{
			Op:       "acquire",
			Resource: resource,
			Err:      fmt.Errorf("get connection: %w", err),
		}
	}

	// Acquire advisory lock
	// pg_advisory_lock blocks until the lock is available
	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key)
	if err != nil {
		conn.Close()
		return nil, &LockError{
			Op:       "acquire",
			Resource: resource,
			Err:      fmt.Errorf("pg_advisory_lock: %w", err),
		}
	}

	// Create handle
	handle := &postgresHandle{
		resource: resource,
		key:      key,
		conn:     conn,
		released: false,
		parent:   l,
	}

	l.mu.Lock()
	l.handles[resource] = handle
	l.mu.Unlock()

	return handle, nil
}

// TryAcquire attempts to acquire lock without blocking.
// Uses pg_try_advisory_lock which returns immediately.
func (l *PostgresAdvisoryLock) TryAcquire(ctx context.Context, resource string) (LockHandle, bool, error) {
	// Check if we already hold this lock
	l.mu.Lock()
	if existing, exists := l.handles[resource]; exists && !existing.released {
		l.mu.Unlock()
		return existing, true, nil
	}
	l.mu.Unlock()

	// Hash resource to int64 key
	key := l.hashResource(resource)

	// Get a dedicated connection
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, false, &LockError{
			Op:       "try_acquire",
			Resource: resource,
			Err:      fmt.Errorf("get connection: %w", err),
		}
	}

	// Try to acquire without blocking
	var acquired bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired)
	if err != nil {
		conn.Close()
		return nil, false, &LockError{
			Op:       "try_acquire",
			Resource: resource,
			Err:      fmt.Errorf("pg_try_advisory_lock: %w", err),
		}
	}

	if !acquired {
		conn.Close()
		return nil, false, nil
	}

	// Create handle
	handle := &postgresHandle{
		resource: resource,
		key:      key,
		conn:     conn,
		released: false,
		parent:   l,
	}

	l.mu.Lock()
	l.handles[resource] = handle
	l.mu.Unlock()

	return handle, true, nil
}

// Close releases all locks.
func (l *PostgresAdvisoryLock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, handle := range l.handles {
		if !handle.released && handle.conn != nil {
			// Advisory locks are automatically released when connection closes
			handle.conn.Close()
		}
	}

	l.handles = make(map[string]*postgresHandle)
	return nil
}

// hashResource converts a resource string to an int64 key for advisory locks.
// Uses FNV-1a hash combined with namespace to create a unique key.
func (l *PostgresAdvisoryLock) hashResource(resource string) int64 {
	h := fnv.New64a()
	h.Write([]byte(resource))
	hash := h.Sum64()

	// Combine with namespace (XOR to preserve distribution)
	return int64(hash) ^ l.namespace
}

// Release releases the advisory lock.
func (h *postgresHandle) Release() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.released {
		return nil // Already released
	}

	h.released = true

	// Remove from parent tracking
	h.parent.mu.Lock()
	delete(h.parent.handles, h.resource)
	h.parent.mu.Unlock()

	if h.conn == nil {
		return nil
	}

	// Explicitly release the advisory lock
	_, err := h.conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", h.key)
	if err != nil {
		// Lock will be released when connection closes anyway
	}

	// Close the connection (also releases the lock if explicit unlock failed)
	return h.conn.Close()
}

// Extend is not applicable for PostgreSQL advisory locks.
// Advisory locks don't have TTL - they're held until released or connection closes.
func (h *postgresHandle) Extend(duration time.Duration) error {
	return nil // No-op, advisory locks don't expire
}

// IsValid returns true if the lock is still held.
func (h *postgresHandle) IsValid() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.released
}

// Resource returns the resource name this lock is for.
func (h *postgresHandle) Resource() string {
	return h.resource
}
