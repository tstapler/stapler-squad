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
// giving up with ErrPoolSaturated — independent of, and much shorter than, any
// caller's own overall call budget (e.g. TriggerTriage's triageCallBudget,
// server/services/backlog_service_triage.go — 30 minutes when this was
// written, since raised to 3 hours; either way this is a small fraction of
// it). Before this
// existed, a call whose ctx already carried a long fixed deadline could
// silently burn that entire budget just waiting for a slot — indistinguishable
// from a genuine hung LLM call once ctx expired. Confirmed live
// (docs/tasks/backlog-feature-improvement.md, 2026-09-08) to reliably park
// bulk-imported backlog items whose triage calls all lost the race for the
// pool's 5 concurrent slots at once. 2 minutes is generous relative to how
// fast slots actually turn over — a concurrent call finishes well inside that
// window in the normal case — while still being a small fraction of any real
// caller's call budget, so a genuinely saturated pool fails fast instead of
// stalling every queued caller for the length of its longest budget.
//
// A var, not a const, so tests can shrink it rather than actually waiting out
// the real 2-minute window (mirrors remediationBackoffSchedule/
// MaxRemediationAttempts in session/backlog_remediation.go).
var maxQueueWait = 2 * time.Minute

// idleTimeout bounds how long a first call (--output-format stream-json) may go
// with no new output line before it's considered stalled and killed with
// ErrIdleTimeout — real progress detection, replacing "wait up to the caller's
// full budget and hope," which a 2026-09-08 incident showed cuts both ways: a
// hung call wasted its entire budget before being caught, while a genuinely
// long-but-active call (BUG-055, session/backlog_lifecycle_triage.go) could be
// cut off mid-work at the same fixed ceiling. This is now the PRIMARY defense
// against a hung call; the caller's own ctx deadline (e.g. triageCallBudget,
// server/services/backlog_service_triage.go) becomes a much larger backstop
// against a call that never stops producing output, not the main protection.
// 10 minutes is generous enough to tolerate one legitimately slow tool call (a
// large test run, a slow web fetch) without false-killing active work, while
// still catching a true hang roughly 18x faster than the old 3-hour ceiling.
//
// A var, not a const, for the same test-injectability reason as maxQueueWait.
var idleTimeout = 10 * time.Minute

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
