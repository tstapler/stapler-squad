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

	// Record schema version
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO schema_version (version, applied_at)
		VALUES (?, ?)
	`, SchemaVersion, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	return nil
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
			last_added_to_queue, last_viewed, last_acknowledged
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		data.Title, data.Path, data.WorkingDir, data.Branch, data.Status,
		data.Height, data.Width, data.CreatedAt, data.UpdatedAt,
		data.AutoYes, data.Prompt, data.Program, data.ExistingWorktree,
		data.Category, data.IsExpanded, data.SessionType, data.TmuxPrefix,
		data.LastTerminalUpdate, data.LastMeaningfulOutput, data.LastOutputSignature,
		data.LastAddedToQueue, data.LastViewed, data.LastAcknowledged,
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

	// Insert diff stats if present
	if data.DiffStats.Content != "" {
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
			last_added_to_queue = ?, last_viewed = ?, last_acknowledged = ?
		WHERE title = ?
	`,
		data.Path, data.WorkingDir, data.Branch, data.Status, data.Height, data.Width,
		time.Now(), data.AutoYes, data.Prompt, data.Program, data.ExistingWorktree,
		data.Category, data.IsExpanded, data.SessionType, data.TmuxPrefix,
		data.LastTerminalUpdate, data.LastMeaningfulOutput, data.LastOutputSignature,
		data.LastAddedToQueue, data.LastViewed, data.LastAcknowledged,
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
	if data.DiffStats.Content != "" {
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
	var data InstanceData

	// Query main session data
	err := r.db.QueryRowContext(ctx, `
		SELECT title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged
		FROM sessions WHERE title = ?
	`, title).Scan(
		&data.Title, &data.Path, &data.WorkingDir, &data.Branch, &data.Status,
		&data.Height, &data.Width, &data.CreatedAt, &data.UpdatedAt,
		&data.AutoYes, &data.Prompt, &data.Program, &data.ExistingWorktree,
		&data.Category, &data.IsExpanded, &data.SessionType, &data.TmuxPrefix,
		&data.LastTerminalUpdate, &data.LastMeaningfulOutput, &data.LastOutputSignature,
		&data.LastAddedToQueue, &data.LastViewed, &data.LastAcknowledged,
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

	// Load child data
	if err := r.loadWorktree(ctx, sessionID, &data); err != nil {
		return nil, fmt.Errorf("failed to load worktree: %w", err)
	}
	if err := r.loadDiffStats(ctx, sessionID, &data); err != nil {
		return nil, fmt.Errorf("failed to load diff stats: %w", err)
	}
	if err := r.loadTags(ctx, sessionID, &data); err != nil {
		return nil, fmt.Errorf("failed to load tags: %w", err)
	}
	if err := r.loadClaudeSession(ctx, sessionID, &data); err != nil {
		return nil, fmt.Errorf("failed to load Claude session: %w", err)
	}

	return &data, nil
}

// List retrieves all sessions from the database
func (r *SQLiteRepository) List(ctx context.Context) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged
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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data
		if err := r.loadWorktree(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load worktree: %w", err)
		}
		if err := r.loadDiffStats(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load diff stats: %w", err)
		}
		if err := r.loadTags(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load tags: %w", err)
		}
		if err := r.loadClaudeSession(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load Claude session: %w", err)
		}

		sessions = append(sessions, data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// ListByStatus retrieves sessions filtered by status
func (r *SQLiteRepository) ListByStatus(ctx context.Context, status Status) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, path, working_dir, branch, status, height, width,
			created_at, updated_at, auto_yes, prompt, program, existing_worktree,
			category, is_expanded, session_type, tmux_prefix,
			last_terminal_update, last_meaningful_output, last_output_signature,
			last_added_to_queue, last_viewed, last_acknowledged
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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data
		if err := r.loadWorktree(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load worktree: %w", err)
		}
		if err := r.loadDiffStats(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load diff stats: %w", err)
		}
		if err := r.loadTags(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load tags: %w", err)
		}
		if err := r.loadClaudeSession(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load Claude session: %w", err)
		}

		sessions = append(sessions, data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// ListByTag retrieves sessions that have a specific tag
func (r *SQLiteRepository) ListByTag(ctx context.Context, tag string) ([]InstanceData, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.path, s.working_dir, s.branch, s.status, s.height, s.width,
			s.created_at, s.updated_at, s.auto_yes, s.prompt, s.program, s.existing_worktree,
			s.category, s.is_expanded, s.session_type, s.tmux_prefix,
			s.last_terminal_update, s.last_meaningful_output, s.last_output_signature,
			s.last_added_to_queue, s.last_viewed, s.last_acknowledged
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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Load child data
		if err := r.loadWorktree(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load worktree: %w", err)
		}
		if err := r.loadDiffStats(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load diff stats: %w", err)
		}
		if err := r.loadTags(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load tags: %w", err)
		}
		if err := r.loadClaudeSession(ctx, sessionID, &data); err != nil {
			return nil, fmt.Errorf("failed to load Claude session: %w", err)
		}

		sessions = append(sessions, data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
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
