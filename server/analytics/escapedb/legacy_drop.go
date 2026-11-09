package escapedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbpool"
)

// LegacyDropEnv selects whether the legacy analytics.db escape_events table is
// removed after the copy to db/escape_events.db: unset/"off" (default) does
// nothing, "dry-run" logs the guard results and what it would do, "on" drops.
const LegacyDropEnv = "STAPLER_SQUAD_ESCAPE_LEGACY_DROP"

// LegacyDropMode is the parsed LegacyDropEnv value.
type LegacyDropMode int

const (
	LegacyDropOff LegacyDropMode = iota
	LegacyDropDryRun
	LegacyDropOn
)

// ParseLegacyDropMode maps an env value to a mode; anything unrecognised is Off,
// because the operation is destructive.
func ParseLegacyDropMode(v string) LegacyDropMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "1", "true", "yes":
		return LegacyDropOn
	case "dry-run", "dryrun", "dry_run":
		return LegacyDropDryRun
	}
	return LegacyDropOff
}

// LegacyDropModeFromEnv reads LegacyDropEnv.
func LegacyDropModeFromEnv() LegacyDropMode { return ParseLegacyDropMode(os.Getenv(LegacyDropEnv)) }

// Outcomes reported by DropLegacyEscapeEvents (also the metric label values).
const (
	DropOutcomeDisabled            = "disabled"
	DropOutcomeNoLegacyTable       = "no_legacy_table"
	DropOutcomeMigrationIncomplete = "migration_incomplete"
	DropOutcomeRefusedUncopied     = "refused_uncopied"
	DropOutcomeEmptyTable          = "empty_table"
	DropOutcomeDryRun              = "dry_run"
	DropOutcomeDropped             = "dropped"
	DropOutcomeError               = "error"
	dropOutcomeGuardsPassed        = "guards_passed"
)

// LegacyDropConfig tunes the drop. Zero values take production defaults; tests
// inject Pause and BeforeChunk.
type LegacyDropConfig struct {
	Mode LegacyDropMode
	// RetentionDays is the escape-event age retention (config EscapeAnalyticsRetentionDays);
	// only legacy rows older than it may be missing from the new database. 0 means none may.
	RetentionDays int
	// TargetHold is the writer hold per delete chunk the chunk size adapts to (default 40 ms).
	TargetHold time.Duration
	// InitialChunk is the first delete chunk in rows (default 500).
	InitialChunk int
	// VacuumPages is the incremental_vacuum step in pages (default 1024).
	VacuumPages int
	// MaxVacuumSteps bounds the vacuum steps per run; the periodic maintenance
	// loop reclaims the rest (default 64).
	MaxVacuumSteps int
	Now            func() time.Time
	// Pause waits between statements so the analytics writer is idle most of the time.
	Pause func(ctx context.Context, d time.Duration) error
	// AfterClassify, if set, runs after a chunk's ids were verified and before they
	// are deleted, with the chunk's row count (tests: size bound, concurrent insert).
	AfterClassify func(rows int)
	// BeforeChunk, if set, runs before each delete chunk; a non-nil error stops the
	// run there (a simulated crash).
	BeforeChunk func() error
}

func (c LegacyDropConfig) withDefaults() LegacyDropConfig {
	if c.TargetHold <= 0 {
		c.TargetHold = 40 * time.Millisecond
	}
	if c.InitialChunk <= 0 {
		c.InitialChunk = 500
	}
	if c.VacuumPages <= 0 {
		c.VacuumPages = 1024
	}
	if c.MaxVacuumSteps <= 0 {
		c.MaxVacuumSteps = 64
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Pause == nil {
		c.Pause = sleepCtx
	}
	return c
}

// LegacyDropResult describes one run.
type LegacyDropResult struct {
	Outcome    string
	LegacyRows int64 // rows in the legacy table when the guards ran
	NewRows    int64 // rows in db/escape_events.db when the guards ran
	Uncopied   int64 // legacy ids absent from the new database and not retention-evictable
	Evictable  int64 // legacy ids absent from the new database but older than the retention cutoff
	Deleted    int64 // legacy rows deleted by this run
	Dropped    bool
}

// DropLegacyEscapeEvents removes the legacy escape_events table from analytics.db
// once every legacy row is known to be safe to lose. What is checked, exactly:
//
//  1. the migration ledger in newPath reads "done" (a missing ledger refuses);
//  2. every legacy row id is present in the new database (same id, not same
//     content), or its wall_time is older than the age-retention cutoff
//     (cfg.RetentionDays, as runEscapeEventRetention computes it), so the new
//     database may legitimately have evicted it. A missing row newer than the
//     cutoff refuses the run (refused_uncopied) with a sample of ids: that is
//     the signature of a legacy write after the ledger turned done, such as an
//     older binary. The per-session cap can also evict younger rows; those are
//     treated as uncopied (conservative) and refuse.
//
// The check runs on read-only connections before any delete and again per chunk,
// and each chunk deletes only the ids it just classified safe, so a legacy
// insert after the check survives. Deletes run on the analytics writer in short
// chunks (adaptive to TargetHold, with a pause between). The empty table is then
// dropped inside a transaction that re-counts it, and freed pages are returned in
// bounded incremental_vacuum steps. A crash or cancel leaves a smaller table and
// the next run continues. db/escape_events.db is never written.
func DropLegacyEscapeEvents(ctx context.Context, analyticsDrv dialect.ExecQuerier, analyticsPath, newPath string, cfg LegacyDropConfig) (LegacyDropResult, error) {
	cfg = cfg.withDefaults()
	if cfg.Mode == LegacyDropOff {
		return LegacyDropResult{Outcome: DropOutcomeDisabled}, nil
	}
	g, err := openDropGuards(ctx, analyticsPath, newPath)
	if err != nil {
		return LegacyDropResult{}, err
	}
	defer g.close()
	res := g.res
	switch res.Outcome {
	case DropOutcomeNoLegacyTable:
		return res, nil
	case DropOutcomeMigrationIncomplete:
		log.Warn("escape legacy drop refused: migration ledger is not done; table left in place", "legacy_rows", res.LegacyRows)
		recordDropOutcome(ctx, res)
		return res, nil
	case DropOutcomeEmptyTable:
		if cfg.Mode == LegacyDropOn {
			if _, err := dropIfEmpty(ctx, analyticsDrv); err != nil {
				return res, err
			}
		}
		return res, nil
	}

	cutoff := cfg.retentionCutoff()
	if err := g.survey(ctx, cutoff, &res); err != nil {
		return res, err
	}
	if res.Uncopied > 0 {
		res.Outcome = DropOutcomeRefusedUncopied
		log.Warn("escape legacy drop refused: legacy rows are missing from the new database and are not old enough to have been evicted; table left in place. To copy them, reset the ledger (see docs/how-to/reclaim-sqlite-free-space.md)",
			"legacy_rows", res.LegacyRows, "new_rows", res.NewRows, "uncopied", res.Uncopied, "evictable", res.Evictable, "sample_ids", g.sample)
		recordDropOutcome(ctx, res)
		return res, nil
	}
	log.Info("escape legacy drop guards passed", "legacy_rows", res.LegacyRows, "new_rows", res.NewRows, "evictable", res.Evictable, "dry_run", cfg.Mode == LegacyDropDryRun)
	if cfg.Mode == LegacyDropDryRun {
		log.Info("escape legacy drop dry run: would delete verified legacy rows in chunks, DROP TABLE escape_events, then incremental_vacuum; set "+LegacyDropEnv+"=on to run",
			"legacy_rows", res.LegacyRows)
		res.Outcome = DropOutcomeDryRun
		recordDropOutcome(ctx, res)
		return res, nil
	}

	deleted, refused, err := g.deleteSafeChunks(ctx, analyticsDrv, cutoff, cfg)
	res.Deleted = deleted
	if err != nil {
		res.Outcome = DropOutcomeError
		recordDropOutcome(ctx, res)
		return res, err
	}
	if refused {
		res.Outcome = DropOutcomeRefusedUncopied
		log.Warn("escape legacy drop stopped: an uncopied legacy row appeared mid-run; it was kept", "deleted", deleted, "sample_ids", g.sample)
		recordDropOutcome(ctx, res)
		return res, nil
	}
	dropped, err := dropIfEmpty(ctx, analyticsDrv)
	if err != nil {
		res.Outcome = DropOutcomeError
		recordDropOutcome(ctx, res)
		return res, err
	}
	if !dropped {
		res.Outcome = DropOutcomeRefusedUncopied
		log.Warn("escape legacy drop kept the table: rows were written to it during the run", "deleted", deleted)
		recordDropOutcome(ctx, res)
		return res, nil
	}
	res.Dropped, res.Outcome = true, DropOutcomeDropped
	log.Info("escape legacy table dropped", "deleted_rows", deleted, "legacy_rows", res.LegacyRows, "new_rows", res.NewRows)
	recordDropOutcome(ctx, res)
	reclaimLegacySpace(ctx, analyticsDrv, analyticsPath, cfg)
	return res, nil
}

// retentionCutoff mirrors runEscapeEventRetention; nil when age retention is off,
// in which case no missing row is ever treated as evicted.
func (c LegacyDropConfig) retentionCutoff() *time.Time {
	if c.RetentionDays <= 0 {
		return nil
	}
	t := c.Now().AddDate(0, 0, -c.RetentionDays)
	return &t
}

// dropGuards holds read-only connections to both databases.
type dropGuards struct {
	legacy, dst *sql.DB
	res         LegacyDropResult
	sample      []string
}

func (g *dropGuards) close() {
	if g.legacy != nil {
		_ = g.legacy.Close()
	}
	if g.dst != nil {
		_ = g.dst.Close()
	}
}

const (
	surveyBatch = 2000
	sampleIDs   = 5
)

// openDropGuards evaluates the cheap guards (table, ledger) and counts both tables.
func openDropGuards(ctx context.Context, analyticsPath, newPath string) (*dropGuards, error) {
	g := &dropGuards{}
	if _, err := os.Stat(analyticsPath); errors.Is(err, os.ErrNotExist) {
		g.res.Outcome = DropOutcomeNoLegacyTable
		return g, nil
	}
	var err error
	if g.legacy, err = dbpool.OpenForeignReadOnly(analyticsPath, 5*time.Second); err != nil {
		return g, err
	}
	var hasTable int
	if err := g.legacy.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'escape_events'").Scan(&hasTable); err != nil {
		return g, fmt.Errorf("escape legacy drop: inspect analytics db: %w", err)
	}
	if hasTable == 0 {
		g.res.Outcome = DropOutcomeNoLegacyTable
		return g, nil
	}
	if err := g.legacy.QueryRowContext(ctx, "SELECT COUNT(*) FROM escape_events").Scan(&g.res.LegacyRows); err != nil {
		return g, fmt.Errorf("escape legacy drop: count legacy rows: %w", err)
	}
	if g.res.LegacyRows == 0 {
		// ent recreates the empty table on every start; nothing to verify or lose.
		g.res.Outcome = DropOutcomeEmptyTable
		return g, nil
	}

	if _, err := os.Stat(newPath); errors.Is(err, os.ErrNotExist) {
		g.res.Outcome = DropOutcomeMigrationIncomplete
		return g, nil
	}
	if g.dst, err = dbpool.OpenForeignReadOnly(newPath, 5*time.Second); err != nil {
		return g, err
	}
	var phase string
	if err := g.dst.QueryRowContext(ctx, "SELECT phase FROM escape_events_migration WHERE id = 1").Scan(&phase); err != nil || phase != phaseDone {
		// A missing ledger table or row means the copy never started.
		g.res.Outcome = DropOutcomeMigrationIncomplete
		return g, nil
	}
	if err := g.dst.QueryRowContext(ctx, "SELECT COUNT(*) FROM escape_events").Scan(&g.res.NewRows); err != nil {
		return g, fmt.Errorf("escape legacy drop: count new rows: %w", err)
	}
	g.res.Outcome = dropOutcomeGuardsPassed
	return g, nil
}

type legacyRow struct {
	id string
	wt sql.NullTime
}

// batchAfter reads up to limit legacy rows with id > after, in id order.
func (g *dropGuards) batchAfter(ctx context.Context, after string, limit int) ([]legacyRow, error) {
	rows, err := g.legacy.QueryContext(ctx, "SELECT id, wall_time FROM escape_events WHERE id > ? ORDER BY id LIMIT ?", after, limit)
	if err != nil {
		return nil, fmt.Errorf("escape legacy drop: read legacy batch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.wt); err != nil {
			return nil, fmt.Errorf("escape legacy drop: scan legacy batch: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// present returns which of the ids exist in the new database.
func (g *dropGuards) present(ctx context.Context, batch []legacyRow) (map[string]bool, error) {
	args := make([]any, len(batch))
	marks := make([]string, len(batch))
	for i, r := range batch {
		args[i], marks[i] = r.id, "?"
	}
	rows, err := g.dst.QueryContext(ctx, "SELECT id FROM escape_events WHERE id IN ("+strings.Join(marks, ",")+")", args...)
	if err != nil {
		return nil, fmt.Errorf("escape legacy drop: look up ids in new database: %w", err)
	}
	defer func() { _ = rows.Close() }()
	got := make(map[string]bool, len(batch))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		got[id] = true
	}
	return got, rows.Err()
}

// classify splits a batch into ids safe to delete (copied, or old enough that the
// new database may have evicted them) and counts the rest as uncopied.
func (g *dropGuards) classify(ctx context.Context, batch []legacyRow, cutoff *time.Time) (safe []string, uncopied, evictable int64, err error) {
	if len(batch) == 0 {
		return nil, 0, 0, nil
	}
	got, err := g.present(ctx, batch)
	if err != nil {
		return nil, 0, 0, err
	}
	for _, r := range batch {
		switch {
		case got[r.id]:
			safe = append(safe, r.id)
		case cutoff != nil && r.wt.Valid && r.wt.Time.Before(*cutoff):
			safe = append(safe, r.id)
			evictable++
		default:
			uncopied++
			if len(g.sample) < sampleIDs {
				g.sample = append(g.sample, r.id)
			}
		}
	}
	return safe, uncopied, evictable, nil
}

// survey classifies the whole legacy table read-only, before any delete.
func (g *dropGuards) survey(ctx context.Context, cutoff *time.Time, res *LegacyDropResult) error {
	after := ""
	for {
		batch, err := g.batchAfter(ctx, after, surveyBatch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		_, unc, ev, err := g.classify(ctx, batch, cutoff)
		if err != nil {
			return err
		}
		res.Uncopied += unc
		res.Evictable += ev
		after = batch[len(batch)-1].id
	}
}

// deleteSafeChunks re-classifies each chunk right before deleting exactly the ids
// it found safe. refused reports an uncopied row seen mid-run (the run stops).
func (g *dropGuards) deleteSafeChunks(ctx context.Context, drv dialect.ExecQuerier, cutoff *time.Time, cfg LegacyDropConfig) (total int64, refused bool, err error) {
	chunk := cfg.InitialChunk
	after := ""
	for {
		if cfg.BeforeChunk != nil {
			if err := cfg.BeforeChunk(); err != nil {
				return total, false, err
			}
		}
		batch, err := g.batchAfter(ctx, after, chunk)
		if err != nil {
			return total, false, err
		}
		if len(batch) == 0 {
			return total, false, nil
		}
		g.sample = g.sample[:0]
		safe, unc, _, err := g.classify(ctx, batch, cutoff)
		if err != nil {
			return total, false, err
		}
		if unc > 0 {
			return total, true, nil
		}
		after = batch[len(batch)-1].id
		if cfg.AfterClassify != nil {
			cfg.AfterClassify(len(safe))
		}
		start := cfg.Now()
		args := make([]any, len(safe))
		marks := make([]string, len(safe))
		for i, id := range safe {
			args[i], marks[i] = id, "?"
		}
		var r sql.Result
		if err := drv.Exec(ctx, "DELETE FROM escape_events WHERE id IN ("+strings.Join(marks, ",")+")", args, &r); err != nil {
			return total, false, fmt.Errorf("escape legacy drop: delete chunk: %w", err)
		}
		n, _ := r.RowsAffected()
		total += n
		elapsed := cfg.Now().Sub(start)
		switch {
		case elapsed > cfg.TargetHold:
			chunk = max(chunk/2, minBatchRows)
		case elapsed < cfg.TargetHold/2:
			chunk = min(chunk*2, maxBatchRows)
		}
		if err := cfg.Pause(ctx, max(3*elapsed, 20*time.Millisecond)); err != nil {
			return total, false, err
		}
	}
}

// dropIfEmpty drops the table in one transaction that first re-counts it, so a
// row written since the last chunk is never dropped with the table. Reports
// false (no error) when the table still has rows or the driver has no transactions.
func dropIfEmpty(ctx context.Context, drv dialect.ExecQuerier) (bool, error) {
	d, ok := drv.(dialect.Driver)
	if !ok {
		return false, nil
	}
	tx, err := d.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("escape legacy drop: begin drop tx: %w", err)
	}
	var rows entsql.Rows
	if err := tx.Query(ctx, "SELECT COUNT(*) FROM escape_events", []any{}, &rows); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("escape legacy drop: recount before drop: %w", err)
	}
	var n int64
	if rows.Next() {
		_ = rows.Scan(&n)
	}
	_ = rows.Close()
	if n > 0 {
		_ = tx.Rollback()
		return false, nil
	}
	if err := tx.Exec(ctx, "DROP TABLE IF EXISTS escape_events", []any{}, new(sql.Result)); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("escape legacy drop: drop table: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("escape legacy drop: commit drop: %w", err)
	}
	return true, nil
}

// reclaimLegacySpace returns freed pages to the filesystem in bounded steps. It
// is best-effort: only databases at auto_vacuum=INCREMENTAL can shrink online;
// others keep the free pages (reused by new rows) until the offline VACUUM in
// docs/how-to/reclaim-sqlite-free-space.md.
func reclaimLegacySpace(ctx context.Context, drv dialect.ExecQuerier, analyticsPath string, cfg LegacyDropConfig) {
	ro, err := dbpool.OpenForeignReadOnly(analyticsPath, 5*time.Second)
	if err != nil {
		log.Warn("escape legacy drop: space check skipped", "err", err)
		return
	}
	defer func() { _ = ro.Close() }()
	var mode int
	if err := ro.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil || mode != 2 {
		log.Warn("escape legacy table dropped but analytics.db is not auto_vacuum=INCREMENTAL; file keeps its size until an offline VACUUM (docs/how-to/reclaim-sqlite-free-space.md)", "auto_vacuum", mode)
		return
	}
	for step := 0; step < cfg.MaxVacuumSteps; step++ {
		var free int64
		if err := ro.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&free); err != nil || free == 0 {
			return
		}
		if err := drv.Exec(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", cfg.VacuumPages), []any{}, new(sql.Result)); err != nil {
			log.Warn("escape legacy drop: incremental vacuum failed; maintenance loop will continue", "err", err)
			return
		}
		if err := cfg.Pause(ctx, 20*time.Millisecond); err != nil {
			return
		}
	}
}

var (
	dropOutcomes metric.Int64Counter
	dropRows     metric.Int64Counter
)

func init() {
	m := otel.Meter("stapler-squad/escapedb")
	var err error
	if dropOutcomes, err = m.Int64Counter("escape_events.legacy_drop.runs", metric.WithDescription("Legacy escape_events drop runs by outcome")); err != nil {
		log.Warn("escapedb: legacy drop metric registration failed", "err", err)
	}
	if dropRows, err = m.Int64Counter("escape_events.legacy_drop.rows_deleted", metric.WithDescription("Legacy escape_events rows deleted from analytics.db")); err != nil {
		log.Warn("escapedb: legacy drop metric registration failed", "err", err)
	}
}

func recordDropOutcome(ctx context.Context, res LegacyDropResult) {
	if dropOutcomes != nil {
		dropOutcomes.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", res.Outcome)))
	}
	if dropRows != nil && res.Deleted > 0 {
		dropRows.Add(ctx, res.Deleted)
	}
}
