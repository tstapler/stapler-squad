package services

// backlog_sources_preview.go — PreviewBackwardSyncImpact handler (Epic 4.4,
// Story 4.4.1). Gates the Settings UI's first-enable-of-backward-sync
// confirmation dialog: before a user flips backward sync ON for a source, the
// UI calls this RPC to find out how many already-imported items would
// immediately transition (per determineBackwardSyncTarget, ADR-002) so the
// blast radius can be shown and confirmed, rather than silently applied on
// the same tick the toggle flips.

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// PreviewBackwardSyncImpact reports how many already-imported items for a
// source would immediately transition to archived if backward sync were
// enabled for it right now. Mirrors TriggerSync's SyncLoop construction and
// preconditions (same plugin registry, same encryption key provider — not a
// second credential flow), but is read-only: it does not advance the
// source's sync cursor and does not record a SourceSyncEvent.
// +api: backlog:preview-backward-sync-impact
func (s *BacklogService) PreviewBackwardSyncImpact(
	ctx context.Context,
	req *connect.Request[sessionv1.PreviewBackwardSyncImpactRequest],
) (*connect.Response[sessionv1.PreviewBackwardSyncImpactResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if s.pluginRegistry == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("sync not configured — no plugin registry wired"))
	}
	if req.Msg.SourceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source_id is required"))
	}
	if _, parseErr := uuid.Parse(req.Msg.SourceId); parseErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid source_id %q: %w", req.Msg.SourceId, parseErr))
	}

	var sl *session.SyncLoop
	if s.syncKeyFunc != nil {
		sl = session.NewSyncLoopWithKeyProvider(s.storage, s.pluginRegistry, s.syncKeyFunc)
	} else {
		sl = session.NewSyncLoop(s.storage, s.pluginRegistry)
	}

	previewCtx, cancel := context.WithTimeout(ctx, defaultTriggerSyncTimeout)
	defer cancel()

	itemCount, sampleTitles, err := sl.PreviewBackwardSyncImpactByID(previewCtx, req.Msg.SourceId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("preview backward sync impact failed: %w", err))
	}

	return connect.NewResponse(&sessionv1.PreviewBackwardSyncImpactResponse{
		ItemCount:    int32(itemCount),
		SampleTitles: sampleTitles,
	}), nil
}
