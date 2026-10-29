// Package prwatch consumes the gh-signal agent inbox and steers the session
// that owns a PR when review, comment or CI activity lands on it. It defines
// its own ports and enums and must not import server/services, github or
// config (ADR-001 decision 6); server/ adapters implement the ports.
package prwatch

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// Kind is the event kind prwatch acts on. It is prwatch's own enum; the
// server adapter maps it onto services.PRWatchKind.
type Kind string

const (
	KindReview        Kind = "review"
	KindComment       Kind = "comment"
	KindReviewComment Kind = "review_comment"
	KindCI            Kind = "ci"
)

// Rendered is the word used in the steer text and operator output.
func (k Kind) Rendered() string {
	switch k {
	case KindReview:
		return "review"
	case KindComment:
		return "comment"
	case KindReviewComment:
		return "inline comment"
	case KindCI:
		return "CI failure"
	}
	return string(k)
}

// KindClass is the coalescing class: human-feedback or ci (D12).
type KindClass string

const (
	ClassHumanFeedback KindClass = "human-feedback"
	ClassCI            KindClass = "ci"
)

// Class returns the coalescing class of the kind.
func (k Kind) Class() KindClass {
	if k == KindCI {
		return ClassCI
	}
	return ClassHumanFeedback
}

// Outcome mirrors services.SteerOutcome.
type Outcome string

const (
	OutcomeDelivered      Outcome = "delivered"
	OutcomeDuplicate      Outcome = "duplicate"
	OutcomeBusy           Outcome = "busy"
	OutcomeGuardBusy      Outcome = "guard_busy"
	OutcomeCoolingDown    Outcome = "cooling_down"
	OutcomeNoStatusSource Outcome = "no_status_source"
	OutcomeNotTracked     Outcome = "not_tracked"
	OutcomeFailed         Outcome = "failed"
	OutcomeUnspecified    Outcome = "unspecified"
)

// SessionState is a session's liveness as the Sessions port reports it.
type SessionState string

const (
	StateLive      SessionState = "live"
	StateSuspended SessionState = "suspended" // paused, hibernated or crashed
	StateGone      SessionState = "gone"      // stopped, failed, archived or removed
)

// SessionRef describes one session as the watch set sees it.
type SessionRef struct {
	UUID  string
	Title string // sanitized, short; operator surface only
	Repo  string // validated owner/name
	PR    int
	State SessionState
	// ResumedAt is set when a suspended session became live again in this
	// process; zero otherwise.
	ResumedAt time.Time
	// SkipReason is non-empty when the session is not watched.
	SkipReason SkipReason
}

// Inbox is the gh-signal inbox client. Contract: every call is loopback-only
// with a per-request timeout; Events returns the oldest pending items
// (at most 200); Ack takes Item.ID (not Event.ID) and is idempotent;
// Unsubscribe takes the subscription ID. Errors are typed by the client
// package. Implementations must be safe for sequential use by one loop.
type Inbox interface {
	Watch(ctx context.Context, consumerID, repo string, pr int) (inbox.WatchResult, error)
	Subscribe(ctx context.Context, consumerID, repo string, pr int) (inbox.Subscription, error)
	Subscriptions(ctx context.Context, consumerID string) ([]inbox.Subscription, error)
	Events(ctx context.Context, consumerID string) ([]inbox.Item, error)
	Ack(ctx context.Context, itemID string) error
	Unsubscribe(ctx context.Context, subscriptionID string) error
	Health(ctx context.Context) (inbox.Health, error)
}

// Sessions is the session view. Contract: List returns the retained set
// (live and suspended) plus sessions skipped with a SkipReason, and returns
// an error without a partial list on failure so callers fail closed (change
// nothing). Program, IsHidden and IsBusy answer for one session UUID; unknown
// sessions are not hidden and not busy. IsBusy is true while the session is
// not ready for a steer (D17).
type Sessions interface {
	List(ctx context.Context) ([]SessionRef, error)
	Program(sessionUUID string) string
	IsHidden(sessionUUID string) bool
	IsBusy(sessionUUID string) bool
}

// SteerRequest is one steer. Text is the fully rendered fixed template; the
// adapter must not alter it. Kinds drive the guard signature.
type SteerRequest struct {
	SessionUUID string
	Repo        string
	PR          int
	Kinds       []Kind
	Text        string
}

// Steerer writes a steer through the guarded path. Contract: it refuses a
// hidden session (OutcomeDeliveredis never returned for one: it returns an
// error), writes nothing for any outcome other than OutcomeDelivered, and
// returns a non-nil error only for infrastructure failure.
type Steerer interface {
	Steer(ctx context.Context, req SteerRequest) (Outcome, error)
}

// NotificationKind is prwatch's own notification kind (D15).
type NotificationKind string

const (
	NotifyCoverageGap     NotificationKind = "coverage_gap"
	NotifySteerFailed     NotificationKind = "steer_failed"
	NotifyHeldLong        NotificationKind = "held_long"
	NotifyHeldDropped     NotificationKind = "held_dropped"
	NotifyLoginUnresolved NotificationKind = "login_unresolved"
	NotifyInboxState      NotificationKind = "inbox_state"
	NotifyStrangerActive  NotificationKind = "stranger_activity"
)

// InboxState is the inbox/leader health state (D15). InboxStateOK means
// healthy.
type InboxState string

const (
	InboxStateOK            InboxState = "ok"
	InboxStateUnreachable   InboxState = "unreachable"
	InboxStateUnauthorized  InboxState = "unauthorized"
	InboxStateTokenMissing  InboxState = "token_missing"
	InboxStateTokenPerms    InboxState = "token_permissions"
	InboxStateNonLoopback   InboxState = "non_loopback_url"
	InboxStateLeaderNotHeld InboxState = "leader_not_held"
)

// Notification is one operator notification. DedupeKey identifies the row:
// a repeat with the same key overwrites rather than adds.
type Notification struct {
	Kind        NotificationKind
	State       InboxState // set for NotifyInboxState only
	SessionUUID string
	DedupeKey   string
	Title       string
	Message     string
}

// Notifier publishes operator notifications. Contract: Notify never blocks
// the loop for long, treats DedupeKey as an overwrite key, and its errors
// are logged by the caller, never retried inline.
type Notifier interface {
	Notify(n Notification) error
}

// Clock supplies time so tests are deterministic.
type Clock interface {
	Now() time.Time
}

// Logger writes structured lines. Contract: msg is a fixed string and kv are
// alternating key, value pairs; callers never pass PR text, actor logins or
// tokens.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
}

// Metrics counts events. Contract: name is a fixed metric name and labels a
// small fixed set (for example drop_reason); implementations must tolerate
// concurrent calls.
type Metrics interface {
	Inc(name string, labels map[string]string)
}

// DispositionKind is the recorded outcome of one event (glossary).
type DispositionKind string

const (
	DispositionSteered DispositionKind = "steered"
	DispositionHeld    DispositionKind = "held"
	DispositionDropped DispositionKind = "dropped"
)

// Disposition is one entry of the status ring buffer. It never holds an
// actor login or event text.
type Disposition struct {
	At          time.Time
	Kind        Kind
	Repo        string
	PR          int
	SessionUUID string
	ActorClass  ActorClass // empty for ci
	Disposition DispositionKind
	Reason      string // a HoldReason or DropReason; empty when steered
}

// Recorder keeps recent dispositions for the status surface. Contract:
// Record is cheap, bounded and safe for concurrent use.
type Recorder interface {
	Record(d Disposition)
}

// LeaderGuard decides which Squad process polls (D9). Contract: TryAcquire
// does not block; ok=false means another process leads; the holder keeps the
// lock until release is called at shutdown. Same shape as wakes.LeaderGuard.
type LeaderGuard interface {
	TryAcquire() (release func(), ok bool, err error)
}

// PRSnapshot is the catch-up read of a session's PR state (D13.2, task 3.4).
type PRSnapshot struct {
	CheckConclusion  string // "failure" is the only value that steers
	ChangesRequested int
	Terminal         bool
	// CheckedAt is the last PR status poll; zero means never polled, so an
	// empty CheckConclusion must not be trusted.
	CheckedAt time.Time
}

// PRSnapshotSource reads the PR snapshot. Contract: no network call; returns
// an error for an unknown session.
type PRSnapshotSource interface {
	Snapshot(sessionUUID string) (PRSnapshot, error)
}

// SelfLogin resolves the operator's GitHub login. Contract: a network call;
// returns "" with a nil error when unauthenticated, which callers treat as
// unresolved (fail closed). The caller owns caching and backoff.
type SelfLogin interface {
	Login(ctx context.Context) (string, error)
}

// CoalesceKey identifies one coalescing slot (D12).
type CoalesceKey struct {
	SessionUUID string
	Repo        string
	PR          int
	Class       KindClass
}

// CoalesceAction is the coalescer's verdict for one item.
type CoalesceAction string

const (
	// CoalesceSteer: steer now.
	CoalesceSteer CoalesceAction = "steer"
	// CoalesceHold: leave unacked (hold reason window_batching).
	CoalesceHold CoalesceAction = "hold"
	// CoalesceCoveredSameSHA: ack (covered_same_head_sha).
	CoalesceCoveredSameSHA CoalesceAction = "covered_same_head_sha"
	// CoalesceFoldOverCap: ack (folded_over_cap).
	CoalesceFoldOverCap CoalesceAction = "folded_over_cap"
)

// Coalescer applies the D12 window rules. Contract: Decide has no side
// effects (held is the number of items already held for the key; headSHA is
// "" when unknown); RecordSteer starts the window or records the CI head SHA
// after a delivered steer; Evict forgets all state for a session (call when
// the session ends, the PR is terminal or the subscription is removed);
// NextEligible is read-only and reports the earliest time the class may
// steer, ok=false meaning "now".
type Coalescer interface {
	Decide(key CoalesceKey, headSHA string, held int) CoalesceAction
	RecordSteer(key CoalesceKey, headSHA string)
	Evict(sessionUUID string)
	NextEligible(key CoalesceKey) (at time.Time, ok bool)
}

// StrangerWatch counts human-feedback steers by actor class per repo and
// raises StrangerActivity above the threshold (D11). Contract: memory only;
// OtherCount is the rolling 7-day count of ActorOther steers for the repo.
type StrangerWatch interface {
	RecordSteer(repo string, pr int, class ActorClass)
	OtherCount(repo string) int
}

// InboxStateTracker turns observed inbox/leader state into notifications
// with grace and recovery (D15). Contract: Observe is called every tick with
// the current state (InboxStateOK when healthy); Current returns the last
// observed state.
type InboxStateTracker interface {
	Observe(state InboxState)
	Current() InboxState
}

// FollowupObserver watches whether the agent took a turn after a delivered
// steer (D17). Contract: Track starts a window of Settings.FollowupObserve;
// Tick is called every poll tick and logs prwatch.steer_followup when a
// window ends.
type FollowupObserver interface {
	Track(sessionUUID, repo string, pr int)
	Tick()
}
