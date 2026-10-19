# Package Manager — Always Bun, Never npm/yarn/pnpm

Every JavaScript project in this repo (`web-app/`, `tests/e2e/`, `website/`, `tools/ci-gates/`, `tools/coverage/`, `tools/docs-gen/`, `tools/scanner/frontend/`) uses Bun as its package manager (`"packageManager": "bun@1.4.3"` in `web-app/package.json`; CI pins the same version in `.github/actions/setup-bun/action.yml`). Each project has a text `bun.lock`. Bun is only the installer and script runner: Jest, Next.js, ESLint, Playwright and the bundler still run on Node, so Node stays installed locally and in CI. Bun migrated from pnpm in [`docs/explanation/bun-migration-feasibility.md`](../explanation/bun-migration-feasibility.md).

**Wrong:**
```bash
cd web-app && npm install
cd web-app && pnpm install
cd web-app && npm run build
```

**Right:**
```bash
cd web-app && bun install            # CI uses: bun install --frozen-lockfile
cd web-app && bun run build
cd web-app && bunx jest path/to/file.test.tsx
```

## Why

`npm install` runs without complaint even when the project is pinned to another package manager. It creates a full, non-deduplicated `node_modules` copy *and* a `package-lock.json` next to the real lockfile, defeating the shared package cache and the shared `node_modules` store (`scripts/shared-node-modules.sh`).

This happened across dozens of stapler-squad's own agent-spawned worktrees (2026-08-10 incident): 45 of 68 worktrees had both lockfiles, and the file-count explosion (3M+ files in duplicated `node_modules` trees) drove the host machine's Btrfs metadata chunk to 97%+ full, causing write failures unrelated to actual disk space.

`web-app/package.json` therefore has a `preinstall` script that fails unless `npm_config_user_agent` starts with `bun/`, so `npm install`, `yarn install` and `pnpm install` fail fast. If you hit that failure, use `bun install`. The guard reads an environment variable that Bun only sets when it is unset: if your shell was launched from inside an npm or pnpm script, `bun install` inherits the outer value and the guard rejects it. Unset `npm_config_user_agent` (or start a clean shell) to proceed.

## Layout: isolated linker

`web-app/bunfig.toml` pins `linker = "isolated"`. Packages live in `node_modules/.bun/` and top-level entries are relative symlinks into it, the same strictness pnpm had (undeclared dependencies do not resolve) and a tree that survives being moved, which `scripts/shared-node-modules.sh` relies on. The hoisted linker also works but hides phantom dependencies.

## Supply chain: lifecycle scripts

Bun does not run a dependency's `preinstall`/`install`/`postinstall` scripts unless the package is listed in `trustedDependencies` (or is on Bun's built-in allowlist, which an explicit list replaces). `web-app/package.json` lists `@swc/core`, `esbuild` and `@vanilla-extract/css`, ported from pnpm's `onlyBuiltDependencies`. After adding a dependency that needs a build step, check `bun pm untrusted` and add it deliberately; never blanket-trust.

## Equivalents

| pnpm | Bun |
|---|---|
| `pnpm install --frozen-lockfile` | `bun install --frozen-lockfile` |
| `pnpm run x` | `bun run x` |
| `pnpm exec x` / `pnpm dlx x` | `bunx x` |
| `pnpm patch` | `bun patch` |
| `pnpm-lock.yaml` | `bun.lock` |
