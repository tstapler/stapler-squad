# Architecture: Put Backlog Behind a Feature Flag by Default

## Existing Infrastructure (no changes needed)

- **Backend flag read**: `config.Config.GetFeatureFlag("backlog")` returns `false` when absent — already default-off in storage.
- **Frontend flag read**: `useFeatureFlag("backlog")` in `FeatureFlagsContext.tsx` returns `false` by default.
- **Known flags registry**: `knownFeatureFlags` in `server/services/session_service.go` already lists `"backlog"`.
- **Navigation gating**: `web-app/src/components/ui/Navigation.tsx` already hides the Backlog nav link when `useFeatureFlag("backlog")` is false.
- **Settings toggle UI**: `/settings/features` page already exists for toggling the flag.

## Current Gap

The navigation link is hidden, but the routes and RPCs are fully open:

| Surface | Gated? |
|---|---|
| Nav link `/backlog` | Yes (Navigation.tsx) |
| Page route `web-app/src/app/backlog/page.tsx` | **No** |
| Page route `web-app/src/app/backlog/board/page.tsx` | **No** |
| All `BacklogService` RPC methods | **No** |

A user who navigates directly to `/backlog` or `/backlog/board` (bookmark, paste, mobile deeplink) bypasses the nav gate entirely. Any client making raw RPC calls hits the service unconditionally.

---

## Recommended Architecture: Option D — Frontend Page Guard + Backend Connect Interceptor

Defense in depth, minimum change surface.

### Why not frontend-only (Option A/B)?

- The backend executes writes (create, triage, status transition, session spawn) regardless of flag state. A hidden UI is not protection.
- The flag is user-toggleable at runtime; a backend gate enforces the invariant regardless of frontend state.

### Why not backend-only (Option C)?

- A direct URL hit (`/backlog`) renders the full page and calls RPCs before a redirect could occur. Users see a broken UI (RPC errors) instead of a clean redirect.
- UX requires an early-exit at the page level.

### Why a Connect interceptor over per-handler checks?

- 20+ RPC methods across `BacklogService`; per-handler checks are repetitive and can be missed when new methods are added.
- A service-level interceptor is applied once at registration and covers all current and future methods automatically.
- `ConnectOptions()` in `server/server.go` already shows the interceptor composition pattern; the project already has `server/interceptors/error_recorder.go` as a reference.

### Why layout.tsx is the right frontend gate point?

- `web-app/src/app/backlog/layout.tsx` wraps both `page.tsx` and `board/page.tsx`. A single guard in the layout covers the entire subtree without touching individual pages.
- The layout is currently a pass-through (`return <>{children}</>`), making it the minimal-disruption insertion point.

---

## Component Boundaries

### Backend

**New file**: `server/interceptors/feature_flag.go`

```go
// NewFeatureFlagInterceptor returns a Connect interceptor that returns
// codes.NotFound for all procedures when the named flag is disabled.
func NewFeatureFlagInterceptor(flag string, cfg *config.Config) connect.Interceptor
```

**Changed file**: `server/server.go`

Add a `BacklogConnectOptions()` helper (or pass additional interceptor to the existing call) when registering `BacklogServiceHandler`:

```go
blPath, blHandler := sessionv1connect.NewBacklogServiceHandler(
    deps.BacklogService,
    BacklogConnectOptions(deps.ErrorRegistry, deps.Config)...,
)
```

Where `BacklogConnectOptions` prepends `NewFeatureFlagInterceptor("backlog", deps.Config)` to the standard options.

### Frontend

**Changed file**: `web-app/src/app/backlog/layout.tsx`

Convert the pass-through layout to a client component that reads the flag and redirects:

```tsx
"use client";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function BacklogLayout({ children }: { children: React.ReactNode }) {
  const enabled = useFeatureFlag("backlog");
  const { isLoading } = useFeatureFlags();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && !enabled) router.replace("/");
  }, [enabled, isLoading, router]);

  if (!enabled) return null; // suppress flash while redirecting
  return <>{children}</>;
}
```

---

## Reusable `FeatureFlagGuard` Component — Not Worth Building Yet

A generic `<FeatureFlagGuard feature="backlog">` wrapper is appropriate if multiple routes need gating. Currently only the `/backlog` subtree is involved. Building the abstraction now adds ceremony with no immediate payoff.

**Decision**: inline the check in `layout.tsx`. If a second feature needs the same pattern, extract then.

---

## File Change Surface

| File | Change |
|---|---|
| `server/interceptors/feature_flag.go` | **New** — `NewFeatureFlagInterceptor` |
| `server/server.go` | **Edit** — `BacklogConnectOptions` helper + pass to `NewBacklogServiceHandler` |
| `web-app/src/app/backlog/layout.tsx` | **Edit** — add flag check + redirect |

**Files that do NOT change:**

- `web-app/src/app/backlog/page.tsx` — no direct modification; protected by layout
- `web-app/src/app/backlog/board/page.tsx` — same
- `server/services/backlog_service.go` — no per-handler changes
- `server/services/session_service.go` — no changes; `knownFeatureFlags` already has "backlog"
- `config/config.go` — no changes; `GetFeatureFlag` already returns false for absent keys
- `web-app/src/components/ui/Navigation.tsx` — already correct
- `web-app/src/lib/contexts/FeatureFlagsContext.tsx` — no changes

---

## Error Response from Backend

When flag is off, the interceptor should return `connect.CodeNotFound` (not `PermissionDenied`). Rationale: the feature does not exist from the user's perspective when disabled; `PermissionDenied` implies they are authenticated but unauthorized, which leaks the existence of the service. `NotFound` is the conventional response for disabled optional features.

---

## Loading State Consideration

`useFeatureFlag` consults the context which starts in `isLoading: true` with `flags: {}`. The layout must not redirect during loading (would send users away on every hard refresh). Gate the redirect on `!isLoading`:

```tsx
if (!isLoading && !enabled) router.replace("/");
if (!enabled) return null;
```

This suppresses a flash of backlog content before the flag resolves.
