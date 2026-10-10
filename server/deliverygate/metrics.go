package deliverygate

import (
	"context"
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
	CounterSuppressed          = "notification_delivery_suppressed_total"          // channel,type,reason,kind
	CounterWouldSuppress       = "notification_delivery_would_suppress_total"      // channel,type,reason,kind
	CounterHiddenDelivered     = "notification_hidden_delivered_total"             // channel,class,kind
	CounterUnresolved          = "notification_delivery_unresolved_total"          // class
	CounterIndexMiss           = "notification_gate_index_miss_total"              // (none)
	CounterIndexRefresh        = "notification_gate_index_refresh_total"           // result
	CounterResolvedLater       = "notification_unresolved_resolved_later_total"    // (none)
	CounterFilterPanic         = "notification_gate_filter_panic_total"            // (none)
	CounterLegacySuppressed    = "notification_legacy_hidden_suppressed_total"     // site,type,class
	CounterRPCUnversioned      = "notification_rpc_unversioned_total"              // (none)
	CounterAuditDegraded       = "hidden_session_audit_degraded_total"             // mode
	CounterCrashCoalesced      = "notification_crash_coalesced_total"              // (none)
	CounterLeaseWedgeNotified  = "hidden_session_write_lease_wedge_notified_total" // (none)
	histogramFilterDurationSec = "notification_gate_filter_duration_seconds"       // OTel only
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
	CounterAuditDegraded:    {"mode"},
}

// maxLabels is the widest counter label set (suppressed: channel, type, reason, kind).
const maxLabels = 4

// ckey is a comparable counter key: building it allocates nothing.
type ckey struct {
	name   string
	labels [maxLabels]string
}

// counter is one (name, labels) series: the in-process value plus the OTel
// measurement option built once, so the hot path allocates nothing.
type counter struct {
	n    atomic.Uint64
	inst metric.Int64Counter
	opt  metric.AddOption
}

// Metrics holds the in-process counter mirror (and feeds the OTel instruments).
// The zero value is not usable; use NewMetrics.
type Metrics struct {
	mu       sync.RWMutex // guards counts' structure only; held for a map access, never across a call out
	counts   map[ckey]*counter
	filterNS atomic.Uint64
	filterN  atomic.Uint64
}

// NewMetrics returns an empty mirror.
func NewMetrics() *Metrics { return &Metrics{counts: map[ckey]*counter{}} }

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
		for _, n := range []string{CounterIndexMiss, CounterResolvedLater, CounterFilterPanic, CounterRPCUnversioned, CounterCrashCoalesced, CounterLeaseWedgeNotified} {
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

func makeKey(name string, labels []string) ckey {
	k := ckey{name: name}
	copy(k.labels[:], labels)
	return k
}

// series returns the counter for (name, labels), creating it on first use.
func (m *Metrics) series(name string, labels []string) *counter {
	k := makeKey(name, labels)
	m.mu.RLock()
	c := m.counts[k]
	m.mu.RUnlock()
	if c != nil {
		return c
	}
	c = &counter{inst: otelInstrument(name)}
	if c.inst != nil {
		keys := counterLabelKeys[name]
		attrs := make([]attribute.KeyValue, 0, len(labels))
		for i, l := range labels {
			if i < len(keys) {
				attrs = append(attrs, attribute.String(keys[i], l))
			}
		}
		c.opt = metric.WithAttributes(attrs...)
	}
	m.mu.Lock()
	if existing := m.counts[k]; existing != nil {
		c = existing
	} else {
		m.counts[k] = c
	}
	m.mu.Unlock()
	return c
}

// Add increments counter name for the given label values (in counterLabelKeys order).
func (m *Metrics) Add(name string, labels ...string) {
	if m == nil {
		return
	}
	c := m.series(name, labels)
	c.n.Add(1)
	if c.inst != nil {
		c.inst.Add(context.Background(), 1, c.opt)
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
	m.mu.RLock()
	c := m.counts[makeKey(name, labels)]
	m.mu.RUnlock()
	if c == nil {
		return 0
	}
	return c.n.Load()
}

// Total sums every label combination of a counter.
func (m *Metrics) Total(name string) uint64 {
	var sum uint64
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, c := range m.counts {
		if k.name == name {
			sum += c.n.Load()
		}
	}
	return sum
}

// Snapshot copies every counter (key is "name|label|label").
func (m *Metrics) Snapshot() map[string]uint64 {
	out := map[string]uint64{}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, c := range m.counts {
		key := k.name
		for _, l := range k.labels {
			if l == "" {
				break
			}
			key += counterLabelSeparator + l
		}
		out[key] = c.n.Load()
	}
	return out
}

// Series is one in-process counter series with its label values keyed by label name.
type Series struct {
	Name   string
	Labels map[string]string
	Count  uint64
}

// Series lists every counter series (the since-process-start view).
func (m *Metrics) Series() []Series {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Series, 0, len(m.counts))
	for k, c := range m.counts {
		s := Series{Name: k.name, Labels: map[string]string{}, Count: c.n.Load()}
		for i, key := range counterLabelKeys[k.name] {
			if i < len(k.labels) {
				s.Labels[key] = k.labels[i]
			}
		}
		out = append(out, s)
	}
	return out
}

var counterShortNames = map[string]string{
	CounterSuppressed:         "suppressed",
	CounterWouldSuppress:      "would_suppress",
	CounterHiddenDelivered:    "hidden_delivered",
	CounterUnresolved:         "unresolved",
	CounterIndexMiss:          "index_miss",
	CounterIndexRefresh:       "index_refresh",
	CounterResolvedLater:      "unresolved_resolved_later",
	CounterFilterPanic:        "filter_panic",
	CounterLegacySuppressed:   "legacy_hidden_suppressed",
	CounterRPCUnversioned:     "rpc_unversioned",
	CounterAuditDegraded:      "audit_degraded",
	CounterCrashCoalesced:     "crash_coalesced",
	CounterLeaseWedgeNotified: "lease_wedge_notified",
}

// ShortCounterName is the name the stats RPC reports for a counter.
func ShortCounterName(name string) string {
	if s, ok := counterShortNames[name]; ok {
		return s
	}
	return name
}
