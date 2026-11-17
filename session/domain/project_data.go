package domain

import "time"

// ProjectData is the domain model for a project that groups sessions.
type ProjectData struct {
	// ID is the unique project name (used as string external identifier)
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
