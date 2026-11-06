# Plan: Fix Pre-existing e2e Failures

## Summary

Four targeted, surgical fixes across three test/page-object files, plus one server-side investigation. No production component changes needed for bugs 1/2/4; bug 3 may require a one-line server change.

---

## Fix 1 — escape-analytics ShowAggregateSummary: add correlationOutcomes to mock

**File:** `tests/e2e/escape-analytics-global-view.spec.ts`

**Change:**

1. Add `correlationOutcomes: number` to `GlobalSummaryFixture` interface.
2. Add `correlationOutcomes: 600` to `THREE_ROW_FIXTURE`.
3. In `buildGlobalSummaryResponse()`, serialize it:
   ```ts
   correlationOutcomes: String(fixture.correlationOutcomes),
   ```
4. Update the `ShowAggregateSummary` test assertion to match `correlationOutcomes` (600), not `totalSequences`:
   ```ts
   await expect(page.getAggregateMangleCounts()).toContainText("600");
   ```
   (This assertion already uses "600"; since we're setting `correlationOutcomes: 600` = same value, the test body doesn't change — just the fixture and builder.)

**Risk:** Low. Test-only change. No ambiguity in the component behavior.

---

## Fix 2 — escape-analytics ShowErrorBanner: exclude announcer-assertive

**File:** `tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts`

**Change:**

```ts
// Before:
return this.page.locator('[role="alert"]:not(#__next-route-announcer__)');

// After:
return this.page.locator('[role="alert"]:not(#__next-route-announcer__):not([data-testid="announcer-assertive"])');
```

**Risk:** Low. Selector is narrowed, not broadened. The real error banner rendered by `EscapeAnalyticsPage.tsx` is a `<div role="alert">` without `data-testid="announcer-assertive"`, so it still matches.

---

## Fix 3 — jules-status-badge: ensure debug seed endpoint is reachable

**Investigation first:** Before coding, verify the exact guard condition in `server/server.go` (line ~1186) and trace what `deps.Storage` vs `deps.BacklogStorage` the `handleSeedJulesWorkSession` handler uses.

**Likely fix (server/server.go):**

Change the registration guard to not require `deps.Storage != nil` when `STAPLER_SQUAD_INSTANCE=e2e-local`:

```go
// Before:
if os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" && deps.Storage != nil {
    debugSeedHandler.RegisterRoutes(mux)
}

// After (option A — simplest, safe because the seed handler is test-only):
if os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" {
    debugSeedHandler.RegisterRoutes(mux)
}
```

Or, if the handler itself panics when storage is nil, fix the handler to return a 503 instead:

```go
if os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" {
    if deps.Storage != nil {
        debugSeedHandler.RegisterRoutes(mux)
    } else {
        // register a stub that returns 503 so tests get a clear error, not HTML
        mux.HandleFunc("/api/debug/backlog/", func(w http.ResponseWriter, r *http.Request) {
            http.Error(w, "debug seed storage unavailable", http.StatusServiceUnavailable)
        })
    }
}
```

**Alternative (no server change):** If the storage is initialized by the time `beforeAll` runs (just not at server startup), move the seed call to happen after `waitForBacklogRPCsEnabled` — but this doesn't fix the root cause.

**Risk:** Low for option A. The `STAPLER_SQUAD_INSTANCE=e2e-local` guard already makes the block test-only. Removing the `deps.Storage` check means the seed handler registers but any seed call that requires storage will return an error from the handler's own nil-check rather than a silent HTML 200.

---

## Fix 4 — multi-window nonToastAlerts: exclude announcer-assertive

**File:** `tests/e2e/multi-window.spec.ts`

**Change:**

```ts
// Before:
function nonToastAlerts(page: Page) {
  return page.locator('[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__)');
}

// After:
function nonToastAlerts(page: Page) {
  return page.locator('[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__):not([data-testid="announcer-assertive"])');
}
```

**Risk:** Low. Same reasoning as Fix 2.

---

## Adversarial Review

**Q: Could the component really be using `globalCorrelationOutcomes` for the counts display? Isn't that a product bug?**
A: Yes, it is likely a product design choice — `correlationOutcomes` counts sequences that had a correlation result, which is the meaningful denominator for mangle rate in this context. The test fixture needs to set this value; the component is correct.

**Q: Won't excluding `announcer-assertive` in selectors hide real accessibility regressions?**
A: The exclusion is only in locators for user-visible error/alert content. The `announcer-assertive` div is a screen-reader-only live region for announcements, not a user-visible alert. Tests for announcer behavior should target it by `data-testid="announcer-assertive"` directly.

**Q: For Bug 3, could the issue be that `enableBacklogFeatureFlag` is called in `beforeAll` but the server started before the flag was toggled?**
A: Yes, but the backlog feature flag controls whether the UI surfaces backlog items, not whether `deps.Storage` is initialized. Storage initialization happens at server startup, independently of feature flags. The flag API call in `beforeAll` modifies runtime config, not startup state.

**Q: Are there other selectors in the test suite that also match the announcer div?**
A: Search `[role="alert"]` usage across `tests/e2e/`. Any locator that queries `role="alert"` without excluding `data-testid="announcer-assertive"` will silently match it. This is a latent correctness hazard — consider adding a shared `alertLocator(page)` helper to `tests/e2e/helpers/` that encodes the standard exclusions.

---

## Implementation Order

1. Fix 1 (escape-analytics fixture) — test-only, zero risk
2. Fix 2 (escape-analytics error banner selector) — test-only, zero risk
3. Fix 4 (multi-window nonToastAlerts) — test-only, zero risk
4. Fix 3 (server registration) — requires server investigation first, then minimal server change

Run `cd tests/e2e && bun run test escape-analytics-global-view.spec.ts` after fixes 1-2 and `bun run test multi-window.spec.ts` after fix 4 before touching the server.
