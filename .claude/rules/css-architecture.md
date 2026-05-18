---
globs:
  - "web-app/**/*.css"
  - "web-app/**/*.module.css"
  - "web-app/**/*.css.ts"
  - "web-app/**/*.tsx"
  - "web-app/**/*.ts"
  - "web-app/**/*.html"
---

# CSS Architecture Rules (ADR-009)

This project has adopted **vanilla-extract** for all new CSS. See `docs/adr/009-vanilla-extract-type-safe-css.md`.

## New Components: Use vanilla-extract

All new component styles go in a `.css.ts` file colocated with the component:

```
components/
  Button/
    Button.tsx
    Button.css.ts   ← styles here, NOT Button.module.css
```

Import tokens from the shared theme contract — never hardcode values or use `var()` strings directly:

```ts
// Button.css.ts
import { style, recipe } from '@vanilla-extract/css';
import { vars } from '../../styles/theme.css';

export const button = recipe({
  base: {
    borderRadius: vars.radii.md,
    fontWeight: 600,
  },
  variants: {
    intent: {
      primary: { background: vars.color.actionPrimary, color: vars.color.textInverse },
      danger:  { background: vars.color.statusDanger,  color: vars.color.textInverse },
    },
    size: {
      sm: { padding: `${vars.space[1]} ${vars.space[2]}`, fontSize: vars.fontSize.sm },
      md: { padding: `${vars.space[2]} ${vars.space[4]}`, fontSize: vars.fontSize.base },
    },
  },
  defaultVariants: { intent: 'primary', size: 'md' },
});
```

```tsx
// Button.tsx
import { button } from './Button.css';
<button className={button({ intent: 'primary', size: 'md' })} />
```

## Existing CSS Modules: Token Names Only

When editing existing `.module.css` files, only reference variables **defined in `globals.css`**. The CI `lint:css` step will fail on any `var(--undefined-var)`.

Defined tokens (check `web-app/src/app/globals.css` for the full list):
- Text: `--text-primary`, `--text-secondary`, `--text-muted`, `--text-disabled`
- Backgrounds: `--background`, `--card-background`, `--hover-background`, `--modal-background`, `--overlay-background`
- Borders: `--border-color`, `--modal-border`, `--input-border`, `--input-focus-border`
- Actions: `--primary`, `--primary-hover`, `--primary-active`, `--primary-text`
- Status: `--success`, `--success-bg`, `--error`, `--error-bg`, `--error-text`, `--warning`, `--warning-bg`
- Terminal: `--terminal-background`, `--terminal-foreground`, `--terminal-border-color`, etc.
- Input: `--input-background`, `--input-text`, `--input-border`, `--input-focus-border`
- Other: `--foreground`, `--header-height`

If you need a token that doesn't exist yet, add it to `globals.css` **first**, then reference it.

## Dynamic Styles (Runtime Values)

vanilla-extract is build-time only. For runtime-dynamic values, use CSS custom properties as a bridge:

```tsx
// Pass dynamic value as inline CSS custom property
<div
  className={dynamicCard}
  style={{ '--card-accent': props.color } as React.CSSProperties}
/>
```

```ts
// dynamicCard.css.ts
export const dynamicCard = style({
  borderLeft: `3px solid var(--card-accent, ${vars.color.actionPrimary})`,
});
```

## Theme Token Contract

The shared token contract lives at `web-app/src/styles/theme.css.ts` (create it if it doesn't exist yet). All design tokens should be defined there using `createTheme`. Never use `var('--token-name')` strings in `.css.ts` files — use `vars.xxx` references.

## Never Do

- `var(--color-bg)`, `var(--color-border)`, `var(--color-text)` — these are not defined (see PR #20)
- Hardcoded hex values in component CSS (`background: '#3b82f6'`) — use `vars.color.xxx`
- New `.module.css` files for new components — use `.css.ts` instead
- `@layer` inside CSS Modules — layer names are global and will conflict
- Runtime CSS-in-JS (`styled-components`, `emotion`) — incompatible with React Server Components
- **Hardcoded `zIndex` numbers** — add a named slot to `zIndex` in `theme-contract.css.ts` and reference `zIndex.mySlot`. Magic numbers cause invisible ordering collisions.
- **`style={{ flexDirection: ... }}` or other layout inline styles** — inline style beats the CSS cascade; use a `data-*` attribute + `selectors` in the `.css.ts` file instead so theme overrides can win.
- **`position: fixed` or `position: absolute` modals/sheets without `createPortal`** — `fixed` positioning silently breaks when any ancestor has a CSS `transform`, `filter`, or `will-change`. Always use `createPortal(..., document.body)` for overlays that must escape the component tree.
