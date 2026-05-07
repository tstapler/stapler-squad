package unfinished

import (
	"time"

	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
)

const (
	// EventUnfinishedWorkUpdated is published when a worktree scan result changes.
	EventUnfinishedWorkUpdated pkgevents.EventType = "unfinished.work_updated"
	// EventUnfinishedWorkRemoved is published when a worktree is dismissed/snoozed/gone.
	EventUnfinishedWorkRemoved pkgevents.EventType = "unfinished.work_removed"
	// EventUnfinishedScanCompleted is published after each full scan pass.
	EventUnfinishedScanCompleted pkgevents.EventType = "unfinished.scan_completed"
)

// UnfinishedEvent extends pkgevents.Event with extra fields for unfinished-work events.
// It reuses the existing pkgevents.Event type by embedding the ScanResult in Context.
type UnfinishedEvent struct {
	pkgevents.Event
	ScanResult  ScanResult
	CompletedAt time.Time
}

func newUnfinishedWorkUpdatedEvent(r ScanResult) *pkgevents.Event {
	return &pkgevents.Event{
		Type:      EventUnfinishedWorkUpdated,
		Timestamp: time.Now(),
		Context:   r.RepoPath + "|" + r.Branch,
		// The service handler retrieves the actual ScanResult from the Scanner.
	}
}

func newScanCompletedEvent() *pkgevents.Event {
	return &pkgevents.Event{
		Type:      EventUnfinishedScanCompleted,
		Timestamp: time.Now(),
	}
}
