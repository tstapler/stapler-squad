package unfinished

import (
	"time"

	"github.com/tstapler/stapler-squad/server/events"
)

const (
	// EventUnfinishedWorkUpdated is published when a worktree scan result changes.
	EventUnfinishedWorkUpdated events.EventType = "unfinished.work_updated"
	// EventUnfinishedWorkRemoved is published when a worktree is dismissed/snoozed/gone.
	EventUnfinishedWorkRemoved events.EventType = "unfinished.work_removed"
	// EventUnfinishedScanCompleted is published after each full scan pass.
	EventUnfinishedScanCompleted events.EventType = "unfinished.scan_completed"
)

// UnfinishedEvent extends events.Event with extra fields for unfinished-work events.
// It reuses the existing events.Event type by embedding the ScanResult in Context.
type UnfinishedEvent struct {
	events.Event
	ScanResult  ScanResult
	CompletedAt time.Time
}

func newUnfinishedWorkUpdatedEvent(r ScanResult) *events.Event {
	return &events.Event{
		Type:      EventUnfinishedWorkUpdated,
		Timestamp: time.Now(),
		Context:   r.RepoPath + "|" + r.Branch,
		// The service handler retrieves the actual ScanResult from the Scanner.
	}
}

func newScanCompletedEvent() *events.Event {
	return &events.Event{
		Type:      EventUnfinishedScanCompleted,
		Timestamp: time.Now(),
	}
}
