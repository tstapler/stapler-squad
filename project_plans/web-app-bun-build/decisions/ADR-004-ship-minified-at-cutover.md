# ADR-004: Minification ships as its own revertible PR (8b) immediately after the cutover; `NODE_ENV` is always explicit

Status: Proposed — default pending owner confirmation (flagged choice F4). Date: 2026-10-19. Amended in the Phase 4 repair (iteration 3), 2026-10-19: the original default "minified at cutover" is replaced by "unminified at cutover, minified in PR 8b" (see Amendments). The file name is kept so links from the plan and requirements stay valid.
Evidence: `research/architecture.md` §7 and F8, `research/features.md` §6, `requirements.md` success metrics, `implementation/pre-mortem.md` failure mode 4.

## Context

`make web-build` runs `NEXT_BUILD_MODE=development next build`: unminified (21.7 MB JS) but compiled with `NODE_ENV=production` (so `Providers.tsx` picks `HttpAnalyticsProvider`). `Bun.build` defaults `process.env.NODE_ENV` to `"development"`, even with `minify: true` (verified on Bun 1.4.3). The success metric is total JS no larger than 6.35 MB and a minified deployed bundle. Changing the bundler and the minifier in the same flip makes a production-only bug (name mangling, dead-code removal, `process.env` inlining) unattributable.

## Decision (default: bundler first, minifier second)

1. `build/build.ts` always sets `process.env.NODE_ENV` explicitly to `"production"` for **every** `build.ts` invocation, whatever the minify setting. Only `dev.ts` sets `"development"`. The profile switch is `WEB_MINIFY` and toggles minification only (source maps stay `linked`); there is no `WEB_BUILD_MODE` and no `--mode` flag. This keeps today's behaviour, where the Makefile's "development-mode" Next build is already `NODE_ENV=production`.
2. Code default of `build.ts` is **minify on**. The Makefile passes `WEB_MINIFY=0` explicitly until PR 8b deletes that line; `verify-output` fails any build with `minify:false` unless `WEB_MINIFY=0` was set explicitly (recorded in `stats.json` as `minifyExplicit`).
3. **PR 8 (cutover) ships Bun unminified**, parity with today's deployed 21.7 MB unminified bundle. The byte budgets (Total JS 6,349,052 B and the others) are enforced in CI on a **minified** build made in the same smoke cell (both outputs are built there), so the number is never lost and the minified output is always one flag away.
4. **PR 8b** (Story 8.6) flips the Makefile default to minified after the first Soak window. Gate: the full Playwright suite (3 runs, baseline comparator) plus the Axe sweep pass against the minified output on the isolated instance, then a second Soak window. PR 8b is a one-line revert (`WEB_MINIFY=0`).
5. Source maps stay `linked` (parity with `productionBrowserSourceMaps: true`). Whether `.map` files are embedded in the Go binary is a separate decision (secondary decision D3 in the plan).

## Alternatives

- **Minified at cutover** (the previous default, F4 original): one flip fewer and the 6.35 MB metric is met on day one, but a rare-path minifier bug is indistinguishable from a Bun bundler bug. The owner may still choose it by folding Story 8.6 into PR 8; the cost is the loss of that attribution.

## Consequences

The requirement "deployed bundle minified" is met at PR 8b, not PR 8 (requirements.md metric reworded accordingly). Minification stays isolated by an explicit switch and its own e2e gate. Look for `constructor.name`, `Function.name` and side-effect-order reliance in Story 8.6.

## Amendments

- Iteration 3 (Phase 4 repair): resolves consistency BLOCKER B1 (`NODE_ENV` contradiction between this ADR and Story 5.5) by fixing Decision 1 as written above and aligning Story 5.5; resolves pre-mortem failure mode 4 by Decisions 2-4; "secondary decision S3" corrected to D3.
