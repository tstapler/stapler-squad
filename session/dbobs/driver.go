// Package dbobs instruments ent's dialect.Driver: per-query timing, a
// slow-query WARN, OTel spans/metrics and database/sql pool stats.
//
// The writer pool is capped at one connection (session/ent_repository.go), so
// writes serialize; reads use a separate pool (session/dbsplit). Time spent
// queued behind another query is the contention signal this package exists to expose.
package dbobs

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// SlowQueryEnv overrides the slow-query threshold, in milliseconds. 0 disables the WARN.
const SlowQueryEnv = "STAPLER_SQUAD_SLOW_QUERY_MS"

// DefaultSlowThreshold is used when SlowQueryEnv is unset or invalid.
const DefaultSlowThreshold = 100 * time.Millisecond

// Config tunes one instrumented driver. The zero value is usable but never logs slow queries.
type Config struct {
	Name          string        // label in logs, stats and metrics; default "ent"
	SlowThreshold time.Duration // <=0 disables the WARN
	Warn          func(msg string, args ...any)
	Tracer        trace.Tracer // nil: telemetry.GetTracer()
}

// ConfigFromEnv returns a Config whose threshold honours SlowQueryEnv.
func ConfigFromEnv(name string) Config {
	return Config{Name: name, SlowThreshold: thresholdFromEnv(os.Getenv(SlowQueryEnv))}
}

func thresholdFromEnv(raw string) time.Duration {
	ms, err := strconv.Atoi(raw)
	if raw == "" || err != nil || ms < 0 {
		return DefaultSlowThreshold
	}
	return time.Duration(ms) * time.Millisecond
}

type collector struct {
	cfg   Config
	db    *sql.DB
	stat  counters
	stmts statementTable
}

type counters struct {
	queries, slow, errors atomic.Int64
	busy                  atomic.Int64 // SQLITE_BUSY / SQLITE_LOCKED errors, a subset of errors
	totalNanos, maxNanos  atomic.Int64
}

// isBusyError reports a lock-contention failure: the writer lock or a table
// lock could not be taken within busy_timeout.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "SQLITE_LOCKED") ||
		strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked")
}

// Wrap instruments drv. db must be the pool drv was opened on (used for
// wait-time and pool stats). Closing the returned driver unregisters it.
func Wrap(drv dialect.Driver, db *sql.DB, cfg Config) dialect.Driver {
	if cfg.Name == "" {
		cfg.Name = "ent"
	}
	if cfg.Warn == nil {
		cfg.Warn = log.Warn
	}
	c := &collector{cfg: cfg, db: db}
	register(c)
	return &driver{Driver: drv, c: c}
}

type driver struct {
	dialect.Driver
	c *collector
}

func (d *driver) Exec(ctx context.Context, q string, args, v any) error {
	return d.c.observe(ctx, q, func() error { return d.Driver.Exec(ctx, q, args, v) })
}

func (d *driver) Query(ctx context.Context, q string, args, v any) error {
	return d.c.observe(ctx, q, func() error { return d.Driver.Query(ctx, q, args, v) })
}

func (d *driver) Tx(ctx context.Context) (dialect.Tx, error) {
	return d.BeginTx(ctx, nil)
}

type txOpener interface {
	BeginTx(context.Context, *sql.TxOptions) (dialect.Tx, error)
}

// BeginTx is required: ent's generated Client.BeginTx type-asserts it on the driver.
func (d *driver) BeginTx(ctx context.Context, opts *sql.TxOptions) (dialect.Tx, error) {
	var tx dialect.Tx
	err := d.c.observe(ctx, "BEGIN", func() error {
		var err error
		if o, ok := d.Driver.(txOpener); ok {
			tx, err = o.BeginTx(ctx, opts)
		} else {
			tx, err = d.Driver.Tx(ctx)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &txWrapper{Tx: tx, c: d.c}, nil
}

func (d *driver) Close() error {
	unregister(d.c)
	return d.Driver.Close()
}

type txWrapper struct {
	dialect.Tx
	c *collector
}

// Commit and Rollback are observed too: a deferred write transaction usually
// fails with SQLITE_BUSY at COMMIT, which Exec/Query alone would never see.
func (t *txWrapper) Commit() error {
	return t.c.observe(context.Background(), "COMMIT", t.Tx.Commit)
}

func (t *txWrapper) Rollback() error {
	return t.c.observe(context.Background(), "ROLLBACK", t.Tx.Rollback)
}

func (t *txWrapper) Exec(ctx context.Context, q string, args, v any) error {
	return t.c.observe(ctx, q, func() error { return t.Tx.Exec(ctx, q, args, v) })
}

func (t *txWrapper) Query(ctx context.Context, q string, args, v any) error {
	return t.c.observe(ctx, q, func() error { return t.Tx.Query(ctx, q, args, v) })
}

// observe runs fn and records duration, approximate pool wait, a span and a slow-query WARN.
func (c *collector) observe(ctx context.Context, q string, fn func() error) error {
	end := c.startSpan(ctx, q)
	waitBefore := c.db.Stats().WaitDuration
	start := time.Now()
	err := fn()
	elapsed := time.Since(start)
	// WaitDuration is pool-wide, so under concurrency this over-attributes
	// queue time from other callers to this query; it is a contention hint.
	wait := c.db.Stats().WaitDuration - waitBefore
	key := stmtKey{statement: NormalizeStatement(q), caller: callerOf()}
	c.stmts.add(key, elapsed, wait)
	c.record(key, elapsed, wait, err)
	end(wait, err)
	return err
}

func (c *collector) record(k stmtKey, elapsed, wait time.Duration, err error) {
	s := &c.stat
	s.queries.Add(1)
	s.totalNanos.Add(int64(elapsed))
	storeMax(&s.maxNanos, int64(elapsed))
	if err != nil {
		s.errors.Add(1)
		if isBusyError(err) {
			s.busy.Add(1)
			recordBusy(c.cfg.Name)
		}
	}
	recordHistogram(c.cfg.Name, elapsed)
	recordPoolWait(c.cfg.Name, wait)
	if c.cfg.SlowThreshold > 0 && elapsed >= c.cfg.SlowThreshold {
		s.slow.Add(1)
		c.cfg.Warn("slow database query",
			"db", c.cfg.Name,
			"duration_ms", elapsed.Milliseconds(),
			"pool_wait_ms", wait.Milliseconds(),
			"caller", k.caller,
			"statement", k.statement)
	}
}

func storeMax(a *atomic.Int64, v int64) {
	for {
		cur := a.Load()
		if v <= cur || a.CompareAndSwap(cur, v) {
			return
		}
	}
}

// startSpan opens a child span only under a valid parent (an RPC span); with
// no parent it costs one context lookup and background queries never become
// orphan root spans.
func (c *collector) startSpan(ctx context.Context, q string) func(time.Duration, error) {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return func(time.Duration, error) {}
	}
	tracer := c.cfg.Tracer
	if tracer == nil {
		tracer = telemetry.GetTracer()
	}
	op := operation(q)
	_, span := tracer.Start(ctx, "db."+op, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "sqlite"),
			attribute.String("db.name", c.cfg.Name),
			attribute.String("db.operation", op),
			attribute.String("db.statement", NormalizeStatement(q))))
	return func(wait time.Duration, err error) {
		span.SetAttributes(attribute.Int64("db.pool_wait_ms", wait.Milliseconds()))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "query failed")
		}
		span.End()
	}
}
