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
	SweepTerminalWakeTriggers(ctx context.Context, now time.Time, retention time.Duration) (int, error)
	CancelWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	ListWakeTriggersByOwner(ctx context.Context, owner string, statuses ...domain.WakeStatus) ([]*session.WakeTriggerData, error)
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
	// WorkspaceKey names the repository the session works in (Instance.WorkspaceKey),
	// "" when unknown; peer help is limited to one workspace.
	WorkspaceKey string
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
	// NotifyPeerStalled: an open peer-help request nobody answered or whose
	// answerer ended (INPUT_REQUIRED, bound to the asker).
	NotifyPeerStalled
)

// Notification is a request to tell the operator something.
type Notification struct {
	Kind NotificationKind
	// OwnerSessionUUID is the wake's owner. A reminder carries it only as
	// metadata.target_session_uuid (no session binding); a failure is bound to it.
	OwnerSessionUUID string
	Title            string
	Message          string
	// GuidanceRequestID is the stalled request of a NotifyPeerStalled; it keys
	// the notification's actions.
	GuidanceRequestID string
	// PeerTitle names the unresponsive answerer, for the "Answer for {peer}" label.
	PeerTitle string
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
	// SessionRoles are the roles of the item's attached ItemSessions.
	SessionRoles []string
}

// BacklogState reads the persisted item state used by the reconcile predicates.
type BacklogState interface {
	ItemState(ctx context.Context, itemID string) (ItemState, error)
}

// SessionFacts is a peer session's current state label. State is "idle" for a
// live session ready for input, otherwise the lowercase lifecycle status.
type SessionFacts struct {
	Exists bool
	State  string
}

// SessionStates reads a session's persisted-or-live state for session_state wakes.
type SessionStates interface {
	SessionState(ctx context.Context, sessionUUID string) (SessionFacts, error)
}

// GuidanceFacts is the persisted state of a guidance request.
type GuidanceFacts struct {
	Exists   bool
	Answered bool
	// Cancelled is a request withdrawn before an answer; the asker is woken
	// with "cancelled" so it stops waiting.
	Cancelled           bool
	CancelReason        string
	Answer              string
	AnswererKind        string
	AnswererSessionUUID string
}

// GuidanceState reads a guidance request for guidance_answered wakes.
type GuidanceState interface {
	Guidance(ctx context.Context, id string) (GuidanceFacts, error)
}

// PRKey names a pull request as "owner/repo" and number.
type PRKey struct {
	Repo   string
	Number int
}

// PRView is one open PR of a poll: its identity and combined check conclusion
// ("success", "failure", "pending" or "").
type PRView struct {
	PRKey
	CheckConclusion string
}

// PRFacts is the by-number lookup of a PR that left the open list.
type PRFacts struct {
	State           string // "open", "merged" or "closed"
	CheckConclusion string
}

// PR states PRFacts.State reports.
const (
	PRStateOpen   = "open"
	PRStateMerged = "merged"
	PRStateClosed = "closed"
)

// PRSource is the server-side PR poller (github.UserPRCache, Spike S4): no
// typed bus event exists for PR or CI changes, so the dispatcher diffs its
// snapshots. Subscribe delivers the full open-PR list after each poll.
type PRSource interface {
	Subscribe(ctx context.Context) (<-chan []PRView, error)
	// Open returns the poller's current view of the PR, or false when the
	// poller does not hold it (not open, not the connected accounts' own, or past the first 100).
	Open(key PRKey) (PRView, bool)
	// Lookup reads a PR by number (the confirm step for one that vanished).
	Lookup(ctx context.Context, key PRKey) (PRFacts, error)
}
