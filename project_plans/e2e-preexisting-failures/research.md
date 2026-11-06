# Research: e2e Pre-existing Failures

## Methodology

Each failure traced end-to-end: test assertion → page object selector → component/server code → mock/fixture gap.

---

## Bug 1 — escape-analytics ShowAggregateSummary

### Trace

**Test assertion** (`escape-analytics-global-view.spec.ts`):
```ts
await expect(page.getAggregateMangleCounts()).toContainText("600");
```
`THREE_ROW_FIXTURE.totalSequences = 600`.

**Page object** (`tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts`):
```ts
getAggregateMangleCounts() {
  return this.getPanel("all_sessions").getByTestId("mangle-counts").first();
}
```

**Component** (`web-app/src/components/analytics/EscapeAnalyticsPage.tsx`):
```tsx
<MangleRateIndicator
  mangleRate={globalMangleRate}
  totalSequences={globalCorrelationOutcomes}  // ← uses correlationOutcomes, not totalSequences
  totalMangled={globalTotalMangled}
/>
```
`MangleRateIndicator` renders `totalSequences.toString()` inside `data-testid="mangle-counts"`.

**Mock fixture** (`tests/e2e/escape-analytics-global-view.spec.ts`):
```ts
function buildGlobalSummaryResponse(fixture) {
  return {
    // ...
    totalSequences: String(fixture.totalSequences),  // "600"
    totalMangled: String(fixture.totalMangled),       // "60"
    // correlationOutcomes NOT serialized
  };
}
```
Hook (`useEscapeAnalyticsGlobalSummary`) initializes `correlationOutcomes` to `0n`. Since the mock omits the field, `globalCorrelationOutcomes` stays `0n`. `MangleRateIndicator` renders `"0"` → assertion for `"600"` fails.

### Fix

Add `correlationOutcomes` to `GlobalSummaryFixture` and `buildGlobalSummaryResponse()`, set to the same value as `totalSequences` in `THREE_ROW_FIXTURE` (or any distinct value that the test assertion checks).

---

## Bug 2 — escape-analytics ShowErrorBanner

### Trace

**Test assertion**:
```ts
await expect(page.getErrorBanner()).toContainText("Failed to load global summary:");
```

**Page object** (`tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts`):
```ts
getErrorBanner() {
  return this.page.locator('[role="alert"]:not(#__next-route-announcer__)');
}
```

**Always-present announcer** (`web-app/src/components/ui/Announcer.tsx`):
```tsx
<div role="alert" aria-atomic="true" data-testid="announcer-assertive" className={srOnly}>
  {assertive}
</div>
```
This div is always in the DOM (even when `assertive` is `""`). It has `role="alert"` and no `id="__next-route-announcer__"`, so the `:not(#__next-route-announcer__)` exclusion does not filter it.

Playwright `locator()` in strict mode errors when multiple elements match; or `toContainText()` on the empty announcer div fails the assertion. Either way the test fails.

### Fix

Update `getErrorBanner()` to also exclude `[data-testid="announcer-assertive"]`:
```ts
return this.page.locator('[role="alert"]:not(#__next-route-announcer__):not([data-testid="announcer-assertive"])');
```

---

## Bug 3 — jules-status-badge HTML response

### Trace

**BacklogMutations.ts:244** (`tests/e2e/pages/BacklogMutations.ts`):
```ts
return (await resp.json()) as { itemId: string; sessionId: string };
// ^ SyntaxError: Unexpected token '<'
```
`resp.json()` fails because the response body is HTML, not JSON.

**Why HTML?** Go's `net/http.ServeMux` most-specific-path matching: if `/api/debug/backlog/seed-jules-work-session` is not registered, the request falls to the `/` catch-all pattern, served by `StaticFileServer` (`server/middleware/static.go`). `StaticFileServer` returns `index.html` with HTTP 200 for any unrecognized path (SPA fallback).

**Registration condition** (`server/server.go` ~line 1186):
```go
if os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" && deps.Storage != nil {
  debugSeedHandler.RegisterRoutes(mux)
}
```
`tests/e2e/helpers/test-server.ts` (line 101) sets `STAPLER_SQUAD_INSTANCE: 'e2e-local'`. The env var check passes.

**Likely cause: `deps.Storage == nil`**. The backlog storage path includes the backlog feature flag being enabled. `enableBacklogFeatureFlag(request)` is called in `beforeAll`, but this happens *after* the test server is spawned (in `global-setup.ts`). The debug seed handler registration happens at server startup — if `deps.Storage` is nil because the backlog feature isn't initialized at startup time, or the storage init for the jules session type is gated on a flag that's off by default, the branch skips registration.

**Secondary evidence**: Tests 1 passes (it only calls `enableBacklogFeatureFlag` and checks a badge state), while tests 2 and 3 both call `seedJulesWorkSessionDirect` and both fail with the same parse error. This confirms the endpoint is simply not registered.

### Fix

Two options:
1. **Preferred**: Make the debug seed handler registration unconditional on `deps.Storage != nil` for the `STAPLER_SQUAD_INSTANCE=e2e-local` case — if the seed handler is only ever called in e2e tests anyway, the storage nil check is defensive-but-wrong.
2. **Alternative**: Ensure the server initializes `deps.Storage` before the registration check, or check `deps.BacklogStorage` separately (the backlog-specific storage).

Need to verify which storage field the jules seed handler actually uses: `backlog_debug_seed_handler.go`'s `handleSeedJulesWorkSession`. If it uses `deps.BacklogStorage`, the condition should check that field specifically.

---

## Bug 4 — multi-window nonToastAlerts

### Trace

**Test assertion** (`tests/e2e/multi-window.spec.ts`):
```ts
await expect(nonToastAlerts(page)).toHaveCount(0);
```

**Helper**:
```ts
function nonToastAlerts(page: Page) {
  return page.locator('[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__)');
}
```

Same `AnnouncerProvider` issue as Bug 2: the always-present `data-testid="announcer-assertive"` div matches this selector. Count is always ≥ 1 → assertion fails.

### Fix

Extend the `:not()` chain in `nonToastAlerts()`:
```ts
return page.locator('[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__):not([data-testid="announcer-assertive"])');
```

---

## Cross-cutting: AnnouncerProvider pattern

Both Bug 2 and Bug 4 trace to the same always-mounted `role="alert"` div in `AnnouncerProvider`. A stronger long-term fix would be to give `AnnouncerProvider`'s assertive div a stable exclusion attribute (it already has `data-testid="announcer-assertive"`, which is exactly what the fix uses). The selector pattern `:not([data-testid="announcer-assertive"])` is the right exclusion in all e2e locators targeting user-visible alerts. A shared page-object helper or a Playwright base fixture that wraps `page.locator('[role="alert"]')` with the standard exclusions would prevent this class of selector drift.
