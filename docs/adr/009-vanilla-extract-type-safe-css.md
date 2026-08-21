# ADR-009: vanilla-extract for Type-Safe CSS

## Status
Accepted

## Context

The web UI frontend (`web-app/`) uses CSS Modules with CSS custom properties defined in `globals.css`. In April 2026, a bug was discovered (PR #20) where `DebugMenu.module.css` referenced 13 undefined CSS custom properties (e.g. `--color-bg`, `--color-border`, `--color-text`). These variables were never defined in `globals.css`, causing the debug menu to render with transparent backgrounds and invisible text. The bug was silent — no browser error, no build error, no lint error.

A broader audit found **49 undefined CSS variable references** across the codebase, using at least three competing naming conventions:

- `--color-bg`, `--color-border`, `--color-text` (undefined, never existed)
- `--card-bg`, `--panel-bg`, `--modal-bg` (undefined aliases for `--card-background`, etc.)
- `--tag-bg`, `--button-bg`, `--item-border` (component-scoped names with no global definition)

### Root Cause

Two structural problems:

1. **No token tier separation.** `globals.css` defines only a flat set of semantic tokens with inconsistent naming (`--modal-background`, `--border-color`, `--text-primary`). There are no primitive tokens underneath them and no enforced naming convention. New components guessed variable names that sounded plausible but were never defined.

2. **No CSS lint.** `next lint` only covers JS/TS. There was no tooling that validated `var(--xxx)` references against the set of defined custom properties. The class of bug was entirely undetectable before visual QA.

### Requirements

Any solution must:
- Catch undefined CSS variable references **at build time or lint time** — not at runtime
- Provide **TypeScript type safety** for CSS property values, not just class names
- Support **design tokens** as a first-class concept with typed references
- Be **zero-runtime** and compatible with React Server Components
- Be **compatible with Next.js 15 App Router** and `output: "export"` (static site)
- Support **gradual migration** — existing CSS Modules continue working during transition

### Options Considered

| Option | Type Safety | RSC Compatible | Token Support | Verdict |
|---|---|---|---|---|
| CSS Modules + `tcm` | Class names only | Full | None | Insufficient — no value checking |
| CSS Modules + stylelint | Class names + defined vars | Full | None | Partial — catches undefined refs but no TS |
| Linaria | Props only (template literals) | Broken for RSC | None | Rejected — RSC incompatibility |
| Panda CSS | Properties + values (codegen) | Full | Config-driven | Strong contender |
| StyleX (Meta) | Properties (csstype) | Full | `defineVars` | Rejected — small external ecosystem, high learning curve |
| **vanilla-extract** | **Properties + values (compile-time)** | **Full** | **`createTheme`** | **Selected** |

### Why vanilla-extract

vanilla-extract was authored by Mark Dalgleish, co-creator of CSS Modules. It is CSS Modules' natural successor for TypeScript-first teams.

**Type safety mechanism**: Styles are authored in `.css.ts` files that run exclusively at build time. All CSS property names and values are typed against the full CSS spec via `csstype`. Typos like `bakground: 'red'` or `colours: vars.color.primry` are TypeScript errors caught in the editor and by `tsc`.

**Token safety via `createThemeContract`**: Design tokens are defined as a typed TypeScript object. All component references to token values go through `vars.color.textPrimary` — not through string-based `var(--color-text-primary)`. A typo on any token path is a compile error, not a silent transparent element.

```ts
// theme.css.ts
export const [lightTheme, vars] = createTheme({
  color: {
    textPrimary: '#111827',
    actionPrimary: '#3b82f6',
  },
  space: { 2: '0.5rem', 4: '1rem' },
});

// Component.css.ts
import { vars } from '../theme.css';

export const button = style({
  color: vars.color.textPrimary,     // typed — typo is a compile error
  padding: vars.space[4],
});
```

**Addons for variant systems**:
- `recipe()` — typed multi-variant component API (replaces manual `clsx` variant assembly)
- `sprinkles` — typed atomic utility generator (typed Tailwind-style utilities)

**Production maturity**: Used in production at Atlassian (Atlaskit design system) and Seek. Core API stable since 2022. ~10k GitHub stars.

**RSC + static export**: All style logic runs at build time. Output is static `.css` files indistinguishable from hand-authored CSS Modules. Compatible with `output: "export"` and React Server Components.

**Gradual migration**: vanilla-extract and CSS Modules coexist in the same project. New components use `.css.ts`; existing `.module.css` files continue working. No big-bang rewrite required.

### Why Not Panda CSS

Panda CSS is a strong alternative, especially for utility-heavy codebases. It was not chosen because:

1. The codebase uses CSS Modules authoring style today. vanilla-extract's `.css.ts` files are a closer conceptual match — you still write class-based styles, just in TypeScript.
2. Panda's codegen step adds a `styled-system/` generated directory that must be managed (committed or gitignored — both options have tradeoffs).
3. vanilla-extract is more mature and the `createTheme` token contract system more directly solves the class of bug that motivated this ADR.

Panda CSS remains a viable future direction if the team moves toward a utility-first authoring style.

## Decision

Adopt **vanilla-extract** as the CSS authoring standard for new components and token definitions in `web-app/`.

### Tooling adopted alongside

**`scripts/check-css-vars.mjs`**: A zero-dependency Node.js script that validates all `var(--xxx)` references in CSS Module files against tokens defined in `globals.css`. Runs in CI via `npm run lint:css-vars`. This covers the migration period while CSS Modules files are progressively converted to `.css.ts`. (Note: `stylelint`'s built-in `no-unknown-custom-properties` rule operates per-file only and produces false positives for CSS Modules consuming tokens from a separate `globals.css`; a custom cross-file script is the appropriate solution.)

**`stylelint` + `stylelint-config-css-modules`**: General CSS linting foundation, required for the design token plugin.

## Consequences

### Positive

- Undefined CSS variable references become compile errors (TypeScript) or CI failures (stylelint) — never silent visual bugs
- Token names are validated by the TypeScript compiler — typos in `vars.color.xxx` are caught before commit
- Dark mode theme switching via `createThemeContract` is type-safe by construction
- Gradual migration — no existing code breaks

### Negative / Constraints

- Dynamic styles (e.g. `background: props.color` where color is a runtime value) cannot be expressed directly. Must use CSS custom properties as a bridge: pass the value as an inline style custom property, consume it via `var()` in the `.css.ts` file.
- `.css.ts` files cannot use runtime values at style creation time — all variants must be pre-declared. This is a feature for design system work but a friction point for highly dynamic one-off styles.
- Turbopack (used in `next dev --turbopack`) support in `@vanilla-extract/next-plugin` is available from v1.3+ but is newer than the webpack integration. If dev-mode issues arise, fall back to `next dev` (without `--turbopack`) while the plugin matures.
- Build setup is slightly more complex than plain CSS Modules (requires `@vanilla-extract/next-plugin` in `next.config.ts`).

### Migration Strategy

1. **Immediate**: Stylelint CI check catches all 49 existing undefined variable references. Fix progressively.
2. **New components**: Author all new components in `.css.ts`. Define a shared `theme.css.ts` early — token contract is the highest-leverage step.
3. **Existing components**: Migrate `.module.css` → `.css.ts` component by component, starting with leaf components.
4. **Token standardisation**: Adopt `--color-{role}-{modifier}` naming (e.g. `--color-text-primary`, `--color-bg-surface`) when re-defining primitives in the theme contract.

## References

- vanilla-extract docs: https://vanilla-extract.style/
- `@vanilla-extract/next-plugin`: https://vanilla-extract.style/documentation/integrations/next/
- `createTheme` / `createThemeContract`: https://vanilla-extract.style/documentation/api/create-theme/
- `recipe()`: https://vanilla-extract.style/documentation/packages/recipes/
- CSS Modules co-existence: vanilla-extract and CSS Modules work side-by-side — no migration deadline
- PR #20 (the bug that motivated this ADR): https://github.com/org/stapler-squad/pull/20
