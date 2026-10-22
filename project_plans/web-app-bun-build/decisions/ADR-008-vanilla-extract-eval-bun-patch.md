# ADR-008: vanilla-extract stays; `eval@0.1.8` is carried as a `bun patch`

Status: Accepted by owner (patch approach); mechanics proposed. Date: 2026-10-19
Evidence: `docs/explanation/bun-migration-feasibility.md` §3.1, `research/stack.md` §3, `research/architecture.md` §6, `research/pitfalls.md` R3 and §3.2. Upstream: vanilla-extract#1664 (open, no comments); oven-sh/bun#33520 (patches silently not applied on a warm cache).

## Context

`@vanilla-extract/integration` evaluates each esbuild-bundled `.css.ts` with the `eval` npm package, which uses `require-like` and `Module._load`; under Bun `_load` returns `undefined`, so `setAdapter` is undefined. A 14-line `vm` + `createRequire` replacement made 336/336 files compile with output identical to Node. A Bun plugin cannot intercept a `require` made by the build script's own process, so a dependency-level change is required.

## Decision

1. `bun patch eval@0.1.8`, committed as `patches/eval@0.1.8.patch` with `patchedDependencies` in the manifest Bun reads (root `package.json`; location follows PR #1100's workspace layout, see plan Unresolved Questions). `@vanilla-extract/integration` (8.0.11), `@vanilla-extract/css` (exact pin at the version Story 3.1 selects, minimum 1.21.3) and `eval` are pinned exactly.
2. Authoring runbook: set `STAPLER_SQUAD_SHARED_NODE_MODULES=0` so the shared tree is not mutated; run `bun patch eval` immediately before editing; verify `stat -c %h` is 1 on the file (a hardlinked edit corrupts the global Bun cache, observed during research).
3. Shared node_modules hash includes `bun.lock` and `patches/**`.
4. Drift guard: a canary `.css.ts` evaluated through the plugin in `make ready` and CI, plus `web-app/scripts/verify-eval-patch.mjs` (marker check, also run as the first stage of `build/build.ts`) that fails with a message naming `patches/` (covers #33520 and a vanilla-extract bump that moves `eval` to another version).
5. Upstream the fix to vanilla-extract (#1664) and report `Module._load` to Bun; neither blocks this project.

## Alternatives (ordered)

(b) `overrides` pointing `eval` at a local ~10-line package under `web-app/vendor/eval-shim`: more transparent than a diff; use if `bun patch` proves brittle (same hash treatment). (c) Run only the VE compile step in a Node subprocess (2.1-2.8 s, needs Node at build time). A fork of `@vanilla-extract/integration` is rejected.

## Consequences

An owned patch on a transitive dependency; every vanilla-extract upgrade needs the canary green. Removable when upstream lands.
