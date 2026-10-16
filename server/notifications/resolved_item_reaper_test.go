package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendItemNotif(t *testing.T, s *NotificationHistoryStore, id, itemID, title string) {
	t.Helper()
	require.NoError(t, s.Append(&NotificationRecord{
		ID: id, SessionID: itemID + title, NotificationType: 7, Title: title,
		Metadata: map[string]string{"item_id": itemID}, CreatedAt: time.Now(),
	}))
}

func unreadIDs(t *testing.T, s *NotificationHistoryStore) map[string]bool {
	t.Helper()
	recs, _, err := s.List(ListOptions{UnreadOnly: true, Limit: MaxNotifications})
	require.NoError(t, err)
	out := map[string]bool{}
	for _, r := range recs {
		out[r.ID] = true
	}
	return out
}

func TestReapResolvedItemNotifications_should_MarkReadOnlyResolvedItems(t *testing.T) {
	s := newTestStore(t)
	appendItemNotif(t, s, "done-any", "done-item", "Autonomous fix stuck")
	appendItemNotif(t, s, "live-multi-resolved", "live-item", multiReasonTitle)
	appendItemNotif(t, s, "live-autonomous", "live-item", "Autonomous fix stuck")
	appendItemNotif(t, s, "stuck-multi", "stuck-item", multiReasonTitle)
	appendItemNotif(t, s, "unknown", "gone-item", multiReasonTitle)

	lookup := func(_ context.Context, _ []string) (map[string]ItemState, error) {
		return map[string]ItemState{
			"done-item":  {Terminal: true},
			"live-item":  {},
			"stuck-item": {HasOpenStuck: true},
		}, nil
	}
	n, err := ReapResolvedItemNotifications(context.Background(), s, lookup)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	unread := unreadIDs(t, s)
	assert.False(t, unread["done-any"], "terminal item's notification is retired")
	assert.False(t, unread["live-multi-resolved"], "escalation with no open stuck row is retired")
	assert.True(t, unread["live-autonomous"], "non-escalation notification on a live item is kept")
	assert.True(t, unread["stuck-multi"], "escalation still backed by an open stuck row is kept")
	assert.True(t, unread["unknown"], "item the lookup did not return is left alone")
}
