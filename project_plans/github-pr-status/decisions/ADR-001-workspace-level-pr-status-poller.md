# ADR-001: Workspace-Level PR Status Poller

**Status:** Accepted
**Date:** 2026-04-09
**Decision:** Use a single workspace-level shared ticker for PR status polling, not per-session goroutines.

## Context

PR status polling requires periodic GitHub API calls for each session that has an associated PR. Two viable patterns exist in the codebase:

1. **Per-session goroutines** -- Each session spawns its own goroutine with independent timing.
2. **Workspace-level shared ticker** -- A single goroutine iterates all sessions on a shared `time.Ticker`.

The `ReviewQueuePoller` (`session/review_queue_poller.go`) already uses pattern 2 successfully. It manages N sessions from a single goroutine with `SetInstances()`, `AddInstance()`, `RemoveInstance()` lifecycle methods.

## Decision

Follow the `ReviewQueuePoller` pattern exactly. Create `session/pr_status_poller.go` as a single workspace-level poller with:

- One goroutine per workspace
- Shared `time.Ticker` at 60-second default interval
- Sequential iteration of all monitored sessions per tick
- `context.WithCancel` for graceful shutdown
- `sync.RWMutex` for thread-safe instance list management

## Consequences

### Positive

- **Resource efficiency:** One goroutine + one ticker for N sessions, not N goroutines.
- **Coordinated timing:** All sessions polled at predictable intervals; no thundering herd.
- **Graceful shutdown:** Single cancel context propagates cleanly.
- **Proven pattern:** `ReviewQueuePoller` validates this approach across 900+ lines of production code.
- **Observable:** Poller holds live instance list; easy to inspect monitored count.
- **Semaphore control:** Easy to add max-concurrent-calls limiter in a single loop.

### Negative

- **Head-of-line blocking:** A slow `gh` call blocks subsequent sessions in that tick. Mitigated by per-call `context.WithTimeout(8s)`.
- **All-or-nothing pause:** Rate limit hits pause the entire poller, not individual sessions. This is actually desirable for rate limit compliance.

## Alternatives Considered

### Per-session goroutines

Rejected. N goroutines for N sessions means uncoordinated API calls, GC pressure from N timers, and no central rate limit enforcement. `ReviewQueuePoller` explicitly avoids this pattern.

### Event-driven (GitHub webhooks)

Rejected for Phase 1. Requires public URL or ngrok tunnel. `gh webhook forward` does not exist as a stable local tool. GitHub has no GraphQL subscriptions or SSE endpoints. Can revisit in Phase 2.

### Reactive on user view

Rejected. Fetching PR status only when the user opens the web UI adds latency and provides no proactive monitoring.
