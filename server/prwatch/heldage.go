package prwatch

import "time"

// HeldEvent is one pending item the loop is holding.
type HeldEvent struct {
	SessionUUID string
	Repo        string
	PR          int
	CreatedAt   time.Time
	Reason      HoldReason
}

// HeldVerdict is the held-age decision for one item.
type HeldVerdict int

const (
	// HeldKeep: leave the item pending.
	HeldKeep HeldVerdict = iota
	// HeldDrop: ack-drop it as held_age_ceiling (notice already raised).
	HeldDrop
)

// HeldAge applies the D13.1 notice and ceiling. Zero thresholds disable the
// matching rule.
type HeldAge struct {
	clock   Clock
	notes   *Notifications
	notice  time.Duration
	ceiling time.Duration
}

// NewHeldAge reads HeldNoticeAfter and HeldAgeCeiling from s.
func NewHeldAge(clock Clock, s Settings, notes *Notifications) *HeldAge {
	return &HeldAge{clock: clock, notes: notes, notice: s.HeldNoticeAfter, ceiling: s.HeldAgeCeiling}
}

// Check raises HeldLong once the item has waited the notice threshold and
// HeldDropped (returning HeldDrop) once it reaches the ceiling. Notifications
// dedupe per session and PR until Delivered.
func (h *HeldAge) Check(it HeldEvent) HeldVerdict {
	age := h.clock.Now().Sub(it.CreatedAt)
	if h.ceiling > 0 && age >= h.ceiling {
		_ = h.notes.HeldDropped(it.SessionUUID, it.Repo, it.PR, it.Reason)
		return HeldDrop
	}
	if h.notice > 0 && age >= h.notice {
		_ = h.notes.HeldLong(it.SessionUUID, it.Repo, it.PR, age, it.Reason)
	}
	return HeldKeep
}

// Delivered clears the held notices for the session and PR after a steer.
func (h *HeldAge) Delivered(session, repo string, pr int) {
	h.notes.SteerDelivered(session, repo, pr)
}

// SessionVerdict is what session liveness allows for one event (D13.2-3).
type SessionVerdict int

const (
	// SessionDeliver: the session is live; proceed to the steer.
	SessionDeliver SessionVerdict = iota
	// SessionHold: leave the item pending (HoldReason says why).
	SessionHold
	// SessionDrop: ack-drop it as session_not_live.
	SessionDrop
)

// SessionGate decides what a session's state allows. changesRequested is the
// review's details.state == changes-requested. A suspended session drops only
// what the on-resume catch-up re-reads (a CI failure, and in mode all a
// changes-requested review); everything else is held. A gone session drops
// everything.
func SessionGate(state SessionState, kind Kind, changesRequested bool, mode HumanFeedbackMode) (SessionVerdict, HoldReason) {
	switch state {
	case StateGone:
		return SessionDrop, ""
	case StateSuspended:
		coveredByCatchup := kind == KindCI ||
			(kind == KindReview && changesRequested && mode == ModeAll)
		if coveredByCatchup {
			return SessionDrop, ""
		}
		return SessionHold, HoldSessionSuspended
	}
	return SessionDeliver, ""
}

// NotTrackedGate decides a not_tracked outcome: transient while the Sessions
// port still lists the session live or suspended (held), session_not_live
// once it lists it gone (D13.3).
func NotTrackedGate(state SessionState) (SessionVerdict, HoldReason) {
	if state == StateGone {
		return SessionDrop, ""
	}
	return SessionHold, HoldNotTracked
}
