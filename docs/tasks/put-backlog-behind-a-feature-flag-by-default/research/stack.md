# Stack Research: Backlog Feature Flag Gate

## 1. Routing Technology

The project uses **Next.js 15.3.2 App Router** (not React Router). Evidence:
- `web-app/src/app/backlog/page.tsx` uses `useRouter`, `useSearchParams` from `next/navigation`
- `web-app/src/app/backlog/board/page.tsx` uses `useRouter` from `next/navigation`
- Directory structure follows Next.js App Router conventions (`app/backlog/page.tsx`, `app/backlog/board/page.tsx`, `app/backlog/layout.tsx`)
- React 19.0.0

In Next.js App Router, **there is no declarative route guard component** (no `<ProtectedRoute>`). The canonical patterns for gating a page are:

**Option A — Guard inside the page component** (simplest, no new files):
```tsx
// page.tsx
const backlogEnabled = useFeatureFlag("backlog");
if (!backlogEnabled) return <notFound />; // or redirect, or a disabled state
```

**Option B — Guard in `layout.tsx`** (applies to both `/backlog` and `/backlog/board` in one place):
```tsx
// app/backlog/layout.tsx — currently just a passthrough fragment
"use client";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { notFound } from "next/navigation";
export default function BacklogLayout({ children }) {
  const enabled = useFeatureFlag("backlog");
  if (!enabled) notFound(); // or redirect("/")
  return <>{children}</>;
}
```

Option B is preferable here: `app/backlog/layout.tsx` already exists as a passthrough (`<>{children}</>`). Putting the guard there covers both `/backlog` and `/backlog/board` with a single change instead of touching two page files.

**Constraint**: `useFeatureFlag` is a React hook that calls `useContext`, so the layout must be a Client Component (`"use client"` directive required). The current layout has no directive — adding it is safe since the layout renders no server data.

**`notFound()` vs redirect**: `notFound()` throws Next.js's internal 404 signal, which renders the nearest `not-found.tsx`. `redirect("/")` is also acceptable but potentially confusing (silently drops the user at home). A 404 or a friendly "Feature not enabled" component is cleaner UX. Check if a `not-found.tsx` exists at the app level before choosing.

---

## 2. Feature Flag Stack Versions and Compatibility

| Layer | Technology | Version |
|---|---|---|
| Config persistence | `config/config.go` `FeatureFlags map[string]bool` | Go 1.22+ |
| Backend RPC | `GetFeatureFlags` / `UpdateFeatureFlag` in `session_service.go` | ConnectRPC (already in prod) |
| Frontend context | `FeatureFlagsContext.tsx` `useFeatureFlag(name)` hook | React 19 / ConnectRPC |
| Settings UI | `/settings/features/page.tsx` toggle | Already live |
| Nav guard | `Navigation.tsx` lines 21–26 | Already implemented |

All layers are already in production. The only missing piece is a page-level guard preventing direct URL access to `/backlog` and `/backlog/board` when the flag is off.

---

## 3. Version Constraints and Compatibility Concerns

- **No version constraints** block this change. The flag infrastructure, hook, and context are all stable and in use.
- **Hydration**: `useFeatureFlag` calls `useContext(FeatureFlagsContext)`. The context fetches flags on mount; during SSR/hydration the `flags` map is `{}`, so `useFeatureFlag("backlog")` returns `false` initially. The guard must handle the loading state (show nothing / spinner) to avoid a flash where the page renders then redirects. Check `isLoading` from `useFeatureFlags()` before redirecting.
- **`"use client"` on layout**: Next.js 15 App Router allows Client Component layouts. Adding `"use client"` to `app/backlog/layout.tsx` is valid and has no negative side effects on page rendering since pages that need server components can still use them inside Client Component layouts (they just cannot be async themselves in that subtree).
- **Board page**: `app/backlog/board/page.tsx` is also unguarded. The layout guard covers it automatically — no change needed to board's page file.

---

## 4. Default-False Guarantee

**Yes, it is already guaranteed by absent-key semantics.**

`config/config.go` line 788–797:
```go
// GetFeatureFlag returns the persisted enabled state of the named feature flag.
// Absent key returns false — all feature flags default to disabled.
func (c *Config) GetFeatureFlag(name string) bool {
    if c == nil || c.FeatureFlags == nil {
        return false
    }
    return c.FeatureFlags[name] // map lookup: missing key → zero value (false)
}
```

On the frontend, `useFeatureFlag` at line 94 of `FeatureFlagsContext.tsx`:
```ts
return flags[name] ?? false;
```

Both the backend (Go map lookup) and the frontend (`?? false` fallback) independently default to `false` for any unrecognized or unset flag. New installs with no `config.json` entry for `"backlog"` will see the flag as disabled at both layers without any additional code.

The `FeatureFlagsProvider` also initializes `flags` as `{}` (empty object), so before the first `getFeatureFlags` RPC completes, `useFeatureFlag("backlog")` returns `false`. This means the guard is safe to apply immediately on mount without waiting for the load.

---

## Summary of Required Changes

| File | Change |
|---|---|
| `web-app/src/app/backlog/layout.tsx` | Add `"use client"`, `useFeatureFlag("backlog")` guard → `notFound()` or redirect if disabled |
| `web-app/src/app/backlog/page.tsx` | No change needed (covered by layout guard) |
| `web-app/src/app/backlog/board/page.tsx` | No change needed (covered by layout guard) |

No backend changes required. No proto changes required. The flag is already registered in `knownFeatureFlags` in `session_service.go`.
