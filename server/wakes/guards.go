package wakes

import "github.com/tstapler/stapler-squad/session/domain"

// VerdictKind is the outcome of the owner-state decision table.
type VerdictKind uint8

const (
	// VerdictWait is the zero value and fails closed: write nothing, keep the
	// row claimed and look again later.
	VerdictWait VerdictKind = iota
	VerdictDeliver
	VerdictSkip
	VerdictNeedsRevive
)

// Verdict is what to do with a claimed steer_text wake. Reason is set for
// VerdictSkip.
type Verdict struct {
	Kind   VerdictKind
	Reason domain.WakeReason
}

// Decide is the delivery decision table of ADR-006 rule 2 for action_kind
// steer_text, the single place the archived, one-shot, terminal-item, deleted,
// stopped and revive rules live. A notify_operator wake never reaches it: it
// writes nothing into the owner, so no guard applies.
func Decide(allowRevive bool, owner OwnerFacts) Verdict {
	switch {
	case !owner.Exists:
		return Verdict{Kind: VerdictSkip, Reason: domain.WakeReasonOwnerMissing}
	case owner.Archived:
		return Verdict{Kind: VerdictSkip, Reason: domain.WakeReasonArchived}
	case owner.OneShot:
		return Verdict{Kind: VerdictSkip, Reason: domain.WakeReasonOneShot}
	case owner.ItemTerminal:
		return Verdict{Kind: VerdictSkip, Reason: domain.WakeReasonItemTerminal}
	case owner.Live:
		return Verdict{Kind: VerdictDeliver}
	case owner.Starting:
		// Same liveness notion as PeerFaultOf: a Creating/Restoring owner is not stopped, so wait.
		return Verdict{Kind: VerdictWait}
	case allowRevive:
		return Verdict{Kind: VerdictNeedsRevive}
	default:
		// Nothing can ever re-admit it, so needs_revive would sit forever.
		return Verdict{Kind: VerdictSkip, Reason: domain.WakeReasonOwnerStopped}
	}
}
