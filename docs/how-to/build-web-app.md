# Build the web UI (Next or Bun)

`WEB_BUILD` selects the bundler. Unset means `next`, the deployed build; `bun` runs the pipeline in `web-app/build/` (plan: `project_plans/web-app-bun-build/`, ADR-006). Both write `web-app/out/`, and `make web-build` copies it to `server/web/dist`.

```bash
make web-build                    # Next, as before
WEB_BUILD=bun make web-build      # Bun; unminified unless WEB_MINIFY=1 is passed
cd web-app && bun build/build.ts  # the pipeline alone; minified unless WEB_MINIFY=0
```

Switching `WEB_BUILD` rebuilds `web-app/out` once (the stamp `web-app/.web-build-mode` changes) and recopies `server/web/dist`; repeating the same mode is a no-op.

## Bun pipeline

Stages, in order: `guard` (the vanilla-extract `eval` patch is applied), `clean`, `theme-map`, `bundle`, `html`, `public`, `verify`, `swap`. A failure names the stage and, where known, the source file. Output goes to `web-app/.out-tmp-<pid>` and replaces `out/` only after `verify` passes, so a failed build leaves the previous `out/` intact.

- Assets keep the `/_next/static/` prefix (ADR-003): Go exempts it from login and serves it as immutable.
- Fonts are copied from `src/app/fonts/` to `/_next/static/media/<name>.<hash>.woff2`. Bun would otherwise inline them as `data:` URIs.
- `verify` fails on a file outside the auth-exempt prefixes and `server/middleware/testdata/public-allowlist.json`. Add an entry with a reason when adding a file to `web-app/public/`.
- Budgets are in `web-app/build/budgets.json` and apply to the minified build; stats go to `web-app/.build/stats.json`.
- `index.html` ships no `<title>`; the router renders it.

## Scripts and targets in each mode

| Entry point | `next` | `bun` |
|---|---|---|
| `make web-build` | `next build` via `bun run build` | `bun build/build.ts` |
| `bun run build` | `build:next` | `build:bun` |
| `bun run dev` | `dev:next` (`next dev`) | `dev:bun` exits with a message; the Bun dev server arrives in PR 5b |
| `bun run start` | `start:next` | `start:bun` exits with a message; Go serves the output |
| `make restart-web`, `web-dev`, `preview` | build through `web-build` | same, mode follows `WEB_BUILD` |
| `make dev-stack` | `next dev` | unchanged until PR 5b |

`bunx tsc -p tsconfig.bun.json --noEmit` type-checks the Bun side; it also includes the `impl.next` files the Bun build does not select, and the base `tsconfig.json` includes the router implementation, so a signature drift in either shows up in one of the two.
