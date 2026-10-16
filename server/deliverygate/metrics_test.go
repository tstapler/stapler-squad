package deliverygate

import (
	"strings"
	"testing"
)

func TestCounters_ShouldUseClosedLabelSetAndNoSessionIDLabel_WhenAnyIncrement(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview, visibleSess)
	f := g.PublishFilter()
	for _, typ := range AllNotificationTypes() {
		f(notif("review:abc", typ, nil))
		f(notif("missing-session", typ, nil))
	}
	g.AllowStatusChange("review:abc")
	g.AllowAutoApprovedRow("review:abc", "allow")

	for key := range g.Metrics().Snapshot() {
		parts := strings.Split(key, "|")
		name, labels := parts[0], parts[1:]
		want := len(counterLabelKeys[name])
		if len(labels) != want {
			t.Errorf("%s has %d labels, want %d (%v)", name, len(labels), want, counterLabelKeys[name])
		}
		for _, l := range labels {
			if l == "review:abc" || l == "u-h1" || l == "missing-session" {
				t.Errorf("counter %q carries a session identity label %q", key, l)
			}
		}
	}
	for name, keys := range counterLabelKeys {
		for _, k := range keys {
			if k == "session_id" || k == "session" {
				t.Errorf("%s declares a session label", name)
			}
		}
	}
}

func TestCounterHelper_ShouldMoveOTelInstrumentAndInProcessMirrorTogether_WhenIncremented(t *testing.T) {
	t.Parallel()
	m := NewMetrics()
	m.Add(CounterUnresolved, "routine")
	m.Add(CounterUnresolved, "routine")
	m.Add(CounterIndexMiss)
	if m.Value(CounterUnresolved, "routine") != 2 || m.Value(CounterIndexMiss) != 1 {
		t.Fatalf("mirror = %v", m.Snapshot())
	}
	// The OTel instrument is registered in the same helper (no-op meter without
	// telemetry, so only registration is assertable here).
	if otelInstrument(CounterUnresolved) == nil || otelInstrument(CounterIndexMiss) == nil {
		t.Fatal("OTel instruments not registered by the counter helper")
	}
}
