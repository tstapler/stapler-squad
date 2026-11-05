package services

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Unary WebSocket bridge counters (Task 5.2.1e). Each is emitted through the
// OpenTelemetry meter like the repo's other counters and tallied in-process so
// tests and diagnostics can read them without a metric reader.

const (
	metricWsUnaryCalls         = "ws_unary_calls_total"
	metricWsUnaryDenied        = "ws_unary_denied_total"
	metricWsUnaryUpgradeDenied = "ws_unary_upgrade_denied_total"
	metricWsUnaryInflight      = "ws_unary_inflight"
	metricWsUnaryZombies       = "ws_unary_zombies"
	metricWsUnaryAbandoned     = "ws_unary_abandoned_total"

	// wsUnaryMethodOther bounds the method label: a method outside the allowlist is
	// attacker-controlled text, so it never becomes a label value.
	wsUnaryMethodOther = "other"
)

var (
	wsUnaryCallsCounter         = counterNamed(metricWsUnaryCalls, "Unary calls served over the WebSocket bridge by method and Connect code")
	wsUnaryDeniedCounter        = counterNamed(metricWsUnaryDenied, "Unary calls refused by the WebSocket bridge's deny-by-default allowlist")
	wsUnaryUpgradeDeniedCounter = counterNamed(metricWsUnaryUpgradeDenied, "Unary WebSocket upgrades refused, by listener and reason")
	wsUnaryConnsOpen            = upDownNamed("ws_unary_connections_open", "Unary WebSocket bridge connections currently open, by listener")
	wsUnaryConnsClosed          = counterNamed("ws_unary_connections_closed_total", "Unary WebSocket bridge connections ended, by listener and reason")
	wsUnaryWriteErrors          = counterNamed("ws_unary_write_errors_total", "Unary WebSocket bridge writer failures (each ends its connection), by listener")
	wsUnaryPingRTT              = histNamed("ws_unary_ping_rtt", "ms", "Server ping to client pong round trip on the unary WebSocket bridge, by listener")
	wsUnaryCallDuration         = histNamed("ws_unary_call_duration", "ms", "Unary call handling time on the WebSocket bridge (dispatch to reply enqueue; abandoned calls are not timed), by method")
	wsUnaryAbandonedCounter     = counterNamed(metricWsUnaryAbandoned, "Unary calls abandoned server-side before the handler returned, by reason (cancel, deadline, shutdown, zombie_cap)")
	wsUnaryZombiesGauge         = sync.OnceValue(func() metric.Int64UpDownCounter {
		g, err := telemetry.GetMeter().Int64UpDownCounter(metricWsUnaryZombies,
			metric.WithDescription("Abandoned unary handlers still running after their slot was released"))
		if err != nil {
			log.Error("services: failed to register "+metricWsUnaryZombies, "error", err)
			return nil
		}
		return g
	})
	wsUnaryInflightGauge = sync.OnceValue(func() metric.Int64UpDownCounter {
		g, err := telemetry.GetMeter().Int64UpDownCounter(metricWsUnaryInflight,
			metric.WithDescription("Unary calls currently executing on the WebSocket bridge"))
		if err != nil {
			log.Error("services: failed to register "+metricWsUnaryInflight, "error", err)
			return nil
		}
		return g
	})
)

func upDownNamed(name, desc string) func() metric.Int64UpDownCounter {
	return sync.OnceValue(func() metric.Int64UpDownCounter {
		c, err := telemetry.GetMeter().Int64UpDownCounter(name, metric.WithDescription(desc))
		if err != nil {
			log.Error("services: failed to register "+name, "error", err)
			return nil
		}
		return c
	})
}

// latencyBucketsMs reaches past the 30s default RPC deadline so timeouts are not lumped into +Inf.
var latencyBucketsMs = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 8000, 15000, 30000, 60000}

func histNamed(name, unit, desc string) func() metric.Float64Histogram {
	return sync.OnceValue(func() metric.Float64Histogram {
		h, err := telemetry.GetMeter().Float64Histogram(name, metric.WithUnit(unit), metric.WithDescription(desc),
			metric.WithExplicitBucketBoundaries(latencyBucketsMs...))
		if err != nil {
			log.Error("services: failed to register "+name, "error", err)
			return nil
		}
		return h
	})
}

// Connection-level and timing instruments; labels stay bounded (listener is a
// server-side constant, method goes through labelMethod, reason is an enum).

func (m *unaryMetrics) connOpened(listener string) {
	if g := wsUnaryConnsOpen(); g != nil {
		g.Add(context.Background(), 1, metric.WithAttributes(attribute.String("listener", listener)))
	}
}

func (m *unaryMetrics) connClosed(listener, reason string) {
	if g := wsUnaryConnsOpen(); g != nil {
		g.Add(context.Background(), -1, metric.WithAttributes(attribute.String("listener", listener)))
	}
	addOne(wsUnaryConnsClosed(), attribute.String("listener", listener), attribute.String("reason", reason))
}

func (m *unaryMetrics) writeError(listener string) {
	addOne(wsUnaryWriteErrors(), attribute.String("listener", listener))
}

func (m *unaryMetrics) pingRTT(listener string, d time.Duration) {
	if h := wsUnaryPingRTT(); h != nil {
		h.Record(context.Background(), float64(d)/float64(time.Millisecond), metric.WithAttributes(attribute.String("listener", listener)))
	}
}

func (m *unaryMetrics) callDuration(method string, d time.Duration) {
	if h := wsUnaryCallDuration(); h != nil {
		h.Record(context.Background(), float64(d)/float64(time.Millisecond), metric.WithAttributes(attribute.String("method", labelMethod(method))))
	}
}

type unaryCallKey struct{ method, code string }
type unaryUpgradeKey struct{ listener, reason string }

// unaryMetrics is the bridge's counter set.
type unaryMetrics struct {
	mu            sync.Mutex
	calls         map[unaryCallKey]int64
	denied        map[string]int64
	upgradeDenied map[unaryUpgradeKey]int64
	abandonedBy   map[string]int64
	inflight      atomic.Int64
	zombies       atomic.Int64
}

func newUnaryMetrics() *unaryMetrics {
	return &unaryMetrics{
		calls:         map[unaryCallKey]int64{},
		denied:        map[string]int64{},
		upgradeDenied: map[unaryUpgradeKey]int64{},
		abandonedBy:   map[string]int64{},
	}
}

// labelMethod keeps the label set bounded to the allowlist plus "other".
func labelMethod(method string) string {
	if wsUnaryMethodAllowed(method) {
		return method
	}
	return wsUnaryMethodOther
}

func (m *unaryMetrics) call(method, code string) {
	method = labelMethod(method)
	m.mu.Lock()
	m.calls[unaryCallKey{method, code}]++
	m.mu.Unlock()
	addOne(wsUnaryCallsCounter(), attribute.String("method", method), attribute.String("code", code))
}

func (m *unaryMetrics) deny(method string) {
	method = labelMethod(method)
	m.mu.Lock()
	m.denied[method]++
	m.mu.Unlock()
	addOne(wsUnaryDeniedCounter(), attribute.String("method", method))
}

func (m *unaryMetrics) upgradeDeny(listener, reason string) {
	m.mu.Lock()
	m.upgradeDenied[unaryUpgradeKey{listener, reason}]++
	m.mu.Unlock()
	addOne(wsUnaryUpgradeDeniedCounter(), attribute.String("listener", listener), attribute.String("reason", reason))
}

func (m *unaryMetrics) inflightDelta(d int64) {
	m.inflight.Add(d)
	if g := wsUnaryInflightGauge(); g != nil {
		g.Add(context.Background(), d)
	}
}

func (m *unaryMetrics) zombieDelta(d int64) {
	m.zombies.Add(d)
	if g := wsUnaryZombiesGauge(); g != nil {
		g.Add(context.Background(), d)
	}
}

// abandoned counts a call whose handler outlived its slot (or refused to), by reason.
func (m *unaryMetrics) abandoned(reason string) {
	m.mu.Lock()
	m.abandonedBy[reason]++
	m.mu.Unlock()
	addOne(wsUnaryAbandonedCounter(), attribute.String("reason", reason))
}

// Zombies returns the ws_unary_zombies gauge value.
func (m *unaryMetrics) Zombies() int64 { return m.zombies.Load() }

// Abandoned returns ws_unary_abandoned_total for a reason.
func (m *unaryMetrics) Abandoned(reason string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.abandonedBy[reason]
}

// Calls returns the tally for a method label and Connect code.
func (m *unaryMetrics) Calls(method, code string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[unaryCallKey{method, code}]
}

// Denied returns ws_unary_denied_total for a method label.
func (m *unaryMetrics) Denied(method string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.denied[method]
}

// UpgradeDenied returns ws_unary_upgrade_denied_total for a listener and reason.
func (m *unaryMetrics) UpgradeDenied(listener, reason string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upgradeDenied[unaryUpgradeKey{listener, reason}]
}

// Inflight returns the ws_unary_inflight gauge value.
func (m *unaryMetrics) Inflight() int64 { return m.inflight.Load() }

// UnaryWsCallCount is one ws_unary_calls_total series.
type UnaryWsCallCount struct {
	Method string `json:"method"`
	Code   string `json:"code"`
	Count  int64  `json:"count"`
}

// UnaryWsDeniedCount is one ws_unary_denied_total series.
type UnaryWsDeniedCount struct {
	Method string `json:"method"`
	Count  int64  `json:"count"`
}

// UnaryWsUpgradeDeniedCount is one ws_unary_upgrade_denied_total series.
type UnaryWsUpgradeDeniedCount struct {
	Listener string `json:"listener"`
	Reason   string `json:"reason"`
	Count    int64  `json:"count"`
}

// UnaryWsAbandonedCount is one ws_unary_abandoned_total series.
type UnaryWsAbandonedCount struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// UnaryWsReport is the /actuator/ws-unary payload. Method labels are the
// allowlisted names plus "other", the same bound the OTel counters use.
type UnaryWsReport struct {
	CallsTotal         []UnaryWsCallCount          `json:"ws_unary_calls_total"`
	DeniedTotal        []UnaryWsDeniedCount        `json:"ws_unary_denied_total"`
	UpgradeDeniedTotal []UnaryWsUpgradeDeniedCount `json:"ws_unary_upgrade_denied_total"`
	Inflight           int64                       `json:"ws_unary_inflight"`
	Zombies            int64                       `json:"ws_unary_zombies"`
	AbandonedTotal     []UnaryWsAbandonedCount     `json:"ws_unary_abandoned_total"`
}

// Report returns a sorted, JSON-ready copy of the tallies. A nil receiver
// (bridge not built yet) yields an empty report.
func (m *unaryMetrics) Report() UnaryWsReport {
	r := UnaryWsReport{
		CallsTotal:         []UnaryWsCallCount{},
		DeniedTotal:        []UnaryWsDeniedCount{},
		UpgradeDeniedTotal: []UnaryWsUpgradeDeniedCount{},
		AbandonedTotal:     []UnaryWsAbandonedCount{},
	}
	if m == nil {
		return r
	}
	m.mu.Lock()
	for k, n := range m.calls {
		r.CallsTotal = append(r.CallsTotal, UnaryWsCallCount{Method: k.method, Code: k.code, Count: n})
	}
	for k, n := range m.denied {
		r.DeniedTotal = append(r.DeniedTotal, UnaryWsDeniedCount{Method: k, Count: n})
	}
	for k, n := range m.upgradeDenied {
		r.UpgradeDeniedTotal = append(r.UpgradeDeniedTotal, UnaryWsUpgradeDeniedCount{Listener: k.listener, Reason: k.reason, Count: n})
	}
	for k, n := range m.abandonedBy {
		r.AbandonedTotal = append(r.AbandonedTotal, UnaryWsAbandonedCount{Reason: k, Count: n})
	}
	m.mu.Unlock()
	r.Inflight = m.inflight.Load()
	r.Zombies = m.zombies.Load()
	sort.Slice(r.AbandonedTotal, func(i, j int) bool { return r.AbandonedTotal[i].Reason < r.AbandonedTotal[j].Reason })
	sort.Slice(r.CallsTotal, func(i, j int) bool {
		a, b := r.CallsTotal[i], r.CallsTotal[j]
		return a.Method+"\x00"+a.Code < b.Method+"\x00"+b.Code
	})
	sort.Slice(r.DeniedTotal, func(i, j int) bool { return r.DeniedTotal[i].Method < r.DeniedTotal[j].Method })
	sort.Slice(r.UpgradeDeniedTotal, func(i, j int) bool {
		a, b := r.UpgradeDeniedTotal[i], r.UpgradeDeniedTotal[j]
		return a.Listener+"\x00"+a.Reason < b.Listener+"\x00"+b.Reason
	})
	return r
}

// Report exposes the bridge's counters for the actuator endpoint; a nil bridge
// reports empty.
func (b *UnaryWsBridge) Report() UnaryWsReport {
	if b == nil {
		return (*unaryMetrics)(nil).Report()
	}
	return b.metrics.Report()
}
