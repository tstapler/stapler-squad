# Validation Plan: Put Backlog Behind a Feature Flag by Default

## Acceptance Criteria → Test Coverage Map

| # | Acceptance Criterion | Test Type | File(s) |
|---|---|---|---|
| AC-1 | Fresh install (no `config.json`) shows no backlog UI and no backlog nav link | Unit (Jest) | `Navigation.test.tsx` (existing — already covered) |
| AC-2 | Direct `/backlog` with flag off redirects to home, no flash of content | Unit (RTL) | `app/backlog/layout.test.tsx` (new) |
| AC-3 | Direct `/backlog/board` with flag off also redirects | Unit (RTL) | `app/backlog/layout.test.tsx` (new) |
| AC-4 | Enabling flag via `/settings/features` makes nav link and pages accessible | E2E | `tests/e2e/backlog.spec.ts` (update) |
| AC-5 | Users with pre-existing `"backlog": true` in config are not broken | Unit (Go) + E2E | `config/feature_flags_test.go` (existing — already covers persistence); `backlog.spec.ts` (add scenario) |
| AC-6 | BacklogService RPCs return `CodeNotFound` when flag is off | Unit (Go) | `server/interceptors/feature_flag_interceptor_test.go` (new) |
| AC-7 | Settings page remains accessible regardless of flag state | E2E | `tests/e2e/settings.spec.ts` (add assertion) or inline in `backlog.spec.ts` |

---

## Test Cases by Acceptance Criterion

### AC-1 — Nav link hidden by default

**Already covered** by `Navigation.test.tsx`:
- `Navigation_should_hide_backlogTab_when_featureFlagFalse` asserts `screen.queryByText("Backlog")` is null when `useFeatureFlag` returns false (the default mock).
- No new tests needed here, but confirm the mock default value in the test file matches the real default (flag off = `false`).

**Verify:** `cd web-app && npx jest --no-coverage --testPathPatterns="Navigation.test"`

---

### AC-2 and AC-3 — Direct URL redirect, no flash

**New test file:** `web-app/src/app/backlog/layout.test.tsx`

These tests render the converted `BacklogLayout` component in isolation using React Testing Library.

#### TC-LAYOUT-001 — Renders null while loading (no flash)

```
Given: useFeatureFlags returns { isLoading: true, flags: {} }
When:  BacklogLayout renders
Then:  The DOM is empty (null rendered, no children mounted)
```
Assert: `container.firstChild` is null.

#### TC-LAYOUT-002 — Redirects to "/" when flag is false and loading is done

```
Given: useFeatureFlags returns { isLoading: false, flags: { backlog: false } }
When:  BacklogLayout renders
Then:  router.replace("/") is called exactly once
And:   No children are rendered
```
Assert: `mockRouter.replace` called with `"/"`, `screen.queryByTestId("children-sentinel")` is null.

#### TC-LAYOUT-003 — Renders children when flag is true and loading is done

```
Given: useFeatureFlags returns { isLoading: false, flags: { backlog: true } }
When:  BacklogLayout renders
Then:  children are rendered
And:   router.replace is NOT called
```
Assert: `screen.getByTestId("children-sentinel")` is in the document, `mockRouter.replace` not called.

#### TC-LAYOUT-004 — Does not redirect a flag-enabled user if loading was slow (race condition guard)

```
Given: useFeatureFlags starts as { isLoading: true } then transitions to { isLoading: false, flags: { backlog: true } }
When:  BacklogLayout renders and loading resolves
Then:  router.replace is never called at any point
```
This guards the `isLoading` race where a legitimately enabled user gets redirected before their flags load.

**Mock setup for layout tests:**

```typescript
jest.mock("next/navigation", () => ({
  useRouter: jest.fn().mockReturnValue({ replace: jest.fn() }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: jest.fn(),
}));
```

**Verify:** `cd web-app && npx jest --no-coverage --testPathPatterns="backlog/layout.test"`

---

### AC-4 — Enabling flag makes nav and pages accessible

**Update `tests/e2e/backlog.spec.ts`:**

Add a `test.describe.configure({ mode: "serial" })` wrapping block and a `beforeAll`/`afterAll` pair:

```typescript
test.beforeAll(async ({ request }) => {
  // Enable the backlog feature flag via the RPC before any backlog test runs.
  await request.post(`${BASE_URL}/session.v1.SessionService/UpdateFeatureFlag`, {
    data: { name: "backlog", enabled: true },
    headers: { "Content-Type": "application/json" },
  });
});

test.afterAll(async ({ request }) => {
  // Restore default (flag off) so other test suites are not affected.
  await request.post(`${BASE_URL}/session.v1.SessionService/UpdateFeatureFlag`, {
    data: { name: "backlog", enabled: false },
    headers: { "Content-Type": "application/json" },
  });
});
```

All existing backlog tests remain unchanged inside the describe block — they already assume the page is reachable.

**New e2e test to add within `backlog.spec.ts`:**

#### TC-E2E-BACKLOG-FLAG-001 — Nav link appears after enabling flag

```
Given: Flag is enabled (set in beforeAll)
When:  User navigates to any page
Then:  Navigation contains a "Backlog" link
```

---

### AC-5 — Pre-existing enabled config is not broken

**Go unit coverage (already exists):**
- `TestSetFeatureFlag_UpdatesExistingMap` in `config/feature_flags_test.go` covers setting `false` on a previously-`true` config; the converse (loading a config that had `true`) is implicitly covered by `TestGetFeatureFlag_Present`.
- `TestGetFeatureFlags_ReflectsControllerState` in `server/services/feature_flags_test.go` verifies the RPC reflects controller state.

**No new Go unit tests needed.**

**E2E scenario to add in `backlog.spec.ts`:**

#### TC-E2E-BACKLOG-FLAG-002 — Flag persists after page reload

```
Given: Flag is set to true via UpdateFeatureFlag
When:  Page is reloaded (full browser reload)
Then:  The backlog nav link is still visible
And:   /backlog loads without redirect
```
This catches any regression where the flag is saved to an in-memory cache that doesn't survive reload.

---

### AC-6 — BacklogService RPCs return CodeNotFound when flag is off

**New test file:** `server/interceptors/feature_flag_interceptor_test.go`

#### TC-INTERCEPTOR-001 — All BacklogService RPCs blocked when flag is false

```
Given: A BacklogService handler wrapped with NewFeatureFlagInterceptor("backlog", cfg)
And:   cfg.GetFeatureFlag("backlog") returns false
When:  Any BacklogService RPC is called (e.g. ListBacklogItems, CreateBacklogItem, TriggerTriage)
Then:  The response is a ConnectRPC error with Code == CodeNotFound
And:   The handler itself is NOT called (no side effects)
```
Use a spy/mock handler that records whether it was invoked. Assert it was not called.

#### TC-INTERCEPTOR-002 — BacklogService RPCs pass through when flag is true

```
Given: cfg.GetFeatureFlag("backlog") returns true
When:  Any BacklogService RPC is called
Then:  The underlying handler IS called
And:   No error is injected by the interceptor
```

#### TC-INTERCEPTOR-003 — Interceptor does not affect non-backlog services

```
Given: The interceptor is wired only to BacklogServiceHandler (as per plan)
When:  A SessionService RPC is called (e.g. ListSessions)
Then:  No interceptor interference — call proceeds normally
```
This is a server wiring test; verify in `server/server.go` review that `NewFeatureFlagInterceptor` is only passed to `BacklogServiceHandler` options, not to the top-level server interceptor chain.

**Verify:** `go test ./server/interceptors/...`

---

### AC-7 — Settings page remains accessible

**E2E test to add** (either in `tests/e2e/settings.spec.ts` or inline in `backlog.spec.ts` outside the `beforeAll/afterAll` scope):

#### TC-E2E-SETTINGS-001 — Settings/features page loads with flag off

```
Given: Flag is NOT enabled (no beforeAll enabling it)
When:  User navigates to /settings/features
Then:  The page loads successfully (200, no redirect)
And:   The backlog toggle control is visible and its state is "off"
```
Use a separate `test.describe` block that does NOT have the flag-enabling `beforeAll`.

---

## Edge Cases

### Loading state race — do not redirect enabled users

The `FeatureFlagsContext` starts with `isLoading: true` and `flags: {}`. During that window, `useFeatureFlag("backlog")` returns `false` (the `?? false` default in the hook). Without an explicit `isLoading` guard, the layout would redirect every user — including those with the flag enabled — on initial render.

**Required guard in `BacklogLayout`:**
```tsx
const { isLoading, flags } = useFeatureFlags();
if (isLoading) return null;           // no flash, wait for real state
if (!flags["backlog"]) { router.replace("/"); return null; }
return <>{children}</>;
```

TC-LAYOUT-001 and TC-LAYOUT-004 specifically cover this.

### Sub-route coverage — `/backlog/board`

The layout guard in `web-app/src/app/backlog/layout.tsx` wraps all routes under the `/backlog` segment. TC-LAYOUT-002 covers `/backlog`; the board page gets the same guard for free. However, verify this in a dedicated test:

#### TC-LAYOUT-005 — Board sub-route is also guarded

```
Given: useFeatureFlags returns { isLoading: false, flags: { backlog: false } }
When:  BacklogBoardPage renders (with layout wrapping it)
Then:  router.replace("/") is called
And:   BacklogBoard component is not mounted
```
Use a shallow integration render of `layout.tsx` wrapping `board/page.tsx`.

---

## Error Scenarios

### Direct URL navigation (flag off)

| Scenario | Expected Behavior | Test |
|---|---|---|
| User types `/backlog` in address bar | Client-side redirect to `/`, no backlog content rendered | TC-LAYOUT-002, TC-E2E-BACKLOG-GUARD-001 |
| User types `/backlog/board` in address bar | Same redirect | TC-LAYOUT-005 |
| Bookmark to `/backlog` opened after flag disabled | Redirect to home | TC-LAYOUT-002 |
| Browser back button to `/backlog` with flag off | Redirect fires again | Covered by TC-LAYOUT-002 (re-render guard) |

**E2E test to add:**

#### TC-E2E-BACKLOG-GUARD-001 — Direct navigation redirects to home with flag off

```
Given: The backlog flag is NOT enabled (outside the beforeAll enabling block)
When:  page.goto(`${BASE_URL}/backlog`)
Then:  page.url() resolves to BASE_URL + "/" (or "/sessions")
And:   No element with data-testid="backlog-page" is in the DOM
```

### API calls with flag off

| Scenario | Expected Behavior | Test |
|---|---|---|
| `ListBacklogItems` called with flag off | `connect.CodeNotFound` error | TC-INTERCEPTOR-001 |
| `CreateBacklogItem` called with flag off | `connect.CodeNotFound` error | TC-INTERCEPTOR-001 |
| `TriggerTriage` called with flag off | `connect.CodeNotFound` error | TC-INTERCEPTOR-001 |
| `GetFeatureFlags` called with flag off | Returns flags list (backlog.enabled=false) — NOT a CodeNotFound | Existing `TestGetFeatureFlags_ReturnsKnownFlags` |
| `UpdateFeatureFlag` called with flag off | Succeeds — this is how you turn the flag on | Existing `TestUpdateFeatureFlag_EnablesController` |

The interceptor must NOT block the feature flag RPCs themselves (`GetFeatureFlags`, `UpdateFeatureFlag`). Those live on `SessionService`, not `BacklogService`, so this is satisfied by the wiring — verify in code review.

---

## Existing Tests That Need Updating

### `tests/e2e/backlog.spec.ts`

**Problem:** `test.beforeEach` currently does `page.goto(.../backlog)` and waits for `data-testid="backlog-page"`. With the flag off by default, the server will return no backlog content and the layout will redirect. Every existing test will fail.

**Fix:** Add `test.beforeAll` to enable the flag (see AC-4 section above). The existing `beforeEach` logic stays unchanged.

**Risk to verify:** The `beforeAll` RPC call must complete before the first `beforeEach` fires. Playwright guarantees this ordering within a `describe` block when `test.describe.configure({ mode: "serial" })` is set. Confirm this is set or add it.

### `web-app/src/app/backlog/layout.tsx` — no existing tests

The file is currently a passthrough with no test. The new tests in `layout.test.tsx` (TC-LAYOUT-001 through TC-LAYOUT-005) are net-new additions.

### `web-app/src/components/ui/Navigation.test.tsx`

No changes needed. Tests already mock `useFeatureFlag` to return `false` by default and explicitly test both states. This test suite is already the specification for AC-1.

### `config/feature_flags_test.go`

No changes needed. Existing tests cover `GetFeatureFlag` (nil config, nil map, missing key, present key) and `SetFeatureFlag` (initializes map, updates existing map, persists to disk). AC-5 backend behavior is already validated.

### `server/services/feature_flags_test.go`

No changes needed. Existing tests cover `GetFeatureFlags` RPC (returns known flags, reflects controller state) and `UpdateFeatureFlag` RPC (enables controller, disables controller, unknown flag). The new interceptor tests are separate files in `server/interceptors/`.

---

## Test Execution Checklist

```
# Unit — Go
go test ./config/... -run TestGetFeatureFlag
go test ./config/... -run TestSetFeatureFlag
go test ./server/services/... -run TestGetFeatureFlags
go test ./server/services/... -run TestUpdateFeatureFlag
go test ./server/interceptors/... -run TestFeatureFlagInterceptor   # new

# Unit — Frontend
cd web-app
npx jest --no-coverage --testPathPatterns="Navigation.test"
npx jest --no-coverage --testPathPatterns="backlog/layout.test"      # new

# Full build + lint
make quick-check

# E2E (requires server running on :8544 with flag defaulting to off)
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
cd tests/e2e && npx playwright test backlog.spec.ts
cd tests/e2e && npx playwright test settings.spec.ts                 # if TC-E2E-SETTINGS-001 added there
```

---

## Success Criteria (Done Definition)

- [ ] `make quick-check` passes with no lint or build errors
- [ ] `go test ./server/interceptors/...` passes (TC-INTERCEPTOR-001 through -003)
- [ ] `npx jest backlog/layout.test` passes (TC-LAYOUT-001 through -005)
- [ ] `npx jest Navigation.test` still passes (no regression on AC-1)
- [ ] Playwright `backlog.spec.ts` passes in CI without modifying the server's default flag state
- [ ] Playwright `TC-E2E-BACKLOG-GUARD-001` demonstrates redirect with flag off
- [ ] Manual smoke: fresh `~/.stapler-squad/` directory, start server, confirm no Backlog nav link and `/backlog` redirects to home
- [ ] Manual smoke: enable flag via `/settings/features`, confirm Backlog link appears, `/backlog` loads, `/backlog/board` loads
