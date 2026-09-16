package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestNotificationPushGateClassification_ForkPressureAndTmuxRecovered covers the two
// server-package rows of the classification table validated against real
// notification-history data (2026-09, notification push-gate redesign). See
// server/services/notification_classification_test.go for the other server-side rows.
func TestNotificationPushGateClassification_ForkPressureAndTmuxRecovered(t *testing.T) {
	t.Run("Fork Pressure warning", func(t *testing.T) {
		event, _ := buildForkPressureNotification(tmux.ForkPressureWarning, tmux.ForkPressureStats{})
		assert.Contains(t, event.NotificationTitle, "Fork Pressure")
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM), event.NotificationPriority,
			"urgent=true, important=false — transient self-monitoring telemetry, must never push")
	})

	t.Run("Fork Pressure critical", func(t *testing.T) {
		event, _ := buildForkPressureNotification(tmux.ForkPressureCritical, tmux.ForkPressureStats{})
		assert.Contains(t, event.NotificationTitle, "Fork Pressure")
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM), event.NotificationPriority,
			"urgent=true, important=false — same as warning, critical must not push either")
	})

	t.Run("Tmux Server Recovered", func(t *testing.T) {
		event := buildTmuxServerRecoveredNotification()
		assert.Equal(t, "Tmux Server Recovered", event.NotificationTitle)
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW), event.NotificationPriority,
			"urgent=false, important=false — recovery already happened, no operator action needed")
	})
}

// TestBuildForkPressureNotification_StableIDAndType is the regression test for
// backlog item cfda07b7-73fb-42e1-a21b-7fdf8a052a14's notification-flapping fix:
// every level (including the clear) must carry the same notification ID and
// NotificationType, so a Warning->Critical escalation and eventual clear update
// one notification record in place (server/notifications/store.go's Append)
// instead of each becoming a new, separately-tracked record. Severity is
// instead conveyed via title/metadata, which must still vary by level.
func TestBuildForkPressureNotification_StableIDAndType(t *testing.T) {
	warning, _ := buildForkPressureNotification(tmux.ForkPressureWarning, tmux.ForkPressureStats{Level: tmux.ForkPressureWarning})
	critical, _ := buildForkPressureNotification(tmux.ForkPressureCritical, tmux.ForkPressureStats{Level: tmux.ForkPressureCritical})
	cleared, _ := buildForkPressureNotification(tmux.ForkPressureOK, tmux.ForkPressureStats{Level: tmux.ForkPressureOK})

	assert.Equal(t, warning.NotificationID, critical.NotificationID, "ID must stay stable Warning -> Critical so Append() merges in place")
	assert.Equal(t, warning.NotificationID, cleared.NotificationID, "ID must stay stable through the clear so Append() merges in place")
	assert.Equal(t, warning.NotificationType, critical.NotificationType, "NotificationType must stay constant Warning -> Critical so the (SessionID, NotificationType) dedup key keeps matching")
	assert.Equal(t, warning.NotificationType, cleared.NotificationType, "NotificationType must stay constant through the clear")

	// Severity must still be distinguishable via title/metadata even though ID
	// and NotificationType are identical.
	assert.NotEqual(t, critical.NotificationTitle, cleared.NotificationTitle)
	assert.Equal(t, "warning", warning.NotificationMetadata["fork_pressure_level"])
	assert.Equal(t, "critical", critical.NotificationMetadata["fork_pressure_level"])
	assert.Equal(t, "ok", cleared.NotificationMetadata["fork_pressure_level"])
}
