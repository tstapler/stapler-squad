package handlers

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Budgets for POST /api/analytics. They are global on purpose: local browsers and same-box
// reverse proxies all arrive from a loopback RemoteAddr (server/middleware/hostguard.go), so a
// per-peer limit would only be a lower global limit that every tab on the machine shares.
//
// Sizing, from the client's behaviour: a tab flushes every 20s (about 3 requests/min) or at 100
// queued events, so 10 tabs send about 30 requests/min against the 1000/min request bucket. One
// RPC emits about 2.5 events, so 50 RPC/s across tabs is about 7,500 events/min; the 60,000
// event budget gives 8x headroom, and cm.* (about 1 per RPC) gets 30,000.
const (
	analyticsRequestsPerMinute = 1000
	analyticsEventsPerMinute   = 60000
	metricEventsPerMinute      = 30000

	scopeGlobalRequests = "global_requests"
	scopeGlobalEvents   = "global_events"
	scopeMetricBudget   = "cm_budget"
)

// minuteBudget is a fixed-window counter.
type minuteBudget struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
	limit   int
}

// take charges n against the window and reports whether it fit; a refused take charges nothing.
func (b *minuteBudget) take(n int) bool {
	if n <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.After(b.resetAt) {
		b.count, b.resetAt = 0, now.Add(time.Minute)
	}
	if b.count+n > b.limit {
		return false
	}
	b.count += n
	return true
}

var rateLimitedCounter = sync.OnceValue(func() metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter("client.metrics.rate_limited",
		metric.WithDescription("POST /api/analytics batches refused with 429, by budget scope"))
	if err != nil {
		log.Error("handlers: failed to register client.metrics.rate_limited", "error", err)
		return nil
	}
	return c
})

var lastRateLimitLog atomic.Int64 // unix seconds

// noteRateLimited counts a refusal and logs at most once a minute (the browser swallows 429s).
func noteRateLimited(scope string) {
	if c := rateLimitedCounter(); c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(attribute.String("scope", scope)))
	}
	now := time.Now().Unix()
	if last := lastRateLimitLog.Load(); now-last >= 60 && lastRateLimitLog.CompareAndSwap(last, now) {
		log.Warn("analytics: refusing client telemetry, budget exhausted", "scope", scope)
	}
}
