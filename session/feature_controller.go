package session

import (
	"context"
	"sync"
)

// BacklogController implements services.FeatureController for the backlog feature.
// It enables/disables the BacklogLifecycleListener and SyncLoop at runtime
// without requiring a server restart.
//
// Enable/Disable are safe to call concurrently.
type BacklogController struct {
	mu       sync.Mutex
	listener *BacklogLifecycleListener
	storage  *Storage
	registry *PluginRegistry
	keyFunc  func() ([]byte, error)

	// syncLoop is the currently running sync loop; nil when disabled.
	syncLoop   *SyncLoop
	syncCancel context.CancelFunc
}

// NewBacklogController creates a controller that manages the given listener.
// storage, registry, and keyFunc are used to create a new SyncLoop on Enable.
func NewBacklogController(
	listener *BacklogLifecycleListener,
	storage *Storage,
	registry *PluginRegistry,
	keyFunc func() ([]byte, error),
) *BacklogController {
	return &BacklogController{
		listener: listener,
		storage:  storage,
		registry: registry,
		keyFunc:  keyFunc,
	}
}

// Enable activates the backlog feature: sets listener enabled and starts the sync loop.
// Idempotent — calling Enable when already enabled is a no-op.
func (c *BacklogController) Enable(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.listener.SetEnabled(true)

	if c.syncLoop != nil {
		return nil // already running
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.syncCancel = cancel

	var sl *SyncLoop
	if c.keyFunc != nil {
		sl = NewSyncLoopWithKeyProvider(c.storage, c.registry, c.keyFunc)
	} else {
		sl = NewSyncLoop(c.storage, c.registry)
	}
	c.syncLoop = sl
	go sl.Start(ctx)
	return nil
}

// Disable deactivates the backlog feature: sets listener disabled and stops the sync loop.
// Idempotent — calling Disable when already disabled is a no-op.
func (c *BacklogController) Disable() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.listener.SetEnabled(false)

	if c.syncLoop == nil {
		return nil // already stopped
	}

	if c.syncCancel != nil {
		c.syncCancel()
		c.syncCancel = nil
	}
	c.syncLoop.Stop()
	c.syncLoop = nil
	return nil
}

// IsEnabled reports whether the backlog feature is currently active.
func (c *BacklogController) IsEnabled() bool {
	return c.listener.enabled.Load()
}
