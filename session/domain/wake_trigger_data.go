package domain

import (
	"errors"
	"time"
)

// DefaultWakePendingPerOwnerCap is the fallback for CreateWakeTriggerInput.PendingCap
// (the "Pending triggers per owner session" row of the plan's Wake limits).
const DefaultWakePendingPerOwnerCap = 5

var (
	// ErrWakeCapExceeded: creating the row would push the owner past its live-trigger cap.
	ErrWakeCapExceeded = errors.New("wake trigger cap exceeded")
	// ErrWakeTransitionRejected: the requested transition is structurally
	// forbidden for this row (for example skipping a pending cron row), as
	// opposed to merely losing a race, which reports applied=false.
	ErrWakeTransitionRejected = errors.New("wake trigger transition rejected")
)

// WakeTriggerData is the plain, ent-independent view of a WakeTrigger row.
type WakeTriggerData struct {
	ID                 string
	DedupKey           WakeDedupKey
	OwnerSessionUUID   string
	Type               WakeTriggerType
	Spec               WakeSpec
	Action             WakeAction
	Status             WakeStatus
	EndReason          WakeReason
	ClaimedAt          *time.Time
	DeliveredAt        *time.Time
	DeliveryAttempts   int
	RateLimitDeferrals int
	NextFireAt         *time.Time
	FireCount          int
	MaxFires           int
	Generation         int
	AllowRevive        bool
	ExpiresAt          *time.Time
	CreatedAt          time.Time
	EndedAt            *time.Time
}

// CreateWakeTriggerInput carries the fields needed by CreateWakeTrigger.
type CreateWakeTriggerInput struct {
	OwnerSessionUUID string
	Spec             WakeSpec
	Action           WakeAction
	// NextFireAt is required for time-based specs (at, cron).
	NextFireAt  *time.Time
	MaxFires    int // <= 0 means 1
	Generation  int
	AllowRevive bool
	ExpiresAt   *time.Time
	// PendingCap bounds the owner's live (pending, claimed, needs_revive)
	// rows; <= 0 means DefaultWakePendingPerOwnerCap.
	PendingCap int
}
