package services

// backlog_service_chat.go — CreateBacklogItemFromChat: a thin front door that
// lets a single free-text chat message either create a new backlog item or
// refine an existing one. It intentionally duplicates none of
// CreateBacklogItem's or TriggerTriage's guards/semantics — it delegates to
// them verbatim so chat-originated items go through the exact same
// triageInFlight single-flight guard, headless pool budget, and orphan-sweep
// reconciliation as form-originated ones (see backlog_service_lifecycle.go's
// CreateBacklogItem and backlog_service_triage.go's TriggerTriage).

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// chatDerivedTitleMaxLen bounds the title synthesized from a chat message's
// first line so long free-text messages don't produce unusably long titles.
const chatDerivedTitleMaxLen = 80

// deriveChatItemTitle synthesizes a BacklogItem title from a chat message's
// first line, truncating to chatDerivedTitleMaxLen. The full message is
// preserved verbatim as the item's Description.
func deriveChatItemTitle(message string) string {
	firstLine := message
	if idx := strings.IndexByte(message, '\n'); idx != -1 {
		firstLine = message[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	runes := []rune(firstLine)
	if len(runes) > chatDerivedTitleMaxLen {
		return string(runes[:chatDerivedTitleMaxLen])
	}
	return firstLine
}

// CreateBacklogItemFromChat handles a single turn of natural-language chat.
// When ExistingItemId is empty, this is the first turn of a new
// chat-originated item: it synthesizes a minimal BacklogItem from the
// message and delegates to CreateBacklogItem. When ExistingItemId is set,
// this is a refinement turn: it delegates to TriggerTriage with
// Feedback = message, reusing the existing feedback/iteration mechanism.
// +api: backlog:create-item-from-chat
func (s *BacklogService) CreateBacklogItemFromChat(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateBacklogItemFromChatRequest],
) (*connect.Response[sessionv1.CreateBacklogItemFromChatResponse], error) {
	message := strings.TrimSpace(req.Msg.Message)
	if message == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("message is required"))
	}

	if existingItemID := strings.TrimSpace(req.Msg.ExistingItemId); existingItemID != "" {
		if _, err := s.TriggerTriage(ctx, connect.NewRequest(&sessionv1.TriggerTriageRequest{
			ItemId:   existingItemID,
			Feedback: message,
		})); err != nil {
			return nil, err
		}

		if s.storage == nil {
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
		}
		updated, err := s.storage.GetBacklogItem(ctx, existingItemID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item: %w", err))
		}

		return connect.NewResponse(&sessionv1.CreateBacklogItemFromChatResponse{
			Item:            backlogItemToProto(updated, s.buildCostLookup()),
			TriageTriggered: true,
		}), nil
	}

	createResp, err := s.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:       deriveChatItemTitle(message),
		Description: message,
	}))
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&sessionv1.CreateBacklogItemFromChatResponse{
		Item:            createResp.Msg.Item,
		TriageTriggered: createResp.Msg.TriageTriggered,
	}), nil
}
