package sessionstream

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// GetDetectionEvents returns recent status-detection events for a session's Claude controller.
// Used by the debug panel (FR-8) — returns an empty list when the session has no active controller.
func (h *Handler) GetDetectionEvents(ctx context.Context, req *connect.Request[sessionv1.GetDetectionEventsRequest]) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
	inst := h.ports.FindInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %q not found", req.Msg.SessionId))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	statusManager := h.ports.StatusManager()
	if statusManager == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	controller, ok := statusManager.GetController(inst.Title)
	if !ok || controller == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	events := controller.GetStatusDetector().RecentEvents(limit)
	protoEvents := make([]*sessionv1.DetectionEventProto, 0, len(events))
	for _, e := range events {
		protoEvents = append(protoEvents, &sessionv1.DetectionEventProto{
			SessionId:       e.SessionID,
			Timestamp:       timestamppb.New(e.Timestamp),
			MatchedPattern:  e.MatchedPattern,
			MatchedCategory: e.MatchedCategory,
			ResultStatus:    int32(e.ResultStatus), //#nosec G115 -- small enum value (DetectedStatus)
		})
	}
	return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{Events: protoEvents}), nil
}
