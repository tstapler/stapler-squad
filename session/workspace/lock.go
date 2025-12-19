package workspace

import (
	"context"
	"time"
)

// DistributedLock provides coordination across multiple pods/processes.
// Implementations:
// - PostgresAdvisoryLock: Uses pg_advisory_lock() for multi-pod deployments
// - SQLiteTransactionLock: Uses BEGIN IMMEDIATE for single-pod deployments
// - NoOpLock: For testing or single-instance deployments without contention
type DistributedLock interface {
	// Acquire obtains an exclusive lock for the given resource.
	// Blocks until the lock is acquired or context is cancelled/times out.
	// Returns a handle that must be released when done.
	Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error)

	// TryAcquire attempts to acquire lock without blocking.
	// Returns (handle, true) if acquired, (nil, false) if unavailable.
	TryAcquire(ctx context.Context, resource string) (LockHandle, bool, error)

	// Close releases any resources held by the lock implementation
	Close() error
}

// LockHandle represents an acquired distributed lock.
// The lock is held until Release() is called or the context is cancelled.
type LockHandle interface {
	// Release releases the lock. Must be called when done.
	// Safe to call multiple times (subsequent calls are no-ops).
	Release() error

	// Extend extends the lock TTL for long-running operations.
	// Not all implementations support this (may return ErrNotSupported).
	Extend(duration time.Duration) error

	// IsValid checks if the lock is still held.
	// Returns false if lock was released or expired.
	IsValid() bool

	// Resource returns the resource name this lock is for.
	Resource() string
}

// CacheInvalidationNotifier handles cross-pod cache invalidation.
// Implementations:
// - PostgresListenNotify: Uses LISTEN/NOTIFY for real-time notifications
// - PollingNotifier: Poll-based invalidation for SQLite (fallback)
type CacheInvalidationNotifier interface {
	// Subscribe starts listening for invalidation events.
	// The handler is called for each invalidation with the workspace path.
	// Runs until context is cancelled.
	Subscribe(ctx context.Context, handler func(workspacePath string)) error

	// Publish broadcasts an invalidation event to all subscribers.
	// workspacePath can be empty string to invalidate all caches.
	Publish(ctx context.Context, workspacePath string) error

	// Close stops listening and releases resources.
	Close() error
}

// LockError represents errors from distributed lock operations
type LockError struct {
	Op       string // Operation: "acquire", "release", "extend"
	Resource string // Resource being locked
	Err      error  // Underlying error
}

func (e *LockError) Error() string {
	return "lock " + e.Op + " for " + e.Resource + ": " + e.Err.Error()
}

func (e *LockError) Unwrap() error {
	return e.Err
}

// Common lock errors
var (
	// ErrLockTimeout indicates lock acquisition timed out
	ErrLockTimeout = &LockError{Op: "acquire", Err: context.DeadlineExceeded}

	// ErrLockNotHeld indicates trying to release a lock that isn't held
	ErrLockNotHeld = &LockError{Op: "release", Err: context.Canceled}

	// ErrNotSupported indicates the operation isn't supported by this implementation
	ErrNotSupported = &LockError{Op: "extend", Err: context.Canceled}
)
