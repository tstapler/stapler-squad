# Architecture Review: async-session-creation
**Date**: 2026-08-26
**Verdict**: CONCERNS

**Constitution check**: `docs/adr/ADR-000-architecture-constitution.md` does not exist in this
repo — Constitution-violations section omitted per instructions.

## Summary (re-review of the single previously-blocked item)

- **Blocker (Task 2.2.2c-2's persist call was `storage.SaveInstances`, a silent no-op for any
  instance where `!inst.Started()`): RESOLVED.** Verified against `session/storage.go`,
  read in full at the relevant range: `SaveInstances`/`saveInstancesToRepo`
  (`session/storage.go:277-302`) still begins with `if !inst.Started() { continue }` exactly as
  previously found — that part of the prior finding is unchanged and confirms the bug was real.
  The fix switches every phase-transition persistence call site to `storage.UpdateInstance`
  (`session/storage.go:546-548`):
  ```go
  // UpdateInstance updates an existing instance in storage.
  func (s *Storage) UpdateInstance(instance *Instance) error {
  	return s.repo.Update(context.Background(), instance.ToInstanceData())
  }
  ```
  This calls `s.repo.Update` unconditionally — no `Started()` check, no other precondition, no
  early-return path of any kind. Confirmed correct at every specified site:
  - Domain Glossary's `CreationProgressUpdatedAt` row (`plan.md:28`) now says "persisted via
    `storage.UpdateInstance`... `SaveInstances` is the wrong call here... `UpdateInstance` calls
    `s.repo.Update` directly with no such gate (`session/storage.go:546-548`)."
  - Migration Plan (`plan.md:66-70`) carries the identical correction.
  - Story 1.1.4 (`plan.md:299-308`) and Story 2.2.2 (`plan.md:578-585`) both specify
    `storage.UpdateInstance`, explicitly citing the `Started()` gate as the reason `SaveInstances`
    is wrong.
  - Task 2.2.2c-2 (`plan.md:600-602`) names the exact call — `storage.UpdateInstance(instance)`,
    not `storage.SaveInstances` — at the exact site (immediately after each `SetCreationProgress`
    call in the pipeline, Tasks 2.2.2a/b/c — i.e. before `Started()` becomes true), and cites both
    the wrong method's gate (`session/storage.go:283-301`, now shifted to 277-302 after a
    docstring-line change but still the same code) and the right method's line range.
  - Task 3.2.1b-2 (`plan.md:739-740`) no longer cross-references 4.1.2d's old direct-storage
    construction; it now explicitly scopes itself to only the nil-`CancelFunc` guard and states a
    direct fixture is fine for that narrower purpose.
  This is a genuine, verified fix — not a reword. `UpdateInstance` is already this codebase's
  existing gate-free precedent (`Storage.AddInstance`, used for the Epic 2.1 initial-creation
  persist, is likewise gate-free), so the remediation matches an established pattern rather than
  inventing a new one.

- **Task 4.1.2d's regression test rewrite: genuinely closes the original gap, with one residual
  gap noted as a new Concern below.** The rewritten task (`plan.md:883-885`) explicitly forbids
  constructing the fixture's `CreationProgressUpdatedAt` by hand via storage — calling that out by
  name as "the original round's mistake" — and instead requires: construct via the real
  instance-construction helper (Task 2.1.1a), call the real `instance.SetCreationProgress(...)`,
  then call the real phase-transition persistence call (`storage.UpdateInstance`, Task 2.2.2c-2)
  for a phase *before* the final worktree/tmux phase, then reload from storage into a fresh
  registry and assert the sweeper uses the reloaded timestamp. This would have caught the original
  bug: had the pipeline still called `SaveInstances` pre-`Started()`, the reload in this test would
  read back a zero-value `CreationProgressUpdatedAt` (or the row wouldn't even exist yet), and the
  assertion "sweeper flips it using the reloaded `CreationProgressUpdatedAt`... not `CreatedAt`"
  would fail. One residual risk, captured as a new Concern below: the task's phrasing lets the test
  call `storage.UpdateInstance` *directly* rather than through whatever phase-transition wrapper
  function the pipeline itself will call — if the implementation later centralizes the
  `SetCreationProgress` + persist pairing into a single helper, a test written to satisfy this
  literal task description could still bypass that helper and call the two pieces separately,
  reopening a narrower version of the same "test proves the primitive works, not that the pipeline
  calls it" gap. Not severe enough to block on, since the task's own intent (documented inline) is
  clearly to exercise the real call, but worth tightening during implementation.

- All other previously-flagged concerns/nitpicks were left untouched by this edit round per the
  fix agent's summary and are carried forward unchanged below (line numbers re-checked against the
  current plan.md).

## Blockers

None.

## Concerns

- [ ] **New — Task 2.2.2c-2 / Story 2.2.2 — per-phase-transition `UpdateInstance` calls introduce
  write amplification and synchronous DB round-trips into the pipeline, unaddressed in the plan.**
  Read `session/ent_repository.go:482+`: `EntRepository.Update` opens a full `ent` transaction,
  queries the row by title, and issues a multi-field `UpdateOne(...).SetX(...)` chain across most
  of the schema (status, path, autoflags, program, working dir, branch, dimensions, prompt, etc.)
  before committing — a non-trivial write, not a narrow single-column touch. The fix now calls this
  once per phase transition (5-6 times per session creation, per the Domain Glossary's phase list)
  instead of once at the pipeline's terminal write, and does so *synchronously* inline in the
  pipeline goroutine between phases (the plan's task descriptions show no `go func` / fire-and-
  forget wrapper around the persist call). This is a real change in write volume and per-phase
  latency that neither Task 2.2.2c-2, Story 2.2.2, nor the Domain Glossary row discusses. It's very
  likely fine in practice — NFRs already state this is "single-user-per-instance, low session
  creation volume... not a throughput concern" and the DB is local — but Story 2.2.2's own
  acceptance criterion ("a plain directory session... completes in low-single-digit milliseconds")
  is now directly in tension with "4-5 additional synchronous full-transaction DB writes per
  creation" and nothing in the plan reconciles the two or defines a bound. **Recommendation**:
  either (a) add a line to Story 2.2.2 or Task 2.2.2c-2 explicitly acknowledging the added DB-write
  count is expected to stay within the low-single-digit-ms budget for a local sqlite/ent backend
  (with a note to verify empirically in Phase 6 if the low-single-digit-ms acceptance criterion
  starts failing), or (b) scope the per-phase persist to only the phases where it's actually load-
  bearing (i.e. skip it for a plain directory session's fast in-memory-only phases, since those
  have nothing worth surviving a restart for) — whichever the implementer judges cheaper, but the
  plan should say which.

- [ ] **New (minor) — Task 4.1.2d — test may exercise `storage.UpdateInstance` directly rather
  than through the pipeline's actual phase-transition call site.** See "Summary" above for the
  detail. **Recommendation**: tighten the task's wording (or the implementer's actual test) to call
  whatever single function/method the real pipeline invokes for a phase transition (even if that's
  just "call `SetCreationProgress` then `storage.UpdateInstance` in that exact sequence, sourced
  from the same helper the pipeline itself calls" rather than reimplementing the two calls
  independently in the test), so a future refactor of the pipeline's persistence wiring can't drift
  away from what the test actually proves again.

- [ ] **Domain Glossary / `FailureReason`** — still modeled as a bare `string` end-to-end (Go
  field, `TryForceStatusIfEpoch` parameter, presumably the proto field, and the frontend's
  `getFailureMessage(failureReason: string)` in Task 5.2.2a); unaffected by this edit round.
  **Recommendation** (unchanged): define `FailureReason` as a small typed Go enum, mirroring
  `Status`'s pattern, and add it as a real proto enum in the new RPC messages rather than a free
  string.

- [ ] **Epic 1.2/2.2/1.1.2c/1.1.4 — new flat fields on `Instance`** — still **four** independent
  top-level fields (`creationEpoch`, `FailureReason`, `cancelFunc`, and
  `creationProgressUpdatedAt`) rather than grouped; unaffected by this edit round. This repo
  already has a precedent for grouping a feature's related fields into one nested value
  (`session/instance_snapshot.go:104-109`'s `Autonomous`/`GitHub` groupings).
  **Recommendation** (unchanged): group all four (plus `FailureReason`) into one `CreationState`
  struct field on `Instance`.

- [ ] **Story 3.2.1 (`CancelSessionCreation`) — inconsistent contract for the same logical
  outcome** — unaffected by this edit round; text is identical to the previous review. When cancel
  is called on an already-`Active` session it returns an RPC error; when cancel loses the live race
  against the pipeline's terminal write (Task 3.2.1c) the same logical outcome returns RPC success
  with an ad hoc "already-running" payload. **Recommendation** (unchanged): give
  `CancelSessionCreationResponse` an explicit `outcome` field covering both cases uniformly.

- [ ] **Epic 2.2 / `server/services/session_service.go` file size** — unaffected by this edit round.
  Epic 2.2 (pipeline), 3.1 (`cleanupPartialCreation`), 3.2, and 3.3 (the two new RPC handlers) are
  all still slated for `session_service.go` itself, while Task 1.3.2a already extracts the far
  smaller metrics registration to its own file for exactly this file-size reason.
  **Recommendation** (unchanged): extract Epic 2.2/3.1/3.2/3.3 into a new
  `server/services/session_creation_pipeline.go`.

- [ ] **Story 2.2.3 / Epic 4.1 — no test for "terminal write arrives after the instance's actor has
  been torn down"** — unaffected by this edit round. The plan tests epoch-mismatch-while-the-actor-
  is-still-alive thoroughly but not the case where Cancel has fully removed the instance from the
  live registry before a superseded pipeline goroutine's belated `TryForceStatusIfEpoch` call
  arrives. **Recommendation** (unchanged): add this explicit test to Epic 3.2 or 6.2's race suite.

- [ ] **Epic 3.2/3.3 phase functions as `*SessionService` methods** — unaffected by this edit round;
  still a NITPICK-adjacent concern about testability, not a call for new interfaces, per the plan's
  own well-justified rejection of a bigger architectural change here.

### Resolved since the previous review (no longer concerns)

- ~~Task 2.2.2c-2 vs. `session/storage.go`'s `saveInstancesToRepo` (the persistence-call
  blocker)~~ — **resolved**, see Summary above.
- ~~Story 3.3.1 — double-retry-click semantics left undecided~~ — **resolved** (carried forward
  from the previous review; the current Story 3.3.1 specifies the bump-then-check shape via
  `TryStartRetry`, Story 1.2.3).
- ~~Story 3.3.1 — retry doesn't explicitly invoke the outgoing pipeline's `cancelFunc`~~ —
  **resolved** (carried forward; Task 3.3.1b now explicitly calls the outgoing attempt's
  `cancelFunc()` before `cleanupPartialCreation`).

## Nitpicks

- Story 5.2.1's "distinct warning-glyph icon (not shared with CRASHED's icon, if CRASHED has
  one...)" is still conditional/uncertain in the plan itself — unaffected by this edit round, worth
  a 2-minute check against `SessionCard.tsx`'s current icon set before Phase 5 starts.
- The Domain Glossary's `TryForceStatusIfEpoch` signature still takes `failureReason string`
  (`plan.md:24`) — once the `FailureReason` typed-enum concern above is addressed, this signature
  should be updated to match; unaffected by this edit round.
- Epic 6.1's task list (Task 6.1.1b-h) still folds all 7 session-creation-mode tests into one
  bundled task description — unaffected by this edit round; consider giving each mode its own
  explicit line item for Phase 6 tracking.
</content>
