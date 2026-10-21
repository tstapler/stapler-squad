# Maintain the vanilla-extract `eval` patch

`web-app/patches/eval@0.1.8.patch` makes `.css.ts` files evaluate under Bun. Under Bun, `eval`'s
`require-like` calls `Module._load`, which returns `undefined`, so vanilla-extract fails with
`setAdapter of undefined`. The patch swaps it for `vm` plus `createRequire`. Why this approach:
`project_plans/web-app-bun-build/decisions/ADR-008-vanilla-extract-eval-bun-patch.md`.

Next.js builds are unaffected: webpack's vanilla-extract plugin evaluates under Node, where the
patched `eval` behaves the same.

## Guards

- `bun run verify:eval-patch` (`make verify-eval-patch`, part of `make ready`) fails if the patch
  marker is missing or `eval` is no longer 0.1.8.
- `bunx jest --selectProjects build` (`make test-web-build`) runs a probe against an unpatched copy
  (must fail) and the installed package (must pass), plus a real `.css.ts` compiled under Bun.
- `scripts/shared-node-modules` hashes `web-app/patches/` into the shared install key, so a patch
  edit never reuses a tree built from the old patch.

## Re-author the patch (after a vanilla-extract bump moves `eval`, or the patch drifts)

1. Use a private install so the shared `node_modules` is not mutated:
   `STAPLER_SQUAD_SHARED_NODE_MODULES=0 bun install` in `web-app/`.
2. `bun patch eval@<version>`, then edit the printed `node_modules/.../eval/eval.js` immediately.
   Check `stat -c %h` on the file is 1 first; a hardlinked edit corrupts Bun's global cache.
3. Keep the first line marker `ssq-patch: eval-bun-createRequire` (`scripts/verify-eval-patch.mjs`).
4. `bun patch --commit node_modules/eval`, update the `overrides`, `patchedDependencies` keys and
   the version in `scripts/verify-eval-patch.mjs` together, and rename the patch file.
5. `rm -rf node_modules && bun install --frozen-lockfile`, then run the guards above.

Bun's `patches` are skipped silently on a warm cache in some cases (oven-sh/bun#33520); that is
what the marker check catches.

## Upstream

- vanilla-extract/vanilla-extract#1664 (open): the replacement diff is not yet commented there.
- Bun: no issue filed yet for `Module._load` returning `undefined`.

Delete the patch, the `overrides` entry and these guards when vanilla-extract ships an `eval`
replacement that works under Bun, or Bun's `Module._load` returns the module.
