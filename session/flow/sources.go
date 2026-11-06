package flow

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// EventRange selects events with From <= At < To (parsed instants). A zero To
// is unbounded. The aggregator passes From already padded by Config.CollapsePad.
// ItemIDs, when non-empty, restricts the read to those items.
type EventRange struct {
	From    time.Time
	To      time.Time
	ItemIDs []ItemID
	// ChunkItems and BatchTimeout are StreamEvents hints; zero uses the
	// reader's defaults. Events and CountEvents ignore them.
	ChunkItems   int
	BatchTimeout time.Duration
}

// EventSource reads normalized status events. Request-review events are
// ordinary Events filtered on To=review, TriggeredBy=agent.
type EventSource interface {
	Events(ctx context.Context, rng EventRange) ([]Event, error)
	CountEvents(ctx context.Context, rng EventRange) (int, error)
	// StreamEvents delivers the range in item-batches, for windows above the row ceiling.
	StreamEvents(ctx context.Context, rng EventRange, fn func(batch []Event) error) error
	// LastEventsBefore returns each listed item's latest event strictly before
	// start (an empty ids list returns nothing).
	LastEventsBefore(ctx context.Context, start time.Time, ids []ItemID) ([]Event, error)
}

// VerdictSource reads review verdicts.
type VerdictSource interface {
	Verdicts(ctx context.Context, window FlowWindow) ([]Verdict, error)
	// AgentDoneClaims reports, per item, whether the agent claimed done on a
	// non-PASS verdict. Informational only (ADR-005).
	AgentDoneClaims(ctx context.Context, window FlowWindow) (map[ItemID]bool, error)
}

// StuckSource reads stuck intervals; Phase 5 episodes plug in behind it.
type StuckSource interface {
	StuckIntervals(ctx context.Context) ([]StuckInterval, error)
}

// StuckCaptureSource is optionally implemented by a StuckSource that records
// episodes; the aggregator uses it to report open-count drift.
type StuckCaptureSource interface {
	StuckCapture(ctx context.Context) (StuckCapture, error)
}

// ItemMeta is the item data the aggregator needs. Title is raw text.
type ItemMeta struct {
	ID     ItemID
	Title  string
	Status domain.BacklogStatus
}

// ItemSource reads item metadata and the pipeline stage order.
type ItemSource interface {
	Items(ctx context.Context, ids []ItemID) ([]ItemMeta, error)
	Stages(ctx context.Context) ([]domain.BacklogStatus, error)
}

// Sources groups the role interfaces so a section fails independently of the others.
type Sources struct {
	Events   EventSource
	Verdicts VerdictSource
	Stuck    StuckSource
	Items    ItemSource
}
