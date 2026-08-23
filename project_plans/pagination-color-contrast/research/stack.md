# Research: Stack (Agent 1)

## CSS/theming system

- vanilla-extract, confirmed. `web-app/src/styles/theme-contract.css.ts:1-3` defines the
  token contract via `createThemeContract({ color: { ... } })` — every token (including
  `primary`, `primaryHover`, `primaryActive`, `primaryDark`, `primaryText`) is declared
  `null` there (contract shape only, no values).
- `web-app/src/styles/theme.css.ts:1-7` imports that contract and builds each theme with
  `createTheme(vars, { ... })` (per the file's own header comment: "this file defines the
  literal hex values behind `vars.color.*` for every theme"). Consuming `.css.ts` files
  must reference `vars.color.primary` etc., never hardcode hex — enforced by
  `.claude/rules/css-architecture.md` and an `eslint-disable no-restricted-syntax` escape
  hatch comment at the top of `theme.css.ts` itself, which is the *only* file allowed to
  contain literal hex values.
- This confirms the fix is a single-line edit inside `theme.css.ts`'s `clean` theme block
  (line 636) — no changes needed to `theme-contract.css.ts` (contract shape is unaffected)
  or to any consuming `.css.ts` file.

## axe-core / Playwright invocation

- `tests/e2e/package.json:18` pins `"@axe-core/playwright": "^4.8.0"`.
- `tests/e2e/accessibility.spec.ts:13-14` imports `{ test, expect } from '@playwright/test'`
  and `AxeBuilder from '@axe-core/playwright'`.
- IT-5.1 tests are at lines 30-77:
  - `IT-5.1: Main page has no critical or serious accessibility violations` (line 30) —
    navigates to `BASE_URL`, waits for `input[aria-label="Search sessions"]`, then runs
    `new AxeBuilder({ page }).exclude('pre, [class*="terminal"], [class*="Terminal"]').analyze()`,
    filters `results.violations` to `impact === 'critical' || 'serious'`, asserts length 0.
  - `IT-5.1: Secondary routes are accessible` (line 63) — same pattern against
    `${BASE_URL}/review-queue`.
  - Both scans run full axe rulesets (not `.withRules(['color-contrast'])`-scoped), so any
    `serious`/`critical` finding of any rule type would fail these tests — the pagination
    "Next" button's `color-contrast` violation is one instance under this broad gate.
  - A third, narrower test (`stuck-item chips pass Axe color-contrast on /unfinished`,
    line 84) and a fourth (`flash overlay, connection indicator, and InlineNotice meet
    4.5:1 contrast...`, line 282) both explicitly scope
    `.withRules(['color-contrast'])` — useful narrower repros if IT-5.1 is slow (full
    120s timeout budget per test, since "Axe scans are CPU-heavy").
  - Verification command per requirements AC #3:
    `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`

## Existing contrast-checking script

- `web-app/scripts/check-theme-contrast.ts` exists and is wired to
  `web-app/package.json:34`: `"check-contrast": "npx --yes tsx scripts/check-theme-contrast.ts"`.
- It implements the WCAG relative-luminance / contrast-ratio formula in plain TS
  (`hexToRgb` → `linearize` → `luminance` → `contrast`, lines 56-79) and checks 4 pairs
  per theme (`textPrimary/background`, `textSecondary/background`,
  `textMuted/cardBackground`, `primaryText/primary`) against `WCAG_AA_NORMAL = 4.5`.
- **Important gap found**: this script does **not** import from `theme.css.ts` — it
  hardcodes its own duplicate `themes` object (lines 16-53) with theme names
  `matrix`, `cyberpunk77`, `wh40k`, `clean`, which do not match the actual theme keys/
  values in `theme.css.ts` (whose themes include `retro` at line 412, and whose `clean`
  theme's *current* `primary` is `#6366f1` at line 636 — but the script's hardcoded
  `clean.primary` is `#7c3aed`, a stale/different value). **This script is out of sync
  with the real theme source and will not catch or verify the actual bug** if run as-is;
  do not rely on `npm run check-contrast` to validate the fix. Either treat it as
  informational only, or (out of scope per AC #2 "token-only edit") flag it as a
  follow-up to make it import `vars`/theme values directly instead of duplicating them.
- No other contrast-checking script, lint rule, or CI job was found repo-wide (grep for
  "contrast" across `web-app/src`, `tests/e2e`, `scripts` surfaced only this script, one
  in-flight code comment convention of documenting ratios inline — see below — and
  unrelated prose hits in `TODO.md`/`CHANGELOG.md`/`docs/gap-analysis.md`).
- **Existing in-file convention**: three theme files already carry inline "before/after
  ratio" comments recording a WCAG fix, all following the same comment shape the
  requirements ask to match:
  - `theme.css.ts:412` (`retro` theme, the requirements' cited precedent):
    `primary: "#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */`
  - `theme.css.ts:642-645` (`retro` theme `success`/`successText`, a second precedent in
    the same file, same pattern).

## Confirmed current values

`web-app/src/styles/theme.css.ts:636-640` (`clean` theme, the default theme per
`web-app/src/lib/contexts/ThemeContext.tsx:30`):
```ts
primary: "#6366f1",
primaryHover: "#818cf8",
primaryActive: "#4f46e5",
primaryDark: "#3730a3",
primaryText: "#ffffff",
```
No existing inline comment on this block (unlike `retro`'s line 412) — confirms the bug
was never fixed here, consistent with the requirements' "not yet fixed" root-cause note.

`theme.css.ts:412` (`retro` theme, precedent — already quoted above): `primary` changed
`#ff2d78` → `#cc245f`, ratio 3.56:1 → 5.27:1.

## Candidate replacement values for `clean.primary`

Computed with the exact WCAG relative-luminance formula from
`check-theme-contrast.ts` (verified against the requirements' stated 4.46:1 for the
current value — this run got 4.467:1, consistent):

| Hex | Contrast vs `#ffffff` | Note |
|---|---|---|
| `#6366f1` (current) | 4.467:1 | FAILS (below 4.5) |
| `#5a5df0` | 4.931:1 | Minimal hue/lightness shift from current; comfortable margin above 4.5 |
| `#5457ef` | 5.268:1 | Same magnitude of margin as the `retro` precedent (5.27:1) — matches project convention numerically |
| `#4f46e5` | 6.288:1 | **This is the exact existing `primaryActive` value** (`theme.css.ts:638`) — reusing it as `primary` avoids introducing any new hex literal at all, at the cost of `primary`/`primaryActive` becoming visually identical unless `primaryActive` is also retuned |

Recommendation for the planning phase to weigh: `#5a5df0` is the smallest perceptual
step off `#6366f1` that clears 4.5:1 with margin (matches AC #5's "plausible indigo, not
jarring"); `#5457ef` mirrors the retro precedent's exact margin size (~5.27:1) if
consistency with that comment's stated ratio is preferred over minimal shift.

## Sources / commands run

- `Read web-app/src/styles/theme.css.ts` (lines 1-14, 400-424, 630-660)
- `Read web-app/src/styles/theme-contract.css.ts` (lines 1-80)
- `Read web-app/scripts/check-theme-contrast.ts` (full file)
- `Read tests/e2e/accessibility.spec.ts` (lines 1-70)
- `grep -n "axe" tests/e2e/package.json web-app/package.json`
- `grep -rn "contrast" -i web-app/src tests/e2e scripts`
- `find . -iname "*contrast*" -not -path "*/node_modules/*" -not -path "*/.git/*"`
- `grep -n "check-contrast" web-app/package.json`
- Node one-liner reimplementing the WCAG contrast formula (matches
  `check-theme-contrast.ts`'s implementation) to compute the candidate table above.
