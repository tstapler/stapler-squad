# Build vs. Buy: Contrast-Checking Tooling

Scope note: this is a token-level fix (one hex value in `web-app/src/styles/theme.css.ts`).
This research covers only what *tool* verifies the replacement color — not the fix itself.

## 1. Manual calculation vs. tool-assisted

**Finding: the repo already has a purpose-built contrast checker.** No need to reach for
`npx wcag-contrast` or add a new npm dependency.

- `web-app/package.json:34` — `"check-contrast": "npx --yes tsx scripts/check-theme-contrast.ts"`
- `web-app/scripts/check-theme-contrast.ts` implements the WCAG relative-luminance formula
  from scratch (no dependency — `hexToRgb` → `linearize` → `luminance` → `contrast`) and
  checks `primaryText`/`primary` plus three other pairs for all 4 themes, exiting 1 on any
  WCAG AA (4.5:1) failure.

**Caveat — the script's data has drifted from the source of truth.** Its `themes.clean`
object hardcodes `primary: "#7c3aed"`, but the actual current value in
`theme.css.ts:636` is `primary: "#6366f1"`. Verified:

```
node -e '...'  →  current #6366f1 vs #fff: 4.47:1   (matches requirements.md's 4.46:1, rounding)
                   script's stale #7c3aed vs #fff:   5.70:1
```

Running `npm run check-contrast` as-is right now would **not** catch this bug — it's
checking a color that hasn't matched `theme.css.ts` for some time. The script reads a
copy-pasted snapshot, not the live theme file.

**Recommendation:** don't hand-derive the luminance formula, and don't add a new
dependency for a one-off. Two good options, in order of preference:
1. Update the `clean` entry in `check-theme-contrast.ts` to the real current values
   (all fields, not just `primary`) and run `npm run check-contrast` to verify the
   candidate replacement — reuses existing, already-correct math, ~1 line diff.
2. If touching the checker script feels out of scope, a `node -e` one-liner using the
   same three-function formula (shown above) is a fine throwaway substitute — it's what
   this research used to confirm the 4.47:1 figure.

## 2. Existing convention and automated guard

The `retro` theme fix (`theme.css.ts:412`) was manual hex selection + inline comment,
not generated or lint-enforced:
```
primary: "#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */
```
This is the established in-file convention and the fix for `clean` should match it exactly.

**No CI/lint guard exists.** Confirmed by grep:
- `web-app/.stylelintrc.js` has no contrast rule (only structural/stylistic rules; explicitly
  notes cross-file var checking is delegated to a separate `check-css-vars.mjs` script — the
  same pattern *could* extend to contrast, but doesn't yet).
- `check-theme-contrast.ts` exists but is **not invoked from any CI workflow** — grepped
  `Makefile` and `.github/workflows/*.yml` for `check-contrast`/`check-theme-contrast`;
  only the `package.json` script definition itself matches. It's a manual, opt-in `npm run`
  command today, not a gate.

So today, contrast regressions are caught by (a) axe-core e2e tests at runtime
(`tests/e2e/accessibility.spec.ts`, which is how this bug was found) or (b) PR review —
not by any static/lint check at commit time.

**Out of scope for this ticket**, but worth flagging as a follow-up: wiring
`npm run check-contrast` into `make lint` or a CI step (e.g. `lint.yml`) would catch this
class of regression before axe-core e2e does, and fixing its stale `clean` snapshot at the
same time closes the gap this research found. Both are small, separable changes from the
one-line token fix this ticket covers.

## 3. Verdict

| Approach | Verdict |
|---|---|
| (a) Hand-calculate replacement hex + inline comment matching `retro`'s convention | **Recommended** — matches existing precedent exactly, no new tooling needed for the fix itself |
| (b) Adopt a new contrast-checking npm library (`wcag-contrast`, `color-contrast-checker`, etc.) | **Not recommended** — redundant; the repo already has an equivalent hand-rolled checker (`check-theme-contrast.ts`) with zero dependencies |
| (c) Write a one-off Node script (or fix + run the existing `check-theme-contrast.ts`) to verify the candidate value before committing | **Recommended** — use the existing script (after correcting its stale `clean` snapshot) or an inline `node -e` using its same formula; either is the leaner choice for a single verification and reuses proven, already-correct math |
