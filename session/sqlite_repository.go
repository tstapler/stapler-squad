package session

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteRepository implements the Repository interface using SQLite as the storage backend.
// It provides efficient querying, indexing, and transactional support for session data.
type SQLiteRepository struct {
	db            *sql.DB
	dbPath        string
	migrationMode bool // When true, enables dual-write mode for migration
}

// NewSQLiteRepository creates a new SQLite repository with the given options.
// The database will be initialized with the schema if it doesn't exist.
func NewSQLiteRepository(opts ...RepositoryOption) (*SQLiteRepository, error) {
	repo := &SQLiteRepository{
		dbPath:        "~/.claude-squad/sessions.db", // Default path
		migrationMode: false,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(repo); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Open database connection with WAL mode for better concurrency
	dbPath := repo.dbPath + "?_journal_mode=WAL&_timeout=5000"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(5) // Allow multiple readers
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	repo.db = db

	// Initialize schema
	if err := repo.initializeSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return repo, nil
}

// initializeSchema creates all tables and indexes if they don't exist
func (r *SQLiteRepository) initializeSchema(ctx context.Context) error {
	// Execute all schema initialization statements
	for _, stmt := range InitializeSchema {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	// Check current schema version and run migrations if needed
	var currentVersion int
	err := r.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		// Table might be empty (new database)
		currentVersion = 0
	}

	// Run migrations based on current version
	if currentVersion < 2 {
		// Run v1 to v2 migration (add GitHub integration and worktree detection fields)
		for _, stmt := range MigrateV1ToV2 {
			// Use "IF NOT EXISTS" pattern for idempotency - SQLite will ignore if column exists
			// ALTER TABLE doesn't support IF NOT EXISTS, so we catch the error
			if _, err := r.db.ExecContext(ctx, stmt); err != nil {
				// Ignore "duplicate column name" errors (column already exists)
				if !isDuplicateColumnError(err) {
					return fmt.Errorf("failed to run v1->v2 migration: %w", err)
				}
			}
		}
	}

	if currentVersion < 3 {
		// Run v2 to v3 migration (normalize context data into dedicated tables)
		for _, stmt := range MigrateV2ToV3 {
			// Migration uses "CREATE TABLE IF NOT EXISTS" for idempotency
			if _, err := r.db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("failed to run v2->v3 migration: %w", err)
			}
		}
	}

	// Record schema version (insert if not exists, update if lower)
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO schema_version (version, applied_at)
		VALUES (?, ?)
		ON CONFLICT(version) DO UPDATE SET applied_at = excluded.applied_at
	`, SchemaVersion, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	return nil
}

// isDuplicateColumnError checks if the error is a "duplicate column name" error
func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsSubstr(errStr, "duplicate column name") || containsSubstr(errStr, "SQLITE_ERROR: duplicate column")
}

// containsSubstr checks if s contains substr (simple helper to avoid strings import)
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Create inserts a new session into the database
func (r *SQLiteRepository) Create(ctx context.Context, data InstanceData) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert main session record
	result, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged,
			github_pr_number, github_pr_url, github_owner, github_repo,
			github_source_ref, cloned_repo_path, main_repo_path, is_worktree
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		data.Title, data.Path, data.WorkingDir, data.Branch, data.Status,
		data.Height, data.Width, data.CreatedAt, data.UpdatedAt,
		data.AutoYes, data.Prompt, data.Program, data.ExistingWorktree,
		data.Category, data.IsExpanded, data.SessionType, data.TmuxPrefix,
		data.LastTerminalUpdate, data.LastMeaningfulOutput, data.LastOutputSignature,
		data.LastAddedToQueue, data.LastViewed, data.LastAcknowledged,
		data.GitHubPRNumber, data.GitHubPRURL, data.GitHubOwner, data.GitHubRepo,
		data.GitHubSourceRef, data.ClonedRepoPath, data.MainRepoPath, data.IsWorktree,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	sessionID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get session ID: %w", err)
	}

	// Insert worktree data if present
	if data.Worktree.RepoPath != "" {
		if err := r.insertWorktree(ctx, tx, sessionID, data.Worktree); err != nil {
			return fmt.Errorf("failed to insert worktree: %w", err)
		}
	}

	// Insert diff stats if present - check for any non-zero values since Content may be excluded from JSON serialization
	if data.DiffStats.Content != "" || data.DiffStats.Added > 0 || data.DiffStats.Removed > 0 {
		if err := r.insertDiffStats(ctx, tx, sessionID, data.DiffStats); err != nil {
			return fmt.Errorf("failed to insert diff stats: %w", err)
		}
	}

	// Insert tags
	if len(data.Tags) > 0 {
		if err := r.insertTags(ctx, tx, sessionID, data.Tags); err != nil {
			return fmt.Errorf("failed to insert tags: %w", err)
		}
	}

	// Insert Claude session data if present
	if data.ClaudeSession.SessionID != "" {
		if err := r.insertClaudeSession(ctx, tx, sessionID, data.ClaudeSession); err != nil {
			return fmt.Errorf("failed to insert Claude session: %w", err)
		}
	}

	return tx.Commit()
}

// Update modifies an existing session in the database
func (r *SQLiteRepository) Update(ctx context.Context, data InstanceData) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update main session record
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions SET
			path = ?, working_dir = ?, branch = ?, status = ?, height = ?, width = ?,
			updated_at = ?, auto_yes = ?, prompt = ?, program = ?, existing_worktree = ?,
			category = ?, is_expanded = ?, session_type = ?, tmux_prefix = ?,
			last_terminal_update = ?, last_meaningful_output = ?, last_output_signature = ?,
			last_added_to_queue = ?, last_viewed = ?, last_acknowledged = ?,
			github_pr_number = ?, github_pr_url = ?, github_owner = ?, github_repo = ?,
			github_source_ref = ?, cloned_repo_path = ?, main_repo_path = ?, is_worktree = ?
		WHERE title = ?
	`,
		data.Path, data.WorkingDir, data.Branch, data.Status, data.Height, data.Width,
		time.Now(), data.AutoYes, data.Prompt, data.Program, data.ExistingWorktree,
		data.Category, data.IsExpanded, data.SessionType, data.TmuxPrefix,
		data.LastTerminalUpdate, data.LastMeaningfulOutput, data.LastOutputSignature,
		data.LastAddedToQueue, data.LastViewed, data.LastAcknowledged,
		data.GitHubPRNumber, data.GitHubPRURL, data.GitHubOwner, data.GitHubRepo,
		data.GitHubSourceRef, data.ClonedRepoPath, data.MainRepoPath, data.IsWorktree,
		data.Title,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", data.Title)
	}

	// Get session ID for child table updates
	var sessionID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", data.Title).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ID: %w", err)
	}

	// Update worktree data (delete and re-insert for simplicity)
	if _, err := tx.ExecContext(ctx, "DELETE FROM worktrees WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("failed to delete old worktree: %w", err)
	}
	if data.Worktree.RepoPath != "" {
		if err := r.insertWorktree(ctx, tx, sessionID, data.Worktree); err != nil {
			return fmt.Errorf("failed to insert worktree: %w", err)
		}
	}

	// Update diff stats
	if _, err := tx.ExecContext(ctx, "DELETE FROM diff_stats WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("failed to delete old diff stats: %w", err)
	}
	if data.DiffStats.Content != "" || data.DiffStats.Added > 0 || data.DiffStats.Removed > 0 {
		if err := r.insertDiffStats(ctx, tx, sessionID, data.DiffStats); err != nil {
			return fmt.Errorf("failed to insert diff stats: %w", err)
		}
	}

	// Update tags (delete and re-insert)
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_tags WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("failed to delete old tags: %w", err)
	}
	if len(data.Tags) > 0 {
		if err := r.insertTags(ctx, tx, sessionID, data.Tags); err != nil {
			return fmt.Errorf("failed to insert tags: %w", err)
		}
	}

	// Update Claude session
	if _, err := tx.ExecContext(ctx, "DELETE FROM claude_sessions WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("failed to delete old Claude session: %w", err)
	}
	if data.ClaudeSession.SessionID != "" {
		if err := r.insertClaudeSession(ctx, tx, sessionID, data.ClaudeSession); err != nil {
			return fmt.Errorf("failed to insert Claude session: %w", err)
		}
	}

	return tx.Commit()
}

// Delete removes a session from the database by title
func (r *SQLiteRepository) Delete(ctx context.Context, title string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE title = ?", title)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", title)
	}

	return nil
}

// Get retrieves a single session by title
func (r *SQLiteRepository) Get(ctx context.Context, title string) (*InstanceData, error) {
	// Get with full data loading (includes diff content)
	return r.GetWithOptions(ctx, title, LoadFull)
}

// GetSessionWithContexts retrieves a session with selective context loading.
// This is the preferred method for code using the new Session domain model.
// It loads only the core session fields and the contexts specified by opts.
func (r *SQLiteRepository) GetSessionWithContexts(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	// Query core session fields
	var (
		status   Status
		program  string
		autoYes  bool
		prompt   string
		createdAt time.Time
		updatedAt time.Time
		sessionID int64
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT id, status, program, auto_yes, prompt, created_at, updated_at
		FROM sessions WHERE title = ?
	`, title).Scan(&sessionID, &status, &program, &autoYes, &prompt, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", title)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Create Session struct with core fields
	session := &Session{
		ID:        title, // Use title as ID for compatibility
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Status:    status,
		Program:   program,
		AutoYes:   autoYes,
		Prompt:    prompt,
	}

	// Load contexts based on options
	if opts.LoadGit {
		gitCtx, err := r.loadGitContext(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load git context: %w", err)
		}
		session.Git = gitCtx
	}

	if opts.LoadFilesystem {
		fsCtx, err := r.loadFilesystemContext(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load filesystem context: %w", err)
		}
		session.Filesystem = fsCtx
	}

	if opts.LoadTerminal {
		termCtx, err := r.loadTerminalContext(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load terminal context: %w", err)
		}
		session.Terminal = termCtx
	}

	if opts.LoadUI {
		uiPrefs, err := r.loadUIPreferences(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load UI preferences: %w", err)
		}
		session.UI = uiPrefs
	}

	if opts.LoadActivity {
		activity, err := r.loadActivityTracking(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load activity tracking: %w", err)
		}
		session.Activity = activity
	}

	if opts.LoadCloud {
		cloudCtx, err := r.loadCloudContext(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to load cloud context: %w", err)
		}
		session.Cloud = cloudCtx
	}

	return session, nil
}

// List retrieves all sessions from the database with summary data (no diff content)
func (r *SQLiteRepository) List(ctx context.Context) ([]InstanceData, error) {
	// Use summary loading (includes all child data except diff content)
	return r.ListWithOptions(ctx, LoadSummary)
}

// ListByStatus retrieves sessions filtered by status with summary data
func (r *SQLiteRepository) ListByStatus(ctx context.Context, status Status) ([]InstanceData, error) {
	// Use summary loading (no diff content)
	return r.ListByStatusWithOptions(ctx, status, LoadSummary)
}

// ListByTag retrieves sessions that have a specific tag with summary data
func (r *SQLiteRepository) ListByTag(ctx context.Context, tag string) ([]InstanceData, error) {
	// Use summary loading (no diff content)
	return r.ListByTagWithOptions(ctx, tag, LoadSummary)
}

// UpdateTimestamps efficiently updates only timestamp fields for a session
func (r *SQLiteRepository) UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sessions SET
			last_terminal_update = ?,
			last_meaningful_output = ?,
			last_output_signature = ?,
			updated_at = ?
		WHERE title = ?
	`, lastTerminalUpdate, lastMeaningfulOutput, lastOutputSignature, time.Now(), title)
	if err != nil {
		return fmt.Errorf("failed to update timestamps: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", title)
	}

	return nil
}

// Close performs cleanup and releases database resources
func (r *SQLiteRepository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// Helper methods for child table operations

func (r *SQLiteRepository) insertWorktree(ctx context.Context, tx *sql.Tx, sessionID int64, worktree GitWorktreeData) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO worktrees (session_id, repo_path, worktree_path, session_name, branch_name, base_commit_sha)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, worktree.RepoPath, worktree.WorktreePath, worktree.SessionName, worktree.BranchName, worktree.BaseCommitSHA)
	return err
}

func (r *SQLiteRepository) insertDiffStats(ctx context.Context, tx *sql.Tx, sessionID int64, stats DiffStatsData) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO diff_stats (session_id, added, removed, content)
		VALUES (?, ?, ?, ?)
	`, sessionID, stats.Added, stats.Removed, stats.Content)
	return err
}

func (r *SQLiteRepository) insertTags(ctx context.Context, tx *sql.Tx, sessionID int64, tags []string) error {
	for _, tag := range tags {
		// Insert tag (ignore if already exists)
		_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO tags (name) VALUES (?)", tag)
		if err != nil {
			return fmt.Errorf("failed to insert tag: %w", err)
		}

		// Get the tag ID (whether we just inserted it or it already existed)
		var tagID int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM tags WHERE name = ?", tag).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("failed to get tag ID: %w", err)
		}

		// Link tag to session
		_, err = tx.ExecContext(ctx, `
			INSERT INTO session_tags (session_id, tag_id) VALUES (?, ?)
		`, sessionID, tagID)
		if err != nil {
			return fmt.Errorf("failed to link tag to session: %w", err)
		}
	}
	return nil
}

func (r *SQLiteRepository) insertClaudeSession(ctx context.Context, tx *sql.Tx, sessionID int64, claude ClaudeSessionData) error {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO claude_sessions (
			session_id, claude_session_id, conversation_id, project_name, last_attached,
			auto_reattach, preferred_session_name, create_new_on_missing,
			show_session_selector, session_timeout_minutes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, claude.SessionID, claude.ConversationID, claude.ProjectName, claude.LastAttached,
		claude.Settings.AutoReattach, claude.Settings.PreferredSessionName, claude.Settings.CreateNewOnMissing,
		claude.Settings.ShowSessionSelector, claude.Settings.SessionTimeoutMinutes)
	if err != nil {
		return err
	}

	claudeSessionID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	// Insert metadata
	for key, value := range claude.Metadata {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO claude_metadata (claude_session_id, key, value) VALUES (?, ?, ?)
		`, claudeSessionID, key, value)
		if err != nil {
			return fmt.Errorf("failed to insert metadata: %w", err)
		}
	}

	return nil
}

func (r *SQLiteRepository) insertGitContext(ctx context.Context, tx *sql.Tx, sessionID int64, git *GitContext) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO git_context (
			session_id, branch, base_commit_sha, worktree_id,
			pr_number, pr_url, owner, repo, source_ref
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, git.Branch, git.BaseCommitSHA, git.WorktreeID,
		git.PRNumber, git.PRURL, git.Owner, git.Repo, git.SourceRef)
	return err
}

func (r *SQLiteRepository) insertFilesystemContext(ctx context.Context, tx *sql.Tx, sessionID int64, fs *FilesystemContext) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO filesystem_context (
			session_id, project_path, working_dir, is_worktree,
			main_repo_path, cloned_repo_path, existing_worktree, session_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, fs.ProjectPath, fs.WorkingDir, fs.IsWorktree,
		fs.MainRepoPath, fs.ClonedRepoPath, fs.ExistingWorktree, fs.SessionType)
	return err
}

func (r *SQLiteRepository) insertTerminalContext(ctx context.Context, tx *sql.Tx, sessionID int64, term *TerminalContext) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO terminal_context (
			session_id, height, width, tmux_session_name,
			tmux_prefix, tmux_server_socket, terminal_type
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, term.Height, term.Width, term.TmuxSessionName,
		term.TmuxPrefix, term.TmuxServerSocket, term.TerminalType)
	return err
}

func (r *SQLiteRepository) insertUIPreferences(ctx context.Context, tx *sql.Tx, sessionID int64, ui *UIPreferences) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ui_preferences (
			session_id, category, is_expanded, grouping_strategy, sort_order
		) VALUES (?, ?, ?, ?, ?)
	`, sessionID, ui.Category, ui.IsExpanded, ui.GroupingStrategy, ui.SortOrder)
	return err
}

func (r *SQLiteRepository) insertActivityTracking(ctx context.Context, tx *sql.Tx, sessionID int64, activity *ActivityTracking) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO activity_tracking (
			session_id, last_terminal_update, last_meaningful_output,
			last_output_signature, last_added_to_queue, last_viewed, last_acknowledged
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, activity.LastTerminalUpdate, activity.LastMeaningfulOutput,
		activity.LastOutputSignature, activity.LastAddedToQueue, activity.LastViewed, activity.LastAcknowledged)
	return err
}

func (r *SQLiteRepository) insertCloudContext(ctx context.Context, tx *sql.Tx, sessionID int64, cloud *CloudContext) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO cloud_context (
			session_id, provider, region, instance_id,
			api_endpoint, api_key_ref, cloud_session_id, conversation_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, cloud.Provider, cloud.Region, cloud.InstanceID,
		cloud.APIEndpoint, cloud.APIKeyRef, cloud.CloudSessionID, cloud.ConversationID)
	return err
}

func (r *SQLiteRepository) loadWorktree(ctx context.Context, sessionID int64, data *InstanceData) error {
	err := r.db.QueryRowContext(ctx, `
		SELECT repo_path, worktree_path, session_name, branch_name, base_commit_sha
		FROM worktrees WHERE session_id = ?
	`, sessionID).Scan(
		&data.Worktree.RepoPath, &data.Worktree.WorktreePath,
		&data.Worktree.SessionName, &data.Worktree.BranchName, &data.Worktree.BaseCommitSHA,
	)
	if err == sql.ErrNoRows {
		// No worktree data - this is fine
		return nil
	}
	return err
}

// loadDiffStatsSummary loads only the counts (added/removed) without the heavy content field
// This is optimized for list operations where we don't need the full diff content
func (r *SQLiteRepository) loadDiffStatsSummary(ctx context.Context, sessionID int64, data *InstanceData) error {
	err := r.db.QueryRowContext(ctx, `
		SELECT added, removed FROM diff_stats WHERE session_id = ?
	`, sessionID).Scan(&data.DiffStats.Added, &data.DiffStats.Removed)
	if err == sql.ErrNoRows {
		// No diff stats - this is fine
		return nil
	}
	return err
}

// loadDiffStats loads the full diff stats including the content field
// Use this only when the diff content is actually needed (e.g., viewing a specific session)
func (r *SQLiteRepository) loadDiffStats(ctx context.Context, sessionID int64, data *InstanceData) error {
	err := r.db.QueryRowContext(ctx, `
		SELECT added, removed, content FROM diff_stats WHERE session_id = ?
	`, sessionID).Scan(&data.DiffStats.Added, &data.DiffStats.Removed, &data.DiffStats.Content)
	if err == sql.ErrNoRows {
		// No diff stats - this is fine
		return nil
	}
	return err
}

func (r *SQLiteRepository) loadTags(ctx context.Context, sessionID int64, data *InstanceData) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.name FROM tags t
		INNER JOIN session_tags st ON t.id = st.tag_id
		WHERE st.session_id = ?
	`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return err
		}
		tags = append(tags, tag)
	}

	data.Tags = tags
	return rows.Err()
}

func (r *SQLiteRepository) loadClaudeSession(ctx context.Context, sessionID int64, data *InstanceData) error {
	var claude ClaudeSessionData
	var claudeSessionID int64

	err := r.db.QueryRowContext(ctx, `
		SELECT id, claude_session_id, conversation_id, project_name, last_attached,
			auto_reattach, preferred_session_name, create_new_on_missing,
			show_session_selector, session_timeout_minutes
		FROM claude_sessions WHERE session_id = ?
	`, sessionID).Scan(
		&claudeSessionID, &claude.SessionID, &claude.ConversationID, &claude.ProjectName, &claude.LastAttached,
		&claude.Settings.AutoReattach, &claude.Settings.PreferredSessionName, &claude.Settings.CreateNewOnMissing,
		&claude.Settings.ShowSessionSelector, &claude.Settings.SessionTimeoutMinutes,
	)
	if err == sql.ErrNoRows {
		// No Claude session - this is fine
		return nil
	}
	if err != nil {
		return err
	}

	// Load metadata
	rows, err := r.db.QueryContext(ctx, `
		SELECT key, value FROM claude_metadata WHERE claude_session_id = ?
	`, claudeSessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	claude.Metadata = make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		claude.Metadata[key] = value
	}

	data.ClaudeSession = claude
	return rows.Err()
}

// loadChildDataWithOptions loads child data based on LoadOptions configuration
// This is a helper method that selectively loads data to optimize performance
func (r *SQLiteRepository) loadChildDataWithOptions(ctx context.Context, sessionID int64, data *InstanceData, options LoadOptions) error {
	// Load worktree if requested
	if options.LoadWorktree {
		if err := r.loadWorktree(ctx, sessionID, data); err != nil {
			return fmt.Errorf("failed to load worktree: %w", err)
		}
	}

	// Load diff stats - either summary or full content
	if options.LoadDiffContent {
		// Load full content (implies stats)
		if err := r.loadDiffStats(ctx, sessionID, data); err != nil {
			return fmt.Errorf("failed to load diff stats: %w", err)
		}
	} else if options.LoadDiffStats {
		// Load only summary (counts)
		if err := r.loadDiffStatsSummary(ctx, sessionID, data); err != nil {
			return fmt.Errorf("failed to load diff stats summary: %w", err)
		}
	}

	// Load tags if requested
	if options.LoadTags {
		if err := r.loadTags(ctx, sessionID, data); err != nil {
			return fmt.Errorf("failed to load tags: %w", err)
		}
	}

	// Load Claude session if requested
	if options.LoadClaudeSession {
		if err := r.loadClaudeSession(ctx, sessionID, data); err != nil {
			return fmt.Errorf("failed to load Claude session: %w", err)
		}
	}

	return nil
}

// Context loading helper methods

// loadGitContext loads git-related context from the git_context table
func (r *SQLiteRepository) loadGitContext(ctx context.Context, sessionID int64) (*GitContext, error) {
	var gitCtx GitContext
	err := r.db.QueryRowContext(ctx, `
		SELECT branch, base_commit_sha, worktree_id, pr_number, pr_url, owner, repo, source_ref
		FROM git_context WHERE session_id = ?
	`, sessionID).Scan(
		&gitCtx.Branch, &gitCtx.BaseCommitSHA, &gitCtx.WorktreeID,
		&gitCtx.PRNumber, &gitCtx.PRURL, &gitCtx.Owner, &gitCtx.Repo, &gitCtx.SourceRef,
	)
	if err == sql.ErrNoRows {
		// No git context - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &gitCtx, nil
}

// loadFilesystemContext loads filesystem-related context from the filesystem_context table
func (r *SQLiteRepository) loadFilesystemContext(ctx context.Context, sessionID int64) (*FilesystemContext, error) {
	var fsCtx FilesystemContext
	var isWorktree int // SQLite stores booleans as integers
	err := r.db.QueryRowContext(ctx, `
		SELECT project_path, working_dir, is_worktree, main_repo_path, cloned_repo_path, existing_worktree, session_type
		FROM filesystem_context WHERE session_id = ?
	`, sessionID).Scan(
		&fsCtx.ProjectPath, &fsCtx.WorkingDir, &isWorktree,
		&fsCtx.MainRepoPath, &fsCtx.ClonedRepoPath, &fsCtx.ExistingWorktree, &fsCtx.SessionType,
	)
	if err == sql.ErrNoRows {
		// No filesystem context - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fsCtx.IsWorktree = isWorktree != 0
	return &fsCtx, nil
}

// loadTerminalContext loads terminal-related context from the terminal_context table
func (r *SQLiteRepository) loadTerminalContext(ctx context.Context, sessionID int64) (*TerminalContext, error) {
	var termCtx TerminalContext
	err := r.db.QueryRowContext(ctx, `
		SELECT height, width, tmux_session_name, tmux_prefix, tmux_server_socket, terminal_type
		FROM terminal_context WHERE session_id = ?
	`, sessionID).Scan(
		&termCtx.Height, &termCtx.Width, &termCtx.TmuxSessionName,
		&termCtx.TmuxPrefix, &termCtx.TmuxServerSocket, &termCtx.TerminalType,
	)
	if err == sql.ErrNoRows {
		// No terminal context - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &termCtx, nil
}

// loadUIPreferences loads UI preferences from the ui_preferences table
func (r *SQLiteRepository) loadUIPreferences(ctx context.Context, sessionID int64) (*UIPreferences, error) {
	var uiPrefs UIPreferences
	var isExpanded int // SQLite stores booleans as integers
	err := r.db.QueryRowContext(ctx, `
		SELECT category, is_expanded, grouping_strategy, sort_order
		FROM ui_preferences WHERE session_id = ?
	`, sessionID).Scan(
		&uiPrefs.Category, &isExpanded, &uiPrefs.GroupingStrategy, &uiPrefs.SortOrder,
	)
	if err == sql.ErrNoRows {
		// No UI preferences - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	uiPrefs.IsExpanded = isExpanded != 0

	// Load tags (tags are stored in a separate table but conceptually part of UI preferences)
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.name FROM tags t
		INNER JOIN session_tags st ON t.id = st.tag_id
		WHERE st.session_id = ?
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	uiPrefs.Tags = tags

	return &uiPrefs, nil
}

// loadActivityTracking loads activity tracking data from the activity_tracking table
func (r *SQLiteRepository) loadActivityTracking(ctx context.Context, sessionID int64) (*ActivityTracking, error) {
	var activity ActivityTracking
	err := r.db.QueryRowContext(ctx, `
		SELECT last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged
		FROM activity_tracking WHERE session_id = ?
	`, sessionID).Scan(
		&activity.LastTerminalUpdate, &activity.LastMeaningfulOutput, &activity.LastOutputSignature,
		&activity.LastAddedToQueue, &activity.LastViewed, &activity.LastAcknowledged,
	)
	if err == sql.ErrNoRows {
		// No activity tracking - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &activity, nil
}

// loadCloudContext loads cloud-related context from the cloud_context table
func (r *SQLiteRepository) loadCloudContext(ctx context.Context, sessionID int64) (*CloudContext, error) {
	var cloudCtx CloudContext
	err := r.db.QueryRowContext(ctx, `
		SELECT provider, region, instance_id, api_endpoint, api_key_ref, cloud_session_id, conversation_id
		FROM cloud_context WHERE session_id = ?
	`, sessionID).Scan(
		&cloudCtx.Provider, &cloudCtx.Region, &cloudCtx.InstanceID,
		&cloudCtx.APIEndpoint, &cloudCtx.APIKeyRef, &cloudCtx.CloudSessionID, &cloudCtx.ConversationID,
	)
	if err == sql.ErrNoRows {
		// No cloud context - this is fine, context is optional
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cloudCtx, nil
}

// GetWithOptions retrieves a single session with selective child data loading
func (r *SQLiteRepository) GetWithOptions(ctx context.Context, title string, options LoadOptions) (*InstanceData, error) {
	var data InstanceData

	// Query main session data
	err := r.db.QueryRowContext(ctx, `
		SELECT title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged,
			COALESCE(github_pr_number, 0), COALESCE(github_pr_url, ''), COALESCE(github_owner, ''), COALESCE(github_repo, ''),
			COALESCE(github_source_ref, ''), COALESCE(cloned_repo_path, ''), COALESCE(main_repo_path, ''), is_worktree
		FROM sessions WHERE title = ?
	`, title).Scan(
		&data.Title, &data.Path, &data.WorkingDir, &data.Branch, &data.Status,
		&data.Height, &data.Width, &data.CreatedAt, &data.UpdatedAt,
		&data.AutoYes, &data.Prompt, &data.Program, &data.ExistingWorktree,
		&data.Category, &data.IsExpanded, &data.SessionType, &data.TmuxPrefix,
		&data.LastTerminalUpdate, &data.LastMeaningfulOutput, &data.LastOutputSignature,
		&data.LastAddedToQueue, &data.LastViewed, &data.LastAcknowledged,
		&data.GitHubPRNumber, &data.GitHubPRURL, &data.GitHubOwner, &data.GitHubRepo,
		&data.GitHubSourceRef, &data.ClonedRepoPath, &data.MainRepoPath, &data.IsWorktree,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", title)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Get session ID for child queries
	var sessionID int64
	err = r.db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", title).Scan(&sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session ID: %w", err)
	}

	// Load child data based on options
	if err := r.loadChildDataWithOptions(ctx, sessionID, &data, options); err != nil {
		return nil, err
	}

	return &data, nil
}

// ListWithOptions retrieves all sessions with selective child data loading
func (r *SQLiteRepository) ListWithOptions(ctx context.Context, options LoadOptions) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged,
			COALESCE(github_pr_number, 0), COALESCE(github_pr_url, ''), COALESCE(github_owner, ''), COALESCE(github_repo, ''),
			COALESCE(github_source_ref, ''), COALESCE(cloned_repo_path, ''), COALESCE(main_repo_path, ''), is_worktree
		FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []InstanceData
	for rows.Next() {
		var data InstanceData
		var sessionID int64

		err := rows.Scan(
			&sessionID, &data.Title, &data.Path, &data.WorkingDir, &data.Branch, &data.Status,
			&data.Height, &data.Width, &data.CreatedAt, &data.UpdatedAt,
			&data.AutoYes, &data.Prompt, &data.Program, &data.ExistingWorktree,
			&data.Category, &data.IsExpanded, &data.SessionType, &data.TmuxPrefix,
			&data.LastTerminalUpdate, &data.LastMeaningfulOutput, &data.LastOutputSignature,
			&data.LastAddedToQueue, &data.LastViewed, &data.LastAcknowledged,
			&data.GitHubPRNumber, &data.GitHubPRURL, &data.GitHubOwner, &data.GitHubRepo,
			&data.GitHubSourceRef, &data.ClonedRepoPath, &data.MainRepoPath, &data.IsWorktree,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data based on options
		if err := r.loadChildDataWithOptions(ctx, sessionID, &data, options); err != nil {
			return nil, err
		}

		sessions = append(sessions, data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// ListByStatusWithOptions retrieves sessions filtered by status with selective loading
func (r *SQLiteRepository) ListByStatusWithOptions(ctx context.Context, status Status, options LoadOptions) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged,
			COALESCE(github_pr_number, 0), COALESCE(github_pr_url, ''), COALESCE(github_owner, ''), COALESCE(github_repo, ''),
			COALESCE(github_source_ref, ''), COALESCE(cloned_repo_path, ''), COALESCE(main_repo_path, ''), is_worktree
		FROM sessions WHERE status = ? ORDER BY created_at DESC
	`, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by status: %w", err)
	}
	defer rows.Close()

	var sessions []InstanceData
	for rows.Next() {
		var data InstanceData
		var sessionID int64

		err := rows.Scan(
			&sessionID, &data.Title, &data.Path, &data.WorkingDir, &data.Branch, &data.Status,
			&data.Height, &data.Width, &data.CreatedAt, &data.UpdatedAt,
			&data.AutoYes, &data.Prompt, &data.Program, &data.ExistingWorktree,
			&data.Category, &data.IsExpanded, &data.SessionType, &data.TmuxPrefix,
			&data.LastTerminalUpdate, &data.LastMeaningfulOutput, &data.LastOutputSignature,
			&data.LastAddedToQueue, &data.LastViewed, &data.LastAcknowledged,
			&data.GitHubPRNumber, &data.GitHubPRURL, &data.GitHubOwner, &data.GitHubRepo,
			&data.GitHubSourceRef, &data.ClonedRepoPath, &data.MainRepoPath, &data.IsWorktree,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data based on options
		if err := r.loadChildDataWithOptions(ctx, sessionID, &data, options); err != nil {
			return nil, err
		}

		sessions = append(sessions, data)
	}

	return sessions, rows.Err()
}

// ListByTagWithOptions retrieves sessions with a specific tag with selective loading
func (r *SQLiteRepository) ListByTagWithOptions(ctx context.Context, tag string, options LoadOptions) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.path, s.working_dir, s.branch, s.status, s.height, s.width,
			s.created_at, s.updated_at, s.auto_yes, s.prompt, s.program, s.existing_worktree,
			s.category, s.is_expanded, s.session_type, s.tmux_prefix,
			s.last_terminal_update, s.last_meaningful_output, s.last_output_signature,
			s.last_added_to_queue, s.last_viewed, s.last_acknowledged,
			COALESCE(s.github_pr_number, 0), COALESCE(s.github_pr_url, ''), COALESCE(s.github_owner, ''), COALESCE(s.github_repo, ''),
			COALESCE(s.github_source_ref, ''), COALESCE(s.cloned_repo_path, ''), COALESCE(s.main_repo_path, ''), s.is_worktree
		FROM sessions s
		INNER JOIN session_tags st ON s.id = st.session_id
		INNER JOIN tags t ON st.tag_id = t.id
		WHERE t.name = ?
		ORDER BY s.created_at DESC
	`, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by tag: %w", err)
	}
	defer rows.Close()

	var sessions []InstanceData
	for rows.Next() {
		var data InstanceData
		var sessionID int64

		err := rows.Scan(
			&sessionID, &data.Title, &data.Path, &data.WorkingDir, &data.Branch, &data.Status,
			&data.Height, &data.Width, &data.CreatedAt, &data.UpdatedAt,
			&data.AutoYes, &data.Prompt, &data.Program, &data.ExistingWorktree,
			&data.Category, &data.IsExpanded, &data.SessionType, &data.TmuxPrefix,
			&data.LastTerminalUpdate, &data.LastMeaningfulOutput, &data.LastOutputSignature,
			&data.LastAddedToQueue, &data.LastViewed, &data.LastAcknowledged,
			&data.GitHubPRNumber, &data.GitHubPRURL, &data.GitHubOwner, &data.GitHubRepo,
			&data.GitHubSourceRef, &data.ClonedRepoPath, &data.MainRepoPath, &data.IsWorktree,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data based on options
		if err := r.loadChildDataWithOptions(ctx, sessionID, &data, options); err != nil {
			return nil, err
		}

		sessions = append(sessions, data)
	}

	return sessions, rows.Err()
}

// --- New Session-based Repository methods ---

// GetSession implements Repository.GetSession using the new Session domain model.
// This is the preferred method for new code - it uses ContextOptions for selective loading.
func (r *SQLiteRepository) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	return r.GetSessionWithContexts(ctx, title, opts)
}

// ListSessions implements Repository.ListSessions using the new Session domain model.
func (r *SQLiteRepository) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error) {
	// Query core session fields
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, status, program, auto_yes, prompt, created_at, updated_at
		FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var (
			sessionID int64
			session   Session
		)

		err := rows.Scan(
			&sessionID, &session.Title, &session.Status, &session.Program,
			&session.AutoYes, &session.Prompt, &session.CreatedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		session.ID = session.Title // Use title as ID for compatibility

		// Load contexts based on options
		if err := r.loadSessionContexts(ctx, sessionID, &session, opts); err != nil {
			return nil, err
		}

		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// CreateSession implements Repository.CreateSession using the new Session domain model.
func (r *SQLiteRepository) CreateSession(ctx context.Context, session *Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert core session record
	result, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			title, status, program, auto_yes, prompt, created_at, updated_at,
			path, working_dir, branch, height, width, existing_worktree,
			category, is_expanded, session_type, tmux_prefix
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', 0, 0, '', '', 0, '', '')
	`,
		session.Title, session.Status, session.Program, session.AutoYes, session.Prompt,
		session.CreatedAt, session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	sessionID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get session ID: %w", err)
	}

	// Insert optional contexts
	if session.Git != nil {
		if err := r.insertGitContext(ctx, tx, sessionID, session.Git); err != nil {
			return fmt.Errorf("failed to insert git context: %w", err)
		}
	}

	if session.Filesystem != nil {
		if err := r.insertFilesystemContext(ctx, tx, sessionID, session.Filesystem); err != nil {
			return fmt.Errorf("failed to insert filesystem context: %w", err)
		}
	}

	if session.Terminal != nil {
		if err := r.insertTerminalContext(ctx, tx, sessionID, session.Terminal); err != nil {
			return fmt.Errorf("failed to insert terminal context: %w", err)
		}
	}

	if session.UI != nil {
		if err := r.insertUIPreferences(ctx, tx, sessionID, session.UI); err != nil {
			return fmt.Errorf("failed to insert UI preferences: %w", err)
		}
	}

	if session.Activity != nil {
		if err := r.insertActivityTracking(ctx, tx, sessionID, session.Activity); err != nil {
			return fmt.Errorf("failed to insert activity tracking: %w", err)
		}
	}

	if session.Cloud != nil {
		if err := r.insertCloudContext(ctx, tx, sessionID, session.Cloud); err != nil {
			return fmt.Errorf("failed to insert cloud context: %w", err)
		}
	}

	return tx.Commit()
}

// UpdateSession implements Repository.UpdateSession using the new Session domain model.
func (r *SQLiteRepository) UpdateSession(ctx context.Context, session *Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update core session fields
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions SET
			status = ?, program = ?, auto_yes = ?, prompt = ?, updated_at = ?
		WHERE title = ?
	`, session.Status, session.Program, session.AutoYes, session.Prompt, time.Now(), session.Title)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", session.Title)
	}

	// Get session ID for context updates
	var sessionID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM sessions WHERE title = ?", session.Title).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ID: %w", err)
	}

	// Update contexts (delete and re-insert pattern for simplicity)
	contextTables := []string{"git_context", "filesystem_context", "terminal_context", "ui_preferences", "activity_tracking", "cloud_context"}
	for _, table := range contextTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE session_id = ?", table), sessionID); err != nil {
			return fmt.Errorf("failed to delete old %s: %w", table, err)
		}
	}

	// Re-insert contexts that are present
	if session.Git != nil {
		if err := r.insertGitContext(ctx, tx, sessionID, session.Git); err != nil {
			return fmt.Errorf("failed to insert git context: %w", err)
		}
	}

	if session.Filesystem != nil {
		if err := r.insertFilesystemContext(ctx, tx, sessionID, session.Filesystem); err != nil {
			return fmt.Errorf("failed to insert filesystem context: %w", err)
		}
	}

	if session.Terminal != nil {
		if err := r.insertTerminalContext(ctx, tx, sessionID, session.Terminal); err != nil {
			return fmt.Errorf("failed to insert terminal context: %w", err)
		}
	}

	if session.UI != nil {
		if err := r.insertUIPreferences(ctx, tx, sessionID, session.UI); err != nil {
			return fmt.Errorf("failed to insert UI preferences: %w", err)
		}
	}

	if session.Activity != nil {
		if err := r.insertActivityTracking(ctx, tx, sessionID, session.Activity); err != nil {
			return fmt.Errorf("failed to insert activity tracking: %w", err)
		}
	}

	if session.Cloud != nil {
		if err := r.insertCloudContext(ctx, tx, sessionID, session.Cloud); err != nil {
			return fmt.Errorf("failed to insert cloud context: %w", err)
		}
	}

	return tx.Commit()
}

// loadSessionContexts loads optional contexts based on ContextOptions.
// This is a helper method used by ListSessions and GetSession.
func (r *SQLiteRepository) loadSessionContexts(ctx context.Context, sessionID int64, session *Session, opts ContextOptions) error {
	if opts.LoadGit {
		gitCtx, err := r.loadGitContext(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load git context: %w", err)
		}
		session.Git = gitCtx
	}

	if opts.LoadFilesystem {
		fsCtx, err := r.loadFilesystemContext(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load filesystem context: %w", err)
		}
		session.Filesystem = fsCtx
	}

	if opts.LoadTerminal {
		termCtx, err := r.loadTerminalContext(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load terminal context: %w", err)
		}
		session.Terminal = termCtx
	}

	if opts.LoadUI {
		uiPrefs, err := r.loadUIPreferences(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load UI preferences: %w", err)
		}
		session.UI = uiPrefs
	}

	if opts.LoadActivity {
		activity, err := r.loadActivityTracking(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load activity tracking: %w", err)
		}
		session.Activity = activity
	}

	if opts.LoadCloud {
		cloudCtx, err := r.loadCloudContext(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load cloud context: %w", err)
		}
		session.Cloud = cloudCtx
	}

	return nil
}
