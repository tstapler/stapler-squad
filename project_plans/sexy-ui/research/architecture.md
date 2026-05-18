# Architecture Research — sexy-ui Redesign

## 1. Routing Framework

**Framework**: Next.js App Router (not Pages Router, not React Router).

Evidence: `web-app/src/app/` uses the file-system-based App Router convention — every route has a `page.tsx` and optional `layout.tsx`. The root `layout.tsx` is a Server Component that injects the FOUC-prevention script.

**Current route inventory** (all under `web-app/src/app/`):

| Route | File | Notes |
|---|---|---|
| `/` | `page.tsx` | Session list (main view) |
| `/config` | `config/page.tsx` | Claude config file editor (Monaco) + network info + passkey mgmt |
| `/settings` | `settings/page.tsx` | Redirects to `/settings/defaults` |
| `/settings/defaults` | `settings/defaults/page.tsx` | ThemePicker + GlobalDefaultsForm + Profiles + DirectoryRules + PushNotifications |
| `/settings/unfinished` | `settings/unfinished/page.tsx` | |
| `/account` | `account/page.tsx` | |
| `/review-queue` | `review-queue/page.tsx` | |
| `/unfinished` | `unfinished/page.tsx` | |
| `/rules` | `rules/page.tsx` | |
| `/history` | `history/page.tsx` | |
| `/logs` | `logs/page.tsx` | |
| `/errors` | `errors/page.tsx` | |
| `/notifications` | `notifications/page.tsx` | |
| `/sessions/new` | `sessions/new/page.tsx` | |
| `/login` | `login/page.tsx` | |
| `/debug/escape-codes` | debug route | |
| `/test/*` | various test pages | |

**No `/help` route exists yet.** The requirements call for a new `/help` route (REQ-6).

Navigation is defined in `web-app/src/lib/routes.ts` (type-safe route constants) and `web-app/src/lib/nav-pages.ts` (icon + label + visibility per surface).

---

## 2. Current Settings / Config Page Layout

**Two separate pages** exist where requirements call for one:

### `/config` — `web-app/src/app/config/page.tsx`
Full-featured page containing:
- Monaco editor for Claude config files (CLAUDE.md, settings.json, etc.)
- Network & Remote Access info (HTTPS URL, CA cert, hostnames, programs)
- Passkey Security (register/revoke passkeys)

This page registers its own local `window.addEventListener('keydown', ...)` for Ctrl+S / Ctrl+1-9 / Ctrl+[/] shortcuts — these are NOT in the centralized `ShortcutRegistry`.

### `/settings/defaults` — `web-app/src/app/settings/defaults/page.tsx`
Contains:
- `ThemePicker` — vanilla-extract theme selector
- `GlobalDefaultsForm` — session defaults
- `ProfilesManager` — named profiles
- `DirectoryRulesManager` — per-directory rules
- `PushNotificationSettings` — web push config

The `/settings` root immediately redirects to `/settings/defaults`; there is no tab structure yet.

**For REQ-3**, the consolidation plan is:
- Create `/settings` as a tabbed layout: General | Config Files | Sessions | Appearance | Keyboard Shortcuts
- The `/config` page content (Monaco editor + network + passkeys) moves into a "Config Files" tab
- The `/settings/defaults` content moves into other tabs
- Both old routes redirect to the new unified `/settings`

---

## 3. Green Color Sources

The matrix-green palette is **entirely contained inside the vanilla-extract theme system**, NOT scattered as hardcoded hex values throughout components. This is the critical architectural advantage for the redesign.

### Primary source: `matrixTheme` in `web-app/src/styles/theme.css.ts`
The `matrixTheme` object (lines 232–325) defines all green values:
- `textPrimary: "#00ff41"`, `textSecondary: "#00cc33"`, `textMuted: "#00b32d"`
- `primary: "#00ff41"`, `primaryHover: "#33ff66"`, `primaryActive: "#00cc33"`
- `success: "#00ff41"`, `inputText: "#00ff41"`, `terminalCursor: "#00ff41"`
- `glowPrimary: "rgba(0,255,65,0.5)"`, `scanlineColor: "rgba(0,255,65,0.03)"`

### Secondary sources (isolated, few instances):
1. `web-app/src/components/sessions/TerminalOutput.tsx` line 878 — `console.log` debug message uses `"color: #00ff00"` (debug only, no user impact)
2. `web-app/src/components/sessions/SessionDetailView.tsx` lines 830, 864 — inline styles use `'var(--color-success, #22c55e)'` as fallback. The `--color-success` var is not defined in `globals.css`, so the `#22c55e` fallback fires. These two instances need fixing.

### Theme application mechanism:
`ThemeContext.tsx` applies themes by adding/removing CSS class strings on `document.documentElement`. The `matrixTheme` is the SSR default class in `layout.tsx` (line 50): `className={matrixTheme ...}`.

**Migration path for REQ-1**: The existing `cleanTheme` in `theme.css.ts` is already a near-match for the Linear/Vercel aesthetic (purple accent `#7c3aed`, deep charcoal `#0f0f11`, Inter font). The redesign should update `cleanTheme` to use the exact indigo/violet palette from the requirements and make it the default. Alternatively, update `darkTheme` to match Linear. Either way, only values in `theme.css.ts` change; components pick up the new tokens automatically.

---

## 4. Keyboard Shortcut Registry

**Fully centralized** in `web-app/src/lib/shortcuts/shortcutRegistry.ts`.

### Architecture:
- `ShortcutRegistry` class with a **singleton** export (`registry`) that registers one `document.addEventListener("keydown", ...)` for the entire app.
- `useShortcut(id, shortcut)` hook in `web-app/src/lib/shortcuts/useShortcut.ts` — components call this on mount to register/deregister their shortcuts.
- Shortcuts have `context: ShortcutContext` where `ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit"`.
- Context detection walks up the DOM from `document.activeElement` looking for `data-context` attribute.
- `registry.getAll()` returns all registered shortcuts grouped by context — this is what `KeyboardShortcutOverlay` renders.

### Overlay component:
`web-app/src/components/ui/KeyboardShortcutOverlay.tsx` — already implements the `?` cheatsheet panel with:
- Search filtering across all shortcuts
- Context grouping headings
- Escape to close, focus trap
- Uses `registry.getAll()` as its source of truth

The `?` trigger shortcut needs to be wired into the registry (currently not registered there — need to check if it's registered elsewhere or missing entirely).

### Exception: `/config` page local shortcuts
The config editor page registers `Ctrl+S / Ctrl+1-9 / Ctrl+[/]` via raw `window.addEventListener` rather than the registry. These are editor-level shortcuts that should remain local to avoid conflicts with global shortcuts.

---

## 5. Vanilla-Extract `createThemeContract` Pattern — How Theme Propagation Works

### Contract definition: `web-app/src/styles/theme-contract.css.ts`
`createThemeContract({ ... })` creates typed CSS custom property placeholders (all `null`). The exported `vars` object holds typed references like `vars.color.textPrimary` that resolve to the actual CSS custom property name at build time.

### Theme implementations: `web-app/src/styles/theme.css.ts`
`createTheme(vars, { ... })` generates a unique CSS class where each contract slot gets a concrete value. There are currently **6 themes**: `lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`.

### Propagation to components:
Any `.css.ts` file that imports `vars` from `theme-contract.css.ts` and uses `vars.color.xxx` will automatically pick up whatever theme class is on `<html>`. The vanilla-extract build step replaces `vars.color.textPrimary` with the CSS custom property reference; the theme class on `<html>` sets the actual value.

### Adding a "Linear/Vercel" theme:
Option A: Update `cleanTheme` values in `theme.css.ts` — touches one object, all 87 component `.css.ts` files that use `vars.color.*` update automatically. No per-component changes needed.

Option B: Add a new `linearTheme` object alongside existing themes, update `ThemeContext.tsx` to include it, update `layout.tsx` to reference it.

The requirements want "one well-designed dark theme" — updating `darkTheme` or `cleanTheme` in place is the lowest-friction approach and avoids a new route in `ThemeContext`.

### Note on `globals.css` bridge variables:
`globals.css` still defines ~60 CSS variables in `:root` for legacy compatibility. The comment at the top explains these are "bridge variables" that map legacy names so any remaining inline-style references still resolve. For REQ-1, these also need updating to match the new slate palette, specifically: `--primary` (currently `#0070f3`), `--success` (currently `#10b981`), and all background variables.

---

## 6. localStorage-Based Onboarding Flag — Best Practices

No onboarding flow exists in the current codebase. The existing localStorage pattern (from `ThemeContext.tsx`) provides a clean reference model:

### Current pattern (theme persistence):
```typescript
// Read on first mount
const stored = localStorage.getItem(STORAGE_KEY);

// Write on change
localStorage.setItem(STORAGE_KEY, name);
```

Wrapped in `try/catch` to handle private browsing mode / storage quota.

### Recommended pattern for `stapler-squad:onboarded` flag (REQ-4):

**SSR hydration issue**: Reading `localStorage` in a Next.js App Router server component will throw. The pattern must use `"use client"` + `useEffect`:

```typescript
const [showOnboarding, setShowOnboarding] = useState(false); // false = safe SSR default (no flash)

useEffect(() => {
  try {
    const done = localStorage.getItem('stapler-squad:onboarded');
    if (!done) setShowOnboarding(true);
  } catch {
    // private mode — don't show onboarding
  }
}, []); // empty dep array = runs once after hydration
```

**Race condition note**: Initializing state to `false` (hidden) and setting to `true` in `useEffect` avoids the SSR/client mismatch that causes React hydration errors. The alternative — reading during render via a `useState` initializer function — only works with React 18+ if the component is purely client-rendered, but can still produce hydration warnings when the server and client disagree.

**Re-trigger pattern**: The `?` shortcut and Settings > Help link should call `localStorage.removeItem('stapler-squad:onboarded')` then re-render the onboarding modal.

### Existing localStorage usage in this codebase:
- `stapler-theme` key — theme persistence (ThemeContext)
- `SessionList` component uses localStorage for grouping/filter state (with instance-prefixed keys)

No collision risk with `stapler-squad:onboarded`.

---

## 7. Component Impact Scope

**Total component files**: 147 `.tsx` files in `web-app/src/components/`

**Total CSS files using `vars.color.*`**: 87 `.css.ts` files in `components/` + 10 in `styles/`

### High-impact areas (most color/spacing references):

| Directory | Description | Estimated impact |
|---|---|---|
| `components/sessions/` | Session list, detail view, terminal, omnibar — core UI | High — every file touches color |
| `components/layout/` | Header, CockpitShell, BottomNav, WorkspaceSwitcher | High — structural chrome |
| `components/ui/` | Shared UI primitives (buttons, badges, overlays) | High — design system foundation |
| `components/settings/` | Settings forms, ThemePicker, etc. | Medium — will be restructured |
| `components/history/` | History cards, filter bar, group view | Medium |
| `styles/` | Pane layout, session cockpit, animations | High — shared layout tokens |

### Components NOT needing theme changes:
- Terminal rendering components (xterm.js, TerminalOutput) — terminal colors are intentionally hardcoded to `terminalTokens` constants that remain dark regardless of theme
- Debug/test pages (`/debug/`, `/test/`)

### Two instances requiring direct fixes (not covered by theme token swap):
1. `components/sessions/SessionDetailView.tsx` — 2 inline styles with `var(--color-success, #22c55e)` fallback; replace with `vars.color.success`
2. `components/sessions/TerminalOutput.tsx` — 1 `console.log` debug string with `#00ff00` (cosmetic, low priority)

---

## 8. Help / Docs Route

**No `/help` route currently exists.** Neither `routes.ts` nor `nav-pages.ts` references a help or docs route.

For REQ-6, the full implementation requires:
1. Add `help: "/help"` to `routes.ts`
2. Add `app/help/page.tsx` and `app/help/layout.tsx`
3. Create markdown doc files (probably in `web-app/public/docs/` or `web-app/src/content/`)
4. Add to `nav-pages.ts` (or expose only from Settings + onboarding flow)

The app currently has no markdown rendering infrastructure — would need `react-markdown` or similar, or a simpler approach using pre-rendered HTML via Next.js `remark`/`mdx` pipeline.

---

## Summary

### Theme system is the critical path
The vanilla-extract `createTheme(vars, {...})` pattern means the entire visual redesign (REQ-1) reduces to editing color values in `theme.css.ts` and bridge values in `globals.css`. All 87 component CSS files automatically pick up the new tokens. Only 2 inline style instances in `SessionDetailView.tsx` need individual attention.

### Settings consolidation is structural (REQ-3)
Two separate pages must merge: `/config` (Monaco editor + network + security) and `/settings/defaults` (form-based settings). The target is a single `/settings` route with a tab or sidebar-section layout. The `/config` page's local keyboard shortcuts (Ctrl+S, etc.) are editor-level and should remain local.

### Onboarding and help are net-new (REQ-4, REQ-5, REQ-6)
No onboarding modal, no `/help` route, and the `?` shortcut overlay (`KeyboardShortcutOverlay`) already exists but the `?` trigger shortcut needs to be wired into the centralized `ShortcutRegistry`. The localStorage pattern for the onboarding flag should follow the same `useEffect`-guarded pattern as the existing theme persistence to avoid SSR hydration mismatches.
