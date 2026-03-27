package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for session counting
	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DatabaseService implements the database/workspace switcher RPC methods.
// It is stateless: each call reads the filesystem directly.
type DatabaseService struct{}

// NewDatabaseService creates a DatabaseService.
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{}
}

// ListDatabases returns all discovered workspace databases with metadata.
func (ds *DatabaseService) ListDatabases(
	ctx context.Context,
	req *connect.Request[sessionv1.ListDatabasesRequest],
) (*connect.Response[sessionv1.ListDatabasesResponse], error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get home directory: %w", err))
	}
	baseDir := filepath.Join(homeDir, ".stapler-squad")

	workspaces, err := config.ListAvailableWorkspaces(baseDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list workspaces: %w", err))
	}

	currentDir, err := config.GetConfigDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current config dir: %w", err))
	}

	databases := make([]*sessionv1.DatabaseInfo, 0, len(workspaces))
	currentWorkspaceID := ""

	for _, ws := range workspaces {
		sessionCount := countSessionsInDB(ws.ConfigDir)
		isCurrent := ws.ConfigDir == currentDir
		if isCurrent {
			currentWorkspaceID = ws.WorkspaceID
		}

		var lastUsedProto *timestamppb.Timestamp
		if !ws.LastUsed.IsZero() {
			lastUsedProto = timestamppb.New(ws.LastUsed)
		}

		databases = append(databases, &sessionv1.DatabaseInfo{
			WorkspaceId:  ws.WorkspaceID,
			Type:         ws.Type,
			Cwd:          ws.CWD,
			Name:         ws.Name,
			ConfigDir:    ws.ConfigDir,
			SessionCount: int32(sessionCount),
			IsCurrent:    isCurrent,
			LastUsed:     lastUsedProto,
		})
	}

	return connect.NewResponse(&sessionv1.ListDatabasesResponse{
		Databases:          databases,
		CurrentWorkspaceId: currentWorkspaceID,
	}), nil
}

// GetCurrentDatabase returns metadata for the currently active workspace database.
func (ds *DatabaseService) GetCurrentDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCurrentDatabaseRequest],
) (*connect.Response[sessionv1.GetCurrentDatabaseResponse], error) {
	currentDir, err := config.GetConfigDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current config dir: %w", err))
	}

	meta, err := config.ReadWorkspaceMeta(currentDir)
	if err != nil {
		// Workspace meta may not exist for legacy workspaces; build a minimal response.
		meta = config.WorkspaceMeta{
			WorkspaceID: filepath.Base(currentDir),
			Type:        "workspace",
			ConfigDir:   currentDir,
			Name:        filepath.Base(currentDir),
		}
	}

	sessionCount := countSessionsInDB(currentDir)

	var lastUsedProto *timestamppb.Timestamp
	if !meta.LastUsed.IsZero() {
		lastUsedProto = timestamppb.New(meta.LastUsed)
	}

	return connect.NewResponse(&sessionv1.GetCurrentDatabaseResponse{
		Database: &sessionv1.DatabaseInfo{
			WorkspaceId:  meta.WorkspaceID,
			Type:         meta.Type,
			Cwd:          meta.CWD,
			Name:         meta.Name,
			ConfigDir:    meta.ConfigDir,
			SessionCount: int32(sessionCount),
			IsCurrent:    true,
			LastUsed:     lastUsedProto,
		},
	}), nil
}

// SwitchDatabase writes a preference file and triggers an exec-based server self-restart.
// The client should poll until the server is back up, then reload the page.
func (ds *DatabaseService) SwitchDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchDatabaseRequest],
) (*connect.Response[sessionv1.SwitchDatabaseResponse], error) {
	targetDir := strings.TrimSpace(req.Msg.ConfigDir)
	if targetDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config_dir is required"))
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get home directory: %w", err))
	}
	baseDir := filepath.Join(homeDir, ".stapler-squad")

	// Security: target must be under baseDir
	if targetDir != baseDir && !strings.HasPrefix(targetDir, baseDir+string(filepath.Separator)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target config_dir must be under %s", baseDir))
	}

	// Verify target directory exists
	if _, err := os.Stat(targetDir); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("target workspace directory not found: %s", targetDir))
	}

	// Write preference file
	if err := config.SetPreferredWorkspace(baseDir, targetDir); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write workspace preference: %w", err))
	}

	// Schedule exec-restart after a short delay so the response can be sent first
	go func() {
		time.Sleep(150 * time.Millisecond)
		execRestart()
	}()

	return connect.NewResponse(&sessionv1.SwitchDatabaseResponse{
		Success: true,
		Message: "Switching workspace, server restarting...",
	}), nil
}

// MergeDatabase copies sessions from a source workspace into the current one.
// Uses INSERT OR IGNORE so existing sessions (matched by title) are never overwritten.
func (ds *DatabaseService) MergeDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.MergeDatabaseRequest],
) (*connect.Response[sessionv1.MergeDatabaseResponse], error) {
	sourceDir := strings.TrimSpace(req.Msg.ConfigDir)
	if sourceDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config_dir is required"))
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get home directory: %w", err))
	}
	baseDir := filepath.Join(homeDir, ".stapler-squad")

	// Security: source must be under baseDir
	if sourceDir != baseDir && !strings.HasPrefix(sourceDir, baseDir+string(filepath.Separator)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source config_dir must be under %s", baseDir))
	}

	sourceDB := filepath.Join(sourceDir, "sessions.db")
	if _, err := os.Stat(sourceDB); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("source database not found: %s", sourceDB))
	}

	currentDir, err := config.GetConfigDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get current config dir: %w", err))
	}

	if sourceDir == currentDir {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source and current workspace are the same"))
	}

	destDB := filepath.Join(currentDir, "sessions.db")

	imported, skipped, err := mergeSessions(ctx, destDB, sourceDB)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("merge failed: %w", err))
	}

	msg := fmt.Sprintf("Merged %d sessions (%d skipped — title already exists)", imported, skipped)
	log.InfoLog.Printf("MergeDatabase: %s from %s", msg, sourceDir)

	return connect.NewResponse(&sessionv1.MergeDatabaseResponse{
		Success:          true,
		Message:          msg,
		SessionsImported: int32(imported),
		SessionsSkipped:  int32(skipped),
	}), nil
}

// mergeSessions copies sessions from sourceDB into destDB.
// Returns (imported, skipped, error).
func mergeSessions(ctx context.Context, destDB, sourceDB string) (int, int, error) {
	db, err := sql.Open("sqlite3", destDB)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open destination database: %w", err)
	}
	defer db.Close()

	// Count sessions before merge
	var before int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&before); err != nil {
		return 0, 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	// Count sessions in source
	var sourceTotal int
	{
		src, err := sql.Open("sqlite3", sourceDB+"?mode=ro")
		if err != nil {
			return 0, 0, fmt.Errorf("failed to open source database: %w", err)
		}
		_ = src.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sourceTotal)
		src.Close()
	}

	// Run migration via ATTACH
	mergeSQL := fmt.Sprintf(`
PRAGMA foreign_keys = OFF;
ATTACH '%s' AS src;

BEGIN;

INSERT OR IGNORE INTO main.sessions
  (title, path, working_dir, branch, status, height, width,
   created_at, updated_at, auto_yes, prompt, program,
   existing_worktree, category, is_expanded, session_type, tmux_prefix,
   last_terminal_update, last_meaningful_output, last_output_signature,
   last_added_to_queue, last_viewed, last_acknowledged)
SELECT
  title, path, working_dir, branch, status, height, width,
  created_at, updated_at, auto_yes, prompt, program,
  existing_worktree, category, is_expanded, session_type, tmux_prefix,
  last_terminal_update, last_meaningful_output, last_output_signature,
  last_added_to_queue, last_viewed, last_acknowledged
FROM src.sessions;

INSERT OR IGNORE INTO main.worktrees
  (repo_path, worktree_path, session_name, branch_name, base_commit_sha, session_worktree)
SELECT lw.repo_path, lw.worktree_path, lw.session_name, lw.branch_name, lw.base_commit_sha, ms.id
FROM src.worktrees lw
JOIN src.sessions ls ON lw.session_worktree = ls.id
JOIN main.sessions ms ON ms.title = ls.title;

INSERT OR IGNORE INTO main.diff_stats
  (added, removed, content, session_diff_stats)
SELECT ld.added, ld.removed, ld.content, ms.id
FROM src.diff_stats ld
JOIN src.sessions ls ON ld.session_diff_stats = ls.id
JOIN main.sessions ms ON ms.title = ls.title;

INSERT OR IGNORE INTO main.tags (name)
SELECT name FROM src.tags;

INSERT OR IGNORE INTO main.claude_sessions
  (claude_session_id, conversation_id, project_name, last_attached,
   auto_reattach, preferred_session_name, create_new_on_missing,
   show_session_selector, session_timeout_minutes, session_claude_session)
SELECT lcs.claude_session_id, lcs.conversation_id, lcs.project_name, lcs.last_attached,
  lcs.auto_reattach, lcs.preferred_session_name, lcs.create_new_on_missing,
  lcs.show_session_selector, lcs.session_timeout_minutes, ms.id
FROM src.claude_sessions lcs
JOIN src.sessions ls ON lcs.session_claude_session = ls.id
JOIN main.sessions ms ON ms.title = ls.title;

INSERT OR IGNORE INTO main.session_tags (session_id, tag_id)
SELECT ms.id, mt.id
FROM src.session_tags lst
JOIN src.sessions ls ON lst.session_id = ls.id
JOIN src.tags lt ON lst.tag_id = lt.id
JOIN main.sessions ms ON ms.title = ls.title
JOIN main.tags mt ON mt.name = lt.name;

COMMIT;
PRAGMA foreign_keys = ON;
`, sourceDB)

	if _, err := db.ExecContext(ctx, mergeSQL); err != nil {
		return 0, 0, fmt.Errorf("merge SQL failed: %w", err)
	}

	var after int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&after); err != nil {
		return 0, 0, fmt.Errorf("failed to count sessions after merge: %w", err)
	}

	imported := after - before
	skipped := sourceTotal - imported
	return imported, skipped, nil
}

// countSessionsInDB opens the sessions.db in configDir and returns the session count.
// Returns 0 on any error (the count is informational only).
func countSessionsInDB(configDir string) int {
	dbPath := filepath.Join(configDir, "sessions.db")
	if _, err := os.Stat(dbPath); err != nil {
		return 0
	}

	db, err := sql.Open("sqlite3", dbPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return 0
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		return 0
	}
	return count
}

// execRestart replaces the current process image with a fresh server instance.
// Uses syscall.Exec on Unix so the new process inherits the same PID.
func execRestart() {
	executable, err := os.Executable()
	if err != nil {
		log.ErrorLog.Printf("exec restart: failed to get executable path: %v", err)
		return
	}

	log.InfoLog.Printf("exec restart: replacing process with %s", executable)
	if err := execSyscall(executable, os.Args, os.Environ()); err != nil {
		log.ErrorLog.Printf("exec restart: exec failed: %v", err)
	}
}
