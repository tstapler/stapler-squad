# Stack Research: pagination Next button color-contrast fix

## 1. Token architecture — vanilla-extract theme contract

- Contract lives in `web-app/src/styles/theme-contract.css.ts` (`createThemeContract`).
  The relevant slots (theme-contract.css.ts:33-38):
  ```
  primary: null,
  primaryHover: null,
  primaryActive: null,
  primaryDark: null,
  primaryText: null,
  ```
  **The actual token name is `vars.color.primary`, not `vars.color.actionPrimary`**
  — `actionPrimary` (used as an illustrative example in
  `.claude/rules/css-architecture.md`'s sample `Button.css.ts` recipe snippet)
  does not exist anywhere in this codebase. Confirmed via
  `grep -rn "actionPrimary" web-app/src` → no matches.
- `web-app/src/styles/theme.css.ts` defines 6 themes via `createTheme(vars, {...})`:
  `lightTheme` (64), `darkTheme` (168), `matrixTheme` (273), `cyberpunk77Theme`
  (385), `wh40kTheme` (497), `cleanTheme` (609). Each supplies concrete hex
  values for every contract slot.
- Clean theme's `primary` is at **theme.css.ts:636**, confirmed via
  `grep -n 'primary: "#6366f1"' web-app/src/styles/theme.css.ts` → `636:    primary: "#6366f1",`.
  `primaryText` is `#ffffff` (theme.css.ts:640, unchanged).

## 2. Consumers of `vars.color.primary` — corrected scope

**Correction (post-review):** an earlier draft of this section claimed
`vars.color.primary` had "exactly two" consumers. That was false — re-verified
live via `grep -rn "vars\.color\.primary\b" web-app/src --include='*.css.ts' |
grep -v theme.css.ts`, which returns **412 matches across 142 files**. The
token is broadly consumed across the codebase, both as a `background`
(button/CTA role) and as a `color` (foreground-text role) in many components.
See `project_plans/pagination-next-color-contrast/implementation/plan.md`
Story 1.1.2 for the analysis of the foreground-usage blast radius.

What *is* still true and narrowly relevant here: the two files that pass
through the two specific visual-sanity-check surfaces named in requirement 5
(the failing element from the axe report, and the CTA button used for the
visual plausibility check) are:
- `web-app/src/components/ui/Button.css.ts` — `primary` intent variant:
  `background: vars.color.primary` (line 35), `color: vars.color.primaryText`
  (line 36), focus outline `vars.color.primary` (line 21).
- `web-app/src/components/ui/ModalTour.css.ts` — the tour/wizard modal's step
  progress dot (`vars.color.primary`, lines 63/67) and its **Next-step button**
  `primaryButton` (line 146-151): `background: vars.color.primary`,
  `color: vars.color.primaryText`.

Both of these two consumers pull the token through the shared `vars` contract
only — there is no second, parallel color source (no hardcoded hex, no CSS
custom property override) feeding either the Button CTA or the ModalTour Next
button. That much (no shadow/duplicate source for *these two* surfaces) is
still accurate. What is **not** accurate is any claim that these are the only
consumers in the codebase, or that theme.css.ts:636 is "the sole source" in
any global sense — it is the sole source for the *value*, but that value
fans out to 142 files, not 2.

### Resolving "pagination Next button"

There is no literal `Pagination` component in `web-app/src` (`find -iname
"*Pagin*"` → no results; no `aria-label="Next page"` etc. anywhere). The
failing element is the tour/wizard **step** Next button:
`web-app/src/components/backlog/BacklogTourModal.tsx` imports
`ModalTour.css.ts` (`import * as styles from "@/components/ui/ModalTour.css"`)
and renders `<button className={styles.primaryButton} onClick={handleNext}>` —
i.e. "pagination" in the requirements refers to paging through tour steps, not
a data-grid pager. This modal auto-renders on the main page for
not-yet-onboarded users, which is why axe's whole-page scan in IT-5.1
("Main page has no critical or serious accessibility violations",
`tests/e2e/accessibility.spec.ts:30`) catches it — no `.include()` scoping is
used on that test, unlike the narrower contrast test at line 316-320.
`OnboardingModal.tsx` has an equivalent Next button but uses its own
`OnboardingModal.css.ts`, not `ModalTour.css.ts` — not in scope.

## 3. Clean theme is the default theme (why IT-5.1 hits it)

`web-app/src/app/layout.tsx`:
- Server-rendered root className hardcodes `cleanTheme`: line 51,
  `` className={`${cleanTheme} ${jetbrainsMono.variable} ...`} ``.
- The FOUC-prevention inline script (line 46) falls back to `m['clean']` when
  `localStorage.getItem('stapler-theme')` is unset — i.e. clean is the
  effective default for a fresh browser context, which is what IT-5.1's
  "Main page" test uses (no theme fixture/localStorage setup, unlike the
  visual-regression project's per-theme `storageState` fixtures).

## 4. Lines that must stay byte-identical (regression guard)

- `theme.css.ts:412` — `cyberpunk77Theme.primary`:
  `"#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */`
  (the requirements doc's label "retro" refers to this theme colloquially —
  there is no theme literally named `retro`; `matrixTheme` is the only other
  candidate and it's untouched either way).
- `theme.css.ts:524` / `528` — `wh40kTheme.primary` (`"#c0a020"`, no comment)
  and `.primaryText` (`"#0c0a08"`).
  Neither theme's primary lines have a comment currently — only clean's
  edited line gains one, matching the convention already used at 389, 412,
  475, 477, 501 (inline `/* was <old> — <reason>; <new> = X.XX:1 ✅ */`).

## 5. Lint exemption mechanics — why hardcoded hex in theme.css.ts is fine

- `web-app/package.json`: `"lint:css": "stylelint 'src/**/*.css' --allow-empty-input"`
  — glob is `*.css` only. `theme.css.ts` is a `.ts` file (vanilla-extract),
  never matched by stylelint at all.
- `"lint:css-vars": "node scripts/check-css-vars.mjs"` — this script only
  validates `var(--xxx)` references inside CSS Modules files against
  `web-app/src/app/globals.css`'s custom-property definitions
  (`web-app/scripts/check-css-vars.mjs:18-32`); it has no hex-literal check
  and doesn't touch `.css.ts` files either.
- Root `make lint` (Makefile:601) runs `golangci-lint` + this repo's custom Go
  linters (`lint-custom`, Makefile:612) — **Go-only**, never invokes any
  web-app/pnpm script. So `make lint` passing after this change is procedural
  (nothing in it inspects `theme.css.ts`), not a meaningful CSS-safety check.
  Confirms requirements' "already exempted from that lint rule" — the
  exemption isn't a special-case allowlist, it's that no lint tool's glob
  reaches this file at all.

## 6. E2E harness — `make build`, `ensureBinary()`, snapshot regen

- `tests/e2e/helpers/test-server.ts:144-157` (`ensureBinary()`):
  ```ts
  const stats = await fs.promises.stat(this.config.buildPath).catch(() => null);
  if (stats && stats.isFile()) {
    const age = Date.now() - stats.mtimeMs;
    if (age < 3600000) { return; }   // reuse binary <1hr old
  }
  await execPromise('go build -o stapler-squad .', { cwd: projectRoot });
  ```
  `buildPath` defaults (line 41) to `path.join(__dirname, '../../../stapler-squad')`
  — i.e. the repo-root `./stapler-squad` binary, the same output path
  `make build` produces. So: run `make build` at repo root immediately before
  `npx playwright test` — this refreshes the binary's mtime, so
  `ensureBinary()`'s own rebuild is skipped (age ≈0 < 1hr) and the server
  spawned by `global-setup.ts` is guaranteed to be the just-built one, not a
  stale cached binary from over an hour ago.
- Test server is fully auto-managed per `CLAUDE.md`'s E2E Tests section
  (`global-setup.ts` spawns an isolated instance on a dynamically-assigned
  free port) — no manual server start needed.
- Command per requirements: `cd tests/e2e && npx playwright test
  accessibility.spec.ts --reporter=line`. Confirmed both a `chromium` and a
  `chromium-dom` project exist in `tests/e2e/playwright.config.ts` (line
  104+ `chromium`; a DOM-renderer variant referenced around line 115-118)
  that both run this spec, matching the requirement's "both `[chromium]` and
  `[chromium-dom]`" pass condition.

### Visual snapshot regeneration

- `tests/e2e/playwright.config.ts:62-63`: `snapshotPathTemplate:
  'tests/snapshots/{projectName}/{testFilePath}/{arg}{ext}'`, `testDir: './'`
  (relative to the config file, i.e. `tests/e2e/`) — matches the existing
  directory `tests/e2e/tests/snapshots/visual-clean/`.
- Project `visual-clean` (lines 96-103) uses
  `storageState: 'tests/e2e/fixtures/clean-theme.json'` and
  `testMatch: '**/visual-regression.spec.ts'` → the spec file is
  `tests/e2e/visual-regression.spec.ts`.
- No npm/package.json script wraps snapshot updates — it's the raw Playwright
  CLI flag. **Exact regen command** (scoped to only the clean-theme project,
  per requirement #5's scope — do not regenerate matrix/cyberpunk77/wh40k
  snapshots, which are unaffected):
  ```bash
  cd tests/e2e && npx playwright test visual-regression.spec.ts \
    --project=visual-clean --update-snapshots
  ```
  Run this against a fresh `make build` binary for the same staleness reason
  as above, then `git add tests/e2e/tests/snapshots/visual-clean/` and commit.

## 7. Contrast verification (WCAG 2.1 relative luminance formula, computed)

| Color pair | Ratio |
|---|---|
| `#ffffff` on `#6366f1` (current) | **4.467:1** — matches requirements' cited 4.46:1, fails 4.5:1 AA threshold |
| `#ffffff` on `#5457ef` (proposed) | **5.268:1** — passes, comment should read `5.27:1` to match the 2-decimal-place convention used elsewhere (e.g. `5.27:1` at line 412, `5.06:1` at 641) |

Proposed replacement comment for theme.css.ts:636, matching the existing
convention exactly:
```
primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.46:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */
```
