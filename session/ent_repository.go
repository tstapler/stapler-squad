package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/analyticsstore"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/guidancestore"
	"github.com/tstapler/stapler-squad/session/instancestore"
	"github.com/tstapler/stapler-squad/session/markerstore"
	"github.com/tstapler/stapler-squad/session/projectstore"
	"github.com/tstapler/stapler-squad/session/rulestore"
	"github.com/tstapler/stapler-squad/session/sessionpr"
	"github.com/tstapler/stapler-squad/session/shellstore"
	"github.com/tstapler/stapler-squad/session/wakestore"
	"github.com/tstapler/stapler-squad/session/webhookstore"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// EntRepository implements the Repository interface using Ent ORM as the storage backend.
// It provides type-safe database operations with automatic schema migrations.
type EntRepository struct {
	// Store (webhooks), Repo (guidance requests) and Triggers (wake triggers), Markers (flow markers), PRStore (pull requests) and Backlog (item sources) methods are
	// promoted; see initStores.
	*webhookstore.Store
	*guidancestore.Repo
	*wakestore.Triggers
	*markerstore.Markers
	*sessionpr.PRStore
	*backlogstore.Backlog
	*shellstore.Shells
	*projectstore.Projects
	*rulestore.Rules
	*analyticsstore.Analytics
	*instancestore.Instances

	Client *ent.Client
	driver dialect.Driver // instrumented writer/reader driver behind client; nil for NewEntRepositoryFromClient
	DbPath string

	migrationMode bool // When true, enables dual-write mode for migration

	// seedURI, when set (tests only, via withSeedURI), names a fully migrated
	// shared-cache in-memory database that NewEntRepository copies into its own
	// database instead of running schema creation and startup migrations — see
	// NewTestEntRepository.
	seedURI string

	// migrations overrides DefaultStartupMigrations (nil = use the default); set via WithMigrations.
	migrations []Migration
}

// NewEntRepository creates a new Ent repository with the given options.
// The database will be initialized with the schema if it doesn't exist.
func NewEntRepository(opts ...RepositoryOption) (*EntRepository, error) {
	repo := &EntRepository{
		DbPath:        "~/.stapler-squad/sessions.db", // Default path
		migrationMode: false,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(repo); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Expand home directory in path if needed
	expandedPath := repo.DbPath
	if len(expandedPath) >= 2 && expandedPath[:2] == "~/" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		expandedPath = filepath.Join(homeDir, expandedPath[2:])
	}

	// Persist the resolved path back onto repo.dbPath (nothing above this
	// point reads the field again — every remaining use in this function
	// already switched to the local expandedPath) so later code, notably
	// BackfillBacklogItemPublicIDs's flock lock-file derivation, sees the
	// real absolute path rather than an unexpanded "~/..." one.
	repo.DbPath = expandedPath

	// Create parent directory if it doesn't exist, unless expandedPath is a
	// "file:" URI DSN (e.g. a shared-cache in-memory database used by tests)
	// rather than a real filesystem path.
	if !strings.HasPrefix(expandedPath, "file:") {
		if err := os.MkdirAll(filepath.Dir(expandedPath), 0750); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// Open database connection. WAL mode is appended for on-disk databases for
	// better concurrency; it's skipped for URI-style DSNs (e.g. shared-cache
	// in-memory databases), which don't support WAL and already carry their
	// own query string that a second "?" would corrupt.
	//
	// _texttotime is required because ent's generated UPDATE...RETURNING
	// statements produce result columns with an empty SQLite decltype (unlike
	// plain SELECTs, which report DATETIME and auto-convert without this
	// flag) — modernc.org/sqlite only upgrades those empty-decltype TEXT
	// values to time.Time when this DSN param is set, otherwise Scan fails
	// with "unsupported Scan...storing driver.Value type string into type
	// *time.Time".
	//
	// _time_format=sqlite is required alongside it: without it, the driver
	// writes time.Time values using Go's time.Time.String() (e.g. "2006-01-02
	// 15:04:05.999999999 -0700 MST"), which its own read-side parser
	// (parseTimeFormats in modernc.org/sqlite) never matches — the offset is
	// space-separated with a zone abbreviation, not the colon-separated
	// "-07:00" the parser expects. That mismatch makes every read fall back
	// to the raw string, hitting the same Scan error above even when
	// _texttotime successfully triggers a parse attempt. _time_format=sqlite
	// makes the driver write with parseTimeFormats[0], the exact layout its
	// own reader tries first.
	//
	// _timezone=UTC is required on top of both: without it, a parsed time.Time
	// carries a distinct, driver-synthesized zero-offset *time.Location (empty
	// name, its own internal zone/tx tables) rather than the time.UTC package
	// singleton. The instant is correct either way, but assert.Equal(t,
	// time.UTC, x.Location()) — used throughout this package's tests, e.g.
	// TestBacklogItem_UpdatedAt_should_BeStoredInUTC_When_CreatedOrTransitioned
	// — does a deep struct comparison and fails on that Location identity
	// mismatch. Setting _timezone=UTC makes every read run through the
	// driver's applyTimezone(t) -> t.In(time.UTC), which sets the Location to
	// the exact singleton (time.LoadLocation("UTC") — what _timezone resolves
	// through — special-cases "UTC" to return that singleton directly).
	// auto_vacuum=INCREMENTAL only takes effect on a database with no tables yet;
	// session/dbmaint reclaims the freelist of such databases in the background.
	// Plain SELECTs run on a separate query_only pool: WAL lets them proceed
	// while the writer connection is busy. On-disk databases open through
	// session/dbpool (ADR-036), which builds the same DSN, enforces WAL and owns
	// the pools. Seeded and URI (in-memory) databases stay single-pool and are
	// opened directly because dbpool accepts only plain file paths.
	var (
		db   *sql.DB
		core *dbpool.CoreDB
	)
	if repo.seedURI == "" && isOnDiskDatabase(expandedPath) {
		opened, err := dbpool.OpenCore(expandedPath, readPoolSize())
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		core, db = &opened, opened.Writer()
	} else {
		dbPath := tuneEntDSN(sqlitedsn.New(expandedPath)).
			WithPragma("auto_vacuum", "incremental").
			WithWAL().
			WithBusyTimeout(5000 * time.Millisecond).
			WithForeignKeysShort().
			WithEntTimeCompat().
			Build()
		var err error
		db, err = sql.Open("sqlite", dbPath) //nolint:norawdb in-memory/URI/seeded test databases are not files dbpool can own
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		// SQLite supports only one writer at a time; serialise all writes (and
		// every transaction) through a single connection to eliminate "database is
		// locked" contention and keep the Go-side dedup/CAS transactions atomic.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(time.Hour)
	}

	if repo.seedURI != "" {
		return newSeededEntRepository(db, repo)
	}

	// Must be read BEFORE client.Schema.Create() below, which is what adds the
	// enabled column this signal depends on being absent — see
	// workflow_enabled_field_migration.go's doc comment for why this exact
	// signal (not a value-based heuristic) is required.
	workflowEnabledColumnAlreadyExisted := workflowEnabledColumnPreexisted(db)

	// Create Ent client with the existing database connection
	var (
		client *ent.Client
		drv    dialect.Driver
	)
	if core != nil {
		drv = core.Driver()
		client = ent.NewClient(ent.Driver(drv))
	} else {
		client, drv = newInstrumentedEntClient(db, nil)
	}

	// Run automatic schema migration. entSchemaCreateMu serializes this call
	// process-wide: even though each caller opens its own isolated SQLite
	// connection, entgo.io/ent/dialect/sql/schema.(*Atlas) has internal
	// package-level state that races under `go test -parallel` when many
	// NewEntRepository calls run schema creation concurrently.
	stopSlowWatch := watchSlowOp("ent schema create (incl. wait for EntSchemaCreateMu)", entSchemaCreateSlowAfter)
	dbpool.EntSchemaCreateMu.Lock()
	err := client.Schema.Create(context.Background())
	dbpool.EntSchemaCreateMu.Unlock()
	stopSlowWatch()
	if err != nil {
		_ = client.Close() // best-effort cleanup; we're already returning the real startup error
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	repo.Client = client
	repo.driver = drv
	repo.initStores()
	repo.WatchGuidanceWrites()

	// One-time-per-database correction for rows that predate the enabled field
	// — see workflow_enabled_field_migration.go's doc comment for why this
	// must be gated on workflowEnabledColumnAlreadyExisted rather than run
	// unconditionally like the startup migrations below. Doesn't implement
	// Migration for the same reason (see ent_repository_migrations.go): the
	// preexisted check has to be taken before Schema.Create() runs, above.
	if !workflowEnabledColumnAlreadyExisted {
		runWorkflowEnabledFieldBackfill(context.Background(), repo)
	}

	// Every other startup data migration is uniform-shaped (idempotent,
	// (ctx, *EntRepository) error) and lives in DefaultStartupMigrations — see
	// session/ent_repository_migrations.go for why the enabled-field backfill
	// above this point is the documented exception that doesn't.
	migrations := repo.migrations
	if migrations == nil {
		migrations = DefaultStartupMigrations()
	}
	for _, m := range migrations {
		if err := m.Run(context.Background(), repo); err != nil {
			_ = client.Close() // best-effort cleanup; we're already returning the real startup error
			return nil, fmt.Errorf("failed to run migration %q: %w", m.Name(), err)
		}
	}

	return repo, nil
}

// NewEntRepositoryFromClient wraps a pre-existing *ent.Client in an EntRepository.
// The caller is responsible for running schema migration on the client beforehand.
// Use this when you need to share an already-opened client across subsystems
// (e.g. injecting a test client or reusing an existing connection).
func NewEntRepositoryFromClient(client *ent.Client) *EntRepository {
	// Not hooked: the client is shared and may already be serving writes, and
	// appending to its hook slice then would race them. Reads stay uncached.
	repo := &EntRepository{Client: client}
	repo.initStores()
	return repo
}

// initStores builds the embedded leaf stores over r.Client; every constructor that
// sets Client must call it.
func (r *EntRepository) initStores() {
	r.Store = webhookstore.New(r.Client)
	r.Triggers = wakestore.New(r.Client)
	r.Markers = markerstore.New(r.Client)
	r.PRStore = sessionpr.New(r.Client)
	r.Shells = shellstore.New(r.Client)
	r.Projects = projectstore.New(r.Client)
	r.Rules = rulestore.New(r.Client)
	r.Analytics = analyticsstore.New(r.Client)
	r.Instances = instancestore.New(r.Client, instancestore.Ports{UpsertSessionPRSet: sessionpr.UpsertSessionPRSetTx, DeleteOwnerPRRows: sessionpr.DeleteOwnerPRRows})
	r.SetDriver(r.driver)
	// Built before Backlog, which takes it as its guidance port; the callback resolves r.Backlog
	// at call time, so it publishes through the Backlog built on the next line.
	r.Repo = guidancestore.New(r.Client, func(ctx context.Context, row *ent.BacklogItem) {
		item := backlogstore.BacklogItemToData(row)
		r.PublishItemChanged(ctx, &item, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated, UpdatedFields: []string{"has_pending_guidance_requests"}})
	})
	r.Backlog = backlogstore.New(r.Client, backlogstore.Ports{Guidance: r.Repo, DeleteOwnerPRRows: sessionpr.DeleteOwnerPRRows, AfterLinkHook: r.AfterLinkHook, DbPath: func() string { return r.DbPath }})
}

// GetEntClient returns the underlying *ent.Client so callers (e.g. ErrorRegistry)
// can operate on entities not managed by the Repository interface.
func (r *EntRepository) GetEntClient() *ent.Client {
	return r.Client
}

// MaintenanceDriver returns the instrumented driver behind the ent client, for
// session/dbmaint. It is nil when the repository was built from a bare client.
func (r *EntRepository) MaintenanceDriver() dialect.ExecQuerier {
	if r.driver == nil {
		return nil
	}
	return r.driver
}

// Close performs cleanup and releases resources
func (r *EntRepository) Close() error {
	if r.Client != nil {
		return r.Client.Close()
	}
	return nil
}
