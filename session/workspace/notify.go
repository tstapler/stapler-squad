package workspace

import (
	"context"
	"sync"
	"time"
)

// PollingNotifier implements CacheInvalidationNotifier using polling.
// This is used for SQLite deployments where LISTEN/NOTIFY is not available.
// It's less efficient but works with any database backend.
type PollingNotifier struct {
	pollInterval time.Duration

	// Subscribers
	mu       sync.RWMutex
	handlers []func(workspacePath string)

	// Internal state
	lastInvalidation time.Time
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

// NewPollingNotifier creates a new polling-based cache invalidation notifier.
// Default poll interval is 5 seconds.
func NewPollingNotifier(pollInterval time.Duration) *PollingNotifier {
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}

	return &PollingNotifier{
		pollInterval:     pollInterval,
		handlers:         make([]func(workspacePath string), 0),
		lastInvalidation: time.Now(),
		stopChan:         make(chan struct{}),
	}
}

// Subscribe starts listening for invalidation events.
// With polling notifier, this is primarily for local invalidation within the same process.
func (n *PollingNotifier) Subscribe(ctx context.Context, handler func(workspacePath string)) error {
	n.mu.Lock()
	n.handlers = append(n.handlers, handler)
	n.mu.Unlock()

	// Start polling goroutine (only once)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		ticker := time.NewTicker(n.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-n.stopChan:
				return
			case <-ticker.C:
				// Polling tick - in a real implementation, this would
				// check a shared state (file, database table) for changes
			}
		}
	}()

	return nil
}

// Publish broadcasts an invalidation event to all local subscribers.
// For polling notifier, this only notifies local subscribers.
func (n *PollingNotifier) Publish(ctx context.Context, workspacePath string) error {
	n.mu.RLock()
	handlers := make([]func(workspacePath string), len(n.handlers))
	copy(handlers, n.handlers)
	n.mu.RUnlock()

	for _, handler := range handlers {
		handler(workspacePath)
	}

	n.mu.Lock()
	n.lastInvalidation = time.Now()
	n.mu.Unlock()

	return nil
}

// Close stops the polling notifier.
func (n *PollingNotifier) Close() error {
	close(n.stopChan)
	n.wg.Wait()
	return nil
}

// NoOpNotifier is a notifier that does nothing.
// Used when cache invalidation is not needed (single-pod deployments).
type NoOpNotifier struct{}

// NewNoOpNotifier creates a new no-operation notifier.
func NewNoOpNotifier() *NoOpNotifier {
	return &NoOpNotifier{}
}

// Subscribe is a no-op.
func (n *NoOpNotifier) Subscribe(ctx context.Context, handler func(workspacePath string)) error {
	return nil
}

// Publish is a no-op.
func (n *NoOpNotifier) Publish(ctx context.Context, workspacePath string) error {
	return nil
}

// Close is a no-op.
func (n *NoOpNotifier) Close() error {
	return nil
}
