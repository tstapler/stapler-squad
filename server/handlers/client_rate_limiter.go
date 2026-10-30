package handlers

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	clientRequestsPerMinute = 300
	clientEventsPerMinute   = 6000
	// globalMetricEventsPerMinute caps cm.* events, which are metrics-only and cost an OTel record each.
	globalMetricEventsPerMinute = 30000
	maxTrackedClients           = 2048
	overflowClientKey           = "overflow"
)

type clientBucket struct {
	requests, events int
	resetAt          time.Time
}

// perClientLimiter is a fixed-window limiter keyed by remote address, in front of the global
// bucket so one flooding client cannot blank telemetry for everyone. The key is the TCP peer
// address only: X-Forwarded-For is attacker-controlled and never read here. Behind a reverse
// proxy every client shares the proxy's bucket, which only makes the limit stricter.
type perClientLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientBucket
	now     func() time.Time
}

func newPerClientLimiter() *perClientLimiter {
	return &perClientLimiter{clients: map[string]*clientBucket{}, now: time.Now}
}

// clientKey returns the peer IP (no port) for r.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return overflowClientKey
	}
	return host
}

// live returns b, reset when its window has elapsed.
func live(b *clientBucket, now time.Time) *clientBucket {
	if now.After(b.resetAt) {
		*b = clientBucket{resetAt: now.Add(time.Minute)}
	}
	return b
}

// purgeExpired drops buckets whose window has elapsed.
func (l *perClientLimiter) purgeExpired(now time.Time) {
	for k, b := range l.clients {
		if now.After(b.resetAt) {
			delete(l.clients, k)
		}
	}
}

// bucket returns key's live bucket. Callers hold l.mu. The map is bounded: when full, expired
// entries are purged and, if none free up, new keys share the overflow bucket.
func (l *perClientLimiter) bucket(key string) *clientBucket {
	now := l.now()
	if b, ok := l.clients[key]; ok {
		return live(b, now)
	}
	if len(l.clients) >= maxTrackedClients {
		l.purgeExpired(now)
	}
	if len(l.clients) >= maxTrackedClients {
		key = overflowClientKey
		if b, ok := l.clients[key]; ok {
			return live(b, now)
		}
	}
	b := &clientBucket{resetAt: now.Add(time.Minute)}
	l.clients[key] = b
	return b
}

func (l *perClientLimiter) allowRequest(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucket(key)
	if b.requests >= clientRequestsPerMinute {
		return false
	}
	b.requests++
	return true
}

func (l *perClientLimiter) allowEvents(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucket(key)
	if b.events+n > clientEventsPerMinute {
		return false
	}
	b.events += n
	return true
}
