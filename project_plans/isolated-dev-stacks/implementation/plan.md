# Implementation Plan: isolated-dev-stacks

Feature: 12-factor dynamic port/config resolution + a Node-based launcher so N≥2 isolated backend+`next dev` stacks can run concurrently without manual port bookkeeping or colliding with the systemd instance, plus an opt-in Playwright dev-mode harness proving the wiring end-to-end.
Date: 2026-07-03
Status: Ready for implementation
ADRs: ADR-001-dev-stack-manifest-and-extra-origins (this project's `decisions/` dir)

---

## Effort Reconciliation (Product Blockers P1 + P4)

This plan spans **9 Epics** (1.1, 1.2, 1.3, 2.1, 2.2, 3.1, 3.2, 5.1, 5.2) broken into **32 Tasks**, each carrying its own per-task hands-on-keyboard estimate. Summed directly from the Task list above, that's **126 minutes (~2.1 hours)** of raw, uninterrupted implementation time.

**This is a floor, not a forecast.** The 126-minute figure is task-execution time only — it does not include the time actually spent debugging a failing test, iterating on a PR review comment, running `make ci` and waiting for it, or the two dedicated CI-wiring/registry tasks (3.2.1h, 5.2.1b) whose own real-world duration depends on how cleanly the rest of the plan lands. Applying a realistic 5–10x multiplier for debug/test/review/CI-wait overhead (a normal range for planning-estimate-to-actual on unfamiliar cross-process code) puts total effort at roughly **10–21 hours** — treat 126 minutes as a lower bound, and this range as the actual planning estimate.

**Verdict: this plan fits comfortably in the lower-to-middle portion of requirements.md's Medium (1–2 week, ~40–80 hour) appetite, with meaningful margin, not at its upper end.** The 10–21 hour estimate is well under even the low end of a one-week (~40 hour) Medium appetite. Nine epics and 32 tasks is a real, multi-day effort with some genuine complexity (3.2.1g's orphan-reconciliation sweep, 3.2.1h's CI wiring, Epic 5's Playwright dev-mode harness carry more design/cross-process-signal complexity than their raw minute counts alone suggest), but the arithmetic does not support calling this an upper-Medium or stretch estimate — it is a moderate, well-margined Medium effort.

Because there is margin rather than schedule pressure, no further descoping is recommended on cost grounds. The two most complex remaining items — Epic 3.2's orphan-reconciliation sweep (Task 3.2.1g, plus its CI test in 3.2.1h) and Epic 5's Playwright dev-mode harness (Epics 5.1/5.2) — should be kept as scoped: per Product Blocker P2's reasoning (see requirements.md's Risk Control section), cutting the orphan-reconciliation sweep would re-open a previously-fixed correctness gap (an un-reaped orphan persistently blocks every subsequent launch of that instance name, undermining the "zero manual bookkeeping" Success Metric rather than just failing once visibly), and neither item is needed as a cut to protect the appetite in the first place.

---

## Step 0.5 — Alternatives Explored for the Launcher's Implementation Language/Shape

Three distinct high-level approaches were brainstormed for "how does the ad-hoc stack launcher get built":

1. **Go-based launcher CLI subcommand** (`stapler-squad dev-stack up`, added to the existing Cobra tree in `main.go`).
   - Strength: single binary, no new runtime dependency, reuses `os/exec`, `executor.StartProcess`, and the existing config-precedence chain directly in the language the rest of the backend is written in.
   - Weakness: the Go process would still have to shell out to and supervise a *Node* child (`next dev`) with no shared code for stdout/stderr multiplexing, readiness polling, or process-group teardown patterns already proven in TypeScript (`tests/e2e/helpers/test-server.ts`) — this duplicates supervision logic in a second language instead of reusing it.

2. **Node/TS launcher script** under `scripts/dev-stack/`, extending the existing `tests/e2e/helpers/test-server.ts` pattern.
   - Strength: directly reuses the already-proven `findFreePort`/`spawn`/`waitForServer` shape, and — critically — the *same* module can be imported by both the interactive manual CLI and the new Playwright dev-mode harness (Phase 5), so there is exactly one implementation of "spawn backend + next dev, wait for both, tear both down" instead of two.
   - Weakness: introduces a second "launcher" implementation surface (TypeScript) for something that ultimately controls the Go backend's lifecycle, and requires a TS execution story (`tsx`/`ts-node`) for the manual, non-Playwright CLI entry point.

3. **Makefile-orchestrated shell script**.
   - Strength: zero new code in Go or TS, lowest ceremony, fits this repo's existing make-target-heavy workflow (`make install-service`, `make quick-check`, etc.).
   - Weakness: shell has no clean primitive for the bind-and-hold port-allocation idiom (needs a real per-language socket held open) or process-group-aware teardown (§ pitfalls.md §2) — it would end up shelling out to a Go or Node helper anyway, just hiding the two-language problem behind an extra layer instead of solving it, and JSON manifest read/write in bash is error-prone.

**Chosen: Option 2 (Node/TS launcher script), wrapped by a thin `make dev-stack` target.** This is also the concrete shape architecture.md §3 independently arrives at ("a script … reusing patterns from `tests/e2e/helpers/test-server.ts`"), and it is the only option that lets Phase 3 (manual launcher) and Phase 5 (Playwright harness) share one implementation instead of two. Options 1 and 3 are recorded as rejected in the Pattern Decisions table below.

This is an infrastructure/dev-tooling problem (config resolution + process orchestration), not a multi-actor business domain — no DDD aggregates/EventStorming table is included, per architecture.md's own framing.

---

## Domain Glossary

| Term | Definition |
|---|---|
| `DevStack` | A named pairing of one backend process + one `next dev` process, sharing one `STAPLER_SQUAD_INSTANCE` identity, spun up ad hoc for manual testing or driven by the Playwright dev-mode harness. |
| `StackInstanceName` | The string value passed as `STAPLER_SQUAD_INSTANCE` (or the workspace-hash default) identifying a `DevStack` — reuses the existing instance-ID mechanism per the requirements' Constraints; never a second, competing identifier. |
| `PortAllocation` | The bind-and-hold idiom: `net.Listen("tcp","localhost:0")` (Go) or `net.createServer().listen(0)` (Node), reading back the OS-assigned port from `Addr()`/`address()`, and holding the listener open until the real server takes over the same socket — replaces the probe-close-reuse pattern that has a TOCTOU gap. |
| `AllocatedPortSet` | An in-process `Set<number>` the launcher keeps for its own lifetime, recording ports already handed out, so two `PortAllocation` calls from the same launcher invocation can never return the same number. |
| `BindRetry` | The bounded retry loop (3 attempts) wrapped around a `PortAllocation` + spawn pair, invoked only when the real bind fails after allocation — mirrors the retry pattern `docs/adr/017-cdp-port-toctou-tradeoff.md` already accepts for CDP ports, absent from today's `findFreePort()`/`pickFreePort()`. |
| `DevStackManifest` | The JSON file at `~/.stapler-squad/instances/<name>/dev-stack.json` recording `instance`, `backendPort`, `frontendPort`, `apiBaseUrl`, `dataDir`, `pid` (launcher's own PID, informational only), and `backendPid`/`frontendPid` (the two children's own PIDs, which double as their process group IDs since both are spawned `detached: true`) for a running `DevStack`. Written by the launcher once both children are ready; read back exactly once — by the *next* `startDevStack()` invocation for the same instance name, during Task 3.2.1g's orphan-reconciliation sweep, which uses `backendPid`/`frontendPid` (not `pid`) to detect and reap orphaned children of a hard-killed prior launcher. Otherwise mirrors `workspace_meta.json`'s human-readable-discovery role. |
| `ExtraOrigins` | The CORS-trusted-origin list read from the new `STAPLER_SQUAD_EXTRA_ORIGINS` env var and appended to the single-origin `srv.SetOrigins(...)` call already in `main.go:283-284` — only ever read when `STAPLER_SQUAD_INSTANCE` is also explicitly set to a non-default value, so the mechanism is a structural no-op for the default/systemd instance (Task 1.2.1a). |
| `ApiBaseUrlOverride` | The explicit `NEXT_PUBLIC_API_URL` value that `getApiBaseUrl()`/`authBase()` must prefer over `window.location.origin` when present, closing the same-origin assumption that breaks once `next dev` and the backend are on different ports. |
| `BackendChild` / `FrontendChild` | The two supervised child processes (Go binary, `next dev`) a `DevStack` launcher owns for the lifetime of one `DevStack`. |
| `StackLauncher` | The Node module (`scripts/dev-stack/launch.ts`) — invoked directly via `make dev-stack` or imported by the Playwright harness — that resolves a `StackInstanceName`, performs two `PortAllocation`s, spawns `BackendChild`+`FrontendChild` with the right env, writes the `DevStackManifest`, prints the startup banner, and owns process-group-aware teardown for both children. Its `startDevStack(name?, opts?)` entry point takes a `seedData` flag (Task 3.2.1i): `false` (the manual CLI's default, for a ≤20s p50 cold start) skips all demo/session seeding; `true` (required for Epic 5.1's Playwright dev-mode path, Task 5.1.1a/b) performs it. |
| `DevModeTestServer` | The Playwright-side counterpart to `TestServer` (`tests/e2e/helpers/test-server.ts`) — in practice, `StackLauncher`'s exported `startDevStack()`/`stopDevStack()` functions consumed by the new `global-setup.dev-mode.ts`, rather than a third independent implementation. |

---

## Pattern Decisions

| Component | Pattern Selected | Alternative Rejected | Reason |
|---|---|---|---|
| Launcher implementation shape (system-level, Step 0.5) | Node/TS script under `scripts/dev-stack/`, shared by manual CLI + Playwright harness | Go CLI subcommand (`stapler-squad dev-stack up`) | Would duplicate spawn/readiness-poll/teardown logic in a second language instead of reusing the already-proven `test-server.ts` shape; architecture.md §3's concrete recommendation is specifically a Node script. |
| Launcher implementation shape (system-level, Step 0.5) | Node/TS script under `scripts/dev-stack/` | Makefile-orchestrated shell script | Shell has no clean bind-and-hold socket primitive or process-group kill semantics; would shell out to Go/Node anyway, adding a third layer instead of solving the two-language problem. |
| Port allocator (Go + Node) | Bind-and-hold (`net.Listen(":0")`, don't close until spawn) + `BindRetry` (3 attempts) | fd-passing / socket activation (`ExtraFiles` handoff, per stack.md §4 option 2) | Already rejected for this exact tradeoff in `docs/adr/017-cdp-port-toctou-tradeoff.md`; disproportionate machinery, and `next dev` has no fd-inheritance flag to receive a handed-off listener. |
| Process launcher | Facade over two supervised `BackendChild`/`FrontendChild` processes, one `StackLauncher` module | Reuse `session/` tmux + git-worktree machinery | Wrong layer — that package is purpose-built for long-lived, interactive, PTY-streamed agent sessions (terminal streaming, tagging, worktree lifecycle); two short-lived dev processes with no user-facing terminal don't fit, per architecture.md §3 and build-vs-buy.md §1b, both independently concluding this. |
| Process launcher | Plain `spawn()`/`os/exec`-based supervision | `overmind`/`foreman`/`hivemind` (Procfile runners) | `overmind` multiplexes through tmux, duplicating this repo's own in-house tmux supervision (`session/tmux/`) with a second competing tool; per build-vs-buy.md §1b. |
| Config resolution (Go) | Extend existing hand-rolled precedence chain (`flag > PORT env > config > default`) in `main.go` with one new env var (`STAPLER_SQUAD_EXTRA_ORIGINS`) | Adopt `caarlos0/env` (or `envconfig`) | Would introduce a second config-loading convention alongside the existing `os.Getenv` + `Config` struct pattern for a feature that only needs one or two new variables; per build-vs-buy.md §3. |
| Port-race fix (in-process) | Hand-rolled `AllocatedPortSet` + delayed-close, held in the launcher process | `sindresorhus/get-port` (npm) | `get-port`'s own docs concede it does not eliminate the cross-process race either — it only adds a dependency for a guarantee this repo can implement in ~10 lines; per build-vs-buy.md §4. |
| `DevStackConfig` (ports/URLs threaded through the launcher) | Value object (`{instance, backendPort, frontendPort, apiBaseUrl}` typed record, immutable once resolved) | Raw primitives (bare `number`/`string` args threaded through function signatures) | Primitive obsession — passing 3-4 same-typed `number`/`string` values positionally makes it easy to swap `backendPort` and `frontendPort` by accident across the ~6 call sites in `launch.ts`; a typed record makes the mistake a compile error. |
| CORS origin extension | Additive: append `STAPLER_SQUAD_EXTRA_ORIGINS` entries to the existing single-origin `SetOrigins` slice | Wildcard/regex origin matching in `CORSWithOrigins` | Over-broad for a feature whose NFR is "no new externally-reachable surface, localhost only" — an explicit, closed allowlist keeps the same security posture as today's single-origin behavior. |
| API base URL resolution (frontend) | Reorder branches: explicit `NEXT_PUBLIC_API_URL` override checked before `window.location.origin` | `next.config.ts` `rewrites()`/proxy layer | Blocked by `output: "export"` even under `next dev` (pitfalls.md §3b: Next.js errors, not just warns, when rewrites are combined with static export) — would require conditionally stripping `output: "export"` for a dev-only config variant, a more invasive change than reordering two `if` branches. |
| Playwright dev-mode harness | Separate opt-in `playwright.dev-mode.config.ts` + `global-setup.dev-mode.ts`, importing `StackLauncher`'s `startDevStack()` | Built-in `webServer` array config | The frontend must know the backend's dynamically-chosen port *before* it starts (strict ordering); Playwright's `webServer` array does not guarantee declaration-order startup, per stack.md §3 — the custom `global-setup.ts`-style sequencing this repo already uses is the right fit, confirmed independently by architecture.md §4. |

---

## Observability Plan
Logs: The `StackLauncher` prints one line per lifecycle event (port allocated, child spawned, child ready, teardown started/finished) to stdout, mirroring the existing `✅ Test server started on http://localhost:{port}` pattern in `tests/e2e/helpers/test-server.ts`. The Go backend's existing `log.Info("Starting web server", "address", address)` (main.go, runtime phase) needs no new fields — the `address` value itself now reflects the real bound port after the Task 1.1.1a listener change.
Metrics: None — this is developer/test tooling with no production request path (per requirements' Observability Requirements, standard request logging is explicitly sufficient).
Alerts: None — a broken `DevStack` fails visibly (process refuses to start, health check times out, `make dev-stack` exits non-zero) rather than silently; no alerting infrastructure is warranted for local dev tooling.

## Risk Control
Feature flag: None needed — every change is either additive (new env var, new CLI subcommand, new files under `scripts/dev-stack/` and `tests/e2e/`) or a browser-side branch reorder that is a no-op when the new env var is unset (Task 2.1.1a/b explicitly include a regression AC proving the unset case is unchanged).
Rollback procedure: Revert the PR. No migration, no data format change to any file the systemd-managed instance reads (the systemd instance never sets `STAPLER_SQUAD_EXTRA_ORIGINS` or reads a `dev-stack.json` manifest), so rollback carries zero risk to the production-style instance.
Staged rollout: Not applicable — internal dev tooling, no user-facing rollout. Merge once `make ci` and the new `test:e2e:dev-mode` script both pass locally.

## Unresolved Questions
- [ ] Should `web-app/package.json`'s `"dev"`/`"start"` scripts use POSIX `${PORT:-3001}` shell substitution, given this repo also ships `session/tmux/tmux_windows.go` implying some Windows-facing code exists elsewhere? — blocks Task 2.2.1a — owner: implementer (a quick check of whether `next dev` is ever expected to run under Windows for this repo's actual contributors resolves this; default to POSIX substitution if no Windows dev workflow is documented for `web-app/`).
- [ ] Does `tsx`/`ts-node` already exist as a devDependency to run `scripts/dev-stack/launch.ts` directly, or does `make dev-stack` need a compile step first? — blocks Task 3.2.1f — owner: implementer, resolved by `grep -n '"tsx"\|"ts-node"' web-app/package.json package.json` before writing the Makefile target.

## Dependency Visualization

```
Phase 1: Backend Dynamic Listen Address & Cross-Cutting Fixes
  Epic 1.1 (listener binds :0, announces real port)
  Epic 1.2 (CORS ExtraOrigins)         \
  Epic 1.3 (hook URL fixes)             |-- independent of each other, all Go-only
        |                               /
        v
Phase 2: Frontend API Base URL Resolution
  Epic 2.1 (getApiBaseUrl/authBase override-wins)   -- independent of Phase 1 to compile,
  Epic 2.2 (next dev/start port parameterized)         but both required before Phase 3 works end-to-end
        |
        v
Phase 3: DevStack Launcher (Node)
  Epic 3.1 (PortAllocation + AllocatedPortSet + BindRetry)
        |
        v
  Epic 3.2 (StackLauncher spawns/tears down BackendChild+FrontendChild;
            Task 3.2.1i: manual-CLI fast path skips seeding, seedData default=false)
        |
        \--- requires Epic 1.1 (real port), Epic 1.2 (CORS), Epic 2.1 (API override), Epic 2.2 (port param)
        |
        v
Phase 5: Playwright Dev-Mode Harness
  (Phase 4 number intentionally unused — "List Running Stacks" was cut during
  planning as scope creep beyond this feature's Medium appetite; see the
  "Out of Scope (deferred during planning)" note below.)
  Epic 5.1 (opt-in config + global-setup,
            imports StackLauncher's startDevStack(),
            MUST pass { seedData: true })
  Epic 5.2 (backlog.dev-mode.spec.ts)
  -- requires Epic 3.2 (startDevStack() export)
```

---

## Phase 1: Backend Dynamic Listen Address & Cross-Cutting Fixes

### Epic 1.1: Backend listener supports OS-assigned ports and announces the real one
Goal: let the Go backend bind `PORT=0`/an unset port and report the actual bound address, which is the prerequisite for the `StackLauncher` (Phase 3) to discover a backend's real port without a separate probe.

#### Story 1.1.1: Backend `Start()` binds via `net.Listen` + `Serve(ln)`, not `ListenAndServe()`
As a developer running an isolated `DevStack`, I want the backend to support binding to an OS-assigned port and reporting the resolved address, so that a launcher script can discover it without a separate free-port probe or a second bind.
Acceptance Criteria:
- `server/server.go`'s `Start()` no longer calls `s.httpServer.ListenAndServe()` (currently line 684) for the plain-HTTP path; it calls `net.Listen("tcp", s.addr)` first, and if the bound address's port differs from what `s.addr` requested (i.e. `:0` was passed), reassigns `s.addr` to the real bound address before calling `s.httpServer.Serve(ln)`.
- Given `PORT=0` is exported and `STAPLER_SQUAD_INSTANCE=devstack-demo` is set, When `./stapler-squad` starts, Then the log line `Starting web server address=localhost:<N>` is printed with `<N>` a real non-zero OS-assigned port (e.g. `54211`, never `0`), and `curl http://localhost:54211/health` returns HTTP 200.
- Given `PORT=54211` (an explicit, already-known port) is exported, When `./stapler-squad` starts, Then it binds exactly `localhost:54211` as before — no regression to the explicit-port path used by `tests/e2e/helpers/test-server.ts` today.
Files: server/server.go, server/services/session_service.go

##### Task 1.1.1a: Replace `ListenAndServe()` with `net.Listen` + `Serve(ln)` (~4 min)
- In `server/server.go`, locate the plain-HTTP branch at line 684 (`err = s.httpServer.ListenAndServe()`). Replace with: `ln, lerr := net.Listen("tcp", s.addr); if lerr != nil { return lerr }; s.addr = ln.Addr().String(); err = s.httpServer.Serve(ln)`. Leave the TLS branch (line 682, `ListenAndServeTLS`) untouched — remote-access HTTPS is explicitly out of scope per requirements.md. This is the ONLY write to `s.addr` after construction, and it happens before `Serve(ln)` blocks — i.e. before any request handling (and therefore before any session creation) can occur — so every later read of `s.GetAddr()` (line 728, already exists) is guaranteed to observe the real bound address, not the pre-bind value. This ordering guarantee is what Task 1.1.1c and Epic 1.3's lazy-URL fix rely on.
- Files: server/server.go

##### Task 1.1.1b: Fix the stale precedence-order comment while touching this code (~2 min)
- In `main.go`, the comment above line 185 (`// Determine listen address: flag > config > PORT env > default`) does not match the actual runtime order (`flag > PORT env > config > default`, per stack.md:24). Update the comment to `// Determine listen address: flag > PORT env > config > default` to match the real code order below it — no logic change.
- Files: main.go

##### Task 1.1.1c: Fix `mcpURL`'s early-binding bug — make it a lazy read of `GetAddr()`, not a construction-time-baked string (~5 min)
- **Why this task exists:** `wireDepsIntoServer` (called from `NewServerWithDeps`, i.e. at server *construction* time — before `Start()` binds the listener) currently does `mcpURL := "http://" + srv.addr + "/mcp"; deps.SessionService.SetMCPServerURL(mcpURL)` (server/server.go:488-489). Under `PORT=0` (the exact mode Story 1.1.1 makes a first-class, tested path), this bakes `http://localhost:0/mcp` into `SessionService` permanently, since `SessionService.SetMCPServerURL` (server/services/session_service.go:590) just stores the string in a plain `mcpServerURL string` field, read later at session-creation time (session_service.go lines 322-323, 624, 1195) — long after `Start()` has resolved the real port.
- **Fix:** Change `SessionService`'s stored value from a baked string to a lazily-invoked provider: rename the field to `mcpServerURLFn func() string` (or add a parallel setter), change `SetMCPServerURL(url string)` to `SetMCPServerURL(fn func() string)`, and update the 3 read sites (session_service.go lines ~322-323, ~624, ~1195) to call `s.mcpServerURLFn()` (nil-checked) instead of reading a stored string directly. At the `wireDepsIntoServer` call site (server/server.go ~488-489), replace the eager computation with `deps.SessionService.SetMCPServerURL(func() string { return "http://" + srv.GetAddr() + "/mcp" })` — the closure captures `srv`, not a string, so it re-reads `srv.GetAddr()` fresh every time a session is created, always observing the real bound address per Task 1.1.1a's ordering guarantee.
- Files: server/server.go, server/services/session_service.go

### Epic 1.2: Backend trusts the `DevStack`'s frontend origin via CORS
Goal: let a `next dev` process on a different port than the backend make cross-origin ConnectRPC calls without CORS rejecting them.

#### Story 1.2.1: `STAPLER_SQUAD_EXTRA_ORIGINS` extends the CORS allowlist, with strict per-entry validation, and is structurally inert for the default/systemd instance
As a developer running a `DevStack`, I want the backend to trust an extra origin supplied via env var, so that `next dev`'s cross-origin calls aren't rejected by CORS — but I want anything that isn't a well-formed localhost origin rejected rather than silently trusted, since `s.origins` backs a credentialed CORS allowlist (`Access-Control-Allow-Credentials: true`, `middleware/cors.go:59-65`) shared with the production-sensitive remote-access path (`server/server.go:663` and `:811` both consume it). I also want this mechanism to be a structural no-op for the systemd-managed instance, so a `STAPLER_SQUAD_EXTRA_ORIGINS` value stray-exported in a shell profile can never widen the production-style instance's CORS trust (pre-mortem.md Failure #3, P2 — flagged twice across review rounds and resolved here rather than deferred again).
Acceptance Criteria:
- `main.go` only reads/honors `STAPLER_SQUAD_EXTRA_ORIGINS` when `STAPLER_SQUAD_INSTANCE` is ALSO explicitly set to a non-empty, non-default value — i.e. the gate is `STAPLER_SQUAD_INSTANCE != ""` (the systemd-managed/default invocation never sets a custom instance name, per `.claude/docs/state-isolation.md`'s workspace-hash-based default). When the gate fails, `STAPLER_SQUAD_EXTRA_ORIGINS` is not read at all — not read-and-rejected, simply never consulted — so the mechanism is a structural no-op for the default invocation regardless of what the env var contains.
- When the gate passes (an explicit `STAPLER_SQUAD_INSTANCE` is set), `main.go` reads `STAPLER_SQUAD_EXTRA_ORIGINS` (comma-separated) after the existing `srv.SetOrigins([]string{localOrigin})` call (line 284); each entry is validated against a strict pattern (`^https?://(localhost|127\.0\.0\.1):\d+$` — exact scheme+host+port, no path/query/wildcard/regex, host restricted to localhost/127.0.0.1) before being appended; entries that fail validation are rejected and logged as a warning, never silently included.
- Given `STAPLER_SQUAD_INSTANCE=devstack-demo` and `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:54212,not-a-valid-origin`, When the backend starts, Then `http://localhost:54212` is added to the trusted origin list, `not-a-valid-origin` is rejected with a logged warning (e.g. `log.Warn("Rejected invalid STAPLER_SQUAD_EXTRA_ORIGINS entry", "entry", "not-a-valid-origin")`), and the server still starts successfully — this is the required regression test for Adversarial Blocker 3.
- Given `STAPLER_SQUAD_INSTANCE=devstack-demo` and `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:54212` and the backend's derived origin is `http://localhost:54211`, When the backend starts, Then `srv.SetOrigins` ends up holding `["http://localhost:54211", "http://localhost:54212"]`, a `fetch()` from a page served at `http://localhost:54212` to `http://localhost:54211/api/v1/sessions` succeeds instead of failing CORS preflight, and the final resolved origin list is logged at startup (so an operator can see the trust list was widened, e.g. `log.Info("CORS trusted origins", "origins", srv.GetOrigins())`).
- Given `STAPLER_SQUAD_EXTRA_ORIGINS` is unset (regardless of `STAPLER_SQUAD_INSTANCE`), When the backend starts, Then behavior is unchanged from today (single-origin allowlist).
- **Given `STAPLER_SQUAD_INSTANCE` is unset/empty (the systemd/default invocation) and `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:54212` is present in the environment anyway (e.g. a stray shell-profile export left over from a prior `DevStack` session), When the backend starts, Then `srv.GetOrigins()` still holds only the single default origin — `STAPLER_SQUAD_EXTRA_ORIGINS` is never read, never logged as accepted or rejected, and has zero effect on the trusted-origin list** — this is the regression test resolving pre-mortem.md's Failure #3 (P2): the gate makes cross-instance CORS bleed structurally impossible for the default instance, not just discouraged.
Files: main.go

##### Task 1.2.1a: Gate on `STAPLER_SQUAD_INSTANCE`, then read/validate/append `STAPLER_SQUAD_EXTRA_ORIGINS`; log the resolved list (~5 min)
- Before doing anything else with `STAPLER_SQUAD_EXTRA_ORIGINS`, check `os.Getenv("STAPLER_SQUAD_INSTANCE") != ""`. If that gate is false (the default/systemd invocation, which never sets a custom instance name), skip the rest of this block entirely — do not read `STAPLER_SQUAD_EXTRA_ORIGINS` at all. This is what makes the mechanism a structural no-op for the systemd instance: a stray shell-profile export of `STAPLER_SQUAD_EXTRA_ORIGINS` has no code path that can reach `srv.SetOrigins` when `STAPLER_SQUAD_INSTANCE` is empty, closing pre-mortem.md's Failure #3 (P2) rather than leaving it as an accepted-but-unmitigated risk.
- When the gate passes: after `srv.SetOrigins([]string{localOrigin})` (main.go line 284), read `os.Getenv("STAPLER_SQUAD_EXTRA_ORIGINS")`; if non-empty, split on `,` and `strings.TrimSpace` each entry; validate each trimmed entry against a strict regex (`^https?://(localhost|127\.0\.0\.1):\d+$`) — anything not matching (wildcard, bare hostname, path/query suffix, non-localhost host) is dropped and logged via `log.Warn(...)` with the offending entry, NOT appended. Append only the entries that pass validation, using the existing `srv.GetOrigins()` accessor (server/server.go:760, already the pattern `main.go:841` uses for the remote-access path) rather than re-building the base slice by hand: `srv.SetOrigins(append(srv.GetOrigins(), validExtraOrigins...))`. After the call, log the final resolved list via `log.Info("CORS trusted origins", "origins", srv.GetOrigins())` so an operator can see it was widened. This closes Adversarial Blocker 3 (validation) and the related nitpick (reuse `GetOrigins()` instead of hand-rebuilding the base slice).
- Files: main.go

##### Task 1.2.1b: Document the new env var, its instance gate, and its validation rule inline (~2 min)
- Add a comment above the new block matching the style of the existing `// PORT env var overrides for test mode` comment (main.go line 189), e.g. `// STAPLER_SQUAD_EXTRA_ORIGINS lets an isolated DevStack's next-dev frontend past CORS. Only honored when STAPLER_SQUAD_INSTANCE is also explicitly set — the default/systemd instance never sets a custom instance name, so this env var is a structural no-op there regardless of its value (ADR-001 §2, pre-mortem.md Failure #3). Each entry must be an exact http(s)://localhost:<port> or http(s)://127.0.0.1:<port> origin — anything else is rejected and logged, never silently trusted.`
- Files: main.go

### Epic 1.3: Hook callback URLs resolve against the running instance, not a hardcoded 8543
Goal: any `DevStack` backend that creates/manages Claude Code sessions must produce working permission-approval and stop-notification hooks, which today are broken for any instance not on port 8543 (features.md §1e — a pre-existing bug this feature must fix).

#### Story 1.3.1: `approval_handler.go` and `hook_injector.go` derive hook URLs from the server's real address
As a Claude Code session managed by an isolated `DevStack` backend, I want permission-approval and stop-notification hook URLs to point at my actual backend's port, so that hooks work identically whether I'm managed by the systemd instance or an ad-hoc `DevStack`.
Acceptance Criteria:
- `server/services/approval_handler.go`'s `hookApprovalURL` (currently a package-level `const` at line 653, `"http://localhost:8543/api/hooks/permission-request"`) is replaced by a value derived from the server's real listen address, threaded through to its existing usage sites (lines 671, 710, 734).
- `server/services/hook_injector.go`'s `hookEndpoint` map (lines 34-40, five literal `http://localhost:8543/...` strings) is built from the same resolved base URL, resolved lazily at hook-injection time, not baked at server-construction time.
- Given a `DevStack` backend is listening on `localhost:54211` and creates a new Claude Code session, When `.claude/settings.local.json` is written for that session, Then its `PermissionRequest` hook entry contains `http://localhost:54211/api/hooks/permission-request`, not `http://localhost:8543/...`.
- Given the systemd-managed instance (still on `localhost:8543`) creates a session, When its `.claude/settings.local.json` is written, Then the hook URL is unchanged (`http://localhost:8543/api/hooks/permission-request`) — no regression for the default/production instance.
- **Given the backend is started with `PORT=0`, When a new session is created, Then the MCP URL (`SessionService`'s `mcpServerURLFn()`, Task 1.1.1c) AND the hook-callback URLs written into that session's `.claude/settings.local.json` (Tasks 1.3.1b/c/d below) all contain the real OS-assigned port (e.g. `54211`), never `0`** — this is the regression test for the early-binding bug found in architecture-review.md; today's plan shape (baking `"http://"+srv.addr` at construction time) would fail this AC, which is exactly why Tasks 1.3.1b-d below switch to a lazily-invoked base-URL provider instead.
Files: server/services/approval_handler.go, server/services/hook_injector.go, server/server.go

##### Task 1.3.1a: Locate construction sites for both structs (~4 min)
- Search `server/services/` for the constructors that build the approval handler and the hook injector (e.g. `NewApprovalHandler`-style, `InjectHooksConfig`'s caller), and their registration point in `server/server.go`. Confirm `srv.GetAddr()` (server/server.go:728, pre-existing accessor) is the correct lazy read to thread through — do NOT reuse the pattern that used to compute `mcpURL := "http://" + srv.addr + "/mcp"` at construction time (server/server.go ~line 488): that eager-string pattern is itself the bug Task 1.1.1c fixes, and Epic 1.3 must not reintroduce it for hook URLs.
- Files: server/services/approval_handler.go, server/services/hook_injector.go, server/server.go

##### Task 1.3.1b: Make `hookApprovalURL` derived via a lazily-invoked base-URL function, not a const or a construction-time string (~5 min)
- Change `hookApprovalURL` from a package-level `const` to a value read through an injected `baseURLFn func() string` field (NOT a `baseURL string` field snapshotted at construction — that would reproduce the exact too-early-bake bug Task 1.1.1c fixes for `mcpURL`), and update its 3 known usage sites (approval_handler.go lines 671, 710, 734) to call `h.baseURLFn() + "/api/hooks/permission-request"` at the point of use instead of reading a stored field. The constructor takes `baseURLFn func() string` as a parameter and stores it as-is (no computation at construction time).
- Files: server/services/approval_handler.go

##### Task 1.3.1c: Make `hookEndpoint` derived via a lazily-invoked base-URL function, not literals or a construction-time string (~5 min)
- Change the `hookEndpoint` map (hook_injector.go lines 34-40) from package-level literals into a function `hookEndpoints(baseURLFn func() string) map[HookName]string` that builds the map fresh from `baseURLFn()` each time it's called, invoked at `InjectHooksConfig`'s call sites (i.e. per-session, at hook-injection time) rather than once at construction — mirroring Task 1.3.1b's fix, not the pre-fix eager-string pattern.
- Files: server/services/hook_injector.go

##### Task 1.3.1d: Wire `func() string { return "http://" + srv.GetAddr() }` into both constructors (~3 min)
- Do NOT reuse "the same place `mcpURL` is computed" for a baked string — that timing (construction time, before `Start()` resolves the real port) is precisely Architecture Blocker 1. Instead, at the same `wireDepsIntoServer` call site, construct one shared closure `baseURLFn := func() string { return "http://" + srv.GetAddr() }` and pass it into whichever constructor(s) Task 1.3.1a identified (the approval handler's `baseURLFn` param from Task 1.3.1b, the hook injector's `baseURLFn` param from Task 1.3.1c), so hook URLs and the MCP URL (Task 1.1.1c) share the exact same lazy-read pattern, each re-evaluated at the moment a session is actually created — never snapshotted before `Start()` runs.
- Files: server/server.go

---

## Phase 2: Frontend API Base URL Resolution

### Epic 2.1: `getApiBaseUrl()`/`authBase()` honor an explicit override before `window.location.origin`
Goal: fix the structural break identified in pitfalls.md §3b — running a real `next dev` on its own port against a separately-ported backend currently 404s every ConnectRPC call because the browser branch never reads `NEXT_PUBLIC_API_URL`.

#### Story 2.1.1: Explicit env override wins over same-origin assumption
As a developer running `next dev` against a separately-ported backend, I want the frontend's API/auth base URL resolution to prefer an explicit override even in the browser, so that ConnectRPC calls and passkey auth reach the real backend port instead of the `next dev` origin.
Acceptance Criteria:
- `web-app/src/lib/config.ts::getApiBaseUrl()` (lines 14-23) checks `process.env.NEXT_PUBLIC_API_URL` before the `typeof window !== 'undefined'` branch, returning it immediately if set; falls back to `window.location.origin + '/api'` only when unset.
- `web-app/src/lib/auth/passkey.ts::authBase()` (lines 19-24) gets the analogous fix, deriving its base from `NEXT_PUBLIC_API_URL` (strip a trailing `/api`, append `/auth`) rather than requiring a second env var to be threaded through the launcher.
- Given `NEXT_PUBLIC_API_URL=http://localhost:54211/api` was exported before `next dev --port 54212` started, When a browser loads `http://localhost:54212` and a component calls `getApiBaseUrl()`, Then it returns `http://localhost:54211/api`, not `http://localhost:54212/api`.
- Given `NEXT_PUBLIC_API_URL` is NOT set (today's production/static-export/e2e-on-8544 path), When `getApiBaseUrl()` is called in the browser, Then it still returns `window.location.origin + '/api'` exactly as before — no regression to the existing same-origin path that `tests/e2e/playwright.config.ts` (baseURL default `http://localhost:8544`) depends on.
Files: web-app/src/lib/config.ts, web-app/src/lib/auth/passkey.ts

##### Task 2.1.1a: Reorder `getApiBaseUrl()` branches (~3 min)
- Edit `web-app/src/lib/config.ts` lines 14-23: check `process.env.NEXT_PUBLIC_API_URL` first and return it immediately if truthy (works in both browser and SSR/build contexts since `NEXT_PUBLIC_*` is always inlined); otherwise keep the existing `typeof window !== 'undefined'` browser-origin branch, then the SSR fallback, unchanged.
- Files: web-app/src/lib/config.ts

##### Task 2.1.1b: Reorder `authBase()` branches (~3 min)
- Edit `web-app/src/lib/auth/passkey.ts` lines 19-24 with the same override-first pattern: if `process.env.NEXT_PUBLIC_API_URL` is set, derive `<origin-without-/api>/auth` from it and return; else keep the existing `window.location.origin + "/auth"` / hardcoded-fallback behavior.
- Files: web-app/src/lib/auth/passkey.ts

##### Task 2.1.1c: Add/extend regression tests for both functions (~4 min)
- Find the existing test(s) covering `getApiBaseUrl()`/`authBase()` (pitfalls.md §3b notes ~20 `__tests__` files already `jest.mock` around 8543 fallbacks) and add two cases each: (1) `NEXT_PUBLIC_API_URL` set → override wins even with `window` defined, (2) unset → today's browser-origin behavior is unchanged.
- Files: web-app/src/lib/__tests__/config.test.ts (or existing equivalent path), web-app/src/lib/auth/__tests__/passkey.test.ts (or existing equivalent path)

### Epic 2.2: `next dev`/`next start` port is parameterized, not hardcoded
Goal: let multiple isolated stacks run their frontends on different ports concurrently, closing the "hardcoded `3001`" gap confirmed in features.md §1f and architecture.md §1.

#### Story 2.2.1: `web-app/package.json` scripts accept `$PORT` instead of hardcoding `3001`
As a developer or the `StackLauncher`, I want `next dev`/`next start` to accept a port at invocation time, so that multiple isolated stacks' frontends don't collide on `3001`.
Acceptance Criteria:
- `web-app/package.json`'s `"dev"` script (line 14) no longer hardcodes `--port 3001` unconditionally; it reads `$PORT` with a `3001` fallback.
- Given a developer runs `PORT=54212 npm run dev` from `web-app/`, When the process starts, Then it binds `localhost:54212`, not `3001`.
- Given a developer runs `npm run dev` with no `PORT` set, When the process starts, Then it binds `localhost:3001` exactly as before — no regression to the default manual workflow.
Files: web-app/package.json

##### Task 2.2.1a: Parameterize `dev`/`start` scripts (~2 min)
- Change `"dev": "next dev --port 3001"` to `"dev": "next dev --port ${PORT:-3001} --hostname localhost"` and `"start": "next start --port 3001"` (line 16) to the equivalent `${PORT:-3001}` form. Confirm this repo's dev docs already assume a POSIX shell (macOS/Linux, per `.claude/docs/codesigning.md`'s macOS focus and the Linux-first CLAUDE.md commands) before relying on `${VAR:-default}` substitution — see Unresolved Questions.
- Files: web-app/package.json

---

## Phase 3: DevStack Launcher (Node)

### Epic 3.1: Bind-and-hold port allocator with in-process reservation and retry
Goal: close the in-process TOCTOU gap flagged in build-vs-buy.md §4 (two ports allocated within the same launcher run must never collide) and add the retry-on-bind-failure loop the ADR-017 pattern calls for but `findFreePort()`/`pickFreePort()` don't yet have.

#### Story 3.1.1: `allocatePort()` never double-allocates within one launcher run and retries on bind failure
As the `StackLauncher`, I want a single port-allocation helper that binds-and-holds instead of probe-and-close, tracks ports already handed out this run, and retries on bind failure, so two ports allocated in the same invocation never collide and a rare cross-process race fails after 3 retries instead of a 90s generic timeout.
Acceptance Criteria:
- `scripts/dev-stack/ports.ts` exports `allocatePort(): Promise<{ port: number; release: () => void }>` that opens `net.createServer().listen(0)` and does NOT close the listener until the caller invokes `release()` immediately before spawning the real child on that port.
- The module keeps a module-level `Set<number>` (`AllocatedPortSet`) populated on every successful allocation; a call that would otherwise return an already-reserved number retries (fresh `net.createServer()`) up to 3 times before throwing.
- Given `allocatePort()` is called twice back-to-back in the same process (e.g. once for backend, once for frontend), When both resolve, Then `port1 !== port2` holds in a unit test.
- Given `release()` has been called for a given port, When a real server later binds that exact port number, Then the bind succeeds (proving `release()` actually freed the OS-level socket, not just the in-process record).
Files: scripts/dev-stack/ports.ts, scripts/dev-stack/ports.test.ts

##### Task 3.1.1a: Create `scripts/dev-stack/ports.ts` with delayed-release `allocatePort()` (~4 min)
- New file exporting `allocatePort(): Promise<{ port: number; release: () => void }>` — binds `net.createServer().listen(0)`, resolves with the port and a `release` closure that calls `srv.close()`; the caller decides when to call `release()` (not eagerly, unlike today's `findFreePort()`).
- Files: scripts/dev-stack/ports.ts

##### Task 3.1.1b: Add `AllocatedPortSet` + `BindRetry` (~3 min)
- Add a module-level `Set<number>` populated on every successful allocation; before returning a newly bound port, check it against the set — if already present (should be rare/impossible given bind-and-hold, but guards a future refactor) or if the real bind throws `EADDRINUSE`, retry with a fresh `net.createServer()` up to 3 times, then throw a clear error.
- Files: scripts/dev-stack/ports.ts

##### Task 3.1.1c: Add unit tests proving no double-allocation and real release (~4 min)
- `scripts/dev-stack/ports.test.ts`: (1) two concurrent `allocatePort()` calls never return the same port, (2) after `release()`, a fresh `net.createServer().listen(returnedPort)` succeeds.
- Files: scripts/dev-stack/ports.test.ts

### Epic 3.2: `StackLauncher` spawns and supervises both children with process-group-aware teardown
Goal: close the orphan-process gap in pitfalls.md §2 — `next dev` (and any grandchildren it spawns) must be reliably killed on teardown, and the launcher must be visually distinct from `make install-service` per pitfalls.md §5.

#### Story 3.2.1: One command starts a named `DevStack` and cleanly tears both children down
As a developer, I want a single command to start one named isolated backend+frontend stack and cleanly tear down both processes (and any grandchildren) on Ctrl-C, so that I never leave an orphaned `next dev` holding a port.
Acceptance Criteria:
- `scripts/dev-stack/launch.ts` exports a pure `startDevStack(name?: string)` core with ZERO ambient `process.on(...)` registrations, plus a thin CLI wrapper (`main()`, only invoked when the file is run directly) that is the sole place signal handlers are registered (see Task 3.2.1a/d — this closes Architecture Blocker 2: the same module is imported as a library by Task 5.1.1a's Playwright harness, which must not inherit CLI-only signal handling).
- The CLI wrapper takes an instance name as its first CLI arg (required — no silent auto-naming for the manual path), allocates a `BackendChild` port and a `FrontendChild` port via `allocatePort()`, spawns the Go binary with `STAPLER_SQUAD_INSTANCE=<name> PORT=<backendPort> STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:<frontendPort>`, waits for `/health` to return 200, then spawns `next dev --port <frontendPort> --hostname localhost` in `web-app/` with `NEXT_PUBLIC_API_URL=http://localhost:<backendPort>/api`.
- Both children are spawned with `detached: true`; teardown sends `process.kill(-child.pid!, 'SIGTERM')` to each child's process group, waits up to 5s for `'exit'`, then `process.kill(-child.pid!, 'SIGKILL')` — mirroring `executor/managed_process.go`'s Setpgid+group-SIGTERM-then-SIGKILL pattern, ported to Node since `next dev` is a Node child.
- When `scripts/dev-stack/launch.ts` is run directly as a CLI (not imported as a library), the CLI wrapper's `process.on('SIGINT', ...)` and `process.on('SIGTERM', ...)` both trigger the same `teardown()` path (closing the gap pitfalls.md §2 flags: neither `global-setup.ts` nor `test-server.ts` registers one today) — but these registrations MUST NOT execute merely by importing the module (see Task 5.1.1a).
- The `FrontendChild` readiness poll (Task 3.2.1c) is bounded (same ~90s ceiling as the backend poll, matching `TestServer.waitForServer()`'s `maxAttempts = 90` in `tests/e2e/helpers/test-server.ts:118`) and throws on timeout. Given the backend has started successfully and `next dev` fails to become ready within that timeout (e.g. due to a compile error), When the timeout fires, Then the launcher kills the already-started `BackendChild` (and its process group, via the same `teardown()` path) and exits non-zero — no orphaned backend process remains, closing Adversarial Blocker 2.
- **Stdout/stderr is captured and surfaced on health-check/readiness-poll timeout (UX Blocker; see Tasks 3.2.1b/3.2.1c):** both `BackendChild` and `FrontendChild` have their combined stdout+stderr buffered in memory as a rolling tail of the last 50 lines (or last 4KB, whichever is hit first) for the lifetime of the poll; this tail is discarded on success and included in the thrown error only on timeout. Given `next dev` writes a webpack compile error to stderr and then hangs without ever binding its port, When the frontend readiness poll (Task 3.2.1c) times out, Then the launcher's error output includes the last N lines of `next dev`'s captured stderr (containing the actual compile error text), not just a generic "timed out waiting for frontend" message — closing the UX Blocker that today's plan shape would otherwise leave a developer with zero diagnostic text for the plan's own named failure mode above.
- Given a stale `dev-stack.json` for instance `demo-1` recording `backendPid: 40213` and `frontendPid: 40217`, where PID `40213` is still alive (a real orphaned process, e.g. a leaked `next dev`/backend process from a prior hard-killed launcher — that launcher's own `pid` field may itself be dead, which is irrelevant to reaping) and `40217` is dead, When `make dev-stack NAME=demo-1` is run again, Then the launcher's startup reconciliation sweep (Task 3.2.1g) checks each of `backendPid`/`frontendPid` independently via `process.kill(pid, 0)`; for `40217` (dead, `ESRCH`) it does nothing further; for `40213` (alive) it sends `process.kill(-40213, 'SIGTERM')` to process group `-40213`, waits up to 5s for the process to exit, escalates to `process.kill(-40213, 'SIGKILL')` if it hasn't, logs that it reaped an orphan (e.g. `log.warn` with the instance name and PID), and confirms `40213` is no longer running via a follow-up `process.kill(40213, 0)` throwing `ESRCH` — only *after* that confirmation does it delete the stale manifest and proceed to allocate fresh ports and start new children, with no manual intervention and no `EADDRINUSE`-style failure — closing Adversarial Blocker 1.
- Given a developer runs `node scripts/dev-stack/launch.ts my-feature-test`, When both children report ready, Then stdout prints a banner containing the literal phrase `NOT the systemd instance`, the instance name `my-feature-test`, and both resolved URLs (e.g. `backend http://localhost:54211 frontend http://localhost:54212`).
- Given the banner has printed, When Ctrl-C is pressed, Then within 5 seconds `ps aux | grep -E 'stapler-squad|next-server'` shows zero matching processes for this stack's PIDs.
- **Cold-start time budget — manual CLI path only, no demo/session seeding (pre-mortem.md Failure #1):** Given the backend binary is already built (`go build .` has already run) and `web-app/node_modules` is already installed (i.e. timing excludes a fresh `go build`/`npm install`), When a developer runs `make dev-stack NAME=demo-1` for a stack that takes Task 3.2.1i's seeding-skip fast path, Then the combined wall-clock time from launcher start to BOTH the backend `/health` check (Task 3.2.1b) and the `next dev` readiness check (Task 3.2.1c) passing is **targeted at p50 ≤ 20s** on typical CI-equivalent hardware. This target is explicitly GROUNDED in, and bounded by, requirements.md's own Non-functional Requirements anchor (today's Playwright global-setup observes ~30-45s for the backend alone, *including* demo/session seeding) — the fast (unseeded) manual-CLI path is expected to land faster than that existing baseline specifically because Task 3.2.1i's fast path skips the demo/session seeding that accounts for most of the ~30-45s figure, not because 20s is a settled, precisely-derived number pulled from nowhere. Treat ≤20s as the target to measure and revisit once Epic 3.2 is actually implemented, not as a hard pre-verified gate — if real measurement lands materially above 20s, that is a finding to record and address during implementation, not a broken promise. Separately, and unrelated to whether the 20s target is hit: the ~90s figures elsewhere in this Epic (Tasks 3.2.1b/c) are `TestServer.waitForServer()`-style timeout *ceilings*, inherited from `maxAttempts = 90`, not startup-time goals — they bound how long the launcher waits before giving up, not how long a successful start should typically take. This AC is explicitly dependent on Task 3.2.1i's fast path: without it, this target is not expected to be met, and the fully-seeded Epic 5.1 Playwright dev-mode path (which intentionally opts back into seeding, per Task 5.1.1a's cross-reference) is exempt from this 20s budget.
Files: scripts/dev-stack/launch.ts

##### Task 3.2.1a: Launcher skeleton + arg parsing + two port allocations (~5 min)
- Create `scripts/dev-stack/launch.ts`, structured from the start as two parts: (1) a pure `startDevStack(name?: string)` core — parses/validates the instance name, calls `allocatePort()` twice for `backendPort`/`frontendPort` (after Task 3.2.1g's startup reconciliation sweep has run for that name), and assembles a `DevStackConfig` value object (`{ instance, backendPort, frontendPort, apiBaseUrl }`) rather than passing four loose primitives around — with NO `process.on(...)` calls anywhere in this core; and (2) a `main()` CLI wrapper that reads `process.argv[2]` as the required instance name (exit with a usage error if missing), calls `startDevStack(name)`, and is the only place that will register signal handlers (Task 3.2.1d). `main()` must only run when the file is executed directly (guard e.g. `require.main === module` / the ESM `import.meta.url` equivalent), never as a side effect of `import`.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1b: Spawn and health-poll the `BackendChild` — capture stdout/stderr tail, surface it on timeout (~7 min)
- Spawn the Go binary: `spawn(buildPath, [], { detached: true, env: { ...process.env, STAPLER_SQUAD_INSTANCE: instance, PORT: String(backendPort), STAPLER_SQUAD_EXTRA_ORIGINS: \`http://localhost:${frontendPort}\` }, stdio: ['ignore','pipe','pipe'] })`, call `release()` on the backend port allocation immediately before spawning, and poll `http://localhost:<backendPort>/health` until 200 (mirror `TestServer.waitForServer()`'s polling shape in `tests/e2e/helpers/test-server.ts`, including its bounded `maxAttempts`/throw-on-timeout behavior).
- **Stdout/stderr capture (UX Blocker):** As soon as the child is spawned, attach `data` listeners to `child.stdout`/`child.stderr` that append into a small in-memory ring buffer capped at the **last 50 lines (or last 4KB, whichever limit is hit first)** combined across both streams, in the order received. This buffering runs unconditionally while the child is alive (cheap — bounded memory, no disk I/O), but the buffer's *contents* are only ever read by the code path below. On health-check **timeout specifically** (never on success, to avoid needless log noise) — before killing the child — join the buffered tail into the thrown error's message, clearly labeled as captured backend output, e.g. `throw new Error(\`Backend health check timed out after ${ceiling}s. Last output from backend process:\\n${tail}\`)`. On success, discard the buffer without ever surfacing it. This closes the UX Blocker: a developer whose backend hangs or crashes during startup now sees the backend's own diagnostic text (stack trace, bind error, etc.) in the launcher's failure output, not just a generic timeout message.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1c: Spawn and readiness-poll the `FrontendChild` — bounded, throws on timeout, capture stdout/stderr tail and surface it on timeout (~7 min)
- Spawn `next dev --port <frontendPort> --hostname localhost` with `cwd: 'web-app'`, `detached: true`, `env: { ...process.env, NEXT_PUBLIC_API_URL: apiBaseUrl }`; call `release()` on the frontend port allocation immediately before spawning; poll `http://localhost:<frontendPort>` until it responds. This poll MUST be explicitly bounded and throw on timeout — reuse the same ~90s ceiling as Task 3.2.1b's backend poll (`TestServer.waitForServer()`'s `maxAttempts = 90` precedent, `tests/e2e/helpers/test-server.ts:118`), since a cold `next dev` compile can be slow but must not hang indefinitely. On timeout, throw rather than resolve, so the caller's `finally`/`teardown()` path (Task 3.2.1d) actually engages and kills the already-started `BackendChild` instead of leaking it — closes Adversarial Blocker 2.
- **Stdout/stderr capture (UX Blocker):** Identical mechanism to Task 3.2.1b, applied to the `FrontendChild`: attach `data` listeners to `child.stdout`/`child.stderr` at spawn time, buffering the last 50 lines (or last 4KB, whichever limit is hit first) combined across both streams. On readiness-poll **timeout specifically** (not on success) — before the thrown error propagates into the `teardown()` path that kills the already-started `BackendChild` — include the captured tail in the thrown error's message, e.g. `throw new Error(\`Frontend readiness check timed out after ${ceiling}s. Last output from next dev:\\n${tail}\`)`. This is the concrete fix for the plan's own named failure mode ("`next dev` fails to become ready due to a compile error," Story 3.2.1's AC) — previously that scenario produced only a generic timeout message with zero diagnostic text; now the actual webpack/compile error `next dev` printed to stderr right before hanging is visible in the launcher's failure output. See Story 3.2.1's amended Given-When-Then AC below for the exact acceptance test.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1d: Process-group-aware teardown, scoped to the CLI wrapper only (~4 min)
- Implement `teardown()` inside the pure `startDevStack()` core (Task 3.2.1a): for each of the two `ChildProcess` handles, `process.kill(-child.pid!, 'SIGTERM')`, race against a 5s timeout waiting for `'exit'`, then `process.kill(-child.pid!, 'SIGKILL')`; also delete `~/.stapler-squad/instances/<name>/dev-stack.json` (Task 3.2.1e) as the last step, so a clean shutdown leaves no manifest for Task 3.2.1g's next-run reconciliation sweep to mistake for an orphan. `teardown()` itself takes no signal-related action and has no ambient side effects — it is just a function the caller can invoke. Registering `teardown` on `process.on('SIGINT')`, `process.on('SIGTERM')`, and in a top-level `finally` happens ONLY inside `main()`, the CLI wrapper from Task 3.2.1a — never at module scope — so that importing `startDevStack` (Task 5.1.1a) never attaches a signal handler to the importing process. This closes Architecture Blocker 2.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1e: Startup banner + `DevStackManifest` write, including `backendPid`/`frontendPid` (~4 min)
- Print the startup banner (instance name, both URLs, the literal phrase "NOT the systemd instance") once both children are ready (i.e. after Task 3.2.1b's backend health-poll AND Task 3.2.1c's frontend readiness-poll have both resolved), and write `~/.stapler-squad/instances/<name>/dev-stack.json` per the format in ADR-001. The manifest MUST include `backendPid: backendChild.pid` and `frontendPid: frontendChild.pid` — each child's own PID, which (since both are spawned with `detached: true`, per Task 3.2.1b/c) doubles as that child's own process group ID. These two fields, not the launcher's own PID, are what Task 3.2.1g's reconciliation sweep on a future run reads back to liveness-check and, if necessary, reap this stack's children — `process.kill(-backendPid, ...)`/`process.kill(-frontendPid, ...)` are the only calls that reach each child's actual process group. Continue to also write `pid: process.pid` (the launcher's own PID) for its narrower informational purpose (ADR-001 §1), and `schemaVersion: 2`, but the manifest is not considered complete/correct for orphan-sweep purposes without `backendPid`/`frontendPid` populated. Because this write only happens once both children are ready, the manifest never exists in a partially-populated state where one child's PID is known and the other isn't. Task 3.2.1d's `teardown()` deletes this file on clean shutdown.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1f: `make dev-stack` target (~2 min)
- Add a `dev-stack` target to `Makefile` wrapping `node scripts/dev-stack/launch.ts $(NAME)` (or `npx tsx scripts/dev-stack/launch.ts $(NAME)` — check for an existing `tsx`/`ts-node` devDependency in `package.json`/`web-app/package.json` first; see Unresolved Questions). Deliberately avoid any substring overlap with `install-service` in the target name, per pitfalls.md §5.
- Files: Makefile

##### Task 3.2.1g: Startup orphan-reconciliation sweep for a hard-killed prior launcher (~7 min)
- **Why this task exists:** `teardown()` (Task 3.2.1d) only runs on `SIGINT`/`SIGTERM`/a top-level `finally` — none of which fire if the launcher process itself is `SIGKILL`ed. requirements.md's Rabbit Holes explicitly demand "explicit cleanup handling, not just a happy-path `stop()` method" for this exact scenario, and pitfalls.md's research already names the precedent to reuse: `session/orphan_sweep.go`'s `ReconcileOrphanedTmuxSessions` (a startup reconciliation sweep).
- **Implementation:** At the very start of `startDevStack(name)` (Task 3.2.1a), before either `allocatePort()` call, check whether `~/.stapler-squad/instances/<name>/dev-stack.json` already exists. If it does: read its `backendPid` and `frontendPid` fields (Task 3.2.1e — NOT the launcher's own `pid` field, which is neither of the children's process group ID and therefore useless for reaping, per Adversarial Blocker 1). Probe each independently for liveness (`process.kill(pid, 0)`, catching `ESRCH` as "dead"). For each PID found alive, reap it BEFORE proceeding: `process.kill(-pid, 'SIGTERM')` (its own process group, since `detached: true` made it its own group leader), wait up to 5s for the process to exit (poll `process.kill(pid, 0)` until it throws `ESRCH`, or use an `'exit'`-equivalent if a handle is available), then `process.kill(-pid, 'SIGKILL')` if it hasn't exited, and log the reap (e.g. `log.warn` including the instance name, which of backend/frontend, and the PID). After both `backendPid` and `frontendPid` have been checked (and reaped if alive) — including the case where the launcher's own `pid` is itself dead, since that has no bearing on whether the children are still running — delete the stale manifest and proceed normally to allocate fresh ports. This sweep must reap on BOTH the "prior launcher dead" and "prior launcher somehow still alive" cases; the branch is keyed off each *child's* liveness, not the launcher's, since the launcher being dead is exactly the scenario in which its children are most likely still orphaned. Mirrors `orphan_sweep.go`'s shape, ported to the Node launcher.
- **Why this complexity is proportionate despite the NFR's general "fails visibly is acceptable" tolerance (requirements.md Non-functional Requirements / Risk Control):** that NFR carve-out is about a *single-shot* failure being acceptable to not gracefully recover from — e.g. a launcher crashing mid-startup and leaving a clear non-zero exit code is fine to leave as "fails visibly, developer notices, developer retries." An un-reaped orphan is a materially different failure shape: it doesn't fail visibly once and stop — it silently and *persistently* squats on that exact port/instance-name combination on every subsequent launch attempt for that instance, indefinitely, until a human manually finds and kills it. That directly undermines requirements.md's own stated Success Metric ("N≥2 concurrent stacks with zero manual bookkeeping") — without this sweep, a developer would have to manually `ps`/`kill` the orphan before every re-launch of that instance name, which is exactly the manual bookkeeping this feature exists to eliminate. This is a repeated-use correctness requirement (does re-launching the same named stack keep working, indefinitely, across hard kills), not optional error-handling polish, so it does not fall under the "dev tooling can fail visibly" carve-out — that carve-out covers single-shot failures being acceptable to not gracefully recover from, not a lingering side effect that breaks a persistent, user-facing success metric on next use. This logic was added specifically to resolve a real BLOCKER from an earlier adversarial review (a hard-killed launcher otherwise leaves orphaned `next dev`/backend processes squatting on ports indefinitely) and is retained for that reason.
- Files: scripts/dev-stack/launch.ts

##### Task 3.2.1h: Unit-test teardown + the orphan-reconciliation sweep in CI, without spawning real `next dev`/the real backend (~5 min)
- **Why this task exists:** Epic 3.1 (`ports.ts`) gets `ports.test.ts` (Task 3.1.1c); Epic 3.2 — two-process supervision, process-group-aware SIGTERM→SIGKILL teardown, and the signal-handling split from Task 3.2.1d — had only a manual `ps aux | grep` AC and no CI-enforced regression test, flagged as a blocker in both independent reviews.
- **Implementation:** New `scripts/dev-stack/launch.test.ts`. Exercise `teardown()` and Task 3.2.1g's reconciliation sweep against either (a) injectable/mocked `spawn`/`kill` functions, asserting `process.kill` is invoked with the expected `(-pid, 'SIGTERM')` then, after the grace window elapses without exit, `(-pid, 'SIGKILL')`; or (b) two real dummy long-lived child processes (e.g. `spawn('sleep', ['300'])`, standing in for the backend/frontend), asserting via a PID-liveness probe that both the processes and their process groups are gone after `teardown()` runs. Either approach must be runnable in CI with no real `stapler-squad` binary or `next dev` involved. **Reconciliation-sweep test must specifically assert against `backendPid`/`frontendPid` from a fixture manifest, not the launcher's own `pid`:** write a fixture `dev-stack.json` containing distinct `backendPid`/`frontendPid` values (one alive, one dead, mirroring Story 3.2.1's AC), and assert the sweep calls `process.kill(-backendPid, ...)`/`process.kill(-frontendPid, ...)` for the live one — never `process.kill(-pid, ...)` on the launcher's own PID field. This directly guards against the exact flaw Adversarial Blocker 1 found (a test that only encodes "kill the launcher's own PID" would pass while leaving both children running), so the test must fail if the implementation reverts to signaling `pid` instead of `backendPid`/`frontendPid`.
- **CI wiring — decided now, not deferred to the implementer:** This repo has no root `package.json` (confirmed — only `web-app/package.json` hosts `jest`/`test:e2e` scripts today per this project's own validation.md Test Stack section), so this test file (and `ports.test.ts` from Task 3.1.1c) do NOT get a new root-level test runner. Instead, add `scripts/dev-stack/**/*.test.ts` to `web-app/package.json`'s EXISTING Jest configuration — either by adding `<rootDir>/../scripts/dev-stack` to its `roots` array (or an equivalent `testMatch`/`testPathIgnorePatterns` adjustment), or via a small `jest.config.js` `projects` entry that adds a second project pointed at `../scripts/dev-stack` alongside the existing `web-app/` project — so that `cd web-app && npx jest` (the exact command root `CLAUDE.md` already documents under Testing: `cd web-app && npx jest --no-coverage`) picks up both new test files with no new root `package.json` and no new CI step. This is a small addition to the CI step that ALREADY runs `web-app`'s Jest suite, not a new test-runner surface. Whatever CI workflow currently invokes `cd web-app && npx jest` needs no new job — it will simply start passing/running these new specs by virtue of Jest's existing `roots`/`projects` config picking them up. Task 3.1.1c's `ports.test.ts` gets this exact same wiring, applied once, since both files live under `scripts/dev-stack/`.
- Files: scripts/dev-stack/launch.test.ts, web-app/package.json (or web-app/jest.config.js)

##### Task 3.2.1i: Manual CLI fast path — skip demo/live-session seeding entirely, distinct from Epic 5.1's fully-seeded Playwright path (~5 min)
- **Why this task exists:** requirements.md's Feasibility Risks section already named this need ("may need a 'skip seeding' fast path for the manual-use case … distinct from the full e2e-seeded case") but it was never converted into a plan.md task, and pre-mortem.md's Failure #1 (P1) flags that without it, Story 3.2.1's new cold-start-budget AC cannot be met — cold start stacks an orphan-reconciliation sweep, two `BindRetry`-guarded port allocations, a backend health-poll, and a `next dev` readiness-poll, and adding demo/live-session seeding on top (the work `tests/e2e/helpers/test-server.ts`'s `seedDemoData()`/`seedLiveSessions()` perform for the Playwright path) would push routine manual use well past a budget developers will actually tolerate.
- **Implementation:** Extend `startDevStack()`'s signature (Task 3.2.1a) to `startDevStack(name?: string, opts?: { seedData?: boolean })`, defaulting `seedData` to `false`. When `seedData` is `false` — which is every invocation reachable from the manual CLI's `main()` wrapper, since `main()` never passes `{ seedData: true }` — `startDevStack()` makes NO call into any `seedDemoData()`/`seedLiveSessions()`-equivalent logic; it proceeds directly from Task 3.2.1e's manifest write to the ready banner. This is a **NEW, separate code branch** gated by the `seedData` flag, not a change to `tests/e2e/helpers/test-server.ts` itself and not a change that would also disable seeding for the Playwright path — `tests/e2e/helpers/test-server.ts`'s own existing callers (the default, non-dev-mode e2e suite) are untouched by this task, since they don't go through `scripts/dev-stack/launch.ts` at all.
- **Cross-reference for future implementers:** When Epic 5.1 (Task 5.1.1a/b) is implemented, `global-setup.dev-mode.ts` MUST call `startDevStack(name, { seedData: true })` — explicitly opting back into seeding — because Epic 5.2's `backlog.dev-mode.spec.ts` needs a seeded backlog item to exist. Do NOT reuse this task's default fast (unseeded) path for the Playwright dev-mode harness; the two paths are intentionally distinct.
- Files: scripts/dev-stack/launch.ts

---

## Out of Scope (deferred during planning): "List Running Stacks"

A Phase 4 "List Running Stacks" epic — extending `WorkspaceMeta` with a `Port` field plus a
liveness dial, surfaced via a new `stapler-squad list-stacks` Cobra subcommand — was drafted
during planning but is **cut from this plan**. It was justified only by an inferred "unstated
need" from research (features.md §3), not by any bullet in requirements.md's Scope → In Scope
section, and its addition pushed the plan's actual phase count past what fits requirements.md's
stated Medium (1–2 week) appetite. "List running stacks" remains a reasonable follow-up feature,
but it is explicitly deferred rather than bundled into this feature's scope — see the matching
note added to requirements.md's Out of Scope section. This does **not** affect the
already-in-scope startup banner (Task 3.2.1e) that prints a running `DevStack`'s assigned ports
and data dir to stdout on launch, which remains the in-scope mechanism for "how does a developer
find their own stack's port" per requirements.md's Observability Requirements — only the
standalone subcommand for querying *other*, already-running stacks is deferred.

---

## Phase 5: Playwright Dev-Mode Harness

### Epic 5.1: Opt-in dual-server Playwright config + global-setup
Goal: prove the Phase 1-3 wiring actually works end-to-end via a real test run, without slowing down the existing default `npm test`/`test:e2e` suite (per requirements' Scope and architecture.md §4's explicit rejection of folding this into the default config).

#### Story 5.1.1: `playwright.dev-mode.config.ts` drives a real `next dev` + real dynamically-ported backend
As a contributor verifying frontend↔backend wiring, I want an opt-in Playwright mode that runs a real `next dev` against a real dynamically-ported backend, so that at least one spec proves the dynamic-port fixes work end-to-end without slowing down the default run.
Acceptance Criteria:
- `scripts/dev-stack/launch.ts` is refactored to also export `startDevStack(name?: string, opts?: { seedData?: boolean }): Promise<{ backendUrl: string; frontendUrl: string; stop: () => Promise<void> }>` alongside its existing CLI `main()` entry point, so the manual CLI and the Playwright harness share one implementation (the `DevModeTestServer` glossary term) — the Playwright path is required to pass `{ seedData: true }` explicitly (Task 3.2.1i's cross-reference); it must not rely on the manual CLI's `seedData: false` default.
- New `tests/e2e/playwright.dev-mode.config.ts` mirrors `tests/e2e/playwright.config.ts` (`fullyParallel: false`, `workers: 1`) but points `globalSetup`/`globalTeardown` at new `global-setup.dev-mode.ts`/`global-teardown.dev-mode.ts`, with `timeout`/`expect.timeout` raised to accommodate a cold `next dev` compile.
- `web-app/package.json` (or the root `package.json`, whichever currently hosts `"test:e2e": "playwright test"`) gains `"test:e2e:dev-mode": "playwright test --config=tests/e2e/playwright.dev-mode.config.ts"`, fully separate from `test:e2e`.
- Given a contributor runs `npm run test:e2e:dev-mode`, When `global-setup.dev-mode.ts` completes, Then `process.env.TEST_SERVER_URL` is set to the `next dev` frontend's URL (e.g. `http://localhost:54212`), not the backend's — mirroring the env-mutation-inheritance pattern already relied upon in `global-setup.ts:15-16` for the single-server mode.
Files: scripts/dev-stack/launch.ts, tests/e2e/playwright.dev-mode.config.ts, tests/e2e/global-setup.dev-mode.ts, tests/e2e/global-teardown.dev-mode.ts, web-app/package.json

##### Task 5.1.1a: Export `startDevStack()`/`stop()` from the launcher — verify it has zero ambient side effects (~4 min)
- Confirm (this should already hold by construction, per Tasks 3.2.1a/d's CLI-core split) that `scripts/dev-stack/launch.ts`'s port-allocation/spawn/health-poll logic is callable as `startDevStack(name?: string, opts?: { seedData?: boolean }): Promise<{ backendUrl, frontendUrl, stop }>`, and that the existing CLI path (`main()`, invoked only when the file is run directly) calls the same function with its `seedData` default (`false`, per Task 3.2.1i) unchanged. **Explicit requirement:** the exported `startDevStack()` — and the `stop()` closure it returns — MUST have zero ambient global side effects at module-import time, specifically NO `process.on('SIGINT'/'SIGTERM')` registrations outside `main()`. This is what makes it safe for `global-setup.dev-mode.ts` (Task 5.1.1b) to `import { startDevStack } from '../../scripts/dev-stack/launch'` and call it directly inside the Playwright test-runner process without attaching a second, conflicting signal handler alongside Playwright's own — closing Architecture Blocker 2. If this task finds any ambient `process.on(...)` call reachable via a bare `import` of the module, that is a regression against Task 3.2.1a/d's split and must be fixed before Epic 5.1 proceeds. **Do not repurpose Task 3.2.1i's fast (unseeded) path for this harness** — the next task (5.1.1b) must call `startDevStack()` with `{ seedData: true }` explicitly.
- Files: scripts/dev-stack/launch.ts

##### Task 5.1.1b: Create `global-setup.dev-mode.ts` (~4 min)
- Copy the shape of `tests/e2e/global-setup.ts` (lines 1-34) but call `startDevStack(name, { seedData: true })` from `scripts/dev-stack/launch.ts` instead of `startGlobalTestServer()` — passing `seedData: true` explicitly, since Epic 5.2's `backlog.dev-mode.spec.ts` needs a seeded backlog item and this harness must NOT take Task 3.2.1i's manual-CLI fast (unseeded) path; set `process.env.TEST_SERVER_URL = frontendUrl`; reuse the existing storageState-fixture-rewriting block (lines 20-34) verbatim since the dynamic-origin behavior it handles is identical.
- Files: tests/e2e/global-setup.dev-mode.ts

##### Task 5.1.1c: Create `global-teardown.dev-mode.ts` (~2 min)
- Mirror `tests/e2e/global-teardown.ts`'s shape, calling the `stop()` returned by `startDevStack()` (held via a module-level reference set in global-setup).
- Files: tests/e2e/global-teardown.dev-mode.ts

##### Task 5.1.1d: Create `playwright.dev-mode.config.ts` (~4 min)
- Clone `tests/e2e/playwright.config.ts` with `globalSetup`/`globalTeardown` pointed at the `-dev-mode` files and `timeout`/`expect.timeout` raised (e.g. `timeout: 60000`) to accommodate a cold `next dev` compile per stack.md §3.
- Files: tests/e2e/playwright.dev-mode.config.ts

##### Task 5.1.1e: Add `test:e2e:dev-mode` npm script (~2 min)
- Add `"test:e2e:dev-mode": "playwright test --config=tests/e2e/playwright.dev-mode.config.ts"` next to the existing `"test:e2e": "playwright test"` entry (confirm which `package.json` currently hosts it before editing).
- Files: web-app/package.json

### Epic 5.2: At least one spec proves backlog works end-to-end in dev-mode
Goal: satisfy the requirements' success metric that a real spec — not just code inspection — proves frontend↔backend wiring works under the new mode, using backlog per the original ask.

#### Story 5.2.1: `backlog.dev-mode.spec.ts` exercises backlog against the real dual-process stack
As a contributor, I want an e2e spec that exercises backlog functionality against the real `next dev` + real dynamically-ported backend, so that the dynamic-port wiring (CORS, API base URL override, hook URLs) is proven correct by an actual test.
Acceptance Criteria:
- `tests/e2e/backlog.dev-mode.spec.ts` starts with `// @feature backlog` (matching the convention already used by every spec in `tests/e2e/*.spec.ts`) and uses `test.describe('backlog-dev-mode', ...)`.
- Reuses the same backlog-item-visible locator(s)/page object(s) as `tests/e2e/backlog.spec.ts`, but runs against `playwright.dev-mode.config.ts`'s dynamically-assigned frontend origin instead of the static-exported build on port 8544.
- Given the dev-mode stack is up and a backlog item exists (seeded the same way `tests/e2e/backlog.spec.ts` seeds one), When the spec navigates to the backlog page, Then the item is visible by its existing `data-testid` locator — proving the browser successfully called the ConnectRPC backend across the `next dev` ↔ backend origin boundary.
- `docs/registry/features/backlog.json` (or wherever `make registry-generate` places the `backlog` feature entry) has `backlog.dev-mode.spec.ts`'s test name added to `testIds` after running `make registry-generate`.
Files: tests/e2e/backlog.dev-mode.spec.ts, docs/registry/features/*.json

##### Task 5.2.1a: Write `backlog.dev-mode.spec.ts` reusing existing locators (~4 min)
- Read `tests/e2e/backlog.spec.ts` in full to identify its seeding mechanism and locator(s)/page object usage, then write `tests/e2e/backlog.dev-mode.spec.ts` reusing the same locators with no other behavioral changes (proving parity under the new mode, not adding new coverage).
- Files: tests/e2e/backlog.dev-mode.spec.ts

##### Task 5.2.1b: Regenerate and commit the feature registry (~2 min)
- Run `make registry-generate` per root `CLAUDE.md`'s Feature Registry section, and commit the resulting diff to `docs/registry/features/*.json` reflecting the new spec's `testIds`.
- Files: docs/registry/features/*.json (generated)

---

## Out of Scope (explicit, per requirements.md)
- Running multiple systemd-managed / LaunchAgent-installed production-style instances side by side.
- Any change to `scripts/install-service.sh`, the systemd unit, or the macOS LaunchAgent.
- Any change to the static-export production build/deploy path (`output: "export"` itself stays unconditional; only the `next dev`/`next start` port and the browser-path env override are touched).
- Remote access / HTTPS multi-instance concerns (`--remote-access`, `remote-port` flag, `startRemoteAccess` in main.go).
- Migrating the entire existing Playwright suite to the new dev-mode harness — only `backlog.dev-mode.spec.ts` is added; `tests/e2e/backlog.spec.ts` and all other existing specs are untouched.
