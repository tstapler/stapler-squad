package analytics

import (
	"context"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

const (
	// escapeEvictChunkRows bounds one delete statement so a single eviction step
	// holds the analytics pool's only connection for tens of milliseconds, not
	// seconds. Each chunk is its own transaction, so queued writers get in between.
	escapeEvictChunkRows = 250

	// escapeEvictSlackDivisor sets the eviction hysteresis: a session is trimmed
	// only once it exceeds cap + cap/divisor rows, then all the way back to cap.
	// Trimming on every flush instead cost one select+delete pair per flush for
	// every session sitting at its cap.
	escapeEvictSlackDivisor = 10
)

// escapeEvictionThreshold is the row count above which a session is trimmed.
func escapeEvictionThreshold(maxRows int) int {
	return maxRows + max(maxRows/escapeEvictSlackDivisor, 1)
}

// evictOldestEscapeEvents deletes up to n of the session's oldest rows by
// wall_time, one escapeEvictChunkRows chunk per statement. When shouldYield
// reports true between chunks it stops early; the caller's count is still over
// the cap so the next trigger resumes. It returns the rows actually deleted.
func evictOldestEscapeEvents(ctx context.Context, client *ent.Client, sessionID string, n int, shouldYield func() bool) (int, error) {
	deletedTotal := 0
	for deletedTotal < n {
		if shouldYield != nil && deletedTotal > 0 && shouldYield() {
			return deletedTotal, nil
		}
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
	}
	return deletedTotal, nil
}
