# Bun.build spike (committed from a session scratchpad; throwaway, not product code)

Proof of concept that `Bun.build` can bundle the web-app with vanilla-extract. Committed so
the plan's evidence and port sources survive a scratchpad cleanup (plan Story 0.7).
Source only: no `node_modules`, lockfile, build output, or `src/` copy. The spike read the app
source from `./src` (a copy/link of `web-app/src`); recreate that to run it.

- `ve-plugin.ts`: vanilla-extract Bun plugin (Task 0.2a ports from here). `eval-shim.cjs` replaces the
  `eval` package (ADR-008); `ve-*.mjs` / `*probe.mjs` compare and probe VE output.
- `build.ts`, `build2.ts`: `Bun.build` runs. `shims/`: replacements for `next/*` imports. `entry.tsx`: entry.
- `devtest/`: dev-server experiment. `rsbuild.config.mjs`, `vite.config.mjs`, `rstest.*`: comparison builds.
- `bunfig.toml*`, `bun-preload.ts`, `jest.setup.js`: Bun test-runner experiments (Bun is not the test runner, REQ-C3).

## Naming options used (NOT exempt-prefix-safe)

`naming: { entry: "[name].[ext]", chunk: "chunks/[name]-[hash].[ext]", asset: "assets/[name]-[hash].[ext]" }`

These output paths are not guaranteed safe for the server's exempt-prefix handling; the production
build must choose names deliberately (see the plan's Epic 6).

Feasibility doc `docs/explanation/bun-migration-feasibility.md` is carried by PR #1100 and is not part of this PR.
