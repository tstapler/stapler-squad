package auth

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Bounds for the dynamic-rpID-registration guard rails in webauthnForHost.
// Each candidate hostname webauthnForHost can't match statically triggers a
// real DNS lookup (hostnameValidator) -- without limits, a client could force
// unbounded lookups by sending many distinct Host headers (negativeCache) or
// hammering the same one (ipLimiter).
const (
	negativeCacheMaxSize = 256
	negativeCacheTTL     = 5 * time.Minute
	ipLimiterMaxSize     = 512
	ipLimiterRate        = rate.Limit(1) // sustained validations per second per source IP
	ipLimiterBurst       = 3
)

// negativeHostnameCache remembers hostnames that recently failed
// hostnameValidator so repeated attempts for the same bad hostname don't
// each re-trigger a DNS lookup. Mirrors GitignoreCache's bounded-TTL,
// evict-oldest-on-insert design (server/services/gitignore_cache.go).
type negativeHostnameCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // hostname -> failedAt
}

func newNegativeHostnameCache() *negativeHostnameCache {
	return &negativeHostnameCache{entries: make(map[string]time.Time)}
}

// IsNegative reports whether hostname failed validation within the last
// negativeCacheTTL.
func (c *negativeHostnameCache) IsNegative(hostname string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	failedAt, ok := c.entries[hostname]
	if !ok {
		return false
	}
	if time.Since(failedAt) > negativeCacheTTL {
		delete(c.entries, hostname)
		return false
	}
	return true
}

// MarkNegative records that hostname just failed validation.
func (c *negativeHostnameCache) MarkNegative(hostname string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[hostname]; !exists && len(c.entries) >= negativeCacheMaxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, t := range c.entries {
			if oldestKey == "" || t.Before(oldestTime) {
				oldestKey, oldestTime = k, t
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[hostname] = time.Now()
}

// sourceIPLimiter bounds how often each source IP may trigger the expensive
// hostnameValidator path. Keyed by an unbounded input (any client IP), so
// entries are capped and evicted like negativeHostnameCache rather than
// relying on an externally-driven Cleanup call.
type sourceIPLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
}

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

func newSourceIPLimiter() *sourceIPLimiter {
	return &sourceIPLimiter{limiters: make(map[string]*ipLimiterEntry)}
}

// Allow reports whether ip may proceed with another validation attempt.
func (l *sourceIPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.limiters[ip]
	if !ok {
		if len(l.limiters) >= ipLimiterMaxSize {
			l.evictOldestLocked()
		}
		entry = &ipLimiterEntry{limiter: rate.NewLimiter(ipLimiterRate, ipLimiterBurst)}
		l.limiters[ip] = entry
	}
	entry.lastUsed = time.Now()
	return entry.limiter.Allow()
}

// evictOldestLocked removes the least-recently-used entry. Caller must hold mu.
func (l *sourceIPLimiter) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for k, e := range l.limiters {
		if oldestKey == "" || e.lastUsed.Before(oldestTime) {
			oldestKey, oldestTime = k, e.lastUsed
		}
	}
	if oldestKey != "" {
		delete(l.limiters, oldestKey)
	}
}

// sourceIP extracts the client IP from a request, stripping the port if
// present. Mirrors isLocalhostRequest's RemoteAddr handling (handlers.go).
func sourceIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
