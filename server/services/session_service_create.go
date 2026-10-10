package services

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/namegen"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/tmux"
	"golang.org/x/crypto/ssh"
)

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
func (s *SessionService) resolveRestartSource(req *sessionv1.CreateSessionRequest, existing []session.InstanceData) (string, error) {
	if req.RestartFromSessionId == "" {
		return "", nil
	}

	if liveSrc := s.FindLiveInstance(req.RestartFromSessionId); liveSrc != nil {
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
func (s *SessionService) checkTitleConflictAndLoadExisting(msg *sessionv1.CreateSessionRequest) ([]session.InstanceData, error) {
	existing, err := s.storage.ListInstanceData()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
	}
	for _, data := range existing {
		if data.Title == msg.Title {
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
func (s *SessionService) dispatchForkSource(ctx context.Context, msg *sessionv1.CreateSessionRequest) error {
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
	srcPath, findErr := session.FindConversationFilePath(ctx, msg.ForkSourceId)
	if findErr != nil {
		return connect.NewError(connect.CodeNotFound,
			fmt.Errorf("fork source conversation not found: %w", findErr))
	}
	if msg.ForkAtMessage < 0 {
		return connect.NewError(connect.CodeInvalidArgument,
			errors.New("fork_at_message must not be negative"))
	}
	lineCount := uint64(msg.ForkAtMessage) //#nosec G115 -- validated non-negative above
	newUUID, forkErr := session.ForkClaudeConversation(srcPath, lineCount, filepath.Dir(srcPath))
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

// resolveSessionDefaults resolves session defaults (global -> directory ->
// profile, or the alias-specific equivalent when msg.AliasName is set),
// applies explicit request fields (env_vars/cli_flags) on top, and folds in
// any custom-program env vars. skip_defaults bypasses the alias/profile
// resolution entirely (for scripted or explicit-empty sessions), but the
// explicit-field merge and custom-program step still run. Returns the
// possibly-alias-resolved path (unchanged when msg.AliasName's Path is empty
// or resolvedPath was already set) alongside the resolved program/autoYes/
// env/CLI-flags and the alias's own session-type override (config.SessionTypeDefault
// when none applies).
func (s *SessionService) resolveAliasSessionDefaults(
	cfg *config.Config,
	msg *sessionv1.CreateSessionRequest,
	resolvedPath string,
	remoteRequested bool,
	program string,
	autoYes bool,
	instanceEnvVars map[string]string,
) (outProgram string, outAutoYes bool, outCLIFlags string, outResolvedPath string, aliasSessionType config.SessionType, err error) {
	aliasSessionType = config.SessionTypeDefault
	resolved, aliasErr := config.ResolveAlias(cfg, msg.AliasName, msg.Branch, msg.Title, "")
	if aliasErr != nil {
		if errors.Is(aliasErr, config.ErrAliasNotFound) {
			return "", false, "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("alias %q not found: %w", msg.AliasName, aliasErr))
		}
		return "", false, "", "", "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve alias %q: %w", msg.AliasName, aliasErr))
	}
	if program == "" {
		program = resolved.Program
	}
	if !autoYes && resolved.AutoYes {
		autoYes = true
	}
	for k, v := range resolved.EnvVars {
		instanceEnvVars[k] = v
	}
	if resolvedPath == "" && resolved.Path != "" {
		var aliasResolvedPath string
		var aliasTildeErr error
		if remoteRequested {
			aliasResolvedPath, aliasTildeErr = rejectRemoteTildePath(resolved.Path)
		} else {
			aliasResolvedPath, aliasTildeErr = expandLocalTildePath(resolved.Path)
		}
		if aliasTildeErr != nil {
			return "", false, "", "", "", connect.NewError(connect.CodeInvalidArgument, aliasTildeErr)
		}
		resolvedPath = aliasResolvedPath
	}
	// Read session type directly from the alias config — it is an alias-specific
	// property, not a cascading default, so it is not part of ResolvedDefaults.
	if alias := config.FindAlias(cfg, msg.AliasName); alias != nil {
		aliasSessionType = alias.SessionType
	}
	return program, autoYes, resolved.CLIFlags, resolvedPath, aliasSessionType, nil
}

func (s *SessionService) resolveSessionDefaults(cfg *config.Config, msg *sessionv1.CreateSessionRequest, resolvedPath string, remoteRequested bool) (
	program string, autoYes bool, instanceEnvVars map[string]string, instanceCLIFlags string, aliasSessionType config.SessionType, outPath string, err error,
) {
	program = msg.Program
	autoYes = msg.AutoYes
	instanceEnvVars = make(map[string]string)
	aliasSessionType = config.SessionTypeDefault // session type from alias config (empty = no override)
	if !msg.SkipDefaults {
		if msg.AliasName != "" {
			var aliasErr error
			program, autoYes, instanceCLIFlags, resolvedPath, aliasSessionType, aliasErr =
				s.resolveAliasSessionDefaults(cfg, msg, resolvedPath, remoteRequested, program, autoYes, instanceEnvVars)
			if aliasErr != nil {
				return "", false, nil, "", "", "", aliasErr
			}
		} else {
			workingDir := msg.WorkingDir
			if workingDir == "" {
				workingDir = resolvedPath
			}
			resolved := config.ResolveDefaults(cfg, workingDir, msg.Profile)
			if program == "" {
				program = resolved.Program
			}
			if !autoYes && resolved.AutoYes {
				autoYes = true
			}
			for k, v := range resolved.EnvVars {
				instanceEnvVars[k] = v
			}
			instanceCLIFlags = resolved.CLIFlags
		}
	}

	// Merge explicit request env_vars on top of resolved defaults.
	for k, v := range msg.EnvVars {
		instanceEnvVars[k] = v
	}
	// Append explicit request cli_flags on top of resolved defaults.
	if msg.CliFlags != "" {
		if instanceCLIFlags != "" {
			instanceCLIFlags += " " + msg.CliFlags
		} else {
			instanceCLIFlags = msg.CliFlags
		}
	}

	// If program refers to a custom program ID, resolve its underlying command, CLI flags, and env vars.
	if program != "" {
		resolvedProg := config.ResolveProgramConfig(cfg, program)
		if resolvedProg.IsCustom {
			for k, v := range resolvedProg.EnvVars {
				if _, exists := instanceEnvVars[k]; !exists {
					instanceEnvVars[k] = v
				}
			}
			// Program CLIFlags are NOT prepended here: buildLaunchCommand resolves them
			// from the stored custom program ID at launch, so doing it here doubles them.
		}
	}

	return program, autoYes, instanceEnvVars, instanceCLIFlags, aliasSessionType, resolvedPath, nil
}

// remapSessionTypeForSpecialCases determines the effective session type --
// explicit msg.SessionType if provided, otherwise inferred from branch via
// resolveSessionType -- then applies, in order: the alias fallback (when the
// alias specified a session type and the request itself didn't), the
// one-off-runs-as-directory collapse, the deferred-GitHub-URL override (force
// NewWorktree since resolveSessionType's empty-branch fallback would
// otherwise misclassify an unresolved clone target as a plain directory), and
// the resume-forces-directory guard (must not create a new worktree that
// would break the --resume lookup). requestedNewWorktree is captured before
// any of these remaps run, so it names the wire-level request itself -- feeds
// startLocked's Epic 1.5 invariant check.
func remapSessionTypeForSpecialCases(msg *sessionv1.CreateSessionRequest, branch string, aliasSessionType config.SessionType, deferredGitHubURL bool) (session.SessionType, bool, error) {
	sessionType, err := resolveSessionType(msg, branch)
	if err != nil {
		return "", false, connect.NewError(connect.CodeInvalidArgument, err)
	}
	requestedNewWorktree := msg.SessionType == sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE
	if aliasSessionType != config.SessionTypeDefault && msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = aliasSessionType
	}

	// One-off sessions run as directory sessions — the path was already generated above.
	if sessionType == session.SessionTypeOneOff {
		sessionType = session.SessionTypeDirectory
	}

	if deferredGitHubURL && msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED &&
		msg.ExistingWorktree == "" {
		sessionType = session.SessionTypeNewWorktree
	}

	// For resume sessions, force DIRECTORY type — we must not create a new worktree
	// that would produce a different project path and break the --resume lookup.
	if msg.ResumeId != "" && msg.ForkSourceId == "" &&
		msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = session.SessionTypeDirectory
	}

	return sessionType, requestedNewWorktree, nil
}

// createRemoteNewWorktree creates the branch and worktree for a remote
// SessionTypeNewWorktree session. The base commit is resolved from resolvedPath
// rather than ambient HEAD (see Instance.newWorktreeFromResolvedBase).
func (s *SessionService) createRemoteNewWorktree(
	ctx context.Context,
	msg *sessionv1.CreateSessionRequest,
	cfg *config.Config,
	resolvedRemote *session.RemoteTarget,
	runner tmux.CommandRunner,
	resolvedPath string,
	branch string,
) (remoteWorkingPath string, worktreeOps *git.RemoteWorktreeOps, remoteWT git.RemoteWorktree, warning string, outBranch string, err error) {
	if branch == "" {
		branch = cfg.BranchPrefix + git.SanitizeBranchName(msg.Title)
	}
	remoteWorkingPath = path.Join(resolvedRemote.BasePath, git.SanitizeBranchName(msg.Title))
	worktreeOps = git.NewRemoteWorktreeOps(runner)
	remoteWT = git.RemoteWorktree{RepoPath: resolvedPath, WorktreePath: remoteWorkingPath, Branch: branch}

	// baseSHA == "" with a nil error means an unborn repo, the one case ambient HEAD is safe to use.
	defaultBranch, baseSHA, resolveErr := git.ResolveRemoteWorktreeBaseCommit(ctx, runner, resolvedPath)
	if resolveErr != nil {
		return "", nil, git.RemoteWorktree{}, "", "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to resolve default branch on remote %q: %w", resolvedRemote.Name, resolveErr))
	}
	branchArgs := []string{"branch", branch}
	if baseSHA != "" {
		branchArgs = append(branchArgs, baseSHA)
		if diverged, ambientBranch := git.RemoteAmbientHEADDivergesFromBase(ctx, runner, resolvedPath, baseSHA); diverged {
			warning = git.FormatAmbientDivergenceWarning(resolvedPath, defaultBranch, ambientBranch)
		}
	}

	// Best-effort: "git branch <name> [sha]" failing because the branch already exists is expected.
	//nolint:norawgitcli // migrating, go-git-fork plan Epic 1.2 (route via session/git/backend)
	if out, branchErr := runner.Run(ctx, resolvedPath, "git", branchArgs...); branchErr != nil &&
		!strings.Contains(string(out), "already exists") {
		return "", nil, git.RemoteWorktree{}, "", "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create branch %q on remote %q: %s (%w)",
				branch, resolvedRemote.Name, strings.TrimSpace(string(out)), branchErr))
	}
	if createErr := worktreeOps.CreateWorktree(ctx, remoteWT); createErr != nil {
		return "", nil, git.RemoteWorktree{}, "", "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create remote worktree on %q: %w", resolvedRemote.Name, createErr))
	}
	return remoteWorkingPath, worktreeOps, remoteWT, warning, branch, nil
}

// setupRemoteSessionTarget implements the remote-target mode-specific block
// (ssh-remote-workspaces Phase 4, Epic 4.2, Task 4.2.1c; extended post-review
// to compose with more than one SessionType per ADR-001 "remote as an
// orthogonal flag" -- every existing SessionType is meaningful on a remote
// host too): when remoteRequested, resolve resolvedRemote's stored identity,
// dial an SSHRunner, and synchronously bring up the remote worktree/directory
// + remote tmux session -- mirroring how the AutonomousMode/OneOff blocks in
// CreateSession do mode-specific work before session.CreateManagedInstance is
// called, so a failure here surfaces as a normal RPC error instead of being
// buried in the async goroutine below.
//
// Supported session types: SessionTypeNewWorktree (resolvedPath is an
// existing repo already on the remote host, mirroring how a local
// SessionTypeNewWorktree session's Path names an existing local repo; a
// fresh worktree is created under the remote's configured base_path),
// SessionTypeExistingWorktree (msg.ExistingWorktree already names a
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
// When !remoteRequested, returns the local defaults (LocalTarget{},
// msg.ExistingWorktree, no warning, sessionType/branch unchanged) --
// mirroring CreateSession's pre-extraction variable initialization that ran
// unconditionally ahead of the `if remoteRequested` block.
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
func (s *SessionService) setupRemoteSessionTarget(
	ctx context.Context,
	msg *sessionv1.CreateSessionRequest,
	cfg *config.Config,
	resolvedRemote *session.RemoteTarget,
	remoteRequested bool,
	sessionType session.SessionType,
	resolvedPath string,
	branch string,
	program string,
) (executionTarget session.ExecutionTarget, existingWorktreeOverride string, remoteCreationWarning string, outSessionType session.SessionType, outBranch string, err error) {
	executionTarget = session.LocalTarget{}
	existingWorktreeOverride = msg.ExistingWorktree
	if !remoteRequested {
		return executionTarget, existingWorktreeOverride, "", sessionType, branch, nil
	}

	switch sessionType {
	case session.SessionTypeNewWorktree, session.SessionTypeExistingWorktree, session.SessionTypeDirectory, session.SessionTypeNewProject:
		// supported, handled below
	default:
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("remote sessions do not support session_type=%q (supported: new_worktree, existing_worktree, directory, new_project)", sessionType))
	}
	if sessionType == session.SessionTypeExistingWorktree && msg.ExistingWorktree == "" {
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			errors.New("existing_worktree path is required for remote session_type=existing_worktree"))
	}
	if sessionType == session.SessionTypeNewProject && resolvedPath == "" {
		// Local NewProject can leave resolvedPath empty and still resolve one later
		// via alias config (see the "path is required" exemption above and the
		// alias-resolution block below) -- but a remote NewProject has no such
		// alias-driven fallback for WHERE on the remote host to init, so this must
		// be resolvable synchronously, right here, or rejected with a clear error
		// rather than remote-initializing an empty/nonsense path.
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			errors.New("path is required for remote session_type=new_project"))
	}
	if s.remoteKeyStore == nil || s.remoteKnownHosts == nil {
		return nil, "", "", "", "", connect.NewError(connect.CodeFailedPrecondition,
			errors.New("remote session support is not configured on this server"))
	}

	target := tmux.SSHTarget{Name: resolvedRemote.Name, Addr: remoteAddr(resolvedRemote.Host)}
	clientConfig := ssh.ClientConfig{
		User:            resolvedRemote.User,
		Auth:            resolveIdentityAuthMethods(ctx, s.remoteKeyStore, resolvedRemote.IdentityRef),
		HostKeyCallback: s.remoteKnownHosts.HostKeyCallback(),
	}
	runner := tmux.NewSSHRunner(target, clientConfig, tmux.WithSSHClientPool(s.sshClientPool()))
	if dialErr := runner.Dial(ctx); dialErr != nil {
		return nil, "", "", "", "", connect.NewError(connect.CodeUnavailable,
			fmt.Errorf("failed to connect to remote %q: %w", resolvedRemote.Name, dialErr))
	}

	// remoteWorkingPath is the remote path the tmux session attaches to.
	// worktreeOps/remoteWT/createdWorktree are populated only for
	// SessionTypeNewWorktree, the one case that actually creates
	// something on the remote host worth rolling back if the tmux
	// bring-up below fails -- ExistingWorktree/Directory attach to a
	// path the caller already asserts exists, so there is nothing this
	// block itself created to clean up.
	var (
		remoteWorkingPath string
		worktreeOps       *git.RemoteWorktreeOps
		remoteWT          git.RemoteWorktree
		createdWorktree   bool
	)
	switch sessionType {
	case session.SessionTypeNewWorktree:
		var newWTErr error
		remoteWorkingPath, worktreeOps, remoteWT, remoteCreationWarning, branch, newWTErr =
			s.createRemoteNewWorktree(ctx, msg, cfg, resolvedRemote, runner, resolvedPath, branch)
		if newWTErr != nil {
			return nil, "", "", "", "", newWTErr
		}
		createdWorktree = true
	case session.SessionTypeExistingWorktree:
		remoteWorkingPath = msg.ExistingWorktree
	case session.SessionTypeDirectory:
		remoteWorkingPath = resolvedPath
	case session.SessionTypeNewProject:
		remoteWorkingPath = resolvedPath
		// No createdWorktree/rollback bookkeeping here, matching
		// InitializeProjectDirectory's own doc comment: unlike CreateWorktree
		// there is no cheap, safe remote rollback for a partial git-init (that
		// doc comment is explicit about not wanting an `rm -rf` embedded in a
		// remote script). A failure after this point can leave a
		// partially-initialized directory on the remote host for an operator
		// to inspect or retry against -- an accepted, documented tradeoff, not
		// an oversight.
		if initErr := git.NewRemoteWorktreeOps(runner).InitializeProjectDirectory(ctx, remoteWorkingPath); initErr != nil {
			return nil, "", "", "", "", connect.NewError(connect.CodeInternal,
				fmt.Errorf("failed to initialize new project on remote %q: %w", resolvedRemote.Name, initErr))
		}
	}

	// Best-effort compensating cleanup if the remote tmux session can't be
	// brought up after the worktree already exists (Task 4.2.1e,
	// adversarial-review.md Blocker 3): the worktree creation above already
	// succeeded, so a failure here must not leave it silently orphaned.
	// Only applies when createdWorktree -- ExistingWorktree/Directory
	// never created anything this block owns, so there is nothing to
	// roll back on tmux failure for those.
	//
	// s.testTmuxServerSocket must be applied here exactly as instance.go's
	// initTmuxSession applies InstanceOptions.TmuxServerSocket (set to the
	// same value a few lines below) -- otherwise this synchronous check
	// and the Instance's own later TmuxSession construction resolve to two
	// DIFFERENT tmux -L server sockets under config.IsTestMode() (each
	// SessionService gets its own isolated per-instance test socket; the
	// package-wide test-isolation fallback ResolveSocket("") would use
	// instead is a different socket entirely), so the session this block
	// creates would silently be invisible to -- and get recreated a second
	// time by -- the Instance's own startLocked() shortly after. Confirmed
	// via TestCreateSession_RemoteTarget_CreatesRemoteWorktreeAndTmuxSession,
	// which fails if the two calls target "test-isolated-<pid>" and
	// "test_server_services_<pid>_<n>" respectively instead of the same socket.
	var remoteSession *tmux.TmuxSession
	if s.testTmuxServerSocket != "" {
		remoteSession = tmux.NewTmuxSessionWithServerSocket(msg.Title, program, "staplersquad_", s.testTmuxServerSocket, tmux.WithRegistry(nil), tmux.WithCommandRunner(runner))
	} else {
		remoteSession = tmux.NewTmuxSessionWithPrefix(msg.Title, program, "staplersquad_", tmux.WithCommandRunner(runner))
	}
	if ensureErr := remoteSession.EnsureRemoteSession(ctx, remoteWorkingPath); ensureErr != nil {
		if !createdWorktree {
			return nil, "", "", "", "", connect.NewError(connect.CodeInternal, fmt.Errorf(
				"failed to start remote tmux session on %q: %w", resolvedRemote.Name, ensureErr))
		}
		if cleanupErr := worktreeOps.RemoveWorktree(ctx, remoteWT); cleanupErr != nil {
			return nil, "", "", "", "", connect.NewError(connect.CodeInternal, fmt.Errorf(
				"failed to start remote tmux session on %q (%v); best-effort cleanup of remote worktree %s ALSO failed (%v) -- this path may be orphaned on the remote host and require manual removal",
				resolvedRemote.Name, ensureErr, remoteWorkingPath, cleanupErr))
		}
		return nil, "", "", "", "", connect.NewError(connect.CodeInternal, fmt.Errorf(
			"failed to start remote tmux session on %q: %w (remote worktree %s was cleaned up)",
			resolvedRemote.Name, ensureErr, remoteWorkingPath))
	}

	executionTarget = session.NewRemoteExecutionTarget(*resolvedRemote, runner)
	// Only NewWorktree/ExistingWorktree originated in a real git worktree on the
	// remote host -- remap those to ExistingWorktree so setupFirstTimeWorktree
	// attaches to remoteWorkingPath instead of re-running worktree creation
	// (see its doc comment). Directory/NewProject-originated sessions are
	// deliberately left as-is: forcing either through ExistingWorktree too (as
	// this used to do unconditionally for Directory) made setupFirstTimeWorktree
	// persist a synthetic GitWorktree row with a placeholder "unknown" branch
	// even when the remote path may not be a meaningful git worktree -- found in
	// pre-ship review. Leaving them alone routes each through
	// setupFirstTimeWorktree's existing default/NewProject cases
	// (SetWorktree(nil), no worktree persisted), identical to their local
	// counterparts, with no ent schema change needed.
	if sessionType != session.SessionTypeDirectory && sessionType != session.SessionTypeNewProject {
		sessionType = session.SessionTypeExistingWorktree
	}
	existingWorktreeOverride = remoteWorkingPath

	return executionTarget, existingWorktreeOverride, remoteCreationWarning, sessionType, branch, nil
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
// +api: session:create
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	ctx, cancel := context.WithTimeout(ctx, createSessionTimeout)
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
	existing, err := s.checkTitleConflictAndLoadExisting(req.Msg)
	if err != nil {
		return nil, err
	}
	if err := s.dispatchForkSource(ctx, req.Msg); err != nil {
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
	restartSourcePath, err := s.resolveRestartSource(req.Msg, existing)
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
	var gitHubRef *session.GitHubRef
	var clonedRepoPath string

	// Union statically-configured hosts with hosts from dynamically-added
	// accounts (gh CLI import, device auth) — mirrors ListGitHubAccounts'
	// host union in github_user_service.go so CreateSession recognizes the
	// same enterprise URLs the omnibar's detector does.
	enterpriseHosts := s.enterpriseHosts(cfg)

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
	deferredGitHubURL := session.IsGitHubURLWithHosts(req.Msg.Path, enterpriseHosts)
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
	program, autoYes, instanceEnvVars, instanceCLIFlags, aliasSessionType, resolvedPath, err := s.resolveSessionDefaults(cfg, req.Msg, resolvedPath, remoteRequested)
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
	if sessionType == session.SessionTypeDirectory && !remoteRequested && !req.Msg.CreateIfMissing {
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
		initialPrompt += s.workspacePeersBlockFor(ctx, resolvedPath)
	}

	// auto_approve is representable-but-invalid for an agent yoloFlagFor can't inject a
	// bypass flag for -- the Omnibar UI disables the checkbox client-side, but that's not
	// a guarantee for other RPC callers (MCP tools, scripts, curl), so the invariant is
	// enforced here too rather than left purely client-enforced.
	if req.Msg.AutoApprove && !session.AutoApproveSupported(program) {
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
	executionTarget, existingWorktreeOverride, remoteCreationWarning, sessionType, branch, err := s.setupRemoteSessionTarget(
		ctx, req.Msg, cfg, resolvedRemote, remoteRequested, sessionType, resolvedPath, branch, program)
	if err != nil {
		return nil, err
	}

	// Build instance options
	instanceOpts := session.InstanceOptions{
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
		MCPServerURL:         s.resolveMCPServerURL(),
		CreateIfMissing:      req.Msg.CreateIfMissing,
		AllowedTools:         req.Msg.AllowedTools,
		PermissionMode:       req.Msg.PermissionMode,
		AutonomousMode:       req.Msg.AutonomousMode,
		WorkflowID:           req.Msg.WorkflowId,
		RequestedNewWorktree: requestedNewWorktree,
		EnvVars:              instanceEnvVars,
		CLIFlags:             instanceCLIFlags,
		TmuxServerSocket:     s.testTmuxServerSocket,
		// ExtraArgs is a direct passthrough of req.Msg.ExtraArgs — unlike CLIFlags, it has no
		// defaults-resolution concept to merge with. It composes with instanceCLIFlags at
		// launch time in buildLaunchCommand: CLIFlags-derived tokens first, ExtraArgs last —
		// an intentional, tested ordering (see TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent).
		ExtraArgs: req.Msg.ExtraArgs,
		// ExecutionTarget carries the dialed remote runner (ssh-remote-workspaces
		// Phase 4 Epic 4.2) built by the mode-specific block above, or the
		// LocalTarget{} default for the overwhelming majority of local sessions.
		ExecutionTarget: executionTarget,
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
		Backend: session.ResolveSessionBackendForTitle(cfg, req.Msg.Title, session.ProcessManagerBackend(req.Msg.GetBackendOverride())),
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
	instance, err := session.CreateManagedInstance(ctx, session.CreateManagedInstanceParams{
		Options:                instanceOpts,
		Storage:                s.storage,
		Registry:               s.registry,
		CreateIfMissing:        req.Msg.CreateIfMissing,
		ResumeID:               req.Msg.ResumeId,
		DeferredPathResolution: deferredGitHubURL,
	})
	if err != nil {
		switch {
		case errors.Is(err, session.ErrPathNotExist), errors.Is(err, session.ErrResumePathNotExist):
			return nil, connect.NewError(connect.CodeNotFound, err)
		case errors.Is(err, session.ErrInstanceConstructionFailed):
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

	// Add the session to the poller so WatchSessions picks it up immediately.
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.AddInstance(instance)
		log.Info("[ReviewQueue] added new session to poller", "session", instance.Title)
	}
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.AddInstance(instance)
	}

	// Record initial_prompt (typed into the session terminal once the session reaches Ready state)
	// in prompt history so it appears in the recent-prompts dropdown.
	if req.Msg.InitialPrompt != "" {
		if _, err := s.promptStore.RecordUsage(req.Msg.InitialPrompt); err != nil {
			log.Warn("failed to record initial prompt usage", "err", err)
		}
	}

	// Publish SessionCreated event so watchers see the Creating-status session immediately.
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))

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
	creatingProto := adapters.InstanceToProto(instance, s.workflowNames())

	// Capture the fencing epoch once, before any phase of the Background
	// Resolution Pipeline runs (Story 2.2.3) -- the only value the
	// pipeline's terminal write presents back to commitTerminalStatus to
	// win the race against a concurrent cancel/retry.
	creationEpoch := instance.CreationEpoch()

	// Perform the actual initialization asynchronously so the RPC returns within milliseconds.
	// Tracked via trackCleanup (not a bare `go func()`) so Shutdown blocks until this
	// goroutine finishes — otherwise it can outlive the test that spawned it and touch a
	// later test's tempdirs/STAPLER_SQUAD_TEST_DIR/tmux-exec-gate directory after that
	// later test's t.Cleanup has already torn them down, producing "sql: database is
	// closed" errors and the cross-iteration "directory not empty" flake in
	// TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent.
	s.trackCleanup(func() {
		s.runBackgroundResolutionPipeline(ctx, creationPipelineParams{
			instance:                instance,
			epoch:                   creationEpoch,
			instanceTitle:           instanceTitle,
			instanceRootDir:         instanceRootDir,
			deferredGitHubURL:       deferredGitHubURL,
			deferredGitHubSourceURL: deferredGitHubSourceURL,
			deferredEnterpriseHosts: deferredEnterpriseHosts,
		})
	})

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: creatingProto,
	}), nil
}

// setupRemoteApprovalHooks wires a remote session's PermissionRequest hook
// round trip end to end (ssh-remote-workspaces Phase 5 correction, ADR-003's
// addendum): constructs and starts a *sshremote.RemoteApprovalRelay for
// instance, stores it on instance so it gets stopped on teardown (Instance.
// destroyChain -> stopRemoteApprovalRelay), then injects the socat-based
// remote hook command via InjectHookConfigRemote using the relay's freshly
// minted bearer token.
//
// Called from CreateSession's async goroutine only when instance.IsRemote();
// every error here is non-fatal by design -- the caller logs and continues,
// exactly like the local InjectHookConfig call site it replaces for remote
// sessions (the session is fully functional without approval-hook config,
// it just falls back to no hook at all for PermissionRequest events).
func (s *SessionService) setupRemoteApprovalHooks(instance *session.Instance, rootDir, title string) error {
	if s.permissionRequestHandler == nil {
		return errors.New("remote approval relay: no PermissionRequestHandler wired (SetPermissionRequestHandler was never called)")
	}
	remoteTarget, ok := instance.GetExecutionTarget().(session.RemoteExecutionTarget)
	if !ok {
		return fmt.Errorf("instance.IsRemote() true but ExecutionTarget is %T, not session.RemoteExecutionTarget", instance.GetExecutionTarget())
	}

	relay, err := sshremote.NewRemoteApprovalRelay(s.sshClientPool(), s.permissionRequestHandler, sshremote.RemoteApprovalRelayTarget{
		RemoteName:      remoteTarget.Target().Name,
		BasePath:        rootDir,
		StableSessionID: instance.GetStableID(),
		Title:           title,
	})
	if err != nil {
		return fmt.Errorf("construct remote approval relay: %w", err)
	}

	// Started against context.Background(), not a request-scoped or
	// server-shutdown-bound context: this relay's lifetime is the session's
	// lifetime, stopped explicitly via instance.destroyChain() ->
	// stopRemoteApprovalRelay() when the session itself is torn down, not
	// when any particular request or the server process happens to be
	// shutting down (a tmux session is meant to survive a server restart;
	// this in-memory relay does not survive one today -- reconnecting
	// existing remote sessions' relays on server restart is a known gap,
	// out of this change's scope, since CreateSession's async goroutine is
	// the only call site wired here; see the final report for this
	// explicitly flagged as a follow-up).
	relay.Start(context.Background())

	token, _ := relay.BearerToken()
	hookTarget := RemoteHookTarget{
		SocketPath:  sshremote.RemoteApprovalSocketPath(rootDir, instance.GetStableID()),
		BearerToken: token,
	}

	injectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := InjectHookConfigRemote(injectCtx, remoteTarget.Runner(), rootDir, instance.GetStableID(), hookTarget); err != nil {
		relay.Stop()
		return fmt.Errorf("inject remote hook config: %w", err)
	}

	instance.SetRemoteApprovalRelay(relay)
	return nil
}

// resolveRemoteTarget resolves req.Msg.Remote.RemoteName (if set) against
// cfg's saved remotes into the plain session.RemoteTarget data (name, host,
// user, base_path, identity_ref) -- ssh-remote-workspaces Phase 4, Task
// 4.2.1b. It does not dial anything; that's the mode-specific block in
// CreateSession below (Task 4.2.1c), which pairs this data with a dialed
// *tmux.SSHRunner into a single session.RemoteExecutionTarget in one atomic
// step.
//
// Returns (nil, nil) when msg.Remote is unset or names no remote (the
// ordinary local-session case, the overwhelming majority of requests).
// Returns a non-nil error, always wrapping an unknown-remote message, when
// msg.Remote.RemoteName is set but does not match any cfg.RemoteByName entry
// -- CreateSession maps this to connect.CodeInvalidArgument before any
// worktree/tmux work begins.
// resolveRemoteTarget resolves req.Msg.Remote against cfg's configured remotes. ok is false
// when the request didn't specify a remote at all (not an error); err is non-nil only when a
// remote WAS requested but its name doesn't match any configured RemoteConfig.
//
// Registry note: this is CreateSession's remote-target extension (docs/registry/features/
// backend/session/create.json's testIds), not a separate registered feature -- a hand-authored
// "session:create-remote-target" per-feature file was tried and found non-durable: the CreateSession
// RPC itself is the only real proto-derived feature id, and registry-generate-backend's
// prune-stale-backend.sh deletes any committed backend file whose id it can't re-derive from
// the proto+markers scan on every run (silently deleted the hand-authored file the first time
// `make registry-generate` ran after adding it).
func resolveRemoteTarget(msg *sessionv1.CreateSessionRequest, cfg *config.Config) (target *session.RemoteTarget, ok bool, err error) {
	if msg.GetRemote() == nil || msg.GetRemote().GetRemoteName() == "" {
		return nil, false, nil
	}
	remoteName := msg.GetRemote().GetRemoteName()
	remoteCfg, found := cfg.RemoteByName(remoteName)
	if !found {
		return nil, false, fmt.Errorf("no remote configured named %q", remoteName)
	}
	return &session.RemoteTarget{
		Name:        remoteCfg.Name,
		Host:        remoteCfg.Host,
		User:        remoteCfg.User,
		BasePath:    remoteCfg.BasePath,
		IdentityRef: remoteCfg.IdentityRef,
	}, true, nil
}

// resolveSessionType maps a CreateSessionRequest + resolved branch to a session.SessionType.
// Priority: explicit session_type > inference from branch/existing_worktree.
// ONE_OFF is returned as SessionTypeOneOff; callers are responsible for converting it to
// SessionTypeDirectory after the one-off directory has been generated.
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) (session.SessionType, error) {
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
			return session.SessionTypeDirectory, nil
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			return session.SessionTypeNewWorktree, nil
		case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
			return session.SessionTypeExistingWorktree, nil
		case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
			return session.SessionTypeNewProject, nil
		case sessionv1.SessionType_SESSION_TYPE_ONE_OFF:
			return session.SessionTypeOneOff, nil
		default:
			// Fail loudly instead of silently downgrading an unrecognized enum value
			// to SessionTypeDirectory -- this exact silent-substitution shape has
			// already caused prior incidents in this repo (see plan's Pattern
			// Decisions table / research/pitfalls.md §1).
			return "", fmt.Errorf("unrecognized session_type %v", msg.SessionType)
		}
	}
	if msg.ExistingWorktree != "" {
		return session.SessionTypeExistingWorktree, nil
	}
	if branch != "" {
		return session.SessionTypeNewWorktree, nil
	}
	return session.SessionTypeDirectory, nil
}

// enterpriseHosts unions statically-configured GitHub Enterprise hosts with hosts
// from dynamically-added accounts (gh CLI import, device auth) — mirrors
// ListGitHubAccounts' host union in github_user_service.go so every caller
// recognizes the same enterprise URLs the omnibar's detector does. CreateSession
// and PreviewDestinationPath both call this so the two never diverge.
func (s *SessionService) enterpriseHosts(cfg *config.Config) []string {
	configuredHosts := cfg.GetGitHubEnterpriseHosts()
	var cachedAccounts []githubpkg.CachedAccount
	if s.userPRCache != nil {
		cachedAccounts = s.userPRCache.GetCachedAccounts()
	}
	seenHosts := make(map[string]bool, len(configuredHosts)+len(cachedAccounts))
	hosts := make([]string, 0, len(configuredHosts)+len(cachedAccounts))
	addHost := func(host string) {
		host = githubpkg.NormalizeHost(host)
		if host == "" || githubpkg.IsGitHubCom(host) || seenHosts[host] {
			return
		}
		seenHosts[host] = true
		hosts = append(hosts, host)
	}
	for _, h := range configuredHosts {
		addHost(h.Host)
	}
	for _, a := range cachedAccounts {
		addHost(a.Host)
	}
	return hosts
}

// PreviewDestinationPath computes where a session's checkout/worktree would land
// without performing any git or filesystem mutation. Used by the Omnibar to show a
// live destination hint before the user submits session creation.
// +api: session:preview-destination-path
func (s *SessionService) PreviewDestinationPath(
	ctx context.Context,
	req *connect.Request[sessionv1.PreviewDestinationPathRequest],
) (*connect.Response[sessionv1.PreviewDestinationPathResponse], error) {
	cfg := config.LoadConfig()

	switch req.Msg.Mode {
	case "github_url":
		ref, err := session.ParseGitHubURLWithHosts(req.Msg.Input, s.enterpriseHosts(cfg))
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "not a recognized GitHub URL",
			}), nil
		}
		path := session.DefaultRepoPathManager.GetRepoPath(ref)
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    path,
			IsExact: true,
		}), nil

	case "new_worktree":
		if req.Msg.RepoPath == "" || req.Msg.SessionName == "" {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "repo_path and session_name are required",
			}), nil
		}
		prefix, err := git.PreviewWorktreePath(req.Msg.RepoPath, req.Msg.SessionName)
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: fmt.Sprintf("could not resolve repo path: %v", err),
			}), nil
		}
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    prefix,
			IsExact: false,
		}), nil

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown mode: %q", req.Msg.Mode))
	}
}
