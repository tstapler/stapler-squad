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
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudemetadata"
	"github.com/tstapler/stapler-squad/session/ent/claudesession"
	"github.com/tstapler/stapler-squad/session/ent/diffstats"
	"github.com/tstapler/stapler-squad/session/ent/project"
	"github.com/tstapler/stapler-squad/session/ent/session"
	entshell "github.com/tstapler/stapler-squad/session/ent/shell"
	"github.com/tstapler/stapler-squad/session/ent/tag"
	"github.com/tstapler/stapler-squad/session/ent/worktree"
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
	r.Instances = instancestore.New(r.Client)
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

// Create inserts a new session into the database
func (r *EntRepository) Create(ctx context.Context, data domain.InstanceData) error {
	// Start transaction
	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create main session
	sessionCreate := tx.Session.Create().
		SetTitle(data.Title).
		SetNillableUUID(nilIfEmpty(data.UUID)).
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetCreatedAt(data.CreatedAt).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetAutoApprove(data.AutoApprove).
		SetAutonomousMode(data.AutonomousMode).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded).
		SetPinned(data.Pinned)

	// Set optional fields
	setSessionCreateIdentityFields(sessionCreate, data)
	setSessionCreateActivityFields(sessionCreate, data)
	setSessionCreateGitHubFields(sessionCreate, data)

	// Link project if specified (look up by name)
	if data.ProjectID != "" {
		proj, err := tx.Project.Query().Where(project.Name(data.ProjectID)).Only(ctx)
		if err == nil {
			sessionCreate.SetProjectID(proj.ID)
		}
		// If project not found, silently continue (session created without project link)
	}

	sess, err := sessionCreate.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if err := createWorktreeRow(ctx, tx, sess.ID, data); err != nil {
		return err
	}
	if err := createDiffStatsRow(ctx, tx, sess.ID, data); err != nil {
		return err
	}
	if err := createTagLinks(ctx, tx, sess, data.Tags); err != nil {
		return err
	}
	if err := createClaudeRows(ctx, tx, sess.ID, data); err != nil {
		return err
	}

	if err := sessionpr.UpsertSessionPRSetTx(ctx, tx, data); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func setSessionCreateIdentityFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if data.WorkingDir != "" {
		sessionCreate.SetWorkingDir(data.WorkingDir)
	}
	if data.Branch != "" {
		sessionCreate.SetBranch(data.Branch)
	}
	if data.Height > 0 {
		sessionCreate.SetHeight(data.Height)
	}
	if data.Width > 0 {
		sessionCreate.SetWidth(data.Width)
	}
	if data.Prompt != "" {
		sessionCreate.SetPrompt(data.Prompt)
	}
	if data.ExistingWorktree != "" {
		sessionCreate.SetExistingWorktree(data.ExistingWorktree)
	}
	if data.Category != "" {
		sessionCreate.SetCategory(data.Category)
	}
	if data.Note != "" {
		sessionCreate.SetNote(data.Note)
	}
	if len(data.RuleTagProvenance) > 0 {
		sessionCreate.SetRuleTagProvenance(data.RuleTagProvenance)
	}
	if len(data.SuppressedRuleTags) > 0 {
		sessionCreate.SetSuppressedRuleTags(suppressedRuleTagsToSlice(data.SuppressedRuleTags))
	}
	if data.SessionType != "" {
		sessionCreate.SetSessionType(string(data.SessionType))
	}
}

func setSessionCreateActivityFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if data.TmuxPrefix != "" {
		sessionCreate.SetTmuxPrefix(data.TmuxPrefix)
	}
	// Backend is write-once per instance today (set at construction via
	// session.ResolveSessionBackend, never reset to "" afterward) -- this guard
	// intentionally only ever sets a non-empty value, mirroring TmuxPrefix above,
	// not clearing an explicit unset like PauseReason/ExitReason do. If a future
	// feature ever needs to unpin an instance's backend back to "", this needs an
	// explicit ClearBackend() branch too, or the clear will silently no-op here.
	if data.Backend != "" {
		sessionCreate.SetBackend(string(data.Backend))
	}
	if data.TymuxSessionID != "" {
		sessionCreate.SetTymuxSessionID(data.TymuxSessionID)
	}
	if !data.LastTerminalUpdate.IsZero() {
		sessionCreate.SetLastTerminalUpdate(data.LastTerminalUpdate)
	}
	if !data.LastMeaningfulOutput.IsZero() {
		sessionCreate.SetLastMeaningfulOutput(data.LastMeaningfulOutput)
	}
	if data.LastOutputSignature != "" {
		sessionCreate.SetLastOutputSignature(data.LastOutputSignature)
	}
	if !data.LastAddedToQueue.IsZero() {
		sessionCreate.SetLastAddedToQueue(data.LastAddedToQueue)
	}
	if !data.LastViewed.IsZero() {
		sessionCreate.SetLastViewed(data.LastViewed)
	}
	if !data.LastAcknowledged.IsZero() {
		sessionCreate.SetLastAcknowledged(data.LastAcknowledged)
	}
	if data.MCPServerURL != "" {
		sessionCreate.SetMcpServerURL(data.MCPServerURL)
	}
	if data.InitialPrompt != "" {
		sessionCreate.SetInitialPrompt(data.InitialPrompt)
	}
}

func setSessionCreateGitHubFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if !data.InitialPromptSentAt.IsZero() {
		sessionCreate.SetInitialPromptSentAt(data.InitialPromptSentAt)
	}
	if data.OneShot {
		sessionCreate.SetOneShot(data.OneShot)
	}
	if data.Hidden {
		sessionCreate.SetHidden(data.Hidden)
	}
	if data.GitHubPRURL != "" {
		sessionCreate.SetGithubPrURL(data.GitHubPRURL)
	}
	if data.GitHubPRNumber > 0 {
		sessionCreate.SetGithubPrNumber(data.GitHubPRNumber)
	}
	if data.GitHubPRStatusTerminal {
		sessionCreate.SetGithubPrStatusTerminal(true)
	}
	if data.GitHubOwner != "" {
		sessionCreate.SetGithubOwner(data.GitHubOwner)
	}
	if data.GitHubRepo != "" {
		sessionCreate.SetGithubRepo(data.GitHubRepo)
	}
	if data.GitHubHost != "" {
		sessionCreate.SetGithubHost(data.GitHubHost)
	}
	if data.ArchivedAt != nil {
		sessionCreate.SetArchivedAt(*data.ArchivedAt)
	}
	if !data.CreationProgressUpdatedAt.IsZero() {
		sessionCreate.SetCreationProgressUpdatedAt(data.CreationProgressUpdatedAt)
	}
}

func createWorktreeRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	if data.Worktree.RepoPath == "" {
		return nil
	}
	if _, err := tx.Worktree.Create().
		SetSessionID(sessionID).
		SetRepoPath(data.Worktree.RepoPath).
		SetWorktreePath(data.Worktree.WorktreePath).
		SetSessionName(data.Worktree.SessionName).
		SetBranchName(data.Worktree.BranchName).
		SetBaseCommitSha(data.Worktree.BaseCommitSHA).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	return nil
}

func createDiffStatsRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	diffCreate := tx.DiffStats.Create().
		SetSessionID(sessionID).
		SetAdded(data.DiffStats.Added).
		SetRemoved(data.DiffStats.Removed)
	if data.DiffStats.Content != "" {
		diffCreate.SetContent(data.DiffStats.Content)
	}
	if _, err := diffCreate.Save(ctx); err != nil {
		return fmt.Errorf("failed to create diff stats: %w", err)
	}
	return nil
}

// createTagLinks collects all tag IDs (creating missing tags) then does one bulk AddTagIDs.
func createTagLinks(ctx context.Context, tx *ent.Tx, sess *ent.Session, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	tagIDs := make([]int, 0, len(tags))
	for _, tagName := range tags {
		t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
		if err != nil {
			if !ent.IsNotFound(err) {
				return fmt.Errorf("failed to query tag %s: %w", tagName, err)
			}
			t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
			if err != nil {
				return fmt.Errorf("failed to create tag %s: %w", tagName, err)
			}
		}
		tagIDs = append(tagIDs, t.ID)
	}
	if err := tx.Session.UpdateOne(sess).AddTagIDs(tagIDs...).Exec(ctx); err != nil {
		return fmt.Errorf("failed to associate tags: %w", err)
	}
	return nil
}

// createClaudeRows creates the Claude session row and its metadata entries, if present.
func createClaudeRows(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	if data.ClaudeSession.ConversationUUID == "" {
		return nil
	}
	claudeSess, err := createClaudeSessionRow(ctx, tx, sessionID, data)
	if err != nil {
		return err
	}
	for key, value := range data.ClaudeSession.Metadata {
		if _, err := tx.ClaudeMetadata.Create().
			SetClaudeSessionID(claudeSess.ID).
			SetKey(key).
			SetValue(value).
			Save(ctx); err != nil {
			return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
		}
	}
	return nil
}

// Update modifies an existing session in the database
func (r *EntRepository) Update(ctx context.Context, data domain.InstanceData) error {
	return r.updateSkipping(ctx, data, domain.UpdateSkip{})
}

// updateSkipping is Update minus the child-table sections the caller knows are
// already persisted unchanged (see savedFingerprints).
func (r *EntRepository) updateSkipping(ctx context.Context, data domain.InstanceData, skip domain.UpdateSkip) error {
	// Start transaction
	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find session by title
	sess, err := tx.Session.Query().Where(session.Title(data.Title)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Update main session fields
	sessionUpdate := tx.Session.UpdateOne(sess).
		SetNillableUUID(nilIfEmpty(data.UUID)).
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetAutoApprove(data.AutoApprove).
		SetAutonomousMode(data.AutonomousMode).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded).
		SetPinned(data.Pinned)

	setOptionalSessionFields(sessionUpdate, data)

	// Update project link (look up by name or clear if empty)
	if data.ProjectID != "" {
		proj, err := tx.Project.Query().Where(project.Name(data.ProjectID)).Only(ctx)
		if err == nil {
			sessionUpdate.SetProjectID(proj.ID)
		}
	} else {
		sessionUpdate.ClearProject()
	}

	if err := sessionUpdate.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if !skip.Worktree {
		if err := r.updateWorktreeRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if !skip.DiffStats {
		if err := r.upsertDiffStatsRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if !skip.Tags {
		if err := r.replaceSessionTags(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if data.ClaudeSession.ConversationUUID != "" && !skip.Claude {
		if err := r.upsertClaudeSessionRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if err := sessionpr.UpsertSessionPRSetTx(ctx, tx, data); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// setOptionalSessionFields applies the session columns Update only sets when
// data carries a value (plus the few that are cleared when empty).
func setOptionalSessionFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	setSessionTextFields(sessionUpdate, data)
	setSessionTimeFields(sessionUpdate, data)
	setSessionLifecycleFields(sessionUpdate, data)
}

func setSessionTextFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	// Update optional fields
	if data.WorkingDir != "" {
		sessionUpdate.SetWorkingDir(data.WorkingDir)
	}
	if data.Branch != "" {
		sessionUpdate.SetBranch(data.Branch)
	}
	if data.Height > 0 {
		sessionUpdate.SetHeight(data.Height)
	}
	if data.Width > 0 {
		sessionUpdate.SetWidth(data.Width)
	}
	if data.Prompt != "" {
		sessionUpdate.SetPrompt(data.Prompt)
	}
	if data.ExistingWorktree != "" {
		sessionUpdate.SetExistingWorktree(data.ExistingWorktree)
	}
	if data.Category != "" {
		sessionUpdate.SetCategory(data.Category)
	}
	// Note is set unconditionally, unlike sibling optional string fields: an empty note is a
	// meaningful, intentionally-reachable state ("cleared"), not an "unset" sentinel, so the
	// guarded-update convention would silently prevent a user from ever clearing it.
	sessionUpdate.SetNote(data.Note)
	// RuleTagProvenance/SuppressedRuleTags are set unconditionally too: retracting the last
	// rule-owned tag or un-suppressing the last tag are meaningful "cleared to empty" states
	// (same rationale as Note above), not an "unset" sentinel a guarded update would preserve.
	sessionUpdate.SetRuleTagProvenance(data.RuleTagProvenance)
	sessionUpdate.SetSuppressedRuleTags(suppressedRuleTagsToSlice(data.SuppressedRuleTags))
	if data.SessionType != "" {
		sessionUpdate.SetSessionType(string(data.SessionType))
	}
	if data.TmuxPrefix != "" {
		sessionUpdate.SetTmuxPrefix(data.TmuxPrefix)
	}
	// See the matching comment in Create above: Backend is write-once per
	// instance today, so this guard is intentionally set-only, never clear.
	if data.Backend != "" {
		sessionUpdate.SetBackend(string(data.Backend))
	}
	// Unlike Backend, this is cleared when empty: a tmux switch or a killed
	// session must not leave a stale id behind. ToInstanceData keeps a loaded,
	// not-yet-restored id (PersistedSessionID), so an unstarted load never clears it.
	sessionUpdate.SetTymuxSessionID(data.TymuxSessionID)
}

func setSessionTimeFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	if !data.LastTerminalUpdate.IsZero() {
		sessionUpdate.SetLastTerminalUpdate(data.LastTerminalUpdate)
	}
	if !data.LastMeaningfulOutput.IsZero() {
		sessionUpdate.SetLastMeaningfulOutput(data.LastMeaningfulOutput)
	}
	if data.LastOutputSignature != "" {
		sessionUpdate.SetLastOutputSignature(data.LastOutputSignature)
	}
	if !data.LastAddedToQueue.IsZero() {
		sessionUpdate.SetLastAddedToQueue(data.LastAddedToQueue)
	}
	if !data.LastViewed.IsZero() {
		sessionUpdate.SetLastViewed(data.LastViewed)
	}
	if !data.LastAcknowledged.IsZero() {
		sessionUpdate.SetLastAcknowledged(data.LastAcknowledged)
	}
	if !data.LastUserResponse.IsZero() {
		sessionUpdate.SetLastUserResponse(data.LastUserResponse)
	}
	if !data.ProcessingGraceUntil.IsZero() {
		sessionUpdate.SetProcessingGraceUntil(data.ProcessingGraceUntil)
	} else {
		sessionUpdate.ClearProcessingGraceUntil()
	}
	if !data.LastPromptDetected.IsZero() {
		sessionUpdate.SetLastPromptDetected(data.LastPromptDetected)
	}
	if data.LastPromptSignature != "" {
		sessionUpdate.SetLastPromptSignature(data.LastPromptSignature)
	}
	if data.MCPServerURL != "" {
		sessionUpdate.SetMcpServerURL(data.MCPServerURL)
	}
	if data.InitialPrompt != "" {
		sessionUpdate.SetInitialPrompt(data.InitialPrompt)
	}
	if !data.InitialPromptSentAt.IsZero() {
		sessionUpdate.SetInitialPromptSentAt(data.InitialPromptSentAt)
	}
}

// setGitHubPRMirrorFields writes the legacy scalar PR columns on a full save.
// A session whose snapshot carries links writes them unconditionally so a detach
// or an open PR can clear the number, URL and terminal flag; a session whose legacy
// number has no link keeps its stored number/URL untouched. A session without links
// keeps the old set-only behavior, so a stale in-memory zero cannot erase data (its
// stale terminal=true is cleared by the immediate title-keyed write, not here).
func setGitHubPRMirrorFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	switch {
	case data.LegacyPRUnrepresented:
		if data.GitHubPRStatusTerminal {
			sessionUpdate.SetGithubPrStatusTerminal(true)
		}
	case data.HasPRLinks:
		sessionUpdate.SetGithubPrURL(data.GitHubPRURL).
			SetGithubPrNumber(data.GitHubPRNumber).
			SetGithubPrStatusTerminal(data.GitHubPRStatusTerminal)
	default:
		if data.GitHubPRURL != "" {
			sessionUpdate.SetGithubPrURL(data.GitHubPRURL)
		}
		if data.GitHubPRNumber > 0 {
			sessionUpdate.SetGithubPrNumber(data.GitHubPRNumber)
		}
		if data.GitHubPRStatusTerminal {
			sessionUpdate.SetGithubPrStatusTerminal(true)
		} else if data.TerminalClearPending {
			sessionUpdate.SetGithubPrStatusTerminal(false)
		}
	}
}

func setSessionLifecycleFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	if data.PauseReason != "" {
		sessionUpdate.SetPauseReason(data.PauseReason)
	} else {
		sessionUpdate.ClearPauseReason()
	}
	if data.ExitReason != "" {
		sessionUpdate.SetExitReason(data.ExitReason)
	} else {
		sessionUpdate.ClearExitReason()
	}
	sessionUpdate.SetOneShot(data.OneShot)
	sessionUpdate.SetHidden(data.Hidden)
	if data.WorkflowID != "" {
		sessionUpdate.SetWorkflowID(data.WorkflowID)
	} else {
		sessionUpdate.ClearWorkflowID()
	}
	if data.ArchivedAt != nil {
		sessionUpdate.SetArchivedAt(*data.ArchivedAt)
	} else {
		sessionUpdate.ClearArchivedAt()
	}
	if !data.CreationProgressUpdatedAt.IsZero() {
		sessionUpdate.SetCreationProgressUpdatedAt(data.CreationProgressUpdatedAt)
	}
	setGitHubPRMirrorFields(sessionUpdate, data)
	if data.GitHubOwner != "" {
		sessionUpdate.SetGithubOwner(data.GitHubOwner)
	}
	if data.GitHubRepo != "" {
		sessionUpdate.SetGithubRepo(data.GitHubRepo)
	}
	if data.GitHubHost != "" {
		sessionUpdate.SetGithubHost(data.GitHubHost)
	}
}

// updateWorktreeRow upserts the session's worktree row.
func (r *EntRepository) updateWorktreeRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Update or create worktree
	if data.Worktree.RepoPath != "" {
		existingWorktree, err := tx.Worktree.Query().Where(worktree.HasSessionWith(session.ID(sess.ID))).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				// Create new worktree
				if _, err := tx.Worktree.Create().
					SetSessionID(sess.ID).
					SetRepoPath(data.Worktree.RepoPath).
					SetWorktreePath(data.Worktree.WorktreePath).
					SetSessionName(data.Worktree.SessionName).
					SetBranchName(data.Worktree.BranchName).
					SetBaseCommitSha(data.Worktree.BaseCommitSHA).
					Save(ctx); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			} else {
				return fmt.Errorf("failed to query worktree: %w", err)
			}
		} else {
			// Update existing worktree
			if err := tx.Worktree.UpdateOne(existingWorktree).
				SetRepoPath(data.Worktree.RepoPath).
				SetWorktreePath(data.Worktree.WorktreePath).
				SetSessionName(data.Worktree.SessionName).
				SetBranchName(data.Worktree.BranchName).
				SetBaseCommitSha(data.Worktree.BaseCommitSHA).
				Exec(ctx); err != nil {
				return fmt.Errorf("failed to update worktree: %w", err)
			}
		}
	}
	return nil
}

// upsertDiffStatsRow upserts the session's diff_stats row.
func (r *EntRepository) upsertDiffStatsRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Upsert diff stats — single INSERT … ON CONFLICT DO UPDATE, no prior SELECT needed.
	diffCreate := tx.DiffStats.Create().
		SetSessionID(sess.ID).
		SetAdded(data.DiffStats.Added).
		SetRemoved(data.DiffStats.Removed)
	if data.DiffStats.Content != "" {
		diffCreate = diffCreate.SetContent(data.DiffStats.Content)
	}
	upsertDiff := diffCreate.
		OnConflictColumns(diffstats.SessionColumn).
		UpdateAdded().
		UpdateRemoved()
	if data.DiffStats.Content != "" {
		upsertDiff = upsertDiff.UpdateContent()
	}
	if err := upsertDiff.Exec(ctx); err != nil {
		return fmt.Errorf("failed to upsert diff stats: %w", err)
	}
	return nil
}

// replaceSessionTags replaces the session's tag associations with data.Tags.
func (r *EntRepository) replaceSessionTags(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Update tags - clear existing and add new ones
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	if len(data.Tags) > 0 {
		tagIDs := make([]int, 0, len(data.Tags))
		for _, tagName := range data.Tags {
			t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
					if err != nil {
						return fmt.Errorf("failed to create tag %s: %w", tagName, err)
					}
				} else {
					return fmt.Errorf("failed to query tag %s: %w", tagName, err)
				}
			}
			tagIDs = append(tagIDs, t.ID)
		}
		if err := tx.Session.UpdateOne(sess).AddTagIDs(tagIDs...).Exec(ctx); err != nil {
			return fmt.Errorf("failed to associate tags: %w", err)
		}
	}
	return nil
}

// upsertClaudeSessionRow creates or updates the Claude session row and its metadata.
func (r *EntRepository) upsertClaudeSessionRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	if data.ClaudeSession.ConversationUUID == "" {
		return nil
	}
	var claudeSessionID int
	existingClaude, err := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Only(ctx)
	switch {
	case err == nil:
		if err := updateClaudeSessionRow(ctx, tx, existingClaude, data); err != nil {
			return err
		}
		claudeSessionID = existingClaude.ID
	case ent.IsNotFound(err):
		created, err := createClaudeSessionRow(ctx, tx, sess.ID, data)
		if err != nil {
			return err
		}
		claudeSessionID = created.ID
	default:
		return fmt.Errorf("failed to query claude session: %w", err)
	}

	// Add metadata entries using the already-known Claude session ID.
	// (pre-fix: this loop re-queried the same ClaudeSession row N times)
	for key, value := range data.ClaudeSession.Metadata {
		if _, err := tx.ClaudeMetadata.Create().
			SetClaudeSessionID(claudeSessionID).
			SetKey(key).
			SetValue(value).
			Save(ctx); err != nil {
			return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
		}
	}
	return nil
}

func createClaudeSessionRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) (*ent.ClaudeSession, error) {
	claudeCreate := tx.ClaudeSession.Create().
		SetSessionID(sessionID).
		SetClaudeSessionID(data.ClaudeSession.ConversationUUID).
		SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
		SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
		SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
		SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)
	if data.ClaudeSession.SquadSessionID != "" {
		claudeCreate.SetConversationID(data.ClaudeSession.SquadSessionID)
	}
	if data.ClaudeSession.ProjectName != "" {
		claudeCreate.SetProjectName(data.ClaudeSession.ProjectName)
	}
	if !data.ClaudeSession.LastAttached.IsZero() {
		claudeCreate.SetLastAttached(data.ClaudeSession.LastAttached)
	}
	if data.ClaudeSession.Settings.PreferredSessionName != "" {
		claudeCreate.SetPreferredSessionName(data.ClaudeSession.Settings.PreferredSessionName)
	}
	created, err := claudeCreate.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create claude session: %w", err)
	}
	return created, nil
}

// updateClaudeSessionRow updates the existing row and clears its metadata so
// the caller can re-add the current entries.
func updateClaudeSessionRow(ctx context.Context, tx *ent.Tx, existing *ent.ClaudeSession, data domain.InstanceData) error {
	claudeUpdate := tx.ClaudeSession.UpdateOne(existing).
		SetClaudeSessionID(data.ClaudeSession.ConversationUUID).
		SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
		SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
		SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
		SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)
	if data.ClaudeSession.SquadSessionID != "" {
		claudeUpdate.SetConversationID(data.ClaudeSession.SquadSessionID)
	}
	if data.ClaudeSession.ProjectName != "" {
		claudeUpdate.SetProjectName(data.ClaudeSession.ProjectName)
	}
	if !data.ClaudeSession.LastAttached.IsZero() {
		claudeUpdate.SetLastAttached(data.ClaudeSession.LastAttached)
	}
	if data.ClaudeSession.Settings.PreferredSessionName != "" {
		claudeUpdate.SetPreferredSessionName(data.ClaudeSession.Settings.PreferredSessionName)
	}
	if err := claudeUpdate.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update claude session: %w", err)
	}
	if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.ID(existing.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear claude metadata: %w", err)
	}
	return nil
}

// Delete removes a session from the database by title
func (r *EntRepository) Delete(ctx context.Context, title string) error {
	// Start transaction for atomic deletion
	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find session by title
	sess, err := tx.Session.Query().Where(session.Title(title)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Delete related entities first (manual cascade)

	// Delete worktree if exists
	if _, err := tx.Worktree.Delete().Where(worktree.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete worktree: %w", err)
	}

	// Delete diff stats if exists
	if _, err := tx.DiffStats.Delete().Where(diffstats.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete diff stats: %w", err)
	}

	// Stamp the conversation UUID onto the session's item_sessions before the claude_sessions
	// row is deleted, so Insights can still attribute the transcript afterward.
	if err := stampItemSessionConversationUUID(ctx, tx, sess); err != nil {
		return err
	}

	// Delete claude session and its metadata if exists
	// Delete all claude metadata associated with claude sessions for this session
	if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.HasSessionWith(session.ID(sess.ID)))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete claude metadata: %w", err)
	}
	// Delete all claude sessions for this session
	if _, err := tx.ClaudeSession.Delete().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete claude sessions: %w", err)
	}

	// Clear tag associations (many-to-many)
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	// Delete shells (sibling tmux sessions) — Shell.session edge is Required(),
	// so leaving any rows here trips a FOREIGN KEY constraint on session delete.
	if _, err := tx.Shell.Delete().Where(entshell.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete shells: %w", err)
	}

	// PR links and detach tombstones are keyed by the session uuid, not an FK.
	if sess.UUID != "" {
		if err := sessionpr.DeleteOwnerPRRows(ctx, tx, domain.SessionPROwner(sess.UUID)); err != nil {
			return err
		}
	}

	// Finally delete the session
	if err := tx.Session.DeleteOne(sess).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Close performs cleanup and releases resources
func (r *EntRepository) Close() error {
	if r.Client != nil {
		return r.Client.Close()
	}
	return nil
}

// nilIfEmpty returns nil if s is empty, otherwise a pointer to s.
// Used to pass optional string fields to Ent's SetNillable* builders.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// suppressedRuleTagsToSlice converts Instance.SuppressedRuleTags' in-memory
// map[string]bool representation to the []string set persisted in the
// suppressed_rule_tags JSON column (session/ent/schema/session.go) — a simpler
// on-disk shape than a bool-valued JSON object, per Task 3.1.2a.
func suppressedRuleTagsToSlice(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for tag, suppressed := range m {
		if suppressed {
			out = append(out, tag)
		}
	}
	return out
}
