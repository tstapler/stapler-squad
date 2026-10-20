package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

type canaryProbe struct {
	canary   *DialogMatchCanary
	clock    *manualClock
	notified int
	counted  map[string]int
}

func newCanaryProbe() *canaryProbe {
	p := &canaryProbe{clock: newManualClock(), counted: map[string]int{}}
	p.canary = NewDialogMatchCanary(p.clock.Now, func(r DialogMatchResult, v string) { p.counted[string(r)+"/"+v]++ }, func() { p.notified++ })
	return p
}

func (p *canaryProbe) record(matches ...bool) {
	for _, m := range matches {
		p.canary.Record(DialogMatchResultOf(m))
		p.clock.Advance(time.Minute)
	}
}

func TestCanary_should_TripOnce_WhenMoreThanHalfOfSixAttemptsMismatch(t *testing.T) {
	t.Parallel()
	p := newCanaryProbe()

	p.record(true, false, false, true, false, false) // 4 of 6 mismatch

	assert.True(t, p.canary.Tripped())
	assert.Equal(t, 1, p.notified, "exactly one operator notification")
	p.record(false, false)
	assert.Equal(t, 1, p.notified, "no repeat while it stays tripped")
}

func TestCanary_should_NotTrip_BelowFiveAttemptsOrAtExactlyHalf(t *testing.T) {
	t.Parallel()
	few := newCanaryProbe()
	few.record(false, false, false, false)
	assert.False(t, few.canary.Tripped(), "four attempts is below the minimum")

	half := newCanaryProbe()
	half.record(false, true, false, true, false, true)
	assert.False(t, half.canary.Tripped(), "50% is not more than 50%")
}

func TestCanary_should_Recover_WhenTheShareDropsBelowHalfOverTheNextFiveAttempts(t *testing.T) {
	t.Parallel()
	p := newCanaryProbe()
	p.record(true, false, false, true, false, false)
	require.True(t, p.canary.Tripped())

	p.record(true, true, true, true, true) // 4 mismatches of 11

	assert.False(t, p.canary.Tripped())
}

func TestCanary_should_ForgetAttemptsOlderThanAnHour(t *testing.T) {
	t.Parallel()
	p := newCanaryProbe()
	p.record(false, false, false, false)
	p.clock.Advance(2 * time.Hour)
	p.record(false)
	assert.False(t, p.canary.Tripped(), "the four old mismatches left the window")
}

func TestCanary_should_CountEveryOutcomeByResultAndVersion(t *testing.T) {
	t.Parallel()
	p := newCanaryProbe()
	p.canary.SetVersionSource(func() string { return "2.1.296" })
	p.record(true, false, false)
	assert.Equal(t, map[string]int{"match/2.1.296": 1, "mismatch/2.1.296": 2}, p.counted)

	var nilCanary *DialogMatchCanary
	assert.NotPanics(t, func() { nilCanary.Record(DialogMatched) })
	assert.False(t, nilCanary.Tripped())
}

func TestBuildDescriptors_ShouldDegradeHookQuestionToReadOnly_WhenCanaryTripped_ExceptWithHookReply(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	key := QuestionKey{SessionUUID: "s1", QuestionID: "q1"}
	f.questions.live[key] = PendingQuestion{QuestionID: "q1", SessionUUID: "s1"}
	canary := newCanaryProbe()
	canary.record(false, false, false, false, false)
	require.True(t, canary.canary.Tripped())
	annotate := func(hookReply bool) *sessionv1.NotificationActionDescriptor {
		b := NewDescriptorBuilder(DescriptorSources{Questions: f.questions, Canary: canary.canary})
		b.flag = func(string) bool { return true }
		b.hookReplyOn = func() bool { return hookReply }
		rec := questionRecord("n1", "s1", "q1")
		b.Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)
		return rec.Actions[0]
	}

	degraded := annotate(false)
	assert.False(t, degraded.Available)
	assert.Equal(t, "read_only", degraded.UnavailableReason)

	assert.True(t, annotate(true).Available, "the hook_reply route is not keystroke-based and keeps answering")

	canary.record(true, true, true, true, true, true, true, true, true, true, true)
	require.False(t, canary.canary.Tripped())
	assert.True(t, annotate(false).Available, "the descriptor returns once the share recovers")
}
