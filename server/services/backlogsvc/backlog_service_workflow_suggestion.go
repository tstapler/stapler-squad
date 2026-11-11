package backlogsvc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// DismissWorkflowSuggestion marks a diagnose-session recommendation dismissed.
// +api: backlog:dismiss-workflow-suggestion
func (s *BacklogService) DismissWorkflowSuggestion(
	ctx context.Context,
	req *connect.Request[sessionv1.DismissWorkflowSuggestionRequest],
) (*connect.Response[sessionv1.DismissWorkflowSuggestionResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" || req.Msg.SuggestionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id and suggestion_id are required"))
	}
	if err := s.storage.DismissWorkflowSuggestion(ctx, req.Msg.ItemId, req.Msg.SuggestionId); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.DismissWorkflowSuggestionResponse{}), nil
}
