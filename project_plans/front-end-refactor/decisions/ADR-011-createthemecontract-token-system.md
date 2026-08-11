# ADR-011: createThemeContract + createTheme Replacing Hand-Rolled vars Wrapper

## Status
Accepted

## Context

ADR-009 adopted vanilla-extract and created `web-app/src/styles/theme.css.ts`. The current implementation is a thin TypeScript wrapper:

```ts
// Current theme.css.ts (hand-rolled wrapper)
export const vars = {
  color: {
    primary: "var(--primary)",
    textPrimary: "var(--text-primary)",
    // ...
  },
  font: { mono: "var(--font-mono)" },
} as const;
```

This approach has two critical deficiencies:

### Deficiency 1: Not type-safe at the value level

`vars.color.textPrimary` resolves to the string `"var(--text-primary)"`. TypeScript considers this a valid CSS value for any property, including invalid ones. The following is a TypeScript no-error but a CSS no-op:

```ts
style({ background: vars.color.textPrimary }) // valid TS, semantically wrong
```

The whole point of vanilla-extract's `createThemeContract` is that it generates typed CSS custom property references whose names are stable across theme implementations. The hand-rolled version provides the same strings, but without the contract enforcement that ensures both `lightTheme` and `darkTheme` define every token.

### Deficiency 2: Dark mode requires manual CSS class management

Currently, `globals.css` uses `@media (prefers-color-scheme: dark)` to override CSS custom properties. This means:
- Dark mode cannot be toggled programmatically (user preference stored in `localStorage`)
- The React component tree has no awareness of the current theme
- Testing requires media query mocking

With `createTheme`, each theme produces a CSS class name. Applying the class to `<html>` switches the theme. This is the standard vanilla-extract pattern and enables programmatic theme switching with no JavaScript-in-CSS hacks.

### Current state of consumers

Only `VcsStatusDisplay.css.ts` currently imports `vars` from `theme.css.ts`. The surface area for migration is minimal.

The 70 `.module.css` files reference CSS custom properties via raw `var(--token-name)` strings. After this migration, those strings remain valid during the Phase 4 migration window — the `createTheme` implementation generates CSS variables with the same names as the current `globals.css` tokens (by convention).

## Decision

Migrate `web-app/src/styles/theme.css.ts` from the hand-rolled wrapper to a proper `createThemeContract` + `createTheme` implementation.

### Two-file structure

```
web-app/src/styles/
  theme-contract.css.ts   ← createThemeContract({}) — shape only, all values null
  theme.css.ts            ← createTheme(vars, {...}) × 2 (light + dark) + re-exports vars
```

Separating contract from implementation allows `packages/core` (Phase 5) to import only `theme-contract.css.ts` (the type shape) without pulling in web-specific color values.

### Token contract shape

The contract must cover all tokens currently defined in `globals.css`. The naming convention changes from `--text-primary` (hyphenated flat) to `vars.color.textPrimary` (camelCase nested) in TypeScript, while the generated CSS variable name is controlled by vanilla-extract (stable hash-based names). During the Phase 4 migration, `globals.css` keeps its original custom property names as a bridge for unconverted `.module.css` files.

```ts
// theme-contract.css.ts
import { createThemeContract } from '@vanilla-extract/css';

export const vars = createThemeContract({
  color: {
    // Text
    textPrimary: null, textSecondary: null, textMuted: null,
    textDisabled: null, textInverse: null,
    // Backgrounds
    background: null, cardBackground: null, hoverBackground: null,
    modalBackground: null, overlayBackground: null,
    // Borders
    borderColor: null, modalBorder: null,
    inputBorder: null, inputFocusBorder: null,
    // Actions
    actionPrimary: null, actionPrimaryHover: null, actionPrimaryActive: null,
    actionPrimaryText: null,
    // Status
    statusSuccess: null, statusSuccessBg: null,
    statusWarning: null, statusWarningBg: null,
    statusDanger: null, statusDangerBg: null, statusDangerText: null,
    // Terminal
    terminalBackground: null, terminalForeground: null, terminalBorder: null,
    // Input
    inputBackground: null, inputText: null,
  },
  space: {
    '0': null, '1': null, '2': null, '3': null, '4': null,
    '6': null, '8': null, '12': null, '16': null,
  },
  radii: { sm: null, md: null, lg: null, full: null },
  fontSize: { xs: null, sm: null, base: null, lg: null, xl: null },
  fontFamily: { sans: null, mono: null },
});
```

### Theme implementation values

Theme values must match the existing `globals.css` token values exactly so no visual change occurs during migration. Example mapping:

| `globals.css` token | `vars.color.*` key | Light value | Dark value |
|---|---|---|---|
| `--text-primary` | `textPrimary` | `#111827` | `#f9fafb` |
| `--background` | `background` | `#ffffff` | `#0f172a` |
| `--primary` | `actionPrimary` | `#3b82f6` | `#60a5fa` |
| `--border-color` | `borderColor` | `#e5e7eb` | `#374151` |

(Full mapping derived from `globals.css` at migration time — not duplicated here.)

### Backward compatibility guarantee

`theme.css.ts` re-exports `vars` so existing `import { vars } from '../../styles/theme.css'` statements continue to work without modification. `VcsStatusDisplay.css.ts` needs no change.

### ThemeProvider component

A small `"use client"` ThemeProvider reads `localStorage.getItem('theme')` and applies `lightTheme` or `darkTheme` class to `<html>`. Falls back to `prefers-color-scheme` if no preference is stored.

```tsx
// components/providers/ThemeProvider.tsx
"use client";
import { lightTheme, darkTheme } from '../../styles/theme.css';

export function ThemeProvider({ children }) {
  // Read preference, apply class to document.documentElement
  // ...
}
```

## Consequences

### Positive
- TypeScript compile error if any theme token is missing from either `lightTheme` or `darkTheme` (enforced by `createTheme` contract validation)
- Programmatic theme switching via CSS class — no media query dependency
- `vars.color.textPrimary` is a stable typed reference, not a raw `var()` string
- Two-file structure prepares for `packages/core` extraction in Phase 5

### Negative / Constraints
- vanilla-extract generates hash-based CSS variable names (e.g., `--color-textPrimary__hash`) rather than the human-readable names in `globals.css`. During Phase 4, `.module.css` files that use `var(--text-primary)` from `globals.css` continue to work — they read from `globals.css`, not from the vanilla-extract generated variables. After Phase 4 (zero `.module.css` files), `globals.css` can be trimmed to a minimal reset.
- Token values are duplicated: once in `globals.css` (for the migration window), once in `theme.css.ts`. This is a temporary two-source state acceptable for 3–6 months.
- Space tokens (`vars.space['4']`) use string keys because numeric keys in TypeScript object types have ordering implications — the string form `vars.space['4']` is slightly more verbose than `vars.space[4]` but avoids surprises.

## References
- vanilla-extract `createThemeContract` docs: https://vanilla-extract.style/documentation/api/create-theme/
- ADR-009: `docs/adr/009-vanilla-extract-type-safe-css.md`
- Research synthesis: `project_plans/front-end-refactor/research/synthesis.md`
- Implementation: Phase 1, Story 1.2 in `docs/tasks/front-end-refactor.md`
