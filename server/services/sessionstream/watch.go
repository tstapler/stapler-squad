package sessionstream

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/server/services/internal/streammetrics"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/session/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WatchSessions streams real-time session events (created/updated/deleted).
// Sends initial snapshot of all sessions, then subscribes to real-time updates.
// +api: session:watch
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity, moved verbatim from server/services; reducing it is separate work
func (h *Handler) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
	done := streammetrics.TrackOpenStream("WatchSessions")
	defer done()

	// Subscribe before building the snapshot so no events are lost between the
	// two phases (snapshot races are resolved by client-side upsert semantics).
	eventCh, subID := h.bus.Subscribe(ctx)
	defer h.bus.Unsubscribe(subID)

	if req.Msg.AfterSeq > 0 {
		// Reconnecting client: replay events missed since last disconnect.
		// This covers the period between disconnect and the new subscription above.
		for _, event := range h.bus.EventsSince(req.Msg.AfterSeq) {
			if event.Session != nil && event.Session.Hidden {
				continue
			}
			if err := stream.Send(convertEventToProto(event)); err != nil {
				return fmt.Errorf("failed to send replayed event: %w", err)
			}
		}
	} else {
		// Fresh connection: send initial snapshot using in-memory poller cache —
		// avoids a full SQLite scan on every new WatchSessions connection.
		instances, polled := h.ports.Polled()
		if !polled {
			var err error
			instances, err = h.ports.LoadWired()
			if err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
			}
		}

		for _, inst := range instances {
			if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
				if inst.Category != *req.Msg.CategoryFilter {
					continue
				}
			}
			if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				// inst.GetStatus() reads the lock-free published snapshot rather than
				// inst.Status directly -- see reconcileSessions' identical fix
				// (9fcded805) for why a raw field read here races with the actor's
				// transitionToLocked write under -race.
				instProto, err := adapters.StatusToProto(domain.Status(inst.GetStatus()))
				if err != nil {
					log.Error("WatchSessions: unrecognized session.Status", "status", inst.GetStatus(), "err", err)
					continue
				}
				if instProto != *req.Msg.StatusFilter {
					continue
				}
			}
			if inst.Hidden {
				continue
			}
			if err := stream.Send(createInitialSnapshotEvent(inst)); err != nil {
				return fmt.Errorf("failed to send initial snapshot: %w", err)
			}
		}
	}

	// Peer sessions arrive on a separate channel; they carry Seq 0 so the client's
	// after_seq bookkeeping only ever sees the local EventBus counter.
	var peerCh <-chan *sessionv1.SessionEvent
	if peers := h.ports.Federation(); peers != nil {
		var cancelPeers func()
		peerCh, cancelPeers = peers.Watch().Subscribe()
		defer cancelPeers()
		if req.Msg.AfterSeq == 0 {
			for _, ps := range peers.FilteredByWatch(req.Msg) {
				if err := stream.Send(&sessionv1.SessionEvent{
					Timestamp: timestamppb.Now(),
					Event:     &sessionv1.SessionEvent_SessionCreated{SessionCreated: &sessionv1.SessionCreatedEvent{Session: ps}},
				}); err != nil {
					return fmt.Errorf("failed to send peer snapshot: %w", err)
				}
			}
		}
	}

	// Stream events until client disconnects or context is canceled
	heartbeat := time.NewTicker(h.heartbeatEvery)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context canceled
			return nil
		case <-heartbeat.C:
			if err := stream.Send(&sessionv1.SessionEvent{Timestamp: timestamppb.Now(), Heartbeat: true}); err != nil {
				return fmt.Errorf("failed to send session heartbeat: %w", err)
			}
		case pe, ok := <-peerCh:
			if !ok {
				peerCh = nil
				continue
			}
			if !federation.PeerEventMatches(pe, req.Msg) {
				continue
			}
			if err := stream.Send(pe); err != nil {
				return fmt.Errorf("failed to send peer event: %w", err)
			}
		case event, ok := <-eventCh:
			if !ok {
				// Event channel closed (should not happen with proper cleanup)
				return nil
			}

			// Apply filters to real-time events
			if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
				if event.Session != nil && event.Session.Category != *req.Msg.CategoryFilter {
					continue
				}
			}

			if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				if event.Session != nil {
					evtProto, err := adapters.StatusToProto(domain.Status(event.Session.GetStatus()))
					if err != nil {
						log.Error("WatchSessions: unrecognized session.Status on event", "status", event.Session.GetStatus(), "err", err)
						continue
					}
					if evtProto != *req.Msg.StatusFilter {
						continue
					}
				}
			}

			if event.Session != nil && event.Session.Hidden {
				continue
			}

			// Convert internal event to protobuf and send
			protoEvent := convertEventToProto(event)
			if err := stream.Send(protoEvent); err != nil {
				return fmt.Errorf("failed to send event: %w", err)
			}
		}
	}
}
