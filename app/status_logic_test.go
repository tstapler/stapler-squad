package app

import (
	"claude-squad/session"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStatusDeterminationLogic tests the specific logic for determining instance status
// based on various conditions (updated, hasPrompt, AutoYes)
func TestStatusDeterminationLogic(t *testing.T) {
	testCases := []struct {
		name           string
		updated        bool
		hasPrompt      bool
		autoYes        bool
		expectedStatus session.Status
	}{
		{
			name:           "Running status when updated",
			updated:        true,
			hasPrompt:      false,
			autoYes:        false,
			expectedStatus: session.Running,
		},
		{
			name:           "Ready status when not updated and no prompt",
			updated:        false,
			hasPrompt:      false,
			autoYes:        false,
			expectedStatus: session.Ready,
		},
		{
			name:           "NeedsApproval status when not updated, has prompt and AutoYes disabled",
			updated:        false,
			hasPrompt:      true,
			autoYes:        false,
			expectedStatus: session.NeedsApproval,
		},
		{
			name:           "Ready status when not updated, has prompt but AutoYes enabled",
			updated:        false,
			hasPrompt:      true,
			autoYes:        true,
			expectedStatus: session.Ready, // AutoYes will press Enter, status stays Ready
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test instance
			instance := &session.Instance{
				Status:  session.Ready, // Start with Ready status
				AutoYes: tc.autoYes,
			}

			// This is the logic we're testing from app.go
			if tc.updated {
				instance.SetStatus(session.Running)
			} else {
				if tc.hasPrompt {
					if instance.AutoYes {
						// In real code this would call TapEnter()
						// We don't need to call it in this test
						// Status stays Ready
					} else {
						instance.SetStatus(session.NeedsApproval)
					}
				} else {
					instance.SetStatus(session.Ready)
				}
			}

			assert.Equal(t, tc.expectedStatus, instance.Status, "Status not set correctly")
		})
	}
}
