# Requirements: isolated-dev-stacks

**Date**: 2026-07-03
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement

Developers working on stapler-squad cannot run an ad-hoc backend/frontend stack (for manual testing, MCP server testing, or the backlog feature, all of which require a live server) without either colliding with the systemd-managed instance the user is actively using for day-to-day session management, or hand-picking free ports and env vars every time. There is also no harness that runs a real `next dev` frontend against a real Go backend for end-to-end testing — the existing Playwright e2e harness only exercises the static-exported build served by the Go binary.

For whom: Tyler (and any future contributor) doing local development/testing on this repo, plus any Claude Code session working inside a worktree that wants to smoke-test the server before shipping a change.

## Baseline

Today:
- The Go backend defaults to `localhost:8543`, overridable only via `--listen-address`, a config file value, or the `PORT` env var (test-mode only path in `main.go`).
- The Next.js dev server is hardcoded to port 3001 (`web-app/package.json`); its API base URL defaults to `http://localhost:8543/api` unless `NEXT_PUBLIC_API_URL` is exported manually.
- Data directory isolation already exists (`STAPLER_SQUAD_INSTANCE`, workspace-hash-based default per `.claude/docs/state-isolation.md`) — this solves *data* collisions but not *port* collisions.
- The existing Playwright harness (`tests/e2e/helpers/test-server.ts`) already dynamically allocates a free backend port and an isolated data dir per run — but it only starts the Go binary (serving the static-exported frontend), never a real `next dev` process.
- Net effect: starting a second manual instance (e.g. inside a worktree session to test a change) either hardcodes port 8543 (colliding with the systemd instance) or requires the developer to manually export `PORT`, `STAPLER_SQUAD_INSTANCE`, and `NEXT_PUBLIC_API_URL` in the right combination, by hand, every time.

## Users / Consumers

- Tyler running local dev/test stacks from a terminal or a worktree session.
- Claude Code sessions (this repo's own product) that need to smoke-test server changes without disturbing the systemd-managed instance.
- Playwright e2e suite (`tests/e2e/`), extended to optionally drive a real `next dev` frontend against a real backend.

## Success Metrics

- A developer can start N (N ≥ 2) fully isolated backend+frontend stacks concurrently on one machine with zero manual port bookkeeping — no `--port` flags, no manually exported env vars beyond one instance-identifying value.
- Starting an ad-hoc/dev/test stack never binds the systemd instance's configured port or touches its data directory, even if both happen to run at the same time (today this is possible only by accident of the developer remembering to set `PORT`).
- A new Playwright e2e mode exists that launches a real `next dev` process wired to a dynamically-ported real backend, and at least one existing e2e spec is migrated to (or a new spec added for) this mode to prove frontend↔backend wiring works end-to-end.

## Appetite

Medium (1–2 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

(Note: the final plan's effort estimate — ~10-21 hours including test/debug/review overhead — lands in the lower-to-middle portion of this range with meaningful margin, not at its upper end; see plan.md's Effort Reconciliation section for the arithmetic.)

## Constraints

- No changes to `scripts/install-service.sh`, the systemd unit, or the macOS LaunchAgent — the systemd-managed production-style instance stays single-instance and untouched by this work.
- Must not require the developer to manually pick or track ports across backend, frontend, and (if applicable) any auxiliary process.
- Must build on the existing `STAPLER_SQUAD_INSTANCE` / workspace-hash isolation mechanism rather than introducing a second, competing isolation identifier.

## Non-functional Requirements

- **Performance SLO**: not specified — this is developer tooling, not a production request path. Cold-start time for a new isolated stack (backend + `next dev`) should stay in the same order of magnitude as today's Playwright global-setup (~30–45s observed for backend alone).
- **Scalability**: expected concurrency is small (2–5 simultaneous stacks on a single dev machine), not applicable at any larger scale.
- **Security classification**: internal/development-only. No new externally-reachable surface — all ports should default to binding `localhost` only, same as today.
- **Data residency**: not applicable.

## Scope

### In Scope
- A 12-factor-style dynamic port/config resolution mechanism for the Go backend (extending the existing `PORT` env var support beyond test-mode) and for the Next.js dev server (replacing the hardcoded `--port 3001` and the manual `NEXT_PUBLIC_API_URL` export), keyed off the same instance identifier used for data-dir isolation today.
- A way to allocate a free port automatically when one isn't explicitly pinned (mirroring `findFreePort()` in `tests/e2e/helpers/test-server.ts`, generalized so both frontend and backend use it consistently).
- Ensuring MCP tooling (served at `/mcp` on the same backend HTTP mux — confirmed no separate port today) continues to resolve correctly against whichever port an isolated stack picked.
- A script/CLI entry point to spin up one named isolated stack (backend + `next dev`) on demand for interactive manual testing, without colliding with the systemd instance or with any other isolated stack.
- A new Playwright e2e mode/harness that starts both a dynamically-ported backend and a dynamically-ported `next dev` process, wires the frontend's API base URL to the chosen backend port automatically, and tears both down cleanly.
- Migrating or adding at least one e2e spec that runs against this new "real frontend + real backend" mode, specifically covering a server-dependent feature (backlog and/or MCP-adjacent behavior) called out in the original ask.

### Out of Scope
- Running multiple systemd-managed / LaunchAgent-installed production-style instances side by side (explicitly deferred per user decision).
- Any change to the static-export production build/deploy path (`output: "export"`, `make install-service`).
- Remote access / HTTPS multi-instance concerns (`--remote-access`, `remote-port` flag) — isolation here is for local dev/test only.
- Migrating the *entire* existing Playwright suite to the new real-frontend mode; only enough specs to prove the mechanism works.
- **Listing/discovering already-running dev stacks** (e.g. a `list-stacks` command showing every named stack's port and liveness) — considered during planning as a natural companion to this feature, and a reasonable follow-up, but explicitly deferred to keep this feature's scope inside its Medium (1–2 week) appetite. The already-in-scope startup banner (prints a stack's own assigned ports/data dir to stdout on launch) still covers "find my own stack's port"; only cross-stack discovery of *other* running stacks is deferred.

## Rabbit Holes

- **WebSocket/terminal streaming reconnection** (`useTerminalStream.ts`, ConnectRPC streaming) may have assumptions baked in about a stable origin/port across a browser session — flag for Phase 3 planning to verify the dynamic-port frontend can still maintain a terminal stream without reconnect storms.
- **Next.js dev server proxy/rewrites** (if `next.config.ts` has any hardcoded rewrite targets pointing at 8543) could silently break when the backend port is dynamic — needs a research pass, not an assumption.
- **Process/port leak on crash**: if a stack's backend or `next dev` process is killed ungracefully (e.g. SIGKILL, terminal closed), stale ports/data dirs could accumulate — needs explicit cleanup handling, not just a happy-path stop() method.
- **macOS-specific behavior**: the systemd rule file only documents Linux systemd + a macOS LaunchAgent path; verify the ad-hoc stack launcher doesn't assume Linux-only tooling.
- **Existing e2e global-setup/teardown** currently assumes exactly one global test server; extending to a "real frontend" mode may require rethinking whether that stays a Playwright global fixture or becomes per-worker.

## Alternatives Considered

- **Docker Compose per-stack isolation**: rejected as first choice — heavier than needed for a Go binary + `next dev` process, and the repo has no existing Docker workflow to build on (per Constraints: build on existing isolation mechanism).
- **Manual env var convention (status quo, just documented better)**: rejected — user explicitly wants this to stop requiring manual bookkeeping ("dynamic 12-factor type configuration").

## Feasibility Risks

- Next.js `next dev` cold start plus Go backend cold start (with demo/live session seeding per `test-server.ts`) could push isolated-stack startup time high enough to make frequent ad-hoc use annoying — may need a "skip seeding" fast path for the manual-use case (distinct from the full e2e-seeded case).
- ConnectRPC transport configuration (`transport.ts`) reads `getApiBaseUrl()` at module-load time in some tests (per grep hits) — if the real app resolves the API base URL similarly, dynamic-port wiring may require a build-time vs. runtime env var distinction (`NEXT_PUBLIC_*` vars are inlined at build time in Next.js static export, but should be runtime-readable in `next dev`).

## Observability Requirements

*(complexity ≥ 3)* Standard request logging is sufficient — this is developer/test tooling, not a production code path. The isolated stack launcher should print its assigned ports and data directory to stdout on startup (mirroring the existing `✅ Test server started on http://localhost:{port}` pattern) so a developer can find and connect to a given stack without additional instrumentation.

## Risk Control

*(complexity ≥ 3)* Not needed for most of this feature (low risk, dev tooling can fail visibly) — with one exception: orphan-process reconciliation (killing leaked backend/next-dev processes from a prior hard-killed launcher) is a correctness requirement, not optional error-handling polish, because an un-reaped orphan persistently blocks every subsequent launch of that instance name, directly undermining the "zero manual bookkeeping" Success Metric rather than just failing once visibly. See plan.md Task 3.2.1g. Everywhere else in this feature, a broken isolated-stack launcher fails visibly (process won't start / health check times out) rather than silently, and the systemd-managed instance is explicitly out of scope so it cannot be destabilized by this work.

## Open Questions

- Should the ad-hoc stack launcher be a new `make` target, a shell script under `scripts/`, or a Go subcommand (`stapler-squad dev-stack ...`)? Defer to Phase 3 planning/architecture review.
- Should isolated stacks get a short human-friendly instance name (e.g. auto-generated or user-supplied) surfaced in logs/terminal titles, reusing `STAPLER_SQUAD_INSTANCE`, or is workspace-hash-based defaulting sufficient for the ad-hoc case? Defer to research phase (state-isolation mechanics).
- Does `next.config.ts` (or any proxy layer) have hardcoded references to port 8543 beyond the `NEXT_PUBLIC_API_URL` fallback already found? Needs a research pass before planning.
