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

// resolveHistoryAdapter returns the HistoryAdapter that claims to handle program, or nil if
// none does. This is the single resolution point shared by PortSessionHistory and the
// checkpoint create/fork paths so adding or changing adapter coverage only needs one edit.
func resolveHistoryAdapter(program string) HistoryAdapter {
	if claude := NewClaudeAdapter(); claude.CanHandle(program) {
		return claude
	}
	if agy := NewAgyAdapter(); agy.CanHandle(program) {
		return agy
	}
	return nil
}
