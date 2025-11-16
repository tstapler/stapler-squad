package session

// SQLite database schema for session persistence
// This schema maps InstanceData fields to a normalized SQLite schema
// with proper indexing for common query patterns.

const (
	// SchemaVersion tracks the database schema version for migrations
	SchemaVersion = 1

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
		last_acknowledged DATETIME
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
