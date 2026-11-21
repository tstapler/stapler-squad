package mcp

import "context"

// TerminalScrollback is the slice of the scrollback manager the terminal tools
// read. *scrollback.ScrollbackManager and a test fake are its two implementations.
type TerminalScrollback interface {
	CurrentSequence(sessionID string) uint64
	GetRecentBytes(sessionID string, bytes int64) ([]byte, error)
	// LastEntrySequence is the sequence of the last of the first limit retained entries; 0 if none or unreadable.
	LastEntrySequence(sessionID string, limit int) uint64
}

// AutoReopener transitions a backlog item back to in_progress and respawns work
// after a failed review verdict. *backlogsvc.BacklogService and a test fake are its
// two implementations.
type AutoReopener interface {
	AutoReopenAfterFailedReview(ctx context.Context, itemID string) error
}
