# Stack Research: isolated-dev-stacks

Date: 2026-07-03
Researcher: Research Agent 1 (Stack)

## 1. Go / Cobra: 12-factor dynamic port & config binding

### Current state in this repo (main.go:185-197)

```go
// Determine listen address: flag > config > PORT env > default
address := cfg.ListenAddress
if address == "" {
    address = "localhost:8543"
}
if port := os.Getenv("PORT"); port != "" {
    address = "localhost:" + port
}
if listenAddrFlag != "" {
    address = listenAddrFlag
}
```

The comment says "flag > config > PORT env > default" but the code's actual runtime precedence is **flag > PORT env > config > default** (PORT is applied after config, then `--listen` flag applied last and wins). This already matches the community-recommended 12-factor precedence order (explicit CLI flag as the highest-priority override, environment variable next, persisted config as the low-priority default) — Cobra's own convention (flag binding via `cmd.Flags()`, with `viper.BindEnv`/`BindPFlag` used in larger apps to get flag > env > config > default "for free") mirrors this exactly. **This repo doesn't use viper**, it hand-rolls the precedence chain, which is fine at this scale but the doc comment vs. code order mismatch is worth fixing while touching this code.

Community-standard pattern (Cobra without Viper, which is what this repo already does):
1. Define the flag with a sensible default (`""` here, meaning "unset").
2. Read `os.Getenv("VAR")` and apply it only if the flag wasn't explicitly set (Cobra flags don't have a built-in "was this explicitly set" without `cmd.Flags().Changed("listen")` — currently this repo instead relies on non-empty-string checks, which works but means a flag can never be used to explicitly restore the default value of `""`; not a real problem here since `""` isn't a meaningful listen address).
3. Config file value is the lowest-priority override above the hardcoded default.

For PORT-only dynamic binding (this feature's need), the idiomatic 12-factor move is to **stop special-casing PORT to "test mode only"** and always honor it before the hardcoded default, which the code above already effectively does — the "test-mode only" framing in the requirements doc is slightly stale; PORT is already read unconditionally in main.go today. The main gap is that `PORT` only overrides `localhost:PORT` — there's no way to set the bind host and port independently via env (only via `--listen`). For isolated dev stacks that's fine since Non-functional Requirements pin everything to `localhost`.

### `:0` OS-assigned ports in Go

Standard idiom: **don't** ask the OS for a random port with a throwaway prior probe, then rebind a second time (this is the exact TOCTOU race the requirements doc flags in `findFreePort()`). Instead:

```go
ln, err := net.Listen("tcp", "localhost:0") // OS picks a free port
addr := ln.Addr().(*net.TCPAddr)
actualPort := addr.Port
// hand the *already-bound* listener to the HTTP server:
err = httpServer.Serve(ln)   // NOT httpServer.ListenAndServe()
```

`http.Server.ListenAndServe()` cannot be used with a pre-chosen dynamic port discovery step because it does the `net.Listen` internally and gives no way to read back the chosen port before serving. The fix is to call `net.Listen` yourself and use `Serve(ln)` — this is a **one-line change** in `server/server.go` (currently at line 684: `s.httpServer.ListenAndServe()`) that would let the backend support `PORT=0` (or an unset port) meaning "OS picks," while still letting the process report the resolved address (e.g. to stdout/log, or via a small "ready" file/port-file) so a wrapper script or Playwright can discover it.

This is the single most consequential finding for Feasibility Risk #2 in the requirements ("`getApiBaseUrl()` may read env vars at module-load time") and for the free-port race in Q4 below — see §4.

## 2. Next.js 15.x: dynamic port & API base URL at process start

### Confirmed: `next dev` already has first-class PORT support

Verified against current Next.js CLI docs (fetched 2026-07-03, docs version 16.2.10, but this behavior is stable since well before 15.x and unchanged in 15.x):

- `next dev -p 4000` or `PORT=4000 next dev` both work out of the box. **No custom scripting is needed to make the dev server's port dynamic** — this repo's `web-app/package.json` `"dev": "next dev --port 3001"` simply needs to stop hardcoding `--port 3001` and instead either read `$PORT` (Next's CLI already does this if no `-p`/`--port` flag is passed) or have the launcher script pass `--port "$PORT"` explicitly.
- **Gotcha (verified, not assumed)**: *"`PORT` cannot be set in `.env` — booting up the HTTP server happens before any other code is initialized."* `PORT` must be a real exported shell/process env var when spawning `next dev`, not a value written into `.env`/`.env.local`. This matters directly for whatever launcher script/CLI this feature builds (Scope item: "script/CLI entry point to spin up one named isolated stack") — it must `export PORT=<n>` (or set it in the `env` passed to `child_process.spawn`) rather than writing a dotenv file and hoping Next.js picks it up.
- Default hostname is `0.0.0.0` (binds all interfaces) unless `-H`/`--hostname` is passed — for this repo's "localhost only" NFR, the launcher should explicitly pass `--hostname localhost` (or `-H localhost`) rather than relying on the default, matching the Go backend's already-localhost-only default.

### Confirmed: build-time inlining vs. dev-time behavior differ, as suspected

From current Next.js docs (Environment Variables guide):
- `NEXT_PUBLIC_`-prefixed vars are inlined into the client JS bundle **at the time the code is compiled** — for `next build` (and thus the static-export production path, `output: "export"`), that means once, at build time, and frozen thereafter (the docs explicitly warn: *"After being built, your app will no longer respond to changes to these environment variables"*).
- For `next dev`, there is no separate "build" step — Next.js compiles on demand — but the **compiled bundle still bakes in whatever `process.env.NEXT_PUBLIC_*` value was present when the dev server (and its webpack/Turbopack config) started**, not the value at request-serve time. Practical implication: changing `NEXT_PUBLIC_API_URL` requires **restarting** `next dev`; a plain browser refresh or HMR update will not pick up a new value. This confirms the requirements doc's suspicion and resolves Feasibility Risk #2's ambiguity — "runtime-readable in `next dev`" is only true across dev-server restarts, not within a running dev-server's lifetime.
- This repo's actual runtime code (`web-app/src/lib/config.ts::getApiBaseUrl()`) already avoids the whole inlining problem for the browser case: `if (typeof window !== 'undefined') return window.location.origin + '/api'`. Since the frontend and backend are proxied/served from the same origin in the static-export/production path, and (per Scope) the isolated dev-stack's `next dev` process would need its own CORS-enabled cross-origin call to the dynamically-ported Go backend, `NEXT_PUBLIC_API_URL` is **only** consulted in the SSR/build fallback branch (`typeof window === 'undefined'`) — which for `output: "export"` + `next dev` client-rendered pages is rarely hit. The dynamic-port wiring therefore mostly needs to flow through the *browser* codepath, meaning: either (a) the isolated stack's `next dev` and Go backend must run on a shared origin (e.g. dev server proxies `/api` to the backend — see rewrites below), or (b) `getApiBaseUrl()` needs a new dev-only branch that reads a runtime-injected value (e.g. `window.__STAPLER_SQUAD_API_PORT__` set via a `<script>` tag Next.js can serve dynamically in dev, or a `NEXT_PUBLIC_API_URL` that *is* respected across dev-server restarts since a fresh `next dev` process is started per isolated stack anyway).
- **No hardcoded `8543` found in `next.config.ts`** — confirms one of the Open Questions. The only `8543` references in the frontend are the two fallback strings in `getApiBaseUrl()` (`web-app/src/lib/config.ts:22`) and possibly test fixtures/playwright config defaults (`tests/e2e/playwright.config.ts:41` defaults `baseURL` to `http://localhost:8544`, not 8543, and is already overridable via `TEST_SERVER_URL`). `next.config.ts` has no `rewrites()`/`redirects()`/proxy block at all currently, so there is no hidden hardcoded proxy target to find — if a same-origin dev proxy is wanted (option (a) above), it would be new code, not a fix to existing hardcoding.

### Turbopack note

Next.js 15's `next dev` defaults discussion (stable Turbopack dev in 15.x/16.x lines) doesn't change any of the above — `--port`/`PORT`/`--hostname` and the build-time-inlining behavior of `NEXT_PUBLIC_*` are CLI/env-loading layer concerns, not bundler-specific.

## 3. Playwright: multiple dynamically-ported dev servers as one fixture

### Built-in `webServer` array support (preferred building block)

Current Playwright docs confirm `testConfig.webServer` accepts **an array** of server configs, each with its own `command`, `url` (preferred over the deprecated `port` option) for readiness polling, and an optional `name` for log prefixing. This is very likely a better fit than continuing to hand-roll `global-setup.ts`/`global-teardown.ts` + a custom `TestServer` class for the *new* dual-server (Go + `next dev`) mode this feature needs, because:
- Each entry polls its own `url` independently and Playwright won't proceed to tests until **all** entries in the array report ready.
- `timeout` (default 60000ms) is configurable per the aggregate wait — worth raising for a cold `next dev` (which can be slow) stacked with a cold Go backend + demo/live session seeding (Feasibility Risk #1).
- `reuseExistingServer: !process.env.CI` is the idiomatic pattern for "don't refuse to start if the dev server the human already has running is still up" locally, while always starting fresh in CI.
- A `wait` option (regex over stdout/stderr, with **named capture groups written into env vars**) exists specifically for cases where the server's dynamically-chosen port must be *discovered from process output* rather than dictated up front — this is the exact mechanism needed if the Go backend is changed (per §1) to bind `:0` and print its resolved port to stdout/log rather than being told a pre-probed port number. Playwright can capture that printed port via `wait` regex into an env var, then that env var can be threaded into the `next dev` server's `env` (e.g. as `NEXT_PUBLIC_API_URL`) in the *second* array entry — solving both the port-discovery race (§1/§4) and the cross-process port hand-off in one mechanism, without a custom `TestServer`/`findFreePort()` class at all.
- If array-based `webServer` proves insufficient (e.g. because seeding demo/live sessions into the Go backend before Playwright's health check needs custom async logic beyond what `command`+`url`+`wait` express), the fallback is exactly what this repo already has: `global-setup.ts` starting both processes manually, still using the "bind :0, print port" pattern for both the Go binary and `next dev`, parsing stdout for both, and writing both ports into `process.env` for workers to inherit (as `global-setup.ts` already does today for the single-backend case via `process.env.TEST_SERVER_URL`).

### Startup ordering & health-check polling

- Best practice confirmed by docs: prefer `url` health checks (which follow 2xx/3xx/400/401/402/403 as "ready" signals) over the deprecated `port`-only check, since a bare TCP accept doesn't guarantee the app inside has finished initializing (e.g. Next.js dev server accepts TCP quickly but may still be compiling the first page).
- For two dependent servers (frontend must know the backend's port before or as it starts), the two array entries are NOT guaranteed to start in declaration order by Playwright itself — if strict ordering is required (start Go backend, capture its port, *then* start `next dev` with that port injected), a custom `global-setup.ts` retains the advantage of explicit `await` sequencing that a config-driven array doesn't give you. This is this repo's actual constraint (frontend needs the backend's dynamically-chosen port), so **the existing custom `global-setup.ts` + `TestServer`-class pattern is likely still the right shape for the new dual-server mode**, extended with a second `TestServer`-like class for `next dev`, rather than replaced wholesale by the array-based `webServer` option. The array option is worth using only if the two servers don't need to know about each other's dynamic values before boot.

### Cleanup on crash

- The one explicit crash-handling guidance in the current docs is scoped to trace capture (wrap setup in try/catch, save a `failed-setup-trace.zip` in catch, then re-throw) — there's no built-in "kill orphaned child process" safety net beyond what the harness author writes.
- This repo's existing `tests/e2e/helpers/test-server.ts` / `global-teardown.ts` should be checked (next research pass or during planning) for whether it already registers `process.on('exit'|'SIGINT'|'SIGTERM')` handlers to kill the spawned Go binary if Playwright itself is killed mid-run — the same pattern needs to cover a second child process (`next dev`) in the new mode, and ideally kill by **process group** (spawn with `detached: true` and kill via negative PID, or track both PIDs explicitly) rather than relying on a single `child.kill()` per process, since `next dev` under the hood may spawn its own worker/compiler subprocesses.

## 4. Go: atomic free-port reservation

### The race in `tests/e2e/helpers/test-server.ts::findFreePort()`

```ts
function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, () => {
      const port = (srv.address() as net.AddressInfo).port;
      srv.close(() => resolve(port));   // <-- window opens here
    });
  });
}
```

Between `srv.close()` and the Go binary's later `net.Listen("tcp", "localhost:<port>")`, any other process on the machine can grab that port. This is a textbook TOCTOU (time-of-check-to-time-of-use) bug, and it's inherent to the "probe-then-release-then-reuse-the-number" pattern — no Go *library* fixes this pattern because the flaw is architectural, not an implementation detail of any particular free-port finder (this includes well-known Go libraries like `phayes/freeport`, which do exactly the same probe-close-return-number dance and carry the identical race by construction).

### The actual fix: eliminate the "probe" step entirely

Two proven patterns, both used by real systems instead of probe-and-hope:

1. **Bind once, don't rebind (recommended here).** Whoever will actually own the socket calls `net.Listen("tcp", "host:0")` itself, reads back the OS-assigned port from `Addr()`, and **serves on that same listener** — no second bind ever happens, so there's no window for another process to steal the port. Applied to this repo: the Go binary should support "give me any free port" (`PORT=0` or an unset/`:0` listen address) by doing the `net.Listen` + `Serve(ln)` change described in §1, and then **announcing** the chosen port (stdout line, log line, or a small `--print-port` / port-file convenience) so the *external* harness (Playwright's `global-setup.ts`, or the new ad-hoc launcher script) discovers the real port after the fact instead of dictating it beforehand. This removes `findFreePort()` from the Go-backend half of the equation entirely — Node no longer needs to probe a port for the Go process at all.
2. **Socket-handoff / activation (only needed if a *parent* process must know the port before the *child* process starts, e.g. to template it into a config file the child reads at startup).** The parent binds `net.Listen("tcp", ":0")`, extracts the raw `*os.File` from the `*net.TCPListener` (`ln.(*net.TCPListener).File()`), passes it to the child via `exec.Cmd.ExtraFiles`, and the child adopts the already-bound socket with `net.FileListener(os.NewFile(3, ""))` (fd 3 being the first `ExtraFiles` entry) instead of binding a port number at all. This is the systemd-socket-activation / Cloudflare `tableflip` pattern and is the only way to make "atomic reserve, hand off to another process, guaranteed no race" true in the general case — it's more machinery than this feature likely needs (pattern 1 is sufficient since the Go binary binds its own port and only needs to *announce* it, not receive a pre-bound socket from a parent), but it's the right answer if a future requirement needs the *frontend launcher* to pre-reserve the backend's port before spawning the Go process.

For the Next.js side of port allocation (the frontend half of "N ≥ 2 concurrent stacks"), the same principle (pattern 1) applies: don't probe a port with a throwaway Node `net.createServer()` and then hope `next dev` gets there first — instead, let a wrapper pick `:0` via a real bind-and-hold (e.g. Node's own `net.createServer().listen(0)` kept open only long enough to read the port, immediately followed, with no other process able to interleave, by `next dev --port <n>` on the *same event-loop tick* before closing the probe — still technically racy across processes on a loaded machine) — but this is materially safer *if and only if* it's combined with pattern 1's real fix: make `next dev` itself capable of "give me `:0`" the way the Go binary would be, or accept the small residual race as acceptable given the localhost-only, single-developer-machine, "2-5 simultaneous stacks" NFR (the requirements doc's own Scalability NFR), where true multi-tenant hostile port-stealing isn't a realistic threat model. **Recommendation for planning phase: don't over-engineer socket handoff for the frontend; the residual race after removing the double-bind on the backend is acceptable for this NFR's threat model — flag this explicitly as an accepted risk in plan.md rather than building fd-passing machinery for `next dev`,** which doesn't support fd-inherited listeners the way a custom Go/Node server would.

## Summary of Answers to Open Questions (flagged in requirements.md)

- **"Does `next.config.ts` have hardcoded references to port 8543?"** No. Confirmed by direct read — no `rewrites()`, no proxy config, no `8543` string anywhere in `web-app/next.config.ts`. The only `8543` fallbacks live in `web-app/src/lib/config.ts::getApiBaseUrl()` (SSR/build-time branch only).
- **"`next dev`'s port dynamically configurable at process-start time"** — already true out of the box via `PORT` env var or `-p`/`--port` flag; the *only* work needed is to stop hardcoding `--port 3001` in `package.json` and to remember `PORT` must be a real process env var, never `.env`-file-based, when the launcher spawns `next dev`.
- **"Behavior differs between `next build && next start` vs `next dev`"** — confirmed: both inline `NEXT_PUBLIC_*` at compile time, but `next dev`'s "compile time" is dev-server-start time (persists across HMR, requires a dev-server restart to pick up a new value), not truly per-request runtime.

## Sources

- Next.js CLI reference (`next dev`/`next start` flags, `PORT` env var, `.env` limitation): https://nextjs.org/docs/app/api-reference/cli/next
- Next.js environment variables guide (`NEXT_PUBLIC_` inlining, load order, runtime vs. build-time): https://nextjs.org/docs/app/guides/environment-variables
- Playwright `webServer` / multiple-servers / health-check config: https://playwright.dev/docs/test-webserver
- Playwright global setup/teardown guidance (try/catch, trace-on-failure pattern): https://playwright.dev/docs/test-global-setup-teardown
- Playwright general test configuration reference: https://playwright.dev/docs/test-configuration
- `phayes/freeport` (illustrative example of the probe-close-reuse pattern and why it still races): https://github.com/phayes/freeport
- In-repo sources read directly: `main.go` (lines 1-220, 600-660), `config/config.go` (lines 55-170), `server/server.go` (`ListenAndServe` call sites), `web-app/package.json` (scripts), `web-app/src/lib/config.ts`, `web-app/next.config.ts`, `tests/e2e/helpers/test-server.ts` (`findFreePort`, `TestServer`), `tests/e2e/global-setup.ts`, `tests/e2e/playwright.config.ts`.
