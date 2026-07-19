# Research: Feature Landscape — isolated-dev-stacks

Date: 2026-07-03
Researcher: Research Agent 2 (Features)

## 1. Existing multi-instance / ephemeral-environment building blocks already in this codebase

### 1a. Free-port allocation pattern already exists — reusable as-is
`session/vnc/manager.go:650-661` (`pickFreePort`) binds `127.0.0.1:0`, reads the OS-assigned
port, then closes the listener:

```go
func pickFreePort() (int, error) {
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    ...
    port := ln.Addr().(*net.TCPAddr).Port
    _ = ln.Close()
    return port, nil
}
```

The comment explicitly documents the tradeoff this feature will inherit: *"There is a narrow
TOCTOU window, but this is acceptable on localhost per ADR-012."* This same idiom is used a
second time for CDP debugging ports in `session/instance_cdp.go` (`allocateCDPPort`, called from
`session/instance.go:789/833/865` — always *before* the tmux session/process is created so the
port can be injected into `ExtraEnv`). **Caveat found while verifying:** the cited "ADR-012" does
not match `docs/adr/012-react-virtuoso-log-viewer.md` — either a stale comment or a different
numbering scheme; flag this so the new feature's port-allocation logic doesn't cite a
nonexistent/mismatched ADR either.

The Playwright harness independently re-implements the identical idiom in TypeScript
(`tests/e2e/helpers/test-server.ts:10-20`, `findFreePort()` via `net.createServer().listen(0)`).
Both allocate late by default (`0` = "pick at start time"), with an env-var override for CI
(`TEST_SERVER_PORT`). **This is the pattern to reuse for backend AND `next dev` port allocation** —
no new primitive needs to be invented, just applied to two more places (backend HTTP listener,
`next dev`) and exposed via a CLI entry point.

### 1b. One-off session type = the closest existing "ephemeral, name-generated, isolated thing" pattern
`SessionTypeOneOff` (`config/types.go:156`, aliased in `session/instance.go:397-398`) already
implements: generate a unique name (`session/namegen` — `Generate()`, `GenerateAndCreate(baseDir,
maxAttempts)`), create a directory under a configurable base dir
(`config.OneOffBaseDirOrDefault()`, default `~/oneoff`), and wire it into the session as a normal
directory session. `server/services/session_service.go:1065-1069` is the canonical reference
implementation cited by `.claude/rules/session-creation-registry.md`. This is a strong template
for "human-friendly generated instance name" if the isolated-dev-stack launcher wants named stacks
without the developer picking a name — same `namegen` package, same generate-and-create-dir
idiom, just pointed at a stack-registry dir instead of a session dir.

### 1c. Workspace-hash isolation already gives every worktree/cwd a stable identity — but for DATA, not PORTS
`config/workspace_meta.go` + `.claude/docs/state-isolation.md` confirm the 4-tier isolation
hierarchy (explicit `STAPLER_SQUAD_INSTANCE` > test-mode PID dir > workspace-hash(cwd) > global
shared). `WorkspaceMeta` struct (`workspace_meta.go:15-22`) currently stores only
`WorkspaceID, Type, CWD, Name, ConfigDir, LastUsed` — **no port field**. `EnsureWorkspaceMeta()`
(called once at startup) writes this file to `~/.stapler-squad/workspaces/{hash}/workspace_meta.json`
(or `instances/{name}/` for named instances). `ListAvailableWorkspaces()` scans both subdirs and
is already consumed by `server/services/database_service.go:41` (`ListAvailableWorkspaces`) to
back a workspace-switcher UI feature — **this is the natural extension point for "list currently
running isolated stacks and their ports"**: add a `Port int` (and maybe `PID int`, `Running bool`)
field to `WorkspaceMeta`, write it at listen-time (not just at config-dir-resolution time), and the
existing scan/list machinery mostly falls out for free. Today this file only proves a workspace
*exists on disk*, not that its process is currently alive/listening — a "live" check would need to
be added (e.g. TCP dial or PID liveness) since stale directories are never cleaned up.

### 1d. No existing CLI "list instances/stacks" subcommand
`main.go` wires up an existing Cobra command tree: root (default: serve), `reset`, `debug`,
`version`, `test-pty`, `list` (session list within one instance's DB), `print-qr-codes`. There is
**no subcommand today that answers "what stacks/instances are currently running and on what
ports"** — confirming this is a real gap the new feature should close (Open Question #1 in
requirements: "make target vs shell script vs Go subcommand?" — a Go subcommand is the most
natural fit since it can reuse `ListAvailableWorkspaces` and could add a liveness probe).

### 1e. Hardcoded `localhost:8543` is baked into the hook-notification system — a real, currently-broken assumption for ANY non-default-port instance
This is the most important find. Two files hardcode the literal string `http://localhost:8543`
as the target for Claude Code hook callbacks that get written into a *session's own*
`.claude/settings.local.json`:

- `server/services/approval_handler.go:653`: `hookApprovalURL = "http://localhost:8543/api/hooks/permission-request"`
- `server/services/hook_injector.go:34-40` (`hookEndpoint` map): stop / pre-tool-use /
  post-tool-use / prompt-submit hooks, all literally `"http://localhost:8543/api/hooks/..."`

These constants are **not derived from the running server's actual `ListenAddress` or the request
context** — they are compile-time string literals. Practically: **any stapler-squad instance that
isn't listening on 8543 already has broken hooks today** (this predates the isolated-dev-stacks
feature — workspace-hash isolation solves data collisions but a second instance on a different
port, e.g. spun up for manual testing today by hand-exporting `PORT`, already silently breaks
permission-approval and stop-notification hooks for any Claude Code session it manages). This is
squarely in scope for "12-factor-style dynamic port/config resolution" and must be fixed
regardless — any isolated stack that itself creates/manages agent sessions (not just an ad-hoc
API smoke-test target) needs these hook URLs to resolve to ITS OWN listen address, not a hardcoded
8543.

### 1f. Frontend hardcodes 8543 as a dev-mode fallback (already env-var overridable) and a fully-hardcoded auth URL
- `web-app/src/lib/config.ts:22`: `getApiBaseUrl()` falls back to
  `process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8543/api'` when not in a browser — already
  overridable via `NEXT_PUBLIC_API_URL`, consistent with the requirements' baseline description.
  Note the **build-time vs runtime risk** flagged in the requirements' Feasibility Risks is real:
  `NEXT_PUBLIC_*` vars are inlined by Next.js at build time for anything that isn't
  server-rendered/runtime-evaluated; for a `next dev` server this is fine (dev server reads env at
  request time), but this same constant is presumably used by the static-export production build
  path too — the isolated-stack feature must ensure it only touches the `next dev` code path.
- `web-app/src/lib/auth/passkey.ts:23`: `return "http://localhost:8543/auth";` — this one is
  **not** env-var overridable at all; it's a plain hardcoded return. This would break passkey
  auth flows against any isolated stack on a non-8543 port and was not mentioned in the
  requirements' "Rabbit Holes" section (which only called out "Next.js dev server proxy/rewrites").
  No `rewrites()`/`proxy` config referencing 8543 was found in `web-app/next.config.ts` itself —
  that specific rabbit hole appears to be a non-issue, but the passkey URL is a previously-unknown
  one that should be added to scope.
- `web-app/package.json:14,16`: `"dev": "next dev --port 3001"` and `"start": "next start --port
  3001"` confirm the requirements' baseline claim — the dev port is a hardcoded CLI flag in the
  npm script, not env-driven at all today.

### 1g. MCP server and backlog service have NO hardcoded host/port assumptions
Grepped `server/mcp/*.go` (all tool files: `tools_backlog.go`, `tools_github.go`,
`tools_lifecycle.go`, `tools_terminal.go`, `tools_vcs.go`, `tools_discovery.go`, `tools_goal.go`,
`server.go`) and `server/services/backlog_service.go` for `http://`, `8543`, `ListenAddress`,
`BaseURL` — zero matches. This confirms the requirements' framing: MCP is mounted on the same HTTP
mux as everything else and calls Go service methods in-process, so it has no separate
self-referential URL to get wrong. The only way an isolated stack's MCP endpoint could resolve
incorrectly is at the CLIENT side (whatever tool/config points an MCP client at `/mcp`) — that
config lives outside this repo's control (e.g. `.mcp.json` or Claude Code settings pointing at a
URL), and should just be told the resolved port, not something this repo needs to fix internally.

## 2. Edge cases / failure modes to design for

1. **Concurrent free-port race**: `pickFreePort()`'s bind-then-close pattern (already accepted as
   a tradeoff for VNC/CDP ports per the ADR-012 comment) has a real TOCTOU window between closing
   the probe listener and the actual server binding it. Two stacks launched within the same
   instant could both pick the same port if launched by two independent `go build` binaries (no
   shared allocator/lockfile). Mitigation options seen nowhere in this codebase yet: retry-on-
   bind-failure loop, or a lockfile/registry of claimed ports under `~/.stapler-squad/`.
2. **Workspace-hash collision for identical worktree paths**: `workspace_meta.go` derives
   `WorkspaceID` from `filepath.Base(configDir)`, and `configDir` is presumably `SHA256(cwd)` per
   `state-isolation.md`. Two *literally identical* absolute paths (e.g. two shells `cd`'d into the
   same worktree) will legitimately hash-collide by design — that's correct dedup behavior for
   data isolation, but if the new port-allocation scheme derives a *deterministic preferred port*
   from the same hash (see research question 4), it must additionally handle "this hash's
   preferred port is already bound by a live instance in a different terminal" — same directory,
   two terminals, one wants a fresh port instead of colliding on the deterministic one.
3. **Orphaned `next dev` after parent Go process dies**: No process-group/supervision link is
   evident between the future launcher and a spawned `next dev` child beyond whatever the launcher
   script does — `tests/e2e/helpers/test-server.ts` uses `spawn()` + `SIGTERM` then `SIGKILL` after
   5s specifically to avoid orphaning the *Go* binary, but there is no `next dev` process in that
   harness at all today (confirming the stated baseline gap in requirements — the e2e harness only
   starts the Go binary). Any new "backend + next dev" launcher needs the same graceful-then-force
   kill discipline applied to BOTH children, plus handling the case where the parent (e.g. a
   Claude Code session's shell) is killed via `SIGKILL` and can't run cleanup at all — an orphan
   `next dev` would keep holding its port and its own file-watcher/HMR websocket.
4. **Developer/agent forgetting which port a named stack is on**: confirmed as a real gap — see
   1c/1d above; no current mechanism surfaces "stack X is on port Y" outside of reading the
   terminal it was launched in.
5. **Global test-server singleton assumption**: `tests/e2e/helpers/test-server.ts:219-226`
   (`getGlobalTestServer()`) is a module-level singleton — exactly the Rabbit Hole called out in
   requirements ("Existing e2e global-setup/teardown assumes exactly one global test server").
   Any new "backend + `next dev`" e2e mode needs its own parallel singleton/lifecycle (or a
   refactor of this one to be parameterizable) rather than trying to extend the existing one
   in-place, since existing specs presumably import `startGlobalTestServer`/`stopGlobalTestServer`
   directly and depend on exactly one instance across the whole Playwright run.
6. **Stale `workspace_meta.json` implies "workspace" when process is long dead**: `EnsureWorkspaceMeta()`
   only writes at startup and is never cleaned up on shutdown/crash — `ListAvailableWorkspaces()`
   will report workspaces that no longer have a live process. Any "list running stacks" feature
   built on top of this needs an explicit liveness check (e.g., dial the recorded port, or check
   PID) rather than trusting the file's mere existence.

## 3. Unstated needs beyond the explicit requirements

- **A way to list currently-running isolated stacks and their ports** — not requested explicitly
  but implied by "the developer forgetting which port a given named stack is on" scenario in the
  research prompt, and there is no existing feature that does this (closest is the workspace list
  RPC in `database_service.go`, which lists *known workspaces on disk*, not *live stacks with
  ports*).
- **A way for a Claude Code session working in a worktree (per this repo's own CLAUDE.md) to avoid
  accidentally running `make install-service`** (which restarts the *shared systemd instance*) when
  its actual intent was "spin up an isolated stack to test in". `CLAUDE.md` explicitly instructs
  `make install-service` as the way to rebuild+restart, with no isolated equivalent documented — a
  session following CLAUDE.md literally while trying to "smoke-test a change" (one of this
  project's own stated Users/Consumers) has no guardrail today stopping it from restarting the
  live systemd service instead of the isolated stack it meant to use. The new feature should give
  this an explicit, differently-named target/command so muscle-memory (`make install-service`)
  doesn't clobber the real instance.
- **Fixing the hook-URL hardcoding (1e above) is implicitly required, not just "nice to have."**
  The requirements only call out MCP resolving correctly under a dynamic port; they don't mention
  hooks at all, but any isolated stack that spawns/manages Claude Code sessions (one of the stated
  use cases — "Claude Code sessions... that need to smoke-test server changes") will silently get
  broken permission-approval/stop-notification hooks unless `hookApprovalURL`/`hookEndpoint` are
  made to resolve against the actual running instance's listen address.
- **A cleanup/teardown story for the passkey auth hardcoded URL** (1f) — not mentioned in
  requirements' Rabbit Holes, needs to be added to scope or explicitly deferred as a known gap.
- **Likely need to distinguish "ephemeral test stack" from "long-running named dev stack"** in
  whatever registry gets built — a stack spun up for a single Playwright run has different
  cleanup/visibility needs (must always tear down, doesn't need to show up in a "list my stacks"
  UI) than a stack a developer names and keeps running across a coding session (should show up,
  should survive being listed later). The one-off session precedent (1b) and the test-mode PID-dir
  precedent (`state-isolation.md` tier 2) are two different existing answers to "ephemeral
  identity" that a unified design should reconcile rather than inventing a third.

## 4. Can ports be deterministically derived from the workspace hash, or must they always be dynamic?

- **Data-dir isolation is already fully deterministic** and keyed by `SHA256(cwd)` (per
  `state-isolation.md` priority 3) or by explicit `STAPLER_SQUAD_INSTANCE` name (priority 1) — this
  identity is stable across restarts of the same worktree/directory.
- **Nothing in the codebase today derives a port from this hash.** All existing port allocation
  (`pickFreePort` for VNC/CDP, `findFreePort` in the e2e harness, the `PORT` env var test-mode
  path in `main.go`) is fully dynamic/late-bound — bind-to-`:0`-and-read, or explicit env var
  override. There is no precedent for hash-derived deterministic ports anywhere in this codebase.
- **A hash-derived preferred port (e.g. `8543 + hash(workspace_id) % N`, with fallback to
  `pickFreePort()` on bind failure) is architecturally consistent with the existing isolation
  identity model and would directly serve the "developer forgetting which port a stack is on"
  problem** (the port becomes predictable/memorable per-worktree instead of announced only at
  start time) — but this is a genuinely new mechanism, not something to "discover already built."
  Given the existing dynamic-allocation precedent is well-tested (VNC/CDP, e2e harness) and the
  deterministic approach adds new collision-handling complexity (two worktrees whose hash
  collides into the same preferred port must fall back gracefully — this is just the general free
  -port race from edge case #1, not a new problem), the safer design is: **try the hash-derived
  port first, fall back to `pickFreePort()`, and persist whichever port was actually bound into
  the (extended) `WorkspaceMeta`** so it can be listed later (closing the "unstated need" in
  section 3). This gets the memorability benefit without abandoning the proven dynamic-allocation
  fallback path.
