package escapedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/session/dbpool"
)

type dropFixture struct {
	dir     string
	drv     dialect.ExecQuerier
	dst     *dbpool.Database
	srcPath string
	dstPath string
}

// newDropFixture seeds n legacy rows and opens both databases; the copy has not run.
func newDropFixture(t *testing.T, n int) *dropFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	seedLegacy(ctx, t, dir, n) // opens and closes analytics.db via the analytics client
	legacy, drv, err := analytics.OpenAnalyticsDBWithDriver(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = legacy.Close() })
	_, dst := openDest(ctx, t, dir)
	return &dropFixture{dir: dir, drv: drv, dst: dst, srcPath: filepath.Join(dir, "analytics.db"), dstPath: Path(dir)}
}

func (f *dropFixture) migrate(t *testing.T) {
	t.Helper()
	st, err := MigrateFromAnalyticsDB(context.Background(), f.dst.Writer(), f.srcPath, MigrationConfig{InitialBatch: 100, Pause: noPause})
	require.NoError(t, err)
	require.True(t, st.Done())
}

func (f *dropFixture) drop(cfg LegacyDropConfig) (LegacyDropResult, error) {
	cfg.Pause = noPause
	return DropLegacyEscapeEvents(context.Background(), f.drv, f.srcPath, f.dstPath, cfg)
}

func rowCount(t *testing.T, path, table string) (int64, bool) {
	t.Helper()
	db, err := dbpool.OpenForeignReadOnly(path, time.Second)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var has int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&has))
	if has == 0 {
		return 0, false
	}
	var n int64
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n, true
}

func TestParseLegacyDropMode_should_DefaultToOff(t *testing.T) {
	for in, want := range map[string]LegacyDropMode{
		"": LegacyDropOff, "off": LegacyDropOff, "garbage": LegacyDropOff, "0": LegacyDropOff,
		"on": LegacyDropOn, "TRUE": LegacyDropOn, "1": LegacyDropOn,
		"dry-run": LegacyDropDryRun, " Dry-Run ": LegacyDropDryRun,
	} {
		assert.Equal(t, want, ParseLegacyDropMode(in), "input %q", in)
	}
}

func TestLegacyDrop_should_DoNothing_when_Off_even_if_everythingIsReady(t *testing.T) {
	f := newDropFixture(t, 300)
	f.migrate(t)

	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOff})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeDisabled, res.Outcome)
	n, has := rowCount(t, f.srcPath, "escape_events")
	assert.True(t, has)
	assert.EqualValues(t, 300, n)
}

func TestLegacyDrop_should_OnlyReport_when_DryRun(t *testing.T) {
	f := newDropFixture(t, 300)
	f.migrate(t)

	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropDryRun})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeDryRun, res.Outcome)
	assert.EqualValues(t, 300, res.LegacyRows)
	assert.EqualValues(t, 300, res.NewRows)
	assert.False(t, res.Dropped)
	n, has := rowCount(t, f.srcPath, "escape_events")
	assert.True(t, has)
	assert.EqualValues(t, 300, n, "dry run must not delete")
}

func TestLegacyDrop_should_DropTable_and_KeepNewDatabase_when_GuardsPass(t *testing.T) {
	f := newDropFixture(t, 1300)
	f.migrate(t)
	before := dumpRows(t, f.dst.Writer())

	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn, InitialChunk: 100})
	require.NoError(t, err)

	assert.Equal(t, DropOutcomeDropped, res.Outcome)
	assert.True(t, res.Dropped)
	assert.EqualValues(t, 1300, res.Deleted)
	_, has := rowCount(t, f.srcPath, "escape_events")
	assert.False(t, has, "legacy table must be gone")
	assert.Equal(t, before, dumpRows(t, f.dst.Writer()), "new database untouched")
}

func TestLegacyDrop_should_Refuse_when_MigrationLedgerMissing(t *testing.T) {
	f := newDropFixture(t, 200) // copy never ran: no ledger table in the new database

	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeMigrationIncomplete, res.Outcome)
	n, has := rowCount(t, f.srcPath, "escape_events")
	assert.True(t, has)
	assert.EqualValues(t, 200, n)
}

func TestLegacyDrop_should_Refuse_when_LedgerNotDone(t *testing.T) {
	for _, phase := range []string{phaseCopying, phaseFailed} {
		t.Run(phase, func(t *testing.T) {
			f := newDropFixture(t, 200)
			f.migrate(t)
			_, err := f.dst.Writer().Exec("UPDATE escape_events_migration SET phase = ? WHERE id = 1", phase)
			require.NoError(t, err)

			res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn})
			require.NoError(t, err)
			assert.Equal(t, DropOutcomeMigrationIncomplete, res.Outcome)
			n, has := rowCount(t, f.srcPath, "escape_events")
			assert.True(t, has)
			assert.EqualValues(t, 200, n)
		})
	}
}

func TestLegacyDrop_should_Refuse_when_NewDatabaseHasFewerRows(t *testing.T) {
	f := newDropFixture(t, 200)
	f.migrate(t)
	_, err := f.dst.Writer().Exec("DELETE FROM escape_events WHERE rowid IN (SELECT rowid FROM escape_events LIMIT 5)")
	require.NoError(t, err)

	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeCountMismatch, res.Outcome)
	assert.EqualValues(t, 200, res.LegacyRows)
	assert.EqualValues(t, 195, res.NewRows)
	n, has := rowCount(t, f.srcPath, "escape_events")
	assert.True(t, has)
	assert.EqualValues(t, 200, n)
}

func TestLegacyDrop_should_ResumeAfterCrash_and_BeIdempotent(t *testing.T) {
	f := newDropFixture(t, 1000)
	f.migrate(t)

	chunks := 0
	crash := errors.New("simulated crash")
	res, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn, InitialChunk: 50, TargetHold: time.Hour, BeforeChunk: func() error {
		chunks++
		if chunks == 3 {
			return crash
		}
		return nil
	}})
	require.ErrorIs(t, err, crash)
	assert.Equal(t, DropOutcomeError, res.Outcome)
	remaining, has := rowCount(t, f.srcPath, "escape_events")
	require.True(t, has, "table survives a mid-way crash")
	assert.Greater(t, remaining, int64(0))
	assert.Less(t, remaining, int64(1000))

	res, err = f.drop(LegacyDropConfig{Mode: LegacyDropOn})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeDropped, res.Outcome)
	assert.Equal(t, remaining, res.Deleted, "second run deletes only what was left")
	_, has = rowCount(t, f.srcPath, "escape_events")
	assert.False(t, has)

	res, err = f.drop(LegacyDropConfig{Mode: LegacyDropOn})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeNoLegacyTable, res.Outcome, "third run is a no-op")
}

func TestLegacyDrop_should_NoOp_when_AnalyticsDatabaseMissing(t *testing.T) {
	dir := t.TempDir()
	res, err := DropLegacyEscapeEvents(context.Background(), nil, filepath.Join(dir, "analytics.db"), Path(dir), LegacyDropConfig{Mode: LegacyDropOn})
	require.NoError(t, err)
	assert.Equal(t, DropOutcomeNoLegacyTable, res.Outcome)
}

func TestLegacyDrop_should_ReturnFreePages_when_AutoVacuumIncremental(t *testing.T) {
	f := newDropFixture(t, 3000)
	f.migrate(t)
	sizeBefore := pageCount(t, f.srcPath)

	_, err := f.drop(LegacyDropConfig{Mode: LegacyDropOn, InitialChunk: 500, VacuumPages: 64})
	require.NoError(t, err)

	assert.Less(t, pageCount(t, f.srcPath), sizeBefore, "incremental vacuum should shrink the file")
}

func pageCount(t *testing.T, path string) int64 {
	t.Helper()
	db, err := dbpool.OpenForeignReadOnly(path, time.Second)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var n int64
	require.NoError(t, db.QueryRow("PRAGMA page_count").Scan(&n))
	return n
}
