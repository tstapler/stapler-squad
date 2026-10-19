package services

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
)

// notification_action_backlog.go delegates BACKLOG_DECISION choices to the
// existing RPCs. A decision is applied once: the item's current status is read
// first, so a retry after a restart (empty idempotency cache) reports
// ALREADY_RESOLVED instead of repeating the transition or the merge.

const (
	backlogStatusReady    = "ready"
	backlogStatusDone     = "done"
	backlogStatusArchived = "archived"
)

// errItemNotFound is returned by ItemStatusReader for an unknown item.
var errItemNotFound = errors.New("backlog item not found")

// ItemStatusReader reads a backlog item's current status.
type ItemStatusReader interface {
	ItemStatus(ctx context.Context, itemID string) (string, error)
}

// WorkSessionFinder returns the UUID of the newest work session of an item, or
// "" when it has none.
type WorkSessionFinder interface {
	LatestWorkSession(ctx context.Context, itemID string) (string, error)
}

// BacklogTransitioner is TransitionBacklogItemStatus.
type BacklogTransitioner interface {
	TransitionBacklogItemStatus(context.Context, *connect.Request[sessionv1.TransitionBacklogItemStatusRequest]) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error)
}

// PRMerger is MergePR.
type PRMerger interface {
	MergePR(context.Context, *connect.Request[sessionv1.MergePRRequest]) (*connect.Response[sessionv1.MergePRResponse], error)
}

// BacklogDeciderDeps are the collaborators of the production BacklogDecider.
type BacklogDeciderDeps struct {
	Status      ItemStatusReader
	Transitions BacklogTransitioner
	Sessions    WorkSessionFinder
	Merger      PRMerger
}

type backlogDecider struct{ BacklogDeciderDeps }

// NewBacklogDecider builds the BacklogDecider over the existing RPCs.
func NewBacklogDecider(d BacklogDeciderDeps) BacklogDecider { return backlogDecider{d} }

func (b backlogDecider) Decide(ctx context.Context, d BacklogDecision) (ActionResult, error) {
	status, err := b.Status.ItemStatus(ctx, d.ItemID)
	if errors.Is(err, errItemNotFound) {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "item_gone"}, nil
	}
	if err != nil {
		return ActionResult{}, err
	}
	switch d.Choice {
	case choiceAccept:
		return b.transition(ctx, d, status, backlogStatusDone, backlogStatusReview)
	case choiceRework:
		return b.transition(ctx, d, status, backlogStatusReady, backlogStatusReview, backlogStatusPRPending)
	case choiceMerge:
		return b.merge(ctx, d, status)
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "unknown choice"), nil
}

// transition moves the item to target when it is in one of from. An item
// already at target was decided by an earlier call.
func (b backlogDecider) transition(ctx context.Context, d BacklogDecision, current, target string, from ...string) (ActionResult, error) {
	if current == target {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
	}
	if !stringIn(from, current) {
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "item_moved"}, nil
	}
	_, err := b.Transitions.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: d.ItemID, TargetStatus: target, ExpectedStatus: current, OverrideReason: d.Reason,
	}))
	return mapTransitionError(err)
}

func mapTransitionError(err error) (ActionResult, error) {
	if err == nil {
		return applied(), nil
	}
	switch connect.CodeOf(err) {
	case connect.CodeFailedPrecondition, connect.CodeInvalidArgument:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	case connect.CodeNotFound:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "item_gone"}, nil
	}
	return ActionResult{}, err
}

// merge merges the pull request of the item's newest work session. It needs the
// operator's inline confirm (design/ux.md 2.4) and is skipped once the item is done.
func (b backlogDecider) merge(ctx context.Context, d BacklogDecision, status string) (ActionResult, error) {
	switch status {
	case backlogStatusDone, backlogStatusArchived:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
	case backlogStatusReview, backlogStatusPRPending:
	default:
		return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "item_moved"}, nil
	}
	if !d.Confirm {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "merge needs confirmation"), nil
	}
	sessionID, err := b.Sessions.LatestWorkSession(ctx, d.ItemID)
	if err != nil {
		return ActionResult{}, err
	}
	if sessionID == "" {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "the item has no work session to merge"), nil
	}
	resp, err := b.Merger.MergePR(ctx, connect.NewRequest(&sessionv1.MergePRRequest{Id: sessionID}))
	if err != nil {
		return mapTransitionError(err)
	}
	if !resp.Msg.GetSuccess() {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, resp.Msg.GetMessage()), nil
	}
	return applied(), nil
}

func stringIn(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// backlogStorage is the storage the production readers use.
type backlogStorage interface {
	GetBacklogItem(ctx context.Context, id string) (*session.BacklogItemData, error)
	ListItemSessions(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error)
}

type storageItemReader struct{ storage backlogStorage }

// NewItemStatusReader reads item status from storage.
func NewItemStatusReader(s backlogStorage) ItemStatusReader { return storageItemReader{s} }

func (r storageItemReader) ItemStatus(ctx context.Context, itemID string) (string, error) {
	item, err := r.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", errItemNotFound
		}
		return "", fmt.Errorf("load backlog item: %w", err)
	}
	return item.Status, nil
}

type storageSessionFinder struct{ storage backlogStorage }

// NewWorkSessionFinder finds an item's newest work session in storage.
func NewWorkSessionFinder(s backlogStorage) WorkSessionFinder { return storageSessionFinder{s} }

func (f storageSessionFinder) LatestWorkSession(ctx context.Context, itemID string) (string, error) {
	rows, err := f.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("list item sessions: %w", err)
	}
	for i := len(rows) - 1; i >= 0; i-- { // oldest first, so scan from the end
		if rows[i].Role == string(session.SessionRoleWork) {
			return rows[i].SessionUUID, nil
		}
	}
	return "", nil
}

// historyItemResolver retires the unread decision rows of an item once decided.
type historyItemResolver struct {
	store *notifications.NotificationHistoryStore
}

// NewNotificationResolver marks an item's decision rows read.
func NewNotificationResolver(store *notifications.NotificationHistoryStore) NotificationResolver {
	return historyItemResolver{store}
}

func (r historyItemResolver) ResolveItemNotifications(itemID string) {
	if r.store == nil {
		return
	}
	_, err := r.store.MarkReadWhere(func(n *notifications.NotificationRecord) bool {
		return n.Metadata[metaItemID] == itemID && n.Metadata[metaDecisionSchema] != ""
	})
	if err != nil {
		log.Warn("[NotificationAction] could not retire decision notifications", "item", itemID, "err", err)
	}
}

func (r historyItemResolver) ResolveQuestionNotifications(questionID string) {
	if r.store == nil {
		return
	}
	if _, err := r.store.ResolveQuestion(questionID); err != nil {
		log.Warn("[NotificationAction] could not retire question notifications", "question", questionID, "err", err)
	}
}

func (r historyItemResolver) ResolveApprovalNotifications(approvalID string) {
	if r.store == nil {
		return
	}
	if _, err := r.store.ResolveApproval(approvalID); err != nil {
		log.Warn("[NotificationAction] could not retire approval notifications", "approval", approvalID, "err", err)
	}
}
