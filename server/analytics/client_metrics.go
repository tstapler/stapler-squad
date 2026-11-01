package analytics

import (
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/tstapler/stapler-squad/gen/proto/go/session/v1" // registers the RPC descriptors knownRPCMethods reads
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Browser-reported network telemetry. The SPA already batches events to POST
// /api/analytics (rate-limited, body-capped, no new auth surface); this file turns
// an allow-list of those events into OTel instruments. Every label value is either
// from a fixed enum or shape-checked and cardinality-capped, because the sender is
// untrusted: a method, route or code from the wire never becomes an unbounded label.

// Event names the client sends. "cm." events exist only to feed metrics and are
// consumed here instead of being stored in the analytics table.
const (
	clientEventPrefix      = "cm."
	eventRPCPrefix         = "rpc."
	eventWebVitalPrefix    = "web_vital."
	eventWsUnaryFallback   = "ws_unary_fallback_total"
	eventCoalesce          = "cm.coalesce"
	eventBatch             = "cm.batch"
	eventBatchFallback     = "cm.batch_fallback"
	eventWsLifecycle       = "cm.ws"
	eventWsSend            = "cm.ws_send"
	eventLongTask          = "cm.longtask"
	maxClientDurationMs    = 10 * 60 * 1000
	maxInflightObservation = 64
	maxBatchObservation    = 64
	maxFallbackCount       = 100000
	otherLabel             = "other"
	reasonKey              = "reason"
	unknownLabel           = "unknown"
)

var (
	transports  = map[string]bool{"ws": true, "http": true, "batch": true, "stream": true}
	coalesceOut = map[string]bool{"started": true, "joined": true}
	wsKinds     = map[string]bool{"unary": true, "watch": true}
	wsEvents    = map[string]bool{"open": true, "close": true, "error": true, "reconnect": true}
	fallbacks   = map[string]bool{"breaker-open": true, "force-http": true, "ws-method-not-allowed": true, "ws-call-timeout": true,
		"ws-resource-exhausted": true, "ws-unavailable": true, "ws-send-queue-full": true,
		"not-eligible": true, "flag-off": true, "flags-not-loaded": true, "remote-flag-off": true, "session-parked": true}
	// routineFallbacks are counted per HTTP call that never tried the socket, not true fallbacks.
	routineFallbacks = map[string]bool{"not-eligible": true, "flag-off": true, "flags-not-loaded": true, "remote-flag-off": true, "session-parked": true}
	batchReason      = map[string]bool{"request-failed": true, "server-declined": true}
	vitalNames       = map[string]bool{"lcp": true, "inp": true, "cls": true, "fcp": true, "ttfb": true}
	vitalRating      = map[string]bool{"good": true, "needs-improvement": true, "poor": true}
	// connectCodes mirrors Connect's canonical code names; the client sends them
	// snake_case ("deadline_exceeded") and anything else becomes "unknown".
	connectCodes = map[string]bool{
		"ok": true, "canceled": true, "unknown": true, "invalid_argument": true, "deadline_exceeded": true,
		"not_found": true, "already_exists": true, "permission_denied": true, "resource_exhausted": true,
		"failed_precondition": true, "aborted": true, "out_of_range": true, "unimplemented": true,
		"internal": true, "unavailable": true, "data_loss": true, "unauthenticated": true,
	}
)

// clientPagesJSON lists the first-path-segment page labels the SPA can report. A jest test
// (longTasks.test.ts) fails when a route in route-meta.ts is missing, so the file cannot drift.
//
//go:embed client_pages.json
var clientPagesJSON []byte

var knownPages = sync.OnceValue(func() map[string]bool {
	var pages []string
	if err := json.Unmarshal(clientPagesJSON, &pages); err != nil {
		log.Error("analytics: bad client_pages.json", "error", err)
	}
	known := make(map[string]bool, len(pages))
	for _, p := range pages {
		known[p] = true
	}
	return known
})

func boundPage(v string) string {
	if knownPages()[v] {
		return v
	}
	return otherLabel
}

// knownRPCMethods is every RPC method name compiled into the binary. The method label is drawn
// only from this set, so a sender cannot spend the label budget on junk names ("first come" caps
// let one bad client push every real method to "other").
var knownRPCMethods = sync.OnceValue(func() map[string]bool {
	known := map[string]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			ms := fd.Services().Get(i).Methods()
			for j := 0; j < ms.Len(); j++ {
				known[string(ms.Get(j).Name())] = true
			}
		}
		return true
	})
	return known
})

func boundMethod(v string) string {
	if knownRPCMethods()[v] {
		return v
	}
	return otherLabel
}

func enum(v string, allowed map[string]bool) string {
	if allowed[v] {
		return v
	}
	return unknownLabel
}

// clientMetrics holds the instruments fed by browser events.
type clientMetrics struct {
	rpcDuration    metric.Float64Histogram
	rpcCodes       metric.Int64Counter
	rpcQueueDelay  metric.Float64Histogram
	rpcInflight    metric.Int64Histogram
	rpcDuplicates  metric.Int64Counter
	rpcCoalesce    metric.Int64Counter
	wsEvents       metric.Int64Counter
	wsPending      metric.Int64Histogram
	wsQueueDelay   metric.Float64Histogram
	wsFallback     metric.Int64Counter
	batchSize      metric.Int64Histogram
	batchFallback  metric.Int64Counter
	webVital       metric.Float64Histogram
	longTask       metric.Float64Histogram
	rejectedEvents metric.Int64Counter
}

func newClientMetrics(m metric.Meter) *clientMetrics {
	cm := &clientMetrics{}
	var err error
	hist := func(name, desc, unit string, bounds []float64) metric.Float64Histogram {
		opts := []metric.Float64HistogramOption{metric.WithDescription(desc), metric.WithUnit(unit)}
		if bounds != nil {
			opts = append(opts, metric.WithExplicitBucketBoundaries(bounds...))
		}
		h, e := m.Float64Histogram(name, opts...)
		if e != nil {
			err = firstErr(err, e)
			h, _ = noop.NewMeterProvider().Meter("").Float64Histogram(name)
		}
		return h
	}
	ihist := func(name, desc string, bounds []float64) metric.Int64Histogram {
		h, e := m.Int64Histogram(name, metric.WithDescription(desc), metric.WithExplicitBucketBoundaries(bounds...))
		if e != nil {
			err = firstErr(err, e)
			h, _ = noop.NewMeterProvider().Meter("").Int64Histogram(name)
		}
		return h
	}
	counter := func(name, desc string) metric.Int64Counter {
		c, e := m.Int64Counter(name, metric.WithDescription(desc))
		if e != nil {
			err = firstErr(err, e)
			c, _ = noop.NewMeterProvider().Meter("").Int64Counter(name)
		}
		return c
	}
	countBounds := []float64{1, 2, 3, 4, 5, 6, 8, 12, 16, 24, 32, 64}
	// Reaches past the 30s default RPC deadline so timeouts are not lumped into +Inf.
	latencyBounds := []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 8000, 15000, 30000, 60000}
	cm.rpcDuration = hist("client.rpc.duration", "Browser-observed unary RPC duration (coalesced flight, includes queueing), by method, transport and outcome class", "ms", latencyBounds)
	cm.rpcCodes = counter("client.rpc.codes", "Browser-observed unary RPC results by full Connect code (no method label, so the code set stays small)")
	cm.rpcQueueDelay = hist("client.rpc.network_queue_delay", "Browser fetch queueing before bytes hit the wire (HTTP/1.1 connection cap), by transport", "ms", latencyBounds)
	cm.rpcInflight = ihist("client.rpc.inflight_at_start", "RPCs already in flight in the tab when a new one started (module-wide counter: includes stream calls until headers and calls on the non-shared transport)", append([]float64{0}, countBounds...))
	cm.rpcDuplicates = counter("client.rpc.duplicate_inflight", "Read RPCs started while an identical one was in flight on the shared transport, by method; coalescible=true means coalescing should have merged it (a miss), false means the method is not coalescible (a candidate)")
	cm.rpcCoalesce = counter("client.rpc.coalesce", "Coalescible unary calls by outcome: started a flight or joined one")
	cm.wsEvents = counter("client.ws.events", "Browser WebSocket lifecycle events by kind, event and close code")
	cm.wsPending = ihist("client.ws.pending_at_send", "Unary WebSocket calls pending (sent, unanswered) when a call was submitted", countBounds)
	cm.wsQueueDelay = hist("client.ws.send_queue_delay", "Time a unary call waited before its frame was written to the socket", "ms", latencyBounds)
	cm.wsFallback = counter("client.ws.unary_fallback", "Unary calls that fell back from WebSocket to HTTP, by reason")
	cm.batchSize = ihist("client.batch.size", "Calls per /batch/unary request", countBounds)
	cm.batchFallback = counter("client.batch.fallback", "Batch members re-run individually, by reason")
	cm.webVital = hist("client.web_vital", "Core Web Vitals (LCP/INP/FCP/TTFB in ms, CLS unitless), by vital, rating and page", "", []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 100, 200, 500, 1000, 2000, 2500, 4000, 6000, 10000})
	cm.longTask = hist("client.long_task.duration", "Main-thread tasks over 50ms, by page", "ms", []float64{50, 75, 100, 150, 250, 500, 1000, 2500, 5000})
	cm.rejectedEvents = counter("client.metrics.rejected", "Browser metric events dropped by server validation, by reason")
	if err != nil {
		log.Error("analytics: failed to register client metrics", "error", err)
	}
	return cm
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

var defaultClientMetrics = sync.OnceValue(func() *clientMetrics { return newClientMetrics(telemetry.GetMeter()) })

func attrs(kv ...attribute.KeyValue) metric.MeasurementOption { return metric.WithAttributes(kv...) }

// observe feeds ev into the metrics. consumed is true for "cm." events that must
// not be stored in the analytics table.
func (c *clientMetrics) observe(ctx context.Context, ev Event) (consumed bool) {
	name := ev.EventName
	consumed = strings.HasPrefix(name, clientEventPrefix)
	switch {
	case strings.HasPrefix(name, eventRPCPrefix):
		c.observeRPC(ctx, ev)
	case strings.HasPrefix(name, eventWebVitalPrefix):
		c.observeWebVital(ctx, ev)
	case name == eventWsUnaryFallback:
		reason := enum(ev.Labels["reason"], fallbacks)
		n := boundedInt(ev.Labels["count"], maxFallbackCount)
		if n == 0 {
			n = 1
		}
		c.wsFallback.Add(ctx, n, attrs(attribute.String("reason", reason)))
		// Routine HTTP routing is high volume and already aggregated into the metric.
		consumed = consumed || routineFallbacks[reason]
	case name == eventCoalesce:
		c.rpcCoalesce.Add(ctx, 1, attrs(
			attribute.String("method", boundMethod(ev.Labels["method"])),
			attribute.String("outcome", enum(ev.Labels["outcome"], coalesceOut))))
	case name == eventBatch:
		c.batchSize.Record(ctx, boundedInt(ev.Labels["size"], maxBatchObservation))
	case name == eventBatchFallback:
		c.batchFallback.Add(ctx, 1, attrs(attribute.String("reason", enum(ev.Labels["reason"], batchReason))))
	case name == eventWsLifecycle:
		c.wsEvents.Add(ctx, 1, attrs(
			attribute.String("kind", enum(ev.Labels["kind"], wsKinds)),
			attribute.String("event", enum(ev.Labels["event"], wsEvents)),
			attribute.String("close_code", closeCodeLabel(ev.Labels["code"]))))
	case name == eventWsSend:
		c.wsPending.Record(ctx, boundedInt(ev.Labels["pending"], maxInflightObservation))
		if ev.DurationMs != nil {
			c.wsQueueDelay.Record(ctx, clampMs(*ev.DurationMs))
		}
	case name == eventLongTask:
		if ev.DurationMs == nil {
			c.reject(ctx, "missing_duration")
		} else {
			c.longTask.Record(ctx, clampMs(*ev.DurationMs), attrs(attribute.String("page", boundPage(ev.Page))))
		}
	case consumed:
		c.rejectedEvents.Add(ctx, 1, attrs(attribute.String("reason", "unknown_event")))
	}
	return consumed
}

// outcomeClass folds the 17 Connect codes into 5 classes so the per-method duration histogram
// does not multiply by every code; the full code lives on client.rpc.codes.
func outcomeClass(code string) string {
	switch code {
	case "ok", "canceled", "deadline_exceeded":
		return code
	case "unavailable", "resource_exhausted":
		return "saturated"
	default:
		return "error"
	}
}

func (c *clientMetrics) reject(ctx context.Context, reason string) {
	c.rejectedEvents.Add(ctx, 1, attrs(attribute.String(reasonKey, reason)))
}

func (c *clientMetrics) observeRPC(ctx context.Context, ev Event) {
	if ev.DurationMs == nil {
		c.reject(ctx, "missing_duration")
		return
	}
	method := boundMethod(ev.Labels["method"])
	transport := enum(ev.Labels["transport"], transports)
	code := ev.Labels["code"]
	switch {
	case code == "" && ev.Labels["ok"] == "true":
		code = "ok"
	case !connectCodes[code]:
		code = unknownLabel
	}
	c.rpcDuration.Record(ctx, clampMs(*ev.DurationMs), attrs(
		attribute.String("method", method),
		attribute.String("transport", transport),
		attribute.String("outcome", outcomeClass(code))))
	c.rpcCodes.Add(ctx, 1, attrs(attribute.String("code", code)))
	if q, err := strconv.ParseInt(ev.Labels["networkQueueDelayMs"], 10, 64); err == nil && q >= 0 {
		c.rpcQueueDelay.Record(ctx, clampMs(q), attrs(attribute.String("transport", transport)))
	}
	if ev.Labels["inflight"] != "" {
		c.rpcInflight.Record(ctx, boundedInt(ev.Labels["inflight"], maxInflightObservation), attrs(attribute.String("transport", transport)))
	}
	if ev.Labels["dup"] == "1" {
		c.rpcDuplicates.Add(ctx, 1, attrs(
			attribute.String("method", method),
			attribute.Bool("coalescible", ev.Labels["coalescible"] == "1")))
	}
}

func (c *clientMetrics) observeWebVital(ctx context.Context, ev Event) {
	vital := strings.TrimPrefix(ev.EventName, eventWebVitalPrefix)
	if !vitalNames[vital] {
		c.reject(ctx, "bad_web_vital")
		return
	}
	if ev.DurationMs == nil {
		c.reject(ctx, "missing_duration")
		return
	}
	v := float64(clampMs(*ev.DurationMs))
	if vital == "cls" { // client sends CLS x10000 in duration_ms
		v /= 10000
	}
	c.webVital.Record(ctx, v, attrs(
		attribute.String("vital", vital),
		attribute.String("rating", enum(ev.Labels["rating"], vitalRating)),
		attribute.String("page", boundPage(ev.Page))))
}

func clampMs(v int64) float64 {
	if v < 0 {
		return 0
	}
	if v > maxClientDurationMs {
		return maxClientDurationMs
	}
	return float64(v)
}

func boundedInt(s string, max int64) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	if n > max {
		return max
	}
	return n
}

// wsCloseCodes are the close codes browsers and the bridge actually produce; anything else is "other".
var wsCloseCodes = map[string]bool{
	"1000": true, "1001": true, "1002": true, "1003": true, "1005": true, "1006": true,
	"1008": true, "1009": true, "1011": true, "1012": true, "1013": true,
}

// closeCodeLabel maps a close code to a bounded label: "none" when absent, "other" when not in wsCloseCodes.
func closeCodeLabel(s string) string {
	if s == "" {
		return "none"
	}
	if wsCloseCodes[s] {
		return s
	}
	return otherLabel
}
