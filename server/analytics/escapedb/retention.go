package escapedb

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// StartRetentionEnforcer deletes escape events older than retentionDays (0
// disables) and trims sessions above maxRowsPerSession (0 disables) once at
// startup and then hourly, until ctx is cancelled. The goroutine is tracked on wg.
func StartRetentionEnforcer(ctx context.Context, client *ent.Client, retentionDays, maxRowsPerSession int, wg *sync.WaitGroup) {
	if client == nil {
		return
	}
	wg.Go(func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		RunRetention(ctx, client, retentionDays, maxRowsPerSession)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RunRetention(ctx, client, retentionDays, maxRowsPerSession)
			}
		}
	})
}

// RunRetention performs one enforcement cycle: age eviction, then the durable per-session cap.
func RunRetention(ctx context.Context, client *ent.Client, retentionDays, maxRowsPerSession int) {
	runEscapeEventRetention(ctx, client, retentionDays)
	runEscapeEventPerSessionRetention(ctx, client, maxRowsPerSession)
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
