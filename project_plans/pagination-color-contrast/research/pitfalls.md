# Pitfalls Research: pagination-color-contrast

Scope: what commonly goes wrong fixing a shared design token (`clean` theme's
`primary: #6366f1`) for WCAG contrast, given 412 consumers across 6 themes
(`lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme` — the "retro"
theme referenced in requirements.md — `wh40kTheme`, `cleanTheme`, all in
`web-app/src/styles/theme.css.ts`).

## 1. Blast radius — literal `#6366f1` outside the token

`grep -rn "6366f1" web-app/src --include="*.ts*" -i` finds exactly 4 hits, 3 inside
`theme.css.ts`'s `cleanTheme` block and 1 elsewhere:

| File:line | What | Fixed by a token-only edit to `primary`? |
|---|---|---|
| `theme.css.ts:631` `borderHover: "#6366f1"` | Same literal, different token (border color on hover), same clean theme | **No** — separate token, not read by this fix. Not an axe color-contrast target (borders aren't checked the same way as text/bg), but it will visually diverge from the new primary shade unless updated together. Not in the ticket's acceptance criteria (token-only edit to `primary`), so this is intentional drift, not a bug — but worth a sentence in the PR description since `borderHover` is clearly meant to echo `primary`/`primaryHover`'s hue. |
| `theme.css.ts:657` `accentText: "#6366f1"` | Duplicate literal (not a `vars.color.primary` reference) — see §3 | **No** — will silently go stale (see below) |
| `web-app/src/app/insights/ModelBreakdownChart.tsx:33` | `PALETTE` array entry for a bar-chart fill color (`data[i].color`), independent of the theme system | **No**, but this is correct/expected — it's a hardcoded categorical data-viz palette (see also `#10b981`, `#f59e0b`, `#ef4444` alongside it), not text-on-background. Confirmed: used only as a `fill`, never paired with white/foreground text in this component. Out of scope, no action needed. |

No other file duplicates `#6366f1` as a literal. The 412 `vars.color.primary`
consumers are genuinely all `vars.color.primary` references (spot-checked a
sample — `Button.css.ts`, `ModalTour.css.ts`, `interactiveBase.css.ts`,
`OmnibarCreationPanel.css.ts`) — so the token-only edit will correctly
propagate to essentially all of them. The two real gaps are `borderHover`
(same-file sibling token, same literal, not touched) and `accentText`
(cross-token duplicate, see §3).

## 2. Adjacent tokens: `primaryHover` fails WORSE than `primary`, and it's on the exact flagged button

Clean theme: `primary: #6366f1`, `primaryHover: #818cf8` (**lighter**),
`primaryActive: #4f46e5` (darker). Computed WCAG relative-luminance contrast
against `#ffffff` (formula verified against the ticket's own stated 4.46:1 for
`primary`):

| Pair (bg / white text) | Ratio | WCAG AA (4.5:1) |
|---|---|---|
| `primary` `#6366f1` | 4.46:1 | **fails** (the reported violation) |
| `primaryHover` `#818cf8` | **~2.98:1** | **fails, worse** |
| `primaryActive` `#4f46e5` | ~6.29:1 | passes |

`primaryHover` is lighter than `primary`, so its white-text contrast is
*worse*, not better — roughly 2.98:1, well below both the 4.5:1 normal-text
threshold and even the relaxed 3:1 large-text threshold.

This directly affects the exact button implicated in the ticket:
`web-app/src/components/ui/ModalTour.css.ts:146-163` (`primaryButton`, the
"Next" button) sets `color: vars.color.primaryText` (white) and
`background: vars.color.primary`, then on `:hover` swaps only the background
to `vars.color.primaryHover` — **without** swapping the text color. Same
pattern in the generic `Button.css.ts` `intent: primary` variant (line 34-42)
and `interactiveBase.css.ts` `variant: primary` (line 66-77), both consumed
across dozens of components. So after a `primary`-only edit, the *resting*
state of these buttons will newly pass 4.5:1, but the *hover* state will still
fail — worse than before, in fact, since the absolute ratio drops further from
threshold.

**Why this won't be caught by IT-5.1 as currently written**: `tests/e2e/accessibility.spec.ts`
never calls `.hover()` before running `AxeBuilder(...).analyze()` (confirmed —
zero `.hover(` calls in the file), so axe only ever scans the resting DOM.
The hover-state failure is real but silent to this specific CI gate.

**Prior art already in this codebase for exactly this problem**:
`web-app/src/components/backlog-stuck/StuckItemsSection.css.ts:59-74`
(`chipActive`) hit this same primary/primaryHover contrast issue previously
and encodes the fix as an explicit pattern — use `primaryActive` (not
`primary`) for the resting/pressed state where it needs white text, and swap
to `vars.color.textInverse` instead of `primaryText` specifically on `:hover`
because `primaryHover` is too light for white text. Comment there: *"primary+textInverse
fails WCAG AA (4.43:1 in the clean theme, needs 4.5:1) — primaryActive+primaryText
is the existing 'pressed state' token pair and passes with margin (6.29:1)...
primaryHover (#818cf8) is a light indigo — textInverse (dark) is the correct
pairing here, not primaryText (white), which would fail contrast against this
specific background."*

**Implication for this ticket**: the acceptance criteria only require fixing
`primary` (resting state), which is sufficient to make IT-5.1 pass as written.
But the hover state of the very same "Next" button remains a real,
already-documented-elsewhere WCAG failure that a strictly token-only,
single-value edit does not touch and that CI cannot currently detect. Worth
either (a) flagging explicitly as a known, pre-existing, out-of-scope gap in
the plan/PR (recommended, since it matches complexity-1 scope), or (b) if the
plan phase wants zero new latent failures, applying the `chipActive` pattern's
`primaryHover`→`textInverse` hover-swap to `ModalTour.css.ts`/`Button.css.ts`
at the same time — but that's a behavior change on ~70+ call sites, well
beyond the ticket's stated "single-token hex edit" complexity-1 scope, and
should not be done silently.

## 3. `accentText` — duplicate literal, will drift, but is explicitly not this ticket's problem

`theme.css.ts:654-657`:
```ts
accentBg: "rgba(99,102,241,0.1)",
accentHover: "rgba(99,102,241,0.2)",
// accentText: unchanged from pre-fix behavior (same as primary) — not in scope for this fix.
accentText: "#6366f1",
```

`accentText` is a **hardcoded duplicate literal**, not a `vars.color.primary`
reference — confirmed via `grep`, it's its own `createTheme` key with its own
string value, and `theme-contract.css.ts`'s token contract has no aliasing
mechanism between `color.primary` and `color.accentText`. Editing `primary`
alone will **not** change `accentText`; `accentText` on `accentBg` (a
different pair — indigo text on a 10%-opacity indigo tint, not white-on-solid)
is a separate contrast calculation entirely and is unaffected either way.

This is intentional prior art, not new: the identical comment
(`// accentText: unchanged from pre-fix behavior (same as primary) — not in
scope for this fix.`) already exists verbatim for **every** theme
(`matrixTheme`, `cyberpunk77Theme`/retro, `wh40kTheme`, `cleanTheme`) — it was
added in the commit that introduced the `accentText` token itself (`70130601`
per `git log`), predating and independent of any of the later per-theme
`primary` contrast fixes. In the retro theme, `accentText` (`#cc245f`)
happens to already equal the *new, fixed* `primary` value — but that's
because it was set that way when the token was created, not because it's
linked. Confirmation: `cyberpunk77Theme.primary` was `#ff2d78` at one point in
history while `accentText` was already `#cc245f` in the same file — i.e. they
were never the same value by reference, only sometimes coincide by literal.

**Net effect for this ticket**: after the fix, `cleanTheme.accentText` will
still read `"#6366f1"` — the *old*, pre-fix primary hex — while
`cleanTheme.primary` reads the new value. This is a new, slightly larger
visual inconsistency (accent text no longer matches the primary brand color
it's meant to echo) but:
- It does not fail any WCAG check (accentText's own contrast against accentBg
  is untouched and was already passing/independent).
- It matches the same "leave accentText alone, it's out of scope" precedent
  used for every other themed fix in this file.
- Per requirements.md's own framing ("a new inconsistency to flag, not
  necessarily fix in this ticket"), this should be named in the PR
  description/plan but not silently fixed — fixing it would mean picking a
  *third* independent hex value with its own contrast math against
  `accentBg`, which is a different, unscoped problem.

## 4. General WCAG/axe-core contrast pitfalls — checked against this specific case

- **Large-text 3:1 vs normal-text 4.5:1 threshold**: WCAG's relaxed 3:1
  threshold only applies to text that is ≥18pt (24px) regular weight, or
  ≥14pt (~18.66px) **and** bold (≥700) — axe-core implements this same
  cutoff. The flagged "Next" button
  (`ModalTour.css.ts:146-152`) uses `vars.fontSize.sm` (14px) at
  `vars.fontWeight.semibold` (600). 14px is below the 18.66px bold-text
  floor regardless of weight, and 600 is technically below the "bold" (700)
  weight axe-core's large-text detection typically expects — so this text
  unambiguously falls under the **stricter 4.5:1 normal-text rule**, matching
  the ticket's own stated threshold. There is no large-text loophole here;
  double-checked so the plan doesn't accidentally target 3:1.
- **Manual-calculation vs axe-core rounding**: axe-core (via `color-contrast`
  from `dequeue`/`axe-core`) computes the same WCAG 2 relative-luminance
  formula as any manual sRGB calculation and does not have known systematic
  rounding divergence at the precision this fix needs (targeting comfortably
  above 4.5:1, e.g. the retro fix landed at 5.27:1 with ~0.77 ratio points of
  margin). The practical pitfall is not calculation precision — it's picking
  a replacement hex that is *just barely* above 4.5:1 (e.g. 4.51:1), which
  can tip back under threshold from font subpixel/anti-aliasing-driven
  measurement noise or a future minor palette tweak. **Recommendation for the
  plan phase**: pick a value with real margin (≥5:1), following the retro
  theme's own precedent margin (5.27:1) rather than the ticket's minimum
  bar.
- Axe-core's `color-contrast` rule does account for `font-weight`/`font-size`
  correctly per above, and does not evaluate `:hover`/`:focus`/`:active`
  pseudo-states unless the test framework explicitly triggers them first
  (confirmed no such triggering exists in this spec file — see §2). It also
  skips elements it cannot statically resolve contrast for (fully transparent
  backgrounds, background images) — not applicable here, `primary` is a flat
  color.

## 5. Snapshot/visual-regression/Storybook risk

- **No Storybook file references `#6366f1` or `primary` in a way that pins
  the hex.** `DrawerNav.stories.tsx` was checked directly — its only "Next"
  match is an unrelated comment about mocking `NavigationContext` for
  Next.js; it does not reference the pagination "Next" button, ModalTour, or
  any hex literal. No Storybook update needed.
- **Visual regression snapshots WILL need regenerating.** `tests/e2e/visual-regression.spec.ts`
  runs a `visual-clean` Playwright project (`tests/e2e/tests/snapshots/visual-clean/`)
  with committed PNG baselines (`session-list-empty.png`, `omnibar-open.png`)
  and a **1% max-pixel-diff threshold** (`maxDiffPixelRatio: 0.01`) enforced
  in CI with no `--update-snapshots`. `vars.color.primary` is used
  extensively on these exact pages (Header, Omnibar, buttons, focus rings —
  142 `.css.ts` files reference it directly). A hue/lightness change to
  `primary` is very likely to exceed 1% pixel diff on the `visual-clean`
  baselines and fail CI independent of the accessibility fix being correct.
  **Action for implementation**: after changing `primary`, regenerate
  clean-theme baselines with
  `npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean`
  and commit the updated PNGs alongside the token change, or the PR will show
  an unrelated-looking visual-regression CI failure.
- **`web-app/scripts/check-theme-contrast.ts` (`npm run check-contrast`) is
  already stale and should not be trusted to validate this fix.** It's a
  second, fully independent, hand-maintained copy of a subset of theme colors
  (only 4 of the 6 themes: `matrix`, `cyberpunk77`, `wh40k`, `clean` — missing
  `light`/`dark`), and its `clean.primary` value is **`#7c3aed`**, not
  `#6366f1` — it does not match `theme.css.ts`'s actual current clean-theme
  primary at all, predating this fix. It is not wired into `make ci`, any
  Makefile target, or `.github/workflows/*` (confirmed by grep — the only
  reference is the `package.json` script definition itself), so it won't
  block or falsely validate this PR either way, but don't reach for
  `npm run check-contrast` as a sanity check on the new hex — it's checking a
  value from a different, disconnected fork of the token data and will give a
  misleading (stale) answer either before or after this fix. This is a
  pre-existing maintenance gap, out of scope to fix here, but worth a
  one-line callout in the plan so nobody wastes time trying to make that
  script "pass" as proof of the fix.

## Summary of actionable flags for the plan/implementation phase

1. Token-only edit to `cleanTheme.primary` (theme.css.ts:636) is sufficient
   for the stated acceptance criteria and IT-5.1 as currently written.
2. `primaryHover` (#818cf8) already fails worse (~2.98:1) than the reported
   bug on the same "Next" button's `:hover` state, but is invisible to
   IT-5.1 (no `.hover()` in the test) — call this out explicitly as
   pre-existing/out-of-scope rather than silently leaving it undocumented.
3. `accentText` and `borderHover` are same-hex sibling tokens that will not
   move with `primary` and will visually drift — expected/matches existing
   per-theme precedent, name it in the PR description, don't fix it here.
4. Pick a replacement hex with real margin above 4.5:1 (≥5:1, matching the
   retro fix's 5.27:1), not a bare-minimum pass.
5. Regenerate `tests/e2e/tests/snapshots/visual-clean/*.png` baselines
   (`--update-snapshots --project=visual-clean`) as part of this change, or
   `visual-regression.spec.ts` fails CI on an unrelated-looking pixel diff.
6. Ignore `npm run check-contrast` / `check-theme-contrast.ts` as a
   verification tool for this fix — its clean-theme data is already stale
   and disconnected from `theme.css.ts`.
