# Architecture Research: isolated-dev-stacks

Date: 2026-07-03
Researcher: Architecture research agent

This is an infrastructure/tooling problem (config resolution + process orchestration), not a
multi-actor business domain, so no Event-Command-Policy table is included.

## 1. Integration points: how the listen address resolves today

### Backend (Go)

`main.go:185-197` — resolution order (highest wins), evaluated once at process start:

```go
address := cfg.ListenAddress          // from config.json
if address == "" { address = "localhost:8543" }
if port := os.Getenv("PORT"); port != "" { address = "localhost:" + port }  // test-mode override
if listenAddrFlag != "" { address = listenAddrFlag }                        // --listen flag
```

- `config/config.go:299` hardcodes the default `cfg.ListenAddress = "localhost:8543"` when writing a
  fresh config.
- The resolved `address` string is passed straight into `server.NewServerWithDeps(address, ...)`
  (`main.go:280`), which stores it as `s.addr` and later does `http.Server{Addr: addr}` /
  `ln.ListenAndServe()` in `server/server.go` (`NewServerWithDeps` ~line 65, `Start` ~line 648-684).
- **CORS origin allowlist is derived from the same `address`**: `main.go:283-284` does
  `localOrigin := fmt.Sprintf("http://%s", address); srv.SetOrigins([]string{localOrigin})`. This
  is a second, easy-to-miss consumer of the listen address — any dev-mode launcher must also add the
  frontend's own origin (e.g. `http://localhost:<next-dev-port>`) to this allowlist, or fetches from
  `next dev` to the backend will be rejected by `middleware.CORSWithOrigins` (`server/server.go:657,811`).
- **MCP URL is already dynamic** — `server/server.go:488-489`: `mcpURL := "http://" + srv.addr + "/mcp"`
  is computed from the *same* `srv.addr` and passed to `SessionService.SetMCPServerURL`, which is what
  gets baked into new sessions' `--mcp-server` flag (`session/instance_tmux.go`,
  `claudeMCPConfigFlag()`). **No code change is needed for MCP to follow a dynamic port** — it already
  does, because it shares the same HTTP mux (`/mcp`) as everything else, per the requirements'
  assumption. There is no separate MCP port to resolve.

### Data directory identity (already solved, reused as the model)

`config/config.go` `GetConfigDirForDir()` (lines 66-152) is the existing precedent for "one instance
identifier → one derived resource". Priority order: `STAPLER_SQUAD_TEST_DIR` → `STAPLER_SQUAD_INSTANCE`
env var → test-mode PID-based dir → preferred-workspace file → **workspace-hash** (`sha256(cwd)[:8]`,
`config/config.go:139-142`) → global shared fallback. `config/workspace_meta.go` writes a
`workspace_meta.json` per resolved config dir (`EnsureWorkspaceMeta`, called once at startup) purely
for *discovery/display* (workspace switcher UI) — it is Go-only, write-once, read-only by other Go
processes, never consumed by the frontend. This is a useful precedent shape for a new "instance
manifest" (see §2) but is not itself sufficient — it stores metadata about a chosen identity, it does
not allocate or record *ports*.

### Frontend (Next.js) — every hardcoded/assumed reference

- `web-app/package.json:14,16` — `"dev": "next dev --port 3001"`, `"start": "next start --port 3001"`.
  Frontend dev port is a **hardcoded literal**, not env-driven at all today.
- `web-app/src/lib/config.ts:14-23` (`getApiBaseUrl()`):
  ```ts
  if (typeof window !== 'undefined') {
    return window.location.origin + '/api';   // <-- ALWAYS wins in the browser
  }
  return process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8543/api';  // SSR/build-time only
  ```
  **This is the single most important finding.** In every case where the app is served *by the Go
  binary itself* (production, static-export, and the existing Playwright harness on port 8544), this
  works because `window.location.origin` IS the backend's origin — frontend and backend are the same
  origin by construction. But in the *new* "real `next dev` + real backend" mode, the browser's
  `window.location.origin` is the **Next dev server's own origin** (e.g. `http://localhost:3001`), not
  the backend's dynamic port. `NEXT_PUBLIC_API_URL` is checked but only in the `else` branch that never
  executes in a browser. Today there is no `next.config.ts` `rewrites()` to proxy `/api` from the Next
  dev origin to the backend, so **all ConnectRPC calls (including terminal streaming, which rides the
  same ConnectRPC transport — no separate `new WebSocket(...)` construction was found anywhere in
  `web-app/src`) would 404 against the Next dev server** unless this function is changed. This is the
  concrete shape of the "ConnectRPC transport / `getApiBaseUrl()` env var timing" risk flagged in
  requirements — it is real and is the top implementation risk for this feature.
- `web-app/src/lib/auth/passkey.ts:19-24` (`authBase()`): identical pattern —
  `window.location.origin + "/auth"` in-browser, `"http://localhost:8543/auth"` hardcoded fallback
  otherwise. Same fix needed as `getApiBaseUrl()`.
- `web-app/next.config.ts`: **no `rewrites()`, `redirects()`, or proxy config exists at all.** The
  config sets `output: "export"` unconditionally (line 9). Next.js does not support `rewrites()` /
  `redirects()` / `headers()` together with `output: "export"` (even under `next dev` it emits a
  warning/no-op), so adding a dev-mode proxy here is not a clean option without conditionally
  stripping `output: "export"` for a dev-only config variant. This confirms the Open Question
  ("Hardcoded 8543 references in next.config.ts proxy layer?") — there is no proxy layer today, and
  the existing `output: "export"` setting actively blocks adding one without a separate dev config.
- No hardcoded `8543` or `window.location.origin`-based `ws://`/`wss://` construction exists anywhere
  else in `web-app/src` — terminal streaming goes through the same ConnectRPC transport/base URL, so
  fixing `getApiBaseUrl()` (and `authBase()`) is sufficient; there is no second WebSocket URL to fix.

### MCP-adjacent Go code

- `server/mcp/server.go` mounts the Streamable HTTP MCP transport at `/mcp` on the *existing* mux —
  confirmed no separate listener/port.
- `server/services/mcp_injector.go` (`InjectMCPConfig`) writes `.claude/settings.local.json` entries
  keyed by *binary path*, not by URL/port — it's for the `mcpServers` stdio-launch config, unrelated to
  the HTTP `/mcp` URL described above. No port assumption to fix here.

## 2. Data flow / consistency: proposed instance-identity → config resolution architecture

Reuse `STAPLER_SQUAD_INSTANCE` (or the workspace-hash it already derives) as the **single identity**,
and add exactly one new artifact: a small JSON **instance manifest** file, following the precedent set
by `workspace_meta.json` but written by the *launcher* (not the Go binary) before either child process
starts, and read by both:

```
~/.stapler-squad/instances/<name>/dev-stack.json
{
  "instance": "<name>",
  "backendPort": 54211,
  "frontendPort": 54212,
  "apiBaseUrl": "http://localhost:54211/api",
  "dataDir": "~/.stapler-squad/instances/<name>"
}
```

Concrete resolution:

- **Backend port (a)**: launcher allocates a free port itself (Go's `net.Listen("tcp", "localhost:0")`
  pattern already used in `tests/demo/helpers.go:253`, `session/cdp/manager.go:200`,
  `session/vnc/manager.go:654` — no new free-port logic needs inventing, just reuse the idiom) and
  passes it to the Go binary the same way the e2e harness already does: `PORT=<port>` env var
  (`main.go:191`), no code change needed on the Go side.
- **Frontend port (b)**: same free-port allocation, passed as `--port <port>` to `next dev` (the
  launcher's own concern — `next dev --port` already supports this; only `package.json`'s hardcoded
  `3001` needs to become a parameter, e.g. `next dev --port ${PORT:-3001}` or simply have the launcher
  invoke `npx next dev --port <port>` directly rather than going through the `dev` npm script).
- **Frontend API base URL (c)**: launcher sets `NEXT_PUBLIC_API_URL=http://localhost:<backendPort>/api`
  in the `next dev` child's environment. This alone is **not sufficient** given finding §1 — 
  `getApiBaseUrl()` must be changed to check `NEXT_PUBLIC_API_URL` (or a new
  `NEXT_PUBLIC_DEV_MODE`/`NEXT_PUBLIC_FORCE_API_URL` flag) *before* falling back to
  `window.location.origin` when running in this mode, otherwise the browser-side branch always wins.
  Minimal fix: reorder the two branches so an explicit env var takes precedence over
  `window.location.origin`, in both `config.ts` and `passkey.ts`. Since `NEXT_PUBLIC_*` vars are inlined
  by webpack at `next dev` compile time (not truly "runtime"), the launcher must set this env var
  *before* spawning `next dev`, and a restart of the dev server is required if the backend port changes
  mid-session — acceptable for this use case since the launcher owns both lifecycles together.
- **Data directory (d)**: already solved — set `STAPLER_SQUAD_INSTANCE=<name>` for the backend process;
  no new mechanism needed. The instance manifest's `dataDir` field is purely informational/for the
  launcher's own bookkeeping (e.g. to print to the developer), Go already resolves it independently via
  `GetConfigDirForDir`.
- **CORS**: launcher-known frontend port must be added to the backend's allowed origins. Since
  `main.go:283-284` currently hardcodes `SetOrigins` to a single derived localOrigin, this needs a small
  extension — likely a new env var (e.g. `STAPLER_SQUAD_EXTRA_ORIGINS`) or CLI flag read in `main.go`
  and appended to the `SetOrigins` call, so the launcher can tell the backend "also trust
  `http://localhost:<frontendPort>`".

This gives one Go-side helper (extend `config.GetConfigDirForDir`'s sibling functions, or add a small
`config.ResolveDevStackPorts()`/env-var read) and one Node-side helper (a tiny `devStackEnv()` used by
both the launcher and `getApiBaseUrl()`/`authBase()`), both keyed off the same instance name — no need
for a shared binary-format manifest to be *parsed* by both languages; env vars passed directly into
each child process's environment are sufficient and avoid a file-parsing race. The manifest JSON file is
useful only as a discovery/debugging artifact (mirroring `workspace_meta.json`), not as the actual
plumbing mechanism.

## 3. Process orchestration architecture

Recommendation: **a single parent process (Node or Go) that spawns and supervises both children**,
matching the existing `tests/e2e/helpers/test-server.ts` `TestServer` class shape (allocate port(s) →
spawn child with env → wait-for-ready → own teardown), extended to spawn *two* children (Go binary +
`next dev`) instead of one.

Why not reuse `session/` (tmux + git-worktree machinery)? That package is purpose-built for
long-lived, interactive, user-facing *agent* sessions: PTY streaming to a browser terminal
(`instance_tmux.go`, `native_process_manager.go`, `tmux_backend.go`), tagging/search
(`tag_manager.go`), git worktree lifecycle (`git_worktree_manager.go`), scrollback persistence, restart-rate
tracking, etc. None of that applies to "run two short-lived dev processes I want stdout/stderr for and
a clean SIGTERM on exit." Routing this through `session/` would mean either faking a tmux session for a
`next dev` process the user never interacts with via terminal, or bypassing 90% of that package's
surface — a "wrong layer" fit even though it's the most powerful process-supervision code in the repo.
The right-sized tool is the same one already used for the e2e backend-only harness: a plain
`child_process.spawn` (or Go `os/exec`) supervisor with free-port allocation, env injection, and a
readiness probe (poll `/api/...` or similar until 200).

Two-terminals-by-hand and docker-compose-style declarative config were both considered and rejected:
manual two-terminal workflow is exactly the "manual port bookkeeping" this feature exists to eliminate,
and a docker-compose-like layer would be new infrastructure disproportionate to "one dev command,"
plus this repo has no existing container tooling to build on.

Concrete shape: a script (Make target wrapping a Node script, reusing patterns from
`tests/e2e/helpers/test-server.ts`) that:
1. Resolves/accepts an instance name (defaults derived the same way `STAPLER_SQUAD_INSTANCE` would be).
2. Allocates two free ports.
3. Spawns the Go binary with `STAPLER_SQUAD_INSTANCE=<name> PORT=<backendPort>` (plus new
   `STAPLER_SQUAD_EXTRA_ORIGINS` for the frontend's origin), waits for it to be ready.
4. Spawns `next dev --port <frontendPort>` with `NEXT_PUBLIC_API_URL=http://localhost:<backendPort>/api`
   in `web-app/`.
5. Writes the instance manifest JSON (discovery only, per §2) and prints both URLs.
6. On SIGINT/SIGTERM, kills both children (mirroring `TestServer.stop()`'s SIGTERM→timeout→SIGKILL
   pattern in `tests/e2e/helpers/test-server.ts`).

## 4. Playwright architecture

Recommendation: a **separate, opt-in Playwright config** (`playwright.dev-mode.config.ts`) with its own
`global-setup` that uses the new dual-process launcher from §3, driven by a separate npm script (e.g.
`test:e2e:live`), *not* folded into the existing `npm test` / default `playwright.config.ts`. Rationale
directly from the existing config: `tests/e2e/playwright.config.ts` is already `fullyParallel: false`,
`workers: 1`, against exactly one global server (`globalSetup: './global-setup.ts'` →
`startGlobalTestServer()` → single backend-only process, per `tests/e2e/global-setup.ts` and
`tests/e2e/helpers/test-server.ts`). Every additional test run pays the full backend+data-seed cold
start once per whole run — adding a `next dev` cold start (webpack compile, not just process spawn) on
top of that for *every* test file would meaningfully worsen the already-serial run, for a mode most
specs don't need (existing specs test the static-exported bundle, which is the actual production
artifact and should stay the default correctness gate).

The new dev-mode `global-setup.ts` should follow the exact pattern already proven in
`tests/e2e/global-setup.ts`: start the dual-stack, then
`process.env.TEST_SERVER_URL = <frontend dev URL>` so it propagates to worker processes (comment at
`global-setup.ts:15-16` confirms this env-mutation-inheritance behavior is relied upon already), and
reuse the storageState-fixture-rewriting trick (lines 20-34) if any dev-mode specs need pre-seeded
localStorage against the dynamic frontend origin. A parameterized *single* config was considered and
rejected: `playwright.config.ts`'s `use.baseURL` and the global-setup's server lifecycle are both
single-server-shaped by construction (one `TestServer` instance, one base URL comment says "dynamically
assigned by global-setup"); branching all of that on a mode flag inside one file/one global-setup is
more invasive than standing up a second small config that imports/reuses the same `TestServer`-style
helper extended for two processes, and it keeps the default `npm test` run's behavior and latency
completely unchanged (directly satisfying the requirement's suggestion to keep this opt-in).

## Summary of concrete files to touch (for planning phase)

- `web-app/src/lib/config.ts` — `getApiBaseUrl()`: check env override before `window.location.origin`.
- `web-app/src/lib/auth/passkey.ts` — `authBase()`: same fix.
- `web-app/package.json` — make `next dev`'s port parameterizable (launcher will pass `--port` directly
  rather than relying on the npm script default).
- `main.go` — extend `SetOrigins` call to accept extra trusted origins from a new env var/flag.
- New launcher script (Make target + Node script, modeled on `tests/e2e/helpers/test-server.ts`).
- New `tests/e2e/playwright.dev-mode.config.ts` + `tests/e2e/global-setup.dev-mode.ts` (or similarly
  named) + at least one spec under the new mode.
- No changes needed to: `server/mcp/server.go`, `server/services/mcp_injector.go`, MCP URL resolution
  (already dynamic), `config/config.go` data-dir resolution (already solved), `session/*` tmux
  machinery (correctly out of scope per §3).
