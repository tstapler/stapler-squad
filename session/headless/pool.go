package headless

import (
	"sync"
	"time"
)

// FeatureKey is a named type for feature identifiers.
// Using a named type (not an alias) prevents accidental string injection at call sites.
type FeatureKey string

// sessionState holds per-feature-key LLM session tracking.
type sessionState struct {
	sessionID         string
	callCount         int
	consecutiveErrors int
}

// PoolConfig configures a Pool.
type PoolConfig struct {
	// MaxCallsPerSession is the maximum number of calls before a session is rotated.
	// Defaults to 25 if zero.
	MaxCallsPerSession int

	// MaxConcurrentSessions is the maximum number of concurrent subprocess calls.
	// Defaults to 5 if zero.
	MaxConcurrentSessions int

	// DefaultModel overrides the claude model used when no model is specified per-call.
	DefaultModel string
}

// Pool manages a map of named LLM feature sessions, providing session reuse
// for prefix-cache optimization and bounded concurrency.
type Pool struct {
	claudeBin string
	cfg       PoolConfig
	runner    ClaudeRunner

	// mu protects the sessions map and keyMu map.
	mu       sync.Mutex
	sessions map[FeatureKey]*sessionState
	keyMu    map[FeatureKey]*sync.Mutex

	// concurrencySem limits max simultaneous subprocess calls.
	concurrencySem chan struct{}
}

// defaultPoolMu protects the package-level default pool variable.
var defaultPoolMu sync.RWMutex

// defaultPool is the package-level pool used by feature functions.
var defaultPool *Pool

// DefaultPool returns the package-level default pool.
// Returns nil if SetDefaultPool has not been called.
func DefaultPool() *Pool {
	defaultPoolMu.RLock()
	defer defaultPoolMu.RUnlock()
	return defaultPool
}

// SetDefaultPool sets the package-level default pool.
// Safe to call concurrently.
func SetDefaultPool(p *Pool) {
	defaultPoolMu.Lock()
	defer defaultPoolMu.Unlock()
	defaultPool = p
}

// maxConsecutiveErrors is the circuit-breaker threshold. When consecutiveErrors
// reaches this count, the session is rotated before the next call.
const maxConsecutiveErrors = 3

// defaultMaxCalls is the fallback when PoolConfig.MaxCallsPerSession is zero.
const defaultMaxCalls = 25

// defaultMaxConcurrent is the fallback when PoolConfig.MaxConcurrentSessions is zero.
const defaultMaxConcurrent = 5

// maxQueueWait bounds how long call() waits for a concurrency-pool slot before
// giving up with ErrPoolSaturated — much shorter than any real caller's own
// call budget, so a saturated pool fails fast instead of silently burning a
// caller's entire budget just waiting for a slot (see BUG-093).
//
// A var, not a const, so tests can shrink it instead of waiting out the real
// 2-minute window (mirrors remediationBackoffSchedule in
// session/backlog_remediation.go).
var maxQueueWait = 2 * time.Minute

// idleTimeout bounds how long a first call (--output-format stream-json) may go
// with no new output line before it's considered stalled and killed with
// ErrIdleTimeout — the primary defense against a hung call. The caller's own
// ctx deadline is now just a backstop against a call that never stops
// producing output (see BUG-093).
//
// A var, not a const, for the same test-injectability reason as maxQueueWait.
var idleTimeout = 10 * time.Minute

// maxFirstCallOutputBytes caps the cumulative stream-json transcript
// readFirstCallStream (caller.go) accumulates for one call, independent of
// the per-line scanner buffer cap — so a long-running, non-idle stream can't
// grow this buffer unbounded before idleTimeout or ctx would otherwise catch it.
var maxFirstCallOutputBytes = 32 * 1024 * 1024

// acquireKeyMu returns (and lazily creates) the per-key mutex.
// Caller must hold p.mu.
func (p *Pool) acquireKeyMu(key FeatureKey) *sync.Mutex {
	mu, ok := p.keyMu[key]
	if !ok {
		mu = &sync.Mutex{}
		p.keyMu[key] = mu
	}
	return mu
}

// rotateSession resets the session state for key without cancelling any
// running subprocess (the subprocess is already done or errored when we rotate).
// Caller must NOT hold the per-key mutex (we acquire it here).
func (p *Pool) rotateSession(key FeatureKey) {
	p.mu.Lock()
	keyMu := p.acquireKeyMu(key)
	p.mu.Unlock()

	keyMu.Lock()
	defer keyMu.Unlock()
	p.mu.Lock()
	p.sessions[key] = &sessionState{}
	p.mu.Unlock()
}
