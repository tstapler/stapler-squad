# ADR-006: Dual-build seam `WEB_BUILD=next|bun`, one output contract (`web-app/out/`)

Status: Proposed — default pending owner confirmation (flagged choice F6). Date: 2026-10-19
Evidence: `research/architecture.md` §8 and F1-F3, F13; `requirements.md` Risk Control.

## Context

The Go binary embeds `web-app/out` (copied to `server/web/dist`, `//go:embed all:dist`). Nothing else about the web build is visible to Go. The pnpm-to-Bun package manager PR (#1100) means Next must still build on a Bun-installed tree for the dual period (unproven; spike S0).

## Decision (default)

1. `WEB_BUILD ?= next` in the Makefile through PR 7, flipped to `bun` in the cutover PR, deleted in the cleanup PR. Both modes write `web-app/out/`; the Bun mode builds in `web-app/.out-tmp-$$` and swaps by rename.
2. A mode stamp file `web-app/.web-build-mode` (rewritten only when the value changes, lives outside `out/`) is a prerequisite of `web-app/out`, so flipping the mode cannot reuse the other mode's output or `server/web/dist`.
3. Source compiles under both: `src/lib/nav/impl` and `src/lib/lazy/impl` are aliased per build (webpack `resolve.alias` for Next, plugin `onResolve` for Bun); `tsc` checks both against `impl.types.ts`. The Bun build fails on any resolution of `next` or `next/*`; it never shims them.
4. CI runs the separate jobs `web-build-smoke-next`, `web-build-smoke-bun`, `e2e-next` and `e2e-bun` (no matrix) in `.github/workflows/build.yml` for both modes until cutover, behind the single aggregate `web-build-gate` whose membership is the repository variable `WEB_BUILD_GATE_MEMBERS`; Jest runs once.
5. Rollback is the Makefile/CI default flip. The last commit containing both builds is tagged `web-dual-build-last` before the cleanup PR.

## Alternatives

- **Big-bang swap in one PR**: smaller total diff, but no revert path short of reverting hundreds of files, and the router, bundler and tooling failures are indistinguishable.
- **Run Bun as a separate parallel directory (`out-bun`)**: duplicates every consumer (Lighthouse, size-limit, e2e).
- **If spike S0 fails** (Next cannot build on a Bun-installed tree): the dual period is not viable. Fallback: Epics 8 and 9 collapse into a single cutover PR with a revert plan, and Next verification happens on a pnpm tree in a throwaway worktree.

## Consequences

Two builds to keep green for a bounded period (F2 drift risk), mitigated by the lint ban on `next/*` outside the facade and by CI running both. The dual period ends at the cleanup PR after the soak.

## Amendments (Phase 3 repair, 2026-10-19)

- Rollback is a ladder with an expiry: flipping the default or reverting PR 8 works until PR 9 merges; afterwards only a forward fix (the `web-dual-build-last` tag restores both builds only by discarding later commits).
- Every realised flip or rollback restarts the service; `make install-service` kills the tmux server and all live sessions unless the deployed unit passes `--tmux-keep-server` (plan Story 8.5 makes the check a precondition and writes the runbook, including branch-protection required-check names).
- The tool and test runners for Bun-only code are specified in plan Story 3.0 (Jest project `build` shelling out to a pinned `bun`; `bun test` stays forbidden).
