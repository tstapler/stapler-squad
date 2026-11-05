package analytics

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newTestClientMetrics(t *testing.T) (*clientMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return newClientMetrics(mp.Meter("test")), reader
}

func collect(t *testing.T, r *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, r.Collect(context.Background(), &rm))
	out := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func histPoints[N int64 | float64](m metricdata.Metrics) []metricdata.HistogramDataPoint[N] {
	return m.Data.(metricdata.Histogram[N]).DataPoints
}

func sumPoints(m metricdata.Metrics) []metricdata.DataPoint[int64] {
	return m.Data.(metricdata.Sum[int64]).DataPoints
}

func ms(v int64) *int64 { return &v }

func TestObserveRPCRecordsDurationByMethodTransportCode(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	consumed := cm.observe(context.Background(), Event{
		EventName: "rpc.ListSessions", EventCategory: "rpc", DurationMs: ms(120),
		Labels: map[string]string{"method": "ListSessions", "transport": "ws", "code": "deadline_exceeded", "inflight": "3", "dup": "1", "networkQueueDelayMs": "40"},
	})
	assert.False(t, consumed, "rpc.* events are still stored")
	got := collect(t, r)
	pts := histPoints[float64](got["client.rpc.duration"])
	require.Len(t, pts, 1)
	m, _ := pts[0].Attributes.Value("method")
	tr, _ := pts[0].Attributes.Value("transport")
	code, _ := pts[0].Attributes.Value("outcome")
	assert.Equal(t, "ListSessions", m.AsString())
	assert.Equal(t, "ws", tr.AsString())
	assert.Equal(t, "deadline_exceeded", code.AsString())
	_, hasCode := pts[0].Attributes.Value("code")
	assert.False(t, hasCode, "the per-method histogram carries the outcome class, not the full code")
	fullCode, _ := sumPoints(got["client.rpc.codes"])[0].Attributes.Value("code")
	assert.Equal(t, "deadline_exceeded", fullCode.AsString())
	assert.Equal(t, int64(1), sumPoints(got["client.rpc.duplicate_inflight"])[0].Value)
	assert.Equal(t, uint64(1), histPoints[int64](got["client.rpc.inflight_at_start"])[0].Count)
	assert.Equal(t, uint64(1), histPoints[float64](got["client.rpc.network_queue_delay"])[0].Count)
}

func TestObserveRPCBoundsHostileLabels(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	for i := 0; i < 500; i++ {
		cm.observe(context.Background(), Event{EventName: "rpc.x", DurationMs: ms(1),
			Labels: map[string]string{"method": fmt.Sprintf("Junk%d", i), "transport": "evil/../path", "code": "<script>"}})
	}
	cm.observe(context.Background(), Event{EventName: "rpc.x", DurationMs: ms(1), Labels: map[string]string{"method": "ListSessions", "ok": "true"}})
	pts := histPoints[float64](collect(t, r)["client.rpc.duration"])
	methods := map[string]bool{}
	for _, p := range pts {
		v, _ := p.Attributes.Value("method")
		methods[v.AsString()] = true
		tr, _ := p.Attributes.Value("transport")
		assert.Equal(t, "unknown", tr.AsString())
	}
	assert.Equal(t, map[string]bool{"other": true, "ListSessions": true}, methods,
		"junk names cannot consume label budget: a real method stays visible after 500 junk ones")
}

func TestObserveClampsDurationsAndCounts(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	cm.observe(context.Background(), Event{EventName: "rpc.Foo", DurationMs: ms(1 << 40), Labels: map[string]string{"method": "Foo", "inflight": "999999"}})
	got := collect(t, r)
	dp := histPoints[float64](got["client.rpc.duration"])[0]
	assert.Equal(t, float64(maxClientDurationMs), dp.Sum)
	assert.Equal(t, int64(maxInflightObservation), histPoints[int64](got["client.rpc.inflight_at_start"])[0].Sum)
}

func TestMetricOnlyEventsAreConsumed(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	ctx := context.Background()
	for _, ev := range []Event{
		{EventName: eventCoalesce, Labels: map[string]string{"method": "GetSession", "outcome": "joined"}},
		{EventName: eventBatch, Labels: map[string]string{"size": "5"}},
		{EventName: eventBatchFallback, Labels: map[string]string{"reason": "request-failed"}},
		{EventName: eventWsLifecycle, Labels: map[string]string{"kind": "unary", "event": "close", "code": "1006"}},
		{EventName: eventWsSend, DurationMs: ms(30), Labels: map[string]string{"pending": "2"}},
		{EventName: eventLongTask, DurationMs: ms(180), Page: "sessions"},
	} {
		assert.True(t, cm.observe(ctx, ev), ev.EventName)
	}
	assert.True(t, cm.observe(ctx, Event{EventName: "cm.bogus"}), "unknown cm.* events are consumed and counted, never stored")
	got := collect(t, r)
	assert.Equal(t, int64(1), sumPoints(got["client.rpc.coalesce"])[0].Value)
	assert.Equal(t, int64(5), histPoints[int64](got["client.batch.size"])[0].Sum)
	assert.Equal(t, int64(1), sumPoints(got["client.batch.fallback"])[0].Value)
	code, _ := sumPoints(got["client.ws.events"])[0].Attributes.Value("close_code")
	assert.Equal(t, "1006", code.AsString())
	assert.Equal(t, uint64(1), histPoints[float64](got["client.ws.send_queue_delay"])[0].Count)
	assert.Equal(t, uint64(1), histPoints[float64](got["client.long_task.duration"])[0].Count)
	assert.Equal(t, int64(1), sumPoints(got["client.metrics.rejected"])[0].Value)
}

func TestWebVitalsCLSIsDescaled(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	cm.observe(context.Background(), Event{EventName: "web_vital.cls", DurationMs: ms(1500), Page: "sessions", Labels: map[string]string{"rating": "good"}})
	cm.observe(context.Background(), Event{EventName: "web_vital.lcp", DurationMs: ms(2100), Page: "Bad Page", Labels: map[string]string{"rating": "weird"}})
	cm.observe(context.Background(), Event{EventName: "web_vital.fid", DurationMs: ms(5)})
	pts := histPoints[float64](collect(t, r)["client.web_vital"])
	require.Len(t, pts, 2)
	for _, p := range pts {
		v, _ := p.Attributes.Value("vital")
		if v.AsString() == "cls" {
			assert.InDelta(t, 0.15, p.Sum, 1e-9)
		} else {
			page, _ := p.Attributes.Value("page")
			rating, _ := p.Attributes.Value("rating")
			assert.Equal(t, "other", page.AsString())
			assert.Equal(t, "unknown", rating.AsString())
		}
	}
}

func TestCloseCodeLabelIsAllowListed(t *testing.T) {
	assert.Equal(t, "1006", closeCodeLabel("1006"))
	assert.Equal(t, "none", closeCodeLabel(""))
	for _, hostile := range []string{"1234", "4001", "99999", "abc", "-1", "1006 "} {
		assert.Equal(t, "other", closeCodeLabel(hostile), hostile)
	}
}

func TestOutcomeClassFoldsCodes(t *testing.T) {
	want := map[string]string{"ok": "ok", "canceled": "canceled", "deadline_exceeded": "deadline_exceeded",
		"unavailable": "saturated", "resource_exhausted": "saturated", "invalid_argument": "error", "internal": "error", "unknown": "error"}
	for code, class := range want {
		assert.Equal(t, class, outcomeClass(code), code)
	}
}

func TestRPCSeriesStayBoundedUnderHostileCodes(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	for _, code := range []string{"invalid_argument", "internal", "data_loss", "aborted", "weird", "<x>"} {
		cm.observe(context.Background(), Event{EventName: "rpc.x", DurationMs: ms(1), Labels: map[string]string{"method": "ListSessions", "code": code}})
	}
	pts := histPoints[float64](collect(t, r)["client.rpc.duration"])
	assert.Len(t, pts, 1, "six distinct error codes collapse to one outcome series per method/transport")
}

func TestPageLabelIsAllowListed(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	for i := 0; i < 100; i++ { // squatting attempt: junk pages must not consume labels
		cm.observe(context.Background(), Event{EventName: eventLongTask, DurationMs: ms(80), Page: fmt.Sprintf("junk-%d", i)})
	}
	for page := range knownPages() {
		cm.observe(context.Background(), Event{EventName: eventLongTask, DurationMs: ms(80), Page: page})
	}
	seen := map[string]bool{}
	for _, p := range histPoints[float64](collect(t, r)["client.long_task.duration"]) {
		v, _ := p.Attributes.Value("page")
		seen[v.AsString()] = true
	}
	assert.True(t, seen["other"])
	assert.True(t, seen["sessions"], "a real page keeps its own label after 100 junk pages")
	assert.Len(t, seen, len(knownPages())+1)
}

func TestMissingDurationIsCountedNotSilentlyDropped(t *testing.T) {
	cm, r := newTestClientMetrics(t)
	ctx := context.Background()
	cm.observe(ctx, Event{EventName: "rpc.ListSessions", Labels: map[string]string{"method": "ListSessions"}})
	cm.observe(ctx, Event{EventName: eventLongTask, Page: "sessions"})
	cm.observe(ctx, Event{EventName: "web_vital.lcp"})
	var n int64
	for _, dp := range sumPoints(collect(t, r)["client.metrics.rejected"]) {
		v, _ := dp.Attributes.Value("reason")
		assert.Equal(t, "missing_duration", v.AsString())
		n += dp.Value
	}
	assert.Equal(t, int64(3), n)
}

// failingMeter makes every instrument registration fail, as a misconfigured SDK could.
type failingMeter struct{ metric.Meter }

func (failingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("boom")
}
func (failingMeter) Int64Histogram(string, ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	return nil, errors.New("boom")
}
func (failingMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, errors.New("boom")
}

func TestFailedRegistrationFallsBackToNoopInstruments(t *testing.T) {
	cm := newClientMetrics(failingMeter{noop.NewMeterProvider().Meter("")})
	assert.NotPanics(t, func() {
		for _, ev := range []Event{
			{EventName: "rpc.ListSessions", DurationMs: ms(5), Labels: map[string]string{"method": "ListSessions", "dup": "1", "inflight": "1", "networkQueueDelayMs": "3"}},
			{EventName: "web_vital.lcp", DurationMs: ms(5)},
			{EventName: eventCoalesce}, {EventName: eventBatch}, {EventName: eventBatchFallback},
			{EventName: eventWsLifecycle}, {EventName: eventWsSend, DurationMs: ms(1)}, {EventName: eventLongTask, DurationMs: ms(60)},
			{EventName: eventWsUnaryFallback}, {EventName: "cm.bogus"},
		} {
			cm.observe(context.Background(), ev)
		}
	})
}

func TestObserve_should_AggregateAndConsumeRoutineFallbacks_ButStoreRealOnes(t *testing.T) {
	cm, _ := newTestClientMetrics(t)
	ctx := context.Background()

	routine := Event{EventName: eventWsUnaryFallback, Labels: map[string]string{"reason": "not-eligible", "count": "40"}}
	assert.True(t, cm.observe(ctx, routine), "routine HTTP routing must not become an analytics row per flush")

	real := Event{EventName: eventWsUnaryFallback, Labels: map[string]string{"reason": "ws-call-timeout"}}
	assert.False(t, cm.observe(ctx, real), "true fallbacks keep being stored")
}
