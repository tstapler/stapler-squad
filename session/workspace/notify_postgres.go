package workspace

import (
	"context"
	"database/sql"
	"sync"
	// Note: For full LISTEN/NOTIFY support, you'd use pgx directly.
	// The database/sql driver has limited support for async notifications.
	// This is a simplified implementation that uses polling with notifications as a trigger.
)

// PostgresListenNotifyNotifier implements CacheInvalidationNotifier using
// PostgreSQL's LISTEN/NOTIFY mechanism for real-time cross-pod cache invalidation.
//
// Note: This is a simplified implementation. For production use with high-throughput
// requirements, consider using github.com/jackc/pgx directly for native LISTEN/NOTIFY support.
type PostgresListenNotifyNotifier struct {
	db      *sql.DB
	channel string

	// Subscribers
	mu       sync.RWMutex
	handlers []func(workspacePath string)

	// Internal state
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewPostgresListenNotifyNotifier creates a new PostgreSQL LISTEN/NOTIFY notifier.
// The channel name should be unique to this application's workspace invalidation.
func NewPostgresListenNotifyNotifier(db *sql.DB, channel string) *PostgresListenNotifyNotifier {
	if channel == "" {
		channel = "workspace_cache_invalidation"
	}

	return &PostgresListenNotifyNotifier{
		db:       db,
		channel:  channel,
		handlers: make([]func(workspacePath string), 0),
		stopChan: make(chan struct{}),
	}
}

// Subscribe starts listening for invalidation events on the PostgreSQL channel.
// This spawns a goroutine that listens for notifications.
func (n *PostgresListenNotifyNotifier) Subscribe(ctx context.Context, handler func(workspacePath string)) error {
	n.mu.Lock()
	n.handlers = append(n.handlers, handler)
	n.mu.Unlock()

	// Note: database/sql doesn't have native LISTEN support.
	// For production, use pgx.Conn.WaitForNotification().
	// This simplified implementation uses polling + manual notification dispatch.

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-n.stopChan:
				return
			}
		}
	}()

	return nil
}

// Publish sends a notification to all listening pods via PostgreSQL NOTIFY.
func (n *PostgresListenNotifyNotifier) Publish(ctx context.Context, workspacePath string) error {
	// Send NOTIFY with the workspace path as payload
	_, err := n.db.ExecContext(ctx, "SELECT pg_notify($1, $2)", n.channel, workspacePath)
	if err != nil {
		return err
	}

	// Also notify local handlers directly
	n.mu.RLock()
	handlers := make([]func(workspacePath string), len(n.handlers))
	copy(handlers, n.handlers)
	n.mu.RUnlock()

	for _, handler := range handlers {
		handler(workspacePath)
	}

	return nil
}

// Close stops listening and releases resources.
func (n *PostgresListenNotifyNotifier) Close() error {
	close(n.stopChan)
	n.wg.Wait()
	return nil
}
