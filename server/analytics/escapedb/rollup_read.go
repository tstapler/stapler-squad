package escapedb

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
)

// AggregateRow is one group of the escape-event aggregate: the dimensions the analytics
// summaries need and how many events fell in each.
type AggregateRow struct {
	SessionID    string `json:"session_id"`
	ProjectPath  string `json:"project_path"`
	SequenceType string `json:"sequence_type"`
	Stage        string `json:"stage"`
	Mangled      bool   `json:"mangled"`
	MangleType   string `json:"mangle_type"`
	Count        int64  `json:"count"`
}

// AggregateQuery selects a session (empty means all) and a closed time range (nil bounds are open).
type AggregateQuery struct {
	SessionID  string
	Start, End *time.Time
}

// Reader answers aggregate queries from rollups for old ranges and raw rows for the ragged
// edges of a range, so history outlives raw-row eviction. Without rollups (nil driver, or a
// backfill still running) it falls back to counting raw rows.
type Reader struct {
	client *ent.Client
	drv    dialect.ExecQuerier // nil: raw only
	tiers  RollupTiers
	now    func() time.Time
}

// NewReader returns a Reader. drv may be nil; tiers.HourlyDays and now default as in RetentionConfig.
func NewReader(client *ent.Client, drv dialect.ExecQuerier, tiers RollupTiers, now func() time.Time) *Reader {
	cfg := RetentionConfig{Tiers: tiers, Now: now}.withDefaults()
	return &Reader{client: client, drv: drv, tiers: cfg.Tiers, now: cfg.Now}
}

// Aggregate returns event counts grouped by session, project, sequence type, stage and mangle outcome.
//
// A range is served as full rollup buckets plus raw rows for the partial hours at its
// ends. Where the range reaches into the daily tier, whole UTC days that overlap it are
// included, so those edges are snapped outward to day boundaries.
func (r *Reader) Aggregate(ctx context.Context, q AggregateQuery) ([]AggregateRow, error) {
	if r.drv == nil {
		return r.rawAggregate(ctx, q.SessionID, rawEdge{lo: q.Start, hi: q.End, hiInclusive: true})
	}
	st, err := readRollupState(ctx, r.drv)
	if err != nil {
		return nil, err
	}
	if !st.BackfillDone {
		return r.rawAggregate(ctx, q.SessionID, rawEdge{lo: q.Start, hi: q.End, hiInclusive: true})
	}
	merged := make(map[AggregateRow]int64)
	rolled, err := r.rollupAggregate(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, row := range rolled {
		merged[AggregateRow{row.SessionID, row.ProjectPath, row.SequenceType, row.Stage, row.Mangled, row.MangleType, 0}] += row.Count
	}
	for _, e := range r.rawEdges(q) {
		rows, err := r.rawAggregate(ctx, q.SessionID, e)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			merged[AggregateRow{row.SessionID, row.ProjectPath, row.SequenceType, row.Stage, row.Mangled, row.MangleType, 0}] += row.Count
		}
	}
	out := make([]AggregateRow, 0, len(merged))
	for k, n := range merged {
		k.Count = n
		out = append(out, k)
	}
	return out, nil
}

type rawEdge struct {
	lo, hi      *time.Time
	hiInclusive bool
}

// rawEdges lists the sub-ranges of q that rollup buckets do not cover: the partial hour at
// each end, when that end lies in the hourly tier.
func (r *Reader) rawEdges(q AggregateQuery) []rawEdge {
	boundary := time.Unix(bucketStart(r.now().AddDate(0, 0, -r.tiers.HourlyDays), RollupDaySecs), 0)
	hourOf := func(t time.Time) time.Time { return time.Unix(bucketStart(t, RollupHourSecs), 0) }
	if q.Start != nil && q.End != nil && hourOf(*q.Start).Equal(hourOf(*q.End)) && !q.Start.Before(boundary) {
		return []rawEdge{{lo: q.Start, hi: q.End, hiInclusive: true}}
	}
	var edges []rawEdge
	if q.Start != nil && !q.Start.Before(boundary) && !q.Start.Equal(hourOf(*q.Start)) {
		next := hourOf(*q.Start).Add(time.Hour)
		edges = append(edges, rawEdge{lo: q.Start, hi: &next})
	}
	if q.End != nil && !q.End.Before(boundary) {
		floor := hourOf(*q.End)
		edges = append(edges, rawEdge{lo: &floor, hi: q.End, hiInclusive: true})
	}
	return edges
}

func (r *Reader) rollupAggregate(ctx context.Context, q AggregateQuery) ([]AggregateRow, error) {
	lo, hi := int64(math.MinInt64/2), int64(math.MaxInt64/2)
	if q.Start != nil {
		lo = ceilSeconds(*q.Start)
	}
	if q.End != nil {
		hi = q.End.Unix()
	}
	// Hourly buckets count only if they lie wholly inside the range; daily ones if they overlap it.
	where := []string{
		"((bucket_secs = ? AND bucket_start >= ? AND bucket_start + bucket_secs <= ?) OR (bucket_secs = ? AND bucket_start + bucket_secs > ? AND bucket_start <= ?))",
	}
	args := []any{RollupHourSecs, lo, hi, RollupDaySecs, lo, hi}
	if q.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, q.SessionID)
	}
	var rows entsql.Rows
	query := "SELECT session_id, project_path, sequence_type, stage, mangled, mangle_type, SUM(event_count) FROM " + rollupTable +
		" WHERE " + strings.Join(where, " AND ") + " GROUP BY session_id, project_path, sequence_type, stage, mangled, mangle_type"
	if err := r.drv.Query(ctx, query, args, &rows); err != nil {
		return nil, fmt.Errorf("escape rollup read: %w", err)
	}
	defer rows.Close()
	var out []AggregateRow
	for rows.Next() {
		var a AggregateRow
		if err := rows.Scan(&a.SessionID, &a.ProjectPath, &a.SequenceType, &a.Stage, &a.Mangled, &a.MangleType, &a.Count); err != nil {
			return nil, fmt.Errorf("escape rollup read scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func ceilSeconds(t time.Time) int64 {
	if t.Nanosecond() == 0 {
		return t.Unix()
	}
	return t.Unix() + 1
}

// rawAggregate counts raw rows with wall_time in [rng.lo, rng.hi) (or [lo, hi] when hiInclusive).
func (r *Reader) rawAggregate(ctx context.Context, sessionID string, rng rawEdge) ([]AggregateRow, error) {
	lo, hi, hiInclusive := rng.lo, rng.hi, rng.hiInclusive
	var filters []predicate.EscapeEvent
	if sessionID != "" {
		filters = append(filters, escapeevent.SessionID(sessionID))
	}
	// wall_time is stored as local-zone text and compared as text, so bounds must be local too.
	if lo != nil {
		filters = append(filters, escapeevent.WallTimeGTE(lo.In(time.Local)))
	}
	if hi != nil && hiInclusive {
		filters = append(filters, escapeevent.WallTimeLTE(hi.In(time.Local)))
	} else if hi != nil {
		filters = append(filters, escapeevent.WallTimeLT(hi.In(time.Local)))
	}
	var out []AggregateRow
	err := r.client.EscapeEvent.Query().Where(filters...).
		GroupBy(
			escapeevent.FieldSessionID, escapeevent.FieldProjectPath, escapeevent.FieldSequenceType,
			escapeevent.FieldStage, escapeevent.FieldMangled, escapeevent.FieldMangleType,
		).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &out)
	if err != nil {
		return nil, fmt.Errorf("escape raw aggregate: %w", err)
	}
	return out, nil
}
