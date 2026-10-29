# Build the web UI (Next or Bun)

`WEB_BUILD` selects the bundler. Unset means `bun` (the `make` default since PR 8, unminified), which runs the pipeline in `web-app/build/`; `next` is an explicit opt-in until PR 9 deletes it (plan: `project_plans/web-app-bun-build/`, ADR-006). Both write `web-app/out/`, and `make web-build` copies it to `server/web/dist`.

```bash
make web-build                    # Bun (default); unminified unless WEB_MINIFY=1 is passed
WEB_BUILD=next make web-build     # Next, explicit opt-in
cd web-app && bun build/build.ts  # the pipeline alone; minified unless WEB_MINIFY=0
```

`make install-service` ships whichever mode is current, so the first install after the flip swaps the deployed UI to the Bun build; it restarts the service, so do it in a planned window (the preflight requires `--tmux-keep-server`).

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

## Variables

| Variable | Values | Effect |
|---|---|---|
| `WEB_BUILD` | `bun` (default for `make`), `next` | Picks the bundler for `make web-build`; `bun run build` inside `web-app/` still defaults to `next` when `WEB_BUILD` is unset (package.json, removed in PR 9). Anything else fails the Makefile. |
| `WEB_MINIFY` | `0`, `1` | `make` passes `0` by default (the cutover build ships unminified, ADR-004). `bun build/build.ts` run directly minifies unless `0`. Byte budgets are enforced only for minified output. |
| `WEB_DEV_HMR` | `0`, `1` | Hot reload of the Bun dev server. The server itself arrives in PR 5b; the flag is documented here so the name is reserved. |

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
- The vanilla-extract `eval` patch: `docs/how-to/patch-vanilla-extract-eval.md`.

## Soak

After PR 8 flips the Makefile and CI default of `WEB_BUILD` to `bun` (unminified; see ADR-004, F4), the owner runs the deployed instance on the Bun build for a bounded window before Next is deleted (PR 9). This section is the owner's watch-list, revert command, and exit criteria for that window; see `project_plans/web-app-bun-build/implementation/plan.md`, Story 8.4, and ADR-007 for the full rationale.

### What to watch

- Browser console for `Unexpected token <` — a stale tab requesting a hashed chunk that no longer exists after a rebuild/restart gets back `index.html` parsed as JS (see `project_plans/web-app-bun-build/research/architecture.md`, finding F5, the SPA-fallback miss).
- The login page's static assets return `200`, not a login redirect or `404` — a regression here means an asset fell outside the auth-exempt prefix (`/_next/static/`).
- The stale-chunk reload counter (`performance.mark('stale-chunk-reload')` / the existing telemetry counter, Task 2.7e) — a rising count under normal use (not just post-deploy) indicates chunk-serving problems, not just one-off staleness.
- HMR crashes or RSS growth in the Bun dev server (`bun run dev`, `WEB_DEV_HMR`) during a dev session.

### Revert command

Flip the Makefile/CI default back and rebuild:

```bash
WEB_BUILD=next make web-build   # confirms Next still builds during the Soak
```

then edit `WEB_BUILD ?= bun` back to `next` in the `Makefile` (and the equivalent CI job default), and restart the deployed service during a planned window (`make install-service`) — never to try a change, only to realize an already-decided revert; see the manual-instance section above for trying things without touching `:8543`. This is valid only until PR 9 merges (Migration Plan rollback ladder); the full step-by-step runbook, including the `--tmux-keep-server` precondition and the `WEB_BUILD_GATE_MEMBERS` removal, is Story 8.5's `docs/how-to/rollback-web-build.md`, written separately. Each realized revert is one of the "zero reverts" the exit criteria below count.

### T3 counting rule (Vite 8 fallback trigger)

ADR-007 defines trigger T3 as: a Bun bundler crash or miscompile appears in **two or more CI runs**. Unlike the other watch-list items above (which are owner-observed on the live instance), T3 is counted from CI, across the whole Soak window, not reset per PR. A single crash is recorded but does not fire T3; a second distinct occurrence (any CI run, any job) does, and is reported to the owner with both occurrences' run numbers — see ADR-007 for what firing a trigger does (switches Stories 5.1-5.6 to a Vite config and drops Epic 3).

### Exit criteria

- **PR 8b** (enable minification, Story 8.6) does not happen until the owner's instance has run the Bun build (`WEB_BUILD=bun`, unminified) for at least **14 calendar days and 3 distinct dev sessions** after PR 8 merged, with **zero reverts** and **no open P1 web-build issue**.
- **PR 9** (delete Next) does not happen until at least **7 further days** have passed after PR 8b merged.
- The owner may **lengthen**, never shorten, either default. Any change to these defaults, and the reason, is recorded in `project_plans/web-app-bun-build/research/cutover-gate-results.md`.

### PR 9 deadline reminders (set once PR 8 actually merges)

PR 9 has a deadline, not just a floor (pre-mortem failure mode 9): the dual-build tax (two CI cells, aliases, conditionals in tests) must not become permanent. Once PR 8 merges on date D, create two dated reminders tied to that date — either calendar entries or a scheduled agent — and record the link to each in `cutover-gate-results.md`:

- **D+21 days** — earliest PR 9 can merge (the 14-day/8b plus 7-day/8b-to-9 floors, assuming no lengthening and PR 8b landing immediately at D+14).
- **D+45 days** — stop-loss decision point. If PR 9 has not merged by D+45, the owner decides in writing, recorded in `cutover-gate-results.md`, between finishing PR 9 and reverting to Next outright. Epics 1, 2c, 2d, and 7a stay valuable under Next either way, so a revert at this point does not waste that work.

A concrete mechanism for the reminder, once there is a real PR-8 merge date: `mcp__stapler-squad__schedule_wake` with the target date/time and a message naming the D+21 or D+45 decision, or (per the `schedule` skill) a cron-scheduled routine with a one-time trigger at that date. No dates are set here — PR 8 has not merged yet.
