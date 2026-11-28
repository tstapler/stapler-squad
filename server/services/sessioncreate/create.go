package sessioncreate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/claudehistory"
	"github.com/tstapler/stapler-squad/session/domain"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/importer"
	"github.com/tstapler/stapler-squad/session/namegen"
)

// CreateSessionTimeout bounds the synchronous portion of CreateSession (fast-fail
// validation, config/remote/restart-source resolution, alias-existence check,
// title-uniqueness check, and instance construction/persist/publish). As of
// Epic 2.1 (async-session-creation), GitHub URL resolution (`git clone`, up to
// ~120s for large repos per research) no longer runs on this synchronous path
// -- it is deferred to the Background Resolution Pipeline (Epic 2.2, see
// RunResolutionPipeline in pipeline.go and its
// own DefaultResolutionTimeout), same as the tmux startup poll (~10s)
// that already ran there. This value stays generous rather than being
// tightened immediately, since other synchronous sub-operations (e.g. remote
// SSH dial/worktree setup) can still be slow.
const CreateSessionTimeout = 150 * time.Second

// resumeIDRe validates the client-supplied resume_id field: must be a standard UUID.
var resumeIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// resolveRestartSource resolves req.RestartFromSessionId (Story 2.3.1) to the
// source session's path, enforcing the still-live guard. FindLiveInstance is
// tried first (it reflects the actual live, in-memory session state -- a
// still-running tmux/process, not just what was last persisted); a
// persisted-storage lookup against existing (mirroring FindInstanceDataByID,
// reusing the slice CreateSession already loaded for its title-collision
// check) is the fallback for a source session that exists but is not
// currently live. Only the "found live, not confirmed" case is rejected
// outright -- a persisted-but-not-live source proceeds without requiring
// ConfirmRestartWithLiveSource, since there is no live process/worktree it
// could collide with. Returns "" with a nil error when RestartFromSessionId
// is unset. Errors are already-wrapped *connect.Error values, ready to
// return directly from CreateSession.
func (h *Handler) resolveRestartSource(req *sessionv1.CreateSessionRequest, existing []domain.InstanceData) (string, error) {
	if req.RestartFromSessionId == "" {
		return "", nil
	}

	if liveSrc := h.d.FindLive(req.RestartFromSessionId); liveSrc != nil {
		if !req.ConfirmRestartWithLiveSource {
			return "", connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("restart source session %q is still running; stop it first or pass confirm_restart_with_live_source to proceed anyway", req.RestartFromSessionId))
		}
		return liveSrc.Path, nil
	}

	for i := range existing {
		if existing[i].MatchesID(req.RestartFromSessionId) {
			return existing[i].Path, nil
		}
	}
	return "", connect.NewError(connect.CodeNotFound,
		fmt.Errorf("restart source session %q not found", req.RestartFromSessionId))
}

// requiresExplicitPath reports whether msg.Path being empty is a validation
// error for this request. Extracted from CreateSession's inline check so it
// can be tested directly (pure function, no I/O) rather than only through a
// full CreateSession -> tmux -> SessionDriver round trip -- see
// TestRequiresExplicitPath.
func requiresExplicitPath(msg *sessionv1.CreateSessionRequest) bool {
	return msg.SessionType != sessionv1.SessionType_SESSION_TYPE_ONE_OFF &&
		// AutonomousMode: the omnibar always submits an empty path for autonomous
		// sessions; see CreateSession's directory-generation block.
		!msg.AutonomousMode &&
		msg.AliasName == "" &&
		msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT &&
		// restart_from_session_id (Story 2.3.1) derives the path from the source
		// session in CreateSession when Path is left empty -- see the
		// restart-source resolution block ahead of "Resolve GitHub URLs to local
		// paths". An empty Path is only a validation error here when there's no
		// such source to derive one from.
		msg.RestartFromSessionId == "" &&
		msg.Path == ""
}

// needsGeneratedOneOffPath reports whether CreateSession must generate a
// fresh scratch directory for this request: an explicit one-off session, or
// an autonomous session created without an explicit path (the omnibar's
// normal flow — the agent needs somewhere to run). Extracted so it's
// directly testable (pure function, no I/O) rather than only through a full
// CreateSession -> tmux -> SessionDriver round trip -- see
// TestNeedsGeneratedOneOffPath.
func needsGeneratedOneOffPath(msg *sessionv1.CreateSessionRequest, resolvedPath string) bool {
	return msg.SessionType == sessionv1.SessionType_SESSION_TYPE_ONE_OFF ||
		(msg.AutonomousMode && resolvedPath == "")
}

// generateOneOffPath creates and returns a fresh scratch directory under
// cfg's configured (or default) one-off base directory. Extracted from
// CreateSession so the directory-generation side effect can be tested
// directly against a real filesystem without any tmux/storage/SessionDriver
// machinery -- see TestGenerateOneOffPath.
func generateOneOffPath(cfg *config.Config) (string, error) {
	baseDir, err := cfg.OneOffBaseDirOrDefault()
	if err != nil {
		return "", fmt.Errorf("failed to resolve one_off_base_dir: %w", err)
	}
	generatedPath, err := namegen.GenerateAndCreate(baseDir, 10)
	if err != nil {
		return "", fmt.Errorf("failed to create one-off directory: %w", err)
	}
	return generatedPath, nil
}

// checkTitleConflictAndLoadExisting loads the raw instance-data rows (via
// ListInstanceData rather than LoadInstances, to avoid the side effect of
// FromInstanceData calling Start() on every session) and rejects msg.Title as
// a duplicate if any existing row already uses it. The returned rows are
// reused by CreateSession's later restart-source resolution.
func (h *Handler) checkTitleConflictAndLoadExisting(msg *sessionv1.CreateSessionRequest) ([]domain.InstanceData, error) {
	existing, err := h.d.Store.ListInstanceData()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
	}
	for _, data := range existing {
		if data.ClaimsTitle(msg.Title) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", msg.Title))
		}
	}
	return existing, nil
}

// dispatchForkSource validates msg.ResumeId (when no fork is requested) and,
// when msg.ForkSourceId is set, copies the source conversation file and
// overwrites msg.ResumeId with the new UUID so the normal start path picks it
// up with --resume. Mutates msg in place, mirroring CreateSession's original
// inline behavior before this extraction.
func (h *Handler) dispatchForkSource(ctx context.Context, msg *sessionv1.CreateSessionRequest) error {
	// Validate client-supplied resume_id before the fork block can overwrite it.
	if msg.ResumeId != "" && msg.ForkSourceId == "" {
		if !resumeIDRe.MatchString(msg.ResumeId) {
			return connect.NewError(connect.CodeInvalidArgument,
				errors.New("resume_id must be a valid UUID"))
		}
	}

	if msg.ForkSourceId == "" {
		return nil
	}

	// Fork dispatch: when fork_source_id is set, copy the source conversation
	// file and set resume_id to the new UUID so the normal start path picks it
	// up with --resume.
	srcPath, findErr := claudehistory.FindConversationFilePath(ctx, msg.ForkSourceId)
	if findErr != nil {
		return connect.NewError(connect.CodeNotFound,
			fmt.Errorf("fork source conversation not found: %w", findErr))
	}
	if msg.ForkAtMessage < 0 {
		return connect.NewError(connect.CodeInvalidArgument,
			errors.New("fork_at_message must not be negative"))
	}
	lineCount := uint64(msg.ForkAtMessage) //#nosec G115 -- validated non-negative above
	newUUID, forkErr := instancepkg.ForkClaudeConversation(srcPath, lineCount, filepath.Dir(srcPath))
	if forkErr != nil {
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("fork conversation failed: %w", forkErr))
	}
	msg.ResumeId = newUUID
	log.Info("[CreateSession] forked conversation",
		"source", msg.ForkSourceId, "new_uuid", newUUID,
		"fork_at_message", msg.ForkAtMessage)
	return nil
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
// +api: session:create
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity moved verbatim from server/services; reducing it is separate work
func (h *Handler) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	ctx, cancel := context.WithTimeout(ctx, CreateSessionTimeout)
	defer cancel()

	// Validate required fields
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	if requiresExplicitPath(req.Msg) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}

	// Diagnostic-only: log the wire-level request shape so a future isolation-bug
	// report doesn't need a fresh repro to see what the server actually received
	// (worktree-envvars-hijack Story 1.3.1). env_var_keys logs KEY NAMES ONLY --
	// never values, since env vars can carry secrets like API base URLs/tokens.
	log.Debug("[CreateSession] request shape",
		"session_type", req.Msg.SessionType,
		"branch", req.Msg.Branch,
		"working_dir", req.Msg.WorkingDir,
		"env_var_keys", slices.Collect(maps.Keys(req.Msg.EnvVars)),
		"profile", req.Msg.Profile,
		"skip_defaults", req.Msg.SkipDefaults,
		"restart_from_session_id", req.Msg.RestartFromSessionId,
		"existing_worktree", req.Msg.ExistingWorktree,
		"alias_name", req.Msg.AliasName,
	)

	// Check if session with this title already exists, and validate/dispatch
	// resume_id and fork_source_id before any other resolution runs.
	existing, err := h.checkTitleConflictAndLoadExisting(req.Msg)
	if err != nil {
		return nil, err
	}
	if err := h.dispatchForkSource(ctx, req.Msg); err != nil {
		return nil, err
	}

	// Load config once; used by the GitHub URL resolution below as well as the
	// one-off path and the defaults/alias path further down.
	cfg := config.LoadConfig()

	// Alias *existence* check (Task 2.1.1a-2, Epic 2.1): kept synchronous and
	// unchanged in behavior, ahead of everything else -- the same fast-fail
	// category as duplicate title/missing path/bad resume_id/fork-source-not-
	// found (requirements.md Constraints). This only checks that the alias
	// exists; the full defaults merge (env vars, CLI flags, path, program,
	// session type) that config.ResolveAlias also performs stays below,
	// unchanged, further down the (now partly deferred) resolution section.
	if req.Msg.AliasName != "" {
		if config.FindAlias(cfg, req.Msg.AliasName) == nil {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("alias %q not found: %w", req.Msg.AliasName, config.ErrAliasNotFound))
		}
	}

	// resolveRemoteTarget is resolved here -- ahead of expandTildePath below, and well
	// ahead of the Directory-mode os.Stat check and the mode-specific block further down
	// -- because both of those, and tilde-expansion, must behave differently for a
	// remote-targeted request. It only depends on req.Msg/cfg (both already available),
	// so resolving it this early is side-effect-free.
	resolvedRemote, remoteRequested, remoteErr := resolveRemoteTarget(req.Msg, cfg)
	if remoteErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, remoteErr)
	}

	// Restart-from-session lineage (Story 2.3.1): resolve the source session's
	// path and enforce the still-live guard before any other path resolution
	// below. See resolveRestartSource's doc comment for the live-vs-persisted
	// lookup order.
	restartSourcePath, err := h.resolveRestartSource(req.Msg, existing)
	if err != nil {
		return nil, err
	}

	// Resolve GitHub URLs to local paths (GOPATH-style: ~/.stapler-squad/repos/<host>/owner/repo)
	var resolvedPath string
	var tildeErr error
	if remoteRequested {
		resolvedPath, tildeErr = rejectRemoteTildePath(req.Msg.Path)
	} else {
		resolvedPath, tildeErr = expandLocalTildePath(req.Msg.Path)
	}
	if tildeErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, tildeErr)
	}
	// An explicit req.Msg.Path always wins over the restart-derived path --
	// only fill in resolvedPath from the source session when the caller left
	// Path empty.
	if resolvedPath == "" && restartSourcePath != "" {
		resolvedPath = restartSourcePath
	}
	branch := req.Msg.Branch
	var gitHubRef *instancepkg.GitHubRef
	var clonedRepoPath string

	// Union statically-configured hosts with hosts from dynamically-added
	// accounts (gh CLI import, device auth) — mirrors ListGitHubAccounts'
	// host union in githubrpc/github_user_service.go so CreateSession recognizes the
	// same enterprise URLs the omnibar's detector does.
	enterpriseHosts := h.EnterpriseHosts(cfg)

	// Epic 2.1 (async-session-creation): only the cheap, local detection
	// (IsGitHubURLWithHosts, a string/regex check) runs synchronously here.
	// The actual clone (ResolveGitHubInputCtxWithHosts, a network call that
	// can take up to ~120s for large repos) is deferred to the trackCleanup
	// background goroutine below, so it never blocks instance
	// construction/save/publish or the RPC response. See plan.md Epic 2.1
	// Story 2.1.1's SLO acceptance criterion. resolvedPath/branch are left as
	// their pre-resolution values (the raw request path/branch) for now; the
	// background goroutine patches them in via Instance.SetGitHubResolution
	// once the clone completes, before Start() runs. Epic 2.2 will fold this
	// into the full named Background Resolution Pipeline.
	deferredGitHubURL := instancepkg.IsGitHubURLWithHosts(req.Msg.Path, enterpriseHosts)
	if deferredGitHubURL {
		log.Info("[CreateSession] detected GitHub URL, deferring resolution to background", "path", req.Msg.Path)
	}

	// One-off session: generate a fresh directory and override resolvedPath.
	// Autonomous sessions created without an explicit path (the omnibar's normal
	// flow) get the same treatment — the agent needs somewhere to run.
	if needsGeneratedOneOffPath(req.Msg, resolvedPath) {
		generatedPath, err := generateOneOffPath(cfg)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resolvedPath = generatedPath
	}

	// Resolve session defaults (global → directory → profile), apply explicit
	// request fields on top, and fold in any custom-program env vars.
	program, autoYes, instanceEnvVars, instanceCLIFlags, aliasSessionType, resolvedPath, err := h.resolveSessionDefaults(cfg, req.Msg, resolvedPath, remoteRequested)
	if err != nil {
		return nil, err
	}

	// Determine session type - use explicit session_type if provided, otherwise infer from fields,
	// then apply the one-off/deferred-GitHub-URL/resume special cases.
	sessionType, requestedNewWorktree, err := remapSessionTypeForSpecialCases(req.Msg, branch, aliasSessionType, deferredGitHubURL)
	if err != nil {
		return nil, err
	}

	// For Directory mode: if path does not exist and create_if_missing is not set, return
	// CodeNotFound so the frontend can show a confirmation dialog. Skipped for a remote
	// target -- create_if_missing is not yet supported for remote Directory sessions (see
	// the remote mode-specific block below), and existence is the remote host's to answer,
	// not this process's local filesystem.
	// create_if_missing=true falls through this whole check; setupFirstTimeWorktree handles creation.
	if sessionType == domain.SessionTypeDirectory && !remoteRequested && !req.Msg.CreateIfMissing {
		if _, statErr := os.Stat(resolvedPath); os.IsNotExist(statErr) {
			if req.Msg.ResumeId != "" {
				return nil, connect.NewError(connect.CodeNotFound,
					fmt.Errorf("cannot resume: project directory no longer exists: %s", resolvedPath))
			}
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("path does not exist: %s", resolvedPath))
		}
	}

	// One-time workspace-peers nudge for genuinely new sessions (not resumes), when the
	// workspacePeersNudgeFlagName feature flag is enabled. Best-effort: any detection/lookup
	// failure just omits the nudge.
	initialPrompt := req.Msg.InitialPrompt
	if req.Msg.ResumeId == "" {
		initialPrompt += h.d.WorkspacePeersBlock(ctx, resolvedPath)
	}

	// auto_approve is representable-but-invalid for an agent yoloFlagFor can't inject a
	// bypass flag for -- the Omnibar UI disables the checkbox client-side, but that's not
	// a guarantee for other RPC callers (MCP tools, scripts, curl), so the invariant is
	// enforced here too rather than left purely client-enforced.
	if req.Msg.AutoApprove && !instancepkg.AutoApproveSupported(program) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("auto_approve is not supported for program %q", program))
	}

	// Remote-target mode-specific block (ssh-remote-workspaces Phase 4, Epic 4.2,
	// Task 4.2.1c; extended post-review to compose with more than one
	// SessionType per ADR-001 "remote as an orthogonal flag" -- every existing
	// SessionType is meaningful on a remote host too): when req.Msg.Remote
	// names a saved remote, resolve its stored identity, dial an SSHRunner, and
	// synchronously bring up the remote worktree/directory + remote tmux
	// session -- mirroring how the AutonomousMode/OneOff blocks above do
	// mode-specific work before session.CreateManagedInstance is called, so a
	// failure here surfaces as a normal RPC error instead of being buried in
	// the async goroutine below.
	//
	// Supported session types: SessionTypeNewWorktree (resolvedPath is an
	// existing repo already on the remote host, mirroring how a local
	// SessionTypeNewWorktree session's Path names an existing local repo; a
	// fresh worktree is created under the remote's configured base_path),
	// SessionTypeExistingWorktree (req.Msg.ExistingWorktree already names a
	// worktree path on the remote host -- no creation, just attach),
	// SessionTypeDirectory (resolvedPath already names a plain directory on
	// the remote host -- no worktree machinery at all, mirrors local
	// Directory sessions), and SessionTypeNewProject (resolvedPath is
	// git-initialized on the remote host via
	// git.RemoteWorktreeOps.InitializeProjectDirectory, mirroring local
	// NewProject's git.InitializeProjectDirectory -- pre-ship review found
	// the CommandRunner-based remote equivalent already existed in
	// session/git/remote_worktree.go with no production caller yet; this is
	// that caller). Any other session type combined with a remote target is
	// rejected up front rather than silently attempting local-filesystem
	// discovery against a path that only exists on the remote host.
	//
	// On success, sessionType/existingWorktreeOverride are remapped to
	// SessionTypeExistingWorktree pointing at the worktree/directory just
	// resolved for NewWorktree/ExistingWorktree specifically -- see
	// setupFirstTimeWorktree's doc comment (session/instance_worktree.go) for
	// why that's the shape the later async Start() goroutine needs for those
	// two: it only attaches to the already-created worktree/tmux session, it
	// never tries to create them again (Setup() is skipped entirely for
	// SessionTypeExistingWorktree). Directory and NewProject are deliberately
	// NOT remapped -- both persist no worktree at all (see
	// instance_worktree.go's setupFirstTimeWorktree default/NewProject
	// cases), so remapping either to ExistingWorktree would make
	// setupFirstTimeWorktree persist a synthetic worktree row with a
	// placeholder "unknown" branch for a path that isn't meaningfully a
	// worktree -- the exact design smell pre-ship review flagged for
	// Directory, which NewProject would otherwise reintroduce.
	executionTarget, existingWorktreeOverride, remoteCreationWarning, sessionType, branch, err := h.setupRemoteSessionTarget(
		ctx, req.Msg, cfg, resolvedRemote, remoteRequested, sessionType, resolvedPath, branch, program)
	if err != nil {
		return nil, err
	}

	// Build instance options
	instanceOpts := instancepkg.InstanceOptions{
		Title:                req.Msg.Title,
		Path:                 resolvedPath,
		WorkingDir:           req.Msg.WorkingDir,
		Branch:               branch,
		Program:              program,
		AutoYes:              autoYes,
		AutoApprove:          req.Msg.AutoApprove,
		Prompt:               req.Msg.Prompt,
		InitialPrompt:        initialPrompt,
		ExistingWorktree:     existingWorktreeOverride,
		CreationWarning:      remoteCreationWarning,
		Category:             req.Msg.Category,
		SessionType:          sessionType,
		TmuxPrefix:           "", // Use default from config
		ResumeId:             req.Msg.ResumeId,
		OneShot:              req.Msg.OneShot,
		ProjectID:            req.Msg.ProjectId,
		MCPServerURL:         h.d.MCPServerURL(),
		CreateIfMissing:      req.Msg.CreateIfMissing,
		AllowedTools:         req.Msg.AllowedTools,
		PermissionMode:       req.Msg.PermissionMode,
		AutonomousMode:       req.Msg.AutonomousMode,
		WorkflowID:           req.Msg.WorkflowId,
		RequestedNewWorktree: requestedNewWorktree,
		EnvVars:              instanceEnvVars,
		CLIFlags:             instanceCLIFlags,
		TmuxServerSocket:     h.d.TmuxServerSocket,
		// ExtraArgs is a direct passthrough of req.Msg.ExtraArgs — unlike CLIFlags, it has no
		// defaults-resolution concept to merge with. It composes with instanceCLIFlags at
		// launch time in buildLaunchCommand: CLIFlags-derived tokens first, ExtraArgs last —
		// an intentional, tested ordering (see TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent).
		ExtraArgs: req.Msg.ExtraArgs,
		// ExecutionTarget carries the dialed remote runner (ssh-remote-workspaces
		// Phase 4 Epic 4.2) built by the mode-specific block above, or the
		// LocalTarget{} default for the overwhelming majority of local sessions.
		ExecutionTarget: executionTarget,
		GitBackend:      h.d.GitBackend(),
		// RestartedFromSessionID records lineage for a restart-from-session
		// request (Story 2.3.1); empty for a normal CreateSession call. The
		// still-live guard and path derivation already ran above.
		RestartedFromSessionID: req.Msg.RestartFromSessionId,
		// Backend resolves the effective ProcessManager backend (tymux-bundled-integration
		// Epic 4.3): req.Msg.BackendOverride as the per-request override, keyed by the
		// sanitized tmux session name (tmux.NewSessionName, via ResolveSessionBackendForTitle)
		// that initTmuxSession will use to create this session's tmux session — NOT the raw
		// req.Msg.Title — since that's what TymuxSessionOverrides/StreamHubSessionOverrides are
		// keyed by (see config.Config.TymuxSessionOverrides's doc comment and
		// tmuxSessionNameForStreamPath's identical derivation in connectrpc_websocket.go).
		Backend: instancepkg.ResolveSessionBackendForTitle(cfg, req.Msg.Title, domain.ProcessManagerBackend(req.Msg.GetBackendOverride())),
		// Tags is a direct passthrough of req.Msg.Tags (Task 2.3.1c) -- set once, synchronously,
		// at construction time, like Title/Path. This exists so MCP's create_session/
		// create_session_for_pr can apply "source:mcp"/PR-derived tags before the async
		// pipeline dispatches, instead of only after AwaitCreationTerminal succeeds -- a
		// pipeline resolution that outlasts the caller's await timeout would otherwise
		// silently never get its tags applied (see server/mcp/tools_lifecycle.go).
		Tags: req.Msg.Tags,
	}

	// Add GitHub metadata if this was a GitHub URL
	if gitHubRef != nil {
		instanceOpts.GitHubOwner = gitHubRef.Owner
		instanceOpts.GitHubRepo = gitHubRef.Repo
		instanceOpts.GitHubHost = gitHubRef.Host
		instanceOpts.GitHubSourceRef = req.Msg.Path
		instanceOpts.ClonedRepoPath = clonedRepoPath
		if gitHubRef.PRNumber > 0 {
			instanceOpts.GitHubPRNumber = gitHubRef.PRNumber
			instanceOpts.GitHubPRURL = gitHubRef.PRURL()
		}
	}

	// Construct and persist the instance (Creating status) via the shared
	// domain function -- see session/create_managed_instance.go (Story
	// 1.2.0a). This does NOT start tmux/the process; that happens in the
	// async goroutine below, exactly as before this extraction.
	instance, err := importer.CreateManagedInstance(ctx, importer.CreateManagedInstanceParams{
		Options:                instanceOpts,
		Storage:                h.d.Store,
		Registry:               h.d.Registry(),
		CreateIfMissing:        req.Msg.CreateIfMissing,
		ResumeID:               req.Msg.ResumeId,
		DeferredPathResolution: deferredGitHubURL,
	})
	if err != nil {
		switch {
		case errors.Is(err, importer.ErrPathNotExist), errors.Is(err, importer.ErrResumePathNotExist):
			return nil, connect.NewError(connect.CodeNotFound, err)
		case errors.Is(err, importer.ErrInstanceConstructionFailed):
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		case errors.Is(err, session.ErrTitleConflict):
			// A second, concurrent CreateSession call won the race against the
			// synchronous ListInstanceData-based uniqueness check above (Task
			// 2.1.1c) -- storage.AddInstance's own title-uniqueness guard is
			// the backstop that turns that race into an error instead of
			// silently overwriting the winner's row.
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		default:
			// Covers ErrInstanceRegistrationFailed and other ErrInstanceSaveFailed causes.
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Index before the async start below so the first hook resolves.
	h.d.IndexForDelivery(instance)

	// Add the session to the poller so WatchSessions picks it up immediately.
	if h.d.Poller() != nil {
		h.d.Poller().AddInstance(instance)
		log.Info("[ReviewQueue] added new session to poller", "session", instance.Title)
	}
	if h.d.TagPoller() != nil {
		h.d.TagPoller().AddInstance(instance)
	}

	// Record initial_prompt (typed into the session terminal once the session reaches Ready state)
	// in prompt history so it appears in the recent-prompts dropdown.
	if req.Msg.InitialPrompt != "" {
		if _, err := h.d.PromptStore.RecordUsage(req.Msg.InitialPrompt); err != nil {
			log.Warn("failed to record initial prompt usage", "err", err)
		}
	}

	// Publish SessionCreated event so watchers see the Creating-status session immediately.
	h.d.Bus.Publish(events.NewSessionCreatedEvent(instance))

	// Capture refs needed inside the goroutine (avoid capturing req.Msg which may be GC'd).
	instanceTitle := instance.Title
	instanceRootDir := instance.GetEffectiveRootDir()
	// deferredGitHubSourceURL/deferredEnterpriseHosts feed the deferred GitHub
	// resolution below (Epic 2.1) -- captured here rather than read from
	// req.Msg inside the goroutine, same GC rationale as instanceTitle above.
	deferredGitHubSourceURL := req.Msg.Path
	deferredEnterpriseHosts := enterpriseHosts

	// Pre-compute the Creating-state proto for the RPC response so the return
	// statement below does not race with the goroutine's SetCreationProgress calls.
	creatingProto := adapters.InstanceToProto(instance, h.d.WorkflowNames())

	// Capture the fencing epoch once, before any phase of the Background
	// Resolution Pipeline runs (Story 2.2.3) -- the only value the
	// pipeline's terminal write presents back to CommitTerminalStatus to
	// win the race against a concurrent cancel/retry.
	creationEpoch := instance.CreationEpoch()

	// Perform the actual initialization asynchronously so the RPC returns within milliseconds.
	// Tracked via trackCleanup (not a bare `go func()`) so Shutdown blocks until this
	// goroutine finishes — otherwise it can outlive the test that spawned it and touch a
	// later test's tempdirs/STAPLER_SQUAD_TEST_DIR/tmux-exec-gate directory after that
	// later test's t.Cleanup has already torn them down, producing "sql: database is
	// closed" errors and the cross-iteration "directory not empty" flake in
	// TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent.
	h.d.Track(func() {
		h.RunResolutionPipeline(ctx, PipelineParams{
			Instance:                instance,
			Epoch:                   creationEpoch,
			Title:                   instanceTitle,
			RootDir:                 instanceRootDir,
			DeferredGitHubURL:       deferredGitHubURL,
			DeferredGitHubSourceURL: deferredGitHubSourceURL,
			DeferredEnterpriseHosts: deferredEnterpriseHosts,
		})
	})

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: creatingProto,
	}), nil
}
