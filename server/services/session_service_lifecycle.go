package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// HibernateSession checkpoints the session state, kills the AI process, and
// transitions the session to Hibernated status.
// +api: session:hibernate
func (s *SessionService) HibernateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.HibernateSessionRequest],
) (*connect.Response[sessionv1.HibernateSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	instance := findInstanceByID(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Stop any running autonomous driver so it does not continue injecting prompts
	// into a session whose process is about to be killed.
	s.autonomousSvc.stopAndDeregisterDriver(instance.Title)

	// Set reason before transitioning so the After hook can read it
	reason := req.Msg.Reason
	if reason == "" {
		reason = "manual"
	}
	instance.SetHibernateReason(reason)

	if err := instance.Hibernate(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	// Remove from the live poller and review queue immediately so the hibernated
	// session does not linger with a stale queue entry until reconcileSessions fires.
	s.removeFromAllPollers(instance.Title)

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.HibernateSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// ResumeHibernatedSession re-launches the AI process for a Hibernated session,
// transitioning it back to Active status.
// +api: session:resume_hibernated
func (s *SessionService) ResumeHibernatedSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ResumeHibernatedSessionRequest],
) (*connect.Response[sessionv1.ResumeHibernatedSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	instance := findInstanceByID(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// This instance was loaded raw from storage above, bypassing
	// Registry.Acquire/WireInstanceCallbacks entirely, so it has no
	// MCPServerURL provider wired yet — without this, the relaunch below
	// would omit --mcp-config and the resumed session could never call
	// session-scoped MCP tools again.
	instance.SetMCPServerURLProvider(s.resolveMCPServerURL)

	if err := instance.ResumeFromHibernation(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeHibernatedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// ResumeCrashedSession re-launches the AI process for a Crashed session (dead
// tmux pane detected by SessionHealthChecker, session/health.go), transitioning
// it back to Active status. The tmux session was already killed when the
// instance was marked Crashed, so Start(false) takes the cold-restore path and
// threads --resume automatically when a conversation UUID is known.
// +api: session:resume_crashed
func (s *SessionService) ResumeCrashedSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ResumeCrashedSessionRequest],
) (*connect.Response[sessionv1.ResumeCrashedSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	instance := findInstanceByID(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// See the matching comment in ResumeHibernatedSession: this instance was
	// loaded raw from storage above and needs its MCPServerURL provider
	// wired explicitly before relaunch.
	instance.SetMCPServerURLProvider(s.resolveMCPServerURL)

	if err := instance.ResumeFromCrash(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeCrashedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// DeleteSession stops and removes a session, cleaning up resources.
// +api: session:delete
// cleanupPartialCreation idempotently tears down whatever subset of
// {tmux session, git worktree/clone directory} an instance has actually
// acquired, and is a no-op (returns nil) for any resource that never came
// into existence. It generalizes DeleteSession's original live-instance
// cleanup (previously a direct instance.Destroy() call) so Cancel (Epic 3.2)
// and Retry (Epic 3.3) can reuse the same already-solved edge cases instead
// of re-deriving them.
//
// instance.Destroy() alone is insufficient here: Destroy() short-circuits
// via its `!i.started.Load()` guard and returns nil without touching either
// resource whenever the instance never reached a fully-started Active state
// (session/instance.go's Destroy) — exactly the state a creation pipeline
// leaves an instance in when it fails or is cancelled after acquiring a
// worktree but before tmux startup completes. For a fully-started instance,
// this defers to Destroy() so DeleteSession's existing guarantees (VNC/CDP
// teardown, diff-stats capture, EventStopped, the destroyChainTimeout bound)
// are preserved unchanged.
//
// Both KillSession() and CleanupWorktree() are independently idempotent —
// they check HasSession()/HasWorktree() before doing anything — so calling
// this twice, or on an instance with zero, one, or both resources present,
// is always safe.
//
// Known accepted limitation (BUG-072, see plan.md's Domain Glossary): the
// underlying cleanup work (git worktree remove) is not itself
// context-bounded, so a hung removal can outlive a caller's timeout as an
// untracked goroutine. Not fixed here — this helper inherits that property
// from DeleteSession's existing chain rather than worsening it.
func cleanupPartialCreation(instance *session.Instance) error {
	if instance.Started() {
		return instance.Destroy()
	}

	// BUG-099: this branch never called Destroy(), so it never called
	// StopSessionDriver — leaving a SessionDriver goroutine the async
	// creation pipeline was still racing to start completely unstoppable.
	// Call it unconditionally, mirroring Destroy()'s own top-of-function
	// call, so any StartSessionDriver arriving after this point refuses to
	// start (driverDestroyed). See docs/bugs/fixed/BUG-099-*.md.
	session.StopSessionDriver(instance)

	// Re-check Started() immediately after StopSessionDriver: if a
	// concurrent Instance.Start() completed while we were stopping the
	// driver, the instance is now fully started and must go through
	// Destroy()'s full teardown (VNC/CDP, diff-stats, EventStopped) instead
	// of falling through to the defense-in-depth guard below, which is only
	// safe for an instance that never finished starting.
	if instance.Started() {
		return instance.Destroy()
	}

	// Task 3.1.1d: defense-in-depth liveness guard. instance.Started() is
	// only flipped true at the very end of Instance.Start() (session/instance.go's
	// startLocked, i.started.Store(true)) -- well after the worktree is set up
	// and the tmux session is actually launched (i.pm().Start(startPath)). A
	// crash in that window leaves an instance that reports Started()==false
	// (and whose persisted Status a caller like Cancel/Retry reads as
	// Creating/Failed) even though both resources genuinely came up healthy.
	// Re-check liveness immediately before each destructive step so that
	// narrow window doesn't cost real, working session state -- see
	// pre-mortem.md failure #2 (P1). This does not apply to the
	// instance.Started() branch above (DeleteSession's normal path for a
	// fully-started session): that branch must still unconditionally destroy
	// a live session the user explicitly asked to delete.
	var errs []error
	if instance.TmuxSessionExists() {
		log.Warn("cleanupPartialCreation: tmux session is alive despite instance.Started()==false; skipping kill (defense-in-depth guard)",
			"session", instance.Title)
	} else if err := instance.KillSession(); err != nil {
		errs = append(errs, fmt.Errorf("failed to kill tmux session: %w", err))
	}

	if worktreePath := instance.GetEffectiveRootDir(); instance.HasGitWorktree() && worktreeLooksAlive(worktreePath) {
		log.Warn("cleanupPartialCreation: worktree looks alive/healthy despite instance.Started()==false; skipping removal (defense-in-depth guard)",
			"session", instance.Title, "path", worktreePath)
	} else if err := instance.CleanupWorktree(); err != nil {
		errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
	}
	return errors.Join(errs...)
}

// worktreeLooksAlive is the "cheap git-status check" backstop Task 3.1.1d
// calls for: it confirms the directory exists and is a functioning git
// checkout, mirroring session/git's own IsDirtyWithHint check
// (`git status --porcelain`) rather than reinventing a second one.
//
// go-git was tried first per .claude/rules/prefer-go-git-over-subshells.md,
// but PlainOpenWithOptions(path, {DetectDotGit: true}).Head() reliably
// returns "reference not found" for a real `git worktree add` checkout
// (verified against a fresh worktree: go-git resolves the linked .git file
// and commondir correctly enough to open the repo, but not to resolve HEAD
// through it) -- worktrees are exactly what every resource this guard checks
// is. That's the specific, documented failure mode the fallback rule asks
// for, so this shells out instead, scoped to a short timeout like every
// other git subprocess call in this file (e.g. the origin/HEAD..HEAD count
// below).
func worktreeLooksAlive(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//nolint:norawgitcli // migrating, go-git-fork plan Epic 1.2 (route via session/git/backend)
	cmd := safeexec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain")
	return cmd.Run() == nil
}

func (s *SessionService) DeleteSession(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteSessionRequest],
) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Verify existence using raw data — no PTY side effects.
	// Match by Title OR UUID so that sessions created after UUID assignment are found correctly.
	dataSlice, err := s.storage.ListInstanceData()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
	}
	sessionTitle := ""
	sessionUUID := req.Msg.Id // fallback: use the supplied ID if UUID not found
	for _, d := range dataSlice {
		if d.Title == req.Msg.Id || d.UUID == req.Msg.Id {
			sessionTitle = d.Title
			if d.UUID != "" {
				sessionUUID = d.UUID
			}
			break
		}
	}
	if sessionTitle == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Stop any running autonomous driver before destroying resources.
	// This prevents the driver goroutine from calling inst.Preview() or SendCommandImmediate
	// on a freed/cleaned-up instance after the session is gone (use-after-delete hazard).
	s.autonomousSvc.stopAndDeregisterDriver(sessionTitle)

	// Capture the live instance BEFORE removing from pollers. removeFromAllPollers
	// (below) evicts this session from the ReviewQueuePoller's instance list — the
	// exact list FindLiveInstance searches — so calling FindLiveInstance after
	// removeFromAllPollers always returned nil here, silently skipping Destroy()'s
	// git worktree cleanup for every delete (live or not) in favor of the
	// tmux-only KillTmuxSessionByTitle fallback. Capturing the pointer first fixes
	// that without reopening the race the ordering comment below is about (that
	// race is between removeFromAllPollers and storage.DeleteInstance, not this).
	liveInst := s.FindLiveInstance(sessionTitle)

	// Refuse before any destructive step below if another live session's real
	// cwd is inside this session's worktree (deleting is Destroy()'s job,
	// reached only via the liveInst-found cleanup goroutine further down —
	// same guard as UpdateSession's pause/stop transitions and MCP's
	// pause_session/stop_session).
	if err := s.RefuseIfWorktreeSharedWithOtherLiveSession(liveInst); err != nil {
		return nil, err
	}

	// Fence out an in-flight Background Resolution Pipeline before cleanup,
	// bumping the epoch before re-reading status exactly like
	// CancelSessionCreation (see its doc comment for why that order, not the
	// reverse, is what resolves the race deterministically). Narrows, but
	// doesn't replace, the Snapshot()-based read fix.
	if liveInst != nil {
		liveInst.BumpCreationEpoch()
		if status, _ := liveInst.StatusAndFailureReason(); status == session.Creating {
			if cancelFunc := liveInst.CreationCancelFunc(); cancelFunc != nil {
				cancelFunc()
			}
		}
	}

	// Remove from all pollers BEFORE deleting from storage. This is atomic from the
	// poller's perspective and closes the race window where external discovery could
	// re-add the session between storage deletion and the old LoadInstances() reload.
	// Use sessionTitle (not req.Msg.Id) — pollers index by title, and req.Msg.Id may be a UUID.
	s.removeFromAllPollers(sessionTitle)

	// Destroy tmux/git resources asynchronously so the RPC returns immediately
	// after storage deletion. Cleanup errors are non-fatal — they are logged and
	// do not affect the success response the caller receives. Both goroutines are
	// tracked via deleteCleanupWG so Shutdown (and tests) can await them instead
	// of letting them outlive the process/test — see deleteCleanupWG's doc comment.
	if liveInst != nil {
		s.trackCleanup(func() {
			err := waitForDestroyLoggingSlowCleanup(func() error {
				return cleanupPartialCreation(liveInst)
			}, s.deleteSessionCleanupTimeout, func() {
				log.Warn("session cleanup still running in background after timeout", "session", req.Msg.Id, "timeout", s.deleteSessionCleanupTimeout)
			})
			if err != nil {
				log.Warn("failed to cleanup session resources", "session", req.Msg.Id, "err", err)
			}
			// Release this instance's actor from the SessionService-wide
			// *session.Registry (Epic 6.2's goleak hammer test caught this
			// missing entirely): Registry.Register (called from
			// CreateManagedInstance for every CreateSession) has no
			// corresponding release anywhere on the delete path, so every
			// deleted session otherwise leaked its runActor goroutine for the
			// life of the process. ForceRelease is a no-op if the registry
			// has no entry for this ID (nil s.registry, or a session that
			// predates registry wiring), and runs after cleanupPartialCreation
			// above has fully finished so it never stops the actor out from
			// under still-in-flight Destroy()/KillSession/CleanupWorktree work.
			if s.registry != nil {
				s.registry.ForceRelease(liveInst.GetStableID())
			}
		})
	} else {
		// Instance is not in the live in-memory poller (e.g. the server restarted
		// since this session was created). Fall back to killing the tmux session by
		// its deterministic name so the Claude process inside it doesn't survive as
		// an orphan after the DB record is gone.
		s.trackCleanup(func() {
			if err := killTmuxSessionByTitle(context.Background(), sessionTitle, true, []string{sessionUUID}); err != nil {
				log.Warn("failed to kill tmux session for non-live instance", "session", req.Msg.Id, "err", err)
			}
			// See the liveInst-present branch's comment above: release any
			// registry entry for this session even though it wasn't found in
			// the poller (e.g. the poller and registry can disagree in edge
			// cases -- FindLiveInstance only ever searches the poller). No-op
			// if the registry has no entry for sessionUUID.
			if s.registry != nil {
				s.registry.ForceRelease(sessionUUID)
			}
		})
	}

	// Cancel any pending approvals BEFORE deleting from storage, so blocked
	// approval-hook goroutines can exit cleanly while the session still exists.
	// Non-fatal: log at warn and continue even if there are no pending approvals.
	if cancelled := s.approvalStore.CancelSession(sessionUUID); len(cancelled) > 0 {
		log.Warn("cancelled pending approvals for deleted session", "session", req.Msg.Id, "count", len(cancelled))
	}

	// Delete from storage using Title (the storage key), not the client-supplied ID which may be a UUID.
	if err := s.storage.DeleteInstance(sessionTitle); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Publish SessionDeleted event to all watchers. Use UUID so the frontend
	// entity adapter (keyed by UUID) matches and tombstones the correct entry.
	s.onSessionDeletedForReply(sessionUUID)
	s.eventBus.Publish(events.NewSessionDeletedEvent(sessionUUID))

	return connect.NewResponse(&sessionv1.DeleteSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' deleted successfully", req.Msg.Id),
	}), nil
}

// CancelSessionCreation cancels an in-progress (Status == Creating) session
// creation (Epic 3.2, Story 3.2.1): it interrupts the Background Resolution
// Pipeline goroutine (if one still exists in this process), cleans up any
// partially-acquired resources via cleanupPartialCreation, and removes the
// instance entirely — a cancelled creation is deleted outright, not left as
// a Failed card (see the async-session-creation plan's Domain Glossary).
//
// Unlike DeleteSession, this does not publish a SessionDeletedEvent: per the
// plan's Epic 3.2 "Known asymmetry" note, that's flagged as adjacent, optional
// follow-up work (Task 3.2.1g), not a requirement of this RPC.
//
// +api: session:cancel-creation
func (s *SessionService) CancelSessionCreation(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelSessionCreationRequest],
) (*connect.Response[sessionv1.CancelSessionCreationResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	inst := s.FindLiveInstance(req.Msg.Id)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	if status, _ := inst.StatusAndFailureReason(); status != session.Creating {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %s is not in Creating status (current status=%s)", req.Msg.Id, status))
	}

	// Bump creationEpoch as its own actor mailbox round-trip BEFORE
	// re-reading status (Task 3.2.1c): this is what lets the two racing
	// writers -- this handler and the Background Resolution Pipeline's own
	// commitTerminalStatus -- resolve deterministically to exactly one
	// outcome. If the pipeline's TryForceStatusIfEpoch call executed inside
	// the actor mailbox before this bump, it already won with the pre-bump
	// epoch and the status read below observes Active; if it executes at or
	// after this bump, it observes a stale epoch and no-ops, leaving Status
	// == Creating for this handler to win instead. Skipping the bump (or
	// combining it with the read into one command) would reopen the race
	// ADR-002 closes.
	inst.BumpCreationEpoch()
	if status, _ := inst.StatusAndFailureReason(); status != session.Creating {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %s is no longer Creating (status=%s); the session finished creating before cancellation could apply", req.Msg.Id, status))
	}

	// Nil-safe: a post-restart instance's pipeline goroutine (and its
	// context.CancelFunc) is process-local and never persisted (ADR-002's
	// Consequences), so a nil CancelFunc here means there is no live
	// goroutine to interrupt in this process, not an error -- proceed
	// straight to cleanup/removal exactly as the live-cancel-func case does.
	if cancelFunc := inst.CreationCancelFunc(); cancelFunc != nil {
		cancelFunc()
	}

	if err := cleanupPartialCreation(inst); err != nil {
		log.Warn("CancelSessionCreation: failed to clean up partial creation resources", "session", req.Msg.Id, "err", err)
	}

	s.removeFromAllPollers(inst.Title)
	if err := s.storage.DeleteInstance(inst.Title); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Release this instance's actor from the SessionService-wide
	// *session.Registry -- see DeleteSession's identical comment (Epic 6.2's
	// goleak hammer test) for why this is required: without it, a cancelled
	// creation leaks its runActor goroutine exactly like a deleted session
	// did. cleanupPartialCreation above already ran synchronously (unlike
	// DeleteSession's backgrounded version), so it's safe to release here
	// immediately. No-op if s.registry has no entry for this ID.
	if s.registry != nil {
		s.registry.ForceRelease(inst.GetStableID())
	}

	RecordSessionCreationMetrics(ctx, "cancelled", time.Since(inst.CreatedAt))

	return connect.NewResponse(&sessionv1.CancelSessionCreationResponse{
		Success: true,
		Message: fmt.Sprintf("session creation '%s' cancelled", req.Msg.Id),
	}), nil
}

// RetrySessionCreation retries a Failed session creation in place (Epic 3.3,
// Story 3.3.1): TryStartRetry is the single validate-Failed + bump-epoch +
// reset-to-Creating operation, atomic inside one actor command (session/
// instance_actor_setters.go). If it reports started == false, the instance
// was no longer Failed by the time its command ran in the mailbox (a
// concurrent retry beat this one, or the instance finished/changed status
// on its own) -- this handler returns FailedPrecondition immediately,
// before touching cleanup or spawning anything, exactly as Task 3.3.1's
// double-click acceptance criterion requires.
//
// On started == true, this stops the outgoing attempt's pipeline goroutine
// (nil-tolerant, same rationale as CancelSessionCreation above), cleans up
// any partially-acquired resources via cleanupPartialCreation, then
// re-spawns the Background Resolution Pipeline against the SAME instance
// ID/storage row using the new epoch TryStartRetry returned. Only a
// SessionUpdatedEvent is published here (and again from within the
// pipeline as it progresses) -- never a second SessionCreatedEvent, so a
// retried instance never produces a duplicate card.
//
// +api: session:retry-creation
func (s *SessionService) RetrySessionCreation(
	ctx context.Context,
	req *connect.Request[sessionv1.RetrySessionCreationRequest],
) (*connect.Response[sessionv1.RetrySessionCreationResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	inst := s.FindLiveInstance(req.Msg.Id)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	newEpoch, started := inst.TryStartRetry()
	if !started {
		status, _ := inst.StatusAndFailureReason()
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %s is not in Failed status (current status=%s)", req.Msg.Id, status))
	}

	// Nil-safe: a post-restart instance's pipeline goroutine (and its
	// context.CancelFunc) is process-local and never persisted (ADR-002's
	// Consequences), so a nil CancelFunc here means there is no live
	// goroutine left to interrupt in this process -- proceed straight to
	// cleanup exactly as CancelSessionCreation's equivalent guard does.
	if cancelFunc := inst.CreationCancelFunc(); cancelFunc != nil {
		cancelFunc()
	}

	if err := cleanupPartialCreation(inst); err != nil {
		log.Warn("RetrySessionCreation: failed to clean up partial creation resources", "session", req.Msg.Id, "err", err)
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status", "creation_progress"}))

	instanceTitle := inst.Title
	instanceRootDir := inst.GetEffectiveRootDir()

	// Re-spawn the Background Resolution Pipeline against the same instance
	// ID/storage row with the new fencing epoch, tracked via trackCleanup
	// exactly like CreateSession's own goroutine dispatch above so Shutdown
	// (and tests) block until it finishes.
	//
	// Known limitation: unlike CreateSession's synchronous prefix, this
	// handler has no request message to re-derive deferredGitHubURL/
	// deferredGitHubSourceURL/deferredEnterpriseHosts from -- those are not
	// persisted on the instance (Story 3.3.1 is scoped to the RPC handler
	// around TryStartRetry, not the pipeline's deferred-GitHub-URL plumbing).
	// A retry therefore always re-runs as a non-deferred-GitHub-URL pipeline;
	// retrying a session that originally failed during GitHub URL resolution
	// itself (FailureReason == GitHubResolutionError, before any
	// GitHubResolution was ever recorded on the instance) is a known gap, not
	// covered by this story's tests.
	s.trackCleanup(func() {
		s.runBackgroundResolutionPipeline(ctx, creationPipelineParams{
			instance:        inst,
			epoch:           newEpoch,
			instanceTitle:   instanceTitle,
			instanceRootDir: instanceRootDir,
		})
	})

	return connect.NewResponse(&sessionv1.RetrySessionCreationResponse{
		Success: true,
		Message: fmt.Sprintf("session creation '%s' retrying", req.Msg.Id),
	}), nil
}

// removeFromAllPollers removes the session from the review queue and all pollers.
// Call this before deleting from storage to close the race window where LoadInstances()
// could re-add the session via external discovery.
func (s *SessionService) removeFromAllPollers(id string) {
	s.unindexSessionForDelivery(id)
	if s.reviewQueueSvc != nil {
		s.reviewQueueSvc.GetQueue().Remove(id)
	}
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.RemoveInstance(id)
	}
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.RemoveInstance(id)
	}
	// Remove from HistoryLinker so the shutdown hook cannot re-persist a
	// deleted session via historyLinker.Instances() → SaveInstances().
	if s.historyLinker != nil {
		s.historyLinker.RemoveInstance(id)
	}
}

// RemoveFromAllPollers is the exported version for use by MCP tools and other
// callers outside the services package that need to clean up after deletion.
func (s *SessionService) RemoveFromAllPollers(id string) {
	s.removeFromAllPollers(id)
}

// RestartSession restarts a session by killing and recreating the tmux session.
// Optionally preserves terminal output for debugging purposes.
func (s *SessionService) RestartSession(
	ctx context.Context,
	req *connect.Request[sessionv1.RestartSessionRequest],
) (*connect.Response[sessionv1.RestartSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Prefer the live in-memory instance so Restart() sees the real started/status
	// state and avoids the LoadInstances side-effect of hot-restoring every session.
	instance := s.FindLiveInstance(req.Msg.Id)
	if instance == nil {
		// Fallback: session not yet tracked by the poller (e.g. brand-new or test).
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		instance = findInstanceByID(instances, req.Msg.Id)
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Restart ends the agent and types a marker into the new pane; a hidden
	// (background) session is read-only for UI actions. Internal restarts
	// (retry, program switch) do not come through this handler.
	if _, err := AccessForUnary(instance, s.guards).Writer(nil); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is a background session and is read-only: %w", req.Msg.Id, err))
	}

	// Restart the instance
	if err := instance.Restart(req.Msg.PreserveOutput); err != nil {
		log.Error("[RestartSession] failed to restart session", "session", instance.Title, "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session: %w", err))
	}

	RefreshInstanceHookProof(instance)

	// Persist the updated instance state.
	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save restarted instance: %w", err))
	}

	// Publish SessionUpdated event
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "updated_at"}))

	message := fmt.Sprintf("Session '%s' restarted successfully", instance.Title)
	if req.Msg.PreserveOutput {
		message += " (terminal output preserved)"
	}

	log.Info(message)

	return connect.NewResponse(&sessionv1.RestartSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
		Success: true,
		Message: message,
	}), nil
}

// RetrySession immediately restarts a session's configurable retry policy
// (session-retry-backoff), bypassing any pending backoff delay — including
// from SESSION_STATUS_PERMANENTLY_FAILED. Clones RestartSession's
// instance-lookup/persist/publish shape, calling inst.RetryNow() instead of
// inst.Restart().
// +api: session:retry
func (s *SessionService) RetrySession(
	ctx context.Context,
	req *connect.Request[sessionv1.RetrySessionRequest],
) (*connect.Response[sessionv1.RetrySessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance := s.FindLiveInstance(req.Msg.Id)
	if instance == nil {
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		instance = findInstanceByID(instances, req.Msg.Id)
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	if err := instance.RetryNow(instance.GetEffectiveRootDir()); err != nil {
		if errors.Is(err, session.ErrRetryInFlight) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		log.Error("[RetrySession] failed to retry session", "session", instance.Title, "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retry session: %w", err))
	}

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save retried instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "updated_at", "retry_attempt"}))

	message := fmt.Sprintf("Session '%s' retry started", instance.Title)
	log.Info(message)

	return connect.NewResponse(&sessionv1.RetrySessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
		Success: true,
		Message: message,
	}), nil
}

// ─── One-Shot ─────────────────────────────────────────────────────────────────

// +api: session:run-one-shot
// RunOneShot executes `claude -p <prompt>` in the session's worktree and returns
// the combined output along with an extracted PR URL and branch divergence status.
func (s *SessionService) RunOneShot(
	ctx context.Context,
	req *connect.Request[sessionv1.RunOneShotRequest],
) (*connect.Response[sessionv1.RunOneShotResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Prompt == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("prompt is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	workDir := inst.GetEffectiveRootDir()
	if workDir == "" {
		workDir = inst.Path
	}

	// Clamp timeout: default 900 s (raised from 120 s for longer operations), max 1800 s.
	timeoutSecs := int(req.Msg.TimeoutSeconds)
	if timeoutSecs <= 0 {
		timeoutSecs = 900
	}
	if timeoutSecs > 1800 {
		timeoutSecs = 1800
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	var outputStr string
	exitCode := 0
	errMsg := ""

	if s.headlessPool != nil {
		// Use headless pool for improved streaming and session reuse.
		sink := headless.DiscardCost
		if concreteStorage := s.GetStorage(); concreteStorage != nil {
			sink = session.CostSinkForSessionUUID(concreteStorage, inst.UUID)
		}
		var callErr error
		var oneShot headless.PoolClient = s.headlessPool
		if s.headlessClient != nil {
			oneShot = s.headlessClient
		}
		outputStr, callErr = oneShot.CallBlocking(runCtx, headless.FeatureKeyCustom, "", req.Msg.Prompt, headless.CallOptions{WorkDir: workDir}, sink)
		if callErr != nil {
			errMsg = callErr.Error()
			exitCode = 1
		}
	} else {
		// Fallback: direct subprocess (requires claude in PATH).
		claudeBin, err := exec.LookPath("claude")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claude binary not found in PATH: %w", err))
		}

		cmd := safeexec.CommandContext(runCtx, claudeBin, "-p", req.Msg.Prompt)
		cmd.Dir = workDir

		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				errMsg = runErr.Error()
			}
		}
		outputStr = string(output)
	}
	prURL := extractPRURL(outputStr)
	branchDiverged := checkBranchDivergence(workDir)

	// Persist the PR URL (and number) back to the session record so the GitHub badge appears
	// and the PRStatusPoller can use the direct-number path instead of branch-name discovery.
	if prURL != "" {
		prNumber := 0
		if ref, parseErr := session.ParseGitHubURL(prURL); parseErr == nil && ref.PRNumber > 0 {
			prNumber = ref.PRNumber
		}
		inst.SetGitHubPR(prURL, prNumber)
		if err := s.storage.SaveInstances(s.allInstances()); err != nil {
			log.Warn("RunOneShot: failed to persist PR URL", "session", inst.Title, "err", err)
		} else {
			s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))
		}

		// This RunOneShot call may have been the Review Queue's manual "Create
		// PR" button for a backlog-linked session — that flow creates the PR
		// entirely outside the automated pushAndCreatePR path, which is the
		// only other place that ever moves a backlog item to pr_pending. Without
		// this call the item is silently left in "review" forever, invisible to
		// ReconcilePRPending (see RecordPRCreatedOutOfBand's doc comment in
		// session/backlog_lifecycle.go for the full root-cause trace). No-op for
		// non-backlog sessions.
		if s.backlogLifecycleListener != nil {
			s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID, prURL, prNumber)
		}
	}

	return connect.NewResponse(&sessionv1.RunOneShotResponse{
		Output:                 outputStr,
		Error:                  errMsg,
		ExitCode:               int32(exitCode),
		PrUrl:                  prURL,
		BranchDivergedFromBase: branchDiverged,
	}), nil
}

// RunOneShotForSession runs a one-shot prompt against a session's worktree without
// the ConnectRPC request/response wrapper, for automation callers. It reuses
// RunOneShot's exact logic (same PR-URL extraction, same PR persistence) so
// automated and manual PR creation share one code path — currently used by the
// opt-in AutoCreatePR review-queue policy (server.ReactiveQueueManager).
// Returns the extracted PR URL, or an error if the prompt failed.
func (s *SessionService) RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error) {
	resp, err := s.RunOneShot(ctx, connect.NewRequest(&sessionv1.RunOneShotRequest{
		SessionId:      sessionID,
		Prompt:         prompt,
		TimeoutSeconds: timeoutSeconds,
	}))
	if err != nil {
		return "", err
	}
	if resp.Msg.Error != "" {
		return "", fmt.Errorf("one-shot prompt failed: %s", resp.Msg.Error)
	}
	return resp.Msg.PrUrl, nil
}

// extractPRURL scans the last 10 non-empty lines of output for a GitHub PR URL
// of the form https://github.com/…/pull/NNN.
func extractPRURL(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	for i := len(lines) - 1; i >= start; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "github.com/") && strings.Contains(line, "/pull/") {
			for _, word := range strings.Fields(line) {
				if strings.Contains(word, "github.com/") && strings.Contains(word, "/pull/") {
					return strings.Trim(word, ".,;:\"'()")
				}
			}
		}
	}
	return ""
}

// checkBranchDivergence returns true when the current branch has commits not
// present on origin/HEAD (i.e., the branch has diverged / is ahead).
func checkBranchDivergence(workDir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//nolint:norawgitcli // migrating, go-git-fork plan Epic 1.2 (route via session/git/backend)
	cmd := safeexec.CommandContext(ctx, "git", "rev-list", "--count", "origin/HEAD..HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	count := strings.TrimSpace(string(out))
	return count != "" && count != "0"
}

// +api: session:unarchive
// UnarchiveSession clears archived_at, restoring the session to the default list.
func (s *SessionService) UnarchiveSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UnarchiveSessionRequest],
) (*connect.Response[sessionv1.UnarchiveSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.FindLiveInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}
	inst.SetArchivedAt(nil)
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
	}
	return connect.NewResponse(&sessionv1.UnarchiveSessionResponse{}), nil
}

// +api: session:pin
// PinSession pins a session so it surfaces in the dedicated Pinned section.
// Idempotent. Archived sessions are rejected (archiving always auto-unpins).
func (s *SessionService) PinSession(
	ctx context.Context,
	req *connect.Request[sessionv1.PinSessionRequest],
) (*connect.Response[sessionv1.PinSessionResponse], error) {
	if err := s.setPinned(req.Msg.SessionId, true); err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionv1.PinSessionResponse{}), nil
}

// +api: session:unpin
// UnpinSession clears the pinned flag, restoring normal grouped position. Idempotent.
func (s *SessionService) UnpinSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UnpinSessionRequest],
) (*connect.Response[sessionv1.UnpinSessionResponse], error) {
	if err := s.setPinned(req.Msg.SessionId, false); err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionv1.UnpinSessionResponse{}), nil
}

func (s *SessionService) setPinned(sessionID string, pinned bool) error {
	if sessionID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.FindLiveInstance(sessionID)
	if inst == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", sessionID))
	}
	if err := inst.SetPinned(pinned); err != nil {
		if errors.Is(err, session.ErrCannotPinArchivedSession) {
			return connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update pin: %w", err))
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
	}
	// Push to other browsers' WatchSessions streams so a pin made in one appears in all.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"pinned"}))
	return nil
}

// +api: session:archive-workflow-sessions
// ArchiveWorkflowSessions delegates to WorkflowService.
func (s *SessionService) ArchiveWorkflowSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveWorkflowSessionsRequest],
) (*connect.Response[sessionv1.ArchiveWorkflowSessionsResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.ArchiveWorkflowSessions(ctx, req)
}

// +api: session:delete-workflow-failed-sessions
// DeleteWorkflowFailedSessions delegates to WorkflowService.
func (s *SessionService) DeleteWorkflowFailedSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteWorkflowFailedSessionsRequest],
) (*connect.Response[sessionv1.DeleteWorkflowFailedSessionsResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.DeleteWorkflowFailedSessions(ctx, req)
}

// maybeAutoArchive archives a workflow- or backlog-spawned session that has just
// stopped. Called in the status-update path whenever a session transitions to
// Stopped. Only archives sessions spawned by a workflow (WorkflowID != "") or by
// the backlog automation pipeline (see IsBacklogOriginatedSession) — this is a
// fallback safety net for backlog sessions specifically, since
// BacklogService.archiveItemWorkSessions is the primary archival path for those
// but only fires on the owning item's terminal/rework transitions, not on the
// session's own exit.
// If the workflow has archive_after_hours > 0, the retention enforcer handles
// time-delayed archival, so we skip immediate archival here (ADR-4). Backlog
// sessions have no such delayed-archival config, so that check is workflow-only.
func (s *SessionService) maybeAutoArchive(inst *session.Instance) {
	if inst == nil {
		return
	}
	if inst.WorkflowID == "" && !inst.IsBacklogOriginatedSession() {
		return
	}
	if inst.WorkflowID != "" {
		// Check if this workflow uses delayed archival via the retention enforcer.
		s.workflowMetaMu.RLock()
		meta, ok := s.workflowMetaCache[inst.WorkflowID]
		s.workflowMetaMu.RUnlock()
		if ok && meta.archiveAfterHours > 0 {
			// Retention enforcer will archive this after the configured delay.
			return
		}
	}
	now := time.Now()
	// CAS: set ArchivedAt only if still nil. Prevents double-archive from concurrent EventExited fires.
	if !inst.SetArchivedAtIfNil(now) {
		return
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		log.Warn("[SessionService] failed to auto-archive workflow session",
			"session", inst.Title, "workflow_id", inst.WorkflowID, "err", err)
	}
}

// ReapPausedTmuxSessions kills any tmux session that is still running for a paused
// Instance. This is a safety net for sessions paused before the kill-on-pause change,
// or for cases where the initial kill attempt fell back to detach.
func (s *SessionService) ReapPausedTmuxSessions() {
	if s.reviewQueuePoller == nil {
		return
	}
	instances := s.reviewQueuePoller.GetInstances()
	for _, inst := range instances {
		if !inst.IsPaused() {
			continue
		}
		if err := inst.KillSession(); err != nil {
			log.Warn("[TmuxReaper] failed to kill tmux for paused session",
				"session", inst.Title, "err", err)
		}
	}
}
