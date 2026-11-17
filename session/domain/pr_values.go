package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidPROwner means an owner did not name exactly one of a session and an item.
	ErrInvalidPROwner = errors.New("session: PR owner must set exactly one of session uuid and backlog item id")
)

// PROwner names who a link or tombstone belongs to: exactly one field is set.
type PROwner struct {
	SessionUUID   string
	BacklogItemID string
}

// SessionPROwner returns the owner for a session link.
func SessionPROwner(sessionUUID string) PROwner { return PROwner{SessionUUID: sessionUUID} }

// ItemPROwner returns the owner for a backlog-item-direct link.
func ItemPROwner(itemID string) PROwner { return PROwner{BacklogItemID: itemID} }

func (o PROwner) Validate() error {
	if (o.SessionUUID == "") == (o.BacklogItemID == "") {
		return ErrInvalidPROwner
	}
	return nil
}

func (o PROwner) IsSession() bool { return o.SessionUUID != "" }

// PRObservationSource says where an observation came from.
type PRObservationSource string

const (
	PRSourceTick              PRObservationSource = "tick"
	PRSourceReconcilerRefresh PRObservationSource = "reconciler_refresh"
	PRSourceWebhook           PRObservationSource = "webhook"
	PRSourceWebhookReopened   PRObservationSource = "webhook_reopened"
)

// PRObservationKind is what an observation reports.
type PRObservationKind string

const (
	PRObservationFetch          PRObservationKind = "fetch"           // a successful fetch of the PR
	PRObservationCountedFailure PRObservationKind = "counted_failure" // a failure that counts toward exhaustion
	PRObservationErrorNote      PRObservationKind = "error_note"      // a non-counted failure; writes poll_error only
)

// PRObservation is one observation of a PR. ObservedAt is the injected clock at
// fetch START; Now is the injected clock at apply time (zero means time.Now()).
type PRObservation struct {
	Source     PRObservationSource
	Kind       PRObservationKind
	ObservedAt time.Time
	Now        time.Time

	// Fetch payload (Kind == PRObservationFetch). State must be open, closed or merged.
	State                 PRState
	IsDraft               bool
	CheckConclusion       string
	ApprovedCount         int
	ChangesRequestedCount int
	URL, HeadBranch       string
	Title                 string
	BaseBranch            string
	OpenedAt              time.Time
	MergedAt, ClosedAt    time.Time
	RateLimitResetAt      time.Time

	// PollError is the poll_error enum for the two failure kinds.
	PollError string
}

// PRDropReason says why an observation was not applied.
type PRDropReason string

const (
	PRDropStale  PRDropReason = "stale"
	PRDropSticky PRDropReason = "sticky"
	PRDropSource PRDropReason = "source"
)

// PRObservationResult reports whether UpdatePullRequestState applied the observation.
type PRObservationResult struct {
	Applied bool
	Dropped PRDropReason
}

// PRObservationDrops counts dropped observations by reason.
type PRObservationDrops struct{ Stale, Sticky, Source int64 }

// ItemGuardView is what the retention sweeper and the cleanup gate need to know
// about one backlog item's own PRs.
type ItemGuardView struct {
	// Direct are the item-direct links (agent-reported, Jules, manual) with the
	// item's tombstoned PRs removed.
	Direct []PRView
	// LegacyUnrepresented is true when the item's scalar pr_number/pr_url is
	// neither carried by a link of the item or its sessions nor covered by a
	// tombstone, or cannot be turned into a ref at all. It fails closed.
	LegacyUnrepresented bool
}
