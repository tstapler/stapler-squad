package services

// jules_usage_counter.go — Epic 4.1, Task 4.1.1a: the process-wide spend/
// activity counters the Observability Plan names
// (project_plans/google-jules-integration/implementation/plan.md's
// Observability Plan section) — jules.session.dispatched,
// jules.session.completed, jules.session.failed, jules.api.rate_limited,
// jules.api.error. JulesDispatchService and session.JulesSessionPoller each
// hold this behind a narrow, locally-declared interface (julesUsageRecorder
// in each of those files) rather than importing this concrete type, so
// SetUsageCounter is optional and every existing test call site that never
// wires one keeps working with all-nil, no-op increments — mirrors this
// codebase's existing SetPoller/SetChainReconciler post-construction wiring
// convention (e.g. CheckpointService.SetPoller).

import "sync/atomic"

// JulesUsageCounter tracks Jules dispatch/poll activity as plain atomic
// counters — one process-wide instance, constructed once in
// server/dependencies.go and shared by JulesDispatchService,
// session.JulesSessionPoller, and JulesConfigService (for GetJulesConfig's
// response). Mirrors session/tmux/fork_metrics.go's forkMonitor shape
// (atomic.Int64 fields + a value-struct Snapshot()).
type JulesUsageCounter struct {
	sessionDispatched atomic.Int64
	sessionCompleted  atomic.Int64
	sessionFailed     atomic.Int64
	apiRateLimited    atomic.Int64
	apiError          atomic.Int64
}

// NewJulesUsageCounter constructs a zeroed JulesUsageCounter.
func NewJulesUsageCounter() *JulesUsageCounter {
	return &JulesUsageCounter{}
}

// IncSessionDispatched increments jules.session.dispatched — called once per
// successfully dispatched Jules session (JulesDispatchService.DispatchToJules).
func (c *JulesUsageCounter) IncSessionDispatched() { c.sessionDispatched.Add(1) }

// IncSessionCompleted increments jules.session.completed — called once per
// Jules session the poller observes reach the COMPLETED state, with or
// without an opened pull request.
func (c *JulesUsageCounter) IncSessionCompleted() { c.sessionCompleted.Add(1) }

// IncSessionFailed increments jules.session.failed — called once per Jules
// session the poller ends as a failure (Jules-reported FAILED, a vanished
// session, a runtime timeout, or an abandoned pre-CreateSession reservation).
func (c *JulesUsageCounter) IncSessionFailed() { c.sessionFailed.Add(1) }

// IncAPIRateLimited increments jules.api.rate_limited — called on every Jules
// API call classified as a 429 (jules.ErrJulesRateLimited).
func (c *JulesUsageCounter) IncAPIRateLimited() { c.apiRateLimited.Add(1) }

// IncAPIError increments jules.api.error — called on every other failed
// Jules API call (auth, not-found, transient 5xx, or unclassified).
func (c *JulesUsageCounter) IncAPIError() { c.apiError.Add(1) }

// JulesUsageSnapshot is a point-in-time read of every counter, returned by
// value so GetJulesConfig can hand it straight to the proto mapper without
// holding a reference into JulesUsageCounter's internals.
type JulesUsageSnapshot struct {
	SessionDispatched int64
	SessionCompleted  int64
	SessionFailed     int64
	APIRateLimited    int64
	APIError          int64
}

// Snapshot reads every counter. Safe to call concurrently with any Inc*
// call — each field is read via its own atomic load, so a concurrent
// increment may or may not be reflected in a given snapshot, but never
// produces a torn/corrupt value.
func (c *JulesUsageCounter) Snapshot() JulesUsageSnapshot {
	return JulesUsageSnapshot{
		SessionDispatched: c.sessionDispatched.Load(),
		SessionCompleted:  c.sessionCompleted.Load(),
		SessionFailed:     c.sessionFailed.Load(),
		APIRateLimited:    c.apiRateLimited.Load(),
		APIError:          c.apiError.Load(),
	}
}
