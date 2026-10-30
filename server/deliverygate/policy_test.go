package deliverygate

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
)

func TestShouldDeliver_ShouldSuppressRoutine_WhenSessionHidden(t *testing.T) {
	t.Parallel()
	got := ShouldDeliver(VisibilityHidden, Facts{Type: tTaskComplete})
	want := Decision{Outcome: OutcomeSuppress, Class: ClassRoutine, Reason: ReasonRoutineForHidden}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestShouldDeliver_ShouldDeliverFailureAndNeedsHuman_WhenHiddenErrorOrApprovalNeeded(t *testing.T) {
	t.Parallel()
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tError}); d.Outcome != OutcomeDeliver || d.Class != ClassFailure {
		t.Errorf("ERROR: %+v", d)
	}
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tApproval}); d.Outcome != OutcomeDeliver || d.Class != ClassNeedsHuman {
		t.Errorf("APPROVAL_NEEDED: %+v", d)
	}
}

func TestShouldDeliver_ShouldHonorHints_WhenWarningHintFailureOrErrorHintRoutine(t *testing.T) {
	t.Parallel()
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tWarning}); d.Outcome != OutcomeSuppress {
		t.Errorf("hidden WARNING no hint: %+v", d)
	}
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tWarning, Hint: HintFailure}); d.Outcome != OutcomeDeliver || d.Class != ClassFailure {
		t.Errorf("hidden WARNING+failure: %+v", d)
	}
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tError, Hint: HintRoutine}); d.Outcome != OutcomeSuppress {
		t.Errorf("hidden ERROR+routine: %+v", d)
	}
	if d := ShouldDeliver(VisibilityVisible, Facts{Type: tError, Hint: HintRoutine}); d.Outcome != OutcomeDeliver {
		t.Errorf("visible ERROR+routine must deliver: %+v", d)
	}
	// A routine stamp must never be able to hide a needs-human event.
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: tApproval, Hint: HintRoutine}); d.Outcome != OutcomeDeliver || d.Class != ClassNeedsHuman {
		t.Errorf("hidden APPROVAL_NEEDED+routine: %+v", d)
	}
}

func TestShouldDeliver_ShouldDeliverAsFailure_WhenHiddenCrashFailureOrPermanentlyFailedError(t *testing.T) {
	t.Parallel()
	for _, typ := range []sessionv1.NotificationType{tFailure, tError} {
		d := ShouldDeliver(VisibilityHidden, Facts{Type: typ})
		if d.Outcome != OutcomeDeliver || d.Class != ClassFailure {
			t.Errorf("%s: %+v", typ, d)
		}
	}
}

func TestShouldDeliver_ShouldFailOpenRoutine_WhenUntrustedLegacyTypeHint(t *testing.T) {
	t.Parallel()
	d := ShouldDeliver(VisibilityHidden, Facts{Type: tTaskComplete, Hint: HintUntrustedType})
	want := Decision{Outcome: OutcomeDeliver, Class: ClassRoutine, Reason: ReasonUntrustedLegacyType}
	if d != want {
		t.Fatalf("got %+v want %+v", d, want)
	}
	v := ShouldDeliver(VisibilityVisible, Facts{Type: tTaskComplete, Hint: HintUntrustedType})
	if v.Outcome != OutcomeDeliver || v.Reason != ReasonVisibleSession {
		t.Fatalf("visible: %+v", v)
	}
}

func TestShouldDeliver_ShouldDeliver_WhenVisibleNotASessionOrUnresolved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v      Visibility
		t      sessionv1.NotificationType
		reason Reason
	}{
		{VisibilityVisible, sessionv1.NotificationType_NOTIFICATION_TYPE_AUTO_APPROVED, ReasonVisibleSession},
		{VisibilityUnresolved, tTaskComplete, ReasonUnresolvedFailOpen},
		{VisibilityNotASession, tTaskComplete, ReasonNotASession},
	}
	for _, c := range cases {
		d := ShouldDeliver(c.v, Facts{Type: c.t})
		if d.Outcome != OutcomeDeliver || d.Reason != c.reason {
			t.Errorf("%s %s: %+v", c.v, c.t, d)
		}
	}
}

// Every proto enum value must have an explicit class; adding a value without
// updating typeClass fails here naming the value.
func TestShouldDeliver_ShouldClassifyEveryNotificationType_WhenRangingOverProtoEnum(t *testing.T) {
	t.Parallel()
	for v, name := range sessionv1.NotificationType_name {
		if _, ok := typeClass[sessionv1.NotificationType(v)]; !ok {
			t.Errorf("NotificationType %s (%d) has no explicit delivery class in typeClass", name, v)
		}
	}
	for typ := range typeClass {
		if _, ok := sessionv1.NotificationType_name[int32(typ)]; !ok {
			t.Errorf("typeClass has %d which is not a proto NotificationType", typ)
		}
	}
}

func TestParseClassHint_ShouldReturnHintNoneAndLog_WhenValueUnknown(t *testing.T) {
	t.Parallel()
	h, ok := ParseClassHint(map[string]string{events.MetadataKeyDeliveryClass: "bogus"})
	if h != HintNone || ok {
		t.Fatalf("unknown value: hint=%v ok=%v", h, ok)
	}
	cases := []struct {
		md   map[string]string
		want ClassHint
	}{
		{nil, HintNone},
		{map[string]string{events.MetadataKeyDeliveryClass: "failure"}, HintFailure},
		{map[string]string{events.MetadataKeyDeliveryClass: "routine"}, HintRoutine},
		{map[string]string{events.MetadataKeyUntrustedType: "true", events.MetadataKeyDeliveryClass: "routine"}, HintUntrustedType},
	}
	for _, c := range cases {
		if got, ok := ParseClassHint(c.md); got != c.want || !ok {
			t.Errorf("%v: got %v ok=%v want %v", c.md, got, ok, c.want)
		}
	}
}

func TestDecision_ShouldUseClosedReasonSet_WhenAnyOutcomeProduced(t *testing.T) {
	t.Parallel()
	closed := map[Reason]bool{
		ReasonRoutineForHidden: true, ReasonFailureForHidden: true, ReasonNeedsHumanForHidden: true,
		ReasonHintPromotedFailure: true, ReasonUntrustedLegacyType: true, ReasonVisibleSession: true,
		ReasonNotASession: true, ReasonUnresolvedFailOpen: true,
	}
	hints := []ClassHint{HintNone, HintFailure, HintRoutine, HintUntrustedType}
	vis := []Visibility{VisibilityUnresolved, VisibilityVisible, VisibilityHidden, VisibilityNotASession}
	for _, typ := range AllNotificationTypes() {
		for _, h := range hints {
			for _, v := range vis {
				d := ShouldDeliver(v, Facts{Type: typ, Hint: h})
				if !closed[d.Reason] {
					t.Fatalf("reason %q outside the closed set for %s/%v/%s", d.Reason, v, h, typ)
				}
				if d.Outcome == OutcomeShadowSuppress {
					t.Fatalf("ShouldDeliver must never return ShadowSuppress")
				}
			}
		}
	}
}

// newEmissions lists every notification emission added by the
// notification-actions-and-agent-messaging project (plan Task 2.2c). A producer
// that adds an emission adds its row here; a row the gate would not deliver for
// a hidden session fails the test.
var newEmissions = []struct {
	name       string
	facts      Facts
	visibility Visibility
	wantClass  DeliveryClass
}{
	{"INPUT_REQUIRED question", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED}, VisibilityHidden, ClassNeedsHuman},
	{"CONFIRMATION_NEEDED backlog decision", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_CONFIRMATION_NEEDED}, VisibilityHidden, ClassNeedsHuman},
	{"peer-help escalation as INPUT_REQUIRED", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED}, VisibilityHidden, ClassNeedsHuman},
	{"wake missed as failure class", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING, Hint: HintFailure}, VisibilityHidden, ClassFailure},
	{"wake skipped as failure class", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING, Hint: HintFailure}, VisibilityHidden, ClassFailure},
	{"wake rate-limited as failure class", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING, Hint: HintFailure}, VisibilityHidden, ClassFailure},
	{"notify_operator reminder (no session binding)", Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_INFO}, VisibilityNotASession, ClassRoutine},
}

func TestShouldDeliver_ShouldDeliverEveryNewEmission_WhenSessionHiddenOrNotASession(t *testing.T) {
	t.Parallel()
	for _, e := range newEmissions {
		d := ShouldDeliver(e.visibility, e.facts)
		if d.Outcome != OutcomeDeliver {
			t.Errorf("%s: outcome %v, want deliver (%+v)", e.name, d.Outcome, d)
		}
		if d.Class != e.wantClass {
			t.Errorf("%s: class %v, want %v", e.name, d.Class, e.wantClass)
		}
	}
}

// The attention reasons ride existing NotificationTypes: no new proto value, class by type.
func TestShouldDeliver_ShouldMapAttentionReasonTypes_WhenProseQuestionAndAwaitingDirection(t *testing.T) {
	t.Parallel()
	if got := ClassOf(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED); got != ClassNeedsHuman {
		t.Errorf("prose_question (INPUT_REQUIRED) class = %v", got)
	}
	if got := ClassOf(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE); got != ClassRoutine {
		t.Errorf("awaiting_direction (TASK_COMPLETE) class = %v", got)
	}
	meta := map[string]string{events.MetadataKeyAttentionReason: events.AttentionReasonAwaitingDirection}
	hint, ok := ParseClassHint(meta)
	if !ok || hint != HintNone {
		t.Fatalf("attention_reason must not change the class hint, got %v ok=%v", hint, ok)
	}
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE, Hint: hint}); d.Outcome != OutcomeSuppress {
		t.Errorf("hidden awaiting_direction: %+v", d)
	}
	if d := ShouldDeliver(VisibilityHidden, Facts{Type: sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED}); d.Outcome != OutcomeDeliver {
		t.Errorf("hidden prose_question: %+v", d)
	}
}
