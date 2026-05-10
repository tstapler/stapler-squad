package analytics

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/analyticsevent"
)

// StartRetentionEnforcer starts a background goroutine that periodically deletes
// analytics events that exceed the configured age or row-count limits.
//
//   - maxRows:    maximum number of rows to retain; oldest rows are deleted first
//     when the count exceeds this limit. Use 0 to disable.
//   - maxAgeDays: rows older than this many days are deleted unconditionally.
//     Use 0 to disable age-based eviction.
//
// The goroutine exits when ctx is cancelled.
func StartRetentionEnforcer(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int) {
	if client == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		// Run once immediately so limits are enforced right after startup.
		runRetention(ctx, client, maxRows, maxAgeDays)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetention(ctx, client, maxRows, maxAgeDays)
			}
		}
	}()
}

// runRetention performs one enforcement cycle: age-based eviction first, then
// count-based eviction if still over limit.
func runRetention(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int) {
	// Phase 1: delete rows older than maxAgeDays.
	if maxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
		deleted, err := client.AnalyticsEvent.Delete().
			Where(analyticsevent.CreatedAtLT(cutoff)).
			Exec(ctx)
		if err != nil {
			log.WarningLog.Printf("[analytics/retention] age eviction failed: %v", err)
		} else if deleted > 0 {
			log.InfoLog.Printf("[analytics/retention] age eviction deleted %d rows (cutoff=%s)", deleted, cutoff.Format(time.RFC3339))
		}
	}

	// Phase 2: delete oldest rows until count is within maxRows.
	if maxRows <= 0 {
		return
	}

	count, err := client.AnalyticsEvent.Query().Count(ctx)
	if err != nil {
		log.WarningLog.Printf("[analytics/retention] count query failed: %v", err)
		return
	}
	if count <= maxRows {
		return
	}

	excess := count - maxRows
	// Fetch the IDs of the oldest excess rows so we can delete exactly that many.
	ids, err := client.AnalyticsEvent.Query().
		Order(analyticsevent.ByCreatedAt()).
		Limit(excess).
		IDs(ctx)
	if err != nil {
		log.WarningLog.Printf("[analytics/retention] oldest-IDs query failed: %v", err)
		return
	}

	deleted, err := client.AnalyticsEvent.Delete().
		Where(analyticsevent.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		log.WarningLog.Printf("[analytics/retention] count eviction failed: %v", err)
		return
	}
	log.InfoLog.Printf("[analytics/retention] count eviction deleted %d rows (was %d, limit %d)", deleted, count, maxRows)
}
