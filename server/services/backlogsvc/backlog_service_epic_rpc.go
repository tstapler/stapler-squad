package backlogsvc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

// maxIntegrationSiblings bounds the per-call lookups and inserts.
const maxIntegrationSiblings = 200

// CreateEpicIntegrationChild exposes OnEpicPlanningComplete to the epic planning flow.
// +api: backlog:create-epic-integration-child
func (s *BacklogService) CreateEpicIntegrationChild(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateEpicIntegrationChildRequest],
) (*connect.Response[sessionv1.CreateEpicIntegrationChildResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if len(req.Msg.SiblingItemIds) > maxIntegrationSiblings {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at most %d sibling_item_ids", maxIntegrationSiblings))
	}
	child, err := s.OnEpicPlanningComplete(ctx, req.Msg.EpicItemId, req.Msg.SiblingItemIds)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if child == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no integration child created: the epic_integration_child_enabled flag is off or the epic has no e2e: acceptance criterion or done_command_template"))
	}
	pb := backlogItemToProto(child, s.engine, s.buildCostLookup())
	s.attachDependencies(ctx, pb)
	return connect.NewResponse(&sessionv1.CreateEpicIntegrationChildResponse{Item: pb}), nil
}
