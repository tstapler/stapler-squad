package ui

import (
	"claude-squad/session"
	"claude-squad/session/git"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

func TestInstanceRendererWithDifferentStatuses(t *testing.T) {
	// Create a spinner for the renderer
	s := spinner.New()
	s.Spinner = spinner.Dot

	// Create renderer
	renderer := &InstanceRenderer{
		spinner: &s,
		width:   100,
	}

	// Setup a mock instance with minimal required fields
	instance := &session.Instance{
		Title:     "Test Instance",
		Branch:    "test-branch",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Helper to set instance to started state with a mock git worktree
	setupStartedInstance := func(i *session.Instance) {
		i.SetTmuxSession(nil) // Just needs to be non-nil for Started() check
		mockWorktree := git.NewGitWorktreeFromStorage(
			"/path/to/repo",
			"/path/to/worktree",
			i.Title,
			i.Branch,
			"abcdef1234567890",
		)
		// Set the git worktree using reflection since there's no public setter
		i.SetGitWorktree(mockWorktree)
	}

	// Test rendering for each status type
	tests := []struct {
		name            string
		status          session.Status
		setupFunc       func(*session.Instance)
		expectedContent []string // Substrings we expect to find in the output
	}{
		{
			name:   "Running Status",
			status: session.Running,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch"}, // Can't reliably test spinner character
		},
		{
			name:   "Ready Status",
			status: session.Ready,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", readyIcon},
		},
		{
			name:   "Paused Status",
			status: session.Paused,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", pausedIcon},
		},
		{
			name:   "NeedsApproval Status",
			status: session.NeedsApproval,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", needsApprovalIcon},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Reset instance for each test
			instance.Status = test.status
			if test.setupFunc != nil {
				test.setupFunc(instance)
			}

			// Render the instance
			result := renderer.Render(instance, 1, false, true)

			// Check that expected content is in the result
			for _, expected := range test.expectedContent {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected rendered output to contain '%s', but got: %s", expected, result)
				}
			}

			// For NeedsApproval, specifically check that the icon is rendered with the right style
			if test.status == session.NeedsApproval {
				expectedStyledIcon := needsApprovalStyle.Render(needsApprovalIcon)
				if !strings.Contains(result, expectedStyledIcon) {
					t.Errorf("Expected NeedsApproval status to be rendered with needsApprovalStyle, but styled icon '%s' not found in: %s", 
						expectedStyledIcon, result)
				}
			}
		})
	}
}