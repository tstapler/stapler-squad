# Pitfalls: Put Backlog Behind a Feature Flag by Default

## Summary

The backlog feature flag infrastructure is largely in place. `BacklogController.Enable()` is already conditionally called at startup based on `cfg.GetFeatureFlag("backlog")`. The remaining work is narrowly scoped: ensure the default is OFF for fresh installs, add a frontend route guard on the backlog page, and confirm no existing code path bypasses the guard. The pitfalls below are ordered roughly by severity.

---

## 1. BacklogLifecycleListener Always Wires to Instances Regardless of Flag

**Risk:** `backlogLifecycleListener.WireToInstance(inst)` is called unconditionally in `server/dependencies.go` at lines 463 and 631 (once during bulk instance init, once for each new instance created at runtime). The listener's `SetEnabled(false)` suppresses its active behaviors, but the wiring itself — attaching hooks, registering callbacks — still happens even when backlog is off.

**Severity:** Low

**Mitigation:** Confirm that `BacklogLifecycleListener` is safe to wire when disabled (i.e., all callbacks check `enabled` before acting). If any hook fires unconditionally, add a guard. This is a pre-existing design; no regression is introduced by this PR, but it should be verified.

---

## 2. Frontend Route Guard Absent — Direct URL Navigation Bypasses Feature Gate

**Risk:** `web-app/src/app/backlog/page.tsx` has no feature flag check. A user who knows the URL can navigate directly to `/backlog` and reach the full page even when the flag is off. The navigation link is hidden in `Header.tsx` and `BottomNav.tsx`, but that does not block direct access.

**Severity:** High

**Mitigation:** Add a guard at the top of `BacklogPageInner` (or in `BacklogPage`) that reads `useFeatureFlags()` and redirects to `/` (or renders a 404-style message) when the flag is off. The guard must also handle the `isLoading` case (see pitfall 3).

The same applies to `/backlog/board` (`web-app/src/app/backlog/board/page.tsx`) — both routes need guarding.

---

## 3. Hydration Flash Before Feature Flags RPC Returns

**Risk:** `FeatureFlagsContext` initializes with `isLoading: true` and `flags: {}`. During that window `useFeatureFlag("backlog")` returns `false`. If the route guard redirects immediately on `false`, it will redirect on every page load before the RPC completes — including for users who have backlog enabled.

**Severity:** High

**Mitigation:** The route guard must treat `isLoading: true` as "do not redirect yet" — render a neutral loading state or nothing until `isLoading` becomes `false`. Pattern:

```tsx
const { flags, isLoading } = useFeatureFlags();
if (isLoading) return <LoadingSpinner />;
if (!flags["backlog"]) { router.replace("/"); return null; }
```

Skipping the `isLoading` check causes all enabled users to briefly see a redirect loop on page load.

---

## 4. BacklogService RPCs Reachable Without the Flag — No Backend Gate

**Risk:** `BacklogService` is always instantiated and registered in `server/server.go` unconditionally. When the backlog flag is off, the HTTP handler still responds to `BacklogService` RPC calls (e.g., `ListBacklogItems`, `CreateBacklogItem`). A client that knows the endpoint can read and write backlog data regardless of the feature flag state.

**Severity:** Medium

**Mitigation (option A — preferred):** Add a Connect interceptor on the `BacklogService` handler that checks `cfg.GetFeatureFlag("backlog")` and returns `connect.CodePermissionDenied` when off. This is analogous to the startup guard in `dependencies.go` line 754.

**Mitigation (option B — minimal):** Accept that the RPC layer is unguarded and rely solely on the frontend guard. This is acceptable for a local-only single-user tool where the threat model is accidental navigation rather than adversarial access.

The chosen mitigation must be documented in the PR to avoid confusion.

---

## 5. GetFeatureFlags / UpdateFeatureFlag Must Never Be Gated

**Risk:** If a backend interceptor is added to gate BacklogService RPCs (pitfall 4), the interceptor must not also gate the `GetFeatureFlags` and `UpdateFeatureFlag` RPCs — those live on `SessionService`, not `BacklogService`, so this is not actually a risk as long as the interceptor is scoped to the correct handler. However, if the implementation accidentally wraps the entire mux, it will lock users out of the settings page needed to re-enable the flag.

**Severity:** High (if interceptor is applied too broadly)

**Mitigation:** Scope any backlog-gating interceptor to the `BacklogService` handler registration only, not to the global `ConnectOptions`. The `SessionService` handler — which owns feature flag RPCs — must be unaffected.

---

## 6. Existing E2E Tests Navigate to `/backlog` Without Setting the Feature Flag

**Risk:** `tests/e2e/backlog.spec.ts` navigates directly to `/backlog` with `goto('/backlog')` and expects `[data-testid="backlog-page"]` to render. If a route guard is added and the flag defaults to off, these tests will fail (they'll hit the redirect instead of the page).

**Severity:** Medium

**Mitigation:** The e2e test server setup must enable the backlog flag before running backlog specs. Options:
- Add a `beforeAll` block that calls `UpdateFeatureFlag` via the RPC to enable backlog.
- Set the flag in the test server's config file (`STAPLER_SQUAD_TEST_DIR` pattern).
- Add a dedicated test helper that toggles the flag.

Without this, CI will break as soon as the route guard lands.

---

## 7. Existing Users With `"backlog": true` Must Not Be Broken

**Risk:** Users who previously enabled the backlog feature have `"backlog": true` in `~/.stapler-squad/config.json`. The code path at `dependencies.go:754` already reads this and calls `backlogCtrl.Enable()` on startup — this is correct behavior and must not be changed.

**Severity:** Low (already handled correctly)

**Mitigation:** No code change needed. Verify that the "default OFF" change only affects fresh installs where the key is absent. `GetFeatureFlag` already returns `false` for absent keys. The only change needed is removing any existing default-to-true initialization if one exists (a quick `grep` found none).

---

## 8. `backlog:conversation-view` Sub-Flag Is Harmless When Backlog Is Off

**Risk:** The sub-flag `backlog:conversation-view` is exposed in `knownFeatureFlags` and shown in the Settings → Features UI regardless of whether backlog itself is enabled. A user could theoretically toggle it while backlog is off, which would persist a meaningless config entry.

**Severity:** Low

**Mitigation:** Optionally filter the sub-flag from the Settings UI when `backlog` is off (similar to how nav pages are filtered). Not strictly necessary since the stored value is harmless and takes effect correctly when backlog is later enabled. Consider whether the Settings page should show a hierarchy or dependency indicator.

---

## 9. Settings Page Discoverability — Users Cannot Find the Flag to Enable It

**Risk:** If the backlog nav link is hidden by default, new users have no obvious path to discover and enable the feature. The Settings → Features page exists at `/settings/features` but is not prominently surfaced.

**Severity:** Low (UX concern, not a correctness bug)

**Mitigation:** Consider adding a mention in the Settings → Features page description, or a tooltip/help text on the hidden nav slot. This is out of scope for the flag-default change itself but should be tracked as a follow-up.

---

## 10. Race Between `UpdateFeatureFlag` and In-Flight Backlog RPCs

**Risk:** When a user disables the backlog flag at runtime, `BacklogController.Disable()` stops the sync loop and sets the listener disabled. However, any in-flight BacklogService RPC (e.g., a slow triage operation) continues executing — there is no cancellation of in-flight work tied to the flag toggle.

**Severity:** Low

**Mitigation:** This is a pre-existing design limitation, not introduced by this PR. The triage harness has its own `Shutdown()` hook wired to server shutdown. Documenting this as a known limitation is sufficient for now.

---

## Checklist for This PR

- [ ] Frontend route guard added to `/backlog` (page.tsx) and `/backlog/board` (board/page.tsx) — handles `isLoading` before checking flag
- [ ] No `"backlog": true` default exists in any config initialization path
- [ ] E2E tests in `backlog.spec.ts` enable the feature flag before navigating
- [ ] Backend interceptor decision documented (gate or accept unguarded RPC layer)
- [ ] If interceptor added, scoped to `BacklogService` handler only, not global mux
- [ ] `make quick-check` passes
