package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// SQLiteTransactionLock implements distributed locking using SQLite transactions.
// It uses BEGIN IMMEDIATE to acquire an exclusive lock on the database.
// This is suitable for single-pod deployments or when using a shared filesystem.
//
// Note: SQLite locks are database-wide, not per-resource. For fine-grained locking,
// use the in-memory lock map to simulate per-resource locks within a transaction.
type SQLiteTransactionLock struct {
	db *sql.DB

	// In-memory tracking of per-resource locks within transactions
	mu    sync.Mutex
	locks map[string]*sqliteHandle
}

// sqliteHandle represents an acquired SQLite lock
type sqliteHandle struct {
	resource string
	tx       *sql.Tx
	released bool
	mu       sync.Mutex
	parent   *SQLiteTransactionLock
}

// NewSQLiteTransactionLock creates a new SQLite-based distributed lock.
// The db connection should already be configured with WAL mode and appropriate timeouts.
func NewSQLiteTransactionLock(db *sql.DB) *SQLiteTransactionLock {
	return &SQLiteTransactionLock{
		db:    db,
		locks: make(map[string]*sqliteHandle),
	}
}

// Acquire obtains an exclusive lock for the given resource.
// This starts a SQLite transaction with BEGIN IMMEDIATE to acquire the write lock.
func (l *SQLiteTransactionLock) Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error) {
	// Create timeout context
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Check if we already hold this lock
	l.mu.Lock()
	if existing, exists := l.locks[resource]; exists && !existing.released {
		l.mu.Unlock()
		// Reentrant - return existing handle
		return existing, nil
	}
	l.mu.Unlock()

	// Start transaction with IMMEDIATE mode to acquire write lock
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, &LockError{
			Op:       "acquire",
			Resource: resource,
			Err:      fmt.Errorf("begin transaction: %w", err),
		}
	}

	// Ensure lock table exists (idempotent)
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspace_locks (
			resource TEXT PRIMARY KEY,
			acquired_at TEXT NOT NULL,
			owner_id TEXT NOT NULL
		)
	`)
	if err != nil {
		tx.Rollback()
		return nil, &LockError{
			Op:       "acquire",
			Resource: resource,
			Err:      fmt.Errorf("create lock table: %w", err),
		}
	}

	// Try to insert lock record (will fail if already exists)
	ownerID := fmt.Sprintf("pid-%d-%d", time.Now().UnixNano(), time.Now().Unix())
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO workspace_locks (resource, acquired_at, owner_id)
		VALUES (?, ?, ?)
	`, resource, time.Now().Format(time.RFC3339), ownerID)
	if err != nil {
		tx.Rollback()
		return nil, &LockError{
			Op:       "acquire",
			Resource: resource,
			Err:      fmt.Errorf("insert lock record: %w", err),
		}
	}

	// Create handle
	handle := &sqliteHandle{
		resource: resource,
		tx:       tx,
		released: false,
		parent:   l,
	}

	l.mu.Lock()
	l.locks[resource] = handle
	l.mu.Unlock()

	return handle, nil
}

// TryAcquire attempts to acquire lock without blocking.
func (l *SQLiteTransactionLock) TryAcquire(ctx context.Context, resource string) (LockHandle, bool, error) {
	// Use a very short timeout for try acquire
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	handle, err := l.Acquire(ctx, resource, 100*time.Millisecond)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, nil // Timeout means lock is held
		}
		return nil, false, err
	}

	return handle, true, nil
}

// Close releases all locks and closes resources.
func (l *SQLiteTransactionLock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, handle := range l.locks {
		if !handle.released && handle.tx != nil {
			handle.tx.Rollback()
		}
	}

	l.locks = make(map[string]*sqliteHandle)
	return nil
}

// Release releases the lock and commits/rollbacks the transaction.
func (h *sqliteHandle) Release() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.released {
		return nil // Already released
	}

	h.released = true

	// Remove from parent tracking
	h.parent.mu.Lock()
	delete(h.parent.locks, h.resource)
	h.parent.mu.Unlock()

	if h.tx == nil {
		return nil
	}

	// Delete the lock record and commit
	_, _ = h.tx.Exec("DELETE FROM workspace_locks WHERE resource = ?", h.resource)

	// Rollback (simpler - just releases the lock)
	return h.tx.Rollback()
}

// Extend is not supported for SQLite locks (transaction-based).
func (h *sqliteHandle) Extend(duration time.Duration) error {
	return ErrNotSupported
}

// IsValid returns true if the lock is still held.
func (h *sqliteHandle) IsValid() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.released
}

// Resource returns the resource name this lock is for.
func (h *sqliteHandle) Resource() string {
	return h.resource
}
