# Plan: Put Backlog Behind a Feature Flag by Default

## Executive Summary

The backlog feature flag infrastructure already exists end-to-end (backend default-off, frontend default-off, nav link hidden, `BacklogController.Enable()` gated), but direct URL navigation to `/backlog` or `/backlog/board` bypasses every gate. This plan closes the gap with a two-layer defense: a client-side layout redirect (primary) and a backend interceptor on the BacklogService RPCs (secondary).

---

## Implementation Approach

### Why two layers

The nav link hiding is sufficient for normal users but not for direct URL access or API callers. The frontend layout guard covers both accidental and intentional URL navigation. The backend interceptor provides defense-in-depth so the RPCs cannot be misused even if the frontend changes.

### Layer 1 — Frontend layout guard (primary fix, ~30 min)

Convert `web-app/src/app/backlog/layout.tsx` from a passthrough to a `"use client"` component. On mount, read `useFeatureFlag("backlog")` via `useFeatureFlags()`. While loading, render `null` (avoids flash). When loaded and flag is false, call `router.replace("/")`. When loaded and flag is true, render `{children}`.

This single file change covers both `/backlog` and `/backlog/board` because Next.js layout files wrap all routes under their segment.

### Layer 2 — Backend interceptor (defense-in-depth, ~45 min)

Add `server/interceptors/feature_flag_interceptor.go` implementing a Connect interceptor that checks a named feature flag on every RPC call. Wire it only to `BacklogServiceHandler` in `server/server.go` so it has zero performance impact on other services. Returns `connect.CodeNotFound` when flag is off — this matches the "feature does not exist" semantic rather than "unauthorized."

### Layer 3 — E2E test update (~20 min)

`tests/e2e/backlog.spec.ts` tests will fail in CI if they do not enable the flag first. Add a `beforeAll` block that calls `UpdateFeatureFlag` RPC with `{"backlog": true}` and a corresponding `afterAll` to restore the default, or use a dedicated test config fixture that pre-enables the flag.

---

## Task Breakdown

| # | Task | File(s) | Est. |
|---|------|---------|------|
| 1 | Convert backlog layout to client component with flag guard | `web-app/src/app/backlog/layout.tsx` | 20 min |
| 2 | Write `NewFeatureFlagInterceptor` | `server/interceptors/feature_flag_interceptor.go` | 30 min |
| 3 | Wire interceptor to BacklogServiceHandler | `server/server.go` | 10 min |
| 4 | Update e2e tests to enable flag in beforeAll | `tests/e2e/backlog.spec.ts` | 20 min |
| 5 | Run `make quick-check` and fix any issues | — | 10 min |
| **Total** | | | **~90 min** |

---

## Dependencies and Blockers

**No blockers.** All prerequisite infrastructure is in place:
- `GetFeatureFlag("backlog")` already exists in `server/services/session_service.go:3908–3925`
- `useFeatureFlag` / `useFeatureFlags()` hooks already exist in `FeatureFlagsContext`
- `connect` interceptor pattern is established in `server/interceptors/` (check for existing examples to match style)
- `UpdateFeatureFlag` RPC already exists for the e2e test setup call

**Dependencies within this task:**
- Task 3 depends on Task 2 (interceptor must exist before it can be wired)
- Task 4 is independent of Tasks 1–3 but should be done last to avoid flaky CI during development

---

## Files Changed and Why

| File | Change | Why |
|------|--------|-----|
| `web-app/src/app/backlog/layout.tsx` | Convert to `"use client"`, add `useFeatureFlags()` guard with `router.replace("/")` | Closes direct URL bypass for all routes under `/backlog` |
| `server/interceptors/feature_flag_interceptor.go` | New file: `NewFeatureFlagInterceptor(flagName string, cfg config.Provider)` | Reusable backend guard; returns `CodeNotFound` when flag is off |
| `server/server.go` | Wire `NewFeatureFlagInterceptor("backlog", cfg)` to `BacklogServiceHandler` only | Enforces flag at RPC layer without affecting other services |
| `tests/e2e/backlog.spec.ts` | Add `beforeAll` to enable flag via `UpdateFeatureFlag` RPC; `afterAll` to restore | Prevents CI test failures now that the flag defaults to off |

---

## Acceptance Criteria

1. Navigating directly to `/backlog` or `/backlog/board` with flag off redirects to `/`
2. With flag on, `/backlog` loads normally
3. Calling any BacklogService RPC with flag off returns `connect.CodeNotFound`
4. `make quick-check` passes
5. E2E backlog tests pass in CI with no changes to the flag default
