package dbobs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

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

func TestStatementsAttributedToCaller(t *testing.T) {
	drv, _ := newDriver(t, Config{Name: "calltest", SlowThreshold: time.Hour, Warn: func(string, ...any) {}})
	for i := 0; i < 3; i++ {
		insertSecret(t, drv, context.Background())
	}

	var top []StatementStat
	for _, s := range Snapshot() {
		if s.Name == "calltest" {
			top = s.TopStatements
		}
	}
	require.NotEmpty(t, top)
	assert.Equal(t, int64(3), top[0].Count)
	assert.Contains(t, top[0].Caller, "TestStatementsAttributedToCaller")
}

func TestSlowQueryLogCarriesCaller(t *testing.T) {
	rec := &warnRecorder{}
	drv, _ := newDriver(t, Config{Name: "slowcaller", SlowThreshold: time.Nanosecond, Warn: rec.warn})
	insertSecret(t, drv, context.Background())
	assert.Contains(t, rec.all(), "TestSlowQueryLogCarriesCaller")
}

func TestStatementsWithDifferentListLengthsShareOneKey(t *testing.T) {
	drv, _ := newDriver(t, Config{Name: "listkeytest", SlowThreshold: time.Hour, Warn: func(string, ...any) {}})
	for _, q := range []string{"SELECT v FROM t WHERE v IN (?, ?)", "SELECT v FROM t WHERE v IN (?, ?, ?, ?)"} {
		args := make([]any, strings.Count(q, "?"))
		for i := range args {
			args[i] = "x"
		}
		var rows entsql.Rows
		require.NoError(t, drv.Query(context.Background(), q, args, &rows))
		_ = rows.Close()
	}
	for _, s := range Snapshot() {
		if s.Name == "listkeytest" {
			require.Len(t, s.TopStatements, 1)
			assert.Equal(t, int64(2), s.TopStatements[0].Count)
		}
	}
}

func TestStatementTable_Should_CountDroppedKeys_When_Full(t *testing.T) {
	var tbl statementTable
	for i := 0; i < maxStatementKeys+3; i++ {
		tbl.add(stmtKey{statement: fmt.Sprintf("SELECT %d", i), caller: "c"}, time.Millisecond, 0)
	}
	assert.Equal(t, int64(3), tbl.droppedKeys())
	assert.Len(t, tbl.top(maxStatementKeys+10), maxStatementKeys)
}

func TestNormalizeStatement(t *testing.T) {
	cases := map[string]string{
		"SELECT  *\n FROM t WHERE a = 'bob''s' AND b = 42":   "SELECT * FROM t WHERE a = ? AND b = ?",
		"SELECT `t1`.`id` FROM t1 WHERE x IN (?, ?)":         "SELECT `t1`.`id` FROM t1 WHERE x IN (?, ...?)",
		"SELECT 1 FROM t WHERE x IN (?)":                     "SELECT ? FROM t WHERE x IN (?)",
		"INSERT INTO t (a, b) VALUES (?, ?), (?, ?), (?, ?)": "INSERT INTO t (a, b) VALUES (?, ...?), ...",
		"UPDATE t SET n = 3.14 WHERE id = 7":                 "UPDATE t SET n = ? WHERE id = ?",
		"SELECT col2 FROM tab_3 WHERE x = 'unterminated":     "SELECT col2 FROM tab_3 WHERE x = ?",
		"SELECT 1 FROM t WHERE k = 0xDEADBEEF":               "SELECT ? FROM t WHERE k = ?",
		"SELECT 1 FROM t WHERE k = 1e10 AND j = 2.5E-3":      "SELECT ? FROM t WHERE k = ? AND j = ?",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeStatement(in), in)
	}
	long := NormalizeStatement("SELECT " + strings.Repeat("a, ", 400) + "b FROM t")
	assert.LessOrEqual(t, len(long), maxStatementLen+3)

	multibyte := NormalizeStatement("SELECT " + strings.Repeat("é", 400) + " FROM t")
	assert.True(t, utf8.ValidString(multibyte), "truncation must not split a rune")
}
