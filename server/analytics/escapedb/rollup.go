package escapedb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// Rollup tiers. Buckets are aligned to UTC so they never shift with DST.
const (
	RollupHourSecs int64 = 3600
	RollupDaySecs  int64 = 86400

	rollupTable      = "escape_event_rollups"
	rollupStateTable = "escape_event_rollup_state"

	// rollupUpsertChunk bounds one multi-row upsert to 12 bind variables per row.
	rollupUpsertChunk = 200
)

// rollupSchema is created by Open. The table is plain SQL rather than an ent entity: the
// additive ON CONFLICT upsert (count += excluded.count, MIN/MAX on first/last seen)
// is not expressible through ent's bulk-upsert builder.
var rollupSchema = []string{
	`CREATE TABLE IF NOT EXISTS ` + rollupTable + ` (
		bucket_secs   INTEGER NOT NULL,
		bucket_start  INTEGER NOT NULL,
		session_id    TEXT    NOT NULL,
		project_path  TEXT    NOT NULL,
		sequence_type TEXT    NOT NULL,
		stage         TEXT    NOT NULL,
		mangled       INTEGER NOT NULL,
		mangle_type   TEXT    NOT NULL,
		event_count   INTEGER NOT NULL,
		byte_total    INTEGER NOT NULL,
		first_seen_ms INTEGER NOT NULL,
		last_seen_ms  INTEGER NOT NULL,
		PRIMARY KEY (bucket_secs, bucket_start, session_id, project_path, sequence_type, stage, mangled, mangle_type)
	) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS escape_event_rollups_session ON ` + rollupTable + ` (session_id, bucket_secs, bucket_start)`,
	`CREATE TABLE IF NOT EXISTS ` + rollupStateTable + ` (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		live_since_ms   INTEGER NOT NULL,
		backfill_cursor TEXT    NOT NULL DEFAULT '',
		backfill_done   INTEGER NOT NULL DEFAULT 0,
		backfilled_rows INTEGER NOT NULL DEFAULT 0
	)`,
}

// EnsureRollupSchema creates the rollup and state tables if missing.
func EnsureRollupSchema(ctx context.Context, drv dialect.ExecQuerier) error {
	for _, stmt := range rollupSchema {
		if err := drv.Exec(ctx, stmt, []any{}, nil); err != nil {
			return fmt.Errorf("escape rollup schema: %w", err)
		}
	}
	return nil
}

// rollupState is the single row in escape_event_rollup_state.
type rollupState struct {
	Exists         bool
	LiveSince      time.Time // rows with wall_time at or after this were counted at write time
	BackfillCursor string
	BackfillDone   bool
	BackfilledRows int64
}

// ensureRollupState inserts the state row on first start, stamping liveSince = now: from
// that moment the writer counts every flushed event, and backfill covers only older rows.
func ensureRollupState(ctx context.Context, q dialect.ExecQuerier, now time.Time) (rollupState, error) {
	err := q.Exec(ctx, `INSERT OR IGNORE INTO `+rollupStateTable+` (id, live_since_ms) VALUES (1, ?)`, []any{now.UnixMilli()}, nil)
	if err != nil {
		return rollupState{}, fmt.Errorf("escape rollup state init: %w", err)
	}
	return readRollupState(ctx, q)
}

func readRollupState(ctx context.Context, q dialect.ExecQuerier) (rollupState, error) {
	var rows entsql.Rows
	err := q.Query(ctx, `SELECT live_since_ms, backfill_cursor, backfill_done, backfilled_rows FROM `+rollupStateTable+` WHERE id = 1`, []any{}, &rows)
	if err != nil {
		return rollupState{}, fmt.Errorf("escape rollup state read: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return rollupState{}, rows.Err()
	}
	var liveMS, done, n int64
	st := rollupState{Exists: true}
	if err := rows.Scan(&liveMS, &st.BackfillCursor, &done, &n); err != nil {
		return rollupState{}, fmt.Errorf("escape rollup state scan: %w", err)
	}
	st.LiveSince, st.BackfillDone, st.BackfilledRows = time.UnixMilli(liveMS), done != 0, n
	return st, rows.Err()
}

// rollupKey is the grouping identity of one rollup row.
type rollupKey struct {
	BucketSecs   int64
	BucketStart  int64
	SessionID    string
	ProjectPath  string
	SequenceType string
	Stage        string
	Mangled      bool
	MangleType   string
}

type rollupVal struct {
	Count, Bytes, FirstSeenMS, LastSeenMS int64
}

type rollupSet map[rollupKey]*rollupVal

// eventFacts is the part of an escape event that a rollup keeps.
type eventFacts struct {
	SessionID, ProjectPath, SequenceType, Stage, MangleType string
	Mangled                                                 bool
	ByteLen                                                 int64
	At                                                      time.Time
}

// bucketStart floors t to a UTC multiple of secs.
func bucketStart(t time.Time, secs int64) int64 {
	u := t.Unix()
	return u - ((u%secs)+secs)%secs
}

func (s rollupSet) add(secs int64, f eventFacts) {
	k := rollupKey{
		BucketSecs: secs, BucketStart: bucketStart(f.At, secs), SessionID: f.SessionID,
		ProjectPath: f.ProjectPath, SequenceType: f.SequenceType, Stage: f.Stage,
		Mangled: f.Mangled, MangleType: f.MangleType,
	}
	ms := f.At.UnixMilli()
	v := s[k]
	if v == nil {
		s[k] = &rollupVal{Count: 1, Bytes: f.ByteLen, FirstSeenMS: ms, LastSeenMS: ms}
		return
	}
	v.Count++
	v.Bytes += f.ByteLen
	v.FirstSeenMS = min(v.FirstSeenMS, ms)
	v.LastSeenMS = max(v.LastSeenMS, ms)
}

const rollupConflictClause = ` ON CONFLICT (bucket_secs, bucket_start, session_id, project_path, sequence_type, stage, mangled, mangle_type) DO UPDATE SET
	event_count = event_count + excluded.event_count,
	byte_total = byte_total + excluded.byte_total,
	first_seen_ms = MIN(first_seen_ms, excluded.first_seen_ms),
	last_seen_ms = MAX(last_seen_ms, excluded.last_seen_ms)`

const rollupInsertPrefix = `INSERT INTO ` + rollupTable + ` (bucket_secs, bucket_start, session_id, project_path, sequence_type, stage, mangled, mangle_type, event_count, byte_total, first_seen_ms, last_seen_ms) VALUES `

// upsertRollups adds every entry of set into the rollup table in multi-row statements.
// Counts are additive, so a caller must make the call atomic with whatever it counts.
func upsertRollups(ctx context.Context, q dialect.ExecQuerier, set rollupSet) error {
	if len(set) == 0 {
		return nil
	}
	keys := make([]rollupKey, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	for start := 0; start < len(keys); start += rollupUpsertChunk {
		chunk := keys[start:min(start+rollupUpsertChunk, len(keys))]
		var sb strings.Builder
		sb.WriteString(rollupInsertPrefix)
		args := make([]any, 0, len(chunk)*12)
		for i, k := range chunk {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?)")
			v := set[k]
			args = append(args, k.BucketSecs, k.BucketStart, k.SessionID, k.ProjectPath, k.SequenceType,
				k.Stage, k.Mangled, k.MangleType, v.Count, v.Bytes, v.FirstSeenMS, v.LastSeenMS)
		}
		sb.WriteString(rollupConflictClause)
		if err := q.Exec(ctx, sb.String(), args, nil); err != nil {
			return fmt.Errorf("escape rollup upsert: %w", err)
		}
	}
	return nil
}

func keyLess(a, b rollupKey) bool {
	if a.BucketSecs != b.BucketSecs {
		return a.BucketSecs < b.BucketSecs
	}
	if a.BucketStart != b.BucketStart {
		return a.BucketStart < b.BucketStart
	}
	return fmt.Sprint(a.SessionID, a.ProjectPath, a.SequenceType, a.Stage, a.Mangled, a.MangleType) <
		fmt.Sprint(b.SessionID, b.ProjectPath, b.SequenceType, b.Stage, b.Mangled, b.MangleType)
}

// withTx runs fn in one transaction on drv and commits it, or rolls back on error.
func withTx(ctx context.Context, drv dialect.Driver, fn func(tx dialect.Tx) error) error {
	tx, err := drv.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// parseWallTime reads a wall_time column that the driver returns as time.Time or as the
// Time.String() text the writer stores.
func parseWallTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		return parseWallTimeText(t)
	case []byte:
		return parseWallTimeText(string(t))
	case sql.NullTime:
		return t.Time, nil
	}
	return time.Time{}, fmt.Errorf("unsupported wall_time type %T", v)
}

func parseWallTimeText(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable wall_time %q", s)
}
