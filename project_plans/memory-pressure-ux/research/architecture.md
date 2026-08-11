# Architecture Research: memory-pressure-ux

## Where Does the Memory Reader Fit?

### Option A: `session/procinfo/` (Recommended)

The existing `session/procinfo/` package already wraps gopsutil for process inspection.
Adding `memreader.go` there follows established conventions:

```
session/procinfo/
  inspector.go          (darwin build tag)
  inspector_other.go    (!darwin build tag)
  memreader.go          (NEW — cross-platform, no build tag needed)
```

**Pro**: Co-located with existing process inspection code. Same package = direct import
without circular deps. No new package surface area.
**Con**: Package name `procinfo` doesn't signal "system memory" — minor naming friction.

### Option B: `session/memory/` (Clean separation)

A new package dedicated to memory measurement:

```
session/memory/
  reader.go           (interface + cross-platform gopsutil impl)
  reader_linux.go     (direct /proc reads for performance)
  reader_test.go      (mock-injectable interface)
```

**Pro**: Clear domain separation. Testable via interface injection.
**Con**: Adds a new package; the project already has `procinfo` with similar scope.

### Decision: `session/memory/` is cleaner for testability

The sweeper must accept a `MemoryReader` interface for testing (see Pitfalls). A standalone
`session/memory/` package with a `Reader` interface is the right call:

```go
package memory

type SystemStats struct {
    UsedPct float64
}

type ProcessStats struct {
    RSSMB int64
}

type Reader interface {
    SystemMemory(ctx context.Context) (SystemStats, error)
    ProcessMemory(ctx context.Context, pid int32) (ProcessStats, error)
}
```

The `HibernationSweeper` gets a `memory.Reader` injected, defaulting to the real
gopsutil implementation. Tests inject a mock.

## Memory Method on Instance vs. Standalone Function

**Standalone function keyed by session name** is strongly preferred:

```go
// session/memory package
func ReadSessionRSS(ctx context.Context, r Reader, inst *session.Instance) (int64, error)
```

Reasons:
1. `Instance` is already a large struct with many responsibilities. Adding memory state
   to it creates more coupling.
2. The cache (30s TTL) should live at the sweeper level, not on each Instance, to
   centralize invalidation logic.
3. The `Instance` struct already has `stateMutex` — adding memory caching there risks
   lock contention.

**Cache design**: A map in the sweeper or a separate cache struct:

```go
type sessionMemoryCache struct {
    mu      sync.Mutex
    entries map[string]memoryCacheEntry // keyed by session UUID
}

type memoryCacheEntry struct {
    rssMB     int64
    fetchedAt time.Time
}

const cacheTTL = 30 * time.Second
```

Cache lives as a field on `HibernationSweeper`. Invalidation on hibernation: the sweeper
calls `cache.Invalidate(inst.UUID)` immediately after `inst.Hibernate()` succeeds.

## Proto Extension Strategy

### Option A: Extend existing `Session` message (Recommended)

Add optional fields to `types.proto`:
```proto
// Per-session memory footprint in MB. Zero when hibernated or unknown.
int32 memory_rss_mb = 52;
// Estimated RAM freed if this session is hibernated. Equals memory_rss_mb for Active sessions.
int32 estimated_savings_mb = 53;
```

And to `ListSessionsResponse` in `session.proto`:
```proto
// Current system memory usage percentage (0–100). Populated by server at list time.
float system_memory_pct = 2;
```

**Pro**: Zero breaking changes (optional fields, existing callers see zero values, NFR-4).
**Pro**: Piggybacks on existing 5-second event push cycle (FR-3.2) without new RPC.
**Pro**: TypeScript types auto-generated from proto; `session.memoryRssMb` works immediately.
**Con**: `ListSessionsResponse` grows to include system-wide state — minor conceptual mismatch.

### Option B: Separate `GetSystemMemory` RPC

```proto
rpc GetSystemMemory(GetSystemMemoryRequest) returns (GetSystemMemoryResponse) {}
message GetSystemMemoryResponse {
    float system_memory_pct = 1;
    bool above_threshold = 2;
}
```

**Pro**: Clean separation of concerns.
**Con**: Extra round-trip, extra polling loop needed in frontend. Contradicts FR-3.2.

**Decision**: Option A. Extend `ListSessionsResponse` with `system_memory_pct` and
`Session` with `memory_rss_mb` + `estimated_savings_mb`.

## Data Flow: Backend → Frontend

```
HibernationSweeper (5-min tick)
  └── reads system memory + per-session RSS
  └── stores in sessionMemoryCache

SessionService.ListSessions / WatchSessions
  └── populates Session.memory_rss_mb from cache
  └── populates ListSessionsResponse.system_memory_pct

WebSocket event stream
  └── SessionEvent carries updated Session with memory fields

Redux store (useSessionService.ts)
  └── upsertSession() updates session in store

SessionList.tsx
  └── reads session.memoryRssMb, passes to SessionCard/SessionRow

SessionCard/SessionRow
  └── displays "N MB" badge adjacent to status

SessionList / Layout header
  └── reads systemMemoryPct from ListSessionsResponse
  └── shows "Memory: N%" badge when above threshold

MemoryPressureCallout (new component)
  └── subscribes to systemMemoryPct
  └── ranks sessions by estimatedSavingsMb descending
  └── shows top-3 with "Hibernate all recommended" action
```

### Where to hold `systemMemoryPct` in React

The `useSessionService` hook returns the sessions array and connection state. It should
also return `systemMemoryPct: number` (0 when below threshold or unknown).

Propagation path:
1. `useSessionService.ts` → extracts `systemMemoryPct` from `ListSessionsResponse`
2. Exposed in `UseSessionServiceReturn` interface
3. Consumed by `SessionList.tsx` and passed down, OR stored in a new
   `SystemMemoryContext` for global access (simpler for the header badge).

**Recommendation**: A lightweight `SystemMemoryContext` exposing `{systemMemoryPct: number, threshold: number}`
lets both the header badge (FR-5) and the pressure callout (FR-6) subscribe without prop drilling.

## Toast / Callout: Existing Notification System

The project has `NotificationContext.tsx` with:
- `addNotification(notification)` — shows a toast + adds to history
- `acknowledgeNotification(id | id[])` — dismisses toast + marks read
- `NotificationToast.tsx` — renders individual toasts

This is appropriate for the pressure callout. However, FR-6 requires:
- Listing up to 3 specific sessions with savings estimates
- A "Hibernate all recommended" bulk button
- Per-session dismissal that persists for the browser session

The existing `NotificationToast` is for generic text notifications. A **custom
`MemoryPressureCallout` component** is better: it receives the top-3 sessions as props,
renders a structured list, and manages its own dismissed-set in component state
(`Set<string>` of session IDs stored in `useState`).

The callout should be rendered at the `SessionList` level (not global), so it can directly
access the sorted sessions and trigger their `onHibernate` callbacks.

## CSS: `statusWarning` Token

The theme contract (`web-app/src/styles/theme-contract.css.ts`) defines:
```ts
color: {
    warning: null,       // amber/orange text and border
    warningBg: null,     // amber background
    warningText: null,   // amber text variant
}
```

There is **no `statusWarning` token** — the requirements reference it but the actual
token names are `vars.color.warning`, `vars.color.warningBg`, `vars.color.warningText`.

For the memory pressure badge (FR-5): use `vars.color.warning` as border/text color
and `vars.color.warningBg` as background.

For the amber left-border highlight (FR-6.5): `SessionCard.css.ts` already has a pattern
at line 150: `background: vars.color.warningBg, borderLeft: '3px solid ${vars.color.warning}'`.
This exact pattern should be used for the pressure-highlight recipe variant on `card`.

## vanilla-extract Pattern for Pressure Highlight

```ts
// SessionCard.css.ts addition
export const cardPressureHighlight = style({
  borderLeft: `3px solid ${vars.color.warning}`,
  background: vars.color.warningBg,
});
```

Applied conditionally in `SessionCard.tsx`:
```tsx
<div className={clsx(card(), session.isHibernateCandidate && cardPressureHighlight)}>
```

Or as a recipe variant on `card`:
```ts
export const card = recipe({
  variants: {
    pressureHighlight: { true: { borderLeft: `3px solid ${vars.color.warning}` } }
  }
});
```

The recipe variant approach is preferred (aligns with ADR-009 vanilla-extract rules).

## Header Badge Placement

The application header component is likely in `web-app/src/app/layout.tsx` or a
`Header.tsx` component. The memory badge (FR-5) should be a simple `NavBadge`-style
component:

```tsx
{systemMemoryPct >= threshold && (
  <span className={memoryBadge}>Memory: {Math.round(systemMemoryPct)}%</span>
)}
```

Style: `background: vars.color.warningBg`, `color: vars.color.warning`.
The existing `NavBadge.tsx` in `web-app/src/components/ui/` is a template to follow.
