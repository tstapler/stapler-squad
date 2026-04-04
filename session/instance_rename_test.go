package session

import (
	"errors"
	"testing"
)

func TestInstanceRename(t *testing.T) {
	tests := []struct {
		name         string
		currentTitle string
		newTitle     string
		wantErr      bool
		errType      error
	}{
		{
			name:         "successful rename",
			currentTitle: "old-session",
			newTitle:     "new-session",
			wantErr:      false,
		},
		{
			name:         "rename to same title",
			currentTitle: "same-session",
			newTitle:     "same-session",
			wantErr:      false,
		},
		{
			name:         "title too long",
			currentTitle: "normal-session",
			newTitle:     "this-is-an-extremely-long-session-title-that-exceeds-the-maximum-allowed-length-for-a-session-title-and-should-fail-validation-when-we-try-to-rename-the-instance-to-this-very-long-title",
			wantErr:      true,
			errType:      ErrInvalidTitleLength,
		},
		{
			name:         "title with invalid characters",
			currentTitle: "normal-session",
			newTitle:     "invalid/session",
			wantErr:      true,
			errType:      ErrInvalidTitleChars,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new instance
			inst := &Instance{
				Title: tt.currentTitle,
			}

			// Attempt to rename
			err := inst.Rename(tt.newTitle)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("Rename() expected error but got none")
				} else if tt.errType != nil && err != tt.errType {
					t.Errorf("Rename() error = %v, want %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("Rename() unexpected error: %v", err)
				}
				// Verify title was updated (or not, if same)
				if inst.Title != tt.newTitle {
					t.Errorf("Title = %s, want %s", inst.Title, tt.newTitle)
				}
			}
		})
	}
}

func TestInstanceRestart(t *testing.T) {
	// Note: This is a placeholder test. Full testing would require mocking tmux.TmuxSession
	// and other dependencies, which is beyond the scope of this basic implementation.

	tests := []struct {
		name           string
		started        bool
		status         Status
		preserveOutput bool
		wantErr        bool
		errType        error
	}{
		{
			name:           "restart unstarted instance",
			started:        false,
			status:         Ready,
			preserveOutput: false,
			wantErr:        true,
			errType:        ErrCannotRestart,
		},
		{
			name:           "restart paused instance",
			started:        true,
			status:         Paused,
			preserveOutput: false,
			wantErr:        true,
			errType:        ErrCannotRestart,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create instance with test state
			inst := &Instance{
				Title:   "test-session",
				started: tt.started,
				Status:  tt.status,
			}

			// Attempt restart
			err := inst.Restart(tt.preserveOutput)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("Restart() expected error but got none")
				} else if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("Restart() error = %v, want %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("Restart() unexpected error: %v", err)
				}
			}
		})
	}
}
