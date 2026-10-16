package deliverygate

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
)

func TestGate_ShouldShadowSuppressAndDeliver_WhenFlagOff(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false, hiddenReview)
	filter := g.PublishFilter()
	if !filter(notif("review:abc", tTaskComplete, nil)) {
		t.Fatal("flag off must deliver")
	}
	m := g.Metrics()
	if got := m.Value(CounterWouldSuppress, "bus", tTaskComplete.String(), string(ReasonRoutineForHidden), "review"); got != 1 {
		t.Fatalf("would_suppress = %d, want 1", got)
	}
	if m.Total(CounterSuppressed) != 0 {
		t.Fatal("suppressed counted with flag off")
	}
}

func TestGate_ShouldRejectRoutineAcceptFailureAndCountWouldSuppress_WhenFlagOnThenOff(t *testing.T) {
	t.Parallel()
	g, _, _, flags := newTestGate(true, hiddenReview)
	f := g.PublishFilter()
	if f(notif("review:abc", tTaskComplete, nil)) {
		t.Fatal("flag on: hidden TASK_COMPLETE must be rejected")
	}
	if !f(notif("review:abc", tError, nil)) {
		t.Fatal("flag on: hidden ERROR must be accepted")
	}
	if got := g.Metrics().Value(CounterHiddenDelivered, "bus", "failure", "review"); got != 1 {
		t.Fatalf("hidden_delivered{failure} = %d", got)
	}
	flags.set(false)
	g.Flags().Reload()
	if !f(notif("review:abc", tTaskComplete, nil)) {
		t.Fatal("flag off: both deliver")
	}
	if g.Metrics().Total(CounterWouldSuppress) != 1 {
		t.Fatalf("would_suppress total = %d", g.Metrics().Total(CounterWouldSuppress))
	}
}

func TestGate_ShouldFailOpenCountAndWarnOnce_WhenPublishBeforeSeed(t *testing.T) {
	t.Parallel()
	clk := newFakeClock()
	lg, recs := newRecLogger()
	flags := &staticFlags{}
	flags.set(true)
	g := NewGate(WithClock(clk.Now), WithLogger(lg), WithFlagLoader(flags.load))
	g.Flags().Reload()
	if g.Seeded() {
		t.Fatal("gate seeded at construction")
	}
	if !g.PublishFilter()(notif("review:abc", tTaskComplete, nil)) {
		t.Fatal("unseeded filter must fail open")
	}
	if got := g.Metrics().Value(CounterIndexMiss); got != 1 {
		t.Fatalf("index_miss = %d", got)
	}
	if got := countMsg(recs(), "delivery_unresolved_fail_open"); got != 1 {
		t.Fatalf("WARN count = %d, want 1", got)
	}
	g.Index().Replace([]Entry{hiddenReview})
	if g.PublishFilter()(notif("review:abc", tTaskComplete, nil)) {
		t.Fatal("after seed the hidden routine event must be filtered")
	}
}

func TestFilter_ShouldPassWithoutIndexAccess_WhenEventIsSessionUpdated(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview)
	ev := &events.Event{Type: events.EventSessionUpdated, SessionID: "review:abc"}
	if !g.PublishFilter()(ev) {
		t.Fatal("non-notification event rejected")
	}
	if g.Metrics().Total(CounterIndexMiss) != 0 || len(g.Metrics().Snapshot()) != 0 {
		t.Fatalf("filter touched gate state for a non-notification event: %v", g.Metrics().Snapshot())
	}
}

func TestFilter_ShouldFailOpenWithoutPanic_WhenEventPayloadNilOrUnknownType(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview)
	f := g.PublishFilter()
	if !f(nil) {
		t.Error("nil event must pass")
	}
	if f(notif("review:abc", sessionv1.NotificationType(9999), nil)) {
		t.Error("unknown type is routine: hidden must be suppressed with the flag on")
	}
	if !f(&events.Event{Type: events.EventNotification}) {
		t.Error("empty notification event must pass (NotASession)")
	}
}

func TestFilter_ShouldRecoverFailOpenCountAndLogOnce_WhenIndexLookupPanics(t *testing.T) {
	t.Parallel()
	g, _, recs, _ := newTestGate(true, hiddenReview)
	g.index.state.Store(nil) // Lookup will nil-dereference
	f := g.PublishFilter()
	for i := 0; i < 5; i++ {
		if !f(notif("review:abc", tTaskComplete, nil)) {
			t.Fatal("panic must fail open")
		}
	}
	if got := g.Metrics().Value(CounterFilterPanic); got != 5 {
		t.Fatalf("filter_panic = %d, want 5", got)
	}
	if got := countMsg(recs(), "delivery_gate_filter_panic"); got != 1 {
		t.Fatalf("ERROR logged %d times in the window, want 1", got)
	}
}

func TestGate_ShouldUseIndexOverStaleEventSession_WhenEventCarriesPartialSnapshot(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview)
	ev := notif("u-h1", tTaskComplete, nil)
	ev.Session = nil // an event with no Instance pointer still resolves from the index
	if g.PublishFilter()(ev) {
		t.Fatal("index must decide, not the event payload")
	}
}

func TestGate_ShouldLogHiddenDeliveryAllowedWithChannel_WhenHiddenFailureDelivered(t *testing.T) {
	t.Parallel()
	g, _, recs, _ := newTestGate(true, hiddenReview)
	g.PublishFilter()(notif("review:abc", tError, nil))
	var found bool
	for _, r := range recs() {
		if r.Msg == "hidden_delivery_allowed" {
			found = true
			if r.Attrs["channel"] != "bus" || r.Attrs["class"] != "failure" {
				t.Errorf("attrs = %v", r.Attrs)
			}
		}
	}
	if !found {
		t.Fatal("hidden_delivery_allowed not logged")
	}
}

func TestShadowMode_ShouldLogWouldSuppressAndDeliver_WhenFlagOff(t *testing.T) {
	t.Parallel()
	g, _, recs, _ := newTestGate(false, hiddenReview)
	if !g.PublishFilter()(notif("review:abc", tInfo, nil)) {
		t.Fatal("shadow mode must deliver")
	}
	if countMsg(recs(), "delivery_would_suppress") != 1 {
		t.Fatalf("records: %+v", recs())
	}
}

func TestMatrixUnit_ShouldHonorHintsAndUntrustedStamp_WhenHiddenEventCarriesMetadata(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview)
	f := g.PublishFilter()
	if !f(notif("review:abc", tWarning, map[string]string{events.MetadataKeyDeliveryClass: "failure"})) {
		t.Error("WARNING stamped failure must deliver")
	}
	if f(notif("review:abc", tError, map[string]string{events.MetadataKeyDeliveryClass: "routine"})) {
		t.Error("ERROR stamped routine must be suppressed")
	}
	if !f(notif("review:abc", tTaskComplete, map[string]string{events.MetadataKeyUntrustedType: "true"})) {
		t.Error("untrusted-type event must deliver (fail open)")
	}
	if got := g.Metrics().Value(CounterHiddenDelivered, "bus", "routine", "review"); got != 1 {
		t.Errorf("hidden_delivered{routine} = %d, want 1 (skew must be visible)", got)
	}
}

func TestAllowStatusChange_ShouldDropHiddenOnly_WhenGateOn(t *testing.T) {
	t.Parallel()
	g, _, _, flags := newTestGate(true, hiddenReview, visibleSess)
	if g.AllowStatusChange("review:abc") {
		t.Error("hidden Stopped push must be dropped with the gate on")
	}
	if !g.AllowStatusChange("my-work") {
		t.Error("visible Stopped push must pass")
	}
	if !g.AllowStatusChange("never-seen") {
		t.Error("unresolved must fail open")
	}
	flags.set(false)
	g.Flags().Reload()
	if !g.AllowStatusChange("review:abc") {
		t.Error("gate off must deliver")
	}
	if got := g.Metrics().Value(CounterWouldSuppress, "push_status", "NOTIFICATION_TYPE_STATUS_CHANGE", string(ReasonRoutineForHidden), "review"); got != 1 {
		t.Errorf("shadow push_status = %d", got)
	}
}

func TestAllowAutoApprovedRow_ShouldDropHiddenAllowKeepDeny_WhenGateOn(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview, visibleSess)
	if g.AllowAutoApprovedRow("review:abc", "allow") {
		t.Error("hidden allow row must be dropped")
	}
	if !g.AllowAutoApprovedRow("review:abc", "deny") {
		t.Error("deny rows are the audit trail and are always kept")
	}
	if !g.AllowAutoApprovedRow("my-work", "allow") {
		t.Error("visible allow row must be kept")
	}
}

// T-OB-01: 166 events in 60s -> one line, then suppressed_since_last on the next window.
func TestLimiter_ShouldLogOnceThenCarrySuppressedSinceLast165_When166EventsIn60s(t *testing.T) {
	t.Parallel()
	g, clk, recs, _ := newTestGate(true, hiddenReview)
	f := g.PublishFilter()
	for i := 0; i < 166; i++ {
		f(notif("review:abc", tTaskComplete, nil))
		clk.Advance(100 * time.Millisecond) // 166 * 100ms = 16.6s < 60s
	}
	if got := countMsg(recs(), "delivery_suppressed"); got != 1 {
		t.Fatalf("lines in window = %d, want 1", got)
	}
	clk.Advance(61 * time.Second)
	f(notif("review:abc", tTaskComplete, nil))
	var last logRecord
	for _, r := range recs() {
		if r.Msg == "delivery_suppressed" {
			last = r
		}
	}
	if got := last.Attrs["suppressed_since_last"]; got != uint64(165) {
		t.Fatalf("suppressed_since_last = %v, want 165", got)
	}
}

func TestLimiter_ShouldBoundMapAt4096Keys_When10000DistinctKeys(t *testing.T) {
	t.Parallel()
	clk := newFakeClock()
	lg, _ := newRecLogger()
	l := newRateLimitedLogger(lg, clk.Now)
	for i := 0; i < 10_000; i++ {
		l.log(slog.LevelInfo, "m", limiterKey{session: fmt.Sprint(i)})
	}
	if got := len(l.items); got != logLimiterKeys {
		t.Fatalf("limiter keys = %d, want %d", got, logLimiterKeys)
	}
}

func TestLogFields_ShouldAlwaysCarryChannelSessionTypeReasonAndNoMessageBody_WhenSuppressedOrAllowed(t *testing.T) {
	t.Parallel()
	g, _, recs, _ := newTestGate(true, hiddenReview)
	ev := notif("review:abc", tTaskComplete, nil)
	ev.NotificationMessage = "SECRET BODY"
	ev.NotificationTitle = "SECRET TITLE"
	g.PublishFilter()(ev)
	g.PublishFilter()(notif("review:abc", tError, nil))
	for _, r := range recs() {
		for k, v := range r.Attrs {
			if s, ok := v.(string); ok && (s == "SECRET BODY" || s == "SECRET TITLE") {
				t.Errorf("record %q leaked title/message in %q", r.Msg, k)
			}
		}
		if r.Msg == "delivery_suppressed" {
			for _, k := range []string{"channel", "session_id", "notification_type", "reason"} {
				if _, ok := r.Attrs[k]; !ok {
					t.Errorf("delivery_suppressed missing %q", k)
				}
			}
		}
	}
}

func TestGate_ShouldRecordFilterDurationHistogram_WhenFilterRuns(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview)
	g.PublishFilter()(notif("review:abc", tTaskComplete, nil))
	if _, n := g.Metrics().FilterDurationTotals(); n != 1 {
		t.Fatalf("filter observations = %d, want 1", n)
	}
}

// T-MX-10: a routine event for a session the index cannot resolve is delivered
// (fail open), counted as unresolved{class=routine}, and WARNs once.
func TestGate_ShouldIncrementUnresolvedCounterAndWarn_WhenRoutineEventForUnresolvedSession(t *testing.T) {
	t.Parallel()
	g, _, recs, _ := newTestGate(true, visibleSess)
	f := g.PublishFilter()
	for i := 0; i < 3; i++ {
		if !f(notif("no-such-session", tTaskComplete, nil)) {
			t.Fatal("unresolved routine event must fail open")
		}
	}
	if got := g.Metrics().Value(CounterUnresolved, "routine"); got != 3 {
		t.Errorf("unresolved{routine} = %d, want 3", got)
	}
	if got := countMsg(recs(), "delivery_unresolved_fail_open"); got != 1 {
		t.Errorf("WARN count = %d, want 1 (rate limited per session/type)", got)
	}
}

func TestAllowQueueItem_ShouldGateHiddenRoutineOnly_AndShadowWhenFlagOff(t *testing.T) {
	t.Parallel()
	g, _, _, flags := newTestGate(true, hiddenReview, visibleSess)
	for _, ch := range []Channel{ChannelSlack, ChannelWebhook} {
		if g.AllowQueueItem(ch, "review:abc", nil, int32(tTaskComplete)) {
			t.Errorf("%s: hidden routine must be dropped with the gate on", ch)
		}
		if !g.AllowQueueItem(ch, "review:abc", nil, int32(tError)) {
			t.Errorf("%s: hidden failure must deliver", ch)
		}
		if !g.AllowQueueItem(ch, "review:abc", nil, int32(tApproval)) {
			t.Errorf("%s: hidden needs-human must deliver", ch)
		}
		if !g.AllowQueueItem(ch, "my-work", nil, int32(tTaskComplete)) {
			t.Errorf("%s: visible session must deliver", ch)
		}
		if !g.AllowQueueItem(ch, "never-seen", nil, int32(tTaskComplete)) {
			t.Errorf("%s: unresolved must fail open", ch)
		}
	}
	flags.set(false)
	g.Flags().Reload()
	if !g.AllowQueueItem(ChannelSlack, "review:abc", nil, int32(tTaskComplete)) {
		t.Error("gate off must deliver")
	}
	if got := g.Metrics().Value(CounterWouldSuppress, "slack", "NOTIFICATION_TYPE_TASK_COMPLETE", string(ReasonRoutineForHidden), "review"); got != 1 {
		t.Errorf("shadow slack = %d, want 1", got)
	}
}
