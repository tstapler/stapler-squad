# Analytics System Requirements

## Project Summary

Add a full analytics platform to stapler-squad covering user behavior (usage intelligence) AND system performance. The platform uses a thin, pluggable `AnalyticsProvider` adapter interface (modeled after OpenFeature's Provider contract) layered over the existing EventBus and a new SQLite-backed store. ESLint rules enforce analytics coverage at four callsite categories.

## Current State (Baseline)

Already in place:
- **OpenTelemetry** backend tracing (otelhttp + otelconnect interceptors) — disabled by default
- **EventBus** (`pkg/events/`) — pub/sub for session lifecycle events
- **Frontend `track()`** (`web-app/src/lib/telemetry.ts`) — fires `POST /api/telemetry`, currently only logged (not persisted)
- **RPC timing** (`web-app/src/lib/telemetry/rpcTiming.ts`) — ConnectRPC interceptor, writes Performance API marks only
- **Web Vitals** (`web-app/src/components/telemetry/WebVitalsReporter.tsx`) — reports via `useReportWebVitals`
- **ErrorEvent** in SQLite ent ORM — deduped, fingerprinted error storage
- **Telemetry HTTP handler** (`server/handlers/telemetry_handler.go`) — rate-limited, logs only

## Functional Requirements

### FR-1: AnalyticsProvider Interface (pluggable adapter layer)

- Define `AnalyticsProvider` interface in `web-app/src/lib/analytics/` with `track(event: AnalyticsEvent): void` and optional `flush(): Promise<void>`
- Ship two concrete providers:
  - `HttpAnalyticsProvider` — posts to `/api/analytics` (replaces current `/api/telemetry` fire-and-forget)
  - `ConsoleAnalyticsProvider` — dev-mode logging
- `AnalyticsContext` (React context) exposes `useAnalytics()` hook returning the active provider
- Provider can be swapped at runtime for testing (mirrors OpenFeature's `OpenFeature.setProvider()` pattern)
- Backend: `AnalyticsProvider` Go interface in `server/analytics/` with `Record(ctx, event)` method; ship `SQLiteAnalyticsProvider` and `LogAnalyticsProvider`

### FR-2: Backend Analytics Storage (SQLite via ent ORM)

- New `AnalyticsEvent` ent entity with fields:
  - `id` (UUID)
  - `event_name` (string, indexed)
  - `event_category` (enum: `user_action`, `performance`, `navigation`, `rpc`)
  - `session_id` (optional string, indexed)
  - `duration_ms` (optional int64)
  - `page` (optional string)
  - `component` (optional string)
  - `labels` (JSON map)
  - `created_at` (time, indexed)
- New `POST /api/analytics` endpoint that persists events via `SQLiteAnalyticsProvider`
- New `GET /api/analytics/summary` endpoint returning: event counts by name/category over configurable time window
- Retention policy: max 90 days / 100k events (configurable via config.json)
- Upgrade existing `/api/telemetry` handler to forward to the new provider (backward compat)

### FR-3: ESLint Analytics Enforcement

Four rules, all in `web-app/eslint-plugin-analytics/` as a local ESLint plugin:

| Rule | Target | Enforcement |
|------|--------|-------------|
| `analytics/require-on-click` | JSX `onClick` props on `<button>`, `<a>`, `[role=button]` | Must call `useAnalytics().track(...)` or have `// analytics-exempt` comment |
| `analytics/require-omnibar-dispatch` | Every `case` in `dispatchOmnibarAction` switch | Must contain a `track(...)` call or exempt comment |
| `analytics/require-page-analytics` | Top-level page/route components (files in `app/**/page.tsx`) | Must call `usePageView()` hook (or `useAnalytics().track('page_view', ...)`) |
| `analytics/require-rpc-analytics` | Every call to hooks from `useSessionService` | Must be accompanied by a `track(...)` call in the same component scope, or exempt |

All rules support `// analytics-exempt: <reason>` inline comment to silence.

### FR-4: Performance Analytics

- Upgrade `WebVitalsReporter` to send CWV to new `/api/analytics` endpoint (not just Performance marks)
- Upgrade `rpcTiming.ts` interceptor to also POST RPC latency to `/api/analytics` with `event_category: "rpc"`
- Backend: aggregate RPC p50/p95/p99 latency in the summary endpoint

### FR-5: Analytics Summary API

`GET /api/analytics/summary?from=<ISO>&to=<ISO>&category=<user_action|performance|rpc|navigation>`

Returns:
```json
{
  "period": { "from": "...", "to": "..." },
  "top_events": [{ "event_name": "...", "count": N, "avg_duration_ms": N }],
  "rpc_latency": [{ "method": "...", "p50": N, "p95": N, "p99": N }],
  "page_views": [{ "page": "...", "count": N }],
  "total_events": N
}
```

## Non-Functional Requirements

- **Self-contained**: no external services required; works offline
- **Privacy**: no PII in events; session IDs are opaque identifiers only
- **Performance**: `track()` calls must be fire-and-forget (non-blocking); backend endpoint must respond < 50ms p99
- **Zero-regression**: existing `/api/telemetry` endpoint must continue to work unchanged
- **Pluggable**: swapping providers (e.g., from SQLite to PostHog) requires only changing the registered provider, not callsites

## Out of Scope

- Real-time analytics dashboard UI (can be added in a follow-up)
- User identity tracking
- A/B testing
- External analytics service integration (PostHog, Mixpanel, etc.) — adapter layer makes this possible later
- Alerting on analytics thresholds

## Acceptance Criteria

1. `POST /api/analytics` stores events in SQLite; `GET /api/analytics/summary` returns correct aggregates
2. ESLint rules block PRs that add `onClick` handlers, omnibar dispatch cases, page components, or RPC hook calls without analytics
3. `useAnalytics()` hook returns the active provider; swapping provider in tests requires one line
4. Web Vitals and RPC latency data appear in summary endpoint response
5. All four ESLint rules have unit tests
6. `make quick-check` passes with new rules enabled
