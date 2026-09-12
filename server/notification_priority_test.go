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
