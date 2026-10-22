# ADR-007: Bundler is `Bun.build` + Bun dev server, with explicit gates that fall back to Vite 8

Status: Accepted by owner (Bun.build); fallback gates proposed (flagged choice F7). Date: 2026-10-19
Evidence: `docs/explanation/bun-migration-feasibility.md` §2-4, §9; `research/build-vs-buy.md` §1; `research/pitfalls.md` §3.

## Context

All of Bun, Vite 8 (5.75-6.25 s) and Rsbuild (6.2-6.6 s) meet the 10 s production-build target; Bun's edge is margin (about 2.9 s) and toolchain unification. Its weakness is maturity: only one component was ever run in the dev server, a `bun test` segfault was seen, and open Bun issues touch exactly our path (#45730 CSS rule loss on 1.4.x, #19951 plugin plus dev server, #40248 HMR import cycles, #33520 patches not applied).

## Decision

Build with `Bun.build` (never the `bun build` CLI, which lacks plugin support) using an HTML entry where spike S1 proves it, otherwise a script entry plus `html.ts`. Serve dev with `Bun.serve` HTML routes and `bunfig.toml [serve.static] plugins`. Pin Bun exactly.

**Fall back to Vite 8 + `@vanilla-extract/vite-plugin` when any trigger fires** (evaluated at the end of Epic 0 and again at Epic 8):

| Trigger | Condition |
|---|---|
| T1 | S2: the real app does not render all 42 routes, or hot reload does not work in the Bun dev server. (Live reload without state-preserving Fast Refresh is amber: owner decides.) |
| T2 | S3: production JS exceeds 6,349,052 B, or the route and `dynamic` sites do not produce lazy chunks. |
| T3 | A Bun bundler crash or miscompile appears in two or more CI runs (monitored through the soak). |
| T4 | S3: CSS ordering differences cause visible regressions that cannot be fixed in one repair wave. |
| T5 | After a vanilla-extract minor upgrade the `.css.ts` evaluation cannot be made green in one repair wave. |
| T6 | S1: the #45730 custom-property census differs from Next's on every pinnable Bun version. |

**What a fallback costs:** Epics 1, 2, 4, 6, 7 are bundler-independent and are kept. Epic 3 (patch) and Stories 5.1-5.6 are replaced by a Vite config using the official plugin; the Bun-specific parts of Epic 6 (setup-bun pin) shrink. The WEB_BUILD seam, `build/verify-output.ts`, budgets and UX specs carry over unchanged.

## Alternatives

Rsbuild + `@vanilla-extract/webpack-plugin` (also benchmarked; would additionally unify Storybook's builder). Choose it instead of Vite only if Vite 8 re-measured on the real app misses the 10 s target.

## Consequences

The owner keeps the fast path; the exit is pre-agreed and cheap. Owning a ~100-line plugin and a patch (ADR-008) is the price.
