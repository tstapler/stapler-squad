# Pitfalls Research: sexy-ui Redesign

_Date: 2026-05-14_

---

## 1. CSS Architecture Overview

### File counts

| Type | Count |
|------|-------|
| `.css.ts` (vanilla-extract) | 117 |
| `.module.css` | 0 |

The entire component layer is already on vanilla-extract. There are **no** legacy CSS Modules to migrate. All component styles consume `vars` from `theme-contract.css.ts` via `import { vars } from "@/styles/theme.css"`.

---

## 2. High-Risk CSS Variables in globals.css

`globals.css` defines ~60 CSS custom properties in `:root`. These are **bridge variables** — they were kept to serve:
- Utility classes (`.card`, `.btn`, `.modal`, `.input`, `.spinner`, `.text-*`)
- Runtime-set layout variables consumed in `.css.ts` files

### Critical bridge vars (46 uses in `.css.ts` files — do NOT rename)

| Variable | Risk | Why |
|---|---|---|
| `--header-height` | **HIGH** | 46 occurrences in `.css.ts` files via `var(--header-height)`; also set at runtime from CSS and overridden by `@media (max-width: 1024px)`. Renaming breaks layout calculations across all pages. |
| `--bottom-nav-height` | **HIGH** | Set at runtime by BottomNav's ResizeObserver; consumed in layout calcs. |
| `--viewport-height` | **HIGH** | Set at runtime by ViewportProvider via `visualViewport` API; fallback is `100dvh`. |
| `--keyboard-height` | MEDIUM | Runtime-set; referenced in mobile layout. |
| `--safe-area-*` | MEDIUM | Wraps `env(safe-area-inset-*)` — referenced in modal and page layout. |
| `--card-index` | MEDIUM | Set via inline style on each card for stagger animation; referenced in `SessionCard.css.ts` via `var(--card-index, 0)`. |
| `--font-mono` | LOW | Referenced in `globals.css` body rule; overridden per-theme in `theme.css.ts`. |

**Migration rule**: Do NOT remove or rename `--header-height`, `--bottom-nav-height`, `--viewport-height`, `--card-index`, `--safe-area-*`, or `--keyboard-height` from `globals.css`. They are runtime-set or used in `calc()` expressions inside `.css.ts` files where `vars.xxx` cannot substitute (CSS custom properties cannot be used inside `@media` queries — this is explicitly documented in `theme-contract.css.ts`).

### Color vars in globals.css: already superseded

The color tokens (`--background`, `--primary`, `--text-primary`, etc.) in `globals.css` are **light-mode defaults** that are never read by components — all components use `vars.color.*` from vanilla-extract. These exist only to support the utility classes (`.btn`, `.card`, etc.) and the `body { color: var(--foreground); background: var(--background) }` rule.

**Risk**: If you change the values of `--background` / `--foreground` / `--primary` in `globals.css`, the utility classes and body rule change, but component styles are unaffected. This asymmetry is a silent inconsistency risk — the theme-background e2e test (`theme-background.spec.ts`) explicitly guards against `body` being white under dark themes via `localStorage.setItem('stapler-theme', 'dark')` before navigation.

---

## 3. Vanilla-extract + Next.js: Known Pitfalls

### 3.1 FOUC / Theme class hydration mismatch

**Current state**: The app has a FOUC-prevention inline script in `layout.tsx` (`foucScript`) that reads `localStorage.getItem('stapler-theme')` and applies the correct vanilla-extract hashed class to `<html>` before React hydrates. The `<html>` element uses `suppressHydrationWarning`.

**Pitfall**: Adding a new theme (e.g., `"linear"`) requires updates in **four** places atomically:
1. `theme.css.ts` — `createTheme(vars, {...})` export
2. `ThemeContext.tsx` — `ThemeName` union and `THEME_CLASSES` map
3. `layout.tsx` — `themeMapJson` object and the `<html className>` attribute
4. A new fixture file in `tests/e2e/fixtures/<name>-theme.json`

If any of these are out of sync, the FOUC script will fall back to `'matrix'` silently and the SSR-rendered class will mismatch the client-applied class — producing a React hydration warning even though `suppressHydrationWarning` is set on `<html>` (it only suppresses `<html>`-level attribute warnings, not descendant mismatches).

**Mitigation**: The `cleanTheme` in `theme.css.ts` is already very close to the Linear/Vercel target (dark charcoal background `#0f0f11`, violet accent `#7c3aed`). Consider **reusing** `cleanTheme` or **renaming** it to `"linear"` rather than adding a fourth dark theme, which would require updating all four places plus a new Playwright visual-regression project.

### 3.2 CSS ordering and specificity with vanilla-extract

Vanilla-extract extracts all styles at build time into a single CSS bundle. The order of class application determines which `createTheme` wins when multiple theme classes are present on `<html>`. The FOUC script removes all known theme classes before adding the new one, so ordering is not a problem during runtime switching — but **during SSR**, the `<html>` element is rendered with the `matrixTheme` class. If the new `linearTheme` class is added to the `<html>` className in `layout.tsx` but not removed by the FOUC script fast enough, users will see a flash of the matrix palette.

**Mitigation**: The existing pattern (FOUC script removes all theme classes then adds the persisted one) handles this correctly — as long as the new theme's class string is included in `allThemeClasses`.

### 3.3 Build-time extraction and dynamic values

vanilla-extract styles are extracted at build time — there is no runtime CSS-in-JS. This means **any color or size that varies at runtime** cannot be in a `.css.ts` file unless it is exposed via a CSS custom property bridge (as `--card-index` and `--viewport-height` are).

**Pitfall for REQ-2 (compact rows)**: The 36–40px row height must be a static value in the `.css.ts` file. Row heights cannot be computed from data at build time. This is safe — but if the hover "inline action row" for REQ-2 is implemented by toggling height (e.g., from `0` to `auto`), `max-height` transitions cannot animate to `auto` — use explicit pixel values instead.

---

## 4. SessionCard Layout Coupling (REQ-2 Risk)

### Current card structure

The `SessionCard` component is a **multi-line card**, not a compact row:
- `card` wrapper: `padding: 16px`, `marginBottom: 12px`, `border-radius: 12px`
- `body` div with `info` column containing multiple `infoRow` elements (Program, Branch, Path, Working Dir, Repository, Pull Request, Cloned To)
- `footer` with timestamps (flex-direction: column)
- A collapsible terminal snapshot pane (`snapshotSection`, 120px fixed height)
- An overflow action menu

**Coupling to data model**: Row content is driven by conditional rendering on session fields (`session.branch`, `session.program`, `session.path`, `session.workingDir`, etc.). Collapsing this to a single-line 36–40px row means:
- The `body`, `info`, `infoRow`, `label`, `value`, and `footer` sections must be hidden or restructured
- The terminal snapshot preview (`snapshotSection`, 120px) must be removed from the list view entirely
- The `desktopActions` overflow menu must move to a hover-revealed inline strip

**Structural risk**: `SessionCard.tsx` is a 700+ line component with many consumers (selection mode, external session indicators, rate-limit badges, review queue badges, fork dialog). A full structural rewrite carries regression risk. Recommended approach: **create a new `SessionRow.tsx`** component for the compact list view and use a feature flag or `viewMode` prop to toggle between `SessionCard` (existing) and `SessionRow` (new compact row), rather than rewriting the existing card.

**CSS coupling**: The existing `SessionCard.css.ts` uses `vars.space["4"]` (16px) padding and `vars.radii.lg` (12px) border-radius for the card. These must both change to achieve 36–40px row height. No `height` or `min-height` is set on the `card` class — height is entirely driven by content. This means REQ-2 row density is achievable purely through content restructuring, not CSS overrides.

---

## 5. Visual Regression Test Breakage

### Committed snapshot baselines

Two snapshot files are committed to git at `tests/e2e/tests/snapshots/chromium/visual-regression.spec.ts/`:
- `session-list-empty.png`
- `omnibar-open.png`

These are taken in the `chromium` project (not the themed `visual-matrix/cyberpunk77/wh40k/clean` projects). The `visual-regression.spec.ts` tests use `toHaveScreenshot` with `maxDiffPixelRatio: 0.01` — **any** visual change to the empty state or the omnibar will fail CI.

**REQ-1 (theme overhaul) will break both snapshots.** The snapshots must be regenerated with `--update-snapshots` after the theme change and committed. This is expected and documented in the spec file header.

**Note**: The four themed visual-regression projects (`visual-matrix`, `visual-cyberpunk77`, `visual-wh40k`, `visual-clean`) do NOT have fixture files for a new `"linear"` theme. If a new theme is added, a new fixture file and a new Playwright project must be added. However, if the approach is to update `cleanTheme` values rather than add a new theme, the `visual-clean` project's baseline must also be regenerated.

### theme-background.spec.ts

This test checks that `#main-content` background is not `rgb(255, 255, 255)` under each dark theme. It reads from `localStorage` before page load. When the theme palette changes, the computed `backgroundColor` value changes but the test only checks `!== white` — **this test will not break** as long as the new theme backgrounds are non-white (which they will be).

### touch-targets.spec.ts

One assertion uses a CSS-class-based selector: `page.locator('[class*="sessionCard"]')`. This matches the hashed vanilla-extract class name if it contains "sessionCard" as a substring. **Vanilla-extract hashes class names at build time** — the generated class for `export const card = style({...})` in `SessionCard.css.ts` will be something like `SessionCard_card__abc123`, which does contain "sessionCard" as a case-insensitive substring. However if the component is renamed or moved, this brittle selector breaks silently (test passes vacuously if the locator returns 0 elements because the test body wraps in a conditional). **Mitigation**: After REQ-2 (creating `SessionRow.tsx`), verify this locator still resolves or add a `data-testid="session-row"` to the new component.

### accessibility.spec.ts (Axe Core)

Runs WCAG 2.1 AA checks on the main page and `/review-queue`. The new indigo/violet accent (#6366f1) has a contrast ratio of ~4.7:1 on `#0f1117` background — which **passes WCAG AA** for large text (3:1) and is borderline for small text (4.5:1 required). Several tokens in the existing themes were already corrected for WCAG compliance (e.g., `textTertiary: "#767676"` comment shows prior violations). The new Linear palette must be verified for contrast before the Axe gate runs.

**High-risk contrast pairs to verify**:
- Indigo `#6366f1` on `#0f1117` background: ~4.7:1 (passes AA for body text minimally)
- Muted text `#475569` on `#0f1117`: ~3.8:1 (FAILS AA for body text — use `#94a3b8` or lighter)
- Secondary text `#94a3b8` on `#0f1117`: ~5.6:1 (passes)

---

## 6. CSS Variable Rename Pitfalls

### What `lint:css` catches

The `make lint:css` step validates that all `var(--xxx)` references in `.css.ts` and `.css` files resolve to defined tokens. It will catch:
- Renamed globals.css variables that are still referenced somewhere
- Typos in `var()` calls

### What it does NOT catch (runtime-only issues)

1. **Inline styles in TSX/TSX**: Any `style={{ color: 'var(--primary)' }}` or `style={{ background: 'var(--card-background)' }}` in component JSX files are **not linted** by `lint:css`. These would silently resolve to the `:root` fallback values (light mode colors) regardless of theme.

   Search shows the body in `layout.tsx` uses inline styles: `style={{ flex: 1, overflow: "hidden", display: "flex", flexDirection: "column" }}` — no color tokens there. But worth a global search for `style=.*var(--` before finalizing.

2. **globals.css utility classes**: The `.card`, `.btn`, `.btn-primary`, `.input`, etc. utility classes in `globals.css` use color tokens like `var(--primary)`, `var(--card-background)`. These utility classes are NOT used by any `.css.ts` component (components use `vars.color.*` exclusively) but could be used in ad-hoc TSX through `className="card"`. There is no automated check for this.

3. **`--status-badge-*` variables**: `globals.css` defines 17 `--status-badge-*` variables (e.g., `--status-badge-approval-bg`, etc.) that mirror the `statusBadge.*` tokens in the theme contract. These are bridge vars kept for the `:root` fallback. They are not in the documented "defined tokens" list in `css-architecture.md` — if a developer unknowingly uses one in a new component, `lint:css` will pass (they are defined) but they won't theme-switch correctly.

4. **The `Header.css.ts` local override**: `Header.css.ts` overrides `vars.color.textPrimary` and `vars.color.textSecondary` inline using vanilla-extract's `vars:` property (CSS custom property override at the component level):
   ```ts
   vars: {
     [vars.color.textPrimary]: "#ededed",
     [vars.color.textSecondary]: "#b4b4b4",
   }
   ```
   Additionally, `backgroundColor: "rgba(26, 26, 26, 0.95)"` is **hardcoded** (not a token). This value won't change when the palette changes unless explicitly updated. Under the new slate palette, the header should use approximately `rgba(15, 17, 23, 0.95)` (the `#0f1117` background at 95% opacity).

---

## 7. Radix UI CSS Variable Conflicts

Radix UI is used minimally: `@radix-ui/react-dialog` (for `Modal.tsx`) and `@radix-ui/react-slot` (for `Button.tsx`). Radix UI injects its own CSS custom properties for portal z-index and animation state (`--radix-dialog-content-transform-origin`, etc.). These are **namespaced under `--radix-`** and will not conflict with any `--text-*`, `--primary`, or `--background` tokens. No risk here.

---

## 8. localStorage + SSR: Onboarding Flow (REQ-4)

### Current pattern (ThemeContext)

The existing `ThemeContext` correctly handles the SSR/localStorage pitfall:
- SSR renders with `initialTheme = "matrix"` (no localStorage access)
- The inline FOUC script runs synchronously before React hydration and applies the stored theme class
- `useEffect` (client-only) reads `localStorage` after mount

### REQ-4 Onboarding pitfall

The onboarding flow needs a `localStorage.getItem('stapler-squad:onboarded')` check. If this check happens **during render** (not in `useEffect`), it will cause an SSR hydration mismatch: the server renders the non-onboarded state, the client reads `false` from localStorage and renders the onboarding modal — React will complain about the mismatch.

**Required pattern** (same as ThemeContext):
```tsx
const [showOnboarding, setShowOnboarding] = useState(false); // false on SSR
useEffect(() => {
  if (!localStorage.getItem('stapler-squad:onboarded')) {
    setShowOnboarding(true);
  }
}, []);
```

**Do NOT** do:
```tsx
// BAD — reads localStorage during render, causes SSR mismatch
const show = typeof window !== 'undefined' && !localStorage.getItem('stapler-squad:onboarded');
```

The `typeof window !== 'undefined'` guard prevents the SSR crash but still causes a hydration mismatch because the server and client render different initial states.

---

## 9. Settings Consolidation (REQ-3)

### Config persistence

The settings are persisted to `~/.stapler-squad/config.json` via the Go backend (ConnectRPC). REQ-3 specifies "no breaking changes to ConnectRPC API surface" — the consolidation is purely a UI routing change and does not require new RPCs. Risk: if old routes (`/config`, `/preferences`) were bookmarked or linked, they should redirect to `/settings` rather than 404.

### No ThemePicker conflict

`ThemePicker.css.ts` exists as a settings component. A new "linear" theme entry in `ThemePicker` would require adding to the `availableThemes` list in `ThemeContext`.

---

## 10. Recommended Mitigations Summary

| Pitfall | Severity | Mitigation |
|---|---|---|
| Visual regression baselines will fail | **BLOCKING** | Run `--update-snapshots` after REQ-1 for all themed projects; commit new baselines as a dedicated commit |
| Axe contrast failures with new palette | **HIGH** | Pre-check `#94a3b8` on `#0f1117` and `#6366f1` on `#0f1117` using a contrast checker before implementing; avoid `#475569` as body text color |
| `--header-height` / `--viewport-height` / `--card-index` must stay in globals.css | **HIGH** | Never rename or remove these; they are runtime-set and can't live in the theme contract |
| Header.css.ts hardcoded `rgba(26,26,26,0.95)` backdrop | **HIGH** | Update to `rgba(15,17,23,0.95)` (or a new `vars.color.headerBackdrop` token) when changing the slate palette |
| SessionCard is multi-line; REQ-2 requires full structural refactor | **HIGH** | Create `SessionRow.tsx` as a new component; do not rewrite `SessionCard.tsx` in place |
| `[class*="sessionCard"]` brittle selector in touch-targets.spec.ts | **MEDIUM** | Add `data-testid="session-row"` to new SessionRow; update the locator |
| New theme adds 4-place sync requirement (theme.css.ts, ThemeContext, layout.tsx, fixture) | **MEDIUM** | Prefer extending `cleanTheme` (rename to "linear") rather than adding a fifth theme |
| Onboarding localStorage read during render (SSR mismatch) | **MEDIUM** | Always gate localStorage reads with `useEffect`; never read during render |
| Hardcoded hex colors in ApprovalAnalyticsPanel.css.ts (3 values) and debug page (10 values) | **LOW** | ApprovalAnalyticsPanel uses chart colors intentionally; debug page is dev-only — both acceptable |
| `--status-badge-*` bridge vars not in documented token list | **LOW** | Document in `css-architecture.md`; do not use in new components |
