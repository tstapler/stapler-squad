# ADR-002: The tymux rollout gate wraps `RegisterBackendProvider`, not a streamhub-style live per-call resolver

**Status**: Accepted
**Date**: 2026-08-25

## Context

Two rollout-safety mechanisms already exist in this codebase and are not interchangeable:

1. **`session.ProcessManagerBackend` / `RegisterBackendProvider`** (`session/backend_factory.go`)
   — a package-level var set **once**, at process startup, from `cfg.ProcessManagerBackend`
   (a plain string, no env var, no gate, no audit trail today —
   `main.go:167-175`). Read synchronously by `NewProcessManager` at each session's
   construction.
2. **`STAPLER_SQUAD_USE_STREAM_HUB`** (`server/services/connectrpc_websocket.go`'s
   `useStreamHub()`, `config.ResolveGlobalStreamHubDefault`) — read **live, via `os.Getenv`,
   on every resolution call**, gated by `RollbackRehearsalCompletedAt`
   (`config/config.go:435-471`), resolved once per *tmux session* (not once per process) via
   `StreamOwnershipLock.Resolve`'s sticky-cache.

Requirements.md's open question: does "the global flag" for tymux become the rehearsal-gated
version of `process_manager_backend`, or a new, separate gate layered on top that then decides
whether `process_manager_backend: tymux` is even honored? The two mechanisms differ
structurally because of *what* they gate: streamhub's flag governs a per-connection choice made
against an already-running tmux session (an in-process object swap); the backend selector
governs which `ProcessManager` implementation gets constructed once, at
`NewInstance`/`NewProcessManager` time, for a specific session. `package session` also cannot
import `server/services` (where streamhub's resolver lives) — a one-way dependency ADR-003
(streamhub's own ADR) establishes — so importing streamhub's live-resolution model wholesale
isn't even structurally available to the backend selector without duplicating
`effectiveStreamHubFlag()`'s own workaround (`session/instance_tmux.go:876`, duplicated because
of exactly this constraint).

## Decision

**Bolt the rehearsal-gate treatment directly onto the existing `RegisterBackendProvider` call
site in `main.go`, evaluated once at startup — not a new live, per-call resolver.**

New pieces, mirroring streamhub's shape exactly but evaluated once instead of per-call:

- `config.go`: `TymuxRollbackRehearsalCompletedAt *time.Time` (mirrors
  `RollbackRehearsalCompletedAt`, kept as its own distinct field — streamhub's rollback and
  tymux's rollback are not the same rehearsal, see ADR-003's rollback-semantics note).
- `config.go`: `ResolveGlobalTymuxDefault(cfg *Config, requested bool) (bool, error)` — same
  contract as `ResolveGlobalStreamHubDefault`: `requested == false` always passes; `requested ==
  true` is refused with `ErrTymuxRollbackRehearsalNotCompleted` unless
  `TymuxRollbackRehearsalCompletedAt` is set.
- `config.go`: `RecordTymuxRollbackRehearsalCompleted()` (mirrors
  `RecordRollbackRehearsalCompleted`).
- `main.go`: the existing block (lines 167-175) becomes a new helper,
  `resolveStartupBackend(cfg *config.Config, tymuxEnvRequested bool) (session.ProcessManagerBackend, error)`,
  called once before `session.RegisterBackendProvider`:

  ```go
  backend := session.ProcessManagerBackend(cfg.ProcessManagerBackend)
  if backend == "" {
      backend = session.BackendTmux
  }
  tymuxRequested := backend == session.BackendTymux || os.Getenv("STAPLER_SQUAD_USE_TYMUX") == "true"
  tymuxEffective, err := config.ResolveGlobalTymuxDefault(cfg, tymuxRequested)
  if err != nil {
      log.Warn("tymux: global default requested but rollback rehearsal not completed; falling back to tmux", "err", err)
  }
  if tymuxEffective {
      backend = session.BackendTymux
  } else if backend == session.BackendTymux {
      backend = session.BackendTmux
  }
  session.RegisterBackendProvider(backend)
  ```

  `RegisterBackendProvider` itself is **not modified** — it stays the dumb, mutex-guarded setter
  it is today. The gate lives entirely in the one caller that decides what to pass it.

The critical property this closes (flagged in `research/pitfalls.md` §3): **hand-editing
`process_manager_backend: "tymux"` directly in `config.json` cannot bypass the gate**, because
that value feeds `tymuxRequested` the same as the env var would, and both paths go through the
identical `ResolveGlobalTymuxDefault` call. There is exactly one source of truth for "is tymux
the effective global default," not two independently-settable knobs that can disagree.

## Consequences

- Restart-only, matching `RegisterBackendProvider`'s existing set-once nature and streamhub's
  own precedent ("env-var-gated and requires a process restart by design" —
  `stream_hub_rollout_service.go:21-23`) — no hot-reload, no risk of splitting an in-flight
  session's backend mid-life from a live config edit.
- `resolveStartupBackend` is a plain function (not a method, not wrapped in an interface),
  independently unit-testable without spinning up `main`'s full `RunE` — addresses the fact that
  `main.go`'s startup block was previously untested inline logic.
- The per-session override (Phase 4 of the implementation plan, mirroring
  `StreamHubSessionOverrides`) is **not** gated by this rehearsal check, exactly like
  streamhub's own per-session override — Story 3.3.1's doc comment on
  `ResolveGlobalStreamHubDefault` is explicit that the gate applies only to the *global*
  default, and this ADR carries that property forward unchanged.

## Alternatives Considered

**Import streamhub's live per-call resolution model wholesale** (a `resolveTymuxBackendLive()`
consulted by `NewProcessManager` on every call, mirroring `useStreamHub()`). Rejected:
structurally mismatched — `NewProcessManager` already resolves its backend once, at
construction, via `opts.Backend` → `getSelectedBackend()` → `defaultBackend` → `BackendTmux`;
turning that into a live per-call re-read would require rebuilding sticky-resolution machinery
(`StreamOwnershipLock`-equivalent) that already exists for a different purpose one layer up
(`Instance.Backend`'s persistence, once Phase 5 wires it — see the implementation plan), making
this the "unjustified generic" duplication the interface-pollution checklist warns against:
solving a problem `ProcessManagerOptions.Backend` already solves, just less directly.

**Two independent knobs** (a new on/off gate that separately decides whether
`process_manager_backend: tymux` is honored, leaving the string field otherwise untouched).
Rejected per `research/pitfalls.md` §3: this is exactly the shape that lets an operator
hand-edit `config.json` around the gate, providing no actual protection.
