# Stack Research: sexy-ui Redesign

_Research date: 2026-05-14_

---

## 1. Current Dependency Inventory

### Runtime framework
- **Next.js 15.3.2** (App Router) + **React 19**
- **TypeScript 5.9.3**

### CSS / styling
| Package | Version | Role |
|---|---|---|
| `@vanilla-extract/css` | `^1.20.1` (devDep — build-time only) | Primary new-component styling |
| `@vanilla-extract/recipes` | `^0.5.7` (runtime) | `recipe()` variant API |
| `@vanilla-extract/next-plugin` | `^2.5.1` | Next.js integration |
| CSS Modules (via Next.js) | built-in | Legacy component styles |
| `stylelint` + `stylelint-config-standard` | `^17.6.0` | CSS lint (`lint:css`, `lint:css-vars`) |

### Animation
**No Framer Motion or any dedicated animation library is installed.** The project already uses:
- CSS `transition` properties in `.css.ts` files (120–200ms ease, reduced-motion aware)
- vanilla-extract `keyframes()` for pulse/glow/slide animations (see `animations.css.ts`)
- `@media (prefers-reduced-motion: reduce)` guards throughout `interactiveBase.css.ts`

### Icon library
- `lucide-react ^1.14.0` — tree-shakeable SVG icons, no CSS overhead

### Other relevant deps
- `fuse.js ^7.3.0` — client-side fuzzy search (directly applicable to REQ-6 docs search)
- `@radix-ui/react-dialog ^1.1.15` — accessible modal primitive (directly applicable to REQ-4 onboarding modal)
- `react-hook-form ^7.63.0` + `zod ^4.1.11` — form validation (Settings consolidation REQ-3)
- `shiki ^4.0.2` — syntax highlighting (docs hub REQ-6)

---

## 2. Current Theme Architecture

### Pattern: `createThemeContract` + per-theme `createTheme`

The codebase uses the canonical multi-theme pattern correctly:

```
theme-contract.css.ts   → createThemeContract(vars)   — contract/blueprint, no CSS emitted
theme.css.ts            → createTheme(vars, {...})     × 6 themes  — each returns a CSS class name
ThemeContext.tsx         → applies theme class to document.documentElement at runtime
```

**Existing themes**: `matrix` (default), `dark`, `light`, `clean` (purple-accent slate), `cyberpunk77`, `wh40k`

The **`cleanTheme`** in `theme.css.ts` is the closest existing analogue to the Linear/Vercel target:
- Background: `#0f0f11` (deep charcoal, close to the `#0f1117` target)
- Card surface: `#1a1a1f`
- Primary: `#7c3aed` (violet — matches the indigo/violet accent requirement)
- Text: `#ededed` / `#b4b4b4` / `#8a8a8a`

**Key insight**: The `cleanTheme` already exists and is ~80% of the way to the Linear/Vercel palette. The redesign should update `cleanTheme` values (or replace them) and make it the **default** theme, rather than building a new theme from scratch.

### Token contract shape (current `theme-contract.css.ts`)

```
vars.color
  text: textPrimary, textSecondary, textMuted, textDisabled, textTertiary, textInverse
  surface: background, cardBackground, hoverBackground, modalBackground, overlayBackground,
           panelBgSecondary, surfaceSubtle, surfaceMuted
  border: borderColor, borderSubtle, borderMuted, borderStrong, borderHover,
          modalBorder, inputBorder, inputFocusBorder
  action: primary, primaryHover, primaryActive, primaryDark, primaryText
  status: success, successBg, warning, warningBg, warningText, error, errorBg, errorText, errorDark
  accent: accentBg, accentHover
  input: inputBackground, inputText, placeholderColor
  terminal: terminalBackground, terminalForeground, terminalBorder, terminalHeaderBg,
            terminalHeaderFg, terminalTabsBg, terminalTextMuted, terminalHoverBg
  fx: glowPrimary, glowSecondary, scanlineColor, terminalCursor
vars.statusBadge  (12 tokens: approvalBg/Fg/Border, inputBg/Fg/Border, completeBg/Fg/Border, etc.)
vars.font         (mono, sans, display)
vars.space        (0, 1, 2, 3, 4, 6, 8, 12, 16)
vars.radii        (sm=4px, md=6px, lg=12px, full=9999px)
vars.fontSize     (xs=12px, sm=14px, base=14px, lg=16px, xl=20px)
vars.fontWeight   (normal=400, medium=500, semibold=600, bold=700)
vars.shadow       (none, sm, md, lg)
```

**Gaps for the Linear/Vercel redesign** — tokens that need adding to the contract:
- `vars.color.statusDot.running`, `.paused`, `.idle` — for REQ-2 colored status dots
- `vars.transition.fast` (`100ms ease`), `vars.transition.base` (`150ms ease`) — for NTH-1 consistency
- No `vars.fontSize` gap — existing scale covers the 12/14/16px needed for compact rows

### Dual-layer CSS variable system

`globals.css` defines a parallel set of CSS custom properties (`--text-primary`, `--background`, etc.) that serve as bridge variables for legacy CSS Modules. These are **not** auto-synced with vanilla-extract tokens. When the `cleanTheme` values are updated in `theme.css.ts`, the `:root` defaults in `globals.css` must also be updated to match — otherwise CSS Module-based components will show stale colors until the VE class is applied (FOUC risk on SSR).

---

## 3. Animation Approach Recommendation

### Decision: CSS transitions only — do NOT add Framer Motion

**Rationale:**
1. Framer Motion is **not** a current dependency. Adding it would cost ~30KB gzip (or ~15KB with `LazyMotion + domAnimation`). The project has a 5MB JS bundle size limit and actively tracks it with `size-limit`.
2. The requirements explicitly state: _"No new heavy animation libraries (Framer Motion is acceptable if already a dependency; otherwise use CSS transitions)"_.
3. The animation requirements are all micro-interactions: 100–150ms hover transitions, a status dot pulse, and an omnibar fade+scale. These are trivially achievable with CSS.
4. The project already has `animations.css.ts` with `keyframes()` infrastructure and `interactiveBase.css.ts` with `hoverHighlight` / `glowOnHover` utility styles — the animation system is already built.

### What to implement with existing tooling

| Animation | Technique | Duration |
|---|---|---|
| Session row hover (bg + action icons) | CSS `transition: background, opacity` via `hoverHighlight` | 120ms ease |
| Button hover/active states | `interactiveButton` recipe (already exists) | 120ms ease |
| Status dot pulse (running) | `keyframes()` + `pulseGlow` (already exists in `animations.css.ts`) | 2s infinite |
| Omnibar open/close | CSS `opacity` + `scale` + `@starting-style` (Chrome 117+, Safari 17.5+) | 150ms ease |
| Grouping header expand/collapse | CSS `height` transition with `overflow: hidden` | 200ms ease |
| Onboarding modal step transition | CSS `opacity` + `translateX` | 200ms ease-out |

**Note on `@starting-style`**: This native CSS feature enables entry animations without JS and is supported in all modern browsers as of 2025. It is the zero-dep alternative to Framer Motion's `AnimatePresence` for the omnibar open animation. Use it for omnibar and modal mount transitions.

---

## 4. Token Structure: Recommended Updates

### Step 1: Update `cleanTheme` to the target Linear/Vercel palette

Replace the current `cleanTheme` values with the precise target palette from requirements.md:

```typescript
// Recommended final values for cleanTheme (becomes the default "Linear" theme)
color: {
  // Text — matches requirements exactly
  textPrimary:   "#e2e8f0",   // was #ededed
  textSecondary: "#94a3b8",   // was #b4b4b4
  textMuted:     "#64748b",   // was #8a8a8a (check WCAG AA on #0f1117 bg)
  textDisabled:  "#475569",   // was #767676

  // Surfaces
  background:      "#0f1117",  // was #0f0f11 — matches "dark background" req
  cardBackground:  "#161b22",  // was #1a1a1f — matches "card surfaces" req
  hoverBackground: "#1e2530",  // was #22222a

  // Borders — subtle, no heavy dividers
  borderColor:  "#1e293b",   // was #2a2a35 — matches req "subtle borders"
  borderSubtle: "#1a2232",   // was #252530

  // Accent — indigo/violet per requirements
  primary:       "#6366f1",  // was #7c3aed — requirements specify indigo
  primaryHover:  "#818cf8",  // was #8b5cf6
  primaryActive: "#4f46e5",  // was #6d28d9

  // Status dots (add to contract — see below)
  // statusDotRunning: "#22c55e"   green
  // statusDotPaused:  "#f59e0b"   amber
  // statusDotIdle:    "#475569"   slate

  // Font — Inter as primary sans
  font: {
    mono: "'JetBrains Mono', 'Fira Code', 'Monaco', monospace",  // per requirements
    sans: "'Inter', system-ui, sans-serif",
    display: "'Inter', system-ui, sans-serif",
  },
}
```

### Step 2: Add missing tokens to the contract

Add to `theme-contract.css.ts` → `vars.color`:

```typescript
// Status indicator dots
statusDotRunning: null,
statusDotPaused:  null,
statusDotIdle:    null,

// Session row compact layout
sessionRowHeight: null,  // "36px" — used in session row .css.ts files
```

Add to `theme-contract.css.ts` as a new top-level `transition` group:

```typescript
transition: {
  fast: null,   // "100ms ease"
  base: null,   // "150ms ease"
  slow: null,   // "250ms ease"
}
```

All six theme definitions in `theme.css.ts` must be updated with the new tokens.

### Step 3: Sync `globals.css` bridge variables

When `cleanTheme` values change, update these `:root` fallback values in `globals.css` to match (prevents FOUC before VE class application):

```css
:root {
  --background:      #0f1117;
  --card-background: #161b22;
  --border-color:    #1e293b;
  --primary:         #6366f1;
  --primary-hover:   #818cf8;
  --text-primary:    #e2e8f0;
  --text-secondary:  #94a3b8;
  --text-muted:      #64748b;
}
```

---

## 5. CSS Variable Migration Strategy

### Current state
- New components: vanilla-extract `.css.ts` files referencing `vars.xxx` — fully migrated
- Legacy components: CSS Modules (`.module.css`) referencing `--css-custom-props` from `globals.css`
- Bridge layer: `globals.css` `:root` defines the same token set as legacy fallback values
- The `@media (prefers-color-scheme: dark)` block was already removed (Story 1.5.3); theme is class-based

### Migration approach for this redesign

**No mass migration needed.** The architecture is already correct. The redesign work is:

1. **Update token values** in `cleanTheme` (theme.css.ts) — all VE components pick up the new palette automatically via `vars.xxx` references.
2. **Update bridge variables** in `globals.css` `:root` — all CSS Module components pick up the new palette automatically via `var(--xxx)` references.
3. **Add new tokens** to the contract for new components (status dots, transition values).
4. **Make `cleanTheme` the default** by changing `ThemeContext.tsx` `initialTheme` default from `"matrix"` to `"clean"`.
5. **Update the FOUC prevention script** in `layout.tsx` (if any inline script applies a default theme class on first load before hydration) to apply the `clean` class.

### What NOT to do
- Do not convert existing `.module.css` files to `.css.ts` as part of this PR — out of scope
- Do not create new CSS custom properties for things that already have `vars.xxx` equivalents
- Do not add `var(--color-bg)`, `var(--color-border)` — undefined tokens, `lint:css-vars` will fail (see ADR-009 rules)

---

## 6. Inter Font Integration

The requirements specify Inter as the primary sans-serif. Next.js 15 has built-in Google Fonts support via `next/font/google`. Add in `layout.tsx`:

```typescript
import { Inter, JetBrains_Mono } from "next/font/google";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-jetbrains-mono",
  display: "swap",
  weight: ["400", "500", "600"],
});
```

Then reference in `cleanTheme`:
```typescript
font: {
  sans: "var(--font-inter, 'Inter', system-ui, sans-serif)",
  mono: "var(--font-jetbrains-mono, 'JetBrains Mono', 'Fira Code', monospace)",
  display: "var(--font-inter, 'Inter', system-ui, sans-serif)",
}
```

This matches the pattern already used by `matrixTheme` and `cyberpunk77Theme` for JetBrains Mono.

---

## 7. Comparable OSS Projects with vanilla-extract Dark Themes

### Findings from web research

The most relevant real-world reference patterns:

**Pattern 1: `createThemeContract` + multiple `createTheme` (this project's pattern)**
Used by: seek-oss internal tools, Vanilla-Extract's own docs site. The VE docs site applies a dark class to `<html>` at runtime — identical to the current `applyThemeClass()` in `ThemeContext.tsx`. This confirms the current architecture is the canonical approach.

**Pattern 2: `createGlobalTheme` for shared tokens + `createThemeContract` for color-only contract**
Advantage: shared tokens (spacing, fonts, radii) are emitted as `:root` CSS vars once, not duplicated per theme. **This project already achieves this outcome** via the `sharedTokens` spread pattern in `theme.css.ts` (all themes share the same `space`, `radii`, `fontSize`, `fontWeight` values).

**Pattern 3: Semantic naming over role-based naming**
Linear/Vercel design systems use semantic token names rather than raw color names:
- `--fg-primary` not `--text-color-main`
- `--bg-surface-raised` not `--card-background`
- `--border-subtle` not `--border-light`

The current contract already uses this pattern (`textPrimary`, `cardBackground`, `borderSubtle`), which is good. No rename needed.

**Token depth insight from Linear's 2025 redesign**: Linear cut color back to near-monochrome black/white with very few accent colors. Their approach: one primary action color (indigo/violet), semantic status colors only, no decorative palette. The `cleanTheme` + indigo accent direction matches this exactly.

---

## Summary of Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Theme strategy | Update `cleanTheme`, make it default | Already 80% correct; avoids breaking other themes |
| New tokens needed | statusDot (3), transition (3) | Compact row + micro-animation consistency |
| Animation library | CSS only (no Framer Motion) | Not a dep; requirements prohibit new heavy libs; CSS sufficient |
| Entry animations | `@starting-style` native CSS | Zero-dep Framer Motion alternative for mount transitions |
| Font loading | `next/font/google` Inter + JetBrains Mono | Matches existing mono pattern; zero FOUT |
| Token naming convention | Keep existing camelCase VE names | Already semantic; no migration cost |
| globals.css | Sync bridge vars after VE token update | Prevents FOUC; no structural change needed |
| Default theme change | `"matrix"` → `"clean"` in ThemeContext | Core deliverable of REQ-1 |
