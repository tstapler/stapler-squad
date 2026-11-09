# Build the web UI

The web UI is built by Bun (`web-app/build/`, ADR-006; plan: `project_plans/web-app-bun-build/`). It writes `web-app/out/`, and `make web-build` copies it to `server/web/dist`, which the Go binary embeds.

```bash
make web-build                    # unminified (WEB_MINIFY defaults to 0 in the Makefile)
cd web-app && bun build/build.ts  # the pipeline alone; minified unless WEB_MINIFY=0
```

`make install-service` ships whatever `web-app/out` holds, and restarts the service, so do it in a planned window (the preflight requires `--tmux-keep-server`).

## Bun pipeline

Stages, in order: `guard` (the vanilla-extract `eval` patch is applied), `clean`, `theme-map`, `bundle`, `html`, `public`, `verify`, `swap`. A failure names the stage and, where known, the source file. Output goes to `web-app/.out-tmp-<pid>` and replaces `out/` only after `verify` passes, so a failed build leaves the previous `out/` intact.

- Assets keep the `/_next/static/` prefix (ADR-003): Go exempts it from login and serves it as immutable. The name is a leftover from the removed Next build; renaming it is the optional Story 9.4.
- Fonts are copied from `src/app/fonts/` to `/_next/static/media/<name>.<hash>.woff2`. Bun would otherwise inline them as `data:` URIs.
- `verify` fails on a file outside the auth-exempt prefixes and `server/middleware/testdata/public-allowlist.json`. Add an entry with a reason when adding a file to `web-app/public/`.
- Budgets are in `web-app/build/budgets.json` and apply to the minified build; stats go to `web-app/.build/stats.json`.
- `index.html` ships no `<title>`; the router renders it. Titles and feature-flag guards live in `web-app/src/app/route-meta.ts`, one row per `page.tsx`.

## Scripts and targets

| Entry point | Runs |
|---|---|
| `make web-build` | `bun build/build.ts` (then copies `out/` to `server/web/dist`) |
| `bun run build` (in `web-app/`) | `bun build/build.ts` |
| `bun run dev` (in `web-app/`) | `bun build/dev.ts`, the Bun dev server with HMR |
| `make restart-web`, `web-dev`, `preview` | build through `web-build` |
| `make dev-stack NAME=...` | an isolated backend plus the Bun dev server |

`bunx tsc -p tsconfig.bun.json --noEmit` type-checks the build tooling; `bunx tsc -p tsconfig.json --noEmit` type-checks the app.

## Variables

| Variable | Values | Effect |
|---|---|---|
| `WEB_MINIFY` | `0`, `1` | `make` passes `0` by default (the deployed build ships unminified, ADR-004). `bun build/build.ts` run directly minifies unless `0`. Byte budgets are enforced only for minified output. |
| `WEB_DEV_HMR` | `0`, `1` | `0` disables hot reload in the Bun dev server (every source change reloads the page). |

## Ports

The Bun dev server uses `62875` from the manual dev port block in `CLAUDE.md`. It binds to `localhost` only. Never point it at the live instance's port.

## Debug build

Build into a scratch directory, minified, with the budget check, without touching `web-app/out`:

```bash
cd web-app
bun build/build.ts --out .build/out-min --stats .build/stats-minified.json
```

Source maps are emitted next to each chunk (`sourcemap: "linked"`). The stage that failed is named in the error.

## Checks that guard the output

- `verify-output` (inside the build) and `server/middleware/built_assets_test.go` both walk every file in `web-app/out` against the auth exemption. Run the Go side locally with `REQUIRE_BUILT_OUT=1 go test ./server/middleware -run TestBuiltAssets` after a build; without `out/` it skips unless `CI` or `REQUIRE_BUILT_OUT=1` is set.
- `web-app/.size-limit.json` caps Total JS at the number in `web-app/build/budgets.json`; `bunx size-limit` checks it against `out/_next/static/**/*.js`.
- `bun run check:dependency-screen` fails on a banned version, a `github:` source, or a `trustedDependencies` entry whose package has no install script.
- `verify-output` and ESLint both refuse an import of `next`; `web-app/package.json` must not list it.
- The vanilla-extract `eval` patch: `docs/how-to/patch-vanilla-extract-eval.md`.

## Output trimming

- `build/plugins/ghostty-wasm.ts` swaps ghostty-web's inlined base64 WASM for a hashed `_next/static/media/ghostty-vt-*.wasm` file, so the lazy ghostty chunk drops from 638 KB to 74 KB. It throws if a ghostty-web upgrade stops inlining the WASM; remove it then.
- `build/stages/prune-css.ts` deletes emitted CSS that no HTML or JS names. Bun writes one per lazy page chunk, none loaded, and each is a subset of the entry CSS.
- Import `fuse.js` statically: a `require("fuse.js")` pulled its CJS build in beside the ESM one.

## Rollback

There is none to Next: PR 9 deleted it. A bad Bun build is fixed forward. The git tag `web-dual-build-last` marks the last commit that still had both builds; checking it out in a scratch worktree is an emergency measure, not a revert, because it discards every commit merged since. See `docs/how-to/rollback-web-build.md` for restoring the previously deployed build.

## History

The cutover ran Bun beside Next from PR 2a to PR 8 and made Bun the default in PR #1206 (2026-10-28); the soak windows, the owner's decision to shorten them and the gate results are in `project_plans/web-app-bun-build/research/cutover-gate-results.md`, and the measurements in `docs/explanation/bun-migration-results.md` (historical).
