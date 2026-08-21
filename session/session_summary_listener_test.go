package session

import (
	"context"
	"testing"
	"time"
)

// fakeSummaryGenerator is a test double satisfying the summaryGenerator interface.
// Requires no ent client, no headless.Pool, and no other SessionSummaryGenerator
// dependency — the listener only ever depends on the interface.
type fakeSummaryGenerator struct {
	calls chan string // sessionUUID of each call
}

func newFakeSummaryGenerator() *fakeSummaryGenerator {
	return &fakeSummaryGenerator{calls: make(chan string, 4)}
}

func (f *fakeSummaryGenerator) GenerateAndPersist(_ context.Context, sessionUUID, _ string, _ time.Time, _ DiffSnapshot, _ string, _ *SessionGoalData, _ string) {
	f.calls <- sessionUUID
}

func TestOnLifecycleEvent_should_DispatchGenerateAndPersist_When_EventExitedOrEventStoppedFireWithNormalReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		event LifecycleEvent
	}{
		{"EventExited", EventExited},
		{"EventStopped", EventStopped},
	}
	for _, tc := range tests {
		event := tc.event
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gen := newFakeSummaryGenerator()
			inst := &Instance{Title: "test-session", UUID: "sess-123", CreatedAt: time.Now()}
			l := &sessionSummaryListener{generator: gen, instance: inst}

			l.OnLifecycleEvent(event, "pty-eof")

			select {
			case uuid := <-gen.calls:
				if uuid != "sess-123" {
					t.Fatalf("expected dispatch for sess-123, got %q", uuid)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for GenerateAndPersist dispatch")
			}
		})
	}
}

func TestOnLifecycleEvent_should_NotDispatch_When_ReasonIsReconcileSessionMissing(t *testing.T) {
	t.Parallel()
	gen := newFakeSummaryGenerator()
	inst := &Instance{Title: "test-session", UUID: "sess-456", CreatedAt: time.Now()}
	l := &sessionSummaryListener{generator: gen, instance: inst}

	l.OnLifecycleEvent(EventExited, reasonReconcileSessionMissing)

	select {
	case uuid := <-gen.calls:
		t.Fatalf("expected no dispatch, got call for %q", uuid)
	case <-time.After(200 * time.Millisecond):
		// expected: no dispatch
	}
}

func TestOnLifecycleEvent_should_NotDispatch_When_EventStarted(t *testing.T) {
	t.Parallel()
	gen := newFakeSummaryGenerator()
	inst := &Instance{Title: "test-session", UUID: "sess-789", CreatedAt: time.Now()}
	l := &sessionSummaryListener{generator: gen, instance: inst}

	l.OnLifecycleEvent(EventStarted, "")

	select {
	case uuid := <-gen.calls:
		t.Fatalf("expected no dispatch, got call for %q", uuid)
	case <-time.After(200 * time.Millisecond):
		// expected: no dispatch
	}
}
