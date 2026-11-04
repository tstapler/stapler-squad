# Plan: e2e triage (Complexity 1)

Migration / Observability / Risk Control: N/A — complexity 1. Domain glossary: none (no new types). Tech debt disposition: Extend as-is (test-only edits).

## Epic 1 — Fix four failing e2e tests (test-side drift; no app changes)

### Story 1.1 — Analytics global view (AC 1, 2)
- Task 1.1a `tests/e2e/escape-analytics-global-view.spec.ts` `buildGlobalSummaryResponse`: also send `correlationOutcomes` (= totalSequences), `correlationCoverage: 1`, `captureHealthy: true`. Root cause: card renders `correlationOutcomes` (`EscapeAnalyticsPage.tsx:296`).
- Task 1.1b `tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts:43` `getErrorBanner`: exclude `[data-testid="announcer-assertive"]`. Root cause: permanent live region `Announcer.tsx:46`.

### Story 1.2 — Multi-window (AC 6)
- Task 1.2a `tests/e2e/multi-window.spec.ts` `nonToastAlerts`: exclude the announcer; update doc comment.

### Story 1.3 — Jules badge (AC 3, 4, 5)
- Task 1.3a `tests/e2e/jules-status-badge.spec.ts` "no badge renders before the first Jules state arrives": additionally hold the `WatchBacklogItems` WebSocket with `page.routeWebSocket`: an async handler that does `await gate; ws.connectToServer();` (Playwright keeps the page socket unopened until the handler resolves, so the transport's `await ws.onopen` blocks; confirm by running). It must gate every connection, count intercepted sockets as a sanity check before asserting no badge, call `releaseGate()` in a `finally`, and drop the vestigial `WatchBacklogItems` `page.route`. Root cause: the watch stream is a WebSocket, which `page.route` cannot gate.
- AC 3, 4: no code change — they pass on a freshly built binary (reported HTML-for-JSON comes from the SPA fallback when the seed route is unregistered: either a stale binary, or a server not started with `STAPLER_SQUAD_INSTANCE=e2e-local` per `server/server.go:1187`, e.g. a manual instance. Verification must go through `global-setup`). Verify by running.

### Story 1.4 — Verification (AC 7, 8)
- Task 1.4a Run the three specs on `chromium` and `chromium-dom` against the fresh binary; show output.
- Task 1.4b Keep assertions intact (diff review).

## Test mapping
Each AC is verified by its own Playwright test; see research/pitfalls.md for reproduction evidence.

## Added after adversarial review (CONCERNS, 0 blockers)
- Task 1.5a Update `tests/e2e/baseline/bun-expected-diff.json` (remove fixed entries) and the stale announcer notes in `build-coupled-specs.json`; check what consumes them first.
- Task 1.5b Sweep `tests/e2e` for unscoped `role=alert` locators (e.g. `pages/TriggersPage.ts:116`); fix only those that actually hit the announcer, verified by running.
- Task 1.5c Update the doc comment above `getErrorBanner` (`EscapeAnalyticsGlobalViewPage.ts:38-40`).
- Task 1.5d Optional: `seedJulesWorkSessionDirect` content-type check for a clearer error. Skipped unless cheap.
- Accepted tradeoff: excluding `announcer-assertive` no longer detects assertive SR announcements in multi-window; consistent with the existing toast exclusion.
- Pin: installed Playwright is 1.56.1 (not the 1.62.1 the spec header claims).
