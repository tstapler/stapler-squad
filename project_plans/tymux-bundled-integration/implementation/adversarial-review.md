# Adversarial Review: tymux-bundled-integration (re-review of patched plan)

**Date**: 2026-08-25
**Verdict**: CONCERNS

Re-reviewed `requirements.md`, the updated `implementation/plan.md`, and updated ADR-001/002/003
against the prior BLOCKED review (1 blocker, 5 concerns, 3 minors), cross-checked against the
actual code in this worktree: `session/backend_factory.go`, `session/instance.go`,
`session/create_managed_instance.go`, `server/services/session_service.go`,
`session/tymux/errors.go`, `session/tymux/session.go`, `session/tymux/stream.go`,
`session/tymux/transport.go`, `session/tmux/tmux.go`, `executor/safeexec/safeexec_pdeathsig_*.go`,
`session/ent/schema/session.go`, `proto/session/v1/session.proto`, `daemon/daemon.go`,
`daemon/daemon_unix.go`, `config/config.go`, `pkg/warren/app.go`, `Makefile`.

## Prior findings: verified resolved

All 1 blocker / 5 concerns / 3 minors from the prior review were checked against the actual
patch, not just re-read — genuinely resolved:

1. **Blocker (single call site for supervision)** — RESOLVED. New Story 2.1.3 / Task 2.1.3a wires
   `EnsureDaemonRunning` into `NewProcessManager`'s `case BackendTymux:` branch
   (confirmed at `session/backend_factory.go:74`, matching the plan's cited line). ADR-003 now
   states plainly ("What 'health-check' means here, explicitly") that this is call-before-use at
   two sites (`main.go` startup, `NewProcessManager`'s lazy path), not an ongoing poll loop — no
   phantom background goroutine is claimed anywhere in the plan or ADR. Task 2.1.3c documents the
   mid-run-crash behavior and confirms (with file:line citations that check out —
   `session/tymux/errors.go`'s `classifyRPCError`/`ErrTymuxdUnreachable` are genuinely used at
   `Start`, `RestoreWithWorkDir`, `Close`, `CapturePane`, `Attach`, `Attach.Send`, `ReviveSession`
   in `session/tymux/session.go`/`stream.go`) that a crash surfaces as a wrapped, non-generic
   error rather than a silent hang. See new Concern below, however — the *mechanism* chosen for
   this fix introduces its own new problem.
2. **`resolveDaemonConfig` → `ResolveDaemonConfig` rename** — RESOLVED. Grepped the whole plan and
   all three ADRs: zero occurrences of the lowercase form remain; every reference (Task 1.3.1a/b,
   Task 2.1.3a, Task 2.2.1b, the summary table) consistently uses the exported name.
3. **Ent schema "opaque JSON like Tags" framing** — RESOLVED, not just softened. Task 5.1.1f now
   states flatly that the schema is fully columnar and `backend` needs its own `field.String`,
   with no contingent "verify first" language. Verified against the actual schema
   (`session/ent/schema/session.go`): every simple field (`title`, `program`, `category`,
   `hidden`, `workflow_id`, etc.) is a discrete column; `Tags` is `edge.To`, not JSON;
   `session_artifacts` is the only genuine opaque-JSON-blob field, and the plan now correctly
   calls it a one-off rather than the norm. The stated regen command
   (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`)
   matches `CLAUDE.md`'s required invocation exactly.
4. **Pdeathsig reinvention** — RESOLVED. Task 2.1.2d now calls `safeexec.EnsurePdeathsig(cmd)`
   directly and explicitly rejects writing new `sysprocattr_linux.go`/`sysprocattr_other.go`
   files. Verified `executor/safeexec.EnsurePdeathsig(cmd *exec.Cmd)` exists
   (`safeexec_pdeathsig_linux.go`/`safeexec_pdeathsig_other.go`), sets `Pdeathsig: syscall.SIGKILL`
   on Linux exactly as the plan states, and is already used by three other call sites the plan
   cites correctly. The plan explicitly adopts SIGKILL and explains why it does *not* diverge to
   SIGTERM — no lingering wrong claim.
5. **macOS Gatekeeper spike** — RESOLVED. New Epic 1.0 (Story 1.0.1, Tasks 1.0.1a/b) runs the
   spike before Epic 1.1/1.2's plumbing, with two named fallbacks (ad-hoc codesign vs. macOS-only
   local compilation) if Gatekeeper blocks the binary, and ADR-001's Consequences section is
   updated to point at it.
6. **TYMUX_VERSION/go.mod sync** — RESOLVED, proportionate. Task 1.1.2b adds a one-line doc
   comment above `TYMUX_VERSION` in the Makefile, explicitly reasoned as "not viable as a script
   assertion yet" (upstream tags aren't 1:1 with the Go module version) rather than
   over-engineering enforcement that can't actually work yet.
7. **ADR-001 alternatives gap (vendoring)** — RESOLVED. ADR-001's Alternatives section now has a
   full paragraph on vendoring prebuilt tarballs into git history, with the actual tradeoff
   (removes the availability risk of a single-maintainer repo with no mirror, costs ~14MB/version
   of repo growth) and an explicit "revisit if releases prove unreliable" framing.
8. **Proto field-number staleness** — RESOLVED and re-verified against current code, not just the
   plan's own re-statement: `proto/session/v1/session.proto`'s `CreateSessionRequest` still tops
   out at `confirm_restart_with_live_source = 33` (no drift since the last review), so `34` is
   genuinely the next free number as Task 4.3.1a now states.

## Concerns

- [ ] **New: routing `EnsureDaemonRunning` through `NewProcessManager`'s `BackendTymux` branch
  makes it synchronous on the `CreateSession` RPC path itself, not deferred to the existing async
  "start tmux/the process" goroutine — a real latency regression for exactly the scenario this
  fix exists to close.** Traced the actual call chain: `server/services/session_service.go`'s
  `CreateSession` handler calls `session.CreateManagedInstance` *synchronously* (its own comment
  at the call site says the async goroutine only starts "after" this — "This does NOT start
  tmux/the process; that happens in the async goroutine below"). `CreateManagedInstance`
  (`session/create_managed_instance.go`) calls `NewInstance(opts)` synchronously, which
  (`session/instance.go:912`) calls `NewProcessManager(context.Background(), BackendTmux,
  ProcessManagerOptions{Backend: instance.Backend})` synchronously — all *before* the async
  goroutine that would normally absorb slow work. Once Task 2.1.3a lands, a `BackendTymux`
  session's `NewProcessManager` call blocks on `EnsureDaemonRunning`'s spawn-and-retry loop right
  here. Task 2.1.2c explicitly reuses tmux's own bound constants (8 attempts, 100ms→3s backoff),
  and tmux's own doc comment for those exact constants
  (`session/tmux/tmux.go:595-616`) states "~9.1s worst-case total wait" — but tmux pays that cost
  **once, at process startup**, in `main.go`'s runtime phase, never per-session. The tymux plan
  copies the same worst-case duration but now pays it **synchronously inside every cold-start
  `CreateSession` RPC call**, which is precisely the "operator flips a canary session onto tymux
  without restarting" scenario the original blocker (and this patch) was built to fix — so the
  headline use case now costs up to ~9-11s of blocked RPC time on its first hit, with no
  discussion anywhere in the plan of this tradeoff, no UX/spinner consideration, and no context
  deadline to bound or cancel it (see next point). **Recommendation**: either move the
  `EnsureDaemonRunning` call to the async goroutine already used for "start tmux/the process"
  (mirroring how `initTmuxSession()`/`Start()` defers tmux's own slow work), so `NewProcessManager`
  stays cheap-to-construct the way it is for `BackendTmux` today, or explicitly document and
  accept the synchronous latency tradeoff in the plan/ADR-003 rather than leaving it undiscussed.
- [ ] **`ctx` plumbed into `EnsureDaemonRunning` carries no real deadline/cancellation at any
  current call site.** Task 2.1.3a's own justification is "`NewProcessManager` already accepts a
  `context.Context` first parameter... no signature change needed, just naming it and using it" —
  but every actual non-test call site today (`session/instance.go:912`,
  `session/external_discovery.go:168`, `session/instance_serialization.go:334`,
  `session/instance_tmux.go:134,137`) passes `context.Background()`, not a request-scoped
  context. "Using ctx" is therefore cosmetic unless these call sites are also updated to pass a
  real, bounded context — otherwise a caller that gives up (client disconnects, a higher-level
  timeout fires) cannot cancel the retry loop described in the concern above, and there is no
  stated overall deadline shorter than the ~9-11s worst case. Not a new task is strictly required
  if the first concern above is resolved by deferring to the async goroutine (which already has
  its own cancellation story), but if the synchronous placement is kept, this needs its own fix.
- [ ] **Concurrent cold-start race between multiple `NewProcessManager(..., BackendTymux, ...)`
  calls is not discussed or tested.** Two sessions created concurrently (e.g. via `CreateSession`
  plus one of the Epic 4.4 call sites, or a batch-create path — `session_service.go` already has a
  `go func(i int, ...)` batch-request pattern) could both observe `checkDaemonHealthy() == false`
  and both call `startDaemonAttempt`, racing to bind the same port. The design likely self-heals
  (the loser's spawn fails to bind, its retry loop then finds the winner's daemon healthy via
  reuse detection) but this depends on `tymuxd` exiting cleanly on a bind failure rather than
  leaving a wedged process, and Task 2.1.2f's unit test list has no test for this concurrent-spawn
  case. Cheap to add as a table/integration test alongside the existing `TestEnsureDaemonRunning_*`
  suite once implemented.

## Minors

- `session/backend_factory.go`'s current doc comments already reference an unrelated "Story
  2.1.3" / "UX-9.2" (from a different, already-shipped feature — the doc comment on
  `ErrUnrecognizedBackend` and `NewProcessManager`, unrelated to this plan's own Story 2.1.3
  numbering). Not a defect in this plan, but worth a heads-up for whoever implements Task 2.1.3a:
  don't conflate the two when editing that file's doc comments, and consider disambiguating the
  new Story 2.1.3 references added to this file (e.g. citing the ADR/plan path) to avoid
  compounding the collision.
- Story 2.1.3's Acceptance Criteria ("an already-healthy daemon adds one cheap `ListSessions`
  round-trip (no spawn)") is accurate for the steady state but reads as if that's the only
  relevant case — the cold-start case (the one the Concerns above are about) deserves equal
  billing in the ACs, not just in Epic 2.1's Goal note.
