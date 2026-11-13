package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"time"
)

// createTestSession creates a minimal InstanceData for use in tests.
func createTestSession(title string) domain.InstanceData {
	now := time.Now()
	return domain.InstanceData{
		Title:      title,
		Path:       "/home/user/project",
		WorkingDir: "/home/user/project",
		Branch:     "main",
		Status:     domain.Paused,
		Height:     24,
		Width:      80,
		CreatedAt:  now,
		UpdatedAt:  now,
		Program:    "claude",
		Category:   "Test",
		Tags:       []string{},
	}
}
