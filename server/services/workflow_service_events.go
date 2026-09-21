package services

// workflow_service_events.go — WatchWorkflows streaming RPC for WorkflowService.
// Mirrors backlog_service_events.go's WatchBacklogItems shape (subscribe before
// snapshotting, then either after_seq replay or a fresh per-workflow snapshot,
// then live fan-out) at a scale appropriate to workflows: there is no
// storage.ListBacklogItems-equivalent filtering/enrichment step, so this file is
// considerably shorter.

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// workflowEventSender is the narrow interface watchWorkflows sends through —
// satisfied structurally by *connect.ServerStream[sessionv1.WorkflowEvent] in
// production and by a fake in tests, mirroring backlogItemEventSender.
type workflowEventSender interface {
	Send(*sessionv1.WorkflowEvent) error
}

// WatchWorkflows streams real-time workflow definition events. Sends an
// initial snapshot (or, on reconnect via after_seq, a replay of buffered
// events) followed by live fan-out.
// +api: workflow:watch
func (s *WorkflowService) WatchWorkflows(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchWorkflowsRequest],
	stream *connect.ServerStream[sessionv1.WorkflowEvent],
) error {
	return s.watchWorkflows(ctx, req.Msg, stream)
}

// watchWorkflows is WatchWorkflows's core logic, extracted behind
// workflowEventSender so it is directly unit-testable without a real RPC
// round-trip.
func (s *WorkflowService) watchWorkflows(
	ctx context.Context,
	msg *sessionv1.WatchWorkflowsRequest,
	sender workflowEventSender,
) error {
	if s.eventBus == nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow event stream is not available"))
	}
	if s.repo == nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow storage not available"))
	}

	done := TrackOpenStream("WatchWorkflows")
	defer done()

	// Subscribe before building the snapshot/replay batch so no events are lost
	// between the two phases — mirrors WatchBacklogItems' identical ordering.
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	if err := s.sendInitialWorkflowPhase(ctx, msg, sender); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-eventCh:
			if !ok {
				return nil
			}
			if evt.Type != events.EventWorkflowChanged || evt.WorkflowPayload == nil {
				continue
			}
			if err := sender.Send(convertEventToWorkflowEvent(evt)); err != nil {
				return fmt.Errorf("failed to send workflow event: %w", err)
			}
		}
	}
}

// sendInitialWorkflowPhase sends either an after_seq replay batch or a fresh
// per-workflow snapshot, then — if that phase sent nothing at all — the
// content-free snapshot-complete marker described below. Split out of
// watchWorkflows to keep that function's own control flow (subscribe, initial
// phase, live loop) readable at a glance.
func (s *WorkflowService) sendInitialWorkflowPhase(
	ctx context.Context,
	msg *sessionv1.WatchWorkflowsRequest,
	sender workflowEventSender,
) error {
	var sent int
	var err error
	if msg.GetAfterSeq() > 0 {
		sent, err = s.replayWorkflowEventsSince(msg.GetAfterSeq(), sender)
	} else {
		sent, err = s.sendWorkflowSnapshot(ctx, sender)
	}
	if err != nil {
		return err
	}

	// A zero-workflow backlog (or a stream connecting before any workflow has
	// ever been created) sends literally zero bytes above — see
	// BacklogItemEvent.snapshot_complete's doc comment for why the client's
	// `for await` loop would otherwise never resolve past its first iteration.
	if sent == 0 {
		if err := sender.Send(&sessionv1.WorkflowEvent{
			Timestamp: timestamppb.Now(),
			Event:     &sessionv1.WorkflowEvent_SnapshotComplete{SnapshotComplete: &sessionv1.WorkflowSnapshotCompleteEvent{}},
		}); err != nil {
			return fmt.Errorf("failed to send snapshot-complete marker: %w", err)
		}
	}

	return nil
}

// replayWorkflowEventsSince sends every buffered workflow event with
// seq > afterSeq, forcing is_snapshot on each (see forceWorkflowIsSnapshot),
// and returns how many it sent.
func (s *WorkflowService) replayWorkflowEventsSince(afterSeq uint64, sender workflowEventSender) (int, error) {
	sent := 0
	for _, evt := range s.eventBus.EventsSince(afterSeq) {
		if evt.Type != events.EventWorkflowChanged || evt.WorkflowPayload == nil {
			continue
		}
		converted := convertEventToWorkflowEvent(evt)
		forceWorkflowIsSnapshot(converted)
		if err := sender.Send(converted); err != nil {
			return sent, fmt.Errorf("failed to send replayed workflow event: %w", err)
		}
		sent++
	}
	return sent, nil
}

// sendWorkflowSnapshot sends one WorkflowCreatedEvent (is_snapshot: true) per
// currently-saved workflow and returns how many it sent.
func (s *WorkflowService) sendWorkflowSnapshot(ctx context.Context, sender workflowEventSender) (int, error) {
	wfs, err := s.repo.ListAll(ctx)
	if err != nil {
		return 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load workflows: %w", err))
	}
	for i, wf := range wfs {
		evt := &sessionv1.WorkflowEvent{
			Timestamp: timestamppb.Now(),
			Event: &sessionv1.WorkflowEvent_WorkflowCreated{
				WorkflowCreated: &sessionv1.WorkflowCreatedEvent{
					Workflow:   entWorkflowToProto(wf),
					IsSnapshot: true,
				},
			},
		}
		if err := sender.Send(evt); err != nil {
			return i, fmt.Errorf("failed to send initial workflow snapshot: %w", err)
		}
	}
	return len(wfs), nil
}

// convertEventToWorkflowEvent converts an internal events.Event carrying a
// WorkflowEventPayload into the wire sessionv1.WorkflowEvent, switching on
// Kind to build the matching oneof variant. Mirrors
// convertEventToBacklogItemEvent's identical switch-on-Kind shape.
func convertEventToWorkflowEvent(evt *events.Event) *sessionv1.WorkflowEvent {
	out := &sessionv1.WorkflowEvent{
		Timestamp: timestamppb.New(evt.Timestamp),
		Seq:       evt.Seq,
	}

	payload := evt.WorkflowPayload
	if payload == nil {
		return out
	}

	switch payload.Kind {
	case events.WorkflowChangeCreated:
		out.Event = &sessionv1.WorkflowEvent_WorkflowCreated{
			WorkflowCreated: &sessionv1.WorkflowCreatedEvent{
				Workflow: entWorkflowToProto(payload.Workflow),
			},
		}
	case events.WorkflowChangeUpdated:
		out.Event = &sessionv1.WorkflowEvent_WorkflowUpdated{
			WorkflowUpdated: &sessionv1.WorkflowUpdatedEvent{
				Workflow: entWorkflowToProto(payload.Workflow),
			},
		}
	case events.WorkflowChangeDeleted:
		out.Event = &sessionv1.WorkflowEvent_WorkflowDeleted{
			WorkflowDeleted: &sessionv1.WorkflowDeletedEvent{
				Id: payload.WorkflowID,
			},
		}
	case events.WorkflowChangeRun:
		out.Event = &sessionv1.WorkflowEvent_WorkflowRun{
			WorkflowRun: &sessionv1.WorkflowRunEvent{
				WorkflowId: payload.WorkflowID,
				SessionId:  payload.SessionID,
			},
		}
	}

	return out
}

// forceWorkflowIsSnapshot sets is_snapshot: true on whichever oneof variant
// evt has populated. Used only by the after_seq replay branch, mirroring
// forceIsSnapshot's identical rationale (backlog_service_events.go): a live
// event published in the race window between Subscribe() and EventsSince()
// can otherwise be delivered via both the replay branch and the live
// fan-out loop, double-flashing the frontend.
func forceWorkflowIsSnapshot(evt *sessionv1.WorkflowEvent) {
	switch e := evt.GetEvent().(type) {
	case *sessionv1.WorkflowEvent_WorkflowCreated:
		e.WorkflowCreated.IsSnapshot = true
	case *sessionv1.WorkflowEvent_WorkflowUpdated:
		e.WorkflowUpdated.IsSnapshot = true
	}
}
