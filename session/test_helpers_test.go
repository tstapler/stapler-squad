package session

import (
	"time"
)

// createTestSession creates a minimal InstanceData for use in tests.
func createTestSession(title string) InstanceData {
	now := time.Now()
	return InstanceData{
		Title:      title,
		Path:       "/home/user/project",
		WorkingDir: "/home/user/project",
		Branch:     "main",
		Status:     Paused,
		Height:     24,
		Width:      80,
		CreatedAt:  now,
		UpdatedAt:  now,
		Program:    "claude",
		Category:   "Test",
		Tags:       []string{},
	}
}
