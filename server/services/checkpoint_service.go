package services

import (
	"context"
	"fmt"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CheckpointService handles checkpoint creation, listing, and conversation-state
// management RPCs. Logic moved verbatim from SessionService (ADR-001).
type CheckpointService struct {
	storage       session.InstanceStore
	eventBus      *events.EventBus
	scrollbackMgr ScrollbackSequencer

	// poller and extDiscovery back the findInstance and allInstances helpers.
	// Both are wired via SetPoller/SetExternalDiscovery after construction,
	// mirroring the SessionService deferred-setter pattern.
	poller       *session.ReviewQueuePoller
	extDiscovery *session.ExternalSessionDiscovery

	// loadInstances provides the fallback instance-load path used in
	// ClearConversationState when the target session is not in the live poller.
	// Wired to SessionService.loadInstancesWithWiring after construction.
	loadInstances func() ([]*session.Instance, error)
}

// NewCheckpointService creates a CheckpointService with the given storage and event bus.
// Call SetPoller, SetExternalDiscovery, SetScrollbackMgr, and SetLoadInstancesFn after
// construction to complete wiring (matching the pattern of project_service.go).
func NewCheckpointService(storage session.InstanceStore, eventBus *events.EventBus) *CheckpointService {
	return &CheckpointService{
		storage:  storage,
		eventBus: eventBus,
	}
}

// SetScrollbackMgr wires the scrollback sequence provider for CreateCheckpoint.
func (cs *CheckpointService) SetScrollbackMgr(mgr ScrollbackSequencer) {
	cs.scrollbackMgr = mgr
}

// SetPoller wires the live-instance poller for fast instance lookup.
func (cs *CheckpointService) SetPoller(p *session.ReviewQueuePoller) {
	cs.poller = p
}

// SetExternalDiscovery wires external session discovery (mux-enabled sessions).
func (cs *CheckpointService) SetExternalDiscovery(d *session.ExternalSessionDiscovery) {
	cs.extDiscovery = d
}

// SetLoadInstancesFn wires the fallback function used when the target session is
// not found in the live poller. Should close over SessionService.loadInstancesWithWiring.
func (cs *CheckpointService) SetLoadInstancesFn(fn func() ([]*session.Instance, error)) {
	cs.loadInstances = fn
}

// findInstance finds an instance by id using the live poller, then external discovery.
func (cs *CheckpointService) findInstance(id string) *session.Instance {
	if cs.poller != nil {
		if inst := cs.poller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if cs.extDiscovery != nil {
		if inst := cs.extDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// findLiveInstance returns the live in-memory instance held by the poller, or nil.
func (cs *CheckpointService) findLiveInstance(id string) *session.Instance {
	if cs.poller == nil {
		return nil
	}
	return cs.poller.FindInstance(id)
}

// allInstances returns all poller-tracked instances (external-discovery sessions excluded).
func (cs *CheckpointService) allInstances() []*session.Instance {
	if cs.poller != nil {
		return cs.poller.GetInstances()
	}
	return nil
}

// CreateCheckpoint creates a new named checkpoint for the specified session.
func (cs *CheckpointService) CreateCheckpoint(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateCheckpointRequest],
) (*connect.Response[sessionv1.CreateCheckpointResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Label == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("label is required"))
	}

	inst := cs.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	var scrollbackSeq uint64
	if cs.scrollbackMgr != nil {
		scrollbackSeq = cs.scrollbackMgr.CurrentSequence(inst.Title)
	}

	cp, err := inst.CreateCheckpoint(req.Msg.Label, scrollbackSeq)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := cs.storage.SaveInstances(cs.allInstances()); err != nil {
		log.Warn("CreateCheckpoint: failed to persist checkpoint", "session", inst.Title, "err", err)
	}

	return connect.NewResponse(&sessionv1.CreateCheckpointResponse{
		Checkpoint: checkpointToProto(cp),
	}), nil
}

// ListCheckpoints returns all checkpoints for the specified session.
func (cs *CheckpointService) ListCheckpoints(
	ctx context.Context,
	req *connect.Request[sessionv1.ListCheckpointsRequest],
) (*connect.Response[sessionv1.ListCheckpointsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	inst := cs.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	checkpoints := inst.GetCheckpoints()
	protos := make([]*sessionv1.CheckpointProto, 0, len(checkpoints))
	for i := range checkpoints {
		protos = append(protos, checkpointToProto(&checkpoints[i]))
	}

	return connect.NewResponse(&sessionv1.ListCheckpointsResponse{
		Checkpoints: protos,
	}), nil
}

// ClearConversationState removes the stored Claude conversation UUID from a session
// so that the next Resume starts a fresh conversation instead of attempting --resume
// with a stale or path-mismatched UUID.
func (cs *CheckpointService) ClearConversationState(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearConversationStateRequest],
) (*connect.Response[sessionv1.ClearConversationStateResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	instance := cs.findLiveInstance(req.Msg.Id)
	if instance == nil && cs.loadInstances != nil {
		instances, err := cs.loadInstances()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		for _, inst := range instances {
			if inst.MatchesID(req.Msg.Id) {
				instance = inst
				break
			}
		}
	}
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	instance.ClearConversationState()

	if err := cs.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist cleared state: %w", err))
	}

	log.Info("cleared conversation state", "session", instance.Title)
	cs.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"claude_session"}))

	return connect.NewResponse(&sessionv1.ClearConversationStateResponse{
		Success: true,
		Message: fmt.Sprintf("Conversation state cleared for session '%s'", instance.Title),
	}), nil
}

// checkpointToProto converts a session.Checkpoint to its proto representation.
// Defined here as a package-level helper so it can be shared by CheckpointService
// and other callers (e.g. ForkSession) that still live in session_service.go.
func checkpointToProto(cp *session.Checkpoint) *sessionv1.CheckpointProto {
	if cp == nil {
		return nil
	}
	return &sessionv1.CheckpointProto{
		Id:             cp.ID,
		SessionId:      cp.SessionID,
		ParentId:       cp.ParentID,
		Label:          cp.Label,
		ScrollbackSeq:  cp.ScrollbackSeq,
		ScrollbackPath: cp.ScrollbackPath,
		ClaudeConvUuid: cp.ClaudeConvUUID,
		GitCommitSha:   cp.GitCommitSHA,
		Timestamp:      timestamppb.New(cp.Timestamp),
	}
}
