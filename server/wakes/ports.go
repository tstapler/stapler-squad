package wakes

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// The dispatcher depends on these ports only; server wiring supplies adapters
// over SessionService, BacklogService and the event bus (ADR-006 rule 3). This
// package must never import server/services or server/mcp.

// Store is the slice of the wake repository the dispatcher uses; both
// *session.EntRepository and *session.Storage satisfy it.
type Store interface {
	timerStore
	CreateWakeTrigger(ctx context.Context, in session.CreateWakeTriggerInput) (*session.WakeTriggerData, bool, error)
	GetWakeTrigger(ctx context.Context, id uuid.UUID) (*session.WakeTriggerData, error)
	MarkWakeDelivered(ctx context.Context, id uuid.UUID, now, nextFire time.Time) (bool, error)
	MarkWakeNeedsRevive(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	ReadmitWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	MarkWakeSkipped(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error)
	DeferWakeTrigger(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (int, bool, error)
	SkipWakeOccurrence(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (bool, error)
	RecordWakeDeliveryAttempt(ctx context.Context, id uuid.UUID) error
	ListPendingWakeTriggers(ctx context.Context) ([]*session.WakeTriggerData, error)
	ListStaleClaimedWakeTriggers(ctx context.Context, claimedBefore time.Time) ([]*session.WakeTriggerData, error)
	ListNeedsReviveWakeTriggers(ctx context.Context) ([]*session.WakeTriggerData, error)
}

// WriteOutcome is what a Steerer.WriteWake attempt did to the pane.
type WriteOutcome uint8

const (
	// WriteNotAttempted is the zero value: nothing reached the pane (lease busy,
	// dialog showing, indeterminate capture). The row stays claimed.
	WriteNotAttempted WriteOutcome = iota
	// WriteDelivered: the text was written and the submit confirmed.
	WriteDelivered
)

// Steerer writes a wake into an owner's pane. Unknown readiness means do not
// write: IsReadyForSteer is false whenever idleness cannot be confirmed, and
// WriteWake re-checks the live pane itself (see WriteIfIdle).
type Steerer interface {
	IsReadyForSteer(sessionUUID string) bool
	WriteWake(ctx context.Context, sessionUUID, text string) (WriteOutcome, error)
}

// Admission is the WIP-cap gate a revive of a stopped owner must pass
// (BacklogService.Admit). It reserves nothing; the revive lock spans the
// call through the revived session counting as live.
type Admission interface {
	Admit(ctx context.Context) (bool, error)
}

// ReviveTarget is the part of *session.Instance a revive touches.
type ReviveTarget interface {
	IsArchived() bool
	RecoverFromStopped()
	Start(firstTimeSetup bool) error
}

// OwnerFacts is everything the guards and the revive path need to know about a
// wake's owner at one instant. The zero value is "owner does not exist".
type OwnerFacts struct {
	Exists       bool
	Archived     bool
	OneShot      bool
	ItemTerminal bool
	// Live: tracked and running, so a pane write is possible. Exists && !Live
	// is a stopped owner.
	Live bool
	// Revive is set for a stopped owner whose instance can be restarted.
	Revive ReviveTarget
}

// OwnerLookup resolves an owner session by UUID.
type OwnerLookup interface {
	Owner(ctx context.Context, sessionUUID string) (OwnerFacts, error)
}

// NotificationKind classifies an operator notification raised by the dispatcher.
type NotificationKind uint8

const (
	// NotifyReminder: a delivered notify_operator wake (INFO, system-level).
	NotifyReminder NotificationKind = iota
	// NotifyFailure: a wake ended missed, skipped or failed (failure class so a
	// hidden-session owner still reaches the operator).
	NotifyFailure
)

// Notification is a request to tell the operator something.
type Notification struct {
	Kind NotificationKind
	// OwnerSessionUUID is the wake's owner. A reminder carries it only as
	// metadata.target_session_uuid (no session binding); a failure is bound to it.
	OwnerSessionUUID string
	Title            string
	Message          string
}

// Notifier raises operator notifications.
type Notifier interface {
	Notify(n Notification)
}

// EventSource is the in-process bus the dispatcher consumes; *events.EventBus
// satisfies it. Subscribe must be called before the first state read.
type EventSource interface {
	Subscribe(ctx context.Context) (<-chan *events.Event, string)
}

// ItemState is the persisted backlog-item state reconcile re-evaluates.
type ItemState struct {
	Exists     bool
	Archived   bool
	HasVerdict bool
	Status     string
}

// BacklogState reads the persisted item state used by the reconcile predicates.
type BacklogState interface {
	ItemState(ctx context.Context, itemID string) (ItemState, error)
}
