package session

import (
	"context"
)

type HistoryAdapter interface {
	Name() string
	CanHandle(program string) bool

	// Import reads this CLI's native format and returns canonical turns.
	Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error)

	// Export writes canonical turns into this CLI's native format so it can resume.
	Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error
}
