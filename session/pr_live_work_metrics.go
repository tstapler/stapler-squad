package session

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Live-work metric names (observability plan, T5.2.4 / T5.4.6 / owner rules R1-R3).
const (
	MetricPRLiveWorkHoldTotal = "ssq_pr_live_work_hold_total"
	MetricLiveWorkProbeWired  = "ssq_live_work_probe_wired"
	MetricRuleLUnsettledItems = "ssq_rule_l_unsettled_items"

	LiveWorkResultHeld       = "held"
	LiveWorkResultMaxHold    = "max_hold"
	LiveWorkResultCapWorking = "cap_working"
	LiveWorkResultCapError   = "cap_error"
	LiveWorkResultErrorHeld  = "error_held"
	LiveWorkResultRuleLSkip  = "rule_l_skip"
)

type liveWorkMetricSet struct {
	results *counterByLabel
	otel    metric.Int64Counter

	probeWired atomic.Int64
	ruleLItems atomic.Int64
	wiredGauge metric.Int64Gauge
	ruleLGauge metric.Int64Gauge
}

var liveWorkMetrics = newLiveWorkMetricSet()

func newLiveWorkMetricSet() *liveWorkMetricSet {
	m := &liveWorkMetricSet{results: newCounterByLabel()}
	meter := telemetry.GetMeter()
	m.otel, _ = meter.Int64Counter(MetricPRLiveWorkHoldTotal)
	m.wiredGauge, _ = meter.Int64Gauge(MetricLiveWorkProbeWired)
	m.ruleLGauge, _ = meter.Int64Gauge(MetricRuleLUnsettledItems)
	return m
}

func (m *liveWorkMetricSet) count(result string) {
	m.results.add(result)
	if m.otel != nil {
		m.otel.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", result)))
	}
}

// LiveWorkResultCount is ssq_pr_live_work_hold_total{result} in this process.
func LiveWorkResultCount(result string) int64 { return liveWorkMetrics.results.get(result) }

// SetLiveWorkProbeWired records ssq_live_work_probe_wired (1 wired, 0 not).
func SetLiveWorkProbeWired(wired bool) {
	v := int64(0)
	if wired {
		v = 1
	}
	liveWorkMetrics.probeWired.Store(v)
	if liveWorkMetrics.wiredGauge != nil {
		liveWorkMetrics.wiredGauge.Record(context.Background(), v)
	}
}

// LiveWorkProbeWired reads ssq_live_work_probe_wired back.
func LiveWorkProbeWired() bool { return liveWorkMetrics.probeWired.Load() == 1 }

// SetRuleLUnsettledItems records how many terminal items the last 60 s sweep left
// unsettled because RULE L skipped a live session.
func SetRuleLUnsettledItems(n int) {
	liveWorkMetrics.ruleLItems.Store(int64(n))
	if liveWorkMetrics.ruleLGauge != nil {
		liveWorkMetrics.ruleLGauge.Record(context.Background(), int64(n))
	}
}

// RuleLUnsettledItems reads ssq_rule_l_unsettled_items back.
func RuleLUnsettledItems() int { return int(liveWorkMetrics.ruleLItems.Load()) }
