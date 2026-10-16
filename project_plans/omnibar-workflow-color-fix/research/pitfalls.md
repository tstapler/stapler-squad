# Pitfalls: omnibar-workflow-color-fix

## 1. Is `accentHover`-as-text a repeated anti-pattern?

No. `grep -rn "vars.color.accentHover" web-app/src/` returns 29 hits; every one
but `AtCommandDropdown.css.ts:42` sets `background`/`boxShadow` (hover states,
pulse animations). `slug` is the only call site using it as `color`. One-file fix
is correctly scoped — no sibling call sites to touch.

## 2. Does `accentText` read acceptably against `cardBackground` in all 7 themes?

**Not fully verified — this is the real risk.** `accentText` was added in
70130601e ("darken InlineNotice's accent text/icon color for WCAG AA contrast")
and tuned/measured against **`accentBg`** (InlineNotice's own background), not
`cardBackground`. Per-theme, from `web-app/src/styles/theme.css.ts`:

| Theme | accentText | Comment at definition |
|---|---|---|
| light | `#003d99` | measured ~9.2:1 vs `accentBg` (`#ebf4fe`) |
| dark | `#2d9cdb` | "~5:1 against accentBg **blended over cardBackground**" — only theme where cardBackground was explicitly considered |
| matrix, cyberpunk77, wh40k, clean | primary color, unchanged | `"unchanged from pre-fix behavior ... not in scope for this fix"` — **no contrast check done at all**, against any background |

So 5 of 7 themes have zero recorded contrast verification for `accentText`,
and even light/dark's numbers are against `accentBg`, a tinted overlay
(`rgba(0,112,243,0.08)` / `rgba(45,156,219,0.1)`), not the plain
`cardBackground` (`#f9f9f9` / `#1a1a1a`) that `AtCommandDropdown.css.ts`'s
`dropdown` style actually uses. `accentBg` is lighter/more-tinted than
`cardBackground` in most themes, so a contrast ratio that passes against
`accentBg` isn't guaranteed to pass against `cardBackground` — likely fine in
practice (accentText is a saturated, darkened token) but not something to
assert without checking. Recommend a quick manual/automated contrast check
(e.g. the existing `tests/e2e/accessibility.spec.ts` Axe pass, which already
runs on PRs touching `web-app/src/`) rather than assuming pass-through
correctness from AC #3's `make lint`/jest gate — neither of those tools does
color-contrast math.

## 3. Snapshot / test breakage risk

None. No test file exists for this component (`find web-app/src -iname
"*AtCommandDropdown*"` returns only `.css.ts` and `.tsx`, no `.test.tsx` or
`__snapshots__`). A color-only vanilla-extract class-hash change has nothing
to invalidate here.

## 4. Same anti-pattern in sibling dropdowns (collateral debt)?

Checked `OmnibarResultList.css.ts`, `OmnibarSessionResult.css.ts`,
`OmnibarPresetList.css.ts` — none use `accentHover`, `accentBg`, or
`accentText` as a text `color`; they use `textPrimary`/`textSecondary`/
`textMuted`/`textTertiary`/`error`/`warning`. No related debt to flag in
these three files.

## Bottom line

The swap itself is safe and correctly scoped (single call site, no test
fallout). The one open question worth a sentence in the PR description: the
5 non-light/dark themes' `accentText` contrast against any background was
never verified in 70130601e, so this fix inherits that pre-existing gap
rather than introducing a new one — worth a follow-up note, not a blocker for
a complexity-1 fix.
