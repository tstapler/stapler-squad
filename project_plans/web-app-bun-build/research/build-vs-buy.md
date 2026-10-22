# Build vs. Buy: web-app Bun build

Date: 2026-10-19. Phase 2 research. Labels: VERIFIED = command run or source opened this session; INFERRED = reasoning, not checked. Registry facts come from `registry.npmjs.org` and `api.npmjs.org` queried 2026-10-19. Bundler timings are not redone; see `docs/explanation/bun-migration-feasibility.md` (sections 2, 4, 9).

## Summary table

| Piece | Recommended | Fallback |
|---|---|---|
| 1 Bundler / dev server | Bun.build (owner's choice holds, with a gate) | Vite 8 + official VE plugin |
| 2 Router | React Router 7 library mode (7.18.4, pinned) behind `@/lib/nav` | TanStack Router >= 1.170.0 |
| 3 VE on Bun | `bun patch` of `eval`, plus upstream PR | vendored evaluator in the plugin |
| 4 Storybook | Keep, it already runs on `react-webpack5`; drop `@storybook/nextjs` | Remove Storybook |
| 5 Lint | Keep ESLint 9 and the custom plugins; replace `next/core-web-vitals` with standalone plugins | none |
| 6 web-vitals | `web-vitals` npm package | PerformanceObserver |
| 7 HTML | Hand-written `index.html` + Bun HTML entry | small post-build script |
| 8 Custom vs library | Custom is acceptable for the VE plugin and facade; not for the router | see section 8 |

## 1. Bundler / dev server

Facts (VERIFIED unless noted): Vite 8.3.4, MIT, 118.8M weekly downloads, published 2026-10-08, npm provenance attestations present. Rsbuild 2.2.14, MIT, 0.89M weekly, published 2026-10-19, attestations present. Bun 1.4.3, MIT, 2.5M weekly on the `bun` npm shim, published 2026-10-18, no attestations on the npm package. Measured by the feasibility doc: Rsbuild 6.2-6.6 s and Vite 7 15.6-16.1 s, both including vanilla-extract (VE); Bun 2.7-3.0 s one pass. Vite 8 at 5.75-6.25 s is the figure in `requirements.md`; it was not re-measured here.

### Bun.build (chosen)
- Pros: fastest measured (about 2.9 s one pass, 84 ms `.css.ts` reload); one runtime for install, scripts and build; no webpack/Rollup layer; the plugin is about 25 lines (spike).
- Cons: young bundler. Only one component was ever exercised in the dev server (feasibility doc, section 9). Custom plugin and patched dependency are owned by us. Official docs mark the HTML-import dev server as a dev-only path and recommend `Bun.build` for production (docs: https://bun.com/docs/bundler/fullstack; a mirror's "Fast Refresh on by default" claim is third-party, INFERRED stale). `bun test` segfaulted in the spike (separate from the bundler, but a maturity signal). Smaller plugin ecosystem than Vite/Rsbuild.
- Verdict: Recommended, conditional on the fallback gate below.

### Vite 8 (+ `@vanilla-extract/vite-plugin`)
- Pros: official VE plugin (VE maintainers' own path), largest ecosystem, no custom build code, mature HMR/Fast Refresh, `@tanstack/router-plugin` and React Router both first-class.
- Cons: slower than Bun here (about 6 s vs about 3 s); still needs HTML/route work; requires re-measuring the Vite 8 figure with the real app.
- Verdict: Viable. Best fallback.

### Rsbuild 2 (+ `@vanilla-extract/webpack-plugin`)
- Pros: 6.2-6.6 s, VE worked first try, webpack-compatible (would also unify Storybook's `react-webpack5` builder), minifier silently dropped invalid keyframes.
- Cons: Rspack-era plugin compat; the same speed as Vite 8 with a smaller community; silent CSS dropping hides bugs.
- Verdict: Viable.

### Is the owner's choice sound? (INFERRED, from the measured numbers)
Yes. Success metric is "<= 10 s production build"; Bun, Vite 8 and Rsbuild all meet it, so Bun's edge is margin and toolchain unification, not necessity. The risk is dev-server fidelity, which is unvalidated. Fall back to Vite 8 if any of these gate checks fails during the build-entry PR:
1. Full app (all 41 routes, real router) does not render and hot-reload in `bun --hot`/HTML-entry dev mode with React Fast Refresh preserving state.
2. Production bundle exceeds 6.35 MB JS, or code splitting of the 4 `next/dynamic` sites does not produce lazy chunks.
3. A Bun bundler crash/miscompile appears in two or more CI runs (not a one-off).
4. CSS chunk ordering for VE output differs from expectation and causes visible regressions that e2e/Axe catch.
5. The `.css.ts` evaluation cannot be kept green after a VE minor upgrade within a day's work.
Because the router and facade work is bundler-independent, a fallback costs only the plugin/entry PR.

## 2. Router

Facts (VERIFIED): `react-router` latest is 8.4.0 (2026-10-14, MIT, 33.3M weekly, maintainers brophdawg11, mjackson, attestations present). 8.x requires Node >= 22.22 and React >= 19.2.7; the repo declares `react ^19.0.0`, so 8.x forces a React bump. The `version-7` dist-tag is 7.18.4 with peer `react >= 18` and Node >= 20. `@tanstack/react-router` latest 1.170.45 (published 2026-10-16, MIT, 12.8M weekly, maintainers tannerlinsley, alemtuzlak, kevinvandy, attestations present). `wouter` 3.13.0 (Unlicense, 1.26M weekly, single maintainer molefrog). `type-router-react` and `@t8/react-router`: the exact names returned "Not found" on the npm registry, so they could not be verified at all (UNVERIFIED; do not adopt).

### TanStack incident status (VERIFIED from the postmortem and the registry)
- Postmortem: https://tanstack.com/blog/npm-supply-chain-compromise-postmortem. On 2026-05-11, 19:20-19:26 UTC, 84 malicious versions across 42 `@tanstack/*` packages were published, two per package. For `@tanstack/react-router` those were 1.169.5 and 1.169.8 (registry timestamps 19:20:42 and 19:26:17). Cause per the postmortem: a `pull_request_target` misconfiguration, cache poisoning and OIDC token extraction from the runner. Advisories: Snyk https://security.snyk.io/vuln/SNYK-JS-TANSTACKREACTROUTER-16640208, CVE-2026-45321, GHSA-g7cv-rxg3-hmpx.
- The malicious publishes used the legitimate trusted-publisher OIDC binding, so provenance would not have flagged them. VERIFIED consequence: attestation presence is not a safety signal for this package.
- Status: the postmortem's 2026-05-15 update says every currently available version is safe; affected versions were deprecated and removed. Registry check: 1.169.5 and 1.169.8 have no attestations and 1.170.0 (2026-05-15) is the first with attestations; the latest 1.170.45 carries SLSA provenance. Whether the publish path changed (trusted publishing vs tokens) is NOT confirmed: the postmortem lists it as an open lesson and defers details to a companion post ("Hardening TanStack After the npm Compromise") I did not read. Verified hardening only: SHA-pinned actions, `repository_owner` guards, full cache purge.
- Safe versions: any version >= 1.170.0 of `@tanstack/react-router` (and plugin packages) post-dates the sweep; pin exactly, and add a repo check that rejects 1.169.5/1.169.8 and anything published 2026-05-11.

### React Router 7 library mode (behind the facade)
- Pros: most-used router (33M weekly), MIT, large maintainer team (Remix/Shopify), stable v7 line (7.18.4, Node >= 20, React >= 18 so no React bump), `useSearchParams`, `Outlet` nested layouts, `<ScrollRestoration>`, `NavLink` active state match the Next surface used (22 `useSearchParams`, 66 push/replace, 16 layouts). Closest to the current mental model, so the facade is nearly a re-export.
- Cons: search params are untyped strings; the typed route features are framework-mode only (INFERRED from the feasibility doc, which flagged it unverified), so about 25 query keys need hand-written parse helpers in the facade. Major version churn (v8 already out). No compromise history found, but no incident search was done beyond TanStack.
- Verdict: Recommended (pin 7.x; treat v8 as a later upgrade).

### TanStack Router
- Pros: typed search params, file-based routes, code-splitting plugin; best fit for 25 query keys.
- Cons: May 2026 compromise (resolved, but the publishing-path hardening is unconfirmed); file-route codegen plugin is a second dependency to trust; routes would be re-expressed rather than facaded; bigger conceptual change.
- Verdict: Viable (>= 1.170.0, exact pin, `ignore-scripts` posture). Not a first choice for supply-chain risk and migration size.

### wouter
- Pros: tiny (about 2 KB, INFERRED), simple hooks.
- Cons: single maintainer; no nested layouts as first-class; search params minimal; the 16 layouts and scroll restoration would be hand-built.
- Verdict: Not recommended.

### type-router-react / @t8/react-router
- Not found on npm under those names (VERIFIED 404). Verdict: Not recommended.

### Hand-rolled router behind the facade
- Pros: zero dependencies; the spike shim was about 12 lines.
- Cons: grows into a router (nested layouts, scroll, active state, blockers, code splitting), all LLM-authored and owned by us.
- Verdict: Not recommended as the production router. Acceptable only as the test double behind the facade.

## 3. vanilla-extract on Bun

Facts (VERIFIED): `eval` 0.1.8, last published 2022-06-17, single maintainer (pierrec), no license field in the registry metadata, 1.65M weekly. VE bumped it to 0.1.8 in PR #1167 (merged). Upstream issue https://github.com/vanilla-extract-css/vanilla-extract/issues/1664 "Vanilla Extract + Bun Runtime Incompatibility" is open since 2025-12-05 with no comments and the same symptom (`require('@vanilla-extract/css/adapter')` undefined). VE is MIT, `@vanilla-extract/integration` 8.0.11 published 2026-10-08 (active).

### `bun patch` of `eval`
- Pros: smallest diff (tested: 336/336 files, output identical to Node); `bun patch` records it in `package.json`/`patches/`; removable when upstream lands.
- Cons: patch must be re-applied if `eval` or VE changes; patches the transitive dep of `@vanilla-extract/integration`, so version drift breaks it loudly (good) but needs a pin.
- Verdict: Recommended, plus a CI test that evaluates all `.css.ts` and diffs against Node.

### Vendor a plugin-side evaluator
- Pros: no dependency patching; uses `compile()` from `@vanilla-extract/integration` and a local `vm` + `createRequire` instead of `processVanillaFile()`'s eval path.
- Cons: re-implements part of `processVanillaFile` (INFERRED); copies VE internals that can change.
- Verdict: Viable; fallback if `bun patch` proves brittle.

### Upstream PR
- Pros: removes the problem for everyone (issue #1664 shows demand); replacing `eval` with `vm`/`createRequire` is small.
- Cons: not on our timeline; maintainer acceptance unknown; Bun's `Module._load` returning `undefined` is arguably a Bun bug, so a Bun-side report is a second route.
- Verdict: Recommended in parallel, not as a blocker.

### Fork
- Cons: owning a fork of `@vanilla-extract/integration` for a 10-line fix.
- Verdict: Not recommended.

## 4. Storybook

Facts (VERIFIED in the worktree): `.storybook/main.ts` already sets `framework.name` to `@storybook/react-webpack5`, not `@storybook/nextjs`, although `package.json` still lists `@storybook/nextjs` (line 136) next to `@storybook/react-webpack5`. 50 `*.stories.tsx` files. Scripts `storybook` and `build-storybook` exist. No workflow in `.github/` or the Makefile references Storybook or Chromatic (grep found only the feasibility doc), although `@chromatic-com/storybook` is installed. The `.storybook/css-ts-mock-loader.js` mocks `.css.ts`.
- Keep (react-webpack5, drop `@storybook/nextjs`): Pros: already configured and independent of Next; removing one dependency finishes the coupling cleanup; Cons: still a second bundler (webpack) in devDependencies; story coverage is 50 files vs 400+ components and not run in CI, so rot risk. Verdict: Recommended (minimum work; confirm `storybook build` still passes once `next` is gone, esp. `next/*` imports in stories and the new facade).
- Move to `@storybook/react-vite`: Pros: one fewer webpack; Cons: needs the VE Vite plugin plus the mock loader replaced; extra work for something with no CI use. Verdict: Viable.
- Remove Storybook: Pros: drops about 10 devDependencies, 50 files, and the second bundler; Cons: lose a visual catalog the owner may use locally. Verdict: Viable; owner decision. If retained, add a CI `build-storybook` job or accept rot.

## 5. Lint

Facts (VERIFIED): `.eslintrc.json` extends `next/core-web-vitals`; `package.json` runs `next lint`; custom plugins `eslint-plugin-boundaries`, `analytics`, `rpc-lifecycle`, 10+ `no-restricted-syntax` ESTree selectors. `@next/eslint-plugin-next` 16.4.2 exists standalone (MIT, 24.5M weekly), but its rules (`no-img-element`, `no-html-link-for-pages`, `no-sync-scripts`) are Next-specific. `next/core-web-vitals` bundles `eslint-plugin-react`, `react-hooks`, `jsx-a11y`, `import` (INFERRED from eslint-config-next structure; not re-read).
- Standalone plugins on ESLint 9 (`eslint-plugin-react-hooks`, `-react`, `-jsx-a11y`, keep boundaries/analytics/rpc-lifecycle): Pros: preserves all custom rules and the `no-restricted-syntax` selectors verbatim; no new tool; the flat-config migration can be done once. Cons: need to list which core-web-vitals rules are actually enforced (open question in requirements); `next lint` is removed upstream in newer Next, so an `eslint` CLI call is needed anyway. Verdict: Recommended.
- Biome: Pros: very fast (2.5.15, MIT/Apache-2.0, 10.9M weekly). Cons: no ESLint custom-plugin API (INFERRED; the three in-house plugins and ESTree selector rules cannot port without rewriting). Verdict: Not recommended.
- oxlint: Pros: fast (1.88.0, MIT, 16.1M weekly, attestations). Cons: JS-plugin support maturity not verified (UNVERIFIED), would likely need ESLint alongside for the custom rules. Verdict: Viable as an additive fast pre-pass; not a replacement.

## 6. web-vitals

Facts (VERIFIED): `web-vitals` 6.2.3, Apache-2.0, Google (philipwalton), 30.9M weekly, published 2026-10-05, no npm attestations. `WebVitalsReporter.tsx:4` imports `useReportWebVitals` from the internal path `next/dist/client/web-vitals`, which wraps `web-vitals` itself (INFERRED).
- `web-vitals` npm: Pros: the reference implementation (attribution, INP/LCP/CLS edge cases, bfcache, visibility rules) that Next wraps, so metrics stay comparable with today; tiny (about 2 KB, INFERRED); Cons: one more dependency. Verdict: Recommended.
- Raw PerformanceObserver: Pros: no dependency; Cons: reimplements CLS session windows, INP interaction grouping and LCP finalization; any LLM-written version would silently drift from published definitions. Verdict: Not recommended.

## 7. HTML generation

- Hand-written `index.html` + Bun HTML entry: Pros: transparent, no plugin, FOUC script and `@font-face` are plain, Go already falls back to `index.html` (`server/middleware/static.go`), and a SPA needs one document. Cons: per-route `<title>` and `viewport` need a small head helper in the router layer; `trailingSlash`/`dir/index.html` deep links rely on the Go fallback, so e2e must confirm (open question). The FOUC script depends on build-time VE class names (feasibility doc), so it must be generated from the VE output or reference stable `data-*` attributes. Verdict: Recommended.
- Plugin (HTML templating/prerender): Pros: per-route static HTML; Cons: nothing needs prerender (no SSR, no SEO); adds a dependency and build time. Verdict: Not recommended.
- Post-build script emitting per-route copies of `index.html`: Cons-only unless e2e proves `dir/index.html` is required. Verdict: Viable, only if needed.

## 8. LLM-generated bespoke code vs libraries

Rule applied (INFERRED, a judgment): custom code is acceptable when it is (a) small, (b) fully covered by a deterministic test that compares against a reference, (c) behind a seam that a library can replace, and (d) not security- or correctness-critical in ways tests cannot see. It is reckless when it re-implements a well-specified, stateful, edge-case-heavy domain.

| Piece | Verdict | Why / required guard |
|---|---|---|
| VE Bun plugin (about 25 lines) | Acceptable | Reference oracle exists: the 336 `.css.ts` outputs must be byte-identical to the Node/Vite plugin build (already done in the spike). Keep as a CI diff test. |
| Navigation facade `@/lib/nav` | Acceptable | Pure re-export/adapter with typed search-param helpers; its value is the seam. Existing test mocks collapse to one module. |
| Router implementation | Reckless | History, nested layouts, scroll restoration, blocking, active-link state, code splitting: a router. Use a library. |
| Typed search-param parsing helpers | Acceptable if tiny | Prefer an existing validator (the repo already uses protobuf/zod? unverified); tests per key. |
| web-vitals measurement | Reckless | See section 6. |
| Head/title helper | Acceptable | `document.title` per route; tested by e2e. |
| Build orchestration script (copy `out/`, define, fonts) | Acceptable | Verified by size/structure checks in CI. |

Standing guards for all custom pieces: pin dependency versions, a differential test against the Node/Vite output, one owner doc (ADR) stating the replacement trigger.

## Open items for planning
1. Owner confirms RR7 vs TanStack; RR7 recommendation assumes 7.x pin, not 8.x (React >= 19.2.7 required by 8.x).
2. Read the TanStack companion hardening post before any TanStack adoption (not read here).
3. Decide Storybook keep/remove; confirm `@storybook/nextjs` is unused (main.ts says so; package.json still lists it).
4. Enumerate enforced `next/core-web-vitals` rules by running ESLint with `--print-config`.
5. oxlint JS-plugin and Biome plugin claims are INFERRED; re-verify only if either becomes a candidate.
6. `eval` has no license field in registry metadata; check its repository before vendoring any of its code (node-eval, pierrec).

## Sources
- https://tanstack.com/blog/npm-supply-chain-compromise-postmortem
- https://security.snyk.io/vuln/SNYK-JS-TANSTACKREACTROUTER-16640208
- https://github.com/vanilla-extract-css/vanilla-extract/issues/1664
- https://bun.com/docs/bundler/fullstack
- https://registry.npmjs.org/ and https://api.npmjs.org/downloads/point/last-week/ (queried 2026-10-19 for all version/maintainer/download figures)
- Repo: `web-app/.storybook/main.ts`, `web-app/.eslintrc.json`, `web-app/package.json`, `web-app/src/components/telemetry/WebVitalsReporter.tsx`, `docs/explanation/bun-migration-feasibility.md`
