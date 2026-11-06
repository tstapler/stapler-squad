// Package deliverygate is the single hidden-session delivery gate (ADR-001).
//
// ShouldDeliver is the one pure policy: a hidden session notifies only for
// failures and needs-human events. Gate wraps it as the EventBus publish filter
// and as the consumer-side gates for the two paths the bus does not reach
// (push status-change, auto-approved history rows).
//
// Lock-order rule: nothing on the publish path takes a lock. The visibility
// index is a copy-on-write atomic pointer and the flag cache an atomic snapshot;
// writer mutexes guard copy-and-swap only and call nothing while held, so
// Upsert is safe under Instance.mu and Publish is safe under any caller lock.
package deliverygate

import (
	"slices"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
)

// Visibility is what the index knows about the session an event is about.
// Only VisibilityHidden is ever gated; the zero value fails open.
type Visibility int

const (
	VisibilityUnresolved  Visibility = iota // not found: delivered and counted
	VisibilityVisible                       // known, not hidden
	VisibilityHidden                        // known, hidden: policy applies
	VisibilityNotASession                   // system or item-level notification
)

func (v Visibility) String() string {
	switch v {
	case VisibilityVisible:
		return "visible"
	case VisibilityHidden:
		return "hidden"
	case VisibilityNotASession:
		return "not_a_session"
	default:
		return "unresolved"
	}
}

// DeliveryClass is the policy class of a notification type.
type DeliveryClass int

const (
	ClassRoutine DeliveryClass = iota
	ClassNeedsHuman
	ClassFailure
)

func (c DeliveryClass) String() string {
	switch c {
	case ClassNeedsHuman:
		return "needs_human"
	case ClassFailure:
		return "failure"
	default:
		return "routine"
	}
}

// Outcome is what the gate does with an event. OutcomeShadowSuppress is the
// Gate's flag-off form of OutcomeSuppress (ShouldDeliver never returns it).
type Outcome int

const (
	OutcomeDeliver Outcome = iota
	OutcomeSuppress
	OutcomeShadowSuppress
)

func (o Outcome) String() string {
	switch o {
	case OutcomeSuppress:
		return "suppress"
	case OutcomeShadowSuppress:
		return "shadow_suppress"
	default:
		return "deliver"
	}
}

// Reason is a closed set of decision reasons (log and counter label values).
type Reason string

const (
	ReasonRoutineForHidden    Reason = "routine_for_hidden"
	ReasonFailureForHidden    Reason = "failure_for_hidden"
	ReasonNeedsHumanForHidden Reason = "needs_human_for_hidden"
	ReasonHintPromotedFailure Reason = "hint_promoted_failure"
	ReasonUntrustedLegacyType Reason = "untrusted_legacy_type"
	ReasonVisibleSession      Reason = "visible_session"
	ReasonNotASession         Reason = "not_a_session"
	ReasonUnresolvedFailOpen  Reason = "unresolved_fail_open"
)

// ClassHint is the producer stamp parsed once at the filter boundary.
type ClassHint int

const (
	HintNone ClassHint = iota
	// HintFailure promotes a hard-stop routine-typed event (e.g. a WARNING) for hidden sessions.
	HintFailure
	// HintRoutine demotes a failure-class event (per-tool hook ERROR) for hidden sessions.
	// It never demotes needs-human: a stamp must not be able to hide an approval.
	HintRoutine
	// HintUntrustedType is stamped server-side on a request from an unversioned
	// ssq-notify: its type numbers may collide with the proto enum, so a hidden
	// session's event delivers (fail open) instead of being trusted as routine.
	HintUntrustedType
)

// Facts is the only policy input besides Visibility.
type Facts struct {
	Type sessionv1.NotificationType
	Hint ClassHint
}

// Decision is the policy result.
type Decision struct {
	Outcome Outcome
	Class   DeliveryClass
	Reason  Reason
}

// typeClass is the explicit class of every NotificationType. The exhaustiveness
// test fails when a proto enum value is missing here.
var typeClass = map[sessionv1.NotificationType]DeliveryClass{
	sessionv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED:         ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED:     ClassNeedsHuman,
	sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED:      ClassNeedsHuman,
	sessionv1.NotificationType_NOTIFICATION_TYPE_CONFIRMATION_NEEDED: ClassNeedsHuman,
	sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE:       ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_PROCESS_STARTED:     ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_PROCESS_FINISHED:    ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR:               ClassFailure,
	sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING:             ClassRoutine, // advisory unless stamped HintFailure
	sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE:             ClassFailure,
	sessionv1.NotificationType_NOTIFICATION_TYPE_INFO:                ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_DEBUG:               ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE:       ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_AUTO_APPROVED:       ClassRoutine,
	sessionv1.NotificationType_NOTIFICATION_TYPE_CUSTOM:              ClassRoutine,
}

// ClassOf returns the type-derived class (unknown values are routine).
func ClassOf(t sessionv1.NotificationType) DeliveryClass {
	return typeClass[t]
}

// ShouldDeliver is the single delivery policy. It is pure: no I/O, no clock,
// no flag. Priority and message text are deliberately not inputs (ADR-002).
func ShouldDeliver(v Visibility, f Facts) Decision {
	class := ClassOf(f.Type)
	switch v {
	case VisibilityVisible:
		return Decision{OutcomeDeliver, class, ReasonVisibleSession}
	case VisibilityNotASession:
		return Decision{OutcomeDeliver, class, ReasonNotASession}
	case VisibilityHidden:
		// handled below
	default:
		return Decision{OutcomeDeliver, class, ReasonUnresolvedFailOpen}
	}

	if f.Hint == HintUntrustedType {
		// The class stays type-derived so hidden_delivered{class=routine} exposes the skew.
		return Decision{OutcomeDeliver, class, ReasonUntrustedLegacyType}
	}
	promoted := false
	switch {
	case f.Hint == HintFailure && class == ClassRoutine:
		class, promoted = ClassFailure, true
	case f.Hint == HintRoutine && class == ClassFailure:
		class = ClassRoutine
	}
	switch class {
	case ClassRoutine:
		return Decision{OutcomeSuppress, class, ReasonRoutineForHidden}
	case ClassNeedsHuman:
		return Decision{OutcomeDeliver, class, ReasonNeedsHumanForHidden}
	default:
		if promoted {
			return Decision{OutcomeDeliver, class, ReasonHintPromotedFailure}
		}
		return Decision{OutcomeDeliver, class, ReasonFailureForHidden}
	}
}

// ParseClassHint reads the producer stamps from event metadata. The untrusted
// stamp is written only by SendNotification (which strips any client value), so
// it is a separate key from the producer-supplied delivery_class.
func ParseClassHint(metadata map[string]string) (ClassHint, bool) {
	if metadata == nil {
		return HintNone, true
	}
	if metadata[events.MetadataKeyUntrustedType] == "true" {
		return HintUntrustedType, true
	}
	switch metadata[events.MetadataKeyDeliveryClass] {
	case "":
		return HintNone, true
	case events.DeliveryClassFailure:
		return HintFailure, true
	case events.DeliveryClassRoutine:
		return HintRoutine, true
	default:
		return HintNone, false
	}
}

// AllNotificationTypes lists the proto enum values in ascending order (tests, matrix).
func AllNotificationTypes() []sessionv1.NotificationType {
	out := make([]sessionv1.NotificationType, 0, len(sessionv1.NotificationType_name))
	for v := range sessionv1.NotificationType_name {
		out = append(out, sessionv1.NotificationType(v))
	}
	slices.Sort(out)
	return out
}
