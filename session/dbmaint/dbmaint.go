// Package dbmaint reclaims free pages from SQLite databases in the background.
//
// SQLite never shrinks a database file when rows are deleted; the pages go on a
// freelist. With auto_vacuum=INCREMENTAL the freelist can be returned to the OS
// in bounded chunks via PRAGMA incremental_vacuum(N), which holds the write lock
// only for the chunk. A Maintainer does that on a ticker, pausing between chunks
// so the single writer is never starved.
//
// A database still at auto_vacuum=NONE cannot be reclaimed online (only a full
// VACUUM can convert it). The Maintainer never runs that blocking VACUUM; it
// logs the offline remedy once and reports the mode in Snapshot.
package dbmaint

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/log"
)

// AutoVacuum is SQLite's PRAGMA auto_vacuum value.
type AutoVacuum int

const (
	AutoVacuumNone        AutoVacuum = 0
	AutoVacuumFull        AutoVacuum = 1
	AutoVacuumIncremental AutoVacuum = 2
)

func (a AutoVacuum) String() string {
	switch a {
	case AutoVacuumNone:
		return "none"
	case AutoVacuumFull:
		return "full"
	case AutoVacuumIncremental:
		return "incremental"
	default:
		return fmt.Sprintf("unknown(%d)", int(a))
	}
}

// Config tunes one Maintainer. Zero fields take the defaults in WithDefaults.
type Config struct {
	Interval     time.Duration // time between ticks
	FreeRatio    float64       // reclaim when free/total pages exceeds this (0..1)
	MinFreePages int64         // ...and free pages exceed this
	ChunkPages   int64         // pages per incremental_vacuum call
	ChunkPause   time.Duration // writer-friendly pause between chunks
	Disabled     bool
}

// Defaults.
const (
	DefaultInterval     = 15 * time.Minute
	DefaultFreeRatio    = 0.20
	DefaultMinFreePages = 5000
	DefaultChunkPages   = 2000
	DefaultChunkPause   = 250 * time.Millisecond
)

// WithDefaults fills zero or invalid fields with the package defaults.
func (c Config) WithDefaults() Config {
	if c.Interval <= 0 {
		c.Interval = DefaultInterval
	}
	if c.FreeRatio <= 0 || c.FreeRatio >= 1 {
		c.FreeRatio = DefaultFreeRatio
	}
	if c.MinFreePages <= 0 {
		c.MinFreePages = DefaultMinFreePages
	}
	if c.ChunkPages <= 0 {
		c.ChunkPages = DefaultChunkPages
	}
	if c.ChunkPause <= 0 {
		c.ChunkPause = DefaultChunkPause
	}
	return c
}

// Clock is the time seam: tests drive ticks and pauses without real waiting.
type Clock interface {
	// NewTicker returns a tick channel and its stop function.
	NewTicker(d time.Duration) (<-chan time.Time, func())
	// Pause blocks for d or until ctx is done, returning ctx.Err() in that case.
	Pause(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) NewTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

func (realClock) Pause(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Option customises a Maintainer.
type Option func(*Maintainer)

// WithClock replaces the real clock (tests).
func WithClock(c Clock) Option { return func(m *Maintainer) { m.clock = c } }

// WithWarn replaces the WARN logger (tests).
func WithWarn(f func(msg string, args ...any)) Option { return func(m *Maintainer) { m.warn = f } }

// Maintainer reclaims free pages of one database. Create with New.
type Maintainer struct {
	name  string
	q     dialect.ExecQuerier
	cfg   Config
	clock Clock
	warn  func(msg string, args ...any)

	running    atomic.Bool // a Tick is in progress; guards against overlap
	warnedNone sync.Once

	mu    sync.Mutex
	state Stats
}

// New returns a Maintainer for the database behind q. q should be the
// dbobs-instrumented driver so maintenance statements appear in slow-query logs
// and metrics, and it must route PRAGMA statements to the writer connection
// (session/dbsplit does).
func New(name string, q dialect.ExecQuerier, cfg Config, opts ...Option) *Maintainer {
	m := &Maintainer{name: name, q: q, cfg: cfg.WithDefaults(), clock: realClock{}, warn: log.Warn}
	m.state.Name = name
	for _, o := range opts {
		o(m)
	}
	return m
}

// Run starts the maintenance loop and registers m for Snapshot until ctx is
// done. It inspects once immediately (cheap; surfaces the auto_vacuum warning
// and gauges at boot) and then ticks every Interval. It blocks, so call it on a
// tracked goroutine.
func (m *Maintainer) Run(ctx context.Context) {
	if m.cfg.Disabled {
		return
	}
	register(m)
	defer unregister(m)

	m.inspectOnly(ctx)
	ticks, stop := m.clock.NewTicker(m.cfg.Interval)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			_ = m.Tick(ctx)
		}
	}
}

// ErrTickInProgress is returned by Tick when a previous tick has not finished.
var ErrTickInProgress = errors.New("dbmaint: tick already in progress")

// Tick runs one maintenance cycle: sample page counts, and when free pages
// exceed the threshold, reclaim them in chunks. Never runs concurrently with
// itself.
func (m *Maintainer) Tick(ctx context.Context) error {
	if !m.running.CompareAndSwap(false, true) {
		return ErrTickInProgress
	}
	defer m.running.Store(false)

	err := m.tick(ctx)
	m.mu.Lock()
	m.state.Runs++
	m.state.LastRun = time.Now()
	if err != nil && ctx.Err() == nil { // a shutdown cancel is not a maintenance failure
		m.state.LastError = err.Error()
		m.state.Errors++
	} else {
		m.state.LastError = ""
	}
	m.mu.Unlock()
	if err != nil && ctx.Err() == nil {
		m.warn("db maintenance failed", "db", m.name, "err", err)
	}
	return err
}

func (m *Maintainer) inspectOnly(ctx context.Context) {
	if _, err := m.sample(ctx); err != nil && ctx.Err() == nil {
		m.warn("db maintenance inspect failed", "db", m.name, "err", err)
	}
}

type sampleResult struct {
	mode        AutoVacuum
	pages, free int64
}

// sample reads the page counts, publishes them to Snapshot and emits the
// one-time offline-remedy WARN for auto_vacuum=NONE.
func (m *Maintainer) sample(ctx context.Context) (sampleResult, error) {
	var s sampleResult
	mode, err := m.pragma(ctx, "auto_vacuum")
	if err != nil {
		return s, err
	}
	if s.pages, err = m.pragma(ctx, "page_count"); err != nil {
		return s, err
	}
	if s.free, err = m.pragma(ctx, "freelist_count"); err != nil {
		return s, err
	}
	pageSize, err := m.pragma(ctx, "page_size")
	if err != nil {
		return s, err
	}
	s.mode = AutoVacuum(mode)

	m.mu.Lock()
	m.state.AutoVacuumMode = s.mode.String()
	m.state.AutoVacuumModeValue = int(s.mode)
	m.state.PageCount, m.state.FreePages, m.state.PageSize = s.pages, s.free, pageSize
	m.state.NeedsOfflineVacuum = s.mode == AutoVacuumNone
	m.mu.Unlock()

	if s.mode == AutoVacuumNone {
		m.warnedNone.Do(func() {
			m.warn("db auto_vacuum is NONE; free pages cannot be reclaimed online. "+
				"Stop the service and run: sqlite3 <db> 'PRAGMA auto_vacuum=INCREMENTAL; VACUUM;' "+
				"(see docs/how-to/reclaim-sqlite-free-space.md)",
				"db", m.name, "free_pages", s.free, "page_count", s.pages)
		})
	}
	return s, nil
}

func (m *Maintainer) tick(ctx context.Context) error {
	s, err := m.sample(ctx)
	if err != nil {
		return err
	}
	if s.mode != AutoVacuumIncremental || !m.overThreshold(s) {
		return nil
	}
	reclaimed, err := m.reclaim(ctx, s.free)
	m.mu.Lock()
	m.state.LastReclaimedPages = reclaimed
	m.state.ReclaimedPagesTotal += reclaimed
	m.mu.Unlock()
	if reclaimed > 0 {
		log.Info("db maintenance reclaimed free pages", "db", m.name, "pages", reclaimed, "was_free", s.free, "page_count", s.pages)
	}
	if err != nil {
		return err
	}
	// Freed pages sit in the WAL until a checkpoint copies them back and truncates
	// the main file. PASSIVE never blocks readers or the writer.
	return m.checkpointPassive(ctx)
}

func (m *Maintainer) overThreshold(s sampleResult) bool {
	return s.free > m.cfg.MinFreePages && float64(s.free) > m.cfg.FreeRatio*float64(s.pages)
}

// reclaim vacuums chunk by chunk until the freelist is empty, a chunk makes no
// progress, or ctx is done. Each chunk is its own short write transaction, and
// the pause lets queued writers in between chunks.
func (m *Maintainer) reclaim(ctx context.Context, free int64) (int64, error) {
	var reclaimed int64
	for free > 0 {
		if err := ctx.Err(); err != nil {
			return reclaimed, err
		}
		if err := m.q.Exec(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", m.cfg.ChunkPages), []any{}, nil); err != nil {
			return reclaimed, fmt.Errorf("incremental_vacuum: %w", err)
		}
		left, err := m.pragma(ctx, "freelist_count")
		if err != nil {
			return reclaimed, err
		}
		if left >= free {
			return reclaimed, nil // no progress; avoid spinning
		}
		reclaimed += free - left
		free = left
		m.mu.Lock()
		m.state.FreePages = left
		m.mu.Unlock()
		if free > 0 {
			if err := m.clock.Pause(ctx, m.cfg.ChunkPause); err != nil {
				return reclaimed, err
			}
		}
	}
	return reclaimed, nil
}

func (m *Maintainer) checkpointPassive(ctx context.Context) error {
	var rows entsql.Rows
	if err := m.q.Query(ctx, "PRAGMA wal_checkpoint(PASSIVE)", []any{}, &rows); err != nil {
		return fmt.Errorf("wal_checkpoint: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("wal_checkpoint: %w", err)
	}
	return nil
}

// pragma reads a single-integer PRAGMA.
func (m *Maintainer) pragma(ctx context.Context, name string) (int64, error) {
	var rows entsql.Rows
	if err := m.q.Query(ctx, "PRAGMA "+name, []any{}, &rows); err != nil {
		return 0, fmt.Errorf("pragma %s: %w", name, err)
	}
	defer rows.Close()
	var v int64
	if !rows.Next() {
		return 0, fmt.Errorf("pragma %s: no row", name)
	}
	if err := rows.Scan(&v); err != nil {
		return 0, fmt.Errorf("pragma %s: %w", name, err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("pragma %s: %w", name, err)
	}
	return v, nil
}

// Stats is one database's maintenance snapshot, served from /actuator/db.
type Stats struct {
	Name                string    `json:"name"`
	AutoVacuumMode      string    `json:"auto_vacuum_mode"`
	AutoVacuumModeValue int       `json:"auto_vacuum_mode_value"`
	NeedsOfflineVacuum  bool      `json:"needs_offline_vacuum"`
	PageCount           int64     `json:"page_count"`
	FreePages           int64     `json:"free_pages"`
	PageSize            int64     `json:"page_size"`
	ReclaimedPagesTotal int64     `json:"reclaimed_pages_total"`
	LastReclaimedPages  int64     `json:"last_reclaimed_pages"`
	Runs                int64     `json:"runs_total"`
	Errors              int64     `json:"errors_total"`
	LastRun             time.Time `json:"last_run"`
	LastError           string    `json:"last_error,omitempty"`
}

func (m *Maintainer) snapshot() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}
