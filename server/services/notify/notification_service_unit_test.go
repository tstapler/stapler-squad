package notify

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/notifications"
)

func TestValidateLocalhostOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		peerAddr      string
		expectedError bool
	}{
		{
			name:          "Localhost IPv4",
			peerAddr:      "127.0.0.1:12345",
			expectedError: false,
		},
		{
			name:          "External IP",
			peerAddr:      "192.168.1.1:12345",
			expectedError: true,
		},
		{
			name:          "IPv6 Localhost",
			peerAddr:      "[::1]:12345",
			expectedError: false,
		},
		{
			name:          "Missing IP",
			peerAddr:      "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateLocalhostAddr(tt.peerAddr)
			if (err != nil) != tt.expectedError {
				t.Errorf("validateLocalhostAddr(%q) error = %v, expectedError %v", tt.peerAddr, err, tt.expectedError)
			}
		})
	}
}

// The history record reports the same pending-decision answer as the predicate.
func TestRecordToProto_ShouldSetIsPendingDecision_WhenUnreadWarningInfoAndAutoRemediatingWarning(t *testing.T) {
	t.Parallel()
	mk := func(typ sessionv1.NotificationType, md map[string]string) *notifications.NotificationRecord {
		return &notifications.NotificationRecord{ID: "n", NotificationType: int32(typ), Metadata: md, CreatedAt: time.Unix(1700000000, 0)}
	}
	warning := sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
	require.True(t, recordToProto(mk(warning, nil)).IsPendingDecision)
	require.False(t, recordToProto(mk(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO, nil)).IsPendingDecision)
	require.False(t, recordToProto(mk(warning, map[string]string{"auto_remediating": "true"})).IsPendingDecision)

	read := mk(warning, nil)
	read.IsRead = true
	require.False(t, recordToProto(read).IsPendingDecision)
}

// T-PR-13: a request listing a pending decision deletes the rest and reports
// the pending id in kept_ids; the pending row survives.
func TestClearNotificationHistory_ShouldReturnKeptIds_WhenRequestListsPendingDecision(t *testing.T) {
	t.Parallel()
	store, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	for _, r := range []*notifications.NotificationRecord{
		{ID: "pending", SessionID: "s1", NotificationType: int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED), CreatedAt: time.Now()},
		{ID: "info-1", SessionID: "s2", NotificationType: int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), CreatedAt: time.Now()},
		{ID: "info-2", SessionID: "s3", NotificationType: int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), CreatedAt: time.Now()},
	} {
		require.NoError(t, store.Append(r))
	}
	ns := &NotificationService{notificationStore: store}

	resp, err := ns.ClearNotificationHistory(context.Background(),
		connect.NewRequest(&sessionv1.ClearNotificationHistoryRequest{NotificationIds: []string{"pending", "info-1"}}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)
	require.EqualValues(t, 1, resp.Msg.ClearedCount)
	require.Equal(t, []string{"pending"}, resp.Msg.KeptIds)
	_, ok := store.GetByID("pending")
	require.True(t, ok)
	_, ok = store.GetByID("info-2")
	require.True(t, ok, "an unlisted row must survive")
}
