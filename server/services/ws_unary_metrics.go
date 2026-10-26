package services

import (
	"context"
	"sync"
	"sync/atomic"

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

	// wsUnaryMethodOther bounds the method label: a method outside the allowlist is
	// attacker-controlled text, so it never becomes a label value.
	wsUnaryMethodOther = "other"
)

var (
	wsUnaryCallsCounter         = counterNamed(metricWsUnaryCalls, "Unary calls served over the WebSocket bridge by method and Connect code")
	wsUnaryDeniedCounter        = counterNamed(metricWsUnaryDenied, "Unary calls refused by the WebSocket bridge's deny-by-default allowlist")
	wsUnaryUpgradeDeniedCounter = counterNamed(metricWsUnaryUpgradeDenied, "Unary WebSocket upgrades refused, by listener and reason")
	wsUnaryInflightGauge        = sync.OnceValue(func() metric.Int64UpDownCounter {
		g, err := telemetry.GetMeter().Int64UpDownCounter(metricWsUnaryInflight,
			metric.WithDescription("Unary calls currently executing on the WebSocket bridge"))
		if err != nil {
			log.Error("services: failed to register "+metricWsUnaryInflight, "error", err)
			return nil
		}
		return g
	})
)

type unaryCallKey struct{ method, code string }
type unaryUpgradeKey struct{ listener, reason string }

// unaryMetrics is the bridge's counter set.
type unaryMetrics struct {
	mu            sync.Mutex
	calls         map[unaryCallKey]int64
	denied        map[string]int64
	upgradeDenied map[unaryUpgradeKey]int64
	inflight      atomic.Int64
}

func newUnaryMetrics() *unaryMetrics {
	return &unaryMetrics{
		calls:         map[unaryCallKey]int64{},
		denied:        map[string]int64{},
		upgradeDenied: map[unaryUpgradeKey]int64{},
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
