package deliverygate

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Channel is the closed label set of delivery paths the gate covers.
type Channel string

const (
	ChannelBus          Channel = "bus"
	ChannelPushStatus   Channel = "push_status"
	ChannelAutoApproved Channel = "auto_approved"
	ChannelSlack        Channel = "slack"
)

// Counter names. Each is mirrored in-process (the OTel meter is a no-op when
// telemetry is not initialized, so the soak cannot depend on it).
const (
	CounterSuppressed          = "notification_delivery_suppressed_total"       // channel,type,reason,kind
	CounterWouldSuppress       = "notification_delivery_would_suppress_total"   // channel,type,reason,kind
	CounterHiddenDelivered     = "notification_hidden_delivered_total"          // channel,class,kind
	CounterUnresolved          = "notification_delivery_unresolved_total"       // class
	CounterIndexMiss           = "notification_gate_index_miss_total"           // (none)
	CounterIndexRefresh        = "notification_gate_index_refresh_total"        // result
	CounterResolvedLater       = "notification_unresolved_resolved_later_total" // (none)
	CounterFilterPanic         = "notification_gate_filter_panic_total"         // (none)
	CounterLegacySuppressed    = "notification_legacy_hidden_suppressed_total"  // site,type,class
	CounterRPCUnversioned      = "notification_rpc_unversioned_total"           // (none)
	histogramFilterDurationSec = "notification_gate_filter_duration_seconds"    // OTel only
	counterLabelSeparator      = "|"
)

// Index refresh result label values.
const (
	RefreshStarted            = "started"
	RefreshSkippedMinInterval = "skipped_min_interval"
	RefreshTimeout            = "timeout"
	RefreshError              = "error"
	RefreshOK                 = "ok"
)

var counterLabelKeys = map[string][]string{
	CounterSuppressed:       {"channel", "type", "reason", "kind"},
	CounterWouldSuppress:    {"channel", "type", "reason", "kind"},
	CounterHiddenDelivered:  {"channel", "class", "kind"},
	CounterUnresolved:       {"class"},
	CounterIndexRefresh:     {"result"},
	CounterLegacySuppressed: {"site", "type", "class"},
}

// Metrics holds the in-process counter mirror (and feeds the OTel instruments).
// The zero value is not usable; use NewMetrics.
type Metrics struct {
	counts   sync.Map // string key -> *atomic.Uint64
	filterNS atomic.Uint64
	filterN  atomic.Uint64
}

// NewMetrics returns an empty mirror.
func NewMetrics() *Metrics { return &Metrics{} }

var (
	otelOnce       sync.Once
	otelCounters   sync.Map // name -> metric.Int64Counter
	otelFilterHist metric.Float64Histogram
)

func otelInstrument(name string) metric.Int64Counter {
	otelOnce.Do(func() {
		meter := telemetry.GetMeter()
		for n := range counterLabelKeys {
			if c, err := meter.Int64Counter(n); err == nil {
				otelCounters.Store(n, c)
			}
		}
		for _, n := range []string{CounterIndexMiss, CounterResolvedLater, CounterFilterPanic, CounterRPCUnversioned} {
			if c, err := meter.Int64Counter(n); err == nil {
				otelCounters.Store(n, c)
			}
		}
		if h, err := meter.Float64Histogram(histogramFilterDurationSec); err == nil {
			otelFilterHist = h
		}
	})
	if v, ok := otelCounters.Load(name); ok {
		return v.(metric.Int64Counter)
	}
	return nil
}

// Add increments counter name for the given label values (in counterLabelKeys order).
func (m *Metrics) Add(name string, labels ...string) {
	if m == nil {
		return
	}
	key := metricKey(name, labels...)
	v, ok := m.counts.Load(key)
	if !ok {
		v, _ = m.counts.LoadOrStore(key, new(atomic.Uint64))
	}
	v.(*atomic.Uint64).Add(1)

	if c := otelInstrument(name); c != nil {
		keys := counterLabelKeys[name]
		attrs := make([]attribute.KeyValue, 0, len(labels))
		for i, l := range labels {
			if i < len(keys) {
				attrs = append(attrs, attribute.String(keys[i], l))
			}
		}
		c.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

// ObserveFilterDuration records one filter invocation's duration.
func (m *Metrics) ObserveFilterDuration(d time.Duration) {
	if m == nil {
		return
	}
	m.filterNS.Add(uint64(d))
	m.filterN.Add(1)
	otelInstrument(CounterIndexMiss) // ensure instruments exist
	if otelFilterHist != nil {
		otelFilterHist.Record(context.Background(), d.Seconds())
	}
}

// FilterDurationTotals returns the summed duration and observation count.
func (m *Metrics) FilterDurationTotals() (time.Duration, uint64) {
	return time.Duration(m.filterNS.Load()), m.filterN.Load()
}

// Value returns the current count for name and the given label values.
func (m *Metrics) Value(name string, labels ...string) uint64 {
	if m == nil {
		return 0
	}
	if v, ok := m.counts.Load(metricKey(name, labels...)); ok {
		return v.(*atomic.Uint64).Load()
	}
	return 0
}

// Total sums every label combination of a counter.
func (m *Metrics) Total(name string) uint64 {
	var sum uint64
	prefix := name + counterLabelSeparator
	m.counts.Range(func(k, v any) bool {
		if ks := k.(string); ks == name || strings.HasPrefix(ks, prefix) {
			sum += v.(*atomic.Uint64).Load()
		}
		return true
	})
	return sum
}

// Snapshot copies every counter (key is "name|label|label").
func (m *Metrics) Snapshot() map[string]uint64 {
	out := map[string]uint64{}
	m.counts.Range(func(k, v any) bool {
		out[k.(string)] = v.(*atomic.Uint64).Load()
		return true
	})
	return out
}

func metricKey(name string, labels ...string) string {
	if len(labels) == 0 {
		return name
	}
	return name + counterLabelSeparator + strings.Join(labels, counterLabelSeparator)
}
