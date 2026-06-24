# Feature Flag Research: Put Backlog Behind a Feature Flag by Default

## 1. Existing Feature-Gating Patterns

### `useFeatureFlag` hook
`web-app/src/lib/contexts/FeatureFlagsContext.tsx` exports two hooks:
- `useFeatureFlags()` — returns the full `{ flags, flagList, isLoading, error, setFlag }` context
- `useFeatureFlag(name: string): boolean` — single-flag convenience hook; returns `flags[name] ?? false`, so an absent key defaults to `false` (flag off)

### Nav-level gating (the current pattern)
There are two nav gating systems and both already gate backlog correctly:

**Navigation.tsx** (desktop sidebar): calls `useFeatureFlag("backlog")` and conditionally includes the backlog item in `navItems`. This is a one-off usage specific to that component.

**NAV_PAGES / Header.tsx / BottomNav.tsx** (newer system): `nav-pages.ts` defines a `featureFlag?: string` field on each `NavPage`. The backlog entry is:
```ts
{ href: routes.backlog, label: "Backlog", icon: LayoutList, bottomNavPrimary: true, featureFlag: "backlog" }
```
`Header.tsx` filters with `NAV_PAGES.filter((p) => !p.featureFlag || flags[p.featureFlag])`. `BottomNav.tsx` does the same.

### `browser-passthrough` — no page gate found
The `browser-passthrough` flag exists in `knownFeatureFlags` in `session_service.go` (line 3918) and in `NAV_PAGES` via `featureFlag`, but there is no frontend page-level or route-level gate on it beyond the nav filter. It follows the same "nav hidden but page accessible" pattern as backlog currently does. This flag is not a model for additional route guards.

### `backlog:conversation-view` — sub-feature gate
`web-app/src/components/backlog/SessionMonitor.tsx` uses `useFeatureFlag("backlog:conversation-view")` to switch UI rendering within a component. This is the only existing example of a non-nav `useFeatureFlag` call gating rendered content.

---

## 2. The Gap: "Nav Hidden" vs "Truly Hidden"

With the flag off, these surfaces remain fully accessible by direct URL:

| Surface | File | Status |
|---|---|---|
| `/backlog` | `web-app/src/app/backlog/page.tsx` | No flag check — renders unconditionally |
| `/backlog/board` | `web-app/src/app/backlog/board/page.tsx` | No flag check — renders unconditionally |
| All BacklogService RPCs | `server/services/backlog_service.go` | No flag guard in any of the ~20 handler methods |

A user who knows the URL can navigate directly to `/backlog` and use the full feature regardless of the flag state.

---

## 3. Patterns to Reuse

### Simplest page gate (client component)
Both backlog pages are `"use client"` components. The pattern from `SessionMonitor.tsx` applies directly:

```tsx
// In BacklogPage / BacklogBoardPage, at the top of the render function:
const backlogEnabled = useFeatureFlag("backlog");
if (!backlogEnabled) {
  // Option A: redirect
  router.replace("/");
  return null;
  // Option B: 404-style
  // return notFound(); // only available in server components
  // Option C: render a disabled notice
}
```

`useRouter` is already imported in both pages. `useFeatureFlag` is already imported in Navigation.tsx and SessionMonitor.tsx — same import path works in the page files.

### Loading state handling
`FeatureFlagsContext` exposes `isLoading`. During initial load, `flags["backlog"]` is `undefined`, which `useFeatureFlag` maps to `false`. This means the page would briefly redirect/blank before flags load. To avoid a flash:

```tsx
const { isLoading } = useFeatureFlags();
const backlogEnabled = useFeatureFlag("backlog");
if (isLoading) return null; // or a loading spinner
if (!backlogEnabled) { router.replace("/"); return null; }
```

### Settings toggle
The settings page at `/settings/features` already renders all `knownFeatureFlags` entries with a toggle. No changes needed there — backlog is already listed.

---

## 4. Similarity: Nav Gate vs Page Gate

The nav gate in `Navigation.tsx` (line 26) is:
```tsx
...(backlogEnabled ? [{ href: routes.backlog, label: "Backlog" }] : [])
```

The page gate is structurally the same — call `useFeatureFlag("backlog")`, branch on the result. The only difference is the action on false: the nav omits the link; the page should redirect or render nothing.

Both `backlog/page.tsx` and `backlog/board/page.tsx` need identical guards since board is reachable via the tab bar inside the list page.

---

## 5. Whether Backend RPC Gating Is Needed

**Short answer: no, for this task.**

The existing threat model for this codebase is single-user / local network. The feature flags are a UX/opt-in control, not a security boundary — the settings page lets any authenticated user enable or disable any flag. Backend RPC gating would add complexity without meaningful protection since:

1. The RPC endpoints are only reachable on `localhost:8543` (or LAN/Tailscale).
2. No other feature flag (including `browser-passthrough`) has backend RPC guards.
3. The flag can be toggled from the settings page at any time.

Frontend page guards (redirecting away from `/backlog` and `/backlog/board` when the flag is off) are the appropriate and sufficient scope for this task. Backend gating would be a separate, larger change and would require threading `config.Config` or a feature flag check into every `BacklogService` method.
