package session

// SQLite database schema for session persistence
// This schema maps InstanceData fields to a normalized SQLite schema
// with proper indexing for common query patterns.

const (
	// SchemaVersion tracks the database schema version for migrations
	SchemaVersion = 3

	// CreateSessionsTable creates the main sessions table
	CreateSessionsTable = `
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT UNIQUE NOT NULL,
		path TEXT NOT NULL,
		working_dir TEXT,
		branch TEXT,
		status INTEGER NOT NULL,
		height INTEGER,
		width INTEGER,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		auto_yes INTEGER DEFAULT 0,
		prompt TEXT,
		program TEXT NOT NULL,
		existing_worktree TEXT,
		category TEXT,
		is_expanded INTEGER DEFAULT 1,
		session_type TEXT,
		tmux_prefix TEXT,
		last_terminal_update DATETIME,
		last_meaningful_output DATETIME,
		last_output_signature TEXT,
		last_added_to_queue DATETIME,
		last_viewed DATETIME,
		last_acknowledged DATETIME,
		-- GitHub integration fields (added in schema v2)
		github_pr_number INTEGER,
		github_pr_url TEXT,
		github_owner TEXT,
		github_repo TEXT,
		github_source_ref TEXT,
		cloned_repo_path TEXT,
		-- Worktree detection fields (added in schema v2)
		main_repo_path TEXT,
		is_worktree INTEGER DEFAULT 0
	);`

	// CreateWorktreesTable stores git worktree information
	CreateWorktreesTable = `
	CREATE TABLE IF NOT EXISTS worktrees (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		repo_path TEXT NOT NULL,
		worktree_path TEXT NOT NULL,
		session_name TEXT NOT NULL,
		branch_name TEXT NOT NULL,
		base_commit_sha TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`

	// CreateDiffStatsTable stores git diff statistics
	CreateDiffStatsTable = `
	CREATE TABLE IF NOT EXISTS diff_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		added INTEGER DEFAULT 0,
		removed INTEGER DEFAULT 0,
		content TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`

	// CreateTagsTable stores session tags (many-to-many relationship)
	CreateTagsTable = `
	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);`

	// CreateSessionTagsTable junction table for many-to-many relationship
	CreateSessionTagsTable = `
	CREATE TABLE IF NOT EXISTS session_tags (
		session_id INTEGER NOT NULL,
		tag_id INTEGER NOT NULL,
		PRIMARY KEY (session_id, tag_id),
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
	);`

	// CreateClaudeSessionsTable stores Claude Code session data
	CreateClaudeSessionsTable = `
	CREATE TABLE IF NOT EXISTS claude_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		claude_session_id TEXT,
		conversation_id TEXT,
		project_name TEXT,
		last_attached DATETIME,
		auto_reattach INTEGER DEFAULT 0,
		preferred_session_name TEXT,
		create_new_on_missing INTEGER DEFAULT 0,
		show_session_selector INTEGER DEFAULT 0,
		session_timeout_minutes INTEGER DEFAULT 0,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`

	// CreateClaudeMetadataTable stores flexible key-value metadata for Claude sessions
	CreateClaudeMetadataTable = `
	CREATE TABLE IF NOT EXISTS claude_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		claude_session_id INTEGER NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		FOREIGN KEY (claude_session_id) REFERENCES claude_sessions(id) ON DELETE CASCADE
	);`

	// CreateSchemaVersionTable tracks schema version for migrations
	CreateSchemaVersionTable = `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL
	);`

	// IndexSessionsTitle creates index on title for fast lookups
	IndexSessionsTitle = `
	CREATE INDEX IF NOT EXISTS idx_sessions_title ON sessions(title);`

	// IndexSessionsStatus creates index on status for filtered queries
	IndexSessionsStatus = `
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);`

	// IndexSessionsCategory creates index on category for grouped queries
	IndexSessionsCategory = `
	CREATE INDEX IF NOT EXISTS idx_sessions_category ON sessions(category);`

	// IndexSessionsLastMeaningfulOutput creates index for review queue queries
	IndexSessionsLastMeaningfulOutput = `
	CREATE INDEX IF NOT EXISTS idx_sessions_last_meaningful_output ON sessions(last_meaningful_output);`

	// IndexSessionsLastAcknowledged creates index for review queue snooze queries
	IndexSessionsLastAcknowledged = `
	CREATE INDEX IF NOT EXISTS idx_sessions_last_acknowledged ON sessions(last_acknowledged);`

	// IndexSessionsCreatedAt creates index for time-based queries
	IndexSessionsCreatedAt = `
	CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);`

	// IndexSessionTagsSessionID creates index on session_tags for faster tag lookups
	IndexSessionTagsSessionID = `
	CREATE INDEX IF NOT EXISTS idx_session_tags_session_id ON session_tags(session_id);`

	// IndexSessionTagsTagID creates index on session_tags for faster tag lookups
	IndexSessionTagsTagID = `
	CREATE INDEX IF NOT EXISTS idx_session_tags_tag_id ON session_tags(tag_id);`

	// IndexTagsName creates index on tags name for faster tag lookups
	IndexTagsName = `
	CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);`
)

// MigrateV1ToV2 adds GitHub integration and worktree detection fields
var MigrateV1ToV2 = []string{
	"ALTER TABLE sessions ADD COLUMN github_pr_number INTEGER;",
	"ALTER TABLE sessions ADD COLUMN github_pr_url TEXT;",
	"ALTER TABLE sessions ADD COLUMN github_owner TEXT;",
	"ALTER TABLE sessions ADD COLUMN github_repo TEXT;",
	"ALTER TABLE sessions ADD COLUMN github_source_ref TEXT;",
	"ALTER TABLE sessions ADD COLUMN cloned_repo_path TEXT;",
	"ALTER TABLE sessions ADD COLUMN main_repo_path TEXT;",
	"ALTER TABLE sessions ADD COLUMN is_worktree INTEGER DEFAULT 0;",
}

// MigrateV2ToV3 normalizes context data into dedicated tables for better organization
var MigrateV2ToV3 = []string{
	// Create git_context table
	`CREATE TABLE IF NOT EXISTS git_context (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		branch TEXT,
		base_commit_sha TEXT,
		worktree_id INTEGER,
		pr_number INTEGER,
		pr_url TEXT,
		owner TEXT,
		repo TEXT,
		source_ref TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (worktree_id) REFERENCES worktrees(id) ON DELETE SET NULL
	);`,

	// Create filesystem_context table
	`CREATE TABLE IF NOT EXISTS filesystem_context (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		project_path TEXT,
		working_dir TEXT,
		is_worktree INTEGER DEFAULT 0,
		main_repo_path TEXT,
		cloned_repo_path TEXT,
		existing_worktree TEXT,
		session_type TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`,

	// Create terminal_context table
	`CREATE TABLE IF NOT EXISTS terminal_context (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		height INTEGER,
		width INTEGER,
		tmux_session_name TEXT,
		tmux_prefix TEXT,
		tmux_server_socket TEXT,
		terminal_type TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`,

	// Create ui_preferences table
	`CREATE TABLE IF NOT EXISTS ui_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		category TEXT,
		is_expanded INTEGER DEFAULT 1,
		grouping_strategy TEXT,
		sort_order TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`,

	// Create activity_tracking table
	`CREATE TABLE IF NOT EXISTS activity_tracking (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		last_terminal_update DATETIME,
		last_meaningful_output DATETIME,
		last_output_signature TEXT,
		last_added_to_queue DATETIME,
		last_viewed DATETIME,
		last_acknowledged DATETIME,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`,

	// Create cloud_context table
	`CREATE TABLE IF NOT EXISTS cloud_context (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL UNIQUE,
		provider TEXT,
		region TEXT,
		instance_id TEXT,
		api_endpoint TEXT,
		api_key_ref TEXT,
		cloud_session_id TEXT,
		conversation_id TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);`,

	// Create indexes for fast lookups
	"CREATE INDEX IF NOT EXISTS idx_git_context_session_id ON git_context(session_id);",
	"CREATE INDEX IF NOT EXISTS idx_filesystem_context_session_id ON filesystem_context(session_id);",
	"CREATE INDEX IF NOT EXISTS idx_terminal_context_session_id ON terminal_context(session_id);",
	"CREATE INDEX IF NOT EXISTS idx_ui_preferences_session_id ON ui_preferences(session_id);",
	"CREATE INDEX IF NOT EXISTS idx_activity_tracking_session_id ON activity_tracking(session_id);",
	"CREATE INDEX IF NOT EXISTS idx_cloud_context_session_id ON cloud_context(session_id);",

	// Migrate existing data from sessions table to context tables
	`INSERT INTO git_context (session_id, branch, pr_number, pr_url, owner, repo, source_ref)
	SELECT id, branch, github_pr_number, github_pr_url, github_owner, github_repo, github_source_ref
	FROM sessions
	WHERE branch IS NOT NULL OR github_pr_number IS NOT NULL OR github_pr_url IS NOT NULL
		OR github_owner IS NOT NULL OR github_repo IS NOT NULL OR github_source_ref IS NOT NULL;`,

	`INSERT INTO filesystem_context (session_id, project_path, working_dir, is_worktree, main_repo_path, cloned_repo_path, existing_worktree, session_type)
	SELECT id, path, working_dir, is_worktree, main_repo_path, cloned_repo_path, existing_worktree, session_type
	FROM sessions;`,

	`INSERT INTO terminal_context (session_id, height, width, tmux_prefix)
	SELECT id, height, width, tmux_prefix
	FROM sessions
	WHERE height IS NOT NULL OR width IS NOT NULL OR tmux_prefix IS NOT NULL;`,

	`INSERT INTO ui_preferences (session_id, category, is_expanded)
	SELECT id, category, is_expanded
	FROM sessions;`,

	`INSERT INTO activity_tracking (session_id, last_terminal_update, last_meaningful_output, last_output_signature, last_added_to_queue, last_viewed, last_acknowledged)
	SELECT id, last_terminal_update, last_meaningful_output, last_output_signature, last_added_to_queue, last_viewed, last_acknowledged
	FROM sessions;`,
}

// InitializeSchema creates all tables and indexes for SQLite storage
var InitializeSchema = []string{
	CreateSessionsTable,
	CreateWorktreesTable,
	CreateDiffStatsTable,
	CreateTagsTable,
	CreateSessionTagsTable,
	CreateClaudeSessionsTable,
	CreateClaudeMetadataTable,
	CreateSchemaVersionTable,
	IndexSessionsTitle,
	IndexSessionsStatus,
	IndexSessionsCategory,
	IndexSessionsLastMeaningfulOutput,
	IndexSessionsLastAcknowledged,
	IndexSessionsCreatedAt,
	IndexSessionTagsSessionID,
	IndexSessionTagsTagID,
	IndexTagsName,
}
