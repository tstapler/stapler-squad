package workspace

import (
	"context"
	"sync"
	"time"
)

// NoOpLock is a no-operation lock for single-instance deployments
// or testing where distributed coordination is not needed.
// It still provides in-memory locking for thread safety.
type NoOpLock struct {
	mu    sync.Mutex
	locks map[string]*noOpHandle
}

// noOpHandle represents an acquired no-op lock
type noOpHandle struct {
	resource string
	released bool
	mu       sync.Mutex
	parent   *NoOpLock
}

// NewNoOpLock creates a new no-operation lock.
func NewNoOpLock() *NoOpLock {
	return &NoOpLock{
		locks: make(map[string]*noOpHandle),
	}
}

// Acquire obtains a lock for the given resource.
// For NoOpLock, this provides in-memory thread safety only.
func (l *NoOpLock) Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error) {
	// Create timeout context if specified
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Try to acquire lock
	for {
		handle, acquired := l.tryAcquireInternal(resource)
		if acquired {
			return handle, nil
		}

		// Check context
		select {
		case <-ctx.Done():
			return nil, &LockError{
				Op:       "acquire",
				Resource: resource,
				Err:      ctx.Err(),
			}
		case <-time.After(10 * time.Millisecond):
			// Retry
		}
	}
}

// TryAcquire attempts to acquire lock without blocking.
func (l *NoOpLock) TryAcquire(ctx context.Context, resource string) (LockHandle, bool, error) {
	handle, acquired := l.tryAcquireInternal(resource)
	return handle, acquired, nil
}

// tryAcquireInternal attempts to acquire the lock.
func (l *NoOpLock) tryAcquireInternal(resource string) (*noOpHandle, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if lock is already held
	if existing, exists := l.locks[resource]; exists && !existing.released {
		return nil, false
	}

	// Create new handle
	handle := &noOpHandle{
		resource: resource,
		released: false,
		parent:   l,
	}
	l.locks[resource] = handle

	return handle, true
}

// Close releases all locks.
func (l *NoOpLock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.locks = make(map[string]*noOpHandle)
	return nil
}

// Release releases the lock.
func (h *noOpHandle) Release() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.released {
		return nil // Already released, no-op
	}

	h.released = true

	// Remove from parent
	h.parent.mu.Lock()
	delete(h.parent.locks, h.resource)
	h.parent.mu.Unlock()

	return nil
}

// Extend is a no-op for NoOpLock (locks don't expire).
func (h *noOpHandle) Extend(duration time.Duration) error {
	return nil // No-op, locks don't expire
}

// IsValid returns true if the lock is still held.
func (h *noOpHandle) IsValid() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.released
}

// Resource returns the resource name this lock is for.
func (h *noOpHandle) Resource() string {
	return h.resource
}
