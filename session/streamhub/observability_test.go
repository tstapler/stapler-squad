package streamhub_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// Story 3.2.2: structured logging & metrics for hub lifecycle events.

// syncLogBuffer is a mutex-guarded bytes.Buffer. slog.Default() is
// process-global, so a hub created by an *earlier* test with a short
// WithTeardownGrace (e.g. TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches)
// can still fire its teardown timer and log through whatever the current
// default logger happens to be at that moment — including a later test's
// captureLogs buffer, concurrently with that later test's own String() read.
// A plain *bytes.Buffer makes that a genuine data race (caught under
// -race); wrapping every access in a mutex doesn't remove the cross-test log
// interleaving (harmless — logMessages only checks that the wanted messages
// appear in order, not that no other messages appear), it just makes the
// concurrent access itself safe.
type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs redirects slog's default logger to a JSON-lines buffer for the
// duration of the test, restoring the previous default on cleanup.
// log.Info/Warn/Error (github.com/tstapler/stapler-squad/log) delegate
// straight to slog's package-level default logger, so this is the seam
// available to assert on structured log output without a bespoke logging
// abstraction.
func captureLogs(t *testing.T) *syncLogBuffer {
	t.Helper()
	buf := &syncLogBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// logMessages parses buf's JSON lines and returns each line's "msg" field,
// in order — enough to assert event ordering without depending on exact
// field formatting.
func logMessages(t *testing.T, buf *syncLogBuffer) []string {
	t.Helper()
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("failed to parse log line %q: %v", line, err)
		}
		msg, _ := rec["msg"].(string)
		msgs = append(msgs, msg)
	}
	return msgs
}

// TestStreamHub_should_LogHubCreatedAttachResizeAndDetach_InOrder is Story
// 3.2.2's core AC: hub creation, subscriber attach, resize negotiation, and
// subscriber detach each produce one structured slog line, in that relative
// order, so an operator can reconstruct "what did the hub do and who was
// attached" after the fact from logs alone.
func TestStreamHub_should_LogHubCreatedAttachResizeAndDetach_InOrder(t *testing.T) {
	buf := captureLogs(t)

	const sessionName = "observed-session"
	hub := streamhub.NewStreamHub(sessionName, nil, streamhub.WithTeardownGrace(time.Hour))

	mt := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(mt, streamhub.SubscriberCapability{CanResize: true})

	size := mustSize(t, 80, 24)
	hub.RequestResize(id, size)

	hub.DetachSubscriber(id)

	msgs := logMessages(t, buf)
	wantInOrder := []string{
		"streamhub hub created",
		"streamhub subscriber attached",
		"streamhub resize negotiated",
		"streamhub subscriber detached",
	}
	idx := 0
	for _, m := range msgs {
		if idx < len(wantInOrder) && m == wantInOrder[idx] {
			idx++
		}
	}
	if idx != len(wantInOrder) {
		t.Fatalf("expected log lines %v to appear in order, got messages: %v", wantInOrder, msgs)
	}
}

// TestStreamHub_should_LogBatchFlushEvent_When_RawOutputFlushes is Story
// 3.2.2's batch-flush AC: a BatchWindow flush (driven here via OnRawOutput)
// emits one structured log line naming frames coalesced, byte count, and
// flush trigger reason.
func TestStreamHub_should_LogBatchFlushEvent_When_RawOutputFlushes(t *testing.T) {
	buf := captureLogs(t)

	hub := streamhub.NewStreamHub("batch-flush-log-test", nil,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithBatchMaxWindow(5*time.Millisecond))
	defer func() { _ = hub.ForceTeardown() }()
	mt := streamhub.NewMemoryTransport()
	hub.AttachSubscriber(mt, streamhub.SubscriberCapability{})

	hub.OnRawOutput([]byte("hello"))
	hub.OnRawOutput([]byte("world"))

	if !waitFor(t, time.Second, func() bool { return len(mt.ReceivedFrames()) == 1 }) {
		t.Fatalf("expected exactly one flushed frame, got %d", len(mt.ReceivedFrames()))
	}

	found := false
	for _, m := range logMessages(t, buf) {
		if m == "streamhub batch flushed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a %q log line in captured output", "streamhub batch flushed")
	}
}

// TestRegisterMetrics_NoError is a smoke check that instrument registration
// against the OTel global (no-op, in tests) meter succeeds — mirrors
// session/unfinished/metrics_test.go's identical pattern/bar for this kind
// of test in this repo.
func TestRegisterMetrics_NoError(t *testing.T) {
	if err := streamhub.RegisterMetrics(); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

// TestActiveHubs_should_IncrementOnCreateAndDecrementOnTeardown backs the
// streamhub_active_hubs gauge: NewStreamHub increments the process-wide
// count and ForceTeardown decrements it. Asserted as a delta rather than an
// absolute value since other tests in this package create hubs that are
// never torn down (long teardown grace, no explicit ForceTeardown call) and
// share this process-wide counter.
func TestActiveHubs_should_IncrementOnCreateAndDecrementOnTeardown(t *testing.T) {
	before := streamhub.ActiveHubs()

	hub := streamhub.NewStreamHub("active-hubs-gauge-test", nil, streamhub.WithTeardownGrace(time.Hour))
	if got := streamhub.ActiveHubs(); got != before+1 {
		t.Fatalf("expected ActiveHubs() to increment by 1 after NewStreamHub, got %d -> %d", before, got)
	}

	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("ForceTeardown: %v", err)
	}
	if got := streamhub.ActiveHubs(); got != before {
		t.Fatalf("expected ActiveHubs() to return to %d after ForceTeardown, got %d", before, got)
	}
}

// TestStreamHubMetrics_should_RecordSubscribersPerHub_When_SubscribersAttach
// backs the streamhub_subscribers_per_hub histogram: each AttachSubscriber
// call records the post-attach subscriber count, and SubscribersPerHub()
// exposes the most recently recorded value — the same testable-accessor bar
// TestActiveHubs_/TestOverlapInvariant_ above already set for their own
// metrics.
func TestStreamHubMetrics_should_RecordSubscribersPerHub_When_SubscribersAttach(t *testing.T) {
	hub := streamhub.NewStreamHub("subscribers-per-hub-metric-test", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()

	hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})
	if got := streamhub.SubscribersPerHub(); got != 1 {
		t.Fatalf("expected SubscribersPerHub() == 1 after the first attach, got %d", got)
	}

	hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})
	if got := streamhub.SubscribersPerHub(); got != 2 {
		t.Fatalf("expected SubscribersPerHub() == 2 after the second attach, got %d", got)
	}
}

// TestStreamHubMetrics_should_IncrementResizeNegotiationsTotal_When_ResizeRequested
// backs the streamhub_resize_negotiations_total counter. Asserted as a delta,
// like TestActiveHubs_ and TestOverlapInvariant_ above, since it is
// process-wide and shared with every other test in this package that calls
// RequestResize.
func TestStreamHubMetrics_should_IncrementResizeNegotiationsTotal_When_ResizeRequested(t *testing.T) {
	before := streamhub.ResizeNegotiationsTotal()

	hub := streamhub.NewStreamHub("resize-negotiations-metric-test", nil, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()

	id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	hub.RequestResize(id, mustSize(t, 80, 24))

	if got := streamhub.ResizeNegotiationsTotal(); got != before+1 {
		t.Fatalf("expected ResizeNegotiationsTotal() to increment by 1 after RequestResize, got %d -> %d", before, got)
	}
}

// TestStreamHubMetrics_should_RecordBatchFlushFramesCoalesced_When_BatchFlushes
// backs the streamhub_batch_flush_frames_coalesced histogram: onBatchFlush
// records FramesCoalesced on every flush, and BatchFlushFramesCoalesced()
// exposes the most recently recorded value.
func TestStreamHubMetrics_should_RecordBatchFlushFramesCoalesced_When_BatchFlushes(t *testing.T) {
	hub := streamhub.NewStreamHub("batch-flush-metric-test", nil,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithBatchMaxWindow(5*time.Millisecond))
	defer func() { _ = hub.ForceTeardown() }()

	mt := streamhub.NewMemoryTransport()
	hub.AttachSubscriber(mt, streamhub.SubscriberCapability{})

	hub.OnRawOutput([]byte("hello"))
	hub.OnRawOutput([]byte("world"))

	if !waitFor(t, time.Second, func() bool { return len(mt.ReceivedFrames()) == 1 }) {
		t.Fatalf("expected exactly one flushed frame, got %d", len(mt.ReceivedFrames()))
	}

	if got := streamhub.BatchFlushFramesCoalesced(); got != 2 {
		t.Fatalf("expected BatchFlushFramesCoalesced() == 2 after two OnRawOutput calls coalesce into one flush, got %d", got)
	}
}
