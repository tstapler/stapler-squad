package dbobs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	_ "modernc.org/sqlite"
)

const secretBind = "s3cr3t-token-value"

type warnRecorder struct {
	mu   sync.Mutex
	logs []string
}

func (w *warnRecorder) warn(msg string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logs = append(w.logs, msg+" "+fmt.Sprint(args...))
}

func (w *warnRecorder) all() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.logs, "\n")
}

func newDriver(t *testing.T, cfg Config) (dialect.Driver, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+cfg.Name+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	drv := Wrap(entsql.OpenDB(dialect.SQLite, db), db, cfg)
	t.Cleanup(func() { _ = drv.Close() })
	_, err = db.Exec("CREATE TABLE t (v TEXT)")
	require.NoError(t, err)
	return drv, db
}

func insertSecret(t *testing.T, drv dialect.ExecQuerier, ctx context.Context) {
	t.Helper()
	var res sql.Result
	require.NoError(t, drv.Exec(ctx, "INSERT INTO t (v) VALUES (?)", []any{secretBind}, &res))
}

func TestSlowQueryLoggedWithoutBindValues(t *testing.T) {
	rec := &warnRecorder{}
	drv, _ := newDriver(t, Config{Name: "slowtest", SlowThreshold: time.Nanosecond, Warn: rec.warn})

	insertSecret(t, drv, context.Background())

	out := rec.all()
	assert.Contains(t, out, "slow database query")
	assert.Contains(t, out, "INSERT INTO t (v) VALUES (?)")
	assert.NotContains(t, out, secretBind)
}

func TestFastQueryNotLoggedAndDisabledThreshold(t *testing.T) {
	rec := &warnRecorder{}
	drv, _ := newDriver(t, Config{Name: "fasttest", SlowThreshold: time.Hour, Warn: rec.warn})
	insertSecret(t, drv, context.Background())
	assert.Empty(t, rec.all())

	off, _ := newDriver(t, Config{Name: "offtest", SlowThreshold: 0, Warn: rec.warn})
	insertSecret(t, off, context.Background())
	assert.Empty(t, rec.all())
}

func TestThresholdFromEnv(t *testing.T) {
	assert.Equal(t, DefaultSlowThreshold, thresholdFromEnv(""))
	assert.Equal(t, DefaultSlowThreshold, thresholdFromEnv("abc"))
	assert.Equal(t, DefaultSlowThreshold, thresholdFromEnv("-5"))
	assert.Equal(t, 250*time.Millisecond, thresholdFromEnv("250"))
	assert.Equal(t, time.Duration(0), thresholdFromEnv("0"))
}

func TestStatsExposure(t *testing.T) {
	drv, _ := newDriver(t, Config{Name: "statstest", SlowThreshold: time.Hour, Warn: func(string, ...any) {}})
	insertSecret(t, drv, context.Background())

	var got *Stats
	for _, s := range Snapshot() {
		if s.Name == "statstest" {
			got = &s
		}
	}
	require.NotNil(t, got, "driver must be registered")
	assert.Equal(t, 1, got.MaxOpen)
	assert.GreaterOrEqual(t, got.Queries, int64(1))
	assert.GreaterOrEqual(t, got.OpenConnections, 1)

	require.NoError(t, drv.Close())
	for _, s := range Snapshot() {
		assert.NotEqual(t, "statstest", s.Name, "Close must unregister")
	}
}

func TestPoolWaitIsObserved(t *testing.T) {
	drv, db := newDriver(t, Config{Name: "waittest", SlowThreshold: time.Hour, Warn: func(string, ...any) {}})
	tx, err := drv.Tx(context.Background())
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		var res sql.Result
		done <- drv.Exec(context.Background(), "INSERT INTO t (v) VALUES (?)", []any{"x"}, &res)
	}()
	require.Eventually(t, func() bool { return db.Stats().WaitCount >= 1 }, 5*time.Second, time.Millisecond)
	require.NoError(t, tx.Commit())
	require.NoError(t, <-done)

	for _, s := range Snapshot() {
		if s.Name == "waittest" {
			assert.GreaterOrEqual(t, s.WaitCount, int64(1))
			assert.GreaterOrEqual(t, s.WaitDurationMs, 0.0)
			return
		}
	}
	t.Fatal("waittest pool missing from Snapshot")
}

func TestSpanIsChildOfParentAndLeaksNoBindValues(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	drv, _ := newDriver(t, Config{Name: "spantest", Tracer: tp.Tracer("test"), Warn: func(string, ...any) {}})

	// Without a parent span nothing is recorded.
	insertSecret(t, drv, context.Background())
	assert.Empty(t, sr.Ended())

	ctx, parent := tp.Tracer("test").Start(context.Background(), "rpc")
	insertSecret(t, drv, ctx)
	parent.End()

	var child sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.Name() == "db.INSERT" {
			child = s
		}
	}
	require.NotNil(t, child)
	assert.Equal(t, parent.SpanContext().SpanID(), child.Parent().SpanID())
	for _, kv := range child.Attributes() {
		assert.NotContains(t, kv.Value.String(), secretBind)
	}
}

func TestTransactionQueriesAreObserved(t *testing.T) {
	rec := &warnRecorder{}
	drv, _ := newDriver(t, Config{Name: "txtest", SlowThreshold: time.Nanosecond, Warn: rec.warn})
	tx, err := drv.Tx(context.Background())
	require.NoError(t, err)
	insertSecret(t, tx, context.Background())
	require.NoError(t, tx.Commit())
	assert.Contains(t, rec.all(), "INSERT INTO t")
	assert.NotContains(t, rec.all(), secretBind)
}

func TestNormalizeStatement(t *testing.T) {
	cases := map[string]string{
		"SELECT  *\n FROM t WHERE a = 'bob''s' AND b = 42": "SELECT * FROM t WHERE a = ? AND b = ?",
		"SELECT `t1`.`id` FROM t1 WHERE x IN (?, ?)":       "SELECT `t1`.`id` FROM t1 WHERE x IN (?, ?)",
		"UPDATE t SET n = 3.14 WHERE id = 7":               "UPDATE t SET n = ? WHERE id = ?",
		"SELECT col2 FROM tab_3 WHERE x = 'unterminated":   "SELECT col2 FROM tab_3 WHERE x = ?",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeStatement(in), in)
	}
	long := NormalizeStatement("SELECT " + strings.Repeat("a, ", 400) + "b FROM t")
	assert.LessOrEqual(t, len(long), maxStatementLen+3)
}
