package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudemetadata"
	"github.com/tstapler/stapler-squad/session/ent/claudesession"
	"github.com/tstapler/stapler-squad/session/ent/diffstats"
	"github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/tag"
	"github.com/tstapler/stapler-squad/session/ent/worktree"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// EntRepository implements the Repository interface using Ent ORM as the storage backend.
// It provides type-safe database operations with automatic schema migrations.
type EntRepository struct {
	client        *ent.Client
	dbPath        string
	migrationMode bool // When true, enables dual-write mode for migration
}

// NewEntRepository creates a new Ent repository with the given options.
// The database will be initialized with the schema if it doesn't exist.
func NewEntRepository(opts ...RepositoryOption) (*EntRepository, error) {
	repo := &EntRepository{
		dbPath:        "~/.stapler-squad/sessions.db", // Default path
		migrationMode: false,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(repo); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Expand home directory in path if needed
	expandedPath := repo.dbPath
	if len(expandedPath) >= 2 && expandedPath[:2] == "~/" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		expandedPath = filepath.Join(homeDir, expandedPath[2:])
	}

	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(expandedPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection with WAL mode for better concurrency
	dbPath := expandedPath + "?_journal_mode=WAL&_timeout=5000&_fk=1"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(5) // Allow multiple readers
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Create Ent client with the existing database connection
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))

	// Run automatic schema migration
	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	repo.client = client

	return repo, nil
}

// Create inserts a new session into the database
func (r *EntRepository) Create(ctx context.Context, data InstanceData) error {
	// Start transaction
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create main session
	sessionCreate := tx.Session.Create().
		SetTitle(data.Title).
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetCreatedAt(data.CreatedAt).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded)

	// Set optional fields
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
	if data.SessionType != "" {
		sessionCreate.SetSessionType(string(data.SessionType))
	}
	if data.TmuxPrefix != "" {
		sessionCreate.SetTmuxPrefix(data.TmuxPrefix)
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

	sess, err := sessionCreate.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Create worktree if present
	if data.Worktree.RepoPath != "" {
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
	}

	// Create diff stats
	diffCreate := tx.DiffStats.Create().
		SetSessionID(sess.ID).
		SetAdded(data.DiffStats.Added).
		SetRemoved(data.DiffStats.Removed)

	if data.DiffStats.Content != "" {
		diffCreate.SetContent(data.DiffStats.Content)
	}

	if _, err := diffCreate.Save(ctx); err != nil {
		return fmt.Errorf("failed to create diff stats: %w", err)
	}

	// Create/associate tags
	if len(data.Tags) > 0 {
		for _, tagName := range data.Tags {
			// Get or create tag
			t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					// Create new tag
					t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
					if err != nil {
						return fmt.Errorf("failed to create tag %s: %w", tagName, err)
					}
				} else {
					return fmt.Errorf("failed to query tag %s: %w", tagName, err)
				}
			}

			// Associate tag with session
			if err := tx.Session.UpdateOne(sess).AddTags(t).Exec(ctx); err != nil {
				return fmt.Errorf("failed to associate tag %s: %w", tagName, err)
			}
		}
	}

	// Create Claude session if present
	if data.ClaudeSession.SessionID != "" {
		claudeCreate := tx.ClaudeSession.Create().
			SetSessionID(sess.ID).
			SetClaudeSessionID(data.ClaudeSession.SessionID).
			SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
			SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
			SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
			SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)

		if data.ClaudeSession.ConversationID != "" {
			claudeCreate.SetConversationID(data.ClaudeSession.ConversationID)
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

		claudeSess, err := claudeCreate.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to create claude session: %w", err)
		}

		// Create Claude metadata entries
		for key, value := range data.ClaudeSession.Metadata {
			if _, err := tx.ClaudeMetadata.Create().
				SetClaudeSessionID(claudeSess.ID).
				SetKey(key).
				SetValue(value).
				Save(ctx); err != nil {
				return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update modifies an existing session in the database
func (r *EntRepository) Update(ctx context.Context, data InstanceData) error {
	// Start transaction
	tx, err := r.client.Tx(ctx)
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
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded)

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
	if data.SessionType != "" {
		sessionUpdate.SetSessionType(string(data.SessionType))
	}
	if data.TmuxPrefix != "" {
		sessionUpdate.SetTmuxPrefix(data.TmuxPrefix)
	}
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

	if err := sessionUpdate.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

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

	// Update diff stats
	existingDiff, err := tx.DiffStats.Query().Where(diffstats.HasSessionWith(session.ID(sess.ID))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			// Create new diff stats
			diffCreate := tx.DiffStats.Create().
				SetSessionID(sess.ID).
				SetAdded(data.DiffStats.Added).
				SetRemoved(data.DiffStats.Removed)

			if data.DiffStats.Content != "" {
				diffCreate.SetContent(data.DiffStats.Content)
			}

			if _, err := diffCreate.Save(ctx); err != nil {
				return fmt.Errorf("failed to create diff stats: %w", err)
			}
		} else {
			return fmt.Errorf("failed to query diff stats: %w", err)
		}
	} else {
		// Update existing diff stats
		diffUpdate := tx.DiffStats.UpdateOne(existingDiff).
			SetAdded(data.DiffStats.Added).
			SetRemoved(data.DiffStats.Removed)

		if data.DiffStats.Content != "" {
			diffUpdate.SetContent(data.DiffStats.Content)
		}

		if err := diffUpdate.Exec(ctx); err != nil {
			return fmt.Errorf("failed to update diff stats: %w", err)
		}
	}

	// Update tags - clear existing and add new ones
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	if len(data.Tags) > 0 {
		for _, tagName := range data.Tags {
			// Get or create tag
			t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					// Create new tag
					t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
					if err != nil {
						return fmt.Errorf("failed to create tag %s: %w", tagName, err)
					}
				} else {
					return fmt.Errorf("failed to query tag %s: %w", tagName, err)
				}
			}

			// Associate tag with session
			if err := tx.Session.UpdateOne(sess).AddTags(t).Exec(ctx); err != nil {
				return fmt.Errorf("failed to associate tag %s: %w", tagName, err)
			}
		}
	}

	// Update Claude session if present
	if data.ClaudeSession.SessionID != "" {
		existingClaude, err := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				// Create new Claude session
				claudeCreate := tx.ClaudeSession.Create().
					SetSessionID(sess.ID).
					SetClaudeSessionID(data.ClaudeSession.SessionID).
					SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
					SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
					SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
					SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)

				if data.ClaudeSession.ConversationID != "" {
					claudeCreate.SetConversationID(data.ClaudeSession.ConversationID)
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

				_, err := claudeCreate.Save(ctx)
				if err != nil {
					return fmt.Errorf("failed to create claude session: %w", err)
				}
			} else {
				return fmt.Errorf("failed to query claude session: %w", err)
			}
		} else {
			// Update existing Claude session
			claudeUpdate := tx.ClaudeSession.UpdateOne(existingClaude).
				SetClaudeSessionID(data.ClaudeSession.SessionID).
				SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
				SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
				SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
				SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)

			if data.ClaudeSession.ConversationID != "" {
				claudeUpdate.SetConversationID(data.ClaudeSession.ConversationID)
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

			// Update Claude metadata - clear existing and add new ones
			if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.ID(existingClaude.ID))).Exec(ctx); err != nil {
				return fmt.Errorf("failed to clear claude metadata: %w", err)
			}
		}

		// Add metadata entries
		for key, value := range data.ClaudeSession.Metadata {
			claudeSess, _ := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Only(ctx)
			if _, err := tx.ClaudeMetadata.Create().
				SetClaudeSessionID(claudeSess.ID).
				SetKey(key).
				SetValue(value).
				Save(ctx); err != nil {
				return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete removes a session from the database by title
func (r *EntRepository) Delete(ctx context.Context, title string) error {
	// Start transaction for atomic deletion
	tx, err := r.client.Tx(ctx)
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

	// Delete claude session and its metadata if exists
	claudeSessions, err := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query claude sessions: %w", err)
	}
	for _, cs := range claudeSessions {
		// Delete claude metadata
		if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.ID(cs.ID))).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete claude metadata: %w", err)
		}
		// Delete claude session
		if err := tx.ClaudeSession.DeleteOne(cs).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete claude session: %w", err)
		}
	}

	// Clear tag associations (many-to-many)
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
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

// Get retrieves a single session by title
func (r *EntRepository) Get(ctx context.Context, title string) (*InstanceData, error) {
	// Find session with all relationships eagerly loaded
	sess, err := r.client.Session.Query().
		Where(session.Title(title)).
		WithWorktree().
		WithDiffStats().
		WithTags().
		WithClaudeSession(func(q *ent.ClaudeSessionQuery) {
			q.WithMetadata()
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("session not found: %s", title)
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Convert to InstanceData
	return r.sessionToInstanceData(sess), nil
}

// List retrieves all sessions from the database
func (r *EntRepository) List(ctx context.Context) ([]InstanceData, error) {
	// Query all sessions with relationships
	sessions, err := r.client.Session.Query().
		WithWorktree().
		WithDiffStats().
		WithTags().
		WithClaudeSession(func(q *ent.ClaudeSessionQuery) {
			q.WithMetadata()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}

	// Convert to InstanceData
	result := make([]InstanceData, len(sessions))
	for i, s := range sessions {
		result[i] = *r.sessionToInstanceData(s)
	}

	return result, nil
}

// ListByStatus retrieves sessions filtered by status
func (r *EntRepository) ListByStatus(ctx context.Context, status Status) ([]InstanceData, error) {
	// Query sessions by status with relationships
	sessions, err := r.client.Session.Query().
		Where(session.Status(int(status))).
		WithWorktree().
		WithDiffStats().
		WithTags().
		WithClaudeSession(func(q *ent.ClaudeSessionQuery) {
			q.WithMetadata()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by status: %w", err)
	}

	// Convert to InstanceData
	result := make([]InstanceData, len(sessions))
	for i, s := range sessions {
		result[i] = *r.sessionToInstanceData(s)
	}

	return result, nil
}

// ListByTag retrieves sessions that have a specific tag
func (r *EntRepository) ListByTag(ctx context.Context, tagName string) ([]InstanceData, error) {
	// Query sessions that have the specified tag
	sessions, err := r.client.Session.Query().
		Where(session.HasTagsWith(tag.Name(tagName))).
		WithWorktree().
		WithDiffStats().
		WithTags().
		WithClaudeSession(func(q *ent.ClaudeSessionQuery) {
			q.WithMetadata()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by tag: %w", err)
	}

	// Convert to InstanceData
	result := make([]InstanceData, len(sessions))
	for i, s := range sessions {
		result[i] = *r.sessionToInstanceData(s)
	}

	return result, nil
}

// UpdateTimestamps efficiently updates only timestamp fields for a session
func (r *EntRepository) UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error {
	// Find session by title
	sess, err := r.client.Session.Query().Where(session.Title(title)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Update only timestamp fields
	update := r.client.Session.UpdateOne(sess).
		SetUpdatedAt(time.Now())

	if !lastTerminalUpdate.IsZero() {
		update.SetLastTerminalUpdate(lastTerminalUpdate)
	}
	if !lastMeaningfulOutput.IsZero() {
		update.SetLastMeaningfulOutput(lastMeaningfulOutput)
	}
	if lastOutputSignature != "" {
		update.SetLastOutputSignature(lastOutputSignature)
	}

	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update timestamps: %w", err)
	}

	return nil
}

// Close performs cleanup and releases resources
func (r *EntRepository) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// sessionToInstanceData converts an Ent Session entity to InstanceData
func (r *EntRepository) sessionToInstanceData(sess *ent.Session) *InstanceData {
	data := &InstanceData{
		Title:               sess.Title,
		Path:                sess.Path,
		WorkingDir:          sess.WorkingDir,
		Branch:              sess.Branch,
		Status:              Status(sess.Status),
		Height:              sess.Height,
		Width:               sess.Width,
		CreatedAt:           sess.CreatedAt,
		UpdatedAt:           sess.UpdatedAt,
		AutoYes:             sess.AutoYes,
		Prompt:              sess.Prompt,
		Program:             sess.Program,
		ExistingWorktree:    sess.ExistingWorktree,
		Category:            sess.Category,
		IsExpanded:          sess.IsExpanded,
		TmuxPrefix:          sess.TmuxPrefix,
		LastOutputSignature: sess.LastOutputSignature,
	}

	// Set optional time fields
	if sess.LastTerminalUpdate != nil {
		data.LastTerminalUpdate = *sess.LastTerminalUpdate
	}
	if sess.LastMeaningfulOutput != nil {
		data.LastMeaningfulOutput = *sess.LastMeaningfulOutput
	}
	if sess.LastAddedToQueue != nil {
		data.LastAddedToQueue = *sess.LastAddedToQueue
	}
	if sess.LastViewed != nil {
		data.LastViewed = *sess.LastViewed
	}
	if sess.LastAcknowledged != nil {
		data.LastAcknowledged = *sess.LastAcknowledged
	}

	// Set session type
	if sess.SessionType != "" {
		data.SessionType = SessionType(sess.SessionType)
	}

	// Convert worktree if present
	if sess.Edges.Worktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      sess.Edges.Worktree.RepoPath,
			WorktreePath:  sess.Edges.Worktree.WorktreePath,
			SessionName:   sess.Edges.Worktree.SessionName,
			BranchName:    sess.Edges.Worktree.BranchName,
			BaseCommitSHA: sess.Edges.Worktree.BaseCommitSha,
		}
	}

	// Convert diff stats if present
	if sess.Edges.DiffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   sess.Edges.DiffStats.Added,
			Removed: sess.Edges.DiffStats.Removed,
			Content: sess.Edges.DiffStats.Content,
		}
	}

	// Convert tags
	if len(sess.Edges.Tags) > 0 {
		data.Tags = make([]string, len(sess.Edges.Tags))
		for i, t := range sess.Edges.Tags {
			data.Tags[i] = t.Name
		}
	}

	// Convert Claude session if present
	if sess.Edges.ClaudeSession != nil {
		cs := sess.Edges.ClaudeSession
		data.ClaudeSession = ClaudeSessionData{
			SessionID:      cs.ClaudeSessionID,
			ConversationID: cs.ConversationID,
			ProjectName:    cs.ProjectName,
			Settings: ClaudeSettings{
				AutoReattach:          cs.AutoReattach,
				PreferredSessionName:  cs.PreferredSessionName,
				CreateNewOnMissing:    cs.CreateNewOnMissing,
				ShowSessionSelector:   cs.ShowSessionSelector,
				SessionTimeoutMinutes: cs.SessionTimeoutMinutes,
			},
		}

		// Set optional time field
		if cs.LastAttached != nil {
			data.ClaudeSession.LastAttached = *cs.LastAttached
		}

		// Convert metadata
		if len(cs.Edges.Metadata) > 0 {
			data.ClaudeSession.Metadata = make(map[string]string)
			for _, m := range cs.Edges.Metadata {
				data.ClaudeSession.Metadata[m.Key] = m.Value
			}
		}
	}

	return data
}

// --- New Session-based Repository methods ---
// These are stub implementations pending full Session-domain migration (Story 2.5).

// GetSession retrieves a session using the new Session domain model.
// Stub: not yet implemented; use InstanceData-based methods instead.
func (r *EntRepository) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	return nil, fmt.Errorf("GetSession not yet implemented for EntRepository")
}

// ListSessions retrieves all sessions using the new Session domain model.
// Stub: not yet implemented; use InstanceData-based methods instead.
func (r *EntRepository) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error) {
	return nil, fmt.Errorf("ListSessions not yet implemented for EntRepository")
}

// CreateSession creates a new session from the Session domain model.
// Stub: not yet implemented; use InstanceData-based methods instead.
func (r *EntRepository) CreateSession(ctx context.Context, session *Session) error {
	return fmt.Errorf("CreateSession not yet implemented for EntRepository")
}

// UpdateSession updates an existing session using the Session domain model.
// Stub: not yet implemented; use InstanceData-based methods instead.
func (r *EntRepository) UpdateSession(ctx context.Context, session *Session) error {
	return fmt.Errorf("UpdateSession not yet implemented for EntRepository")
}

// GetWithOptions retrieves a single session with selective child data loading.
// EntRepository: Delegates to Get with full loading.
func (r *EntRepository) GetWithOptions(ctx context.Context, title string, options LoadOptions) (*InstanceData, error) {
	return r.Get(ctx, title)
}

// ListWithOptions retrieves all sessions with selective child data loading.
// EntRepository: Delegates to List with full loading.
func (r *EntRepository) ListWithOptions(ctx context.Context, options LoadOptions) ([]InstanceData, error) {
	return r.List(ctx)
}

// ListByStatusWithOptions retrieves sessions filtered by status with selective loading.
// EntRepository: Delegates to ListByStatus with full loading.
func (r *EntRepository) ListByStatusWithOptions(ctx context.Context, status Status, options LoadOptions) ([]InstanceData, error) {
	return r.ListByStatus(ctx, status)
}

// ListByTagWithOptions retrieves sessions with a specific tag with selective loading.
// EntRepository: Delegates to ListByTag with full loading.
func (r *EntRepository) ListByTagWithOptions(ctx context.Context, tag string, options LoadOptions) ([]InstanceData, error) {
	return r.ListByTag(ctx, tag)
}
