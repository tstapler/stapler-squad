# CSS Architecture & Tooling Gaps — Research Findings

## Theme Contract & Token Coverage

`web-app/src/styles/theme-contract.css.ts` defines the full token contract via `createThemeContract`. Token groups:

- **Color** (55 tokens): text (6), surfaces/backgrounds (8), borders (8), primary action (5), status (9), accent (2), inputs (3), terminal (8), cyberpunk/glow (4)
- **statusBadge** (15 tokens): per-status bg/fg/border triples for approval, input, complete, uncommitted, idle, stale, processing
- **font** (3): mono, sans, display
- **space** (9 scale steps): 0–16
- **radii** (4): sm/md/lg/full
- **fontSize** (5): xs/sm/base/lg/xl
- **fontWeight** (4): normal/medium/semibold/bold
- **shadow** (4): none/sm/md/lg

`web-app/src/styles/theme.css.ts` (619 lines) implements two themes (light and dark) using `createTheme` against this contract. The terminal tokens are hardcoded to dark values in the light theme (line 45: `terminalBackground: "#1e1e1e"`, line 46: `terminalForeground: "#d4d4d4"`) — this is intentional and documented inline, not a token gap.

**Token gaps:** The contract notably lacks tokens for:
- Chart / analytics colors (used in `ApprovalAnalyticsPanel.css.ts` with hardcoded hex)
- Line-height / letter-spacing
- Animation duration / easing (handled via `styles/animations.css.ts` with hardcoded values)
- `zIndex` and `breakpoints` are exported as plain constants (not theme tokens) because CSS custom properties cannot be used in `@media` queries — this is intentional and documented at line 140–161 of `theme-contract.css.ts`

## Hardcoded Hex Values in `.css.ts` Files

Scanning all 115 `.css.ts` files found hardcoded hex colors in the following locations:

**Legitimately hardcoded (intentional):**
- `styles/theme.css.ts` (619 lines): terminal defaults (`#1e1e1e`, `#d4d4d4`) — documented
- `components/settings/ThemePicker.css.ts` (lines 67–98): theme preview swatches use hardcoded gradients that _represent_ each theme's palette — these cannot use `vars.*` tokens by definition

**Header dark override (borderline):**
- `components/layout/Header.css.ts` (lines 22–23): `[vars.color.textPrimary]: "#ededed"` and `[vars.color.textSecondary]: "#b4b4b4"` — these are CSS variable overrides scoped to the dark header. The comment (line 163) explains this is a WCAG AA contrast fix. A dedicated `headerTextPrimary` token would be cleaner.

**Actual violations — values that should be tokens:**
- `app/debug/escape-codes/page.css.ts` (lines 189–199): 11 escape-code type badges each use direct Tailwind-style hex values (`#3b82f6`, `#8b5cf6`, `#ec4899`, etc.). These are debug-page UI but should still reference `vars.color.*` or define named chart tokens.
- `components/sessions/ApprovalAnalyticsPanel.css.ts` (lines 300, 315, 320): Three chart bars use `background: "#8b5cf6"`, `"#3b82f6"`, `"#f97316"`. No corresponding chart color tokens exist in the contract.

## Hardcoded Pixel Values in `.css.ts` Files

Grep for `"[0-9]+px"` across all `.css.ts` files found **987 occurrences** across ~20+ files. The majority are in component-level layout files. The most widespread are spacing and dimension values that predate the `vars.space` token adoption:

Files with the most hardcoded px values:
- `components/ui/NotificationPanel.css.ts` (710 lines)
- `components/sessions/SessionCard.css.ts` (808 lines)
- `components/sessions/SessionDetail.css.ts` (607 lines)
- `app/page.css.ts`
- All `app/*/page.css.ts` and `app/*/history.css.ts` files

The `vars.space` contract only covers 9 scale steps (0–16). Values like `"320px"` (column widths in `sessionCockpit.css.ts`), `"48px"` (icon sizes), and `"24px"` (common spacing) appear frequently without tokens. These could be tokenized as `layout.columnWidth.sessionList`, etc.

## ESLint Configuration

`web-app/.eslintrc.json` extends `next/core-web-vitals` and adds:

**`eslint-plugin-boundaries`** (version 6.0.2): enforces import layer rules. 13 element types defined; `sessions` / `unfinished` / `history` / `logs` / `settings` / `telemetry` can only import from `providers`, `ui`, `shared`, `lib`, `gen`. This rule runs in CI and blocks cross-layer violations.

**`no-restricted-syntax` rules (6 custom rules):**
1. Blocks `'100vh'` literals (use `var(--viewport-height)`)
2. Blocks `'100dvh'` bare literals
3. Blocks `'100lvh'` literals
4. Template-literal variants of the above three
5. **Blocks hardcoded hex in JSX `style` props** (`JSXAttribute[name.name='style'] > ... > Literal[value=/^#.../]`) — catches `style={{ color: '#fff' }}` but not hex values inside `.css.ts` files

**Critical gap:** The hex-in-style-prop rule only applies to JSX `style` attributes. Hardcoded hex values inside `.css.ts` files (which is where the actual violations are: `ApprovalAnalyticsPanel.css.ts`, `page.css.ts`) are not covered by any ESLint rule.

**Missing tooling:**
- No ESLint rule for hardcoded hex/px inside `.css.ts` files — a custom rule or `eslint-plugin-no-restricted-css-values` (does not exist; would need to be custom) is needed
- No `@typescript-eslint/strict` ruleset — only `next/core-web-vitals` baseline
- `stylelint` is installed (`^17.6.0` with `stylelint-config-standard` and `stylelint-config-css-modules`) but there are no `.module.css` files — it is unclear if `stylelint` runs against `.css.ts` files (it does not by default)
- No `eslint-plugin-react-hooks` explicit config (it is included transitively via `next/core-web-vitals`)
- No import-order enforcement beyond `boundaries`
