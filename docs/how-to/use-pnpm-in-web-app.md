# Package Manager — Always pnpm in web-app/, Never npm/yarn

`web-app/` is pinned to pnpm (`"packageManager": "pnpm@10.27.0"` in `web-app/package.json`). Always use `pnpm` there, never `npm` or `yarn`.

**Wrong:**
```bash
cd web-app && npm install
cd web-app && npm run build
```

**Right:**
```bash
cd web-app && pnpm install
cd web-app && pnpm build
```

## Why

`npm install` runs without complaint even though the project is pinned to pnpm — the `packageManager` field isn't enforced by anything on disk (corepack isn't enabled; `npm`/`pnpm` both resolve to plain Homebrew binaries). Running `npm install` creates a full, non-deduplicated `node_modules` copy *and* a `package-lock.json` alongside the existing `pnpm-lock.yaml`, silently defeating pnpm's hardlinked global store.

This happened across dozens of stapler-squad's own agent-spawned worktrees (2026-08-10 incident) — 45 of 68 worktrees had both lockfiles, and the resulting file-count explosion (3M+ files in duplicated `node_modules` trees) drove the host machine's Btrfs metadata chunk to 97%+ full, causing write failures unrelated to actual disk space.

`web-app/package.json` now has a `"preinstall": "npx only-allow pnpm"` script that fails fast if `npm install` or `yarn install` runs instead of `pnpm install`. If you hit that failure, use `pnpm install`.
