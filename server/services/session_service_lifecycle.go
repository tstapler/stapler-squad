package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/sessioncreate"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	"github.com/tstapler/stapler-squad/session/headless"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/markerstore"
)

func (s *SessionService) findLifecycleInstance(instances []*instancepkg.Instance, id string) *instancepkg.Instance {
	if s.registry != nil {
		if live := s.registry.Lookup(id); live != nil {
			return live.Instance
		}
	}
	if live := s.FindLiveInstance(id); live != nil {
		return live
	}
	return findInstanceByID(instances, id)
}

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

	instance := s.findLifecycleInstance(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Retain before transitioning: poller removal must not discard the teardown barrier.
	if s.registry != nil {
		instance = s.registry.Retain(instance).Instance
	}

	// Stop any running autonomous driver so it does not continue injecting prompts
	// into a session whose process is about to be killed.
	s.AutonomousSvc.StopDriverForSession(instance.Title)

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

	if err := s.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
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

	instance := s.findLifecycleInstance(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Preserve the canonical instance's in-flight hibernation teardown barrier.
	instance.SetMCPServerURLProvider(s.resolveMCPServerURL)

	if err := instance.ResumeFromHibernation(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeHibernatedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// ResumeCrashedSession re-launches the AI process for a Crashed session (dead
// tmux pane detected by SessionHealthChecker, session/autonomous/health.go), transitioning
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

	if err := s.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeCrashedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// DeleteSession stops and removes a session, cleaning up resources.
// +api: session:delete
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
	sessionTitle, tmuxTitle := "", ""
	sessionUUID := req.Msg.Id // fallback: use the supplied ID if UUID not found
	for _, d := range dataSlice {
		if d.Title == req.Msg.Id || d.UUID == req.Msg.Id {
			sessionTitle, tmuxTitle = d.Title, d.TmuxNameTitle()
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
	s.AutonomousSvc.StopDriverForSession(sessionTitle)

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

	// Refuse before the storage record goes away: the cleanup below runs after the delete
	// has already been acknowledged, so a dirty worktree it declined to remove would be
	// orphaned. Only force discards uncommitted work.
	if liveInst != nil && !req.Msg.Force {
		if err := liveInst.CheckWorktreeRemovable(); err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
	}

	// Fence out an in-flight Background Resolution Pipeline before cleanup,
	// bumping the epoch before re-reading status exactly like
	// CancelSessionCreation (see its doc comment for why that order, not the
	// reverse, is what resolves the race deterministically). Narrows, but
	// doesn't replace, the Snapshot()-based read fix.
	if liveInst != nil {
		liveInst.BumpCreationEpoch()
		if status, _ := liveInst.StatusAndFailureReason(); status == domain.Creating {
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
				return sessioncreate.CleanupPartialCreation(liveInst, req.Msg.Force)
			}, s.deleteSessionCleanupTimeout, func() {
				log.Warn("session cleanup still running in background after timeout", "session", req.Msg.Id, "timeout", s.deleteSessionCleanupTimeout)
			})
			if err != nil {
				log.Warn("failed to cleanup session resources", "session", req.Msg.Id, "err", err)
			}
			// Release this instance's actor from the SessionService-wide
			// *registry.Registry (Epic 6.2's goleak hammer test caught this
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
			if err := killTmuxSessionByTitle(context.Background(), tmuxTitle, true, []string{sessionUUID}); err != nil {
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
	s.replyService().SessionDeleted(sessionUUID)
	s.eventBus.Publish(events.NewSessionDeletedEvent(sessionUUID))

	return connect.NewResponse(&sessionv1.DeleteSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' deleted successfully", req.Msg.Id),
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
	if s.ReviewQueuePoller != nil {
		s.ReviewQueuePoller.RemoveInstance(id)
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
	if _, err := access.AccessForUnary(instance, s.guards).Writer(nil); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is a background session and is read-only: %w", req.Msg.Id, err))
	}

	// Restart the instance, migrating backends first if BackendOverride names
	// a different one — see RestartSessionRequest.backend_override's doc
	// comment and session.Instance.Restart's migrating branch.
	if err := instance.Restart(req.Msg.PreserveOutput, domain.ProcessManagerBackend(req.Msg.GetBackendOverride())); err != nil {
		log.Error("[RestartSession] failed to restart session", "session", instance.Title, "err", err)
		if errors.Is(err, instancepkg.ErrUnrecognizedBackend) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session: %w", err))
	}

	approvals.RefreshInstanceHookProof(instance)

	// Persist the updated instance state.
	if err := s.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save restarted instance: %w", err))
	}
	s.concStorage.RecordManualNudgeForSession(ctx, instance.GetStableID(), markerstore.ManualNudgeRestart)

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

	// Retry revives the session, so an archived (deliberately retired) one must be unarchived first.
	if instance.IsArchived() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session %s is archived; unarchive it before retrying", req.Msg.Id))
	}

	if err := instance.RetryNow(instance.GetEffectiveRootDir()); err != nil {
		if errors.Is(err, instancepkg.ErrRetryInFlight) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		log.Error("[RetrySession] failed to retry session", "session", instance.Title, "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retry session: %w", err))
	}

	// Publish before saving: the retry already took effect in memory, so watchers (e.g. a
	// read-only terminal stream waiting for Active) must hear it even if persistence fails.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "updated_at", "retry_attempt"}))

	if err := s.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save retried instance: %w", err))
	}

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
		if ref, parseErr := instancepkg.ParseGitHubURL(prURL); parseErr == nil && ref.PRNumber > 0 {
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
	ahead, err := gitprovider.Backend().CountCommits(ctx, backend.Local{Root: backend.RepoRoot(workDir)},
		backend.RangeSpec{Exclude: "origin/HEAD", Include: "HEAD"})
	return err == nil && ahead > 0
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
	// Tell watchers (terminal streams that attached read-only while archived) before saving.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"archived_at"}))
	if err := s.storage.SaveInstances([]*instancepkg.Instance{inst}); err != nil {
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
		if errors.Is(err, instancepkg.ErrCannotPinArchivedSession) {
			return connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update pin: %w", err))
	}
	if err := s.storage.SaveInstances([]*instancepkg.Instance{inst}); err != nil {
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
func (s *SessionService) maybeAutoArchive(inst *instancepkg.Instance) {
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
	if err := s.storage.SaveInstances([]*instancepkg.Instance{inst}); err != nil {
		log.Warn("[SessionService] failed to auto-archive workflow session",
			"session", inst.Title, "workflow_id", inst.WorkflowID, "err", err)
	}
	s.releaseArchivedProcesses(context.Background(), inst)
}

// ReapPausedTmuxSessions kills any tmux session that is still running for a paused
// Instance. This is a safety net for sessions paused before the kill-on-pause change,
// or for cases where the initial kill attempt fell back to detach.
func (s *SessionService) ReapPausedTmuxSessions() {
	if s.ReviewQueuePoller == nil {
		return
	}
	instances := s.ReviewQueuePoller.GetInstances()
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
