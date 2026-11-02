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
	DropOutcomeCountMismatch       = "count_mismatch"
	DropOutcomeDryRun              = "dry_run"
	DropOutcomeDropped             = "dropped"
	DropOutcomeError               = "error"
	dropOutcomeGuardsPassed        = "guards_passed"
)

// LegacyDropConfig tunes the drop. Zero values take production defaults; tests
// inject Pause and BeforeChunk.
type LegacyDropConfig struct {
	Mode LegacyDropMode
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
	Deleted    int64 // legacy rows deleted by this run
	Dropped    bool
}

// DropLegacyEscapeEvents removes the legacy escape_events table from analytics.db
// once the copy is provably complete. Guards, all required: the migration ledger
// in newPath reads "done", and newPath holds at least as many rows as the legacy
// table. Both counts are read on read-only connections, so neither writer is held.
//
// The legacy rows are deleted in short chunks on the analytics writer (adaptive
// to TargetHold, with a pause between), then the now-empty table is dropped in
// one trivial statement and freed pages are returned in bounded incremental_vacuum
// steps. A crash or cancel leaves a smaller legacy table and the guards still
// hold on the next run, which continues from there. Rows are only ever removed
// from the legacy copy; db/escape_events.db is never written.
func DropLegacyEscapeEvents(ctx context.Context, analyticsDrv dialect.ExecQuerier, analyticsPath, newPath string, cfg LegacyDropConfig) (LegacyDropResult, error) {
	cfg = cfg.withDefaults()
	res := LegacyDropResult{Outcome: DropOutcomeDisabled}
	if cfg.Mode == LegacyDropOff {
		return res, nil
	}
	res, err := checkLegacyDropGuards(ctx, analyticsPath, newPath)
	if err != nil {
		return res, err
	}
	switch res.Outcome {
	case DropOutcomeNoLegacyTable:
		recordDropOutcome(ctx, res)
		return res, nil
	case DropOutcomeMigrationIncomplete:
		log.Warn("escape legacy drop refused: migration ledger is not done; table left in place", "legacy_rows", res.LegacyRows)
		recordDropOutcome(ctx, res)
		return res, nil
	case DropOutcomeCountMismatch:
		log.Warn("escape legacy drop refused: new database has fewer rows than the legacy table; table left in place",
			"legacy_rows", res.LegacyRows, "new_rows", res.NewRows)
		recordDropOutcome(ctx, res)
		return res, nil
	}
	log.Info("escape legacy drop guards passed", "legacy_rows", res.LegacyRows, "new_rows", res.NewRows, "dry_run", cfg.Mode == LegacyDropDryRun)
	if cfg.Mode == LegacyDropDryRun {
		log.Info("escape legacy drop dry run: would delete legacy rows in chunks, DROP TABLE escape_events, then incremental_vacuum; set "+LegacyDropEnv+"=on to run",
			"legacy_rows", res.LegacyRows)
		res.Outcome = DropOutcomeDryRun
		recordDropOutcome(ctx, res)
		return res, nil
	}

	deleted, err := deleteLegacyChunks(ctx, analyticsDrv, cfg)
	res.Deleted = deleted
	if err != nil {
		res.Outcome = DropOutcomeError
		recordDropOutcome(ctx, res)
		return res, err
	}
	if err := analyticsDrv.Exec(ctx, "DROP TABLE IF EXISTS escape_events", []any{}, new(sql.Result)); err != nil {
		res.Outcome = DropOutcomeError
		recordDropOutcome(ctx, res)
		return res, fmt.Errorf("escape legacy drop: drop table: %w", err)
	}
	res.Dropped, res.Outcome = true, DropOutcomeDropped
	log.Info("escape legacy table dropped", "deleted_rows", deleted, "legacy_rows", res.LegacyRows, "new_rows", res.NewRows)
	recordDropOutcome(ctx, res)
	reclaimLegacySpace(ctx, analyticsDrv, analyticsPath, cfg)
	return res, nil
}

func checkLegacyDropGuards(ctx context.Context, analyticsPath, newPath string) (LegacyDropResult, error) {
	res := LegacyDropResult{}
	if _, err := os.Stat(analyticsPath); errors.Is(err, os.ErrNotExist) {
		res.Outcome = DropOutcomeNoLegacyTable
		return res, nil
	}
	legacy, err := dbpool.OpenForeignReadOnly(analyticsPath, 5*time.Second)
	if err != nil {
		return res, err
	}
	defer func() { _ = legacy.Close() }()
	var hasTable int
	if err := legacy.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'escape_events'").Scan(&hasTable); err != nil {
		return res, fmt.Errorf("escape legacy drop: inspect analytics db: %w", err)
	}
	if hasTable == 0 {
		res.Outcome = DropOutcomeNoLegacyTable
		return res, nil
	}
	if err := legacy.QueryRowContext(ctx, "SELECT COUNT(*) FROM escape_events").Scan(&res.LegacyRows); err != nil {
		return res, fmt.Errorf("escape legacy drop: count legacy rows: %w", err)
	}

	if _, err := os.Stat(newPath); errors.Is(err, os.ErrNotExist) {
		res.Outcome = DropOutcomeMigrationIncomplete
		return res, nil
	}
	dst, err := dbpool.OpenForeignReadOnly(newPath, 5*time.Second)
	if err != nil {
		return res, err
	}
	defer func() { _ = dst.Close() }()
	var phase string
	err = dst.QueryRowContext(ctx, "SELECT phase FROM escape_events_migration WHERE id = 1").Scan(&phase)
	if err != nil || phase != phaseDone {
		// A missing ledger table or row means the copy never started.
		res.Outcome = DropOutcomeMigrationIncomplete
		return res, nil
	}
	if err := dst.QueryRowContext(ctx, "SELECT COUNT(*) FROM escape_events").Scan(&res.NewRows); err != nil {
		return res, fmt.Errorf("escape legacy drop: count new rows: %w", err)
	}
	if res.NewRows < res.LegacyRows {
		res.Outcome = DropOutcomeCountMismatch
		return res, nil
	}
	res.Outcome = dropOutcomeGuardsPassed
	return res, nil
}

// deleteLegacyChunks empties the legacy table with rowid-bounded DELETEs, one
// statement per chunk so the writer is released between them.
func deleteLegacyChunks(ctx context.Context, drv dialect.ExecQuerier, cfg LegacyDropConfig) (int64, error) {
	var total int64
	chunk := cfg.InitialChunk
	for {
		if cfg.BeforeChunk != nil {
			if err := cfg.BeforeChunk(); err != nil {
				return total, err
			}
		}
		start := cfg.Now()
		var r sql.Result
		if err := drv.Exec(ctx, "DELETE FROM escape_events WHERE rowid IN (SELECT rowid FROM escape_events LIMIT ?)", []any{chunk}, &r); err != nil {
			return total, fmt.Errorf("escape legacy drop: delete chunk: %w", err)
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return total, nil
		}
		total += n
		elapsed := cfg.Now().Sub(start)
		switch {
		case elapsed > cfg.TargetHold:
			chunk = max(chunk/2, minBatchRows)
		case elapsed < cfg.TargetHold/2:
			chunk = min(chunk*2, maxBatchRows)
		}
		if err := cfg.Pause(ctx, max(3*elapsed, 20*time.Millisecond)); err != nil {
			return total, err
		}
	}
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
