package session

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Metric names of the PR tracking poller (observability plan, T3.3.3).
const (
	MetricPRPollTotal          = "ssq_pr_poll_total"
	MetricPRPollFailuresTotal  = "ssq_pr_poll_failures_total"
	MetricSessionPRCount       = "ssq_session_pr_count"
	MetricPRPinTotal           = "ssq_pr_pin_total"
	MetricPRDiscoveryListCalls = "ssq_pr_discovery_list_calls_total"
	MetricPRUnverifiedTerminal = "ssq_pr_unverified_terminal_rows"
)

// prPollMetrics keeps an in-process tally next to each OpenTelemetry instrument so
// tests can read counts without an exporter.
type prPollMetrics struct {
	polls         *counterByLabel
	failures      *counterByLabel
	sessionCounts *counterByLabel
	pins          *counterByLabel
	discoveryList atomic.Int64
	unverified    atomic.Int64

	pollOtel, failOtel, discoveryOtel, pinOtel metric.Int64Counter
	sessionPRCount                             metric.Int64Histogram
	unverifiedGauge                            metric.Int64Gauge
}

var prMetrics = newPRPollMetrics()

func newPRPollMetrics() *prPollMetrics {
	m := &prPollMetrics{polls: newCounterByLabel(), failures: newCounterByLabel(), sessionCounts: newCounterByLabel(), pins: newCounterByLabel()}
	meter := telemetry.GetMeter()
	m.pollOtel, _ = meter.Int64Counter(MetricPRPollTotal)
	m.failOtel, _ = meter.Int64Counter(MetricPRPollFailuresTotal)
	m.discoveryOtel, _ = meter.Int64Counter(MetricPRDiscoveryListCalls)
	m.pinOtel, _ = meter.Int64Counter(MetricPRPinTotal)
	m.sessionPRCount, _ = meter.Int64Histogram(MetricSessionPRCount)
	m.unverifiedGauge, _ = meter.Int64Gauge(MetricPRUnverifiedTerminal)
	return m
}

func (m *prPollMetrics) poll(state domain.PRState) {
	m.polls.Add(string(state))
	if m.pollOtel != nil {
		m.pollOtel.Add(context.Background(), 1, metric.WithAttributes(attribute.String("state", string(state))))
	}
}

func (m *prPollMetrics) failure(reason string) {
	m.failures.Add(reason)
	if m.failOtel != nil {
		m.failOtel.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
}

func (m *prPollMetrics) discoveryListCall() {
	m.discoveryList.Add(1)
	if m.discoveryOtel != nil {
		m.discoveryOtel.Add(context.Background(), 1)
	}
}

func (m *prPollMetrics) observeSessionPRCount(n int) {
	m.sessionCounts.Add(strconv.Itoa(n))
	if m.sessionPRCount != nil {
		m.sessionPRCount.Record(context.Background(), int64(n))
	}
}

// pin counts one user pin or unpin of a session PR.
func (m *prPollMetrics) pin(pinned bool) {
	action := "unpin"
	if pinned {
		action = "pin"
	}
	m.pins.Add(action)
	if m.pinOtel != nil {
		m.pinOtel.Add(context.Background(), 1, metric.WithAttributes(attribute.String("action", action)))
	}
}

func (m *prPollMetrics) setUnverified(n int) {
	m.unverified.Store(int64(n))
	if m.unverifiedGauge != nil {
		m.unverifiedGauge.Record(context.Background(), int64(n))
	}
}

// PRPollCount is ssq_pr_poll_total{state} in this process.
func PRPollCount(state domain.PRState) int64 { return prMetrics.polls.Get(string(state)) }

// PRPollFailureCount is ssq_pr_poll_failures_total{reason} in this process.
func PRPollFailureCount(reason string) int64 { return prMetrics.failures.Get(reason) }

// counterByLabel is a tiny labelled in-process counter.
type counterByLabel struct {
	mu sync.Mutex
	m  map[string]int64
}

func newCounterByLabel() *counterByLabel { return &counterByLabel{m: map[string]int64{}} }

// Add increments label; exported so the metrics files outside the Instance package can use it.
func (c *counterByLabel) Add(label string) {
	c.mu.Lock()
	c.m[label]++
	c.mu.Unlock()
}

// Get returns label's count.
func (c *counterByLabel) Get(label string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[label]
}

// SessionPRCountObservations is how many times ssq_session_pr_count observed n in this process.
func SessionPRCountObservations(n int) int64 { return prMetrics.sessionCounts.Get(strconv.Itoa(n)) }

// PRPinCount is ssq_pr_pin_total{action} ("pin" or "unpin") in this process.
func PRPinCount(action string) int64 { return prMetrics.pins.Get(action) }

// Recorders the prpoller package calls; the counters stay here because the session
// PR-set editors feed the same registry.
func RecordPRPoll(state domain.PRState) { prMetrics.poll(state) }
func RecordPRPollFailure(reason string) { prMetrics.failure(reason) }
func RecordPRDiscoveryListCall()        { prMetrics.discoveryListCall() }
func SetPRUnverified(n int)             { prMetrics.setUnverified(n) }
