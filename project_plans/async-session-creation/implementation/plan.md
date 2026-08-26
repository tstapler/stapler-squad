# Implementation Plan: async-session-creation

**Feature**: Restructure `CreateSession` so every session type creates its
`ManagedInstance` and returns to the RPC caller before any slow resolution
work (GitHub clone, alias/branch inference, worktree/tmux startup), which
then runs in a fenced, observable background pipeline with cancel, retry,
and stale-timeout handling.
**Date**: 2026-08-26
**Status**: Ready for implementation
**ADRs**: ADR-001 (`SESSION_STATUS_FAILED`, non-terminal), ADR-002 (creation-epoch
fencing token), ADR-003 (linked-root-span for background goroutine tracing)

---

## Domain Glossary
*(Ubiquitous language — use these exact names in code, tests, and PR description)*

| Term | Definition | Notes |
|------|-----------|-------|
| `CreateSession` synchronous prefix | The portion of the RPC handler that still runs before the RPC returns: fast-fail validation, fork dispatch, config/remote/restart-source resolution, title-uniqueness check, instance construction, `storage.SaveInstances`, `SessionCreatedEvent` publish. | Must stay small and reviewed as one unit — see `research/architecture.md` §7's "synchronous prefix becomes the new single point of total failure." |
| Background Resolution Pipeline | The single tracked goroutine (`s.trackCleanup`) that runs, in order, GitHub-URL resolution → alias/default resolution → branch/session-type inference → worktree setup → tmux startup, publishing `creation_progress` between phases and making exactly one terminal status write at the end. | Merges today's synchronous L1912-1938 GitHub-resolution logic with today's existing L2397-2439 worktree/tmux tail into one pipeline. |
| Creation Phase | One named step of the Background Resolution Pipeline (`ResolvingGitHubURL`, `CloningRepository`, `ResolvingDefaults`, `InferringBranch`, `SettingUpWorktree`, `StartingSession`). | Each phase transition calls `SetCreationProgress` + publishes `SessionUpdatedEvent{"creation_progress"}`. |
| `creationEpoch` | A `uint64` field on `session.Instance`, actor-owned, incremented exactly on cancel-issued and on retry-started. A background writer captures it at spawn and must present the same value to win the terminal write. | See ADR-002. Not used for non-terminal (`SetCreationProgress`-only) writes. |
| `TryForceStatusIfEpoch(epoch, status, failureReason) bool` | The one atomic, actor-routed compound check-and-set: applies a terminal status+failure-reason write and returns `true` only if `epoch` still matches `creationEpoch`; otherwise no-ops and returns `false`. | The single serialization point for every terminal write (success, resolution failure, startup failure, stale-timeout). Internally the only caller of `setFailureReasonLocked` — there is no public `SetFailureReason` (see ADR-002). |
| `TryStartRetry() (newEpoch uint64, started bool)` | Atomic, actor-routed compound check-and-set for *starting* a retry: no-ops (`started=false`) unless `Status == Failed`, otherwise bumps `creationEpoch`, resets to `Creating` with fresh `creation_progress`, and returns the new epoch. | Closes the double-click-retry race — see ADR-002's addendum. `RetrySessionCreation`'s first state-mutating call, before cleanup or pipeline spawn. |
| `SESSION_STATUS_FAILED` | New enum value (`= 11`) on `SessionStatus` representing "this session's creation never completed" — distinct from `CRASHED` (post-Running failure) and non-terminal (unlike `STOPPED`). | See ADR-001. |
| `FailureReason` | A short, distinct enum/string stored on the instance alongside `SESSION_STATUS_FAILED`, one of `GitHubResolutionError`, `StartupError`, `Stale`, `Cancelled`. Rendered as different copy on the session card and in the toast. Written only inside `TryForceStatusIfEpoch`'s command closure — no independent public setter. | Cancelled is not itself terminal-Failed (see `Cancelled` outcome below) but shares the reason vocabulary for logging consistency. |
| `CreationProgressUpdatedAt` | A `time.Time` field on `session.Instance`, actor-owned, bumped by the same locked helper `SetCreationProgress` already calls, and persisted via `storage.UpdateInstance` at each phase transition (not only at the terminal write) — `SaveInstances` is the wrong call here because it silently skips any instance where `!inst.Started()`, and `started` isn't set until the pipeline's final (worktree/tmux) phase; `UpdateInstance` calls `s.repo.Update` directly with no such gate (`session/storage.go:546-548`). | The Stale-Creation Sweeper's actual staleness clock — survives a process restart, unlike in-process elapsed time. Falls back to `CreatedAt` only for an instance that never received a single progress update (Task 4.1.2f). |
| Cancel-in-progress | The user action (and its RPC, `CancelSessionCreation`) that stops a `Creating` session's background pipeline: bumps `creationEpoch`, calls the instance's stored background `context.CancelFunc`, then runs idempotent cleanup and removes the instance. | Distinct outcome from `SESSION_STATUS_FAILED` — a cancelled session is deleted, not left in a Failed card (see UX research §4: "a user-initiated cancel and a system-detected failure should never look identical"). |
| Retry-in-progress | The user action (and its RPC, `RetrySessionCreation`) on a `Failed` session: calls `TryStartRetry()` (validate-`Failed`+bump-`creationEpoch`+reset-to-`Creating`, atomically), then runs idempotent cleanup, then re-spawns the Background Resolution Pipeline against the **same** instance ID/storage row. | Never publishes a second `SessionCreatedEvent` — only `SessionUpdatedEvent`s. A second concurrent retry call's `TryStartRetry()` deterministically returns `started=false` — see ADR-002's addendum. |
| Idempotent Creation Cleanup | The shared cleanup primitive (extended from `DeleteSession`'s existing cleanup path) that removes a clone/worktree directory and kills any tmux session for an instance, safe to call whether zero, some, or all of those resources actually exist. | Shared by cancel, retry (pre-resolution step), and `DeleteSession` itself. |
| Stale-Creation Sweeper | New ticker-driven background type (`StaleCreationSweeper`, structural sibling of `StaleSessionNotifier`) that scans `Creating`-status instances and flips any whose persisted `CreationProgressUpdatedAt` (falling back to `CreatedAt` if never updated) exceeds a configurable threshold to `SESSION_STATUS_FAILED` with `FailureReason = Stale`. | Threshold: `CreationStaleConfig.ThresholdMinutesOrDefault()`, default 10, config-overridable — mirrors `config.StaleSessionConfig`'s shape. |
| Background Resolution Context | The `context.Context` the pipeline actually runs against: `context.WithTimeout(context.WithoutCancel(rpcCtx), maxCreationResolutionTimeout)`. Never a direct derivative of the RPC's own `ctx` beyond `WithoutCancel`. | See `research/pitfalls.md` §2 — the single most common bug class in this refactor shape. |
| `StartLinkedBackgroundSpan` | New `telemetry` helper producing a new-root span linked (via `trace.WithLinks`) to the RPC's trace, used once per Background Resolution Pipeline invocation. | See ADR-003. |
| `session.creation.outcome` | New OTel counter, one increment per terminal pipeline outcome, attributed by `outcome ∈ {success, failed, stale, cancelled}`. | |
| `session.creation.duration_ms` | New OTel histogram of pipeline wall-clock duration, same `outcome` attribute. | |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall async-creation architecture | Linear phase pipeline inside one goroutine, running within the existing actor/FSM infrastructure | Extending `session/instance_state.go`'s existing FSM + `trackCleanup` (PoEAA: closest to Transaction Script per phase, orchestrated, not a Domain Model rewrite) | (1) Event sourcing; (2) Saga/multi-phase orchestrator framework | (1) No event-store/replay model exists anywhere in this codebase — `events.EventBus` is pub/sub fan-out, not a source of truth; would be a second persistence paradigm for no benefit. (2) A saga's defining feature (independently schedulable/resumable steps with a coordinator) is unneeded at this project's scale (single-process, single-instance, no distributed coordination, explicitly out of scope) — a single ordered function with one `recover()` and one cleanup path is sufficient and matches this repo's existing precedent (`research/architecture.md` §1). |
| Terminal status write serialization | Fencing token (`creationEpoch` + one atomic actor-routed `TryForceStatusIfEpoch`) | GoF Behavioral: Guarded Suspension / State pattern variant; standard distributed-systems fencing-token technique | (1) Instance-scoped `sync.Mutex` outside the actor; (2) "wait for goroutine to fully exit" via `WaitGroup` before allowing cancel/retry | (1) The actor mailbox already is the instance's serialization point — a second independent lock is a competing mechanism for the same data (interface-pollution-checklist's over-engineering concern, generalized to concurrency primitives). (2) Waiting on a wedged clone subprocess could block cancel/retry for the full timeout window; a fencing token lets cancel/retry return immediately while guaranteeing the stale writer's eventual write is a no-op. See ADR-002. |
| Background goroutine lifecycle | Extend `trackCleanup`/`deleteCleanupWG` (existing) | This repo's own prior fix for a real goroutine-leak test flake (`session_service.go:2390-2396`) | Bare `go func()`; `golang.org/x/sync/errgroup` | Bare goroutine: already caused a documented flake in this exact codebase, no shutdown-await guarantee. `errgroup`: designed for bounded fan-out with joint `Wait()` and first-error-cancels-rest semantics; this is one long-lived goroutine per creation reporting via mutation+events, not a blocking multi-goroutine join — doesn't fit the shape any better than a bare goroutine (`research/build-vs-buy.md` §1). |
| Background-goroutine context lifetime | `context.WithTimeout(context.WithoutCancel(rpcCtx), maxCreationResolutionTimeout)` + stored per-instance `context.CancelFunc` | Stdlib `context.WithoutCancel` (Go 1.21+), current OTel-Go community idiom for "outlives the request, keeps trace values" | Plain `context.Background()` (no request-scoped values) | This repo's existing convention is `context.Background()` for detached work, but ADR-003's trace-linking need means the background span must be able to read the RPC's trace context to link to it — `context.WithoutCancel(ctx)` preserves that value while still fully detaching cancellation, which `context.Background()` cannot do without re-deriving the trace ID by hand. |
| Tracing span for the pipeline | New-root span + `trace.WithLinks` (`StartLinkedBackgroundSpan`) | OpenTelemetry spec's Link primitive (`opentelemetry.io/docs/specs/otel/trace/api/#link`) | Child span of the RPC's span (`trace.ContextWithSpan`) | Datadog (this repo's stated APM target) can render a trace as "complete" once its root span closes; a child span still running after a now-fast RPC root span closes risks display glitches. See ADR-003. |
| Stale-creation detection | New `StaleCreationSweeper`, structural sibling of `StaleSessionNotifier` (ticker + edge-triggered dedup map + config threshold) | This repo's own established pattern — 3 existing independently-tuned sweepers (`StaleSessionNotifier`, review-queue 5-min, backlog 2-hr) | `robfig/cron/v3` (already a dependency) | cron is built for calendar-style scheduling, not "poll in-memory/persisted state on an interval, fire a side effect on threshold crossing" — would introduce a second scheduling idiom alongside 3 existing ticker-based sweepers for the same kind of check (`research/build-vs-buy.md` §1). |
| Status/FSM extension | Add `Failed` to `session/instance_state.go`'s `transitionIndex` as a first-class transition (`Creating→Failed`, `Failed→Creating`) | This repo's existing FSM table (PoEAA: State pattern already in place) | `github.com/looplab/fsm` | Would require translating this repo's `int`-backed `Status`+proto mapping into fsm's string-event vocabulary, duplicating guard/callback logic and losing the existing actor-mailbox lock-discipline documentation — strictly more work than extending the table already in place (`research/build-vs-buy.md` §1). |
| Cancel-in-progress cleanup | Extend/generalize `DeleteSession`'s existing cleanup ordering (stop-before-destroy, idempotent-per-step) into a shared `cleanupPartialCreation(instance)` helper called by cancel, retry, and `DeleteSession` | PoEAA: Service Layer method reused across 3 call sites rather than duplicated | A second, divergent cleanup routine written fresh for cancel | `DeleteSession` already solves "the resource might not be fully live yet" (`FindLiveInstance`-before-`removeFromAllPollers` ordering, tolerant of already-absent resources) — reusing it avoids re-deriving the same edge cases `research/pitfalls.md` §5 already names. |
| Retry/Cancel RPC surface | Two new, narrow RPC methods (`RetrySessionCreation`, `CancelSessionCreation`) rather than overloading `DeleteSession`/`RestartSession` | PoEAA: thin Service Layer methods, one per distinct use case | Overload `DeleteSession` with a "cancel-only" flag; overload `RestartSession` for retry | `RestartSession` creates a *new* session lineage from a *finished* one — semantically wrong for "resume resolution on the same never-finished instance" (`research/features.md` §7). Overloading `DeleteSession` with a boolean would create a same-typed-parameter-pile / hidden-mode smell (`.claude/rules/primitive-obsession-checklist.md`'s spirit) for two operations with different preconditions (`Creating` vs. `Failed`) and different postconditions (row removed vs. row reset). |
| Frontend optimistic state | Extend existing Redux + `WatchSessions`-stream-dispatch pattern in `useSessionService.ts` | This repo's established pattern (`research/stack.md` §c) | React 19's `useOptimistic` hook | `useOptimistic` is designed around a single async transition tied to one component's local state; a session card outlives the dialog that created it and is viewed from multiple places (session list, backlog links) — Redux+stream-dispatch already generalizes correctly across those consumers; adding `useOptimistic` would be a second, inconsistent state mechanism for the same data. |
| Failed-state visual token | New, distinct CSS token `statusCreationFailed` (not reused `statusCrashed`) | UX research recommendation (`research/ux.md` §6.1) | Reuse `statusCrashed` styling | Crashed-after-running and failed-before-running are different enough states that a future design pass may want to differentiate visually; reusing the token now would make that harder to discover later, and reusing a token across two semantically distinct enum values invites the same "silent conflation" ADR-001 was written to avoid at the backend layer. |

---

## Migration Plan

No database schema migration in the `ent` sense — `session/ent/schema` gains
new hand-written fields (`creationEpoch`, `FailureReason`,
`CreationProgressUpdatedAt time.Time`) per the `--feature sql/upsert`
generation constraint from `CLAUDE.md`. `CreationProgressUpdatedAt` is the
field the Stale-Creation Sweeper (Epic 4.1) actually reads — unlike
`CreationProgress` itself (still `json:"-"`, still never persisted, still
event-bus-only), this timestamp is bumped by the same locked helper
`SetCreationProgress` calls and is persisted via `storage.UpdateInstance`
(not `SaveInstances`, which silently no-ops for any instance whose
`Started()` is still false — true for every pipeline phase before the final
worktree/tmux phase, see Task 2.2.2c-2) at each phase transition, so it
survives a process restart, per requirements'
"must use a persisted created-at timestamp, not in-process elapsed time"
constraint. No backfill needed: existing persisted instances have no
in-flight creation state to migrate (a `Creating`-status instance from
before this deploy is, by definition, either already `Active`/`Stopped`, or
has no `CreationProgressUpdatedAt` value yet and falls back to `CreatedAt`
for the sweeper's staleness check, or will be picked up by the new
Stale-Creation Sweeper on first tick after upgrade and correctly flipped to
`Failed`/`Stale` — this is the intended, not accidental, behavior for any
row genuinely orphaned across the deploy boundary).

`proto/session/v1/types.proto`'s new `SESSION_STATUS_FAILED = 11` is
wire-compatible (additive) — no proto migration tooling needed, run
`make proto-gen` once added.

## Observability Plan

- **Logs**: one `log.Info`/`log.Error` per Creation Phase transition and at
  the terminal write, following `server/services/session_service.go`'s
  existing `log.Info("[CreateSession] ...", "session", ..., "err", ...)`
  convention — session ID, phase name, per-phase duration, and (on failure)
  `FailureReason`.
- **Metrics**: `session.creation.outcome` (`Int64Counter`, attributes:
  `outcome`) and `session.creation.duration_ms` (`Float64Histogram`,
  attributes: `outcome`), both via `telemetry.GetMeter()` — see Domain
  Glossary. Emitted once per terminal write, at the same call site as the
  terminal log line and the `TryForceStatusIfEpoch` call, so the three never
  drift out of sync.
- **Alerts**: none — explicitly out of scope per requirements.md (local dev
  tool, no oncall). The metric is for local `--profile`/OTEL-pipeline
  debugging only.
- **Tracing**: `telemetry.StartLinkedBackgroundSpan` per pipeline invocation
  (ADR-003), with `span.AddEvent(...)` per phase transition and
  `span.SetStatus(codes.Error, ...)` on failure.

## Risk Control

- **Feature flag**: none — explicit user decision (requirements.md
  Constraints). This is the single highest-risk aspect of this project:
  every one of the 7 session-creation touchpoints
  (`.claude/docs/session-creation-registry.md`) and all 7 session-creation
  modes (directory, one-off, restart, fork, alias, autonomous, remote) now
  depend on the same shared pipeline skeleton with no way to roll out to a
  subset or fall back to the old path short of a full revert. Phase 4
  (pre-mortem, run via `sdd:4-validate`) and Phase 6 (verification) must
  both explicitly re-verify this project's own Epic 6.1 checklist against
  every touchpoint/mode before merge — **the reviewer running Phase 4's
  pre-mortem should treat "no flag" as the top adversarial-scrutiny target**,
  specifically probing: (a) can the shared pre-branch skeleton (synchronous
  prefix + pipeline dispatch scaffold) be kept small enough to review
  exhaustively, and (b) is there a test that exercises all 7 modes through
  the new path in one CI run so a regression in one mode can't ship unnoticed
  alongside passing tests for the other 6.
- **Rollback procedure**: `git revert` of the merge commit(s) + redeploy
  (`make install-service`). No data migration to undo (see Migration Plan)
  — a revert returns `CreateSession` to fully synchronous behavior and any
  `Failed`/`Creating` rows left over are cosmetic (harmless orphaned status
  values the old code never reads).
- **Staged rollout**: none possible (single binary, no flag) — mitigated
  entirely by pre-merge verification breadth (Phase 6, Epic 6.1) rather than
  a staged production rollout.

## Unresolved Questions

- [ ] Exact copy/wording for each `FailureReason` variant's user-facing
  message (toast text + card text) — blocks Story 5.3 — owner: Tyler
  (product-decision-shaped, not a technical blocker; a reasonable default
  is drafted in Story 5.3's acceptance criteria and can ship as-is if no
  response is needed before Phase 5).
- [ ] Whether Cancel needs a two-step confirmation (per `research/ux.md` §3's
  note that this is "a product decision, not purely an a11y one") — blocks
  Story 5.4 — owner: Tyler. Plan defaults to single-click cancel (low
  "damage" — an aborted clone, not data loss) unless told otherwise.
- [ ] `maxCreationResolutionTimeout`'s exact value (the background pipeline's
  own upper bound, distinct from the Stale-Creation Sweeper's threshold) —
  blocks Story 2.2 — owner: implementer, resolved empirically in Story 2.2
  itself by instrumenting a real GHE clone against `github.netflix.net` and
  picking a value with headroom (recommendation: 10 minutes, matching the
  Stale-Creation Sweeper's default so the two backstops agree, per
  `research/pitfalls.md` §2's note that the two must be consistent).

## Dependency Visualization

```
Phase 1: Foundation (status enum, FSM, epoch primitive, telemetry helper)
   |
   |-- Epic 1.1: SESSION_STATUS_FAILED (proto + FSM + adapters + CreationProgressUpdatedAt)
   |-- Epic 1.2: creationEpoch + TryForceStatusIfEpoch + TryStartRetry
   |-- Epic 1.3: StartLinkedBackgroundSpan + creation metrics
   v
Phase 2: Backend CreateSession restructure
   |
   |-- Epic 2.1: Synchronous prefix (instance created+published before resolution)
   |-- Epic 2.2: Background Resolution Pipeline (merged GitHub + worktree/tmux)
   v
Phase 3: Cancel & Retry RPCs  ------------------\
   |                                              \
   |-- Epic 3.1: Idempotent Creation Cleanup       \
   |-- Epic 3.2: CancelSessionCreation RPC          +--> both depend on Phase 1 + Phase 2
   |-- Epic 3.3: RetrySessionCreation RPC          /
   v                                              /
Phase 4: Stale-Creation Sweeper  -----------------
   |
   |-- Epic 4.1: CreationStaleConfig + StaleCreationSweeper
   v
Phase 5: Frontend (depends on Phase 1-4 backend surface existing)
   |
   |-- Epic 5.1: Omnibar early-close (mostly free once RPC is fast)
   |-- Epic 5.2: SessionCard Failed-state rendering
   |-- Epic 5.3: Failure toast integration
   |-- Epic 5.4: Retry/Cancel buttons on the card
   v
Phase 6: Registry verification, cross-cutting tests, docs
   |
   |-- Epic 6.1: 7-touchpoint / 7-session-type re-verification
   |-- Epic 6.2: Race/leak tests (-race, goleak, repeated create/fail/retry)
   |-- Epic 6.3: Registry + doc updates (make registry-generate, CLAUDE.md refs)
```

---

## Phase 1: Foundation

### Epic 1.1: `SESSION_STATUS_FAILED` status value

**Goal**: A new, non-terminal `Failed` status exists end-to-end (proto → Go
FSM → adapters → frontend enum), with no exhaustive switch left un-updated.

#### Story 1.1.1: Add the proto enum value
**As a** backend developer, **I want** `SESSION_STATUS_FAILED` in the wire
protocol, **so that** every layer above proto has a value to map to.
**Acceptance Criteria**:
- `SessionStatus` gains `SESSION_STATUS_FAILED = 11` with a doc comment
  stating it is non-terminal and distinct from `CRASHED`.
  - *Given* `proto/session/v1/types.proto`'s `SessionStatus` enum, *When*
    `make proto-gen` runs after the edit, *Then* generated Go/TS code exposes
    `SessionStatus.FAILED` (or equivalent) with wire value `11` and no
    `allow_alias` conflict.
**Files**: `proto/session/v1/types.proto`

##### Task 1.1.1a: Add the enum value + doc comment (~3 min)
- Add `SESSION_STATUS_FAILED = 11;` after the existing `SESSION_STATUS_CRASHED = 10;` line (`types.proto:392`), with a doc comment per ADR-001.
- Files: `proto/session/v1/types.proto`

##### Task 1.1.1b: Regenerate and confirm build (~2 min)
- Run `make proto-gen`, then `go build ./...` and `cd web-app && npx tsc --noEmit` to confirm the new value compiles through both stacks.
- Files: generated output only (not committed per repo convention — confirm `.gitignore` still excludes it)

#### Story 1.1.2: Extend the Go FSM with `Failed` transitions
**As a** backend developer, **I want** `Creating→Failed` and `Failed→Creating`
to be legal, validated transitions, **so that** retry and pipeline-failure
paths use the real FSM instead of only the `ForceStatus` escape hatch.
**Acceptance Criteria**:
- `session.Status` gains a `Failed` constant; `transitionIndex` gains both
  new transition entries.
  - *Given* an `Instance` with `Status == Creating`, *When*
    `instance.TransitionTo(Failed)` is called, *Then* it succeeds and the
    instance's `Status` reads `Failed`.
  - *Given* an `Instance` with `Status == Failed`, *When*
    `instance.TransitionTo(Creating)` is called, *Then* it succeeds
    (this is the transition retry uses).
  - *Given* an `Instance` with `Status == Stopped`, *When*
    `instance.TransitionTo(Failed)` is attempted, *Then* it returns
    `ErrInvalidTransition{From: Stopped, To: Failed}` (Stopped stays
    terminal — no new outgoing edges).
**Files**: `session/instance_state.go`

##### Task 1.1.2a: Add `Failed` status constant + doc comment (~2 min)
- Add `Failed Status = <next int>` near the existing `Status` constants, doc comment cross-referencing ADR-001 ("non-terminal; see `SESSION_STATUS_FAILED`").
- Files: `session/instance_state.go`

##### Task 1.1.2b: Add transition table entries (~3 min)
- Add `transitionIndex[transitionKey{Creating, Failed}]` and `transitionIndex[transitionKey{Failed, Creating}]` entries (`TransitionDef{}`, no special Guard/After needed at the FSM layer — epoch gating happens one layer up per ADR-002).
- Files: `session/instance_state.go`

##### Task 1.1.2c: Add `FailureReason` field + read-only accessor (no public setter) (~4 min)
- Add `failureReason string` to the actor-owned state struct and a public read-only `FailureReason() string` accessor, actor-routed like every other read. Do **not** add a public `SetFailureReason` — `FailureReason` is terminal-write metadata (meaningful only when `Status == Failed`), not progress text, so it must not be independently settable outside the epoch gate (unlike `SetCreationProgress`, which ADR-002 deliberately leaves ungated). Add only an unexported `setFailureReasonLocked` helper, callable exclusively from inside `TryForceStatusIfEpoch`'s own command closure (Task 1.2.2a) — mirroring this file's documented `xxxLocked`/public-wrapper convention (`session/instance_actor_setters.go:3-15`), where `Locked` helpers exist for composition inside other actor commands, not as standalone public writers.
- Files: `session/instance_actor_setters.go`

##### Task 1.1.2d: Unit tests for the new transitions (~5 min)
- Add table-driven tests: `Creating→Failed` legal, `Failed→Creating` legal, `Stopped→Failed` illegal, `Failed→Active` illegal (must go through `Creating` first). Follow existing naming convention in `session/instance_concurrency_test.go`.
- Files: `session/instance_state_test.go` (or sibling `_test.go` matching existing file layout)

#### Story 1.1.3: Update every exhaustive switch over `Status`/`SessionStatus`
**As a** developer relying on status rendering/filtering, **I want** every
switch over the status enum to handle `Failed` explicitly, **so that** no
call site silently renders "Unknown" or mismatches a filter.
**Acceptance Criteria**:
- `adapters.StatusToProto`, `adapters.InstanceToProto`'s status branch, and
  `WatchSessions`'s `StatusFilter` mapping all have a `Failed`/`FAILED` arm.
  - *Given* an `Instance` with `Status == Failed`, *When*
    `adapters.StatusToProto(instance.Status)` is called, *Then* it returns
    `SessionStatus.FAILED`, not `UNSPECIFIED`.
  - *Given* a `WatchSessions` request with `StatusFilter = [FAILED]`, *When*
    an instance transitions to `Failed` and publishes `SessionUpdatedEvent`,
    *Then* the subscriber receives it (filter matches).
**Files**: wherever `adapters.StatusToProto`/`InstanceToProto` live (grep
`func StatusToProto` in Task 1.1.3a to find the exact file — not yet located
in this survey), `server/services/session_service.go` (`WatchSessions`'s
filter mapping).

##### Task 1.1.3a: Locate and update `adapters.StatusToProto`/`InstanceToProto` (~5 min)
- `grep -rn "func StatusToProto\|func InstanceToProto"` to find the exact file (likely under `server/services/adapters/` or `session/`), add the `Failed` case to both.
- Files: TBD by grep (record actual path in the PR description)

##### Task 1.1.3b: Update `WatchSessions`'s `StatusFilter` mapping (~3 min)
- Add `Failed` to whatever mapping table `WatchSessions` (`session_service.go:3317`) uses to translate `StatusFilter` proto values into `session.Status` for comparison.
- Files: `server/services/session_service.go`

##### Task 1.1.3c: Add exhaustiveness guard (~4 min)
- Add a `default:` arm to each switch that returns an explicit error (Go) rather than silently falling through, so a *future* new status value fails loudly instead of repeating this exact gap. Add `exhaustive` linter check if not already enabled for `Status`-typed switches (check `.golangci.yml` first — Task, not a separate story, since it's a small config addition riding along).
- Files: `adapters.go` (from 1.1.3a), `server/services/session_service.go`, `.golangci.yml` if needed

#### Story 1.1.4: Persisted `CreationProgressUpdatedAt` timestamp
**As the** Stale-Creation Sweeper (Epic 4.1), **I want** a persisted,
restart-surviving "last progress" timestamp per instance, **so that** a
`Creating` row orphaned by a killed process is caught correctly on the new
process's first sweep, using how far it actually got — not just its
Creating-onset time.
**Acceptance Criteria**:
- `session.Instance` gains an actor-owned `creationProgressUpdatedAt
  time.Time` field with a public read accessor
  (`CreationProgressUpdatedAt() time.Time`); the existing locked helper
  behind `SetCreationProgress` sets it to `time.Now()` on every call,
  in the same command as the progress-text write.
  - *Given* an instance whose pipeline calls `SetCreationProgress("Cloning
    repository...")`, *When* the call returns, *Then*
    `instance.CreationProgressUpdatedAt()` reflects that call's time.
- Each phase transition in the Background Resolution Pipeline (Epic 2.2)
  persists the instance via `storage.UpdateInstance` (not `SaveInstances`,
  which silently skips any instance where `!inst.Started()` — true for
  every pre-worktree/tmux phase) after the `SetCreationProgress` call, not
  only at the pipeline's terminal write — so `CreationProgressUpdatedAt`
  round-trips through storage and survives a process restart.
  - *Given* a pipeline that has completed 2 of 6 phases when the process is
    killed, *When* the process restarts and reloads instances from storage,
    *Then* the reloaded instance's `CreationProgressUpdatedAt` reflects the
    2nd phase's transition time, not the instance's `CreatedAt`.
- `session/ent/schema` gains the corresponding hand-written field (see
  Migration Plan).
**Files**: `session/instance_state.go`, `session/instance_actor_setters.go`,
`session/ent/schema`, `server/services/session_service.go` (pipeline
phase-transition call sites, Epic 2.2)

##### Task 1.1.4a: Add `creationProgressUpdatedAt` field + read accessor (~3 min)
- Files: `session/instance_state.go`, `session/instance_actor_setters.go`

##### Task 1.1.4b: Bump it inside `SetCreationProgress`'s existing locked helper (~3 min)
- Same command, same mailbox round-trip — not a second write.
- Files: `session/instance_actor_setters.go`

##### Task 1.1.4c: Add the ent schema field (~3 min)
- Follow the `--feature sql/upsert` generation command from `CLAUDE.md`; do not commit generated output.
- Files: `session/ent/schema`

##### Task 1.1.4d: Unit test — timestamp updates on every `SetCreationProgress` call (~3 min)
- Files: `session/instance_state_test.go`

### Epic 1.2: `creationEpoch` fencing primitive

**Goal**: `TryForceStatusIfEpoch` and `TryStartRetry` both exist, are
actor-routed, and each have their own focused test suite proving the
fencing guarantees ADR-002 describes — the first for terminal writes, the
second for double-click-retry.

#### Story 1.2.1: Add `creationEpoch` field and epoch-bump helpers
**As a** backend developer, **I want** a per-instance epoch counter that
cancel and retry each bump exactly once, **so that** stale background
writers can be detected.
**Acceptance Criteria**:
- `Instance.CreationEpoch() uint64` reads the current epoch; a new
  actor-routed `bumpCreationEpoch()` (unexported, called only from cancel
  and retry code paths added in Phase 3) increments it.
  - *Given* a freshly-constructed `Instance` (epoch `0`), *When*
    `bumpCreationEpoch()` is called twice (simulating cancel-then-retry),
    *Then* `CreationEpoch()` returns `2`.
**Files**: `session/instance_state.go`, `session/instance_actor_setters.go`

##### Task 1.2.1a: Add `creationEpoch uint64` field to actor state (~2 min)
- Add the field to the actor-owned struct alongside `failureReason`/`creationProgress`.
- Files: `session/instance_state.go`

##### Task 1.2.1b: Add `CreationEpoch()` read + `bumpCreationEpoch()` write, actor-routed (~4 min)
- Mirror `SetCreationProgress`'s `sendSyncErr`/`send` routing pattern exactly.
- Files: `session/instance_actor_setters.go`

#### Story 1.2.2: `TryForceStatusIfEpoch` — the one atomic terminal-write primitive
**As a** background pipeline / stale sweeper, **I want** a single
compound check-and-set call, **so that** a stale writer can never win a
terminal status transition.
**Acceptance Criteria**:
- `TryForceStatusIfEpoch(capturedEpoch uint64, s Status, failureReason string) bool`
  applies the write and returns `true` iff `capturedEpoch == creationEpoch`
  at the moment the command executes inside the actor mailbox; otherwise
  no-ops and returns `false`.
  - *Given* an `Instance` with `creationEpoch == 3`, *When*
    `TryForceStatusIfEpoch(3, Active, "")` is called, *Then* it returns
    `true` and `Status()` reads `Active`.
  - *Given* an `Instance` with `creationEpoch == 3`, *When*
    `TryForceStatusIfEpoch(2, Active, "")` is called (stale caller, epoch
    already bumped past 2 by a cancel), *Then* it returns `false` and
    `Status()` is unchanged.
  - *Given* two goroutines calling `TryForceStatusIfEpoch` concurrently with
    the same captured epoch (racing success-vs-stale-timeout), *When* run
    under `go test -race -count=50`, *Then* exactly one call returns `true`.
**Files**: `session/instance_state.go`, `session/instance_actor_setters.go`

##### Task 1.2.2a: Implement `TryForceStatusIfEpoch` as one actor command (~5 min)
- Single `sendSyncErr`/`send` closure that reads `creationEpoch`, compares, and conditionally calls the existing internal status-set logic plus the new unexported `setFailureReasonLocked` (Task 1.1.2c) — all inside one enqueued command, per ADR-002's "compound check-and-set entirely inside one enqueued command" requirement. `setFailureReasonLocked` must have no other caller — this is the single place `failureReason` is ever written, guaranteeing it can only change in the same atomic step as `Status`.
- Files: `session/instance_actor_setters.go`

##### Task 1.2.2b: Unit tests for the fencing guarantee (~5 min)
- Table test: epoch-matches → true+applied; epoch-mismatch → false+unchanged; concurrent race (`-race -count=50`) → exactly one winner (use an atomic counter of `true` returns, assert it equals 1).
- Files: `session/instance_state_test.go` or a new `session/instance_epoch_test.go`

#### Story 1.2.3: `TryStartRetry` — the one atomic primitive for starting a retry
**As a** `RetrySessionCreation` handler, **I want** a single compound
check-and-set that validates `Failed`, bumps the epoch, and marks the retry
started, all in one enqueued command, **so that** two concurrent retry
clicks can never both spawn a live pipeline sharing the same epoch.
**Acceptance Criteria** (see ADR-002's addendum for the full rationale):
- `TryStartRetry() (newEpoch uint64, started bool)` no-ops (`started=false`,
  `newEpoch=0`) unless `Status == Failed` at the moment the command executes
  inside the actor mailbox; otherwise it bumps `creationEpoch`, resets
  `Status` to `Creating` with fresh `creation_progress`, and returns
  `(newEpoch, true)`.
  - *Given* an instance with `Status == Failed`, *When* `TryStartRetry()` is
    called, *Then* it returns `(N+1, true)` where `N` was the prior epoch,
    and `Status()` reads `Creating`.
  - *Given* an instance with `Status == Active` (not `Failed`), *When*
    `TryStartRetry()` is called, *Then* it returns `(0, false)` and the
    instance is untouched.
  - *Given* two goroutines calling `TryStartRetry()` concurrently on the
    same `Failed` instance (double-click retry), *When* run under
    `go test -race -count=50`, *Then* exactly one call returns
    `started == true`; the other observes `Status` already `Creating` (no
    longer `Failed`) and returns `(0, false)`.
**Files**: `session/instance_state.go`, `session/instance_actor_setters.go`

##### Task 1.2.3a: Implement `TryStartRetry` as one actor command (~5 min)
- Single `sendSyncErr`/`send` closure: check `Status == Failed`, and if so, bump `creationEpoch`, transition to `Creating`, reset `creation_progress` — all inside one enqueued command, mirroring Task 1.2.2a's shape exactly.
- Files: `session/instance_actor_setters.go`

##### Task 1.2.3b: Unit tests for the double-click-retry fencing guarantee (~5 min)
- Table test per the Given/When/Then above, including the concurrent `-race -count=50` case with an atomic counter of `true` returns asserting it equals exactly 1.
- Files: `session/instance_state_test.go` or `session/instance_epoch_test.go`

### Epic 1.3: Telemetry helper + creation metrics

**Goal**: `telemetry.StartLinkedBackgroundSpan` exists and is safe to call
with OTel disabled; the two creation metrics are registered.

#### Story 1.3.1: `StartLinkedBackgroundSpan`
**As a** backend developer instrumenting any goroutine that outlives its
triggering request, **I want** a reusable helper implementing ADR-003's
pattern, **so that** future call sites don't hand-roll it.
**Acceptance Criteria**:
- `telemetry.StartLinkedBackgroundSpan(ctx, name) (context.Context, trace.Span)`
  returns a working span (no-op-safe) linked to `ctx`'s existing span if
  present.
  - *Given* OTel is disabled (no provider configured), *When*
    `StartLinkedBackgroundSpan(context.Background(), "session.create.resolve")`
    is called, *Then* it returns a non-nil no-op span and does not panic.
  - *Given* OTel is enabled and `ctx` carries an active RPC span, *When*
    `StartLinkedBackgroundSpan(ctx, "session.create.resolve")` is called,
    *Then* the returned span has `trace.WithNewRoot()` semantics (new trace
    ID) and one `Link` pointing at the RPC span's `SpanContext`.
**Files**: `telemetry/telemetry.go`

##### Task 1.3.1a: Implement the helper (~3 min)
- Per ADR-003's code block exactly.
- Files: `telemetry/telemetry.go`

##### Task 1.3.1b: Unit test with OTel disabled (~3 min)
- Assert no panic and a non-nil span/context returned when no provider is configured.
- Files: `telemetry/telemetry_test.go`

#### Story 1.3.2: Creation outcome/duration metrics
**As an** operator debugging locally, **I want** a counter and histogram for
creation outcomes, **so that** I can see success/failed/stale/cancelled
rates and durations via the existing OTEL pipeline.
**Acceptance Criteria**:
- `session.creation.outcome` (`Int64Counter`) and
  `session.creation.duration_ms` (`Float64Histogram`) are registered once
  (package-level, following the existing `safeexec.sigkill_escalations`
  registration idiom) and both accept an `outcome` attribute.
  - *Given* a completed pipeline run with outcome `failed`, *When* the
    terminal write happens, *Then* `session.creation.outcome` is
    incremented once with `outcome="failed"` and
    `session.creation.duration_ms` records the elapsed wall-clock ms with
    the same attribute.
**Files**: `server/services/session_service.go` (or a new
`server/services/session_creation_metrics.go` if the registration block
is non-trivial — see Task 1.3.2a)

##### Task 1.3.2a: Register the two instruments (~4 min)
- Follow the `Provider.Meter()`/`GetMeter()` idiom already used for `safeexec.sigkill_escalations`; place in a new small file `server/services/session_creation_metrics.go` if `session_service.go` is already large (it is — ~250+ lines just for `CreateSession`).
- Files: `server/services/session_creation_metrics.go` (new)

---

## Phase 2: Backend `CreateSession` restructure

### Epic 2.1: Synchronous prefix — instance created and published before resolution

**Goal**: For every session type, `storage.SaveInstances` +
`SessionCreatedEvent` publish happen immediately after the (unchanged)
fast-fail/uniqueness checks, before any GitHub/alias/branch/worktree work
starts.

#### Story 2.1.1: Reorder `CreateSession`'s synchronous section
**As a** user creating any session, **I want** the RPC to return almost
immediately, **so that** the session appears in my list within ~1s
regardless of session type.
**Acceptance Criteria**:
- Fast-fail validation, fork dispatch, config/remote/restart-source
  resolution, and title-uniqueness check remain synchronous and unchanged
  in behavior (Constraints).
  - *Given* a `CreateSessionRequest` with an empty `title`, *When*
    `CreateSession` is called, *Then* it returns a synchronous RPC error
    (`InvalidArgument`) exactly as today, with no `Instance` created.
  - *Given* a `CreateSessionRequest` with a `title` matching an existing
    session, *When* `CreateSession` is called, *Then* it returns a
    synchronous `AlreadyExists` RPC error, with no new `Instance` created.
- Instance construction, `storage.SaveInstances`, and
  `eventBus.Publish(NewSessionCreatedEvent(instance))` happen immediately
  after the uniqueness check, for every session type — no GitHub
  resolution, alias/default resolution, or branch inference runs before
  this point.
  - *Given* a `CreateSessionRequest` with a `github_url` pointing at a slow
    GHE host, *When* `CreateSession` is called, *Then* the RPC returns
    within the SLO (<500ms p99) with `status=CREATING` and a non-empty
    `creation_progress`, before the GitHub clone has necessarily completed
    (or even started).
**Files**: `server/services/session_service.go`

##### Task 2.1.1a: Extract current L1912-1938 GitHub-detection call + everything through instance construction into named helper functions (~5 min)
- Pure extraction (no behavior change yet) so Task 2.1.1b can reorder call sites cleanly; e.g. `resolveGitHubInput(...)`, `constructInstance(...)` as named locals or small methods — keep the shared pre-branch skeleton minimal per `research/architecture.md` §7's mitigation.
- Files: `server/services/session_service.go`

##### Task 2.1.1b: Move instance-construction+save+publish ahead of resolution calls (~5 min)
- Reorder so `storage.SaveInstances`+`eventBus.Publish(NewSessionCreatedEvent(...))` happen before the (now-deferred) GitHub/alias/branch calls; snapshot `InstanceToProto` for the RPC response at this point (mirrors today's pre-goroutine snapshot discipline, just earlier).
- Files: `server/services/session_service.go`

##### Task 2.1.1c: Confirm `storage.SaveInstances` ordering preserves the title-uniqueness race property (~3 min)
- Add/adjust a test asserting two rapid `CreateSession` calls with the same title: second one's uniqueness check reliably sees the first's persisted row (per `research/features.md` §8 / `research/architecture.md` §5.3).
- Files: `server/services/session_service_test.go`

### Epic 2.2: Background Resolution Pipeline

**Goal**: One `trackCleanup`-dispatched goroutine runs the full merged
pipeline (GitHub resolution → alias/defaults → branch inference →
worktree/tmux) against a properly-detached, timed-out, cancelable context,
publishing progress and making exactly one terminal write via
`TryForceStatusIfEpoch`.

#### Story 2.2.1: Background Resolution Context construction
**As a** background pipeline, **I want** a context detached from the RPC
but carrying its trace values and independently timed out, **so that** the
RPC returning doesn't kill in-progress resolution, and a wedged clone can't
hang forever.
**Acceptance Criteria**:
- The pipeline's context is `context.WithTimeout(context.WithoutCancel(rpcCtx), maxCreationResolutionTimeout)`, never a direct derivative retaining the RPC's own cancellation.
  - *Given* a `CreateSession` RPC call whose client disconnects immediately
    after the RPC returns, *When* the background pipeline is mid-clone,
    *Then* the clone subprocess continues running (not canceled by the
    client disconnect) — verified via a test that cancels the RPC's
    request `ctx` right after `CreateSession` returns and asserts the
    pipeline still completes.
  - *Given* a pipeline whose resolution hangs past `maxCreationResolutionTimeout`, *When* the timeout elapses, *Then* the pipeline's context is done, its `safeexec.CommandContext` subprocess (if any) receives the cancellation, and the pipeline makes a terminal `Failed` write with `FailureReason=StartupError` (or a dedicated `Timeout` reason if introduced — decide during implementation, default to reusing `StartupError`/`GitHubResolutionError` per phase).
- A `context.CancelFunc` for this context is stored on the instance (new field, actor-owned) at goroutine-spawn time, reachable by the cancel RPC (Epic 3.2).
**Files**: `server/services/session_service.go`, `session/instance_state.go`
(new field), `session/instance_actor_setters.go`

##### Task 2.2.1a: Add `cancelFunc context.CancelFunc` field + setter/getter, actor-routed (~4 min)
- Same pattern as `creationEpoch` — set once at spawn, read once at cancel time, cleared on goroutine exit. Unlike `creationEpoch`, this field is process-local by nature (a `context.CancelFunc` cannot be persisted or reconstructed) — any instance loaded from storage without a live goroutine spawned in the current process has a `nil` value here; see Task 3.2.1b's nil-guard, which is the required handling, not an edge case to special-case away.
- Files: `session/instance_state.go`, `session/instance_actor_setters.go`

##### Task 2.2.1b: Construct the Background Resolution Context in `CreateSession` (~3 min)
- `bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), maxCreationResolutionTimeout)`; store `cancel` on the instance before spawning; `maxCreationResolutionTimeout` as a new package const (value: see Unresolved Questions — default 10 min pending empirical confirmation).
- Files: `server/services/session_service.go`

##### Task 2.2.1c: Regression test — RPC-context cancellation must not kill the pipeline (~5 min)
- Per the Given-When-Then above; this is the test that would have caught `research/pitfalls.md` §2's "single most common bug."
- Files: `server/services/session_service_test.go`

#### Story 2.2.2: Merge GitHub/alias/branch resolution into the pipeline, ahead of worktree/tmux
**As a** background pipeline, **I want** GitHub-URL resolution, alias/default
resolution, and branch/session-type inference to run in the same goroutine
as today's worktree/tmux tail, **so that** every session type goes through
one consistent, observable pipeline.
**Acceptance Criteria**:
- Today's `ResolveGitHubInputCtxWithHosts` call is moved from the RPC's
  synchronous section into the pipeline, called against the Background
  Resolution Context (not `ctx`).
  - *Given* a session created from a `github_url` pointing at
    `github.netflix.net`, *When* the pipeline runs, *Then*
    `creation_progress` transitions through `"Resolving GitHub URL..."` →
    `"Cloning repository..."` → (alias/branch phases) →
    `"Setting up worktree..."` → `"Starting session..."`, each backed by a
    `SessionUpdatedEvent{"creation_progress"}` publish.
  - *Given* a plain directory session (no GitHub URL), *When* the pipeline
    runs, *Then* it completes in low-single-digit milliseconds (no network
    I/O in its phases) and the RPC caller sees no latency regression.
- Each phase's logic lives in its own already-tested function (not inlined
  into the pipeline dispatcher), per `research/architecture.md` §7's
  mitigation for "a bug in the shared skeleton breaks every session type."
- Each phase transition's `SetCreationProgress` call (which also bumps
  `CreationProgressUpdatedAt`, Story 1.1.4) is followed by a
  `storage.UpdateInstance` persist (not `storage.SaveInstances`, which
  silently skips any instance where `!inst.Started()` — true for every
  phase before Task 2.2.2c's worktree/tmux startup, i.e. exactly the
  slow/hangable phases this project's Problem Statement is about), so the
  Stale-Creation Sweeper's restart-survival guarantee (Epic 4.1) actually
  holds — not only the pipeline's one terminal write persists.
**Files**: `server/services/session_service.go`

##### Task 2.2.2a: Move GitHub-URL detection/clone call into the pipeline (~5 min)
- Relocate the L1912-1938-era call (now against `bgCtx`), wrapped with `SetCreationProgress("Resolving GitHub URL...")` / `"Cloning repository..."` before/after.
- Files: `server/services/session_service.go`

##### Task 2.2.2b: Move alias/default resolution + branch/session-type inference into the pipeline (~5 min)
- Locate the exact current call sites (between old L1938 and instance construction — confirm exact lines while editing) and relocate as their own phase functions with progress messages.
- Files: `server/services/session_service.go`

##### Task 2.2.2c: Keep worktree/tmux startup as the pipeline's final phase (~4 min)
- Adapt today's L2397-2413 tail to be the last phase of the same pipeline function rather than a separately-dispatched goroutine — same `SetCreationProgress`/`instance.Start(true)` logic, now sharing one `trackCleanup` invocation and one terminal-write call site with the earlier phases.
- Files: `server/services/session_service.go`

##### Task 2.2.2c-2: Persist `CreationProgressUpdatedAt` after each phase transition (~3 min)
- Add a `storage.UpdateInstance(instance)` call — **not** `storage.SaveInstances`, which starts with `if !inst.Started() { continue }` (`session/storage.go:283-301`) and is a silent no-op for every phase before Task 2.2.2c's worktree/tmux startup (`started` only flips true inside `startLocked`, called from that final phase) — immediately after each `SetCreationProgress` call in the pipeline (Tasks 2.2.2a/b/c), not only at the terminal write. `UpdateInstance` (`session/storage.go:546-548`) calls `s.repo.Update` directly with no `Started()` gate, matching how the initial-creation persist (`Storage.AddInstance`, Epic 2.1) is also gate-free. This is what Epic 4.1's restart-survival test (Task 4.1.2d) actually depends on.
- Files: `server/services/session_service.go`

##### Task 2.2.2d: Wrap the whole pipeline in one `recover()` (~3 min)
- A detached goroutine's unrecovered panic crashes the process; add `defer func() { if r := recover(); ... }()` at the top of the pipeline function, logging and making a terminal `Failed` write on panic recovery (via `TryForceStatusIfEpoch`, gated the same as any other failure).
- Files: `server/services/session_service.go`

#### Story 2.2.3: Terminal write via `TryForceStatusIfEpoch` + span/metrics
**As a** background pipeline, **I want** exactly one terminal write, one
span, and one metric emission per creation attempt, **so that** no race
between cancel/retry/stale-timeout can double-publish or silently drop the
result.
**Acceptance Criteria**:
- The pipeline captures `instance.CreationEpoch()` once at spawn and passes
  it to `TryForceStatusIfEpoch` at its one terminal call site (success or
  any failure branch).
  - *Given* the pipeline succeeds and its captured epoch still matches
    current, *When* it calls `TryForceStatusIfEpoch(epoch, Active, "")`,
    *Then* it returns `true`, the instance transitions to `Active`, exactly
    one `SessionUpdatedEvent{"status"}` is published, and
    `session.creation.outcome{outcome="success"}` increments by 1.
  - *Given* the pipeline fails at the GitHub-resolution phase, *When* it
    calls `TryForceStatusIfEpoch(epoch, Failed, "GitHubResolutionError")`,
    *Then* the instance transitions to `Failed` with that reason, and
    `session.creation.outcome{outcome="failed"}` increments by 1.
  - *Given* a cancel has already bumped the epoch before the pipeline's
    terminal write, *When* the pipeline calls `TryForceStatusIfEpoch` with
    its stale captured epoch, *Then* it returns `false`, no event is
    published, and no metric increments (the cancel path owns the outcome
    in this case — see Epic 3.2).
- `StartLinkedBackgroundSpan` wraps the whole pipeline invocation; span ends
  (with `codes.Error` set on failure) at the same point the terminal write
  happens.
**Files**: `server/services/session_service.go`

##### Task 2.2.3a: Wire `TryForceStatusIfEpoch` at the single terminal call site (~4 min)
- One call site for all outcomes (success/failure/panic-recovered), passing the appropriate `Status`/`FailureReason`.
- Files: `server/services/session_service.go`

##### Task 2.2.3b: Wire span start/end + metric emission around the pipeline (~4 min)
- `ctx, span := telemetry.StartLinkedBackgroundSpan(bgCtx, "session.create.resolve")`; `defer span.End()`; increment counter/histogram at the terminal-write call site (only when `TryForceStatusIfEpoch` returns `true`, so a superseded writer doesn't double-count against the actor that actually won).
- Files: `server/services/session_service.go`

##### Task 2.2.3c: Test — exactly one terminal event under a cancel/success race (~5 min)
- Per `research/pitfalls.md` §4's explicit ask: assert exactly one terminal `SessionUpdatedEvent` publishes even when cancel and pipeline-success are made to race (`-race -count=50`).
- Files: `server/services/session_service_test.go`

---

## Phase 3: Cancel & Retry RPCs

### Epic 3.1: Idempotent Creation Cleanup

**Goal**: One shared cleanup helper, safe to call on any subset of
already-present resources, used by `DeleteSession`, cancel, and retry.

#### Story 3.1.1: Extract `cleanupPartialCreation`
**As a** developer implementing cancel/retry, **I want** one idempotent
cleanup primitive, **so that** neither path re-derives `DeleteSession`'s
already-solved edge cases.
**Acceptance Criteria**:
- `cleanupPartialCreation(instance *session.Instance) error` kills any live
  tmux session, removes any worktree/clone directory, and is a no-op (not
  an error) for any step whose resource never existed.
  - *Given* an instance whose pipeline failed before any worktree existed,
    *When* `cleanupPartialCreation(instance)` is called, *Then* it returns
    `nil` with no error about a missing directory/tmux session.
  - *Given* an instance whose pipeline failed after worktree creation but
    before tmux startup, *When* `cleanupPartialCreation(instance)` is
    called, *Then* the worktree directory is removed and no tmux-related
    error occurs.
**Files**: `server/services/session_service.go` (extracted from
`DeleteSession`'s existing cleanup logic at `session_service.go:3193+`)

##### Task 3.1.1a: Extract the shared steps from `DeleteSession`'s cleanup into `cleanupPartialCreation` (~5 min)
- Preserve `DeleteSession`'s existing ordering rationale (kill process before deleting directory; `FindLiveInstance` before `removeFromAllPollers`) verbatim — this is a refactor-extraction, not a rewrite.
- Files: `server/services/session_service.go`

##### Task 3.1.1b: Have `DeleteSession` call the extracted helper (~3 min)
- Confirm `DeleteSession`'s existing tests still pass unchanged (pure refactor).
- Files: `server/services/session_service.go`

##### Task 3.1.1c: Idempotency tests — call twice, call on a never-started instance (~4 min)
- Files: `server/services/session_service_test.go`

### Epic 3.2: `CancelSessionCreation` RPC

**Goal**: A user can cancel a `Creating` session; its background pipeline
stops, resources are cleaned up, and the instance is removed — with no
lingering "brief flash of Running" if cancel loses a race with success.

#### Story 3.2.1: RPC handler + proto method
**As a** user, **I want** to cancel a stuck-in-Creating session, **so that**
I don't have to wait out a slow/hung clone — including one stuck from
before the server itself last restarted.
**Acceptance Criteria**:
- New `CancelSessionCreation(ctx, req) (*CancelSessionCreationResponse, error)`
  RPC: looks up the instance, validates `Status == Creating` (else returns
  `FailedPrecondition`), bumps `creationEpoch`, and — only if the stored
  `context.CancelFunc` is non-nil — calls it; a `nil` `CancelFunc` (the
  instance's pipeline goroutine was spawned in a since-restarted process
  and no longer exists in this one) is not an error and is not skipped
  silently either — it means "no live work to interrupt," and the handler
  proceeds straight to `cleanupPartialCreation` and instance/storage-row
  removal exactly as it would after a successful interrupt, since the
  state-machine transition (not goroutine interruption) is the only thing
  actually required in that case. Returns success either way.
  - *Given* an instance with `Status == Creating`, *When*
    `CancelSessionCreation` is called with its ID, *Then* the instance is
    removed from storage, its tmux/worktree resources (if any) are cleaned
    up, and a subsequent `GetSession` for that ID returns `NotFound`.
  - *Given* an instance with `Status == Creating` whose `context.CancelFunc`
    is `nil` (constructed directly via storage with no goroutine ever
    spawned in the current process — the post-restart case), *When*
    `CancelSessionCreation` is called with its ID, *Then* the handler does
    not panic or error on the nil check, bumps the epoch, runs
    `cleanupPartialCreation`, and removes the instance exactly as the live
    case does.
  - *Given* an instance with `Status == Active` (already finished), *When*
    `CancelSessionCreation` is called with its ID, *Then* it returns a
    `FailedPrecondition` error and the instance is untouched (cancel only
    applies to `Creating`, per Non-Goals).
  - *Given* cancel is called at the exact moment the pipeline's terminal
    write is in flight (race), *When* both complete, *Then* exactly one
    outcome wins deterministically: either the instance ends up `Active`
    (cancel's `TryForceStatusIfEpoch`-gated no-op path recognizes it lost
    the race and skips cleanup... — see Task 3.2.1c for the precise
    resolution) or the instance is removed with resources cleaned up; the
    card never shows a "flash of Cancelled then Running."
**Files**: `proto/session/v1/session.proto`, `server/services/session_service.go`

##### Task 3.2.1a: Add `CancelSessionCreation` RPC + request/response messages to proto (~4 min)
- Files: `proto/session/v1/session.proto`

##### Task 3.2.1b: Implement the handler: lookup, status check, epoch bump, nil-guarded CancelFunc call (~5 min)
- `cancelFunc := instance.CancelFunc(); if cancelFunc != nil { cancelFunc() }` — a `nil` value (process restarted since this instance's pipeline was spawned; the field is process-local and never persisted, per ADR-002's Consequences) means there is no live goroutine to interrupt in this process, not an error condition. Proceed to `cleanupPartialCreation` + removal in both branches.
- Files: `server/services/session_service.go`

##### Task 3.2.1b-2: Regression test — cancel a `Creating` instance with no live goroutine in this process (~5 min)
- Construct an instance directly via storage with `Status=Creating` and a `nil` `CancelFunc` — a direct fixture is fine here since this test only exercises the nil-`CancelFunc` guard, not the phase-transition persistence path Task 4.1.2d covers — and no goroutine ever spawned for it in the test process. Assert `CancelSessionCreation` succeeds: no nil-pointer panic, epoch bumped, `cleanupPartialCreation` runs, instance removed.
- Files: `server/services/session_service_test.go`

##### Task 3.2.1c: Resolve the cancel-vs-success race explicitly (~5 min)
- After bumping the epoch and calling `cancelFunc()`, the handler must check whether the pipeline's terminal write already won (i.e., instance is already `Active`) *before* running cleanup/removal — if so, cancel itself becomes a no-op that reports the actual state (`Active`) rather than proceeding to destroy a successfully-created session. Implement as: bump epoch (this is itself a mailbox round-trip) → re-read `Status()` → if `Active`, return success-but-already-running response; if still `Creating`/not-yet-terminal, proceed with cancelFunc+cleanup+removal.
- Files: `server/services/session_service.go`

##### Task 3.2.1d: Test the cancel-vs-success race under `-race -count=50` (~5 min)
- Files: `server/services/session_service_test.go`

##### Task 3.2.1e: Wire `outcome="cancelled"` metric emission (~2 min)
- Files: `server/services/session_service.go`

### Epic 3.3: `RetrySessionCreation` RPC

**Goal**: A user can retry a `Failed` session in place — same instance ID,
no duplicate row, no second `SessionCreatedEvent`.

#### Story 3.3.1: RPC handler
**As a** user, **I want** to retry a failed session creation without
re-entering the omnibar, **so that** I don't have to retype the same
inputs.
**Acceptance Criteria**:
- New `RetrySessionCreation(ctx, req)` RPC: looks up the instance, calls
  `instance.TryStartRetry()` (Story 1.2.3) as its first state-mutating
  step — this is the single validate-`Failed`+bump-epoch+reset-to-`Creating`
  operation, atomic inside one actor command. If `started == false`, return
  `FailedPrecondition` immediately, before touching cleanup or spawning
  anything. If `started == true`, call the outgoing attempt's stored
  `cancelFunc()` (tolerating nil/already-called, for symmetry with Cancel
  and so a slow post-write span/metric tail from the prior attempt can't
  race the new one), run `cleanupPartialCreation`, then spawn the
  Background Resolution Pipeline with the epoch `TryStartRetry` returned,
  publishing `SessionUpdatedEvent` (never `SessionCreatedEvent`).
  - *Given* an instance with `Status == Failed`, `FailureReason =
    GitHubResolutionError`, *When* `RetrySessionCreation` is called, *Then*
    the same instance ID transitions `Failed → Creating`, exactly zero new
    `SessionCreatedEvent`s are published for that ID (ever, across the
    original attempt and the retry), and the pipeline re-runs from a clean
    slate (any partial clone directory from the first attempt has been
    removed by `cleanupPartialCreation` first).
  - *Given* two rapid `RetrySessionCreation` calls on the same `Failed`
    instance (impatient double-click), *When* both are processed, *Then*
    exactly one call's `TryStartRetry()` observes `started == true` and
    spawns the one live pipeline for that instance; the other observes
    `started == false` (the instance is no longer `Failed` by the time its
    own command runs in the mailbox) and returns `FailedPrecondition`
    without running cleanup or spawning a pipeline — resolved now, not left
    to implementation-time judgment, mirroring Task 3.2.1c's
    bump-then-check shape for Cancel.
**Files**: `proto/session/v1/session.proto`, `server/services/session_service.go`

##### Task 3.3.1a: Add `RetrySessionCreation` RPC + messages to proto (~4 min)
- Files: `proto/session/v1/session.proto`

##### Task 3.3.1b: Implement the handler around `TryStartRetry` (~5 min)
- Lookup → `TryStartRetry()` → branch on `started` → (if true) `cancelFunc()` (nil-tolerant) → `cleanupPartialCreation` → spawn pipeline with the returned epoch → publish `SessionUpdatedEvent`. No separate epoch-read-then-bump composition — `TryStartRetry` is the one call that does both atomically.
- Files: `server/services/session_service.go`

##### Task 3.3.1c: Test — no duplicate `SessionCreatedEvent`, no duplicate storage row (~5 min)
- Files: `server/services/session_service_test.go`

##### Task 3.3.1d: Test — double-click retry deterministically spawns exactly one live pipeline (~5 min)
- Fire two concurrent `RetrySessionCreation` calls at the same `Failed` instance under `-race -count=50`; assert exactly one returns success and one returns `FailedPrecondition`, and that `goleak`-style inspection (or a pipeline-entry counter) shows only one pipeline goroutine ever ran for that instance.
- Files: `server/services/session_service_test.go`

---

## Phase 4: Stale-Creation Sweeper

### Epic 4.1: `CreationStaleConfig` + `StaleCreationSweeper`

**Goal**: A `Creating` session whose last progress update is older than the
configured threshold (default 10 minutes) is automatically flipped to
`Failed`/`Stale`, surviving a server restart correctly.

#### Story 4.1.1: Config
**As an** operator, **I want** a configurable staleness threshold, **so
that** I can tune it without a code change if 10 minutes proves wrong for
my network.
**Acceptance Criteria**:
- `config.CreationStaleConfig{ThresholdMinutes int}` with
  `ThresholdMinutesOrDefault()` defaulting to 10, mirroring
  `StaleSessionConfig`'s shape exactly.
  - *Given* no `creation_stale` config section is set, *When*
    `ThresholdMinutesOrDefault()` is called, *Then* it returns `10`.
  - *Given* `creation_stale.threshold_minutes = 20` in config, *When*
    the sweeper reads it, *Then* it uses `20` minutes as the threshold.
**Files**: `config/types.go`

##### Task 4.1.1a: Add `CreationStaleConfig` struct + default const (~4 min)
- Files: `config/types.go`

#### Story 4.1.2: `StaleCreationSweeper`
**As a** user, **I want** a session that's been silently stuck in Creating
(e.g. because the server restarted mid-clone) to eventually show up as
Failed instead of hanging forever, **so that** I notice and can retry or
clean it up.
**Acceptance Criteria**:
- Ticker-driven sweeper scans `Creating`-status instances; for each, if
  `time.Since(lastProgress) > threshold` — where `lastProgress` is
  `instance.CreationProgressUpdatedAt()` (Story 1.1.4), or `instance.CreatedAt()`
  if `CreationProgressUpdatedAt` is the zero value (never updated) — calls
  `TryForceStatusIfEpoch(instance.CreationEpoch(), Failed, "Stale")`.
  - *Given* an instance in `Creating` status whose last persisted
    `creation_progress` update was 15 minutes ago (threshold 10), *When*
    the sweeper ticks, *Then* the instance transitions to `Failed` with
    `FailureReason = Stale`, a `SessionUpdatedEvent` publishes, and
    `session.creation.outcome{outcome="stale"}` increments.
  - *Given* an instance in `Creating` status left over from a process that
    was killed before this deploy (persisted `Creating` row, no live
    goroutine in the *current* process), *When* the sweeper ticks after
    server restart and finds its last-update timestamp older than
    threshold, *Then* it is flipped to `Failed`/`Stale` on this process's
    first sweep, without needing any in-process bookkeeping about whether
    a goroutine for it ever ran here.
  - *Given* an instance in `Creating` status whose last update was 2
    minutes ago (below threshold), *When* the sweeper ticks, *Then* it is
    left untouched.
  - *Given* the sweeper flips an instance to `Failed`/`Stale` at the exact
    moment its (still-alive, network-was-just-slow) pipeline was about to
    write `Active`, *When* both race, *Then* whichever holds the current
    epoch at write time wins (per Epic 1.2's guarantee) — test this
    explicitly, don't just assume it from the primitive's other tests.
- The timestamp compared is `CreationProgressUpdatedAt` (Story 1.1.4), the
  **persisted** last-progress-update time (not in-process elapsed/monotonic
  time) — round-trips correctly through `session/ent`/JSON storage (per
  `research/pitfalls.md` §6).
**Files**: `server/services/stale_creation_sweeper.go` (new, structural
sibling of `server/services/stale_session_notifier.go`)

##### Task 4.1.2a: Scaffold `StaleCreationSweeper` mirroring `StaleSessionNotifier`'s structure (~5 min)
- Ticker interval, constructor tolerant of nil `eventBus`, `Start(ctx)` method.
- Files: `server/services/stale_creation_sweeper.go`

##### Task 4.1.2b: Implement the scan-and-flip logic using persisted timestamps (~5 min)
- Read `instance.CreationProgressUpdatedAt()` (falling back to `CreatedAt` when zero-valued), per Story 1.1.4's field.
- Files: `server/services/stale_creation_sweeper.go`

##### Task 4.1.2c: Wire the sweeper's `Start(ctx)` into server startup (~3 min)
- Alongside `StaleSessionNotifier`'s existing startup wiring.
- Files: `server/server.go`

##### Task 4.1.2d: Test — server-restart orphan case, exercised through the real pipeline write path, not a storage-constructed fixture (~7 min)
- Do **not** construct the instance's `CreationProgressUpdatedAt` by writing it directly into a fixture row via storage — that was the original round's mistake: a fixture built by hand can pass even if the pipeline's actual persistence call (Task 2.2.2c-2) is broken or wired to the wrong storage method (`SaveInstances` silently no-ops pre-`Started()`; this exact bug shipped past this test once already). Instead, drive the real code path: construct an instance via the pipeline's own instance-construction helper (Task 2.1.1a), call `instance.SetCreationProgress(...)` and the pipeline's phase-transition persistence call (`storage.UpdateInstance`, Task 2.2.2c-2) directly — i.e. invoke the actual function/method the pipeline calls for a phase transition, not `storage.SaveInstances` and not a hand-set struct field — for at least one phase before the final worktree/tmux phase (e.g. simulate "Cloning repository..." completing), advance a fake clock (or sleep past a short test threshold) past the staleness threshold, and only then simulate the restart by reloading the instance from storage into a fresh in-memory registry with no live goroutine/`cancelFunc` for it. Assert the sweeper flips it using the reloaded `CreationProgressUpdatedAt`, proving both that the value survived the reload AND that it was written by the real pre-`Started()` phase-transition path, not `CreatedAt` or a fixture shortcut. Task 4.1.2f (below) separately covers the zero-progress-updates-ever fallback case.
- Files: `server/services/stale_creation_sweeper_test.go`, `server/services/session_service_test.go` (if the phase-transition persistence call needs a small test seam to invoke directly, e.g. exporting the pipeline's phase-persist helper for test use)

##### Task 4.1.2e: Test — stale-flip vs. genuine-late-success race (~5 min)
- Files: `server/services/stale_creation_sweeper_test.go`

##### Task 4.1.2f: Test — zero-progress-updates-ever case (goroutine-spawn failure backstop) (~4 min)
- Per `research/architecture.md` §7's explicit callout: an instance published at Creating with no progress update ever must still go stale on schedule, using the Creating-onset time as the baseline.
- Files: `server/services/stale_creation_sweeper_test.go`

---

## Phase 5: Frontend

### Epic 5.1: Omnibar early-close

**Goal**: `Omnibar.tsx` closes as soon as the (now-fast) RPC returns; no
change needed to the await/close call pattern itself per
`research/features.md` §4, but verify it end-to-end.

#### Story 5.1.1: Verify/confirm early-close behavior
**As a** user, **I want** the omnibar to close immediately after hitting
Create, **so that** I'm not staring at a frozen dialog.
**Acceptance Criteria**:
- *Given* a `github_url` session creation against a slow GHE host, *When*
  the user hits Create, *Then* the omnibar closes within ~500ms (matching
  the new RPC SLO), well before the background pipeline finishes.
**Files**: `web-app/src/components/sessions/Omnibar.tsx` (verification only
— per `research/features.md` §4, the existing `await onCreateSession(...);
onClose();` pattern already does the right thing once the RPC itself is
fast; this story exists to add regression coverage, not change the call
pattern)

##### Task 5.1.1a: Add a Playwright assertion that the omnibar closes before pipeline completion for a slow-path session (~5 min)
- Mock/stub a slow background resolution (or use a test-mode hook) so the assertion is deterministic, not timing-flaky.
- Files: `tests/e2e/session-lifecycle.spec.ts` (or a new spec file per e2e conventions)

### Epic 5.2: `SessionCard` Failed-state rendering

**Goal**: `SessionCard.tsx` renders `Failed` with a distinct icon+color, a
persistent (not just toast) error message, extending the existing live
region rather than duplicating it.

#### Story 5.2.1: Status color/text/icon
**As a** user scanning my session list, **I want** a Failed session to look
visually distinct from Creating/Crashed/Stopped, **so that** I can tell at
a glance which sessions need my attention.
**Acceptance Criteria**:
- `getStatusColor`/`getStatusText` (`SessionCard.tsx:258-306`) each gain a
  `SessionStatus.FAILED` case using a new `statusCreationFailed` token
  (not `statusCrashed`) and label `"Failed"`.
  - *Given* `session.status === SessionStatus.FAILED`, *When*
    `SessionCard` renders, *Then* the status pill shows the
    `statusCreationFailed` color, text `"Failed"`, and a distinct
    warning-glyph icon (not shared with `CRASHED`'s icon, if `CRASHED` has
    one — otherwise introduce one for both per WCAG 1.4.1's icon+color
    requirement).
**Files**: `web-app/src/components/sessions/SessionCard.tsx`,
`web-app/src/components/sessions/SessionCard.css.ts`

##### Task 5.2.1a: Add `statusCreationFailed` CSS token (~2 min)
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 5.2.1b: Add `FAILED` case to `getStatusColor`/`getStatusText` (~3 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.2.1c: Add a distinct icon for Failed vs. Crashed (~4 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`, `SessionCard.css.ts`

#### Story 5.2.2: Persistent failure message + extended live region
**As a** screen-reader user, **I want** the Failed transition announced via
the same already-mounted live region used for Creating progress, **so
that** the announcement reliably fires (per the existing code comment's
NVDA rationale).
**Acceptance Criteria**:
- The existing `role="status"` span (`SessionCard.tsx:951-954`) has its
  text content updated to the failure message on transition to `Failed`,
  and its `aria-live` value switches from `"polite"` to `"assertive"` at
  that moment (per UX research §3) — no second live region is mounted.
  - *Given* a session transitions `Creating → Failed` with
    `failureReason = "GitHubResolutionError"`, *When* the DOM updates,
    *Then* the same `role="status"` span's text changes to a human-readable
    message (e.g. `"Failed to resolve GitHub URL: <error>"`) and its
    `aria-live` attribute reads `"assertive"`.
  - *Given* a session transitions `Creating → Failed` with `failureReason =
    "Stale"`, *When* the DOM updates, *Then* the message reads distinctly
    (e.g. `"This session creation appears to have stalled."`), not the
    same generic text as a resolution error (per UX research §2/§4's
    "three different messages, not one generic Failed card").
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.2.2a: Add failure-message-by-reason mapping (~4 min)
- A small `getFailureMessage(failureReason: string): string` function with the three variants (`GitHubResolutionError`, `StartupError`, `Stale`) plus a generic fallback.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.2.2b: Extend the existing live region for Failed (content + `aria-live` toggle) (~4 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.2.2c: Frontend unit test asserting the live region updates in place (no remount) (~5 min)
- Files: `web-app/src/components/sessions/SessionCard.test.tsx`

### Epic 5.3: Failure toast

**Goal**: A toast fires at the moment of failure, routed through the
existing `NotificationToast`/`NotificationContext` system, with copy
distinguishing the three `FailureReason` variants.

#### Story 5.3.1: New notification variant for creation failure
**As a** user who's looking at a different part of the app when a creation
fails, **I want** a toast notice, **so that** I notice the failure even if
I'm not watching that card.
**Acceptance Criteria**:
- A new `NotificationData` variant (e.g. `sessionCreationFailed`) dispatched
  from a `useEffect` watching for `status` transitions into `FAILED` in
  Redux state (mirroring the existing effect-cluster pattern at
  `useSessionService.ts:1124-1194`).
  - *Given* a session transitions to `FAILED` while the user has the app
    open on a different page/session, *When* the transition is observed via
    the `WatchSessions` stream, *Then* a toast fires with failure-reason-
    specific copy and does not require the user to be viewing that card.
  - *Given* the toast auto-closes per `toastAutoCloseMs`, *When* it
    disappears, *Then* the session card's persistent Failed state (Epic
    5.2) remains as the durable record — the toast is not the only place
    the error is visible.
**Files**: `web-app/src/lib/hooks/useSessionService.ts`,
`web-app/src/lib/contexts/NotificationContext.tsx`,
`web-app/src/lib/notification-policy.ts`

##### Task 5.3.1a: Add the new `NotificationData` variant + copy for the three failure reasons (~5 min)
- Files: `web-app/src/lib/contexts/NotificationContext.tsx`

##### Task 5.3.1b: Add the `useEffect` dispatching the toast on `status → FAILED` transition (~5 min)
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 5.3.1c: Confirm/extend `toastAutoCloseMs` policy for this notification type (longer than routine toasts, per UX research §4) (~3 min)
- Files: `web-app/src/lib/notification-policy.ts`

### Epic 5.4: Retry/Cancel buttons on the card

**Goal**: Real `<button>` elements, accessible, positioned in the existing
progress-row real estate, wired to the two new RPCs.

#### Story 5.4.1: Cancel button on a `Creating` card
**As a** user, **I want** to cancel a session that's still creating, **so
that** I don't have to wait it out.
**Acceptance Criteria**:
- A `<button aria-label="Cancel session creation">` appears in the
  progress-row area (`SessionCard.tsx:955-961`) while `isCreating`, calls
  `CancelSessionCreation`, and (per Unresolved Questions) ships single-click
  by default.
  - *Given* a `Creating` session card, *When* the user clicks "Cancel
    session creation", *Then* the RPC is called with the instance ID and,
    on success, the card is removed from the list (instance deleted).
  - *Given* the RPC returns `FailedPrecondition` (lost the race, session
    already `Active`), *When* the response arrives, *Then* the card updates
    to show `Active` (via the normal stream event, not a stale local
    optimistic removal) rather than disappearing incorrectly.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`,
`web-app/src/lib/hooks/useSessionService.ts` (new RPC call wrapper)

##### Task 5.4.1a: Add `cancelSessionCreation` call wrapper in `useSessionService.ts` (~4 min)
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 5.4.1b: Add the Cancel `<button>` to `SessionCard.tsx`, following the snapshot-toggle button precedent (~5 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.4.1c: Playwright test — cancel removes the card; cancel-loses-race shows Active (~5 min)
- Files: `tests/e2e/session-lifecycle.spec.ts`

#### Story 5.4.2: Retry button on a `Failed` card
**As a** user, **I want** to retry a failed creation with one click, **so
that** I don't have to re-enter the omnibar.
**Acceptance Criteria**:
- A `<button aria-label="Retry creating session">` appears alongside the
  Failed message, calls `RetrySessionCreation`, and the same card
  transitions `Failed → Creating → (Active|Failed)` in place.
  - *Given* a `Failed` session card, *When* the user clicks "Retry creating
    session", *Then* the same card (same list position, same ID) shows
    `Creating` with fresh progress text, never a second new card.
  - *Given* the user double-clicks Retry rapidly, *When* both clicks are
    processed, *Then* the button disables immediately on first click
    (standard idempotent-submit-guard, per UX research §4) so the second
    click is a no-op at the UI layer, not just relying on backend dedup.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`,
`web-app/src/lib/hooks/useSessionService.ts`

##### Task 5.4.2a: Add `retrySessionCreation` call wrapper (~4 min)
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 5.4.2b: Add the Retry `<button>` with immediate-disable-on-click guard (~5 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 5.4.2c: Playwright test — retry-in-place, no duplicate card, double-click guarded (~5 min)
- Files: `tests/e2e/session-lifecycle.spec.ts`

---

## Phase 6: Registry verification, cross-cutting tests, docs

### Epic 6.1: 7-touchpoint / 7-session-type re-verification

**Goal**: Every one of the 7 session-creation touchpoints
(`.claude/docs/session-creation-registry.md`) and all 7 session-creation
modes (directory, one-off, restart, fork, alias, autonomous, remote) are
exercised through the new create-then-resolve-async path in one CI run, per
this plan's Risk Control section — the highest-priority verification given
there is no feature flag.

#### Story 6.1.1: Enumerate and test each of the 7 touchpoints against the new path
**As the** team shipping a live-critical RPC restructure with no rollback
flag, **we want** explicit, itemized proof every touchpoint still works,
**so that** a regression in the least-frequently-tested mode (e.g. restart)
doesn't ship silently.
**Acceptance Criteria**:
- For each of the 7 modes, an existing or new test creates a session of
  that type through `CreateSession` and asserts it reaches `Active` via the
  new pipeline (not just that the old synchronous assertions still pass).
  - *Given* a `restart_from_session_id` request, *When* `CreateSession` is
    called, *Then* the resulting instance reaches `Active` via the
    Background Resolution Pipeline, with `creation_progress` observed
    transitioning at least once before `Active`.
  - *Given* each of the other 6 modes (directory, one-off, fork, alias,
    autonomous, remote), *When* `CreateSession` is called for each, *Then*
    each reaches `Active` the same way, in the same test run.
**Files**: `server/services/session_service_test.go` (extend existing
per-mode tests or add a table-driven test iterating all 7)

##### Task 6.1.1a: Read `.claude/docs/session-creation-registry.md` and `.claude/docs/session-creation-registry.md`'s 7-touchpoint list; produce a literal checklist in the PR description (~4 min)
- Files: none (documentation of verification, goes in PR body)

##### Task 6.1.1b-h: One task per session-creation mode, asserting it reaches `Active` via the new pipeline (~5 min each, 7 tasks)
- directory, one-off, restart, fork, alias, autonomous, remote — each gets its own focused test or an extension of its existing test.
- Files: `server/services/session_service_test.go`

### Epic 6.2: Race/leak tests

**Goal**: No goroutine pile-up, no data race, across repeated
create/fail/cancel/retry cycles.

#### Story 6.2.1: `goleak` + `-race` hammer test
**As the** team, **we want** proof the new pipeline doesn't leak goroutines
under repeated failure/retry, **so that** a flaky-network dev session
doesn't slowly exhaust the process.
**Acceptance Criteria**:
- A test creates, fails, retries, and cancels sessions in a loop (e.g. 50
  iterations) under `go test -race`, then asserts via `goleak.VerifyNone`
  that no pipeline goroutine remains.
  - *Given* 50 iterations of create-with-injected-failure → retry → cancel,
    *When* the test completes and calls `s.Shutdown()`, *Then*
    `goleak.VerifyNone(t)` reports zero unexpected goroutines.
**Files**: `server/services/session_service_test.go`

##### Task 6.2.1a: Add the hammer test using an injectable failure point in the pipeline (~5 min)
- Files: `server/services/session_service_test.go`

##### Task 6.2.1b: Run under `-race -count=10` in CI (confirm `make ci`/`make ready` picks this up) (~3 min)
- Files: none (verification of existing Make target coverage)

### Epic 6.3: Registry + doc updates

**Goal**: `docs/registry/features/` and this repo's cross-reference docs
stay in sync with the new RPCs/components.

#### Story 6.3.1: Registry regeneration
**As the** team, **we want** `make registry-generate` run and its diff
committed, **so that** the new `CancelSessionCreation`/`RetrySessionCreation`
RPCs and any new React markers are tracked.
**Acceptance Criteria**:
- *Given* the new RPCs have `// +api:` markers and `SessionCard.tsx`
  already has its `// +feature:` marker, *When* `make registry-generate`
  runs, *Then* the per-feature JSON files under `docs/registry/features/`
  reflect the new RPCs with no manual edits needed beyond the marker
  additions themselves.
**Files**: `docs/registry/features/*.json` (generated), `server/services/session_service.go` (add `// +api:` markers)

##### Task 6.3.1a: Add `// +api: session:cancel-creation` / `// +api: session:retry-creation` markers (~2 min)
- Files: `server/services/session_service.go`

##### Task 6.3.1b: Run `make registry-generate`, commit the diff (~2 min)
- Files: `docs/registry/features/*.json`

##### Task 6.3.1c: Update `.claude/docs/session-creation-registry.md` if any of the 7 touchpoints' descriptions changed shape (~3 min)
- Files: `.claude/docs/session-creation-registry.md`
