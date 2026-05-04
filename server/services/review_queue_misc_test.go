package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// --------------------------------------------------------------------------
// GetReviewQueue
// --------------------------------------------------------------------------

// TestGetReviewQueue_ReturnsEmpty verifies that a freshly-created
// ReviewQueueService returns an empty queue with no error.
func TestGetReviewQueue_ReturnsEmpty(t *testing.T) {
	rqs, _, _, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)

	resp, err := rqs.GetReviewQueue(context.Background(), connect.NewRequest(&sessionv1.GetReviewQueueRequest{}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ReviewQueue)
	require.Empty(t, resp.Msg.ReviewQueue.Items, "expected empty review queue on a fresh service")
}

// TestGetReviewQueue_WithItems verifies that items added to the review queue
// are returned by GetReviewQueue.
func TestGetReviewQueue_WithItems(t *testing.T) {
	rqs, _, _, cleanup := setupRQSFixture(t)
	t.Cleanup(cleanup)

	// Add an item directly to the underlying queue.
	rqs.reviewQueue.Add(&session.ReviewItem{
		SessionID:   "sess-abc",
		SessionName: "my-session",
		Reason:      session.ReasonIdle,
		Priority:    session.PriorityLow,
	})

	resp, err := rqs.GetReviewQueue(context.Background(), connect.NewRequest(&sessionv1.GetReviewQueueRequest{}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ReviewQueue)
	require.Len(t, resp.Msg.ReviewQueue.Items, 1, "expected one item in the review queue")
	require.Equal(t, "sess-abc", resp.Msg.ReviewQueue.Items[0].SessionId)
}
