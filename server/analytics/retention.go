package analytics

import (
	"context"
	"sync"
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
// The goroutine exits when ctx is cancelled and is tracked on wg so the caller
// can join it.
func StartRetentionEnforcer(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int, escapeRetentionDays int, escapeMaxRowsPerSession int, wg *sync.WaitGroup) {
	if client == nil {
		return
	}
	wg.Go(func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		// Run once immediately so limits are enforced right after startup.
		runRetention(ctx, client, maxRows, maxAgeDays, escapeRetentionDays, escapeMaxRowsPerSession)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetention(ctx, client, maxRows, maxAgeDays, escapeRetentionDays, escapeMaxRowsPerSession)
			}
		}
	})
}

// runRetention performs one enforcement cycle: age-based eviction first, then
// count-based eviction if still over limit. Also deletes escape_event rows
// older than escapeRetentionDays.
func runRetention(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int, escapeRetentionDays int, escapeMaxRowsPerSession int) {
	// Phase 0: enforce escape-event age and durable per-session limits.
	runEscapeEventRetention(ctx, client, escapeRetentionDays)
	runEscapeEventPerSessionRetention(ctx, client, escapeMaxRowsPerSession)

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

// runEscapeEventPerSessionRetention removes the oldest rows above the durable
// per-session cap. Unlike the writer's cache, this remains correct across restarts.
// One grouped scan of the (session_id, wall_time) index finds the over-cap
// sessions, instead of a DISTINCT scan plus one COUNT per session.
func runEscapeEventPerSessionRetention(ctx context.Context, client *ent.Client, maxRows int) {
	if maxRows <= 0 {
		return
	}
	var counts []struct {
		SessionID string `json:"session_id"`
		Count     int    `json:"count"`
	}
	if err := client.EscapeEvent.Query().GroupBy(escapeevent.FieldSessionID).Aggregate(ent.Count()).Scan(ctx, &counts); err != nil {
		log.Warn("analytics/retention escape_event session count query failed", "err", err)
		return
	}
	for _, c := range counts {
		if c.Count <= maxRows {
			continue
		}
		// The writer may have trimmed this session since the grouped scan; re-count so
		// eviction never goes below the cap.
		count, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(c.SessionID)).Count(ctx)
		if err != nil || count <= maxRows {
			continue
		}
		deleted, err := evictOldestEscapeEvents(ctx, client, c.SessionID, count-maxRows, nil)
		if err != nil {
			log.Warn("analytics/retention escape_event per-session eviction failed", "err", err, "session_id", c.SessionID)
		}
		if deleted > 0 {
			log.Info("analytics/retention escape_event per-session eviction deleted rows", "deleted", deleted, "session_id", c.SessionID, "limit", maxRows)
		}
	}
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
