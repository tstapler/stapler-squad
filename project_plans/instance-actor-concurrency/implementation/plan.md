# Implementation Plan: Instance Actor + Atomic Snapshot Migration (Item 2)

**Feature**: Replace `session.Instance`'s `stateMutex deadlock.RWMutex` with one actor
goroutine per `Instance` (commands via buffered channel mailbox) + a lock-free
`atomic.Pointer[InstanceSnapshot]` copy-on-write read path.
**Date**: 2026-06-30
**Status**: Ready for implementation
**Scope note**: Item 1 (the `SwitchWorkspace` reentrant-deadlock fix in
`session/instance_workspace.go` + its regression test) is already implemented and merged.
This plan covers **only Item 2** from `requirements.md`, including the R2.11-R2.18
`Registry`/`LiveInstance` lifecycle-layer scope added after the third adversarial review pass
(R2.10 is superseded — no tasks target it; Epic 2.5 implements R2.11-R2.18 instead).
**Source research**: `../research/stack.md`, `../research/features.md`,
`../research/architecture.md`, `../research/pitfalls.md`, `../research/architecture-registry.md`,
`../research/pitfalls-registry.md`

---

## Guiding constraint (read before touching Epic 5)

Per `pitfalls.md` §4: reads can migrate file-by-file (a stale read is merely stale), but
**writes cannot**. Once the actor exists and is the snapshot's publisher, any field still
written directly by an unconverted caller races the actor's next `snapshot.Store()` and
gets silently clobbered the instant any other command is processed on that `Instance` —
deterministic data loss, not a probabilistic race. Epics 1–2 (snapshot + reader migration)
are safe to land and ship independently, one PR at a time. Epic 3 only lights up the actor
for fields verified to have exactly one writer site (so "convert this field's writer" is
already a complete, atomic operation, not a partial one). **Epic 4 and Epic 5 must each
land as a single merged unit** — every story inside them is a slice of one indivisible
write-side cutover for a cluster of fields with multiple writers, broken into stories only
for implementation tractability, not for independent shipping. Do not merge story 4.1
without 4.2–4.6, and do not merge any Epic 5 story without all the others — see the
"Atomicity gate" callout at the top of each of those epics.

---

## Dependency Visualization

```
Epic 1 (additive InstanceSnapshot + buildSnapshot, mutex untouched)
    │
    ├──► Epic 2 (unguarded readers → snapshot.Load(), independently shippable per story)
    │         │
    │         └──► re-run pprof capture (Story 2.4) — confirms reader-contention fix
    │
    ├──► Epic 2.5 (Registry + LiveInstance lifecycle layer: map+mutex+refcount dedup,
    │               Acquire/release/ForceRelease/WithInstance, DI wiring, ~30+ call-site
    │               conversion to InstanceData or Registry.Acquire — independent of Epic
    │               1/2's output, but MUST land before Epic 3's Task 3.1c ships; resolves
    │               adversarial-review.md §2/§3, implements ADR-031/R2.11-R2.18,
    │               supersedes ADR-030's call-site classification)
    │
    └──► Epic 3 (actor/mailbox plumbing proven on single-writer fields — Task 3.1c's
                  actor-spawn extension is now reached only through Registry's internal
                  `newLiveInstance`, per Epic 2.5's dedup guarantee)
              │
              └──► Epic 4 (state-machine core: transitionTo/Pause/Approve/Deny/
                            StopController/hibernate-resume/SwitchWorkspace — ATOMIC UNIT)
                        │
                        └──► Epic 5 (session_service.go + background-goroutine writers
                                      — ATOMIC UNIT, depends on Epic 4's xxxLocked twins)
                                  │
                                  └──► Epic 6 (instance_tmux.go RLock-across-I/O sites —
                                                depends on Epic 4's Status semantics)
                                            │
                                            └──► Epic 7 (delete stateMutex — compiler-
                                                          enforced completeness check)
```

---

## Epic 1: Additive `InstanceSnapshot` Infrastructure

**Architecture ref**: `architecture.md` §1.2–1.3, §7 PR1. **Risk**: low-but-not-none — adding
the `snapshot` field and `buildSnapshot()` function is itself inert, and `stateMutex` stays
exactly as-is and continues to guard every existing critical section. **However**, Story 1.2's
requirement that every existing mutator additionally call the full-struct `buildSnapshot()`
inside its locked region is a **strict widening of today's unsynchronized-read race surface**,
not a no-op: today, an unconverted mutator (e.g. `Pause()`) only reads/writes the handful of
fields it itself touches while holding `stateMutex`, so it races only the unguarded writers of
those specific fields. Once every mutator also builds a full ~90-field snapshot mirror on every
call, **every** mutator invocation now reads every field on every call — including the ~28
fields `server/services/session_service.go` writes with no lock at all (confirmed at
`session_service.go:3436,3438` and others). This doesn't change any return value or persisted
state, but it can change what `go test -race` reports. Story 1.3 below exists specifically to
measure this before Epic 2 begins, so this epic's risk label is evidence-based rather than
asserted.

### Story 1.1: Define `InstanceSnapshot` and `buildSnapshot()`

**As a** developer building the read-side migration, **I want** a single struct that
mirrors `Instance`'s full mutable-field set, **so that** every future read site has one
authoritative, non-drifting source instead of an ad hoc "fields read today" allowlist
(the same allowlist drift that produced the unguarded-`InstanceToProto`/`ToInstanceData`
problem in the first place — `requirements.md` background).

**Acceptance Criteria**:
- `InstanceSnapshot` includes every field listed in `architecture.md` §1.2's struct
  (identity/config/status/GitHub-PR/checkpoint/review-state/permissions fields), excluding
  manager/dependency objects (`gitManager`, `vncManager`, `cdpManager`, `processManager`,
  `controllerManager`, `tagManager`, `shellRepo`, `historyDetector`) and callback
  registrations (`lifecycleListeners`, `onRateLimitDetected`, `onStatusChange`).
- `buildSnapshot(i *Instance) *InstanceSnapshot` deep-copies every reference-typed field
  per `stack.md`'s table: `Tags` ([]string — new backing array), `Checkpoints`
  (`CheckpointList` — new backing array, shallow element copy is fine), `Permissions`
  (`InstancePermissions` — copy `RequiresConfirmation` map key-by-key), `EnvVars`
  (`map[string]string` — new map), `ExternalMetadata`/`Artifacts` (pointer — copy the
  pointee, not just the pointer), `ArchivedAt`/`RateLimitAutoResume` (`*time.Time`/`*bool` —
  copy the pointee).
- `Instance` gains an `atomic.Pointer[InstanceSnapshot]` field named `snapshot`.
- Construction guarantee: **every** code path that builds an `*Instance` calls a single shared
  internal helper (e.g. `finishInstanceConstruction(i *Instance)`) that calls
  `i.snapshot.Store(buildSnapshot(i))` synchronously before the `*Instance` is returned to any
  caller — `Load()` must never observe `nil` (per `stack.md` §1's "fix at construction, not at
  every read site" rule). No defensive nil-checks at read sites. This helper is **not**
  optional sugar inside `NewInstance` — it must be the single choke point every construction
  site calls, because this codebase builds `*Instance` values in (at least) four independent
  places, only one of which is `NewInstance`:
  - `session/instance.go` `NewInstance` (the primary path)
  - `session/instance_serialization.go:129` `FromInstanceData(data InstanceData) (*Instance, error)`
    — deserializes every persisted session from storage; this is how every session that
    survives a server restart is reconstructed
  - `session/external_discovery.go:137` `handleNewSession(...)` — builds
    `instance := &Instance{...}` directly for externally-discovered tmux sessions (ssq-mux)
  - `session/session.go:325` `SessionToInstance(s *Session) *Instance` — legacy adapter,
    explicitly commented "enables interoperability during the migration period"

  None of the latter three call `NewInstance` internally — they all build `&Instance{...}`
  struct literals directly and return them. Each one **must** be audited during Task 1.1c and
  rewired to call the shared helper before returning. Skipping any of them leaves that
  construction path with `snapshot.Load() == nil` and, once Epic 3 lands, no actor goroutine —
  any mutation routed through `sendSync` against such an `Instance` blocks forever waiting for
  a reply from an actor that was never spawned (a guaranteed hang on every session restored via
  `FromInstanceData`, i.e. most sessions in any non-trivial running deployment).

**Files**: new `session/instance_snapshot.go`; `session/instance.go` (add `snapshot` field,
shared construction helper); `session/instance_serialization.go` (`FromInstanceData` call
site); `session/external_discovery.go` (`handleNewSession` call site); `session/session.go`
(`SessionToInstance` call site).

#### Task 1.1a: Write the `InstanceSnapshot` struct (~20 min)
- New file `session/instance_snapshot.go`. Copy the field list from `architecture.md`
  §1.2 verbatim, renaming `started` → `Started` (exported) as architecture.md specifies.
- Add a doc comment stating this struct's field set must be updated whenever a field is
  added to `Instance` — this is the one place allowed to "know about" every mutable field.

#### Task 1.1b: Write `buildSnapshot(i *Instance) *InstanceSnapshot` (~30 min)
- Same file. One function, called only from inside a `stateMutex`-held critical section in
  this epic (Story 1.2) — safe to read `i`'s fields directly here.
- Apply the deep-copy rules above field-by-field; do not skip any reference-typed field.

#### Task 1.1c: Add `snapshot atomic.Pointer[InstanceSnapshot]` to `Instance` + a single shared construction helper called from every construction site (~45 min)
- `session/instance.go`: add the `snapshot` field near `stateMutex` (line ~332).
- Write one shared internal helper, e.g. `finishInstanceConstruction(i *Instance)`, whose body
  is exactly `i.snapshot.Store(buildSnapshot(i))` (Epic 3's Task 3.1c later extends this same
  helper to also `go i.runActor()`, so this is the one choke point for both the initial
  snapshot publish and the actor spawn).
- Audit and wire **all four** known `*Instance` construction sites to call this helper as the
  last step before returning the `*Instance` to any caller:
  1. `session/instance.go` `NewInstance`
  2. `session/instance_serialization.go:129` `FromInstanceData`
  3. `session/external_discovery.go:137` `handleNewSession`
  4. `session/session.go:325` `SessionToInstance`
  Grep for `&Instance{` across `session/` and `server/` before closing this task, to confirm no
  fifth construction site exists that this list missed.
- Unit test: construct an `Instance` via **each** of the four paths above (not just
  `NewInstance`), assert `i.snapshot.Load() != nil` immediately after each.

### Story 1.2: Publish a snapshot at the end of every existing mutator

**As a** developer, **I want** every method that currently mutates `Instance` fields under
`stateMutex` to also publish a fresh snapshot before releasing the lock, **so that** Epic
2's readers have correct data to switch to before any write-path code changes.

**Acceptance Criteria**:
- Every method in `session/instance*.go` that mutates fields under `i.stateMutex.Lock()`
  gets `i.snapshot.Store(buildSnapshot(i))` added as the last statement inside the locked
  region (before `Unlock()`), not after.
- No behavior change to any method's return value, error handling, or side effects.
- A table-driven test exercises a representative sample of mutators (`Pause`, `Approve`,
  `Deny`, `transitionTo`, `UpdatePRStatus`, `SetLastMeaningfulOutput`, `MarkViewed`) and
  asserts `i.snapshot.Load()` reflects the post-call state for the field(s) each one
  touches.

**Files**: `session/instance.go`, `session/instance_state.go`, `session/instance_terminal.go`,
`session/instance_checkpoint.go`, `session/instance_workspace.go`, `session/instance_hibernate.go`,
`session/instance_controller.go`, `session/state_machine.go`.

#### Task 1.2a: Instrument `instance_state.go`'s mutators (~30 min)
- `transitionTo` (`session/instance_state.go:32`) and any other `Status`/field mutators in
  this file — add the `Store` call before each `Unlock()`.

#### Task 1.2b: Instrument `instance_terminal.go`'s `UpdatePRStatus` (~15 min)
- `UpdatePRStatus` (`session/instance_terminal.go:247`) writes 8 fields per R2.5 — add one
  `Store` call after all 8 are written, still inside the lock.

#### Task 1.2c: Instrument `instance_checkpoint.go`, `instance_workspace.go`,
`instance_hibernate.go`, `instance_controller.go` mutators (~45 min)
- Each file's lock-held mutators (`CreateCheckpoint`, `SwitchWorkspace`,
  `hibernateProcess`/`resumeFromHibernation`, `StartController`/`StopController`) get the
  same `Store` call added before `Unlock()`.
- `SwitchWorkspace` (`session/instance_workspace.go:85-219`, post-Item-1) already has 3
  `unlock()`-then-`Start()` call sites (verify current line numbers — as of this revision they
  are **162, 212, 222** post-Item-1, not the 148/197/206 originally cited in
  `requirements.md`/`pitfalls.md`; re-verify before converting since the file may shift again
  during Epic 1) — add the snapshot publish immediately before each `unlock()` call, not after,
  so the snapshot reflects state as of the lock release.

#### Task 1.2d: Write the table-driven snapshot-publish test (~30 min)
- New test in `session/instance_snapshot_test.go`: construct an `Instance`, call each
  representative mutator, assert the relevant `Load()` field updated.

### Story 1.3: Quantify Story 1.2's race-surface widening before Epic 2 starts

**As a** developer who just labeled Epic 1 "pure addition," **I want** to verify with the race
detector — not just argue from first principles — that building a full-struct snapshot inside
every lock-held mutator doesn't introduce `-race` findings beyond today's baseline, **so that**
Epic 2 doesn't build on an Epic-1 output whose transitional-state safety was only asserted, not
tested.

**Acceptance Criteria**:
- A test (new or added to an existing suite) runs one Story-1.2-converted mutator (e.g.
  `Pause()`) concurrently with one of the known-unguarded `session_service.go` direct field
  writers (e.g. the `GitHubPRURL`/`GitHubPRNumber` writes at `session_service.go:3436,3438`),
  under `go test -race`.
- `go test -race` is run once against the pre-Story-1.2 code (mutator has no `buildSnapshot`
  call — this is the baseline) and once against the post-Story-1.2 code (mutator now builds
  the full snapshot), both exercising the same concurrent-writer test; the two `-race` outputs
  are diffed.
- The finding is documented directly in this epic's PR description: either (a) no new `-race`
  findings appear beyond the pre-existing unguarded-writer race itself — the widened read
  surface is benign because the fields were already racy and are now merely read by one more
  caller, not a new bug category — or (b) new findings appear, in which case Epic 2 does not
  start until either the finding is explicitly accepted as already-known/already-tracked, or
  `buildSnapshot`'s field scope is narrowed for the transitional period (per the tradeoff
  `pitfalls.md` §4 already describes for the write side, applied here to reads).
- This story gates Epic 2's start. It does not block Epic 1's own merge — Epic 1 remains
  additive and independently revertible — but the "pure addition" framing is only valid once
  this story's finding is documented as (a) above.

**Files**: `session/instance_snapshot_test.go` (or co-located with Story 1.2's test); no
production code changes (verification-only story).

#### Task 1.3a: Write/extend a `-race` test pairing a converted mutator with a concurrent unconverted writer (~30 min)
- One goroutine calls `inst.Pause()` (or another Story-1.2-converted mutator) in a loop; a
  second goroutine directly writes `inst.GitHubPRURL`/`inst.GitHubPRNumber` (the same
  unguarded-write pattern `session_service.go:3436,3438` uses today) in a loop; run both
  concurrently for a short fixed duration or iteration count under `go test -race`.

#### Task 1.3b: Run the before/after `-race` comparison and document the result (~20 min)
- Run the Task 1.3a test against pre-1.2 and post-1.2 code; diff the `-race` output; record the
  outcome (new findings or none) in the PR description per the acceptance criteria above. Do
  not start Epic 2 until this is documented.

---

## Epic 2: Migrate Unguarded Readers to `snapshot.Load()`

**Architecture ref**: `architecture.md` §7 PR2. **Risk**: low — these read paths take **no
lock today** (`requirements.md` background), so switching to `Load()` is a strict
improvement (eliminates torn reads) and regresses nothing. Per `pitfalls.md` §4, each
story below is independently shippable.

### Story 2.1: `InstanceToProto` and `ToInstanceData`

**Acceptance Criteria**:
- `InstanceToProto` (`server/adapters/instance_adapter.go:15`) reads exclusively from
  `inst.Snapshot()` (new accessor — see Task 2.1a) instead of direct field access.
- `ToInstanceData` (`session/instance_serialization.go:22`) does the same.
- Existing tests for both functions pass unchanged (behavior-preserving).

**Files**: `server/adapters/instance_adapter.go`, `session/instance_serialization.go`,
`session/instance.go` (new `Snapshot()` accessor).

#### Task 2.1a: Add `func (i *Instance) Snapshot() *InstanceSnapshot { return i.snapshot.Load() }` (~5 min)
- `session/instance.go`, near the `snapshot` field declaration.

#### Task 2.1b: Rewrite `InstanceToProto` to read from `inst.Snapshot()` (~30 min)
- `server/adapters/instance_adapter.go:15` — replace every `inst.Field` access with
  `snap := inst.Snapshot(); snap.Field`.

#### Task 2.1c: Rewrite `ToInstanceData` to read from `i.Snapshot()` (~30 min)
- `session/instance_serialization.go:22` — same transform.

### Story 2.2: Poller read paths — `CapacityMonitor`, `ReviewQueuePoller`, `PRStatusPoller`

**Acceptance Criteria**:
- `CapacityMonitor.evaluate`/`evaluateInstance` (`server/services/capacity_monitor.go:138,153`,
  reading `inst.Status`/`inst.Program` with zero locking today per `pitfalls.md` §3) read
  from `inst.Snapshot()`.
- `ReviewQueuePoller`'s read-only paths (status checks ahead of `reconcileSessions` —
  Epic 4 owns the write side) read from `inst.Snapshot()`.
- `PRStatusPoller.fetchAndUpdatePRStatus`'s pre-update reads (`session/pr_status_poller.go:271`)
  read from `inst.Snapshot()`; the write (`UpdatePRStatus`) stays as-is until Epic 4.

**Files**: `server/services/capacity_monitor.go`, `session/review_queue_poller.go`,
`session/pr_status_poller.go`.

#### Task 2.2a: Convert `CapacityMonitor.evaluate`/`evaluateInstance` reads (~20 min)
- `server/services/capacity_monitor.go:138-209` — replace direct `inst.Status`/`inst.Program`
  reads with a single `snap := inst.Snapshot()` at the top of each function, then `snap.Field`.

#### Task 2.2b: Convert `ReviewQueuePoller`'s read-only status checks (~20 min)
- `session/review_queue_poller.go` — everywhere a field is read purely to decide whether to
  act (not the `reconcileSessions` writes themselves, which Epic 4 Story 4.3 owns).

#### Task 2.2c: Convert `PRStatusPoller.fetchAndUpdatePRStatus`'s pre-fetch reads (~15 min)
- `session/pr_status_poller.go:271-368` — read `inst.Snapshot()` for the precondition/
  comparison fields; leave the actual `UpdatePRStatus` write call alone (Epic 4 Story 4.4
  converts it, since it's also a TOCTOU fix per `pitfalls.md` §3).

### Story 2.3: `ConnectRPCWebSocketHandler` streaming reads

**Acceptance Criteria**:
- The per-connection WebSocket streaming handler's `Instance` field reads
  (`server/services/connectrpc_websocket.go`) go through `inst.Snapshot()`.

**Files**: `server/services/connectrpc_websocket.go`.

#### Task 2.3a: Locate and convert the handler's direct field reads (~30 min)
- Use `ast-grep`/`grep` for `inst\.\w+\b` or `i\.\w+\b` field access in this file outside
  any existing lock, replace with `snap := inst.Snapshot(); snap.Field`.

### Story 2.4: Re-run the pprof mutex/block profile capture

**Acceptance Criteria**:
- Run `./stapler-squad --profile --trace` (per `.claude/docs/profiling.md`) under
  representative session load, capture `go tool pprof`'s mutex/block profile.
- Confirm reader-side contention attributed to `stateMutex.RLock()` on the migrated paths
  (Story 2.1–2.3) has dropped relative to the original investigation's ~169s cumulative
  reader-wait baseline (`requirements.md` background) — full elimination isn't expected yet
  since writers still hold `stateMutex`, but the reader-pileup-behind-`StopController()`
  symptom that started this investigation should visibly improve.

**Files**: none (verification step only — capture results inform whether Epic 3+ priority
holds, not a code change).

#### Task 2.4a: Capture and compare before/after mutex profiles (~30 min)
- Document the before/after comparison inline in the PR description for Story 2.1-2.3's PR.

---

## Epic 2.5: `Registry` + `LiveInstance` Lifecycle Layer (replaces the Group A/B call-site classification)

**Supersedes**: the previous revision of this epic (Group A/B classification + per-call-site
`Stop()`-after-use), which `implementation/adversarial-review.md` (third pass, §2/§3) found
does not close the leak it claims to close and additionally has a duplicate-actor correctness
hazard at `daemon/daemon.go:292`. **Implements**:
`../decisions/ADR-031-registry-live-instance-type-split.md` (supersedes `ADR-030`'s
*implementation*, not its rejection of pure decoupled activation), `requirements.md`
R2.11-R2.18. **Resolves**: `adversarial-review.md` §2 (the partial `Stop()`-after-use fix) and
§3 (both "no fix needed" dispositions were wrong — `daemon.go:292` and `loadInstancesWithWiring`).
**Also resolves the fourth adversarial review pass's five blocking findings** (Story 2.5.3's
`onConstruct` hook + Task 2.5.7g for finding 1; Task 2.5.7i for finding 2; Story 2.5.3's blocking
`stopActor()` note + Epic 3 Task 3.1b for finding 3; Epic 3 Task 3.1a/3.1b's `select`-based
send + `drainMailboxOnStop` for finding 4; Task 2.5.7h's `Register`-before-`storage.AddInstance`
ordering for finding 5; Story 2.5.8's `InstanceAcquirer`-typed field for the non-blocking finding
6).
**Also resolves adversarial-review.md's fifth pass, finding 2** (`Register`/`storage.AddInstance`
rollback — distinct from the fourth pass's identically-numbered finding 2 above; disambiguated
throughout this plan as "fifth-pass ... finding 2"): Task 2.5.7h's `ForceRelease`-based rollback
clause and its two new unit tests. The fifth pass's finding 1 (select-race steady-state hang) is
Epic 3's to close, not this epic's — see Epic 3's header below.
**Gates**: Epic 3's Task 3.1c (the actor-spawn extension) — see the Sequencing note below for
why `Registry` can land before the actor itself exists. Independent of Epic 1/2's output (never
touches `InstanceSnapshot`/`stateMutex`), so it can be developed in parallel with Epics 1-2 as
long as it completes first.

**Naming note**: R2.12 permits either renaming `Instance` or wrapping it in a new type. This
plan renames `session.Instance` → `session.LiveInstance` (mechanical rename, no method-body
changes) so "the actor-owning handle" has a name distinct from the read-only `InstanceData`,
per ADR-031. Epics 1, 3, 4, 5, 6, 7 in this plan were all written against `*Instance`/`Instance`
— those sections are **not** rewritten by this epic; read every `*Instance`/`Instance`
reference in them as `*LiveInstance`/`LiveInstance` once this epic merges. The type, its
fields, and every method those epics describe are unchanged by the rename — only the exported
name changes, and (per Story 2.5.2) so does who is allowed to construct one.

**Sequencing note**: `Registry`'s dedup guarantee ("at most one live construction per session
ID") is orthogonal to whether `LiveInstance` is still `stateMutex`-based (true through Epic 3)
or actor-based (Epic 3+) internally — the adversarial review's actual finding
(`daemon.go:292` building a second `*Instance` for an already-tracked session) is a
*construction*-multiplicity bug, independent of actor timing. At this epic's merge point,
`Registry.Acquire`'s internal `newLiveInstance` is a thin wrapper around today's
`FromInstanceData`, and its paired teardown (`stopActor()`, so named because Epic 3 gives it
real meaning) is just "drop the reference, let GC reclaim" — there is no live goroutine to stop
yet. Epic 3's Task 3.1c later extends `newLiveInstance` to also `go i.runActor()` and gives
`stopActor()` its real cancel-and-drain body; `Registry`'s own map/refcount logic does not
change at all when that happens. This is why `Registry` can land in its current position
(before Epic 3) without waiting on actor plumbing that doesn't exist yet.

### Story 2.5.1: Lightweight `InstanceData` read path + full call-site catalog

**As a** developer resolving the adversarial review's blockers, **I want** a query path that
returns read-only session data without ever constructing a full `*LiveInstance`, plus an
authoritative, re-verified catalog of every construction call site, **so that** Story 2.5.6/2.5.7
have a single source of truth for which sites need `InstanceData` and which need `Registry`.

**Acceptance Criteria**:
- No new struct is introduced. `session.InstanceData` (`session/storage.go:17`) already carries
  every field the cataloged Group A call sites need, and `Storage.ListInstanceData()`
  (`session/storage.go:359`) already returns `[]InstanceData` with **no** call to
  `FromInstanceData` and therefore no `Start()` side effect.
- New method `func (d InstanceData) MatchesID(id string) bool` replicates `Instance.MatchesID`'s
  (`session/instance_terminal.go:37-45`) Title/stable-ID arms; the `GetTmuxSessionName()` arm is
  intentionally not replicated (no cataloged call site needs it — document the omission).
- New `func (s *Storage) FindInstanceDataByID(id string) (*InstanceData, error)`: calls
  `ListInstanceData()`, linear-scans with `MatchesID`, returns `(nil, nil)` on no match,
  `(nil, err)` on a storage error, `(&data, nil)` on a match. **Pointer-returning, not the
  `(InstanceData, bool, error)` shape the superseded revision of this story specified** —
  this is the signature `Registry.Acquire` (Story 2.5.3) actually calls
  (`architecture-registry.md` §1: `if data == nil { return nil, nil, ErrSessionNotFound }`);
  reconciling the two designs, the pointer-return shape wins since `Registry` is now the
  primary caller.
- New `func (s *Storage) ListInstanceIDs() ([]string, error)`: calls `ListInstanceData()`, maps
  each row to `GetStableID()`. Used by `Registry.AcquireAll()` (Story 2.5.3).
- Full catalog of every `LoadInstances()`/`FromInstanceData` call site — the ~20 the
  adversarial review's second pass cited, plus the two sites its third pass found were wrongly
  dispositioned "no fix needed" (`daemon/daemon.go:292`, `session_service.go:338`
  `loadInstancesWithWiring`), plus `server/dependencies.go:438`'s canonical registry
  construction (no longer out of scope — see Story 2.5.5) — is written into this story's PR
  description, split into Group A (Story 2.5.6) and Group B (Story 2.5.7).

**Files**: `session/storage.go` (or new `session/instance_data.go`).

#### Task 2.5.1a: Add `InstanceData.MatchesID` (~20 min)
- Table-driven test: match-by-Title, match-by-UUID, no-match.

#### Task 2.5.1b: Add `Storage.FindInstanceDataByID` (pointer-returning) + `Storage.ListInstanceIDs` (~30 min)
- Unit tests against a fake `Repository`: match/no-match/error propagation for the former;
  correct ID list for the latter.

#### Task 2.5.1c: Re-catalog every call site into Group A / Group B (~45 min)
- **Group A** (converted in Story 2.5.6, unchanged from the superseded revision's catalog):
  `server/services/github_service.go:32`, `server/services/unfinished_work_service.go:60,88`,
  `server/services/session_service.go:3251` (`BatchCreateSessions`),
  `server/mcp/tools_discovery.go:101,164,210`, `server/mcp/tools_vcs.go:158`,
  `server/mcp/tools_goal.go:267,282`, `server/mcp/tools_terminal.go:182,390`,
  `server/services/session_image_upload_handler.go:137`,
  `server/services/backlog_service.go:1065`, `main.go:510` — re-verify each line number.
- **Group B** (converted in Story 2.5.7): `server/services/workspace_service.go:76`,
  `server/services/session_service.go:1626,1686`, `server/mcp/tools_lifecycle.go:332,385,459`,
  `server/mcp/tools_terminal.go:690`, `server/services/terminal_websocket.go:49`,
  `session/health.go:47,201`, `session/hibernation_sweeper.go:211` — same shape as the
  superseded revision — **plus, newly in scope per the third review's §3**:
  `server/services/session_service.go:338` (`loadInstancesWithWiring`, reused by ~10 call sites
  at `887,1000,1407,1874,1971,2461,2531,2906`) and `daemon/daemon.go:292`
  (`detectAndAddNewSessions`). The third review found both of these were wrongly dispositioned
  "no fix needed, legitimate secondary registry" by the superseded revision; both get the same
  `Registry.Acquire` conversion as every other Group B site, not a bespoke exemption.
- `main.go:422` (`test-pty` debug CLI) needs no conversion — one-shot process exit, moot per the
  original catalog's finding; note it and move on.

---

### Story 2.5.2: `session.LiveInstance` type split

**As a** developer closing the two-roles-in-one-type defect at the root (`ADR-031`), **I want**
the actor-owning handle to be its own named type, obtainable only through `Registry`, **so
that** "does this caller hold a disposable read or the one live handle" is a type-level fact,
not a comment.

**Acceptance Criteria**:
- `session.Instance` renamed to `session.LiveInstance` (mechanical — no method bodies change).
- No exported constructor for a live handle remains reachable outside the `session` package
  after this epic: `NewInstance`/`FromInstanceData`/`handleNewSession`/`SessionToInstance`
  (Epic 1 Task 1.1c's four construction sites) remain, but the only caller of each becomes
  `session.Registry`'s own internal construction path (Story 2.5.3). Grep for
  `session.NewInstance(`, `session.FromInstanceData(`, `session.SessionToInstance(` outside
  `session/` before closing this task — any hit outside `session/registry.go` is a Group A/B
  site Story 2.5.6/2.5.7 missed.
- **Resolved (R2.18a)**: session **creation** (a brand-new, not-yet-persisted session) calls
  `NewInstance` directly and hands the result to `Registry.Register(instance *LiveInstance) (ReleaseFunc, error)`
  — a sibling method to `Acquire`, not an extended `Acquire` (that would overload `Acquire`
  with a third "constructing now" outcome its other callers would need to special-case).
  `Register` mirrors `Acquire`'s dedup check but skips the storage lookup. Implemented in Epic
  2.5 Task 2.5.7h.

**Files**: `session/instance*.go` (rename only), every caller (mechanical, compiler-enumerated).

#### Task 2.5.2a: Rename `Instance` → `LiveInstance` across `session/` (~1 hr, mechanical)
- `go build ./...` enumerates every caller needing an update.

#### Task 2.5.2b: Confirm no exported live-handle constructor remains reachable outside `session.Registry` (~20 min)
- Grep per the acceptance criteria above; document the creation-path open question for
  Story 2.5.7 to resolve.

---

### Story 2.5.3: `Registry` core type — map+mutex+refcount, `Acquire`/`release`/`ForceRelease`/`AcquireAll`/`Shutdown`

**As a** developer making "two live actors for one session" structurally impossible, **I want**
a single dedup point in front of every `LiveInstance` construction, **so that** `daemon.go`'s
duplicate-actor hazard and `loadInstancesWithWiring`'s hot-path leak (`adversarial-review.md`
§3) cannot recur regardless of which future call site is added.

**Acceptance Criteria** (merges `architecture-registry.md` §1's base design,
`pitfalls-registry.md` §2's idempotent-release requirement, and `pitfalls-registry.md` §3's
Design A acquire-during-teardown synchronization — the three research sketches are reconciled
into one implementation below):

```go
// session/registry.go

// ReleaseFunc is returned by Acquire/Register: refcount-gated, safe to call from any holder,
// any number of times (idempotent — see makeRelease below). type-driven-audit.md finding B:
// a distinct named type from ForceReleaseFunc so the two are no longer interchangeable-looking
// bare func()s once stored generically (e.g. ReviewQueuePoller.releases map[string]func(),
// daemon.go's releases *[]func()) — nothing about a bare func() signals which teardown
// semantics it carries, which is exactly the ambiguity that made "call release() instead of
// ForceRelease()" the wrong-but-plausible choice at Task 2.5.7h's CreateSession rollback site.
type ReleaseFunc func()

// ForceReleaseFunc marks an unconditional-teardown closure: evicts every holder regardless of
// refcount. Never store where a ReleaseFunc is expected — assigning one to the other without
// an explicit conversion is a compile error, by design. No wrapper of ForceRelease exists yet
// in this plan (ForceRelease is always called directly with a sessionID, never wrapped into a
// closure) — this type exists so that if a future caller *does* wrap it, the wrapper's return
// type says so instead of silently degrading to bare func(). Deliberately NOT given a
// reason-argument (e.g. ForceReleaseFunc(reason ForceReleaseReason)) — type-driven-audit.md
// finding B confirmed ForceRelease has exactly two callers in this whole plan (DeleteSession's
// force-invalidate, CreateSession's abort-on-AddInstance-failure), both already documented at
// the call site; a reason-argument would mostly duplicate that doc comment for two callers,
// which is the over-engineering this skill warns against.
type ForceReleaseFunc func()

type Registry struct {
    storage *Storage
    mu      sync.Mutex
    entries map[string]*registryEntry
    // onConstruct, if non-nil, is invoked exactly once per genuinely-new LiveInstance —
    // i.e. only on the branch that actually installs a fresh entry into r.entries. It is
    // NEVER invoked on a refcount++ of an already-live entry, and NEVER invoked on the
    // losing side of a construction race (that branch discards its LiveInstance via
    // stopActor() instead — see Acquire below). This is Registry's injection point for
    // caller-supplied post-construction wiring, since Registry itself has no reference to
    // SessionService and cannot perform SessionService-bound wiring
    // (adversarial-review.md finding 1 / Story 2.5.7g). Register (below) deliberately does
    // NOT invoke onConstruct — see Register's doc comment for why.
    onConstruct func(*LiveInstance)
}
type registryEntry struct {
    instance *LiveInstance
    refcount int
}
var ErrSessionNotFound = errors.New("session: not found in storage")
var ErrSessionAlreadyRegistered = errors.New("session: already registered")

// NewRegistry constructs a Registry. onConstruct may be nil (e.g. daemon.go's own Registry,
// Task 2.5.7f/2.5.7i — the daemon process has no SessionService to wire callbacks for);
// Acquire nil-checks before calling it.
func NewRegistry(storage *Storage, onConstruct func(*LiveInstance)) *Registry {
    return &Registry{storage: storage, entries: make(map[string]*registryEntry), onConstruct: onConstruct}
}

func (r *Registry) Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error) {
    r.mu.Lock()
    if e, ok := r.entries[sessionID]; ok {
        e.refcount++
        r.mu.Unlock()
        return e.instance, r.makeRelease(sessionID), nil
    }
    r.mu.Unlock() // release before storage I/O — construction must not block unrelated Acquires

    data, err := r.storage.FindInstanceDataByID(sessionID)
    if err != nil { return nil, nil, fmt.Errorf("registry: acquire %q: %w", sessionID, err) }
    if data == nil { return nil, nil, ErrSessionNotFound }
    live, err := newLiveInstance(*data, r.storage)
    if err != nil { return nil, nil, fmt.Errorf("registry: construct %q: %w", sessionID, err) }

    r.mu.Lock()
    if e, ok := r.entries[sessionID]; ok {
        live.stopActor() // lost the construction race — discard ours, adopt the winner's
        e.refcount++
        r.mu.Unlock()
        return e.instance, r.makeRelease(sessionID), nil
    }
    r.entries[sessionID] = &registryEntry{instance: live, refcount: 1}
    r.mu.Unlock() // unlock before onConstruct — wiring may do non-trivial work, must not run under r.mu
    if r.onConstruct != nil {
        r.onConstruct(live) // exactly once: this is the sole genuine-construction branch
    }
    return live, r.makeRelease(sessionID), nil
}

// Register is the construction-time counterpart to Acquire (R2.18a): CreateSession builds a
// brand-new *LiveInstance via NewInstance (no persisted row exists yet for Acquire to look
// up) and hands it to Register instead of going through Acquire's storage-lookup path.
//
// Register deliberately does NOT invoke onConstruct: CreateSession already performs its own
// explicit post-construction wiring immediately after NewInstance
// (session_service.go:1287-1291 today, unchanged by this epic) — routing Register through
// onConstruct too would wire the same 5 SessionService callbacks twice on every session
// creation. onConstruct exists solely to backfill that wiring for the Acquire-from-storage
// path (Story 2.5.7g), which has no other caller positioned to do it.
//
// No double-checked locking here (unlike Acquire): Register has no storage I/O to release
// the lock around, so the whole check-then-insert runs under one lock acquisition.
func (r *Registry) Register(instance *LiveInstance) (ReleaseFunc, error) {
    sessionID := instance.GetStableID()
    r.mu.Lock()
    if _, ok := r.entries[sessionID]; ok {
        r.mu.Unlock()
        return nil, fmt.Errorf("registry: register %q: %w", sessionID, ErrSessionAlreadyRegistered)
    }
    r.entries[sessionID] = &registryEntry{instance: instance, refcount: 1}
    r.mu.Unlock()
    return r.makeRelease(sessionID), nil
}

// makeRelease: one sync.Once per Acquire call. A double-call on THIS acquisition's release
// is a no-op (pitfalls-registry.md §2); two independent Acquires each get their own,
// independently-firing release closure.
func (r *Registry) makeRelease(sessionID string) ReleaseFunc {
    var once sync.Once
    return func() { once.Do(func() { r.release(sessionID) }) }
}

// release: decrement/zero-check/map-delete share one critical section with Acquire's own
// check-then-increment (pitfalls-registry.md §3 Design A) — a concurrent Acquire either runs
// first (sees the entry, increments; this release's zero-check then sees refcount>0 and skips
// teardown) or this release's section runs first (deletes the entry; the concurrent Acquire
// then finds nothing and builds an independent new one — correct, not a duplicate). stopActor()
// runs outside the lock but MUST NOT RETURN until the actor's run loop has actually exited —
// not merely "cancel() and return" — because release()'s guarantee ("an Acquire for this ID
// after release() returns is guaranteed a fresh, independent LiveInstance, never one still
// doing tmux I/O") depends on it (adversarial-review.md finding 3; see Epic 3 Task 3.1b/3.1c
// for stopActor()'s exact blocking implementation — a `done chan struct{}` closed by the run
// loop, awaited here via `i.cancel(); <-i.done`).
func (r *Registry) release(sessionID string) {
    r.mu.Lock()
    e, ok := r.entries[sessionID]
    if !ok {
        r.mu.Unlock()
        log.Warn("registry: release for unknown sessionID", "id", sessionID)
        return
    }
    e.refcount--
    if e.refcount > 0 { r.mu.Unlock(); return }
    delete(r.entries, sessionID)
    r.mu.Unlock()
    e.instance.stopActor()
}

// ForceRelease tears down sessionID's actor/map-entry immediately, regardless of refcount
// (R2.18 — DeleteSession's force-invalidate, Story 2.5.9). Other holders' *LiveInstance
// pointers stay valid Go values; their next send()/sendSync() must return a typed error
// (Story 2.5.9), never hang.
//
// Also used by CreateSession (Task 2.5.7h, fifth-pass adversarial-review.md finding 2) to
// abort a Register()'d entry when the immediately-following storage.AddInstance fails: the
// entry was never confirmed, so an unconditional teardown is required rather than the
// refcount-gated release() Register itself returns — a concurrent Acquire racing in between
// Register succeeding and this abort running would otherwise leave a phantom, unpersisted
// entry alive at refcount 1 after a plain release() only decremented it from 2.
func (r *Registry) ForceRelease(sessionID string) {
    r.mu.Lock()
    e, ok := r.entries[sessionID]
    if !ok { r.mu.Unlock(); return }
    delete(r.entries, sessionID)
    r.mu.Unlock()
    e.instance.stopActor()
}

// AcquireAll acquires every session known to Storage in one call; returns one release
// closing over all of them — sugar for sweep-style callers (health.go, hibernation_sweeper.go).
func (r *Registry) AcquireAll() ([]*LiveInstance, ReleaseFunc, error) {
    ids, err := r.storage.ListInstanceIDs()
    if err != nil { return nil, nil, err }
    var live []*LiveInstance
    var releases []ReleaseFunc
    for _, id := range ids {
        l, release, err := r.Acquire(id)
        if err != nil { log.Warn("AcquireAll: skipping", "id", id, "err", err); continue }
        live = append(live, l)
        releases = append(releases, release)
    }
    return live, func() { for _, release := range releases { release() } }, nil
}

// Shutdown force-stops every actor regardless of refcount — the ADR-029 shutdownHooks safety
// net, registered once in Story 2.5.5's DI wiring.
func (r *Registry) Shutdown() {
    r.mu.Lock()
    entries := r.entries
    r.entries = make(map[string]*registryEntry)
    r.mu.Unlock()
    for _, e := range entries { e.instance.stopActor() }
}

func (r *Registry) Storage() *Storage { return r.storage } // used by daemon.go's fix, Story 2.5.7
```

- **`ReleaseFunc`/`ForceReleaseFunc` named types** (`type-driven-audit.md` finding B): `Acquire`
  and `Register` return `ReleaseFunc`, not a bare `func()` — closes the risk that a future call
  site conflates a refcount-gated `release()` with an unconditional `ForceRelease` once either
  is stored generically (e.g. `ReviewQueuePoller.releases map[string]session.ReleaseFunc`,
  Story 2.5.8). `ForceRelease` itself keeps its own distinct signature (`func (r *Registry)
  ForceRelease(sessionID string)`, no closure returned) rather than being retrofitted to return
  a `ForceReleaseFunc` — it has exactly two callers today (`DeleteSession`'s force-invalidate,
  `CreateSession`'s abort-on-`AddInstance`-failure per Task 2.5.7h), both calling it directly
  with a session ID, never as a stored closure; `ForceReleaseFunc` exists only so that if a
  future wrapper of `ForceRelease` is ever introduced, its return type says so explicitly
  instead of silently degrading to the same bare `func()` shape `ReleaseFunc` uses. No
  reason-argument (e.g. a `ForceReleaseReason` parameter) is added to either type — two
  documented callers don't justify it; this is a discipline fix via distinct types, not a
  runtime-checked justification mechanism.
- `sync.Mutex`, not `RWMutex` — every `Acquire`/`release` is a write (refcount mutation), so
  there is no read-dominant population to justify a split lock (`architecture-registry.md` §1).
- `stopActor()` at this epic's merge point is a no-op-ish teardown (no actor exists yet — see
  the epic header's Sequencing note); Epic 3 Task 3.1c gives it its real body. **Even though
  there is no real actor to wait for yet, Epic 3 Task 3.1c's `stopActor()` body must block on a
  `done` channel closed by the run loop, not merely call `cancel()`** — see the ADR-029
  reconciliation note in Epic 3 Story 3.1 below (adversarial-review.md finding 3).
- **`onConstruct`'s invocation-once guarantee** (adversarial-review.md finding 1): `Acquire`
  calls `r.onConstruct(live)` exactly once — only in the branch that installs a brand-new
  `registryEntry` into `r.entries` (i.e. neither the top-of-function refcount++ hit, nor the
  losing side of the double-checked-locking race, which discards `live` via `stopActor()`
  instead). `Register` never calls `onConstruct` at all (see `Register`'s doc comment above).
  `SessionService` supplies the hook at `NewRegistry(storage, svc.WireInstanceCallbacks)` time
  (Story 2.5.5) — a new exported `SessionService.WireInstanceCallbacks(inst *session.LiveInstance)`
  method that replicates exactly what `loadInstancesWithWiring` does today for each instance
  (`SetReviewQueue`, conditional `SetStatusManager`, the 5 `wireX` calls, the `MCPServerURL`
  backfill — `session_service.go:341-355` today), extracted verbatim so Story 2.5.7g's
  `loadInstancesWithWiring` conversion has this wiring run automatically for every session its
  ~10 former callers now `Acquire`/`WithInstance` instead of losing it.

**Files**: new `session/registry.go`.

#### Task 2.5.3a: Implement `Registry`/`registryEntry`/`NewRegistry`/`Acquire`/`makeRelease`/`release` (~1.5 hr)
- Unit tests for the three `Acquire` outcomes (not-found, fresh-construct, refcount++) using a
  fake `Storage`.
- Unit test for `onConstruct`: fires exactly once for a fresh construction; does **not** fire on
  a second `Acquire` for the same already-live ID (refcount++ path); does **not** fire on the
  discarded loser of a construction race (use a call-counting fake to assert this directly, not
  just goroutine count — mirrors Story 2.5.10's Test 4 shape).

#### Task 2.5.3b: Implement `ForceRelease`, `AcquireAll`, `Shutdown`, `Storage()` accessor (~1 hr)
- Unit tests: `ForceRelease` removes the entry regardless of a >1 refcount; `AcquireAll`
  skips-and-logs on a per-ID error rather than failing the whole batch; `Shutdown` empties the
  map and stops every entry.

#### Task 2.5.3c: Implement `Registry.Register` + `ErrSessionAlreadyRegistered` (~30 min)
- Implements R2.18a's construction-time counterpart to `Acquire`, used by `CreateSession`
  (Task 2.5.7h). Unit tests: registering a fresh session ID succeeds at refcount 1; registering
  an already-present session ID returns `ErrSessionAlreadyRegistered` without mutating the
  existing entry's refcount; `onConstruct` (if set) is **never** invoked by `Register` in either
  case (assert via a call-counting fake, since this is the one place in `Registry` that
  deliberately does not call the hook — see `Register`'s doc comment for why).

---

### Story 2.5.4: `InstanceAcquirer`/`RegistryInspector` interfaces (R2.17) + `WithInstance` (pitfalls-registry.md §1)

**As a** developer whose call site only ever needs "give me the live handle for this ID," **I
want** the narrowest possible interface and a helper that makes forgetting `release()`
structurally impossible for the common synchronous case, **so that** the centralization
ADR-031 promises isn't undone by the same "remember to call X" discipline that caused the
original `stateMutex` problem.

**Acceptance Criteria**:
```go
type InstanceAcquirer interface {
    Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error) // ReleaseFunc: Story 2.5.3 (type-driven-audit.md finding B)
}
type RegistryInspector interface {
    List() []*LiveInstance
    Count() int
}
func (r *Registry) List() []*LiveInstance  // snapshot of entries' instances, lock held only for the copy
func (r *Registry) Count() int             // len(r.entries), lock held only for the read

func (r *Registry) WithInstance(ctx context.Context, sessionID string, fn func(*LiveInstance) error) error {
    inst, release, err := r.Acquire(sessionID)
    if err != nil { return err }
    defer release()
    return fn(inst)
}
```
- `*Registry` satisfies both interfaces; nothing forces combining them (a single-handle caller
  is typed against `InstanceAcquirer` only, never handed `List`/`Count`) — matches
  `WorkspaceService`'s existing 1-method `LiveInstanceFinder` convention (`workspace_service.go:32-34`).
- `WithInstance` is the **preferred** entry point for every synchronous, single-call-stack Group
  B site (Story 2.5.7); raw `Acquire`/`release()` is reserved for genuinely long-lived holders
  (`terminal_websocket.go`, `ReviewQueuePoller`'s cache, `AutonomousDriver`'s background
  goroutine).
- CI grep guard (defense in depth, per `pitfalls-registry.md` §1): a `make`-wired check
  requiring a `release(` call in the same function body as any `\.Acquire\(` — won't catch
  every control-flow shape, but catches the common regression pre-merge, same spirit as
  `pitfalls.md` §4's unguarded-write check.

**Files**: `session/registry.go` (interfaces, `List`/`Count`/`WithInstance`), new CI script +
`Makefile` target.

#### Task 2.5.4a: Add `InstanceAcquirer`/`RegistryInspector` + `List`/`Count` (~30 min)

#### Task 2.5.4b: Add `WithInstance` (~20 min)
- Unit test: error path (not-found) returns before `fn` runs; success path calls `fn` and
  releases exactly once even if `fn` panics (`defer release()`, not a bare call).

#### Task 2.5.4c: Write and wire the CI grep guard (~30 min)
- Test against a deliberately-forgotten-`release()` fixture (should fail) and the converted
  Story 2.5.7 call sites (should pass).

---

### Story 2.5.5: DI wiring — construct `Registry` in `BuildServiceDeps`, inject into every consumer

**As a** developer following this repo's existing manual-DI convention, **I want** `Registry`
built once and threaded through the same staged `BuildCoreDeps → BuildServiceDeps →
BuildRuntimeDeps → BuildDependencies` pipeline every other shared, stateful dependency uses,
**so that** there's no new architectural idiom and no package-level global
(`architecture-registry.md` §2).

**Acceptance Criteria**:
- `Registry` constructed in `BuildServiceDeps` (`server/dependencies.go:322-353`), not
  `BuildCoreDeps` (too early — no lifecycle managers live there) or `BuildRuntimeDeps` (too late
  — `dependencies.go:438`'s startup enumeration, converted below, needs `Registry` to already
  exist).
- `Registry` is constructed with `SessionService.WireInstanceCallbacks` as its `onConstruct` hook
  (Story 2.5.3's new field): `registry := session.NewRegistry(core.Storage,
  core.SessionService.WireInstanceCallbacks)`. `SessionService` is already available at
  `BuildServiceDeps` time (it's part of `CoreDeps`), so this ordering requires no new staging.
  `WireInstanceCallbacks` (new exported method on `SessionService`, added by Story 2.5.7g) is the
  extracted body of today's `loadInstancesWithWiring` per-instance wiring block — this is what
  closes adversarial-review.md finding 1 (the 5 dropped callback wirings) at the DI-wiring level.
- `ServiceDeps` gains a `Registry *session.Registry` field; `RuntimeDeps` (already embedding
  `*ServiceDeps`) gets `rt.Registry` for free.
- `Registry` added to `ToServerDeps()` (`dependencies.go:90-123`) so `NewServerWithDeps` can
  wire it into every consuming service's constructor: `SessionService`, `WorkspaceService`,
  `GitHubService`, `UnfinishedWorkService`, the MCP tool handlers, `HealthChecker`,
  `HibernationSweeper` (R2.15) — constructor-parameter injection, matching this repo's existing
  `warren.Set` convention, no package-level global. **`daemon.Daemon` is removed from this list**
  (adversarial-review.md finding 2, confirmed against current code: no `Daemon` type exists —
  `RunDaemon`/`detectAndAddNewSessions` are free functions in `daemon/daemon.go`, and the
  `daemon` package is never imported by `server/dependencies.go`/`server/server.go` — the
  `--daemon` process is a wholly separate invocation from `main.go` that never enters this DI
  graph. `daemon/daemon.go`'s own `Registry` is constructed independently — see Story 2.5.7's new
  Task 2.5.7i, not this story.
- `BuildRuntimeDeps` Step 5 (`dependencies.go:438`'s startup enumeration, previously flagged
  "out of scope" by the superseded revision) is rewritten to use `Registry.Acquire` per-ID
  instead of a raw `LoadInstances()`/`FromInstanceData` loop:
  ```go
  dataList, err := storage.ListInstanceData()
  for _, data := range dataList {
      live, release, err := svc.Registry.Acquire(data.GetStableID())
      if err != nil { log.Warn("startup: acquire failed", "session", data.Title, "err", err); continue }
      instances = append(instances, live)
      releases = append(releases, release) // held for process lifetime — canonical long-lived owner
  }
  ```
- `Registry.Shutdown` registered as a `shutdownHooks` entry (`server/server.go:51,173,387,393,400`'s
  existing pattern) — this **is** ADR-029's secondary safety-net trigger; Epic 3 Task 3.1e must
  not register a second, competing entry that also iterates `ReviewQueuePoller.GetInstances()`.

**Files**: `server/dependencies.go`, `server/server.go`.

#### Task 2.5.5a: Construct `Registry` in `BuildServiceDeps` with `SessionService.WireInstanceCallbacks` as its `onConstruct` hook; add to `ServiceDeps`/`ToServerDeps()` (~30 min)

#### Task 2.5.5b: Rewrite `BuildRuntimeDeps` Step 5 (dependencies.go:438) to use `Registry.Acquire` (~30 min)
- These releases are held for the process's lifetime (the canonical long-lived registry owner)
  — store them alongside `instances` wherever that slice already lives.

#### Task 2.5.5c: Thread `Registry`/`InstanceAcquirer`/`RegistryInspector` into consumer constructors (~1 hr)
- Each consumer takes the narrowest interface it actually needs (most: `InstanceAcquirer`;
  enumeration callers: `RegistryInspector`). `daemon.go` is explicitly **not** one of these
  consumers — see Task 2.5.7i.
- **Clarification, not a bug (`type-driven-audit.md` finding E)**: `WithInstance` (Story 2.5.4)
  is a concrete method on `*Registry`, not part of the `InstanceAcquirer` interface (which has
  exactly one method, `Acquire`) — so "most consumers take the narrowest interface"
  above does not mean *every* consumer can be typed against `InstanceAcquirer`. Any consumer
  that needs to call `.WithInstance(...)` — `tools_lifecycle.go`'s conversion (Task 2.5.7c) is
  the concrete example — must be typed against `*Registry` or a wider interface that includes
  `WithInstance`, not `InstanceAcquirer` alone. This is self-resolving (the compiler rejects a
  `.WithInstance(...)` call on an `InstanceAcquirer`-typed field immediately), so it isn't a
  design defect, but call it out here rather than letting Task 2.5.7c's implementer discover it
  via a failed build.

#### Task 2.5.5d: Register `Registry.Shutdown` as a shutdownHooks entry (~15 min)

---

### Story 2.5.6: Convert Group A call sites to `InstanceData`

(unchanged in substance from the superseded revision's Stories 2.5.2/2.5.3 — these conversions
were never the adversarial review's objection; folded in here for a single authoritative
call-site story.)

**Acceptance Criteria**: every Group A site (Task 2.5.1c's list) replaces `LoadInstances()` +
linear scan with `FindInstanceDataByID`/`ListInstanceData()`; no behavior change; existing
tests pass; a regression assertion (call-counting fake `Repository`, or absence of a
`Start()`-triggered side effect) proves `FromInstanceData` is no longer invoked.

**Files**: `server/services/github_service.go`, `server/services/unfinished_work_service.go`,
`server/services/session_service.go` (`BatchCreateSessions` only), `server/mcp/tools_discovery.go`,
`server/mcp/tools_vcs.go`, `server/mcp/tools_goal.go`, `server/mcp/tools_terminal.go` (2 of 3
functions), `server/services/session_image_upload_handler.go`, `server/services/backlog_service.go`,
`main.go`, `session/pr_tracking.go`.

#### Task 2.5.6a: Convert `unfinished_work_service.go`'s two index builders (~20 min)

#### Task 2.5.6b: Convert `session_service.go`'s `BatchCreateSessions` pre-check (~15 min)

#### Task 2.5.6c: Convert `tools_discovery.go`'s `listSessions`/`getSession`/`searchSessions` (~1 hr)
- Add `instanceDataToSummary`/`instanceDataToDetail`/`matchesSearchData` siblings of the
  existing `*LiveInstance`-typed helpers.

#### Task 2.5.6d: Convert `tools_vcs.go`'s `findInstance` (~20 min)

#### Task 2.5.6e: Convert `tools_goal.go`'s `findInstanceByID`/`findInstanceByUUID` (~15 min)

#### Task 2.5.6f: Convert `tools_terminal.go`'s `readSessionOutput`/`waitForOutput` (not `:690`, Group B) (~20 min)

#### Task 2.5.6g: Convert `session_image_upload_handler.go`'s fallback branch (~15 min)

#### Task 2.5.6h: Convert `backlog_service.go` (~15 min)

#### Task 2.5.6i: Convert `main.go`'s `listSessionsCmd` (~20 min)

#### Task 2.5.6j: Extract `pr_tracking.go`'s six methods to free functions; convert `github_service.go` end-to-end (~1.5 hr)
- Same extraction shape as the superseded revision's Story 2.5.3 — `RefreshPRInfoFor(owner, repo
  string, prNumber int)`-style free functions; existing `(i *LiveInstance)` methods become thin
  wrappers.

---

### Story 2.5.7: Convert Group B call sites — `Registry.Acquire`/`WithInstance`

**As a** developer closing the adversarial review's two blocking findings at the root, **I
want** every call site that needs a live, mutable handle to go through `Registry` instead of a
raw `LoadInstances()` scan, **so that** no sibling instance is ever constructed-and-discarded
and no two actors can ever race over one session's tmux target.

**Acceptance Criteria** — each sub-task implements the corresponding sketch from
`architecture-registry.md` §5:

- `workspace_service.go`'s `findInstanceFast` (§5.1): `ws.registry.Acquire(id)`, map
  `ErrSessionNotFound` to `connect.CodeNotFound`; callers (`SwitchWorkspace`, `GetVCSStatus`,
  etc.) `defer release()`.
- `session_service.go`'s `HibernateSession`/`ResumeHibernatedSession` (§5.2): `Acquire` +
  `defer release()`; single-instance `storage.SaveInstance(live.ToInstanceData())` replaces the
  whole-list `SaveInstances(instances)` re-save — closes §2's finding at the root for these two
  sites, since `Acquire` only ever touches the requested ID.
- `tools_lifecycle.go`'s `stop_session`/`updateSession`/`findAndHydrate` (§5.3): `WithInstance`
  for all three (synchronous, single-call-stack) — no `defer release()` for an implementer to
  forget.
- `terminal_websocket.go`'s `HandleWebSocket` (§5.4): raw `Acquire`/`defer release()` (long-lived
  holder — release fires on connection close, not on function return).
- `health.go`'s `CheckAllSessions`/`RecoverUnhealthySessions` and `hibernation_sweeper.go`'s
  `sweep` (`liveProvider == nil` branch) (§5.5/5.6): `Registry.AcquireAll()` + `defer
  releaseAll()` replaces the whole-list load; loop body unchanged — the fix for the two
  "full-sweep, not one-off fallback" sites §2 flagged as needing a different shape than the
  other 8.
- `daemon/daemon.go`'s `detectAndAddNewSessions` (§5.7): rewritten per the review's own
  recommended fix — skip reconstructing any title already in `existingTitles`, `Acquire` only
  genuinely new `InstanceData` rows. No `FromInstanceData`/`Start()` ever runs for an
  already-tracked session, closing §3b's duplicate-actor hazard at the root. **This requires a
  `*session.Registry` to call `Acquire` on — see Task 2.5.7i below for where that Registry comes
  from, since (confirmed against current code, adversarial-review.md finding 2) `daemon.go`'s
  process never enters `server/dependencies.go`'s DI graph and has no `Registry` of its own
  today.**
- `session_service.go:338`'s `loadInstancesWithWiring` (§3 point 4): its ~10 callers
  (`ListSessions:887`, `GetSession:1000`, `UpdateSession:1407`, `WatchSessions:1874`,
  `StreamTerminal:1971`, `RenameSession:2461`, `RestartSession:2531`,
  `ClearConversationState:2906`) convert to per-ID `Acquire`/`WithInstance` — closing §3a's
  finding that this was never a registry, just a disposable per-call construction shared by
  more callers than any other Group B site, including two very hot RPCs. The 5
  `SessionService`-bound callback wirings this function performs on every returned instance
  (`wireRateLimitCallbacks`/`wireStatusChangeCallback`/`wireClaudeSessionIDCallback`/
  `wireAutoArchiveCallback`/`wireSessionExitedPublisher`, plus the `SetReviewQueue`/
  `SetStatusManager`/`MCPServerURL`-backfill wiring — `session_service.go:341-355` today) are
  **not** dropped: they move into `SessionService.WireInstanceCallbacks`, a new exported method
  with the same body, passed to `Registry` as its `onConstruct` hook (Story 2.5.3/2.5.5). Since
  `onConstruct` fires exactly once per genuinely-new construction, every session any of these 10
  callers `Acquire`s for the first time gets this wiring automatically — the callers themselves
  need no per-call-site wiring code, closing adversarial-review.md finding 1 at the root instead
  of requiring each of the ~10 call sites to remember to replicate it.
- `session_service.go`'s `CreateSession` path: resolved (R2.18a) — `Registry.Register(instance
  *LiveInstance) (ReleaseFunc, error)` is a new sibling method to `Acquire`, not an
  extended `Acquire`. `CreateSession` builds the brand-new `LiveInstance` via `NewInstance`
  (no persisted data exists yet for `Acquire` to look up) and hands it to `Register` **before**
  `storage.AddInstance` persists it (adversarial-review.md finding 5 — see Task 2.5.7h for the
  exact ordering and why). `Register` inserts it into the registry's map at refcount 1, erroring
  on `ErrSessionAlreadyRegistered` if an entry for that session ID already exists. This keeps
  `Acquire` (lookup-existing) and `Register` (construct-new) as two narrow, single-purpose entry
  points into the same dedup guarantee, rather than overloading `Acquire` with a third
  "constructing now" outcome that its callers would need to special-case. `Register` does not
  invoke `onConstruct` — `CreateSession` keeps its own existing explicit wiring call
  (`session_service.go:1287-1291`, unchanged by this epic).

**Files**: `server/services/workspace_service.go`, `server/services/session_service.go`,
`server/mcp/tools_lifecycle.go`, `server/services/terminal_websocket.go`, `session/health.go`,
`session/hibernation_sweeper.go`, `daemon/daemon.go`.

#### Task 2.5.7a: Convert `workspace_service.go`'s `findInstanceFast` (~30 min)

#### Task 2.5.7b: Convert `session_service.go`'s `HibernateSession`/`ResumeHibernatedSession` (~45 min)

#### Task 2.5.7c: Convert `tools_lifecycle.go`'s 3 fallbacks to `WithInstance` (~45 min)

#### Task 2.5.7d: Convert `terminal_websocket.go`'s `HandleWebSocket` (~30 min)

#### Task 2.5.7e: Convert `health.go`/`hibernation_sweeper.go` to `Registry.AcquireAll()` (~45 min)

#### Task 2.5.7f: Fix `daemon/daemon.go`'s `detectAndAddNewSessions` (~45 min)
- Rewrite the function's signature to `detectAndAddNewSessions(currentInstances
  *[]*session.LiveInstance, releases *[]session.ReleaseFunc, registry *session.Registry) error`
  (`architecture-registry.md` §5.7) — skip reconstructing any title already in
  `existingTitles`, `registry.Acquire()` only genuinely new `InstanceData` rows found via
  `registry.Storage().ListInstanceData()`, append each new instance's release to `*releases`
  (mirroring `currentInstances`'s append). The `registry`/`releases` parameters are threaded
  through this task's three existing call sites (`watchForNewSessions`'s initial call at
  `daemon.go:218`, its `select` loop's two calls at `daemon.go:246,277`, and the periodic
  state-refresh goroutine's call at `daemon.go:118`) — all of which already exist and pass
  `currentInstances`/`storage` today, so this is a mechanical signature-threading change once
  Task 2.5.7i (below) gives `RunDaemon` a `registry` and `releases` slice to pass down.
- Regression test: seed ≥2 tracked sessions, run the tick logic twice, assert no second
  construction for an already-tracked title via a call-counting fake (not just goroutine
  count, since the hazard is "two live objects," not "one leaked goroutine").

#### Task 2.5.7g: Convert `loadInstancesWithWiring`'s ~10 callers to per-ID `Acquire`/`WithInstance` (~1.5 hr)
- Extract `loadInstancesWithWiring`'s per-instance wiring block (`session_service.go:341-355`:
  `SetReviewQueue`, conditional `SetStatusManager`, the 5 `wireX` calls, the `MCPServerURL`
  backfill) into a new exported method `func (s *SessionService) WireInstanceCallbacks(inst
  *session.LiveInstance)` with an unchanged body — this becomes the `onConstruct` hook `Registry`
  is constructed with (Task 2.5.5a). Delete `loadInstancesWithWiring` itself once all ~10 callers
  are converted; nothing should call it after this task.
- Convert the 10 callers (`ListSessions:887`, `GetSession:1000`, `UpdateSession:1407`,
  `WatchSessions:1874`, `StreamTerminal:1971`, `RenameSession:2461`, `RestartSession:2531`,
  `ClearConversationState:2906`, plus 2 more identified during this task's own re-grep) to
  per-ID `Acquire`/`WithInstance` — they no longer call any `wireX` method directly; `Registry`'s
  `onConstruct` hook does it for them, exactly once per session, the first time any of these
  callers (or any other caller) `Acquire`s it.
- Regression test: `Acquire` the same session ID from two different simulated call sites (e.g.
  once via a stand-in for `GetSession`, once via a stand-in for `WatchSessions`); assert the 5
  `wireX` callbacks fire exactly once total across both calls, not once per call (proves the
  hook's invocation-once guarantee holds across multiple independent callers of the same
  already-live session, the exact scenario `loadInstancesWithWiring`'s ~10 shared callers create
  today).
- Consider, per `architecture-registry.md` §3's alternative suggestion, whether
  `s.reviewQueuePoller == nil` should instead be treated as a startup-ordering bug rather than
  a steady-state condition every RPC handler tolerates forever — document the decision either
  way.

#### Task 2.5.7h: Add `Registry.Register` and wire `CreateSession`'s initial `*LiveInstance` through it, called before `storage.AddInstance`, with rollback on subsequent `AddInstance` failure (~1.25 hr)
- Implements R2.18a: `Registry.Register(instance *LiveInstance) (ReleaseFunc, error)`
  (Story 2.5.3 Task 2.5.3c), the construction-time counterpart to `Acquire`. Unlike `Acquire`,
  `Register` needs no double-checked-locking dance (no storage I/O to release the lock around):
  the whole check-then-insert runs under one `r.mu` acquisition.
- **Ordering fix (closes adversarial-review.md finding 5, confirmed against current code):**
  `CreateSession` calls `Register(instance)` immediately after `NewInstance` succeeds and
  **before** `storage.AddInstance(instance)` runs — today's order is `NewInstance` (line 1250) →
  `storage.AddInstance` (line 1257, persists) → `reviewQueuePoller.AddInstance` (line 1263,
  live-tracks); the fix inserts `Register` between 1250 and 1257, not after 1257 where
  `reviewQueuePoller.AddInstance` sits today. This closes the race window entirely: no
  ticker-driven sweep converted to `Registry.AcquireAll()` in this same epic (`health.go`,
  `hibernation_sweeper.go`, and `daemon.go` per Task 2.5.7i) can ever observe a persisted-but-
  not-yet-registered row in storage, because the row is not persisted until after `Register` has
  already claimed the session ID in the registry's map — there is no window between persistence
  and registration for a sweep to land in.
- **Collision defense-in-depth** (should not occur given the ordering above — R2.18a's own
  erroring semantics are preserved unchanged — but specify the behavior rather than leaving it
  silent, per adversarial-review.md finding 5): if `Register` ever returns
  `ErrSessionAlreadyRegistered` (e.g. a duplicate/reused session ID from a bug elsewhere),
  `CreateSession` must (a) call `instance.stopActor()` on its own already-actor-spawned
  `*LiveInstance` before returning — the loser-cleanup `Acquire`'s own double-checked-locking
  gets for free via `live.stopActor()` on its losing branch, which `Register`'s bare-error
  semantics does not provide automatically — and (b) return `connect.NewError(connect.CodeInternal,
  ...)` to the client and log at Error level, not Warn: this is not an expected steady-state
  condition the way `Acquire`'s refcount++ path is, so it should be loud, not silently swallowed.
  Note this branch has **no registry entry to roll back** (the session was never registered in
  the first place), unlike the adjacent `AddInstance`-failure path below.
- **Rollback on `storage.AddInstance` failure (closes the fifth-pass adversarial-review.md
  finding 2 / `pitfalls-register-rollback.md`): `Register()` succeeding and the immediately-
  following `storage.AddInstance` subsequently failing is an adjacent, equally-real failure
  path the ordering fix above opens and does not by itself close.** If `storage.AddInstance`
  fails after `Register(instance)` has already succeeded, `CreateSession` must call
  `s.registry.ForceRelease(instance.GetStableID())` — **synchronously, inline, before
  returning its error** — so no `Registry` entry, and no live actor, is ever left behind for a
  session ID `Storage` has no row for:
  ```go
  if _, err := s.registry.Register(instance); err != nil {
      instance.stopActor()
      return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to register instance: %w", err))
  }
  if err := s.storage.AddInstance(instance); err != nil {
      // Register() succeeded but persistence failed: abort the registration synchronously
      // before returning. NOT the release() closure Register returned — see below.
      s.registry.ForceRelease(instance.GetStableID())
      return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
  }
  ```
  **This must be `ForceRelease(instance.GetStableID())`, explicitly NOT the `release()` closure
  `Register` itself returned.** `release()` (via `makeRelease`) only tears down the entry when
  refcount reaches zero — `Register` inserts at refcount 1, so `release()` usually decrements
  1→0 and correctly tears down, but `Registry.Acquire` checks `r.entries` first, with no storage
  lookup gating it: if any other goroutine calls `Acquire(sameSessionID)` in the window between
  `Register()` succeeding and this cleanup running (e.g. a client retry against a
  predictable/reused ID), that `Acquire` finds the phantom entry, bumps its refcount to 2, and
  hands back a live `*LiveInstance` for a session with no storage row — before this cleanup
  path even runs. If the cleanup then calls the plain `release()` closure, it decrements 2→1 and
  does **not** tear down, leaving the exact phantom-entry bug this fix exists to close, now
  recurring inside the fix itself. `ForceRelease` sidesteps this: it deletes the map entry and
  stops the actor **unconditionally**, regardless of current refcount, so any holder that raced
  in ahead of the cleanup is left with a `*LiveInstance` whose actor is now stopped — its next
  `send`/`sendSync` gets the same typed `ErrInstanceStopped` any other force-released holder
  gets (Story 2.5.9c's contract), which is the correct outcome for an aborted registration. The
  cleanup must run synchronously, inline, in the same goroutine as the `return` — an async
  `go registry.ForceRelease(id)` before returning would reopen a narrower version of the same
  bug (a smaller but still-nonzero window where a concurrent `Acquire` observes the phantom
  entry).
- **`ForceRelease` doc-comment addition** (no body change): add a sentence to `Registry.ForceRelease`'s
  doc comment (Story 2.5.3) noting this second caller: *"Also used by `CreateSession` to abort a
  `Register()`'d entry when the immediately-following `storage.AddInstance` fails — the entry was
  never confirmed, so an unconditional teardown (not a refcount-gated `release()`) is required to
  correctly handle a concurrent `Acquire` racing in before the abort runs (see Task 2.5.7h)."*
- Unit test: `Register` succeeds and is called before the equivalent `storage.AddInstance` point
  in a simulated `CreateSession` flow; a forced `ErrSessionAlreadyRegistered` collision results in
  the caller's instance having `stopActor()` called on it (assert via a call-counting fake) and a
  `connect.CodeInternal` error surfaced, not a leaked goroutine or a silently-ignored error.
- **Additional unit test — rollback on `AddInstance` failure (finding 2)**: fake
  `Storage.AddInstance` to return an error in a simulated `CreateSession` flow; assert (a)
  `registry.Count()` is unchanged from before the call (or a subsequent `Acquire` for the same ID
  returns `ErrSessionNotFound`, not the phantom instance), and (b) the instance's actor has fully
  stopped (e.g. via a call-counting/goroutine-count fake), not just "map entry gone."
- **Additional unit test — concurrent-`Acquire`-during-abort race (distinguishes `ForceRelease`
  from `release()`)**: use a test-only hook to pause `CreateSession`'s error path after
  `Register()` succeeds but before `ForceRelease` runs; from a second goroutine, call
  `registry.Acquire(sameID)` (bumping refcount to 2); let the paused path proceed and call
  `ForceRelease`; assert the entry is gone (`registry.Count()` back to baseline) and the second
  goroutine's held `*LiveInstance`'s next `send`/`sendSync` returns `ErrInstanceStopped`. This
  test would fail (entry stays alive at refcount 1) if the fix used `Register`'s returned
  `release()` instead of `ForceRelease`, making it the regression test that specifically catches
  the wrong-but-plausible implementation choice.

#### Task 2.5.7i: Give `daemon/daemon.go`'s `RunDaemon` its own `session.Registry`, route the initial seed load through it (~45 min)
- **New task, added per adversarial-review.md finding 2** (confirmed against current code:
  `daemon.go` has no `Daemon` type — `RunDaemon` is a free function; `daemon/daemon.go` is
  imported only by `main.go`, never by `server/dependencies.go`/`server/server.go`; the
  `--daemon` process is architecturally unreachable from the DI graph Story 2.5.5 wires
  `Registry` into). `RunDaemon` must construct and own an entirely separate `Registry` instance,
  scoped to its own process — it does not, and cannot, share the main server process's `Registry`.
- In `RunDaemon` (`daemon.go`), immediately after `storage, err :=
  session.NewStorageWithRepository(repo)` (line 32), add: `registry :=
  session.NewRegistry(storage, nil)` — `onConstruct` is `nil` here, not
  `SessionService.WireInstanceCallbacks`, because the daemon process has no `SessionService`
  (it never enters `server/dependencies.go`'s DI graph); document this as a deliberate,
  process-boundary-driven asymmetry with the main server process's `Registry`, not an oversight.
- Rewrite the **initial seed load** (`daemon.go:37`, currently `instances, err :=
  storage.LoadInstances()` — uncataloged by any prior task per adversarial-review.md finding 2)
  to enumerate via `storage.ListInstanceData()` and `Acquire` each ID through the new `registry`,
  mirroring Task 2.5.5b's `BuildRuntimeDeps` Step 5 rewrite exactly:
  ```go
  var releases []session.ReleaseFunc
  dataList, err := storage.ListInstanceData()
  if err != nil { return fmt.Errorf("failed to load instances: %w", err) }
  for _, data := range dataList {
      live, release, err := registry.Acquire(data.GetStableID())
      if err != nil { log.Warn("daemon: acquire failed", "session", data.Title, "err", err); continue }
      instances = append(instances, live)
      releases = append(releases, release)
  }
  ```
- Thread `registry` and `&releases` through to `watchForNewSessions` (new parameters) and
  through to every `detectAndAddNewSessions` call site Task 2.5.7f converts.
- At shutdown (`daemon.go`'s existing `close(stopCh); wg.Wait()` block, before
  `storage.SaveInstances(instances)`), add `registry.Shutdown()` — force-stops every actor this
  process's `Registry` tracks regardless of refcount, mirroring Story 2.5.5d's `shutdownHooks`
  pattern in the main server process, manually invoked here since this `Registry` is never
  wired into `server.go`'s `shutdownHooks` (different process, no shared `Registry`).
- Regression test: construct a `RunDaemon`-equivalent test harness seeding ≥1 persisted session,
  confirm the initial seed load produces exactly one `LiveInstance` per session via
  `registry.Acquire` (not raw `storage.LoadInstances()`), and that a subsequent
  `detectAndAddNewSessions` tick for an already-seeded title does not construct a second one
  (same shape as Task 2.5.7f's own regression test, applied to the initial-load path too).
- **Catalog (not yet convert) `daemon.go`'s direct `AutoYes` writes** (fifth-pass
  adversarial-review.md non-blocking finding, §4): `daemon.go:43` (the initial seed loop) and
  `daemon.go:312` (inside `detectAndAddNewSessions`) both write `instance.AutoYes = true`
  directly — the same pattern `architecture-registry.md` §5.7's own sketch preserves. This task
  formally routes `daemon.go` through `Registry`/`LiveInstance` for the first time, which is
  what makes these writes worth tracking now rather than rediscovering them during Epic 5: they
  are out of scope for conversion *here* (no actor exists yet at Epic 2.5's merge point — Epic 3
  spawns it), but note in a code comment at each write site that this field becomes
  actor-mutated state once Epic 3/7 land, and that Story 5.4's CI guard (extended to scan
  `daemon/daemon.go`) and Epic 7 Story 7.1e's field-unexport will both catch it if it isn't
  converted by then.

---

### Story 2.5.8: `ReviewQueuePoller` coexistence — `SetInstances`/`AddInstance`/`RemoveInstance` route through `Registry`

**As a** developer integrating `Registry` with the one existing "list of live instances"
consumer, **I want** only the 3 mutating entry points changed, **so that** `ReviewQueuePoller`
becomes a long-lived `Registry` consumer rather than a second, competing construction path.

**Acceptance Criteria** (per `architecture-registry.md` §3):
- `ReviewQueuePoller` gains a `registry session.InstanceAcquirer` field (**not** the concrete
  `*Registry`, per adversarial-review.md finding 6 — `ReviewQueuePoller` only ever calls
  `Acquire` here, never `List`/`Count`, so the narrowest interface Story 2.5.4 defines for
  exactly this shape of consumer is the correct type, matching `WorkspaceService`'s
  `InstanceAcquirer`-typed field precedent and Task 2.5.5c's own "narrowest interface" rule)
  and a `releases map[string]session.ReleaseFunc` mirroring `rqp.instances` membership.
- `AddInstance(sessionID string) error` acquires by ID (`externalDiscovery.OnSessionAdded`'s
  call-site change, `dependencies.go:633-647`), stores the release alongside the instance.
- `RemoveInstance(instanceTitle string)` additionally calls the stored `release()` (outside
  `rqp.mu`) after its existing filter loop.
- `GetInstances()`/`FindInstance()` (`review_queue_poller.go:877,848-860`) stay **unchanged** —
  already-borrowed references, same pattern as `FindLiveInstance` (`session_service.go:431`).
- `SetInstances` (`review_queue_poller.go:152-167`) converts to acquiring each ID via
  `Registry` rather than accepting already-built instances directly.

**Files**: `session/review_queue_poller.go`, `server/dependencies.go` (`:438,483,633-659`).

#### Task 2.5.8a: Add `registry` (typed `session.InstanceAcquirer`)/`releases` fields; convert `AddInstance` (~30 min)

#### Task 2.5.8b: Convert `RemoveInstance` to also release (~20 min)

#### Task 2.5.8c: Convert `SetInstances`'s bulk seed (~30 min)

#### Task 2.5.8d: Update `dependencies.go`'s 3 call sites wiring these entry points (~20 min)

---

### Story 2.5.9: `DeleteSession` force-invalidate (R2.18)

**As a** developer resolving `pitfalls-registry.md` §4's flagged judgment call, **I want**
`DeleteSession` to tear down a session's actor immediately regardless of other holders, **so
that** "delete means gone now" matches today's user-facing timing instead of waiting on an
unrelated long-poll client's reference.

**Acceptance Criteria** (implements R2.18 — the requirements author's resolution of
`pitfalls-registry.md` §4's Option 1 vs. Option 2 judgment call, in favor of **Option 1**,
force-invalidate):
- `DeleteSession` (`session_service.go:1722-1817`) acquires the target via `Registry.Acquire`,
  runs `Destroy()` with the same fire-and-forget async timing as today (detached goroutine,
  errors logged/non-fatal, RPC does not wait), then calls `Registry.ForceRelease(sessionID)` —
  **not** an ordinary `release()` — regardless of how many other holders (e.g.
  `terminal_websocket.go`'s streaming handler mid-connection, `ReviewQueuePoller`'s cache
  entry) currently hold a reference. This replaces Epic 3's original Story 3.1 "`Stop()` only
  after `Destroy()` returns, only from the one deletion goroutine" ordering rule — the ordering
  guarantee (actor alive for the whole `Destroy()` call) still holds, but the *decision* of when
  to force-teardown now belongs to `Registry`, not to `DeleteSession`'s own call-site
  sequencing (see Epic 3's updated Story 3.1 acceptance criteria for the reconciled text).
- Storage deletion (client-visible RPC success) remains synchronous and immediate, unchanged.
- Any other holder's next `send`/`sendSync` against the force-released `*LiveInstance` returns
  a typed error (e.g. `ErrInstanceStopped`) rather than hanging — an Epic 3 actor-mechanics
  requirement (`send`/`sendSync` must check whether the actor's context is already canceled
  before/while enqueueing), cross-referenced here since `ForceRelease` is what triggers it.
- Clean up the three verbatim-duplicate `s.approvalStore.CancelSession(sessionUUID)` blocks
  (`session_service.go:1783-1802`, `pitfalls-registry.md` §4's aside) — harmless no-ops today,
  trivial to fix while this function is already being touched.

**Files**: `server/services/session_service.go` (`DeleteSession`), `session/instance_actor.go`
(Epic 3 — typed error on send-to-stopped-actor, cross-referenced).

#### Task 2.5.9a: Convert `DeleteSession` to `Acquire` + `Destroy()` + `ForceRelease` (~45 min)

#### Task 2.5.9b: Remove the 3 duplicate `CancelSession` calls, keep one (~10 min)

#### Task 2.5.9c: Regression test — force-release while a second holder's reference is outstanding, assert the second holder's next command returns a typed error, not a hang (~30 min)
- Deadlock-by-timeout shape (`pitfalls.md` §6 Pattern 1).

---

### Story 2.5.10: `Registry` tests — refcount, idempotent release, acquire-during-teardown race, dedup

**As a** developer proving `Registry`'s guarantees hold under concurrency (not just the
single-threaded happy path), **I want** the tests `pitfalls-registry.md` §5-6 specifies, **so
that** "at most one actor per session" and "release is safe to call more than once" are
verified, not asserted.

**Acceptance Criteria** (non-sleep-based throughout, per this migration's standing testing
discipline):
- **Test 1 — refcount is shared, teardown only at zero**: two `Acquire`s for the same ID return
  the identical `*LiveInstance` (`require.Same`); releasing one leaves the entry alive
  (`require.Never`); releasing the second tears it down (`require.Eventually`).
- **Test 2 — acquire-during-teardown, synchronized via an injected stop hook, not sleep**: a
  test-only `withStopHook` lets the test hold `stopActor()` open with a channel while a
  concurrent `Acquire` for the same ID is proven still blocked until the held `Stop()`
  completes, then proven to return a **fresh, independent** `*LiveInstance` (`require.NotSame`)
  once it does — directly exercises Design A's synchronization guarantee (Story 2.5.3).
- **Test 3 — idempotent release**: calling the same `release()` closure twice does not
  double-decrement; an `acquireCount == releaseCount` test-only decorator at teardown catches
  both the closure-idempotency shape and the two-independent-releases-double-counting-one-acquisition
  shape.
- **Test 4 — retargeted Epic-3 goleak test, N `Acquire`s for the same session**: call
  `Registry.Acquire(sessionID)` N times for the **same** ID (modeling `loadInstancesWithWiring`'s
  ~10 callers), assert exactly **one** actor goroutine exists (post-Epic-3; pre-Epic-3, assert
  exactly one `LiveInstance` construction via a call-counting fake `Storage`), release all N,
  assert goroutine/construction count returns to baseline. Strictly stronger than the
  superseded revision's design — N independently-constructed-and-cleaned-up instances are
  goroutine-count-indistinguishable from one shared actor; only testing through `Acquire`
  catches "no duplication," not just "no leak."
- **Test 5 — sibling-leak guard**: seed the registry/storage with ≥2 sessions, `Acquire`/
  `release()` only session A repeatedly, assert session B's `LiveInstance`/actor is **never**
  constructed — targets `adversarial-review.md` §2's exact finding (siblings from the same
  `LoadInstances()` call leaking) at the `Registry` boundary instead of the old raw-storage
  boundary.
- `GitHubService.GetPRInfo`'s own goleak assertion (superseded revision's Task 2.5.4a) is
  retargeted: post-`Registry`, its lookup goes through `InstanceData` (Group A, Story 2.5.6),
  not `Registry.Acquire` at all — its goleak test now asserts **zero** `LiveInstance`
  construction across N calls, a stronger claim than "goroutine count returns to baseline."

**Files**: new `session/registry_test.go`.

#### Task 2.5.10a: Write Test 1 (refcount shared) (~30 min)

#### Task 2.5.10b: Write Test 2 (acquire-during-teardown, `withStopHook` injection) (~45 min)

#### Task 2.5.10c: Write Test 3 (idempotent release, acquireCount==releaseCount decorator) (~30 min)

#### Task 2.5.10d: Write Test 4 (dedup goleak, N Acquires one session) (~30 min)

#### Task 2.5.10e: Write Test 5 (sibling-leak guard, ≥2 sessions) (~30 min)

#### Task 2.5.10f: Retarget `GitHubService.GetPRInfo`'s goleak test to assert zero construction (~20 min)

---

## Epic 3: Prove Actor Plumbing on a Low-Risk Write Path

**Architecture ref**: `architecture.md` §1.1, §1.3, §2, §6, §7 PR3; `stack.md` §2-4;
`features.md` §4 (goleak recommendation). **Risk**: contained — this epic only converts
fields confirmed (by grep, not assumption) to have exactly one writer, so "convert this
write path" is already a complete cutover for those specific fields, consistent with the
`pitfalls.md` §4 reads-vs-writes asymmetry.

**Also resolves adversarial-review.md's fifth pass, finding 1** (the select-race steady-state
hang — distinct from the fourth pass's identically-numbered finding 1, "dropped callback
wiring," which Epic 2.5 already resolved): Story 3.1's non-blocking priority pre-check, added
to `send`/`sendSync`/`sendSyncErr`/`sendCtx` in Task 3.1a, per `pitfalls-select-race.md`.

**Open design decision to resolve before Task 3.1a** — see "Open Decisions" section at the
bottom of this plan: actor stop-signal mechanism (`context.Context` vs dedicated `stopCh`).

### Story 3.1: Core actor primitives — `command`, `send`, `sendSync`, `runActor`, lifecycle

**As a** developer implementing the rest of this migration, **I want** the mailbox/command/
response-channel plumbing to exist and be tested in isolation before any state-machine code
depends on it, **so that** Epic 4's higher-risk migration isn't debugging actor mechanics
and reentrancy rules at the same time.

**Acceptance Criteria**:
- `command{name string; fn func(s *instanceState); fail func(error)}` per `architecture.md`
  §1.1 — closure-based, not a discriminated struct-per-command union (see "Open Decisions":
  flagged for an ADR, but this is the directionally-decided shape per `stack.md`/`architecture.md`).
  **`fail`** is the addition this story makes to close adversarial-review.md finding 4 (R2.18's
  send/sendSync race, detailed below): `nil` for plain `send`'s fire-and-forget commands
  (nobody is waiting on a reply, so there is nothing to fail), populated for `sendSync`/
  `sendSyncErr`/`sendCtx` commands with a closure that delivers a typed error to the same reply
  channel `fn` would have written a success value to.
- `instanceState{inst *Instance}` — the actor's private working-set wrapper.
- `(i *Instance) send(name string, fn func(s *instanceState))` — fire-and-forget. Races the
  mailbox send itself against `ctx.Done()` in one `select` (not a precondition check before an
  unconditional send — closes the TOCTOU half of finding 4): `select { case i.mailbox <- cmd: ;
  case <-i.ctx.Done(): return }`. No reply channel exists for `send`, so there is nothing to
  surface an error through on the canceled-context branch; log at Debug level for observability
  and drop, matching the existing fire-and-forget contract.
  **Fifth-pass fix (`adversarial-review.md` finding 1 / `pitfalls-select-race.md`): this bare
  `select` alone is not a deterministic rejection of sends issued long after the actor has
  already exited.** Go's `select` picks uniformly at random among simultaneously-ready cases —
  it has no notion of "prefer the case that's been ready longer" — so once `stopActor()` has
  fully returned, `i.mailbox` is open with spare buffer capacity (nothing closes it; see Task
  3.1b), and `case i.mailbox <- cmd:` stays genuinely ready indefinitely. A caller sending
  against an already-long-dead actor therefore has a real, bounded (~1-in-32, per ADR-027's
  mailbox capacity) per-call chance of enqueuing into a mailbox nobody will ever read instead of
  taking the `ctx.Done()` branch — a hang for `sendSync`, a silent drop for `send`. The fix is a
  **non-blocking priority pre-check** immediately before the existing blocking select, not a
  replacement for it:
  ```go
  select {
  case <-i.ctx.Done():
      return // or the typed-error return, for sendSync/sendCtx below
  default:
  }
  select {
  case i.mailbox <- cmd:
  case <-i.ctx.Done():
      return
  }
  ```
  This deterministically catches every call made after `ctx` has been canceled for any nonzero
  amount of time (a canceled `context.Context` never un-cancels, so `default` is never taken
  once `ctx.Done()` is closed), while the existing blocking select + `runActor`'s
  `drainMailboxOnStop` (unchanged) continue to own the genuinely-concurrent instant where
  cancellation races the send itself. **This exact non-blocking `select { ...; default: }`
  priority-check shape matches an idiom already established elsewhere in this codebase** —
  e.g. `session/mux/multiplexer.go:493-497`, `session/tmux/server_registry.go:307-311`,
  `session/hibernation_sweeper.go:270-274`, `session/cdp/manager.go:409-412`, and
  `session/external_tmux_streamer.go:278-281` all use the identical
  cancellation-pre-check-before-further-work pattern — the same "match existing idiom"
  precedent this plan already applies elsewhere (e.g. ADR-029's `PRStatusPoller`
  reconciliation). Applies identically to `sendSync`/`sendSyncErr`/`sendCtx` below.
- `sendSync[T any](i *Instance, name string, fn func(s *instanceState) T) (T, error)` — buffered
  `chan syncResult[T]` of capacity 1 (so the actor's reply send never blocks on an abandoned
  caller, per `stack.md` §4 idiom #1). **Returns `(T, error)`, not bare `T`** — this is the
  signature fix finding 4 requires: without an error return, `sendSync` has nowhere to surface
  `ErrInstanceStopped`. Adds the same non-blocking `select { case <-i.ctx.Done(): return zero,
  ErrInstanceStopped; default: }` priority pre-check described above, then races the mailbox
  send against `ctx.Done()` in the same `select` shape as `send`; on the `ctx.Done()` branch,
  returns `(zero value of T, ErrInstanceStopped)` immediately without ever touching the mailbox.
- `sendSyncErr(i *Instance, name string, fn func(s *instanceState) error) error` — sugar over
  `sendSync[error]` for the common case (Epic 4's `Pause`/`Start`/etc.): `result, err :=
  sendSync(i, name, fn); if err != nil { return err }; return result` — flattens the "actor
  already stopped" outer error and the command's own inner `error` result into one return value,
  since for `T = error` there would otherwise be two different errors to reconcile at every call
  site. Inherits the priority pre-check via its delegation to `sendSync`; no separate fix needed.
- `(i *Instance) sendCtx(ctx context.Context, name string, fn func(s *instanceState)) error` —
  context-bounded send for poller call sites (Epic 5/6 use this; this epic just defines it).
  **Receiver form resolved**: a method on `*Instance`, matching `send`'s existing method form —
  `pitfalls-select-race.md` flagged this as ambiguous in an earlier draft (a bare function
  signature with no explicit `i` receiver/param was shown alongside `sendSync`/`sendSyncErr`'s
  explicit-`i`-param form); since `sendCtx` is not generic (unlike `sendSync`, which must be a
  package-level function because Go has no generic methods), there is no reason for it to differ
  from `send`'s method form, so it is one. Adds the same non-blocking priority pre-check as
  `send`/`sendSync` above (checking both `ctx` and `i.ctx`), then the same select-based race
  against **both** the passed-in `ctx` and the actor's own `i.ctx`, returning
  `ErrInstanceStopped` on the latter and `ctx.Err()` on the former.
- `runActor()`: `for { select { case cmd := <-i.mailbox: cmd.fn(s); i.snapshot.Store(buildSnapshot(s.inst)) ; case <-i.ctx.Done(): i.drainMailboxOnStop(); close(i.done); return } }`.
  **`drainMailboxOnStop` — the explicit fix for finding 4's second half**: because a command can
  legitimately finish being enqueued (the sender's `select` picked the `i.mailbox <- cmd` case)
  in the same instant `ctx` is canceled, `runActor`'s own `select` can pick the `ctx.Done()`
  branch without ever having read that already-buffered command — the per-send `select` fix
  above closes the window for *new* sends racing cancellation, but does not by itself guarantee
  an *already-enqueued* command gets a reply. `drainMailboxOnStop` closes that residual gap by
  non-blockingly draining every command still sitting in `i.mailbox` at exit and calling each
  one's `fail(ErrInstanceStopped)` (commands with `fail == nil` — plain `send` calls — are simply
  discarded, since nobody is waiting on them):
  ```go
  func (i *LiveInstance) drainMailboxOnStop() {
      for {
          select {
          case cmd := <-i.mailbox:
              if cmd.fail != nil { cmd.fail(ErrInstanceStopped) }
          default:
              return
          }
      }
  }
  ```
  This is what makes R2.18's "well-defined error, not a hang" guarantee hold unconditionally: no
  `sendSync`/`sendSyncErr`/`sendCtx` caller can be left blocked on `<-reply` after
  `ForceRelease`/`stopActor()` fires, whether their command raced past the cancellation check or
  was already sitting in the buffer when cancellation happened.
- `Instance` gains `mailbox chan command` sized per the "Open Decisions" buffer-size
  resolution, plus the chosen stop-signal field.
- The actor goroutine is spawned via the same shared construction helper Task 1.1c introduced
  (`finishInstanceConstruction`/equivalent), extended here to add `go i.runActor()` after the
  initial snapshot publish. Per Epic 2.5's Sequencing note, this helper is now reached only
  through `session.Registry`'s internal `newLiveInstance` (Epic 2.5 Story 2.5.2/2.5.3) —
  `Registry`'s dedup guarantee (at most one construction per session ID) already existed before
  this task; this task just gives `newLiveInstance`'s paired `stopActor()` its first real body
  (`i.cancel(); <-i.done` — cancel the actor's context, then **block** until `runActor()`'s
  `defer close(i.done)` confirms the run loop has actually exited and drained; see Task 3.1b)
  instead of the no-op/GC-reliant teardown it had pre-Epic-3. The actor is the **sole** mutator
  of the fields converted in Story 3.2 — no
  other goroutine writes `LastViewed`/`LastMeaningfulOutput`/`Acknowledged` directly once this
  lands (verify via the grep in Task 3.2a).
- `(i *LiveInstance) stopActor()` (the internal teardown `Registry.release()`/`ForceRelease()`
  call — Epic 2.5 Story 2.5.3) signals the actor to exit after draining.
  **Ordering requirement — this supersedes any "stop before removal" assumption elsewhere in
  this plan, and reflects R2.18's resolution that `Registry`, not the individual call site, owns
  the force-teardown decision (Epic 2.5 Story 2.5.9)**: `DeleteSession`
  (`server/services/session_service.go`, `DeleteSession` at ~line 1722, converted to
  `Registry.Acquire`/`ForceRelease` by Epic 2.5 Story 2.5.9) must still preserve the same
  relative ordering Epic 2.5 established, now expressed as `Registry` API calls rather than a
  raw `LiveInstance.Stop()`:
  1. `removeFromAllPollers(sessionTitle)` (`session_service.go:1827,1832`) — poller/
     history-linker deregistration only. **Must not** call `Registry.ForceRelease` here — the
     actor must still be alive for `Destroy()` below.
  2. The existing async `go func() { live.Destroy() }()` block (currently ~line 1761-1764,
     runs after `removeFromAllPollers` returns). `Destroy()` (`session/instance.go:941`) calls
     `i.StopController()` and `i.KillSession()`, both of which Epic 4 (Story 4.1c, Story 4.2b)
     converts into `sendSync`-routed public methods that block the caller until the actor
     processes the command and replies — so **the actor must still be alive** for the entire
     duration of this `Destroy()` call, exactly as before.
  3. Only **after** that same `go func()`'s call to `live.Destroy()` returns (regardless of
     error) does that goroutine call `registry.ForceRelease(sessionID)` as its last statement —
     tearing down the actor **immediately, regardless of any other holder's outstanding
     refcount** (R2.18), rather than the ordinary refcounted `release()` every other caller uses.
  Calling `ForceRelease` any earlier — e.g. inside `removeFromAllPollers`, or in the same
  goroutine before `Destroy()` returns — cancels the actor's `ctx` before `Destroy()`'s own
  `sendSync` calls can complete; the run loop exits via `case <-ctx.Done(): return`; those
  `sendSync` calls then send into a mailbox nobody is reading, hanging forever on **every single
  session deletion** with nothing watching the stuck channel send (ADR-024's own stated
  tradeoff: `linkdata/deadlock` detects the mutex version of this; nothing watches a stuck
  channel send the same way) — this is the same hazard the superseded ordering rule guarded
  against; only the API surface changed, from `inst.Stop()` to `registry.ForceRelease(id)`.
  Any *other* holder of the same session's `*LiveInstance` (e.g. a WebSocket stream
  mid-connection, `ReviewQueuePoller`'s cache entry) is unaffected by this ordering rule
  directly — their next `send`/`sendSync` against the now-force-released actor must return a
  typed error (Epic 2.5 Story 2.5.9's acceptance criteria), which this story's `send`/`sendSync`/
  `sendCtx` implementation (Task 3.1a) guarantees two ways, not one: (1) a `select` racing the
  mailbox send itself against `ctx.Done()` (not a precondition check before an unconditional
  send — this closes the TOCTOU adversarial-review.md finding 4 identified, where a check-then-
  send has a window between the check and the send for a concurrent `ForceRelease` to land), and
  (2) `runActor`'s `drainMailboxOnStop` (above) failing any command that already made it into the
  mailbox before cancellation was observed. Both are required — (1) alone does not cover a
  command that legitimately finished enqueuing in the same instant `ctx` was canceled.
  The `Acquire`-returns-`ErrSessionNotFound` case (session not in storage at all — e.g. after a
  server restart with no persisted row) never had an actor to begin with, so no `ForceRelease`
  call applies there.

**Files**: new `session/instance_actor.go` (mailbox, command, send/sendSync/sendCtx, runActor,
`stopActor`); `session/instance.go`/`session/registry.go` (give `newLiveInstance`'s
`finishInstanceConstruction` call the actor-spawn extension; give `stopActor()` its real body —
`Registry`'s own map/refcount logic, from Epic 2.5, is unchanged); `server/server.go` (Task
3.1e, verification only — `shutdownHooks` registration already happened in Epic 2.5 Story
2.5.5). Group B call-site wiring (`workspace_service.go`, `tools_lifecycle.go`,
`tools_terminal.go`, `terminal_websocket.go`, `health.go`, `hibernation_sweeper.go`) is entirely
Epic 2.5 Story 2.5.7's responsibility now, not this epic's — no files from that list are
touched here.

#### Task 3.1a: Write `command`/`instanceState`/`send`/`sendSync`/`sendSyncErr`/`sendCtx` (~1 hr)
- New file `session/instance_actor.go`. Implement exactly the signatures above, following
  `stack.md` §4's idiomatic-details list (buffered reply channel, command-carries-its-own-
  reply, context-aware variant for `sendCtx`).
- **Closes adversarial-review.md finding 4 (R2.18's send/sendSync race):** `send`/`sendSync`/
  `sendCtx` must race the mailbox send itself against `ctx.Done()` in one `select` —
  `select { case i.mailbox <- cmd: ...; case <-i.ctx.Done(): return ErrInstanceStopped }` — not a
  `ctx.Err() != nil` precondition check performed before an unconditional send. A precondition
  check leaves a TOCTOU window (context cancels between the check and the send); the `select`
  form has no such window because the send and the cancellation-check are the same atomic
  language construct.
- **Also closes the fifth-pass adversarial-review.md finding 1 (steady-state hang, distinct from
  finding 4 above):** the bare `select` above is necessary but not sufficient — Go's `select`
  picks uniformly at random among simultaneously-ready cases (per the Go spec; no source-order or
  ready-duration priority), so once the actor has been stopped for any nonzero amount of time,
  `case <-i.ctx.Done():` and `case i.mailbox <- cmd:` (mailbox left open with spare capacity —
  nothing closes it, per Task 3.1b/`pitfalls-select-race.md` §3) are *both* ready, and the send
  branch can still be picked, silently enqueuing into a mailbox nobody will ever read. `send`/
  `sendSync`/`sendCtx` must each add a **non-blocking priority pre-check** immediately before
  their existing blocking select: `select { case <-i.ctx.Done(): return ...; default: }`. This
  deterministically catches every call made after `ctx` has been canceled for any nonzero
  duration (a canceled context never un-cancels, so `default` is never taken once `ctx.Done()`
  is closed) — the "long after teardown" steady-state case finding 1 is concerned with. The
  existing blocking select (finding 4's fix) remains required for the genuinely narrow window
  where cancellation happens *during* the pre-check/send, which `drainMailboxOnStop` already
  closes; both mechanisms are required together, neither alone is sufficient. This pre-check
  matches the non-blocking `select { ...; default: }` cancellation-guard idiom already used in
  this codebase, e.g. `session/mux/multiplexer.go:493-497` (see the Story 3.1 acceptance
  criteria above for the full precedent list) — reuse that exact shape, don't invent a new one.
- `sendSync`'s new `(T, error)` signature (this task changes it from bare `T`, per the Story 3.1
  acceptance criteria above) requires every existing usage sketch elsewhere in this plan that
  assumed bare `T` to instead use `sendSyncErr` (`T = error` case) or handle the tuple return —
  confirm no call site in Epic 4/5/6's task text assumes single-value `sendSync` before closing
  this task; if one is found, it should already read `sendSyncErr` in intent (methods returning
  `error`) — fix the citation there rather than reintroducing bare-`T` `sendSync`.
- Unit tests: a `send`/`sendSync`/`sendCtx` call racing a concurrent `cancel()` (via a test-only
  hook, not `time.Sleep`) returns `ErrInstanceStopped` and never blocks past a short timeout,
  covering both "canceled before the select is entered" and "canceled while the select is
  entered" (the actual TOCTOU window this task closes).
- **Additional unit test for the priority pre-check (fifth-pass finding 1)**: call `send`/
  `sendSync` **after** `stopActor()`/`ForceRelease` has already returned (not concurrently with
  it — a genuinely stopped actor, not a stop-in-progress one), in a loop of at least 100
  iterations, asserting `ErrInstanceStopped` (or the dropped-and-logged behavior for `send`) on
  **every single call**. A single-call assertion is insufficient here and would pass most runs
  even against the un-fixed bare-select code, since the bug is probabilistic (bounded by
  mailbox capacity, ~1-in-32 per call) rather than deterministic — this loop is what actually
  distinguishes "pre-check present" from "pre-check missing."

#### Task 3.1b: Write `runActor()` + `drainMailboxOnStop` + lifecycle fields + `stopActor()` (~1 hr)
- Same file. Wire the chosen stop-signal mechanism (decided per "Open Decisions" below).
- **Closes adversarial-review.md finding 4's second half:** implement `drainMailboxOnStop`
  exactly as specified in the Story 3.1 acceptance criteria above — on `ctx.Done()`, before
  returning from `runActor()`, non-blockingly drain `i.mailbox` and call `cmd.fail(ErrInstanceStopped)`
  on every command still sitting in it (commands with `fail == nil`, i.e. plain `send` calls, are
  discarded). This is what prevents a `sendSync`/`sendSyncErr` caller whose command finished
  enqueuing in the same instant `ctx` was canceled from blocking on `<-reply` forever — Task
  3.1a's per-send `select` fix alone does not cover this case, only commands racing a *new* send
  against cancellation.
- **Closes adversarial-review.md finding 3 (ADR-029/Design-A blocking-semantics reconciliation
  — see the addendum in `../decisions/ADR-029-actor-shutdown-context-cancelfunc.md`):**
  `stopActor()` must **block until the actor's run loop has actually returned**, not merely call
  `cancel()` and return. Add a `done chan struct{}` field, closed via `defer close(i.done)` as the
  very last statement inside `runActor()` (after `drainMailboxOnStop` runs, before the function
  returns), and give `stopActor()` the body `i.cancel(); <-i.done`. This is what makes
  `Registry.release()`'s own doc comment true as implemented ("an `Acquire` for this ID after
  `release()` returns is guaranteed a fresh, independent `LiveInstance`, never one mid-teardown")
  — a bare `cancel()`-and-return `stopActor()` would let `release()` return while the old actor
  is still mid-command (e.g. still doing tmux I/O), reopening a narrower version of the
  two-actors-one-tmux-target hazard `Registry` exists to close.
- `stopActor()` must additionally be idempotent (safe to call once; document that calling twice
  is a bug, or guard with `sync.Once` around `i.cancel()` — `<-i.done` is already safe to read
  from multiple goroutines since `close()` is itself idempotent-safe-to-observe). Note
  `Registry.release()`/`ForceRelease()` already guard against double-invocation at the `Registry`
  layer (Epic 2.5 Story 2.5.3's `sync.Once` per-`Acquire` release) — this task's own idempotency
  guard is defense in depth, not the only line of defense.
- Regression test: exercises `pitfalls-registry.md` §3's Test 2
  (`TestRegistry_AcquireDuringTeardown_NeverReusesADyingActor`) against this task's real
  `stopActor()` body — the acceptance criteria explicitly calls this test out as the one that
  would fail (via a race, not deterministically — the hardest kind of gap to catch in review) if
  `stopActor()` regresses to a bare `cancel()`.

#### Task 3.1c: Give `newLiveInstance`'s `stopActor()` its real body; extend `finishInstanceConstruction` to spawn `runActor` (~45 min)
- `session/instance.go`: extend the shared `finishInstanceConstruction` helper introduced in
  Epic 1 Task 1.1c to also `go i.runActor()` after the initial snapshot publish. Because Epic
  2.5 already made `session.Registry`'s internal `newLiveInstance` the **only** caller of this
  helper (no external package calls `NewInstance`/`FromInstanceData`/`handleNewSession`/
  `SessionToInstance` directly after Epic 2.5 Story 2.5.2), this single edit gives every live
  handle an actor — there is no separate "wire up N construction sites" step, because Epic 2.5
  already collapsed them to one.
- Give `LiveInstance.stopActor()` (referenced by `Registry.release()`/`ForceRelease()` since
  Epic 2.5 Story 2.5.3) its real body per Task 3.1b: `i.cancel(); <-i.done` — **blocking** until
  `runActor()`'s own `defer close(i.done)` fires, not a bare `cancel()`-and-return (this is the
  adversarial-review.md finding 3 fix; see Task 3.1b for the full rationale and the `done`
  channel's placement in `runActor()`). Pre-this-task, `stopActor()` was a no-op; `Registry`'s
  map/refcount logic itself does not change.
- `server/services/session_service.go`'s `DeleteSession` already calls
  `registry.ForceRelease(sessionID)` per Epic 2.5 Story 2.5.9's conversion — no further change
  needed here; confirm during this task that `ForceRelease`'s call to `stopActor()` now actually
  tears down a live goroutine instead of no-op'ing.

#### Task 3.1d: goleak-based actor-goroutine-leak regression test — reuses Epic 2.5's `Registry`-based harness (~20 min)
- Per this plan's testing-retarget requirement (retarget, don't duplicate): this test goes
  through `Registry.Acquire`/`release()` (Epic 2.5 Story 2.5.10's Test 4), not raw
  `FromInstanceData`/`Stop()` construction — there is no "raw construction" path any external
  test should exercise, since Epic 2.5 made `Registry` the only entry point.
- Add `go.uber.org/goleak` as a direct test dependency if Epic 2.5 hasn't already (confirm via
  `grep -rl goleak --include=*.go .`).
- Re-run Epic 2.5 Story 2.5.10's Test 4/Test 5 now that `stopActor()` has a real body (this
  task's Task 3.1c) — pre-this-task those tests verified dedup/no-double-construction only;
  post-this-task they additionally verify actual goroutine teardown. Document both runs'
  results in this epic's PR description, same "re-run and document" pattern this plan uses
  elsewhere (Story 1.3).

#### Task 3.1e: Confirm the ADR-029 shutdownHooks safety-net trigger — already registered by Epic 2.5, verify only (~15 min)
- ADR-029 requires **two** triggers for actor teardown: the primary (deletion, Task 3.1c/Epic
  2.5 Story 2.5.9) and a secondary safety net. Epic 2.5 Story 2.5.5 already registered
  `Registry.Shutdown()` (Epic 2.5 Story 2.5.3) as a `shutdownHooks` entry
  (`server/server.go:51,173,387,393,400`'s existing pattern) — `Registry.Shutdown()` iterates
  its own `entries` map and calls `stopActor()` on each, which achieves exactly what this task
  originally proposed doing via `ReviewQueuePoller.GetInstances()`.
- **Do not** register a second, competing `shutdownHooks` entry here — `Registry.Shutdown()`
  already covers every session `Registry` knows about, which (per Epic 2.5's DI wiring) is now
  the same set `ReviewQueuePoller.GetInstances()` would have enumerated, since
  `ReviewQueuePoller` itself is a `Registry` consumer (Epic 2.5 Story 2.5.8). Confirm this
  coverage is actually equivalent during this task (e.g. assert `Registry`'s entry count
  matches `ReviewQueuePoller.GetInstances()`'s count in a running test scenario) rather than
  re-adding the old mechanism.
- Extend the goleak test from Task 3.1d to cover the shutdown path too: start N instances (via
  `Registry.Acquire`), invoke `Registry.Shutdown()` (not just per-instance `release()`/
  `ForceRelease`), assert no actor goroutines remain.

### Story 3.2: Migrate single-writer fields to actor commands

**As a** developer validating the actor design under real load before betting the state
machine on it, **I want** the lowest-traffic, single-writer mutators converted first,
**so that** `send`/`sendSync` are exercised in production with minimal blast radius.

**Acceptance Criteria**:
- `MarkViewed`, `MarkAcknowledged` (or equivalent — confirm exact method names in
  `session/instance_state.go:108-121` region), and `SetLastMeaningfulOutput` route through
  `send`/`sendSync` instead of `stateMutex.Lock()`.
- Confirmed via grep (not assumption) that no other goroutine writes these specific fields
  directly anywhere in `server/services/` or `session/` — if one is found, exclude that
  field from this story and handle it in Epic 4/5 instead.
- Each method's internal logic moves into an unexported `xxxLocked(s *instanceState, ...)`
  twin (establishing the pattern Epic 4 depends on at larger scale) — the public method
  becomes `func (i *Instance) MarkViewed() { i.send("MarkViewed", func(s *instanceState) { markViewedLocked(s) }) }`.

**Files**: `session/instance_state.go` (or wherever these methods currently live —
confirm exact file during Task 3.2a).

#### Task 3.2a: Grep-confirm single-writer status for the 3 candidate fields (~15 min)
- `grep -rn "LastViewed\s*=\|Acknowledged\s*=\|LastMeaningfulOutput\s*=" session/ server/services/`
  — confirm each assignment site is inside the method being converted, not elsewhere.

#### Task 3.2b: Convert `MarkViewed`/`MarkAcknowledged` to `xxxLocked` + `send` (~30 min)
- Extract current lock-held body into `markViewedLocked(s *instanceState)` /
  `markAcknowledgedLocked(s *instanceState)`; public method becomes a thin `send` wrapper.

#### Task 3.2c: Convert `SetLastMeaningfulOutput` to `xxxLocked` + `send` (~20 min)
- Same transform. Note `ReviewState.lastMeaningfulOutputNs`'s existing `atomic.Int64`
  shadow (`session/review_state.go:77-96`) — leave that dual-representation as-is for now
  (`pitfalls.md` §5 explicitly says don't generalize it, but also don't break it); the
  actor-routed setter should still call `SyncAtomicTimestamps()` from inside `xxxLocked`.

#### Task 3.2d: `require.Eventually`-based async test for each converted method (~30 min)
- Per `pitfalls.md` §6 Pattern 2: `inst.MarkViewed(); require.Eventually(t, func() bool { return !inst.Snapshot().LastViewed.IsZero() }, time.Second, time.Millisecond)`.

#### Task 3.2e: Add a `testFlush()` test helper (~20 min)
- Per `pitfalls.md` §6: a no-op sentinel command with an ack channel, giving tests an exact
  "everything sent so far has been applied" barrier without polling. Add to
  `session/instance_actor.go` (test-only build tag or exported only for `_test.go` use, per
  existing repo convention — check how other actor-adjacent test helpers are scoped).

---

## Epic 4: State-Machine Core Migration — ATOMICITY GATE

**Architecture ref**: `architecture.md` §3, §4, §7 PR4; `pitfalls.md` §2, §3.
**Risk**: highest in this plan — `transitionTo`, `Pause`, `Approve`, `Deny`,
`StartController`/`StopController`, hibernate/resume, and `SwitchWorkspace` all call each
other today under the same `stateMutex`. Per `architecture.md` §7: *"these call each other,
so the internal/public (xxxLocked) split must land consistently across the whole cluster in
one pass or the self-deadlock hazard reappears at the migrated/unmigrated boundary."*

**>>> Atomicity gate: Stories 4.1–4.6 must merge to `main` together, in one PR or one tight
merge sequence with no intermediate state reachable in production. Do not ship Story 4.2
without 4.1/4.3/4.4 landed in the same change — a half-converted cluster reintroduces
exactly the bug class this migration exists to remove (per `pitfalls.md` §2's three prior
incidents: `wireRateLimitCallbacks`, `CreateCheckpoint`/`tryExtractConversationUUID`, and the
live doc/code contradiction in `instance_claude.go:291-293`). <<<**

### Story 4.1: `xxxLocked` twins for the state-machine core

**As a** developer converting compound operations to actor commands, **I want** every
public method in this cluster split into a lock-free internal twin operating on
`*instanceState`, **so that** command handlers can call each other directly (ordinary
function calls, no re-send) without risking the self-`sendSync` deadlock `pitfalls.md` §2
identifies as "the same bug class, not a new one."

**Acceptance Criteria**:
- `transitionTo` (`session/instance_state.go:32`) → `transitionToLocked(s *instanceState, ctx context.Context, to Status) error`, preserving `transitionIndex` guard/after-hook semantics (`state_machine.go:29-31`) exactly.
- `Pause`/`Approve`/`Deny` → `pauseLocked`/`approveLocked`/`denyLocked(s *instanceState, ...)`.
- `StartController`/`StopController`/`GetController` (`session/instance_controller.go:19,136,150`) → internal twins; per `pitfalls.md` §2 point 1, the existing `wireRateLimitCallbacks` fix (passing the controller in directly instead of fetching via `GetController()`) is the precedent — preserve that call-site discipline inside the actor too.
- Hibernate/resume (`hibernateProcess`/`resumeFromHibernation`) → internal twins; the
  `go func()` workaround in `state_machine.go:44-53` (spawned specifically to avoid
  re-entering `stateMutex`) is **inlined directly into the `After` closure** — the actor
  model removes the reason for the extra goroutine hop (`architecture.md` §3).
- `CreateCheckpoint`'s existing "extract before lock" ordering (`instance_checkpoint.go:28-115`,
  `tryExtractConversationUUID` called before any lock) is preserved verbatim inside the
  actor command — same call-order rule, now enforced by command-handler structure instead
  of a comment (closing the live doc/code contradiction `pitfalls.md` §2 point 3 found at
  `instance_claude.go:291-293`).
- Internal twins are **functions taking `*instanceState`, not methods on `*Instance`** (per
  `architecture.md` §4's ast-grep-enforceability note) — this is what makes Story 4.5's lint
  rule possible.
- Public methods become thin: `func (i *Instance) Pause() error { return sendSyncErr(i, "Pause", func(s *instanceState) error { return pauseLocked(s) }) }`.

**Files**: `session/instance_state.go`, `session/state_machine.go`,
`session/instance_controller.go`, `session/instance_hibernate.go`,
`session/instance_checkpoint.go`.

#### Task 4.1a: Extract `transitionToLocked` + wire through `sendSyncErr` (~45 min)
- `session/instance_state.go:32` — move body into `transitionToLocked`, given existing
  callers inside this file/cluster call the `Locked` twin directly, external callers
  (outside this cluster — Epic 5's `review_queue_poller.go`/`instance_controller.go` EOF
  callback) keep calling the public `transitionTo` until Story 4.3/4.4.

#### Task 4.1b: Extract `pauseLocked`/`approveLocked`/`denyLocked` (~45 min)
- Same file or `session/instance_terminal.go` (confirm exact current location of `Pause`/
  `Approve`/`Deny` first), following the `Pause()` example in `architecture.md` §2.

#### Task 4.1c: Extract `StartController`/`StopController` internal twins (~30 min)
- `session/instance_controller.go:19,136` — preserve the `wireRateLimitCallbacks` fix's
  "pass controller in directly" discipline inside the new `startControllerLocked`/
  `stopControllerLocked`.

#### Task 4.1d: Inline hibernate/resume `After`-hook logic, removing the `go func()` hop (~45 min)
- `session/state_machine.go:44-53` — replace `go i.hibernateProcess(ctx)` /
  `go i.resumeFromHibernation(ctx)` with direct in-closure calls to the new
  `hibernateProcessLocked`/`resumeFromHibernationLocked`. If I/O inside these (checkpoint
  write, tmux kill) risks head-of-line-blocking other queued commands for this `Instance`
  (per `architecture.md` §3's closing note), dispatch that specific I/O to a detached
  goroutine that re-enters only via a normal external `send(...)` on completion — never a
  callback into the actor directly.

#### Task 4.1e: Verify `CreateCheckpoint`'s extract-before-lock ordering survives the conversion (~20 min)
- `session/instance_checkpoint.go:28-115` — the `tryExtractConversationUUID` call must
  remain the first statement in the command closure, before any `s.inst` field mutation.
  Update the stale comment in `instance_claude.go:291-293` to match reality (it currently
  claims the opposite of what the code does — close out `pitfalls.md` §2 point 3).

### Story 4.2: `SwitchWorkspace` → actor command, `Start()` reentrancy retired

**As a** developer finishing the work Item 1 started under the mutex, **I want**
`SwitchWorkspace` converted to a single command closure that calls `startLocked` directly,
**so that** the `Start()`-reentrancy hazard becomes structurally impossible instead of
fixed-by-discipline.

**Acceptance Criteria**:
- `SwitchWorkspace` (`session/instance_workspace.go:75-219`) becomes one `sendSync` command.
- The 3 call sites currently calling `i.Start(false)` after Item 1's `unlock()` (currently at
  lines **162, 212, 222** post-Item-1 — not the 148/197/206 originally cited in
  `requirements.md`/`pitfalls.md`; re-verify current line numbers before converting, since the
  file may shift again during Epic 1) are rewritten to call `startLocked(s, false)` directly —
  `architecture.md` §4: *"a deliberate, auditable step, not automatic."*
- `i.KillSession()` (called twice, currently at lines **155, 194** post-Item-1 — not the
  142/180 originally cited in `requirements.md`/`pitfalls.md`; re-verify current line numbers
  before converting, since the file may shift again during Epic 1) and
  `switchRevision`/`switchWorktree` (currently at lines **203, 205** — not 189/191; same
  re-verify caveat) also get internal twins per Story 4.1's pattern, called directly inside
  the command.
- Item 1's regression test (R1.3, the existing timeout-bounded test in
  `session/instance_workspace_test.go`) is **kept and re-targeted** at the actor-based
  `SwitchWorkspace` — same done-channel-plus-`time.After(1s)` shape, now exercising the
  command path instead of the mutex path (`pitfalls.md` §6 Pattern 1, `architecture.md` §4).

**Files**: `session/instance_workspace.go`, `session/instance_workspace_test.go`,
`session/instance.go` (`startLocked`).

#### Task 4.2a: Extract `startLocked(s *instanceState, firstTimeSetup bool) error` (~30 min)
- `session/instance.go:696,714` (`Start`/`start`) — move logic into `startLocked`; public
  `Start()` becomes `sendSyncErr(i, "Start", func(s *instanceState) error { return startLocked(s, firstTimeSetup) })`.
- **`startMu`/`restartMu` analysis (do not skip)**: `session/instance.go`'s `startMu`
  (`deadlock.Mutex`, line ~335) is acquired at the very first line of `start()` (line ~718,
  `i.startMu.Lock(); defer i.startMu.Unlock()`) — the exact function this task converts. It and
  `restartMu` (line ~347) are separate lock fields from `stateMutex`, absent from every mutex
  catalog in `requirements.md`/`pitfalls.md`/`architecture.md` and from this plan until now.
  While extracting `startLocked`, determine whether `startMu`/`restartMu` become redundant
  under the actor's natural per-Instance command serialization (only one command executes at a
  time per `Instance` once Epic 3+4 land) — or whether some code path still calls
  `start()`/`Start()` outside the actor, in which case `startMu` and the actor's serialization
  would no longer agree on what "concurrent" means. Document the finding here; Epic 7 Story
  7.1 makes the final remove-or-retain call once the whole cluster is actor-routed.

#### Task 4.2b: Convert `SwitchWorkspace` to a single command, rewriting the 3 `Start` call sites (~1 hr)
- `session/instance_workspace.go:85-219` — wrap the whole body in one `sendSyncErr` command;
  replace `i.Start(false)` at all 3 sites with `startLocked(s, false)`. Extract
  `killSessionLocked`/`switchRevisionLocked`/`switchWorktreeLocked` twins for the other
  internal calls in this method.

#### Task 4.2c: Re-target the existing `SwitchWorkspace` regression test at the actor path (~20 min)
- `session/instance_workspace_test.go` — same timeout-bounded shape, now calling the
  command-routed `SwitchWorkspace`. Confirm it still fails fast (not hangs) if someone
  reintroduces a `sendSync`-on-self mistake.

### Story 4.3: External `transitionTo` callers converted in the same pass

**As a** developer closing the migrated/unmigrated boundary risk, **I want** every external
caller of `transitionTo` converted to send a command rather than call the public method
directly while assuming lock semantics, **so that** Epic 4 doesn't leave a half-migrated
caller outside the cluster.

**Acceptance Criteria**:
- `instance_controller.go:61-67`'s `if i.Status == Active { i.transitionTo(ctx, Stopped) }`
  (today atomic only because the EOF callback runs under the same lock acquisition) is
  rewritten so the **precondition check happens inside the command handler**, not by the
  caller before sending — per `pitfalls.md` §3's TOCTOU fix: a `TransitionCommand{From:
  Active, To: Stopped}` (or equivalent) checked against current state when the actor
  processes it, no-op or typed failure if the precondition no longer holds.
- `review_queue_poller.go:437-467`'s `reconcileSessions` (`if inst.Status == Active {...}`/
  `if inst.Status == Stopped {...}`, today running under a lock acquired by the *poller*)
  is rewritten the same way — one command per transition attempt, precondition checked
  actor-side, sent via `sendCtx` with a short timeout (poller sweeps many instances; one
  wedged actor must not stall the whole sweep, per `architecture.md` §5).
- `session/instance.go:737-746`'s `SetOnExitCallback` closure inside `start()` — structurally
  identical to the `instance_controller.go:61-67` site above (a check-then-transition,
  `i.stateMutex.Lock(); if i.Status == Active { i.transitionTo(...) }; i.stateMutex.Unlock()`,
  fired from an external goroutine — here, the tmux control-mode reader; there, the
  `ClaudeController`'s exit detector) — is converted the same way: the precondition check moves
  inside the command handler, not the callback. **Special hazard**: this closure is lexically
  defined *inside* `start()`, the function Task 4.2a converts to `startLocked(s *instanceState,
  ...)`. Once that conversion lands, `s` is in lexical scope inside this closure, creating a
  real risk that an implementer mechanically rewrites it to call `transitionToLocked(s, ...)`
  directly because it "looks like" it should (it's textually inside a `Locked` function) — that
  would be **wrong**: this closure executes on the tmux-reader goroutine at callback-fire time,
  not on the actor goroutine at command-process time, so calling a `*instanceState`-taking
  internal twin directly from it reintroduces an unsynchronized direct mutation, exactly the bug
  class this migration removes. The correct fix is the same as the other two sites in this
  story: the closure calls the actor via `send`/`sendCtx` (not a direct `Locked` twin call), and
  the precondition (`Status == Active`) is (re-)checked inside that command's handler at
  processing time. Story 4.5's ast-grep lint rule does **not** catch this specific mistake (it
  only flags a `Locked`-taking function calling a public mailbox-routed method on `i`, not a
  lexically-nested-but-different-goroutine closure calling a `Locked` twin) — call this out
  explicitly in code review for this task.

**Files**: `session/instance_controller.go`, `session/review_queue_poller.go`,
`session/instance.go`.

#### Task 4.3a: Convert the EOF-callback transition check (~30 min)
- `session/instance_controller.go:61-67` — replace the check-then-call with a
  precondition-bearing command per the TOCTOU fix above.

#### Task 4.3b: Convert `reconcileSessions`'s two check-then-transition sites (~45 min)
- `session/review_queue_poller.go:442-449,457-463` — same fix, using `sendCtx` with a
  short (e.g. 2s) timeout per-instance so one stuck actor doesn't stall the sweep.

#### Task 4.3c: Convert the `start()`-internal exit-callback transition check (~30 min)
- `session/instance.go:737-746` (the `SetOnExitCallback` closure inside `start()`) — same
  precondition-in-handler fix as Task 4.3a. Do **not** rewrite this closure to call
  `transitionToLocked(s, ...)` directly even though `s` is lexically in scope after Task 4.2a's
  conversion — this closure runs on the tmux-reader goroutine, not the actor goroutine; route
  it through `send`/`sendCtx` like Task 4.3a/4.3b. Add a code comment at the closure site
  calling out why it must not call the `Locked` twin directly, since the lint rule (Story 4.5)
  won't catch a regression here.
- **Rename/extract mitigation (`type-driven-audit.md` finding C)**: a capability-token type was
  investigated and rejected for this hazard — an unexported marker struct (e.g. `type
  actorToken struct{}`) buys nothing here because the mistake occurs *within* the same package,
  same file, same function being converted; any code in `session/` can already construct an
  empty struct literal and pass it through, so the token proves nothing about which goroutine is
  actually calling. Apply the two lighter, structural mitigations instead, since the residual
  risk here is purely visual/naming, not a type-system gap:
  1. Rename `startLocked`'s parameter from `s` to something that signals "actor-only" (e.g.
     `actorState`) — reduces the visual temptation for a future edit to treat any
     lexically-nested closure as automatically safe to call it with, since the name itself now
     reads as a claim about which goroutine owns it.
  2. Define this exit-callback as a **named, top-level function** taking `i *LiveInstance` (not
     an inline closure defined lexically inside `start()`/`startLocked`) that itself calls
     `i.send(...)`/`i.sendCtx(...)` — moving the callback out of `startLocked`'s lexical scope
     entirely removes `s`/`actorState`'s visual reachability from the callback body, which is
     the actual mechanism of the hazard (Task 4.3c's fix already routes through `send`/`sendCtx`;
     this additionally removes the temptation for a future edit to reintroduce the mistake).

### Story 4.4: `UpdatePRStatus` / `PRStatusPoller.applyPRUpdate` TOCTOU fix

**As a** developer fixing the existing inconsistent-locking bug on the GitHub PR fields
(`requirements.md` background: "locked on one writer, unlocked on another"), **I want** the
priority-changed decision made inside the command handler itself, **so that** the
diff-across-two-independently-fetched-values race (`pitfalls.md` §3) doesn't get copied
into the new architecture as a pattern other authors imitate.

**Acceptance Criteria**:
- `UpdatePRStatus` (`session/instance_terminal.go:247-258`) becomes one command handling
  all 8 fields (R2.5: one command, one resulting snapshot, no partial-update visibility).
- The handler itself computes `priorityChanged := newPriority != s.inst.GitHubPRPriority`
  (or equivalent) **before** mutating, and returns/acts on that result directly — `PRStatusPoller.applyPRUpdate` (`session/pr_status_poller.go:368-406`) no longer reads
  `oldPriority` itself and diffs after the fact; it receives the changed-or-not result from
  the command's response.

**Files**: `session/instance_terminal.go`, `session/pr_status_poller.go`.

#### Task 4.4a: Convert `UpdatePRStatus` to a single command computing its own diff (~45 min)
- `session/instance_terminal.go:247-258` — `sendSync` returning a small result struct (e.g.
  `{PriorityChanged bool}`) per `stack.md` §4 idiomatic detail #3.

#### Task 4.4b: Update `applyPRUpdate` to consume the command's result instead of diffing externally (~20 min)
- `session/pr_status_poller.go:368-406` — remove the separate RLock-read-then-compare;
  fire `onUpdated` based on the command's returned `PriorityChanged`.

### Story 4.5: ast-grep lint rule against self-`sendSync` calls

**As a** developer preventing recurrence of the exact bug class this migration exists to
remove, **I want** an automated check that flags any function taking `*instanceState` that
calls a public, mailbox-routed method on `i` (e.g. `i.Start(...)`, `i.Pause()`), **so that**
the next contributor can't reintroduce the Item-1-shaped deadlock undetected.

**Acceptance Criteria**:
- An `ast-grep`/`sg` rule (or equivalent CI lint step) matches `i\.<PublicMethod>\(` calls
  inside any function whose signature includes a `*instanceState` parameter, flagging it as
  an error — per `architecture.md` §4's enforceability note ("keep internal helpers as
  functions taking `*instanceState`, not methods on `*Instance`, so an ast-grep rule ...
  can catch a closure calling the public, mailbox-routed API on itself").
- **Known, documented gap this rule does NOT catch (`type-driven-audit.md` finding C,
  confirmed by this plan's own Task 4.3c)**: a closure literal lexically nested inside a
  `*instanceState`-taking function (e.g. `startLocked`) that both references the outer
  actor-state parameter *and* is passed to something shaped like callback registration
  (`SetOnExitCallback`, `RegisterCompletionCallback`, `RegisterTurnCallback`) but then calls a
  `Locked` twin (e.g. `transitionToLocked(s, ...)`) directly instead of routing through
  `send`/`sendCtx`. This rule's base pattern only flags a `*instanceState`-taking
  function/method whose *own* body calls a public mailbox-routed method — it cannot detect
  that a nested closure, passed out to a different goroutine's callback registration, still
  closes over and calls a `Locked` twin. **Tighten the rule** to additionally flag: any closure
  literal lexically nested inside a `*instanceState`-taking function that (a) references the
  outer `*instanceState` parameter (by whatever name, per Task 4.3c's rename mitigation) in its
  body, AND (b) is passed as an argument to a call matching `Register\w*Callback\(|SetOn\w*Callback\(`
  — i.e. "a closure escaping via callback registration that still touches the actor-only
  state." This is structurally pattern-matchable with ast-grep (closure-literal-as-argument to
  a callback-registration call site, referencing an outer parameter) — no runtime capability
  type is needed; a capability-token type was investigated and rejected for this same hazard
  (see Task 4.3c) since the mistake occurs within the same package/file/function, where an
  unexported marker type proves nothing about caller-goroutine identity.
- Rule is wired into `make lint` (or a new `make` target documented in this migration's
  final PR) so it runs in CI, not just locally.
- Rule is validated against a deliberately-reintroduced violation (temporarily) to confirm
  it actually fires, then the violation is reverted — validate against **both** the base
  pattern (a `*instanceState`-taking function calling a public method on `i` directly) and the
  tightened closure-escaping-via-callback-registration pattern above, since they are two
  distinct match shapes.

**Files**: new ast-grep rule config (location depends on this repo's existing `sg` rule
directory — check for one before creating a new convention), `Makefile` (`lint` target).

#### Task 4.5a: Write the ast-grep rule (~45 min)
- Pattern 1 (base): a function/method with a `*instanceState` parameter whose body contains
  `i.<Identifier>(...)` where `<Identifier>` is a public method on `*Instance` known to be
  mailbox-routed (the `xxxLocked` twins are exempt by construction since they take
  `*instanceState`, not `*Instance`, as the receiver-equivalent).
- Pattern 2 (closure-escaping-via-callback-registration, `type-driven-audit.md` finding C):
  a closure literal, lexically nested inside a `*instanceState`-taking function, that both
  references the outer actor-state parameter and is passed directly as an argument to a call
  matching `Register\w*Callback\(|SetOn\w*Callback\(` — this is the mistake Task 4.3c's
  `SetOnExitCallback` closure is the canonical example of, and the base pattern above provably
  does not catch it (the nested closure is not itself a `*instanceState`-taking function/method
  — it's a closure literal passed as a value).

#### Task 4.5b: Wire into `make lint`; validate against a deliberate violation (~30 min)
- Temporarily reintroduce `i.Start(false)` inside a `*instanceState`-taking function,
  confirm the base pattern (Pattern 1) fires, revert.
- Temporarily reintroduce a closure literal referencing the outer `*instanceState` parameter
  and passed to a `SetOnExitCallback`-shaped call that then calls a `Locked` twin directly,
  confirm the tightened pattern (Pattern 2, `type-driven-audit.md` finding C) fires, revert.

### Story 4.6: Regression sweep and manual verification

**Acceptance Criteria**:
- `go test -race ./session/... ./server/services/...` passes.
- Item 1's re-targeted regression test (Story 4.2c) passes under `-race` and on its own
  with a tight timeout.
- Manual pass: pause → resume → hibernate → resume → program-switch, confirmed identical
  to pre-migration behavior (R2.9).

**Files**: none (verification only).

#### Task 4.6a: Run the full race-detector + manual verification pass (~45 min)
- Document results in the PR description for this epic's merge.

---

## Epic 5: `server/services/session_service.go` + Background-Goroutine Writer Cutover — ATOMICITY GATE

**Architecture ref**: `architecture.md` §7 PR5-7; `pitfalls.md` §4. **Depends on**: Epic 4's
`xxxLocked` twins and command-handler pattern.

**>>> Atomicity gate, stated explicitly per `pitfalls.md` §4: every direct writer of a
shared `Instance` field — `session_service.go`'s ~28 sites, `autonomous_driver.go`'s two
background-goroutine callbacks, and `pr_status_poller.go`'s remaining writes — must convert
to commands in the same merged change that flips the actor live for these fields. Splitting
this epic into Stories 5.1–5.3 is for implementation tractability (parallelizable review,
smaller diffs to read) — it is NOT permission to merge Story 5.1 to `main` while Story 5.3
is still in flight. If `GitHubPRURL`/`GitHubPRNumber` (today: "locked on one writer
(PRStatusPoller), unlocked on another (RunOneShot)" per `requirements.md`) end up
half-converted across a merge boundary, the unconverted writer's direct assignment will be
silently clobbered by the actor's next snapshot publish — deterministic data loss. Land all
of 5.1–5.4 behind one feature branch / merge window; do not ship piecemeal. <<<**

### Story 5.1: Session creation path cluster

**As a** developer converting the highest-traffic write cluster, **I want** every direct
field write in the session-creation flow converted to a command, **so that** creation
behaves identically while no longer racing the actor.

**Acceptance Criteria**:
- Every direct `inst.Field = ...` / `i.Field = ...` assignment in `session_service.go`'s
  `CreateSession` path (including the async `CreateSession` goroutine and the
  already-worked-around race at `session_service.go:1280-1281` per `requirements.md`
  background) is replaced with a `send`/`sendSync` command using the relevant `xxxLocked`
  twin (extracting one if Epic 4 didn't already cover this field).
- The manual workaround comment at `session_service.go:1280-1281` is removed — the actor
  model makes the underlying race structurally impossible, so the comment's workaround is
  no longer needed (confirm by reading the comment's actual mitigation and ensuring the
  command-based path doesn't need it).

**Files**: `server/services/session_service.go` (creation path only).

#### Task 5.1a: Catalog every direct write in the creation path (~30 min)
- `grep -n "inst\.\w\+ = \|i\.\w\+ = " server/services/session_service.go` scoped to the
  `CreateSession` function and its async goroutine — produce the exact list before
  converting (don't convert by memory of `requirements.md`'s "~28 sites" estimate; verify
  current line numbers since the file may have shifted since that audit).

#### Task 5.1b: Convert each catalogued write site to a command (~2-3 hr, split further per
sub-cluster if the catalog from 5.1a is large)
- For each site: if an `xxxLocked` twin for that field already exists from Epic 4, call it;
  otherwise extract one following the established pattern.

#### Task 5.1c: Remove the stale workaround comment at `session_service.go:1280-1281` and
add a regression test proving the underlying race is gone (~30 min)

### Story 5.2: Pause/resume/rename/program-switch RPC cluster

**Acceptance Criteria**:
- All direct field writes in the pause/resume/rename/program-switch RPC handlers convert to
  commands (most of `Pause`/`Approve`/`Deny`/`transitionTo` is already actor-routed from
  Epic 4 — this story covers `session_service.go`-side fields these RPCs touch directly,
  e.g. metadata fields not owned by the state machine itself).
- R2.9 holds: `UpdateSession` (pause/resume/rename/program-switch) behaves identically from
  the client's perspective — this is the most recently-touched area per `git log` (the
  working tree's uncommitted `RuleBuilderForm.tsx`/`ApprovalRulesPanel.tsx` changes and the
  recent `fix(session): program switching now saves correctly for all cases` commit suggest
  this path is actively fragile; be conservative and test thoroughly here).

**Files**: `server/services/session_service.go` (RPC handlers).

#### Task 5.2a: Catalog direct writes in pause/resume/rename/program-switch handlers (~30 min)
- Same grep-and-verify approach as Task 5.1a, scoped to these RPC handlers.

#### Task 5.2b: Convert each site to a command (~2 hr)

#### Task 5.2c: Manual + automated regression test for program-switch specifically (~30 min)
- Given the recent `86a02b16 fix(session): program switching now saves correctly for all
  cases` commit, add an explicit test asserting program-switch persists correctly through
  the actor path, not just through manual click-testing.

### Story 5.3: PR-status + review-queue writer cluster

**Acceptance Criteria**:
- The two AutonomousDriver-related background-goroutine callbacks that write `Instance`
  fields directly are converted to commands. **These callback bodies live in
  `server/services/session_service.go`, not `session/autonomous_driver.go`** —
  `session/autonomous_driver.go` only *defines* the callback types (`CompletionCallback`,
  `TurnCallback`) and their registration methods (`RegisterCompletionCallback`,
  `RegisterTurnCallback`); it contains zero direct `Instance` field assignments. The actual
  write sites are:
  - `server/services/session_service.go`'s `buildTurnCallback` (~line 758) — writes
    `liveInst.AutonomousTurn`/`liveInst.AutonomousMaxTurns` (~lines 761-762)
  - `server/services/session_service.go`'s `onAutonomousDriverComplete` (~line 3666) — writes
    `inst.AutonomousMode`/`AutonomousTurn`/`AutonomousMaxTurns`/`AutonomousOutcome` (~lines
    3684-3690)
  Both are registered via `driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)` /
  `driver.RegisterTurnCallback(s.buildTurnCallback(inst))` at
  `session_service.go:788-789,806-807,1339`. Verify current line numbers before converting —
  this file shifts frequently.
- `pr_status_poller.go`'s remaining direct writes (beyond `UpdatePRStatus`, already handled
  in Epic 4 Story 4.4) — e.g. any writes around `pr_status_poller.go:301-303,328-330` per
  `pitfalls.md` §4 — convert to commands.
- `review_queue_poller.go`'s writes (beyond the `reconcileSessions` transitions handled in
  Epic 4 Story 4.3) convert to commands.

**Files**: `server/services/session_service.go` (`buildTurnCallback`,
`onAutonomousDriverComplete` — the actual AutonomousDriver write sites), `session/pr_status_poller.go`,
`session/review_queue_poller.go`. (`session/autonomous_driver.go` itself needs no field-write
conversion — it has none — but confirm during Task 5.3a that this hasn't changed.)

#### Task 5.3a: Catalog the AutonomousDriver callback write sites in `session_service.go` (~20 min)
- `grep -n "AutonomousTurn\s*=\|AutonomousMaxTurns\s*=\|AutonomousMode\s*=\|AutonomousOutcome\s*=" server/services/session_service.go`
  — confirm current line numbers for `buildTurnCallback` and `onAutonomousDriverComplete`
  (cited above as ~758-762 and ~3666-3690; re-verify, do not assume). Do **not** grep
  `session/autonomous_driver.go` expecting to find the writes there — it only defines the
  callback types/registration methods, not the write bodies.

#### Task 5.3b: Convert `buildTurnCallback`/`onAutonomousDriverComplete` in `session_service.go` to commands (~1 hr)
- Route each callback's field writes through `send`/`sendSync` against the relevant `Instance`,
  using or extracting an `xxxLocked` twin per Story 4.1's pattern.

#### Task 5.3c: Catalog and convert `pr_status_poller.go`'s remaining direct writes (~45 min)
- Verify current line numbers for the `pitfalls.md`-cited `301-303,328-330` region before
  converting (file may have shifted).

#### Task 5.3d: Catalog and convert `review_queue_poller.go`'s remaining writes (~45 min)

### Story 5.4: CI guard against new direct field writes outside `session/instance*.go`

**As a** developer preventing regression of this epic's invariant, **I want** a CI check
that fails if any future commit adds a direct `inst.Field = ` / `i.Field = ` assignment in
`server/services/`, `daemon/daemon.go`, or other non-`session/instance*.go` files, **so that**
"missing one call site" becomes a CI failure instead of a silent reintroduction of the
clobbering bug.

**Acceptance Criteria**:
- A grep-based (or ast-grep-based) CI check scans `server/services/`, `session/
  autonomous_driver.go`, `session/pr_status_poller.go`, `session/review_queue_poller.go`,
  **and `daemon/daemon.go`** for `\w+\.\w+\s*=\s*` patterns matching known `Instance`-typed
  variable names, excluding legitimate local-variable assignments — tune for signal, following
  `pitfalls.md` §4's suggested approach ("a CI grep check ... would catch a forgotten call site
  before merge — same spirit as this repo's existing registry-coverage CI checks").
  **`daemon/daemon.go` addition closes the fifth-pass adversarial-review.md non-blocking
  finding**: Task 2.5.7i formally routes `daemon.go` through `session.Registry`/
  `session.LiveInstance` for the first time, and `daemon.go:43,312`'s direct `instance.AutoYes
  = true` writes were never in `requirements.md`'s original writer catalog nor this story's
  original file list — this closes that gap.
- Wired into `make ci` or a dedicated `make` target documented alongside `make
  registry-diff`'s existing pattern.
- **This CI guard is superseded, but not yet replaced, by Epic 7 Story 7.1e's field-unexport
  change** (`type-driven-audit.md` finding A): once `LiveInstance`'s mutable fields are
  unexported, a direct field write becomes a `go build` failure — a strictly stronger,
  compiler-enforced guarantee that needs no file list kept in sync by hand, including for files
  (like `daemon/daemon.go`, added above) that a hand-maintained grep scope could still miss in
  the future. This guard is **not** dead weight in the meantime, though: Epic 5 (where this
  story lands) merges before Epic 7 does, so for the transitional period between the two, this
  grep-based check is the only thing enforcing the invariant at all — keep it wired into CI
  through that window and only consider retiring it once Story 7.1e has landed and a full
  `go build ./...` pass confirms no direct-write call site survives without it.

**Files**: new script (location: follow existing convention, e.g. alongside
`make registry-generate`'s tooling), `Makefile`.

#### Task 5.4a: Write and tune the grep/ast-grep CI guard (~45 min)
- Test against the pre-Epic-5 codebase (should fail, given the known direct writes) and
  post-Epic-5 codebase (should pass) to confirm correct signal. Include `daemon/daemon.go`'s
  `AutoYes` writes in the pre-fix failing case and Task 2.5.7i's converted form in the
  post-fix passing case.

#### Task 5.4b: Wire into CI (~15 min)

---

## Epic 6: `instance_tmux.go` RLock-Across-I/O Migration

**Architecture ref**: `architecture.md` §5, §7 PR8; `requirements.md` R2.8.
**Depends on**: Epic 4's `Status`-check synchronization semantics being stable (architecture.md:
"these depend on synchronization semantics PR 4 changes the synchronization story for").

### Story 6.1: Convert the 10 `RLock()`-across-tmux-subprocess-I/O sites

**As a** developer removing the last category of lock-held-across-slow-I/O call sites,
**I want** each site to read its precondition from the snapshot, release any lock
conceptually (there is none once Epic 4/5 land), and perform tmux I/O unguarded, **so that**
a slow tmux subprocess call never blocks an actor command or another reader.

**Acceptance Criteria**:
- All 10 `RLock()`-across-I/O sites in `session/instance_tmux.go` read their precondition
  field(s) via `inst.Snapshot()` before performing tmux I/O.
- Pure-read tmux calls perform I/O outside any actor command entirely.
- Tmux calls that must mutate state route through a command (`sendCtx` with a short
  timeout, e.g. 2s, at sweep-style poller call sites, per `architecture.md` §5) — accepting
  that this specific command may be slow, and that pollers sweeping all instances skip
  (log-and-continue) on `context.DeadlineExceeded` rather than blocking the whole sweep
  (R2.8, `architecture.md` §5).

**Files**: `session/instance_tmux.go`.

#### Task 6.1a: Catalog the 10 `RLock()`-across-I/O sites with current line numbers (~20 min)
- `grep -n "RLock()" session/instance_tmux.go` (confirmed 10 matches as of this plan's
  research pass — re-verify count/line numbers before converting, since Epics 1-5 may have
  touched this file incidentally).

#### Task 6.1b: Convert read-only precondition sites (~1.5 hr, likely the majority of the 10)
- For each: replace `i.stateMutex.RLock(); check; ... I/O ...; RUnlock()` with
  `snap := inst.Snapshot(); check snap.Field; ... I/O ...` (no lock at all).

#### Task 6.1c: Convert any mutate-then-I/O sites to `sendCtx`-routed commands (~1 hr)
- For sites that both check a precondition and need to record the outcome as a field
  mutation, route through `sendCtx` with a 2s timeout, log-and-skip on
  `context.DeadlineExceeded` at poller call sites.

#### Task 6.1d: Regression test + manual tmux-session-lifecycle smoke test (~30 min)
- Confirm session creation/pause/resume/deletion still correctly interacts with tmux after
  this conversion.

---

## Epic 7: Final `stateMutex` Deletion + `LiveInstance` Field Unexport

**Architecture ref**: `architecture.md` §7 PR9; R2.6. **Risk**: low by design — the
compiler is the audit mechanism.

**Also resolves `type-driven-audit.md` finding A** (the highest-severity finding of that
audit): Story 7.1's field-unexport task (7.1e) closes the gap where "the actor is the sole
mutator" was, through Epics 1-6, a comment claim rather than a compiler-enforced one — every
`LiveInstance` field stayed directly assignable from any package until this epic.

### Story 7.1: Delete the mutex field, unexport `LiveInstance`'s mutable fields, and fix all resulting compile errors

**As a** developer completing the migration, **I want** `stateMutex` removed from
`Instance` entirely, **so that** any remaining direct-lock call site becomes a compile
error instead of a silent gap in a manual 75-site audit.

**Acceptance Criteria**:
- `i.stateMutex` field declaration (`session/instance.go:332`) is deleted.
- Every resulting compile error (`i.stateMutex.Lock()`/`RLock()`/etc. with no such field) is
  fixed by removing the now-dead lock/unlock calls (the surrounding logic should already be
  command-routed by Epic 4/5/6 — this is cleanup, not new logic).
- No parallel locking scheme is left behind anywhere in `session/instance*.go` (R2.6). This
  explicitly includes `startMu` (`session/instance.go:335`) and `restartMu`
  (`session/instance.go:347`) — two separate `deadlock.Mutex` fields, distinct from
  `stateMutex`, that guard `start()`/restart-rate-tracking. Deleting `stateMutex` alone
  produces no compile error for code still using `startMu`/`restartMu`, so R2.6 could be
  silently violated without the compiler catching it. Per Task 4.2a's analysis, either remove
  `startMu`/`restartMu` here (if confirmed redundant under the actor's per-Instance
  serialization) or explicitly document, in a comment on the field declarations, why they must
  be retained and how they interact with the actor.
- `github.com/linkdata/deadlock` import is removed from `session/instance.go` specifically
  if no longer used in that file — **note**: this package is used elsewhere in the
  codebase (`session/pr_status_poller.go`, `session/review_queue_poller.go`,
  `session/health.go`, `session/tmux/tmux.go`, and others per a repo-wide grep) for
  unrelated locks, so do **not** remove the `go.mod` dependency itself, only the
  now-unused import in files this migration touched.
- **Unexport `LiveInstance`'s mutable state fields (closes `type-driven-audit.md` finding A —
  the biggest gap this audit found: without this, "the actor is the sole mutator" is a comment
  claim, not a compiler-enforced one).** Story 2.5.2's rename gated only the *constructor*
  surface (making `Registry.Acquire`/`Register` the only way to obtain a live handle); it never
  addressed field *visibility* — every mutable field (`Status`, `Title`, `AutoYes`,
  `GitHubPRURL`, `GitHubPRNumber`, `AutonomousTurn`, etc., ~90 fields) stays a
  directly-addressable exported identifier through Epics 1-6, so `liveInst.AutoYes = true`
  still compiles from any package, any time, even after every writer this plan knows about has
  been converted to `send`/`sendSync`. Unexport these fields (`status`, `title`, `autoYes`,
  `gitHubPRURL`, `gitHubPRNumber`, …), keeping `Snapshot()` (unchanged) as the read path and
  routing every write through the already-unexported, already-package-scoped `instanceState`
  wrapper the `xxxLocked` functions already operate on — **no new mechanism required**, this
  wrapper already exists in the design (Story 3.1's `instanceState{inst *Instance}`):
  ```go
  // Before — compiles today, and still compiles after Epics 1-6 land:
  type LiveInstance struct {
      Status, Title, AutoYes, GitHubPRURL, GitHubPRNumber ... // ~90 exported fields
      mailbox chan command; ctx context.Context; cancel context.CancelFunc
      done chan struct{}; snapshot atomic.Pointer[InstanceSnapshot]
  }
  liveInst.AutoYes = true // compiles from any package, any time

  // After this task:
  type LiveInstance struct {
      status, title, autoYes, gitHubPRURL, gitHubPRNumber ... // same fields, unexported
      mailbox chan command; ctx context.Context; cancel context.CancelFunc
      done chan struct{}; snapshot atomic.Pointer[InstanceSnapshot]
  }
  func (i *LiveInstance) Snapshot() *InstanceSnapshot { return i.snapshot.Load() } // reads, unchanged

  liveInst.AutoYes = true // compile error: unexported
  liveInst.send("SetAutoYes", func(s *instanceState) { s.inst.autoYes = true }) // only path that compiles
  ```
  This is the natural slot for this change: every writer is already actor-routed by the time
  Epics 4/5 land, so the compiler enumerates every remaining direct-write site the exact same
  way it enumerates dangling `stateMutex.Lock()` calls in Task 7.1b above — including
  `daemon/daemon.go`'s `AutoYes` writes (see the Story 2.5.7i cross-reference below), which
  Story 5.4's file-scoped grep guard cannot see but an unexported field's compile error can't
  miss, by construction. **This is also what makes Story 5.4's CI guard partially redundant
  going forward** — see the note added to Story 5.4's acceptance criteria.

**Files**: `session/instance.go` and any other file with a compile error after the field
deletion (the compiler enumerates these — don't pre-guess the full list); `session/instance*.go`
(field unexport); `daemon/daemon.go` (its `AutoYes` writes become compile errors here too, per
the Story 2.5.7i cross-reference, and must be converted to `send`/`sendSync` calls against its
own `Registry`-acquired `*LiveInstance`s).

#### Task 7.1a: Delete the `stateMutex` field declaration (~5 min)
- `session/instance.go:332`.

#### Task 7.1b: Fix every resulting compile error (~1-2 hr depending on what Epics 1-6 missed)
- `go build ./...`, fix each error by removing dead lock/unlock calls. Any error here is the
  compiler catching something Epics 1-6 should have already converted — treat each one as a
  signal to double check the surrounding logic is genuinely command-routed, not just
  "delete the lock call and hope."

#### Task 7.1c: Remove now-unused `deadlock` import from touched files only (~10 min)
- Confirm via `goimports`/`go build` which files actually lost their only use of the
  package; leave `go.mod`'s dependency in place (used elsewhere per the repo-wide grep
  above).

#### Task 7.1d: Resolve `startMu`/`restartMu` per Task 4.2a's findings (~30 min)
- `session/instance.go:335,347` — apply the remove-or-retain decision from Task 4.2a's
  analysis: if `start()`/`Start()` is confirmed to have no caller outside the actor once Epic 4
  lands, delete both fields and their `Lock()`/`Unlock()` call sites (same cleanup pattern as
  Task 7.1b). If any caller still exists outside the actor, keep the fields and add a comment
  at each declaration explaining the retained-lock rationale and how it interacts with the
  actor's command serialization, so a future reader doesn't mistake this for an oversight.

#### Task 7.1e: Unexport `LiveInstance`'s mutable state fields; route all access through `instanceState` (~2 hr)
- Implements `type-driven-audit.md` finding A. Lowercase every mutable field on `LiveInstance`
  (`Status`→`status`, `Title`→`title`, `AutoYes`→`autoYes`, `GitHubPRURL`→`gitHubPRURL`,
  `GitHubPRNumber`→`gitHubPRNumber`, and the rest of the ~90-field set — enumerate via
  `go build ./...`'s resulting errors, the same "compiler enumerates the list" approach Task
  7.1b already uses for `stateMutex`). Keep genuinely immutable-after-construction fields
  (`ID`, `UUID`, `CreatedAt`) exported per R2.1, unaffected by this task.
- `Snapshot()` and any other pure-read exported method are unaffected — they already read via
  `i.snapshot.Load()`, not the raw fields.
- Fix every resulting compile error the same way Task 7.1b does: each direct write becomes a
  `send`/`sendSync` call using the field's existing `xxxLocked` twin (Epic 4/5 should have
  already created one for every field with a real writer; if the compiler finds a field with no
  `xxxLocked` twin yet, that's a signal Epic 4/5's catalog missed a writer — extract one here
  rather than working around the unexport).
- Explicitly confirms `daemon/daemon.go`'s `AutoYes` writes (`daemon.go:43,312` — flagged as a
  non-blocking gap by the fifth-pass adversarial review, §4, since Story 5.4's CI guard never
  scanned `daemon/daemon.go`) are caught here: once `autoYes` is unexported, `daemon.go`'s
  `instance.AutoYes = true` sites become compile errors regardless of any CI guard's file list,
  and must convert to `send`/`sendSync` against `daemon.go`'s own `Registry`-acquired
  `*LiveInstance` (Task 2.5.7i).
- Regression test: `go build ./...` is the test — a successful build after this task is
  affirmative proof no direct-field-write call site (cataloged or not, in `session/`,
  `server/services/`, or `daemon/daemon.go`) survived the migration.

### Story 7.2: Full regression sweep

**Acceptance Criteria**:
- `go test -race ./session/... ./server/services/...` run immediately **before** Task 7.1a
  (mutex still present — confirms Epics 1-6 introduced no regressions) and again **after**
  (mutex gone — confirms removal itself is clean). Both runs documented in the PR.
- Final pprof mutex/block profile capture: contention attributed to `Instance` access has
  dropped to ~0 (no mutex left to contend on) — compare against Epic 2 Story 2.4's
  intermediate capture and the original `requirements.md` baseline.
- Manual UI check repeated one more time: pause/resume, rename, program switch, session
  list/watch stream behave identically to pre-migration (R2.9 final sign-off).
- `make build && make test && make lint` all green.

**Files**: none (verification only).

#### Task 7.2a: Run the before/after race-detector pair (~20 min)

#### Task 7.2b: Final pprof capture and three-way comparison (baseline → post-Epic-2 → post-Epic-7) (~30 min)

#### Task 7.2c: Final manual UI verification pass (~30 min)

---

## Testing Approach (applies across all epics)

Per `pitfalls.md` §6, use these three patterns — never `time.Sleep(N)`-based polling
(consistent with this repo's existing Playwright `waitForTimeout` ban):

1. **Deadlock-by-timeout** (self-deadlock regressions — Epic 3's actor wiring, Epic 4's
   `SwitchWorkspace`/state-machine conversions): run the operation in a goroutine, signal
   over `done := make(chan bool)`, `select { case <-done: ; case <-time.After(1*time.Second):
   t.Fatal(...) }`. Modeled on the existing `TestWireRateLimitCallbacks_NoDeadlock`
   (`session/wire_callbacks_concurrency_test.go:12-35`) and Item 1's regression test.
2. **`require.Eventually`** (async command-applied visibility — most fire-and-forget
   commands across Epics 3-6): poll `inst.Snapshot()` at a tight 1ms interval, matching the
   existing idiom in `server/services/capacity_monitor_test.go:219-221`.
3. **Request-response sync point** (no new pattern needed): commands with a `chan error`/
   `chan T` reply already block the *caller* until applied — use this directly for
   converted RPC-handler call sites instead of reaching for `Eventually`.
4. **`testFlush()` barrier** (Epic 3 Task 3.2e): for fire-and-forget commands in test setup/
   teardown where you need an exact "everything sent so far has landed" point without
   polling.
5. **goleak / `runtime.NumGoroutine()` convergence** (Epic 2.5 Story 2.5.10, reused by Epic 3
   Task 3.1d): actor-goroutine leak detection on `Registry.Acquire`/`release()`/`ForceRelease`,
   not raw construction — the dedup/sibling-leak/acquire-during-teardown properties can only be
   verified through `Registry`, since Epic 2.5 removed every other construction path.

---

## Open Decisions — flag for an ADR or explicit resolution before the dependent epic starts

| # | Decision | Status | Recommendation | Blocks |
|---|---|---|---|---|
| 1 | **Command shape**: closure (`command{name, fn}`) vs. discriminated struct-per-command union | **Resolved.** `../decisions/ADR-028-closure-commands-not-discriminated-union.md` (Accepted) settles this: closures, not a discriminated union — the ~75 existing locked method bodies each already *is* its own behavior, with no second file where that behavior must be independently re-registered, so there is no "missing case" failure mode for an exhaustive switch to guard against (unlike `OmnibarAction`/the session-creation registry, which solve a genuinely different cross-file dispatch problem). | No further action — implement per ADR-028 and `architecture.md` §1.1. | Epic 3 (unblocked) |
| 2 | **Actor stop-signal mechanism**: dedicated `stopCh` vs. reusing a `context.Context` | **Resolved.** `../decisions/ADR-029-actor-shutdown-context-cancelfunc.md` (Accepted, with a one-line addendum added per adversarial-review.md finding 3) settles this: each actor selects on a `context.Context`/`CancelFunc` pair, matching `PRStatusPoller`'s existing idiom exactly (`ctx`/`cancel` fields, a `Stop()`-equivalent that calls `cancel()`, `select` includes `case <-ctx.Done(): return`). ADR-029 also specifies **two** `Stop()` triggers — primary (deletion) and a secondary `shutdownHooks` safety net — both now implemented as tasks (Story 3.1 Tasks 3.1c and 3.1e). **`stopActor()` additionally blocks until the run loop has actually exited** (a `done` channel, per Task 3.1b) — the ADR-029 addendum clarifies this is consistent with, not a divergence from, `PRStatusPoller.Stop()`'s actual implementation (`session/pr_status_poller.go:157-164`, which already blocks via `p.wg.Wait()`, not merely `cancel()`-and-return as ADR-029's own prose summary understated). | No further action — implement per ADR-029 and its addendum. | Epic 3 (unblocked) |
| 3 | **Mailbox buffer size**: `architecture.md` §5 gives a range ("16-32") without picking a number | Resolved by `../decisions/ADR-027-mailbox-buffer-capacity.md` (Accepted) — 32 as the starting default, same rationale below | Pick 32 as a starting default (errs toward absorbing bursts of concurrent RPCs touching one `Instance`); record the choice and the "revisit only if profiling shows it's hot" rationale (matching R2.7's own standard) as a code comment on the `mailbox` field declaration, consistent with ADR-027. | Epic 3 |
| 4 | **Per-poller `sendCtx` timeout** for sweep-style call sites (Epic 5 Story 5.3, Epic 6 Story 6.1) | `architecture.md` §5 suggests "e.g. 2s" as an example, not a confirmed value | Use 2s as the default for `CapacityMonitor`/`ReviewQueuePoller`/`PRStatusPoller` sweep loops; confirm it doesn't exceed each poller's own tick interval (`ReviewQueuePoller` ticks every 2-8s per `requirements.md`) — a 2s per-instance timeout inside a 2s-tick poller could itself cause pile-up under load; consider scaling the timeout down (e.g. 500ms) if profiling during Epic 5/6 shows sweep duration creeping up. No ADR needed. | Epic 5, Epic 6 |
| 5 | **`DeleteSession` vs. other `Registry` holders** (`pitfalls-registry.md` §4's Option 1 vs. Option 2 judgment call) | **Resolved.** `requirements.md` R2.18 picks **Option 1** — force-invalidate: `Registry.ForceRelease` tears down the actor immediately regardless of other holders' refcount; in-flight callers observe a typed error on their next interaction. Implemented in Epic 2.5 Story 2.5.9. | No further action — implement per R2.18 and Epic 2.5 Story 2.5.9. | Epic 2.5 (unblocked), Epic 3 (`send`/`sendSync`'s typed-error-on-stopped-actor requirement) |
| 6 | **Acquire-during-teardown synchronization**: Design A (hold the registry mutex across decrement+zero-check+delete; block `release()` on `Stop()` completion, running outside the lock) vs. Design B (a `tearingDown`/`doneCh` flag `Acquire` checks and retries against) | **Resolved.** `pitfalls-registry.md` §3 recommends Design A as simpler (no extra fields, no retry loop); adopted in Epic 2.5 Story 2.5.3. | No further action — implement per Design A. Revisit only if `stopActor()`/`Stop()` becomes slow enough that blocking every `release()` on it is measurable (per `pitfalls-registry.md` §3's own caveat) — Design B remains on record as the fallback. | Epic 2.5 (unblocked) |
| 7 | **`CreateSession`'s interaction with `Registry`**: does session creation call `Registry.Acquire` (extended to tolerate a "constructing now, not yet in storage" third outcome) or a new `Registry.Register(id, *LiveInstance)` (mirroring `Acquire`'s dedup check, skipping the storage lookup)? | **Resolved.** `requirements.md` R2.18a picks a new sibling method, `Registry.Register(instance *LiveInstance) (ReleaseFunc, error)` (`ReleaseFunc` per Story 2.5.3/`type-driven-audit.md` finding B) — not an extended `Acquire`. Keeps `Acquire` (lookup-existing) and `Register` (construct-new) as two narrow, single-purpose entry points into the same dedup guarantee rather than overloading `Acquire` with a third outcome its other callers would need to special-case. Implemented in Epic 2.5 Task 2.5.7h. | No further action — implement per R2.18a and Task 2.5.7h. | Epic 2.5 (unblocked) |

---

## Task Summary

| Epic | Stories | Tasks | Key Files |
|---|---|---|---|
| 1 — Additive snapshot infra | 3 | 9 | `session/instance_snapshot.go` (new), `session/instance.go`, `session/instance_state.go`, `session/instance_terminal.go`, `session/instance_checkpoint.go`, `session/instance_workspace.go`, `session/instance_hibernate.go`, `session/instance_controller.go`, `session/instance_serialization.go` (`FromInstanceData`), `session/external_discovery.go` (`handleNewSession`), `session/session.go` (`SessionToInstance`) |
| 2 — Unguarded reader migration | 4 | 8 | `server/adapters/instance_adapter.go`, `session/instance_serialization.go`, `server/services/capacity_monitor.go`, `session/review_queue_poller.go`, `session/pr_status_poller.go`, `server/services/connectrpc_websocket.go` |
| 2.5 — `Registry` + `LiveInstance` lifecycle layer (resolves adversarial-review §2/§3 and the fourth-pass §1-§6 findings, implements ADR-031/R2.11-R2.18, supersedes ADR-030's call-site classification) | 10 | 47 | `session/registry.go` (new), `session/registry_test.go` (new), `session/storage.go` (or new `session/instance_data.go`), `session/instance*.go` (rename `Instance`→`LiveInstance`), `session/pr_tracking.go`, `server/dependencies.go`, `server/server.go` (shutdownHooks), `session/review_queue_poller.go`, `daemon/daemon.go` (own `Registry`, not DI-wired — Task 2.5.7i), `server/services/session_service.go` (`DeleteSession`, `HibernateSession`/`ResumeHibernatedSession`, `BatchCreateSessions`, `loadInstancesWithWiring` callers + new `WireInstanceCallbacks` extraction, `CreateSession`), `server/services/workspace_service.go`, `server/services/github_service.go`, `server/services/unfinished_work_service.go`, `server/mcp/tools_discovery.go`, `server/mcp/tools_vcs.go`, `server/mcp/tools_goal.go`, `server/mcp/tools_terminal.go`, `server/mcp/tools_lifecycle.go`, `server/services/session_image_upload_handler.go`, `server/services/backlog_service.go`, `server/services/terminal_websocket.go`, `session/health.go`, `session/hibernation_sweeper.go`, `main.go` |
| 3 — Actor plumbing proof (actor-spawn now gated by Epic 2.5's `Registry`, not per-construction-site wiring) | 2 | 10 | `session/instance_actor.go` (new), `session/instance.go`, `session/registry.go` (`stopActor()` real, blocking body — Task 3.1b/3.1c), `server/server.go` (verification only — shutdownHooks already registered by Epic 2.5), `session/instance_state.go` |
| 4 — State-machine core (atomic unit) | 6 | 16 | `session/instance_state.go`, `session/state_machine.go`, `session/instance_controller.go`, `session/instance_hibernate.go`, `session/instance_checkpoint.go`, `session/instance_workspace.go`, `session/instance_terminal.go`, `session/pr_status_poller.go`, `session/review_queue_poller.go`, `session/instance_claude.go`, `session/instance.go` (exit-callback TOCTOU site) |
| 5 — session_service.go + background writers (atomic unit) | 4 | 12 | `server/services/session_service.go` (incl. `buildTurnCallback`/`onAutonomousDriverComplete`), `session/pr_status_poller.go`, `session/review_queue_poller.go`, `Makefile` |
| 6 — instance_tmux.go RLock-across-I/O | 1 | 4 | `session/instance_tmux.go` |
| 7 — Final stateMutex deletion + `LiveInstance` field unexport (`type-driven-audit.md` finding A) | 2 | 8 | `session/instance.go` (`stateMutex`, `startMu`, `restartMu`), `session/instance*.go` (field unexport, Task 7.1e), `daemon/daemon.go` (`AutoYes` writes converted per Task 7.1e's compile-error enumeration), plus any file flagged by the compiler |

**Totals: 8 Epics (7 numbered + Epic 2.5), 32 Stories, 114 Tasks.**
