# ADR-003: Frontend Observability Approach

**Status**: Accepted
**Date**: 2026-04-16
**Deciders**: Tyler Stapler

---

## Context

The Go backend already ships OpenTelemetry instrumentation (`OTEL_ENABLED=true`) covering HTTP requests, ConnectRPC endpoints, history cache, and search. The frontend has no observability at all. The user reports "perceived slowness on click" but there is no instrumentation to determine whether the bottleneck is React rendering, RPC latency, terminal streaming, or something else.

Two approaches were evaluated:

- **Option A (Custom JSON endpoint)**: `POST /api/telemetry` with a JSON body `{event, duration_ms, session_id, timestamp, labels}`. Backend logs the event as structured JSON. No new npm package. Ships in ~2 days.
- **Option B (OTel JS SDK)**: Install `@opentelemetry/sdk-web` + `@opentelemetry/exporter-trace-otlp-http`. Auto-instrumentation covers fetch, navigation, and user interactions. Exports to the same OTLP backend the Go server uses, enabling correlated frontend+backend traces. Ships in ~1 week.

The OTel JS SDK bundle cost was verified at approximately 60 KB gzipped (confirmed via signoz.io research). Dynamic import is required to avoid blocking first paint. React 19 strict mode double-initialization is a known risk requiring a module-level singleton guard.

---

## Decision

**Phase 1: Custom JSON endpoint.**

Implement `POST /api/telemetry` in the Go HTTP router (not as a ConnectRPC endpoint — plain JSON to keep the client simple). The backend handler logs events as structured JSON via the existing `slog`/`zap` logger. No database storage, no forwarding to OTLP in Phase 1.

Instrument these four events in Phase 1:
1. `session_attach` — time from click-to-first-terminal-output (already measured in `TerminalOutput.tsx` via `metricsRef`, just needs to POST the result)
2. `rpc_round_trip` — time for `StreamTerminal` first byte after connect
3. `page_navigation` — time from route change to interactive
4. `rpc_list_sessions` — latency of the most frequent background poll

Client-side implementation: a single `telemetry.ts` module with a `track(event: string, durationMs: number, labels?: Record<string, string>)` function. Calls are fire-and-forget (`fetch` with no `await` on the result). Sampled at 100% (single user).

**Phase 2** (separate PR, if needed): Migrate to `@opentelemetry/sdk-web` loaded via dynamic import when trace correlation with backend OTel spans is needed. The custom endpoint becomes a fallback.

---

## Rationale

The primary goal is to answer "which click is slow?" for a single developer using the app. A custom JSON endpoint achieves this in 2 days. The OTel SDK achieves the same plus backend correlation, but requires 5-7 days of setup and carries bundle risk (60 KB gzipped is non-trivial for a web terminal where first-paint speed matters).

`TerminalOutput.tsx` already measures all four of the key metrics in `metricsRef` and logs them to `console.log`. Phase 1 is literally wiring those existing measurements to an HTTP POST rather than a console statement.

The single-user context eliminates sampling concerns. 100% capture rate is correct here.

---

## Consequences

**Positive:**
- Zero npm dependencies added.
- Ships in 2 days.
- Actionable data immediately: slow attaches, slow RPCs, slow navigation events are all captured.
- No bundle size impact.
- No CORS configuration needed (same origin).

**Negative / Accepted costs:**
- Phase 1 events are not correlated with backend OTel traces (separate log entries).
- No visualization beyond `grep`/`jq` on backend logs in Phase 1.
- When Phase 2 (OTel SDK) ships, the instrumentation points must be migrated to use OTel spans.

---

## Alternatives Not Chosen

**Sentry**: Rejected. External SaaS, data leaves the machine, and the cost/benefit doesn't make sense for a single-user tool. Session replay and error aggregation are not needed.

**OTel SDK in Phase 1**: Rejected due to bundle size risk (60 KB gzipped verified), React 19 strict-mode initialization complexity, and the need for a running OTLP collector. These are solvable but add 1 week to a feature that can ship in 2 days with Option A.
