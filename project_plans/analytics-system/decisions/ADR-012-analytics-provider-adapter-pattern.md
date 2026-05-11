# ADR-012: Analytics Provider Adapter Pattern

## Status
Accepted

## Context

The analytics system requirements call for a pluggable, swappable analytics backend. The team evaluated three approaches:

1. Adopt PostHog self-hosted directly, tying callsites to the PostHog JS SDK (`posthog.capture()`).
2. Adopt Plausible's minimal event API directly, sending raw HTTP events to a Plausible instance.
3. Build a thin `AnalyticsProvider` adapter interface that concrete implementations hide behind.

The project is a local-first, self-contained tool with no external service dependencies (per the non-functional requirements: "no external services required; works offline"). Adopting a third-party SDK directly at callsites would couple all instrumented code to that vendor's API surface, making future provider changes require touching every `track()` callsite across the codebase.

The existing codebase already has a fire-and-forget `track()` call in `web-app/src/lib/telemetry.ts` that posts to `/api/telemetry`. The analytics system needs to preserve backward compatibility with that endpoint while introducing a richer, persisted storage layer underneath.

The OpenFeature project (CNCF) has solved the same structural problem for feature flags: a single `Provider` interface with `initialize()`, `onClose()`, and typed evaluation methods, allowing vendor backends to be swapped by calling `OpenFeature.setProvider(new MyProvider())` at the application root. This design has proven stable across a large ecosystem of provider implementations.

## Decision

Implement a thin `AnalyticsProvider` interface modeled after the OpenFeature Provider contract. The interface lives at `web-app/src/lib/analytics/types.ts` (TypeScript) and `server/analytics/provider.go` (Go), with the following structure:

**TypeScript interface:**

```typescript
export interface AnalyticsProvider {
  readonly metadata: { readonly name: string };
  initialize?(): Promise<void>;    // idempotent; called by AnalyticsContext on mount
  onClose?(): Promise<void>;       // flush queue; called on unmount / page unload
  track(event: AnalyticsEvent): void;  // fire-and-forget at all callsites
  flush?(): Promise<void>;         // drain buffered events; awaited on shutdown
}
```

**Go interface:**

```go
type AnalyticsProvider interface {
    Name() string
    Record(ctx context.Context, event AnalyticsEvent) error
    Flush(ctx context.Context) error
    Close() error
}
```

Two concrete providers ship at launch:
- `HttpAnalyticsProvider` (frontend) — batches events (up to 25 events or 2-second timer) and POSTs to `/api/analytics`; uses `navigator.sendBeacon()` on page unload
- `ConsoleAnalyticsProvider` (frontend, dev mode) — `console.debug("[analytics]", event)`, synchronous, no batching
- `SQLiteAnalyticsProvider` (backend) — persists to `analytics.db` via ent ORM
- `LogAnalyticsProvider` (backend) — structured log output, used in tests and when analytics DB is unavailable

Provider registration mirrors `OpenFeature.setProvider()`:

```typescript
// app root or test setup — one line to swap the entire backend
setAnalyticsProvider(new ConsoleAnalyticsProvider());
```

The `useAnalytics()` React hook returns the currently registered provider from `AnalyticsContext`. The context value is memoized via `useMemo` with a stable `track` wrapper held in a `useRef`, preventing re-render cascades when the provider is swapped.

## Alternatives Considered

**PostHog SDK (`posthog-js` / `posthog-node`) adopted directly**

PostHog provides a mature SDK with batching, retry, and a rich dashboard. However:
- Requires a running PostHog instance (self-hosted or cloud), violating the offline/no-external-services requirement
- Couples all 50+ future `track()` callsites to `posthog.capture()` — any migration away requires a mass callsite update
- The PostHog Node SDK is 200 kB+ and pulls in HTTP client dependencies that conflict with the project's minimal server footprint

The adapter pattern does not close the door on PostHog; a `PostHogAnalyticsProvider` implementing the interface can be added later without touching any instrumented code.

**Plausible Analytics API adopted directly**

Plausible's API (`POST /api/event`) is minimal and privacy-focused, matching the project's no-PII stance. However:
- Plausible is pageview-centric; its schema is a poor fit for RPC latency events and omnibar dispatch tracking
- Same coupling problem as PostHog — callsites would reference Plausible-specific field names
- Plausible has no self-hosted free tier for custom event retention queries

**Raw OpenTelemetry events (OTel SDK)**

The project already instruments backend HTTP handlers with `otelhttp`. Extending OTel to carry user-action events would unify the telemetry pipeline but:
- OTel events (`log.Emit`) are not designed for aggregation queries (p50/p95 RPC latency, top events by count)
- OTel's TypeScript SDK is 300+ kB before tree-shaking; adding it to the frontend bundle is disproportionate for this use case
- The `GET /api/analytics/summary` response shape requires SQL-style aggregation that OTel collectors do not provide without a backend like Jaeger or Tempo (more external services)

## Consequences

**Positive:**
- All instrumented callsites call `useAnalytics().track(event)` — a stable API that does not change when the backend provider changes
- Tests swap in `ConsoleAnalyticsProvider` or a mock with one line; no HTTP mocking required for unit tests of instrumented components
- Adding a PostHog, Plausible, or OTel provider in the future requires writing one class, not touching 50+ callsites
- The interface's `flush()` method enables reliable event delivery on page unload via the `pagehide` / `visibilitychange` listener pattern (borrowed from PostHog SDK design)
- `metadata.name` on every provider enables logging to identify which backend is active without code inspection

**Negative / Trade-offs:**
- An extra abstraction layer means debugging a missing event requires checking both the callsite and the provider implementation
- The `HttpAnalyticsProvider`'s internal batch queue means events are not immediately persisted — a browser crash within the 2-second flush window can drop events (acceptable for a local analytics system; not acceptable for billing or security audit logs)
- Provider swapping at runtime (after `initialize()`) is possible but requires calling `onClose()` on the old provider first; the `AnalyticsContext` implementation must enforce this ordering or events can be lost

**Pitfall mitigations:**
- The context value is memoized; `track()` is wrapped in a stable `useRef` function to prevent React re-render cascades (see pitfalls research, section 5)
- `track()` implementations must never throw — errors are caught and swallowed internally, preserving the fire-and-forget contract at callsites
