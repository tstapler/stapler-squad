package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WatchSessions streams real-time session events (created/updated/deleted).
// Sends initial snapshot of all sessions, then subscribes to real-time updates.
// +api: session:watch
func (s *SessionService) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
	done := TrackOpenStream("WatchSessions")
	defer done()

	// Subscribe before building the snapshot so no events are lost between the
	// two phases (snapshot races are resolved by client-side upsert semantics).
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	if req.Msg.AfterSeq > 0 {
		// Reconnecting client: replay events missed since last disconnect.
		// This covers the period between disconnect and the new subscription above.
		for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {
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
		var instances []*session.Instance
		if s.reviewQueuePoller != nil {
			instances = s.reviewQueuePoller.GetInstances()
		} else {
			var err error
			instances, err = s.loadInstancesWithWiring()
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
				instProto, err := adapters.StatusToProto(session.Status(inst.GetStatus()))
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

	// Stream events until client disconnects or context is canceled
	heartbeat := time.NewTicker(events.HeartbeatInterval)
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
					evtProto, err := adapters.StatusToProto(session.Status(event.Session.GetStatus()))
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

// GetSessionDiff retrieves the current git diff for a session.
func (s *SessionService) GetSessionDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDiffRequest],
) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	var diffStats *git.DiffStats

	instance := s.findInstance(req.Msg.Id)
	if instance != nil {
		// Live session: refresh (if the cached value is stale) and read.
		if err := instance.RefreshDiffStatsIfStale(); err != nil {
			log.Warn("failed to update diff stats", "session", req.Msg.Id, "err", err)
		}
		diffStats = instance.GetDiffStats()
	} else {
		// Completed session: reconstruct worktree from DB and compute diff on-demand.
		allData, err := s.storage.ListInstanceData()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
		}
		var found *session.InstanceData
		for i := range allData {
			if allData[i].MatchesID(req.Msg.Id) {
				found = &allData[i]
				break
			}
		}
		if found == nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
		}
		if found.Worktree.WorktreePath != "" {
			wt := git.NewGitWorktreeFromStorage(
				found.Worktree.RepoPath,
				found.Worktree.WorktreePath,
				found.Worktree.SessionName,
				found.Worktree.BranchName,
				found.Worktree.BaseCommitSHA,
			)
			diffStats = wt.Diff()
		} else if found.Path != "" {
			// ponytail: directory sessions have no worktree; use the session path directly.
			// resolveBaseCommitSHA() will find the merge-base with main/master as fallback.
			wt := git.NewGitWorktreeFromStorage(found.Path, found.Path, found.Title, "", "")
			if wt != nil {
				diffStats = wt.Diff()
			}
		}
	}

	if diffStats == nil {
		return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
			DiffStats: &sessionv1.DiffStats{},
		}), nil
	}

	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
		DiffStats: &sessionv1.DiffStats{
			Added:   int32(diffStats.Added),   //#nosec G115 -- diff line count, bounded well under int32 max
			Removed: int32(diffStats.Removed), //#nosec G115 -- diff line count, bounded well under int32 max
			Content: diffStats.Content,
		},
	}), nil
}

// GetReviewQueue returns sessions needing user attention with priority ordering.
func (s *SessionService) GetReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.GetReviewQueueRequest],
) (*connect.Response[sessionv1.GetReviewQueueResponse], error) {
	return s.reviewQueueSvc.GetReviewQueue(ctx, req)
}

// AcknowledgeSession marks a session as acknowledged in the review queue.
// The session won't reappear in the queue until it receives an update.
func (s *SessionService) AcknowledgeSession(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeSessionRequest],
) (*connect.Response[sessionv1.AcknowledgeSessionResponse], error) {
	return s.reviewQueueSvc.AcknowledgeSession(ctx, req)
}

// GetLogs retrieves application logs with optional filtering and search.
func (s *SessionService) GetLogs(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLogsRequest],
) (*connect.Response[sessionv1.GetLogsResponse], error) {
	return s.utilitySvc.GetLogs(ctx, req)
}

// WatchReviewQueue streams real-time review queue events.
func (s *SessionService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	return s.reviewQueueSvc.WatchReviewQueue(ctx, req, stream)
}
