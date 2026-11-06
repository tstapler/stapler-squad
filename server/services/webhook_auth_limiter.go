package services

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// Defaults for the authentication-failure limiter. A legitimate client never fails
// authentication, so the budget can be tight; it only has to bound CPU, database and log
// amplification from a prober, since a 256-bit token cannot be guessed at any rate.
const (
	defaultAuthFailuresPerWindow = 10
	defaultAuthFailureWindow     = time.Minute
	defaultAuthLimiterMaxKeys    = 4096

	// ipv6KeyPrefixBits groups an IPv6 client by its /64, the smallest allocation an ISP
	// hands out, so a prober cannot evade the limit by rotating addresses within its own /64.
	ipv6KeyPrefixBits = 64
)

// authFailureLimiter counts failed authentications per client and, once a client exhausts
// its budget inside a fixed window, reports it blocked until the window ends. It is consulted
// BEFORE the credential is checked: a blocked client is refused without any lookup, so a
// valid token presented during a block is refused too (otherwise the block would not stop
// the guesses it exists to stop). Successful requests neither consume nor refund budget:
// a refund on success would let an attacker sharing a NAT with a legitimate client reset
// their own count.
type authFailureLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	max     int
	window  time.Duration
	maxKeys int
	entries map[string]*failureWindow
}

type failureWindow struct {
	count int
	start time.Time
}

func newAuthFailureLimiter() *authFailureLimiter {
	return &authFailureLimiter{
		now:     time.Now,
		max:     defaultAuthFailuresPerWindow,
		window:  defaultAuthFailureWindow,
		maxKeys: defaultAuthLimiterMaxKeys,
		entries: make(map[string]*failureWindow),
	}
}

// blocked reports whether key is currently blocked and, if so, how long until it is not.
func (l *authFailureLimiter) blocked(key string) (retryAfter time.Duration, isBlocked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return 0, false
	}
	elapsed := l.now().Sub(e.start)
	if elapsed >= l.window || e.count < l.max {
		return 0, false
	}
	return l.window - elapsed, true
}

// recordFailure counts one failed authentication for key and reports whether this failure
// is the one that just exhausted the budget (so the caller can log the block exactly once).
func (l *authFailureLimiter) recordFailure(key string) (justBlocked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	e, ok := l.entries[key]
	if !ok {
		l.makeRoomLocked(now)
		e = &failureWindow{start: now}
		l.entries[key] = e
	}
	if now.Sub(e.start) >= l.window {
		e.count, e.start = 0, now
	}
	e.count++
	return e.count == l.max
}

// makeRoomLocked keeps the table bounded so address churn cannot grow memory without limit:
// it drops expired windows first, then, if still full, the oldest live one.
func (l *authFailureLimiter) makeRoomLocked(now time.Time) {
	if len(l.entries) < l.maxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range l.entries {
		if now.Sub(e.start) >= l.window {
			delete(l.entries, k)
			continue
		}
		if oldestKey == "" || e.start.Before(oldest) {
			oldestKey, oldest = k, e.start
		}
	}
	if len(l.entries) >= l.maxKeys && oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

// clientLimiterKey derives the limiter key from the connection's peer address only.
// X-Forwarded-For and similar headers are deliberately ignored: nothing in this deployment
// sits behind a trusted proxy, and honouring them would let a caller pick its own key.
func clientLimiterKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is6() {
		if prefix, err := addr.Prefix(ipv6KeyPrefixBits); err == nil {
			return prefix.String()
		}
	}
	return addr.String()
}
