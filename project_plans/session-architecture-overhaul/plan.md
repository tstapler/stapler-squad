# Combined Implementation Plan: Concurrency + Domain Decomposition

Merges two CLEAN plans into one sequenced roadmap:

| Source plan | Scope | Status |
|---|---|---|
| `project_plans/instance-actor-concurrency/` (IAC) | Actor model + Registry + atomic snapshot | CLEAN (6 review passes), 8 Epics / 32 Stories / 114 Tasks |
| `project_plans/core-domain-decomposition/` (CDD) | Service extraction + tmux normalization + field grouping | CLEAN (4 review passes), 3 Epics / 9 Stories / 25 Tasks |

Total scope: **11 Epics · 41 Stories · 139 Tasks**

For full task breakdowns, ADRs, and rationale see the source plans. This document is a sequencing overlay only — it defines wave ordering and cross-plan gates; it does not duplicate tasks.

---

## Dependency graph

```
Wave A (parallel, no prior gate)
  IAC Epic 1 ─────────────────────────────────┐
  IAC Epic 2 ─────────────────────────────────┤
  IAC Epic 2.5 ───────────────────────────────┤──► Wave B
  CDD Epic 1 (Stories 1.1, 1.3-1.6) ──────────┤
  CDD Epic 2 ─────────────────────────────────┘

Wave B (after ALL of Wave A)
  IAC Epic 3 ──────────────────────────────────► Wave C

Wave C (sequential ATOMICITY GATEs, one-after-one)
  IAC Epic 4 ──► IAC Epic 5 ──────────────────► Wave D

Wave D (parallel, after IAC Epics 4+5 both land)
  CDD Story 1.2 (AutonomousOrchestrationService)
  CDD Epic 3 (InstanceSnapshot field grouping)
  IAC Epic 6 (instance_tmux.go RLock cleanup)
  IAC Epic 7 (stateMutex deletion + field unexporting)
```

---

## Wave A — Foundational (fully parallelizable)

All Wave A work is additive or mechanically safe. PRs in this wave can land in any order relative to each other within a wave.

### IAC Epic 1 — `InstanceSnapshot` struct (additive)

Source: `instance-actor-concurrency/implementation/plan.md` Epic 1

Adds `InstanceSnapshot` and `buildSnapshot()` to `session/instance.go`. No existing code removed. This is the substrate everything else reads from.

**Gate for CDD Epic 3**: CDD Epic 3 is hard-blocked on this landing. Do not start CDD Epic 3 until `InstanceSnapshot` exists in the codebase.

### IAC Epic 2 — Migrate all readers to `snapshot.Load()`

Source: `instance-actor-concurrency/implementation/plan.md` Epic 2

Converts `InstanceToProto`, `ToInstanceData`, `CapacityMonitor`, `ReviewQueuePoller`, `ConnectRPCWebSocketHandler` to read from `atomic.Pointer[InstanceSnapshot].Load()` instead of live struct fields. Writer still exists — no behavior change yet.

### IAC Epic 2.5 — Registry + LiveInstance lifecycle (47 tasks)

Source: `instance-actor-concurrency/implementation/plan.md` Epic 2.5

Type-split: `InstanceData` (free-to-construct, read-only value) vs `LiveInstance` (actor-owning, only obtainable via `Registry.Acquire()` or `Registry.Register()`). Registry injected into all services via `server/dependencies.go`. This is the largest Wave A epic.

**Merge coordination with CDD Epic 1**: Both modify `session_service.go`. IAC 2.5 adds Registry injection to SessionService's constructor; CDD Epic 1 removes method bodies. They touch different zones of the file. If landing in the same week: land CDD Epic 1's method extractions first (simpler diff), then rebase IAC 2.5's constructor injection on top. Do not land both in the same PR.

### CDD Epic 1 — Extract service clusters (Stories 1.1, 1.3-1.6 only)

Source: `core-domain-decomposition/implementation/plan.md` Epic 1, **excluding Story 1.2**

| Story | Action |
|---|---|
| 1.1 | Extract `CheckpointService` from `session_service.go` |
| 1.3 | Audit and inline `Set*/Get*` wiring setters |
| 1.4 | Delegate `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` to existing services |
| 1.5 | Extract `TerminalService` (`GetTerminalSnapshot`/`WriteToSession`) |
| 1.6 | Extract `FeatureFlagService` |

Story 1.2 (`AutonomousOrchestrationService`) is deferred to **Wave D** — it moves `onAutonomousDriverComplete`/`buildTurnCallback`, the same methods IAC Epic 5 converts to actor commands. Landing both simultaneously would silently drop one side's changes on merge.

### CDD Epic 2 — Tmux normalization at `Preview()`

Source: `core-domain-decomposition/implementation/plan.md` Epic 2

Normalize at `instance_terminal.go`'s `Preview()` single return point — route both branches (`ctrl.GetRecentOutput()` and `pm().CapturePaneContent()`) through `PTYNormalizer{}.Normalize(...)` before returning. Fully independent of IAC.

---

## Wave B — Actor plumbing proof (after all Wave A lands)

### IAC Epic 3 — Prove actor goroutine plumbing

Source: `instance-actor-concurrency/implementation/plan.md` Epic 3

Spins up one real actor goroutine per `LiveInstance`, wires the mailbox channel, verifies `goleak` shows no leaked goroutines, confirms stop signal via `context.WithCancel` (ADR-029 pattern). No state-machine commands yet — the goroutine just loops and processes a `noop` command.

**Gate**: cannot start until IAC Epics 1+2+2.5 are all merged (the actor needs the snapshot infrastructure and the Registry to exist).

---

## Wave C — ATOMICITY GATEs (sequential, one-after-one)

These two epics MUST land in order. Do not attempt them in parallel. Each is an atomic PR — no partial landing.

### IAC Epic 4 — State-machine core (ATOMICITY GATE)

Source: `instance-actor-concurrency/implementation/plan.md` Epic 4

Converts `transitionTo`'s guard/hook state machine, `UpdatePRStatus`'s 8-field write, and checkpoint creation to single commands processed atomically by the actor. The ATOMICITY GATE label means: all tasks in this epic land in one PR, or none do — a partial landing leaves the codebase in an inconsistent state where some state transitions are actor-routed and others are still direct writes.

**Gate**: only start after IAC Epic 3 (actor goroutine) is merged and passing.

### IAC Epic 5 — `session_service.go` writer cutover (ATOMICITY GATE)

Source: `instance-actor-concurrency/implementation/plan.md` Epic 5

Converts all remaining `session_service.go` direct writes to actor commands, including `AutonomousDriver`'s background goroutine callbacks (`onAutonomousDriverComplete`, `buildTurnCallback` — see Story 5.3 Task 5.3b). Same ATOMICITY GATE rule: one PR, all-or-nothing.

**Gate**: only start after IAC Epic 4 is merged and passing.

**Enables CDD Story 1.2 + CDD Epic 3**: once this lands, Wave D work is safe to begin.

---

## Wave D — Cleanup (parallel, after IAC Epics 4+5 both land)

All Wave D items are independent of each other.

### CDD Story 1.2 — `AutonomousOrchestrationService` (coordination gate)

Source: `core-domain-decomposition/implementation/plan.md` Epic 1 Story 1.2

**Before starting**: confirm IAC Epic 5 has landed. Its Task 5.3b already converted `onAutonomousDriverComplete`/`buildTurnCallback` to actor commands in `session_service.go`. Story 1.2 then extracts the actor-routed versions into `AutonomousOrchestrationService` — no duplicate conversion, no merge conflict.

### CDD Epic 3 — `InstanceSnapshot` field grouping

Source: `core-domain-decomposition/implementation/plan.md` Epic 3

Groups `InstanceSnapshot` fields into named sub-structs (PR/checkpoint/autonomous clusters). Safe now that IAC Epics 4+5 have stabilized the struct's field set.

**Note**: Tasks 3.1a and 3.1b also touch fields that CDD Story 1.2 (AutonomousOrchestrationService) cares about. If both are in flight simultaneously, land Story 1.2 first (it renames methods, not fields), then rebase Epic 3 on top.

### IAC Epic 6 — `instance_tmux.go` RLock-across-I/O

Source: `instance-actor-concurrency/implementation/plan.md` Epic 6

Converts 10 sites that hold `RLock()` across tmux subprocess I/O to instead read a snapshot field for the precondition check, then perform I/O without holding anything. These are performance cleanups, not correctness fixes (the actor model already eliminates the races).

### IAC Epic 7 — `stateMutex` deletion + field unexporting

Source: `instance-actor-concurrency/implementation/plan.md` Epic 7 (including Task 7.1e)

Deletes `stateMutex` from `Instance` entirely. Exports any field that was only kept exported for the old lock-guarded access pattern; tightens unexported-field discipline per the `type-driven-design` skill audit (5 findings A-E from `research/type-driven-audit.md`).

**This is the finish line**: once this PR merges, `go build ./...` with no references to `stateMutex` anywhere in the codebase proves the migration is complete.

---

## Cross-cutting constraints

### Files most at risk of merge conflict

| File | Touched by |
|---|---|
| `session/instance.go` | IAC Epics 1, 2.5, 3, 4, 5, 7 |
| `server/services/session_service.go` | IAC Epic 2.5, IAC Epic 5, CDD Epics 1 (all stories) |
| `session/instance_terminal.go` | CDD Epic 2, IAC Epic 6 (if RLock sites are in this file) |
| `server/dependencies.go` | IAC Epic 2.5 (Registry injection) |

When two epics both touch a file, land the mechanically simpler one first and rebase.

### `go generate` / proto: no changes required

Neither plan modifies proto definitions. Run `make build` (not `make generate-proto`) after each Epic lands.

### Test verification per epic

Each Epic must pass before the next begins:
```bash
make build && make test
go test -race ./session/... ./server/services/...
```

Run `go test -race` before ANY change to baseline existing races, and again after each Wave C epic to confirm races are eliminated, not added.

---

## Totals

| Wave | Epics | Stories | Tasks | Parallelizable |
|---|---|---|---|---|
| A | 5 (IAC 1,2,2.5 + CDD 1+2) | 22 | 99 | Yes — all items parallel |
| B | 1 (IAC 3) | 3 | 10 | N/A (single epic) |
| C | 2 (IAC 4+5) | 5 | 22 | No — strictly sequential |
| D | 4 (CDD 1.2, CDD 3, IAC 6+7) | 11 | 18 | Yes — all items parallel |
| **Total** | **11** | **41** | **139** | |

Critical path (maximum blocking length): Wave A → Wave B → IAC Epic 4 → IAC Epic 5 → Wave D.
Wave A's longest epic (IAC 2.5, 47 tasks) drives the critical path into Wave B.
