package escapedb

import (
	"context"
	"sync"
	"time"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// RollupTiers sets how long each rollup tier is kept, in days. Hourly rollups older than
// HourlyDays merge into daily ones; daily rollups older than DailyDays are deleted (0 keeps them).
type RollupTiers struct{ HourlyDays, DailyDays int }

// RetentionConfig configures the periodic maintenance cycle. Zero fields take defaults.
type RetentionConfig struct {
	RawRetentionDays  int // raw events older than this are deleted (0 disables)
	MaxRowsPerSession int // raw rows kept per session (0 disables)
	Tiers             RollupTiers
	Interval          time.Duration
	// ChunkPause is the wait between small transactions so queued writers get in.
	ChunkPause time.Duration
	// AgeChunksPerRun bounds how many age-eviction chunks one cycle deletes; a large
	// backlog drains over several cycles instead of monopolizing the writer connection.
	AgeChunksPerRun   int
	BackfillChunkRows int
	Now               func() time.Time
}

const (
	defaultHourlyRollupDays = 14
	defaultDailyRollupDays  = 730
	defaultAgeChunksPerRun  = 200
)

func (c RetentionConfig) withDefaults() RetentionConfig {
	if c.Tiers.HourlyDays <= 0 {
		c.Tiers.HourlyDays = defaultHourlyRollupDays
	}
	if c.Tiers.DailyDays == 0 {
		c.Tiers.DailyDays = defaultDailyRollupDays
	}
	if c.Interval <= 0 {
		c.Interval = 10 * time.Minute
	}
	if c.ChunkPause < 0 {
		c.ChunkPause = 0
	}
	if c.AgeChunksPerRun <= 0 {
		c.AgeChunksPerRun = defaultAgeChunksPerRun
	}
	if c.BackfillChunkRows <= 0 {
		c.BackfillChunkRows = defaultBackfillChunkRows
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Retention keeps the escape-events database bounded: it backfills and compacts rollups and
// then trims raw rows by age and by per-session cap. It runs off the writer's hot path.
type Retention struct {
	client *ent.Client
	drv    dialect.Driver // nil: rollups disabled, raw trimming runs unconditionally
	cfg    RetentionConfig

	pauseHook func(context.Context) error // tests replace the inter-chunk wait
}

// NewRetention returns the maintenance job. drv may be nil when rollups are not in use.
func NewRetention(client *ent.Client, drv dialect.Driver, cfg RetentionConfig) *Retention {
	return &Retention{client: client, drv: drv, cfg: cfg.withDefaults()}
}

// Start runs one cycle at startup and then every Interval until ctx is cancelled.
// The goroutine is tracked on wg.
func (r *Retention) Start(ctx context.Context, wg *sync.WaitGroup) {
	if r == nil || r.client == nil {
		return
	}
	wg.Go(func() {
		ticker := time.NewTicker(r.cfg.Interval)
		defer ticker.Stop()
		r.Run(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.Run(ctx)
			}
		}
	})
}

// Run performs one cycle. Raw rows are deleted only once every pre-existing row is counted
// in a rollup, so no history is lost to a deletion that outruns the backfill.
func (r *Retention) Run(ctx context.Context) {
	if r.drv != nil {
		if err := r.maintainRollups(ctx); err != nil {
			log.Warn("analytics/retention escape rollup maintenance failed", "err", err)
		}
		st, err := readRollupState(ctx, r.drv)
		if err != nil || !st.BackfillDone {
			return
		}
	}
	r.evictByAge(ctx)
	r.evictOverCap(ctx)
}

func (r *Retention) maintainRollups(ctx context.Context) error {
	if _, err := ensureRollupState(ctx, r.drv, r.cfg.Now()); err != nil {
		return err
	}
	if err := backfillRollups(ctx, r.drv, r.cfg.BackfillChunkRows, r.pause); err != nil {
		return err
	}
	return compactRollups(ctx, r.drv, r.cfg.Now(), r.cfg.Tiers)
}

// pause waits ChunkPause or until ctx is cancelled.
func (r *Retention) pause(ctx context.Context) error {
	if r.pauseHook != nil {
		return r.pauseHook(ctx)
	}
	if r.cfg.ChunkPause <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(r.cfg.ChunkPause):
		return nil
	}
}

// evictByAge deletes raw rows older than RawRetentionDays in chunks, at most AgeChunksPerRun per cycle.
func (r *Retention) evictByAge(ctx context.Context) {
	if r.cfg.RawRetentionDays <= 0 {
		return
	}
	// Local: wall_time is stored as local-zone text and compared as text.
	cutoff := r.cfg.Now().In(time.Local).AddDate(0, 0, -r.cfg.RawRetentionDays)
	total := 0
	for range r.cfg.AgeChunksPerRun {
		ids, err := r.client.EscapeEvent.Query().Where(escapeevent.WallTimeLT(cutoff)).
			Limit(escapeEvictChunkRows).IDs(ctx)
		if err != nil || len(ids) == 0 {
			if err != nil {
				log.Warn("analytics/retention escape_event age eviction failed", "err", err)
			}
			break
		}
		deleted, err := r.client.EscapeEvent.Delete().Where(escapeevent.IDIn(ids...)).Exec(ctx)
		total += deleted
		if err != nil {
			log.Warn("analytics/retention escape_event age eviction failed", "err", err)
			break
		}
		if r.pause(ctx) != nil {
			break
		}
	}
	if total > 0 {
		log.Info("analytics/retention escape_event age eviction deleted rows", "deleted", total, "cutoff", cutoff.Format(time.RFC3339))
	}
}

// evictOverCap removes the oldest rows above the per-session cap. One grouped scan of the
// (session_id, session_seq) index finds the over-cap sessions.
func (r *Retention) evictOverCap(ctx context.Context) {
	maxRows := r.cfg.MaxRowsPerSession
	if maxRows <= 0 {
		return
	}
	var counts []struct {
		SessionID string `json:"session_id"`
		Count     int    `json:"count"`
	}
	if err := r.client.EscapeEvent.Query().GroupBy(escapeevent.FieldSessionID).Aggregate(ent.Count()).Scan(ctx, &counts); err != nil {
		log.Warn("analytics/retention escape_event session count query failed", "err", err)
		return
	}
	for _, c := range counts {
		if c.Count <= maxRows {
			continue
		}
		deleted, err := evictOldestEscapeEvents(ctx, r.client, c.SessionID, c.Count-maxRows, r.pause)
		if err != nil {
			log.Warn("analytics/retention escape_event per-session eviction failed", "err", err, "session_id", c.SessionID)
		}
		if deleted > 0 {
			log.Info("analytics/retention escape_event per-session eviction deleted rows", "deleted", deleted, "session_id", c.SessionID, "limit", maxRows)
		}
	}
}
