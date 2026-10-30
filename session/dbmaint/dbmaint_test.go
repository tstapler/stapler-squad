package dbmaint

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/tstapler/stapler-squad/session/dbobs"
)

// fakeClock drives ticks by hand and runs onPause instead of waiting.
type fakeClock struct {
	ticks   chan time.Time
	pauses  atomic.Int64
	onPause func(ctx context.Context) error
}

func newFakeClock() *fakeClock { return &fakeClock{ticks: make(chan time.Time)} }

func (c *fakeClock) NewTicker(time.Duration) (<-chan time.Time, func()) { return c.ticks, func() {} }

func (c *fakeClock) Pause(ctx context.Context, _ time.Duration) error {
	c.pauses.Add(1)
	if c.onPause != nil {
		return c.onPause(ctx)
	}
	return ctx.Err()
}

type warnLog struct{ n atomic.Int64 }

func (w *warnLog) warn(string, ...any) { w.n.Add(1) }

// openDB opens a single-writer WAL database. autoVacuum "" leaves the SQLite default (NONE).
func openDB(t *testing.T, autoVacuum string) (dialect.Driver, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.db")
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	if autoVacuum != "" {
		dsn += "&_pragma=auto_vacuum(" + autoVacuum + ")"
	}
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	drv := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, db), db, dbobs.Config{Name: t.Name(), Warn: func(string, ...any) {}})
	t.Cleanup(func() { _ = drv.Close() })
	return drv, path
}

func exec(t *testing.T, d dialect.ExecQuerier, q string, args ...any) {
	t.Helper()
	require.NoError(t, d.Exec(context.Background(), q, append([]any{}, args...), nil))
}

func scalar(t *testing.T, d dialect.ExecQuerier, q string) int64 {
	t.Helper()
	var rows entsql.Rows
	require.NoError(t, d.Query(context.Background(), q, []any{}, &rows))
	defer rows.Close()
	require.True(t, rows.Next())
	var v int64
	require.NoError(t, rows.Scan(&v))
	return v
}

// fillAndDelete creates roughly 1000+ free pages.
func fillAndDelete(t *testing.T, d dialect.ExecQuerier) {
	t.Helper()
	exec(t, d, "CREATE TABLE blob_t (id INTEGER PRIMARY KEY, b BLOB)")
	exec(t, d, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 2000)
		INSERT INTO blob_t (b) SELECT zeroblob(2048) FROM n`)
	exec(t, d, "DELETE FROM blob_t")
}

func testConfig() Config {
	return Config{Interval: time.Minute, FreeRatio: 0.2, MinFreePages: 100, ChunkPages: 100, ChunkPause: time.Millisecond}
}

func TestTick_ShouldReclaimFreePagesInChunks_When_IncrementalAndOverThreshold(t *testing.T) {
	drv, path := openDB(t, "incremental")
	fillAndDelete(t, drv)
	freeBefore := scalar(t, drv, "PRAGMA freelist_count")
	require.Greater(t, freeBefore, int64(500))
	exec(t, drv, "PRAGMA wal_checkpoint(TRUNCATE)")
	sizeBefore := fileSize(t, path)

	clock := newFakeClock()
	var writes int
	clock.onPause = func(ctx context.Context) error {
		// The writer connection must be free while the maintainer pauses.
		exec(t, drv, "INSERT INTO blob_t (b) VALUES (zeroblob(10))")
		writes++
		return ctx.Err()
	}
	w := &warnLog{}
	m := New("t", drv, testConfig(), WithClock(clock), WithWarn(w.warn))

	require.NoError(t, m.Tick(context.Background()))

	assert.Less(t, scalar(t, drv, "PRAGMA freelist_count"), int64(100), "freelist drained")
	assert.Greater(t, clock.pauses.Load(), int64(1), "reclaimed in multiple chunks")
	assert.Equal(t, int(clock.pauses.Load()), writes, "writer wrote between every chunk")
	exec(t, drv, "PRAGMA wal_checkpoint(TRUNCATE)")
	assert.Less(t, fileSize(t, path), sizeBefore, "file shrank")
	st := m.snapshot()
	assert.Equal(t, "incremental", st.AutoVacuumMode)
	assert.GreaterOrEqual(t, st.ReclaimedPagesTotal, freeBefore-100)
	assert.Equal(t, int64(1), st.Runs)
	assert.Empty(t, st.LastError)
	assert.Zero(t, w.n.Load())
}

func TestTick_ShouldWarnOnceAndNeverVacuum_When_AutoVacuumNone(t *testing.T) {
	drv, _ := openDB(t, "")
	fillAndDelete(t, drv)
	freeBefore := scalar(t, drv, "PRAGMA freelist_count")
	clock := newFakeClock()
	w := &warnLog{}
	m := New("t", drv, testConfig(), WithClock(clock), WithWarn(w.warn))

	require.NoError(t, m.Tick(context.Background()))
	require.NoError(t, m.Tick(context.Background()))

	assert.Equal(t, freeBefore, scalar(t, drv, "PRAGMA freelist_count"), "no vacuum ran")
	assert.Equal(t, int64(0), scalar(t, drv, "PRAGMA auto_vacuum"), "mode not converted")
	assert.Equal(t, int64(1), w.n.Load(), "one WARN across ticks")
	assert.Zero(t, clock.pauses.Load())
	st := m.snapshot()
	assert.True(t, st.NeedsOfflineVacuum)
	assert.Equal(t, "none", st.AutoVacuumMode)
	assert.Equal(t, freeBefore, st.FreePages)
}

func TestTick_ShouldSkip_When_BelowThreshold(t *testing.T) {
	drv, _ := openDB(t, "incremental")
	fillAndDelete(t, drv)
	free := scalar(t, drv, "PRAGMA freelist_count")
	cfg := testConfig()
	cfg.MinFreePages = free + 1
	clock := newFakeClock()
	m := New("t", drv, cfg, WithClock(clock))

	require.NoError(t, m.Tick(context.Background()))

	assert.Equal(t, free, scalar(t, drv, "PRAGMA freelist_count"))
	assert.Zero(t, clock.pauses.Load())
}

func TestTick_ShouldStopMidReclaim_When_ContextCancelled(t *testing.T) {
	drv, _ := openDB(t, "incremental")
	fillAndDelete(t, drv)
	ctx, cancel := context.WithCancel(context.Background())
	clock := newFakeClock()
	clock.onPause = func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}
	m := New("t", drv, testConfig(), WithClock(clock))

	err := m.Tick(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(1), clock.pauses.Load(), "stopped after the first chunk")
	assert.Greater(t, scalar(t, drv, "PRAGMA freelist_count"), int64(0), "remaining pages left for the next tick")
	assert.Greater(t, m.snapshot().ReclaimedPagesTotal, int64(0), "first chunk still counted")
}

func TestTick_ShouldReturnErrTickInProgress_When_Overlapping(t *testing.T) {
	drv, _ := openDB(t, "incremental")
	fillAndDelete(t, drv)
	clock := newFakeClock()
	inPause, release := make(chan struct{}), make(chan struct{})
	clock.onPause = func(context.Context) error {
		inPause <- struct{}{}
		<-release
		return nil
	}
	m := New("t", drv, testConfig(), WithClock(clock))
	done := make(chan error, 1)
	go func() { done <- m.Tick(context.Background()) }()
	<-inPause

	require.ErrorIs(t, m.Tick(context.Background()), ErrTickInProgress)

	clock.onPause = nil
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, m.Tick(context.Background()), "usable again after the first tick ends")
}

func TestRun_ShouldTickOnClockAndExitOnCancel(t *testing.T) {
	drv, _ := openDB(t, "incremental")
	fillAndDelete(t, drv)
	clock := newFakeClock()
	m := New("t", drv, testConfig(), WithClock(clock))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()

	clock.ticks <- time.Now() // received by Run, which then ticks
	clock.ticks <- time.Now() // accepted only after the first tick finished
	assert.Less(t, scalar(t, drv, "PRAGMA freelist_count"), int64(100))
	assert.NotEmpty(t, Snapshot(), "registered while running")

	cancel()
	<-done
	for _, s := range Snapshot() {
		assert.NotEqual(t, "t", s.Name, "unregistered after Run returns")
	}
}

func TestRun_ShouldDoNothing_When_Disabled(t *testing.T) {
	drv, _ := openDB(t, "incremental")
	cfg := testConfig()
	cfg.Disabled = true
	m := New("t", drv, cfg, WithClock(newFakeClock()))
	m.Run(context.Background()) // returns immediately
	assert.Empty(t, Snapshot())
}

func TestConfigFromLookup(t *testing.T) {
	env := map[string]string{
		EnvInterval: "5m", EnvFreeRatio: "0.5", EnvMinFreePages: "7", EnvChunkPages: "9",
		EnvChunkPause: "1s", EnvDisable: "true",
	}
	got := configFromLookup(func(k string) string { return env[k] })
	assert.Equal(t, Config{Interval: 5 * time.Minute, FreeRatio: 0.5, MinFreePages: 7, ChunkPages: 9, ChunkPause: time.Second, Disabled: true}, got)

	def := configFromLookup(func(string) string { return "junk" })
	assert.Equal(t, Config{Interval: DefaultInterval, FreeRatio: DefaultFreeRatio, MinFreePages: DefaultMinFreePages,
		ChunkPages: DefaultChunkPages, ChunkPause: DefaultChunkPause}, def)
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Size()
}
