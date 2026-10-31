package escapedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

var errSimulatedCrash = errors.New("simulated crash before commit")

func noPause(context.Context, time.Duration) error { return nil }

// seedLegacy creates analytics.db under dir holding n escape events with random ids.
func seedLegacy(ctx context.Context, t *testing.T, dir string, n int) *ent.Client {
	t.Helper()
	legacy, err := analytics.OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = legacy.Close() })
	rng := rand.New(rand.NewSource(7))
	base := time.Now().Add(-time.Hour).Round(0)
	for lo := 0; lo < n; lo += 200 {
		var creators []*ent.EscapeEventCreate
		for i := lo; i < min(lo+200, n); i++ {
			creators = append(creators, legacy.EscapeEvent.Create().
				SetID(fmt.Sprintf("%016x", rng.Uint64())).SetSessionID(fmt.Sprintf("s%d", i%7)).
				SetStage("pty_read").SetSequenceType("SGR").SetByteLength(i).SetRawBytes([]byte{byte(i), 1}).
				SetWallTime(base.Add(time.Duration(i)*time.Second)).SetSessionSeq(int64(i)))
		}
		require.NoError(t, legacy.EscapeEvent.CreateBulk(creators...).Exec(ctx))
	}
	return legacy
}

func dumpRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT id || '|' || session_id || '|' || byte_length || '|' || hex(raw_bytes) || '|' || CAST(wall_time AS TEXT) || '|' || session_seq FROM escape_events ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		require.NoError(t, rows.Scan(&r))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func openDest(ctx context.Context, t *testing.T, dir string) (*ent.Client, *dbpool.Database) {
	t.Helper()
	client, db, err := Open(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client, db
}

func TestMigrate_should_CopyEveryRowIdentically_And_LeaveSourceUntouched(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy := seedLegacy(ctx, t, dir, 1200)
	_, dst := openDest(ctx, t, dir)

	st, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), filepath.Join(dir, "analytics.db"), MigrationConfig{InitialBatch: 100, Pause: noPause})
	require.NoError(t, err)

	assert.True(t, st.Done())
	assert.EqualValues(t, 1200, st.Scanned)
	assert.EqualValues(t, 1200, st.Inserted)
	srcRows, err := legacy.EscapeEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1200, srcRows, "legacy table is left in place")

	legacyDB, err := dbpool.OpenForeignReadOnly(filepath.Join(dir, "analytics.db"), time.Second)
	require.NoError(t, err)
	defer legacyDB.Close()
	assert.Equal(t, dumpRows(t, legacyDB), dumpRows(t, dst.Writer()))

	// A finished ledger makes the next start a no-op.
	again, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), filepath.Join(dir, "analytics.db"), MigrationConfig{Pause: noPause})
	require.NoError(t, err)
	assert.True(t, again.Done())
	assert.EqualValues(t, 1200, again.Inserted)
}

func TestMigrate_should_ResumeFromCursor_When_CrashHitsMidCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seedLegacy(ctx, t, dir, 1000)
	_, dst := openDest(ctx, t, dir)
	srcPath := filepath.Join(dir, "analytics.db")

	batches := 0
	crash := MigrationConfig{InitialBatch: 100, Pause: noPause, BeforeCommit: func() error {
		batches++
		if batches == 3 {
			return errSimulatedCrash
		}
		return nil
	}}
	st, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), srcPath, crash)
	require.ErrorIs(t, err, errSimulatedCrash)
	assert.False(t, st.Done())

	// The failed batch rolled back: destination rows equal the ledger, no partial batch.
	var rows int64
	require.NoError(t, dst.Writer().QueryRow("SELECT COUNT(*) FROM escape_events").Scan(&rows))
	var ledgerInserted, ledgerScanned int64
	require.NoError(t, dst.Writer().QueryRow("SELECT inserted, scanned FROM escape_events_migration").Scan(&ledgerInserted, &ledgerScanned))
	assert.Equal(t, rows, ledgerInserted)
	assert.Equal(t, rows, ledgerScanned)
	assert.Positive(t, rows)
	assert.Less(t, rows, int64(1000))

	st, err = MigrateFromAnalyticsDB(ctx, dst.Writer(), srcPath, MigrationConfig{InitialBatch: 100, Pause: noPause})
	require.NoError(t, err)
	assert.True(t, st.Done())
	assert.EqualValues(t, 1000, st.Scanned)

	legacyDB, err := dbpool.OpenForeignReadOnly(srcPath, time.Second)
	require.NoError(t, err)
	defer legacyDB.Close()
	assert.Equal(t, dumpRows(t, legacyDB), dumpRows(t, dst.Writer()), "resumed copy ends with identical rows")
}

func TestMigrate_should_CatchRowsLandingBelowTheCursor_When_SourceGrowsDuringCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy := seedLegacy(ctx, t, dir, 500)
	_, dst := openDest(ctx, t, dir)

	injected := false
	cfg := MigrationConfig{InitialBatch: 100, Pause: noPause, BeforeCommit: func() error {
		if injected {
			return nil
		}
		injected = true // an old binary wrote a row whose id sorts before the cursor
		return legacy.EscapeEvent.Create().SetID("0000000000000000").SetSessionID("late").SetStage("x").
			SetSequenceType("SGR").SetByteLength(1).SetWallTime(time.Now().Round(0)).SetSessionSeq(1).Exec(ctx)
	}}
	st, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), filepath.Join(dir, "analytics.db"), cfg)
	require.NoError(t, err)
	assert.True(t, st.Done())
	assert.Equal(t, 1, st.Pass, "count mismatch forced one verification re-scan")
	var n int
	require.NoError(t, dst.Writer().QueryRow("SELECT COUNT(*) FROM escape_events").Scan(&n))
	assert.Equal(t, 501, n)
}

func TestMigrate_should_FinishImmediately_When_NoLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, dst := openDest(ctx, t, dir)
	st, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), filepath.Join(dir, "analytics.db"), MigrationConfig{Pause: noPause})
	require.NoError(t, err)
	assert.True(t, st.Done())
	assert.Zero(t, st.Scanned)
}

func TestMigrate_should_RefuseWritesToAttachedSource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seedLegacy(ctx, t, dir, 3)
	_, dst := openDest(ctx, t, dir)
	conn, err := dst.Writer().Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "ATTACH DATABASE ? AS src", "file:"+filepath.Join(dir, "analytics.db")+"?mode=ro")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "DELETE FROM src.escape_events")
	require.Error(t, err, "the source is attached read-only")
}

func TestAdaptBatch_should_KeepWriterHoldNearTarget(t *testing.T) {
	m := &migrator{cfg: MigrationConfig{}.withDefaults()}
	assert.Equal(t, 250, m.adaptBatch(500, 80*time.Millisecond), "slow batch halves")
	assert.Equal(t, minBatchRows, m.adaptBatch(minBatchRows, time.Second), "floor")
	assert.Equal(t, 1000, m.adaptBatch(500, 5*time.Millisecond), "fast batch doubles")
	assert.Equal(t, maxBatchRows, m.adaptBatch(maxBatchRows, time.Millisecond), "ceiling")
	assert.Equal(t, 500, m.adaptBatch(500, 30*time.Millisecond), "in band keeps size")
}

// Two files, two single-connection writers: a held analytics writer must not
// delay an escape insert, which is the contention the split removes.
func TestEscapeInsert_should_NotQueueBehindAnalyticsWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	analyticsClient, err := analytics.OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer analyticsClient.Close()
	escape := openForTest(ctx, t, dir)

	// Pin analytics.db's only writer connection inside a write transaction.
	held, err := analyticsClient.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = held.Rollback() }()
	_, err = held.AnalyticsEvent.Create().SetID("held").SetEventName("e").SetEventCategory("c").Save(ctx)
	require.NoError(t, err)

	insert := func(c *ent.Client, timeout time.Duration) error {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return c.EscapeEvent.Create().SetID(fmt.Sprintf("e%d", time.Now().UnixNano())).SetSessionID("s").SetStage("x").
			SetSequenceType("SGR").SetByteLength(1).SetWallTime(time.Now().Round(0)).SetSessionSeq(1).Exec(cctx)
	}
	require.NoError(t, insert(escape, 5*time.Second), "escape insert has its own writer")
	// Control: the same insert through the held analytics.db writer cannot proceed.
	err = insert(analyticsClient, 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded, "analytics.db's single writer is pinned, so the insert waits out its deadline")
}

func TestMigrate_should_KeepLegacyRowsWithNullColumns_Not_DropThemSilently(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "analytics.db"))
	require.NoError(t, err)
	for _, q := range []string{
		"CREATE TABLE escape_events (id text PRIMARY KEY, session_id text, project_path text, stage text, sequence_type text, sequence_subtype text, sequence_signature text, byte_length integer, payload_hash text, raw_bytes blob, mangled integer, mangle_type text, wall_time datetime, session_seq integer)",
		"INSERT INTO escape_events (id, session_id, stage, sequence_type, byte_length, mangled, wall_time, session_seq) VALUES ('a','s','x','SGR',1,0,'2026-01-01 00:00:00+00:00',1)",
		"INSERT INTO escape_events (id, session_id) VALUES ('b','s')",
		"INSERT INTO escape_events (id) VALUES ('c')",
	} {
		_, err := legacy.Exec(q)
		require.NoError(t, err)
	}
	require.NoError(t, legacy.Close())
	_, dst := openDest(ctx, t, dir)

	st, err := MigrateFromAnalyticsDB(ctx, dst.Writer(), filepath.Join(dir, "analytics.db"), MigrationConfig{Pause: noPause})
	require.NoError(t, err)
	assert.True(t, st.Done())
	assert.EqualValues(t, 3, st.Scanned)
	assert.EqualValues(t, 3, st.Inserted)
	var n int
	require.NoError(t, dst.Writer().QueryRow("SELECT COUNT(*) FROM escape_events").Scan(&n))
	assert.Equal(t, 3, n)

	// ent must be able to read the defaulted rows back.
	client := openForTest(ctx, t, dir)
	got, err := client.EscapeEvent.Query().Where(escapeevent.IDIn("a", "b", "c")).Order(ent.Asc("id")).All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "", got[2].SessionID)
	assert.EqualValues(t, 0, got[2].SessionSeq)
	assert.False(t, got[2].Mangled)
	assert.Equal(t, 1970, got[2].WallTime.Year())
}
