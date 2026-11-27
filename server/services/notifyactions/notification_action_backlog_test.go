package notifyactions

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
)

// itemWorld is an in-memory backlog item whose status the fake RPCs mutate, so a
// repeated decision sees the state the first one left behind.
type itemWorld struct {
	status      string
	transitions atomic.Int32
	merges      atomic.Int32
	lastTarget  string
	lastExpect  string
	mergeErr    error
	sessionID   string
	statusErr   error
}

func (w *itemWorld) ItemStatus(context.Context, string) (string, error) {
	return w.status, w.statusErr
}

func (w *itemWorld) LatestWorkSession(context.Context, string) (string, error) {
	return w.sessionID, nil
}

func (w *itemWorld) TransitionBacklogItemStatus(_ context.Context, req *connect.Request[sessionv1.TransitionBacklogItemStatusRequest]) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error) {
	w.transitions.Add(1)
	w.lastTarget, w.lastExpect = req.Msg.TargetStatus, req.Msg.ExpectedStatus
	if w.mergeErr != nil {
		return nil, w.mergeErr
	}
	w.status = req.Msg.TargetStatus
	return connect.NewResponse(&sessionv1.TransitionBacklogItemStatusResponse{}), nil
}

func (w *itemWorld) MergePR(context.Context, *connect.Request[sessionv1.MergePRRequest]) (*connect.Response[sessionv1.MergePRResponse], error) {
	w.merges.Add(1)
	w.status = backlogStatusDone
	return connect.NewResponse(&sessionv1.MergePRResponse{Success: true}), nil
}

func (w *itemWorld) decider() BacklogDecider {
	return NewBacklogDecider(BacklogDeciderDeps{Status: w, Transitions: w, Sessions: w, Merger: w})
}

func decide(t *testing.T, b BacklogDecider, choice string, confirm bool) questionkit.ActionResult {
	t.Helper()
	res, err := b.Decide(context.Background(), BacklogDecision{ItemID: "item1", Choice: choice, Confirm: confirm, Reason: "needs tests"})
	require.NoError(t, err)
	return res
}

func TestBacklogDecider_ShouldMarkDone_WhenAcceptedFromReview(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusReview}
	assert.Equal(t, outApplied, decide(t, w.decider(), notify.ChoiceAccept, false).Outcome)
	assert.Equal(t, backlogStatusDone, w.lastTarget)
	assert.Equal(t, notify.BacklogStatusReview, w.lastExpect, "the transition is guarded by the status the operator saw")
	retry := decide(t, w.decider(), notify.ChoiceAccept, false)
	assert.Equal(t, outResolved, retry.Outcome, "a retry sees done")
	assert.Equal(t, backlogStatusDone, retry.Detail, "the value that resolved it")
	assert.EqualValues(t, 1, w.transitions.Load())
}

func TestBacklogDecider_ShouldSendBackToReady_WhenReworkFromReview(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusReview}
	assert.Equal(t, outApplied, decide(t, w.decider(), notify.ChoiceRework, false).Outcome)
	assert.Equal(t, backlogStatusReady, w.lastTarget)
	assert.Equal(t, outResolved, decide(t, w.decider(), notify.ChoiceRework, false).Outcome)
	assert.EqualValues(t, 1, w.transitions.Load())
}

func TestBacklogDecider_ShouldReturnAlreadyResolvedWithoutMerging_WhenMergeRetriedAfterRestart(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusReview, sessionID: "work-1"}
	d := NewNotificationActionDispatcher(ActionDelegates{Backlog: w.decider()}, nil, testFlagRegistry())
	d.flags, d.metrics = fakeActionFlags{off: map[string]bool{}}, &recordingMetrics{}
	req := func() *sessionv1.InvokeNotificationActionRequest {
		return &sessionv1.InvokeNotificationActionRequest{
			ActionKind: sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION, TargetKey: "item1", IdempotencyKey: "k1",
			Params: &sessionv1.NotificationActionParams{Choice: notify.ChoiceMerge, Confirm: true},
		}
	}
	first, err := d.Invoke(context.Background(), ActionCall{Msg: req()})
	require.NoError(t, err)
	require.Equal(t, outApplied, first.Outcome)
	require.EqualValues(t, 1, w.merges.Load())

	// A restart empties the in-memory cache; the item's own state is the guard.
	restarted := NewNotificationActionDispatcher(ActionDelegates{Backlog: w.decider()}, nil, testFlagRegistry())
	restarted.flags, restarted.metrics = fakeActionFlags{off: map[string]bool{}}, &recordingMetrics{}
	retry, err := restarted.Invoke(context.Background(), ActionCall{Msg: req()})
	require.NoError(t, err)
	assert.Equal(t, outResolved, retry.Outcome)
	assert.EqualValues(t, 1, w.merges.Load(), "no second merge call")
}

func TestBacklogDecider_ShouldRequireConfirmAndSession_WhenMerging(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusPRPending, sessionID: "work-1"}
	assert.Equal(t, outInvalid, decide(t, w.decider(), notify.ChoiceMerge, false).Outcome)
	assert.Zero(t, w.merges.Load())

	noSession := &itemWorld{status: notify.BacklogStatusReview}
	assert.Equal(t, outInvalid, decide(t, noSession.decider(), notify.ChoiceMerge, true).Outcome)
	assert.Zero(t, noSession.merges.Load())
}

func TestBacklogDecider_ShouldReportStale_WhenItemMovedOrGone(t *testing.T) {
	t.Parallel()
	moved := &itemWorld{status: "in_progress"}
	res := decide(t, moved.decider(), notify.ChoiceAccept, false)
	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "item_moved", res.StaleReason)

	gone := &itemWorld{statusErr: errItemNotFound}
	res = decide(t, gone.decider(), notify.ChoiceAccept, false)
	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "item_gone", res.StaleReason)
}

func TestBacklogDecider_ShouldKeepRPCGuards_WhenTransitionRefused(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusReview, mergeErr: connect.NewError(connect.CodeFailedPrecondition, errors.New("verdict required"))}
	res := decide(t, w.decider(), notify.ChoiceAccept, false)
	assert.Equal(t, outInvalid, res.Outcome)
	assert.Contains(t, res.Detail, "verdict required")
}

func TestBacklogDecider_ShouldRejectUnknownChoice(t *testing.T) {
	t.Parallel()
	w := &itemWorld{status: notify.BacklogStatusReview}
	assert.Equal(t, outInvalid, decide(t, w.decider(), "teleport", true).Outcome)
}
