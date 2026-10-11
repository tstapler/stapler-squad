package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// classifyPauseResumeErr maps a Pause()/Resume() error to the appropriate connect
// error code. Permission and state-machine rejections are the caller's fault
// (FailedPrecondition, not a 500); anything else is an unexpected operational
// failure (git/tmux errors) and stays CodeInternal.
func classifyPauseResumeErr(err error, opDesc string) *connect.Error {
	var transErr session.ErrInvalidTransition
	if errors.As(err, &transErr) ||
		errors.Is(err, session.ErrPauseNotPermitted) ||
		errors.Is(err, session.ErrResumeNotPermitted) ||
		errors.Is(err, session.ErrDirtyStateUnknown) {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("failed to %s session: %w", opDesc, err))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s session: %w", opDesc, err))
}

// classifyStopErr maps a StopByUser() error to the appropriate connect error code,
// using the same permission/state-machine-rejection-vs-operational-failure split as
// classifyPauseResumeErr.
func classifyStopErr(err error, opDesc string) *connect.Error {
	var transErr session.ErrInvalidTransition
	if errors.As(err, &transErr) || errors.Is(err, session.ErrPauseNotPermitted) ||
		errors.Is(err, session.ErrDirtyStateUnknown) {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("failed to %s session: %w", opDesc, err))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s session: %w", opDesc, err))
}

// applyTitleRename renames instance to msg.Title's in-memory title when it
// differs from the current one, rejecting the request if another instance in
// instances already uses that title. Returns false (no error) when msg.Title
// is nil, empty, or unchanged -- a no-op the caller treats as "nothing to
// record".
func applyTitleRename(msg *sessionv1.UpdateSessionRequest, instance *session.Instance, instances []*session.Instance) (bool, error) {
	if msg.Title == nil || *msg.Title == "" || *msg.Title == instance.Title {
		return false, nil
	}
	for _, inst := range instances {
		if inst.Title == *msg.Title {
			return false, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", *msg.Title))
		}
	}
	instance.SetTitleDirect(*msg.Title)
	return true, nil
}

// applyTagsUpdate sets instance's tags from msg.Tags. In proto3, an empty
// repeated field is indistinguishable from "not provided", so clients send
// tags=[""] to clear all tags. Returns false (no error) when msg.Tags is
// empty -- a no-op the caller treats as "nothing to record".
func applyTagsUpdate(msg *sessionv1.UpdateSessionRequest, instance *session.Instance) (bool, error) {
	if len(msg.Tags) == 0 {
		return false, nil
	}
	tags := msg.Tags
	if len(tags) == 1 && tags[0] == "" {
		tags = nil // Clear all tags
	}
	if err := instance.SetTags(tags); err != nil {
		return false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tags: %w", err))
	}
	return true, nil
}

// applyStatusTransition handles a pause/resume/stop request against
// instance's current status: stop (when not already stopped), pause (when
// not already paused, recording PauseReason), or resume (when currently
// paused and the target isn't Paused). Each transition first clears with
// RefuseIfWorktreeSharedWithOtherLiveSession where applicable (stop/pause,
// not resume) and maps a rejection to the appropriate connect error code via
// classifyStopErr/classifyPauseResumeErr. Returns false (no error) when
// msg.Status is unset/UNSPECIFIED or already matches instance's current
// status -- a no-op the caller treats as "nothing to record".
func (s *SessionService) applyStatusTransition(msg *sessionv1.UpdateSessionRequest, instance *session.Instance) (bool, error) {
	if msg.Status == nil || *msg.Status == sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
		return false, nil
	}
	targetStatus := adapters.ProtoToStatus(*msg.Status)

	switch {
	case targetStatus == session.Stopped && instance.Status != session.Stopped:
		if err := s.RefuseIfWorktreeSharedWithOtherLiveSession(instance); err != nil {
			return false, err
		}
		if err := instance.StopByUser(); err != nil {
			return false, classifyStopErr(err, "stop")
		}
		return true, nil
	case targetStatus == session.Paused && instance.Status != session.Paused:
		if err := s.RefuseIfWorktreeSharedWithOtherLiveSession(instance); err != nil {
			return false, err
		}
		if err := instance.Pause(); err != nil {
			return false, classifyPauseResumeErr(err, "pause")
		}
		// Set pause reason after a successful transition — mirrors HibernateSession
		// pattern, and avoids stamping the reason on a request that got rejected
		// (permission denied, invalid transition).
		if msg.PauseReason == nil || *msg.PauseReason == "" {
			instance.SetPauseReason(session.PauseReasonManual)
		} else {
			instance.SetPauseReason(*msg.PauseReason)
		}
		return true, nil
	case targetStatus != session.Paused && instance.Status == session.Paused:
		// Resume from paused state
		if err := instance.Resume(); err != nil {
			return false, classifyPauseResumeErr(err, "resume")
		}
		// Clear pause reason only after a successful resume.
		instance.SetPauseReason("")
		return true, nil
	default:
		return false, nil
	}
}

// UpdateSession modifies session properties (pause/resume, category, title).
// +api: session:update
func (s *SessionService) UpdateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateSessionRequest],
) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Use the live poller list to avoid LoadInstances side-effects (Start() on Active
	// sessions) that can silently drop sessions if tmux is unavailable, which would then
	// clobber the poller's complete list via SetInstances.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var loadErr error
		instances, loadErr = s.loadInstancesWithWiring()
		if loadErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", loadErr))
		}
	}

	// Find the instance to update
	instance := findInstanceByID(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Captured before any rename below mutates instance.Title in-memory. A narrow
	// metadata update must key its WHERE clause off the pre-rename title, or it misses
	// the DB row entirely once instance.Title has already moved to the new value.
	currentTitle := instance.Title

	// Validate the note length before any field below mutates live in-memory state
	// (SetTitleDirect/SetCategory publish immediately via snapshot.Store, not staged
	// until SaveInstances) — otherwise a rejected request could still leave title/category
	// changes visible to concurrent readers.
	if req.Msg.Note != nil && len(*req.Msg.Note) > session.MaxNoteLength {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("note exceeds maximum length of %d bytes", session.MaxNoteLength))
	}

	// Track which fields are being updated for event publishing
	var updatedFields []string

	// Metadata fields (title/category/note/working_dir) persist via a single narrow
	// UPDATE (UpdateInstanceMetadata) instead of the full-row SaveInstances rewrite.
	// A non-nil pointer here means "this field was part of the request".
	var metaTitle, metaCategory, metaNote, metaWorkingDir *string

	// sideEffectChanged tracks fields (tags, status, rate_limit_enabled, autonomous_mode)
	// that still need the full-row SaveInstances write — e.g. tags requires managing the
	// tags M2M relation, which a narrow column UPDATE can't replicate.
	var sideEffectChanged bool

	// Handle title update (before status change so rename is atomic with resume)
	if renamed, err := applyTitleRename(req.Msg, instance, instances); err != nil {
		return nil, err
	} else if renamed {
		updatedFields = append(updatedFields, "title")
		metaTitle = req.Msg.Title
	}

	// Handle category update
	if req.Msg.Category != nil {
		instance.SetCategory(*req.Msg.Category)
		updatedFields = append(updatedFields, "category")
		metaCategory = req.Msg.Category
	}

	// Handle note update. Length already validated above.
	if req.Msg.Note != nil {
		instance.SetNote(*req.Msg.Note)
		updatedFields = append(updatedFields, "note")
		metaNote = req.Msg.Note
	}

	// Handle tags update.
	if updated, err := applyTagsUpdate(req.Msg, instance); err != nil {
		return nil, err
	} else if updated {
		updatedFields = append(updatedFields, "tags")
		sideEffectChanged = true
	}

	// Handle program update. Empty string means "System default" — resolve to the
	// configured default so the DB NotEmpty constraint is satisfied. Consolidated with
	// the capacity-monitor auto-fallback path (UpdateSessionProgram below) via
	// Instance.SwitchProgram so the two entry points can't drift or double-restart.
	if req.Msg.Program != nil {
		// Flush any pending title/category/note rename now, keyed on currentTitle,
		// before SwitchProgram's callback below can trigger its own SaveInstances
		// call. That call persists via instance.ToInstanceData(), whose Title is
		// already the in-memory-renamed value — looking the DB row up by that new
		// title (before the narrow rename below has run) misses the still-old-titled
		// row and duplicates it via saveInstancesToRepo's Create fallback, exactly
		// the orphaned/duplicate-row bug this file's UpdateSessionMetadata exists to
		// avoid. Flushing here first keeps every later persist call in this handler
		// looking up the same, already-correct row.
		if metaTitle != nil || metaCategory != nil || metaNote != nil {
			if err := s.storage.UpdateInstanceMetadata(currentTitle, metaTitle, metaCategory, metaNote, nil); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
			}
			if metaTitle != nil {
				currentTitle = *metaTitle
			}
			metaTitle, metaCategory, metaNote = nil, nil, nil
		}
		changed, _, switchErr := instance.SwitchProgram(ctx, *req.Msg.Program, func() error {
			return s.storage.SaveInstances([]*session.Instance{instance})
		})
		if changed {
			updatedFields = append(updatedFields, "program")
		}
		if switchErr != nil {
			log.Error("[UpdateSession] failed to restart session after program change", "session", instance.Title, "err", switchErr)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session after program change: %w", switchErr))
		}
	}

	// Handle working directory update
	if req.Msg.WorkingDir != nil {
		instance.SetWorkingDir(*req.Msg.WorkingDir)
		updatedFields = append(updatedFields, "working_dir")
		metaWorkingDir = req.Msg.WorkingDir
	}

	// Handle rate limit enabled toggle. SetRateLimitEnabled persists to the
	// struct field. Also apply to the live poller instance (which has a running
	// controller) so the change takes effect immediately without a restart.
	if req.Msg.RateLimitEnabled != nil {
		instance.SetRateLimitEnabled(*req.Msg.RateLimitEnabled)
		if s.reviewQueuePoller != nil {
			if liveInst := s.reviewQueuePoller.FindInstance(req.Msg.Id); liveInst != nil {
				liveInst.SetRateLimitEnabled(*req.Msg.RateLimitEnabled)
			}
		}
		updatedFields = append(updatedFields, "rate_limit_enabled")
		sideEffectChanged = true
	}

	// Handle autonomous mode toggle. Starting/stopping the AutonomousDriver is a
	// live side-effect; we only act when the value actually changes.
	if req.Msg.AutonomousMode != nil && *req.Msg.AutonomousMode != instance.AutonomousMode {
		if *req.Msg.AutonomousMode && s.headlessPool == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("autonomous mode requires a headless LLM to be configured"))
		}
		instance.SetAutonomousMode(*req.Msg.AutonomousMode, "")
		if instance.AutonomousMode {
			s.StartAutonomousDriverForInstance(instance)
		} else {
			s.autonomousSvc.stopAndDeregisterDriver(instance.Title)
		}
		updatedFields = append(updatedFields, "autonomous_mode")
		sideEffectChanged = true
	}

	// Handle auto-approve toggle. Restart-on-Active-change (serialized against a
	// concurrent program switch via restartTriggerMu) is handled inside SetAutoApprove.
	// Same server-side invariant as CreateSession: auto_approve=true is rejected for an
	// agent yoloFlagFor can't inject a bypass flag for, not just disabled client-side.
	if req.Msg.AutoApprove != nil && *req.Msg.AutoApprove != instance.AutoApprove {
		if *req.Msg.AutoApprove && !session.AutoApproveSupported(instance.Program) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("auto_approve is not supported for program %q", instance.Program))
		}
		if err := instance.SetAutoApprove(*req.Msg.AutoApprove, func() error {
			return s.storage.SaveInstances([]*session.Instance{instance})
		}); err != nil {
			log.Error("[UpdateSession] failed to restart session after auto-approve change", "session", instance.Title, "err", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session after auto-approve change: %w", err))
		}
		updatedFields = append(updatedFields, "auto_approve")
	}

	// Handle steering: inject a message into an active session. Autonomous
	// sessions keep the existing ClaudeController command-queue path (ADR-001);
	// non-autonomous, Instance-backed sessions fall back to the same PTY send
	// primitive the MCP steer_session tool already uses (tools_terminal.go's
	// SendKeys fallback branch) so browser-originated steering reaches ordinary
	// backlog work/review sessions too, not just autonomous ones.
	if req.Msg.SteerMessage != nil && *req.Msg.SteerMessage != "" {
		if len(*req.Msg.SteerMessage) > session.MaxSteerMessageLength {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("steer_message exceeds maximum length of %d bytes", session.MaxSteerMessageLength))
		}
		if err := s.steerInstance(ctx, instance, *req.Msg.SteerMessage); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, connect.NewError(connect.CodeDeadlineExceeded, err)
			}
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
	}

	// Handle status change (pause/resume) LAST - after all metadata updates.
	// This ensures that if Resume() fails, no partial metadata changes are persisted
	// (save only happens after all changes succeed).
	if changed, err := s.applyStatusTransition(req.Msg, instance); err != nil {
		return nil, err
	} else if changed {
		updatedFields = append(updatedFields, "status")
		sideEffectChanged = true
	}

	// Persist changes. The narrow metadata UPDATE runs first so a title rename lands
	// under currentTitle in the DB before any side-effecting SaveInstances call below
	// looks the row up by the already-in-memory-mutated new title — doing it in the
	// other order would miss the still-old-titled DB row and orphan it via
	// SaveInstances' Update-fails-so-Create fallback (see UpdateSessionMetadata).
	if metaTitle != nil || metaCategory != nil || metaNote != nil || metaWorkingDir != nil {
		if err := s.storage.UpdateInstanceMetadata(currentTitle, metaTitle, metaCategory, metaNote, metaWorkingDir); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
		}
	}
	if sideEffectChanged {
		if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
		}
	}

	// Publish events based on what was updated
	if len(updatedFields) > 0 {
		s.publishSessionUpdatedEvent(instance, updatedFields)
	}

	return connect.NewResponse(&sessionv1.UpdateSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// steerInstance injects message into instance's active session. Autonomous
// sessions keep the existing ClaudeController command-queue path;
// non-autonomous, Instance-backed sessions fall back to the same PTY send
// primitive the MCP steer_session tool already uses. Returns only plain
// fmt.Errorf-wrapped errors — never connect.NewError/connect.Code* — since
// SteerActiveSession calls this in-process from BacklogService; UpdateSession
// is the sole caller that translates the error into a connect.Code.
func (s *SessionService) steerInstance(ctx context.Context, instance *session.Instance, message string) error {
	if instance.AutonomousMode {
		controller := instance.GetController()
		if controller == nil {
			return fmt.Errorf("steer autonomous session %q: controller not started", instance.Title)
		}

		// SendCommandImmediate's own ~5min internal timeout doesn't protect
		// against the raw PTY write itself hanging, which would leak
		// steerActiveSessionForPRFix's steerInFlight guard forever.
		errCh := make(chan error, 1)
		go func() {
			_, sendErr := controller.SendCommandImmediate(message + "\r")
			errCh <- sendErr
		}()

		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		select {
		case sendErr := <-errCh:
			if sendErr != nil {
				return fmt.Errorf("steer autonomous session %q: %w", instance.Title, sendErr)
			}
		case <-timeoutCtx.Done():
			return fmt.Errorf("timed out steering autonomous session %q: %w", instance.Title, timeoutCtx.Err())
		}
		s.notifySteerSent(instance, message)
		return nil
	}

	// Non-autonomous sessions get the same PTY send primitive the MCP
	// steer_session tool falls back to (session.SubmitContentWithEnter,
	// bounded with a generous timeout so a browser click against a
	// wedged/dead session can't hang this goroutine forever) — content and
	// the submit keystroke travel as two separate SendKeys writes (BUG-031),
	// never concatenated.
	if err := session.SubmitContentWithEnter(ctx, instance, message); err != nil {
		return fmt.Errorf("steer session %q: %w", instance.Title, err)
	}
	s.notifySteerSent(instance, message)
	return nil
}

// notifySteerSent logs and publishes the "steering input sent" notification
// shared by both the autonomous and non-autonomous steer branches in
// UpdateSession.
func (s *SessionService) notifySteerSent(instance *session.Instance, steerMessage string) {
	log.Info("[UpdateSession] steering message sent", "session", instance.Title)
	s.eventBus.Publish(events.NewNotificationEvent(
		instance.UUID, instance.Title, fmt.Sprintf("steer-%s", instance.UUID),
		int32(10),                    // NotificationType_INFO
		derivePriority(false, false), // urgent, important — confirms a user-initiated action, no decision needed
		"Steering input sent",
		fmt.Sprintf("%s: %s", instance.Title, steerMessage),
		nil,
	))
}
