package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/notifications"
)

// The same stored history page is served to both listeners; only the stamped
// request record differs (Task 2.2d).
func TestGetNotificationHistory_ShouldMarkTierBDesktopOnlyOnRemoteAndAvailableOnLocal_WhenSamePageServed(t *testing.T) {
	t.Parallel()
	store, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	require.NoError(t, store.Append(&notifications.NotificationRecord{
		ID: "n1", SessionID: "s1", NotificationType: int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
		Title: "Question", CreatedAt: time.Now(), Metadata: map[string]string{metaQuestionID: "q1"},
	}))

	f := newDescriptorFixture()
	q := PendingQuestion{QuestionID: "q1", SessionUUID: "s1"}
	q.Dialog.Labels = []string{"Redis", "Memcached"}
	f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q1"}] = q

	ns := &NotificationService{notificationStore: store}
	ns.SetDescriptorBuilder(f.builder())

	serve := func(ctx context.Context) *sessionv1.NotificationActionDescriptor {
		resp, err := ns.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.Notifications, 1)
		acts := resp.Msg.Notifications[0].Actions
		require.Len(t, acts, 2)
		return acts[0]
	}

	local := serve(stampedContext(t, nil, ListenerLocal, false, "localhost:8543", nil))
	remote := serve(stampedContext(t, nil, ListenerRemote, true, "onyx:8444", nil))

	assert.True(t, local.Available)
	assert.False(t, remote.Available)
	assert.Equal(t, "desktop_only", remote.UnavailableReason)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, remote.Tier)
}
