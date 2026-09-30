# Pitfalls: pagination/tour "Next" button color-contrast fix

Research for backlog item `45d2cd48-ceed-4f99-af21-a401abd9cbe5`. All findings
below are VERIFIED by reading the actual files listed (paths are relative to
repo root unless noted).

## 0. What "pagination Next button" actually is

There is no literal list-pagination "Next" control on the routes IT-5.1 scans
(`/` and `/review-queue`). `web-app/src/app/history/page.tsx` has a real
paginated-list "Next" button, but it uses the **legacy** `className="btn
btn-primary"` (globals.css bridge var), not the VE token, and IT-5.1 never
visits `/history`. The `getByRole("button", { name: "Next" })` usage in
`web-app/src/components/backlog/BacklogTourModal.test.tsx` and
`web-app/src/components/onboarding/__tests__/OnboardingModal.hooks.test.tsx`
points at the real culprit: a step-navigation "Next" button in a tour/onboarding
modal (rendered via `ModalTour.css.ts`'s `primaryButton`, which the requirements
doc also names explicitly for visual sanity-check). That component consumes
`vars.color.primary` / `vars.color.primaryText` directly
(`web-app/src/components/ui/ModalTour.css.ts:150-151`), confirming the
vanilla-extract token — not any legacy CSS var — is what actually reaches the
failing DOM node. This matters for point 3 below: verify the modal is actually
visible in whatever page state IT-5.1 (or the regenerated visual snapshots)
capture, don't assume it based on the word "pagination."

## 1. `primary` reused elsewhere with a different contrast requirement?

`web-app/src/styles/theme.css.ts` clean theme block (lines 627-657) hardcodes
`#6366f1` **three separate times**, only one of which is `primary`:

```
631:    borderHover: "#6366f1",   // non-text UI component (3:1 threshold, not 4.5:1) — untouched by this fix
636:    primary: "#6366f1",        // <- the only line requirements ask to change
657:    accentText: "#6366f1",     // text color on accentBg — untouched by this fix
```

`accentBg: "rgba(99,102,241,0.1)"` (line 654) is the literal rgba decomposition
of `#6366f1` too. None of `borderHover`, `accentText`, `accentBg` derive from
`vars.color.primary` — they're independently hardcoded. Requirements correctly
scope the edit to line 636 only, and per requirement 2 those three stay
untouched. Consequence: before the fix, `primary`/`borderHover`/`accentText`
were all identical (`#6366f1`); after, `primary` becomes `#5457ef` while the
other two remain `#6366f1`. This is not a contrast regression (each pair's own
ratio is unchanged) but it is a new, previously-absent color mismatch between
the button/CTA color and the accent border/text color in the same theme —
worth a one-line callout in the PR description so it doesn't read as an
oversight, even though it's out of scope to fix here.

`theme-contract.css.ts:55-58` has a pre-existing comment confirming
`accentText` was deliberately given its own per-theme value specifically
*because* `primary` alone doesn't reliably hit 4.5:1 against `accentBg` in
every theme — i.e. this divergence pattern is an established, intentional
precedent in this codebase, not a new problem.

## 2. Other hardcoded `#6366f1` literals (repo-wide grep)

```
web-app/src/app/globals.css:46:                    --primary: #6366f1;
web-app/src/app/insights/ModelBreakdownChart.tsx:33:  "#6366f1",
web-app/src/styles/theme.css.ts:631,636,657          (see §1)
```

- **`globals.css:46`** — a *legacy* CSS custom property, explicitly called
  "bridge-mapped" in the file's own header comment (lines 1-9), claimed
  fully migrated ("Story 1.5 COMPLETE"). That claim is stale: `var(--primary)`
  is still directly consumed by several components that use the legacy
  `.btn-primary` class or reference `var(--primary)`/`var(--primary-text)`
  inline, confirmed via grep:
  `web-app/src/app/config/ConfigPageContent.tsx`,
  `web-app/src/app/history/page.tsx`,
  `web-app/src/components/history/ForkModal.tsx`,
  `web-app/src/components/history/HistoryFilterBar.tsx`,
  `web-app/src/components/history/HistoryDetailPanel.tsx`,
  `web-app/src/components/settings/GlobalDefaultsForm.tsx`,
  `web-app/src/components/settings/AliasesManager.tsx`,
  `web-app/src/components/settings/ProfilesManager.tsx`,
  `web-app/src/components/settings/DirectoryRulesManager.tsx`.
  These buttons will keep rendering the **old** `#6366f1` after this fix
  ships, producing a visible two-tone indigo inconsistency between
  VE-token-driven primary buttons (new `#5457ef`) and legacy-CSS-var-driven
  primary buttons (still `#6366f1`) elsewhere in the app. Requirements
  explicitly rule this out of scope ("token-only edit... do not touch any
  file that consumes the primary token"), and `globals.css` is not itself a
  *consumer* of `vars.color.primary` (it's a parallel, independently-defined
  value) — so this is not a violation of requirement 2, but it is a real,
  visible, unaddressed inconsistency worth naming in the PR body so it isn't
  mistaken for a bug in review.
- **`ModelBreakdownChart.tsx:33`** — a standalone categorical chart-color
  palette entry, unrelated to the theme/token system entirely. No action
  needed; flagging only so it isn't mistaken for a missed consumer.

## 3. Risk of regenerating visual-regression snapshots blind

`tests/e2e/tests/snapshots/visual-clean/visual-regression.spec.ts/` contains
exactly **two** PNGs: `session-list-empty.png` and `omnibar-open.png`
(`tests/e2e/visual-regression.spec.ts:26-48`). Both are **full-page**
screenshots (`await expect(page).toHaveScreenshot(...)`, no element scoping),
asserted with `maxDiffPixelRatio: 0.01` (up to 1% of total page pixels may
already differ without failing).

Regenerating via `--update-snapshots --project=visual-clean` (the documented
command in the spec's own header comment, lines 11-15) accepts **the entire
current render** as the new golden baseline — not just the pixels touched by
the token change. Two concrete risks:

1. **Masking an unrelated regression.** Any other in-flight visual drift
   present at regeneration time (uncommitted layout change, a flaky
   session-card ordering artifact, a font-loading race despite
   `reducedMotion`) gets silently baked into the new baseline and never
   flagged again.
2. **The fix may not even be visible in either baseline.** Neither screenshot
   name suggests it captures an open tour/onboarding modal (`session list
   empty` and `omnibar open` are both plausible states with no modal on
   screen) — confirm the changed button is actually in-frame before trusting
   a near-zero diff as validation of the fix. If it isn't, blindly running
   `--update-snapshots` may produce a byte-identical (or near-identical) PNG
   that "passes" without proving anything about the color change.

Mitigation: after regenerating, diff old vs. new PNGs (`git diff --stat`
won't show pixel content; use an image diff or visually inspect) to confirm
the changed region is confined to plausible primary-colored UI, and confirm
at least one of the two flows actually renders the affected button/modal —
if not, this requirement's "visual sanity" goal isn't actually being
exercised by these two snapshots and should be checked manually via the
manual-instance workflow (`CLAUDE.md`'s "Manual/interactive testing" section)
instead.

## 4. `make lint` / CSS-token-rule exemption — verified against actual config, not just the doc

Three separate lint layers touch CSS in this repo; checked each directly:

- **stylelint** (`web-app/.stylelintrc.js`, invoked via
  `pnpm run lint:css` → `stylelint 'src/**/*.css' --allow-empty-input`,
  `web-app/package.json:18`): glob only matches `*.css`. `theme.css.ts` is a
  `.ts` file — never matched. No exposure.
- **`lint:css-vars`** (`web-app/scripts/check-css-vars.mjs`, run via
  `pnpm run lint:css-vars`): only walks `.module.css` files checking
  `var(--xxx)` references against `globals.css` definitions. Doesn't touch
  `.ts` files either. No exposure.
- **ESLint** (`web-app/.eslintrc.json:115-130`): there **is** a
  `no-restricted-syntax` rule targeting `files: ["**/*.css.ts"]` — which
  *does* match `theme.css.ts` — for
  `Property > Literal[value=/^#[0-9a-fA-F]{3,8}$/]`, i.e. hardcoded hex
  literals, with message "Hardcoded hex colors are not allowed in .css.ts
  files." **This rule is NOT actually exempted for theme.css.ts** — its
  severity is `"warn"`, not `"error"` (line 119: `"no-restricted-syntax":
  ["warn", ...]`). Every other hardcoded hex already in this file (all the
  existing `primary`/`primaryHover`/etc. values across all 6 themes) already
  triggers this same warning today — the new `#5457ef` literal is
  consistent with, not an exception to, that existing (non-blocking) noise.
  `web-app/package.json:17`'s `"lint": "next lint && ..."` does not pass
  `--max-warnings 0`, so `next lint` exits 0 regardless of these warnings.

- **Crucially: `make lint` (the command requirement 6 names) never invokes
  any of the above.** `Makefile:601-608`'s `lint` target runs only
  `golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet` plus
  `lint-custom` (project Go linters: hotpolllog, nocommandpattern,
  norawexec, tmuxsocketscope) — it is Go-only and never shells into
  `web-app/`. Since this change touches zero `.go` files, `make lint` will
  pass trivially regardless of whether the CSS change is correct — it
  provides **no actual signal** about the frontend lint pipeline. If the
  intent behind requirement 6 is "the frontend lint tooling is clean on this
  change," that requires separately running `cd web-app && npm run lint`
  (or `pnpm run lint`) — `make lint` alone does not exercise it.

## 5. e2e binary-staleness risk — deeper than the 1-hour cache alone

`tests/e2e/helpers/test-server.ts:144-157`'s `ensureBinary()`:

```ts
const stats = await fs.promises.stat(this.config.buildPath).catch(() => null);
if (stats && stats.isFile()) {
  const age = Date.now() - stats.mtimeMs;
  if (age < 3600000) { return; }  // reuse binary unconditionally if <1h old
}
console.log('Building Go binary...');
await execPromise('go build -o stapler-squad .', { cwd: projectRoot });  // NOTE: bare `go build`, not `make build`
```

Two distinct risks, not one:

1. **The documented one** (already named in requirement 3): any
   `./stapler-squad` binary present and under 1 hour old — regardless of
   whether it reflects the current source tree — is silently reused. A
   leftover binary from an unrelated earlier task in the same worktree can
   cause a false pass/fail with no error message.
2. **A sharper one: even when `ensureBinary()` *does* decide to rebuild, it
   runs bare `go build -o stapler-squad .`, not `make build`.** Per
   `Makefile:132`, the real `stapler-squad` target's dependency chain is
   `server/web/dist` ← `web-app/out` ← `$(WEB_FILES)` (`web-app/src/**`) —
   i.e. `make build` regenerates the Next.js static export and copies it
   into `server/web/dist` (which `server/web/embed.go:8`'s `//go:embed
   all:dist` bakes into the Go binary) whenever any file under
   `web-app/src` changed, including `theme.css.ts`. A bare `go build`
   embeds whatever is **already sitting in `server/web/dist`** — it does
   not touch the Next.js build step at all. So: if a stale/missing binary
   triggers `ensureBinary()`'s fallback rebuild *without* `server/web/dist`
   having first been refreshed by an explicit `make build` (or `make
   web-build`), the resulting binary will embed old web assets and the CSS
   fix will not actually be present in the tested binary — yet the test run
   will proceed without any error, since `go build` succeeds regardless.
   **The safe sequence is always: run `make build` yourself immediately
   before the Playwright run** (as requirement 3 already says), never rely
   on `ensureBinary()`'s own rebuild path to pick up a `web-app/src` change.
