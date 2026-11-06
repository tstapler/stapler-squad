# Bun migration: measured before/after results (Story 8.3)

Measured 2026-10-22 on this Linux host (Manjaro, 24 CPUs, 61 GB RAM; host load average 2-14 throughout,
other agent sessions sharing the host — see each row's range) at commit `19803da19` (`main`, after PR #1144).
Bun 1.4.3, Node 26.8.1. Script: `scripts/measure-web-build.sh` for the build-time rows; commands are given
inline for the rest. `:8543` was never touched; no `make install-service` was run.

This is the gate Story 8.3's AC requires before PR 8 (the cutover) may merge — see the plan's
`project_plans/web-app-bun-build/implementation/plan.md`, Story 8.3. `web-app/scripts/__tests__/results-doc.test.mjs`
parses this file and fails the build if a required row is missing or has neither a number nor a stated reason.

## Provenance: reused vs. re-measured

| Row(s) | Source |
|---|---|
| M1, M2 (build time, the 5x ratio gate) | **Re-measured today** via `scripts/measure-web-build.sh 5` — the plan's earlier gates (S3's `baseline-timings.sh`) called the bundler directly (`bun run build`, `bun build/build.ts`), not through `make web-build`; this story's AC specifically requires the Makefile path ("including the Go embed copy"), which is a different protocol, and the commit has moved on since S3 (`c7d385890` to `19803da19`, including PR 4/5a/6a/#1144) |
| M3, F6 (total JS, minified and unminified) | **Re-measured today** (`WEB_MINIFY=1`/`WEB_MINIFY=0 bun build/build.ts`) — gate S3's number (5,680,778 B minified, at `c7d385890`) predates PR #1144, which specifically moved `react-markdown` out of the Initial JS chunk and changed these byte counts |
| M4 (dev-loop reload) | **Reused**: Next side from Task 0.4f's baseline (`next dev`, n=5); Bun side from gate S2 (PR 5b's dev server, `theme.css.ts` edit, single measurement) — both already in `project_plans/web-app-bun-build/research/spike-results.md`. Not re-measured: standing up the dev server for a matching n=5 Bun-side run was out of this story's scope (Story 8.3 is the build/footprint gate, not a dev-server gate) |
| M5 (Jest) | **Re-measured today** (`bunx jest --no-coverage`) |
| M6 (no `next` dependency) | Not measured by number — it is a binary fact, checked by `rg` below |
| F1-F5 (dependency footprint, "before") | **Reused** from Task 0.4f (gate S3), measured at `c7d385890`; current `main`'s `package.json` has since dropped one unrelated devDependency (`@storybook/nextjs`, -1), noted below |
| F1-F5 ("after") | **Not measured** — see the note on each row |
| D3 (binary size delta) | **Re-measured today** (`make build WEB_BUILD=next` vs `WEB_BUILD=bun`, `ls -la stapler-squad`) |

## Success metrics (`requirements.md`, 6 bullets)

| ID | Metric | Value | Status | Note |
|---|---|---|---|---|
| `M1` | Production-equivalent build (`make web-build`, warm median, n=5) | Next 33.90 s (range 33.51-36.49 s) / Bun 1.88 s (range 1.80-1.91 s) | PASS | Bun warm median is comfortably under the 10 s target. Cold medians (n=5): Next 61.51 s (range 57.67-84.68 s), Bun 1.86 s (range 1.85-2.01 s; Bun has no persistent build cache, gates S1/S3, so cold ≈ warm). Host load 4.0-14.1 during the Next cold runs |
| `M2` | `make web-build` end-to-end at least 5x faster than the 54-88 s baseline (THE GATE) | 33.90 s / 1.88 s = **18.03x** | **PASS** (≥ 5x) | Secondary ratio, Next's cache cleared before every run (R2-C8d, not a gate): 61.51 s / 1.88 s = 32.72x. Both numbers are far past the 5x floor; no owner decision needed under plan Story 8.3's "miss" clause |
| `M3` | Total shipped JS ≤ 6,349,052 B, minified build; deployed bundle minified | jsBytes 5,419,563 B (minified, `WEB_MINIFY=1`); initialJsBytes 1,310,179 B | PASS on the byte budget; **not yet met** on "deployed bundle minified" | The CI-enforced budget (`web-app/build/budgets.json`'s `totalJsBytes: 6349052`) passes with headroom (85.4% of budget) and is *lower* than gate S3's 5,680,778 B measured at `c7d385890` — consistent with PR #1144 (lazy-loading `BacklogComposeWidget` to drop `react-markdown` from Initial JS) reducing bytes further, not a regression. Per plan sequencing (ADR-004 amendment, Story 8.6/PR 8b), the deployed build ships **unminified** until the first Soak window passes — see F6 |
| `M4` | Dev server hot reload for `.css.ts` and React components | `.css.ts` token edit to computed-style change: Next 10,676 ms median (n=5, range 7,651-14,926 ms) vs. Bun ~100 ms (single measurement, confirmed under 150 ms, 0 full reloads) — about **100x** faster. Comment-edit-to-DOM (component reload): Next 566 ms median (n=5, range 489-1,108 ms); Bun: 0 full reloads observed in all 3 probed pages, but no matching n≥5 latency number was taken (see Provenance) | PASS on `.css.ts` (reused numbers; well under the 2 s target); **gap** on the component-reload latency comparison | Source: Task 0.4f (Next) and gate S2 §S2-csstoken/§S2-fastrefresh (Bun), `project_plans/web-app-bun-build/research/spike-results.md`. Bun's dev server lost React state on 2 of 3 probed pages (`SessionList`, `Omnibar`) — live module swap without state-preserving Fast Refresh, not a crash or full reload; gate S2's own verdict is AMBER on this point, owner sign-off pending |
| `M5` | Zero functional regression: Jest passes with no new failures | 781 suites, 9,121 tests (9,116 passed, 5 skipped), **0 failing** | PASS | `cd web-app && bunx jest --no-coverage`, 31.78 s. Compares favourably with gate S0's control (628 suites/7,801 tests at the #1100 boundary, 0 failing) and gate S3's point (760 suites/8,903 tests at `b9258fe35`, 0 failing): suite/test counts grew (more tests landed since), failures stayed at 0 |
| `M6` | No `next` dependency left in `web-app/package.json` | `next`, `eslint-config-next`, `@next/eslint-plugin-next`, `@vanilla-extract/next-plugin` all still present today | **not yet met** | Pending Story 9.2 (Phase 9, after the Soak and after PR 8/8b). `rg '"next"' web-app/package.json` today returns 4 matches; this is expected at this point in the rollout, not a defect |

## Footprint: dependency and install-time, before/after (Story 9.2's goal)

Footprint rows have no pass line (plan Story 8.3 AC): a row that does not shrink is stated, not failed.

| ID | Metric | Before (Task 0.4f, gate S3, `c7d385890`) | After (post-Story 9.2) | Note |
|---|---|---|---|---|
| `F1` | Direct dependency count (`dependencies` + `devDependencies`) | 65 + 59 = 124 | not yet measurable | Pending Story 9.2's dependency removal (`next`, `eslint-config-next`, `@next/eslint-plugin-next`, `@vanilla-extract/next-plugin`, possibly `@storybook/nextjs`). Current `main` (`19803da19`) is already 65 + 58 = 123 — one devDependency (`@storybook/nextjs`) was dropped since the baseline for unrelated reasons, not Story 9.2 |
| `F2` | Installed package count (`bun pm ls --all \| wc -l`) | 1,902 | not yet measurable | Pending Story 9.2; not re-measured today (would require a scratch worktree with the removals applied, which this story explicitly permits deferring rather than fabricating) |
| `F3` | `du -sk web-app/node_modules` | 1,416,080 KB (~1.35 GB) | not yet measurable | Pending Story 9.2. Host load was 27-30 during the Task 0.4f install runs (other agent sessions), so that baseline is n=1 per temperature, not the full n≥5 protocol — stated in gate S3's own record |
| `F4` | `bun install --frozen-lockfile`, cold cache | 32.68 s (n=1) | not yet measurable | Pending Story 9.2; same n=1 caveat as F3 |
| `F5` | `bun install --frozen-lockfile`, warm cache | 0.34 s (n=1) | not yet measurable | Pending Story 9.2; same n=1 caveat as F3 |
| `F6` | Total shipped JS, unminified (`WEB_MINIFY=0`, the deployed default until Story 8.6/PR 8b) | Next dev-mode baseline (pre-migration, `requirements.md`): 21.7 MB (21,666,571 B measured on the pnpm control, gate S0 table A) | Bun unminified: jsBytes **11,088,187 B** (re-measured today, `WEB_MINIFY=0 bun build/build.ts`) | Already **48.8% smaller** than the pre-migration unminified baseline, before minification is even turned on. `cssBytes` unminified: 4,682,536 B; `gzipJsBytes`: 2,385,522 B |

Task 9.2a (per the plan) re-measures F1-F5's "after" column on the real tree once Story 9.2 lands and updates this file; it is not fabricated here.

## Binary size delta (D3: linked source maps shipped in the embedded dist)

`make build WEB_BUILD=<next|bun>` (default `WEB_MINIFY=0`), then `ls -la stapler-squad`, each from a clean
`web-app/out`/`server/web/dist`/`stapler-squad`:

| | Dist size (`du -sk server/web/dist`) | `stapler-squad` binary size |
|---|---:|---:|
| `WEB_BUILD=next` | 55,268 KB | 208,592,410 B |
| `WEB_BUILD=bun` (unminified, incl. 137 linked `.map` files, decision D3) | 39,960 KB | 193,047,954 B |

| `D3` | Metric | Value | Status | Note |
|---|---|---|---|---|
| `D3` | Binary size delta (`go build` output, decision D3) | Bun binary is **15,544,456 B (14.82 MiB) smaller**, 7.45% | INFO (no pass line) | Despite shipping linked source maps for all 137 chunks (D3's "keep parity" decision), the Bun-embedded binary is still smaller — Next's unminified dev-mode JS output (today's deployed baseline) is large enough that even Bun's unminified build plus its source maps nets smaller. Minification (Story 8.6/PR 8b) will shrink the Bun side further; this row will be re-measured then |

## Open items for the owner (not blocking this story, per its own scope)

- M4's Bun-side component-reload latency has no matching n≥5 number (gap stated above, not fabricated); gate S2's own AMBER verdict (Fast Refresh does not preserve state on 2 of 3 probed pages) is unresolved and is called out there, not here.
- F1-F5's "after" column awaits Story 9.2.
- M3's "deployed bundle minified" sub-clause awaits Story 8.6/PR 8b, which also appends the minified-default numbers to this file (plan AC).
