package dbpool

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/dbsplit"
)

const (
	connMaxLifetime = time.Hour
	closeBudget     = 2 * time.Second
	closeBusy       = 250 * time.Millisecond
)

// Database is one open SQLite file: a single writer connection, an optional
// query_only read pool, and the instrumented ent driver over both. Close it
// through Driver (ent's client.Close) or Close; both are idempotent.
type Database struct {
	spec    Spec
	writer  *sql.DB
	reader  *sql.DB // nil when the profile has no read pool
	drv     dialect.Driver
	started time.Time

	journalMode string // verified at open
	synchronous string // writer connection, read at open

	closeOnce sync.Once
	closeErr  error
}

// Open opens spec's file, enforces WAL and returns the Database. It fails if
// the journal mode is not "wal" after open, instead of trusting the DSN.
func Open(spec Spec) (*Database, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dirOf(spec.Path), 0o750); err != nil {
		return nil, fmt.Errorf("dbpool %s: create dir: %w", spec.UseCase, err)
	}
	writer, err := sql.Open("sqlite", writerDSN(spec))
	if err != nil {
		return nil, fmt.Errorf("dbpool %s: open: %w", spec.UseCase, err)
	}
	// SQLite allows one writer; one connection also keeps ent's dedup/CAS transactions atomic.
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(connMaxLifetime)

	mode, syncMode, err := enforceWAL(context.Background(), writer)
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("dbpool %s: %w", spec.UseCase, err)
	}

	var reader *sql.DB
	if spec.Profile.Readers > 0 {
		reader, err = openReader(spec)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
	}

	d := &Database{spec: spec, writer: writer, reader: reader, started: time.Now(), journalMode: mode, synchronous: syncMode}
	d.drv = d.compose()
	register(d)
	return d, nil
}

func openReader(spec Spec) (*sql.DB, error) {
	db, err := sql.Open("sqlite", readerDSN(spec))
	if err != nil {
		return nil, fmt.Errorf("dbpool %s: open read pool: %w", spec.UseCase, err)
	}
	n := spec.Profile.Readers
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n)
	db.SetConnMaxLifetime(connMaxLifetime)
	return db, nil
}

// enforceWAL makes the file WAL (a persistent, idempotent header change) and reads it back.
func enforceWAL(ctx context.Context, db *sql.DB) (mode, synchronous string, err error) {
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return "", "", fmt.Errorf("set journal_mode=WAL: %w", err)
	}
	if err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return "", "", fmt.Errorf("read journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		return "", "", fmt.Errorf("journal_mode is %q, want wal", mode)
	}
	var syncLevel int
	if err = db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncLevel); err != nil {
		return "", "", fmt.Errorf("read synchronous: %w", err)
	}
	return strings.ToLower(mode), synchronousName(syncLevel), nil
}

func synchronousName(level int) string {
	switch level {
	case 0:
		return "off"
	case 1:
		return "normal"
	case 2:
		return "full"
	case 3:
		return "extra"
	}
	return "unknown"
}

func writerDSN(spec Spec) string {
	b := sqlitedsn.New(spec.Path)
	if spec.autoVacuumIncremental {
		b.WithPragma("auto_vacuum", "incremental")
	}
	b.WithWAL().WithBusyTimeout(spec.Profile.BusyTimeout)
	switch spec.foreignKeys {
	case fkShort:
		b.WithForeignKeysShort()
	case fkLong:
		b.WithForeignKeys()
	}
	switch strings.ToUpper(spec.Profile.Synchronous) {
	case "":
	case "NORMAL":
		b.WithSynchronousNormal()
	default:
		b.WithPragma("synchronous", strings.ToUpper(spec.Profile.Synchronous))
	}
	if spec.Profile.WALAutocheckpoint > 0 {
		b.WithPragma("wal_autocheckpoint", fmt.Sprint(spec.Profile.WALAutocheckpoint))
	}
	if spec.Profile.JournalSizeLimit > 0 {
		b.WithPragma("journal_size_limit", fmt.Sprint(spec.Profile.JournalSizeLimit))
	}
	if spec.entTimeCompat {
		b.WithEntTimeCompat()
	}
	return b.Build()
}

func readerDSN(spec Spec) string {
	b := sqlitedsn.New(spec.Path).WithWAL().WithBusyTimeout(spec.Profile.BusyTimeout).WithQueryOnly()
	if strings.EqualFold(spec.Profile.Synchronous, "NORMAL") {
		b.WithSynchronousNormal()
	}
	if spec.entTimeCompat {
		b.WithEntTimeCompat()
	}
	return b.Build()
}

// compose builds the instrumented, split ent driver over the pools.
func (d *Database) compose() dialect.Driver {
	w := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, d.writer), d.writer, dbobs.ConfigFromEnv(d.spec.PoolName))
	if d.reader == nil {
		return &managed{Driver: w, db: d}
	}
	r := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, d.reader), d.reader, dbobs.ConfigFromEnv(d.spec.PoolName+"-read"))
	return &managed{Driver: dbsplit.New(w, r), db: d}
}

// Driver is the ent driver: reads go to the read pool when there is one.
// Closing it closes the Database.
func (d *Database) Driver() dialect.Driver { return d.drv }

// Writer is the single writer connection's pool, for raw maintenance SQL
// (incremental_vacuum, index cleanup) that must run on the writer.
func (d *Database) Writer() *sql.DB { return d.writer }

// UseCase reports which logical database this is.
func (d *Database) UseCase() UseCase { return d.spec.UseCase }

// Path is the database file.
func (d *Database) Path() string { return d.spec.Path }

// Close checkpoints best-effort and closes the pools. It is idempotent.
func (d *Database) Close() error {
	d.closeOnce.Do(func() {
		unregister(d)
		if d.reader != nil {
			_ = d.reader.Close() // readers first so the checkpoint is not blocked by them
		}
		d.checkpoint()
		d.closeErr = d.drv.(*managed).Driver.Close()
	})
	return d.closeErr
}

// checkpoint runs a bounded TRUNCATE checkpoint (no PRAGMA optimize: it would write
// sqlite_stat1 and change query plans on the two existing files). A failure
// is logged and ignored: the WAL replays on the next open.
func (d *Database) checkpoint() {
	ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
	defer cancel()
	conn, err := d.writer.Conn(ctx)
	if err != nil {
		log.Warn("dbpool: skipped close-time checkpoint, writer connection busy", "db", d.spec.UseCase, "err", err)
		return
	}
	defer func() { _ = conn.Close() }()
	for _, q := range []string{
		fmt.Sprintf("PRAGMA busy_timeout=%d", closeBusy.Milliseconds()),
		"PRAGMA wal_checkpoint(TRUNCATE)",
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			log.Warn("dbpool: close-time pragma failed", "db", d.spec.UseCase, "pragma", q, "err", err)
			return
		}
	}
}

// managed forwards to the composed driver and routes Close through Database.Close.
type managed struct {
	dialect.Driver
	db *Database
}

// BeginTx is required: ent's generated Client.BeginTx type-asserts it on the driver.
func (m *managed) BeginTx(ctx context.Context, opts *sql.TxOptions) (dialect.Tx, error) {
	if o, ok := m.Driver.(interface {
		BeginTx(context.Context, *sql.TxOptions) (dialect.Tx, error)
	}); ok {
		return o.BeginTx(ctx, opts)
	}
	return m.Tx(ctx)
}

func (m *managed) Close() error { return m.db.Close() }

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, os.PathSeparator); i > 0 {
		return path[:i]
	}
	return "."
}
