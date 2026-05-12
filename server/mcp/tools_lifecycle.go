package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type lifecycleHandlers struct {
	store session.InstanceStore
	svc   *services.SessionService
}

// CreateSessionResult is returned by create_session.
type CreateSessionResult struct {
	MCPResult
	Session            *SessionDetail `json:"session,omitempty"`
	MCPInjectionFailed bool           `json:"mcp_injection_failed,omitempty"`
}

func registerLifecycleTools(s *mcpserver.MCPServer, lh *lifecycleHandlers) {
	s.AddTool(
		mcpgo.NewTool("create_session",
			mcpgo.WithDescription("Create and start a new Stapler Squad session (tmux + optional git worktree). Returns the new session. Rate-limited to 3 per minute."),
			mcpgo.WithString("title", mcpgo.Description("Unique name for the session"), mcpgo.Required()),
			mcpgo.WithString("path", mcpgo.Description("Absolute path to the repository root"), mcpgo.Required()),
			mcpgo.WithString("branch", mcpgo.Description("Git branch name (creates if missing; required for new_worktree session type)")),
			mcpgo.WithString("program", mcpgo.Description("Program to run: claude or aider (default: claude)"), mcpgo.Enum("claude", "aider")),
			mcpgo.WithString("session_type", mcpgo.Description("Session type: directory, new_worktree, existing_worktree (default: directory)"),
				mcpgo.Enum("directory", "new_worktree", "existing_worktree")),
			mcpgo.WithArray("tags", mcpgo.Description("Tags for organizing the session")),
			mcpgo.WithBoolean("inject_mcp", mcpgo.Description("Inject MCP server config into session's .claude/settings.local.json (default true)"),
				mcpgo.DefaultBool(true)),
			mcpgo.WithArray("hooks", mcpgo.Description("Built-in hook names to inject (default: [permission_approval, stop_notification])")),
		),
		lh.createSession,
	)

	s.AddTool(
		mcpgo.NewTool("pause_session",
			mcpgo.WithDescription("Pause a running session. Commits uncommitted changes, removes git worktree (preserving branch), and stops the tmux process."),
			mcpgo.WithString("session_id", mcpgo.Description("Session ID (title) to pause"), mcpgo.Required()),
		),
		lh.pauseSession,
	)

	s.AddTool(
		mcpgo.NewTool("resume_session",
			mcpgo.WithDescription("Resume a paused session. Recreates the git worktree and restarts the tmux session."),
			mcpgo.WithString("session_id", mcpgo.Description("Session ID (title) to resume"), mcpgo.Required()),
		),
		lh.resumeSession,
	)

	s.AddTool(
		mcpgo.NewTool("stop_session",
			mcpgo.WithDescription("Stop and permanently destroy a session, removing its tmux process and git worktree. Irreversible — requires confirm=true."),
			mcpgo.WithString("session_id", mcpgo.Description("Session ID (title) to stop"), mcpgo.Required()),
			mcpgo.WithBoolean("confirm", mcpgo.Description("Must be true to confirm destruction of the session"), mcpgo.Required()),
		),
		lh.stopSession,
	)

	s.AddTool(
		mcpgo.NewTool("update_session",
			mcpgo.WithDescription("Update session metadata (title, tags, category) or toggle MCP injection. Does not change session status."),
			mcpgo.WithString("session_id", mcpgo.Description("Session ID (title) to update"), mcpgo.Required()),
			mcpgo.WithString("title", mcpgo.Description("New title for the session")),
			mcpgo.WithArray("tags", mcpgo.Description("Replace session tags")),
			mcpgo.WithString("category", mcpgo.Description("Session category")),
			mcpgo.WithBoolean("inject_mcp", mcpgo.Description("Inject MCP config into the session")),
			mcpgo.WithBoolean("remove_mcp", mcpgo.Description("Remove MCP config from the session")),
		),
		lh.updateSession,
	)
}

// lifecycleResult wraps a session state change response.
type lifecycleResult struct {
	MCPResult
	Session *SessionDetail `json:"session,omitempty"`
}

func (lh *lifecycleHandlers) createSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if !createSessionLimiter.allow("global") {
		return errResult(ErrRateLimitExceeded, "create_session rate limit exceeded (max 3 per minute)",
			"Wait before creating another session."), nil
	}

	args := req.GetArguments()
	title, _ := args["title"].(string)
	path, _ := args["path"].(string)
	branch, _ := args["branch"].(string)
	program, _ := args["program"].(string)
	sessionTypeStr, _ := args["session_type"].(string)

	if title == "" {
		return errResult(ErrInvalidArgument, "title is required", ""), nil
	}
	if path == "" {
		return errResult(ErrInvalidArgument, "path is required", "Provide the absolute path to the repository root."), nil
	}

	// Path traversal defense.
	if !filepath.IsAbs(path) || strings.Contains(path, "..") {
		return errResult(ErrInvalidPath, "path must be absolute and must not contain '..' components", ""), nil
	}
	if _, err := os.Stat(path); err != nil {
		return errResult(ErrInvalidPath, fmt.Sprintf("path does not exist: %v", err), ""), nil
	}

	if program == "" {
		program = "claude"
	}

	var sessionType session.SessionType
	switch sessionTypeStr {
	case "new_worktree":
		sessionType = session.SessionTypeNewWorktree
	case "existing_worktree":
		sessionType = session.SessionTypeExistingWorktree
	case "directory", "":
		sessionType = session.SessionTypeDirectory
	default:
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid session_type %q", sessionTypeStr),
			"Valid values: directory, new_worktree, existing_worktree"), nil
	}

	var tags []string
	if raw, ok := args["tags"]; ok {
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					tags = append(tags, s)
				}
			}
		}
	}

	// Check for title collision before starting. Use ListInstanceData (raw DB read)
	// rather than LoadInstances to avoid spawning PTY processes as a side effect.
	existing, err := lh.store.ListInstanceData()
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
	}
	for _, data := range existing {
		if data.Title == title {
			return errResult(ErrInvalidArgument, fmt.Sprintf("session with title %q already exists", title),
				"Choose a different title."), nil
		}
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:       title,
		Path:        path,
		Branch:      branch,
		Program:     program,
		SessionType: sessionType,
		Tags:        tags,
	})
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("create session: %v", err), ""), nil
	}

	if err := inst.Start(true); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("start session: %v", err), ""), nil
	}

	// MCP injection: write our server config into the session's .claude/settings.local.json.
	// inject_mcp defaults to true when not explicitly provided.
	shouldInjectMCP := true
	if v, ok := args["inject_mcp"].(bool); ok {
		shouldInjectMCP = v
	}
	var mcpInjectionFailed bool
	if shouldInjectMCP {
		if injErr := injectMCPConfig(inst.GetEffectiveRootDir()); injErr != nil {
			log.Warn("mcp MCP injection failed for session", "title", title, "err", injErr)
			mcpInjectionFailed = true
		}
	}

	// Hook injection: inject permission_approval hook (always) + any requested hooks.
	var hookNames []services.HookName
	if rawHooks, ok := args["hooks"]; ok {
		if arr, ok := rawHooks.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					hookNames = append(hookNames, services.HookName(s))
				}
			}
		}
	}
	if err := services.InjectHooksConfig(inst.GetEffectiveRootDir(), inst.Title, hookNames); err != nil {
		log.Warn("mcp hook injection failed for session", "title", title, "err", err)
	}

	// Save to storage. AddInstance persists just this new instance.
	if err := lh.store.AddInstance(inst); err != nil {
		// Best-effort cleanup; don't fail the session just because save failed.
		log.Error("mcp save instance failed after creating session", "title", title, "err", err)
		return errResult(ErrInternalError, fmt.Sprintf("save session: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(CreateSessionResult{
		MCPResult:          MCPResult{Success: true},
		Session:            &detail,
		MCPInjectionFailed: mcpInjectionFailed,
	}), nil
}

// injectMCPConfig writes the MCP server entry into <rootDir>/.claude/settings.local.json.
// Uses os.Executable() for the binary path so it survives PATH changes.
// Non-fatal: caller should log the error and continue.
func injectMCPConfig(rootDir string) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	return services.InjectMCPConfig(rootDir, binaryPath)
}

func (lh *lifecycleHandlers) pauseSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	inst, findErr := lh.findAndHydrate(sessionID)
	if findErr != nil {
		return findErr, nil
	}

	if inst.Status == session.Paused {
		return errResult("SESSION_ALREADY_PAUSED", fmt.Sprintf("session %q is already paused", sessionID), ""), nil
	}

	if err := inst.Pause(); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("pause session: %v", err), ""), nil
	}

	if err := lh.store.SaveInstances([]*session.Instance{inst}); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("save: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(lifecycleResult{MCPResult: MCPResult{Success: true}, Session: &detail}), nil
}

func (lh *lifecycleHandlers) resumeSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	inst, findErr := lh.findAndHydrate(sessionID)
	if findErr != nil {
		return findErr, nil
	}

	if inst.Status != session.Paused {
		return errResult(ErrInvalidStatusTrans,
			fmt.Sprintf("session %q is not paused (current status: %s)", sessionID, inst.Status),
			"Only paused sessions can be resumed."), nil
	}

	if err := inst.Resume(); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("resume session: %v", err), ""), nil
	}

	if err := lh.store.SaveInstances([]*session.Instance{inst}); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("save: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(lifecycleResult{MCPResult: MCPResult{Success: true}, Session: &detail}), nil
}

func (lh *lifecycleHandlers) stopSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	confirm, _ := args["confirm"].(bool)
	if !confirm {
		return errResult(ErrConfirmationRequired,
			"Stopping a session removes its tmux process and git worktree. Pass confirm=true to proceed.",
			"Call stop_session again with confirm=true to confirm."), nil
	}

	// Prefer the live in-memory instance from the poller to avoid spawning a new PTY.
	inst := lh.svc.FindLiveInstance(sessionID)
	if inst == nil {
		// Not in poller — verify it exists in storage before attempting destroy.
		existing, err := lh.store.ListInstanceData()
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
		}
		var found bool
		for _, data := range existing {
			if data.Title == sessionID || data.UUID == sessionID {
				found = true
				break
			}
		}
		if !found {
			return errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID),
				"Use list_sessions or search_sessions to find valid session IDs."), nil
		}
		// Load just this one session so we can call Destroy() for tmux cleanup.
		instances, err := lh.store.LoadInstances()
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
		}
		for _, candidate := range instances {
			if candidate.MatchesID(sessionID) {
				inst = candidate
				break
			}
		}
	}

	if inst != nil {
		// Hydrate for tmux access if the session is not paused (paused sessions have no tmux session).
		if inst.Status != session.Paused && !inst.Started() {
			if startErr := inst.Start(false); startErr != nil {
				log.Warn("mcp hydration failed for stop, attempting destroy anyway", "session", sessionID, "err", startErr)
			}
		}
		if err := inst.Destroy(); err != nil {
			log.Warn("mcp destroy session had errors", "session", sessionID, "err", err)
		}
	}

	// Resolve sessionID (which may be a UUID) to the canonical title required by
	// pollers and storage, which are keyed by title.
	sessionTitle := sessionID
	if inst != nil {
		sessionTitle = inst.Title
	}

	// Remove from all pollers BEFORE storage deletion to close the race window
	// where external discovery could re-add the session between delete and poller update.
	lh.svc.RemoveFromAllPollers(sessionTitle)

	if err := lh.store.DeleteInstance(sessionTitle); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("delete from storage: %v", err), ""), nil
	}

	return okResult(MCPResult{Success: true}), nil
}

func (lh *lifecycleHandlers) updateSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	// Prefer the live in-memory instance so we don't spawn a new PTY as a side effect
	// of FromInstanceData. Fall back to LoadInstances only when the poller is not wired.
	inst := lh.svc.FindLiveInstance(sessionID)
	if inst == nil {
		instances, err := lh.store.LoadInstances()
		if err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
		}
		for _, candidate := range instances {
			if candidate.MatchesID(sessionID) {
				inst = candidate
				break
			}
		}
	}
	if inst == nil {
		return errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID),
			"Use list_sessions or search_sessions to find valid session IDs."), nil
	}

	if title, ok := args["title"].(string); ok && title != "" {
		inst.Title = title
	}
	if rawTags, ok := args["tags"]; ok {
		var tags []string
		if arr, ok := rawTags.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					tags = append(tags, s)
				}
			}
		}
		if err := inst.SetTags(tags); err != nil {
			return errResult(ErrInternalError, fmt.Sprintf("set tags: %v", err), ""), nil
		}
	}
	if cat, ok := args["category"].(string); ok {
		inst.Category = cat
	}

	// MCP injection toggle on existing session.
	if injectMCP, ok := args["inject_mcp"].(bool); ok && injectMCP {
		if injErr := injectMCPConfig(inst.GetEffectiveRootDir()); injErr != nil {
			log.Warn("mcp update MCP injection failed", "session", sessionID, "err", injErr)
		}
	}
	if removeMCP, ok := args["remove_mcp"].(bool); ok && removeMCP {
		if rmErr := services.RemoveMCPConfig(inst.GetEffectiveRootDir()); rmErr != nil {
			log.Warn("mcp update MCP removal failed", "session", sessionID, "err", rmErr)
		}
	}

	if err := lh.store.SaveInstances([]*session.Instance{inst}); err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("save: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(lifecycleResult{MCPResult: MCPResult{Success: true}, Session: &detail}), nil
}

// findAndHydrate returns the live in-memory instance for sessionID, ensuring it is
// connected to its tmux session (hydrated). It prefers the live instance from the
// ReviewQueuePoller to avoid spawning PTY processes as a side effect. Falls back to
// LoadInstances() when the poller is not wired or the session is not in the poller.
// Returns a non-nil *mcpgo.CallToolResult (error result) when not found or not started.
func (lh *lifecycleHandlers) findAndHydrate(sessionID string) (*session.Instance, *mcpgo.CallToolResult) {
	// Try the live poller instance first — avoids LoadInstances() side effects.
	if inst := lh.svc.FindLiveInstance(sessionID); inst != nil {
		if !inst.Started() && inst.Status != session.Paused {
			if startErr := inst.Start(false); startErr != nil {
				return nil, errResult(ErrInternalError,
					fmt.Sprintf("hydrate session %q: %v", sessionID, startErr), "")
			}
		}
		return inst, nil
	}

	// Poller doesn't have the session (poller not wired, or external/untracked session).
	instances, err := lh.store.LoadInstances()
	if err != nil {
		return nil, errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), "")
	}
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			if !inst.Started() && inst.Status != session.Paused {
				if startErr := inst.Start(false); startErr != nil {
					return nil, errResult(ErrInternalError,
						fmt.Sprintf("hydrate session %q: %v", sessionID, startErr), "")
				}
			}
			return inst, nil
		}
	}

	return nil, errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID),
		"Use list_sessions or search_sessions to find valid session IDs.")
}
