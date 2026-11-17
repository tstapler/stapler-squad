package services

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"context"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DetachItemPullRequest removes the item-direct link to pr and tombstones it for
// the item, so Jules and branch discovery do not bring it back. It is the
// operator's release for an item-direct retention hold, so it works for an item
// in any status. Session links to the same PR are untouched.
// UI-only: never reachable from MCP tools (server/mcp/tools_pr_rpc_unreachable_test.go).
// +api: session:detach-item-pr
func (s *SessionService) DetachItemPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.DetachItemPullRequestRequest],
) (*connect.Response[sessionv1.DetachItemPullRequestResponse], error) {
	itemID := req.Msg.GetItemId()
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("item_id is required"))
	}
	ref, err := prRefFromProto(req.Msg.GetPr())
	if err != nil {
		return nil, err
	}
	store, ok := s.storage.(*session.Storage)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("item PR detach needs ent-backed storage"))
	}
	item, gerr := store.GetBacklogItem(ctx, itemID)
	if gerr != nil || item == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %s not found", itemID))
	}
	found, err := store.DetachItemPR(ctx, itemID, ref)
	if err != nil {
		return nil, prSetError(err)
	}
	if !found {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s is not attached to this item", ref))
	}
	log.Info("item PR detached by user", "item", itemID, "pr", ref.String())
	// The detach may have released the last hold on a finished item's worktrees: re-run
	// the gate now instead of waiting for the sweep. The gate decides; this never forces.
	if c := s.prRefs.itemCleaner; c != nil && session.IsTerminalStatus(domain.BacklogStatus(item.Status)) {
		c.CleanupTerminalItem(session.WithCleanupPath(ctx, session.CleanupPathDetach), itemID)
	}
	return connect.NewResponse(&sessionv1.DetachItemPullRequestResponse{}), nil
}
