# ADR-005: Bundle budgets are re-expressed as Total JS, Initial JS and Largest chunk, with one source of truth

Status: Proposed — default pending owner confirmation (flagged choice F5). Date: 2026-10-19
Evidence: `research/stack.md` §9, `research/architecture.md` §5, `research/pitfalls.md` §2.2 (size-limit row).

## Context

`web-app/.size-limit.json` and the `size-limit` key in `web-app/package.json` duplicate each other and glob `out/_next/static/chunks/framework-*.js` (650 KB) and `main-*.js` (400 KB), plus Total JS 5 MB. Bun emits no `framework-*` or `main-*` chunks, so those globs would match zero files. The spike measured 5.43 MB total JS, which already exceeds the 5 MB budget.

## Decision (default)

1. One file, `web-app/build/budgets.json`, holds three numbers: **Total JS** (all `*.js` under `out/_next/static/`, excluding `*.map`), **Initial JS** (entry plus `modulepreload` and `<script src>` closure from `out/index.html`) and **Largest chunk**. Values come from the first real Bun build (Story 5.9), not the old Next numbers; the Total JS ceiling can never exceed 6,349,052 B (Next production).
2. `build/verify-output.ts` asserts all three after every build and fails the build.
3. `size-limit` is kept only for Total JS (so the existing CI step and PR comment still work). The duplicate config is deleted; the surviving config reads its limit from `budgets.json`. A test proves it fails, not passes, when the glob matches zero files.
4. Budgets ratchet down by an explicit PR. gzip budgets are recorded in `stats.json` for information.

## Alternatives

- Keep `framework-*`/`main-*`: impossible without forcing named chunks.
- Only Total JS: lazy routes mean Total JS no longer protects first-load cost, which is what users feel.
- Move to `bundlesize`/another tool: unneeded churn.

## Consequences

The "Initial JS" number is new, with no Next baseline; it is set from the S3 spike and the first build, and reviewed at the cutover PR.

## Amendments (Phase 3 repair, 2026-10-19)

- The Initial JS ceiling is no longer set from the Bun build alone: Story 0.4d measures Next per-route first-load JS and today's 5 MB Total JS gate status; the ceiling is the measured Bun value x 1.05 and must not exceed the largest Next first-load x 1.10, otherwise the result is AMBER and the owner decides (Story 5.9).
- Moving Total JS from 5 MB to 6,349,052 B is recorded as a relaxation or a correction with numbers, and needs the owner's decision in Story 0.6 (Story 6.4).
