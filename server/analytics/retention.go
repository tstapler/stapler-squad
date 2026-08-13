package analytics

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/analyticsevent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// StartRetentionEnforcer starts a background goroutine that periodically deletes
// analytics events that exceed the configured age or row-count limits.
//
//   - maxRows:              maximum number of rows to retain; oldest rows are deleted first
//     when the count exceeds this limit. Use 0 to disable.
//   - maxAgeDays:           rows older than this many days are deleted unconditionally.
//     Use 0 to disable age-based eviction.
//   - escapeRetentionDays:  escape_event rows older than this many days are deleted.
//     Use 0 to disable escape event age-based eviction.
//
// The goroutine exits when ctx is cancelled.
func StartRetentionEnforcer(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int, escapeRetentionDays int) {
	if client == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		// Run once immediately so limits are enforced right after startup.
		runRetention(ctx, client, maxRows, maxAgeDays, escapeRetentionDays)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetention(ctx, client, maxRows, maxAgeDays, escapeRetentionDays)
			}
		}
	}()
}

// runRetention performs one enforcement cycle: age-based eviction first, then
// count-based eviction if still over limit. Also deletes escape_event rows
// older than escapeRetentionDays.
func runRetention(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int, escapeRetentionDays int) {
	// Phase 0: delete escape_event rows older than escapeRetentionDays.
	runEscapeEventRetention(ctx, client, escapeRetentionDays)

	// Phase 1: delete rows older than maxAgeDays.
	if maxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
		deleted, err := client.AnalyticsEvent.Delete().
			Where(analyticsevent.CreatedAtLT(cutoff)).
			Exec(ctx)
		if err != nil {
			log.Warn("analytics/retention age eviction failed", "err", err)
		} else if deleted > 0 {
			log.Info("analytics/retention age eviction deleted rows", "deleted", deleted, "cutoff", cutoff.Format(time.RFC3339))
		}
	}

	// Phase 2: delete oldest rows until count is within maxRows.
	if maxRows <= 0 {
		return
	}

	count, err := client.AnalyticsEvent.Query().Count(ctx)
	if err != nil {
		log.Warn("analytics/retention count query failed", "err", err)
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
		log.Warn("analytics/retention oldest-IDs query failed", "err", err)
		return
	}

	deleted, err := client.AnalyticsEvent.Delete().
		Where(analyticsevent.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		log.Warn("analytics/retention count eviction failed", "err", err)
		return
	}
	log.Info("analytics/retention count eviction deleted rows", "deleted", deleted, "was", count, "limit", maxRows)
}

// runEscapeEventRetention deletes escape_event rows older than retentionDays.
// Called from runRetention when escapeRetentionDays > 0.
func runEscapeEventRetention(ctx context.Context, client *ent.Client, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted, err := client.EscapeEvent.Delete().
		Where(escapeevent.WallTimeLT(cutoff)).
		Exec(ctx)
	if err != nil {
		log.Warn("analytics/retention escape_event age eviction failed", "err", err)
	} else if deleted > 0 {
		log.Info("analytics/retention escape_event age eviction deleted rows", "deleted", deleted, "cutoff", cutoff.Format(time.RFC3339))
	}
}
