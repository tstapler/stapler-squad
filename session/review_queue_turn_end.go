package session

import (
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// awaitingDirectionTTL is how long an untouched awaiting_direction item stays open.
const awaitingDirectionTTL = 24 * time.Hour

// turnEndSignal is the queue decision a recorded TurnEnded maps to.
type turnEndSignal struct {
	reason   AttentionReason
	priority Priority
	ctx      string
}

// signalForTurnEnd maps every TurnEndKind to its signal. The switch carries no default
// so the exhaustive linter fails when a kind is added without a case. ok is false for
// Unknown, which is no signal.
func signalForTurnEnd(te TurnEnded) (sig turnEndSignal, ok bool) {
	switch te.Kind {
	case TurnEndKindQuestion:
		return turnEndSignal{ReasonProseQuestion, PriorityMedium, te.Excerpt}, true
	case TurnEndKindFinished:
		return turnEndSignal{ReasonAwaitingDirection, PriorityLow, te.Excerpt}, true
	case TurnEndKindError:
		dec, _ := reasonForStatus(detection.StatusError, te.Excerpt)
		return turnEndSignal{dec.Reason, dec.Priority, dec.Context}, true
	case TurnEndKindUnknown:
		return turnEndSignal{}, false
	}
	return turnEndSignal{}, false
}

// applyTurnEnded folds the session's recorded TurnEnded into v when the ProseQuestion
// option is on. An outstanding high-priority item (approval, tests, error) always keeps
// precedence; awaiting_direction also replaces the Low task_complete/idle/stale items so a
// session has one item.
func (d *DefaultStatusDeterminer) applyTurnEnded(snap *InstanceSnapshot, opts DeterminerOptions, v *verdict) {
	if !opts.ProseQuestion || snap.TurnEnded.IsZero() {
		return
	}
	te := snap.TurnEnded
	sig, ok := signalForTurnEnd(te)
	if !ok || turnEndOver(snap, te, sig) {
		return
	}
	if v.shouldAdd && v.priority <= PriorityHigh {
		return
	}
	replacesLow := sig.reason == ReasonAwaitingDirection &&
		(v.reason == ReasonTaskComplete || v.reason == ReasonIdle || v.reason == ReasonStale)
	if v.shouldAdd && sig.priority >= v.priority && !replacesLow {
		return
	}
	v.reason, v.priority, v.ctx, v.shouldAdd = sig.reason, sig.priority, sig.ctx, true
	v.episode = te.Episode
}

// turnEndOver reports whether a finished turn is no longer the current state: the agent
// produced new output after it, or the item has sat untouched past the TTL.
func turnEndOver(snap *InstanceSnapshot, te TurnEnded, sig turnEndSignal) bool {
	if sig.reason != ReasonAwaitingDirection {
		return false
	}
	return snap.LastMeaningfulChangeAt.After(te.At) || time.Since(te.At) >= awaitingDirectionTTL
}
