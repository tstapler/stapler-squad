package escapedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbpool"
)

// The copy moves rows of the legacy analytics.db escape_events table into the new
// database. escape_events.id is a random hex string, so the cursor is a keyset on
// id (the primary-key index), not a rowid, which VACUUM may renumber. Each batch
// is one IMMEDIATE transaction on the destination's only writer connection that
// inserts the rows AND advances the ledger cursor, so a crash leaves cursor and
// rows consistent and the copy resumes from the cursor. INSERT OR IGNORE makes a
// re-copied row harmless. The source table is never modified or dropped here.

const (
	phaseCopying = "copying"
	phaseDone    = "done"
	phaseFailed  = "failed"

	// maxVerifyPasses bounds full re-scans when the source count disagrees with
	// the scanned count (source rows landed below the cursor).
	maxVerifyPasses = 3

	minBatchRows = 50
	maxBatchRows = 2000

	escapeColumns = "id, session_id, project_path, stage, sequence_type, sequence_subtype, sequence_signature, " +
		"byte_length, payload_hash, raw_bytes, mangled, mangle_type, wall_time, session_seq"

	ledgerDDL = `CREATE TABLE IF NOT EXISTS escape_events_migration (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		phase TEXT NOT NULL,
		cursor TEXT NOT NULL,
		scanned INTEGER NOT NULL,
		inserted INTEGER NOT NULL,
		pass INTEGER NOT NULL,
		updated_at TEXT NOT NULL)`
)

// MigrationConfig tunes the copy. Zero values take production defaults; tests
// inject Now, Pause and BeforeCommit.
type MigrationConfig struct {
	// TargetHold is the writer hold per batch the batch size adapts to (default 40 ms, under the 50 ms budget).
	TargetHold time.Duration
	// InitialBatch is the first batch size in rows (default 500).
	InitialBatch int
	// Now is the clock used to time batches.
	Now func() time.Time
	// Pause waits between batches so the writer is idle most of the time; it
	// returns early with ctx.Err() on cancellation.
	Pause func(ctx context.Context, d time.Duration) error
	// BeforeCommit, if set, runs inside each batch transaction before COMMIT;
	// a non-nil error rolls the batch back (a simulated crash mid-copy).
	BeforeCommit func() error
}

// MigrationStatus is the ledger row.
type MigrationStatus struct {
	Phase    string
	Cursor   string
	Scanned  int64
	Inserted int64
	Pass     int
}

// Done reports whether the copy verified and finished.
func (s MigrationStatus) Done() bool { return s.Phase == phaseDone }

func (c MigrationConfig) withDefaults() MigrationConfig {
	if c.TargetHold <= 0 {
		c.TargetHold = 40 * time.Millisecond
	}
	if c.InitialBatch <= 0 {
		c.InitialBatch = 500
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Pause == nil {
		c.Pause = sleepCtx
	}
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// MigrateFromAnalyticsDB copies escape_events from the legacy analytics.db at
// srcPath into dst (the escape-events writer pool) and marks the ledger done
// only after the source row count equals the number of rows scanned. It returns
// when finished, cancelled or failed; calling it again resumes.
func MigrateFromAnalyticsDB(ctx context.Context, dst *sql.DB, srcPath string, cfg MigrationConfig) (MigrationStatus, error) {
	m := &migrator{db: dst, srcPath: srcPath, cfg: cfg.withDefaults()}
	return m.run(ctx)
}

type migrator struct {
	db      *sql.DB
	srcPath string
	cfg     MigrationConfig
}

func (m *migrator) run(ctx context.Context) (MigrationStatus, error) {
	st, err := m.loadLedger(ctx)
	if err != nil || st.Phase != phaseCopying {
		return st, err
	}
	if _, err := os.Stat(m.srcPath); errors.Is(err, os.ErrNotExist) {
		return m.finish(ctx, st, "no source database")
	}
	srcHas, err := m.sourceHasTable(ctx)
	if err != nil {
		return st, err
	}
	if !srcHas {
		return m.finish(ctx, st, "no source table")
	}

	batch := m.cfg.InitialBatch
	for {
		start := m.cfg.Now()
		next, n, err := m.copyBatch(ctx, st.Cursor, batch)
		if err != nil {
			return st, err
		}
		elapsed := m.cfg.Now().Sub(start)
		if n == 0 {
			return m.verify(ctx, st)
		}
		st = next
		batch = m.adaptBatch(batch, elapsed)
		if err := m.cfg.Pause(ctx, max(3*elapsed, 20*time.Millisecond)); err != nil {
			return st, err
		}
	}
}

// adaptBatch halves the batch when it held the writer past the target and doubles
// it when well under, so the hold stays near TargetHold on any disk.
func (m *migrator) adaptBatch(batch int, elapsed time.Duration) int {
	switch {
	case elapsed > m.cfg.TargetHold:
		return max(batch/2, minBatchRows)
	case elapsed < m.cfg.TargetHold/2:
		return min(batch*2, maxBatchRows)
	}
	return batch
}

// copyBatch copies up to limit rows above cursor in one transaction and returns the
// new ledger state and the number of rows scanned (0 when the source is exhausted).
func (m *migrator) copyBatch(ctx context.Context, cursor string, limit int) (MigrationStatus, int, error) {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: writer conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// ATTACH is per physical connection; a read-only URI so the source can never be written.
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS src", "file:"+m.srcPath+"?mode=ro"); err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: attach source: %w", err)
	}
	defer m.detach(conn)

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	var maxID string
	var n int
	if err := conn.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), ''), COUNT(*) FROM (SELECT id FROM src.escape_events WHERE id > ? ORDER BY id LIMIT ?)",
		cursor, limit).Scan(&maxID, &n); err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: scan batch bounds: %w", err)
	}
	if n == 0 {
		return MigrationStatus{}, 0, nil
	}
	res, err := conn.ExecContext(ctx, "INSERT OR IGNORE INTO main.escape_events ("+escapeColumns+") SELECT "+escapeColumns+
		" FROM src.escape_events WHERE id > ? AND id <= ?", cursor, maxID)
	if err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: insert batch: %w", err)
	}
	inserted, _ := res.RowsAffected()
	if _, err := conn.ExecContext(ctx,
		"UPDATE escape_events_migration SET cursor = ?, scanned = scanned + ?, inserted = inserted + ?, updated_at = ? WHERE id = 1",
		maxID, n, inserted, m.cfg.Now().UTC().Format(time.RFC3339)); err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: advance ledger: %w", err)
	}
	if m.cfg.BeforeCommit != nil {
		if err := m.cfg.BeforeCommit(); err != nil {
			return MigrationStatus{}, 0, err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return MigrationStatus{}, 0, fmt.Errorf("escape migration: commit: %w", err)
	}
	committed = true
	st, err := m.readLedger(ctx, conn)
	return st, n, err
}

// detach drops the attachment so the pooled connection is clean; a failed
// DETACH discards the connection instead of returning it attached.
func (m *migrator) detach(conn *sql.Conn) {
	ctx := context.WithoutCancel(context.Background())
	if _, err := conn.ExecContext(ctx, "DETACH DATABASE src"); err != nil {
		log.Warn("escape migration: detach failed, discarding connection", "err", err)
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
}

// verify compares the source row count with the rows scanned. Equal marks the
// ledger done; a difference (source rows landed below the cursor, e.g. an old
// binary wrote to the legacy table) restarts the scan, up to maxVerifyPasses.
func (m *migrator) verify(ctx context.Context, st MigrationStatus) (MigrationStatus, error) {
	src, err := dbpool.OpenForeignReadOnly(m.srcPath, 5*time.Second)
	if err != nil {
		return st, err
	}
	defer func() { _ = src.Close() }()
	var srcRows int64
	if err := src.QueryRowContext(ctx, "SELECT COUNT(*) FROM escape_events").Scan(&srcRows); err != nil {
		return st, fmt.Errorf("escape migration: count source: %w", err)
	}
	if srcRows == st.Scanned {
		return m.finish(ctx, st, fmt.Sprintf("verified %d source rows", srcRows))
	}
	log.Warn("escape migration: source count differs from scanned rows", "source", srcRows, "scanned", st.Scanned, "pass", st.Pass)
	scanned := st.Scanned
	next := st.Pass + 1
	phase := phaseCopying
	if next >= maxVerifyPasses {
		phase = phaseFailed
	}
	if _, err := m.db.ExecContext(ctx,
		"UPDATE escape_events_migration SET phase = ?, cursor = '', scanned = 0, pass = ?, updated_at = ? WHERE id = 1",
		phase, next, m.cfg.Now().UTC().Format(time.RFC3339)); err != nil {
		return st, err
	}
	st.Phase, st.Cursor, st.Scanned, st.Pass = phase, "", 0, next
	if phase == phaseFailed {
		return st, fmt.Errorf("escape migration: source has %d rows but %d scanned after %d passes; legacy table left in place", srcRows, scanned, next)
	}
	return m.run(ctx)
}

func (m *migrator) finish(ctx context.Context, st MigrationStatus, why string) (MigrationStatus, error) {
	if _, err := m.db.ExecContext(ctx, "UPDATE escape_events_migration SET phase = ?, updated_at = ? WHERE id = 1",
		phaseDone, m.cfg.Now().UTC().Format(time.RFC3339)); err != nil {
		return st, fmt.Errorf("escape migration: mark done: %w", err)
	}
	st.Phase = phaseDone
	log.Info("escape migration: complete", "why", why, "scanned", st.Scanned, "inserted", st.Inserted)
	return st, nil
}

func (m *migrator) sourceHasTable(ctx context.Context) (bool, error) {
	src, err := dbpool.OpenForeignReadOnly(m.srcPath, 5*time.Second)
	if err != nil {
		return false, err
	}
	defer func() { _ = src.Close() }()
	var n int
	if err := src.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'escape_events'").Scan(&n); err != nil {
		return false, fmt.Errorf("escape migration: inspect source: %w", err)
	}
	return n > 0, nil
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (m *migrator) loadLedger(ctx context.Context) (MigrationStatus, error) {
	if _, err := m.db.ExecContext(ctx, ledgerDDL); err != nil {
		return MigrationStatus{}, fmt.Errorf("escape migration: create ledger: %w", err)
	}
	if _, err := m.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO escape_events_migration (id, phase, cursor, scanned, inserted, pass, updated_at) VALUES (1, ?, '', 0, 0, 0, ?)",
		phaseCopying, m.cfg.Now().UTC().Format(time.RFC3339)); err != nil {
		return MigrationStatus{}, fmt.Errorf("escape migration: seed ledger: %w", err)
	}
	return m.readLedger(ctx, m.db)
}

func (m *migrator) readLedger(ctx context.Context, q queryRower) (MigrationStatus, error) {
	var st MigrationStatus
	err := q.QueryRowContext(ctx, "SELECT phase, cursor, scanned, inserted, pass FROM escape_events_migration WHERE id = 1").
		Scan(&st.Phase, &st.Cursor, &st.Scanned, &st.Inserted, &st.Pass)
	if err != nil {
		return st, fmt.Errorf("escape migration: read ledger: %w", err)
	}
	return st, nil
}
