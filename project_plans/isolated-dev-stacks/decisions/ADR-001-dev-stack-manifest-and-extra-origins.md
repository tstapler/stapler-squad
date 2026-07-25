# ADR-001: DevStack Manifest Format and CORS Extra-Origins Mechanism

Date: 2026-07-03
Status: Accepted
Project: isolated-dev-stacks

## Context

Two new conventions are introduced by this feature that have no precedent elsewhere in the
codebase and are not raw reuse of an existing pattern, so they need to be written down:

1. A discovery-only JSON manifest file per running isolated dev stack.
2. A new env var that extends the backend's CORS-trusted-origin allowlist.

Everything else in this feature (bind-and-hold port allocation, `ManagedProcess`-style
process-group teardown, the `flag > env > config > default` precedence chain) is a direct
reuse of an existing, already-documented pattern (see `docs/adr/017-cdp-port-toctou-tradeoff.md`
for the port-allocation tradeoff specifically) and does not need a new ADR.

## Decision

### 1. `DevStackManifest` file format

Path: `~/.stapler-squad/instances/<name>/dev-stack.json`, written once by the Node
`StackLauncher` (`scripts/dev-stack/launch.ts`) after both children report ready, and never
read by any running process — it exists purely for a human (or a future tool) to `cat` it and
find a stack's URLs without scrolling terminal history.

```json
{
  "schemaVersion": 2,
  "instance": "my-feature-test",
  "backendPort": 54211,
  "frontendPort": 54212,
  "apiBaseUrl": "http://localhost:54211/api",
  "dataDir": "/Users/tyler/.stapler-squad/instances/my-feature-test",
  "pid": 40213,
  "backendPid": 40215,
  "frontendPid": 40219
}
```

This mirrors the existing `workspace_meta.json` precedent (`config/workspace_meta.go`) in
spirit — a small, write-once-per-startup JSON file for discovery — but is deliberately a
*separate* file rather than an extension of `workspace_meta.json` itself, because it is written
by the Node launcher (a different process/language than the Go binary that owns
`workspace_meta.json`) and needs to exist *before* the Go binary has necessarily written its own
metadata.

A cross-stack "list all running stacks" survey (which would have extended `WorkspaceMeta` with
its own `Port` field and a Go-owned liveness dial) was considered during planning but deferred
as scope creep beyond this feature's Medium (1-2 week) appetite — see plan.md's "Out of Scope
(deferred during planning)" note and requirements.md's Out of Scope section. `dev-stack.json`'s
format decision above stands on its own regardless: it exists as a launcher-owned,
human-readable convenience artifact for a single stack, independent of whether a future,
separately-scoped feature ever adds a Go-owned cross-stack discovery path on top of
`WorkspaceMeta`.

**`backendPid`/`frontendPid` fields and lifecycle (not purely informational — also drive orphan
reconciliation): these, not `pid`, are what the reap logic actually signals.** Because both
`BackendChild` and `FrontendChild` are spawned with `detached: true` (implementation plan Task
3.2.1b/c), each child is the *leader of its own, separate process group* — its own PID doubles as
its process group ID under POSIX semantics. That means `process.kill(-backendPid, 'SIGTERM'/'SIGKILL')`
and `process.kill(-frontendPid, 'SIGTERM'/'SIGKILL')` are the only calls that actually reach each
child's full process group (the child itself plus anything it spawned, e.g. `next dev`'s own
`next-server` grandchild) — this is also exactly why Task 3.2.1d's teardown code already issues
two independent per-child kill calls rather than one. `backendPid`/`frontendPid` are captured and
written at the point in Task 3.2.1e where both children have reported ready, and are the fields
the startup reconciliation sweep (Task 3.2.1g) reads back on the *next* `startDevStack()` invocation
for the same instance name: it liveness-checks each PID independently (`process.kill(pid, 0)`,
catching `ESRCH` as "dead") and, for any still alive, signals that PID's process group
(SIGTERM, escalating to SIGKILL after a grace period) *before* proceeding to allocate fresh ports —
this is how a hard `SIGKILL` of a prior launcher (which never reaches `teardown()`, and therefore
never fires `SIGINT`/`SIGTERM`/a top-level `finally`) gets its orphaned children actually reaped,
rather than merely detected. A clean shutdown's `teardown()` deletes the whole manifest file, so
its mere presence on the next launch always implies either one or more live orphans or a
stale-but-checkable record — never an ambiguous one.

**`pid` field (retained, narrower scope):** `pid` continues to record the Node `StackLauncher`
process's own PID at the moment the manifest is written. It is kept for its original informational
value (a human `cat`ing the file can see which launcher process produced it, e.g. to correlate
with `ps`/shell job control while the launcher is still running) but is **not** used by the reap
logic — the launcher process's own PID is not the process group ID of either child, so it cannot
be used to signal them. All orphan detection and reaping is driven exclusively by
`backendPid`/`frontendPid`.

**`schemaVersion` field:** added in this revision (bumped to `2`) since the manifest's read-path
semantics changed materially (a reader must now know to look for `backendPid`/`frontendPid`, not
just `pid`). Not required by any current consumer to branch on, but present so a future manifest
format change has a field to gate on rather than needing to infer format from key presence.

### 2. `STAPLER_SQUAD_EXTRA_ORIGINS` env var

A new, comma-separated env var read once in `main.go`, appended to the existing single-origin
`srv.SetOrigins([]string{localOrigin})` call. Only additive — never replaces or widens matching
semantics (no wildcards, no regex); each entry must be an exact origin string
(`http://localhost:<port>` or `http://127.0.0.1:<port>`, scheme `http` or `https`), consistent
with the NFR that no new externally-reachable surface is introduced. This is the minimal
extension to an existing single-purpose mechanism, not a new general-purpose CORS configuration
system.

**Gated on `STAPLER_SQUAD_INSTANCE` being explicitly set — a structural no-op for the default/systemd instance.** `main.go` only reads `STAPLER_SQUAD_EXTRA_ORIGINS` at all when `STAPLER_SQUAD_INSTANCE` is also explicitly set to a non-empty, non-default value (implementation plan Task 1.2.1a). The default/systemd-managed invocation never sets a custom instance name (it relies on the workspace-hash-based default per `.claude/docs/state-isolation.md`), so this gate means a stray `STAPLER_SQUAD_EXTRA_ORIGINS` value left in a shell profile — from a prior `DevStack` session — has no code path by which it can reach `srv.SetOrigins` for the systemd instance, regardless of its contents. This closes pre-mortem.md's Failure #3 (P2): rather than accepting the cross-instance CORS-bleed risk as a known, unmitigated concern (as two prior review rounds had left it), the mechanism is now structurally incapable of affecting the default instance's trusted-origin list.

**This is a concrete implementation requirement, not just an aspirational invariant.**
`s.origins` (`server/server.go:50`) is a single shared, credentialed CORS trust list — consumed by
`middleware.CORSWithOrigins` on both the plain-HTTP path (`server/server.go:663`) and the
remote-access HTTPS path (`server/server.go:811`), and `middleware/cors.go:59-65` sets
`Access-Control-Allow-Credentials: true` for any exact-matched origin. Because of this shared,
credentialed blast radius, the parsing code in `main.go` (implementation plan Task 1.2.1a) MUST
validate every `STAPLER_SQUAD_EXTRA_ORIGINS` entry against `^https?://(localhost|127\.0\.0\.1):\d+$`
before it is ever passed to `srv.SetOrigins`. An entry that fails this check is dropped and logged
as a warning — it must never be silently trusted. The resolved final origin list must also be
logged at startup, so a malformed or unexpectedly-broad value (e.g. inherited from a shell rc file)
is visible to whoever started the process, not just a possible later security surprise.

## Consequences

- Both conventions are dev/test-only; the systemd-managed instance never sets
  `STAPLER_SQUAD_EXTRA_ORIGINS` and never has a `dev-stack.json` manifest, so there is no
  behavior change to the production-style instance. Because `STAPLER_SQUAD_EXTRA_ORIGINS` is
  additionally gated on `STAPLER_SQUAD_INSTANCE` being explicitly set (§2 above), this holds even
  if `STAPLER_SQUAD_EXTRA_ORIGINS` is accidentally present in the systemd instance's environment
  (e.g. a lingering shell-profile export) — the gate, not just convention, prevents it from taking
  effect.
- If a future feature needs a *general* multi-origin CORS policy (not just one dev-stack's
  frontend), this ADR's narrow, comma-separated-env-var mechanism should be revisited rather than
  assumed sufficient — it was sized for "2-5 concurrent local dev stacks," not a general policy
  surface.
