package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// mcpAwaitTerminalTimeout bounds create_session/create_session_for_pr's wait
// for the Background Resolution Pipeline to reach a terminal status
// (async-session-creation Epic 2.3). Today's MCP path was unbounded before
// this change (inst.Start(true) ran with no context deadline threaded into
// its GitHub/worktree/tmux calls at all) -- 150s is therefore a new,
// deliberately chosen cap, picked to align with the frontend's own SLO
// expectation for CreateSession (createSessionTimeout in
// server/services/session_service.go), not a preservation of a prior bound.
// Intentionally shorter than the pipeline's own maxCreationResolutionTimeout
// (10 min) so a genuinely slow pipeline degrades to the "still creating"
// result (see mapCreationOutcome) rather than holding the MCP tool call open
// for the pipeline's full budget.
const mcpAwaitTerminalTimeout = 150 * time.Second

type lifecycleHandlers struct {
	store session.InstanceStore
	svc   *services.SessionService
}

// CreateSessionResult is returned by create_session.
type CreateSessionResult struct {
	MCPResult
	Session            *SessionDetail `json:"session,omitempty"`
	MCPInjectionFailed bool           `json:"mcp_injection_failed,omitempty"`
	// StillCreating is true when mcpAwaitTerminalTimeout elapsed before the
	// Background Resolution Pipeline reached a terminal status. Session is
	// still returned (Creating status, ID valid) — the session was
	// successfully created and is genuinely still resolving, not failed or
	// broken. Use get_session to check on it.
	StillCreating bool `json:"still_creating,omitempty"`
}

func registerLifecycleTools(s *mcpserver.MCPServer, lh *lifecycleHandlers) {
	s.AddTool(
		mcpgo.NewTool("create_session",
			mcpgo.WithDescription("Create and start a new Stapler Squad session (tmux + optional git worktree). By default, injects this MCP server into the child session's .claude/settings.local.json so the new session can use all Stapler Squad tools. Waits up to 150s for the session to finish starting up; if it's still resolving after that, returns a still_creating result (session_id present, safe to poll with get_session) rather than an error or an open-ended hang. Rate-limited to 3 per minute.\n\nNOTE: Do not use this tool just to run commands or execute tasks — spawn an Agent subagent instead. Reserve create_session for cases where a USER INTERACTABLE, persistent tmux session is genuinely needed (e.g. long-running background work, multi-turn Claude Code sessions the user will actively monitor or control)."),
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
	return lh.createSessionWithAwaitTimeout(ctx, req, mcpAwaitTerminalTimeout)
}

// createSessionWithAwaitTimeout is create_session's real implementation,
// parameterized on the AwaitCreationTerminal timeout. The registered tool
// handler (createSession, above) always uses mcpAwaitTerminalTimeout; tests
// call this directly with a much shorter timeout to exercise the
// still_creating outcome (Story 2.3.4) without waiting out the real 150s
// bound — the same interval-parameterized-implementation seam
// AwaitCreationTerminal itself uses (server/services/session_creation_await.go).
func (lh *lifecycleHandlers) createSessionWithAwaitTimeout(ctx context.Context, req mcpgo.CallToolRequest, awaitTimeout time.Duration) (*mcpgo.CallToolResult, error) {
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

	protoSessionType, typeErr := mcpSessionTypeToProto(sessionTypeStr)
	if typeErr != nil {
		return errResult(ErrInvalidArgument, typeErr.Error(),
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
	tags = append(tags, "source:mcp")

	// Check for title collision before starting. Use ListInstanceData (raw DB read)
	// rather than LoadInstances to avoid spawning PTY processes as a side effect.
	// This is a fast, agent-friendly pre-check in addition to CreateSession's own
	// synchronous title-uniqueness check below -- a TOCTOU race between the two
	// is not a correctness gap, since CreateSession still authoritatively rejects
	// a duplicate that slips past this pre-check (async-session-creation Epic
	// 2.3, Story 2.3.1).
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

	createResp, createErr := lh.svc.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       title,
		Path:        path,
		Branch:      branch,
		Program:     program,
		SessionType: protoSessionType,
		// Tags (Task 2.3.1c) are applied at construction time, synchronously, rather than via a
		// post-hoc SetTags call after AwaitCreationTerminal succeeds -- a pipeline resolution
		// that outlasts awaitTimeout (StillCreating path below) would otherwise silently never
		// get these tags applied.
		Tags: tags,
	}))
	if createErr != nil {
		return mapCreateSessionRPCError(createErr, title), nil
	}
	sessionID := createResp.Msg.Session.Id

	outcome, awaitErr := lh.svc.AwaitCreationTerminal(ctx, sessionID, awaitTimeout)
	if errors.Is(awaitErr, services.ErrCreationAwaitTimeout) {
		return okResult(CreateSessionResult{
			MCPResult: MCPResult{Success: true},
			Session: &SessionDetail{SessionSummary: SessionSummary{
				ID:     sessionID,
				Title:  title,
				Status: session.Creating.String(),
			}},
			StillCreating: true,
		}), nil
	}
	if result := mapCreationOutcome(outcome, awaitErr); result != nil {
		return result, nil
	}

	// The pipeline reached Active: the worktree/tmux state MCP/hook injection
	// depend on now exists. Re-fetch the live instance rather than reusing a
	// stale local reference -- there isn't one; this handler never constructs
	// an *Instance itself anymore (Epic 2.1/2.2's CreateSession does).
	inst := lh.svc.FindLiveInstance(sessionID)
	if inst == nil {
		return errResult(ErrInternalError,
			fmt.Sprintf("session %q reached Active but is no longer findable", sessionID), ""), nil
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

	// Persist the tag/injection changes above. The instance is Started() now
	// (Active), so SaveInstances (unlike a Creating-status row) will not
	// silently no-op this write.
	if err := lh.store.SaveInstances([]*session.Instance{inst}); err != nil {
		log.Error("mcp save instance failed after session creation", "title", title, "err", err)
		return errResult(ErrInternalError, fmt.Sprintf("save session: %v", err), ""), nil
	}

	detail := instanceToDetail(inst)
	return okResult(CreateSessionResult{
		MCPResult:          MCPResult{Success: true},
		Session:            &detail,
		MCPInjectionFailed: mcpInjectionFailed,
	}), nil
}

// mcpSessionTypeToProto maps create_session/create_session_for_pr's MCP-level
// session_type string onto the proto SessionType enum CreateSession's request
// takes. Mirrors the string set the old inline session.SessionType switch
// validated (server/adapters.sessionTypeToProto performs the equivalent
// mapping for the *response* direction but is unexported outside its
// package, so this is a small, deliberate duplicate of that switch's cases
// for the request direction).
func mcpSessionTypeToProto(sessionTypeStr string) (sessionv1.SessionType, error) {
	switch sessionTypeStr {
	case "new_worktree":
		return sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE, nil
	case "existing_worktree":
		return sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE, nil
	case "directory", "":
		return sessionv1.SessionType_SESSION_TYPE_DIRECTORY, nil
	default:
		return 0, fmt.Errorf("invalid session_type %q", sessionTypeStr)
	}
}

// mapCreateSessionRPCError maps a synchronous CreateSession RPC error onto the
// errResult codes MCP callers see today. Defense-in-depth for the TOCTOU
// window the title-collision pre-check leaves open, not the primary error
// path -- the MCP-level pre-checks above already catch the common cases
// before CreateSession is ever called (async-session-creation Epic 2.3, Task
// 2.3.1b).
func mapCreateSessionRPCError(err error, title string) *mcpgo.CallToolResult {
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return errResult(ErrInternalError, fmt.Sprintf("create session: %v", err), "")
	}
	switch connectErr.Code() {
	case connect.CodeAlreadyExists:
		return errResult(ErrInvalidArgument, fmt.Sprintf("session with title %q already exists", title),
			"Choose a different title.")
	case connect.CodeNotFound:
		return errResult(ErrInvalidArgument, connectErr.Message(), "")
	case connect.CodeInvalidArgument:
		return errResult(ErrInvalidArgument, connectErr.Message(), "")
	default:
		return errResult(ErrInternalError, fmt.Sprintf("create session: %v", connectErr.Message()), "")
	}
}

// mapCreationOutcome maps AwaitCreationTerminal's Failed/vanished/caller-ctx
// exits onto the MCP result shape (async-session-creation Epic 2.3, Story
// 2.3.4). The timeout exit (services.ErrCreationAwaitTimeout) is handled by
// each caller directly, before calling this, since its "still creating"
// result carries a tool-specific result type (CreateSessionResult vs
// CreateSessionForPRResult) that this shared helper can't build generically.
//
// Consumes only the CreationOutcome snapshot or sentinel error
// AwaitCreationTerminal returned -- never a second, separate read of
// Status()/FailureReason() off the actor (see CreationOutcome's doc comment
// for why a second read would be unsafe).
//
// Returns nil when the pipeline succeeded (outcome.Status is Active/Running)
// -- the caller should proceed with post-creation steps. Returns a non-nil
// error result for every other case: Failed, vanished (cancelled), or the
// caller's own ctx ending first.
func mapCreationOutcome(outcome services.CreationOutcome, err error) *mcpgo.CallToolResult {
	switch {
	case err == nil:
		if outcome.Status == session.Failed {
			return errResult(ErrSessionCreationFailed,
				fmt.Sprintf("session creation failed: %s", outcome.FailureReason),
				"Use get_session to inspect the session, or call create_session again to retry.")
		}
		return nil // Active/Running: proceed with post-creation steps.
	case errors.Is(err, services.ErrCreationVanished):
		return errResult(ErrSessionCreationCancelled,
			"session creation was cancelled while waiting for it to complete",
			"The session no longer exists; call create_session again if you still need it.")
	default:
		return errResult(ErrSessionCreationAwaitCanceled,
			"the wait for session creation was ended by the caller's own request context (not a pipeline timeout or failure); the session may still be resolving",
			"Use get_session to check on its status.")
	}
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

	// MCP tool pause is always user-initiated — record as manual.
	inst.PauseReason = session.PauseReasonManual
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

	inst.PauseReason = ""
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
