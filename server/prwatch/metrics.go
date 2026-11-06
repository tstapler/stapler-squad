package prwatch

import (
	"sort"
	"strings"
	"sync"
)

// Metric names. Squad has no Prometheus registry; /actuator/metrics is a flat
// JSON snapshot of in-process counters, so CounterSet follows that model.
const (
	MetricDropped                     = "prwatch_dropped_total"                         // label drop_reason
	MetricHeld                        = "prwatch_held_total"                            // label hold_reason
	MetricSteers                      = "prwatch_steers_total"                          // labels kind, actor_class
	MetricCatchupSteers               = "prwatch_catchup_steers_total"                  // not in latency or alt4_class metrics
	MetricTickOverrun                 = "prwatch_tick_overrun_total"                    // a tick skipped because the last one was still running
	MetricDuplicateSharedGuard        = "prwatch_duplicate_shared_guard_total"          // acked Duplicate via the shared guard signature
	MetricCISteerInsideFeedbackWindow = "prwatch_ci_steer_inside_feedback_window_total" // CI steer made inside a human-feedback window
)

// CounterSet is an in-memory Metrics implementation, safe for concurrent use.
type CounterSet struct {
	mu     sync.Mutex
	counts map[string]int64
}

var _ Metrics = (*CounterSet)(nil)

// NewCounterSet returns an empty CounterSet.
func NewCounterSet() *CounterSet {
	return &CounterSet{counts: map[string]int64{}}
}

// CounterKey renders name and labels as name{k=v,...} with sorted labels.
func CounterKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k + "=" + labels[k])
	}
	b.WriteByte('}')
	return b.String()
}

// Inc adds one to the counter for name and labels.
func (c *CounterSet) Inc(name string, labels map[string]string) {
	key := CounterKey(name, labels)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int64{}
	}
	c.counts[key]++
}

// Value returns the counter for exactly name and labels.
func (c *CounterSet) Value(name string, labels map[string]string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[CounterKey(name, labels)]
}

// Total sums every label combination of name.
func (c *CounterSet) Total(name string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	var n int64
	for k, v := range c.counts {
		if k == name || strings.HasPrefix(k, name+"{") {
			n += v
		}
	}
	return n
}

// Snapshot returns a copy of every counter keyed by CounterKey.
func (c *CounterSet) Snapshot() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int64, len(c.counts))
	for k, v := range c.counts {
		out[k] = v
	}
	return out
}
