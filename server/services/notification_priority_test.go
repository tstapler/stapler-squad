package services

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"github.com/stretchr/testify/assert"
)

// TestDerivePriority_AllFourQuadrants covers every (urgent, important) combination the
// Eisenhower-style push-gate redesign relies on: derivePriority must be a pure,
// exhaustive mapping onto the four NotificationPriority levels.
func TestDerivePriority_AllFourQuadrants(t *testing.T) {
	tests := []struct {
		name      string
		urgent    bool
		important bool
		want      int32
	}{
		{"urgent and important → URGENT", true, true, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT)},
		{"important only → HIGH", false, true, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)},
		{"urgent only → MEDIUM", true, false, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)},
		{"neither → LOW", false, false, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, derivePriority(tt.urgent, tt.important))
		})
	}
}
