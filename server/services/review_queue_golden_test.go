package services

// Differential golden for the review queue RPCs and filter conversions, recorded
// before the package move; the body is unchanged by it.
// Regenerate with -update-review-queue-golden.

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/queue"
)

var updateReviewQueueGolden = flag.Bool("update-review-queue-golden", false, "rewrite testdata/review_queue.golden")

func TestReviewQueueGolden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	for v := range sessionv1.Priority_name {
		p := sessionv1.Priority(v)
		add("priority %s -> %v", p, convertProtoPriorities([]sessionv1.Priority{p}))
	}
	for v := range sessionv1.AttentionReason_name {
		r := sessionv1.AttentionReason(v)
		add("reason %s -> %v", r, convertProtoReasons([]sessionv1.AttentionReason{r}))
	}
	filters := &WatchReviewQueueFilters{
		PriorityFilter: []queue.Priority{queue.PriorityHigh}, ReasonFilter: []queue.AttentionReason{queue.ReasonErrorState},
		SessionIDs: []string{"a"}, IncludeStatistics: true, InitialSnapshot: true,
	}
	add("filters %v %v %v %v %v", filters.GetPriorityFilter(), filters.GetReasonFilter(), filters.GetSessionIDs(),
		filters.GetIncludeStatistics(), filters.GetInitialSnapshot())

	q := queue.NewReviewQueue()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := NewReviewQueueService(q, storage, bus)
	store := approvalkit.NewApprovalStore("")
	require.NoError(t, store.Create(&approvalkit.PendingApproval{ID: "approval-1", SessionID: "needs-approval", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}))
	svc.SetApprovalStore(store)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	q.Add(&queue.ReviewItem{SessionID: "needs-approval", SessionName: "a", Reason: queue.ReasonApprovalPending, Priority: queue.PriorityHigh, DetectedAt: at, LastActivity: at})
	q.Add(&queue.ReviewItem{SessionID: "errored", SessionName: "b", Reason: queue.ReasonErrorState, Priority: queue.PriorityUrgent, DetectedAt: at, LastActivity: at})

	high := sessionv1.Priority_PRIORITY_HIGH
	errored := sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE
	for label, req := range map[string]*sessionv1.GetReviewQueueRequest{
		"all": {}, "priority": {PriorityFilter: &high}, "reason": {ReasonFilter: &errored},
	} {
		res, err := svc.GetReviewQueue(ctx, connect.NewRequest(req))
		require.NoError(t, err)
		ids := []string{}
		for _, item := range res.Msg.ReviewQueue.Items {
			ids = append(ids, fmt.Sprintf("%s/%s/%s/%s", item.SessionId, item.Priority, item.Reason, item.Metadata["pending_approval_id"]))
		}
		slices.Sort(ids)
		add("queue %s -> %v total=%d", label, ids, res.Msg.ReviewQueue.TotalItems)
	}

	for _, id := range []string{"", "errored", "unknown"} {
		res, err := svc.AcknowledgeSession(ctx, connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{Id: id}))
		msg := ""
		if err == nil {
			msg = svctest.CanonicalProto(t, res.Msg)
		}
		add("ack %q -> %s %s remaining=%d", id, msg, svctest.CanonicalError(err), len(q.List()))
	}
	watchErr := svc.WatchReviewQueue(ctx, connect.NewRequest(&sessionv1.WatchReviewQueueRequest{}), nil)
	add("watch unwired -> %s manager=%v", svctest.CanonicalError(watchErr), svc.GetReactiveQueueManager())

	slices.Sort(out)
	svctest.CompareGolden(t, filepath.Join("testdata", "review_queue.golden"), out, *updateReviewQueueGolden)
}
