package sessioncreate

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/domain"
)

// CancelSessionCreation cancels an in-progress (Status == Creating) session
// creation (Epic 3.2, Story 3.2.1): it interrupts the Background Resolution
// Pipeline goroutine (if one still exists in this process), cleans up any
// partially-acquired resources via CleanupPartialCreation, and removes the
// instance entirely — a cancelled creation is deleted outright, not left as
// a Failed card (see the async-session-creation plan's Domain Glossary).
//
// Unlike DeleteSession, this does not publish a SessionDeletedEvent: per the
// plan's Epic 3.2 "Known asymmetry" note, that's flagged as adjacent, optional
// follow-up work (Task 3.2.1g), not a requirement of this RPC.
//
// +api: session:cancel-creation
func (h *Handler) CancelSessionCreation(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelSessionCreationRequest],
) (*connect.Response[sessionv1.CancelSessionCreationResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	inst := h.d.FindLive(req.Msg.Id)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	if status, _ := inst.StatusAndFailureReason(); status != domain.Creating {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %s is not in Creating status (current status=%s)", req.Msg.Id, status))
	}

	// Bump creationEpoch as its own actor mailbox round-trip BEFORE
	// re-reading status (Task 3.2.1c): this is what lets the two racing
	// writers -- this handler and the Background Resolution Pipeline's own
	// CommitTerminalStatus -- resolve deterministically to exactly one
	// outcome. If the pipeline's TryForceStatusIfEpoch call executed inside
	// the actor mailbox before this bump, it already won with the pre-bump
	// epoch and the status read below observes Active; if it executes at or
	// after this bump, it observes a stale epoch and no-ops, leaving Status
	// == Creating for this handler to win instead. Skipping the bump (or
	// combining it with the read into one command) would reopen the race
	// ADR-002 closes.
	inst.BumpCreationEpoch()
	if status, _ := inst.StatusAndFailureReason(); status != domain.Creating {
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

	if err := CleanupPartialCreation(inst, false); err != nil {
		log.Warn("CancelSessionCreation: failed to clean up partial creation resources", "session", req.Msg.Id, "err", err)
	}

	h.d.RemoveFromAllPollers(inst.Title)
	if err := h.d.Store.DeleteInstance(inst.Title); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Release this instance's actor from the SessionService-wide
	// *registry.Registry -- see DeleteSession's identical comment (Epic 6.2's
	// goleak hammer test) for why this is required: without it, a cancelled
	// creation leaks its runActor goroutine exactly like a deleted session
	// did. CleanupPartialCreation above already ran synchronously (unlike
	// DeleteSession's backgrounded version), so it's safe to release here
	// immediately. No-op if h.d.Registry() has no entry for this ID.
	if h.d.Registry() != nil {
		h.d.Registry().ForceRelease(inst.GetStableID())
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
// any partially-acquired resources via CleanupPartialCreation, then
// re-spawns the Background Resolution Pipeline against the SAME instance
// ID/storage row using the new epoch TryStartRetry returned. Only a
// SessionUpdatedEvent is published here (and again from within the
// pipeline as it progresses) -- never a second SessionCreatedEvent, so a
// retried instance never produces a duplicate card.
//
// +api: session:retry-creation
func (h *Handler) RetrySessionCreation(
	ctx context.Context,
	req *connect.Request[sessionv1.RetrySessionCreationRequest],
) (*connect.Response[sessionv1.RetrySessionCreationResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	inst := h.d.FindLive(req.Msg.Id)
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

	if err := CleanupPartialCreation(inst, false); err != nil {
		log.Warn("RetrySessionCreation: failed to clean up partial creation resources", "session", req.Msg.Id, "err", err)
	}

	h.d.Bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status", "creation_progress"}))

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
	h.d.Track(func() {
		h.RunResolutionPipeline(ctx, PipelineParams{
			Instance: inst,
			Epoch:    newEpoch,
			Title:    instanceTitle,
			RootDir:  instanceRootDir,
		})
	})

	return connect.NewResponse(&sessionv1.RetrySessionCreationResponse{
		Success: true,
		Message: fmt.Sprintf("session creation '%s' retrying", req.Msg.Id),
	}), nil
}
