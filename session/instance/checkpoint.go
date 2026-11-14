package instance

import (
	"github.com/google/uuid"
)

// newCheckpointID generates a new unique checkpoint ID.
func newCheckpointID() string {
	return uuid.New().String()
}
