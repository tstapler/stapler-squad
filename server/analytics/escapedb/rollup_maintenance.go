package escapedb

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/log"
)

const (
	defaultBackfillChunkRows = 5000
	maxCompactDaysPerRun     = 31
)

// backfillRollups folds the raw rows that predate write-time counting into hourly rollups,
// then marks the backfill done. It reads the table in id order, one chunk per transaction,
// and advances a persisted cursor in the same transaction as each chunk's additive upsert,
// so a crash or restart resumes without double counting. Rows at or after the state's
// live_since were already counted by the writer and are skipped.
func backfillRollups(ctx context.Context, drv dialect.Driver, chunkRows int, pause func(context.Context) error) error {
	for {
		st, err := readRollupState(ctx, drv)
		if err != nil || st.BackfillDone {
			return err
		}
		set, lastID, scanned, err := scanBackfillChunk(ctx, drv, st, chunkRows)
		if err != nil {
			return err
		}
		if scanned == 0 {
			return drv.Exec(ctx, `UPDATE `+rollupStateTable+` SET backfill_done = 1 WHERE id = 1`, []any{}, nil)
		}
		err = withTx(ctx, drv, func(tx dialect.Tx) error {
			if err := upsertRollups(ctx, tx, set); err != nil {
				return err
			}
			return tx.Exec(ctx, `UPDATE `+rollupStateTable+` SET backfill_cursor = ?, backfilled_rows = backfilled_rows + ? WHERE id = 1`,
				[]any{lastID, scanned}, nil)
		})
		if err != nil {
			return fmt.Errorf("escape rollup backfill: %w", err)
		}
		if err := pause(ctx); err != nil {
			return err
		}
	}
}

// scanBackfillChunk reads the next chunkRows raw rows after the cursor and aggregates the
// pre-live ones. scanned counts every row read, so the cursor advances past skipped rows.
func scanBackfillChunk(ctx context.Context, drv dialect.Driver, st rollupState, chunkRows int) (set rollupSet, lastID string, scanned int64, err error) {
	var rows entsql.Rows
	const q = "SELECT `id`, `session_id`, COALESCE(`project_path`, ''), `stage`, `sequence_type`, `byte_length`, `mangled`, COALESCE(`mangle_type`, ''), `wall_time` " +
		"FROM `escape_events` WHERE `id` > ? ORDER BY `id` LIMIT ?"
	if err = drv.Query(ctx, q, []any{st.BackfillCursor, chunkRows}, &rows); err != nil {
		return nil, "", 0, fmt.Errorf("escape rollup backfill read: %w", err)
	}
	defer rows.Close()
	set = make(rollupSet)
	for rows.Next() {
		var f eventFacts
		var wall any
		if err = rows.Scan(&lastID, &f.SessionID, &f.ProjectPath, &f.Stage, &f.SequenceType, &f.ByteLen, &f.Mangled, &f.MangleType, &wall); err != nil {
			return nil, "", 0, fmt.Errorf("escape rollup backfill scan: %w", err)
		}
		scanned++
		if f.At, err = parseWallTime(wall); err != nil {
			log.Warn("escape rollup backfill: skipping row", "id", lastID, "err", err)
			err = nil
			continue
		}
		if f.At.Before(st.LiveSince) {
			set.add(RollupHourSecs, f)
		}
	}
	return set, lastID, scanned, rows.Err()
}

// compactRollups merges whole UTC days of hourly rollups older than tiers.HourlyDays into
// daily rollups (one transaction per day, insert-then-delete so readers never see both or
// neither) and prunes daily rollups older than tiers.DailyDays (0 keeps them forever).
func compactRollups(ctx context.Context, drv dialect.Driver, now time.Time, tiers RollupTiers) error {
	cutoff := bucketStart(now.AddDate(0, 0, -tiers.HourlyDays), RollupDaySecs)
	days, err := hourlyDaysBefore(ctx, drv, cutoff)
	if err != nil {
		return err
	}
	for _, day := range days {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := compactDay(ctx, drv, day); err != nil {
			return err
		}
	}
	if tiers.DailyDays <= 0 {
		return nil
	}
	pruneBefore := bucketStart(now.AddDate(0, 0, -tiers.DailyDays), RollupDaySecs)
	return drv.Exec(ctx, `DELETE FROM `+rollupTable+` WHERE bucket_secs = ? AND bucket_start < ?`, []any{RollupDaySecs, pruneBefore}, nil)
}

func hourlyDaysBefore(ctx context.Context, q dialect.ExecQuerier, cutoff int64) ([]int64, error) {
	var rows entsql.Rows
	err := q.Query(ctx, `SELECT DISTINCT bucket_start - (bucket_start % ?) AS day FROM `+rollupTable+
		` WHERE bucket_secs = ? AND bucket_start < ? ORDER BY day LIMIT ?`,
		[]any{RollupDaySecs, RollupHourSecs, cutoff, maxCompactDaysPerRun}, &rows)
	if err != nil {
		return nil, fmt.Errorf("escape rollup compaction scan: %w", err)
	}
	defer rows.Close()
	var days []int64
	for rows.Next() {
		var d int64
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("escape rollup compaction scan: %w", err)
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

func compactDay(ctx context.Context, drv dialect.Driver, day int64) error {
	return withTx(ctx, drv, func(tx dialect.Tx) error {
		insert := rollupInsertPrefix[:len(rollupInsertPrefix)-len(" VALUES ")] +
			` SELECT ?, ?, session_id, project_path, sequence_type, stage, mangled, mangle_type, SUM(event_count), SUM(byte_total), MIN(first_seen_ms), MAX(last_seen_ms)
			FROM ` + rollupTable + ` WHERE bucket_secs = ? AND bucket_start >= ? AND bucket_start < ?
			GROUP BY session_id, project_path, sequence_type, stage, mangled, mangle_type` + rollupConflictClause
		args := []any{RollupDaySecs, day, RollupHourSecs, day, day + RollupDaySecs}
		if err := tx.Exec(ctx, insert, args, nil); err != nil {
			return fmt.Errorf("escape rollup compact day %d: %w", day, err)
		}
		return tx.Exec(ctx, `DELETE FROM `+rollupTable+` WHERE bucket_secs = ? AND bucket_start >= ? AND bucket_start < ?`,
			[]any{RollupHourSecs, day, day + RollupDaySecs}, nil)
	})
}
