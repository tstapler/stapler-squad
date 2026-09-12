# Adversarial Review: durable-guidance-requests

**Date**: 2026-09-12
**Verdict**: READY FOR IMPLEMENTATION — 0 blockers, 0 concerns, 5 minors (all
non-blocking, unchanged carry-forwards). This is the fifth pass. Rounds 2, 3,
and 4 each found a new compile-blocking gap in the same seam (Epic 1.7's
`AutonomousDriver`↔storage integration); this round verifies round 4's fixes
against real source AND against an actual `go build`, per round 4's own
recommendation to stop trusting a sixth prose-only pass. Both checks pass.

## Fix Verification (round-4 patch), re-verified against real source

All claims below were checked by reading the current file content directly,
not by trusting the plan's own citations:

- **`GuidanceStorage` interface replaced with concrete `*Storage`.** Task
  1.7.0a now reads "Decision: use the concrete `*Storage` type here too, not
  an interface," with the reasoning (the narrow interface needed a new method
  every round). `AutonomousDriver`'s current struct
  (`session/autonomous_driver.go:71-110`) has no `storage` field yet — the
  plan's diff has not been applied to source (expected, no implementation
  exists), and adding `storage *Storage` alongside `costSink` is a clean,
  unremarkable addition, confirmed by reading the full struct body.
- **`GetItemSessionBySessionUUID`'s real signature matches the plan's every
  citation of it.** `session/storage.go:1195-1198` (exact):
  ```go
  func (s *Storage) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error) {
      return s.repo.GetItemSessionBySessionUUID(ctx, sessionUUID)
  }
  ```
  On a concrete `*Storage` field this is trivially callable from `run()`; no
  interface-narrowing gap remains (the round-4 Blocker is closed by the
  interface-to-concrete-type switch, not by adding a fourth method).
- **`d.sessionUUID` no longer appears as a live reference anywhere in the
  plan** — `grep -n "d\.sessionUUID" plan.md` returns exactly two hits, both
  inside the negation "there is no `d.sessionUUID` field... and none needs to
  be added" (Tasks 1.7.1b and 1.7.2a). Both tasks now explicitly say to reuse
  `sessionID`, the `run()`-local variable confirmed still declared at
  `session/autonomous_driver.go:265` (`sessionID := d.inst.UUID`), in scope
  through the rest of `run()` including the turn loop (lines 312-421) and the
  post-loop exhaustion branch (line 424+) — verified by reading the function
  directly, not just trusting the plan's line-number citations.
- **The blanket `return` is gone; the skip is now targeted and the generic
  notification still fires.** Task 1.7.1d's current text explicitly rejects
  the blanket-return approach ("An earlier draft of this task wrapped...
  `return`, but `return` exits `onAutonomousDriverComplete` entirely, which
  also skips the generic... push-notification block") and instead wraps only
  `UpdateItemSessionEnded`/`AutoRespawnAutonomousWork` in a targeted
  `if !outcome.AwaitingGuidance { ... } else { log.Info(...) }`, plus adds an
  explicit `else if outcome.AwaitingGuidance` branch to the generic
  notification's title/body so it says "paused, waiting for an answer"
  instead of the misleading stuck-session text. Verified against real source
  at both edit locations: the pre-switch guard
  (`autonomous_orchestration_service.go:316-324`, exact — `if !outcome.Done {
  MarkStuck(...) }` today, becomes `if !outcome.Done && !outcome.AwaitingGuidance`),
  the `SessionRoleWork` block (lines 348-426, exact — `UpdateItemSessionEnded`
  at line 379, `AutoRespawnAutonomousWork` dispatch at lines 382-426), and the
  generic notification block (lines 573-607, exact — `if !inst.Hidden {
  a.bus.Publish(...) }`, confirmed this is the *only* fall-through path for
  `SessionRoleWork`, unlike `SessionRoleTriage`/`SessionRoleReview` which
  `return` early with their own explicit comments saying so).
- **The session-level outcome-badge guard is present and at the right,
  distinct location.** `if outcome.Done { inst.AutonomousOutcome = "done" }
  else { inst.AutonomousOutcome = "stuck" }` at real lines 273-277 (exact,
  confirmed) becomes `else if !outcome.AwaitingGuidance`, per Task 1.7.1d's
  text — a separate edit from the pre-switch guard, earlier in execution
  order, matching the plan's own claim that these are two distinct edit
  locations.

## Build Verification (new this round — see prompt's Step 2)

Per round 4's explicit recommendation ("do one literal compile-level dry run
of just its diff... rather than commissioning a fifth prose review round on
faith"), this round did not stop at re-reading prose. A standalone scratch Go
module was built at
`/tmp/claude-1000/-home-tstapler-Programming-stapler-squad/461f6d2c-8662-434e-8e4b-bc787016b7f5/scratchpad/guidance_plan_typecheck/`
(outside the repo; left in place as an evidence trail, not committed anywhere)
containing:

- `session/storage_stub.go` — a stub `*Storage` with `repo *EntRepository`
  (matching the real field name, `session/storage.go:260-262`, exact), plus
  `CreateGuidanceRequest`, `GetGuidanceRequest`, `ListGuidanceRequests`,
  `AnswerGuidanceRequest`, `ExpireGuidanceRequest`, `MarkGuidanceRequestDelivered`,
  `GetItemSessionBySessionUUID` (real signature), `MarkStuck`,
  `UpdateItemSessionEnded` — signatures taken from Epic 1.2's task text and
  the real source, not invented.
- `session/headless_stub.go` — minimal `HeadlessPoolClient`/`Instance` stand-ins.
- `session/autonomous_driver_stub.go` — a stub `AutonomousDriver` with the
  `storage *Storage` field, `WithGuidanceStorage(*Storage) DriverOption`, the
  `AwaitingGuidance bool` outcome field, and a `run()` method implementing
  Task 1.7.1b (create-on-`directiveAskQuestion` + `GetItemSessionBySessionUUID`
  lookup, using `sessionID`, not `d.sessionUUID`), Task 1.7.2a (per-turn
  `ListGuidanceRequests` check), Task 1.7.2b (`MarkGuidanceRequestDelivered`
  call), and Task 1.7.1c's exhaustion-classification branch — written as an
  implementer following the plan text literally, not simplified.
- `services/autonomous_orchestration_service_stub.go` — a stub
  `AutonomousOrchestrationService` implementing Task 1.7.0b's `withGuidanceStorage`
  helper (mirroring `withCostSink`'s exact nil-guard shape) at both real call
  sites (`StartAutonomousDriverForInstance`, `StartAutonomousDriverWithTimeout`),
  and `onAutonomousDriverComplete` implementing all of Task 1.7.1d's guard
  changes: the session-level badge `else if !outcome.AwaitingGuidance`, the
  pre-switch `if !outcome.Done && !outcome.AwaitingGuidance`, the targeted
  `if !outcome.AwaitingGuidance { end+respawn } else { log }` (not a blanket
  return), and the generic notification's `AwaitingGuidance` branch.
- `services/session_creation_pipeline_stub.go` — Task 1.7.0b's third call
  site (`concreteStorage := s.GetStorage()`, reused for both `costOpt` and
  `guidanceOpt`).

Commands run and their literal output:

```
$ cd .../scratchpad/guidance_plan_typecheck && go mod init scratch
go: creating new go.mod: module scratch

$ go build ./...
EXIT: 0

$ go vet ./...
VET EXIT: 0

$ gofmt -l .
services/autonomous_orchestration_service_stub.go
```

`go build ./...` and `go vet ./...` both exited 0 — the plan's described diff,
implemented literally as specified (concrete `*Storage` field, `sessionID`
reuse at both call sites, three `WithGuidanceStorage` call sites, the
targeted-skip guard structure), type-checks with no compile errors. `gofmt
-l` flagged one file for whitespace-only formatting (struct alignment in a
throwaway stub) — not a plan defect, not investigated further since it has no
bearing on whether the described Go is well-typed.

**Caveat, stated plainly**: this exercises type/signature/scope correctness
only (the exact class of bug rounds 2-4 each found) — it does not, and
cannot, verify runtime behavior, business logic correctness, ent-schema
compatibility, or anything the real `*Storage`/`*Instance`/`*EntRepository`
types would additionally constrain that isn't captured in a hand-written
stub. It is evidence the plan's Go *shape* is now internally consistent, not
a substitute for actually implementing and testing it.

## Blockers

None found this round.

## Concerns

None found this round. Both of round 4's Concerns (`d.sessionUUID` non-field,
the blanket-`return`'s silent notification skip) are now fixed in the plan
text itself, verified above against both source and a working build.

## Minors

Carried forward, unchanged, still non-blocking (re-verified this round, no
new minors found):

- Task 1.2.2a's citation "mirroring `BacklogService.GetBacklogItem`'s
  error-mapping convention at `server/services/backlog_service_triage.go:
  2679-2683`" still points at inline code inside `TriggerTriage`, not a
  dedicated RPC handler method, so "convention" overstates what's actually
  there.
- Task 1.3.2b's "pass the guidance request's own ID as (or embedded in) the
  `sessionID` argument" to `Notifier.Notify` still conflates the `item_id`
  metadata key with a value that's neither an item ID nor a session ID.
- ADR-001's "`NotificationService`'s RPCs, `ApprovalRulesPanel`, and
  `GoalPanel` were each added as backend-then-UI-follow-up slices
  historically (see `git log` on those files)" (lines 64-66, re-read this
  round) still asserts this without an actual `git log` citation or output.
- No explicit "ordinary DONE/WAIT/NEXT_MESSAGE-only run is unaffected"
  regression test exists in Story 1.7.3 (Tasks 1.7.3a/b/c cover the
  question-directive path, the cross-restart pickup, and the
  completion-callback skip — none is a plain-old-run-is-unchanged regression
  test), analogous to Task 1.6.3b's equivalent for triage.
- Task 1.1.2d says `*Storage`'s guidance passthrough methods delegate via
  "`s.entRepo.XGuidanceRequest(...)`" — confirmed again this round by reading
  `session/storage.go:260-262`: the real field is `repo`, not `entRepo`
  (`type Storage struct { repo *EntRepository }`). Trivial rename; the
  build-verification stub above used the correct field name to isolate this
  from the shape-correctness check, so it would not have masked a real
  compile error had one existed elsewhere.

## What held up well (re-confirmed this pass)

- Every line-number and signature citation added in round 4's patch — the
  struct fields, `GetItemSessionBySessionUUID`'s signature, the three
  construction call sites, the pre-switch/role-switch/generic-notification
  block boundaries — was independently re-verified against current source
  this round and is accurate.
- The concrete-`*Storage`-over-narrow-interface decision (Task 1.7.0a) holds
  up under an actual build: all three call sites, `run()`'s two storage
  call-sites, and `onAutonomousDriverComplete`'s guard logic all compile
  against one consistent `*Storage` type with zero interface-satisfaction
  friction — the exact problem the narrow interface kept hitting.
- The `AwaitingGuidance`-outcome design (Task 1.7.1c) and the
  keep-session-alive decision (Task 1.7.1d) continue to hold up; nothing in
  this pass found a new problem with either.
- Epic 1.1-1.6 remain internally consistent: `grep -n "d\.sessionUUID\b"` and
  `grep -n "entRepo"` across the whole plan file surfaced no additional
  occurrences beyond the ones already tracked above, and no new cross-epic
  name mismatch was found scanning `GuidanceRequestFilter`/`GuidanceRequestData`
  field usage across Epics 1.1, 1.5, 1.6, and 1.7.

## Final holistic pass: is the plan ready?

**Yes.** After five rounds — one finding a real design gap (round 2), one
finding a missing capability entirely (round 3), one finding a
one-line-interface gap plus two smaller issues (round 4), and this round
finding nothing new in the same seam plus a clean `go build` of the described
diff — the pattern that motivated rounds 2 through 4's caution has run its
course: the seam that produced three consecutive new blockers now produces
zero, under a strictly stronger check (an actual build, not another read).
The five remaining Minors are all pre-existing, small, non-blocking citation/
naming nits with no compile or design impact — none of them would block a
competent implementer, and none has survived un-mentioned; each has been
re-verified as still-present and still-trivial across at least two rounds.

Recommendation: **proceed to implementation.** No sixth prose review is
warranted for Epic 1.7 specifically. Ordinary code review during
implementation (which will naturally run a real build against real types,
not stubs) is sufficient to catch the residual Minors above if they're not
fixed proactively; none rises to a level worth blocking on.
