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
func StartRetentionEnforcer(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int, escapeRetentionDays int, escapeMaxRowsPerSession int) {
	if client == nil {
		return
	}
	go func() {
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
	}()
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
func runEscapeEventPerSessionRetention(ctx context.Context, client *ent.Client, maxRows int) {
	if maxRows <= 0 {
		return
	}
	sessionIDs, err := client.EscapeEvent.Query().Unique(true).Select(escapeevent.FieldSessionID).Strings(ctx)
	if err != nil {
		log.Warn("analytics/retention escape_event session query failed", "err", err)
		return
	}
	for _, sessionID := range sessionIDs {
		count, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(sessionID)).Count(ctx)
		if err != nil || count <= maxRows {
			continue
		}
		excess, deletedTotal := count-maxRows, 0
		for excess > 0 {
			limit := excess
			if limit > 500 {
				limit = 500
			}
			ids, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(sessionID)).
				Order(escapeevent.ByWallTime()).Limit(limit).IDs(ctx)
			if err != nil || len(ids) == 0 {
				log.Warn("analytics/retention escape_event oldest rows query failed", "err", err, "session_id", sessionID)
				break
			}
			deleted, err := client.EscapeEvent.Delete().Where(escapeevent.IDIn(ids...)).Exec(ctx)
			if err != nil {
				log.Warn("analytics/retention escape_event per-session eviction failed", "err", err, "session_id", sessionID)
				break
			}
			if deleted == 0 {
				break
			}
			deletedTotal += deleted
			excess -= deleted
		}
		if deletedTotal > 0 {
			log.Info("analytics/retention escape_event per-session eviction deleted rows", "deleted", deletedTotal, "session_id", sessionID, "limit", maxRows)
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
