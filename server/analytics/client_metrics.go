package analytics

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	maxLabeledPages        = 32
	maxInflightObservation = 64
	maxBatchObservation    = 64
	otherLabel             = "other"
	unknownLabel           = "unknown"
)

var (
	pageShape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,23}$`)

	transports  = map[string]bool{"ws": true, "http": true, "batch": true, "stream": true}
	coalesceOut = map[string]bool{"started": true, "joined": true}
	wsKinds     = map[string]bool{"unary": true, "watch": true}
	wsEvents    = map[string]bool{"open": true, "close": true, "error": true, "reconnect": true}
	fallbacks   = map[string]bool{"breaker-open": true, "force-http": true, "ws-method-not-allowed": true, "ws-call-timeout": true}
	batchReason = map[string]bool{"request-failed": true, "server-declined": true}
	vitalNames  = map[string]bool{"lcp": true, "inp": true, "cls": true, "fcp": true, "ttfb": true}
	vitalRating = map[string]bool{"good": true, "needs-improvement": true, "poor": true}
	// connectCodes mirrors Connect's canonical code names; the client sends them
	// snake_case ("deadline_exceeded") and anything else becomes "unknown".
	connectCodes = map[string]bool{
		"ok": true, "canceled": true, "unknown": true, "invalid_argument": true, "deadline_exceeded": true,
		"not_found": true, "already_exists": true, "permission_denied": true, "resource_exhausted": true,
		"failed_precondition": true, "aborted": true, "out_of_range": true, "unimplemented": true,
		"internal": true, "unavailable": true, "data_loss": true, "unauthenticated": true,
	}
)

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

// cappedSet admits at most max distinct values; later new values collapse to "other".
type cappedSet struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	max   int
	shape *regexp.Regexp
}

func newCappedSet(max int, shape *regexp.Regexp) *cappedSet {
	return &cappedSet{seen: map[string]struct{}{}, max: max, shape: shape}
}

func (c *cappedSet) bound(v string) string {
	if !c.shape.MatchString(v) {
		return otherLabel
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[v]; ok {
		return v
	}
	if len(c.seen) >= c.max {
		return otherLabel
	}
	c.seen[v] = struct{}{}
	return v
}

func enum(v string, allowed map[string]bool) string {
	if allowed[v] {
		return v
	}
	return unknownLabel
}

// clientMetrics holds the instruments fed by browser events.
type clientMetrics struct {
	pages *cappedSet

	rpcDuration    metric.Float64Histogram
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
	cm := &clientMetrics{pages: newCappedSet(maxLabeledPages, pageShape)}
	var err error
	hist := func(name, desc, unit string, bounds []float64) metric.Float64Histogram {
		opts := []metric.Float64HistogramOption{metric.WithDescription(desc), metric.WithUnit(unit)}
		if bounds != nil {
			opts = append(opts, metric.WithExplicitBucketBoundaries(bounds...))
		}
		h, e := m.Float64Histogram(name, opts...)
		err = firstErr(err, e)
		return h
	}
	ihist := func(name, desc string, bounds []float64) metric.Int64Histogram {
		h, e := m.Int64Histogram(name, metric.WithDescription(desc), metric.WithExplicitBucketBoundaries(bounds...))
		err = firstErr(err, e)
		return h
	}
	counter := func(name, desc string) metric.Int64Counter {
		c, e := m.Int64Counter(name, metric.WithDescription(desc))
		err = firstErr(err, e)
		return c
	}
	countBounds := []float64{1, 2, 3, 4, 5, 6, 8, 12, 16, 24, 32, 64}
	// Reaches past the 30s default RPC deadline so timeouts are not lumped into +Inf.
	latencyBounds := []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 8000, 15000, 30000, 60000}
	cm.rpcDuration = hist("client.rpc.duration", "Browser-observed unary RPC duration (coalesced flight, includes queueing), by method, transport and Connect code", "ms", latencyBounds)
	cm.rpcQueueDelay = hist("client.rpc.network_queue_delay", "Browser fetch queueing before bytes hit the wire (HTTP/1.1 connection cap), by transport", "ms", latencyBounds)
	cm.rpcInflight = ihist("client.rpc.inflight_at_start", "Unary RPCs already in flight in the tab when a new one started", countBounds)
	cm.rpcDuplicates = counter("client.rpc.duplicate_inflight", "Unary RPCs started while an identical one was already in flight (coalescing missed it), by method")
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
		c.wsFallback.Add(ctx, 1, attrs(attribute.String("reason", enum(ev.Labels["reason"], fallbacks))))
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
		if ev.DurationMs != nil {
			c.longTask.Record(ctx, clampMs(*ev.DurationMs), attrs(attribute.String("page", c.pages.bound(ev.Page))))
		}
	case consumed:
		c.rejectedEvents.Add(ctx, 1, attrs(attribute.String("reason", "unknown_event")))
	}
	return consumed
}

func (c *clientMetrics) observeRPC(ctx context.Context, ev Event) {
	if ev.DurationMs == nil {
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
		attribute.String("code", code)))
	if q, err := strconv.ParseInt(ev.Labels["networkQueueDelayMs"], 10, 64); err == nil && q >= 0 {
		c.rpcQueueDelay.Record(ctx, clampMs(q), attrs(attribute.String("transport", transport)))
	}
	if ev.Labels["inflight"] != "" {
		c.rpcInflight.Record(ctx, boundedInt(ev.Labels["inflight"], maxInflightObservation), attrs(attribute.String("transport", transport)))
	}
	if ev.Labels["dup"] == "1" {
		c.rpcDuplicates.Add(ctx, 1, attrs(attribute.String("method", method)))
	}
}

func (c *clientMetrics) observeWebVital(ctx context.Context, ev Event) {
	vital := strings.TrimPrefix(ev.EventName, eventWebVitalPrefix)
	if !vitalNames[vital] || ev.DurationMs == nil {
		c.rejectedEvents.Add(ctx, 1, attrs(attribute.String("reason", "bad_web_vital")))
		return
	}
	v := float64(clampMs(*ev.DurationMs))
	if vital == "cls" { // client sends CLS x10000 in duration_ms
		v /= 10000
	}
	c.webVital.Record(ctx, v, attrs(
		attribute.String("vital", vital),
		attribute.String("rating", enum(ev.Labels["rating"], vitalRating)),
		attribute.String("page", c.pages.bound(ev.Page))))
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

// closeCodeLabel keeps RFC 6455 / registered application close codes only; "" is a client-side drop with no code.
func closeCodeLabel(s string) string {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1000 || n > 4999 {
		return "none"
	}
	return strconv.Itoa(n)
}
