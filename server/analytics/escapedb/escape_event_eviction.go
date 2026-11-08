package escapedb

import (
	"context"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// escapeEvictChunkRows bounds one delete statement so a single eviction step holds the
// analytics pool's only connection for tens of milliseconds, not seconds. Each chunk is
// its own transaction, so queued writers get in between.
const escapeEvictChunkRows = 250

// evictOldestEscapeEvents deletes up to n of the session's oldest rows by wall_time, one
// escapeEvictChunkRows chunk per statement, calling pause between chunks (a non-nil error
// from pause stops early). It returns the rows actually deleted.
func evictOldestEscapeEvents(ctx context.Context, client *ent.Client, sessionID string, n int, pause func(context.Context) error) (int, error) {
	deletedTotal := 0
	for deletedTotal < n {
		ids, err := client.EscapeEvent.Query().
			Where(escapeevent.SessionID(sessionID)).
			Order(escapeevent.ByWallTime()).
			Limit(min(n-deletedTotal, escapeEvictChunkRows)).
			IDs(ctx)
		if err != nil || len(ids) == 0 {
			return deletedTotal, err
		}
		deleted, err := client.EscapeEvent.Delete().Where(escapeevent.IDIn(ids...)).Exec(ctx)
		deletedTotal += deleted
		if err != nil || deleted == 0 {
			return deletedTotal, err
		}
		if pause != nil && pause(ctx) != nil {
			return deletedTotal, nil
		}
	}
	return deletedTotal, nil
}
