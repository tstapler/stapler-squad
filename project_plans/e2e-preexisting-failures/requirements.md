# Requirements: e2e — Fix Pre-existing Test Failures (escape-analytics, jules-status-badge, multi-window)

## Item ID
`fb352a5d-f19a-4f67-a7df-95a9232769ef`

## Problem Statement

Three e2e spec files contain failing tests that reproduce on both the Next and Bun builds. These are not flaky tests — each has a deterministic root cause verified in source code. They block clean CI signal for those specs.

## Failing Tests (per item description)

### 1. `escape-analytics-global-view.spec.ts` — `ShowAggregateSummary`
**Root cause:** The mock response builder `buildGlobalSummaryResponse()` serializes only `totalSequences` and `totalMangled`. The component (`EscapeAnalyticsPage.tsx`) passes `globalCorrelationOutcomes` (not `globalTotalSequences`) to `MangleRateIndicator`'s `totalSequences` prop, which renders it in `data-testid="mangle-counts"`. Because `correlationOutcomes` is not in the mock, it defaults to `0n`. The test asserts "600" (the `totalSequences` value) appears in `mangle-counts` → never matches.

**File:** `tests/e2e/escape-analytics-global-view.spec.ts`, fixture `THREE_ROW_FIXTURE`, function `buildGlobalSummaryResponse()`  
**Component reference:** `web-app/src/components/analytics/EscapeAnalyticsPage.tsx` (passes `globalCorrelationOutcomes` to `MangleRateIndicator.totalSequences`)

### 2. `escape-analytics-global-view.spec.ts` — `ShowErrorBanner`
**Root cause:** `AnnouncerProvider` (`web-app/src/components/ui/Announcer.tsx`) always mounts `<div role="alert" aria-atomic="true" data-testid="announcer-assertive" class={srOnly}>` — an empty but present DOM node. `EscapeAnalyticsGlobalViewPage.getErrorBanner()` uses the selector `'[role="alert"]:not(#__next-route-announcer__)'`, which matches the always-present announcer div in addition to any real error banner. Playwright's strict-mode or `toContainText("Failed to load global summary:")` fails because the empty announcer element doesn't contain that text.

**File:** `tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts`, `getErrorBanner()` method  
**Root div:** `web-app/src/components/ui/Announcer.tsx` line 46

### 3. `jules-status-badge.spec.ts` — Tests 2 and 3
**Root cause:** `seedJulesWorkSessionDirect()` in `tests/e2e/pages/BacklogMutations.ts` calls `POST /api/debug/backlog/seed-jules-work-session`. When the test server's debug seed handler block is not registered, Go's `net/http.ServeMux` falls through to the `/` catch-all, which is the SPA `StaticFileServer` (`server/middleware/static.go`) returning `index.html` with HTTP 200. `BacklogMutations.ts:244` then calls `resp.json()` on HTML → `SyntaxError: Unexpected token '<'`. The handler is registered only when `os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" && deps.Storage != nil`. The test server helper sets `STAPLER_SQUAD_INSTANCE=e2e-local` (verified in `tests/e2e/helpers/test-server.ts` line 101), so the gap is `deps.Storage == nil` at startup time — the storage initialization happens after the debug routes branch.

**File:** `tests/e2e/pages/BacklogMutations.ts:244`, `server/services/backlog_debug_seed_handler.go`, `server/server.go` lines 1186-1209

### 4. `multi-window.spec.ts` — `renaming to empty reverts silently and renaming to a duplicate name is allowed silently`
**Root cause:** Same `AnnouncerProvider` issue. `nonToastAlerts(page)` uses `'[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__)'`. The always-mounted `announcer-assertive` div matches this selector (it has `role="alert"` but no `data-testid="toast"` and no `id="__next-route-announcer__"`). `expect(nonToastAlerts(page)).toHaveCount(0)` fails because count is always at least 1.

**File:** `tests/e2e/multi-window.spec.ts`, `nonToastAlerts()` function

## Acceptance Criteria

1. `escape-analytics-global-view.spec.ts ShowAggregateSummary` passes: `mangle-counts` shows the `correlationOutcomes` value from the mock fixture.
2. `escape-analytics-global-view.spec.ts ShowErrorBanner` passes: the error banner locator matches only the real error banner div, not the announcer assertive div.
3. `jules-status-badge.spec.ts` tests 2 and 3 pass: `seedJulesWorkSessionDirect` receives a valid JSON response (not HTML) from the seed endpoint.
4. `multi-window.spec.ts renaming...` passes: `nonToastAlerts(page)` count is 0 when no toast is shown.
5. All four fixes pass on both Next and Bun builds (per item requirement).
6. No other e2e tests regress.

## Out of Scope

- Refactoring `AnnouncerProvider` itself — selector exclusion is the minimal targeted fix.
- Fixing any other failures beyond the four listed.
- Changes to Go backend business logic.
