package sessioncreate

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/remote"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// createRemoteNewWorktree creates the branch and worktree for a remote
// SessionTypeNewWorktree session. The base commit is resolved from resolvedPath
// rather than ambient HEAD (see Instance.newWorktreeFromResolvedBase).
func (h *Handler) createRemoteNewWorktree(
	ctx context.Context,
	msg *sessionv1.CreateSessionRequest,
	cfg *config.Config,
	resolvedRemote *instancepkg.RemoteTarget,
	runner tmux.CommandRunner,
	resolvedPath string,
	branch string,
) (remoteWorkingPath string, worktreeOps *git.RemoteWorktreeOps, remoteWT git.RemoteWorktree, warning string, outBranch string, err error) {
	if branch == "" {
		branch = cfg.BranchPrefix + git.SanitizeBranchName(msg.Title)
	}
	remoteWorkingPath = path.Join(resolvedRemote.BasePath, git.SanitizeBranchName(msg.Title))
	worktreeOps = h.remoteWorktreeOps(runner, resolvedRemote.Name)
	remoteWT = git.RemoteWorktree{RepoPath: resolvedPath, WorktreePath: remoteWorkingPath, Branch: branch}

	// baseSHA == "" with a nil error means an unborn repo, the one case ambient HEAD is safe to use.
	defaultBranch, baseSHA, resolveErr := git.ResolveRemoteWorktreeBaseCommit(ctx, runner, resolvedPath)
	if resolveErr != nil {
		return "", nil, git.RemoteWorktree{}, "", "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to resolve default branch on remote %q: %w", resolvedRemote.Name, resolveErr))
	}
	branchReq := backend.CreateBranchRequest{Name: backend.BranchName(branch)}
	if baseSHA != "" {
		branchReq.Base = backend.RefName(baseSHA)
		if diverged, ambientBranch := git.RemoteAmbientHEADDivergesFromBase(ctx, runner, resolvedPath, baseSHA); diverged {
			warning = git.FormatAmbientDivergenceWarning(resolvedPath, defaultBranch, ambientBranch)
		}
	}

	// Best-effort: "git branch <name> [sha]" failing because the branch already exists is expected.
	remoteLoc := backend.Remote{Host: backend.RemoteHost(resolvedRemote.Name), Path: backend.RemotePath(resolvedPath), Runner: runner}
	if branchErr := gitprovider.Backend().CreateBranch(ctx, remoteLoc, branchReq); branchErr != nil &&
		!strings.Contains(branchErr.Error(), "already exists") {
		return "", nil, git.RemoteWorktree{}, "", "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create branch %q on remote %q: %w", branch, resolvedRemote.Name, branchErr))
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
func (h *Handler) setupRemoteSessionTarget(
	ctx context.Context,
	msg *sessionv1.CreateSessionRequest,
	cfg *config.Config,
	resolvedRemote *instancepkg.RemoteTarget,
	remoteRequested bool,
	sessionType domain.SessionType,
	resolvedPath string,
	branch string,
	program string,
) (executionTarget instancepkg.ExecutionTarget, existingWorktreeOverride string, remoteCreationWarning string, outSessionType domain.SessionType, outBranch string, err error) {
	executionTarget = instancepkg.LocalTarget{}
	existingWorktreeOverride = msg.ExistingWorktree
	if !remoteRequested {
		return executionTarget, existingWorktreeOverride, "", sessionType, branch, nil
	}

	switch sessionType {
	case domain.SessionTypeNewWorktree, domain.SessionTypeExistingWorktree, domain.SessionTypeDirectory, domain.SessionTypeNewProject:
		// supported, handled below
	default:
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("remote sessions do not support session_type=%q (supported: new_worktree, existing_worktree, directory, new_project)", sessionType))
	}
	if sessionType == domain.SessionTypeExistingWorktree && msg.ExistingWorktree == "" {
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			errors.New("existing_worktree path is required for remote session_type=existing_worktree"))
	}
	if sessionType == domain.SessionTypeNewProject && resolvedPath == "" {
		// Local NewProject can leave resolvedPath empty and still resolve one later
		// via alias config (see the "path is required" exemption above and the
		// alias-resolution block below) -- but a remote NewProject has no such
		// alias-driven fallback for WHERE on the remote host to init, so this must
		// be resolvable synchronously, right here, or rejected with a clear error
		// rather than remote-initializing an empty/nonsense path.
		return nil, "", "", "", "", connect.NewError(connect.CodeInvalidArgument,
			errors.New("path is required for remote session_type=new_project"))
	}
	keyStore, knownHosts := h.d.RemoteStores()
	if keyStore == nil || knownHosts == nil {
		return nil, "", "", "", "", connect.NewError(connect.CodeFailedPrecondition,
			errors.New("remote session support is not configured on this server"))
	}

	target := tmux.SSHTarget{Name: resolvedRemote.Name, Addr: remote.RemoteAddr(resolvedRemote.Host)}
	clientConfig := ssh.ClientConfig{
		User:            resolvedRemote.User,
		Auth:            remote.ResolveIdentityAuthMethods(ctx, keyStore, resolvedRemote.IdentityRef),
		HostKeyCallback: knownHosts.HostKeyCallback(),
	}
	runner := tmux.NewSSHRunner(target, clientConfig, tmux.WithSSHClientPool(h.d.SSHClientPool()))
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
	case domain.SessionTypeNewWorktree:
		var newWTErr error
		remoteWorkingPath, worktreeOps, remoteWT, remoteCreationWarning, branch, newWTErr =
			h.createRemoteNewWorktree(ctx, msg, cfg, resolvedRemote, runner, resolvedPath, branch)
		if newWTErr != nil {
			return nil, "", "", "", "", newWTErr
		}
		createdWorktree = true
	case domain.SessionTypeExistingWorktree:
		remoteWorkingPath = msg.ExistingWorktree
	case domain.SessionTypeDirectory:
		remoteWorkingPath = resolvedPath
	case domain.SessionTypeNewProject:
		remoteWorkingPath = resolvedPath
		// No createdWorktree/rollback bookkeeping here, matching
		// InitializeProjectDirectory's own doc comment: unlike CreateWorktree
		// there is no cheap, safe remote rollback for a partial git-init (that
		// doc comment is explicit about not wanting an `rm -rf` embedded in a
		// remote script). A failure after this point can leave a
		// partially-initialized directory on the remote host for an operator
		// to inspect or retry against -- an accepted, documented tradeoff, not
		// an oversight.
		if initErr := h.remoteWorktreeOps(runner, resolvedRemote.Name).InitializeProjectDirectory(ctx, remoteWorkingPath); initErr != nil {
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
	// h.d.TmuxServerSocket must be applied here exactly as instance.go's
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
	if h.d.TmuxServerSocket != "" {
		remoteSession = tmux.NewTmuxSessionWithServerSocket(msg.Title, program, "staplersquad_", h.d.TmuxServerSocket, tmux.WithRegistry(nil), tmux.WithCommandRunner(runner))
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

	executionTarget = instancepkg.NewRemoteExecutionTarget(*resolvedRemote, runner)
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
	if sessionType != domain.SessionTypeDirectory && sessionType != domain.SessionTypeNewProject {
		sessionType = domain.SessionTypeExistingWorktree
	}
	existingWorktreeOverride = remoteWorkingPath

	return executionTarget, existingWorktreeOverride, remoteCreationWarning, sessionType, branch, nil
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
func (h *Handler) setupRemoteApprovalHooks(instance *instancepkg.Instance, rootDir, title string) error {
	if h.d.PermissionHandler() == nil {
		return errors.New("remote approval relay: no PermissionRequestHandler wired (SetPermissionRequestHandler was never called)")
	}
	remoteTarget, ok := instance.GetExecutionTarget().(instancepkg.RemoteExecutionTarget)
	if !ok {
		return fmt.Errorf("instance.IsRemote() true but ExecutionTarget is %T, not session.RemoteExecutionTarget", instance.GetExecutionTarget())
	}

	relay, err := sshremote.NewRemoteApprovalRelay(h.d.SSHClientPool(), h.d.PermissionHandler(), sshremote.RemoteApprovalRelayTarget{
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
	hookTarget := approvals.RemoteHookTarget{
		SocketPath:  sshremote.RemoteApprovalSocketPath(rootDir, instance.GetStableID()),
		BearerToken: token,
	}

	injectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := approvals.InjectHookConfigRemote(injectCtx, remoteTarget.Runner(), rootDir, instance.GetStableID(), hookTarget); err != nil {
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
func resolveRemoteTarget(msg *sessionv1.CreateSessionRequest, cfg *config.Config) (target *instancepkg.RemoteTarget, ok bool, err error) {
	if msg.GetRemote() == nil || msg.GetRemote().GetRemoteName() == "" {
		return nil, false, nil
	}
	remoteName := msg.GetRemote().GetRemoteName()
	remoteCfg, found := cfg.RemoteByName(remoteName)
	if !found {
		return nil, false, fmt.Errorf("no remote configured named %q", remoteName)
	}
	return &instancepkg.RemoteTarget{
		Name:        remoteCfg.Name,
		Host:        remoteCfg.Host,
		User:        remoteCfg.User,
		BasePath:    remoteCfg.BasePath,
		IdentityRef: remoteCfg.IdentityRef,
	}, true, nil
}

// remoteWorktreeOps builds the worktree ops for a remote runner, routing git through the
// host's backend. The runner passed here is the value that reaches Backend as Remote.Runner.
func (h *Handler) remoteWorktreeOps(runner tmux.CommandRunner, host string) *git.RemoteWorktreeOps {
	return git.NewRemoteWorktreeOps(runner, git.WithGitBackend(h.d.GitBackend()), git.WithRemoteHost(host))
}
