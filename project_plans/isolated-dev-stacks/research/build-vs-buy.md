# Build vs. Buy: isolated-dev-stacks

Date: 2026-07-03
Agent: Research Agent 6 (Build vs. Buy)

## Codebase baseline (checked before recommending anything)

- `go.mod`: no config-parsing dependency of any kind (no `envconfig`, `caarlos0/env`, `viper`, etc.) and no free-port library (no `phayes/freeport` or similar). All env access in the Go codebase is plain `os.Getenv(...)` (see `config/config.go:74,100,132,350,715` and `main.go:191`).
- `main.go:185-197` already implements the exact precedence chain the feature needs: **flag > config file > `PORT` env > default (`localhost:8543`)**. This is hand-rolled, ad hoc, and not centralized in a struct/library — but it already exists and works.
- `web-app/package.json`: `dev` script is `next dev --port 3001` (hardcoded). No `concurrently`, `npm-run-all`, `foreman`, `overmind`, or `hivemind` in `dependencies`/`devDependencies`.
- `tests/e2e/helpers/test-server.ts:10-20`: `findFreePort()` opens a `net.createServer()`, binds to port 0, reads the assigned port, then **closes the listener** and hands the bare number back to the caller, who spawns the Go binary with `PORT=<n>` several steps later (`ensureBinary`, `seedDemoData` run in between). This is a classic check-then-close-then-use TOCTOU gap — the port is free at check time but not held.
- `session/` already contains a mature process-supervision layer purpose-built for "manage a child process/pty, tear down cleanly": `session/tmux_process_manager.go`, `session/native_process_manager.go`, `session/tmux/` (tmux.go, tmux_unix.go, tmux_windows.go), with lifecycle methods (`Start`, `Close`, `IsAlive`, `supervise`, `SetOnExitCallback`) that already handle both a real-tmux backend and a native-PTY backend, cross-platform.

---

## 1. Dynamic port allocation + process orchestration

### 1a. Free-port allocation

**`github.com/phayes/freeport`** (Go)
- Pros: trivial one-function API (`freeport.GetFreePort()`), zero transitive deps.
- Cons: latest tagged release is 1.0.2 from Oct 2017; 36 commits total; no activity since. It does *exactly* what the hand-rolled `net.Listen(":0")` + close pattern already used in this repo's own e2e helper does — it does **not** hold the socket open, so it carries the identical TOCTOU exposure the requirements doc is worried about. Adding it buys zero risk reduction over what's already written, for a new dependency.
- Verdict: **Not recommended.** No improvement over in-house code and effectively unmaintained.

**Hand-rolled `net.Listen(":0")` (Go side) / `net.createServer().listen(0)` (TS side, already in use)**
- Pros: zero dependencies, one function, already proven in this codebase for the existing e2e harness.
- Cons: TOCTOU window between "free at check" and "free at bind" unless the same listener/fd is reused.
- Verdict: **Recommended**, provided the TOCTOU gap is closed (see §4) rather than accepted as-is.

### 1b. Process orchestration ("run backend + `next dev` together, tear down cleanly")

Candidates considered: `concurrently` (npm), `npm-run-all` (npm), `foreman`/`overmind`/`hivemind` (Procfile-style, external binaries).

- `concurrently` / `npm-run-all`: pure task runners — they interleave stdout and propagate a kill signal to child processes on Ctrl-C, but have no concept of health-checking, dynamic port injection, or per-stack isolation. They'd need a wrapper script anyway to allocate ports and set env vars before invoking them, and they add a Node dependency for something this repo's Go binary/CLI can trivially do with `os/exec`.
- `foreman`/`hivemind`: read a `Procfile`, spawn processes, and (notably for `overmind`) multiplex their output through **a tmux session** — i.e., these tools reimplement, using tmux, the exact pattern `session/tmux_process_manager.go` and `session/tmux/` already implement in this repo, natively, in Go, integrated with the existing session model, config, and instance lifecycle. `overmind` is MIT-licensed, still tagged-released (v2.5.1, Mar 2024) and Unix/macOS-only (requires tmux installed separately) — but even at its healthiest, adopting it means running "tmux managed by a second orchestration tool" alongside "tmux managed by this repo's own session package," i.e. two competing supervisors for the same primitive.
- Verdict: **Not recommended** to add any of these as a dependency. **Recommended** to build the isolated-stack launcher as a thin Go CLI/script that (a) allocates ports, (b) sets env vars per the 12-factor precedence already established in `main.go`, and (c) uses `os/exec` (or, if visibility/attach-ability into the running dev stack is valuable, the repo's own `session/` tmux/native-process-manager abstractions) to start and stop the backend + `next dev` pair. This is a few dozen lines glue, not a new subsystem, and keeps exactly one process-supervision mechanism in the codebase instead of two.

---

## 2. Docker Compose as an alternative

The requirements.md "Alternatives Considered" section already rejects Docker Compose on two grounds: (a) heavier than needed for "a Go binary + `next dev` process," and (b) no existing Docker workflow to build on, which the Constraints section separately locks in (no Docker/compose in use for dev).

This holds up under scrutiny, and the cost side is worse than the doc even states:
- **New dev dependency surface**: contributors would need Docker Desktop/Engine installed and running just to do ad-hoc manual testing or MCP server testing — a much higher bar than "run `./stapler-squad`" for a repo that currently has zero Docker footprint.
- **Dev-loop speed**: `next dev` and the Go binary both rely on fast local recompilation/HMR. Containerizing either means either (a) rebuilding an image on every change (dead on arrival for iteration speed), or (b) bind-mounting source and running the dev servers inside the container anyway — at which point Docker adds process/network-namespace overhead and platform-specific volume-mount slowness (especially on macOS, which this repo explicitly supports per the LaunchAgent/codesigning docs) without adding any isolation Go/Node don't already give you via distinct ports + distinct data directories.
- **Isolation actually needed**: the isolation this feature needs is port + filesystem-state isolation, which the existing `STAPLER_SQUAD_INSTANCE` / workspace-hash mechanism plus per-stack free ports already deliver without namespaces or containers. Docker's stronger isolation (network namespaces, cgroups) solves a problem this feature doesn't have.
- Verdict: **Not recommended.** The rejection in requirements.md stands; the real-world cost (Docker Desktop as a new required dev tool, image-rebuild-on-every-change latency, macOS volume-mount overhead) is larger than the doc implies, and the isolation gained doesn't map to gaps that native process/port isolation leaves open.

---

## 3. 12-factor config libraries (Go)

Checked `go.mod` and `config/config.go`: this repo has never adopted `envconfig`, `caarlos0/env`, `viper`, or any struct-tag-based env parser. The existing pattern is explicit, scattered `os.Getenv("X")` calls plus a hand-maintained precedence chain in `main.go` (flag > config-file field > env var > default) and a JSON-persisted `Config` struct (`config/config.go`) for everything that isn't transient.

- `github.com/caarlos0/env`: MIT, 6.2k stars, actively released (v11.4.1 as of Jan 2026), self-describes as "feature-complete" (bug-fix-only maintenance mode going forward) — mature and safe, but it's a full config-loading library aimed at apps that build their entire config from a tagged struct at startup.
- `github.com/kelseyhightower/envconfig`: MIT, 5.5k stars, but last tagged release is 1.4.0 from 2019, with 27 open issues / 30 open PRs — stale relative to `caarlos0/env`.
- Adopting either for *just* this feature's few new env vars (backend port, frontend port, API base URL, instance/workspace id — all of which already have a mechanism) would introduce a second config-loading convention alongside the existing hand-rolled `os.Getenv` + `Config` struct pattern, which is exactly the "second competing identifier/style" the requirements doc's Constraints section warns against for the instance-ID mechanism, and the same logic applies to config style.
- Verdict: **Not recommended** to adopt a config library for this feature specifically. **Recommended**: extend the existing `main.go` precedence chain and `Config` struct pattern (flag > config > env > default) to the one or two new variables this feature needs (e.g. a frontend port), matching the codebase's existing convention rather than introducing a second one. If a future, much larger config surface ever justifies `caarlos0/env`, that's a separate, repo-wide decision — out of scope here.

---

## 4. Free-port allocation: hand-rolled fix vs. library, verdict

The current `findFreePort()` in `tests/e2e/helpers/test-server.ts` has a genuine TOCTOU gap: it opens a listener on port 0, reads the OS-assigned port, **closes the listener**, and returns a bare number — with real work (`ensureBinary()`, `seedDemoData()`, multiple `execPromise` calls) happening before that port number is ever bound again by the spawned Go process. On a single dev machine this is low-probability but not zero: any other process on the machine (another `next dev`, another ad-hoc stack from this very feature, a browser, anything ephemeral) can grab that port in the intervening window, especially once N≥2 concurrent isolated stacks (the explicit success metric) are being spun up back-to-back and racing for the ephemeral port range.

Investigated the most credible off-the-shelf fix, `sindresorhus/get-port` (npm, MIT, actively released — v7.2.0 as of Mar 2026):
- It candidly documents the same residual risk for the *cross-process* case: "There is a very tiny chance of a race condition if another process starts using the same port as you between the time you get the port and actually start using it" — for multi-process races it does **not** claim to eliminate the gap, only to make it rare and to expect an `EADDRINUSE` retry.
- What it *does* solve well is the *in-process* race (parallel calls to `getPort()` returning the same number to two callers within the same Node process): it holds returned ports in a lock table for 15-30 seconds (or for the process lifetime with a `reserve` option), which the current in-house `findFreePort()` has no equivalent for. That specific failure mode — this feature spinning up N≥2 stacks concurrently *from the same test-runner/CLI process* — is exactly the scenario the success metric calls out, and it's the one part of the race this codebase doesn't currently guard against at all.

Verdict, split by failure mode:
- **In-process race across N concurrently-started stacks** (the actual N≥2 success metric): **library-shaped guarantee is worth it**, but doesn't require adding a new npm dependency — the same effect (hold/lock returned ports for the life of the launcher process) can be replicated in ~10 lines by keeping the allocating socket open (don't close it immediately; pass its fd or delay close until just before spawning the real server) or by maintaining an in-memory "already handed out" set for the duration of the launcher process. **Recommended: hand-rolled fix**, specifically closing the currently-open gap between "listener closed" and "process spawned" by either (a) not closing the probe listener until immediately before spawn, or (b) tracking allocated ports in-process so two concurrent `findFreePort()` calls from the same launcher can never collide.
- **Cross-process race against some unrelated process on the same machine**: negligible actual risk given the stated context (single dev machine, low concurrency, developer-triggered, visible failure mode). No library — including `get-port` — claims to fully eliminate this, and it isn't worth adding a dependency (Go or npm) to shave an already-small probability down further. **Recommended: accept + fail loudly** (health-check timeout / bind error surfaces immediately, per the requirements doc's Risk Control section, which already accepts "fails visibly" as sufficient for this feature).
- **Net verdict: hand-rolled fix, not a library.** The specific bug this repo needs to fix (own-process double-allocation across N concurrent stacks) is solvable without a new dependency, and the residual cross-process risk is not eliminated by any candidate library either — so the cost of adding `get-port` (or a Go equivalent) buys no additional safety over the free fix, only a new dependency.

---

## Summary table

| Area | Candidate | Verdict |
|---|---|---|
| Free-port allocation (Go) | `phayes/freeport` | Not recommended (stale, no TOCTOU fix) |
| Free-port allocation (general) | hand-rolled `net.Listen(":0")` + in-process hold | Recommended |
| Process orchestration | `concurrently` / `npm-run-all` | Not recommended (no port/health-check logic, new dep for glue code) |
| Process orchestration | `foreman` / `overmind` / `hivemind` | Not recommended (overmind duplicates in-repo tmux supervision) |
| Process orchestration | reuse `session/` tmux/native-process-manager or plain `os/exec` | Recommended |
| Isolation strategy | Docker Compose | Not recommended (confirms requirements.md rejection; cost is worse than stated) |
| Config library (Go) | `caarlos0/env` | Viable in the abstract (healthy, mature) but not recommended for this feature — would fork config style |
| Config library (Go) | `kelseyhightower/envconfig` | Not recommended (stale: 2019 last release) |
| Config approach | extend existing `main.go`/`Config` precedence chain | Recommended |
| Port-race fix | `sindresorhus/get-port` (npm) | Not recommended — doesn't cover the risk that matters here any better than a free fix |
| Port-race fix | hand-rolled in-process port-reservation / delayed-close | Recommended |
